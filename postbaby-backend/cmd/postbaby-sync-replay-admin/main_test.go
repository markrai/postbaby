package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"postbaby-backend/internal/store"

	_ "modernc.org/sqlite"
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

func TestRunApplyIsNotImplementedAndDoesNotMutateState(t *testing.T) {
	t.Setenv(envEnableInternalSyncReplayCLI, "1")
	t.Setenv(envDeploymentMode, "selfhosted")

	fixture := createReplayPreflightFixture(t, replayPreflightFixtureOptions{})
	before := snapshotReplayAdminState(t, fixture.dbPath, fixture.ownerKey, fixture.appID)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run([]string{
		"apply",
		"--db", fixture.dbPath,
		"--owner-key", fixture.ownerKey,
		"--app-id", fixture.appID,
		"--observation-id", fixture.observationIDString,
	}, stdout, stderr)
	if exitCode != 2 {
		t.Fatalf("expected exit code 2, got %d stderr=%q", exitCode, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("expected empty stdout, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "apply mode is not implemented") {
		t.Fatalf("unexpected stderr: %q", stderr.String())
	}

	after := snapshotReplayAdminState(t, fixture.dbPath, fixture.ownerKey, fixture.appID)
	assertReplayAdminStateEqual(t, before, after)
}

func TestRunPreflightRejectsUnsupportedOutputFormat(t *testing.T) {
	fixture := createReplayPreflightFixture(t, replayPreflightFixtureOptions{})
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run([]string{
		"preflight",
		"--db", fixture.dbPath,
		"--owner-key", fixture.ownerKey,
		"--app-id", fixture.appID,
		"--observation-id", fixture.observationIDString,
		"--output", "yaml",
	}, stdout, stderr)
	if exitCode != 2 {
		t.Fatalf("expected exit code 2, got %d stderr=%q", exitCode, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("expected empty stdout, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "error=unsupported output format: yaml") {
		t.Fatalf("unexpected stderr: %q", stderr.String())
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

func TestRunPreflightFailsForNonexistentDBPath(t *testing.T) {
	t.Setenv(envEnableInternalSyncReplayCLI, "1")
	t.Setenv(envDeploymentMode, "selfhosted")

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	missingPath := filepath.Join(t.TempDir(), "missing.db")

	exitCode := run([]string{
		"preflight",
		"--db", missingPath,
		"--owner-key", "owner",
		"--app-id", "postbaby-web",
		"--observation-id", "42",
	}, stdout, stderr)
	if exitCode == 0 {
		t.Fatalf("expected nonexistent db path to fail")
	}

	result := decodePreflightResult(t, stdout.String())
	if result.Status != preflightStatusInternalError {
		t.Fatalf("unexpected preflight status: %+v", result)
	}
	if !containsString(result.Reasons, "database_file_unavailable") {
		t.Fatalf("expected database_file_unavailable reason, got %+v", result.Reasons)
	}
	if !strings.Contains(stderr.String(), "status="+preflightStatusInternalError) {
		t.Fatalf("unexpected stderr audit line: %q", stderr.String())
	}
}

func TestRunPreflightFailsForInvalidDeploymentMode(t *testing.T) {
	t.Setenv(envEnableInternalSyncReplayCLI, "1")
	t.Setenv(envDeploymentMode, "banana")

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
		t.Fatalf("expected invalid deployment mode to fail")
	}

	result := decodePreflightResult(t, stdout.String())
	if result.Status != preflightStatusInternalError {
		t.Fatalf("unexpected preflight status: %+v", result)
	}
	if !containsString(result.Reasons, "invalid_deployment_mode") {
		t.Fatalf("expected invalid_deployment_mode reason, got %+v", result.Reasons)
	}
}

func TestRunPreflightAllowsBlankDeploymentMode(t *testing.T) {
	t.Setenv(envEnableInternalSyncReplayCLI, "1")
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
	if exitCode != 0 {
		t.Fatalf("expected blank deployment mode to behave like local/static, got exit code %d stderr=%q", exitCode, stderr.String())
	}

	result := decodePreflightResult(t, stdout.String())
	if result.Status != preflightStatusSafe {
		t.Fatalf("unexpected preflight status: %+v", result)
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

func TestRunPreflightRefusesCloudMultiUserAlias(t *testing.T) {
	t.Setenv(envEnableInternalSyncReplayCLI, "1")
	t.Setenv(envDeploymentMode, cloudMultiUserAlias)

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
		t.Fatalf("expected cloud_multi_user alias to be refused")
	}

	result := decodePreflightResult(t, stdout.String())
	if result.Status != preflightStatusRefusedCloud {
		t.Fatalf("unexpected preflight status: %+v", result)
	}
}

func TestRunPreflightJSONSafeAndDoesNotMutateState(t *testing.T) {
	t.Setenv(envEnableInternalSyncReplayCLI, "1")
	t.Setenv(envDeploymentMode, "selfhosted")

	fixture := createReplayPreflightFixture(t, replayPreflightFixtureOptions{})
	beforeState := snapshotReplayAdminState(t, fixture.dbPath, fixture.ownerKey, fixture.appID)
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

	afterState := snapshotReplayAdminState(t, fixture.dbPath, fixture.ownerKey, fixture.appID)
	assertReplayAdminStateEqual(t, beforeState, afterState)
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
	beforeState := snapshotReplayAdminState(t, fixture.dbPath, fixture.ownerKey, fixture.appID)
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

	afterState := snapshotReplayAdminState(t, fixture.dbPath, fixture.ownerKey, fixture.appID)
	assertReplayAdminStateEqual(t, beforeState, afterState)
}

func TestRunPreflightJSONIdempotentAlreadyApplied(t *testing.T) {
	t.Setenv(envEnableInternalSyncReplayCLI, "1")
	t.Setenv(envDeploymentMode, "selfhosted")

	fixture := createReplayPreflightFixture(t, replayPreflightFixtureOptions{
		applicationScenario: replayApplicationScenarioAllMatched,
	})
	beforeState := snapshotReplayAdminState(t, fixture.dbPath, fixture.ownerKey, fixture.appID)
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
		t.Fatalf("expected idempotent already-applied preflight to succeed, got exit code %d stderr=%q", exitCode, stderr.String())
	}

	result := decodePreflightResult(t, stdout.String())
	if result.Status != preflightStatusIdempotent ||
		result.CompareAndApplyStatus != store.SyncMutationReplayCompareAndApplyStatusAlreadyApplied ||
		result.RecoveryStatus != store.SyncMutationReplayRecoveryStatusAlreadyAppliedRequiresIdempotentExit {
		t.Fatalf("unexpected already-applied result: %+v", result)
	}
	if result.MatchingApplicationRowCount != len(fixture.mutationIDs) {
		t.Fatalf("expected %d matching rows, got %+v", len(fixture.mutationIDs), result)
	}

	afterState := snapshotReplayAdminState(t, fixture.dbPath, fixture.ownerKey, fixture.appID)
	assertReplayAdminStateEqual(t, beforeState, afterState)
}

func TestRunPreflightJSONPartialApplicationRows(t *testing.T) {
	t.Setenv(envEnableInternalSyncReplayCLI, "1")
	t.Setenv(envDeploymentMode, "selfhosted")

	fixture := createReplayPreflightFixture(t, replayPreflightFixtureOptions{
		receiptCount:        2,
		applicationScenario: replayApplicationScenarioPartial,
	})
	beforeState := snapshotReplayAdminState(t, fixture.dbPath, fixture.ownerKey, fixture.appID)
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
		t.Fatalf("expected partial application rows preflight to fail")
	}

	result := decodePreflightResult(t, stdout.String())
	if result.Status != preflightStatusPartialRows ||
		result.CompareAndApplyStatus != store.SyncMutationReplayCompareAndApplyStatusAlreadyApplied ||
		result.RecoveryStatus != store.SyncMutationReplayRecoveryStatusPartialApplicationRows {
		t.Fatalf("unexpected partial application rows result: %+v", result)
	}
	if result.MatchingApplicationRowCount != 1 {
		t.Fatalf("expected one matching row, got %+v", result)
	}

	afterState := snapshotReplayAdminState(t, fixture.dbPath, fixture.ownerKey, fixture.appID)
	assertReplayAdminStateEqual(t, beforeState, afterState)
}

func TestRunPreflightJSONApplicationRowsWithoutMatchingCanonicalState(t *testing.T) {
	t.Setenv(envEnableInternalSyncReplayCLI, "1")
	t.Setenv(envDeploymentMode, "selfhosted")

	fixture := createReplayPreflightFixture(t, replayPreflightFixtureOptions{
		applicationScenario: replayApplicationScenarioMismatchedCanonical,
	})
	beforeState := snapshotReplayAdminState(t, fixture.dbPath, fixture.ownerKey, fixture.appID)
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
		t.Fatalf("expected mismatched application rows preflight to fail")
	}

	result := decodePreflightResult(t, stdout.String())
	if result.Status != preflightStatusRowsMismatch ||
		result.CompareAndApplyStatus != store.SyncMutationReplayCompareAndApplyStatusAlreadyApplied ||
		result.RecoveryStatus != store.SyncMutationReplayRecoveryStatusApplicationRowsWithoutMatchingCanonicalState {
		t.Fatalf("unexpected mismatched application rows result: %+v", result)
	}

	afterState := snapshotReplayAdminState(t, fixture.dbPath, fixture.ownerKey, fixture.appID)
	assertReplayAdminStateEqual(t, beforeState, afterState)
}

func TestRunPreflightJSONCanonicalStateWithoutApplicationRows(t *testing.T) {
	t.Setenv(envEnableInternalSyncReplayCLI, "1")
	t.Setenv(envDeploymentMode, "selfhosted")

	fixture := createReplayPreflightFixture(t, replayPreflightFixtureOptions{
		mutateCanonicalToPreviewWithoutRows: true,
	})
	beforeState := snapshotReplayAdminState(t, fixture.dbPath, fixture.ownerKey, fixture.appID)
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
		t.Fatalf("expected canonical-state-without-rows preflight to fail")
	}

	result := decodePreflightResult(t, stdout.String())
	if result.Status != preflightStatusCanonicalMismatch ||
		result.RecoveryStatus != store.SyncMutationReplayRecoveryStatusCanonicalStateWithoutApplicationRows {
		t.Fatalf("unexpected canonical-state-without-rows result: %+v", result)
	}

	afterState := snapshotReplayAdminState(t, fixture.dbPath, fixture.ownerKey, fixture.appID)
	assertReplayAdminStateEqual(t, beforeState, afterState)
}

func TestRunPreflightJSONInvalidObservationScope(t *testing.T) {
	t.Setenv(envEnableInternalSyncReplayCLI, "1")
	t.Setenv(envDeploymentMode, "selfhosted")

	fixture := createReplayPreflightFixture(t, replayPreflightFixtureOptions{})
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run([]string{
		"preflight",
		"--db", fixture.dbPath,
		"--owner-key", "other-owner",
		"--app-id", fixture.appID,
		"--observation-id", fixture.observationIDString,
	}, stdout, stderr)
	if exitCode == 0 {
		t.Fatalf("expected invalid observation scope to fail")
	}

	result := decodePreflightResult(t, stdout.String())
	if result.Status != preflightStatusInvalidScope ||
		result.CompareAndApplyStatus != store.SyncMutationReplayCompareAndApplyStatusInvalidObservationScope ||
		result.RecoveryStatus != store.SyncMutationReplayRecoveryStatusInvalidObservationScope {
		t.Fatalf("unexpected invalid observation scope result: %+v", result)
	}
}

func TestRunPreflightJSONMissingObservation(t *testing.T) {
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
		"--observation-id", strconv.FormatInt(fixture.observationID+999999, 10),
	}, stdout, stderr)
	if exitCode == 0 {
		t.Fatalf("expected missing observation to fail")
	}

	result := decodePreflightResult(t, stdout.String())
	if result.Status != preflightStatusMissingObservation ||
		result.CompareAndApplyStatus != store.SyncMutationReplayCompareAndApplyStatusMissingObservation ||
		result.RecoveryStatus != store.SyncMutationReplayRecoveryStatusMissingObservation {
		t.Fatalf("unexpected missing observation result: %+v", result)
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
	duplicateItemIDs                    bool
	mutateCanonicalAfterObservation     bool
	mutateCanonicalToPreviewWithoutRows bool
	receiptCount                        int
	applicationScenario                 replayApplicationScenario
}

type replayPreflightFixture struct {
	dbPath              string
	ownerKey            string
	appID               string
	observationID       int64
	observationIDString string
	mutationIDs         []string
}

type replayApplicationScenario string

const (
	replayApplicationScenarioNone                replayApplicationScenario = ""
	replayApplicationScenarioAllMatched          replayApplicationScenario = "all_matched"
	replayApplicationScenarioPartial             replayApplicationScenario = "partial"
	replayApplicationScenarioMismatchedCanonical replayApplicationScenario = "mismatched_canonical"
)

type replayPreflightStateSnapshot struct {
	DocumentVersion int64
	DocumentBody    string
	Applications    []store.SyncMutationReplayApplication
	Receipts        []replayReceiptStatusRow
}

type replayReceiptStatusRow struct {
	MutationID string
	Status     string
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
	receiptCount := options.receiptCount
	if receiptCount <= 0 {
		receiptCount = 1
	}
	body := buildReplaySnapshotBodyForCommand(t, options.duplicateItemIDs)
	before, err := sqliteStore.PutDocument(context.Background(), ownerKey, appID, body, nil)
	if err != nil {
		t.Fatalf("seed document: %v", err)
	}

	mutationIDs, receiptInputs := buildReplayReceiptInputsForCommand(receiptCount)
	if _, err := sqliteStore.AcceptSyncMutationReceipts(context.Background(), ownerKey, appID, receiptInputs); err != nil {
		t.Fatalf("accept replay receipt: %v", err)
	}
	observation, err := sqliteStore.RecordSyncMutationReplayDryRunObservation(context.Background(), ownerKey, appID)
	if err != nil {
		t.Fatalf("record observation: %v", err)
	}

	if err := seedReplayApplicationsForCommandScenario(t, sqliteStore, ownerKey, appID, before, observation.ID, mutationIDs, options.applicationScenario); err != nil {
		t.Fatalf("seed replay applications: %v", err)
	}

	if options.mutateCanonicalAfterObservation {
		expectedVersion := before.Version
		if _, err := sqliteStore.PutDocument(context.Background(), ownerKey, appID, buildReplaySnapshotBodyForUpdatedCommand(t), &expectedVersion); err != nil {
			t.Fatalf("mutate canonical document after observation: %v", err)
		}
	}

	if options.mutateCanonicalToPreviewWithoutRows {
		dryRun, err := sqliteStore.ReplaySyncMutationReceiptsDryRun(context.Background(), ownerKey, appID)
		if err != nil {
			t.Fatalf("build dry-run preview: %v", err)
		}
		currentDoc, err := sqliteStore.GetDocument(context.Background(), ownerKey, appID)
		if err != nil {
			t.Fatalf("load current document before preview body update: %v", err)
		}
		expectedVersion := currentDoc.Version
		if _, err := sqliteStore.PutDocument(context.Background(), ownerKey, appID, dryRun.PreviewBody, &expectedVersion); err != nil {
			t.Fatalf("set canonical document to preview body: %v", err)
		}
	}

	return replayPreflightFixture{
		dbPath:              dbPath,
		ownerKey:            ownerKey,
		appID:               appID,
		observationID:       observation.ID,
		observationIDString: strconv.FormatInt(observation.ID, 10),
		mutationIDs:         mutationIDs,
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

func snapshotReplayAdminState(t *testing.T, dbPath, ownerKey, appID string) replayPreflightStateSnapshot {
	t.Helper()

	sqliteStore := openReplayAdminTestStore(t, dbPath)
	doc, err := sqliteStore.GetDocument(context.Background(), ownerKey, appID)
	if err != nil {
		t.Fatalf("load document snapshot: %v", err)
	}
	applications, err := sqliteStore.ListSyncMutationReplayApplications(context.Background(), ownerKey, appID)
	if err != nil {
		t.Fatalf("list replay applications snapshot: %v", err)
	}
	receipts := loadReplayReceiptStatusRowsForCommand(t, dbPath, ownerKey, appID)

	return replayPreflightStateSnapshot{
		DocumentVersion: doc.Version,
		DocumentBody:    string(doc.Body),
		Applications:    applications,
		Receipts:        receipts,
	}
}

func assertReplayAdminStateEqual(t *testing.T, before, after replayPreflightStateSnapshot) {
	t.Helper()

	if before.DocumentVersion != after.DocumentVersion || before.DocumentBody != after.DocumentBody {
		t.Fatalf("document changed during preflight\nbefore=%+v\nafter=%+v", before, after)
	}
	if !reflect.DeepEqual(before.Applications, after.Applications) {
		t.Fatalf("application rows changed during preflight\nbefore=%+v\nafter=%+v", before.Applications, after.Applications)
	}
	if !reflect.DeepEqual(before.Receipts, after.Receipts) {
		t.Fatalf("receipt rows changed during preflight\nbefore=%+v\nafter=%+v", before.Receipts, after.Receipts)
	}
}

func loadReplayReceiptStatusRowsForCommand(t *testing.T, dbPath, ownerKey, appID string) []replayReceiptStatusRow {
	t.Helper()

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open raw sqlite connection: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Fatalf("close raw sqlite connection: %v", err)
		}
	}()

	rows, err := db.Query(
		`SELECT mutation_id, status
		FROM sync_mutation_receipts
		WHERE owner_key = ? AND app_id = ?
		ORDER BY mutation_id ASC`,
		ownerKey,
		appID,
	)
	if err != nil {
		t.Fatalf("query replay receipt rows: %v", err)
	}
	defer rows.Close()

	result := make([]replayReceiptStatusRow, 0)
	for rows.Next() {
		var row replayReceiptStatusRow
		if err := rows.Scan(&row.MutationID, &row.Status); err != nil {
			t.Fatalf("scan replay receipt row: %v", err)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate replay receipt rows: %v", err)
	}

	return result
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

func buildReplayReceiptInputsForCommand(receiptCount int) ([]string, []store.SyncMutationReceiptInput) {
	mutationIDs := make([]string, 0, receiptCount)
	inputs := make([]store.SyncMutationReceiptInput, 0, receiptCount)
	for i := 0; i < receiptCount; i++ {
		mutationID := "mut-create"
		entityID := "item-3"
		if i > 0 {
			mutationID = "mut-create-" + strconv.Itoa(i+1)
			entityID = "item-" + strconv.Itoa(i+3)
		}
		payload := `{"tabId":"tab-1","name":"Third","position":{"top":"20px","left":"20px"}}`
		if i > 0 {
			top := 20 + (i * 10)
			left := 20 + (i * 10)
			payload = `{"tabId":"tab-1","name":"Third ` + strconv.Itoa(i+1) + `","position":{"top":"` + strconv.Itoa(top) + `px","left":"` + strconv.Itoa(left) + `px"}}`
		}
		mutationIDs = append(mutationIDs, mutationID)
		inputs = append(inputs, buildReplayReceiptInputForCommand(mutationID, "Node", entityID, "CreateNode", payload))
	}
	return mutationIDs, inputs
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

func seedReplayApplicationsForCommandScenario(t *testing.T, sqliteStore *store.Store, ownerKey, appID string, before store.Document, observationID int64, mutationIDs []string, scenario replayApplicationScenario) error {
	t.Helper()

	ctx := context.Background()
	hashBefore := hashReplayBytesForCommand(before.Body)
	switch scenario {
	case replayApplicationScenarioNone:
		return nil
	case replayApplicationScenarioAllMatched:
		for _, mutationID := range mutationIDs {
			if _, err := sqliteStore.RecordSyncMutationReplayApplicationInert(ctx, ownerKey, appID, SyncMutationReplayApplicationInputForCommand(mutationID, observationID, before.Version, hashBefore, nil, nil)); err != nil {
				return err
			}
		}
		return nil
	case replayApplicationScenarioPartial:
		if len(mutationIDs) == 0 {
			return nil
		}
		_, err := sqliteStore.RecordSyncMutationReplayApplicationInert(ctx, ownerKey, appID, SyncMutationReplayApplicationInputForCommand(mutationIDs[0], observationID, before.Version, hashBefore, nil, nil))
		return err
	case replayApplicationScenarioMismatchedCanonical:
		if len(mutationIDs) == 0 {
			return nil
		}
		mismatchedVersionAfter := before.Version + 9
		mismatchedHashAfter := "definitely-not-current-hash"
		_, err := sqliteStore.RecordSyncMutationReplayApplicationInert(ctx, ownerKey, appID, SyncMutationReplayApplicationInputForCommand(mutationIDs[0], observationID, before.Version, hashBefore, &mismatchedVersionAfter, &mismatchedHashAfter))
		return err
	default:
		t.Fatalf("unsupported replay application scenario: %s", scenario)
		return nil
	}
}

func SyncMutationReplayApplicationInputForCommand(mutationID string, observationID, versionBefore int64, hashBefore string, versionAfter *int64, hashAfter *string) store.SyncMutationReplayApplicationInput {
	return store.SyncMutationReplayApplicationInput{
		MutationID:                     mutationID,
		ApplicationStatus:              store.SyncMutationReplayApplicationStatusApplied,
		ApplicationReason:              "policy_allowed",
		CanonicalDocumentVersionBefore: versionBefore,
		CanonicalDocumentHashBefore:    hashBefore,
		CanonicalDocumentVersionAfter:  versionAfter,
		CanonicalDocumentHashAfter:     hashAfter,
		ReplayObservationID:            &observationID,
	}
}

func hashReplayBytesForCommand(body json.RawMessage) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func decodePreflightResult(t *testing.T, output string) preflightResult {
	t.Helper()

	var result preflightResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("decode preflight result %q: %v", output, err)
	}
	return result
}
