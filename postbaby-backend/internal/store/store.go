package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var (
	ErrDocumentNotFound = errors.New("document not found")
	ErrVersionConflict  = errors.New("version conflict")
)

const (
	dbOperationTimeout       = 8 * time.Second
	dbStartupTimeout         = 10 * time.Second
	slowDBOperationThreshold = 750 * time.Millisecond
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
			created_at TEXT NOT NULL
		)`,
	)
	if err != nil {
		return s.wrapDBError("db_init_create_users", started, fmt.Errorf("create users table: %w", err))
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
		`CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at)`,
	)
	if err != nil {
		return s.wrapDBError("db_init_create_sessions_expires_idx", started, fmt.Errorf("create sessions expires_at index: %w", err))
	}

	s.logDBOperation("db_init_create_schema", started, nil)
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
