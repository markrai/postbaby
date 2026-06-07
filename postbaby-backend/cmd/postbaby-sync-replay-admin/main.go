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
	commandModeApply                  = "apply"
	defaultOutputFormat               = "json"
	envEnableInternalSyncReplayCLI    = "POSTBABY_ENABLE_INTERNAL_SYNC_REPLAY_CLI"
	envEnableInternalSyncReplayApply  = "POSTBABY_ENABLE_INTERNAL_SYNC_REPLAY_APPLY"
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
	applyStatusRefusedDisabled        = "refused_disabled"
	applyStatusRefusedApplyDisabled   = "refused_apply_disabled"
	applyStatusRefusedCloud           = "refused_cloud_deployment"
	applyStatusInvalidConfirmation    = "invalid_confirmation"
	applyStatusMissingObservation     = "missing_observation"
	applyStatusInvalidScope           = "invalid_observation_scope"
	applyStatusStaleCanonical         = "stale_canonical_document"
	applyStatusStaleReceiptSet        = "stale_receipt_set"
	applyStatusBlockedSnapshot        = "blocked_snapshot"
	applyStatusPartialRows            = "partial_application_rows"
	applyStatusRowsMismatch           = "application_rows_without_matching_canonical_state"
	applyStatusCanonicalMismatch      = "canonical_state_without_application_rows"
	applyStatusAbortedPolicy          = "aborted_policy"
	applyStatusApplied                = "applied"
	applyStatusIdempotent             = "idempotent_exit_already_applied"
	applyStatusInternalError          = "internal_error"
)

type replayAdminStore interface {
	Close() error
	EvaluateSyncMutationReplayCompareAndApplyPreconditions(ctx context.Context, ownerKey, appID string, observationID int64) (store.SyncMutationReplayCompareAndApplyEvaluation, error)
	EvaluateSyncMutationReplayRecoveryState(ctx context.Context, ownerKey, appID string, observationID int64) (store.SyncMutationReplayRecoveryEvaluation, error)
	ApplySyncMutationReplayAuthoritativeForTestOnly(ctx context.Context, ownerKey, appID string, observationID int64, options store.SyncMutationReplayAuthoritativeApplyOptions) (store.SyncMutationReplayAuthoritativeApplyResult, error)
}

type commandDeps struct {
	openStore func(dbPath string) (replayAdminStore, error)
}

type preflightOptions struct {
	dbPath        string
	ownerKey      string
	appID         string
	observationID int64
	output        string
	verbose       bool
}

type applyOptions struct {
	dbPath                               string
	ownerKey                             string
	confirmOwnerKey                      string
	appID                                string
	confirmAppID                         string
	observationID                        int64
	confirmObservationID                 int64
	output                               string
	verbose                              bool
	iUnderstandThisMutatesCanonicalState bool
}

type replayPreflightEvaluation struct {
	Status                      string
	CompareAndApplyStatus       string
	RecoveryStatus              string
	Reasons                     []string
	Warnings                    []string
	AppliedMutationIDs          []string
	MatchingApplicationRowCount int
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

type applyResult struct {
	Mode                           string                                                `json:"mode"`
	OwnerKey                       string                                                `json:"ownerKey"`
	AppID                          string                                                `json:"appId"`
	ObservationID                  int64                                                 `json:"observationId"`
	Status                         string                                                `json:"status"`
	PreflightStatus                string                                                `json:"preflightStatus"`
	CompareAndApplyStatus          string                                                `json:"compareAndApplyStatus"`
	RecoveryStatus                 string                                                `json:"recoveryStatus"`
	CanonicalDocumentVersionBefore *int64                                                `json:"canonicalDocumentVersionBefore"`
	CanonicalDocumentVersionAfter  *int64                                                `json:"canonicalDocumentVersionAfter"`
	CanonicalDocumentHashBefore    *string                                               `json:"canonicalDocumentHashBefore"`
	CanonicalDocumentHashAfter     *string                                               `json:"canonicalDocumentHashAfter"`
	InsertedApplicationRowCount    int                                                   `json:"insertedApplicationRowCount"`
	MutationResults                []store.SyncMutationReplayAuthoritativeMutationResult `json:"mutationResults"`
	Reasons                        []string                                              `json:"reasons"`
	Warnings                       []string                                              `json:"warnings"`
	CanonicalStateChanged          bool                                                  `json:"canonicalStateChanged"`
	DocumentVersionAdvanced        bool                                                  `json:"documentVersionAdvanced"`
	ApplicationRowsInserted        bool                                                  `json:"applicationRowsInserted"`
}

var (
	standardLoggerMu   sync.Mutex
	defaultCommandDeps = commandDeps{
		openStore: func(dbPath string) (replayAdminStore, error) {
			return store.Open(dbPath)
		},
	}
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	return runWithDeps(defaultCommandDeps, args, stdout, stderr)
}

func runWithDeps(deps commandDeps, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}

	switch args[0] {
	case commandModePreflight:
		return runPreflightWithDeps(deps, args[1:], stdout, stderr)
	case commandModeApply:
		return runApplyWithDeps(deps, args[1:], stdout, stderr)
	case "-h", "--help", "help":
		printUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "error=unknown command: %s\n", args[0])
		printUsage(stderr)
		return 2
	}
}

func runPreflightWithDeps(deps commandDeps, args []string, stdout, stderr io.Writer) int {
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
		writePreflightAuditLine(stderr, result)
		return 1
	}

	deploymentMode, err := currentDeploymentMode()
	if err != nil {
		result.Status = preflightStatusInternalError
		result.Reasons = []string{"invalid_deployment_mode", err.Error()}
		writePreflightResult(stdout, opts.output, opts.verbose, result)
		writePreflightAuditLine(stderr, result)
		return 1
	}
	if deploymentMode == config.DeploymentModeCloud {
		result.Status = preflightStatusRefusedCloud
		result.Reasons = []string{"cloud_deployment_refused"}
		writePreflightResult(stdout, opts.output, opts.verbose, result)
		writePreflightAuditLine(stderr, result)
		return 1
	}

	if err := validateDBPath(opts.dbPath); err != nil {
		result.Status = preflightStatusInternalError
		result.Reasons = []string{"database_file_unavailable", err.Error()}
		writePreflightResult(stdout, opts.output, opts.verbose, result)
		writePreflightAuditLine(stderr, result)
		return 1
	}

	return withSuppressedStandardLogger(func() int {
		sqliteStore, err := deps.openStore(opts.dbPath)
		if err != nil {
			result.Status = preflightStatusInternalError
			result.Reasons = []string{"open_store_failed", err.Error()}
			writePreflightResult(stdout, opts.output, opts.verbose, result)
			writePreflightAuditLine(stderr, result)
			return 1
		}
		defer func() {
			_ = sqliteStore.Close()
		}()

		evaluation, err := evaluatePreflightWithStore(context.Background(), sqliteStore, opts.ownerKey, opts.appID, opts.observationID)
		if err != nil {
			result.Status = preflightStatusInternalError
			result.Reasons = []string{"preflight_evaluation_failed", err.Error()}
			writePreflightResult(stdout, opts.output, opts.verbose, result)
			writePreflightAuditLine(stderr, result)
			return 1
		}

		populatePreflightResultFromEvaluation(&result, evaluation)
		writePreflightResult(stdout, opts.output, opts.verbose, result)
		writePreflightAuditLine(stderr, result)

		if result.Status == preflightStatusSafe || result.Status == preflightStatusIdempotent {
			return 0
		}
		return 1
	})
}

func runApplyWithDeps(deps commandDeps, args []string, stdout, stderr io.Writer) int {
	opts, err := parseApplyOptions(args, stderr)
	if err != nil {
		return 2
	}

	result := applyResult{
		Mode:                        commandModeApply,
		OwnerKey:                    opts.ownerKey,
		AppID:                       opts.appID,
		ObservationID:               opts.observationID,
		Status:                      applyStatusInternalError,
		MutationResults:             []store.SyncMutationReplayAuthoritativeMutationResult{},
		Reasons:                     []string{},
		Warnings:                    []string{},
		InsertedApplicationRowCount: 0,
		CanonicalStateChanged:       false,
		DocumentVersionAdvanced:     false,
		ApplicationRowsInserted:     false,
	}

	if !envFlagEnabled(envEnableInternalSyncReplayCLI) {
		result.Status = applyStatusRefusedDisabled
		result.Reasons = []string{"internal_sync_replay_cli_disabled"}
		writeApplyResult(stdout, opts.output, opts.verbose, result)
		writeApplyAuditLine(stderr, result)
		return 1
	}
	if !envFlagEnabled(envEnableInternalSyncReplayApply) {
		result.Status = applyStatusRefusedApplyDisabled
		result.Reasons = []string{"internal_sync_replay_apply_disabled"}
		writeApplyResult(stdout, opts.output, opts.verbose, result)
		writeApplyAuditLine(stderr, result)
		return 1
	}

	deploymentMode, err := currentDeploymentMode()
	if err != nil {
		result.Status = applyStatusInternalError
		result.Reasons = []string{"invalid_deployment_mode", err.Error()}
		writeApplyResult(stdout, opts.output, opts.verbose, result)
		writeApplyAuditLine(stderr, result)
		return 1
	}
	if deploymentMode == config.DeploymentModeCloud {
		result.Status = applyStatusRefusedCloud
		result.Reasons = []string{"cloud_deployment_refused"}
		writeApplyResult(stdout, opts.output, opts.verbose, result)
		writeApplyAuditLine(stderr, result)
		return 1
	}

	confirmationReasons := validateApplyConfirmation(opts)
	if len(confirmationReasons) > 0 {
		result.Status = applyStatusInvalidConfirmation
		result.Reasons = confirmationReasons
		writeApplyResult(stdout, opts.output, opts.verbose, result)
		writeApplyAuditLine(stderr, result)
		return 2
	}

	if err := validateDBPath(opts.dbPath); err != nil {
		result.Status = applyStatusInternalError
		result.Reasons = []string{"database_file_unavailable", err.Error()}
		writeApplyResult(stdout, opts.output, opts.verbose, result)
		writeApplyAuditLine(stderr, result)
		return 1
	}

	return withSuppressedStandardLogger(func() int {
		sqliteStore, err := deps.openStore(opts.dbPath)
		if err != nil {
			result.Status = applyStatusInternalError
			result.Reasons = []string{"open_store_failed", err.Error()}
			writeApplyResult(stdout, opts.output, opts.verbose, result)
			writeApplyAuditLine(stderr, result)
			return 1
		}
		defer func() {
			_ = sqliteStore.Close()
		}()

		preflightEvaluation, err := evaluatePreflightWithStore(context.Background(), sqliteStore, opts.ownerKey, opts.appID, opts.observationID)
		if err != nil {
			result.Status = applyStatusInternalError
			result.Reasons = []string{"preflight_evaluation_failed", err.Error()}
			writeApplyResult(stdout, opts.output, opts.verbose, result)
			writeApplyAuditLine(stderr, result)
			return 1
		}
		populateApplyResultFromPreflight(&result, preflightEvaluation)

		switch preflightEvaluation.Status {
		case preflightStatusSafe:
		case preflightStatusIdempotent:
			result.Status = applyStatusIdempotent
			writeApplyResult(stdout, opts.output, opts.verbose, result)
			writeApplyAuditLine(stderr, result)
			return 0
		default:
			result.Status = mapPreflightStatusToApplyStatus(preflightEvaluation.Status)
			writeApplyResult(stdout, opts.output, opts.verbose, result)
			writeApplyAuditLine(stderr, result)
			return 1
		}

		storeResult, err := sqliteStore.ApplySyncMutationReplayAuthoritativeForTestOnly(
			context.Background(),
			opts.ownerKey,
			opts.appID,
			opts.observationID,
			store.SyncMutationReplayAuthoritativeApplyOptions{
				AllowInternalAuthoritativeReplay: true,
			},
		)
		if err != nil {
			result.Status = applyStatusInternalError
			result.Reasons = mergeUniqueStrings(result.Reasons, []string{"authoritative_apply_failed", err.Error()})
			writeApplyResult(stdout, opts.output, opts.verbose, result)
			writeApplyAuditLine(stderr, result)
			return 1
		}

		populateApplyResultFromStore(&result, storeResult)

		switch storeResult.Status {
		case store.SyncMutationReplayAuthoritativeApplyStatusApplied:
			result.Status = applyStatusApplied
			writeApplyResult(stdout, opts.output, opts.verbose, result)
			writeApplyAuditLine(stderr, result)
			return 0
		case store.SyncMutationReplayAuthoritativeApplyStatusIdempotentExitAlreadyApplied:
			result.Status = applyStatusIdempotent
			writeApplyResult(stdout, opts.output, opts.verbose, result)
			writeApplyAuditLine(stderr, result)
			return 0
		case store.SyncMutationReplayAuthoritativeApplyStatusAbortedPolicy:
			result.Status = applyStatusAbortedPolicy
			result.Reasons = mergeUniqueStrings(result.Reasons, []string{"authoritative_policy_aborted"}, extractMutationReasons(storeResult.MutationResults))
			writeApplyResult(stdout, opts.output, opts.verbose, result)
			writeApplyAuditLine(stderr, result)
			return 1
		case store.SyncMutationReplayAuthoritativeApplyStatusAbortedPreconditions,
			store.SyncMutationReplayAuthoritativeApplyStatusAbortedRecovery:
			refreshedEvaluation, refreshErr := evaluatePreflightWithStore(context.Background(), sqliteStore, opts.ownerKey, opts.appID, opts.observationID)
			if refreshErr == nil {
				populateApplyResultFromPreflight(&result, refreshedEvaluation)
				mappedStatus := mapPreflightStatusToApplyStatus(refreshedEvaluation.Status)
				if mappedStatus != applyStatusInternalError && mappedStatus != applyStatusApplied {
					result.Status = mappedStatus
					writeApplyResult(stdout, opts.output, opts.verbose, result)
					writeApplyAuditLine(stderr, result)
					return 1
				}
			}
			result.Status = applyStatusInternalError
			result.Reasons = mergeUniqueStrings(result.Reasons, []string{"authoritative_apply_aborted_without_stable_classification"})
			writeApplyResult(stdout, opts.output, opts.verbose, result)
			writeApplyAuditLine(stderr, result)
			return 1
		default:
			result.Status = applyStatusInternalError
			result.Reasons = mergeUniqueStrings(result.Reasons, []string{"unexpected_store_apply_status", storeResult.Status})
			writeApplyResult(stdout, opts.output, opts.verbose, result)
			writeApplyAuditLine(stderr, result)
			return 1
		}
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

func parseApplyOptions(args []string, stderr io.Writer) (applyOptions, error) {
	var opts applyOptions
	flags := flag.NewFlagSet("postbaby-sync-replay-admin apply", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&opts.dbPath, "db", "", "SQLite database path")
	flags.StringVar(&opts.ownerKey, "owner-key", "", "Owner key")
	flags.StringVar(&opts.confirmOwnerKey, "confirm-owner-key", "", "Confirm owner key")
	flags.StringVar(&opts.appID, "app-id", "", "App id")
	flags.StringVar(&opts.confirmAppID, "confirm-app-id", "", "Confirm app id")
	flags.Int64Var(&opts.observationID, "observation-id", 0, "Replay dry-run observation id")
	flags.Int64Var(&opts.confirmObservationID, "confirm-observation-id", 0, "Confirm replay dry-run observation id")
	flags.BoolVar(&opts.iUnderstandThisMutatesCanonicalState, "i-understand-this-mutates-canonical-state", false, "Required acknowledgement for canonical mutation")
	flags.StringVar(&opts.output, "output", defaultOutputFormat, "Output format: json or text")
	flags.BoolVar(&opts.verbose, "verbose", false, "Include extra fields in text output")
	if err := flags.Parse(args); err != nil {
		return applyOptions{}, err
	}

	if strings.TrimSpace(opts.dbPath) == "" {
		fmt.Fprintln(stderr, "error=missing required flag: --db")
		return applyOptions{}, errors.New("missing db path")
	}
	if strings.TrimSpace(opts.ownerKey) == "" {
		fmt.Fprintln(stderr, "error=missing required flag: --owner-key")
		return applyOptions{}, errors.New("missing owner key")
	}
	if strings.TrimSpace(opts.appID) == "" {
		fmt.Fprintln(stderr, "error=missing required flag: --app-id")
		return applyOptions{}, errors.New("missing app id")
	}
	if opts.observationID <= 0 {
		fmt.Fprintln(stderr, "error=missing required flag: --observation-id")
		return applyOptions{}, errors.New("missing observation id")
	}
	opts.output = strings.ToLower(strings.TrimSpace(opts.output))
	if opts.output != "json" && opts.output != "text" {
		fmt.Fprintf(stderr, "error=unsupported output format: %s\n", opts.output)
		return applyOptions{}, errors.New("unsupported output format")
	}
	if flags.NArg() > 0 {
		fmt.Fprintf(stderr, "error=unexpected arguments: %s\n", strings.Join(flags.Args(), " "))
		return applyOptions{}, errors.New("unexpected arguments")
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

func validateApplyConfirmation(opts applyOptions) []string {
	reasons := make([]string, 0, 4)
	if !opts.iUnderstandThisMutatesCanonicalState {
		reasons = append(reasons, "missing_danger_confirmation")
	}
	if strings.TrimSpace(opts.confirmOwnerKey) == "" {
		reasons = append(reasons, "missing_confirm_owner_key")
	} else if opts.confirmOwnerKey != opts.ownerKey {
		reasons = append(reasons, "mismatched_confirm_owner_key")
	}
	if strings.TrimSpace(opts.confirmAppID) == "" {
		reasons = append(reasons, "missing_confirm_app_id")
	} else if opts.confirmAppID != opts.appID {
		reasons = append(reasons, "mismatched_confirm_app_id")
	}
	if opts.confirmObservationID == 0 {
		reasons = append(reasons, "missing_confirm_observation_id")
	} else if opts.confirmObservationID != opts.observationID {
		reasons = append(reasons, "mismatched_confirm_observation_id")
	}
	return reasons
}

func evaluatePreflightWithStore(ctx context.Context, sqliteStore replayAdminStore, ownerKey, appID string, observationID int64) (replayPreflightEvaluation, error) {
	compareEvaluation, err := sqliteStore.EvaluateSyncMutationReplayCompareAndApplyPreconditions(ctx, ownerKey, appID, observationID)
	if err != nil {
		return replayPreflightEvaluation{}, fmt.Errorf("compare_and_apply_evaluation_failed: %w", err)
	}

	recoveryEvaluation, err := sqliteStore.EvaluateSyncMutationReplayRecoveryState(ctx, ownerKey, appID, observationID)
	if err != nil {
		return replayPreflightEvaluation{}, fmt.Errorf("recovery_evaluation_failed: %w", err)
	}

	appliedMutationIDs := cloneStrings(recoveryEvaluation.AppliedMutationIDs)
	if len(appliedMutationIDs) == 0 {
		appliedMutationIDs = cloneStrings(compareEvaluation.AppliedMutationIDs)
	}

	return replayPreflightEvaluation{
		Status:                      derivePreflightStatus(compareEvaluation, recoveryEvaluation),
		CompareAndApplyStatus:       compareEvaluation.Status,
		RecoveryStatus:              recoveryEvaluation.Status,
		Reasons:                     mergeUniqueStrings(compareEvaluation.Reasons, recoveryEvaluation.Reasons),
		Warnings:                    mergeUniqueStrings(compareEvaluation.Warnings, recoveryEvaluation.Warnings),
		AppliedMutationIDs:          appliedMutationIDs,
		MatchingApplicationRowCount: recoveryEvaluation.MatchingApplicationRowCount,
	}, nil
}

func populatePreflightResultFromEvaluation(result *preflightResult, evaluation replayPreflightEvaluation) {
	result.Status = evaluation.Status
	result.CompareAndApplyStatus = evaluation.CompareAndApplyStatus
	result.RecoveryStatus = evaluation.RecoveryStatus
	result.Reasons = cloneStrings(evaluation.Reasons)
	result.Warnings = cloneStrings(evaluation.Warnings)
	result.AppliedMutationIDs = cloneStrings(evaluation.AppliedMutationIDs)
	result.MatchingApplicationRowCount = evaluation.MatchingApplicationRowCount
	result.CanonicalStateChanged = false
	result.DocumentVersionAdvanced = false
	result.ApplicationRowsInserted = false
}

func populateApplyResultFromPreflight(result *applyResult, evaluation replayPreflightEvaluation) {
	result.PreflightStatus = evaluation.Status
	result.CompareAndApplyStatus = evaluation.CompareAndApplyStatus
	result.RecoveryStatus = evaluation.RecoveryStatus
	result.Reasons = cloneStrings(evaluation.Reasons)
	result.Warnings = cloneStrings(evaluation.Warnings)
}

func populateApplyResultFromStore(result *applyResult, storeResult store.SyncMutationReplayAuthoritativeApplyResult) {
	if storeResult.CanonicalDocumentVersionBefore > 0 {
		result.CanonicalDocumentVersionBefore = int64Pointer(storeResult.CanonicalDocumentVersionBefore)
	}
	if strings.TrimSpace(storeResult.CanonicalDocumentHashBefore) != "" {
		result.CanonicalDocumentHashBefore = stringPointer(storeResult.CanonicalDocumentHashBefore)
	}
	result.CanonicalDocumentVersionAfter = storeResult.CanonicalDocumentVersionAfter
	result.CanonicalDocumentHashAfter = storeResult.CanonicalDocumentHashAfter
	result.InsertedApplicationRowCount = storeResult.InsertedApplicationRowCount
	result.MutationResults = cloneMutationResults(storeResult.MutationResults)
	result.CanonicalStateChanged = pointersDiffer(result.CanonicalDocumentHashBefore, result.CanonicalDocumentHashAfter)
	result.DocumentVersionAdvanced = pointersDifferInt64(result.CanonicalDocumentVersionBefore, result.CanonicalDocumentVersionAfter)
	result.ApplicationRowsInserted = result.InsertedApplicationRowCount > 0
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

func mapPreflightStatusToApplyStatus(status string) string {
	switch status {
	case preflightStatusIdempotent:
		return applyStatusIdempotent
	case preflightStatusMissingObservation:
		return applyStatusMissingObservation
	case preflightStatusInvalidScope:
		return applyStatusInvalidScope
	case preflightStatusStaleCanonical:
		return applyStatusStaleCanonical
	case preflightStatusStaleReceiptSet:
		return applyStatusStaleReceiptSet
	case preflightStatusBlockedSnapshot:
		return applyStatusBlockedSnapshot
	case preflightStatusPartialRows:
		return applyStatusPartialRows
	case preflightStatusRowsMismatch:
		return applyStatusRowsMismatch
	case preflightStatusCanonicalMismatch:
		return applyStatusCanonicalMismatch
	default:
		return applyStatusInternalError
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

func writeApplyResult(stdout io.Writer, output string, verbose bool, result applyResult) {
	if output == "text" {
		writeTextApplyResult(stdout, verbose, result)
		return
	}

	encoder := json.NewEncoder(stdout)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(result)
}

func writeTextApplyResult(stdout io.Writer, verbose bool, result applyResult) {
	fmt.Fprintf(stdout, "mode=%s\n", result.Mode)
	fmt.Fprintf(stdout, "status=%s\n", result.Status)
	fmt.Fprintf(stdout, "owner_key=%s\n", result.OwnerKey)
	fmt.Fprintf(stdout, "app_id=%s\n", result.AppID)
	fmt.Fprintf(stdout, "observation_id=%d\n", result.ObservationID)
	if result.CanonicalDocumentVersionBefore != nil {
		fmt.Fprintf(stdout, "canonical_document_version_before=%d\n", *result.CanonicalDocumentVersionBefore)
	}
	if result.CanonicalDocumentVersionAfter != nil {
		fmt.Fprintf(stdout, "canonical_document_version_after=%d\n", *result.CanonicalDocumentVersionAfter)
	}
	fmt.Fprintf(stdout, "reasons=%s\n", strings.Join(result.Reasons, ","))
	fmt.Fprintf(stdout, "warnings=%s\n", strings.Join(result.Warnings, ","))
	if verbose {
		fmt.Fprintf(stdout, "preflight_status=%s\n", result.PreflightStatus)
		fmt.Fprintf(stdout, "compare_and_apply_status=%s\n", result.CompareAndApplyStatus)
		fmt.Fprintf(stdout, "recovery_status=%s\n", result.RecoveryStatus)
		fmt.Fprintf(stdout, "inserted_application_row_count=%d\n", result.InsertedApplicationRowCount)
		fmt.Fprintf(stdout, "canonical_state_changed=%t\n", result.CanonicalStateChanged)
		fmt.Fprintf(stdout, "document_version_advanced=%t\n", result.DocumentVersionAdvanced)
		fmt.Fprintf(stdout, "application_rows_inserted=%t\n", result.ApplicationRowsInserted)
		fmt.Fprintf(stdout, "mutation_results=%s\n", formatMutationResults(result.MutationResults))
	}
}

func writePreflightAuditLine(stderr io.Writer, result preflightResult) {
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

func writeApplyAuditLine(stderr io.Writer, result applyResult) {
	fmt.Fprintf(
		stderr,
		"ts=%s mode=%s owner_key=%s app_id=%s observation_id=%d status=%s preflight_status=%s compare_and_apply_status=%s recovery_status=%s canonical_document_version_before=%s canonical_document_version_after=%s canonical_document_hash_before=%s canonical_document_hash_after=%s inserted_application_row_count=%d reasons_count=%d warnings_count=%d\n",
		time.Now().UTC().Format(time.RFC3339),
		result.Mode,
		result.OwnerKey,
		result.AppID,
		result.ObservationID,
		result.Status,
		result.PreflightStatus,
		result.CompareAndApplyStatus,
		result.RecoveryStatus,
		formatOptionalInt64(result.CanonicalDocumentVersionBefore),
		formatOptionalInt64(result.CanonicalDocumentVersionAfter),
		formatOptionalString(result.CanonicalDocumentHashBefore),
		formatOptionalString(result.CanonicalDocumentHashAfter),
		result.InsertedApplicationRowCount,
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

func cloneMutationResults(values []store.SyncMutationReplayAuthoritativeMutationResult) []store.SyncMutationReplayAuthoritativeMutationResult {
	if len(values) == 0 {
		return []store.SyncMutationReplayAuthoritativeMutationResult{}
	}
	return append([]store.SyncMutationReplayAuthoritativeMutationResult{}, values...)
}

func extractMutationReasons(values []store.SyncMutationReplayAuthoritativeMutationResult) []string {
	reasons := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value.ApplicationReason) != "" {
			reasons = append(reasons, value.ApplicationReason)
		}
	}
	return reasons
}

func formatMutationResults(values []store.SyncMutationReplayAuthoritativeMutationResult) string {
	if len(values) == 0 {
		return ""
	}
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, fmt.Sprintf("%s:%s:%s", value.MutationID, value.ApplicationStatus, value.ApplicationReason))
	}
	return strings.Join(parts, ",")
}

func formatOptionalInt64(value *int64) string {
	if value == nil {
		return ""
	}
	return fmt.Sprintf("%d", *value)
}

func formatOptionalString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func int64Pointer(value int64) *int64 {
	return &value
}

func stringPointer(value string) *string {
	return &value
}

func pointersDiffer(left, right *string) bool {
	if left == nil || right == nil {
		return false
	}
	return *left != *right
}

func pointersDifferInt64(left, right *int64) bool {
	if left == nil || right == nil {
		return false
	}
	return *left != *right
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "usage:")
	fmt.Fprintln(w, "  postbaby-sync-replay-admin preflight --db PATH --owner-key OWNER --app-id APP --observation-id ID [--output json|text] [--verbose]")
	fmt.Fprintln(w, "  postbaby-sync-replay-admin apply --db PATH --owner-key OWNER --confirm-owner-key OWNER --app-id APP --confirm-app-id APP --observation-id ID --confirm-observation-id ID --i-understand-this-mutates-canonical-state [--output json|text] [--verbose]")
}

func withSuppressedStandardLogger(run func() int) int {
	standardLoggerMu.Lock()
	defer standardLoggerMu.Unlock()

	previousWriter := log.Writer()
	log.SetOutput(io.Discard)
	defer log.SetOutput(previousWriter)

	return run()
}
