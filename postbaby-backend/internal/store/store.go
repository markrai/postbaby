package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	_ "modernc.org/sqlite"
)

var (
	ErrDocumentNotFound                              = errors.New("document not found")
	ErrVersionConflict                               = errors.New("version conflict")
	ErrSyncMutationReplayDryRunObservationNotFound   = errors.New("sync mutation replay dry-run observation not found")
	ErrSyncMutationReplayAuthoritativeApplyFailpoint = errors.New("sync mutation replay authoritative apply failpoint")
)

const (
	dbOperationTimeout                = 8 * time.Second
	dbStartupTimeout                  = 10 * time.Second
	slowDBOperationThreshold          = 750 * time.Millisecond
	SyncMutationReceiptStatusAccepted = "accepted"
	replayTabsSnapshotKey             = "tabs"
	replayCoordMin                    = -100000
	replayCoordMax                    = 100000
	replayItemsPerTabMax              = 500
	replayEdgesPerTabMax              = 2000
	replayItemTextCharsMax            = 4000
	replayItemWidthMin                = 120
	replayItemWidthMax                = 600
	replayItemHeightMin               = 80
	replayItemHeightMax               = 600
	replayWarningMissingTab           = "missing_tab"
	replayWarningInvalidPayload       = "invalid_payload"
	replayWarningInvalidPosition      = "invalid_position"
	replayWarningInvalidUpdateValue   = "invalid_update_value"
	replayWarningUnknownUpdateField   = "unknown_update_field"
	replayWarningSkippedMissingNode   = "skipped_missing_node"
	replayWarningSkippedMissingEdge   = "skipped_missing_edge"
	replayWarningSkippedMissingEP     = "skipped_missing_endpoint"
	replayWarningUnknownOperation     = "unknown_operation"
	replayWarningMissingEntityID      = "missing_entity_id"
	replayWarningSelfEdge             = "self_edge"

	DefaultSyncMutationReplayObservationRetention     = 720 * time.Hour
	DefaultSyncMutationReplayReceiptRetention         = 2160 * time.Hour
	DefaultSyncMutationReplayRetainedObservationCount = 5
	DefaultSyncMutationReplayRetainedReceiptCount     = 100
)

var replayItemShapeSet = map[string]struct{}{
	"default":            {},
	"circle":             {},
	"square":             {},
	"triangle":           {},
	"diamond":            {},
	"upsideDownTriangle": {},
	"hexagon":            {},
	"oval":               {},
}

var replayFixedRatioShapeSet = map[string]struct{}{
	"circle":             {},
	"square":             {},
	"triangle":           {},
	"diamond":            {},
	"upsideDownTriangle": {},
	"hexagon":            {},
}

type VersionConflictError struct {
	CurrentVersion *int64
}

func (e *VersionConflictError) Error() string {
	return ErrVersionConflict.Error()
}

func (e *VersionConflictError) Unwrap() error {
	return ErrVersionConflict
}

type Store struct {
	db          *sql.DB
	dbPath      string
	journalMode string
}

type Document struct {
	ID        int64
	OwnerKey  string
	AppID     string
	Body      json.RawMessage
	Version   int64
	UpdatedAt time.Time
}

type DocumentMeta struct {
	AppID     string
	Version   int64
	UpdatedAt time.Time
}

type SyncMutationDryRunResult struct {
	SourceVersion     int64
	ConsideredCount   int
	AppliedCount      int
	SkippedCount      int
	WarningCount      int
	OrderedMutationID []string
	PreviewBody       json.RawMessage
	MutationResults   []SyncMutationDryRunMutationResult
	Warnings          []SyncMutationDryRunWarning
}

type SyncMutationDryRunMutationResult struct {
	MutationID    string
	OperationType string
	Outcome       string
}

type SyncMutationDryRunWarning struct {
	MutationID string
	Code       string
	Message    string
}

type SyncMutationReplayDryRunObservation struct {
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
	CreatedAt                        time.Time
}

type SyncMutationReplayApplicationInput struct {
	MutationID                     string
	ApplicationStatus              string
	ApplicationReason              string
	CanonicalDocumentVersionBefore int64
	CanonicalDocumentHashBefore    string
	CanonicalDocumentVersionAfter  *int64
	CanonicalDocumentHashAfter     *string
	ReplayObservationID            *int64
}

type SyncMutationReplayApplication struct {
	ID                             int64
	OwnerKey                       string
	AppID                          string
	MutationID                     string
	ApplicationStatus              string
	ApplicationReason              string
	CanonicalDocumentVersionBefore int64
	CanonicalDocumentHashBefore    string
	CanonicalDocumentVersionAfter  *int64
	CanonicalDocumentHashAfter     *string
	ReplayObservationID            *int64
	CreatedAt                      time.Time
}

type SyncMutationReplayApplicationResult struct {
	Application SyncMutationReplayApplication
	Duplicate   bool
}

type SyncMutationReplayDiagnostics struct {
	OwnerKey                   string
	AppID                      string
	ReceiptCount               int64
	ReceiptOldestCreatedAt     *time.Time
	ReceiptNewestCreatedAt     *time.Time
	ReceiptOldestAcceptedAt    *time.Time
	ReceiptNewestAcceptedAt    *time.Time
	ReceiptPayloadBytes        int64
	ObservationCount           int64
	ObservationOldestCreatedAt *time.Time
	ObservationNewestCreatedAt *time.Time
	LatestObservation          *SyncMutationReplayDryRunObservation
	ApplicationCount           int64
	ApplicationOldestCreatedAt *time.Time
	ApplicationNewestCreatedAt *time.Time
	ApplicationStatusCounts    map[string]int64
	DBFileBytes                *int64
}

type SyncMutationReplayCompactOptions struct {
	Now                      time.Time
	ObservationRetention     time.Duration
	ReceiptRetention         time.Duration
	RetainedObservationCount int
	RetainedReceiptCount     int
	Execute                  bool
}

type SyncMutationReplayCompactResult struct {
	OwnerKey                        string
	AppID                           string
	Execute                         bool
	ObservationRetention            time.Duration
	ReceiptRetention                time.Duration
	ObservationCutoff               time.Time
	ReceiptCutoff                   time.Time
	RetainedObservationCount        int
	RetainedReceiptCount            int
	ProtectedReceiptCount           int
	RetainedApplicationCount        int64
	CandidateObservationDeleteCount int64
	CandidateReceiptDeleteCount     int64
	DeletedObservationCount         int64
	DeletedReceiptCount             int64
}

type SyncMutationReplayCompareAndApplyPreconditions struct {
	OwnerKey                         string
	AppID                            string
	ReplayObservationID              int64
	ExpectedCanonicalDocumentVersion int64
	ExpectedCanonicalDocumentHash    string
	ExpectedReceiptCount             int
	ExpectedFirstMutationID          string
	ExpectedLastMutationID           string
	ExpectedReceiptHighWatermark     string
	ExpectedPreviewHash              string
}

type SyncMutationReplayCompareAndApplyEvaluation struct {
	Status                      string
	AllowedForFutureTransaction bool
	Preconditions               SyncMutationReplayCompareAndApplyPreconditions
	Reasons                     []string
	Warnings                    []string
	AppliedMutationIDs          []string
}

type SyncMutationReplayRecoveryEvaluation struct {
	Status                      string
	Reasons                     []string
	Warnings                    []string
	CompareAndApplyEvaluation   SyncMutationReplayCompareAndApplyEvaluation
	AppliedMutationIDs          []string
	MatchingApplicationRowCount int
}

type SyncMutationReplayAuthoritativeReadiness struct {
	Ready    bool
	Areas    []SyncMutationReplayAuthoritativeAreaReadiness
	Blockers []string
	Warnings []string
}

type SyncMutationReplayAuthoritativeAreaReadiness struct {
	Area   string
	Status string
	Detail string
}

type SyncMutationReplayAuthoritativePolicyEvaluation struct {
	Status   string
	Reasons  []string
	Warnings []string
}

type SyncMutationReplayAuthoritativePreview struct {
	Status                                string
	DryRunPreviewHash                     string
	AuthoritativePreviewHash              string
	AuthoritativePreviewHashMatchesDryRun bool
	ApplicationStatusCounts               map[string]int64
	MutationResults                       []SyncMutationReplayAuthoritativeMutationResult
	PolicyAbort                           bool
	PolicyAbortMutationID                 string
	PolicyAbortOperationType              string
	PolicyAbortStatus                     string
	PolicyAbortReasons                    []string
	Reasons                               []string
	Warnings                              []string
	CompareAndApplyStatus                 string
	RecoveryStatus                        string
}

type SyncMutationReplayAuthoritativeApplyOptions struct {
	AllowInternalAuthoritativeReplay bool
	FailAfterApplicationRowInserts   *int
}

type SyncMutationReplayAuthoritativeMutationResult struct {
	MutationID        string
	OperationType     string
	ApplicationStatus string
	ApplicationReason string
}

type SyncMutationReplayAuthoritativeApplyResult struct {
	Status                         string
	CanonicalDocumentVersionBefore int64
	CanonicalDocumentVersionAfter  *int64
	CanonicalDocumentHashBefore    string
	CanonicalDocumentHashAfter     *string
	InsertedApplicationRowCount    int
	MutationResults                []SyncMutationReplayAuthoritativeMutationResult
}

type SyncDeltaMetadataOptions struct {
	SinceVersion         int64
	ApplicationWatermark *int64
	IncludeApplications  bool
	Limit                int
}

type SyncDeltaMetadata struct {
	AppID                    string
	CurrentDocumentVersion   int64
	CurrentDocumentHash      string
	ClientVersion            int64
	RequiresSnapshotRefresh  bool
	Reason                   string
	ApplicationWatermark     *int64
	NextApplicationWatermark *int64
	Applications             []SyncDeltaApplicationMetadata
	Warnings                 []string
}

type SyncDeltaApplicationMetadata struct {
	MutationID                     string
	ApplicationStatus              string
	ApplicationReason              string
	CanonicalDocumentVersionBefore int64
	CanonicalDocumentVersionAfter  *int64
	CanonicalDocumentHashBefore    string
	CanonicalDocumentHashAfter     *string
	ReplayObservationID            *int64
	CreatedAt                      time.Time
}

type syncMutationReplayAuthoritativeStagedApplication struct {
	MutationID        string
	OperationType     string
	ApplicationStatus string
	ApplicationReason string
}

type syncMutationReplayAuthoritativeProgressivePreview struct {
	PreviewBody              json.RawMessage
	MutationResults          []SyncMutationReplayAuthoritativeMutationResult
	StagedApplications       []syncMutationReplayAuthoritativeStagedApplication
	Warnings                 []string
	PolicyAbort              bool
	PolicyAbortMutationID    string
	PolicyAbortOperationType string
	PolicyAbortStatus        string
	PolicyAbortReasons       []string
}

type replaySnapshotPolicyAnalysis struct {
	DuplicateItemIDs         bool
	DuplicateEdgeIDs         bool
	MalformedLegacyEdges     bool
	UnknownLegacyShapes      bool
	MalformedLegacyPositions bool
}

type replaySnapshot map[string]string

type replayTab struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Items       []replayItem `json:"items"`
	ColorIndex  int          `json:"colorIndex"`
	GridSetting string       `json:"gridSetting"`
	Edges       []replayEdge `json:"edges"`
}

type replayItem struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Color    string         `json:"color"`
	Position replayPosition `json:"position"`
	Shape    string         `json:"shape,omitempty"`
	Width    *float64       `json:"width,omitempty"`
	Height   *float64       `json:"height,omitempty"`
}

type replayPosition struct {
	Top  string `json:"top"`
	Left string `json:"left"`
}

type replayEdge struct {
	ID         string `json:"id"`
	FromItemID string `json:"fromItemId"`
	ToItemID   string `json:"toItemId"`
	Kind       string `json:"kind,omitempty"`
}

const (
	replayAuthoritativeStatusReady                                               = "ready"
	replayAuthoritativeStatusPartiallyReady                                      = "partially_ready"
	replayAuthoritativeStatusBlocked                                             = "blocked"
	replayAuthoritativeStatusIntentionallyDeferred                               = "intentionally_deferred"
	replayAuthoritativePolicyStatusAllowed                                       = "allowed"
	replayAuthoritativePolicyStatusSkip                                          = "skip"
	replayAuthoritativePolicyStatusConflict                                      = "conflict"
	replayAuthoritativePolicyStatusBlocked                                       = "blocked"
	replayAuthoritativePolicyStatusFatal                                         = "fatal"
	SyncMutationReplayApplicationStatusApplied                                   = "authoritativeApplied"
	SyncMutationReplayApplicationStatusSkipped                                   = "authoritativeSkipped"
	SyncMutationReplayApplicationStatusConflict                                  = "authoritativeConflict"
	SyncMutationReplayApplicationStatusFailed                                    = "authoritativeFailed"
	SyncMutationReplayCompareAndApplyStatusAllowed                               = "allowed_for_future_transaction"
	SyncMutationReplayCompareAndApplyStatusStaleCanonicalDocument                = "stale_canonical_document"
	SyncMutationReplayCompareAndApplyStatusStaleReceiptSet                       = "stale_receipt_set"
	SyncMutationReplayCompareAndApplyStatusBlockedSnapshot                       = "blocked_snapshot"
	SyncMutationReplayCompareAndApplyStatusAlreadyApplied                        = "already_applied"
	SyncMutationReplayCompareAndApplyStatusMissingObservation                    = "missing_observation"
	SyncMutationReplayCompareAndApplyStatusInvalidObservationScope               = "invalid_observation_scope"
	SyncMutationReplayRecoveryStatusSafeToAttemptTransaction                     = "safe_to_attempt_transaction"
	SyncMutationReplayRecoveryStatusStaleObservationRequiresRedryrun             = "stale_observation_requires_redryrun"
	SyncMutationReplayRecoveryStatusBlockedSnapshotRequiresCleanup               = "blocked_snapshot_requires_cleanup"
	SyncMutationReplayRecoveryStatusAlreadyAppliedRequiresIdempotentExit         = "already_applied_requires_idempotent_exit"
	SyncMutationReplayRecoveryStatusPartialApplicationRows                       = "partial_application_rows"
	SyncMutationReplayRecoveryStatusApplicationRowsWithoutMatchingCanonicalState = "application_rows_without_matching_canonical_state"
	SyncMutationReplayRecoveryStatusCanonicalStateWithoutApplicationRows         = "canonical_state_without_application_rows"
	SyncMutationReplayRecoveryStatusMissingObservation                           = "missing_observation"
	SyncMutationReplayRecoveryStatusInvalidObservationScope                      = "invalid_observation_scope"
	SyncMutationReplayAuthoritativePreviewStatusAvailable                        = "available"
	SyncMutationReplayAuthoritativePreviewStatusUnavailablePreconditions         = "unavailable_preconditions"
	SyncMutationReplayAuthoritativePreviewStatusWouldAbortPolicy                 = "would_abort_policy"
	SyncMutationReplayAuthoritativeApplyStatusRefusedInternalGate                = "refused_internal_gate"
	SyncMutationReplayAuthoritativeApplyStatusApplied                            = "applied"
	SyncMutationReplayAuthoritativeApplyStatusAbortedPreconditions               = "aborted_preconditions"
	SyncMutationReplayAuthoritativeApplyStatusAbortedRecovery                    = "aborted_recovery"
	SyncMutationReplayAuthoritativeApplyStatusAbortedPolicy                      = "aborted_policy"
	SyncMutationReplayAuthoritativeApplyStatusIdempotentExitAlreadyApplied       = "idempotent_exit_already_applied"
	SyncDeltaMetadataReasonUpToDate                                              = "up_to_date"
	SyncDeltaMetadataReasonDocumentVersionChanged                                = "document_version_changed"
	SyncDeltaMetadataReasonApplicationsAvailable                                 = "applications_available"
	SyncDeltaMetadataReasonSnapshotRequiredClientVersionAhead                    = "snapshot_required_client_version_ahead"
	SyncDeltaMetadataReasonSnapshotRequiredStaleVersion                          = "snapshot_required_stale_version"
	SyncDeltaMetadataReasonSnapshotRequiredTooManyApplications                   = "snapshot_required_too_many_applications"
	syncDeltaMetadataWarningVersionAdvancedWithoutBodyChange                     = "document_version_advanced_without_body_change"
	syncDeltaMetadataDefaultLimit                                                = 100
	syncDeltaMetadataMaxLimit                                                    = 1000
)

var syncMutationReplayApplicationStatusSet = map[string]struct{}{
	SyncMutationReplayApplicationStatusApplied:  {},
	SyncMutationReplayApplicationStatusSkipped:  {},
	SyncMutationReplayApplicationStatusConflict: {},
	SyncMutationReplayApplicationStatusFailed:   {},
}

func Open(dbPath string) (*Store, error) {
	openStarted := time.Now()
	log.Printf("db_open_start path=%q", dbPath)

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	store := &Store{db: db, dbPath: dbPath}
	startupCtx, cancel := context.WithTimeout(context.Background(), dbStartupTimeout)
	pragmaStatus, err := store.applyPragmas(startupCtx)
	cancel()
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	store.journalMode = pragmaStatus.journalMode
	log.Printf(
		"db_startup path=%q wal_enabled=%t journal_mode=%q",
		dbPath,
		pragmaStatus.walEnabled,
		pragmaStatus.journalMode,
	)

	if err := store.init(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}

	log.Printf("db_open_done path=%q elapsed_ms=%d", dbPath, time.Since(openStarted).Milliseconds())
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Health(ctx context.Context) error {
	started := time.Now()
	ctx, cancel := withDBTimeout(ctx)
	defer cancel()

	var value int
	err := s.db.QueryRowContext(ctx, `SELECT 1`).Scan(&value)
	if err != nil {
		return s.wrapDBError("health_check", started, err)
	}

	s.logDBOperation("health_check", started, nil)
	return nil
}

func (s *Store) GetDocument(ctx context.Context, ownerKey, appID string) (Document, error) {
	started := time.Now()
	ctx, cancel := withDBTimeout(ctx)
	defer cancel()

	row := s.db.QueryRowContext(
		ctx,
		`SELECT id, owner_key, app_id, body_json, version, updated_at
		FROM documents
		WHERE owner_key = ? AND app_id = ?`,
		ownerKey,
		appID,
	)

	doc, err := scanDocument(row)
	if err != nil {
		if errors.Is(err, ErrDocumentNotFound) {
			return Document{}, err
		}
		return Document{}, s.wrapDBError("read_document", started, err)
	}

	s.logDBOperation("read_document", started, nil)
	return doc, nil
}

func (s *Store) GetDocumentMeta(ctx context.Context, ownerKey, appID string) (DocumentMeta, error) {
	started := time.Now()
	ctx, cancel := withDBTimeout(ctx)
	defer cancel()

	row := s.db.QueryRowContext(
		ctx,
		`SELECT app_id, version, updated_at
		FROM documents
		WHERE owner_key = ? AND app_id = ?`,
		ownerKey,
		appID,
	)

	var meta DocumentMeta
	var updatedAt string
	if err := row.Scan(&meta.AppID, &meta.Version, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return DocumentMeta{}, ErrDocumentNotFound
		}
		return DocumentMeta{}, s.wrapDBError("read_document_meta", started, fmt.Errorf("scan document meta: %w", err))
	}

	parsedUpdatedAt, err := time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		return DocumentMeta{}, fmt.Errorf("parse meta updated_at: %w", err)
	}

	meta.UpdatedAt = parsedUpdatedAt
	s.logDBOperation("read_document_meta", started, nil)
	return meta, nil
}

func (s *Store) PutDocument(ctx context.Context, ownerKey, appID string, body json.RawMessage, expectedVersion *int64) (Document, error) {
	started := time.Now()
	ctx, cancel := withDBTimeout(ctx)
	defer cancel()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Document{}, s.wrapDBError("write_begin_transaction", started, fmt.Errorf("begin transaction: %w", err))
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	var currentID int64
	var currentVersion int64
	err = tx.QueryRowContext(
		ctx,
		`SELECT id, version FROM documents WHERE owner_key = ? AND app_id = ?`,
		ownerKey,
		appID,
	).Scan(&currentID, &currentVersion)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Document{}, s.wrapDBError("write_load_current_document", started, fmt.Errorf("load current document: %w", err))
	}

	updatedAt := time.Now().UTC().Format(time.RFC3339)

	if errors.Is(err, sql.ErrNoRows) {
		if expectedVersion != nil && *expectedVersion != 0 {
			return Document{}, &VersionConflictError{}
		}

		result, execErr := tx.ExecContext(
			ctx,
			`INSERT INTO documents (owner_key, app_id, body_json, version, updated_at)
			VALUES (?, ?, ?, ?, ?)`,
			ownerKey,
			appID,
			string(body),
			1,
			updatedAt,
		)
		if execErr != nil {
			return Document{}, s.wrapDBError("write_insert_document", started, fmt.Errorf("insert document: %w", execErr))
		}

		id, execErr := result.LastInsertId()
		if execErr != nil {
			return Document{}, s.wrapDBError("write_insert_last_id", started, fmt.Errorf("read inserted id: %w", execErr))
		}

		if execErr := tx.Commit(); execErr != nil {
			return Document{}, s.wrapDBError("write_commit_insert", started, fmt.Errorf("commit insert: %w", execErr))
		}
		committed = true
		s.logDBOperation("write_insert_document", started, nil)

		return Document{
			ID:        id,
			OwnerKey:  ownerKey,
			AppID:     appID,
			Body:      cloneJSON(body),
			Version:   1,
			UpdatedAt: mustParseTimestamp(updatedAt),
		}, nil
	}

	if expectedVersion != nil && *expectedVersion != currentVersion {
		currentVersionCopy := currentVersion
		return Document{}, &VersionConflictError{CurrentVersion: &currentVersionCopy}
	}

	newVersion := currentVersion + 1
	updateQuery := `UPDATE documents SET body_json = ?, version = ?, updated_at = ? WHERE id = ?`
	args := []any{string(body), newVersion, updatedAt, currentID}
	if expectedVersion != nil {
		updateQuery = `UPDATE documents SET body_json = ?, version = ?, updated_at = ? WHERE id = ? AND version = ?`
		args = append(args, currentVersion)
	}

	result, execErr := tx.ExecContext(ctx, updateQuery, args...)
	if execErr != nil {
		return Document{}, s.wrapDBError("write_update_document", started, fmt.Errorf("update document: %w", execErr))
	}

	if expectedVersion != nil {
		rowsAffected, rowsErr := result.RowsAffected()
		if rowsErr != nil {
			return Document{}, s.wrapDBError("write_update_rows_affected", started, fmt.Errorf("read update result: %w", rowsErr))
		}
		if rowsAffected != 1 {
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				return Document{}, s.wrapDBError("write_rollback_conflict_update", started, fmt.Errorf("rollback conflict update: %w", rollbackErr))
			}
			currentVersionPtr, lookupErr := s.getCurrentVersion(ctx, ownerKey, appID)
			if lookupErr != nil && !errors.Is(lookupErr, ErrDocumentNotFound) {
				return Document{}, s.wrapDBError("write_load_current_version_after_conflict", started, fmt.Errorf("load current version after conflict: %w", lookupErr))
			}
			s.logDBOperation("write_versioned_update_conflict", started, nil)
			return Document{}, &VersionConflictError{CurrentVersion: currentVersionPtr}
		}
	}

	if execErr := tx.Commit(); execErr != nil {
		return Document{}, s.wrapDBError("write_commit_update", started, fmt.Errorf("commit update: %w", execErr))
	}
	committed = true
	s.logDBOperation("write_versioned_update", started, nil)

	return Document{
		ID:        currentID,
		OwnerKey:  ownerKey,
		AppID:     appID,
		Body:      cloneJSON(body),
		Version:   newVersion,
		UpdatedAt: mustParseTimestamp(updatedAt),
	}, nil
}

func (s *Store) AcceptSyncMutationReceipts(ctx context.Context, ownerKey, appID string, receipts []SyncMutationReceiptInput) ([]SyncMutationReceiptResult, error) {
	started := time.Now()
	ctx, cancel := withDBTimeout(ctx)
	defer cancel()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, s.wrapDBError("write_begin_sync_mutation_receipts", started, fmt.Errorf("begin transaction: %w", err))
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	results := make([]SyncMutationReceiptResult, 0, len(receipts))
	for _, receiptInput := range receipts {
		acceptedAt := time.Now().UTC().Format(time.RFC3339)
		payloadJSON := string(cloneJSON(receiptInput.Payload))
		var baseRevisionValue any
		if receiptInput.BaseRevision != nil {
			baseRevisionValue = *receiptInput.BaseRevision
		}

		insertResult, execErr := tx.ExecContext(
			ctx,
			`INSERT OR IGNORE INTO sync_mutation_receipts (
				owner_key,
				app_id,
				mutation_id,
				client_id,
				device_id,
				protocol,
				entity_type,
				entity_id,
				operation_type,
				payload_json,
				base_revision,
				status,
				created_at,
				accepted_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			ownerKey,
			appID,
			receiptInput.MutationID,
			receiptInput.ClientID,
			receiptInput.DeviceID,
			receiptInput.Protocol,
			receiptInput.EntityType,
			receiptInput.EntityID,
			receiptInput.OperationType,
			payloadJSON,
			baseRevisionValue,
			SyncMutationReceiptStatusAccepted,
			acceptedAt,
			acceptedAt,
		)
		if execErr != nil {
			return nil, s.wrapDBError("write_insert_sync_mutation_receipt", started, fmt.Errorf("insert sync mutation receipt: %w", execErr))
		}

		rowsAffected, rowsErr := insertResult.RowsAffected()
		if rowsErr != nil {
			return nil, s.wrapDBError("write_sync_mutation_receipt_rows_affected", started, fmt.Errorf("read rows affected: %w", rowsErr))
		}

		storedReceipt, loadErr := scanSyncMutationReceipt(tx.QueryRowContext(
			ctx,
			`SELECT
				id,
				owner_key,
				app_id,
				mutation_id,
				client_id,
				device_id,
				protocol,
				entity_type,
				entity_id,
				operation_type,
				payload_json,
				base_revision,
				status,
				created_at,
				accepted_at
			FROM sync_mutation_receipts
			WHERE owner_key = ? AND app_id = ? AND mutation_id = ?`,
			ownerKey,
			appID,
			receiptInput.MutationID,
		))
		if loadErr != nil {
			return nil, s.wrapDBError("read_sync_mutation_receipt", started, loadErr)
		}

		results = append(results, SyncMutationReceiptResult{
			Receipt:   storedReceipt,
			Duplicate: rowsAffected == 0,
		})
	}

	if err := tx.Commit(); err != nil {
		return nil, s.wrapDBError("write_commit_sync_mutation_receipts", started, fmt.Errorf("commit sync mutation receipts: %w", err))
	}
	committed = true
	s.logDBOperation("write_accept_sync_mutation_receipts", started, nil)

	return results, nil
}

func (s *Store) CountSyncMutationReceipts(ctx context.Context, ownerKey, appID string) (int64, error) {
	started := time.Now()
	ctx, cancel := withDBTimeout(ctx)
	defer cancel()

	var count int64
	if err := s.db.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM sync_mutation_receipts WHERE owner_key = ? AND app_id = ?`,
		ownerKey,
		appID,
	).Scan(&count); err != nil {
		return 0, s.wrapDBError("count_sync_mutation_receipts", started, fmt.Errorf("count sync mutation receipts: %w", err))
	}

	s.logDBOperation("count_sync_mutation_receipts", started, nil)
	return count, nil
}

func (s *Store) GetSyncMutationReplayDiagnostics(ctx context.Context, ownerKey, appID string) (SyncMutationReplayDiagnostics, error) {
	started := time.Now()
	ctx, cancel := withDBTimeout(ctx)
	defer cancel()

	result := SyncMutationReplayDiagnostics{
		OwnerKey:                ownerKey,
		AppID:                   appID,
		ApplicationStatusCounts: map[string]int64{},
	}

	var receiptOldestCreatedAt sql.NullString
	var receiptNewestCreatedAt sql.NullString
	var receiptOldestAcceptedAt sql.NullString
	var receiptNewestAcceptedAt sql.NullString
	if err := s.db.QueryRowContext(
		ctx,
		`SELECT
			COUNT(*),
			MIN(created_at),
			MAX(created_at),
			MIN(accepted_at),
			MAX(accepted_at),
			COALESCE(SUM(LENGTH(payload_json)), 0)
		FROM sync_mutation_receipts
		WHERE owner_key = ? AND app_id = ?`,
		ownerKey,
		appID,
	).Scan(
		&result.ReceiptCount,
		&receiptOldestCreatedAt,
		&receiptNewestCreatedAt,
		&receiptOldestAcceptedAt,
		&receiptNewestAcceptedAt,
		&result.ReceiptPayloadBytes,
	); err != nil {
		return SyncMutationReplayDiagnostics{}, s.wrapDBError("read_sync_mutation_replay_diagnostics_receipts", started, fmt.Errorf("summarize sync mutation receipts: %w", err))
	}

	parsedReceiptOldestCreatedAt, err := parseNullableTimestamp(receiptOldestCreatedAt)
	if err != nil {
		return SyncMutationReplayDiagnostics{}, s.wrapDBError("read_sync_mutation_replay_diagnostics_receipts", started, err)
	}
	parsedReceiptNewestCreatedAt, err := parseNullableTimestamp(receiptNewestCreatedAt)
	if err != nil {
		return SyncMutationReplayDiagnostics{}, s.wrapDBError("read_sync_mutation_replay_diagnostics_receipts", started, err)
	}
	parsedReceiptOldestAcceptedAt, err := parseNullableTimestamp(receiptOldestAcceptedAt)
	if err != nil {
		return SyncMutationReplayDiagnostics{}, s.wrapDBError("read_sync_mutation_replay_diagnostics_receipts", started, err)
	}
	parsedReceiptNewestAcceptedAt, err := parseNullableTimestamp(receiptNewestAcceptedAt)
	if err != nil {
		return SyncMutationReplayDiagnostics{}, s.wrapDBError("read_sync_mutation_replay_diagnostics_receipts", started, err)
	}
	result.ReceiptOldestCreatedAt = parsedReceiptOldestCreatedAt
	result.ReceiptNewestCreatedAt = parsedReceiptNewestCreatedAt
	result.ReceiptOldestAcceptedAt = parsedReceiptOldestAcceptedAt
	result.ReceiptNewestAcceptedAt = parsedReceiptNewestAcceptedAt

	var observationOldestCreatedAt sql.NullString
	var observationNewestCreatedAt sql.NullString
	if err := s.db.QueryRowContext(
		ctx,
		`SELECT COUNT(*), MIN(created_at), MAX(created_at)
		FROM sync_mutation_replay_dry_run_observations
		WHERE owner_key = ? AND app_id = ?`,
		ownerKey,
		appID,
	).Scan(&result.ObservationCount, &observationOldestCreatedAt, &observationNewestCreatedAt); err != nil {
		return SyncMutationReplayDiagnostics{}, s.wrapDBError("read_sync_mutation_replay_diagnostics_observations", started, fmt.Errorf("summarize sync mutation replay observations: %w", err))
	}

	parsedObservationOldestCreatedAt, err := parseNullableTimestamp(observationOldestCreatedAt)
	if err != nil {
		return SyncMutationReplayDiagnostics{}, s.wrapDBError("read_sync_mutation_replay_diagnostics_observations", started, err)
	}
	parsedObservationNewestCreatedAt, err := parseNullableTimestamp(observationNewestCreatedAt)
	if err != nil {
		return SyncMutationReplayDiagnostics{}, s.wrapDBError("read_sync_mutation_replay_diagnostics_observations", started, err)
	}
	result.ObservationOldestCreatedAt = parsedObservationOldestCreatedAt
	result.ObservationNewestCreatedAt = parsedObservationNewestCreatedAt

	latestObservation, found, err := getLatestSyncMutationReplayDryRunObservationFromQuerier(ctx, s.db, ownerKey, appID)
	if err != nil {
		return SyncMutationReplayDiagnostics{}, s.wrapDBError("read_sync_mutation_replay_diagnostics_latest_observation", started, fmt.Errorf("load latest sync mutation replay observation: %w", err))
	}
	if found {
		result.LatestObservation = &latestObservation
	}

	var applicationOldestCreatedAt sql.NullString
	var applicationNewestCreatedAt sql.NullString
	if err := s.db.QueryRowContext(
		ctx,
		`SELECT COUNT(*), MIN(created_at), MAX(created_at)
		FROM sync_mutation_replay_applications
		WHERE owner_key = ? AND app_id = ?`,
		ownerKey,
		appID,
	).Scan(&result.ApplicationCount, &applicationOldestCreatedAt, &applicationNewestCreatedAt); err != nil {
		return SyncMutationReplayDiagnostics{}, s.wrapDBError("read_sync_mutation_replay_diagnostics_applications", started, fmt.Errorf("summarize sync mutation replay applications: %w", err))
	}

	parsedApplicationOldestCreatedAt, err := parseNullableTimestamp(applicationOldestCreatedAt)
	if err != nil {
		return SyncMutationReplayDiagnostics{}, s.wrapDBError("read_sync_mutation_replay_diagnostics_applications", started, err)
	}
	parsedApplicationNewestCreatedAt, err := parseNullableTimestamp(applicationNewestCreatedAt)
	if err != nil {
		return SyncMutationReplayDiagnostics{}, s.wrapDBError("read_sync_mutation_replay_diagnostics_applications", started, err)
	}
	result.ApplicationOldestCreatedAt = parsedApplicationOldestCreatedAt
	result.ApplicationNewestCreatedAt = parsedApplicationNewestCreatedAt

	rows, err := s.db.QueryContext(
		ctx,
		`SELECT application_status, COUNT(*)
		FROM sync_mutation_replay_applications
		WHERE owner_key = ? AND app_id = ?
		GROUP BY application_status
		ORDER BY application_status ASC`,
		ownerKey,
		appID,
	)
	if err != nil {
		return SyncMutationReplayDiagnostics{}, s.wrapDBError("read_sync_mutation_replay_diagnostics_application_statuses", started, fmt.Errorf("summarize sync mutation replay application statuses: %w", err))
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			return SyncMutationReplayDiagnostics{}, s.wrapDBError("read_sync_mutation_replay_diagnostics_application_statuses_scan", started, fmt.Errorf("scan sync mutation replay application status count: %w", err))
		}
		result.ApplicationStatusCounts[status] = count
	}
	if err := rows.Err(); err != nil {
		return SyncMutationReplayDiagnostics{}, s.wrapDBError("read_sync_mutation_replay_diagnostics_application_statuses_rows", started, fmt.Errorf("iterate sync mutation replay application status counts: %w", err))
	}

	if info, err := os.Stat(s.dbPath); err == nil && !info.IsDir() {
		dbFileBytes := info.Size()
		result.DBFileBytes = &dbFileBytes
	}

	s.logDBOperation("read_sync_mutation_replay_diagnostics", started, nil)
	return result, nil
}

func (s *Store) CompactSyncMutationReplayArtifacts(ctx context.Context, ownerKey, appID string, options SyncMutationReplayCompactOptions) (SyncMutationReplayCompactResult, error) {
	started := time.Now()
	normalizedOptions, err := normalizeSyncMutationReplayCompactOptions(options)
	if err != nil {
		return SyncMutationReplayCompactResult{}, err
	}

	result := SyncMutationReplayCompactResult{
		OwnerKey:             ownerKey,
		AppID:                appID,
		Execute:              normalizedOptions.Execute,
		ObservationRetention: normalizedOptions.ObservationRetention,
		ReceiptRetention:     normalizedOptions.ReceiptRetention,
		ObservationCutoff:    normalizedOptions.Now.Add(-normalizedOptions.ObservationRetention),
		ReceiptCutoff:        normalizedOptions.Now.Add(-normalizedOptions.ReceiptRetention),
	}

	ctx, cancel := withDBTimeout(ctx)
	defer cancel()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SyncMutationReplayCompactResult{}, s.wrapDBError("write_begin_sync_mutation_replay_compact", started, fmt.Errorf("begin sync mutation replay compaction transaction: %w", err))
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	observations, err := listSyncMutationReplayDryRunObservationsFromQuerier(ctx, tx, ownerKey, appID)
	if err != nil {
		return SyncMutationReplayCompactResult{}, s.wrapDBError("read_sync_mutation_replay_compact_observations", started, fmt.Errorf("load sync mutation replay observations for compaction: %w", err))
	}
	receipts, err := listSyncMutationReceiptRowsForCompact(ctx, tx, ownerKey, appID)
	if err != nil {
		return SyncMutationReplayCompactResult{}, s.wrapDBError("read_sync_mutation_replay_compact_receipts", started, fmt.Errorf("load sync mutation receipts for compaction: %w", err))
	}
	applications, err := listSyncMutationReplayApplicationsFromQuerier(ctx, tx, ownerKey, appID)
	if err != nil {
		return SyncMutationReplayCompactResult{}, s.wrapDBError("read_sync_mutation_replay_compact_applications", started, fmt.Errorf("load sync mutation replay applications for compaction: %w", err))
	}
	result.RetainedApplicationCount = int64(len(applications))

	referencedObservationIDs := make(map[int64]struct{})
	applicationMutationIDs := make(map[string]struct{})
	for _, application := range applications {
		applicationMutationIDs[application.MutationID] = struct{}{}
		if application.ReplayObservationID != nil {
			referencedObservationIDs[*application.ReplayObservationID] = struct{}{}
		}
	}

	retainedObservationIDs := make(map[int64]struct{})
	retainedObservations := make([]SyncMutationReplayDryRunObservation, 0, len(observations))
	observationDeleteIDs := make([]int64, 0)
	for index, observation := range observations {
		retain := index < normalizedOptions.RetainedObservationCount || !observation.CreatedAt.Before(result.ObservationCutoff)
		if _, referenced := referencedObservationIDs[observation.ID]; referenced {
			retain = true
		}
		if retain {
			retainedObservationIDs[observation.ID] = struct{}{}
			retainedObservations = append(retainedObservations, observation)
			continue
		}
		observationDeleteIDs = append(observationDeleteIDs, observation.ID)
	}
	result.RetainedObservationCount = len(retainedObservationIDs)

	latestReceiptIDs := latestSyncMutationReceiptIDs(receipts, normalizedOptions.RetainedReceiptCount)
	observationProtectedReceiptIDs := syncMutationReceiptIDsProtectedByObservations(receipts, retainedObservations)
	result.ProtectedReceiptCount = len(observationProtectedReceiptIDs)
	receiptDeleteIDs := make([]int64, 0)
	for _, receipt := range receipts {
		if !receipt.AcceptedAt.Before(result.ReceiptCutoff) {
			continue
		}
		if _, retain := latestReceiptIDs[receipt.ID]; retain {
			continue
		}
		if _, retain := applicationMutationIDs[receipt.MutationID]; retain {
			continue
		}
		if _, retain := observationProtectedReceiptIDs[receipt.ID]; retain {
			continue
		}
		receiptDeleteIDs = append(receiptDeleteIDs, receipt.ID)
	}
	result.RetainedReceiptCount = len(receipts) - len(receiptDeleteIDs)
	result.CandidateObservationDeleteCount = int64(len(observationDeleteIDs))
	result.CandidateReceiptDeleteCount = int64(len(receiptDeleteIDs))

	if normalizedOptions.Execute {
		deletedObservations, err := deleteSyncMutationReplayObservationRowsByID(ctx, tx, ownerKey, appID, observationDeleteIDs)
		if err != nil {
			return SyncMutationReplayCompactResult{}, s.wrapDBError("write_sync_mutation_replay_compact_observations", started, fmt.Errorf("delete sync mutation replay observations: %w", err))
		}
		deletedReceipts, err := deleteSyncMutationReceiptRowsByID(ctx, tx, ownerKey, appID, receiptDeleteIDs)
		if err != nil {
			return SyncMutationReplayCompactResult{}, s.wrapDBError("write_sync_mutation_replay_compact_receipts", started, fmt.Errorf("delete sync mutation receipts: %w", err))
		}
		result.DeletedObservationCount = deletedObservations
		result.DeletedReceiptCount = deletedReceipts
	}

	if err := tx.Commit(); err != nil {
		return SyncMutationReplayCompactResult{}, s.wrapDBError("write_commit_sync_mutation_replay_compact", started, fmt.Errorf("commit sync mutation replay compaction transaction: %w", err))
	}
	committed = true
	s.logDBOperation("write_sync_mutation_replay_compact", started, nil)

	return result, nil
}

func MapSyncMutationReplayPolicyStatusToApplicationStatus(policyStatus string) (string, bool) {
	switch strings.TrimSpace(policyStatus) {
	case replayAuthoritativePolicyStatusAllowed:
		return SyncMutationReplayApplicationStatusApplied, true
	case replayAuthoritativePolicyStatusSkip:
		return SyncMutationReplayApplicationStatusSkipped, true
	case replayAuthoritativePolicyStatusConflict:
		return SyncMutationReplayApplicationStatusConflict, true
	case replayAuthoritativePolicyStatusFatal:
		return SyncMutationReplayApplicationStatusFailed, true
	default:
		return "", false
	}
}

func (s *Store) RecordSyncMutationReplayApplicationInert(ctx context.Context, ownerKey, appID string, input SyncMutationReplayApplicationInput) (SyncMutationReplayApplicationResult, error) {
	started := time.Now()
	ctx, cancel := withDBTimeout(ctx)
	defer cancel()

	mutationID := strings.TrimSpace(input.MutationID)
	applicationStatus := strings.TrimSpace(input.ApplicationStatus)
	applicationReason := strings.TrimSpace(input.ApplicationReason)
	if mutationID == "" {
		return SyncMutationReplayApplicationResult{}, fmt.Errorf("record sync mutation replay application inert: mutation id is required")
	}
	if applicationReason == "" {
		return SyncMutationReplayApplicationResult{}, fmt.Errorf("record sync mutation replay application inert: application reason is required")
	}
	if _, ok := syncMutationReplayApplicationStatusSet[applicationStatus]; !ok {
		return SyncMutationReplayApplicationResult{}, fmt.Errorf("record sync mutation replay application inert: application status %q is invalid", applicationStatus)
	}
	if strings.TrimSpace(input.CanonicalDocumentHashBefore) == "" {
		return SyncMutationReplayApplicationResult{}, fmt.Errorf("record sync mutation replay application inert: canonical document hash before is required")
	}
	if input.CanonicalDocumentHashAfter != nil && strings.TrimSpace(*input.CanonicalDocumentHashAfter) == "" {
		return SyncMutationReplayApplicationResult{}, fmt.Errorf("record sync mutation replay application inert: canonical document hash after must be non-empty when provided")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SyncMutationReplayApplicationResult{}, s.wrapDBError("write_begin_sync_mutation_replay_application_inert", started, fmt.Errorf("begin sync mutation replay application inert transaction: %w", err))
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	var receiptID int64
	if err := tx.QueryRowContext(
		ctx,
		`SELECT id
		FROM sync_mutation_receipts
		WHERE owner_key = ? AND app_id = ? AND mutation_id = ? AND status = ?
		LIMIT 1`,
		ownerKey,
		appID,
		mutationID,
		SyncMutationReceiptStatusAccepted,
	).Scan(&receiptID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SyncMutationReplayApplicationResult{}, fmt.Errorf("record sync mutation replay application inert: accepted receipt %q not found", mutationID)
		}
		return SyncMutationReplayApplicationResult{}, s.wrapDBError("read_sync_mutation_replay_application_receipt", started, fmt.Errorf("load accepted receipt for inert replay application: %w", err))
	}

	if input.ReplayObservationID != nil {
		var observationID int64
		if err := tx.QueryRowContext(
			ctx,
			`SELECT id
			FROM sync_mutation_replay_dry_run_observations
			WHERE id = ? AND owner_key = ? AND app_id = ?
			LIMIT 1`,
			*input.ReplayObservationID,
			ownerKey,
			appID,
		).Scan(&observationID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return SyncMutationReplayApplicationResult{}, fmt.Errorf("record sync mutation replay application inert: replay observation %d not found for owner/app", *input.ReplayObservationID)
			}
			return SyncMutationReplayApplicationResult{}, s.wrapDBError("read_sync_mutation_replay_application_observation", started, fmt.Errorf("load replay observation for inert replay application: %w", err))
		}
	}

	createdAt := time.Now().UTC().Format(time.RFC3339)
	var versionAfterValue any
	var hashAfterValue any
	var replayObservationValue any
	if input.CanonicalDocumentVersionAfter != nil {
		versionAfterValue = *input.CanonicalDocumentVersionAfter
	}
	if input.CanonicalDocumentHashAfter != nil {
		hashAfterValue = strings.TrimSpace(*input.CanonicalDocumentHashAfter)
	}
	if input.ReplayObservationID != nil {
		replayObservationValue = *input.ReplayObservationID
	}

	insertResult, err := tx.ExecContext(
		ctx,
		`INSERT OR IGNORE INTO sync_mutation_replay_applications (
			owner_key,
			app_id,
			mutation_id,
			application_status,
			application_reason,
			canonical_document_version_before,
			canonical_document_hash_before,
			canonical_document_version_after,
			canonical_document_hash_after,
			replay_observation_id,
			created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ownerKey,
		appID,
		mutationID,
		applicationStatus,
		applicationReason,
		input.CanonicalDocumentVersionBefore,
		strings.TrimSpace(input.CanonicalDocumentHashBefore),
		versionAfterValue,
		hashAfterValue,
		replayObservationValue,
		createdAt,
	)
	if err != nil {
		return SyncMutationReplayApplicationResult{}, s.wrapDBError("write_insert_sync_mutation_replay_application_inert", started, fmt.Errorf("insert sync mutation replay application inert row: %w", err))
	}

	rowsAffected, err := insertResult.RowsAffected()
	if err != nil {
		return SyncMutationReplayApplicationResult{}, s.wrapDBError("write_sync_mutation_replay_application_inert_rows_affected", started, fmt.Errorf("read inert replay application rows affected: %w", err))
	}

	application, err := scanSyncMutationReplayApplication(tx.QueryRowContext(
		ctx,
		`SELECT
			id,
			owner_key,
			app_id,
			mutation_id,
			application_status,
			application_reason,
			canonical_document_version_before,
			canonical_document_hash_before,
			canonical_document_version_after,
			canonical_document_hash_after,
			replay_observation_id,
			created_at
		FROM sync_mutation_replay_applications
		WHERE owner_key = ? AND app_id = ? AND mutation_id = ?`,
		ownerKey,
		appID,
		mutationID,
	))
	if err != nil {
		return SyncMutationReplayApplicationResult{}, s.wrapDBError("read_sync_mutation_replay_application_inert", started, fmt.Errorf("load inert replay application row: %w", err))
	}

	if err := tx.Commit(); err != nil {
		return SyncMutationReplayApplicationResult{}, s.wrapDBError("write_commit_sync_mutation_replay_application_inert", started, fmt.Errorf("commit inert replay application row: %w", err))
	}
	committed = true
	s.logDBOperation("write_record_sync_mutation_replay_application_inert", started, nil)

	return SyncMutationReplayApplicationResult{
		Application: application,
		Duplicate:   rowsAffected == 0,
	}, nil
}

func (s *Store) ListSyncMutationReplayApplications(ctx context.Context, ownerKey, appID string) ([]SyncMutationReplayApplication, error) {
	started := time.Now()
	ctx, cancel := withDBTimeout(ctx)
	defer cancel()

	applications, err := listSyncMutationReplayApplicationsFromQuerier(ctx, s.db, ownerKey, appID)
	if err != nil {
		return nil, s.wrapDBError("read_list_sync_mutation_replay_applications", started, fmt.Errorf("list sync mutation replay applications: %w", err))
	}

	s.logDBOperation("read_list_sync_mutation_replay_applications", started, nil)
	return applications, nil
}

func (s *Store) GetSyncDeltaMetadata(ctx context.Context, ownerKey, appID string, options SyncDeltaMetadataOptions) (SyncDeltaMetadata, error) {
	started := time.Now()
	if options.SinceVersion < 0 {
		return SyncDeltaMetadata{}, fmt.Errorf("get sync delta metadata: since version must be non-negative")
	}
	if options.ApplicationWatermark != nil && *options.ApplicationWatermark < 0 {
		return SyncDeltaMetadata{}, fmt.Errorf("get sync delta metadata: application watermark must be non-negative")
	}

	ctx, cancel := withDBTimeout(ctx)
	defer cancel()

	doc, err := loadReplayDocumentFromQuerier(ctx, s.db, ownerKey, appID)
	if err != nil {
		return SyncDeltaMetadata{}, s.wrapDBError("read_get_sync_delta_metadata_document", started, fmt.Errorf("load sync delta metadata document: %w", err))
	}
	applications, err := listSyncMutationReplayApplicationsFromQuerier(ctx, s.db, ownerKey, appID)
	if err != nil {
		return SyncDeltaMetadata{}, s.wrapDBError("read_get_sync_delta_metadata_applications", started, fmt.Errorf("load sync delta metadata applications: %w", err))
	}

	result := SyncDeltaMetadata{
		AppID:                   appID,
		CurrentDocumentVersion:  doc.Version,
		CurrentDocumentHash:     hashReplayObservationBytes(doc.Body),
		ClientVersion:           options.SinceVersion,
		RequiresSnapshotRefresh: false,
		Reason:                  SyncDeltaMetadataReasonUpToDate,
		ApplicationWatermark:    cloneInt64Pointer(options.ApplicationWatermark),
		Applications:            []SyncDeltaApplicationMetadata{},
		Warnings:                []string{},
	}

	committedApplications := filterCommittedSyncDeltaApplications(applications)
	result.NextApplicationWatermark = currentSyncDeltaApplicationWatermark(committedApplications)

	if options.SinceVersion > doc.Version {
		result.RequiresSnapshotRefresh = true
		result.Reason = SyncDeltaMetadataReasonSnapshotRequiredClientVersionAhead
		s.logDBOperation("read_get_sync_delta_metadata", started, nil)
		return result, nil
	}

	if options.IncludeApplications {
		filteredApplications := filterSyncDeltaApplicationsForOptions(committedApplications, options)
		limit := normalizeSyncDeltaMetadataLimit(options.Limit)
		if len(filteredApplications) > limit {
			result.RequiresSnapshotRefresh = true
			result.Reason = SyncDeltaMetadataReasonSnapshotRequiredTooManyApplications
			s.logDBOperation("read_get_sync_delta_metadata", started, nil)
			return result, nil
		}
		if len(filteredApplications) > 0 {
			result.Reason = SyncDeltaMetadataReasonApplicationsAvailable
			result.Applications = buildSyncDeltaApplicationMetadataSlice(filteredApplications)
			result.NextApplicationWatermark = cloneInt64Pointer(&filteredApplications[len(filteredApplications)-1].ID)
			if hasSyncDeltaVersionAdvanceWithoutBodyChange(result.Applications) {
				result.Warnings = append(result.Warnings, syncDeltaMetadataWarningVersionAdvancedWithoutBodyChange)
			}
			s.logDBOperation("read_get_sync_delta_metadata", started, nil)
			return result, nil
		}
	}

	if doc.Version != options.SinceVersion {
		result.Reason = SyncDeltaMetadataReasonDocumentVersionChanged
	}

	s.logDBOperation("read_get_sync_delta_metadata", started, nil)
	return result, nil
}

func (s *Store) ApplySyncMutationReplayAuthoritativeInternal(ctx context.Context, ownerKey, appID string, observationID int64, options SyncMutationReplayAuthoritativeApplyOptions) (SyncMutationReplayAuthoritativeApplyResult, error) {
	started := time.Now()
	result := SyncMutationReplayAuthoritativeApplyResult{
		Status:          SyncMutationReplayAuthoritativeApplyStatusRefusedInternalGate,
		MutationResults: []SyncMutationReplayAuthoritativeMutationResult{},
	}
	if !options.AllowInternalAuthoritativeReplay {
		return result, nil
	}

	ctx, cancel := withDBTimeout(ctx)
	defer cancel()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SyncMutationReplayAuthoritativeApplyResult{}, s.wrapDBError("write_begin_sync_mutation_replay_authoritative_internal", started, fmt.Errorf("begin sync mutation replay authoritative transaction: %w", err))
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	observation, found, err := getSyncMutationReplayDryRunObservationByIDFromQuerier(ctx, tx, observationID)
	if err != nil {
		return SyncMutationReplayAuthoritativeApplyResult{}, s.wrapDBError("read_sync_mutation_replay_authoritative_observation_internal", started, fmt.Errorf("load sync mutation replay observation for authoritative apply: %w", err))
	}
	if !found {
		result.Status = SyncMutationReplayAuthoritativeApplyStatusAbortedPreconditions
		return result, nil
	}
	if observation.OwnerKey != ownerKey || observation.AppID != appID {
		result.Status = SyncMutationReplayAuthoritativeApplyStatusAbortedPreconditions
		return result, nil
	}

	doc, err := loadReplayDocumentFromQuerier(ctx, tx, ownerKey, appID)
	if err != nil {
		return SyncMutationReplayAuthoritativeApplyResult{}, s.wrapDBError("read_sync_mutation_replay_authoritative_document_internal", started, fmt.Errorf("load sync mutation replay document for authoritative apply: %w", err))
	}
	receipts, err := listAcceptedSyncMutationReceiptsFromQuerier(ctx, tx, ownerKey, appID)
	if err != nil {
		return SyncMutationReplayAuthoritativeApplyResult{}, s.wrapDBError("read_sync_mutation_replay_authoritative_receipts_internal", started, fmt.Errorf("load sync mutation replay receipts for authoritative apply: %w", err))
	}
	applications, err := listSyncMutationReplayApplicationsFromQuerier(ctx, tx, ownerKey, appID)
	if err != nil {
		return SyncMutationReplayAuthoritativeApplyResult{}, s.wrapDBError("read_sync_mutation_replay_authoritative_applications_internal", started, fmt.Errorf("load sync mutation replay applications for authoritative apply: %w", err))
	}

	result.CanonicalDocumentVersionBefore = doc.Version
	result.CanonicalDocumentHashBefore = hashReplayObservationBytes(doc.Body)

	dryRunResult, err := buildSyncMutationDryRunResult(doc, receipts)
	if err != nil {
		return SyncMutationReplayAuthoritativeApplyResult{}, s.wrapDBError("build_sync_mutation_replay_authoritative_dry_run_internal", started, fmt.Errorf("build sync mutation replay dry-run result for authoritative apply: %w", err))
	}
	compareEvaluation, err := evaluateSyncMutationReplayCompareAndApplyAgainstState(observation, doc, receipts, dryRunResult, applications)
	if err != nil {
		return SyncMutationReplayAuthoritativeApplyResult{}, s.wrapDBError("evaluate_sync_mutation_replay_authoritative_preconditions_internal", started, fmt.Errorf("evaluate sync mutation replay authoritative preconditions: %w", err))
	}
	recoveryEvaluation := evaluateSyncMutationReplayRecoveryAgainstState(observation, doc, receipts, dryRunResult, applications, compareEvaluation)

	switch recoveryEvaluation.Status {
	case SyncMutationReplayRecoveryStatusAlreadyAppliedRequiresIdempotentExit:
		result.Status = SyncMutationReplayAuthoritativeApplyStatusIdempotentExitAlreadyApplied
		return result, nil
	case SyncMutationReplayRecoveryStatusPartialApplicationRows,
		SyncMutationReplayRecoveryStatusApplicationRowsWithoutMatchingCanonicalState,
		SyncMutationReplayRecoveryStatusCanonicalStateWithoutApplicationRows,
		SyncMutationReplayRecoveryStatusBlockedSnapshotRequiresCleanup:
		result.Status = SyncMutationReplayAuthoritativeApplyStatusAbortedRecovery
		return result, nil
	}

	if compareEvaluation.Status != SyncMutationReplayCompareAndApplyStatusAllowed {
		switch compareEvaluation.Status {
		case SyncMutationReplayCompareAndApplyStatusStaleCanonicalDocument,
			SyncMutationReplayCompareAndApplyStatusStaleReceiptSet,
			SyncMutationReplayCompareAndApplyStatusMissingObservation,
			SyncMutationReplayCompareAndApplyStatusInvalidObservationScope:
			result.Status = SyncMutationReplayAuthoritativeApplyStatusAbortedPreconditions
		default:
			result.Status = SyncMutationReplayAuthoritativeApplyStatusAbortedRecovery
		}
		return result, nil
	}

	if recoveryEvaluation.Status == SyncMutationReplayRecoveryStatusAlreadyAppliedRequiresIdempotentExit {
		result.Status = SyncMutationReplayAuthoritativeApplyStatusIdempotentExitAlreadyApplied
		return result, nil
	}
	if recoveryEvaluation.Status != SyncMutationReplayRecoveryStatusSafeToAttemptTransaction {
		result.Status = SyncMutationReplayAuthoritativeApplyStatusAbortedRecovery
		return result, nil
	}

	preview, err := buildSyncMutationReplayAuthoritativeProgressivePreview(doc, receipts)
	if err != nil {
		return SyncMutationReplayAuthoritativeApplyResult{}, s.wrapDBError("build_sync_mutation_replay_authoritative_preview_internal", started, fmt.Errorf("build authoritative sync mutation replay preview: %w", err))
	}
	result.MutationResults = preview.MutationResults

	if preview.PolicyAbort {
		result.Status = SyncMutationReplayAuthoritativeApplyStatusAbortedPolicy
		return result, nil
	}
	if len(preview.StagedApplications) == 0 {
		result.Status = SyncMutationReplayAuthoritativeApplyStatusIdempotentExitAlreadyApplied
		return result, nil
	}

	versionAfter := doc.Version + 1
	hashAfter := hashReplayObservationBytes(preview.PreviewBody)
	updatedAt := time.Now().UTC().Format(time.RFC3339)

	updateResult, err := tx.ExecContext(
		ctx,
		`UPDATE documents SET body_json = ?, version = ?, updated_at = ? WHERE id = ? AND version = ?`,
		string(preview.PreviewBody),
		versionAfter,
		updatedAt,
		doc.ID,
		doc.Version,
	)
	if err != nil {
		return SyncMutationReplayAuthoritativeApplyResult{}, s.wrapDBError("write_sync_mutation_replay_authoritative_document_internal", started, fmt.Errorf("update canonical document during authoritative replay apply: %w", err))
	}
	rowsAffected, err := updateResult.RowsAffected()
	if err != nil {
		return SyncMutationReplayAuthoritativeApplyResult{}, s.wrapDBError("write_sync_mutation_replay_authoritative_document_rows_affected_internal", started, fmt.Errorf("read canonical document rows affected during authoritative replay apply: %w", err))
	}
	if rowsAffected != 1 {
		result.Status = SyncMutationReplayAuthoritativeApplyStatusAbortedPreconditions
		return result, nil
	}

	if options.FailAfterApplicationRowInserts != nil && *options.FailAfterApplicationRowInserts == 0 {
		return SyncMutationReplayAuthoritativeApplyResult{}, ErrSyncMutationReplayAuthoritativeApplyFailpoint
	}

	insertedApplicationRowCount := 0
	for _, stagedApplication := range preview.StagedApplications {
		_, err := tx.ExecContext(
			ctx,
			`INSERT INTO sync_mutation_replay_applications (
				owner_key,
				app_id,
				mutation_id,
				application_status,
				application_reason,
				canonical_document_version_before,
				canonical_document_hash_before,
				canonical_document_version_after,
				canonical_document_hash_after,
				replay_observation_id,
				created_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			ownerKey,
			appID,
			stagedApplication.MutationID,
			stagedApplication.ApplicationStatus,
			stagedApplication.ApplicationReason,
			doc.Version,
			result.CanonicalDocumentHashBefore,
			versionAfter,
			hashAfter,
			observation.ID,
			updatedAt,
		)
		if err != nil {
			return SyncMutationReplayAuthoritativeApplyResult{}, s.wrapDBError("write_sync_mutation_replay_authoritative_application_internal", started, fmt.Errorf("insert authoritative replay application row: %w", err))
		}
		insertedApplicationRowCount++
		if options.FailAfterApplicationRowInserts != nil && insertedApplicationRowCount == *options.FailAfterApplicationRowInserts {
			return SyncMutationReplayAuthoritativeApplyResult{}, ErrSyncMutationReplayAuthoritativeApplyFailpoint
		}
	}

	if err := tx.Commit(); err != nil {
		return SyncMutationReplayAuthoritativeApplyResult{}, s.wrapDBError("write_commit_sync_mutation_replay_authoritative_internal", started, fmt.Errorf("commit authoritative sync mutation replay transaction: %w", err))
	}
	committed = true
	s.logDBOperation("write_apply_sync_mutation_replay_authoritative_internal", started, nil)

	result.Status = SyncMutationReplayAuthoritativeApplyStatusApplied
	result.CanonicalDocumentVersionAfter = &versionAfter
	result.CanonicalDocumentHashAfter = &hashAfter
	result.InsertedApplicationRowCount = insertedApplicationRowCount
	return result, nil
}

func (s *Store) EvaluateSyncMutationReplayCompareAndApplyPreconditions(ctx context.Context, ownerKey, appID string, observationID int64) (SyncMutationReplayCompareAndApplyEvaluation, error) {
	evaluation := SyncMutationReplayCompareAndApplyEvaluation{
		Status: SyncMutationReplayCompareAndApplyStatusMissingObservation,
		Preconditions: SyncMutationReplayCompareAndApplyPreconditions{
			OwnerKey:            ownerKey,
			AppID:               appID,
			ReplayObservationID: observationID,
		},
		Reasons:            make([]string, 0),
		Warnings:           make([]string, 0),
		AppliedMutationIDs: make([]string, 0),
	}

	observation, found, err := s.getSyncMutationReplayDryRunObservationByID(ctx, observationID)
	if err != nil {
		return SyncMutationReplayCompareAndApplyEvaluation{}, err
	}
	if !found {
		return evaluation, nil
	}

	evaluation.Preconditions = buildSyncMutationReplayCompareAndApplyPreconditions(observation)
	if observation.OwnerKey != ownerKey || observation.AppID != appID {
		evaluation.Status = SyncMutationReplayCompareAndApplyStatusInvalidObservationScope
		evaluation.Preconditions.OwnerKey = ownerKey
		evaluation.Preconditions.AppID = appID
		evaluation.Reasons = append(evaluation.Reasons, "observation_owner_app_mismatch")
		return evaluation, nil
	}

	doc, receipts, err := s.loadSyncMutationReplayDryRunInputs(ctx, ownerKey, appID)
	if err != nil {
		return SyncMutationReplayCompareAndApplyEvaluation{}, err
	}

	result, err := buildSyncMutationDryRunResult(doc, receipts)
	if err != nil {
		return SyncMutationReplayCompareAndApplyEvaluation{}, err
	}

	applications, err := s.ListSyncMutationReplayApplications(ctx, ownerKey, appID)
	if err != nil {
		return SyncMutationReplayCompareAndApplyEvaluation{}, err
	}

	return evaluateSyncMutationReplayCompareAndApplyAgainstState(observation, doc, receipts, result, applications)
}

func (s *Store) EvaluateSyncMutationReplayRecoveryState(ctx context.Context, ownerKey, appID string, observationID int64) (SyncMutationReplayRecoveryEvaluation, error) {
	recovery := SyncMutationReplayRecoveryEvaluation{
		Status:   SyncMutationReplayRecoveryStatusMissingObservation,
		Reasons:  []string{"missing_observation"},
		Warnings: make([]string, 0),
		CompareAndApplyEvaluation: SyncMutationReplayCompareAndApplyEvaluation{
			Status: SyncMutationReplayCompareAndApplyStatusMissingObservation,
			Preconditions: SyncMutationReplayCompareAndApplyPreconditions{
				OwnerKey:            ownerKey,
				AppID:               appID,
				ReplayObservationID: observationID,
			},
			Reasons:            []string{"missing_observation"},
			Warnings:           make([]string, 0),
			AppliedMutationIDs: make([]string, 0),
		},
		AppliedMutationIDs: make([]string, 0),
	}

	observation, found, err := s.getSyncMutationReplayDryRunObservationByID(ctx, observationID)
	if err != nil {
		return SyncMutationReplayRecoveryEvaluation{}, err
	}
	if !found {
		return recovery, nil
	}

	if observation.OwnerKey != ownerKey || observation.AppID != appID {
		recovery.Status = SyncMutationReplayRecoveryStatusInvalidObservationScope
		recovery.Reasons = []string{"observation_owner_app_mismatch"}
		recovery.CompareAndApplyEvaluation = SyncMutationReplayCompareAndApplyEvaluation{
			Status: SyncMutationReplayCompareAndApplyStatusInvalidObservationScope,
			Preconditions: SyncMutationReplayCompareAndApplyPreconditions{
				OwnerKey:                         ownerKey,
				AppID:                            appID,
				ReplayObservationID:              observationID,
				ExpectedCanonicalDocumentVersion: observation.CanonicalDocumentVersionObserved,
				ExpectedCanonicalDocumentHash:    observation.CanonicalDocumentHashObserved,
				ExpectedReceiptCount:             observation.ReceiptCountConsidered,
				ExpectedFirstMutationID:          observation.FirstOrderedMutationID,
				ExpectedLastMutationID:           observation.LastOrderedMutationID,
				ExpectedReceiptHighWatermark:     observation.OrderedReceiptHighWatermark,
				ExpectedPreviewHash:              observation.PreviewHash,
			},
			Reasons:            []string{"observation_owner_app_mismatch"},
			Warnings:           make([]string, 0),
			AppliedMutationIDs: make([]string, 0),
		}
		return recovery, nil
	}

	doc, receipts, err := s.loadSyncMutationReplayDryRunInputs(ctx, ownerKey, appID)
	if err != nil {
		return SyncMutationReplayRecoveryEvaluation{}, err
	}

	result, err := buildSyncMutationDryRunResult(doc, receipts)
	if err != nil {
		return SyncMutationReplayRecoveryEvaluation{}, err
	}

	applications, err := s.ListSyncMutationReplayApplications(ctx, ownerKey, appID)
	if err != nil {
		return SyncMutationReplayRecoveryEvaluation{}, err
	}

	compareEvaluation, err := evaluateSyncMutationReplayCompareAndApplyAgainstState(observation, doc, receipts, result, applications)
	if err != nil {
		return SyncMutationReplayRecoveryEvaluation{}, err
	}

	return evaluateSyncMutationReplayRecoveryAgainstState(observation, doc, receipts, result, applications, compareEvaluation), nil
}

func (s *Store) EvaluateSyncMutationReplayAuthoritativePreview(ctx context.Context, ownerKey, appID string, observationID int64) (SyncMutationReplayAuthoritativePreview, error) {
	preview := SyncMutationReplayAuthoritativePreview{
		Status:                  SyncMutationReplayAuthoritativePreviewStatusUnavailablePreconditions,
		ApplicationStatusCounts: map[string]int64{},
		MutationResults:         []SyncMutationReplayAuthoritativeMutationResult{},
		PolicyAbortReasons:      []string{},
		Reasons:                 []string{},
		Warnings:                []string{},
	}

	observation, found, err := s.getSyncMutationReplayDryRunObservationByID(ctx, observationID)
	if err != nil {
		return SyncMutationReplayAuthoritativePreview{}, err
	}
	if !found {
		preview.Reasons = []string{"missing_observation"}
		preview.CompareAndApplyStatus = SyncMutationReplayCompareAndApplyStatusMissingObservation
		preview.RecoveryStatus = SyncMutationReplayRecoveryStatusMissingObservation
		return preview, nil
	}
	if observation.OwnerKey != ownerKey || observation.AppID != appID {
		preview.Reasons = []string{"observation_owner_app_mismatch"}
		preview.CompareAndApplyStatus = SyncMutationReplayCompareAndApplyStatusInvalidObservationScope
		preview.RecoveryStatus = SyncMutationReplayRecoveryStatusInvalidObservationScope
		return preview, nil
	}

	doc, receipts, err := s.loadSyncMutationReplayDryRunInputs(ctx, ownerKey, appID)
	if err != nil {
		return SyncMutationReplayAuthoritativePreview{}, err
	}

	dryRunResult, err := buildSyncMutationDryRunResult(doc, receipts)
	if err != nil {
		return SyncMutationReplayAuthoritativePreview{}, err
	}
	preview.DryRunPreviewHash = hashReplayObservationBytes(dryRunResult.PreviewBody)

	applications, err := s.ListSyncMutationReplayApplications(ctx, ownerKey, appID)
	if err != nil {
		return SyncMutationReplayAuthoritativePreview{}, err
	}

	compareEvaluation, err := evaluateSyncMutationReplayCompareAndApplyAgainstState(observation, doc, receipts, dryRunResult, applications)
	if err != nil {
		return SyncMutationReplayAuthoritativePreview{}, err
	}
	recoveryEvaluation := evaluateSyncMutationReplayRecoveryAgainstState(observation, doc, receipts, dryRunResult, applications, compareEvaluation)

	preview.CompareAndApplyStatus = compareEvaluation.Status
	preview.RecoveryStatus = recoveryEvaluation.Status
	preview.Reasons = mergeReplayStringSets(compareEvaluation.Reasons, recoveryEvaluation.Reasons)
	preview.Warnings = mergeReplayStringSets(compareEvaluation.Warnings, recoveryEvaluation.Warnings)

	if compareEvaluation.Status != SyncMutationReplayCompareAndApplyStatusAllowed ||
		recoveryEvaluation.Status != SyncMutationReplayRecoveryStatusSafeToAttemptTransaction {
		return preview, nil
	}

	progressivePreview, err := buildSyncMutationReplayAuthoritativeProgressivePreview(doc, receipts)
	if err != nil {
		return SyncMutationReplayAuthoritativePreview{}, err
	}
	preview.MutationResults = cloneReplayAuthoritativeMutationResults(progressivePreview.MutationResults)
	preview.ApplicationStatusCounts = countReplayMutationApplicationStatuses(progressivePreview.MutationResults)
	preview.Warnings = mergeReplayStringSets(preview.Warnings, progressivePreview.Warnings)

	if progressivePreview.PolicyAbort {
		preview.Status = SyncMutationReplayAuthoritativePreviewStatusWouldAbortPolicy
		preview.PolicyAbort = true
		preview.PolicyAbortMutationID = progressivePreview.PolicyAbortMutationID
		preview.PolicyAbortOperationType = progressivePreview.PolicyAbortOperationType
		preview.PolicyAbortStatus = progressivePreview.PolicyAbortStatus
		preview.PolicyAbortReasons = cloneStringSlice(progressivePreview.PolicyAbortReasons)
		preview.Reasons = mergeReplayStringSets(preview.Reasons, preview.PolicyAbortReasons)
		return preview, nil
	}

	preview.Status = SyncMutationReplayAuthoritativePreviewStatusAvailable
	preview.AuthoritativePreviewHash = hashReplayObservationBytes(progressivePreview.PreviewBody)
	preview.AuthoritativePreviewHashMatchesDryRun = preview.AuthoritativePreviewHash == preview.DryRunPreviewHash
	return preview, nil
}

func (s *Store) ReplaySyncMutationReceiptsDryRun(ctx context.Context, ownerKey, appID string) (SyncMutationDryRunResult, error) {
	doc, receipts, err := s.loadSyncMutationReplayDryRunInputs(ctx, ownerKey, appID)
	if err != nil {
		return SyncMutationDryRunResult{}, err
	}

	return buildSyncMutationDryRunResult(doc, receipts)
}

func (s *Store) RecordSyncMutationReplayDryRunObservation(ctx context.Context, ownerKey, appID string) (SyncMutationReplayDryRunObservation, error) {
	started := time.Now()

	doc, receipts, err := s.loadSyncMutationReplayDryRunInputs(ctx, ownerKey, appID)
	if err != nil {
		return SyncMutationReplayDryRunObservation{}, err
	}

	result, err := buildSyncMutationDryRunResult(doc, receipts)
	if err != nil {
		return SyncMutationReplayDryRunObservation{}, err
	}

	createdAt := time.Now().UTC()
	observation := buildSyncMutationReplayDryRunObservation(doc, receipts, result, createdAt)

	dbCtx, cancel := withDBTimeout(ctx)
	defer cancel()

	insertResult, err := s.db.ExecContext(
		dbCtx,
		`INSERT INTO sync_mutation_replay_dry_run_observations (
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
			created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		observation.OwnerKey,
		observation.AppID,
		observation.CanonicalDocumentVersionObserved,
		observation.CanonicalDocumentHashObserved,
		observation.ReceiptCountConsidered,
		observation.FirstOrderedMutationID,
		observation.LastOrderedMutationID,
		observation.OrderedReceiptHighWatermark,
		observation.AppliedCount,
		observation.SkippedCount,
		observation.WarningCount,
		observation.PreviewHash,
		observation.CreatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return SyncMutationReplayDryRunObservation{}, s.wrapDBError("write_record_sync_mutation_replay_dry_run_observation", started, fmt.Errorf("insert sync mutation replay dry-run observation: %w", err))
	}

	observationID, err := insertResult.LastInsertId()
	if err != nil {
		return SyncMutationReplayDryRunObservation{}, s.wrapDBError("write_record_sync_mutation_replay_dry_run_observation_last_insert_id", started, fmt.Errorf("read sync mutation replay dry-run observation id: %w", err))
	}
	observation.ID = observationID

	s.logDBOperation("write_record_sync_mutation_replay_dry_run_observation", started, nil)
	return observation, nil
}

type syncMutationReplayQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

type syncMutationReceiptRowForCompact struct {
	ID         int64
	MutationID string
	CreatedAt  time.Time
	AcceptedAt time.Time
}

func loadReplayDocumentFromQuerier(ctx context.Context, querier syncMutationReplayQuerier, ownerKey, appID string) (Document, error) {
	return scanDocument(querier.QueryRowContext(
		ctx,
		`SELECT id, owner_key, app_id, body_json, version, updated_at
		FROM documents
		WHERE owner_key = ? AND app_id = ?`,
		ownerKey,
		appID,
	))
}

func listAcceptedSyncMutationReceiptsFromQuerier(ctx context.Context, querier syncMutationReplayQuerier, ownerKey, appID string) ([]SyncMutationReceipt, error) {
	rows, err := querier.QueryContext(
		ctx,
		`SELECT
			id,
			owner_key,
			app_id,
			mutation_id,
			client_id,
			device_id,
			protocol,
			entity_type,
			entity_id,
			operation_type,
			payload_json,
			base_revision,
			status,
			created_at,
			accepted_at
		FROM sync_mutation_receipts
		WHERE owner_key = ? AND app_id = ? AND status = ?
		ORDER BY accepted_at ASC, created_at ASC, mutation_id ASC`,
		ownerKey,
		appID,
		SyncMutationReceiptStatusAccepted,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	receipts := make([]SyncMutationReceipt, 0)
	for rows.Next() {
		receipt, scanErr := scanSyncMutationReceipt(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		receipts = append(receipts, receipt)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return receipts, nil
}

func listSyncMutationReceiptRowsForCompact(ctx context.Context, querier syncMutationReplayQuerier, ownerKey, appID string) ([]syncMutationReceiptRowForCompact, error) {
	rows, err := querier.QueryContext(
		ctx,
		`SELECT id, mutation_id, created_at, accepted_at
		FROM sync_mutation_receipts
		WHERE owner_key = ? AND app_id = ? AND status = ?
		ORDER BY accepted_at ASC, created_at ASC, mutation_id ASC`,
		ownerKey,
		appID,
		SyncMutationReceiptStatusAccepted,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	receipts := make([]syncMutationReceiptRowForCompact, 0)
	for rows.Next() {
		var receipt syncMutationReceiptRowForCompact
		var createdAt string
		var acceptedAt string
		if err := rows.Scan(&receipt.ID, &receipt.MutationID, &createdAt, &acceptedAt); err != nil {
			return nil, err
		}
		parsedCreatedAt, err := time.Parse(time.RFC3339, createdAt)
		if err != nil {
			return nil, fmt.Errorf("parse sync mutation receipt created_at: %w", err)
		}
		parsedAcceptedAt, err := time.Parse(time.RFC3339, acceptedAt)
		if err != nil {
			return nil, fmt.Errorf("parse sync mutation receipt accepted_at: %w", err)
		}
		receipt.CreatedAt = parsedCreatedAt.UTC()
		receipt.AcceptedAt = parsedAcceptedAt.UTC()
		receipts = append(receipts, receipt)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return receipts, nil
}

func listSyncMutationReplayApplicationsFromQuerier(ctx context.Context, querier syncMutationReplayQuerier, ownerKey, appID string) ([]SyncMutationReplayApplication, error) {
	rows, err := querier.QueryContext(
		ctx,
		`SELECT
			id,
			owner_key,
			app_id,
			mutation_id,
			application_status,
			application_reason,
			canonical_document_version_before,
			canonical_document_hash_before,
			canonical_document_version_after,
			canonical_document_hash_after,
			replay_observation_id,
			created_at
		FROM sync_mutation_replay_applications
		WHERE owner_key = ? AND app_id = ?
		ORDER BY created_at ASC, id ASC`,
		ownerKey,
		appID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	applications := make([]SyncMutationReplayApplication, 0)
	for rows.Next() {
		application, scanErr := scanSyncMutationReplayApplication(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		applications = append(applications, application)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return applications, nil
}

func listSyncMutationReplayDryRunObservationsFromQuerier(ctx context.Context, querier syncMutationReplayQuerier, ownerKey, appID string) ([]SyncMutationReplayDryRunObservation, error) {
	rows, err := querier.QueryContext(
		ctx,
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
			created_at
		FROM sync_mutation_replay_dry_run_observations
		WHERE owner_key = ? AND app_id = ?
		ORDER BY created_at DESC, id DESC`,
		ownerKey,
		appID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	observations := make([]SyncMutationReplayDryRunObservation, 0)
	for rows.Next() {
		observation, scanErr := scanSyncMutationReplayDryRunObservation(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		observations = append(observations, observation)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return observations, nil
}

func getLatestSyncMutationReplayDryRunObservationFromQuerier(ctx context.Context, querier syncMutationReplayQuerier, ownerKey, appID string) (SyncMutationReplayDryRunObservation, bool, error) {
	observation, err := scanSyncMutationReplayDryRunObservation(querier.QueryRowContext(
		ctx,
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
			created_at
		FROM sync_mutation_replay_dry_run_observations
		WHERE owner_key = ? AND app_id = ?
		ORDER BY created_at DESC, id DESC
		LIMIT 1`,
		ownerKey,
		appID,
	))
	if err != nil {
		if errors.Is(err, ErrSyncMutationReplayDryRunObservationNotFound) {
			return SyncMutationReplayDryRunObservation{}, false, nil
		}
		return SyncMutationReplayDryRunObservation{}, false, err
	}
	return observation, true, nil
}

func filterCommittedSyncDeltaApplications(applications []SyncMutationReplayApplication) []SyncMutationReplayApplication {
	filtered := make([]SyncMutationReplayApplication, 0, len(applications))
	for _, application := range applications {
		if application.ApplicationStatus != SyncMutationReplayApplicationStatusApplied &&
			application.ApplicationStatus != SyncMutationReplayApplicationStatusSkipped {
			continue
		}
		filtered = append(filtered, application)
	}
	return filtered
}

func filterSyncDeltaApplicationsForOptions(applications []SyncMutationReplayApplication, options SyncDeltaMetadataOptions) []SyncMutationReplayApplication {
	filtered := make([]SyncMutationReplayApplication, 0, len(applications))
	for _, application := range applications {
		if options.ApplicationWatermark != nil {
			if application.ID <= *options.ApplicationWatermark {
				continue
			}
			filtered = append(filtered, application)
			continue
		}
		if application.CanonicalDocumentVersionAfter == nil {
			continue
		}
		if *application.CanonicalDocumentVersionAfter <= options.SinceVersion {
			continue
		}
		filtered = append(filtered, application)
	}
	return filtered
}

func buildSyncDeltaApplicationMetadataSlice(applications []SyncMutationReplayApplication) []SyncDeltaApplicationMetadata {
	metadata := make([]SyncDeltaApplicationMetadata, 0, len(applications))
	for _, application := range applications {
		metadata = append(metadata, SyncDeltaApplicationMetadata{
			MutationID:                     application.MutationID,
			ApplicationStatus:              application.ApplicationStatus,
			ApplicationReason:              application.ApplicationReason,
			CanonicalDocumentVersionBefore: application.CanonicalDocumentVersionBefore,
			CanonicalDocumentVersionAfter:  cloneInt64Pointer(application.CanonicalDocumentVersionAfter),
			CanonicalDocumentHashBefore:    application.CanonicalDocumentHashBefore,
			CanonicalDocumentHashAfter:     cloneStringPointer(application.CanonicalDocumentHashAfter),
			ReplayObservationID:            cloneInt64Pointer(application.ReplayObservationID),
			CreatedAt:                      application.CreatedAt,
		})
	}
	return metadata
}

func currentSyncDeltaApplicationWatermark(applications []SyncMutationReplayApplication) *int64 {
	if len(applications) == 0 {
		return nil
	}
	maxID := applications[0].ID
	for _, application := range applications[1:] {
		if application.ID > maxID {
			maxID = application.ID
		}
	}
	return &maxID
}

func normalizeSyncDeltaMetadataLimit(limit int) int {
	switch {
	case limit <= 0:
		return syncDeltaMetadataDefaultLimit
	case limit > syncDeltaMetadataMaxLimit:
		return syncDeltaMetadataMaxLimit
	default:
		return limit
	}
}

func hasSyncDeltaVersionAdvanceWithoutBodyChange(applications []SyncDeltaApplicationMetadata) bool {
	for _, application := range applications {
		if application.CanonicalDocumentVersionAfter == nil || application.CanonicalDocumentHashAfter == nil {
			continue
		}
		if *application.CanonicalDocumentVersionAfter == application.CanonicalDocumentVersionBefore {
			continue
		}
		if *application.CanonicalDocumentHashAfter == application.CanonicalDocumentHashBefore {
			return true
		}
	}
	return false
}

func getSyncMutationReplayDryRunObservationByIDFromQuerier(ctx context.Context, querier syncMutationReplayQuerier, observationID int64) (SyncMutationReplayDryRunObservation, bool, error) {
	observation, err := scanSyncMutationReplayDryRunObservation(querier.QueryRowContext(
		ctx,
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
			created_at
		FROM sync_mutation_replay_dry_run_observations
		WHERE id = ?`,
		observationID,
	))
	if err != nil {
		if errors.Is(err, ErrSyncMutationReplayDryRunObservationNotFound) {
			return SyncMutationReplayDryRunObservation{}, false, nil
		}
		return SyncMutationReplayDryRunObservation{}, false, err
	}
	return observation, true, nil
}

func (s *Store) loadSyncMutationReplayDryRunInputs(ctx context.Context, ownerKey, appID string) (Document, []SyncMutationReceipt, error) {
	doc, err := s.GetDocument(ctx, ownerKey, appID)
	if err != nil {
		return Document{}, nil, err
	}

	receipts, err := s.listAcceptedSyncMutationReceipts(ctx, ownerKey, appID)
	if err != nil {
		return Document{}, nil, err
	}

	return doc, receipts, nil
}

func buildSyncMutationDryRunResult(doc Document, receipts []SyncMutationReceipt) (SyncMutationDryRunResult, error) {
	snapshot, err := decodeReplaySnapshot(doc.Body)
	if err != nil {
		return SyncMutationDryRunResult{}, err
	}

	tabs, err := decodeReplayTabs(snapshot[replayTabsSnapshotKey])
	if err != nil {
		return SyncMutationDryRunResult{}, err
	}

	result := SyncMutationDryRunResult{
		SourceVersion:     doc.Version,
		ConsideredCount:   len(receipts),
		OrderedMutationID: make([]string, 0, len(receipts)),
		MutationResults:   make([]SyncMutationDryRunMutationResult, 0, len(receipts)),
		Warnings:          make([]SyncMutationDryRunWarning, 0),
	}

	for _, receipt := range receipts {
		result.OrderedMutationID = append(result.OrderedMutationID, receipt.MutationID)
		outcome := "skipped"

		switch receipt.OperationType {
		case "CreateNode":
			if replayCreateNode(&tabs, receipt, &result) {
				outcome = "applied"
			}
		case "UpdateNode":
			if replayUpdateNode(tabs, receipt, &result) {
				outcome = "applied"
			}
		case "MoveNode":
			if replayMoveNode(tabs, receipt, &result) {
				outcome = "applied"
			}
		case "DeleteNode":
			if replayDeleteNode(tabs, receipt, &result) {
				outcome = "applied"
			}
		case "CreateEdge":
			if replayCreateEdge(tabs, receipt, &result) {
				outcome = "applied"
			}
		case "DeleteEdge":
			if replayDeleteEdge(tabs, receipt, &result) {
				outcome = "applied"
			}
		case "CreateTab":
			if replayCreateTab(&tabs, receipt, &result) {
				outcome = "applied"
			}
		case "UpdateTab":
			if replayUpdateTab(tabs, receipt, &result) {
				outcome = "applied"
			}
		case "DeleteTab":
			if replayDeleteTab(&tabs, receipt, &result) {
				outcome = "applied"
			}
		case "ReorderTabs":
			if replayReorderTabs(&tabs, receipt, &result) {
				outcome = "applied"
			}
		case "ClearTabNodes":
			if replayClearTabNodes(tabs, receipt, &result) {
				outcome = "applied"
			}
		default:
			recordReplayWarning(&result, receipt, replayWarningUnknownOperation, "dry-run replay skipped an unknown operation type")
		}

		if outcome == "applied" {
			result.AppliedCount++
		} else {
			result.SkippedCount++
		}

		result.MutationResults = append(result.MutationResults, SyncMutationDryRunMutationResult{
			MutationID:    receipt.MutationID,
			OperationType: receipt.OperationType,
			Outcome:       outcome,
		})
	}

	previewBody, err := encodeReplaySnapshot(snapshot, tabs)
	if err != nil {
		return SyncMutationDryRunResult{}, err
	}

	result.PreviewBody = previewBody
	result.WarningCount = len(result.Warnings)
	return result, nil
}

func buildSyncMutationReplayAuthoritativeProgressivePreview(doc Document, receipts []SyncMutationReceipt) (syncMutationReplayAuthoritativeProgressivePreview, error) {
	snapshot, err := decodeReplaySnapshot(doc.Body)
	if err != nil {
		return syncMutationReplayAuthoritativeProgressivePreview{}, err
	}

	tabs, err := decodeReplayTabs(snapshot[replayTabsSnapshotKey])
	if err != nil {
		return syncMutationReplayAuthoritativeProgressivePreview{}, err
	}

	preview := syncMutationReplayAuthoritativeProgressivePreview{
		MutationResults:    make([]SyncMutationReplayAuthoritativeMutationResult, 0, len(receipts)),
		StagedApplications: make([]syncMutationReplayAuthoritativeStagedApplication, 0, len(receipts)),
		Warnings:           []string{},
	}

	for _, receipt := range receipts {
		policyEvaluation := evaluateSyncMutationReplayAuthoritativePolicyAgainstTabs(tabs, receipt)
		preview.Warnings = mergeReplayStringSets(preview.Warnings, policyEvaluation.Warnings)
		switch policyEvaluation.Status {
		case replayAuthoritativePolicyStatusAllowed:
			if !applyReplayMutationForAuthoritativePreview(&tabs, receipt) {
				preview.PolicyAbort = true
				preview.PolicyAbortMutationID = receipt.MutationID
				preview.PolicyAbortOperationType = receipt.OperationType
				preview.PolicyAbortStatus = replayAuthoritativePolicyStatusFatal
				preview.PolicyAbortReasons = []string{"apply_after_allowed_failed"}
				return preview, nil
			}
			preview.MutationResults = append(preview.MutationResults, SyncMutationReplayAuthoritativeMutationResult{
				MutationID:        receipt.MutationID,
				OperationType:     receipt.OperationType,
				ApplicationStatus: SyncMutationReplayApplicationStatusApplied,
				ApplicationReason: "policy_allowed",
			})
			preview.StagedApplications = append(preview.StagedApplications, syncMutationReplayAuthoritativeStagedApplication{
				MutationID:        receipt.MutationID,
				OperationType:     receipt.OperationType,
				ApplicationStatus: SyncMutationReplayApplicationStatusApplied,
				ApplicationReason: "policy_allowed",
			})
		case replayAuthoritativePolicyStatusSkip:
			applicationReason := buildSyncMutationReplayApplicationReason(policyEvaluation)
			preview.MutationResults = append(preview.MutationResults, SyncMutationReplayAuthoritativeMutationResult{
				MutationID:        receipt.MutationID,
				OperationType:     receipt.OperationType,
				ApplicationStatus: SyncMutationReplayApplicationStatusSkipped,
				ApplicationReason: applicationReason,
			})
			preview.StagedApplications = append(preview.StagedApplications, syncMutationReplayAuthoritativeStagedApplication{
				MutationID:        receipt.MutationID,
				OperationType:     receipt.OperationType,
				ApplicationStatus: SyncMutationReplayApplicationStatusSkipped,
				ApplicationReason: applicationReason,
			})
		case replayAuthoritativePolicyStatusConflict, replayAuthoritativePolicyStatusBlocked, replayAuthoritativePolicyStatusFatal:
			diagnosticStatus, ok := MapSyncMutationReplayPolicyStatusToApplicationStatus(policyEvaluation.Status)
			if ok {
				preview.MutationResults = append(preview.MutationResults, SyncMutationReplayAuthoritativeMutationResult{
					MutationID:        receipt.MutationID,
					OperationType:     receipt.OperationType,
					ApplicationStatus: diagnosticStatus,
					ApplicationReason: buildSyncMutationReplayApplicationReason(policyEvaluation),
				})
			}
			preview.PolicyAbort = true
			preview.PolicyAbortMutationID = receipt.MutationID
			preview.PolicyAbortOperationType = receipt.OperationType
			preview.PolicyAbortStatus = policyEvaluation.Status
			preview.PolicyAbortReasons = cloneStringSlice(policyEvaluation.Reasons)
			return preview, nil
		default:
			preview.PolicyAbort = true
			preview.PolicyAbortMutationID = receipt.MutationID
			preview.PolicyAbortOperationType = receipt.OperationType
			preview.PolicyAbortStatus = replayAuthoritativePolicyStatusFatal
			preview.PolicyAbortReasons = []string{"unknown_policy_status"}
			return preview, nil
		}
	}

	previewBody, err := encodeReplaySnapshot(snapshot, tabs)
	if err != nil {
		return syncMutationReplayAuthoritativeProgressivePreview{}, err
	}
	preview.PreviewBody = previewBody
	return preview, nil
}

func buildSyncMutationReplayDryRunObservation(doc Document, receipts []SyncMutationReceipt, result SyncMutationDryRunResult, createdAt time.Time) SyncMutationReplayDryRunObservation {
	firstOrderedMutationID := ""
	lastOrderedMutationID := ""
	if len(result.OrderedMutationID) > 0 {
		firstOrderedMutationID = result.OrderedMutationID[0]
		lastOrderedMutationID = result.OrderedMutationID[len(result.OrderedMutationID)-1]
	}

	return SyncMutationReplayDryRunObservation{
		OwnerKey:                         doc.OwnerKey,
		AppID:                            doc.AppID,
		CanonicalDocumentVersionObserved: doc.Version,
		CanonicalDocumentHashObserved:    hashReplayObservationBytes(doc.Body),
		ReceiptCountConsidered:           result.ConsideredCount,
		FirstOrderedMutationID:           firstOrderedMutationID,
		LastOrderedMutationID:            lastOrderedMutationID,
		OrderedReceiptHighWatermark:      buildReplayReceiptHighWatermark(receipts),
		AppliedCount:                     result.AppliedCount,
		SkippedCount:                     result.SkippedCount,
		WarningCount:                     result.WarningCount,
		PreviewHash:                      hashReplayObservationBytes(result.PreviewBody),
		CreatedAt:                        createdAt.UTC(),
	}
}

func buildSyncMutationReplayCompareAndApplyPreconditions(observation SyncMutationReplayDryRunObservation) SyncMutationReplayCompareAndApplyPreconditions {
	return SyncMutationReplayCompareAndApplyPreconditions{
		OwnerKey:                         observation.OwnerKey,
		AppID:                            observation.AppID,
		ReplayObservationID:              observation.ID,
		ExpectedCanonicalDocumentVersion: observation.CanonicalDocumentVersionObserved,
		ExpectedCanonicalDocumentHash:    observation.CanonicalDocumentHashObserved,
		ExpectedReceiptCount:             observation.ReceiptCountConsidered,
		ExpectedFirstMutationID:          observation.FirstOrderedMutationID,
		ExpectedLastMutationID:           observation.LastOrderedMutationID,
		ExpectedReceiptHighWatermark:     observation.OrderedReceiptHighWatermark,
		ExpectedPreviewHash:              observation.PreviewHash,
	}
}

func evaluateSyncMutationReplayCompareAndApplyAgainstState(observation SyncMutationReplayDryRunObservation, currentDoc Document, currentReceipts []SyncMutationReceipt, currentResult SyncMutationDryRunResult, currentApplications []SyncMutationReplayApplication) (SyncMutationReplayCompareAndApplyEvaluation, error) {
	snapshot, err := decodeReplaySnapshot(currentDoc.Body)
	if err != nil {
		return SyncMutationReplayCompareAndApplyEvaluation{}, err
	}

	tabs, err := decodeReplayTabs(snapshot[replayTabsSnapshotKey])
	if err != nil {
		return SyncMutationReplayCompareAndApplyEvaluation{}, err
	}

	preconditions := buildSyncMutationReplayCompareAndApplyPreconditions(observation)
	evaluation := SyncMutationReplayCompareAndApplyEvaluation{
		Status:        SyncMutationReplayCompareAndApplyStatusAllowed,
		Preconditions: preconditions,
		Reasons:       make([]string, 0, 4),
		Warnings:      make([]string, 0),
	}

	snapshotAnalysis := analyzeReplaySnapshotForAuthoritativePolicy(tabs)
	evaluation.Warnings = snapshotAnalysis.warnings()
	evaluation.AppliedMutationIDs = matchingReplayApplicationMutationIDs(currentReceipts, currentApplications)
	if len(evaluation.AppliedMutationIDs) > 0 {
		evaluation.Status = SyncMutationReplayCompareAndApplyStatusAlreadyApplied
		evaluation.Reasons = append(evaluation.Reasons, "application_rows_exist_for_candidate_receipts")
		return evaluation, nil
	}

	if snapshotAnalysis.DuplicateItemIDs {
		evaluation.Reasons = append(evaluation.Reasons, "snapshot_contains_duplicate_item_ids")
	}
	if snapshotAnalysis.DuplicateEdgeIDs {
		evaluation.Reasons = append(evaluation.Reasons, "snapshot_contains_duplicate_edge_ids")
	}
	if len(evaluation.Reasons) > 0 {
		evaluation.Status = SyncMutationReplayCompareAndApplyStatusBlockedSnapshot
		return evaluation, nil
	}

	if currentDoc.Version != preconditions.ExpectedCanonicalDocumentVersion {
		evaluation.Reasons = append(evaluation.Reasons, "canonical_document_version_changed")
	}
	if hashReplayObservationBytes(currentDoc.Body) != preconditions.ExpectedCanonicalDocumentHash {
		evaluation.Reasons = append(evaluation.Reasons, "canonical_document_hash_changed")
	}
	if len(evaluation.Reasons) > 0 {
		evaluation.Status = SyncMutationReplayCompareAndApplyStatusStaleCanonicalDocument
		return evaluation, nil
	}

	receiptCount, firstMutationID, lastMutationID, highWatermark := summarizeReplayReceiptOrdering(currentReceipts)
	if receiptCount != preconditions.ExpectedReceiptCount {
		evaluation.Reasons = append(evaluation.Reasons, "receipt_count_changed")
	}
	if firstMutationID != preconditions.ExpectedFirstMutationID {
		evaluation.Reasons = append(evaluation.Reasons, "first_mutation_id_changed")
	}
	if lastMutationID != preconditions.ExpectedLastMutationID {
		evaluation.Reasons = append(evaluation.Reasons, "last_mutation_id_changed")
	}
	if highWatermark != preconditions.ExpectedReceiptHighWatermark {
		evaluation.Reasons = append(evaluation.Reasons, "receipt_high_watermark_changed")
	}
	if hashReplayObservationBytes(currentResult.PreviewBody) != preconditions.ExpectedPreviewHash {
		evaluation.Reasons = append(evaluation.Reasons, "preview_hash_changed_since_observation")
	}
	if len(evaluation.Reasons) > 0 {
		evaluation.Status = SyncMutationReplayCompareAndApplyStatusStaleReceiptSet
		return evaluation, nil
	}

	evaluation.AllowedForFutureTransaction = true
	return evaluation, nil
}

func evaluateSyncMutationReplayRecoveryAgainstState(observation SyncMutationReplayDryRunObservation, currentDoc Document, currentReceipts []SyncMutationReceipt, currentResult SyncMutationDryRunResult, currentApplications []SyncMutationReplayApplication, compareEvaluation SyncMutationReplayCompareAndApplyEvaluation) SyncMutationReplayRecoveryEvaluation {
	recovery := SyncMutationReplayRecoveryEvaluation{
		Status:                    SyncMutationReplayRecoveryStatusMissingObservation,
		Reasons:                   []string{"missing_observation"},
		Warnings:                  cloneStringSlice(compareEvaluation.Warnings),
		CompareAndApplyEvaluation: compareEvaluation,
		AppliedMutationIDs:        cloneStringSlice(compareEvaluation.AppliedMutationIDs),
	}

	matchingApplications := matchingReplayApplications(currentReceipts, currentApplications)
	recovery.MatchingApplicationRowCount = len(matchingApplications)
	currentCanonicalHash := hashReplayObservationBytes(currentDoc.Body)

	if len(matchingApplications) == 0 &&
		len(currentReceipts) > 0 &&
		currentCanonicalHash == observation.PreviewHash &&
		currentCanonicalHash != observation.CanonicalDocumentHashObserved {
		recovery.Status = SyncMutationReplayRecoveryStatusCanonicalStateWithoutApplicationRows
		recovery.Reasons = []string{"current_canonical_matches_observed_preview_without_application_rows"}
		return recovery
	}

	switch compareEvaluation.Status {
	case SyncMutationReplayCompareAndApplyStatusAllowed:
		recovery.Status = SyncMutationReplayRecoveryStatusSafeToAttemptTransaction
		recovery.Reasons = []string{}
	case SyncMutationReplayCompareAndApplyStatusStaleCanonicalDocument, SyncMutationReplayCompareAndApplyStatusStaleReceiptSet:
		recovery.Status = SyncMutationReplayRecoveryStatusStaleObservationRequiresRedryrun
		recovery.Reasons = cloneStringSlice(compareEvaluation.Reasons)
	case SyncMutationReplayCompareAndApplyStatusBlockedSnapshot:
		recovery.Status = SyncMutationReplayRecoveryStatusBlockedSnapshotRequiresCleanup
		recovery.Reasons = cloneStringSlice(compareEvaluation.Reasons)
	case SyncMutationReplayCompareAndApplyStatusAlreadyApplied:
		if len(matchingApplications) < len(currentReceipts) {
			recovery.Status = SyncMutationReplayRecoveryStatusPartialApplicationRows
			recovery.Reasons = []string{"application_rows_exist_for_subset_of_candidate_receipts"}
			return recovery
		}

		mismatchReasons := replayApplicationCanonicalMismatchReasons(matchingApplications, currentDoc)
		if len(mismatchReasons) > 0 {
			recovery.Status = SyncMutationReplayRecoveryStatusApplicationRowsWithoutMatchingCanonicalState
			recovery.Reasons = mismatchReasons
			return recovery
		}

		recovery.Status = SyncMutationReplayRecoveryStatusAlreadyAppliedRequiresIdempotentExit
		recovery.Reasons = cloneStringSlice(compareEvaluation.Reasons)
	case SyncMutationReplayCompareAndApplyStatusMissingObservation:
		recovery.Status = SyncMutationReplayRecoveryStatusMissingObservation
		recovery.Reasons = cloneStringSlice(compareEvaluation.Reasons)
	case SyncMutationReplayCompareAndApplyStatusInvalidObservationScope:
		recovery.Status = SyncMutationReplayRecoveryStatusInvalidObservationScope
		recovery.Reasons = cloneStringSlice(compareEvaluation.Reasons)
	default:
		recovery.Status = SyncMutationReplayRecoveryStatusStaleObservationRequiresRedryrun
		recovery.Reasons = cloneStringSlice(compareEvaluation.Reasons)
	}

	return recovery
}

func EvaluateSyncMutationReplayAuthoritativePolicy(body json.RawMessage, receipt SyncMutationReceipt) (SyncMutationReplayAuthoritativePolicyEvaluation, error) {
	snapshot, err := decodeReplaySnapshot(body)
	if err != nil {
		return SyncMutationReplayAuthoritativePolicyEvaluation{}, err
	}

	tabs, err := decodeReplayTabs(snapshot[replayTabsSnapshotKey])
	if err != nil {
		return SyncMutationReplayAuthoritativePolicyEvaluation{}, err
	}

	return evaluateSyncMutationReplayAuthoritativePolicyAgainstTabs(tabs, receipt), nil
}

func EvaluateSyncMutationReplayAuthoritativeReadiness(observation SyncMutationReplayDryRunObservation, currentDoc Document, currentReceipts []SyncMutationReceipt) (SyncMutationReplayAuthoritativeReadiness, error) {
	snapshot, err := decodeReplaySnapshot(currentDoc.Body)
	if err != nil {
		return SyncMutationReplayAuthoritativeReadiness{}, err
	}

	tabs, err := decodeReplayTabs(snapshot[replayTabsSnapshotKey])
	if err != nil {
		return SyncMutationReplayAuthoritativeReadiness{}, err
	}

	readiness := SyncMutationReplayAuthoritativeReadiness{
		Areas: make([]SyncMutationReplayAuthoritativeAreaReadiness, 0, 24),
	}
	snapshotAnalysis := analyzeReplaySnapshotForAuthoritativePolicy(tabs)

	appendReplayAuthoritativeArea(&readiness, "document_version_gating", replayAuthoritativeStatusReady, "CLI-authoritative apply verifies the observed canonical version and hash immediately before the transaction commits.")
	appendReplayAuthoritativeArea(&readiness, "receipt_selection", replayAuthoritativeStatusPartiallyReady, "Accepted receipts are scoped by owner and app and protected by application-row boundaries, but receipt selection is still whole-set rather than operator-subset or causal-merge based.")
	appendReplayAuthoritativeArea(&readiness, "replay_ordering", replayAuthoritativeStatusPartiallyReady, "Deterministic ordering exists by accepted_at, created_at, and mutation_id, but it is not a full causal merge model.")
	appendReplayAuthoritativeArea(&readiness, "transaction_boundary", replayAuthoritativeStatusReady, "Store-level CLI-authoritative apply writes the canonical snapshot and replay application rows in one SQLite transaction.")
	appendReplayAuthoritativeArea(&readiness, "canonical_snapshot_observation", replayAuthoritativeStatusReady, "Dry-run observations capture version and hash, can be created by the CLI, and gate CLI-authoritative canonical writes.")
	appendReplayAuthoritativeArea(&readiness, "receipt_status_model", replayAuthoritativeStatusReady, "Replay application rows durably distinguish applied, skipped, conflict, and failed outcomes without changing receipt ACK meaning.")
	appendReplayAuthoritativeArea(&readiness, "replay_progress_state", replayAuthoritativeStatusReady, "Dry-run observations and durable application rows are compared for stale receipt sets, partial progress, and already-applied idempotent exits.")
	appendReplayAuthoritativeArea(&readiness, "rollback_behavior", replayAuthoritativeStatusPartiallyReady, "Recovery evaluation can identify stale observations, blocked snapshots, partial application rows, and canonical/application mismatches, but there is still no automated rollback transaction.")
	appendReplayAuthoritativeArea(&readiness, "idempotency", replayAuthoritativeStatusReady, "Receipt acceptance is idempotent and repeated CLI-authoritative apply exits idempotently when application rows match canonical state.")
	appendReplayAuthoritativeArea(&readiness, "conflict_behavior", replayAuthoritativeStatusPartiallyReady, "Conflict application statuses and policy-abort previews exist, but conflict resolution remains snapshot/operator based rather than a mutation-level merge model.")
	appendReplayAuthoritativeArea(&readiness, "malformed_legacy_snapshot_policy", replayAuthoritativeStatusPartiallyReady, "Policy preserves unrelated malformed legacy data during replay admission, but legacy cleanup remains a separate future decision.")
	appendReplayAuthoritativeArea(&readiness, "duplicate_item_id_policy", replayAuthoritativeStatusReady, "Policy blocks authoritative replay when the snapshot contains duplicate item ids.")
	appendReplayAuthoritativeArea(&readiness, "duplicate_edge_id_policy", replayAuthoritativeStatusReady, "Policy blocks authoritative replay when the snapshot contains duplicate edge ids.")
	appendReplayAuthoritativeArea(&readiness, "duplicate_endpoint_edge_policy", replayAuthoritativeStatusReady, "Dry-run matches the frontend rule that duplicate endpoint pairs are rejected in either direction regardless of edge kind.")
	appendReplayAuthoritativeArea(&readiness, "self_edge_policy", replayAuthoritativeStatusReady, "Policy skips new self-edge mutations while preserving unrelated legacy self-edges.")
	appendReplayAuthoritativeArea(&readiness, "missing_endpoint_policy", replayAuthoritativeStatusReady, "Policy preserves unrelated malformed legacy edges while rejecting new missing-endpoint mutations.")
	appendReplayAuthoritativeArea(&readiness, "missing_tab_policy", replayAuthoritativeStatusReady, "Replay never creates tabs implicitly and reports missing_tab deterministically.")
	appendReplayAuthoritativeArea(&readiness, "per_tab_item_limit", replayAuthoritativeStatusReady, "CLI-authoritative preview and apply skip CreateNode mutations that would exceed the per-tab item limit.")
	appendReplayAuthoritativeArea(&readiness, "per_tab_edge_limit", replayAuthoritativeStatusReady, "CLI-authoritative preview and apply skip CreateEdge mutations that would exceed the per-tab edge limit.")
	appendReplayAuthoritativeArea(&readiness, "item_width_height_semantics", replayAuthoritativeStatusReady, "CLI-authoritative policy preserves literal stored width and height values and rejects invalid dimensions instead of silently deriving them.")
	appendReplayAuthoritativeArea(&readiness, "shape_specific_sizing_behavior", replayAuthoritativeStatusReady, "CLI-authoritative policy preserves literal dimensions for fixed-ratio shapes and reports that divergence in authoritative preview.")
	appendReplayAuthoritativeArea(&readiness, "text_length_limit_behavior", replayAuthoritativeStatusReady, "CLI-authoritative preview and apply skip oversize text mutations instead of preserving or truncating them silently.")
	appendReplayAuthoritativeArea(&readiness, "unknown_fields", replayAuthoritativeStatusReady, "Policy skips unknown update fields deterministically.")
	appendReplayAuthoritativeArea(&readiness, "unknown_operations", replayAuthoritativeStatusReady, "Receipt acceptance rejects unsupported operations, and policy evaluation treats unknown operations as fatal.")
	appendReplayAuthoritativeArea(&readiness, "server_side_validation_requirements", replayAuthoritativeStatusReady, "Validation policy is wired into the CLI-authoritative compare-and-apply path and its read-only authoritative preview.")
	appendReplayAuthoritativeArea(&readiness, "applied_receipt_tracking", replayAuthoritativeStatusReady, "Durable application rows provide the applied-receipt boundary for CLI recovery, idempotency, and delta metadata.")
	appendReplayAuthoritativeArea(&readiness, "delta_pull", replayAuthoritativeStatusIntentionallyDeferred, "Revisioned delta pull remains out of scope until a separate behavior-changing phase proves it is safe.")

	addReplayAuthoritativeBlocker(&readiness, "authoritative_replay_not_product_path")
	addReplayAuthoritativeBlockerIf(&readiness, currentDoc.Version != observation.CanonicalDocumentVersionObserved, "canonical_snapshot_changed_since_observation")
	addReplayAuthoritativeBlockerIf(&readiness, hashReplayObservationBytes(currentDoc.Body) != observation.CanonicalDocumentHashObserved, "canonical_snapshot_changed_since_observation")

	receiptCount, firstMutationID, lastMutationID, highWatermark := summarizeReplayReceiptOrdering(currentReceipts)
	addReplayAuthoritativeBlockerIf(&readiness, receiptCount != observation.ReceiptCountConsidered, "receipt_set_changed_since_observation")
	addReplayAuthoritativeBlockerIf(&readiness, firstMutationID != observation.FirstOrderedMutationID, "receipt_set_changed_since_observation")
	addReplayAuthoritativeBlockerIf(&readiness, lastMutationID != observation.LastOrderedMutationID, "receipt_set_changed_since_observation")
	addReplayAuthoritativeBlockerIf(&readiness, highWatermark != observation.OrderedReceiptHighWatermark, "receipt_set_changed_since_observation")
	addReplayAuthoritativeBlockerIf(&readiness, snapshotAnalysis.DuplicateItemIDs, "snapshot_contains_duplicate_item_ids")
	addReplayAuthoritativeBlockerIf(&readiness, snapshotAnalysis.DuplicateEdgeIDs, "snapshot_contains_duplicate_edge_ids")

	readiness.Warnings = snapshotAnalysis.warnings()
	readiness.Ready = len(readiness.Blockers) == 0
	return readiness, nil
}

func evaluateSyncMutationReplayAuthoritativePolicyAgainstTabs(tabs []replayTab, receipt SyncMutationReceipt) SyncMutationReplayAuthoritativePolicyEvaluation {
	analysis := analyzeReplaySnapshotForAuthoritativePolicy(tabs)
	warnings := analysis.warnings()
	reasons := make([]string, 0, 2)
	if analysis.DuplicateItemIDs {
		reasons = append(reasons, "snapshot_contains_duplicate_item_ids")
	}
	if analysis.DuplicateEdgeIDs {
		reasons = append(reasons, "snapshot_contains_duplicate_edge_ids")
	}
	if len(reasons) > 0 {
		return SyncMutationReplayAuthoritativePolicyEvaluation{
			Status:   replayAuthoritativePolicyStatusBlocked,
			Reasons:  reasons,
			Warnings: warnings,
		}
	}

	entityID := normalizeReplayIdentifier(receipt.EntityID)
	if entityID == "" {
		return replayAuthoritativeFatalPolicyEvaluation(warnings, "missing_entity_id")
	}

	switch receipt.OperationType {
	case "CreateNode":
		return evaluateReplayAuthoritativeCreateNodePolicy(tabs, receipt, entityID, warnings)
	case "UpdateNode":
		return evaluateReplayAuthoritativeUpdateNodePolicy(tabs, receipt, entityID, warnings)
	case "MoveNode":
		return evaluateReplayAuthoritativeMoveNodePolicy(tabs, receipt, entityID, warnings)
	case "DeleteNode":
		return evaluateReplayAuthoritativeDeleteNodePolicy(tabs, receipt, entityID, warnings)
	case "CreateEdge":
		return evaluateReplayAuthoritativeCreateEdgePolicy(tabs, receipt, entityID, warnings)
	case "DeleteEdge":
		return evaluateReplayAuthoritativeDeleteEdgePolicy(tabs, receipt, entityID, warnings)
	case "CreateTab":
		return evaluateReplayAuthoritativeCreateTabPolicy(tabs, receipt, entityID, warnings)
	case "UpdateTab":
		return evaluateReplayAuthoritativeUpdateTabPolicy(tabs, receipt, entityID, warnings)
	case "DeleteTab":
		return evaluateReplayAuthoritativeDeleteTabPolicy(tabs, receipt, entityID, warnings)
	case "ReorderTabs":
		return evaluateReplayAuthoritativeReorderTabsPolicy(tabs, receipt, warnings)
	case "ClearTabNodes":
		return evaluateReplayAuthoritativeClearTabNodesPolicy(tabs, receipt, entityID, warnings)
	default:
		return replayAuthoritativeFatalPolicyEvaluation(warnings, "unknown_operation")
	}
}

func evaluateReplayAuthoritativeCreateNodePolicy(tabs []replayTab, receipt SyncMutationReceipt, entityID string, warnings []string) SyncMutationReplayAuthoritativePolicyEvaluation {
	var payload struct {
		TabID    string         `json:"tabId"`
		Name     string         `json:"name"`
		Position replayPosition `json:"position"`
		Shape    string         `json:"shape"`
		Width    *float64       `json:"width"`
		Height   *float64       `json:"height"`
	}
	if err := json.Unmarshal(receipt.Payload, &payload); err != nil {
		return replayAuthoritativeFatalPolicyEvaluation(warnings, "invalid_payload")
	}

	tab := findReplayTab(tabs, normalizeReplayIdentifier(payload.TabID))
	if tab == nil {
		return replayAuthoritativeConflictPolicyEvaluation(warnings, "missing_tab")
	}
	if findReplayItemAcrossTabs(tabs, entityID) != nil {
		return replayAuthoritativeSkipPolicyEvaluation(warnings, "duplicate_item_id_existing")
	}
	if len(tab.Items) >= replayItemsPerTabMax {
		return replayAuthoritativeSkipPolicyEvaluation(warnings, "item_limit_exceeded")
	}
	if countReplayAuthoritativeTextUnits(payload.Name) > replayItemTextCharsMax {
		return replayAuthoritativeSkipPolicyEvaluation(warnings, "text_limit_exceeded")
	}
	if _, ok := normalizeReplayPosition(payload.Position); !ok {
		return replayAuthoritativeSkipPolicyEvaluation(warnings, "invalid_position")
	}
	if !replayAuthoritativeDimensionsAllowed(payload.Width, payload.Height) {
		return replayAuthoritativeSkipPolicyEvaluation(warnings, "invalid_dimensions")
	}
	return replayAuthoritativeAllowedPolicyEvaluation(warnings)
}

func evaluateReplayAuthoritativeUpdateNodePolicy(tabs []replayTab, receipt SyncMutationReceipt, entityID string, warnings []string) SyncMutationReplayAuthoritativePolicyEvaluation {
	var payload struct {
		TabID   string                     `json:"tabId"`
		Changes map[string]json.RawMessage `json:"changes"`
	}
	if err := json.Unmarshal(receipt.Payload, &payload); err != nil {
		return replayAuthoritativeFatalPolicyEvaluation(warnings, "invalid_payload")
	}

	tab := findReplayTab(tabs, normalizeReplayIdentifier(payload.TabID))
	if tab == nil {
		return replayAuthoritativeConflictPolicyEvaluation(warnings, "missing_tab")
	}
	item := findReplayItem(tab, entityID)
	if item == nil {
		return replayAuthoritativeConflictPolicyEvaluation(warnings, "missing_node")
	}

	resultWarnings := cloneReplayWarnings(warnings)
	targetShape := normalizeReplayShape(item.Shape)
	widthPresent := false
	heightPresent := false

	for field, rawValue := range payload.Changes {
		switch field {
		case "name", "text":
			var value string
			if err := json.Unmarshal(rawValue, &value); err != nil {
				return replayAuthoritativeFatalPolicyEvaluation(resultWarnings, "invalid_payload")
			}
			if countReplayAuthoritativeTextUnits(value) > replayItemTextCharsMax {
				return replayAuthoritativeSkipPolicyEvaluation(resultWarnings, "text_limit_exceeded")
			}
		case "color":
			var value string
			if err := json.Unmarshal(rawValue, &value); err != nil {
				return replayAuthoritativeFatalPolicyEvaluation(resultWarnings, "invalid_payload")
			}
		case "shape":
			var value string
			if err := json.Unmarshal(rawValue, &value); err != nil {
				return replayAuthoritativeFatalPolicyEvaluation(resultWarnings, "invalid_payload")
			}
			targetShape = normalizeReplayShape(value)
		case "width":
			value, ok := decodeReplayNumber(rawValue)
			if !ok || !replayAuthoritativeDimensionWithinBounds(value, replayItemWidthMin, replayItemWidthMax) {
				return replayAuthoritativeSkipPolicyEvaluation(resultWarnings, "invalid_dimensions")
			}
			widthPresent = true
		case "height":
			value, ok := decodeReplayNumber(rawValue)
			if !ok || !replayAuthoritativeDimensionWithinBounds(value, replayItemHeightMin, replayItemHeightMax) {
				return replayAuthoritativeSkipPolicyEvaluation(resultWarnings, "invalid_dimensions")
			}
			heightPresent = true
		default:
			return replayAuthoritativeSkipPolicyEvaluation(resultWarnings, "unknown_update_field")
		}
	}

	if replayShapeUsesFixedRatio(targetShape) && (widthPresent != heightPresent) {
		resultWarnings = appendReplayWarningIfMissing(resultWarnings, "preserve_literal_fixed_ratio_dimensions")
	}
	return replayAuthoritativeAllowedPolicyEvaluation(resultWarnings)
}

func evaluateReplayAuthoritativeMoveNodePolicy(tabs []replayTab, receipt SyncMutationReceipt, entityID string, warnings []string) SyncMutationReplayAuthoritativePolicyEvaluation {
	var payload struct {
		TabID    string         `json:"tabId"`
		Position replayPosition `json:"position"`
	}
	if err := json.Unmarshal(receipt.Payload, &payload); err != nil {
		return replayAuthoritativeFatalPolicyEvaluation(warnings, "invalid_payload")
	}

	tab := findReplayTab(tabs, normalizeReplayIdentifier(payload.TabID))
	if tab == nil {
		return replayAuthoritativeConflictPolicyEvaluation(warnings, "missing_tab")
	}
	if findReplayItem(tab, entityID) == nil {
		return replayAuthoritativeConflictPolicyEvaluation(warnings, "missing_node")
	}
	if _, ok := normalizeReplayPosition(payload.Position); !ok {
		return replayAuthoritativeSkipPolicyEvaluation(warnings, "invalid_position")
	}
	return replayAuthoritativeAllowedPolicyEvaluation(warnings)
}

func evaluateReplayAuthoritativeDeleteNodePolicy(tabs []replayTab, receipt SyncMutationReceipt, entityID string, warnings []string) SyncMutationReplayAuthoritativePolicyEvaluation {
	var payload struct {
		TabID string `json:"tabId"`
	}
	if err := json.Unmarshal(receipt.Payload, &payload); err != nil {
		return replayAuthoritativeFatalPolicyEvaluation(warnings, "invalid_payload")
	}

	tab := findReplayTab(tabs, normalizeReplayIdentifier(payload.TabID))
	if tab == nil {
		return replayAuthoritativeConflictPolicyEvaluation(warnings, "missing_tab")
	}
	if findReplayItem(tab, entityID) == nil {
		return replayAuthoritativeSkipPolicyEvaluation(warnings, "missing_node")
	}
	return replayAuthoritativeAllowedPolicyEvaluation(warnings)
}

func evaluateReplayAuthoritativeCreateEdgePolicy(tabs []replayTab, receipt SyncMutationReceipt, entityID string, warnings []string) SyncMutationReplayAuthoritativePolicyEvaluation {
	var payload struct {
		TabID      string `json:"tabId"`
		FromItemID string `json:"fromItemId"`
		ToItemID   string `json:"toItemId"`
	}
	if err := json.Unmarshal(receipt.Payload, &payload); err != nil {
		return replayAuthoritativeFatalPolicyEvaluation(warnings, "invalid_payload")
	}

	tab := findReplayTab(tabs, normalizeReplayIdentifier(payload.TabID))
	if tab == nil {
		return replayAuthoritativeConflictPolicyEvaluation(warnings, "missing_tab")
	}
	if findReplayEdgeAcrossTabs(tabs, entityID) != nil {
		return replayAuthoritativeSkipPolicyEvaluation(warnings, "duplicate_edge_id_existing")
	}

	fromItemID := normalizeReplayIdentifier(payload.FromItemID)
	toItemID := normalizeReplayIdentifier(payload.ToItemID)
	if fromItemID == "" || toItemID == "" {
		return replayAuthoritativeConflictPolicyEvaluation(warnings, "missing_endpoint")
	}
	if fromItemID == toItemID {
		return replayAuthoritativeSkipPolicyEvaluation(warnings, "self_edge")
	}
	if findReplayEdgeByEndpoints(tab, fromItemID, toItemID) != nil {
		return replayAuthoritativeSkipPolicyEvaluation(warnings, "duplicate_endpoint_edge")
	}
	if findReplayItem(tab, fromItemID) == nil || findReplayItem(tab, toItemID) == nil {
		return replayAuthoritativeConflictPolicyEvaluation(warnings, "missing_endpoint")
	}
	if len(tab.Edges) >= replayEdgesPerTabMax {
		return replayAuthoritativeSkipPolicyEvaluation(warnings, "edge_limit_exceeded")
	}
	return replayAuthoritativeAllowedPolicyEvaluation(warnings)
}

func evaluateReplayAuthoritativeDeleteEdgePolicy(tabs []replayTab, receipt SyncMutationReceipt, entityID string, warnings []string) SyncMutationReplayAuthoritativePolicyEvaluation {
	var payload struct {
		TabID string `json:"tabId"`
	}
	if err := json.Unmarshal(receipt.Payload, &payload); err != nil {
		return replayAuthoritativeFatalPolicyEvaluation(warnings, "invalid_payload")
	}

	tab := findReplayTab(tabs, normalizeReplayIdentifier(payload.TabID))
	if tab == nil {
		return replayAuthoritativeConflictPolicyEvaluation(warnings, "missing_tab")
	}
	if findReplayEdgeIndex(tab, entityID) < 0 {
		return replayAuthoritativeSkipPolicyEvaluation(warnings, "missing_edge")
	}
	return replayAuthoritativeAllowedPolicyEvaluation(warnings)
}

func evaluateReplayAuthoritativeCreateTabPolicy(tabs []replayTab, receipt SyncMutationReceipt, entityID string, warnings []string) SyncMutationReplayAuthoritativePolicyEvaluation {
	var payload struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(receipt.Payload, &payload); err != nil {
		return replayAuthoritativeFatalPolicyEvaluation(warnings, "invalid_payload")
	}

	if findReplayTab(tabs, entityID) != nil {
		return replayAuthoritativeSkipPolicyEvaluation(warnings, "duplicate_tab_id_existing")
	}
	if countReplayAuthoritativeTextUnits(payload.Name) > replayItemTextCharsMax {
		return replayAuthoritativeSkipPolicyEvaluation(warnings, "text_limit_exceeded")
	}
	return replayAuthoritativeAllowedPolicyEvaluation(warnings)
}

func evaluateReplayAuthoritativeUpdateTabPolicy(tabs []replayTab, receipt SyncMutationReceipt, entityID string, warnings []string) SyncMutationReplayAuthoritativePolicyEvaluation {
	var payload struct {
		TabID   string                     `json:"tabId"`
		Changes map[string]json.RawMessage `json:"changes"`
	}
	if err := json.Unmarshal(receipt.Payload, &payload); err != nil {
		return replayAuthoritativeFatalPolicyEvaluation(warnings, "invalid_payload")
	}
	if !replayTabPayloadIDMatchesEntity(entityID, payload.TabID) {
		return replayAuthoritativeFatalPolicyEvaluation(warnings, "invalid_payload")
	}

	tab := findReplayTab(tabs, entityID)
	if tab == nil {
		return replayAuthoritativeConflictPolicyEvaluation(warnings, "missing_tab")
	}
	if len(payload.Changes) == 0 {
		return replayAuthoritativeSkipPolicyEvaluation(warnings, "empty_changes")
	}
	for field, rawValue := range payload.Changes {
		switch field {
		case "name":
			var value string
			if err := json.Unmarshal(rawValue, &value); err != nil {
				return replayAuthoritativeFatalPolicyEvaluation(warnings, "invalid_payload")
			}
			if countReplayAuthoritativeTextUnits(value) > replayItemTextCharsMax {
				return replayAuthoritativeSkipPolicyEvaluation(warnings, "text_limit_exceeded")
			}
		default:
			return replayAuthoritativeSkipPolicyEvaluation(warnings, "unknown_update_field")
		}
	}
	return replayAuthoritativeAllowedPolicyEvaluation(warnings)
}

func evaluateReplayAuthoritativeDeleteTabPolicy(tabs []replayTab, receipt SyncMutationReceipt, entityID string, warnings []string) SyncMutationReplayAuthoritativePolicyEvaluation {
	var payload struct {
		TabID string `json:"tabId"`
	}
	if err := json.Unmarshal(receipt.Payload, &payload); err != nil {
		return replayAuthoritativeFatalPolicyEvaluation(warnings, "invalid_payload")
	}
	if !replayTabPayloadIDMatchesEntity(entityID, payload.TabID) {
		return replayAuthoritativeFatalPolicyEvaluation(warnings, "invalid_payload")
	}
	if findReplayTabIndex(tabs, entityID) < 0 {
		return replayAuthoritativeSkipPolicyEvaluation(warnings, "missing_tab")
	}
	if len(tabs) <= 1 {
		return replayAuthoritativeSkipPolicyEvaluation(warnings, "last_tab")
	}
	return replayAuthoritativeAllowedPolicyEvaluation(warnings)
}

func evaluateReplayAuthoritativeReorderTabsPolicy(tabs []replayTab, receipt SyncMutationReceipt, warnings []string) SyncMutationReplayAuthoritativePolicyEvaluation {
	var payload struct {
		TabIDs []string `json:"tabIds"`
	}
	if err := json.Unmarshal(receipt.Payload, &payload); err != nil {
		return replayAuthoritativeFatalPolicyEvaluation(warnings, "invalid_payload")
	}
	order, ok := normalizeReplayTabOrder(tabs, payload.TabIDs)
	if !ok {
		return replayAuthoritativeSkipPolicyEvaluation(warnings, "invalid_tab_order")
	}
	if replayTabOrderAlreadyMatches(tabs, order) {
		return replayAuthoritativeSkipPolicyEvaluation(warnings, "already_ordered")
	}
	return replayAuthoritativeAllowedPolicyEvaluation(warnings)
}

func evaluateReplayAuthoritativeClearTabNodesPolicy(tabs []replayTab, receipt SyncMutationReceipt, entityID string, warnings []string) SyncMutationReplayAuthoritativePolicyEvaluation {
	var payload struct {
		TabID string `json:"tabId"`
	}
	if err := json.Unmarshal(receipt.Payload, &payload); err != nil {
		return replayAuthoritativeFatalPolicyEvaluation(warnings, "invalid_payload")
	}
	if !replayTabPayloadIDMatchesEntity(entityID, payload.TabID) {
		return replayAuthoritativeFatalPolicyEvaluation(warnings, "invalid_payload")
	}
	tab := findReplayTab(tabs, entityID)
	if tab == nil {
		return replayAuthoritativeConflictPolicyEvaluation(warnings, "missing_tab")
	}
	if len(tab.Items) == 0 {
		return replayAuthoritativeSkipPolicyEvaluation(warnings, "missing_node")
	}
	return replayAuthoritativeAllowedPolicyEvaluation(warnings)
}

func (s *Store) listAcceptedSyncMutationReceipts(ctx context.Context, ownerKey, appID string) ([]SyncMutationReceipt, error) {
	started := time.Now()
	ctx, cancel := withDBTimeout(ctx)
	defer cancel()

	rows, err := s.db.QueryContext(
		ctx,
		`SELECT
			id,
			owner_key,
			app_id,
			mutation_id,
			client_id,
			device_id,
			protocol,
			entity_type,
			entity_id,
			operation_type,
			payload_json,
			base_revision,
			status,
			created_at,
			accepted_at
		FROM sync_mutation_receipts
		WHERE owner_key = ? AND app_id = ? AND status = ?
		ORDER BY accepted_at ASC, created_at ASC, mutation_id ASC`,
		ownerKey,
		appID,
		SyncMutationReceiptStatusAccepted,
	)
	if err != nil {
		return nil, s.wrapDBError("read_list_sync_mutation_receipts", started, fmt.Errorf("list sync mutation receipts: %w", err))
	}
	defer rows.Close()

	receipts := make([]SyncMutationReceipt, 0)
	for rows.Next() {
		receipt, scanErr := scanSyncMutationReceipt(rows)
		if scanErr != nil {
			return nil, s.wrapDBError("read_list_sync_mutation_receipts_scan", started, scanErr)
		}
		receipts = append(receipts, receipt)
	}
	if err := rows.Err(); err != nil {
		return nil, s.wrapDBError("read_list_sync_mutation_receipts_rows", started, fmt.Errorf("iterate sync mutation receipts: %w", err))
	}

	s.logDBOperation("read_list_sync_mutation_receipts", started, nil)
	return receipts, nil
}

func decodeReplaySnapshot(body json.RawMessage) (replaySnapshot, error) {
	var snapshot replaySnapshot
	if err := json.Unmarshal(body, &snapshot); err != nil {
		return nil, fmt.Errorf("decode replay snapshot: %w", err)
	}
	if snapshot == nil {
		return nil, fmt.Errorf("decode replay snapshot: empty snapshot")
	}
	return snapshot, nil
}

func decodeReplayTabs(value string) ([]replayTab, error) {
	if strings.TrimSpace(value) == "" {
		return []replayTab{}, nil
	}

	var tabs []replayTab
	if err := json.Unmarshal([]byte(value), &tabs); err != nil {
		return nil, fmt.Errorf("decode replay tabs: %w", err)
	}
	for index := range tabs {
		if tabs[index].Items == nil {
			tabs[index].Items = []replayItem{}
		}
		if tabs[index].Edges == nil {
			tabs[index].Edges = []replayEdge{}
		}
	}
	return tabs, nil
}

func encodeReplaySnapshot(snapshot replaySnapshot, tabs []replayTab) (json.RawMessage, error) {
	tabsJSON, err := json.Marshal(tabs)
	if err != nil {
		return nil, fmt.Errorf("encode replay tabs: %w", err)
	}

	clone := make(replaySnapshot, len(snapshot))
	for key, value := range snapshot {
		clone[key] = value
	}
	clone[replayTabsSnapshotKey] = string(tabsJSON)

	encoded, err := json.Marshal(clone)
	if err != nil {
		return nil, fmt.Errorf("encode replay snapshot: %w", err)
	}
	return json.RawMessage(encoded), nil
}

func replayCreateTab(tabs *[]replayTab, receipt SyncMutationReceipt, result *SyncMutationDryRunResult) bool {
	var payload struct {
		Name        string `json:"name"`
		ColorIndex  *int   `json:"colorIndex"`
		GridSetting string `json:"gridSetting"`
		OrderIndex  *int   `json:"orderIndex"`
	}
	if err := json.Unmarshal(receipt.Payload, &payload); err != nil {
		recordReplayWarning(result, receipt, replayWarningInvalidPayload, "dry-run replay could not decode create-tab payload")
		return false
	}

	entityID, ok := replayEntityID(receipt, result)
	if !ok {
		return false
	}
	if findReplayTab(*tabs, entityID) != nil {
		return false
	}

	tab := replayTab{
		ID:          entityID,
		Name:        payload.Name,
		Items:       []replayItem{},
		ColorIndex:  normalizeReplayTabColorIndex(payload.ColorIndex),
		GridSetting: normalizeReplayTabGridSetting(payload.GridSetting),
		Edges:       []replayEdge{},
	}
	insertReplayTab(tabs, tab, payload.OrderIndex)
	return true
}

func replayUpdateTab(tabs []replayTab, receipt SyncMutationReceipt, result *SyncMutationDryRunResult) bool {
	var payload struct {
		TabID   string                     `json:"tabId"`
		Changes map[string]json.RawMessage `json:"changes"`
	}
	if err := json.Unmarshal(receipt.Payload, &payload); err != nil {
		recordReplayWarning(result, receipt, replayWarningInvalidPayload, "dry-run replay could not decode update-tab payload")
		return false
	}

	entityID, ok := replayEntityID(receipt, result)
	if !ok {
		return false
	}
	if !replayTabPayloadIDMatchesEntity(entityID, payload.TabID) {
		recordReplayWarning(result, receipt, replayWarningInvalidPayload, "dry-run replay skipped update-tab because the tab id does not match the entity id")
		return false
	}

	tab := findReplayTab(tabs, entityID)
	if tab == nil {
		recordReplayWarning(result, receipt, replayWarningMissingTab, "dry-run replay skipped update-tab because the tab is missing")
		return false
	}

	applied := false
	for field, rawValue := range payload.Changes {
		switch field {
		case "name":
			var value string
			if err := json.Unmarshal(rawValue, &value); err != nil {
				recordReplayWarning(result, receipt, replayWarningInvalidUpdateValue, "dry-run replay skipped a tab name update because the value is invalid")
				continue
			}
			tab.Name = value
			applied = true
		default:
			recordReplayWarning(result, receipt, replayWarningUnknownUpdateField, "dry-run replay skipped an unknown update-tab field")
		}
	}
	return applied
}

func replayDeleteTab(tabs *[]replayTab, receipt SyncMutationReceipt, result *SyncMutationDryRunResult) bool {
	var payload struct {
		TabID string `json:"tabId"`
	}
	if err := json.Unmarshal(receipt.Payload, &payload); err != nil {
		recordReplayWarning(result, receipt, replayWarningInvalidPayload, "dry-run replay could not decode delete-tab payload")
		return false
	}

	entityID, ok := replayEntityID(receipt, result)
	if !ok {
		return false
	}
	if !replayTabPayloadIDMatchesEntity(entityID, payload.TabID) {
		recordReplayWarning(result, receipt, replayWarningInvalidPayload, "dry-run replay skipped delete-tab because the tab id does not match the entity id")
		return false
	}

	tabIndex := findReplayTabIndex(*tabs, entityID)
	if tabIndex < 0 || len(*tabs) <= 1 {
		return false
	}
	*tabs = append((*tabs)[:tabIndex], (*tabs)[tabIndex+1:]...)
	return true
}

func replayReorderTabs(tabs *[]replayTab, receipt SyncMutationReceipt, result *SyncMutationDryRunResult) bool {
	var payload struct {
		TabIDs []string `json:"tabIds"`
	}
	if err := json.Unmarshal(receipt.Payload, &payload); err != nil {
		recordReplayWarning(result, receipt, replayWarningInvalidPayload, "dry-run replay could not decode reorder-tabs payload")
		return false
	}

	order, ok := normalizeReplayTabOrder(*tabs, payload.TabIDs)
	if !ok {
		recordReplayWarning(result, receipt, replayWarningInvalidPayload, "dry-run replay skipped reorder-tabs because the tab ids do not match the current snapshot")
		return false
	}
	if replayTabOrderAlreadyMatches(*tabs, order) {
		return false
	}

	tabByID := make(map[string]replayTab, len(*tabs))
	for _, tab := range *tabs {
		tabByID[tab.ID] = tab
	}
	reordered := make([]replayTab, 0, len(order))
	for _, tabID := range order {
		reordered = append(reordered, tabByID[tabID])
	}
	*tabs = reordered
	return true
}

func replayClearTabNodes(tabs []replayTab, receipt SyncMutationReceipt, result *SyncMutationDryRunResult) bool {
	var payload struct {
		TabID string `json:"tabId"`
	}
	if err := json.Unmarshal(receipt.Payload, &payload); err != nil {
		recordReplayWarning(result, receipt, replayWarningInvalidPayload, "dry-run replay could not decode clear-tab-nodes payload")
		return false
	}

	entityID, ok := replayEntityID(receipt, result)
	if !ok {
		return false
	}
	if !replayTabPayloadIDMatchesEntity(entityID, payload.TabID) {
		recordReplayWarning(result, receipt, replayWarningInvalidPayload, "dry-run replay skipped clear-tab-nodes because the tab id does not match the entity id")
		return false
	}

	tab := findReplayTab(tabs, entityID)
	if tab == nil {
		recordReplayWarning(result, receipt, replayWarningMissingTab, "dry-run replay skipped clear-tab-nodes because the tab is missing")
		return false
	}
	if len(tab.Items) == 0 {
		return false
	}
	tab.Items = []replayItem{}
	return true
}

func replayCreateNode(tabs *[]replayTab, receipt SyncMutationReceipt, result *SyncMutationDryRunResult) bool {
	var payload struct {
		TabID    string         `json:"tabId"`
		Name     string         `json:"name"`
		Color    string         `json:"color"`
		Position replayPosition `json:"position"`
		Shape    string         `json:"shape"`
		Width    *float64       `json:"width"`
		Height   *float64       `json:"height"`
	}
	if err := json.Unmarshal(receipt.Payload, &payload); err != nil {
		recordReplayWarning(result, receipt, replayWarningInvalidPayload, "dry-run replay could not decode create-node payload")
		return false
	}

	entityID, ok := replayEntityID(receipt, result)
	if !ok {
		return false
	}

	tab := findReplayTab(*tabs, normalizeReplayIdentifier(payload.TabID))
	if tab == nil {
		recordReplayWarning(result, receipt, replayWarningMissingTab, "dry-run replay skipped create-node because the tab is missing")
		return false
	}
	if findReplayItemAcrossTabs(*tabs, entityID) != nil {
		return false
	}

	position, ok := normalizeReplayPosition(payload.Position)
	if !ok {
		recordReplayWarning(result, receipt, replayWarningInvalidPosition, "dry-run replay skipped create-node because the position is invalid")
		return false
	}

	item := replayItem{
		ID:       entityID,
		Name:     payload.Name,
		Color:    payload.Color,
		Position: position,
	}
	if payload.Shape != "" {
		item.Shape = normalizeReplayShape(payload.Shape)
	}
	if payload.Width != nil {
		item.Width = float64Pointer(float64(normalizeReplayDimension(*payload.Width, replayItemWidthMin, replayItemWidthMax)))
	}
	if payload.Height != nil {
		item.Height = float64Pointer(float64(normalizeReplayDimension(*payload.Height, replayItemHeightMin, replayItemHeightMax)))
	}

	tab.Items = append(tab.Items, item)
	return true
}

func replayUpdateNode(tabs []replayTab, receipt SyncMutationReceipt, result *SyncMutationDryRunResult) bool {
	var payload struct {
		TabID   string                     `json:"tabId"`
		Changes map[string]json.RawMessage `json:"changes"`
	}
	if err := json.Unmarshal(receipt.Payload, &payload); err != nil {
		recordReplayWarning(result, receipt, replayWarningInvalidPayload, "dry-run replay could not decode update-node payload")
		return false
	}

	entityID, ok := replayEntityID(receipt, result)
	if !ok {
		return false
	}

	tab := findReplayTab(tabs, normalizeReplayIdentifier(payload.TabID))
	if tab == nil {
		recordReplayWarning(result, receipt, replayWarningMissingTab, "dry-run replay skipped update-node because the tab is missing")
		return false
	}

	item := findReplayItem(tab, entityID)
	if item == nil {
		recordReplayWarning(result, receipt, replayWarningSkippedMissingNode, "dry-run replay skipped update-node because the node is missing")
		return false
	}

	applied := false
	for field, rawValue := range payload.Changes {
		switch field {
		case "name", "text":
			var value string
			if err := json.Unmarshal(rawValue, &value); err != nil {
				recordReplayWarning(result, receipt, replayWarningInvalidUpdateValue, "dry-run replay skipped a node text update because the value is invalid")
				continue
			}
			item.Name = value
			applied = true
		case "color":
			var value string
			if err := json.Unmarshal(rawValue, &value); err != nil {
				recordReplayWarning(result, receipt, replayWarningInvalidUpdateValue, "dry-run replay skipped a node color update because the value is invalid")
				continue
			}
			item.Color = value
			applied = true
		case "shape":
			var value string
			if err := json.Unmarshal(rawValue, &value); err != nil {
				recordReplayWarning(result, receipt, replayWarningInvalidUpdateValue, "dry-run replay skipped a node shape update because the value is invalid")
				continue
			}
			item.Shape = normalizeReplayShape(value)
			applied = true
		case "width":
			value, ok := decodeReplayNumber(rawValue)
			if !ok {
				recordReplayWarning(result, receipt, replayWarningInvalidUpdateValue, "dry-run replay skipped a node width update because the value is invalid")
				continue
			}
			item.Width = float64Pointer(float64(normalizeReplayDimension(value, replayItemWidthMin, replayItemWidthMax)))
			applied = true
		case "height":
			value, ok := decodeReplayNumber(rawValue)
			if !ok {
				recordReplayWarning(result, receipt, replayWarningInvalidUpdateValue, "dry-run replay skipped a node height update because the value is invalid")
				continue
			}
			item.Height = float64Pointer(float64(normalizeReplayDimension(value, replayItemHeightMin, replayItemHeightMax)))
			applied = true
		default:
			recordReplayWarning(result, receipt, replayWarningUnknownUpdateField, "dry-run replay skipped an unknown update-node field")
		}
	}

	return applied
}

func replayMoveNode(tabs []replayTab, receipt SyncMutationReceipt, result *SyncMutationDryRunResult) bool {
	var payload struct {
		TabID    string         `json:"tabId"`
		Position replayPosition `json:"position"`
	}
	if err := json.Unmarshal(receipt.Payload, &payload); err != nil {
		recordReplayWarning(result, receipt, replayWarningInvalidPayload, "dry-run replay could not decode move-node payload")
		return false
	}

	entityID, ok := replayEntityID(receipt, result)
	if !ok {
		return false
	}

	tab := findReplayTab(tabs, normalizeReplayIdentifier(payload.TabID))
	if tab == nil {
		recordReplayWarning(result, receipt, replayWarningMissingTab, "dry-run replay skipped move-node because the tab is missing")
		return false
	}

	item := findReplayItem(tab, entityID)
	if item == nil {
		recordReplayWarning(result, receipt, replayWarningSkippedMissingNode, "dry-run replay skipped move-node because the node is missing")
		return false
	}

	position, ok := normalizeReplayPosition(payload.Position)
	if !ok {
		recordReplayWarning(result, receipt, replayWarningInvalidPosition, "dry-run replay skipped move-node because the position is invalid")
		return false
	}

	item.Position = position
	return true
}

func replayDeleteNode(tabs []replayTab, receipt SyncMutationReceipt, result *SyncMutationDryRunResult) bool {
	var payload struct {
		TabID string `json:"tabId"`
	}
	if err := json.Unmarshal(receipt.Payload, &payload); err != nil {
		recordReplayWarning(result, receipt, replayWarningInvalidPayload, "dry-run replay could not decode delete-node payload")
		return false
	}

	entityID, ok := replayEntityID(receipt, result)
	if !ok {
		return false
	}

	tab := findReplayTab(tabs, normalizeReplayIdentifier(payload.TabID))
	if tab == nil {
		recordReplayWarning(result, receipt, replayWarningMissingTab, "dry-run replay skipped delete-node because the tab is missing")
		return false
	}

	itemIndex := findReplayItemIndex(tab, entityID)
	if itemIndex < 0 {
		return false
	}

	tab.Items = append(tab.Items[:itemIndex], tab.Items[itemIndex+1:]...)

	filteredEdges := tab.Edges[:0]
	for _, edge := range tab.Edges {
		if edge.FromItemID == entityID || edge.ToItemID == entityID {
			continue
		}
		filteredEdges = append(filteredEdges, edge)
	}
	tab.Edges = filteredEdges
	return true
}

func replayCreateEdge(tabs []replayTab, receipt SyncMutationReceipt, result *SyncMutationDryRunResult) bool {
	var payload struct {
		TabID      string `json:"tabId"`
		FromItemID string `json:"fromItemId"`
		ToItemID   string `json:"toItemId"`
		Kind       string `json:"kind"`
	}
	if err := json.Unmarshal(receipt.Payload, &payload); err != nil {
		recordReplayWarning(result, receipt, replayWarningInvalidPayload, "dry-run replay could not decode create-edge payload")
		return false
	}

	entityID, ok := replayEntityID(receipt, result)
	if !ok {
		return false
	}

	tab := findReplayTab(tabs, normalizeReplayIdentifier(payload.TabID))
	if tab == nil {
		recordReplayWarning(result, receipt, replayWarningMissingTab, "dry-run replay skipped create-edge because the tab is missing")
		return false
	}
	if findReplayEdgeAcrossTabs(tabs, entityID) != nil {
		return false
	}

	fromItemID := normalizeReplayIdentifier(payload.FromItemID)
	toItemID := normalizeReplayIdentifier(payload.ToItemID)
	if fromItemID == "" || toItemID == "" {
		recordReplayWarning(result, receipt, replayWarningSkippedMissingEP, "dry-run replay skipped create-edge because an endpoint is missing")
		return false
	}
	if fromItemID == toItemID {
		recordReplayWarning(result, receipt, replayWarningSelfEdge, "dry-run replay skipped create-edge because self-edges are not allowed")
		return false
	}
	if findReplayItem(tab, fromItemID) == nil || findReplayItem(tab, toItemID) == nil {
		recordReplayWarning(result, receipt, replayWarningSkippedMissingEP, "dry-run replay skipped create-edge because an endpoint is missing")
		return false
	}
	if findReplayEdgeByEndpoints(tab, fromItemID, toItemID) != nil {
		return false
	}

	tab.Edges = append(tab.Edges, replayEdge{
		ID:         entityID,
		FromItemID: fromItemID,
		ToItemID:   toItemID,
		Kind:       normalizeReplayEdgeKind(payload.Kind),
	})
	return true
}

func replayDeleteEdge(tabs []replayTab, receipt SyncMutationReceipt, result *SyncMutationDryRunResult) bool {
	var payload struct {
		TabID string `json:"tabId"`
	}
	if err := json.Unmarshal(receipt.Payload, &payload); err != nil {
		recordReplayWarning(result, receipt, replayWarningInvalidPayload, "dry-run replay could not decode delete-edge payload")
		return false
	}

	entityID, ok := replayEntityID(receipt, result)
	if !ok {
		return false
	}

	tab := findReplayTab(tabs, normalizeReplayIdentifier(payload.TabID))
	if tab == nil {
		recordReplayWarning(result, receipt, replayWarningMissingTab, "dry-run replay skipped delete-edge because the tab is missing")
		return false
	}

	edgeIndex := findReplayEdgeIndex(tab, entityID)
	if edgeIndex < 0 {
		return false
	}

	tab.Edges = append(tab.Edges[:edgeIndex], tab.Edges[edgeIndex+1:]...)
	return true
}

func applyReplayMutationForAuthoritativePreview(tabs *[]replayTab, receipt SyncMutationReceipt) bool {
	previewResult := SyncMutationDryRunResult{
		Warnings: []SyncMutationDryRunWarning{},
	}

	switch receipt.OperationType {
	case "CreateNode":
		return replayCreateNode(tabs, receipt, &previewResult)
	case "UpdateNode":
		return replayUpdateNode(*tabs, receipt, &previewResult)
	case "MoveNode":
		return replayMoveNode(*tabs, receipt, &previewResult)
	case "DeleteNode":
		return replayDeleteNode(*tabs, receipt, &previewResult)
	case "CreateEdge":
		return replayCreateEdge(*tabs, receipt, &previewResult)
	case "DeleteEdge":
		return replayDeleteEdge(*tabs, receipt, &previewResult)
	case "CreateTab":
		return replayCreateTab(tabs, receipt, &previewResult)
	case "UpdateTab":
		return replayUpdateTab(*tabs, receipt, &previewResult)
	case "DeleteTab":
		return replayDeleteTab(tabs, receipt, &previewResult)
	case "ReorderTabs":
		return replayReorderTabs(tabs, receipt, &previewResult)
	case "ClearTabNodes":
		return replayClearTabNodes(*tabs, receipt, &previewResult)
	default:
		return false
	}
}

func recordReplayWarning(result *SyncMutationDryRunResult, receipt SyncMutationReceipt, code, message string) {
	result.Warnings = append(result.Warnings, SyncMutationDryRunWarning{
		MutationID: receipt.MutationID,
		Code:       code,
		Message:    message,
	})
}

func findReplayTab(tabs []replayTab, tabID string) *replayTab {
	for index := range tabs {
		if tabs[index].ID == tabID {
			return &tabs[index]
		}
	}
	return nil
}

func findReplayTabIndex(tabs []replayTab, tabID string) int {
	for index := range tabs {
		if tabs[index].ID == tabID {
			return index
		}
	}
	return -1
}

func replayTabPayloadIDMatchesEntity(entityID, payloadTabID string) bool {
	normalizedPayloadTabID := normalizeReplayIdentifier(payloadTabID)
	return normalizedPayloadTabID == "" || normalizedPayloadTabID == entityID
}

func insertReplayTab(tabs *[]replayTab, tab replayTab, orderIndex *int) {
	index := len(*tabs)
	if orderIndex != nil {
		index = *orderIndex
		if index < 0 {
			index = 0
		}
		if index > len(*tabs) {
			index = len(*tabs)
		}
	}

	*tabs = append(*tabs, replayTab{})
	copy((*tabs)[index+1:], (*tabs)[index:])
	(*tabs)[index] = tab
}

func normalizeReplayTabColorIndex(value *int) int {
	if value == nil || *value < 0 {
		return 0
	}
	return *value
}

func normalizeReplayTabGridSetting(value string) string {
	switch strings.TrimSpace(value) {
	case "kanban", "importance", "eisenhower", "priority", "smartgoals", "swot", "calendar", "nowlater", "week", "graphpaper":
		return strings.TrimSpace(value)
	default:
		return "none"
	}
}

func normalizeReplayTabOrder(tabs []replayTab, tabIDs []string) ([]string, bool) {
	if len(tabIDs) != len(tabs) {
		return nil, false
	}

	existing := make(map[string]struct{}, len(tabs))
	for _, tab := range tabs {
		tabID := normalizeReplayIdentifier(tab.ID)
		if tabID == "" {
			return nil, false
		}
		existing[tabID] = struct{}{}
	}

	seen := make(map[string]struct{}, len(tabIDs))
	order := make([]string, 0, len(tabIDs))
	for _, tabID := range tabIDs {
		normalizedID := normalizeReplayIdentifier(tabID)
		if normalizedID == "" {
			return nil, false
		}
		if _, ok := existing[normalizedID]; !ok {
			return nil, false
		}
		if _, ok := seen[normalizedID]; ok {
			return nil, false
		}
		seen[normalizedID] = struct{}{}
		order = append(order, normalizedID)
	}

	return order, true
}

func replayTabOrderAlreadyMatches(tabs []replayTab, order []string) bool {
	if len(tabs) != len(order) {
		return false
	}
	for index := range tabs {
		if tabs[index].ID != order[index] {
			return false
		}
	}
	return true
}

func findReplayItem(tab *replayTab, itemID string) *replayItem {
	for index := range tab.Items {
		if tab.Items[index].ID == itemID {
			return &tab.Items[index]
		}
	}
	return nil
}

func findReplayItemIndex(tab *replayTab, itemID string) int {
	for index := range tab.Items {
		if tab.Items[index].ID == itemID {
			return index
		}
	}
	return -1
}

func findReplayItemAcrossTabs(tabs []replayTab, itemID string) *replayItem {
	for index := range tabs {
		item := findReplayItem(&tabs[index], itemID)
		if item != nil {
			return item
		}
	}
	return nil
}

func findReplayEdgeIndex(tab *replayTab, edgeID string) int {
	for index := range tab.Edges {
		if tab.Edges[index].ID == edgeID {
			return index
		}
	}
	return -1
}

func findReplayEdgeAcrossTabs(tabs []replayTab, edgeID string) *replayEdge {
	for index := range tabs {
		for edgeIndex := range tabs[index].Edges {
			if tabs[index].Edges[edgeIndex].ID == edgeID {
				return &tabs[index].Edges[edgeIndex]
			}
		}
	}
	return nil
}

func findReplayEdgeByEndpoints(tab *replayTab, fromItemID, toItemID string) *replayEdge {
	for index := range tab.Edges {
		edge := &tab.Edges[index]
		if (edge.FromItemID == fromItemID && edge.ToItemID == toItemID) || (edge.FromItemID == toItemID && edge.ToItemID == fromItemID) {
			return edge
		}
	}
	return nil
}

func replayEntityID(receipt SyncMutationReceipt, result *SyncMutationDryRunResult) (string, bool) {
	entityID := normalizeReplayIdentifier(receipt.EntityID)
	if entityID == "" {
		recordReplayWarning(result, receipt, replayWarningMissingEntityID, "dry-run replay skipped a mutation because the entity id is missing")
		return "", false
	}
	return entityID, true
}

func normalizeReplayIdentifier(value string) string {
	return strings.TrimSpace(value)
}

func normalizeReplayPosition(position replayPosition) (replayPosition, bool) {
	top, ok := parseReplayCoordinate(position.Top)
	if !ok {
		return replayPosition{}, false
	}
	left, ok := parseReplayCoordinate(position.Left)
	if !ok {
		return replayPosition{}, false
	}
	return replayPosition{
		Top:  formatReplayCoordinate(top),
		Left: formatReplayCoordinate(left),
	}, true
}

func parseReplayCoordinate(value string) (int64, bool) {
	trimmed := strings.TrimSpace(value)
	if !strings.HasSuffix(trimmed, "px") {
		return 0, false
	}

	numericValue, err := strconv.ParseFloat(strings.TrimSuffix(trimmed, "px"), 64)
	if err != nil {
		return 0, false
	}

	rounded := int64(math.Round(numericValue))
	if rounded < replayCoordMin {
		rounded = replayCoordMin
	}
	if rounded > replayCoordMax {
		rounded = replayCoordMax
	}
	return rounded, true
}

func formatReplayCoordinate(value int64) string {
	return strconv.FormatInt(value, 10) + "px"
}

func normalizeReplayDimension(value float64, minValue, maxValue int) int {
	rounded := int(math.Round(value))
	if rounded < minValue {
		return minValue
	}
	if rounded > maxValue {
		return maxValue
	}
	return rounded
}

func buildReplayReceiptHighWatermark(receipts []SyncMutationReceipt) string {
	if len(receipts) == 0 {
		return ""
	}

	lastReceipt := receipts[len(receipts)-1]
	return lastReceipt.AcceptedAt.UTC().Format(time.RFC3339) + "|" + lastReceipt.CreatedAt.UTC().Format(time.RFC3339) + "|" + lastReceipt.MutationID
}

func summarizeReplayReceiptOrdering(receipts []SyncMutationReceipt) (int, string, string, string) {
	count := len(receipts)
	if count == 0 {
		return 0, "", "", ""
	}
	return count, receipts[0].MutationID, receipts[count-1].MutationID, buildReplayReceiptHighWatermark(receipts)
}

func normalizeSyncMutationReplayCompactOptions(options SyncMutationReplayCompactOptions) (SyncMutationReplayCompactOptions, error) {
	if options.ObservationRetention < 0 {
		return SyncMutationReplayCompactOptions{}, fmt.Errorf("compact sync mutation replay artifacts: observation retention must be non-negative")
	}
	if options.ReceiptRetention < 0 {
		return SyncMutationReplayCompactOptions{}, fmt.Errorf("compact sync mutation replay artifacts: receipt retention must be non-negative")
	}
	if options.RetainedObservationCount < 0 {
		return SyncMutationReplayCompactOptions{}, fmt.Errorf("compact sync mutation replay artifacts: retained observation count must be non-negative")
	}
	if options.RetainedReceiptCount < 0 {
		return SyncMutationReplayCompactOptions{}, fmt.Errorf("compact sync mutation replay artifacts: retained receipt count must be non-negative")
	}
	if options.Now.IsZero() {
		options.Now = time.Now().UTC()
	} else {
		options.Now = options.Now.UTC()
	}
	if options.ObservationRetention == 0 {
		options.ObservationRetention = DefaultSyncMutationReplayObservationRetention
	}
	if options.ReceiptRetention == 0 {
		options.ReceiptRetention = DefaultSyncMutationReplayReceiptRetention
	}
	if options.RetainedObservationCount == 0 {
		options.RetainedObservationCount = DefaultSyncMutationReplayRetainedObservationCount
	}
	if options.RetainedReceiptCount == 0 {
		options.RetainedReceiptCount = DefaultSyncMutationReplayRetainedReceiptCount
	}
	return options, nil
}

func latestSyncMutationReceiptIDs(receipts []syncMutationReceiptRowForCompact, retainedCount int) map[int64]struct{} {
	result := make(map[int64]struct{})
	if retainedCount <= 0 {
		return result
	}
	start := len(receipts) - retainedCount
	if start < 0 {
		start = 0
	}
	for _, receipt := range receipts[start:] {
		result[receipt.ID] = struct{}{}
	}
	return result
}

func syncMutationReceiptIDsProtectedByObservations(receipts []syncMutationReceiptRowForCompact, observations []SyncMutationReplayDryRunObservation) map[int64]struct{} {
	protected := make(map[int64]struct{})
	for _, observation := range observations {
		if observation.ReceiptCountConsidered <= 0 || observation.OrderedReceiptHighWatermark == "" {
			continue
		}
		watermarkIndex := -1
		for index, receipt := range receipts {
			if buildSyncMutationReceiptRowHighWatermark(receipt) == observation.OrderedReceiptHighWatermark {
				watermarkIndex = index
				break
			}
		}
		if watermarkIndex < 0 || watermarkIndex+1 < observation.ReceiptCountConsidered {
			continue
		}
		if observation.FirstOrderedMutationID != "" && receipts[0].MutationID != observation.FirstOrderedMutationID {
			continue
		}
		if observation.LastOrderedMutationID != "" && receipts[watermarkIndex].MutationID != observation.LastOrderedMutationID {
			continue
		}
		for index := 0; index < observation.ReceiptCountConsidered; index++ {
			protected[receipts[index].ID] = struct{}{}
		}
	}
	return protected
}

func buildSyncMutationReceiptRowHighWatermark(receipt syncMutationReceiptRowForCompact) string {
	return receipt.AcceptedAt.UTC().Format(time.RFC3339) + "|" + receipt.CreatedAt.UTC().Format(time.RFC3339) + "|" + receipt.MutationID
}

func deleteSyncMutationReplayObservationRowsByID(ctx context.Context, tx *sql.Tx, ownerKey, appID string, ids []int64) (int64, error) {
	return deleteSyncMutationReplayRowsByID(ctx, tx, `DELETE FROM sync_mutation_replay_dry_run_observations WHERE owner_key = ? AND app_id = ? AND id IN (`, ownerKey, appID, ids)
}

func deleteSyncMutationReceiptRowsByID(ctx context.Context, tx *sql.Tx, ownerKey, appID string, ids []int64) (int64, error) {
	return deleteSyncMutationReplayRowsByID(ctx, tx, `DELETE FROM sync_mutation_receipts WHERE owner_key = ? AND app_id = ? AND id IN (`, ownerKey, appID, ids)
}

func deleteSyncMutationReplayRowsByID(ctx context.Context, tx *sql.Tx, queryPrefix, ownerKey, appID string, ids []int64) (int64, error) {
	const chunkSize = 400
	if len(ids) == 0 {
		return 0, nil
	}

	var total int64
	for start := 0; start < len(ids); start += chunkSize {
		end := start + chunkSize
		if end > len(ids) {
			end = len(ids)
		}
		chunk := ids[start:end]
		placeholders := strings.TrimRight(strings.Repeat("?,", len(chunk)), ",")
		args := make([]any, 0, 2+len(chunk))
		args = append(args, ownerKey, appID)
		for _, id := range chunk {
			args = append(args, id)
		}
		execResult, err := tx.ExecContext(ctx, queryPrefix+placeholders+")", args...)
		if err != nil {
			return 0, err
		}
		rowsAffected, err := execResult.RowsAffected()
		if err != nil {
			return 0, err
		}
		total += rowsAffected
	}
	return total, nil
}

func hashReplayObservationBytes(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func replayAuthoritativeAllowedPolicyEvaluation(warnings []string) SyncMutationReplayAuthoritativePolicyEvaluation {
	return SyncMutationReplayAuthoritativePolicyEvaluation{
		Status:   replayAuthoritativePolicyStatusAllowed,
		Reasons:  []string{},
		Warnings: cloneReplayWarnings(warnings),
	}
}

func replayAuthoritativeSkipPolicyEvaluation(warnings []string, reasons ...string) SyncMutationReplayAuthoritativePolicyEvaluation {
	return SyncMutationReplayAuthoritativePolicyEvaluation{
		Status:   replayAuthoritativePolicyStatusSkip,
		Reasons:  append([]string{}, reasons...),
		Warnings: cloneReplayWarnings(warnings),
	}
}

func replayAuthoritativeConflictPolicyEvaluation(warnings []string, reasons ...string) SyncMutationReplayAuthoritativePolicyEvaluation {
	return SyncMutationReplayAuthoritativePolicyEvaluation{
		Status:   replayAuthoritativePolicyStatusConflict,
		Reasons:  append([]string{}, reasons...),
		Warnings: cloneReplayWarnings(warnings),
	}
}

func replayAuthoritativeFatalPolicyEvaluation(warnings []string, reasons ...string) SyncMutationReplayAuthoritativePolicyEvaluation {
	return SyncMutationReplayAuthoritativePolicyEvaluation{
		Status:   replayAuthoritativePolicyStatusFatal,
		Reasons:  append([]string{}, reasons...),
		Warnings: cloneReplayWarnings(warnings),
	}
}

func buildSyncMutationReplayApplicationReason(policyEvaluation SyncMutationReplayAuthoritativePolicyEvaluation) string {
	if len(policyEvaluation.Reasons) == 0 {
		if policyEvaluation.Status == replayAuthoritativePolicyStatusAllowed {
			return "policy_allowed"
		}
		return "policy_skip"
	}
	return strings.Join(policyEvaluation.Reasons, ",")
}

func cloneStringSlice(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	return append([]string{}, values...)
}

func cloneInt64Pointer(value *int64) *int64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneReplayWarnings(warnings []string) []string {
	return cloneStringSlice(warnings)
}

func cloneReplayAuthoritativeMutationResults(values []SyncMutationReplayAuthoritativeMutationResult) []SyncMutationReplayAuthoritativeMutationResult {
	if len(values) == 0 {
		return []SyncMutationReplayAuthoritativeMutationResult{}
	}
	return append([]SyncMutationReplayAuthoritativeMutationResult{}, values...)
}

func countReplayMutationApplicationStatuses(values []SyncMutationReplayAuthoritativeMutationResult) map[string]int64 {
	counts := make(map[string]int64)
	for _, value := range values {
		if strings.TrimSpace(value.ApplicationStatus) == "" {
			continue
		}
		counts[value.ApplicationStatus]++
	}
	return counts
}

func mergeReplayStringSets(groups ...[]string) []string {
	result := make([]string, 0)
	seen := make(map[string]struct{})
	for _, group := range groups {
		for _, value := range group {
			if strings.TrimSpace(value) == "" {
				continue
			}
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = struct{}{}
			result = append(result, value)
		}
	}
	return result
}

func appendReplayWarningIfMissing(warnings []string, warning string) []string {
	for _, existing := range warnings {
		if existing == warning {
			return warnings
		}
	}
	return append(warnings, warning)
}

func replayAuthoritativeDimensionsAllowed(width, height *float64) bool {
	if width != nil && !replayAuthoritativeDimensionWithinBounds(*width, replayItemWidthMin, replayItemWidthMax) {
		return false
	}
	if height != nil && !replayAuthoritativeDimensionWithinBounds(*height, replayItemHeightMin, replayItemHeightMax) {
		return false
	}
	return true
}

func replayAuthoritativeDimensionWithinBounds(value float64, minValue, maxValue int) bool {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return false
	}
	rounded := int(math.Round(value))
	return rounded >= minValue && rounded <= maxValue
}

func countReplayAuthoritativeTextUnits(value string) int {
	return len(utf16.Encode([]rune(value)))
}

func replayShapeUsesFixedRatio(value string) bool {
	normalized := normalizeReplayShape(value)
	_, ok := replayFixedRatioShapeSet[normalized]
	return ok
}

func appendReplayAuthoritativeArea(readiness *SyncMutationReplayAuthoritativeReadiness, area, status, detail string) {
	readiness.Areas = append(readiness.Areas, SyncMutationReplayAuthoritativeAreaReadiness{
		Area:   area,
		Status: status,
		Detail: detail,
	})
	if status == replayAuthoritativeStatusBlocked {
		addReplayAuthoritativeBlocker(readiness, area)
	}
}

func addReplayAuthoritativeBlockerIf(readiness *SyncMutationReplayAuthoritativeReadiness, condition bool, blocker string) {
	if condition {
		addReplayAuthoritativeBlocker(readiness, blocker)
	}
}

func addReplayAuthoritativeBlocker(readiness *SyncMutationReplayAuthoritativeReadiness, blocker string) {
	for _, existing := range readiness.Blockers {
		if existing == blocker {
			return
		}
	}
	readiness.Blockers = append(readiness.Blockers, blocker)
}

func analyzeReplaySnapshotForAuthoritativePolicy(tabs []replayTab) replaySnapshotPolicyAnalysis {
	analysis := replaySnapshotPolicyAnalysis{}
	seenItemIDs := make(map[string]struct{})
	seenEdgeIDs := make(map[string]struct{})

	for index := range tabs {
		tab := &tabs[index]
		for itemIndex := range tab.Items {
			item := &tab.Items[itemIndex]
			if _, exists := seenItemIDs[item.ID]; exists {
				analysis.DuplicateItemIDs = true
			} else {
				seenItemIDs[item.ID] = struct{}{}
			}

			if _, ok := normalizeReplayPosition(item.Position); !ok {
				analysis.MalformedLegacyPositions = true
			}
			normalizedShape := normalizeReplayIdentifier(item.Shape)
			if normalizedShape != "" && normalizeReplayShape(normalizedShape) != normalizedShape {
				analysis.UnknownLegacyShapes = true
			}
		}

		for edgeIndex := range tab.Edges {
			edge := &tab.Edges[edgeIndex]
			if _, exists := seenEdgeIDs[edge.ID]; exists {
				analysis.DuplicateEdgeIDs = true
			} else {
				seenEdgeIDs[edge.ID] = struct{}{}
			}

			if strings.TrimSpace(edge.ID) == "" ||
				strings.TrimSpace(edge.FromItemID) == "" ||
				strings.TrimSpace(edge.ToItemID) == "" ||
				edge.FromItemID == edge.ToItemID ||
				findReplayItem(tab, edge.FromItemID) == nil ||
				findReplayItem(tab, edge.ToItemID) == nil {
				analysis.MalformedLegacyEdges = true
			}
		}
	}

	return analysis
}

func matchingReplayApplicationMutationIDs(receipts []SyncMutationReceipt, applications []SyncMutationReplayApplication) []string {
	if len(receipts) == 0 || len(applications) == 0 {
		return nil
	}

	applicationMutationIDs := make(map[string]struct{}, len(applications))
	for _, application := range applications {
		applicationMutationIDs[application.MutationID] = struct{}{}
	}

	matchingMutationIDs := make([]string, 0)
	for _, receipt := range receipts {
		if _, ok := applicationMutationIDs[receipt.MutationID]; ok {
			matchingMutationIDs = append(matchingMutationIDs, receipt.MutationID)
		}
	}
	if len(matchingMutationIDs) == 0 {
		return nil
	}
	return matchingMutationIDs
}

func matchingReplayApplications(receipts []SyncMutationReceipt, applications []SyncMutationReplayApplication) []SyncMutationReplayApplication {
	if len(receipts) == 0 || len(applications) == 0 {
		return nil
	}

	receiptMutationIDs := make(map[string]struct{}, len(receipts))
	for _, receipt := range receipts {
		receiptMutationIDs[receipt.MutationID] = struct{}{}
	}

	matchingApplications := make([]SyncMutationReplayApplication, 0)
	for _, application := range applications {
		if _, ok := receiptMutationIDs[application.MutationID]; ok {
			matchingApplications = append(matchingApplications, application)
		}
	}
	if len(matchingApplications) == 0 {
		return nil
	}
	return matchingApplications
}

func replayApplicationCanonicalMismatchReasons(applications []SyncMutationReplayApplication, currentDoc Document) []string {
	if len(applications) == 0 {
		return nil
	}

	reasons := make([]string, 0, 2)
	currentHash := hashReplayObservationBytes(currentDoc.Body)
	for _, application := range applications {
		if application.CanonicalDocumentVersionAfter != nil && *application.CanonicalDocumentVersionAfter != currentDoc.Version {
			reasons = appendReplayWarningIfMissing(reasons, "application_rows_reference_mismatched_canonical_version_after")
		}
		if application.CanonicalDocumentHashAfter != nil && *application.CanonicalDocumentHashAfter != currentHash {
			reasons = appendReplayWarningIfMissing(reasons, "application_rows_reference_mismatched_canonical_hash_after")
		}
	}
	if len(reasons) == 0 {
		return nil
	}
	return reasons
}

func (analysis replaySnapshotPolicyAnalysis) warnings() []string {
	warnings := make([]string, 0, 3)
	if analysis.MalformedLegacyPositions {
		warnings = append(warnings, "preserve_malformed_legacy_positions")
	}
	if analysis.UnknownLegacyShapes {
		warnings = append(warnings, "preserve_unknown_legacy_shapes")
	}
	if analysis.MalformedLegacyEdges {
		warnings = append(warnings, "preserve_malformed_legacy_edges")
	}
	return warnings
}

func normalizeReplayShape(value string) string {
	normalized := normalizeReplayIdentifier(value)
	if normalized == "" {
		return ""
	}
	if _, ok := replayItemShapeSet[normalized]; ok {
		return normalized
	}
	return "default"
}

func normalizeReplayEdgeKind(value string) string {
	if strings.TrimSpace(value) == "arrow" {
		return "arrow"
	}
	return "line"
}

func decodeReplayNumber(value json.RawMessage) (float64, bool) {
	var number float64
	if err := json.Unmarshal(value, &number); err != nil {
		return 0, false
	}
	return number, true
}

func float64Pointer(value float64) *float64 {
	return &value
}

func (s *Store) init(ctx context.Context) error {
	started := time.Now()

	if err := s.Health(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}

	ctx, cancel := withDBTimeout(ctx)
	defer cancel()
	_, err := s.db.ExecContext(
		ctx,
		`CREATE TABLE IF NOT EXISTS documents (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			owner_key TEXT NOT NULL,
			app_id TEXT NOT NULL,
			body_json TEXT NOT NULL,
			version INTEGER NOT NULL,
			updated_at TEXT NOT NULL,
			UNIQUE(owner_key, app_id)
		)`,
	)
	if err != nil {
		return s.wrapDBError("db_init_create_schema", started, fmt.Errorf("create schema: %w", err))
	}

	_, err = s.db.ExecContext(
		ctx,
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL COLLATE NOCASE UNIQUE,
			password_hash TEXT NOT NULL,
			owner_key TEXT NOT NULL UNIQUE,
			is_admin INTEGER NOT NULL,
			account_status TEXT NOT NULL DEFAULT 'active',
			checkout_expires_at TEXT,
			created_at TEXT NOT NULL
		)`,
	)
	if err != nil {
		return s.wrapDBError("db_init_create_users", started, fmt.Errorf("create users table: %w", err))
	}

	if err := s.ensureColumn(ctx, "users", "account_status", "TEXT NOT NULL DEFAULT 'active'"); err != nil {
		return s.wrapDBError("db_init_users_account_status", started, err)
	}
	if err := s.ensureColumn(ctx, "users", "checkout_expires_at", "TEXT"); err != nil {
		return s.wrapDBError("db_init_users_checkout_expires_at", started, err)
	}

	_, err = s.db.ExecContext(
		ctx,
		`CREATE TABLE IF NOT EXISTS sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			token_hash TEXT NOT NULL UNIQUE,
			expires_at TEXT NOT NULL,
			created_at TEXT NOT NULL,
			last_seen_at TEXT NOT NULL,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
	)
	if err != nil {
		return s.wrapDBError("db_init_create_sessions", started, fmt.Errorf("create sessions table: %w", err))
	}

	_, err = s.db.ExecContext(
		ctx,
		`CREATE TABLE IF NOT EXISTS account_entitlements (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			entitlement_key TEXT NOT NULL,
			status TEXT NOT NULL,
			source TEXT NOT NULL,
			valid_until TEXT,
			updated_at TEXT NOT NULL,
			UNIQUE(user_id, entitlement_key),
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
	)
	if err != nil {
		return s.wrapDBError("db_init_create_account_entitlements", started, fmt.Errorf("create account_entitlements table: %w", err))
	}

	_, err = s.db.ExecContext(
		ctx,
		`CREATE TABLE IF NOT EXISTS billing_customers (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			provider TEXT NOT NULL,
			provider_customer_id TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			UNIQUE(user_id, provider),
			UNIQUE(provider, provider_customer_id),
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
	)
	if err != nil {
		return s.wrapDBError("db_init_create_billing_customers", started, fmt.Errorf("create billing_customers table: %w", err))
	}

	_, err = s.db.ExecContext(
		ctx,
		`CREATE TABLE IF NOT EXISTS billing_subscriptions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			provider TEXT NOT NULL,
			provider_subscription_id TEXT NOT NULL,
			status TEXT NOT NULL,
			valid_until TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			UNIQUE(provider, provider_subscription_id),
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
	)
	if err != nil {
		return s.wrapDBError("db_init_create_billing_subscriptions", started, fmt.Errorf("create billing_subscriptions table: %w", err))
	}

	_, err = s.db.ExecContext(
		ctx,
		`CREATE TABLE IF NOT EXISTS sync_mutation_receipts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			owner_key TEXT NOT NULL,
			app_id TEXT NOT NULL,
			mutation_id TEXT NOT NULL,
			client_id TEXT NOT NULL,
			device_id TEXT NOT NULL,
			protocol TEXT NOT NULL,
			entity_type TEXT NOT NULL,
			entity_id TEXT NOT NULL,
			operation_type TEXT NOT NULL,
			payload_json TEXT NOT NULL,
			base_revision INTEGER,
			status TEXT NOT NULL,
			created_at TEXT NOT NULL,
			accepted_at TEXT NOT NULL,
			UNIQUE(owner_key, app_id, mutation_id)
		)`,
	)
	if err != nil {
		return s.wrapDBError("db_init_create_sync_mutation_receipts", started, fmt.Errorf("create sync_mutation_receipts table: %w", err))
	}

	_, err = s.db.ExecContext(
		ctx,
		`CREATE TABLE IF NOT EXISTS sync_mutation_replay_dry_run_observations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			owner_key TEXT NOT NULL,
			app_id TEXT NOT NULL,
			canonical_document_version_observed INTEGER NOT NULL,
			canonical_document_hash_observed TEXT NOT NULL,
			receipt_count_considered INTEGER NOT NULL,
			first_ordered_mutation_id TEXT NOT NULL,
			last_ordered_mutation_id TEXT NOT NULL,
			ordered_receipt_high_watermark TEXT NOT NULL,
			applied_count INTEGER NOT NULL,
			skipped_count INTEGER NOT NULL,
			warning_count INTEGER NOT NULL,
			preview_hash TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
	)
	if err != nil {
		return s.wrapDBError("db_init_create_sync_mutation_replay_dry_run_observations", started, fmt.Errorf("create sync_mutation_replay_dry_run_observations table: %w", err))
	}

	_, err = s.db.ExecContext(
		ctx,
		`CREATE TABLE IF NOT EXISTS sync_mutation_replay_applications (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			owner_key TEXT NOT NULL,
			app_id TEXT NOT NULL,
			mutation_id TEXT NOT NULL,
			application_status TEXT NOT NULL,
			application_reason TEXT NOT NULL,
			canonical_document_version_before INTEGER NOT NULL,
			canonical_document_hash_before TEXT NOT NULL,
			canonical_document_version_after INTEGER,
			canonical_document_hash_after TEXT,
			replay_observation_id INTEGER,
			created_at TEXT NOT NULL,
			UNIQUE(owner_key, app_id, mutation_id)
		)`,
	)
	if err != nil {
		return s.wrapDBError("db_init_create_sync_mutation_replay_applications", started, fmt.Errorf("create sync_mutation_replay_applications table: %w", err))
	}

	_, err = s.db.ExecContext(
		ctx,
		`CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at)`,
	)
	if err != nil {
		return s.wrapDBError("db_init_create_sessions_expires_idx", started, fmt.Errorf("create sessions expires_at index: %w", err))
	}

	_, err = s.db.ExecContext(
		ctx,
		`CREATE INDEX IF NOT EXISTS idx_account_entitlements_user_id ON account_entitlements(user_id)`,
	)
	if err != nil {
		return s.wrapDBError("db_init_create_account_entitlements_user_idx", started, fmt.Errorf("create account_entitlements user_id index: %w", err))
	}

	_, err = s.db.ExecContext(
		ctx,
		`CREATE INDEX IF NOT EXISTS idx_billing_customers_user_id ON billing_customers(user_id)`,
	)
	if err != nil {
		return s.wrapDBError("db_init_create_billing_customers_user_idx", started, fmt.Errorf("create billing_customers user_id index: %w", err))
	}

	_, err = s.db.ExecContext(
		ctx,
		`CREATE INDEX IF NOT EXISTS idx_billing_subscriptions_user_id ON billing_subscriptions(user_id)`,
	)
	if err != nil {
		return s.wrapDBError("db_init_create_billing_subscriptions_user_idx", started, fmt.Errorf("create billing_subscriptions user_id index: %w", err))
	}

	_, err = s.db.ExecContext(
		ctx,
		`CREATE INDEX IF NOT EXISTS idx_sync_mutation_receipts_owner_app_created_at ON sync_mutation_receipts(owner_key, app_id, created_at)`,
	)
	if err != nil {
		return s.wrapDBError("db_init_create_sync_mutation_receipts_owner_app_created_idx", started, fmt.Errorf("create sync_mutation_receipts owner/app/created_at index: %w", err))
	}

	_, err = s.db.ExecContext(
		ctx,
		`CREATE INDEX IF NOT EXISTS idx_sync_mutation_replay_dry_run_observations_owner_app_created_at ON sync_mutation_replay_dry_run_observations(owner_key, app_id, created_at)`,
	)
	if err != nil {
		return s.wrapDBError("db_init_create_sync_mutation_replay_dry_run_observations_owner_app_created_idx", started, fmt.Errorf("create sync_mutation_replay_dry_run_observations owner/app/created_at index: %w", err))
	}

	_, err = s.db.ExecContext(
		ctx,
		`CREATE INDEX IF NOT EXISTS idx_sync_mutation_replay_applications_owner_app_created_at ON sync_mutation_replay_applications(owner_key, app_id, created_at)`,
	)
	if err != nil {
		return s.wrapDBError("db_init_create_sync_mutation_replay_applications_owner_app_created_idx", started, fmt.Errorf("create sync_mutation_replay_applications owner/app/created_at index: %w", err))
	}

	s.logDBOperation("db_init_create_schema", started, nil)
	return nil
}

func (s *Store) ensureColumn(ctx context.Context, tableName, columnName, definition string) error {
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`PRAGMA table_info(%s)`, tableName))
	if err != nil {
		return fmt.Errorf("query table info for %s: %w", tableName, err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name string
		var columnType string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			return fmt.Errorf("scan table info for %s: %w", tableName, err)
		}
		if name == columnName {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate table info for %s: %w", tableName, err)
	}

	if _, err := s.db.ExecContext(ctx, fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s %s`, tableName, columnName, definition)); err != nil {
		return fmt.Errorf("add column %s.%s: %w", tableName, columnName, err)
	}
	return nil
}

func (s *Store) getSyncMutationReplayDryRunObservationByID(ctx context.Context, observationID int64) (SyncMutationReplayDryRunObservation, bool, error) {
	started := time.Now()
	ctx, cancel := withDBTimeout(ctx)
	defer cancel()

	observation, err := scanSyncMutationReplayDryRunObservation(s.db.QueryRowContext(
		ctx,
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
			created_at
		FROM sync_mutation_replay_dry_run_observations
		WHERE id = ?`,
		observationID,
	))
	if err != nil {
		if errors.Is(err, ErrSyncMutationReplayDryRunObservationNotFound) {
			s.logDBOperation("read_sync_mutation_replay_dry_run_observation_by_id", started, nil)
			return SyncMutationReplayDryRunObservation{}, false, nil
		}
		return SyncMutationReplayDryRunObservation{}, false, s.wrapDBError("read_sync_mutation_replay_dry_run_observation_by_id", started, fmt.Errorf("load sync mutation replay dry-run observation by id: %w", err))
	}

	s.logDBOperation("read_sync_mutation_replay_dry_run_observation_by_id", started, nil)
	return observation, true, nil
}

func scanDocument(row interface {
	Scan(dest ...any) error
}) (Document, error) {
	var doc Document
	var body string
	var updatedAt string
	if err := row.Scan(&doc.ID, &doc.OwnerKey, &doc.AppID, &body, &doc.Version, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Document{}, ErrDocumentNotFound
		}
		return Document{}, fmt.Errorf("scan document: %w", err)
	}

	parsedUpdatedAt, err := time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		return Document{}, fmt.Errorf("parse updated_at: %w", err)
	}

	doc.Body = json.RawMessage(body)
	doc.UpdatedAt = parsedUpdatedAt
	return doc, nil
}

func scanSyncMutationReceipt(row interface {
	Scan(dest ...any) error
}) (SyncMutationReceipt, error) {
	var receipt SyncMutationReceipt
	var payloadJSON string
	var baseRevision sql.NullInt64
	var createdAt string
	var acceptedAt string
	if err := row.Scan(
		&receipt.ID,
		&receipt.OwnerKey,
		&receipt.AppID,
		&receipt.MutationID,
		&receipt.ClientID,
		&receipt.DeviceID,
		&receipt.Protocol,
		&receipt.EntityType,
		&receipt.EntityID,
		&receipt.OperationType,
		&payloadJSON,
		&baseRevision,
		&receipt.Status,
		&createdAt,
		&acceptedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SyncMutationReceipt{}, fmt.Errorf("sync mutation receipt not found")
		}
		return SyncMutationReceipt{}, fmt.Errorf("scan sync mutation receipt: %w", err)
	}

	receipt.Payload = json.RawMessage(payloadJSON)
	if baseRevision.Valid {
		value := baseRevision.Int64
		receipt.BaseRevision = &value
	}
	receipt.CreatedAt = mustParseTimestamp(createdAt)
	receipt.AcceptedAt = mustParseTimestamp(acceptedAt)
	return receipt, nil
}

func scanSyncMutationReplayDryRunObservation(row interface {
	Scan(dest ...any) error
}) (SyncMutationReplayDryRunObservation, error) {
	var observation SyncMutationReplayDryRunObservation
	var createdAt string
	if err := row.Scan(
		&observation.ID,
		&observation.OwnerKey,
		&observation.AppID,
		&observation.CanonicalDocumentVersionObserved,
		&observation.CanonicalDocumentHashObserved,
		&observation.ReceiptCountConsidered,
		&observation.FirstOrderedMutationID,
		&observation.LastOrderedMutationID,
		&observation.OrderedReceiptHighWatermark,
		&observation.AppliedCount,
		&observation.SkippedCount,
		&observation.WarningCount,
		&observation.PreviewHash,
		&createdAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SyncMutationReplayDryRunObservation{}, ErrSyncMutationReplayDryRunObservationNotFound
		}
		return SyncMutationReplayDryRunObservation{}, fmt.Errorf("scan sync mutation replay dry-run observation: %w", err)
	}

	observation.CreatedAt = mustParseTimestamp(createdAt)
	return observation, nil
}

func scanSyncMutationReplayApplication(row interface {
	Scan(dest ...any) error
}) (SyncMutationReplayApplication, error) {
	var application SyncMutationReplayApplication
	var versionAfter sql.NullInt64
	var hashAfter sql.NullString
	var replayObservationID sql.NullInt64
	var createdAt string
	if err := row.Scan(
		&application.ID,
		&application.OwnerKey,
		&application.AppID,
		&application.MutationID,
		&application.ApplicationStatus,
		&application.ApplicationReason,
		&application.CanonicalDocumentVersionBefore,
		&application.CanonicalDocumentHashBefore,
		&versionAfter,
		&hashAfter,
		&replayObservationID,
		&createdAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SyncMutationReplayApplication{}, fmt.Errorf("sync mutation replay application not found")
		}
		return SyncMutationReplayApplication{}, fmt.Errorf("scan sync mutation replay application: %w", err)
	}

	if versionAfter.Valid {
		value := versionAfter.Int64
		application.CanonicalDocumentVersionAfter = &value
	}
	if hashAfter.Valid {
		value := hashAfter.String
		application.CanonicalDocumentHashAfter = &value
	}
	if replayObservationID.Valid {
		value := replayObservationID.Int64
		application.ReplayObservationID = &value
	}
	application.CreatedAt = mustParseTimestamp(createdAt)
	return application, nil
}

func cloneJSON(value json.RawMessage) json.RawMessage {
	return append(json.RawMessage(nil), value...)
}

func CurrentVersionFromConflict(err error) (*int64, bool) {
	var conflict *VersionConflictError
	if !errors.As(err, &conflict) {
		return nil, false
	}

	return conflict.CurrentVersion, true
}

func (s *Store) getCurrentVersion(ctx context.Context, ownerKey, appID string) (*int64, error) {
	started := time.Now()
	ctx, cancel := withDBTimeout(ctx)
	defer cancel()

	var version int64
	err := s.db.QueryRowContext(
		ctx,
		`SELECT version FROM documents WHERE owner_key = ? AND app_id = ?`,
		ownerKey,
		appID,
	).Scan(&version)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrDocumentNotFound
		}
		return nil, s.wrapDBError("read_current_version", started, fmt.Errorf("scan current version: %w", err))
	}

	s.logDBOperation("read_current_version", started, nil)
	return &version, nil
}

type pragmaState struct {
	journalMode string
	walEnabled  bool
}

func (s *Store) applyPragmas(ctx context.Context) (pragmaState, error) {
	started := time.Now()

	var journalMode string
	if err := s.db.QueryRowContext(ctx, `PRAGMA journal_mode=WAL;`).Scan(&journalMode); err != nil {
		return pragmaState{}, s.wrapDBError("db_pragma_journal_mode", started, fmt.Errorf("set journal mode: %w", err))
	}
	if _, err := s.db.ExecContext(ctx, `PRAGMA busy_timeout=5000;`); err != nil {
		return pragmaState{}, s.wrapDBError("db_pragma_busy_timeout", started, fmt.Errorf("set busy timeout: %w", err))
	}
	if _, err := s.db.ExecContext(ctx, `PRAGMA synchronous=NORMAL;`); err != nil {
		return pragmaState{}, s.wrapDBError("db_pragma_synchronous", started, fmt.Errorf("set synchronous mode: %w", err))
	}
	if _, err := s.db.ExecContext(ctx, `PRAGMA foreign_keys=ON;`); err != nil {
		return pragmaState{}, s.wrapDBError("db_pragma_foreign_keys", started, fmt.Errorf("enable foreign keys: %w", err))
	}

	s.logDBOperation("db_pragma_configure", started, nil)
	return pragmaState{
		journalMode: journalMode,
		walEnabled:  strings.EqualFold(journalMode, "wal"),
	}, nil
}

func withDBTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(ctx, dbOperationTimeout)
}

func (s *Store) wrapDBError(op string, started time.Time, err error) error {
	s.logDBOperation(op, started, err)
	return fmt.Errorf("%s: %w", op, err)
}

func (s *Store) logDBOperation(op string, started time.Time, err error) {
	elapsed := time.Since(started)
	locked := isLockedError(err)
	deadlineExceeded := errors.Is(err, context.DeadlineExceeded)

	shouldLog := err != nil || elapsed >= slowDBOperationThreshold || strings.HasPrefix(op, "write_") || strings.HasPrefix(op, "db_")
	if !shouldLog {
		return
	}

	if err != nil {
		log.Printf(
			"db_op op=%s path=%q elapsed_ms=%d locked=%t deadline_exceeded=%t err=%v",
			op,
			s.dbPath,
			elapsed.Milliseconds(),
			locked,
			deadlineExceeded,
			err,
		)
		return
	}

	log.Printf(
		"db_op op=%s path=%q elapsed_ms=%d locked=%t deadline_exceeded=%t",
		op,
		s.dbPath,
		elapsed.Milliseconds(),
		false,
		false,
	)
}

func isLockedError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "database is locked")
}

func parseNullableTimestamp(value sql.NullString) (*time.Time, error) {
	if !value.Valid {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, value.String)
	if err != nil {
		return nil, fmt.Errorf("parse timestamp %q: %w", value.String, err)
	}
	parsed = parsed.UTC()
	return &parsed, nil
}

func mustParseTimestamp(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic(err)
	}

	return parsed
}
