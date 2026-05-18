package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"postbaby-backend/internal/store"
)

func TestGrantWritesManualHostedSyncEntitlement(t *testing.T) {
	t.Parallel()

	dbPath := createCommandTestUser(t, "dev-grant-user")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run([]string{
		"grant",
		"--db", dbPath,
		"--username", "dev-grant-user",
		"--entitlement", store.EntitlementKeyHostedSync,
	}, stdout, stderr)
	if exitCode != 0 {
		t.Fatalf("expected success, got exit code %d stderr=%q", exitCode, stderr.String())
	}

	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
	if !strings.Contains(stdout.String(), "status=active") || !strings.Contains(stdout.String(), "source=manual") {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}

	sqliteStore := openCommandTestStore(t, dbPath)
	user, err := sqliteStore.GetUserByUsername(context.Background(), "dev-grant-user")
	if err != nil {
		t.Fatalf("load user: %v", err)
	}
	entitlement, err := sqliteStore.GetAccountEntitlement(context.Background(), user.ID, store.EntitlementKeyHostedSync)
	if err != nil {
		t.Fatalf("load entitlement: %v", err)
	}
	if entitlement.Status != store.EntitlementStatusActive || entitlement.Source != store.EntitlementSourceManual {
		t.Fatalf("unexpected entitlement: %+v", entitlement)
	}
	if entitlement.ValidUntil != nil {
		t.Fatalf("expected nil valid_until, got %+v", entitlement.ValidUntil)
	}
}

func TestRevokeUpdatesHostedSyncEntitlement(t *testing.T) {
	t.Parallel()

	dbPath := createCommandTestUser(t, "dev-revoke-user")
	sqliteStore, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open seeded store: %v", err)
	}
	user, err := sqliteStore.GetUserByUsername(context.Background(), "dev-revoke-user")
	if err != nil {
		t.Fatalf("load user: %v", err)
	}
	if _, err := sqliteStore.PutAccountEntitlement(context.Background(), user.ID, store.EntitlementKeyHostedSync, store.EntitlementStatusActive, store.EntitlementSourceManual, nil); err != nil {
		t.Fatalf("seed entitlement: %v", err)
	}
	if err := sqliteStore.Close(); err != nil {
		t.Fatalf("close seeded store: %v", err)
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exitCode := run([]string{
		"revoke",
		"--db", dbPath,
		"--username", "dev-revoke-user",
		"--entitlement", store.EntitlementKeyHostedSync,
	}, stdout, stderr)
	if exitCode != 0 {
		t.Fatalf("expected success, got exit code %d stderr=%q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "status=canceled") {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}

	sqliteStore = openCommandTestStore(t, dbPath)
	entitlement, err := sqliteStore.GetAccountEntitlement(context.Background(), user.ID, store.EntitlementKeyHostedSync)
	if err != nil {
		t.Fatalf("load entitlement: %v", err)
	}
	if entitlement.Status != store.EntitlementStatusCanceled || entitlement.Source != store.EntitlementSourceManual {
		t.Fatalf("unexpected entitlement: %+v", entitlement)
	}
}

func TestShowReportsNoneWhenEntitlementMissing(t *testing.T) {
	t.Parallel()

	dbPath := createCommandTestUser(t, "dev-show-user")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run([]string{
		"show",
		"--db", dbPath,
		"--username", "dev-show-user",
	}, stdout, stderr)
	if exitCode != 0 {
		t.Fatalf("expected success, got exit code %d stderr=%q", exitCode, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("expected empty stderr, got %q", stderr.String())
	}
	if !strings.Contains(stdout.String(), "status=none") || !strings.Contains(stdout.String(), "entitlement=hosted_sync") {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
}

func TestRunRejectsUnsupportedEntitlement(t *testing.T) {
	t.Parallel()

	dbPath := createCommandTestUser(t, "dev-unsupported-user")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run([]string{
		"grant",
		"--db", dbPath,
		"--username", "dev-unsupported-user",
		"--entitlement", "something_else",
	}, stdout, stderr)
	if exitCode != 2 {
		t.Fatalf("expected exit code 2, got %d stderr=%q", exitCode, stderr.String())
	}
	if !strings.Contains(stderr.String(), "error=unsupported entitlement: something_else") {
		t.Fatalf("unexpected stderr: %q", stderr.String())
	}
}

func TestRunFailsWhenUserDoesNotExist(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "postbaby-entitlement-dev.db")
	sqliteStore, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if err := sqliteStore.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exitCode := run([]string{
		"grant",
		"--db", dbPath,
		"--username", "missing-user",
		"--entitlement", store.EntitlementKeyHostedSync,
	}, stdout, stderr)
	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d stderr=%q", exitCode, stderr.String())
	}
	if !strings.Contains(stderr.String(), "error=user not found: missing-user") {
		t.Fatalf("unexpected stderr: %q", stderr.String())
	}
}

func TestRunFailsWhenDatabaseFileDoesNotExist(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "missing-postbaby-entitlement-dev.db")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run([]string{
		"grant",
		"--db", dbPath,
		"--username", "missing-user",
		"--entitlement", store.EntitlementKeyHostedSync,
	}, stdout, stderr)
	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d stderr=%q", exitCode, stderr.String())
	}
	if !strings.Contains(stderr.String(), "error=database file does not exist: "+dbPath) {
		t.Fatalf("unexpected stderr: %q", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("expected empty stdout, got %q", stdout.String())
	}
}

func createCommandTestUser(t *testing.T, username string) string {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "postbaby-entitlement-dev.db")
	sqliteStore, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if _, err := sqliteStore.CreateUser(context.Background(), username, "argon-hash", username+"-owner-key"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := sqliteStore.Close(); err != nil {
		t.Fatalf("close seeded store: %v", err)
	}

	return dbPath
}

func openCommandTestStore(t *testing.T, dbPath string) *store.Store {
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
