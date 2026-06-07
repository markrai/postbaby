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
