package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var (
	ErrDocumentNotFound = errors.New("document not found")
	ErrVersionConflict  = errors.New("version conflict")
)

const (
	dbOperationTimeout                = 8 * time.Second
	dbStartupTimeout                  = 10 * time.Second
	slowDBOperationThreshold          = 750 * time.Millisecond
	SyncMutationReceiptStatusAccepted = "accepted"
	replayTabsSnapshotKey             = "tabs"
	replayCoordMin                    = -100000
	replayCoordMax                    = 100000
	replayItemWidthMin                = 120
	replayItemWidthMax                = 600
	replayItemHeightMin               = 80
	replayItemHeightMax               = 600
)

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

func (s *Store) ReplaySyncMutationReceiptsDryRun(ctx context.Context, ownerKey, appID string) (SyncMutationDryRunResult, error) {
	doc, err := s.GetDocument(ctx, ownerKey, appID)
	if err != nil {
		return SyncMutationDryRunResult{}, err
	}

	receipts, err := s.listAcceptedSyncMutationReceipts(ctx, ownerKey, appID)
	if err != nil {
		return SyncMutationDryRunResult{}, err
	}

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
		default:
			recordReplayWarning(&result, receipt, "unknown_operation", "dry-run replay skipped an unknown operation type")
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
		recordReplayWarning(result, receipt, "invalid_payload", "dry-run replay could not decode create-node payload")
		return false
	}

	tab := findReplayTab(*tabs, payload.TabID)
	if tab == nil {
		recordReplayWarning(result, receipt, "missing_tab", "dry-run replay skipped create-node because the tab is missing")
		return false
	}
	if findReplayItemAcrossTabs(*tabs, receipt.EntityID) != nil {
		return false
	}

	position, ok := normalizeReplayPosition(payload.Position)
	if !ok {
		recordReplayWarning(result, receipt, "invalid_position", "dry-run replay skipped create-node because the position is invalid")
		return false
	}

	item := replayItem{
		ID:       receipt.EntityID,
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
		recordReplayWarning(result, receipt, "invalid_payload", "dry-run replay could not decode update-node payload")
		return false
	}

	tab := findReplayTab(tabs, payload.TabID)
	if tab == nil {
		recordReplayWarning(result, receipt, "missing_tab", "dry-run replay skipped update-node because the tab is missing")
		return false
	}

	item := findReplayItem(tab, receipt.EntityID)
	if item == nil {
		recordReplayWarning(result, receipt, "skipped_missing_node", "dry-run replay skipped update-node because the node is missing")
		return false
	}

	applied := false
	for field, rawValue := range payload.Changes {
		switch field {
		case "name", "text":
			var value string
			if err := json.Unmarshal(rawValue, &value); err != nil {
				recordReplayWarning(result, receipt, "invalid_update_value", "dry-run replay skipped a node text update because the value is invalid")
				continue
			}
			item.Name = value
			applied = true
		case "color":
			var value string
			if err := json.Unmarshal(rawValue, &value); err != nil {
				recordReplayWarning(result, receipt, "invalid_update_value", "dry-run replay skipped a node color update because the value is invalid")
				continue
			}
			item.Color = value
			applied = true
		case "shape":
			var value string
			if err := json.Unmarshal(rawValue, &value); err != nil {
				recordReplayWarning(result, receipt, "invalid_update_value", "dry-run replay skipped a node shape update because the value is invalid")
				continue
			}
			item.Shape = normalizeReplayShape(value)
			applied = true
		case "width":
			value, ok := decodeReplayNumber(rawValue)
			if !ok {
				recordReplayWarning(result, receipt, "invalid_update_value", "dry-run replay skipped a node width update because the value is invalid")
				continue
			}
			item.Width = float64Pointer(float64(normalizeReplayDimension(value, replayItemWidthMin, replayItemWidthMax)))
			applied = true
		case "height":
			value, ok := decodeReplayNumber(rawValue)
			if !ok {
				recordReplayWarning(result, receipt, "invalid_update_value", "dry-run replay skipped a node height update because the value is invalid")
				continue
			}
			item.Height = float64Pointer(float64(normalizeReplayDimension(value, replayItemHeightMin, replayItemHeightMax)))
			applied = true
		default:
			recordReplayWarning(result, receipt, "unknown_update_field", "dry-run replay skipped an unknown update-node field")
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
		recordReplayWarning(result, receipt, "invalid_payload", "dry-run replay could not decode move-node payload")
		return false
	}

	tab := findReplayTab(tabs, payload.TabID)
	if tab == nil {
		recordReplayWarning(result, receipt, "missing_tab", "dry-run replay skipped move-node because the tab is missing")
		return false
	}

	item := findReplayItem(tab, receipt.EntityID)
	if item == nil {
		recordReplayWarning(result, receipt, "skipped_missing_node", "dry-run replay skipped move-node because the node is missing")
		return false
	}

	position, ok := normalizeReplayPosition(payload.Position)
	if !ok {
		recordReplayWarning(result, receipt, "invalid_position", "dry-run replay skipped move-node because the position is invalid")
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
		recordReplayWarning(result, receipt, "invalid_payload", "dry-run replay could not decode delete-node payload")
		return false
	}

	tab := findReplayTab(tabs, payload.TabID)
	if tab == nil {
		recordReplayWarning(result, receipt, "missing_tab", "dry-run replay skipped delete-node because the tab is missing")
		return false
	}

	itemIndex := findReplayItemIndex(tab, receipt.EntityID)
	if itemIndex < 0 {
		return false
	}

	tab.Items = append(tab.Items[:itemIndex], tab.Items[itemIndex+1:]...)

	filteredEdges := tab.Edges[:0]
	for _, edge := range tab.Edges {
		if edge.FromItemID == receipt.EntityID || edge.ToItemID == receipt.EntityID {
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
		recordReplayWarning(result, receipt, "invalid_payload", "dry-run replay could not decode create-edge payload")
		return false
	}

	tab := findReplayTab(tabs, payload.TabID)
	if tab == nil {
		recordReplayWarning(result, receipt, "missing_tab", "dry-run replay skipped create-edge because the tab is missing")
		return false
	}
	if findReplayEdgeAcrossTabs(tabs, receipt.EntityID) != nil {
		return false
	}
	if findReplayItem(tab, payload.FromItemID) == nil || findReplayItem(tab, payload.ToItemID) == nil {
		recordReplayWarning(result, receipt, "skipped_missing_endpoint", "dry-run replay skipped create-edge because an endpoint is missing")
		return false
	}

	tab.Edges = append(tab.Edges, replayEdge{
		ID:         receipt.EntityID,
		FromItemID: payload.FromItemID,
		ToItemID:   payload.ToItemID,
		Kind:       normalizeReplayEdgeKind(payload.Kind),
	})
	return true
}

func replayDeleteEdge(tabs []replayTab, receipt SyncMutationReceipt, result *SyncMutationDryRunResult) bool {
	var payload struct {
		TabID string `json:"tabId"`
	}
	if err := json.Unmarshal(receipt.Payload, &payload); err != nil {
		recordReplayWarning(result, receipt, "invalid_payload", "dry-run replay could not decode delete-edge payload")
		return false
	}

	tab := findReplayTab(tabs, payload.TabID)
	if tab == nil {
		recordReplayWarning(result, receipt, "missing_tab", "dry-run replay skipped delete-edge because the tab is missing")
		return false
	}

	edgeIndex := findReplayEdgeIndex(tab, receipt.EntityID)
	if edgeIndex < 0 {
		return false
	}

	tab.Edges = append(tab.Edges[:edgeIndex], tab.Edges[edgeIndex+1:]...)
	return true
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

func normalizeReplayShape(value string) string {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return ""
	}
	if slices.Contains([]string{"default", "circle", "square", "triangle", "diamond", "upsideDownTriangle", "hexagon", "oval"}, normalized) {
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

func mustParseTimestamp(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic(err)
	}

	return parsed
}
