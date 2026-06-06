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

	for _, tableName := range []string{"documents", "users", "sessions", "account_entitlements", "billing_customers", "billing_subscriptions", "sync_mutation_receipts"} {
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

func TestAcceptSyncMutationReceiptsStoresAcceptedReceiptsIdempotently(t *testing.T) {
	t.Parallel()

	docStore := openTestStore(t)
	ctx := context.Background()

	firstResults, err := docStore.AcceptSyncMutationReceipts(ctx, "owner", "postbaby-web", []SyncMutationReceiptInput{
		buildTestSyncMutationReceiptInput("mut-1", "CreateNode"),
	})
	if err != nil {
		t.Fatalf("accept first sync mutation receipt: %v", err)
	}
	if len(firstResults) != 1 {
		t.Fatalf("expected one receipt result, got %d", len(firstResults))
	}
	if firstResults[0].Duplicate {
		t.Fatal("expected first receipt insertion to be new")
	}
	if firstResults[0].Receipt.Status != SyncMutationReceiptStatusAccepted {
		t.Fatalf("expected accepted status, got %q", firstResults[0].Receipt.Status)
	}

	count, err := docStore.CountSyncMutationReceipts(ctx, "owner", "postbaby-web")
	if err != nil {
		t.Fatalf("count sync mutation receipts after first insert: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one stored receipt, got %d", count)
	}

	secondResults, err := docStore.AcceptSyncMutationReceipts(ctx, "owner", "postbaby-web", []SyncMutationReceiptInput{
		buildTestSyncMutationReceiptInput("mut-1", "CreateNode"),
	})
	if err != nil {
		t.Fatalf("accept duplicate sync mutation receipt: %v", err)
	}
	if len(secondResults) != 1 {
		t.Fatalf("expected one duplicate receipt result, got %d", len(secondResults))
	}
	if !secondResults[0].Duplicate {
		t.Fatal("expected duplicate receipt result")
	}
	if secondResults[0].Receipt.ID != firstResults[0].Receipt.ID {
		t.Fatalf("expected duplicate receipt to reuse row id %d, got %d", firstResults[0].Receipt.ID, secondResults[0].Receipt.ID)
	}
	if !secondResults[0].Receipt.AcceptedAt.Equal(firstResults[0].Receipt.AcceptedAt) {
		t.Fatalf("expected duplicate receipt accepted_at %v, got %v", firstResults[0].Receipt.AcceptedAt, secondResults[0].Receipt.AcceptedAt)
	}

	count, err = docStore.CountSyncMutationReceipts(ctx, "owner", "postbaby-web")
	if err != nil {
		t.Fatalf("count sync mutation receipts after duplicate insert: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected duplicate insertion to preserve one row, got %d", count)
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

func TestProvisionalUserLifecycle(t *testing.T) {
	t.Parallel()

	docStore := openTestStore(t)
	ctx := context.Background()
	expiresAt := time.Now().UTC().Add(time.Hour)

	user, err := docStore.CreateProvisionalUser(ctx, "checkout-user", "argon-hash", "checkout-owner-key", expiresAt)
	if err != nil {
		t.Fatalf("create provisional user: %v", err)
	}
	if user.AccountStatus != AccountStatusCheckoutPending || user.CheckoutExpiresAt == nil {
		t.Fatalf("expected checkout-pending user with expiry, got %+v", user)
	}

	loaded, err := docStore.GetUserByUsername(ctx, "checkout-user")
	if err != nil {
		t.Fatalf("load provisional user: %v", err)
	}
	if loaded.AccountStatus != AccountStatusCheckoutPending || loaded.CheckoutExpiresAt == nil {
		t.Fatalf("unexpected loaded provisional user: %+v", loaded)
	}

	if err := docStore.ActivateUser(ctx, user.ID); err != nil {
		t.Fatalf("activate provisional user: %v", err)
	}
	activated, err := docStore.GetUserByUsername(ctx, "checkout-user")
	if err != nil {
		t.Fatalf("load activated user: %v", err)
	}
	if activated.AccountStatus != AccountStatusActive || activated.CheckoutExpiresAt != nil {
		t.Fatalf("expected active user after activation, got %+v", activated)
	}
}

func TestDeleteExpiredProvisionalUsersRemovesAccountsAndSessions(t *testing.T) {
	t.Parallel()

	docStore := openTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	expired, err := docStore.CreateProvisionalUser(ctx, "expired-checkout", "argon-hash", "expired-owner", now.Add(-time.Hour))
	if err != nil {
		t.Fatalf("create expired provisional user: %v", err)
	}
	current, err := docStore.CreateProvisionalUser(ctx, "current-checkout", "argon-hash", "current-owner", now.Add(time.Hour))
	if err != nil {
		t.Fatalf("create current provisional user: %v", err)
	}
	if _, err := docStore.CreateSession(ctx, expired.ID, "expired-token", now.Add(time.Hour)); err != nil {
		t.Fatalf("create expired provisional session: %v", err)
	}
	if _, err := docStore.CreateSession(ctx, current.ID, "current-token", now.Add(time.Hour)); err != nil {
		t.Fatalf("create current provisional session: %v", err)
	}

	deleted, err := docStore.DeleteExpiredProvisionalUsers(ctx, now)
	if err != nil {
		t.Fatalf("delete expired provisional users: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected one expired provisional user deleted, got %d", deleted)
	}
	if _, err := docStore.GetUserByUsername(ctx, "expired-checkout"); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("expected expired provisional user removed, got %v", err)
	}
	if _, err := docStore.GetSessionUserByTokenHash(ctx, "expired-token"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("expected expired provisional session removed, got %v", err)
	}
	if _, err := docStore.GetUserByUsername(ctx, "current-checkout"); err != nil {
		t.Fatalf("expected current provisional user to remain: %v", err)
	}
	if _, err := docStore.GetSessionUserByTokenHash(ctx, "current-token"); err != nil {
		t.Fatalf("expected current provisional session to remain: %v", err)
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

func TestPutAndGetBillingCustomer(t *testing.T) {
	t.Parallel()

	docStore := openTestStore(t)
	ctx := context.Background()
	user, err := docStore.CreateUser(ctx, "billing-user", "argon-hash", "billing-owner")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	saved, err := docStore.PutBillingCustomer(ctx, user.ID, "stripe", "cus_123")
	if err != nil {
		t.Fatalf("put billing customer: %v", err)
	}

	if saved.UserID != user.ID || saved.Provider != "stripe" || saved.ProviderCustomerID != "cus_123" {
		t.Fatalf("unexpected saved billing customer: %+v", saved)
	}

	loaded, err := docStore.GetBillingCustomer(ctx, user.ID, "stripe")
	if err != nil {
		t.Fatalf("get billing customer: %v", err)
	}
	if loaded.ProviderCustomerID != "cus_123" {
		t.Fatalf("unexpected loaded billing customer: %+v", loaded)
	}

	byProviderID, err := docStore.GetBillingCustomerByProviderCustomerID(ctx, "stripe", "cus_123")
	if err != nil {
		t.Fatalf("get billing customer by provider id: %v", err)
	}
	if byProviderID.UserID != user.ID {
		t.Fatalf("expected provider lookup to return user %d, got %+v", user.ID, byProviderID)
	}
}

func TestPutBillingSubscriptionUpsertsAndPreservesValidUntil(t *testing.T) {
	t.Parallel()

	docStore := openTestStore(t)
	ctx := context.Background()
	user, err := docStore.CreateUser(ctx, "billing-sub-user", "argon-hash", "billing-sub-owner")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	validUntil := time.Date(2026, time.May, 20, 12, 0, 0, 0, time.UTC)
	if _, err := docStore.PutBillingSubscription(ctx, user.ID, "stripe", "sub_123", EntitlementStatusActive, &validUntil); err != nil {
		t.Fatalf("put billing subscription: %v", err)
	}

	replacement, err := docStore.PutBillingSubscription(ctx, user.ID, "stripe", "sub_123", EntitlementStatusCanceled, nil)
	if err != nil {
		t.Fatalf("replace billing subscription: %v", err)
	}
	if replacement.Status != EntitlementStatusCanceled || replacement.ValidUntil != nil {
		t.Fatalf("unexpected replacement billing subscription: %+v", replacement)
	}

	loaded, err := docStore.GetBillingSubscriptionByProviderSubscriptionID(ctx, "stripe", "sub_123")
	if err != nil {
		t.Fatalf("get billing subscription: %v", err)
	}
	if loaded.UserID != user.ID || loaded.Status != EntitlementStatusCanceled || loaded.ValidUntil != nil {
		t.Fatalf("unexpected loaded billing subscription: %+v", loaded)
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

func buildTestSyncMutationReceiptInput(mutationID, operationType string) SyncMutationReceiptInput {
	baseRevision := int64(6)
	return SyncMutationReceiptInput{
		MutationID:    mutationID,
		ClientID:      "client-1",
		DeviceID:      "device-1",
		Protocol:      "PB-SYNC/1",
		EntityType:    "Node",
		EntityID:      "item-1",
		OperationType: operationType,
		Payload:       json.RawMessage(`{"tabId":"tab-1","position":{"top":"0px","left":"0px"}}`),
		BaseRevision:  &baseRevision,
	}
}
