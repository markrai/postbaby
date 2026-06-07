package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"postbaby-backend/internal/config"
	"postbaby-backend/internal/store"
)

const (
	commandModePreflight              = "preflight"
	defaultOutputFormat               = "json"
	envEnableInternalSyncReplayCLI    = "POSTBABY_ENABLE_INTERNAL_SYNC_REPLAY_CLI"
	envDeploymentMode                 = "POSTBABY_DEPLOYMENT_MODE"
	cloudMultiUserAlias               = "cloud_multi_user"
	preflightStatusSafe               = "safe_to_attempt_transaction"
	preflightStatusIdempotent         = "idempotent_exit_already_applied"
	preflightStatusRefusedDisabled    = "refused_disabled"
	preflightStatusRefusedCloud       = "refused_cloud_deployment"
	preflightStatusMissingObservation = "missing_observation"
	preflightStatusInvalidScope       = "invalid_observation_scope"
	preflightStatusStaleCanonical     = "stale_canonical_document"
	preflightStatusStaleReceiptSet    = "stale_receipt_set"
	preflightStatusBlockedSnapshot    = "blocked_snapshot"
	preflightStatusPartialRows        = "partial_application_rows"
	preflightStatusRowsMismatch       = "application_rows_without_matching_canonical_state"
	preflightStatusCanonicalMismatch  = "canonical_state_without_application_rows"
	preflightStatusInternalError      = "internal_error"
)

type preflightOptions struct {
	dbPath        string
	ownerKey      string
	appID         string
	observationID int64
	output        string
	verbose       bool
}

type preflightResult struct {
	Mode                        string   `json:"mode"`
	OwnerKey                    string   `json:"ownerKey"`
	AppID                       string   `json:"appId"`
	ObservationID               int64    `json:"observationId"`
	Status                      string   `json:"status"`
	CompareAndApplyStatus       string   `json:"compareAndApplyStatus"`
	RecoveryStatus              string   `json:"recoveryStatus"`
	Reasons                     []string `json:"reasons"`
	Warnings                    []string `json:"warnings"`
	AppliedMutationIDs          []string `json:"appliedMutationIds"`
	MatchingApplicationRowCount int      `json:"matchingApplicationRowCount"`
	CanonicalStateChanged       bool     `json:"canonicalStateChanged"`
	DocumentVersionAdvanced     bool     `json:"documentVersionAdvanced"`
	ApplicationRowsInserted     bool     `json:"applicationRowsInserted"`
}

var standardLoggerMu sync.Mutex

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}

	switch args[0] {
	case commandModePreflight:
		return runPreflight(args[1:], stdout, stderr)
	case "apply":
		fmt.Fprintln(stderr, "error=apply mode is not implemented; use preflight only in this phase")
		printUsage(stderr)
		return 2
	case "-h", "--help", "help":
		printUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "error=unknown command: %s\n", args[0])
		printUsage(stderr)
		return 2
	}
}

func runPreflight(args []string, stdout, stderr io.Writer) int {
	opts, err := parsePreflightOptions(args, stderr)
	if err != nil {
		return 2
	}

	result := preflightResult{
		Mode:                        commandModePreflight,
		OwnerKey:                    opts.ownerKey,
		AppID:                       opts.appID,
		ObservationID:               opts.observationID,
		Status:                      preflightStatusInternalError,
		Reasons:                     []string{},
		Warnings:                    []string{},
		AppliedMutationIDs:          []string{},
		MatchingApplicationRowCount: 0,
		CanonicalStateChanged:       false,
		DocumentVersionAdvanced:     false,
		ApplicationRowsInserted:     false,
	}

	if !envFlagEnabled(envEnableInternalSyncReplayCLI) {
		result.Status = preflightStatusRefusedDisabled
		result.Reasons = []string{"internal_sync_replay_cli_disabled"}
		writePreflightResult(stdout, opts.output, opts.verbose, result)
		writeAuditLine(stderr, result)
		return 1
	}

	deploymentMode, err := currentDeploymentMode()
	if err != nil {
		result.Status = preflightStatusInternalError
		result.Reasons = []string{"invalid_deployment_mode", err.Error()}
		writePreflightResult(stdout, opts.output, opts.verbose, result)
		writeAuditLine(stderr, result)
		return 1
	}
	if deploymentMode == config.DeploymentModeCloud {
		result.Status = preflightStatusRefusedCloud
		result.Reasons = []string{"cloud_deployment_refused"}
		writePreflightResult(stdout, opts.output, opts.verbose, result)
		writeAuditLine(stderr, result)
		return 1
	}

	if err := validateDBPath(opts.dbPath); err != nil {
		result.Status = preflightStatusInternalError
		result.Reasons = []string{"database_file_unavailable", err.Error()}
		writePreflightResult(stdout, opts.output, opts.verbose, result)
		writeAuditLine(stderr, result)
		return 1
	}

	return withSuppressedStandardLogger(func() int {
		sqliteStore, err := store.Open(opts.dbPath)
		if err != nil {
			result.Status = preflightStatusInternalError
			result.Reasons = []string{"open_store_failed", err.Error()}
			writePreflightResult(stdout, opts.output, opts.verbose, result)
			writeAuditLine(stderr, result)
			return 1
		}
		defer func() {
			_ = sqliteStore.Close()
		}()

		ctx := context.Background()
		compareEvaluation, err := sqliteStore.EvaluateSyncMutationReplayCompareAndApplyPreconditions(ctx, opts.ownerKey, opts.appID, opts.observationID)
		if err != nil {
			result.Status = preflightStatusInternalError
			result.Reasons = []string{"compare_and_apply_evaluation_failed", err.Error()}
			writePreflightResult(stdout, opts.output, opts.verbose, result)
			writeAuditLine(stderr, result)
			return 1
		}

		recoveryEvaluation, err := sqliteStore.EvaluateSyncMutationReplayRecoveryState(ctx, opts.ownerKey, opts.appID, opts.observationID)
		if err != nil {
			result.Status = preflightStatusInternalError
			result.Reasons = []string{"recovery_evaluation_failed", err.Error()}
			writePreflightResult(stdout, opts.output, opts.verbose, result)
			writeAuditLine(stderr, result)
			return 1
		}

		result.CompareAndApplyStatus = compareEvaluation.Status
		result.RecoveryStatus = recoveryEvaluation.Status
		result.Reasons = mergeUniqueStrings(compareEvaluation.Reasons, recoveryEvaluation.Reasons)
		result.Warnings = mergeUniqueStrings(compareEvaluation.Warnings, recoveryEvaluation.Warnings)
		result.AppliedMutationIDs = cloneStrings(recoveryEvaluation.AppliedMutationIDs)
		if len(result.AppliedMutationIDs) == 0 {
			result.AppliedMutationIDs = cloneStrings(compareEvaluation.AppliedMutationIDs)
		}
		result.MatchingApplicationRowCount = recoveryEvaluation.MatchingApplicationRowCount
		result.Status = derivePreflightStatus(compareEvaluation, recoveryEvaluation)

		writePreflightResult(stdout, opts.output, opts.verbose, result)
		writeAuditLine(stderr, result)

		if result.Status == preflightStatusSafe || result.Status == preflightStatusIdempotent {
			return 0
		}
		return 1
	})
}

func parsePreflightOptions(args []string, stderr io.Writer) (preflightOptions, error) {
	var opts preflightOptions
	flags := flag.NewFlagSet("postbaby-sync-replay-admin preflight", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&opts.dbPath, "db", "", "SQLite database path")
	flags.StringVar(&opts.ownerKey, "owner-key", "", "Owner key")
	flags.StringVar(&opts.appID, "app-id", "", "App id")
	flags.Int64Var(&opts.observationID, "observation-id", 0, "Replay dry-run observation id")
	flags.StringVar(&opts.output, "output", defaultOutputFormat, "Output format: json or text")
	flags.BoolVar(&opts.verbose, "verbose", false, "Include extra fields in text output")
	if err := flags.Parse(args); err != nil {
		return preflightOptions{}, err
	}

	if strings.TrimSpace(opts.dbPath) == "" {
		fmt.Fprintln(stderr, "error=missing required flag: --db")
		return preflightOptions{}, errors.New("missing db path")
	}
	if strings.TrimSpace(opts.ownerKey) == "" {
		fmt.Fprintln(stderr, "error=missing required flag: --owner-key")
		return preflightOptions{}, errors.New("missing owner key")
	}
	if strings.TrimSpace(opts.appID) == "" {
		fmt.Fprintln(stderr, "error=missing required flag: --app-id")
		return preflightOptions{}, errors.New("missing app id")
	}
	if opts.observationID <= 0 {
		fmt.Fprintln(stderr, "error=missing required flag: --observation-id")
		return preflightOptions{}, errors.New("missing observation id")
	}
	opts.output = strings.ToLower(strings.TrimSpace(opts.output))
	if opts.output != "json" && opts.output != "text" {
		fmt.Fprintf(stderr, "error=unsupported output format: %s\n", opts.output)
		return preflightOptions{}, errors.New("unsupported output format")
	}
	if flags.NArg() > 0 {
		fmt.Fprintf(stderr, "error=unexpected arguments: %s\n", strings.Join(flags.Args(), " "))
		return preflightOptions{}, errors.New("unexpected arguments")
	}
	return opts, nil
}

func currentDeploymentMode() (config.DeploymentMode, error) {
	raw := strings.TrimSpace(os.Getenv(envDeploymentMode))
	if raw == "" {
		return config.DeploymentModeStatic, nil
	}

	switch strings.ToLower(raw) {
	case string(config.DeploymentModeStatic):
		return config.DeploymentModeStatic, nil
	case string(config.DeploymentModeSelfHosted):
		return config.DeploymentModeSelfHosted, nil
	case string(config.DeploymentModeCloud), cloudMultiUserAlias:
		return config.DeploymentModeCloud, nil
	default:
		return "", fmt.Errorf("%s must be one of %q, %q, %q, or %q", envDeploymentMode, config.DeploymentModeStatic, config.DeploymentModeSelfHosted, config.DeploymentModeCloud, cloudMultiUserAlias)
	}
}

func validateDBPath(dbPath string) error {
	_, err := os.Stat(dbPath)
	if err == nil {
		return nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("database file does not exist: %s", dbPath)
	}
	return fmt.Errorf("stat database file: %w", err)
}

func derivePreflightStatus(compare store.SyncMutationReplayCompareAndApplyEvaluation, recovery store.SyncMutationReplayRecoveryEvaluation) string {
	switch recovery.Status {
	case store.SyncMutationReplayRecoveryStatusMissingObservation:
		return preflightStatusMissingObservation
	case store.SyncMutationReplayRecoveryStatusInvalidObservationScope:
		return preflightStatusInvalidScope
	case store.SyncMutationReplayRecoveryStatusBlockedSnapshotRequiresCleanup:
		return preflightStatusBlockedSnapshot
	case store.SyncMutationReplayRecoveryStatusPartialApplicationRows:
		return preflightStatusPartialRows
	case store.SyncMutationReplayRecoveryStatusApplicationRowsWithoutMatchingCanonicalState:
		return preflightStatusRowsMismatch
	case store.SyncMutationReplayRecoveryStatusCanonicalStateWithoutApplicationRows:
		return preflightStatusCanonicalMismatch
	case store.SyncMutationReplayRecoveryStatusAlreadyAppliedRequiresIdempotentExit:
		return preflightStatusIdempotent
	case store.SyncMutationReplayRecoveryStatusStaleObservationRequiresRedryrun:
		switch compare.Status {
		case store.SyncMutationReplayCompareAndApplyStatusStaleCanonicalDocument:
			return preflightStatusStaleCanonical
		case store.SyncMutationReplayCompareAndApplyStatusStaleReceiptSet:
			return preflightStatusStaleReceiptSet
		default:
			return preflightStatusInternalError
		}
	case store.SyncMutationReplayRecoveryStatusSafeToAttemptTransaction:
		switch compare.Status {
		case store.SyncMutationReplayCompareAndApplyStatusAllowed:
			return preflightStatusSafe
		case store.SyncMutationReplayCompareAndApplyStatusAlreadyApplied:
			return preflightStatusIdempotent
		case store.SyncMutationReplayCompareAndApplyStatusBlockedSnapshot:
			return preflightStatusBlockedSnapshot
		case store.SyncMutationReplayCompareAndApplyStatusStaleCanonicalDocument:
			return preflightStatusStaleCanonical
		case store.SyncMutationReplayCompareAndApplyStatusStaleReceiptSet:
			return preflightStatusStaleReceiptSet
		case store.SyncMutationReplayCompareAndApplyStatusMissingObservation:
			return preflightStatusMissingObservation
		case store.SyncMutationReplayCompareAndApplyStatusInvalidObservationScope:
			return preflightStatusInvalidScope
		default:
			return preflightStatusInternalError
		}
	default:
		return preflightStatusInternalError
	}
}

func writePreflightResult(stdout io.Writer, output string, verbose bool, result preflightResult) {
	if output == "text" {
		writeTextPreflightResult(stdout, verbose, result)
		return
	}

	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(result)
}

func writeTextPreflightResult(stdout io.Writer, verbose bool, result preflightResult) {
	fmt.Fprintf(stdout, "mode=%s\n", result.Mode)
	fmt.Fprintf(stdout, "status=%s\n", result.Status)
	fmt.Fprintf(stdout, "owner_key=%s\n", result.OwnerKey)
	fmt.Fprintf(stdout, "app_id=%s\n", result.AppID)
	fmt.Fprintf(stdout, "observation_id=%d\n", result.ObservationID)
	fmt.Fprintf(stdout, "reasons=%s\n", strings.Join(result.Reasons, ","))
	fmt.Fprintf(stdout, "warnings=%s\n", strings.Join(result.Warnings, ","))
	if verbose {
		fmt.Fprintf(stdout, "compare_and_apply_status=%s\n", result.CompareAndApplyStatus)
		fmt.Fprintf(stdout, "recovery_status=%s\n", result.RecoveryStatus)
		fmt.Fprintf(stdout, "matching_application_row_count=%d\n", result.MatchingApplicationRowCount)
		fmt.Fprintf(stdout, "applied_mutation_ids=%s\n", strings.Join(result.AppliedMutationIDs, ","))
		fmt.Fprintf(stdout, "canonical_state_changed=%t\n", result.CanonicalStateChanged)
		fmt.Fprintf(stdout, "document_version_advanced=%t\n", result.DocumentVersionAdvanced)
		fmt.Fprintf(stdout, "application_rows_inserted=%t\n", result.ApplicationRowsInserted)
	}
}

func writeAuditLine(stderr io.Writer, result preflightResult) {
	fmt.Fprintf(
		stderr,
		"ts=%s mode=%s owner_key=%s app_id=%s observation_id=%d status=%s reasons_count=%d warnings_count=%d\n",
		time.Now().UTC().Format(time.RFC3339),
		result.Mode,
		result.OwnerKey,
		result.AppID,
		result.ObservationID,
		result.Status,
		len(result.Reasons),
		len(result.Warnings),
	)
}

func envFlagEnabled(name string) bool {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return false
	}
	switch strings.ToLower(raw) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func mergeUniqueStrings(groups ...[]string) []string {
	result := make([]string, 0)
	seen := make(map[string]struct{})
	for _, group := range groups {
		for _, value := range group {
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = struct{}{}
			result = append(result, value)
		}
	}
	return result
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	return append([]string{}, values...)
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "usage: postbaby-sync-replay-admin preflight --db PATH --owner-key OWNER --app-id APP --observation-id ID [--output json|text] [--verbose]")
}

func withSuppressedStandardLogger(run func() int) int {
	standardLoggerMu.Lock()
	defer standardLoggerMu.Unlock()

	previousWriter := log.Writer()
	log.SetOutput(io.Discard)
	defer log.SetOutput(previousWriter)

	return run()
}
