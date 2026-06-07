package main

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"postbaby-backend/internal/store"
)

func TestRunPreflightRejectsMissingRequiredFlags(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run([]string{"preflight"}, stdout, stderr)
	if exitCode != 2 {
		t.Fatalf("expected exit code 2, got %d stderr=%q", exitCode, stderr.String())
	}
	if !strings.Contains(stderr.String(), "error=missing required flag: --db") {
		t.Fatalf("unexpected stderr: %q", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("expected empty stdout, got %q", stdout.String())
	}
}

func TestRunPreflightRefusesWhenEnvGateDisabled(t *testing.T) {
	t.Setenv(envEnableInternalSyncReplayCLI, "")
	t.Setenv(envDeploymentMode, "")

	fixture := createReplayPreflightFixture(t, replayPreflightFixtureOptions{})
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run([]string{
		"preflight",
		"--db", fixture.dbPath,
		"--owner-key", fixture.ownerKey,
		"--app-id", fixture.appID,
		"--observation-id", fixture.observationIDString,
	}, stdout, stderr)
	if exitCode == 0 {
		t.Fatalf("expected nonzero exit code when env gate is disabled")
	}

	result := decodePreflightResult(t, stdout.String())
	if result.Status != preflightStatusRefusedDisabled {
		t.Fatalf("unexpected preflight status: %+v", result)
	}
	if !strings.Contains(stderr.String(), "mode=preflight") || !strings.Contains(stderr.String(), "status="+preflightStatusRefusedDisabled) {
		t.Fatalf("unexpected stderr audit line: %q", stderr.String())
	}
}

func TestRunPreflightRefusesInCloudMode(t *testing.T) {
	t.Setenv(envEnableInternalSyncReplayCLI, "1")
	t.Setenv(envDeploymentMode, "cloud")

	fixture := createReplayPreflightFixture(t, replayPreflightFixtureOptions{})
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run([]string{
		"preflight",
		"--db", fixture.dbPath,
		"--owner-key", fixture.ownerKey,
		"--app-id", fixture.appID,
		"--observation-id", fixture.observationIDString,
	}, stdout, stderr)
	if exitCode == 0 {
		t.Fatalf("expected nonzero exit code in cloud deployment")
	}

	result := decodePreflightResult(t, stdout.String())
	if result.Status != preflightStatusRefusedCloud {
		t.Fatalf("unexpected preflight status: %+v", result)
	}
	if !strings.Contains(stderr.String(), "status="+preflightStatusRefusedCloud) {
		t.Fatalf("unexpected stderr audit line: %q", stderr.String())
	}
}

func TestRunPreflightJSONSafeAndDoesNotMutateState(t *testing.T) {
	t.Setenv(envEnableInternalSyncReplayCLI, "1")
	t.Setenv(envDeploymentMode, "selfhosted")

	fixture := createReplayPreflightFixture(t, replayPreflightFixtureOptions{})
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run([]string{
		"preflight",
		"--db", fixture.dbPath,
		"--owner-key", fixture.ownerKey,
		"--app-id", fixture.appID,
		"--observation-id", fixture.observationIDString,
	}, stdout, stderr)
	if exitCode != 0 {
		t.Fatalf("expected success, got exit code %d stderr=%q", exitCode, stderr.String())
	}

	result := decodePreflightResult(t, stdout.String())
	if result.Mode != commandModePreflight ||
		result.Status != preflightStatusSafe ||
		result.CompareAndApplyStatus != store.SyncMutationReplayCompareAndApplyStatusAllowed ||
		result.RecoveryStatus != store.SyncMutationReplayRecoveryStatusSafeToAttemptTransaction {
		t.Fatalf("unexpected safe preflight result: %+v", result)
	}
	if result.CanonicalStateChanged || result.DocumentVersionAdvanced || result.ApplicationRowsInserted {
		t.Fatalf("expected preflight to report no mutation, got %+v", result)
	}
	if result.MatchingApplicationRowCount != 0 || len(result.AppliedMutationIDs) != 0 {
		t.Fatalf("expected no application progress, got %+v", result)
	}
	if !strings.Contains(stderr.String(), "mode=preflight") || !strings.Contains(stderr.String(), "status="+preflightStatusSafe) {
		t.Fatalf("unexpected stderr audit line: %q", stderr.String())
	}

	sqliteStore := openReplayAdminTestStore(t, fixture.dbPath)
	after, err := sqliteStore.GetDocument(context.Background(), fixture.ownerKey, fixture.appID)
	if err != nil {
		t.Fatalf("load document after preflight: %v", err)
	}
	if after.Version != fixture.before.Version || string(after.Body) != string(fixture.before.Body) {
		t.Fatalf("expected preflight to preserve document, before=%+v after=%+v", fixture.before, after)
	}
	applications, err := sqliteStore.ListSyncMutationReplayApplications(context.Background(), fixture.ownerKey, fixture.appID)
	if err != nil {
		t.Fatalf("list applications after preflight: %v", err)
	}
	if len(applications) != 0 {
		t.Fatalf("expected no replay application rows, got %+v", applications)
	}
}

func TestRunPreflightJSONStaleCanonicalDocument(t *testing.T) {
	t.Setenv(envEnableInternalSyncReplayCLI, "1")
	t.Setenv(envDeploymentMode, "selfhosted")

	fixture := createReplayPreflightFixture(t, replayPreflightFixtureOptions{mutateCanonicalAfterObservation: true})
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run([]string{
		"preflight",
		"--db", fixture.dbPath,
		"--owner-key", fixture.ownerKey,
		"--app-id", fixture.appID,
		"--observation-id", fixture.observationIDString,
	}, stdout, stderr)
	if exitCode == 0 {
		t.Fatalf("expected stale canonical preflight to fail")
	}

	result := decodePreflightResult(t, stdout.String())
	if result.Status != preflightStatusStaleCanonical ||
		result.CompareAndApplyStatus != store.SyncMutationReplayCompareAndApplyStatusStaleCanonicalDocument ||
		result.RecoveryStatus != store.SyncMutationReplayRecoveryStatusStaleObservationRequiresRedryrun {
		t.Fatalf("unexpected stale canonical result: %+v", result)
	}
}

func TestRunPreflightJSONBlockedSnapshot(t *testing.T) {
	t.Setenv(envEnableInternalSyncReplayCLI, "1")
	t.Setenv(envDeploymentMode, "selfhosted")

	fixture := createReplayPreflightFixture(t, replayPreflightFixtureOptions{duplicateItemIDs: true})
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run([]string{
		"preflight",
		"--db", fixture.dbPath,
		"--owner-key", fixture.ownerKey,
		"--app-id", fixture.appID,
		"--observation-id", fixture.observationIDString,
	}, stdout, stderr)
	if exitCode == 0 {
		t.Fatalf("expected blocked snapshot preflight to fail")
	}

	result := decodePreflightResult(t, stdout.String())
	if result.Status != preflightStatusBlockedSnapshot ||
		result.CompareAndApplyStatus != store.SyncMutationReplayCompareAndApplyStatusBlockedSnapshot ||
		result.RecoveryStatus != store.SyncMutationReplayRecoveryStatusBlockedSnapshotRequiresCleanup {
		t.Fatalf("unexpected blocked snapshot result: %+v", result)
	}
}

func TestRunPreflightTextOutput(t *testing.T) {
	t.Setenv(envEnableInternalSyncReplayCLI, "1")
	t.Setenv(envDeploymentMode, "selfhosted")

	fixture := createReplayPreflightFixture(t, replayPreflightFixtureOptions{})
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run([]string{
		"preflight",
		"--db", fixture.dbPath,
		"--owner-key", fixture.ownerKey,
		"--app-id", fixture.appID,
		"--observation-id", fixture.observationIDString,
		"--output", "text",
		"--verbose",
	}, stdout, stderr)
	if exitCode != 0 {
		t.Fatalf("expected success, got exit code %d stderr=%q", exitCode, stderr.String())
	}

	text := stdout.String()
	if !strings.Contains(text, "status="+preflightStatusSafe) ||
		!strings.Contains(text, "owner_key="+fixture.ownerKey) ||
		!strings.Contains(text, "app_id="+fixture.appID) ||
		!strings.Contains(text, "observation_id="+fixture.observationIDString) {
		t.Fatalf("unexpected text output: %q", text)
	}
}

type replayPreflightFixtureOptions struct {
	duplicateItemIDs                bool
	mutateCanonicalAfterObservation bool
}

type replayPreflightFixture struct {
	dbPath              string
	ownerKey            string
	appID               string
	observationID       int64
	observationIDString string
	before              store.Document
}

func createReplayPreflightFixture(t *testing.T, options replayPreflightFixtureOptions) replayPreflightFixture {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "postbaby-sync-replay-admin.db")
	sqliteStore, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() {
		if err := sqliteStore.Close(); err != nil {
			t.Fatalf("close seeded store: %v", err)
		}
	}()

	ownerKey := "owner"
	appID := "postbaby-web"
	body := buildReplaySnapshotBodyForCommand(t, options.duplicateItemIDs)
	before, err := sqliteStore.PutDocument(context.Background(), ownerKey, appID, body, nil)
	if err != nil {
		t.Fatalf("seed document: %v", err)
	}
	if _, err := sqliteStore.AcceptSyncMutationReceipts(context.Background(), ownerKey, appID, []store.SyncMutationReceiptInput{
		buildReplayReceiptInputForCommand("mut-create", "Node", "item-3", "CreateNode", `{"tabId":"tab-1","name":"Third","position":{"top":"20px","left":"20px"}}`),
	}); err != nil {
		t.Fatalf("accept replay receipt: %v", err)
	}
	observation, err := sqliteStore.RecordSyncMutationReplayDryRunObservation(context.Background(), ownerKey, appID)
	if err != nil {
		t.Fatalf("record observation: %v", err)
	}

	if options.mutateCanonicalAfterObservation {
		expectedVersion := before.Version
		if _, err := sqliteStore.PutDocument(context.Background(), ownerKey, appID, buildReplaySnapshotBodyForUpdatedCommand(t), &expectedVersion); err != nil {
			t.Fatalf("mutate canonical document after observation: %v", err)
		}
	}

	return replayPreflightFixture{
		dbPath:              dbPath,
		ownerKey:            ownerKey,
		appID:               appID,
		observationID:       observation.ID,
		observationIDString: strconv.FormatInt(observation.ID, 10),
		before:              before,
	}
}

func openReplayAdminTestStore(t *testing.T, dbPath string) *store.Store {
	t.Helper()

	sqliteStore, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}

	t.Cleanup(func() {
		if err := sqliteStore.Close(); err != nil {
			t.Fatalf("close store: %v", err)
		}
	})

	return sqliteStore
}

func buildReplaySnapshotBodyForCommand(t *testing.T, duplicateItemIDs bool) json.RawMessage {
	t.Helper()

	tabsJSON := `[{"id":"tab-1","name":"Main","items":[],"colorIndex":0,"gridSetting":"none","edges":[]}]`
	if duplicateItemIDs {
		tabsJSON = `[{"id":"tab-1","name":"Main","items":[{"id":"dup-item","name":"First","position":{"top":"0px","left":"0px"}},{"id":"dup-item","name":"Second","position":{"top":"10px","left":"10px"}}],"colorIndex":0,"gridSetting":"none","edges":[]}]`
	}

	body, err := json.Marshal(map[string]string{
		"tabs":        tabsJSON,
		"activeTabId": "tab-1",
	})
	if err != nil {
		t.Fatalf("marshal replay snapshot body: %v", err)
	}
	return json.RawMessage(body)
}

func buildReplaySnapshotBodyForUpdatedCommand(t *testing.T) json.RawMessage {
	t.Helper()

	body, err := json.Marshal(map[string]string{
		"tabs":        `[{"id":"tab-1","name":"Main","items":[{"id":"item-1","name":"Existing","position":{"top":"5px","left":"5px"}}],"colorIndex":0,"gridSetting":"none","edges":[]}]`,
		"activeTabId": "tab-1",
	})
	if err != nil {
		t.Fatalf("marshal updated replay snapshot body: %v", err)
	}
	return json.RawMessage(body)
}

func buildReplayReceiptInputForCommand(mutationID, entityType, entityID, operationType, payload string) store.SyncMutationReceiptInput {
	baseRevision := int64(6)
	return store.SyncMutationReceiptInput{
		MutationID:    mutationID,
		ClientID:      "client-1",
		DeviceID:      "device-1",
		Protocol:      "PB-SYNC/1",
		EntityType:    entityType,
		EntityID:      entityID,
		OperationType: operationType,
		Payload:       json.RawMessage(payload),
		BaseRevision:  &baseRevision,
	}
}

func decodePreflightResult(t *testing.T, output string) preflightResult {
	t.Helper()

	var result preflightResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("decode preflight result %q: %v", output, err)
	}
	return result
}
