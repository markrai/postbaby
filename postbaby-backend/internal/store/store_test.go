package store

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOpenCreatesSchema(t *testing.T) {
	t.Parallel()

	docStore := openTestStore(t)

	for _, tableName := range []string{"documents", "users", "sessions", "account_entitlements"} {
		var found string
		err := docStore.db.QueryRowContext(
			context.Background(),
			`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`,
			tableName,
		).Scan(&found)
		if err != nil {
			t.Fatalf("query schema for %s: %v", tableName, err)
		}

		if found != tableName {
			t.Fatalf("unexpected table name: %q", found)
		}
	}
}

func TestSchemaEnforcesUniqueOwnerAndAppID(t *testing.T) {
	t.Parallel()

	docStore := openTestStore(t)

	_, err := docStore.db.ExecContext(
		context.Background(),
		`INSERT INTO documents (owner_key, app_id, body_json, version, updated_at)
		VALUES (?, ?, ?, ?, ?)`,
		"owner",
		"app",
		`{"first":true}`,
		1,
		"2026-05-06T12:00:00Z",
	)
	if err != nil {
		t.Fatalf("insert first row: %v", err)
	}

	_, err = docStore.db.ExecContext(
		context.Background(),
		`INSERT INTO documents (owner_key, app_id, body_json, version, updated_at)
		VALUES (?, ?, ?, ?, ?)`,
		"owner",
		"app",
		`{"second":true}`,
		1,
		"2026-05-06T12:00:01Z",
	)
	if err == nil {
		t.Fatal("expected unique constraint error")
	}
}

func TestPutDocumentVersioning(t *testing.T) {
	t.Parallel()

	docStore := openTestStore(t)
	ctx := context.Background()

	first, err := docStore.PutDocument(ctx, "owner", "app", json.RawMessage(`{"name":"first"}`), nil)
	if err != nil {
		t.Fatalf("create document: %v", err)
	}

	if first.Version != 1 {
		t.Fatalf("expected version 1, got %d", first.Version)
	}

	second, err := docStore.PutDocument(ctx, "owner", "app", json.RawMessage(`{"name":"second"}`), nil)
	if err != nil {
		t.Fatalf("update document without version: %v", err)
	}

	if second.Version != 2 {
		t.Fatalf("expected version 2, got %d", second.Version)
	}

	expectedVersion := second.Version
	third, err := docStore.PutDocument(ctx, "owner", "app", json.RawMessage(`{"name":"third"}`), &expectedVersion)
	if err != nil {
		t.Fatalf("update document with version: %v", err)
	}

	if third.Version != 3 {
		t.Fatalf("expected version 3, got %d", third.Version)
	}

	conflictingVersion := int64(1)
	_, err = docStore.PutDocument(ctx, "owner", "app", json.RawMessage(`{"name":"conflict"}`), &conflictingVersion)
	if !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("expected version conflict, got %v", err)
	}
}

func TestPutDocumentStoresUTCTimestamp(t *testing.T) {
	t.Parallel()

	docStore := openTestStore(t)
	ctx := context.Background()

	created, err := docStore.PutDocument(ctx, "owner", "app", json.RawMessage(`{"name":"first"}`), nil)
	if err != nil {
		t.Fatalf("create document: %v", err)
	}

	if created.UpdatedAt.Location() != timeUTC() {
		t.Fatalf("expected UTC location, got %v", created.UpdatedAt.Location())
	}

	var storedUpdatedAt string
	err = docStore.db.QueryRowContext(
		ctx,
		`SELECT updated_at FROM documents WHERE owner_key = ? AND app_id = ?`,
		"owner",
		"app",
	).Scan(&storedUpdatedAt)
	if err != nil {
		t.Fatalf("query stored timestamp: %v", err)
	}

	if !strings.HasSuffix(storedUpdatedAt, "Z") {
		t.Fatalf("expected UTC timestamp, got %q", storedUpdatedAt)
	}
}

func TestCreateInitialUserAndLookup(t *testing.T) {
	t.Parallel()

	docStore := openTestStore(t)
	ctx := context.Background()

	user, err := docStore.CreateInitialUser(ctx, "owner", "argon-hash", "owner-key")
	if err != nil {
		t.Fatalf("create initial user: %v", err)
	}

	if !user.IsAdmin || user.OwnerKey != "owner-key" {
		t.Fatalf("unexpected user: %+v", user)
	}

	loaded, err := docStore.GetUserByUsername(ctx, "OWNER")
	if err != nil {
		t.Fatalf("load user by username: %v", err)
	}

	if loaded.ID != user.ID || loaded.Username != "owner" {
		t.Fatalf("unexpected loaded user: %+v", loaded)
	}
}

func TestCreateInitialUserOnlyAllowsFirstAccount(t *testing.T) {
	t.Parallel()

	docStore := openTestStore(t)
	ctx := context.Background()

	if _, err := docStore.CreateInitialUser(ctx, "owner", "argon-hash", "owner-key"); err != nil {
		t.Fatalf("create first user: %v", err)
	}

	if _, err := docStore.CreateInitialUser(ctx, "second", "argon-hash-2", "owner-key-2"); !errors.Is(err, ErrSetupAlreadyComplete) {
		t.Fatalf("expected setup already complete, got %v", err)
	}
}

func TestCreateUserAllowsAdditionalAccounts(t *testing.T) {
	t.Parallel()

	docStore := openTestStore(t)
	ctx := context.Background()

	if _, err := docStore.CreateInitialUser(ctx, "owner", "argon-hash", "owner-key"); err != nil {
		t.Fatalf("create initial user: %v", err)
	}

	user, err := docStore.CreateUser(ctx, "guest", "argon-hash-2", "guest-owner-key")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	if user.IsAdmin {
		t.Fatalf("expected hosted user to be non-admin, got %+v", user)
	}
	if user.OwnerKey != "guest-owner-key" {
		t.Fatalf("expected owner key to be preserved, got %q", user.OwnerKey)
	}
}

func TestCreateUserRejectsDuplicateUsername(t *testing.T) {
	t.Parallel()

	docStore := openTestStore(t)
	ctx := context.Background()

	if _, err := docStore.CreateUser(ctx, "owner", "argon-hash", "owner-key"); err != nil {
		t.Fatalf("create first user: %v", err)
	}

	if _, err := docStore.CreateUser(ctx, "OWNER", "argon-hash-2", "owner-key-2"); !errors.Is(err, ErrUsernameTaken) {
		t.Fatalf("expected username taken, got %v", err)
	}
}

func TestSessionLifecycle(t *testing.T) {
	t.Parallel()

	docStore := openTestStore(t)
	ctx := context.Background()
	user, err := docStore.CreateInitialUser(ctx, "owner", "argon-hash", "owner-key")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	session, err := docStore.CreateSession(ctx, user.ID, "token-hash", time.Now().UTC().Add(24*time.Hour))
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	loaded, err := docStore.GetSessionUserByTokenHash(ctx, "token-hash")
	if err != nil {
		t.Fatalf("load session: %v", err)
	}

	if loaded.User.ID != user.ID || loaded.Session.ID != session.ID {
		t.Fatalf("unexpected session user: %+v", loaded)
	}

	if err := docStore.TouchSession(ctx, session.ID, time.Now().UTC().Add(5*time.Minute)); err != nil {
		t.Fatalf("touch session: %v", err)
	}

	if err := docStore.DeleteSessionByTokenHash(ctx, "token-hash"); err != nil {
		t.Fatalf("delete session: %v", err)
	}

	if _, err := docStore.GetSessionUserByTokenHash(ctx, "token-hash"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("expected session not found, got %v", err)
	}
}

func TestGetAccountEntitlementReturnsNotFoundWhenMissing(t *testing.T) {
	t.Parallel()

	docStore := openTestStore(t)
	ctx := context.Background()

	if _, err := docStore.GetAccountEntitlement(ctx, 999, EntitlementKeyHostedSync); !errors.Is(err, ErrEntitlementNotFound) {
		t.Fatalf("expected entitlement not found, got %v", err)
	}
}

func TestPutAndGetAccountEntitlement(t *testing.T) {
	t.Parallel()

	docStore := openTestStore(t)
	ctx := context.Background()
	user, err := docStore.CreateUser(ctx, "entitled-user", "argon-hash", "entitled-owner-key")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	validUntil := time.Date(2026, time.May, 18, 15, 0, 0, 0, time.UTC)
	saved, err := docStore.PutAccountEntitlement(
		ctx,
		user.ID,
		EntitlementKeyHostedSync,
		EntitlementStatusActive,
		EntitlementSourceManual,
		&validUntil,
	)
	if err != nil {
		t.Fatalf("put account entitlement: %v", err)
	}

	if saved.UserID != user.ID || saved.EntitlementKey != EntitlementKeyHostedSync || saved.Status != EntitlementStatusActive || saved.Source != EntitlementSourceManual {
		t.Fatalf("unexpected saved entitlement: %+v", saved)
	}
	if saved.ValidUntil == nil || !saved.ValidUntil.Equal(validUntil) {
		t.Fatalf("expected valid_until %v, got %+v", validUntil, saved.ValidUntil)
	}

	loaded, err := docStore.GetAccountEntitlement(ctx, user.ID, EntitlementKeyHostedSync)
	if err != nil {
		t.Fatalf("get account entitlement: %v", err)
	}

	if loaded.UserID != user.ID || loaded.EntitlementKey != EntitlementKeyHostedSync || loaded.Status != EntitlementStatusActive || loaded.Source != EntitlementSourceManual {
		t.Fatalf("unexpected loaded entitlement: %+v", loaded)
	}
	if loaded.ValidUntil == nil || !loaded.ValidUntil.Equal(validUntil) {
		t.Fatalf("expected loaded valid_until %v, got %+v", validUntil, loaded.ValidUntil)
	}
}

func TestPutAccountEntitlementUpdatesExistingRow(t *testing.T) {
	t.Parallel()

	docStore := openTestStore(t)
	ctx := context.Background()
	user, err := docStore.CreateUser(ctx, "upsert-user", "argon-hash", "upsert-owner-key")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	firstValidUntil := time.Date(2026, time.May, 18, 15, 0, 0, 0, time.UTC)
	if _, err := docStore.PutAccountEntitlement(
		ctx,
		user.ID,
		EntitlementKeyHostedSync,
		EntitlementStatusPastDue,
		EntitlementSourceManual,
		&firstValidUntil,
	); err != nil {
		t.Fatalf("put first account entitlement: %v", err)
	}

	if _, err := docStore.PutAccountEntitlement(
		ctx,
		user.ID,
		EntitlementKeyHostedSync,
		EntitlementStatusActive,
		EntitlementSourceAdmin,
		nil,
	); err != nil {
		t.Fatalf("put replacement account entitlement: %v", err)
	}

	loaded, err := docStore.GetAccountEntitlement(ctx, user.ID, EntitlementKeyHostedSync)
	if err != nil {
		t.Fatalf("get updated account entitlement: %v", err)
	}

	if loaded.Status != EntitlementStatusActive || loaded.Source != EntitlementSourceAdmin {
		t.Fatalf("expected updated entitlement status/source, got %+v", loaded)
	}
	if loaded.ValidUntil != nil {
		t.Fatalf("expected valid_until to be cleared, got %+v", loaded.ValidUntil)
	}
}

func TestAccountEntitlementsSchemaEnforcesUniqueUserAndEntitlementKey(t *testing.T) {
	t.Parallel()

	docStore := openTestStore(t)
	ctx := context.Background()
	user, err := docStore.CreateUser(ctx, "unique-entitlement-user", "argon-hash", "unique-entitlement-owner")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	_, err = docStore.db.ExecContext(
		ctx,
		`INSERT INTO account_entitlements (user_id, entitlement_key, status, source, valid_until, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		user.ID,
		EntitlementKeyHostedSync,
		EntitlementStatusActive,
		EntitlementSourceManual,
		nil,
		"2026-05-17T12:00:00Z",
	)
	if err != nil {
		t.Fatalf("insert first entitlement row: %v", err)
	}

	_, err = docStore.db.ExecContext(
		ctx,
		`INSERT INTO account_entitlements (user_id, entitlement_key, status, source, valid_until, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		user.ID,
		EntitlementKeyHostedSync,
		EntitlementStatusCanceled,
		EntitlementSourceAdmin,
		nil,
		"2026-05-17T12:00:01Z",
	)
	if err == nil {
		t.Fatal("expected unique constraint error for duplicate entitlement row")
	}
}

func TestBootstrapOwnerKeyPreservesExistingDocumentOwner(t *testing.T) {
	t.Parallel()

	docStore := openTestStore(t)
	ctx := context.Background()

	if _, err := docStore.PutDocument(ctx, "legacy-owner", "postbaby-web", json.RawMessage(`{"tabs":"[]"}`), nil); err != nil {
		t.Fatalf("put document: %v", err)
	}

	ownerKey, err := docStore.BootstrapOwnerKey(ctx)
	if err != nil {
		t.Fatalf("bootstrap owner key: %v", err)
	}

	if ownerKey != "legacy-owner" {
		t.Fatalf("expected legacy-owner, got %q", ownerKey)
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "postbaby-test.db")
	docStore, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open test store: %v", err)
	}

	t.Cleanup(func() {
		if err := docStore.Close(); err != nil {
			t.Fatalf("close test store: %v", err)
		}
	})

	return docStore
}

func timeUTC() *time.Location {
	return time.UTC
}
