package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOpenCreatesSchema(t *testing.T) {
	t.Parallel()

	docStore := openTestStore(t)

	for _, tableName := range []string{"documents", "users", "sessions", "account_entitlements", "billing_customers", "billing_subscriptions", "sync_mutation_receipts", "sync_mutation_replay_dry_run_observations"} {
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

func TestReplaySyncMutationReceiptsDryRunCreateNodeAndDuplicateIdempotency(t *testing.T) {
	t.Parallel()

	docStore := openTestStore(t)
	ctx := context.Background()
	seedReplayDocument(t, docStore, "owner", "postbaby-web", []replayTab{
		{
			ID:          "tab-1",
			Name:        "Main",
			Items:       []replayItem{},
			ColorIndex:  0,
			GridSetting: "none",
			Edges:       []replayEdge{},
		},
	})

	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:00Z", "2026-06-01T12:00:00Z", SyncMutationReceiptInput{
		MutationID:    "mut-create-1",
		ClientID:      "client-1",
		DeviceID:      "device-1",
		Protocol:      "PB-SYNC/1",
		EntityType:    "Node",
		EntityID:      "item-1",
		OperationType: "CreateNode",
		Payload:       json.RawMessage(`{"tabId":"tab-1","name":"First","color":"yellow","shape":"circle","position":{"top":"12px","left":"24px"}}`),
	})
	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:01Z", "2026-06-01T12:00:01Z", SyncMutationReceiptInput{
		MutationID:    "mut-create-2",
		ClientID:      "client-1",
		DeviceID:      "device-1",
		Protocol:      "PB-SYNC/1",
		EntityType:    "Node",
		EntityID:      "item-1",
		OperationType: "CreateNode",
		Payload:       json.RawMessage(`{"tabId":"tab-1","name":"Duplicate","color":"blue","position":{"top":"100px","left":"100px"}}`),
	})

	before, err := docStore.GetDocument(ctx, "owner", "postbaby-web")
	if err != nil {
		t.Fatalf("load document before replay: %v", err)
	}

	result, err := docStore.ReplaySyncMutationReceiptsDryRun(ctx, "owner", "postbaby-web")
	if err != nil {
		t.Fatalf("dry-run replay: %v", err)
	}
	if result.SourceVersion != before.Version {
		t.Fatalf("expected source version %d, got %d", before.Version, result.SourceVersion)
	}
	if result.ConsideredCount != 2 || result.AppliedCount != 1 || result.SkippedCount != 1 {
		t.Fatalf("unexpected dry-run counts: %+v", result)
	}
	if result.WarningCount != 0 {
		t.Fatalf("expected no warnings, got %+v", result.Warnings)
	}

	tabs := extractReplayPreviewTabs(t, result.PreviewBody)
	if len(tabs) != 1 {
		t.Fatalf("expected one preview tab, got %d", len(tabs))
	}
	if len(tabs[0].Items) != 1 {
		t.Fatalf("expected one preview item, got %d", len(tabs[0].Items))
	}
	item := tabs[0].Items[0]
	if item.ID != "item-1" || item.Name != "First" || item.Shape != "circle" {
		t.Fatalf("unexpected preview item: %+v", item)
	}
	if item.Position.Top != "12px" || item.Position.Left != "24px" {
		t.Fatalf("unexpected preview position: %+v", item.Position)
	}

	after, err := docStore.GetDocument(ctx, "owner", "postbaby-web")
	if err != nil {
		t.Fatalf("load document after replay: %v", err)
	}
	if after.Version != before.Version {
		t.Fatalf("expected replay to preserve document version %d, got %d", before.Version, after.Version)
	}
	if string(after.Body) != string(before.Body) {
		t.Fatalf("expected replay to preserve stored document body, before=%s after=%s", before.Body, after.Body)
	}
}

func TestReplaySyncMutationReceiptsDryRunUpdateMoveAndDeleteNode(t *testing.T) {
	t.Parallel()

	docStore := openTestStore(t)
	ctx := context.Background()
	seedReplayDocument(t, docStore, "owner", "postbaby-web", []replayTab{
		{
			ID:   "tab-1",
			Name: "Main",
			Items: []replayItem{
				{ID: "item-1", Name: "First", Color: "yellow", Position: replayPosition{Top: "0px", Left: "0px"}},
				{ID: "item-2", Name: "Second", Color: "blue", Position: replayPosition{Top: "10px", Left: "10px"}},
			},
			ColorIndex:  0,
			GridSetting: "none",
			Edges: []replayEdge{
				{ID: "edge-1", FromItemID: "item-1", ToItemID: "item-2", Kind: "arrow"},
			},
		},
	})

	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:00Z", "2026-06-01T12:00:00Z", SyncMutationReceiptInput{
		MutationID:    "mut-update",
		ClientID:      "client-1",
		DeviceID:      "device-1",
		Protocol:      "PB-SYNC/1",
		EntityType:    "Node",
		EntityID:      "item-1",
		OperationType: "UpdateNode",
		Payload:       json.RawMessage(`{"tabId":"tab-1","changes":{"name":"Renamed","color":"green","shape":"diamond","width":601,"height":40}}`),
	})
	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:01Z", "2026-06-01T12:00:01Z", SyncMutationReceiptInput{
		MutationID:    "mut-move",
		ClientID:      "client-1",
		DeviceID:      "device-1",
		Protocol:      "PB-SYNC/1",
		EntityType:    "Node",
		EntityID:      "item-1",
		OperationType: "MoveNode",
		Payload:       json.RawMessage(`{"tabId":"tab-1","position":{"top":"999999px","left":"-999999px"}}`),
	})
	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:02Z", "2026-06-01T12:00:02Z", SyncMutationReceiptInput{
		MutationID:    "mut-delete",
		ClientID:      "client-1",
		DeviceID:      "device-1",
		Protocol:      "PB-SYNC/1",
		EntityType:    "Node",
		EntityID:      "item-2",
		OperationType: "DeleteNode",
		Payload:       json.RawMessage(`{"tabId":"tab-1"}`),
	})

	result, err := docStore.ReplaySyncMutationReceiptsDryRun(ctx, "owner", "postbaby-web")
	if err != nil {
		t.Fatalf("dry-run replay: %v", err)
	}
	if result.AppliedCount != 3 || result.SkippedCount != 0 {
		t.Fatalf("unexpected dry-run counts: %+v", result)
	}

	tabs := extractReplayPreviewTabs(t, result.PreviewBody)
	tab := findReplayTab(tabs, "tab-1")
	if tab == nil {
		t.Fatal("expected preview tab")
	}
	if len(tab.Items) != 1 {
		t.Fatalf("expected one preview item after delete, got %d", len(tab.Items))
	}
	item := findReplayItem(tab, "item-1")
	if item == nil {
		t.Fatal("expected item-1 to remain")
	}
	if item.Name != "Renamed" || item.Color != "green" || item.Shape != "diamond" {
		t.Fatalf("unexpected updated item: %+v", item)
	}
	if item.Width == nil || *item.Width != 600 {
		t.Fatalf("expected width clamp to 600, got %+v", item.Width)
	}
	if item.Height == nil || *item.Height != 80 {
		t.Fatalf("expected height clamp to 80, got %+v", item.Height)
	}
	if item.Position.Top != "100000px" || item.Position.Left != "-100000px" {
		t.Fatalf("unexpected moved item position: %+v", item.Position)
	}
	if len(tab.Edges) != 0 {
		t.Fatalf("expected dependent edge removal, got %+v", tab.Edges)
	}
}

func TestReplaySyncMutationReceiptsDryRunCreateEdgeDeleteEdgeAndMissingEndpoint(t *testing.T) {
	t.Parallel()

	docStore := openTestStore(t)
	ctx := context.Background()
	seedReplayDocument(t, docStore, "owner", "postbaby-web", []replayTab{
		{
			ID:   "tab-1",
			Name: "Main",
			Items: []replayItem{
				{ID: "item-1", Name: "First", Color: "yellow", Position: replayPosition{Top: "0px", Left: "0px"}},
				{ID: "item-2", Name: "Second", Color: "blue", Position: replayPosition{Top: "10px", Left: "10px"}},
			},
			ColorIndex:  0,
			GridSetting: "none",
			Edges:       []replayEdge{},
		},
	})

	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:00Z", "2026-06-01T12:00:00Z", SyncMutationReceiptInput{
		MutationID:    "mut-create-edge",
		ClientID:      "client-1",
		DeviceID:      "device-1",
		Protocol:      "PB-SYNC/1",
		EntityType:    "Edge",
		EntityID:      "edge-1",
		OperationType: "CreateEdge",
		Payload:       json.RawMessage(`{"tabId":"tab-1","fromItemId":"item-1","toItemId":"item-2","kind":"arrow"}`),
	})
	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:01Z", "2026-06-01T12:00:01Z", SyncMutationReceiptInput{
		MutationID:    "mut-create-edge-missing",
		ClientID:      "client-1",
		DeviceID:      "device-1",
		Protocol:      "PB-SYNC/1",
		EntityType:    "Edge",
		EntityID:      "edge-2",
		OperationType: "CreateEdge",
		Payload:       json.RawMessage(`{"tabId":"tab-1","fromItemId":"item-1","toItemId":"missing","kind":"line"}`),
	})
	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:02Z", "2026-06-01T12:00:02Z", SyncMutationReceiptInput{
		MutationID:    "mut-delete-edge",
		ClientID:      "client-1",
		DeviceID:      "device-1",
		Protocol:      "PB-SYNC/1",
		EntityType:    "Edge",
		EntityID:      "edge-1",
		OperationType: "DeleteEdge",
		Payload:       json.RawMessage(`{"tabId":"tab-1"}`),
	})
	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:03Z", "2026-06-01T12:00:03Z", SyncMutationReceiptInput{
		MutationID:    "mut-delete-edge-missing",
		ClientID:      "client-1",
		DeviceID:      "device-1",
		Protocol:      "PB-SYNC/1",
		EntityType:    "Edge",
		EntityID:      "edge-404",
		OperationType: "DeleteEdge",
		Payload:       json.RawMessage(`{"tabId":"tab-1"}`),
	})

	result, err := docStore.ReplaySyncMutationReceiptsDryRun(ctx, "owner", "postbaby-web")
	if err != nil {
		t.Fatalf("dry-run replay: %v", err)
	}
	if result.AppliedCount != 2 || result.SkippedCount != 2 {
		t.Fatalf("unexpected dry-run counts: %+v", result)
	}
	assertReplayWarningCodes(t, result.Warnings, []string{"skipped_missing_endpoint"})

	tabs := extractReplayPreviewTabs(t, result.PreviewBody)
	tab := findReplayTab(tabs, "tab-1")
	if tab == nil {
		t.Fatal("expected preview tab")
	}
	if len(tab.Edges) != 0 {
		t.Fatalf("expected no preview edges after create/delete cycle, got %+v", tab.Edges)
	}
}

func TestReplaySyncMutationReceiptsDryRunMissingTabAndUnknownOperation(t *testing.T) {
	t.Parallel()

	docStore := openTestStore(t)
	ctx := context.Background()
	seedReplayDocument(t, docStore, "owner", "postbaby-web", []replayTab{
		{
			ID:          "tab-1",
			Name:        "Main",
			Items:       []replayItem{},
			ColorIndex:  0,
			GridSetting: "none",
			Edges:       []replayEdge{},
		},
	})

	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:00Z", "2026-06-01T12:00:00Z", SyncMutationReceiptInput{
		MutationID:    "mut-missing-tab",
		ClientID:      "client-1",
		DeviceID:      "device-1",
		Protocol:      "PB-SYNC/1",
		EntityType:    "Node",
		EntityID:      "item-1",
		OperationType: "CreateNode",
		Payload:       json.RawMessage(`{"tabId":"tab-404","name":"First","color":"yellow","position":{"top":"0px","left":"0px"}}`),
	})
	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:01Z", "2026-06-01T12:00:01Z", SyncMutationReceiptInput{
		MutationID:    "mut-unknown",
		ClientID:      "client-1",
		DeviceID:      "device-1",
		Protocol:      "PB-SYNC/1",
		EntityType:    "Node",
		EntityID:      "item-1",
		OperationType: "UnknownOperation",
		Payload:       json.RawMessage(`{"tabId":"tab-1"}`),
	})

	result, err := docStore.ReplaySyncMutationReceiptsDryRun(ctx, "owner", "postbaby-web")
	if err != nil {
		t.Fatalf("dry-run replay: %v", err)
	}
	if result.AppliedCount != 0 || result.SkippedCount != 2 {
		t.Fatalf("unexpected dry-run counts: %+v", result)
	}
	assertReplayWarningCodes(t, result.Warnings, []string{"missing_tab", "unknown_operation"})

	tabs := extractReplayPreviewTabs(t, result.PreviewBody)
	if len(tabs) != 1 || tabs[0].ID != "tab-1" {
		t.Fatalf("expected replay to preserve original tab set, got %+v", tabs)
	}
}

func TestReplaySyncMutationReceiptsDryRunOrderingAndScope(t *testing.T) {
	t.Parallel()

	docStore := openTestStore(t)
	ctx := context.Background()
	seedReplayDocument(t, docStore, "owner", "postbaby-web", []replayTab{
		{
			ID:   "tab-1",
			Name: "Main",
			Items: []replayItem{
				{ID: "item-1", Name: "First", Color: "yellow", Position: replayPosition{Top: "0px", Left: "0px"}},
			},
			ColorIndex:  0,
			GridSetting: "none",
			Edges:       []replayEdge{},
		},
	})

	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:01Z", "2026-06-01T12:00:09Z", SyncMutationReceiptInput{
		MutationID:    "mut-accepted-first",
		ClientID:      "client-1",
		DeviceID:      "device-1",
		Protocol:      "PB-SYNC/1",
		EntityType:    "Node",
		EntityID:      "item-1",
		OperationType: "MoveNode",
		Payload:       json.RawMessage(`{"tabId":"tab-1","position":{"top":"10px","left":"10px"}}`),
	})
	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:02Z", "2026-06-01T12:00:01Z", SyncMutationReceiptInput{
		MutationID:    "mut-created-first",
		ClientID:      "client-1",
		DeviceID:      "device-1",
		Protocol:      "PB-SYNC/1",
		EntityType:    "Node",
		EntityID:      "item-1",
		OperationType: "MoveNode",
		Payload:       json.RawMessage(`{"tabId":"tab-1","position":{"top":"20px","left":"20px"}}`),
	})
	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:02Z", "2026-06-01T12:00:05Z", SyncMutationReceiptInput{
		MutationID:    "mut-a",
		ClientID:      "client-1",
		DeviceID:      "device-1",
		Protocol:      "PB-SYNC/1",
		EntityType:    "Node",
		EntityID:      "item-1",
		OperationType: "MoveNode",
		Payload:       json.RawMessage(`{"tabId":"tab-1","position":{"top":"30px","left":"30px"}}`),
	})
	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:02Z", "2026-06-01T12:00:05Z", SyncMutationReceiptInput{
		MutationID:    "mut-b",
		ClientID:      "client-1",
		DeviceID:      "device-1",
		Protocol:      "PB-SYNC/1",
		EntityType:    "Node",
		EntityID:      "item-1",
		OperationType: "MoveNode",
		Payload:       json.RawMessage(`{"tabId":"tab-1","position":{"top":"40px","left":"40px"}}`),
	})
	insertAcceptedReplayReceipt(t, docStore, "other-owner", "postbaby-web", "2026-06-01T12:00:03Z", "2026-06-01T12:00:03Z", SyncMutationReceiptInput{
		MutationID:    "mut-other-owner",
		ClientID:      "client-2",
		DeviceID:      "device-2",
		Protocol:      "PB-SYNC/1",
		EntityType:    "Node",
		EntityID:      "item-1",
		OperationType: "MoveNode",
		Payload:       json.RawMessage(`{"tabId":"tab-1","position":{"top":"999px","left":"999px"}}`),
	})
	insertAcceptedReplayReceipt(t, docStore, "owner", "other-app", "2026-06-01T12:00:04Z", "2026-06-01T12:00:04Z", SyncMutationReceiptInput{
		MutationID:    "mut-other-app",
		ClientID:      "client-3",
		DeviceID:      "device-3",
		Protocol:      "PB-SYNC/1",
		EntityType:    "Node",
		EntityID:      "item-1",
		OperationType: "MoveNode",
		Payload:       json.RawMessage(`{"tabId":"tab-1","position":{"top":"888px","left":"888px"}}`),
	})

	result, err := docStore.ReplaySyncMutationReceiptsDryRun(ctx, "owner", "postbaby-web")
	if err != nil {
		t.Fatalf("dry-run replay: %v", err)
	}
	if result.ConsideredCount != 4 {
		t.Fatalf("expected only same-owner same-app receipts to be considered, got %d", result.ConsideredCount)
	}
	expectedOrder := []string{"mut-accepted-first", "mut-created-first", "mut-a", "mut-b"}
	if len(result.OrderedMutationID) != len(expectedOrder) {
		t.Fatalf("expected %d ordered mutation ids, got %d", len(expectedOrder), len(result.OrderedMutationID))
	}
	for index, expected := range expectedOrder {
		if result.OrderedMutationID[index] != expected {
			t.Fatalf("expected ordered mutation %d to be %q, got %q", index, expected, result.OrderedMutationID[index])
		}
	}

	tabs := extractReplayPreviewTabs(t, result.PreviewBody)
	tab := findReplayTab(tabs, "tab-1")
	if tab == nil {
		t.Fatal("expected preview tab")
	}
	item := findReplayItem(tab, "item-1")
	if item == nil {
		t.Fatal("expected preview item")
	}
	if item.Position.Top != "40px" || item.Position.Left != "40px" {
		t.Fatalf("expected deterministic final move position, got %+v", item.Position)
	}

	repeatedResult, err := docStore.ReplaySyncMutationReceiptsDryRun(ctx, "owner", "postbaby-web")
	if err != nil {
		t.Fatalf("repeat dry-run replay: %v", err)
	}
	if string(repeatedResult.PreviewBody) != string(result.PreviewBody) {
		t.Fatalf("expected repeated dry-run preview to stay stable, first=%s second=%s", result.PreviewBody, repeatedResult.PreviewBody)
	}
	if len(repeatedResult.OrderedMutationID) != len(result.OrderedMutationID) {
		t.Fatalf("expected repeated dry-run ordering length %d, got %d", len(result.OrderedMutationID), len(repeatedResult.OrderedMutationID))
	}
	for index, mutationID := range result.OrderedMutationID {
		if repeatedResult.OrderedMutationID[index] != mutationID {
			t.Fatalf("expected repeated dry-run mutation %d to be %q, got %q", index, mutationID, repeatedResult.OrderedMutationID[index])
		}
	}
}

func TestReplaySyncMutationReceiptsDryRunCreateUpdateMoveSequence(t *testing.T) {
	t.Parallel()

	docStore := openTestStore(t)
	ctx := context.Background()
	seedReplayDocument(t, docStore, "owner", "postbaby-web", []replayTab{
		{
			ID:          "tab-1",
			Name:        "Main",
			Items:       []replayItem{},
			ColorIndex:  0,
			GridSetting: "none",
			Edges:       []replayEdge{},
		},
	})

	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:00Z", "2026-06-01T12:00:00Z",
		buildReplayReceiptInput("mut-create", "Node", "item-1", "CreateNode", `{"tabId":"tab-1","name":"First","color":"yellow","shape":"circle","position":{"top":"10px","left":"20px"}}`),
	)
	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:01Z", "2026-06-01T12:00:01Z",
		buildReplayReceiptInput("mut-update", "Node", "item-1", "UpdateNode", `{"tabId":"tab-1","changes":{"name":"Renamed","color":"green","shape":"hexagon","width":167}}`),
	)
	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:02Z", "2026-06-01T12:00:02Z",
		buildReplayReceiptInput("mut-move", "Node", "item-1", "MoveNode", `{"tabId":"tab-1","position":{"top":"30px","left":"40px"}}`),
	)

	result, err := docStore.ReplaySyncMutationReceiptsDryRun(ctx, "owner", "postbaby-web")
	if err != nil {
		t.Fatalf("dry-run replay: %v", err)
	}
	if result.AppliedCount != 3 || result.SkippedCount != 0 || result.WarningCount != 0 {
		t.Fatalf("unexpected dry-run counts: %+v", result)
	}

	tabs := extractReplayPreviewTabs(t, result.PreviewBody)
	tab := findReplayTab(tabs, "tab-1")
	if tab == nil {
		t.Fatal("expected preview tab")
	}
	item := findReplayItem(tab, "item-1")
	if item == nil {
		t.Fatal("expected preview item")
	}
	if item.Name != "Renamed" || item.Color != "green" || item.Shape != "hexagon" {
		t.Fatalf("unexpected preview item after create/update/move: %+v", item)
	}
	if item.Width == nil || *item.Width != 167 {
		t.Fatalf("expected stored width 167, got %+v", item.Width)
	}
	if item.Position.Top != "30px" || item.Position.Left != "40px" {
		t.Fatalf("unexpected preview position: %+v", item.Position)
	}
}

func TestReplaySyncMutationReceiptsDryRunCreateEdgeAndMoveSequence(t *testing.T) {
	t.Parallel()

	docStore := openTestStore(t)
	ctx := context.Background()
	seedReplayDocument(t, docStore, "owner", "postbaby-web", []replayTab{
		{
			ID:   "tab-1",
			Name: "Main",
			Items: []replayItem{
				{ID: "item-2", Name: "Anchor", Color: "blue", Position: replayPosition{Top: "100px", Left: "100px"}},
			},
			ColorIndex:  0,
			GridSetting: "none",
			Edges:       []replayEdge{},
		},
	})

	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:00Z", "2026-06-01T12:00:00Z",
		buildReplayReceiptInput("mut-create", "Node", "item-1", "CreateNode", `{"tabId":"tab-1","name":"First","color":"yellow","position":{"top":"10px","left":"20px"}}`),
	)
	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:01Z", "2026-06-01T12:00:01Z",
		buildReplayReceiptInput("mut-edge", "Edge", "edge-1", "CreateEdge", `{"tabId":"tab-1","fromItemId":"item-1","toItemId":"item-2","kind":"arrow"}`),
	)
	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:02Z", "2026-06-01T12:00:02Z",
		buildReplayReceiptInput("mut-move", "Node", "item-1", "MoveNode", `{"tabId":"tab-1","position":{"top":"60px","left":"70px"}}`),
	)

	result, err := docStore.ReplaySyncMutationReceiptsDryRun(ctx, "owner", "postbaby-web")
	if err != nil {
		t.Fatalf("dry-run replay: %v", err)
	}
	if result.AppliedCount != 3 || result.SkippedCount != 0 || result.WarningCount != 0 {
		t.Fatalf("unexpected dry-run counts: %+v", result)
	}

	tabs := extractReplayPreviewTabs(t, result.PreviewBody)
	tab := findReplayTab(tabs, "tab-1")
	if tab == nil {
		t.Fatal("expected preview tab")
	}
	item := findReplayItem(tab, "item-1")
	if item == nil {
		t.Fatal("expected created node to exist")
	}
	if item.Position.Top != "60px" || item.Position.Left != "70px" {
		t.Fatalf("expected moved position, got %+v", item.Position)
	}
	if len(tab.Edges) != 1 {
		t.Fatalf("expected one preview edge, got %+v", tab.Edges)
	}
	if tab.Edges[0].FromItemID != "item-1" || tab.Edges[0].ToItemID != "item-2" || tab.Edges[0].Kind != "arrow" {
		t.Fatalf("unexpected preview edge: %+v", tab.Edges[0])
	}
}

func TestReplaySyncMutationReceiptsDryRunDeleteNodeRemovesDependentCreatedEdge(t *testing.T) {
	t.Parallel()

	docStore := openTestStore(t)
	ctx := context.Background()
	seedReplayDocument(t, docStore, "owner", "postbaby-web", []replayTab{
		{
			ID:   "tab-1",
			Name: "Main",
			Items: []replayItem{
				{ID: "item-2", Name: "Anchor", Color: "blue", Position: replayPosition{Top: "100px", Left: "100px"}},
			},
			ColorIndex:  0,
			GridSetting: "none",
			Edges:       []replayEdge{},
		},
	})

	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:00Z", "2026-06-01T12:00:00Z",
		buildReplayReceiptInput("mut-create", "Node", "item-1", "CreateNode", `{"tabId":"tab-1","name":"First","color":"yellow","position":{"top":"10px","left":"20px"}}`),
	)
	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:01Z", "2026-06-01T12:00:01Z",
		buildReplayReceiptInput("mut-edge", "Edge", "edge-1", "CreateEdge", `{"tabId":"tab-1","fromItemId":"item-1","toItemId":"item-2","kind":"line"}`),
	)
	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:02Z", "2026-06-01T12:00:02Z",
		buildReplayReceiptInput("mut-delete", "Node", "item-1", "DeleteNode", `{"tabId":"tab-1"}`),
	)

	result, err := docStore.ReplaySyncMutationReceiptsDryRun(ctx, "owner", "postbaby-web")
	if err != nil {
		t.Fatalf("dry-run replay: %v", err)
	}
	if result.AppliedCount != 3 || result.SkippedCount != 0 || result.WarningCount != 0 {
		t.Fatalf("unexpected dry-run counts: %+v", result)
	}

	tabs := extractReplayPreviewTabs(t, result.PreviewBody)
	tab := findReplayTab(tabs, "tab-1")
	if tab == nil {
		t.Fatal("expected preview tab")
	}
	if findReplayItem(tab, "item-1") != nil {
		t.Fatal("expected deleted node to be absent")
	}
	if findReplayItem(tab, "item-2") == nil {
		t.Fatal("expected anchor node to remain")
	}
	if len(tab.Edges) != 0 {
		t.Fatalf("expected dependent edge removal, got %+v", tab.Edges)
	}
}

func TestReplaySyncMutationReceiptsDryRunDeleteNodeThenUpdateSkipsMissingNode(t *testing.T) {
	t.Parallel()

	docStore := openTestStore(t)
	ctx := context.Background()
	seedReplayDocument(t, docStore, "owner", "postbaby-web", []replayTab{
		{
			ID:   "tab-1",
			Name: "Main",
			Items: []replayItem{
				{ID: "item-1", Name: "First", Color: "yellow", Position: replayPosition{Top: "0px", Left: "0px"}},
			},
			ColorIndex:  0,
			GridSetting: "none",
			Edges:       []replayEdge{},
		},
	})

	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:00Z", "2026-06-01T12:00:00Z",
		buildReplayReceiptInput("mut-delete", "Node", "item-1", "DeleteNode", `{"tabId":"tab-1"}`),
	)
	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:01Z", "2026-06-01T12:00:01Z",
		buildReplayReceiptInput("mut-update", "Node", "item-1", "UpdateNode", `{"tabId":"tab-1","changes":{"name":"Renamed"}}`),
	)

	result, err := docStore.ReplaySyncMutationReceiptsDryRun(ctx, "owner", "postbaby-web")
	if err != nil {
		t.Fatalf("dry-run replay: %v", err)
	}
	if result.AppliedCount != 1 || result.SkippedCount != 1 {
		t.Fatalf("unexpected dry-run counts: %+v", result)
	}
	assertReplayWarningCodes(t, result.Warnings, []string{replayWarningSkippedMissingNode})

	tabs := extractReplayPreviewTabs(t, result.PreviewBody)
	tab := findReplayTab(tabs, "tab-1")
	if tab == nil {
		t.Fatal("expected preview tab")
	}
	if findReplayItem(tab, "item-1") != nil {
		t.Fatal("expected deleted node to remain absent")
	}
}

func TestReplaySyncMutationReceiptsDryRunCreateEdgeBeforeEndpointsExistSkipsWithoutBackfill(t *testing.T) {
	t.Parallel()

	docStore := openTestStore(t)
	ctx := context.Background()
	seedReplayDocument(t, docStore, "owner", "postbaby-web", []replayTab{
		{
			ID:          "tab-1",
			Name:        "Main",
			Items:       []replayItem{},
			ColorIndex:  0,
			GridSetting: "none",
			Edges:       []replayEdge{},
		},
	})

	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:00Z", "2026-06-01T12:00:00Z",
		buildReplayReceiptInput("mut-edge-early", "Edge", "edge-1", "CreateEdge", `{"tabId":"tab-1","fromItemId":"item-1","toItemId":"item-2","kind":"arrow"}`),
	)
	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:01Z", "2026-06-01T12:00:01Z",
		buildReplayReceiptInput("mut-create-1", "Node", "item-1", "CreateNode", `{"tabId":"tab-1","name":"First","color":"yellow","position":{"top":"0px","left":"0px"}}`),
	)
	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:02Z", "2026-06-01T12:00:02Z",
		buildReplayReceiptInput("mut-create-2", "Node", "item-2", "CreateNode", `{"tabId":"tab-1","name":"Second","color":"blue","position":{"top":"20px","left":"20px"}}`),
	)

	result, err := docStore.ReplaySyncMutationReceiptsDryRun(ctx, "owner", "postbaby-web")
	if err != nil {
		t.Fatalf("dry-run replay: %v", err)
	}
	if result.AppliedCount != 2 || result.SkippedCount != 1 {
		t.Fatalf("unexpected dry-run counts: %+v", result)
	}
	assertReplayWarningCodes(t, result.Warnings, []string{replayWarningSkippedMissingEP})

	tabs := extractReplayPreviewTabs(t, result.PreviewBody)
	tab := findReplayTab(tabs, "tab-1")
	if tab == nil {
		t.Fatal("expected preview tab")
	}
	if len(tab.Items) != 2 {
		t.Fatalf("expected both nodes to exist, got %+v", tab.Items)
	}
	if len(tab.Edges) != 0 {
		t.Fatalf("expected skipped edge to stay absent, got %+v", tab.Edges)
	}
}

func TestReplaySyncMutationReceiptsDryRunDuplicateDeleteNodeAndDeleteEdgeAreIdempotent(t *testing.T) {
	t.Parallel()

	docStore := openTestStore(t)
	ctx := context.Background()
	seedReplayDocument(t, docStore, "owner", "postbaby-web", []replayTab{
		{
			ID:   "tab-1",
			Name: "Main",
			Items: []replayItem{
				{ID: "item-1", Name: "First", Color: "yellow", Position: replayPosition{Top: "0px", Left: "0px"}},
				{ID: "item-2", Name: "Second", Color: "blue", Position: replayPosition{Top: "10px", Left: "10px"}},
			},
			ColorIndex:  0,
			GridSetting: "none",
			Edges: []replayEdge{
				{ID: "edge-1", FromItemID: "item-1", ToItemID: "item-2", Kind: "line"},
			},
		},
	})

	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:00Z", "2026-06-01T12:00:00Z",
		buildReplayReceiptInput("mut-delete-edge-1", "Edge", "edge-1", "DeleteEdge", `{"tabId":"tab-1"}`),
	)
	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:01Z", "2026-06-01T12:00:01Z",
		buildReplayReceiptInput("mut-delete-edge-2", "Edge", "edge-1", "DeleteEdge", `{"tabId":"tab-1"}`),
	)
	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:02Z", "2026-06-01T12:00:02Z",
		buildReplayReceiptInput("mut-delete-node-1", "Node", "item-2", "DeleteNode", `{"tabId":"tab-1"}`),
	)
	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:03Z", "2026-06-01T12:00:03Z",
		buildReplayReceiptInput("mut-delete-node-2", "Node", "item-2", "DeleteNode", `{"tabId":"tab-1"}`),
	)

	result, err := docStore.ReplaySyncMutationReceiptsDryRun(ctx, "owner", "postbaby-web")
	if err != nil {
		t.Fatalf("dry-run replay: %v", err)
	}
	if result.AppliedCount != 2 || result.SkippedCount != 2 || result.WarningCount != 0 {
		t.Fatalf("unexpected dry-run counts: %+v", result)
	}

	tabs := extractReplayPreviewTabs(t, result.PreviewBody)
	tab := findReplayTab(tabs, "tab-1")
	if tab == nil {
		t.Fatal("expected preview tab")
	}
	if findReplayItem(tab, "item-2") != nil {
		t.Fatal("expected deleted node to be absent")
	}
	if len(tab.Edges) != 0 {
		t.Fatalf("expected deleted edge to stay absent, got %+v", tab.Edges)
	}
}

func TestReplaySyncMutationReceiptsDryRunPayloadWarnings(t *testing.T) {
	t.Parallel()

	docStore := openTestStore(t)
	ctx := context.Background()
	seedReplayDocument(t, docStore, "owner", "postbaby-web", []replayTab{
		{
			ID:   "tab-1",
			Name: "Main",
			Items: []replayItem{
				{ID: "item-1", Name: "First", Color: "yellow", Position: replayPosition{Top: "0px", Left: "0px"}},
			},
			ColorIndex:  0,
			GridSetting: "none",
			Edges:       []replayEdge{},
		},
	})

	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:00Z", "2026-06-01T12:00:00Z",
		buildReplayReceiptInput("mut-missing-tab", "Node", "item-2", "CreateNode", `{"name":"NoTab","position":{"top":"0px","left":"0px"}}`),
	)
	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:01Z", "2026-06-01T12:00:01Z",
		buildReplayReceiptInput("mut-missing-entity", "Node", "", "CreateNode", `{"tabId":"tab-1","name":"NoId","position":{"top":"0px","left":"0px"}}`),
	)
	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:02Z", "2026-06-01T12:00:02Z",
		buildReplayReceiptInput("mut-unknown-field", "Node", "item-1", "UpdateNode", `{"tabId":"tab-1","changes":{"bogus":"x"}}`),
	)
	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:03Z", "2026-06-01T12:00:03Z",
		buildReplayReceiptInput("mut-invalid-dims", "Node", "item-1", "UpdateNode", `{"tabId":"tab-1","changes":{"width":"wide","height":[]}}`),
	)
	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:04Z", "2026-06-01T12:00:04Z",
		buildReplayReceiptInput("mut-bad-position", "Node", "item-1", "MoveNode", `{"tabId":"tab-1","position":{"top":"bad","left":"0px"}}`),
	)
	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:05Z", "2026-06-01T12:00:05Z",
		buildReplayReceiptInput("mut-missing-endpoint", "Edge", "edge-1", "CreateEdge", `{"tabId":"tab-1","fromItemId":"item-1","kind":"line"}`),
	)
	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:06Z", "2026-06-01T12:00:06Z",
		buildReplayReceiptInput("mut-self-edge", "Edge", "edge-2", "CreateEdge", `{"tabId":"tab-1","fromItemId":"item-1","toItemId":"item-1","kind":"arrow"}`),
	)
	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:07Z", "2026-06-01T12:00:07Z",
		buildReplayReceiptInput("mut-delete-edge-no-tab", "Edge", "edge-3", "DeleteEdge", `{}`),
	)
	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:08Z", "2026-06-01T12:00:08Z",
		buildReplayReceiptInput("mut-wrong-type", "Node", "item-1", "MoveNode", `[]`),
	)

	result, err := docStore.ReplaySyncMutationReceiptsDryRun(ctx, "owner", "postbaby-web")
	if err != nil {
		t.Fatalf("dry-run replay: %v", err)
	}
	if result.AppliedCount != 0 || result.SkippedCount != 9 || result.WarningCount != 10 {
		t.Fatalf("unexpected dry-run counts: %+v", result)
	}
	assertReplayWarningCodes(t, result.Warnings, []string{
		replayWarningMissingTab,
		replayWarningMissingEntityID,
		replayWarningUnknownUpdateField,
		replayWarningInvalidUpdateValue,
		replayWarningInvalidUpdateValue,
		replayWarningInvalidPosition,
		replayWarningSkippedMissingEP,
		replayWarningSelfEdge,
		replayWarningMissingTab,
		replayWarningInvalidPayload,
	})

	tabs := extractReplayPreviewTabs(t, result.PreviewBody)
	tab := findReplayTab(tabs, "tab-1")
	if tab == nil {
		t.Fatal("expected preview tab")
	}
	item := findReplayItem(tab, "item-1")
	if item == nil {
		t.Fatal("expected original item to remain")
	}
	if item.Name != "First" || item.Position.Top != "0px" || item.Position.Left != "0px" {
		t.Fatalf("expected malformed payloads to preserve original item, got %+v", item)
	}
	if len(tab.Edges) != 0 {
		t.Fatalf("expected malformed payloads to avoid creating edges, got %+v", tab.Edges)
	}
}

func TestReplaySyncMutationReceiptsDryRunPreviewNotPersistedAndStableAcrossCalls(t *testing.T) {
	t.Parallel()

	docStore := openTestStore(t)
	ctx := context.Background()
	before := seedReplayDocument(t, docStore, "owner", "postbaby-web", []replayTab{
		{
			ID:          "tab-1",
			Name:        "Main",
			Items:       []replayItem{},
			ColorIndex:  0,
			GridSetting: "none",
			Edges:       []replayEdge{},
		},
	})

	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:00Z", "2026-06-01T12:00:00Z",
		buildReplayReceiptInput("mut-create", "Node", "item-1", "CreateNode", `{"tabId":"tab-1","name":"First","color":"yellow","position":{"top":"10px","left":"20px"}}`),
	)

	firstResult, err := docStore.ReplaySyncMutationReceiptsDryRun(ctx, "owner", "postbaby-web")
	if err != nil {
		t.Fatalf("first dry-run replay: %v", err)
	}
	secondResult, err := docStore.ReplaySyncMutationReceiptsDryRun(ctx, "owner", "postbaby-web")
	if err != nil {
		t.Fatalf("second dry-run replay: %v", err)
	}
	if string(firstResult.PreviewBody) != string(secondResult.PreviewBody) {
		t.Fatalf("expected stable preview body across calls, first=%s second=%s", firstResult.PreviewBody, secondResult.PreviewBody)
	}
	if string(firstResult.PreviewBody) == string(before.Body) {
		t.Fatalf("expected preview output to differ from the stored document body, body=%s preview=%s", before.Body, firstResult.PreviewBody)
	}

	after, err := docStore.GetDocument(ctx, "owner", "postbaby-web")
	if err != nil {
		t.Fatalf("load document after repeated dry-run replay: %v", err)
	}
	if after.Version != before.Version {
		t.Fatalf("expected dry-run replay to preserve version %d, got %d", before.Version, after.Version)
	}
	if string(after.Body) != string(before.Body) {
		t.Fatalf("expected dry-run replay to avoid persisting preview output, before=%s after=%s", before.Body, after.Body)
	}
}

func TestReplaySyncMutationReceiptsDryRunMalformedLegacySnapshotIsStable(t *testing.T) {
	t.Parallel()

	docStore := openTestStore(t)
	ctx := context.Background()
	before := seedReplayDocumentBody(t, docStore, "owner", "postbaby-web", buildReplayRawSnapshotBody(t, `[
		{"id":"tab-empty","name":"Empty"},
		{"id":"tab-legacy","name":"Legacy","items":[
			{"id":"item-no-position","name":"NoPos","color":"yellow"},
			{"id":"item-bad-position","name":"BadPos","color":"blue","position":{"top":"bad","left":"oops"},"shape":"mystery"},
			{"id":"dup-item","name":"FirstDup","color":"green","position":{"top":"0px","left":"0px"}},
			{"id":"dup-item","name":"SecondDup","color":"purple","position":{"top":"10px","left":"10px"}}
		],"edges":[
			{"id":"edge-missing-from","toItemId":"item-bad-position","kind":"line"},
			{"id":"edge-missing-to","fromItemId":"item-bad-position","kind":"line"},
			{"id":"edge-missing-node","fromItemId":"item-bad-position","toItemId":"missing","kind":"arrow"},
			{"id":"edge-self","fromItemId":"item-bad-position","toItemId":"item-bad-position","kind":"line"},
			{"id":"dup-edge","fromItemId":"dup-item","toItemId":"item-bad-position","kind":"line"},
			{"id":"dup-edge","fromItemId":"item-bad-position","toItemId":"dup-item","kind":"arrow"}
		]}
	]`))

	firstResult, err := docStore.ReplaySyncMutationReceiptsDryRun(ctx, "owner", "postbaby-web")
	if err != nil {
		t.Fatalf("first dry-run replay: %v", err)
	}
	secondResult, err := docStore.ReplaySyncMutationReceiptsDryRun(ctx, "owner", "postbaby-web")
	if err != nil {
		t.Fatalf("second dry-run replay: %v", err)
	}
	if firstResult.ConsideredCount != 0 || firstResult.AppliedCount != 0 || firstResult.SkippedCount != 0 || firstResult.WarningCount != 0 {
		t.Fatalf("unexpected dry-run counts for malformed legacy snapshot: %+v", firstResult)
	}
	if string(firstResult.PreviewBody) != string(secondResult.PreviewBody) {
		t.Fatalf("expected malformed legacy replay preview to stay stable, first=%s second=%s", firstResult.PreviewBody, secondResult.PreviewBody)
	}

	tabs := extractReplayPreviewTabs(t, firstResult.PreviewBody)
	emptyTab := findReplayTab(tabs, "tab-empty")
	if emptyTab == nil {
		t.Fatal("expected empty legacy tab")
	}
	if len(emptyTab.Items) != 0 || len(emptyTab.Edges) != 0 {
		t.Fatalf("expected missing collections to normalize to empty arrays, got items=%d edges=%d", len(emptyTab.Items), len(emptyTab.Edges))
	}

	legacyTab := findReplayTab(tabs, "tab-legacy")
	if legacyTab == nil {
		t.Fatal("expected legacy tab")
	}
	if len(legacyTab.Items) != 4 {
		t.Fatalf("expected four legacy items, got %+v", legacyTab.Items)
	}
	if len(legacyTab.Edges) != 6 {
		t.Fatalf("expected six legacy edges, got %+v", legacyTab.Edges)
	}

	itemNoPosition := findReplayItem(legacyTab, "item-no-position")
	if itemNoPosition == nil {
		t.Fatal("expected item with missing position to load")
	}
	if itemNoPosition.Position.Top != "" || itemNoPosition.Position.Left != "" {
		t.Fatalf("expected missing position to decode to empty strings, got %+v", itemNoPosition.Position)
	}

	itemBadPosition := findReplayItem(legacyTab, "item-bad-position")
	if itemBadPosition == nil {
		t.Fatal("expected item with malformed position to load")
	}
	if itemBadPosition.Position.Top != "bad" || itemBadPosition.Position.Left != "oops" {
		t.Fatalf("expected malformed position to remain untouched, got %+v", itemBadPosition.Position)
	}
	if itemBadPosition.Shape != "mystery" {
		t.Fatalf("expected unknown legacy shape to remain untouched, got %+v", itemBadPosition)
	}

	if countReplayItemsByID(legacyTab, "dup-item") != 2 {
		t.Fatalf("expected duplicate item ids to remain loaded, got %+v", legacyTab.Items)
	}
	if countReplayEdgesByID(legacyTab, "dup-edge") != 2 {
		t.Fatalf("expected duplicate edge ids to remain loaded, got %+v", legacyTab.Edges)
	}

	after, err := docStore.GetDocument(ctx, "owner", "postbaby-web")
	if err != nil {
		t.Fatalf("load document after malformed legacy replay: %v", err)
	}
	if after.Version != before.Version {
		t.Fatalf("expected malformed legacy replay to preserve version %d, got %d", before.Version, after.Version)
	}
	if string(after.Body) != string(before.Body) {
		t.Fatalf("expected malformed legacy replay to preserve canonical body, before=%s after=%s", before.Body, after.Body)
	}
}

func TestReplaySyncMutationReceiptsDryRunMalformedLegacyDuplicateIDsUseFirstMatch(t *testing.T) {
	t.Parallel()

	docStore := openTestStore(t)
	ctx := context.Background()
	seedReplayDocumentBody(t, docStore, "owner", "postbaby-web", buildReplayRawSnapshotBody(t, `[
		{"id":"tab-1","name":"Main","items":[
			{"id":"dup-item","name":"FirstDup","color":"green","position":{"top":"0px","left":"0px"}},
			{"id":"dup-item","name":"SecondDup","color":"purple","position":{"top":"10px","left":"10px"}},
			{"id":"anchor","name":"Anchor","color":"blue","position":{"top":"20px","left":"20px"}}
		],"edges":[
			{"id":"dup-edge","fromItemId":"dup-item","toItemId":"anchor","kind":"line"},
			{"id":"dup-edge","fromItemId":"anchor","toItemId":"dup-item","kind":"arrow"}
		]}
	]`))

	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:00Z", "2026-06-01T12:00:00Z",
		buildReplayReceiptInput("mut-update-dup-item", "Node", "dup-item", "UpdateNode", `{"tabId":"tab-1","changes":{"color":"red"}}`),
	)
	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:01Z", "2026-06-01T12:00:01Z",
		buildReplayReceiptInput("mut-delete-dup-edge", "Edge", "dup-edge", "DeleteEdge", `{"tabId":"tab-1"}`),
	)

	result, err := docStore.ReplaySyncMutationReceiptsDryRun(ctx, "owner", "postbaby-web")
	if err != nil {
		t.Fatalf("dry-run replay: %v", err)
	}
	if result.AppliedCount != 2 || result.SkippedCount != 0 || result.WarningCount != 0 {
		t.Fatalf("unexpected dry-run counts: %+v", result)
	}

	tabs := extractReplayPreviewTabs(t, result.PreviewBody)
	tab := findReplayTab(tabs, "tab-1")
	if tab == nil {
		t.Fatal("expected preview tab")
	}
	if countReplayItemsByID(tab, "dup-item") != 2 {
		t.Fatalf("expected duplicate item ids to remain, got %+v", tab.Items)
	}
	if tab.Items[0].ID != "dup-item" || tab.Items[0].Color != "red" {
		t.Fatalf("expected first duplicate item to be updated, got %+v", tab.Items[0])
	}
	if tab.Items[1].ID != "dup-item" || tab.Items[1].Color != "purple" {
		t.Fatalf("expected second duplicate item to remain unchanged, got %+v", tab.Items[1])
	}
	if countReplayEdgesByID(tab, "dup-edge") != 1 {
		t.Fatalf("expected one duplicate edge id to remain after first-match delete, got %+v", tab.Edges)
	}
	if tab.Edges[0].ID != "dup-edge" || tab.Edges[0].FromItemID != "anchor" || tab.Edges[0].ToItemID != "dup-item" || tab.Edges[0].Kind != "arrow" {
		t.Fatalf("expected second duplicate edge to remain after first-match delete, got %+v", tab.Edges[0])
	}
}

func TestReplaySyncMutationReceiptsDryRunCreateEdgeDuplicateEndpointSemantics(t *testing.T) {
	t.Parallel()

	docStore := openTestStore(t)
	ctx := context.Background()
	seedReplayDocument(t, docStore, "owner", "postbaby-web", []replayTab{
		{
			ID:   "tab-1",
			Name: "Main",
			Items: []replayItem{
				{ID: "item-1", Name: "First", Color: "yellow", Position: replayPosition{Top: "0px", Left: "0px"}},
				{ID: "item-2", Name: "Second", Color: "blue", Position: replayPosition{Top: "10px", Left: "10px"}},
				{ID: "item-3", Name: "Third", Color: "green", Position: replayPosition{Top: "20px", Left: "20px"}},
			},
			ColorIndex:  0,
			GridSetting: "none",
			Edges: []replayEdge{
				{ID: "edge-1", FromItemID: "item-1", ToItemID: "item-2", Kind: "line"},
			},
		},
	})

	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:00Z", "2026-06-01T12:00:00Z",
		buildReplayReceiptInput("mut-same-endpoints", "Edge", "edge-2", "CreateEdge", `{"tabId":"tab-1","fromItemId":"item-1","toItemId":"item-2","kind":"line"}`),
	)
	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:01Z", "2026-06-01T12:00:01Z",
		buildReplayReceiptInput("mut-reversed-endpoints", "Edge", "edge-3", "CreateEdge", `{"tabId":"tab-1","fromItemId":"item-2","toItemId":"item-1","kind":"line"}`),
	)
	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:02Z", "2026-06-01T12:00:02Z",
		buildReplayReceiptInput("mut-different-kind", "Edge", "edge-4", "CreateEdge", `{"tabId":"tab-1","fromItemId":"item-1","toItemId":"item-2","kind":"arrow"}`),
	)
	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:03Z", "2026-06-01T12:00:03Z",
		buildReplayReceiptInput("mut-duplicate-id", "Edge", "edge-1", "CreateEdge", `{"tabId":"tab-1","fromItemId":"item-1","toItemId":"item-3","kind":"line"}`),
	)
	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:04Z", "2026-06-01T12:00:04Z",
		buildReplayReceiptInput("mut-unique-edge", "Edge", "edge-5", "CreateEdge", `{"tabId":"tab-1","fromItemId":"item-1","toItemId":"item-3","kind":"arrow"}`),
	)

	result, err := docStore.ReplaySyncMutationReceiptsDryRun(ctx, "owner", "postbaby-web")
	if err != nil {
		t.Fatalf("dry-run replay: %v", err)
	}
	if result.AppliedCount != 1 || result.SkippedCount != 4 || result.WarningCount != 0 {
		t.Fatalf("unexpected dry-run counts: %+v", result)
	}

	tabs := extractReplayPreviewTabs(t, result.PreviewBody)
	tab := findReplayTab(tabs, "tab-1")
	if tab == nil {
		t.Fatal("expected preview tab")
	}
	if len(tab.Edges) != 2 {
		t.Fatalf("expected original edge plus one unique new edge, got %+v", tab.Edges)
	}
	if tab.Edges[0].ID != "edge-1" || tab.Edges[0].FromItemID != "item-1" || tab.Edges[0].ToItemID != "item-2" || tab.Edges[0].Kind != "line" {
		t.Fatalf("expected original edge to remain untouched, got %+v", tab.Edges[0])
	}
	if tab.Edges[1].ID != "edge-5" || tab.Edges[1].FromItemID != "item-1" || tab.Edges[1].ToItemID != "item-3" || tab.Edges[1].Kind != "arrow" {
		t.Fatalf("expected only unique endpoint pair edge to be added, got %+v", tab.Edges[1])
	}
}

func TestRecordSyncMutationReplayDryRunObservationStoresObservationOnly(t *testing.T) {
	t.Parallel()

	docStore := openTestStore(t)
	ctx := context.Background()
	before := seedReplayDocument(t, docStore, "owner", "postbaby-web", []replayTab{
		{
			ID:          "tab-1",
			Name:        "Main",
			Items:       []replayItem{},
			ColorIndex:  0,
			GridSetting: "none",
			Edges:       []replayEdge{},
		},
	})

	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:00Z", "2026-06-01T12:00:00Z",
		buildReplayReceiptInput("mut-create", "Node", "item-1", "CreateNode", `{"tabId":"tab-1","name":"First","color":"yellow","position":{"top":"10px","left":"20px"}}`),
	)
	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:01Z", "2026-06-01T12:00:01Z",
		buildReplayReceiptInput("mut-self-edge", "Edge", "edge-1", "CreateEdge", `{"tabId":"tab-1","fromItemId":"item-1","toItemId":"item-1","kind":"line"}`),
	)

	beforeReceipts := loadAllReplayReceiptRowsForTest(t, docStore, "owner", "postbaby-web")
	observation, err := docStore.RecordSyncMutationReplayDryRunObservation(ctx, "owner", "postbaby-web")
	if err != nil {
		t.Fatalf("record dry-run observation: %v", err)
	}

	if observation.OwnerKey != "owner" || observation.AppID != "postbaby-web" {
		t.Fatalf("unexpected observation scope: %+v", observation)
	}
	if observation.CanonicalDocumentVersionObserved != before.Version {
		t.Fatalf("expected observed version %d, got %d", before.Version, observation.CanonicalDocumentVersionObserved)
	}
	if observation.ReceiptCountConsidered != 2 || observation.AppliedCount != 1 || observation.SkippedCount != 1 || observation.WarningCount != 1 {
		t.Fatalf("unexpected observation counts: %+v", observation)
	}
	if observation.FirstOrderedMutationID != "mut-create" || observation.LastOrderedMutationID != "mut-self-edge" {
		t.Fatalf("unexpected ordered mutation ids: %+v", observation)
	}
	if observation.OrderedReceiptHighWatermark != "2026-06-01T12:00:01Z|2026-06-01T12:00:01Z|mut-self-edge" {
		t.Fatalf("unexpected receipt high watermark: %q", observation.OrderedReceiptHighWatermark)
	}
	if observation.CanonicalDocumentHashObserved == "" || observation.PreviewHash == "" {
		t.Fatalf("expected non-empty hashes, got %+v", observation)
	}

	storedObservation := loadReplayDryRunObservationForTest(t, docStore, observation.ID)
	if storedObservation.OwnerKey != observation.OwnerKey ||
		storedObservation.AppID != observation.AppID ||
		storedObservation.CanonicalDocumentVersionObserved != observation.CanonicalDocumentVersionObserved ||
		storedObservation.CanonicalDocumentHashObserved != observation.CanonicalDocumentHashObserved ||
		storedObservation.ReceiptCountConsidered != observation.ReceiptCountConsidered ||
		storedObservation.FirstOrderedMutationID != observation.FirstOrderedMutationID ||
		storedObservation.LastOrderedMutationID != observation.LastOrderedMutationID ||
		storedObservation.OrderedReceiptHighWatermark != observation.OrderedReceiptHighWatermark ||
		storedObservation.AppliedCount != observation.AppliedCount ||
		storedObservation.SkippedCount != observation.SkippedCount ||
		storedObservation.WarningCount != observation.WarningCount ||
		storedObservation.PreviewHash != observation.PreviewHash {
		t.Fatalf("stored observation mismatch: stored=%+v returned=%+v", storedObservation, observation)
	}

	after, err := docStore.GetDocument(ctx, "owner", "postbaby-web")
	if err != nil {
		t.Fatalf("load document after observation: %v", err)
	}
	if after.Version != before.Version {
		t.Fatalf("expected dry-run observation to preserve version %d, got %d", before.Version, after.Version)
	}
	if string(after.Body) != string(before.Body) {
		t.Fatalf("expected dry-run observation to preserve body, before=%s after=%s", before.Body, after.Body)
	}

	afterReceipts := loadAllReplayReceiptRowsForTest(t, docStore, "owner", "postbaby-web")
	assertReplayReceiptRowsEqual(t, beforeReceipts, afterReceipts)

	if count := countReplayDryRunObservationsForTest(t, docStore, "owner", "postbaby-web"); count != 1 {
		t.Fatalf("expected one stored observation, got %d", count)
	}
}

func TestRecordSyncMutationReplayDryRunObservationScopeAndStability(t *testing.T) {
	t.Parallel()

	docStore := openTestStore(t)
	ctx := context.Background()
	seedReplayDocument(t, docStore, "owner", "postbaby-web", []replayTab{
		{
			ID:          "tab-1",
			Name:        "Main",
			Items:       []replayItem{},
			ColorIndex:  0,
			GridSetting: "none",
			Edges:       []replayEdge{},
		},
	})

	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:01Z", "2026-06-01T12:00:09Z",
		buildReplayReceiptInput("mut-accepted-first", "Node", "item-1", "CreateNode", `{"tabId":"tab-1","name":"First","color":"yellow","position":{"top":"10px","left":"10px"}}`),
	)
	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:02Z", "2026-06-01T12:00:01Z",
		buildReplayReceiptInput("mut-created-first", "Node", "item-1", "MoveNode", `{"tabId":"tab-1","position":{"top":"20px","left":"20px"}}`),
	)
	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:02Z", "2026-06-01T12:00:05Z",
		buildReplayReceiptInput("mut-a", "Node", "item-1", "MoveNode", `{"tabId":"tab-1","position":{"top":"30px","left":"30px"}}`),
	)
	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:02Z", "2026-06-01T12:00:05Z",
		buildReplayReceiptInput("mut-b", "Node", "item-1", "MoveNode", `{"tabId":"tab-1","position":{"top":"40px","left":"40px"}}`),
	)
	insertAcceptedReplayReceipt(t, docStore, "other-owner", "postbaby-web", "2026-06-01T12:00:03Z", "2026-06-01T12:00:03Z",
		buildReplayReceiptInput("mut-other-owner", "Node", "item-2", "CreateNode", `{"tabId":"tab-1","name":"Other","position":{"top":"50px","left":"50px"}}`),
	)
	insertAcceptedReplayReceipt(t, docStore, "owner", "other-app", "2026-06-01T12:00:04Z", "2026-06-01T12:00:04Z",
		buildReplayReceiptInput("mut-other-app", "Node", "item-3", "CreateNode", `{"tabId":"tab-1","name":"Elsewhere","position":{"top":"60px","left":"60px"}}`),
	)

	firstObservation, err := docStore.RecordSyncMutationReplayDryRunObservation(ctx, "owner", "postbaby-web")
	if err != nil {
		t.Fatalf("record first dry-run observation: %v", err)
	}
	secondObservation, err := docStore.RecordSyncMutationReplayDryRunObservation(ctx, "owner", "postbaby-web")
	if err != nil {
		t.Fatalf("record second dry-run observation: %v", err)
	}

	if firstObservation.ReceiptCountConsidered != 4 || firstObservation.AppliedCount != 4 || firstObservation.SkippedCount != 0 || firstObservation.WarningCount != 0 {
		t.Fatalf("unexpected first observation counts: %+v", firstObservation)
	}
	if firstObservation.FirstOrderedMutationID != "mut-accepted-first" || firstObservation.LastOrderedMutationID != "mut-b" {
		t.Fatalf("unexpected first observation ordering: %+v", firstObservation)
	}
	if firstObservation.OrderedReceiptHighWatermark != "2026-06-01T12:00:02Z|2026-06-01T12:00:05Z|mut-b" {
		t.Fatalf("unexpected first observation watermark: %q", firstObservation.OrderedReceiptHighWatermark)
	}
	if secondObservation.ReceiptCountConsidered != firstObservation.ReceiptCountConsidered ||
		secondObservation.FirstOrderedMutationID != firstObservation.FirstOrderedMutationID ||
		secondObservation.LastOrderedMutationID != firstObservation.LastOrderedMutationID ||
		secondObservation.OrderedReceiptHighWatermark != firstObservation.OrderedReceiptHighWatermark ||
		secondObservation.CanonicalDocumentHashObserved != firstObservation.CanonicalDocumentHashObserved ||
		secondObservation.PreviewHash != firstObservation.PreviewHash {
		t.Fatalf("expected stable repeated observations, first=%+v second=%+v", firstObservation, secondObservation)
	}

	if count := countReplayDryRunObservationsForTest(t, docStore, "owner", "postbaby-web"); count != 2 {
		t.Fatalf("expected two stored observations for owner/app, got %d", count)
	}
	if count := countReplayDryRunObservationsForTest(t, docStore, "other-owner", "postbaby-web"); count != 0 {
		t.Fatalf("expected no stored observations for other owner, got %d", count)
	}
}

func TestRecordSyncMutationReplayDryRunObservationChangesWhenReceiptsChange(t *testing.T) {
	t.Parallel()

	docStore := openTestStore(t)
	ctx := context.Background()
	seedReplayDocument(t, docStore, "owner", "postbaby-web", []replayTab{
		{
			ID:          "tab-1",
			Name:        "Main",
			Items:       []replayItem{},
			ColorIndex:  0,
			GridSetting: "none",
			Edges:       []replayEdge{},
		},
	})

	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:00Z", "2026-06-01T12:00:00Z",
		buildReplayReceiptInput("mut-create-1", "Node", "item-1", "CreateNode", `{"tabId":"tab-1","name":"First","color":"yellow","position":{"top":"10px","left":"20px"}}`),
	)

	firstObservation, err := docStore.RecordSyncMutationReplayDryRunObservation(ctx, "owner", "postbaby-web")
	if err != nil {
		t.Fatalf("record first observation: %v", err)
	}

	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:01Z", "2026-06-01T12:00:01Z",
		buildReplayReceiptInput("mut-create-2", "Node", "item-2", "CreateNode", `{"tabId":"tab-1","name":"Second","color":"blue","position":{"top":"30px","left":"40px"}}`),
	)

	secondObservation, err := docStore.RecordSyncMutationReplayDryRunObservation(ctx, "owner", "postbaby-web")
	if err != nil {
		t.Fatalf("record second observation: %v", err)
	}

	if secondObservation.CanonicalDocumentHashObserved != firstObservation.CanonicalDocumentHashObserved {
		t.Fatalf("expected canonical document hash to stay stable when only receipts change, first=%q second=%q", firstObservation.CanonicalDocumentHashObserved, secondObservation.CanonicalDocumentHashObserved)
	}
	if secondObservation.ReceiptCountConsidered != firstObservation.ReceiptCountConsidered+1 {
		t.Fatalf("expected receipt count to increase from %d to %d, got %d", firstObservation.ReceiptCountConsidered, firstObservation.ReceiptCountConsidered+1, secondObservation.ReceiptCountConsidered)
	}
	if secondObservation.LastOrderedMutationID != "mut-create-2" {
		t.Fatalf("expected last ordered mutation id mut-create-2, got %q", secondObservation.LastOrderedMutationID)
	}
	if secondObservation.OrderedReceiptHighWatermark != "2026-06-01T12:00:01Z|2026-06-01T12:00:01Z|mut-create-2" {
		t.Fatalf("unexpected second observation watermark: %q", secondObservation.OrderedReceiptHighWatermark)
	}
	if secondObservation.PreviewHash == firstObservation.PreviewHash {
		t.Fatalf("expected preview hash to change when accepted receipts change, hash=%q", secondObservation.PreviewHash)
	}
}

func TestRecordSyncMutationReplayDryRunObservationChangesWhenCanonicalSnapshotChanges(t *testing.T) {
	t.Parallel()

	docStore := openTestStore(t)
	ctx := context.Background()
	initial := seedReplayDocument(t, docStore, "owner", "postbaby-web", []replayTab{
		{
			ID:          "tab-1",
			Name:        "Main",
			Items:       []replayItem{},
			ColorIndex:  0,
			GridSetting: "none",
			Edges:       []replayEdge{},
		},
	})

	firstObservation, err := docStore.RecordSyncMutationReplayDryRunObservation(ctx, "owner", "postbaby-web")
	if err != nil {
		t.Fatalf("record first observation: %v", err)
	}

	updated, err := docStore.PutDocument(ctx, "owner", "postbaby-web", buildReplaySnapshotBody(t, []replayTab{
		{
			ID:   "tab-1",
			Name: "Main",
			Items: []replayItem{
				{ID: "item-1", Name: "First", Color: "yellow", Position: replayPosition{Top: "10px", Left: "20px"}},
			},
			ColorIndex:  0,
			GridSetting: "none",
			Edges:       []replayEdge{},
		},
	}), nil)
	if err != nil {
		t.Fatalf("update canonical document: %v", err)
	}
	if updated.Version <= initial.Version {
		t.Fatalf("expected canonical document version to advance from %d, got %d", initial.Version, updated.Version)
	}

	secondObservation, err := docStore.RecordSyncMutationReplayDryRunObservation(ctx, "owner", "postbaby-web")
	if err != nil {
		t.Fatalf("record second observation: %v", err)
	}

	if firstObservation.ReceiptCountConsidered != 0 || secondObservation.ReceiptCountConsidered != 0 {
		t.Fatalf("expected zero considered receipts, first=%+v second=%+v", firstObservation, secondObservation)
	}
	if secondObservation.CanonicalDocumentVersionObserved != updated.Version {
		t.Fatalf("expected observed version %d, got %d", updated.Version, secondObservation.CanonicalDocumentVersionObserved)
	}
	if secondObservation.CanonicalDocumentHashObserved == firstObservation.CanonicalDocumentHashObserved {
		t.Fatalf("expected canonical document hash to change when snapshot changes, hash=%q", secondObservation.CanonicalDocumentHashObserved)
	}
	if secondObservation.PreviewHash == firstObservation.PreviewHash {
		t.Fatalf("expected preview hash to change when snapshot changes, hash=%q", secondObservation.PreviewHash)
	}
}

func TestRecordSyncMutationReplayDryRunObservationMalformedLegacySnapshotIsStable(t *testing.T) {
	t.Parallel()

	docStore := openTestStore(t)
	ctx := context.Background()
	before := seedReplayDocumentBody(t, docStore, "owner", "postbaby-web", buildReplayRawSnapshotBody(t, `[
		{"id":"tab-1","name":"Main","colorIndex":0,"gridSetting":"none"},
		{"id":"tab-2","name":"Legacy","items":[
			{"id":"item-missing-position","name":"No Position"},
			{"id":"item-bad-position","name":"Bad Position","position":{"top":"bad","left":"20px"}},
			{"id":"item-unknown-shape","name":"Odd Shape","position":{"top":"30px","left":"40px"},"shape":"blob"}
		],"edges":[
			{"id":"edge-missing-from","toItemId":"item-bad-position","kind":"arrow"},
			{"id":"edge-missing-to","fromItemId":"item-unknown-shape","kind":"line"},
			{"id":"edge-missing-node","fromItemId":"item-unknown-shape","toItemId":"ghost","kind":"arrow"},
			{"id":"edge-self","fromItemId":"item-unknown-shape","toItemId":"item-unknown-shape","kind":"line"}
		]}
	]`))

	firstObservation, err := docStore.RecordSyncMutationReplayDryRunObservation(ctx, "owner", "postbaby-web")
	if err != nil {
		t.Fatalf("record first malformed legacy observation: %v", err)
	}
	secondObservation, err := docStore.RecordSyncMutationReplayDryRunObservation(ctx, "owner", "postbaby-web")
	if err != nil {
		t.Fatalf("record second malformed legacy observation: %v", err)
	}

	if firstObservation.ReceiptCountConsidered != 0 || secondObservation.ReceiptCountConsidered != 0 {
		t.Fatalf("expected zero considered receipts for malformed legacy snapshot, first=%+v second=%+v", firstObservation, secondObservation)
	}
	if secondObservation.CanonicalDocumentHashObserved != firstObservation.CanonicalDocumentHashObserved ||
		secondObservation.PreviewHash != firstObservation.PreviewHash {
		t.Fatalf("expected stable malformed legacy observation hashes, first=%+v second=%+v", firstObservation, secondObservation)
	}

	after, err := docStore.GetDocument(ctx, "owner", "postbaby-web")
	if err != nil {
		t.Fatalf("load malformed legacy document after observations: %v", err)
	}
	if after.Version != before.Version {
		t.Fatalf("expected malformed legacy observations to preserve version %d, got %d", before.Version, after.Version)
	}
	if string(after.Body) != string(before.Body) {
		t.Fatalf("expected malformed legacy observations to preserve body, before=%s after=%s", before.Body, after.Body)
	}
}

func TestReplaySyncMutationReceiptsDryRunDoesNotEnforcePerTabItemLimit(t *testing.T) {
	t.Parallel()

	docStore := openTestStore(t)
	ctx := context.Background()
	tab := replayTab{
		ID:          "tab-1",
		Name:        "Main",
		Items:       make([]replayItem, 0, 500),
		ColorIndex:  0,
		GridSetting: "none",
		Edges:       []replayEdge{},
	}
	for index := 0; index < 500; index += 1 {
		tab.Items = append(tab.Items, replayItem{
			ID:       fmt.Sprintf("seed-item-%d", index),
			Name:     fmt.Sprintf("Seed %d", index),
			Color:    "yellow",
			Position: replayPosition{Top: fmt.Sprintf("%dpx", index), Left: fmt.Sprintf("%dpx", index)},
		})
	}
	seedReplayDocument(t, docStore, "owner", "postbaby-web", []replayTab{tab})

	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:00Z", "2026-06-01T12:00:00Z",
		buildReplayReceiptInput("mut-over-limit-item", "Node", "item-over-limit", "CreateNode", `{"tabId":"tab-1","name":"Over Limit","color":"blue","position":{"top":"10px","left":"20px"}}`),
	)

	result, err := docStore.ReplaySyncMutationReceiptsDryRun(ctx, "owner", "postbaby-web")
	if err != nil {
		t.Fatalf("dry-run replay: %v", err)
	}
	if result.AppliedCount != 1 || result.SkippedCount != 0 || result.WarningCount != 0 {
		t.Fatalf("unexpected dry-run counts: %+v", result)
	}

	tabs := extractReplayPreviewTabs(t, result.PreviewBody)
	replayTab := findReplayTab(tabs, "tab-1")
	if replayTab == nil {
		t.Fatal("expected preview tab")
	}
	if len(replayTab.Items) != 501 {
		t.Fatalf("expected dry-run replay to exceed the frontend item limit, got %d items", len(replayTab.Items))
	}
}

func TestReplaySyncMutationReceiptsDryRunDoesNotEnforcePerTabEdgeLimit(t *testing.T) {
	t.Parallel()

	docStore := openTestStore(t)
	ctx := context.Background()
	tab := buildReplayTabWithEdgeLimitState()
	seedReplayDocument(t, docStore, "owner", "postbaby-web", []replayTab{tab})

	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:00Z", "2026-06-01T12:00:00Z",
		buildReplayReceiptInput("mut-over-limit-edge", "Edge", "edge-over-limit", "CreateEdge", `{"tabId":"tab-1","fromItemId":"seed-item-2000","toItemId":"seed-item-2001","kind":"line"}`),
	)

	result, err := docStore.ReplaySyncMutationReceiptsDryRun(ctx, "owner", "postbaby-web")
	if err != nil {
		t.Fatalf("dry-run replay: %v", err)
	}
	if result.AppliedCount != 1 || result.SkippedCount != 0 || result.WarningCount != 0 {
		t.Fatalf("unexpected dry-run counts: %+v", result)
	}

	tabs := extractReplayPreviewTabs(t, result.PreviewBody)
	replayTab := findReplayTab(tabs, "tab-1")
	if replayTab == nil {
		t.Fatal("expected preview tab")
	}
	if len(replayTab.Edges) != 2001 {
		t.Fatalf("expected dry-run replay to exceed the frontend edge limit, got %d edges", len(replayTab.Edges))
	}
}

func TestReplaySyncMutationReceiptsDryRunPreservesOversizeText(t *testing.T) {
	t.Parallel()

	docStore := openTestStore(t)
	ctx := context.Background()
	seedReplayDocument(t, docStore, "owner", "postbaby-web", []replayTab{
		{
			ID:   "tab-1",
			Name: "Main",
			Items: []replayItem{
				{ID: "item-1", Name: "First", Color: "yellow", Position: replayPosition{Top: "0px", Left: "0px"}},
			},
			ColorIndex:  0,
			GridSetting: "none",
			Edges:       []replayEdge{},
		},
	})

	oversizeText := strings.Repeat("x", 4001)
	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:00Z", "2026-06-01T12:00:00Z",
		buildReplayReceiptInput("mut-oversize-text", "Node", "item-1", "UpdateNode", `{"tabId":"tab-1","changes":{"name":"`+oversizeText+`"}}`),
	)

	result, err := docStore.ReplaySyncMutationReceiptsDryRun(ctx, "owner", "postbaby-web")
	if err != nil {
		t.Fatalf("dry-run replay: %v", err)
	}
	if result.AppliedCount != 1 || result.WarningCount != 0 {
		t.Fatalf("expected oversize text to be preserved by dry-run replay, got %+v", result)
	}

	tabs := extractReplayPreviewTabs(t, result.PreviewBody)
	replayTab := findReplayTab(tabs, "tab-1")
	if replayTab == nil {
		t.Fatal("expected preview tab")
	}
	item := findReplayItem(replayTab, "item-1")
	if item == nil {
		t.Fatal("expected preview item")
	}
	if len(item.Name) != len(oversizeText) {
		t.Fatalf("expected oversize text length %d, got %d", len(oversizeText), len(item.Name))
	}
}

func TestReplaySyncMutationReceiptsDryRunDoesNotDeriveFixedRatioShapeHeight(t *testing.T) {
	t.Parallel()

	docStore := openTestStore(t)
	ctx := context.Background()
	initialWidth := 170.0
	initialHeight := 170.0
	seedReplayDocument(t, docStore, "owner", "postbaby-web", []replayTab{
		{
			ID:   "tab-1",
			Name: "Main",
			Items: []replayItem{
				{
					ID:       "item-1",
					Name:     "Circle",
					Color:    "yellow",
					Position: replayPosition{Top: "0px", Left: "0px"},
					Shape:    "circle",
					Width:    &initialWidth,
					Height:   &initialHeight,
				},
			},
			ColorIndex:  0,
			GridSetting: "none",
			Edges:       []replayEdge{},
		},
	})

	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:00Z", "2026-06-01T12:00:00Z",
		buildReplayReceiptInput("mut-circle-width-only", "Node", "item-1", "UpdateNode", `{"tabId":"tab-1","changes":{"width":220}}`),
	)

	result, err := docStore.ReplaySyncMutationReceiptsDryRun(ctx, "owner", "postbaby-web")
	if err != nil {
		t.Fatalf("dry-run replay: %v", err)
	}
	if result.AppliedCount != 1 || result.WarningCount != 0 {
		t.Fatalf("unexpected dry-run counts: %+v", result)
	}

	tabs := extractReplayPreviewTabs(t, result.PreviewBody)
	replayTab := findReplayTab(tabs, "tab-1")
	if replayTab == nil {
		t.Fatal("expected preview tab")
	}
	item := findReplayItem(replayTab, "item-1")
	if item == nil || item.Width == nil || item.Height == nil {
		t.Fatalf("expected preview item with stored dimensions, got %+v", item)
	}
	if *item.Width != 220 {
		t.Fatalf("expected updated width 220, got %v", *item.Width)
	}
	if *item.Height != 170 {
		t.Fatalf("expected dry-run replay to preserve stored height 170 instead of deriving a fixed-ratio height, got %v", *item.Height)
	}
}

func TestEvaluateSyncMutationReplayAuthoritativePolicyCreateNodeOverItemLimit(t *testing.T) {
	t.Parallel()

	tab := replayTab{
		ID:          "tab-1",
		Name:        "Main",
		Items:       make([]replayItem, 0, 500),
		ColorIndex:  0,
		GridSetting: "none",
		Edges:       []replayEdge{},
	}
	for index := 0; index < 500; index += 1 {
		tab.Items = append(tab.Items, replayItem{
			ID:       fmt.Sprintf("seed-item-%d", index),
			Name:     fmt.Sprintf("Seed %d", index),
			Color:    "yellow",
			Position: replayPosition{Top: fmt.Sprintf("%dpx", index), Left: fmt.Sprintf("%dpx", index)},
		})
	}

	body := buildReplaySnapshotBody(t, []replayTab{tab})
	evaluation, err := EvaluateSyncMutationReplayAuthoritativePolicy(body, SyncMutationReceipt{
		EntityID:      "item-over-limit",
		OperationType: "CreateNode",
		Payload:       json.RawMessage(`{"tabId":"tab-1","name":"Over Limit","position":{"top":"10px","left":"20px"}}`),
	})
	if err != nil {
		t.Fatalf("evaluate authoritative policy: %v", err)
	}
	assertReplayPolicyStatus(t, evaluation, replayAuthoritativePolicyStatusSkip)
	assertReplayPolicyReasons(t, evaluation.Reasons, []string{"item_limit_exceeded"})
}

func TestEvaluateSyncMutationReplayAuthoritativePolicyCreateEdgeOverEdgeLimit(t *testing.T) {
	t.Parallel()

	body := buildReplaySnapshotBody(t, []replayTab{buildReplayTabWithEdgeLimitState()})
	evaluation, err := EvaluateSyncMutationReplayAuthoritativePolicy(body, SyncMutationReceipt{
		EntityID:      "edge-over-limit",
		OperationType: "CreateEdge",
		Payload:       json.RawMessage(`{"tabId":"tab-1","fromItemId":"seed-item-2000","toItemId":"seed-item-2001","kind":"line"}`),
	})
	if err != nil {
		t.Fatalf("evaluate authoritative policy: %v", err)
	}
	assertReplayPolicyStatus(t, evaluation, replayAuthoritativePolicyStatusSkip)
	assertReplayPolicyReasons(t, evaluation.Reasons, []string{"edge_limit_exceeded"})
}

func TestEvaluateSyncMutationReplayAuthoritativePolicyOversizeText(t *testing.T) {
	t.Parallel()

	body := buildReplaySnapshotBody(t, []replayTab{
		{
			ID:   "tab-1",
			Name: "Main",
			Items: []replayItem{
				{ID: "item-1", Name: "First", Color: "yellow", Position: replayPosition{Top: "0px", Left: "0px"}},
			},
			ColorIndex:  0,
			GridSetting: "none",
			Edges:       []replayEdge{},
		},
	})
	oversizeText := strings.Repeat("x", 4001)
	evaluation, err := EvaluateSyncMutationReplayAuthoritativePolicy(body, SyncMutationReceipt{
		EntityID:      "item-1",
		OperationType: "UpdateNode",
		Payload:       json.RawMessage(`{"tabId":"tab-1","changes":{"name":"` + oversizeText + `"}}`),
	})
	if err != nil {
		t.Fatalf("evaluate authoritative policy: %v", err)
	}
	assertReplayPolicyStatus(t, evaluation, replayAuthoritativePolicyStatusSkip)
	assertReplayPolicyReasons(t, evaluation.Reasons, []string{"text_limit_exceeded"})
}

func TestEvaluateSyncMutationReplayAuthoritativePolicyFixedRatioWidthOnlyUpdate(t *testing.T) {
	t.Parallel()

	initialWidth := 170.0
	initialHeight := 170.0
	body := buildReplaySnapshotBody(t, []replayTab{
		{
			ID:   "tab-1",
			Name: "Main",
			Items: []replayItem{
				{
					ID:       "item-1",
					Name:     "Circle",
					Color:    "yellow",
					Position: replayPosition{Top: "0px", Left: "0px"},
					Shape:    "circle",
					Width:    &initialWidth,
					Height:   &initialHeight,
				},
			},
			ColorIndex:  0,
			GridSetting: "none",
			Edges:       []replayEdge{},
		},
	})
	evaluation, err := EvaluateSyncMutationReplayAuthoritativePolicy(body, SyncMutationReceipt{
		EntityID:      "item-1",
		OperationType: "UpdateNode",
		Payload:       json.RawMessage(`{"tabId":"tab-1","changes":{"width":220}}`),
	})
	if err != nil {
		t.Fatalf("evaluate authoritative policy: %v", err)
	}
	assertReplayPolicyStatus(t, evaluation, replayAuthoritativePolicyStatusAllowed)
	assertReplayWarnings(t, evaluation.Warnings, []string{"preserve_literal_fixed_ratio_dimensions"})
}

func TestEvaluateSyncMutationReplayAuthoritativePolicyPreservesMalformedLegacyWarnings(t *testing.T) {
	t.Parallel()

	body := buildReplayRawSnapshotBody(t, `[
		{
			"id":"tab-1",
			"name":"Legacy",
			"items":[
				{"id":"item-1","name":"Odd","position":{"top":"bad","left":"20px"},"shape":"blob"},
				{"id":"item-2","name":"Second","position":{"top":"0px","left":"0px"}}
			],
			"edges":[
				{"id":"edge-orphan","fromItemId":"item-1","toItemId":"ghost","kind":"line"}
			]
		}
	]`)
	evaluation, err := EvaluateSyncMutationReplayAuthoritativePolicy(body, SyncMutationReceipt{
		EntityID:      "item-2",
		OperationType: "MoveNode",
		Payload:       json.RawMessage(`{"tabId":"tab-1","position":{"top":"10px","left":"20px"}}`),
	})
	if err != nil {
		t.Fatalf("evaluate authoritative policy: %v", err)
	}
	assertReplayPolicyStatus(t, evaluation, replayAuthoritativePolicyStatusAllowed)
	assertReplayWarnings(t, evaluation.Warnings, []string{
		"preserve_malformed_legacy_positions",
		"preserve_unknown_legacy_shapes",
		"preserve_malformed_legacy_edges",
	})
}

func TestEvaluateSyncMutationReplayAuthoritativePolicyBlocksDuplicateItemIDs(t *testing.T) {
	t.Parallel()

	body := buildReplayRawSnapshotBody(t, `[
		{
			"id":"tab-1",
			"name":"Legacy",
			"items":[
				{"id":"dup-item","name":"First","position":{"top":"0px","left":"0px"}},
				{"id":"dup-item","name":"Second","position":{"top":"10px","left":"10px"}}
			],
			"edges":[]
		}
	]`)
	evaluation, err := EvaluateSyncMutationReplayAuthoritativePolicy(body, SyncMutationReceipt{
		EntityID:      "item-1",
		OperationType: "CreateNode",
		Payload:       json.RawMessage(`{"tabId":"tab-1","name":"First","position":{"top":"10px","left":"20px"}}`),
	})
	if err != nil {
		t.Fatalf("evaluate authoritative policy: %v", err)
	}
	assertReplayPolicyStatus(t, evaluation, replayAuthoritativePolicyStatusBlocked)
	assertReplayPolicyReasons(t, evaluation.Reasons, []string{"snapshot_contains_duplicate_item_ids"})
}

func TestEvaluateSyncMutationReplayAuthoritativePolicyBlocksDuplicateEdgeIDs(t *testing.T) {
	t.Parallel()

	body := buildReplayRawSnapshotBody(t, `[
		{
			"id":"tab-1",
			"name":"Legacy",
			"items":[
				{"id":"item-1","name":"First","position":{"top":"0px","left":"0px"}},
				{"id":"item-2","name":"Second","position":{"top":"10px","left":"10px"}}
			],
			"edges":[
				{"id":"dup-edge","fromItemId":"item-1","toItemId":"item-2","kind":"line"},
				{"id":"dup-edge","fromItemId":"item-2","toItemId":"item-1","kind":"line"}
			]
		}
	]`)
	evaluation, err := EvaluateSyncMutationReplayAuthoritativePolicy(body, SyncMutationReceipt{
		EntityID:      "edge-1",
		OperationType: "DeleteEdge",
		Payload:       json.RawMessage(`{"tabId":"tab-1"}`),
	})
	if err != nil {
		t.Fatalf("evaluate authoritative policy: %v", err)
	}
	assertReplayPolicyStatus(t, evaluation, replayAuthoritativePolicyStatusBlocked)
	assertReplayPolicyReasons(t, evaluation.Reasons, []string{"snapshot_contains_duplicate_edge_ids"})
}

func TestEvaluateSyncMutationReplayAuthoritativeReadinessBlockedByContractGaps(t *testing.T) {
	t.Parallel()

	docStore := openTestStore(t)
	ctx := context.Background()
	seedReplayDocument(t, docStore, "owner", "postbaby-web", []replayTab{
		{
			ID:          "tab-1",
			Name:        "Main",
			Items:       []replayItem{},
			ColorIndex:  0,
			GridSetting: "none",
			Edges:       []replayEdge{},
		},
	})

	insertAcceptedReplayReceipt(t, docStore, "owner", "postbaby-web", "2026-06-01T12:00:00Z", "2026-06-01T12:00:00Z",
		buildReplayReceiptInput("mut-create", "Node", "item-1", "CreateNode", `{"tabId":"tab-1","name":"First","color":"yellow","position":{"top":"10px","left":"20px"}}`),
	)

	observation, err := docStore.RecordSyncMutationReplayDryRunObservation(ctx, "owner", "postbaby-web")
	if err != nil {
		t.Fatalf("record observation: %v", err)
	}
	currentDoc, err := docStore.GetDocument(ctx, "owner", "postbaby-web")
	if err != nil {
		t.Fatalf("load current document: %v", err)
	}
	currentReceipts := loadAcceptedReplayReceiptsForTest(t, docStore, "owner", "postbaby-web")

	readiness, err := EvaluateSyncMutationReplayAuthoritativeReadiness(observation, currentDoc, currentReceipts)
	if err != nil {
		t.Fatalf("evaluate readiness: %v", err)
	}
	if readiness.Ready {
		t.Fatalf("expected authoritative replay readiness to stay blocked, got %+v", readiness)
	}
	assertReplayReadinessBlockers(t, readiness.Blockers, []string{
		"document_version_gating",
		"transaction_boundary",
		"receipt_status_model",
		"replay_progress_state",
		"rollback_behavior",
		"conflict_behavior",
		"applied_receipt_tracking",
	})
	assertReplayAreaStatus(t, readiness.Areas, "duplicate_endpoint_edge_policy", replayAuthoritativeStatusReady)
	assertReplayAreaStatus(t, readiness.Areas, "missing_tab_policy", replayAuthoritativeStatusReady)
	assertReplayAreaStatus(t, readiness.Areas, "replay_ordering", replayAuthoritativeStatusPartiallyReady)
	assertReplayAreaStatus(t, readiness.Areas, "per_tab_item_limit", replayAuthoritativeStatusPartiallyReady)
	assertReplayAreaStatus(t, readiness.Areas, "per_tab_edge_limit", replayAuthoritativeStatusPartiallyReady)
	assertReplayAreaStatus(t, readiness.Areas, "text_length_limit_behavior", replayAuthoritativeStatusPartiallyReady)
	assertReplayAreaStatus(t, readiness.Areas, "shape_specific_sizing_behavior", replayAuthoritativeStatusPartiallyReady)
	assertReplayAreaStatus(t, readiness.Areas, "duplicate_item_id_policy", replayAuthoritativeStatusPartiallyReady)
	assertReplayAreaStatus(t, readiness.Areas, "duplicate_edge_id_policy", replayAuthoritativeStatusPartiallyReady)
	assertReplayAreaStatus(t, readiness.Areas, "server_side_validation_requirements", replayAuthoritativeStatusPartiallyReady)
	assertReplayAreaStatus(t, readiness.Areas, "delta_pull", replayAuthoritativeStatusIntentionallyDeferred)
}

func TestEvaluateSyncMutationReplayAuthoritativeReadinessBlocksChangedCanonicalDocument(t *testing.T) {
	t.Parallel()

	docStore := openTestStore(t)
	ctx := context.Background()
	seedReplayDocument(t, docStore, "owner", "postbaby-web", []replayTab{
		{
			ID:          "tab-1",
			Name:        "Main",
			Items:       []replayItem{},
			ColorIndex:  0,
			GridSetting: "none",
			Edges:       []replayEdge{},
		},
	})

	observation, err := docStore.RecordSyncMutationReplayDryRunObservation(ctx, "owner", "postbaby-web")
	if err != nil {
		t.Fatalf("record observation: %v", err)
	}

	_, err = docStore.PutDocument(ctx, "owner", "postbaby-web", buildReplaySnapshotBody(t, []replayTab{
		{
			ID:   "tab-1",
			Name: "Main",
			Items: []replayItem{
				{ID: "item-1", Name: "Changed", Color: "yellow", Position: replayPosition{Top: "0px", Left: "0px"}},
			},
			ColorIndex:  0,
			GridSetting: "none",
			Edges:       []replayEdge{},
		},
	}), nil)
	if err != nil {
		t.Fatalf("change canonical document: %v", err)
	}

	currentDoc, err := docStore.GetDocument(ctx, "owner", "postbaby-web")
	if err != nil {
		t.Fatalf("load current document: %v", err)
	}
	currentReceipts := loadAcceptedReplayReceiptsForTest(t, docStore, "owner", "postbaby-web")

	readiness, err := EvaluateSyncMutationReplayAuthoritativeReadiness(observation, currentDoc, currentReceipts)
	if err != nil {
		t.Fatalf("evaluate readiness: %v", err)
	}
	assertReplayReadinessBlockers(t, readiness.Blockers, []string{"canonical_snapshot_changed_since_observation"})
}

func TestEvaluateSyncMutationReplayAuthoritativeReadinessWarnsOnMalformedLegacyAndDuplicateIDs(t *testing.T) {
	t.Parallel()

	docStore := openTestStore(t)
	ctx := context.Background()
	seedReplayDocumentBody(t, docStore, "owner", "postbaby-web", buildReplayRawSnapshotBody(t, `[
		{
			"id":"tab-1",
			"name":"Legacy",
			"items":[
				{"id":"dup-item","name":"First","position":{"top":"0px","left":"0px"}},
				{"id":"dup-item","name":"Second","position":{"top":"10px","left":"10px"}}
			],
			"edges":[
				{"id":"dup-edge","fromItemId":"dup-item","toItemId":"ghost","kind":"line"},
				{"id":"dup-edge","fromItemId":"dup-item","toItemId":"dup-item","kind":"arrow"}
			]
		}
	]`))

	observation, err := docStore.RecordSyncMutationReplayDryRunObservation(ctx, "owner", "postbaby-web")
	if err != nil {
		t.Fatalf("record observation: %v", err)
	}
	currentDoc, err := docStore.GetDocument(ctx, "owner", "postbaby-web")
	if err != nil {
		t.Fatalf("load current document: %v", err)
	}
	currentReceipts := loadAcceptedReplayReceiptsForTest(t, docStore, "owner", "postbaby-web")

	readiness, err := EvaluateSyncMutationReplayAuthoritativeReadiness(observation, currentDoc, currentReceipts)
	if err != nil {
		t.Fatalf("evaluate readiness: %v", err)
	}
	assertReplayReadinessBlockers(t, readiness.Blockers, []string{
		"snapshot_contains_duplicate_item_ids",
		"snapshot_contains_duplicate_edge_ids",
	})
	assertReplayWarnings(t, readiness.Warnings, []string{"preserve_malformed_legacy_edges"})
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

func buildReplayReceiptInput(mutationID, entityType, entityID, operationType, payload string) SyncMutationReceiptInput {
	baseRevision := int64(6)
	return SyncMutationReceiptInput{
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

func seedReplayDocumentBody(t *testing.T, docStore *Store, ownerKey, appID string, body json.RawMessage) Document {
	t.Helper()

	doc, err := docStore.PutDocument(context.Background(), ownerKey, appID, body, nil)
	if err != nil {
		t.Fatalf("seed replay document body: %v", err)
	}
	return doc
}

func seedReplayDocument(t *testing.T, docStore *Store, ownerKey, appID string, tabs []replayTab) Document {
	t.Helper()

	doc, err := docStore.PutDocument(context.Background(), ownerKey, appID, buildReplaySnapshotBody(t, tabs), nil)
	if err != nil {
		t.Fatalf("seed replay document: %v", err)
	}
	return doc
}

func buildReplaySnapshotBody(t *testing.T, tabs []replayTab) json.RawMessage {
	t.Helper()

	tabsJSON, err := json.Marshal(tabs)
	if err != nil {
		t.Fatalf("marshal replay tabs: %v", err)
	}

	body, err := json.Marshal(map[string]string{
		"tabs":        string(tabsJSON),
		"activeTabId": "tab-1",
	})
	if err != nil {
		t.Fatalf("marshal replay snapshot body: %v", err)
	}
	return json.RawMessage(body)
}

func buildReplayRawSnapshotBody(t *testing.T, tabsJSON string) json.RawMessage {
	t.Helper()

	body, err := json.Marshal(map[string]string{
		"tabs":        tabsJSON,
		"activeTabId": "tab-1",
	})
	if err != nil {
		t.Fatalf("marshal raw replay snapshot body: %v", err)
	}
	return json.RawMessage(body)
}

func extractReplayPreviewTabs(t *testing.T, previewBody json.RawMessage) []replayTab {
	t.Helper()

	snapshot, err := decodeReplaySnapshot(previewBody)
	if err != nil {
		t.Fatalf("decode preview snapshot: %v", err)
	}

	tabs, err := decodeReplayTabs(snapshot[replayTabsSnapshotKey])
	if err != nil {
		t.Fatalf("decode preview tabs: %v", err)
	}
	return tabs
}

func insertAcceptedReplayReceipt(t *testing.T, docStore *Store, ownerKey, appID, acceptedAt, createdAt string, input SyncMutationReceiptInput) {
	t.Helper()

	var baseRevisionValue any
	if input.BaseRevision != nil {
		baseRevisionValue = *input.BaseRevision
	}

	_, err := docStore.db.ExecContext(
		context.Background(),
		`INSERT INTO sync_mutation_receipts (
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
		input.MutationID,
		input.ClientID,
		input.DeviceID,
		input.Protocol,
		input.EntityType,
		input.EntityID,
		input.OperationType,
		string(input.Payload),
		baseRevisionValue,
		SyncMutationReceiptStatusAccepted,
		createdAt,
		acceptedAt,
	)
	if err != nil {
		t.Fatalf("insert accepted replay receipt: %v", err)
	}
}

func countReplayItemsByID(tab *replayTab, itemID string) int {
	count := 0
	for _, item := range tab.Items {
		if item.ID == itemID {
			count++
		}
	}
	return count
}

func buildReplayTabWithEdgeLimitState() replayTab {
	tab := replayTab{
		ID:          "tab-1",
		Name:        "Main",
		Items:       make([]replayItem, 0, 2002),
		ColorIndex:  0,
		GridSetting: "none",
		Edges:       make([]replayEdge, 0, 2000),
	}

	for index := 0; index <= 2001; index += 1 {
		tab.Items = append(tab.Items, replayItem{
			ID:       fmt.Sprintf("seed-item-%d", index),
			Name:     fmt.Sprintf("Seed %d", index),
			Color:    "yellow",
			Position: replayPosition{Top: fmt.Sprintf("%dpx", index), Left: fmt.Sprintf("%dpx", index)},
		})
	}

	for index := 0; index < 2000; index += 1 {
		tab.Edges = append(tab.Edges, replayEdge{
			ID:         fmt.Sprintf("seed-edge-%d", index),
			FromItemID: fmt.Sprintf("seed-item-%d", index),
			ToItemID:   fmt.Sprintf("seed-item-%d", index+1),
			Kind:       "line",
		})
	}

	return tab
}

func countReplayEdgesByID(tab *replayTab, edgeID string) int {
	count := 0
	for _, edge := range tab.Edges {
		if edge.ID == edgeID {
			count++
		}
	}
	return count
}

func assertReplayWarningCodes(t *testing.T, warnings []SyncMutationDryRunWarning, expectedCodes []string) {
	t.Helper()

	if len(warnings) != len(expectedCodes) {
		t.Fatalf("expected %d warnings, got %d: %+v", len(expectedCodes), len(warnings), warnings)
	}

	remaining := make(map[string]int, len(expectedCodes))
	for _, code := range expectedCodes {
		remaining[code]++
	}

	for _, warning := range warnings {
		remaining[warning.Code]--
	}

	for code, count := range remaining {
		if count != 0 {
			t.Fatalf("expected warning code counts %v, got mismatch for %q in %+v", expectedCodes, code, warnings)
		}
	}
}

func assertReplayReadinessBlockers(t *testing.T, actual, expected []string) {
	t.Helper()

	for _, blocker := range expected {
		found := false
		for _, actualBlocker := range actual {
			if actualBlocker == blocker {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected blocker %q in %+v", blocker, actual)
		}
	}
}

func assertReplayPolicyStatus(t *testing.T, evaluation SyncMutationReplayAuthoritativePolicyEvaluation, expectedStatus string) {
	t.Helper()

	if evaluation.Status != expectedStatus {
		t.Fatalf("expected policy status %q, got %+v", expectedStatus, evaluation)
	}
}

func assertReplayPolicyReasons(t *testing.T, actual, expected []string) {
	t.Helper()

	if len(actual) != len(expected) {
		t.Fatalf("expected %d policy reasons, got %d: %+v", len(expected), len(actual), actual)
	}

	remaining := make(map[string]int, len(expected))
	for _, reason := range expected {
		remaining[reason]++
	}
	for _, reason := range actual {
		remaining[reason]--
	}
	for reason, count := range remaining {
		if count != 0 {
			t.Fatalf("expected policy reasons %v, got mismatch for %q in %+v", expected, reason, actual)
		}
	}
}

func assertReplayAreaStatus(t *testing.T, areas []SyncMutationReplayAuthoritativeAreaReadiness, area, expectedStatus string) {
	t.Helper()

	for _, areaReadiness := range areas {
		if areaReadiness.Area == area {
			if areaReadiness.Status != expectedStatus {
				t.Fatalf("expected area %q to have status %q, got %+v", area, expectedStatus, areaReadiness)
			}
			return
		}
	}

	t.Fatalf("expected readiness area %q in %+v", area, areas)
}

func assertReplayWarnings(t *testing.T, actual, expected []string) {
	t.Helper()

	if len(actual) != len(expected) {
		t.Fatalf("expected %d readiness warnings, got %d: %+v", len(expected), len(actual), actual)
	}

	remaining := make(map[string]int, len(expected))
	for _, warning := range expected {
		remaining[warning]++
	}
	for _, warning := range actual {
		remaining[warning]--
	}
	for warning, count := range remaining {
		if count != 0 {
			t.Fatalf("expected readiness warnings %v, got mismatch for %q in %+v", expected, warning, actual)
		}
	}
}

func loadReplayDryRunObservationForTest(t *testing.T, docStore *Store, observationID int64) SyncMutationReplayDryRunObservation {
	t.Helper()

	var observation SyncMutationReplayDryRunObservation
	var createdAt string
	err := docStore.db.QueryRowContext(
		context.Background(),
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
	).Scan(
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
	)
	if err != nil {
		t.Fatalf("load replay dry-run observation %d: %v", observationID, err)
	}

	observation.CreatedAt = mustParseTimestamp(createdAt)
	return observation
}

func countReplayDryRunObservationsForTest(t *testing.T, docStore *Store, ownerKey, appID string) int64 {
	t.Helper()

	var count int64
	err := docStore.db.QueryRowContext(
		context.Background(),
		`SELECT COUNT(*) FROM sync_mutation_replay_dry_run_observations WHERE owner_key = ? AND app_id = ?`,
		ownerKey,
		appID,
	).Scan(&count)
	if err != nil {
		t.Fatalf("count replay dry-run observations: %v", err)
	}
	return count
}

func loadAllReplayReceiptRowsForTest(t *testing.T, docStore *Store, ownerKey, appID string) []SyncMutationReceipt {
	t.Helper()

	rows, err := docStore.db.QueryContext(
		context.Background(),
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
		WHERE owner_key = ? AND app_id = ?
		ORDER BY id ASC`,
		ownerKey,
		appID,
	)
	if err != nil {
		t.Fatalf("query replay receipt rows: %v", err)
	}
	defer rows.Close()

	receipts := make([]SyncMutationReceipt, 0)
	for rows.Next() {
		receipt, scanErr := scanSyncMutationReceipt(rows)
		if scanErr != nil {
			t.Fatalf("scan replay receipt row: %v", scanErr)
		}
		receipts = append(receipts, receipt)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate replay receipt rows: %v", err)
	}
	return receipts
}

func loadAcceptedReplayReceiptsForTest(t *testing.T, docStore *Store, ownerKey, appID string) []SyncMutationReceipt {
	t.Helper()

	receipts, err := docStore.listAcceptedSyncMutationReceipts(context.Background(), ownerKey, appID)
	if err != nil {
		t.Fatalf("load accepted replay receipts: %v", err)
	}
	return receipts
}

func assertReplayReceiptRowsEqual(t *testing.T, expected, actual []SyncMutationReceipt) {
	t.Helper()

	if len(actual) != len(expected) {
		t.Fatalf("expected %d replay receipt rows, got %d", len(expected), len(actual))
	}

	for index := range expected {
		if actual[index].ID != expected[index].ID ||
			actual[index].OwnerKey != expected[index].OwnerKey ||
			actual[index].AppID != expected[index].AppID ||
			actual[index].MutationID != expected[index].MutationID ||
			actual[index].ClientID != expected[index].ClientID ||
			actual[index].DeviceID != expected[index].DeviceID ||
			actual[index].Protocol != expected[index].Protocol ||
			actual[index].EntityType != expected[index].EntityType ||
			actual[index].EntityID != expected[index].EntityID ||
			actual[index].OperationType != expected[index].OperationType ||
			string(actual[index].Payload) != string(expected[index].Payload) ||
			!equalInt64Pointers(actual[index].BaseRevision, expected[index].BaseRevision) ||
			actual[index].Status != expected[index].Status ||
			!actual[index].CreatedAt.Equal(expected[index].CreatedAt) ||
			!actual[index].AcceptedAt.Equal(expected[index].AcceptedAt) {
			t.Fatalf("replay receipt row %d mismatch: expected=%+v actual=%+v", index, expected[index], actual[index])
		}
	}
}

func equalInt64Pointers(left, right *int64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
