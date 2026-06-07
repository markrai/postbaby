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

func TestRunApplyRefusesWhenEnvGateDisabled(t *testing.T) {
	t.Setenv(envEnableInternalSyncReplayCLI, "")
	t.Setenv(envEnableInternalSyncReplayApply, "1")
	t.Setenv(envDeploymentMode, "selfhosted")

	fixture := createReplayPreflightFixture(t, replayPreflightFixtureOptions{})
	spyHolder := &replayAdminStoreSpy{}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := runWithDeps(makeReplayAdminCommandDeps(t, spyHolder, nil), buildApplyArgs(fixture), stdout, stderr)
	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d stderr=%q", exitCode, stderr.String())
	}

	result := decodeApplyResult(t, stdout.String())
	if result.Status != applyStatusRefusedDisabled {
		t.Fatalf("unexpected apply result: %+v", result)
	}
	if spyHolder.store != nil {
		t.Fatalf("expected apply refusal before store open")
	}
	if !strings.Contains(stderr.String(), "status="+applyStatusRefusedDisabled) {
		t.Fatalf("unexpected stderr audit line: %q", stderr.String())
	}
}

func TestRunApplyRefusesWhenApplyGateDisabled(t *testing.T) {
	t.Setenv(envEnableInternalSyncReplayCLI, "1")
	t.Setenv(envEnableInternalSyncReplayApply, "")
	t.Setenv(envDeploymentMode, "selfhosted")

	fixture := createReplayPreflightFixture(t, replayPreflightFixtureOptions{})
	spyHolder := &replayAdminStoreSpy{}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := runWithDeps(makeReplayAdminCommandDeps(t, spyHolder, nil), buildApplyArgs(fixture), stdout, stderr)
	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d stderr=%q", exitCode, stderr.String())
	}

	result := decodeApplyResult(t, stdout.String())
	if result.Status != applyStatusRefusedApplyDisabled {
		t.Fatalf("unexpected apply result: %+v", result)
	}
	if spyHolder.store != nil {
		t.Fatalf("expected apply refusal before store open")
	}
}

func TestRunApplyRefusesInCloudMode(t *testing.T) {
	t.Setenv(envEnableInternalSyncReplayCLI, "1")
	t.Setenv(envEnableInternalSyncReplayApply, "1")
	t.Setenv(envDeploymentMode, "cloud")

	fixture := createReplayPreflightFixture(t, replayPreflightFixtureOptions{})
	spyHolder := &replayAdminStoreSpy{}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := runWithDeps(makeReplayAdminCommandDeps(t, spyHolder, nil), buildApplyArgs(fixture), stdout, stderr)
	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d stderr=%q", exitCode, stderr.String())
	}

	result := decodeApplyResult(t, stdout.String())
	if result.Status != applyStatusRefusedCloud {
		t.Fatalf("unexpected apply result: %+v", result)
	}
	if spyHolder.store != nil {
		t.Fatalf("expected cloud refusal before store open")
	}
}

func TestRunApplyRefusesCloudMultiUserAlias(t *testing.T) {
	t.Setenv(envEnableInternalSyncReplayCLI, "1")
	t.Setenv(envEnableInternalSyncReplayApply, "1")
	t.Setenv(envDeploymentMode, cloudMultiUserAlias)

	fixture := createReplayPreflightFixture(t, replayPreflightFixtureOptions{})
	spyHolder := &replayAdminStoreSpy{}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := runWithDeps(makeReplayAdminCommandDeps(t, spyHolder, nil), buildApplyArgs(fixture), stdout, stderr)
	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d stderr=%q", exitCode, stderr.String())
	}

	result := decodeApplyResult(t, stdout.String())
	if result.Status != applyStatusRefusedCloud {
		t.Fatalf("unexpected apply result: %+v", result)
	}
	if spyHolder.store != nil {
		t.Fatalf("expected cloud_multi_user refusal before store open")
	}
}

func TestRunApplyRejectsInvalidConfirmation(t *testing.T) {
	t.Setenv(envEnableInternalSyncReplayCLI, "1")
	t.Setenv(envEnableInternalSyncReplayApply, "1")
	t.Setenv(envDeploymentMode, "selfhosted")

	fixture := createReplayPreflightFixture(t, replayPreflightFixtureOptions{})

	for _, testCase := range []struct {
		name           string
		args           func([]string) []string
		expectedReason string
	}{
		{
			name: "missing_danger_flag",
			args: func(args []string) []string {
				return removeBoolFlag(args, "--i-understand-this-mutates-canonical-state")
			},
			expectedReason: "missing_danger_confirmation",
		},
		{
			name: "missing_confirm_owner",
			args: func(args []string) []string {
				return removeFlagValue(args, "--confirm-owner-key")
			},
			expectedReason: "missing_confirm_owner_key",
		},
		{
			name: "mismatched_confirm_owner",
			args: func(args []string) []string {
				return replaceFlagValue(args, "--confirm-owner-key", "other-owner")
			},
			expectedReason: "mismatched_confirm_owner_key",
		},
		{
			name: "missing_confirm_app",
			args: func(args []string) []string {
				return removeFlagValue(args, "--confirm-app-id")
			},
			expectedReason: "missing_confirm_app_id",
		},
		{
			name: "mismatched_confirm_app",
			args: func(args []string) []string {
				return replaceFlagValue(args, "--confirm-app-id", "other-app")
			},
			expectedReason: "mismatched_confirm_app_id",
		},
		{
			name: "missing_confirm_observation",
			args: func(args []string) []string {
				return removeFlagValue(args, "--confirm-observation-id")
			},
			expectedReason: "missing_confirm_observation_id",
		},
		{
			name: "mismatched_confirm_observation",
			args: func(args []string) []string {
				return replaceFlagValue(args, "--confirm-observation-id", "999999")
			},
			expectedReason: "mismatched_confirm_observation_id",
		},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			spyHolder := &replayAdminStoreSpy{}
			beforeState := snapshotReplayAdminState(t, fixture.dbPath, fixture.ownerKey, fixture.appID)
			stdout := &bytes.Buffer{}
			stderr := &bytes.Buffer{}

			exitCode := runWithDeps(makeReplayAdminCommandDeps(t, spyHolder, nil), testCase.args(buildApplyArgs(fixture)), stdout, stderr)
			if exitCode != 2 {
				t.Fatalf("expected exit code 2, got %d stderr=%q", exitCode, stderr.String())
			}

			result := decodeApplyResult(t, stdout.String())
			if result.Status != applyStatusInvalidConfirmation {
				t.Fatalf("unexpected apply result: %+v", result)
			}
			if !containsString(result.Reasons, testCase.expectedReason) {
				t.Fatalf("expected confirmation reason %q, got %+v", testCase.expectedReason, result.Reasons)
			}
			if spyHolder.store != nil {
				t.Fatalf("expected invalid confirmation to refuse before store open")
			}

			afterState := snapshotReplayAdminState(t, fixture.dbPath, fixture.ownerKey, fixture.appID)
			assertReplayAdminStateEqual(t, beforeState, afterState)
		})
	}
}

func TestRunApplyUnsafePreflightBlocksApply(t *testing.T) {
	t.Setenv(envEnableInternalSyncReplayCLI, "1")
	t.Setenv(envEnableInternalSyncReplayApply, "1")
	t.Setenv(envDeploymentMode, "selfhosted")

	fixture := createReplayPreflightFixture(t, replayPreflightFixtureOptions{duplicateItemIDs: true})
	spyHolder := &replayAdminStoreSpy{}
	beforeState := snapshotReplayAdminState(t, fixture.dbPath, fixture.ownerKey, fixture.appID)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := runWithDeps(makeReplayAdminCommandDeps(t, spyHolder, nil), buildApplyArgs(fixture), stdout, stderr)
	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d stderr=%q", exitCode, stderr.String())
	}

	result := decodeApplyResult(t, stdout.String())
	if result.Status != applyStatusBlockedSnapshot || result.PreflightStatus != preflightStatusBlockedSnapshot {
		t.Fatalf("unexpected blocked apply result: %+v", result)
	}
	if spyHolder.store == nil {
		t.Fatalf("expected store to open for same-invocation preflight")
	}
	if spyHolder.store.applyCallCount != 0 {
		t.Fatalf("expected blocked preflight to prevent apply call, got %d", spyHolder.store.applyCallCount)
	}

	afterState := snapshotReplayAdminState(t, fixture.dbPath, fixture.ownerKey, fixture.appID)
	assertReplayAdminStateEqual(t, beforeState, afterState)
}

func TestRunApplyIdempotentAlreadyAppliedExitsZeroWithoutCallingApply(t *testing.T) {
	t.Setenv(envEnableInternalSyncReplayCLI, "1")
	t.Setenv(envEnableInternalSyncReplayApply, "1")
	t.Setenv(envDeploymentMode, "selfhosted")

	fixture := createReplayPreflightFixture(t, replayPreflightFixtureOptions{
		applicationScenario: replayApplicationScenarioAllMatched,
	})
	spyHolder := &replayAdminStoreSpy{}
	beforeState := snapshotReplayAdminState(t, fixture.dbPath, fixture.ownerKey, fixture.appID)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := runWithDeps(makeReplayAdminCommandDeps(t, spyHolder, nil), buildApplyArgs(fixture), stdout, stderr)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d stderr=%q", exitCode, stderr.String())
	}

	result := decodeApplyResult(t, stdout.String())
	if result.Status != applyStatusIdempotent || result.PreflightStatus != preflightStatusIdempotent {
		t.Fatalf("unexpected idempotent apply result: %+v", result)
	}
	if spyHolder.store == nil {
		t.Fatalf("expected store to open for idempotent preflight")
	}
	if spyHolder.store.applyCallCount != 0 {
		t.Fatalf("expected idempotent preflight to skip apply call, got %d", spyHolder.store.applyCallCount)
	}

	afterState := snapshotReplayAdminState(t, fixture.dbPath, fixture.ownerKey, fixture.appID)
	assertReplayAdminStateEqual(t, beforeState, afterState)
}

func TestRunApplyJSONSuccessMutatesDocumentVersionAndApplicationsWithoutMutatingReceiptsOrObservations(t *testing.T) {
	t.Setenv(envEnableInternalSyncReplayCLI, "1")
	t.Setenv(envEnableInternalSyncReplayApply, "1")
	t.Setenv(envDeploymentMode, "selfhosted")

	fixture := createReplayPreflightFixture(t, replayPreflightFixtureOptions{})
	spyHolder := &replayAdminStoreSpy{}
	beforeState := snapshotReplayAdminState(t, fixture.dbPath, fixture.ownerKey, fixture.appID)
	sqliteStore := openReplayAdminTestStore(t, fixture.dbPath)
	dryRun, err := sqliteStore.ReplaySyncMutationReceiptsDryRun(context.Background(), fixture.ownerKey, fixture.appID)
	if err != nil {
		t.Fatalf("build dry-run preview before apply: %v", err)
	}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := runWithDeps(makeReplayAdminCommandDeps(t, spyHolder, nil), buildApplyArgs(fixture), stdout, stderr)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d stderr=%q", exitCode, stderr.String())
	}

	result := decodeApplyResult(t, stdout.String())
	if result.Status != applyStatusApplied ||
		result.PreflightStatus != preflightStatusSafe ||
		result.CompareAndApplyStatus != store.SyncMutationReplayCompareAndApplyStatusAllowed ||
		result.RecoveryStatus != store.SyncMutationReplayRecoveryStatusSafeToAttemptTransaction {
		t.Fatalf("unexpected apply result: %+v", result)
	}
	if result.CanonicalDocumentVersionBefore == nil || result.CanonicalDocumentVersionAfter == nil {
		t.Fatalf("expected canonical versions in apply result, got %+v", result)
	}
	if result.CanonicalDocumentHashBefore == nil || result.CanonicalDocumentHashAfter == nil {
		t.Fatalf("expected canonical hashes in apply result, got %+v", result)
	}
	if !result.CanonicalStateChanged || !result.DocumentVersionAdvanced || !result.ApplicationRowsInserted {
		t.Fatalf("expected apply result to report canonical mutation, got %+v", result)
	}
	if result.InsertedApplicationRowCount != 1 {
		t.Fatalf("expected one inserted application row, got %+v", result)
	}
	if len(result.MutationResults) != 1 || result.MutationResults[0].ApplicationStatus != store.SyncMutationReplayApplicationStatusApplied {
		t.Fatalf("unexpected mutation results: %+v", result.MutationResults)
	}
	if spyHolder.store == nil {
		t.Fatalf("expected store to open for apply")
	}
	if spyHolder.store.applyCallCount != 1 {
		t.Fatalf("expected exactly one apply call, got %d", spyHolder.store.applyCallCount)
	}
	if !spyHolder.store.lastApplyOptions.AllowInternalAuthoritativeReplay {
		t.Fatalf("expected internal authoritative replay gate to be enabled in store apply options")
	}
	if !strings.Contains(stderr.String(), "mode=apply") || !strings.Contains(stderr.String(), "status="+applyStatusApplied) || !strings.Contains(stderr.String(), "preflight_status="+preflightStatusSafe) {
		t.Fatalf("unexpected stderr audit line: %q", stderr.String())
	}

	afterState := snapshotReplayAdminState(t, fixture.dbPath, fixture.ownerKey, fixture.appID)
	if afterState.DocumentVersion != beforeState.DocumentVersion+1 {
		t.Fatalf("expected document version to advance once from %d to %d, got %d", beforeState.DocumentVersion, beforeState.DocumentVersion+1, afterState.DocumentVersion)
	}
	if afterState.DocumentBody != string(dryRun.PreviewBody) {
		t.Fatalf("expected canonical document body to match preview body\nafter=%s\npreview=%s", afterState.DocumentBody, dryRun.PreviewBody)
	}
	if len(afterState.Applications) != 1 {
		t.Fatalf("expected one application row after apply, got %+v", afterState.Applications)
	}
	if afterState.Applications[0].ApplicationStatus != store.SyncMutationReplayApplicationStatusApplied {
		t.Fatalf("unexpected application row after apply: %+v", afterState.Applications[0])
	}
	assertReplayAdminReceiptsAndObservationsEqual(t, beforeState, afterState)
}

func TestRunApplyPolicyAbortDoesNotWrite(t *testing.T) {
	t.Setenv(envEnableInternalSyncReplayCLI, "1")
	t.Setenv(envEnableInternalSyncReplayApply, "1")
	t.Setenv(envDeploymentMode, "selfhosted")

	fixture := createReplayPreflightFixture(t, replayPreflightFixtureOptions{
		customReceiptInputs: []store.SyncMutationReceiptInput{
			buildReplayReceiptInputForCommand("mut-invalid-payload", "Node", "item-1", "MoveNode", `[]`),
		},
	})
	spyHolder := &replayAdminStoreSpy{}
	beforeState := snapshotReplayAdminState(t, fixture.dbPath, fixture.ownerKey, fixture.appID)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := runWithDeps(makeReplayAdminCommandDeps(t, spyHolder, nil), buildApplyArgs(fixture), stdout, stderr)
	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d stderr=%q", exitCode, stderr.String())
	}

	result := decodeApplyResult(t, stdout.String())
	if result.Status != applyStatusAbortedPolicy {
		t.Fatalf("unexpected policy abort result: %+v", result)
	}
	if len(result.MutationResults) != 1 || result.MutationResults[0].ApplicationStatus != store.SyncMutationReplayApplicationStatusFailed {
		t.Fatalf("unexpected policy abort mutation results: %+v", result.MutationResults)
	}
	if result.CanonicalStateChanged || result.DocumentVersionAdvanced || result.ApplicationRowsInserted {
		t.Fatalf("expected policy abort to report no committed mutation, got %+v", result)
	}
	if spyHolder.store == nil || spyHolder.store.applyCallCount != 1 {
		t.Fatalf("expected exactly one apply call before policy abort, got %+v", spyHolder.store)
	}

	afterState := snapshotReplayAdminState(t, fixture.dbPath, fixture.ownerKey, fixture.appID)
	assertReplayAdminStateEqual(t, beforeState, afterState)
}

func TestRunApplyInternalErrorFailpointFailsClosed(t *testing.T) {
	t.Setenv(envEnableInternalSyncReplayCLI, "1")
	t.Setenv(envEnableInternalSyncReplayApply, "1")
	t.Setenv(envDeploymentMode, "selfhosted")

	fixture := createReplayPreflightFixture(t, replayPreflightFixtureOptions{})
	spyHolder := &replayAdminStoreSpy{}
	beforeState := snapshotReplayAdminState(t, fixture.dbPath, fixture.ownerKey, fixture.appID)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := runWithDeps(makeReplayAdminCommandDeps(t, spyHolder, func(testStore *countingReplayAdminStore) {
		testStore.applyFunc = func(ctx context.Context, ownerKey, appID string, observationID int64, options store.SyncMutationReplayAuthoritativeApplyOptions) (store.SyncMutationReplayAuthoritativeApplyResult, error) {
			failAfter := 0
			options.FailAfterApplicationRowInserts = &failAfter
			return testStore.inner.ApplySyncMutationReplayAuthoritativeInternal(ctx, ownerKey, appID, observationID, options)
		}
	}), buildApplyArgs(fixture), stdout, stderr)
	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d stderr=%q", exitCode, stderr.String())
	}

	result := decodeApplyResult(t, stdout.String())
	if result.Status != applyStatusInternalError {
		t.Fatalf("unexpected failpoint result: %+v", result)
	}
	if !containsString(result.Reasons, "authoritative_apply_failed") {
		t.Fatalf("expected authoritative_apply_failed reason, got %+v", result.Reasons)
	}
	if spyHolder.store == nil || spyHolder.store.applyCallCount != 1 {
		t.Fatalf("expected one apply call for failpoint case, got %+v", spyHolder.store)
	}

	afterState := snapshotReplayAdminState(t, fixture.dbPath, fixture.ownerKey, fixture.appID)
	assertReplayAdminStateEqual(t, beforeState, afterState)
}

func TestRunApplyTextOutput(t *testing.T) {
	t.Setenv(envEnableInternalSyncReplayCLI, "1")
	t.Setenv(envEnableInternalSyncReplayApply, "1")
	t.Setenv(envDeploymentMode, "selfhosted")

	fixture := createReplayPreflightFixture(t, replayPreflightFixtureOptions{})
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	args := append(buildApplyArgs(fixture), "--output", "text", "--verbose")
	exitCode := run(args, stdout, stderr)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d stderr=%q", exitCode, stderr.String())
	}

	text := stdout.String()
	if !strings.Contains(text, "status="+applyStatusApplied) ||
		!strings.Contains(text, "owner_key="+fixture.ownerKey) ||
		!strings.Contains(text, "app_id="+fixture.appID) ||
		!strings.Contains(text, "observation_id="+fixture.observationIDString) {
		t.Fatalf("unexpected apply text output: %q", text)
	}
}

func TestRunApplyUnsafePreflightStatusesDoNotReachApply(t *testing.T) {
	t.Setenv(envEnableInternalSyncReplayCLI, "1")
	t.Setenv(envEnableInternalSyncReplayApply, "1")
	t.Setenv(envDeploymentMode, "selfhosted")

	for _, testCase := range []struct {
		name              string
		fixtureOptions    replayPreflightFixtureOptions
		mutateState       func(t *testing.T, sqliteStore *store.Store, fixture replayPreflightFixture)
		argsForFixture    func(fixture replayPreflightFixture) []string
		expectedStatus    string
		expectedPreflight string
	}{
		{
			name:              "stale_canonical_document",
			fixtureOptions:    replayPreflightFixtureOptions{mutateCanonicalAfterObservation: true},
			expectedStatus:    applyStatusStaleCanonical,
			expectedPreflight: preflightStatusStaleCanonical,
		},
		{
			name: "stale_receipt_set",
			mutateState: func(t *testing.T, sqliteStore *store.Store, fixture replayPreflightFixture) {
				t.Helper()
				if _, err := sqliteStore.AcceptSyncMutationReceipts(context.Background(), fixture.ownerKey, fixture.appID, []store.SyncMutationReceiptInput{
					buildReplayReceiptInputForCommand("mut-late-receipt", "Node", "item-9", "CreateNode", `{"tabId":"tab-1","name":"Late","position":{"top":"80px","left":"80px"}}`),
				}); err != nil {
					t.Fatalf("insert late accepted receipt: %v", err)
				}
			},
			expectedStatus:    applyStatusStaleReceiptSet,
			expectedPreflight: preflightStatusStaleReceiptSet,
		},
		{
			name: "partial_application_rows",
			fixtureOptions: replayPreflightFixtureOptions{
				receiptCount:        2,
				applicationScenario: replayApplicationScenarioPartial,
			},
			expectedStatus:    applyStatusPartialRows,
			expectedPreflight: preflightStatusPartialRows,
		},
		{
			name: "application_rows_without_matching_canonical_state",
			fixtureOptions: replayPreflightFixtureOptions{
				applicationScenario: replayApplicationScenarioMismatchedCanonical,
			},
			expectedStatus:    applyStatusRowsMismatch,
			expectedPreflight: preflightStatusRowsMismatch,
		},
		{
			name: "canonical_state_without_application_rows",
			fixtureOptions: replayPreflightFixtureOptions{
				mutateCanonicalToPreviewWithoutRows: true,
			},
			expectedStatus:    applyStatusCanonicalMismatch,
			expectedPreflight: preflightStatusCanonicalMismatch,
		},
		{
			name: "missing_observation",
			argsForFixture: func(fixture replayPreflightFixture) []string {
				missingObservationID := strconv.FormatInt(fixture.observationID+999999, 10)
				args := buildApplyArgs(fixture)
				args = replaceFlagValue(args, "--observation-id", missingObservationID)
				args = replaceFlagValue(args, "--confirm-observation-id", missingObservationID)
				return args
			},
			expectedStatus:    applyStatusMissingObservation,
			expectedPreflight: preflightStatusMissingObservation,
		},
		{
			name: "invalid_observation_scope",
			argsForFixture: func(fixture replayPreflightFixture) []string {
				args := buildApplyArgs(fixture)
				args = replaceFlagValue(args, "--owner-key", "other-owner")
				args = replaceFlagValue(args, "--confirm-owner-key", "other-owner")
				return args
			},
			expectedStatus:    applyStatusInvalidScope,
			expectedPreflight: preflightStatusInvalidScope,
		},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			fixture := createReplayPreflightFixture(t, testCase.fixtureOptions)
			sqliteStore := openReplayAdminTestStore(t, fixture.dbPath)
			if testCase.mutateState != nil {
				testCase.mutateState(t, sqliteStore, fixture)
			}
			beforeState := snapshotReplayAdminState(t, fixture.dbPath, fixture.ownerKey, fixture.appID)
			spyHolder := &replayAdminStoreSpy{}
			stdout := &bytes.Buffer{}
			stderr := &bytes.Buffer{}

			args := buildApplyArgs(fixture)
			if testCase.argsForFixture != nil {
				args = testCase.argsForFixture(fixture)
			}

			exitCode := runWithDeps(makeReplayAdminCommandDeps(t, spyHolder, nil), args, stdout, stderr)
			if exitCode != 1 {
				t.Fatalf("expected exit code 1, got %d stderr=%q", exitCode, stderr.String())
			}

			result := decodeApplyResult(t, stdout.String())
			if result.Status != testCase.expectedStatus || result.PreflightStatus != testCase.expectedPreflight {
				t.Fatalf("unexpected unsafe preflight result: %+v", result)
			}
			if result.CanonicalStateChanged || result.DocumentVersionAdvanced || result.ApplicationRowsInserted {
				t.Fatalf("expected unsafe preflight result to report no committed mutation, got %+v", result)
			}
			if spyHolder.store == nil {
				t.Fatalf("expected store to open for same-invocation preflight")
			}
			if spyHolder.store.applyCallCount != 0 {
				t.Fatalf("expected unsafe preflight to block apply call, got %d", spyHolder.store.applyCallCount)
			}

			afterState := snapshotReplayAdminState(t, fixture.dbPath, fixture.ownerKey, fixture.appID)
			assertReplayAdminStateEqual(t, beforeState, afterState)
		})
	}
}

func TestRunApplyRaceCanonicalChangeAfterPreflightFailsClosed(t *testing.T) {
	t.Setenv(envEnableInternalSyncReplayCLI, "1")
	t.Setenv(envEnableInternalSyncReplayApply, "1")
	t.Setenv(envDeploymentMode, "selfhosted")

	fixture := createReplayPreflightFixture(t, replayPreflightFixtureOptions{})
	spyHolder := &replayAdminStoreSpy{}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	var expectedState replayPreflightStateSnapshot

	exitCode := runWithDeps(makeReplayAdminCommandDeps(t, spyHolder, func(testStore *countingReplayAdminStore) {
		testStore.applyFunc = func(ctx context.Context, ownerKey, appID string, observationID int64, options store.SyncMutationReplayAuthoritativeApplyOptions) (store.SyncMutationReplayAuthoritativeApplyResult, error) {
			putReplayDocumentBodyForCommand(t, testStore.inner, ownerKey, appID, buildReplaySnapshotBodyForUpdatedCommand(t))
			expectedState = snapshotReplayAdminState(t, fixture.dbPath, fixture.ownerKey, fixture.appID)
			return testStore.inner.ApplySyncMutationReplayAuthoritativeInternal(ctx, ownerKey, appID, observationID, options)
		}
	}), buildApplyArgs(fixture), stdout, stderr)
	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d stderr=%q", exitCode, stderr.String())
	}

	result := decodeApplyResult(t, stdout.String())
	if result.Status != applyStatusStaleCanonical || result.PreflightStatus != preflightStatusStaleCanonical {
		t.Fatalf("unexpected stale canonical race result: %+v", result)
	}
	if result.CanonicalStateChanged || result.DocumentVersionAdvanced || result.ApplicationRowsInserted {
		t.Fatalf("expected stale canonical race to report no committed mutation, got %+v", result)
	}
	if spyHolder.store == nil || spyHolder.store.applyCallCount != 1 {
		t.Fatalf("expected exactly one apply call before stale canonical abort, got %+v", spyHolder.store)
	}

	afterState := snapshotReplayAdminState(t, fixture.dbPath, fixture.ownerKey, fixture.appID)
	assertReplayAdminStateEqual(t, expectedState, afterState)
}

func TestRunApplyRaceReceiptSetChangeAfterPreflightFailsClosed(t *testing.T) {
	t.Setenv(envEnableInternalSyncReplayCLI, "1")
	t.Setenv(envEnableInternalSyncReplayApply, "1")
	t.Setenv(envDeploymentMode, "selfhosted")

	fixture := createReplayPreflightFixture(t, replayPreflightFixtureOptions{})
	spyHolder := &replayAdminStoreSpy{}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	var expectedState replayPreflightStateSnapshot

	exitCode := runWithDeps(makeReplayAdminCommandDeps(t, spyHolder, func(testStore *countingReplayAdminStore) {
		testStore.applyFunc = func(ctx context.Context, ownerKey, appID string, observationID int64, options store.SyncMutationReplayAuthoritativeApplyOptions) (store.SyncMutationReplayAuthoritativeApplyResult, error) {
			if _, err := testStore.inner.AcceptSyncMutationReceipts(ctx, ownerKey, appID, []store.SyncMutationReceiptInput{
				buildReplayReceiptInputForCommand("mut-race-late-receipt", "Node", "item-99", "CreateNode", `{"tabId":"tab-1","name":"Late","position":{"top":"90px","left":"90px"}}`),
			}); err != nil {
				t.Fatalf("insert late accepted receipt: %v", err)
			}
			expectedState = snapshotReplayAdminState(t, fixture.dbPath, fixture.ownerKey, fixture.appID)
			return testStore.inner.ApplySyncMutationReplayAuthoritativeInternal(ctx, ownerKey, appID, observationID, options)
		}
	}), buildApplyArgs(fixture), stdout, stderr)
	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d stderr=%q", exitCode, stderr.String())
	}

	result := decodeApplyResult(t, stdout.String())
	if result.Status != applyStatusStaleReceiptSet || result.PreflightStatus != preflightStatusStaleReceiptSet {
		t.Fatalf("unexpected stale receipt set race result: %+v", result)
	}
	if result.CanonicalStateChanged || result.DocumentVersionAdvanced || result.ApplicationRowsInserted {
		t.Fatalf("expected stale receipt set race to report no committed mutation, got %+v", result)
	}
	if spyHolder.store == nil || spyHolder.store.applyCallCount != 1 {
		t.Fatalf("expected exactly one apply call before stale receipt abort, got %+v", spyHolder.store)
	}

	afterState := snapshotReplayAdminState(t, fixture.dbPath, fixture.ownerKey, fixture.appID)
	assertReplayAdminStateEqual(t, expectedState, afterState)
}

func TestRunApplyRaceExternalPartialApplicationRowsAfterPreflightFailsClosed(t *testing.T) {
	t.Setenv(envEnableInternalSyncReplayCLI, "1")
	t.Setenv(envEnableInternalSyncReplayApply, "1")
	t.Setenv(envDeploymentMode, "selfhosted")

	fixture := createReplayPreflightFixture(t, replayPreflightFixtureOptions{receiptCount: 2})
	spyHolder := &replayAdminStoreSpy{}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	var expectedState replayPreflightStateSnapshot

	exitCode := runWithDeps(makeReplayAdminCommandDeps(t, spyHolder, func(testStore *countingReplayAdminStore) {
		testStore.applyFunc = func(ctx context.Context, ownerKey, appID string, observationID int64, options store.SyncMutationReplayAuthoritativeApplyOptions) (store.SyncMutationReplayAuthoritativeApplyResult, error) {
			currentDoc, err := testStore.inner.GetDocument(ctx, ownerKey, appID)
			if err != nil {
				t.Fatalf("load current document for external application row seed: %v", err)
			}
			hashBefore := hashReplayBytesForCommand(currentDoc.Body)
			if _, err := testStore.inner.RecordSyncMutationReplayApplicationInert(ctx, ownerKey, appID, SyncMutationReplayApplicationInputForCommand(fixture.mutationIDs[0], observationID, currentDoc.Version, hashBefore, nil, nil)); err != nil {
				t.Fatalf("insert external partial application row: %v", err)
			}
			expectedState = snapshotReplayAdminState(t, fixture.dbPath, fixture.ownerKey, fixture.appID)
			return testStore.inner.ApplySyncMutationReplayAuthoritativeInternal(ctx, ownerKey, appID, observationID, options)
		}
	}), buildApplyArgs(fixture), stdout, stderr)
	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d stderr=%q", exitCode, stderr.String())
	}

	result := decodeApplyResult(t, stdout.String())
	if result.Status != applyStatusPartialRows || result.PreflightStatus != preflightStatusPartialRows {
		t.Fatalf("unexpected partial rows race result: %+v", result)
	}
	if result.CanonicalStateChanged || result.DocumentVersionAdvanced || result.ApplicationRowsInserted {
		t.Fatalf("expected partial rows race to report no committed mutation, got %+v", result)
	}
	if spyHolder.store == nil || spyHolder.store.applyCallCount != 1 {
		t.Fatalf("expected exactly one apply call before partial rows abort, got %+v", spyHolder.store)
	}

	afterState := snapshotReplayAdminState(t, fixture.dbPath, fixture.ownerKey, fixture.appID)
	assertReplayAdminStateEqual(t, expectedState, afterState)
}

func TestRunApplyRaceDuplicateItemBlockerAfterPreflightFailsClosed(t *testing.T) {
	t.Setenv(envEnableInternalSyncReplayCLI, "1")
	t.Setenv(envEnableInternalSyncReplayApply, "1")
	t.Setenv(envDeploymentMode, "selfhosted")

	fixture := createReplayPreflightFixture(t, replayPreflightFixtureOptions{})
	spyHolder := &replayAdminStoreSpy{}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	var expectedState replayPreflightStateSnapshot

	exitCode := runWithDeps(makeReplayAdminCommandDeps(t, spyHolder, func(testStore *countingReplayAdminStore) {
		testStore.applyFunc = func(ctx context.Context, ownerKey, appID string, observationID int64, options store.SyncMutationReplayAuthoritativeApplyOptions) (store.SyncMutationReplayAuthoritativeApplyResult, error) {
			putReplayDocumentBodyForCommand(t, testStore.inner, ownerKey, appID, buildReplaySnapshotBodyForCommand(t, true))
			expectedState = snapshotReplayAdminState(t, fixture.dbPath, fixture.ownerKey, fixture.appID)
			return testStore.inner.ApplySyncMutationReplayAuthoritativeInternal(ctx, ownerKey, appID, observationID, options)
		}
	}), buildApplyArgs(fixture), stdout, stderr)
	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d stderr=%q", exitCode, stderr.String())
	}

	result := decodeApplyResult(t, stdout.String())
	if result.Status != applyStatusBlockedSnapshot || result.PreflightStatus != preflightStatusBlockedSnapshot {
		t.Fatalf("unexpected duplicate-item blocker race result: %+v", result)
	}
	if result.CanonicalStateChanged || result.DocumentVersionAdvanced || result.ApplicationRowsInserted {
		t.Fatalf("expected duplicate-item blocker race to report no committed mutation, got %+v", result)
	}
	if spyHolder.store == nil || spyHolder.store.applyCallCount != 1 {
		t.Fatalf("expected exactly one apply call before blocked snapshot abort, got %+v", spyHolder.store)
	}

	afterState := snapshotReplayAdminState(t, fixture.dbPath, fixture.ownerKey, fixture.appID)
	assertReplayAdminStateEqual(t, expectedState, afterState)
}

func TestRunApplyRaceDuplicateEdgeBlockerAfterPreflightFailsClosed(t *testing.T) {
	t.Setenv(envEnableInternalSyncReplayCLI, "1")
	t.Setenv(envEnableInternalSyncReplayApply, "1")
	t.Setenv(envDeploymentMode, "selfhosted")

	fixture := createReplayPreflightFixture(t, replayPreflightFixtureOptions{})
	spyHolder := &replayAdminStoreSpy{}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	var expectedState replayPreflightStateSnapshot

	exitCode := runWithDeps(makeReplayAdminCommandDeps(t, spyHolder, func(testStore *countingReplayAdminStore) {
		testStore.applyFunc = func(ctx context.Context, ownerKey, appID string, observationID int64, options store.SyncMutationReplayAuthoritativeApplyOptions) (store.SyncMutationReplayAuthoritativeApplyResult, error) {
			putReplayDocumentBodyForCommand(t, testStore.inner, ownerKey, appID, buildReplaySnapshotBodyWithDuplicateEdgeIDsForCommand(t))
			expectedState = snapshotReplayAdminState(t, fixture.dbPath, fixture.ownerKey, fixture.appID)
			return testStore.inner.ApplySyncMutationReplayAuthoritativeInternal(ctx, ownerKey, appID, observationID, options)
		}
	}), buildApplyArgs(fixture), stdout, stderr)
	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d stderr=%q", exitCode, stderr.String())
	}

	result := decodeApplyResult(t, stdout.String())
	if result.Status != applyStatusBlockedSnapshot || result.PreflightStatus != preflightStatusBlockedSnapshot {
		t.Fatalf("unexpected duplicate-edge blocker race result: %+v", result)
	}
	if result.CanonicalStateChanged || result.DocumentVersionAdvanced || result.ApplicationRowsInserted {
		t.Fatalf("expected duplicate-edge blocker race to report no committed mutation, got %+v", result)
	}
	if spyHolder.store == nil || spyHolder.store.applyCallCount != 1 {
		t.Fatalf("expected exactly one apply call before blocked snapshot abort, got %+v", spyHolder.store)
	}

	afterState := snapshotReplayAdminState(t, fixture.dbPath, fixture.ownerKey, fixture.appID)
	assertReplayAdminStateEqual(t, expectedState, afterState)
}

func TestRunApplyAllSkippedAdvancesVersionWithoutChangingBodyAndIsIdempotent(t *testing.T) {
	t.Setenv(envEnableInternalSyncReplayCLI, "1")
	t.Setenv(envEnableInternalSyncReplayApply, "1")
	t.Setenv(envDeploymentMode, "selfhosted")

	fixture := createReplayPreflightFixture(t, replayPreflightFixtureOptions{
		customReceiptInputs: []store.SyncMutationReceiptInput{
			buildReplayReceiptInputForCommand("mut-delete-missing", "Node", "missing-item", "DeleteNode", `{"tabId":"tab-1"}`),
		},
	})
	spyHolder := &replayAdminStoreSpy{}
	beforeState := snapshotReplayAdminState(t, fixture.dbPath, fixture.ownerKey, fixture.appID)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := runWithDeps(makeReplayAdminCommandDeps(t, spyHolder, nil), buildApplyArgs(fixture), stdout, stderr)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d stderr=%q", exitCode, stderr.String())
	}

	result := decodeApplyResult(t, stdout.String())
	if result.Status != applyStatusApplied || result.PreflightStatus != preflightStatusSafe {
		t.Fatalf("unexpected all-skipped apply result: %+v", result)
	}
	if result.CanonicalStateChanged {
		t.Fatalf("expected all-skipped apply to preserve canonical bytes, got %+v", result)
	}
	if !result.DocumentVersionAdvanced || !result.ApplicationRowsInserted {
		t.Fatalf("expected all-skipped apply to advance version and insert rows, got %+v", result)
	}
	if result.InsertedApplicationRowCount != 1 {
		t.Fatalf("expected one inserted skipped application row, got %+v", result)
	}
	if len(result.MutationResults) != 1 || result.MutationResults[0].ApplicationStatus != store.SyncMutationReplayApplicationStatusSkipped {
		t.Fatalf("unexpected all-skipped mutation results: %+v", result.MutationResults)
	}
	if spyHolder.store == nil || spyHolder.store.applyCallCount != 1 {
		t.Fatalf("expected exactly one apply call for all-skipped case, got %+v", spyHolder.store)
	}

	afterState := snapshotReplayAdminState(t, fixture.dbPath, fixture.ownerKey, fixture.appID)
	if afterState.DocumentVersion != beforeState.DocumentVersion+1 {
		t.Fatalf("expected all-skipped apply to advance version once from %d to %d, got %d", beforeState.DocumentVersion, beforeState.DocumentVersion+1, afterState.DocumentVersion)
	}
	if afterState.DocumentBody != beforeState.DocumentBody {
		t.Fatalf("expected all-skipped apply to preserve body bytes\nbefore=%s\nafter=%s", beforeState.DocumentBody, afterState.DocumentBody)
	}
	if len(afterState.Applications) != 1 {
		t.Fatalf("expected one skipped application row after apply, got %+v", afterState.Applications)
	}
	if afterState.Applications[0].ApplicationStatus != store.SyncMutationReplayApplicationStatusSkipped {
		t.Fatalf("unexpected all-skipped application row: %+v", afterState.Applications[0])
	}
	assertReplayAdminReceiptsAndObservationsEqual(t, beforeState, afterState)

	secondSpyHolder := &replayAdminStoreSpy{}
	secondStdout := &bytes.Buffer{}
	secondStderr := &bytes.Buffer{}
	exitCode = runWithDeps(makeReplayAdminCommandDeps(t, secondSpyHolder, nil), buildApplyArgs(fixture), secondStdout, secondStderr)
	if exitCode != 0 {
		t.Fatalf("expected repeated all-skipped apply to exit 0, got %d stderr=%q", exitCode, secondStderr.String())
	}
	secondResult := decodeApplyResult(t, secondStdout.String())
	if secondResult.Status != applyStatusIdempotent || secondResult.PreflightStatus != preflightStatusIdempotent {
		t.Fatalf("unexpected repeated all-skipped apply result: %+v", secondResult)
	}
	if secondSpyHolder.store == nil || secondSpyHolder.store.applyCallCount != 0 {
		t.Fatalf("expected repeated all-skipped apply to exit before store apply, got %+v", secondSpyHolder.store)
	}

	afterSecondState := snapshotReplayAdminState(t, fixture.dbPath, fixture.ownerKey, fixture.appID)
	assertReplayAdminStateEqual(t, afterState, afterSecondState)
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
	customReceiptInputs                 []store.SyncMutationReceiptInput
	customDocumentBody                  json.RawMessage
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
	Observations    []replayObservationRow
	Receipts        []replayReceiptStatusRow
}

type replayReceiptStatusRow struct {
	MutationID string
	Status     string
}

type replayObservationRow struct {
	ID                               int64
	OwnerKey                         string
	AppID                            string
	CanonicalDocumentVersionObserved int64
	CanonicalDocumentHashObserved    string
	ReceiptCountConsidered           int
	FirstOrderedMutationID           string
	LastOrderedMutationID            string
	OrderedReceiptHighWatermark      string
	AppliedCount                     int
	SkippedCount                     int
	WarningCount                     int
	PreviewHash                      string
	CreatedAt                        string
}

type replayAdminStoreSpy struct {
	store *countingReplayAdminStore
}

type countingReplayAdminStore struct {
	inner            *store.Store
	applyCallCount   int
	lastApplyOptions store.SyncMutationReplayAuthoritativeApplyOptions
	applyFunc        func(ctx context.Context, ownerKey, appID string, observationID int64, options store.SyncMutationReplayAuthoritativeApplyOptions) (store.SyncMutationReplayAuthoritativeApplyResult, error)
}

func (s *countingReplayAdminStore) Close() error {
	return s.inner.Close()
}

func (s *countingReplayAdminStore) EvaluateSyncMutationReplayCompareAndApplyPreconditions(ctx context.Context, ownerKey, appID string, observationID int64) (store.SyncMutationReplayCompareAndApplyEvaluation, error) {
	return s.inner.EvaluateSyncMutationReplayCompareAndApplyPreconditions(ctx, ownerKey, appID, observationID)
}

func (s *countingReplayAdminStore) EvaluateSyncMutationReplayRecoveryState(ctx context.Context, ownerKey, appID string, observationID int64) (store.SyncMutationReplayRecoveryEvaluation, error) {
	return s.inner.EvaluateSyncMutationReplayRecoveryState(ctx, ownerKey, appID, observationID)
}

func (s *countingReplayAdminStore) ApplySyncMutationReplayAuthoritativeInternal(ctx context.Context, ownerKey, appID string, observationID int64, options store.SyncMutationReplayAuthoritativeApplyOptions) (store.SyncMutationReplayAuthoritativeApplyResult, error) {
	s.applyCallCount++
	s.lastApplyOptions = options
	if s.applyFunc != nil {
		return s.applyFunc(ctx, ownerKey, appID, observationID, options)
	}
	return s.inner.ApplySyncMutationReplayAuthoritativeInternal(ctx, ownerKey, appID, observationID, options)
}

func makeReplayAdminCommandDeps(t *testing.T, holder *replayAdminStoreSpy, configure func(*countingReplayAdminStore)) commandDeps {
	t.Helper()

	return commandDeps{
		openStore: func(dbPath string) (replayAdminStore, error) {
			inner, err := store.Open(dbPath)
			if err != nil {
				return nil, err
			}
			wrapped := &countingReplayAdminStore{inner: inner}
			if configure != nil {
				configure(wrapped)
			}
			if holder != nil {
				holder.store = wrapped
			}
			return wrapped, nil
		},
	}
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
	body := options.customDocumentBody
	if len(body) == 0 {
		body = buildReplaySnapshotBodyForCommand(t, options.duplicateItemIDs)
	}
	before, err := sqliteStore.PutDocument(context.Background(), ownerKey, appID, body, nil)
	if err != nil {
		t.Fatalf("seed document: %v", err)
	}

	var mutationIDs []string
	var receiptInputs []store.SyncMutationReceiptInput
	if len(options.customReceiptInputs) > 0 {
		mutationIDs = make([]string, 0, len(options.customReceiptInputs))
		receiptInputs = make([]store.SyncMutationReceiptInput, 0, len(options.customReceiptInputs))
		for _, receiptInput := range options.customReceiptInputs {
			mutationIDs = append(mutationIDs, receiptInput.MutationID)
			receiptInputs = append(receiptInputs, receiptInput)
		}
	} else {
		mutationIDs, receiptInputs = buildReplayReceiptInputsForCommand(receiptCount)
	}
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
	observations := loadReplayObservationRowsForCommand(t, dbPath, ownerKey, appID)
	receipts := loadReplayReceiptStatusRowsForCommand(t, dbPath, ownerKey, appID)

	return replayPreflightStateSnapshot{
		DocumentVersion: doc.Version,
		DocumentBody:    string(doc.Body),
		Applications:    applications,
		Observations:    observations,
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
	if !reflect.DeepEqual(before.Observations, after.Observations) {
		t.Fatalf("observation rows changed during preflight\nbefore=%+v\nafter=%+v", before.Observations, after.Observations)
	}
	if !reflect.DeepEqual(before.Receipts, after.Receipts) {
		t.Fatalf("receipt rows changed during preflight\nbefore=%+v\nafter=%+v", before.Receipts, after.Receipts)
	}
}

func assertReplayAdminReceiptsAndObservationsEqual(t *testing.T, before, after replayPreflightStateSnapshot) {
	t.Helper()

	if !reflect.DeepEqual(before.Observations, after.Observations) {
		t.Fatalf("observation rows changed unexpectedly\nbefore=%+v\nafter=%+v", before.Observations, after.Observations)
	}
	if !reflect.DeepEqual(before.Receipts, after.Receipts) {
		t.Fatalf("receipt rows changed unexpectedly\nbefore=%+v\nafter=%+v", before.Receipts, after.Receipts)
	}
}

func loadReplayObservationRowsForCommand(t *testing.T, dbPath, ownerKey, appID string) []replayObservationRow {
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
		`SELECT
			id,
			owner_key,
			app_id,
			canonical_document_version_observed,
			canonical_document_hash_observed,
			receipt_count_considered,
			first_ordered_mutation_id,
			last_ordered_mutation_id,
			ordered_receipt_high_watermark,
			applied_count,
			skipped_count,
			warning_count,
			preview_hash,
			strftime('%Y-%m-%dT%H:%M:%fZ', created_at)
		FROM sync_mutation_replay_dry_run_observations
		WHERE owner_key = ? AND app_id = ?
		ORDER BY id ASC`,
		ownerKey,
		appID,
	)
	if err != nil {
		t.Fatalf("query replay observation rows: %v", err)
	}
	defer rows.Close()

	result := make([]replayObservationRow, 0)
	for rows.Next() {
		var row replayObservationRow
		if err := rows.Scan(
			&row.ID,
			&row.OwnerKey,
			&row.AppID,
			&row.CanonicalDocumentVersionObserved,
			&row.CanonicalDocumentHashObserved,
			&row.ReceiptCountConsidered,
			&row.FirstOrderedMutationID,
			&row.LastOrderedMutationID,
			&row.OrderedReceiptHighWatermark,
			&row.AppliedCount,
			&row.SkippedCount,
			&row.WarningCount,
			&row.PreviewHash,
			&row.CreatedAt,
		); err != nil {
			t.Fatalf("scan replay observation row: %v", err)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate replay observation rows: %v", err)
	}

	return result
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

	return buildReplaySnapshotBodyFromTabsJSONForCommand(t, tabsJSON)
}

func buildReplaySnapshotBodyFromTabsJSONForCommand(t *testing.T, tabsJSON string) json.RawMessage {
	t.Helper()

	body, err := json.Marshal(map[string]string{
		"tabs":        tabsJSON,
		"activeTabId": "tab-1",
	})
	if err != nil {
		t.Fatalf("marshal replay snapshot body: %v", err)
	}
	return json.RawMessage(body)
}

func buildReplaySnapshotBodyWithDuplicateEdgeIDsForCommand(t *testing.T) json.RawMessage {
	t.Helper()

	return buildReplaySnapshotBodyFromTabsJSONForCommand(t, `[{"id":"tab-1","name":"Legacy","items":[{"id":"item-1","name":"First","position":{"top":"0px","left":"0px"}},{"id":"item-2","name":"Second","position":{"top":"10px","left":"10px"}}],"colorIndex":0,"gridSetting":"none","edges":[{"id":"dup-edge","fromItemId":"item-1","toItemId":"item-2","kind":"line"},{"id":"dup-edge","fromItemId":"item-2","toItemId":"item-1","kind":"arrow"}]}]`)
}

func buildReplaySnapshotBodyWithExistingItemForCommand(t *testing.T, itemID string) json.RawMessage {
	t.Helper()

	return buildReplaySnapshotBodyFromTabsJSONForCommand(t, `[{"id":"tab-1","name":"Main","items":[{"id":"`+itemID+`","name":"Existing","position":{"top":"0px","left":"0px"}}],"colorIndex":0,"gridSetting":"none","edges":[]}]`)
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

func putReplayDocumentBodyForCommand(t *testing.T, sqliteStore *store.Store, ownerKey, appID string, body json.RawMessage) store.Document {
	t.Helper()

	currentDoc, err := sqliteStore.GetDocument(context.Background(), ownerKey, appID)
	if err != nil {
		t.Fatalf("load current document before mutation: %v", err)
	}
	expectedVersion := currentDoc.Version
	updatedDoc, err := sqliteStore.PutDocument(context.Background(), ownerKey, appID, body, &expectedVersion)
	if err != nil {
		t.Fatalf("mutate canonical document for command test: %v", err)
	}
	return updatedDoc
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

func buildApplyArgs(fixture replayPreflightFixture) []string {
	return []string{
		commandModeApply,
		"--db", fixture.dbPath,
		"--owner-key", fixture.ownerKey,
		"--confirm-owner-key", fixture.ownerKey,
		"--app-id", fixture.appID,
		"--confirm-app-id", fixture.appID,
		"--observation-id", fixture.observationIDString,
		"--confirm-observation-id", fixture.observationIDString,
		"--i-understand-this-mutates-canonical-state",
	}
}

func removeFlagValue(args []string, flagName string) []string {
	result := make([]string, 0, len(args))
	for index := 0; index < len(args); index++ {
		if args[index] == flagName {
			index++
			continue
		}
		result = append(result, args[index])
	}
	return result
}

func replaceFlagValue(args []string, flagName, replacement string) []string {
	result := append([]string{}, args...)
	for index := 0; index < len(result)-1; index++ {
		if result[index] == flagName {
			result[index+1] = replacement
			return result
		}
	}
	return result
}

func removeBoolFlag(args []string, flagName string) []string {
	result := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == flagName {
			continue
		}
		result = append(result, arg)
	}
	return result
}

func decodePreflightResult(t *testing.T, output string) preflightResult {
	t.Helper()

	var result preflightResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("decode preflight result %q: %v", output, err)
	}
	return result
}

func decodeApplyResult(t *testing.T, output string) applyResult {
	t.Helper()

	var result applyResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("decode apply result %q: %v", output, err)
	}
	return result
}
