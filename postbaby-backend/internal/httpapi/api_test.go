package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"postbaby-backend/internal/auth"
	"postbaby-backend/internal/config"
	"postbaby-backend/internal/entitlement"
	"postbaby-backend/internal/store"

	_ "modernc.org/sqlite"
)

const testPassword = "correct-horse-battery"

func TestHealthReturnsOK(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t, config.DeploymentModeSelfHosted)
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp struct {
		OK bool `json:"ok"`
	}
	decodeResponse(t, rec, &resp)
	if !resp.OK {
		t.Fatal("expected ok response")
	}
}

func TestHealthReturnsOKInCloudMultiUserMode(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t, config.DeploymentModeCloud)
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp struct {
		OK bool `json:"ok"`
	}
	decodeResponse(t, rec, &resp)
	if !resp.OK {
		t.Fatal("expected ok response")
	}
}

func TestDocumentRequiresAuthentication(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t, config.DeploymentModeSelfHosted)
	req := httptest.NewRequest(http.MethodGet, "/api/document?appId=demo", nil)
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	assertErrorResponse(t, rec, http.StatusUnauthorized, "unauthorized")
	assertNoStore(t, rec)
}

func TestDocumentRejectsInvalidSessionCookie(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t, config.DeploymentModeSelfHosted)
	req := httptest.NewRequest(http.MethodGet, "/api/document?appId=demo", nil)
	req.AddCookie(&http.Cookie{Name: "postbaby_session", Value: "bogus"})
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	assertErrorResponse(t, rec, http.StatusUnauthorized, "unauthorized")
}

func TestDocumentMetaRequiresAuthentication(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t, config.DeploymentModeSelfHosted)
	req := httptest.NewRequest(http.MethodGet, "/api/document/meta?appId=demo", nil)
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	assertErrorResponse(t, rec, http.StatusUnauthorized, "unauthorized")
	assertNoStore(t, rec)
}

func TestDocumentMetaRequiresAppID(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnv(t, config.DeploymentModeSelfHosted)
	req := httptest.NewRequest(http.MethodGet, "/api/document/meta", nil)
	req.AddCookie(env.cookie)
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	assertErrorResponse(t, rec, http.StatusBadRequest, "invalid_request")
}

func TestDocumentMetaRejectsWrongMethod(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnv(t, config.DeploymentModeSelfHosted)
	req := httptest.NewRequest(http.MethodPut, "/api/document/meta?appId=demo", nil)
	req.AddCookie(env.cookie)
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	assertErrorResponse(t, rec, http.StatusMethodNotAllowed, "method_not_allowed")
	if allow := rec.Header().Get("Allow"); allow != http.MethodGet {
		t.Fatalf("expected Allow header %q, got %q", http.MethodGet, allow)
	}
}

func TestDocumentMetaReturnsExistsFalseWhenMissing(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnv(t, config.DeploymentModeSelfHosted)
	req := httptest.NewRequest(http.MethodGet, "/api/document/meta?appId=demo", nil)
	req.AddCookie(env.cookie)
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp struct {
		OK     bool   `json:"ok"`
		AppID  string `json:"appId"`
		Exists bool   `json:"exists"`
	}
	decodeResponse(t, rec, &resp)

	if !resp.OK || resp.AppID != "demo" || resp.Exists {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestDocumentMetaReturnsDocumentMetadataAfterSave(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnv(t, config.DeploymentModeSelfHosted)
	performJSONRequest(t, env.handler, env.cookie, http.MethodPut, "/api/document", map[string]any{
		"appId": "demo",
		"data":  snapshot("tab-1", "[]"),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/document/meta?appId=demo", nil)
	req.AddCookie(env.cookie)
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp struct {
		OK        bool   `json:"ok"`
		AppID     string `json:"appId"`
		Exists    bool   `json:"exists"`
		Version   int64  `json:"version"`
		UpdatedAt string `json:"updatedAt"`
	}
	decodeResponse(t, rec, &resp)

	if !resp.OK || resp.AppID != "demo" || !resp.Exists || resp.Version != 1 || resp.UpdatedAt == "" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	assertNoStore(t, rec)
}

func TestGetDocumentRequiresAppID(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnv(t, config.DeploymentModeSelfHosted)
	req := httptest.NewRequest(http.MethodGet, "/api/document", nil)
	req.AddCookie(env.cookie)
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	assertErrorResponse(t, rec, http.StatusBadRequest, "invalid_request")
}

func TestPutDocumentRejectsInvalidJSON(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnv(t, config.DeploymentModeSelfHosted)
	req := httptest.NewRequest(http.MethodPut, "/api/document", strings.NewReader(`{"appId":"demo","data":`))
	req.AddCookie(env.cookie)
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	assertErrorResponse(t, rec, http.StatusBadRequest, "invalid_json")
}

func TestPutDocumentRejectsOversizedBody(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnv(t, config.DeploymentModeSelfHosted)
	oversized := `{"appId":"demo","data":"` + strings.Repeat("a", int(MaxDocumentBodyBytes)) + `"}`
	req := httptest.NewRequest(http.MethodPut, "/api/document", strings.NewReader(oversized))
	req.AddCookie(env.cookie)
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	assertErrorResponse(t, rec, http.StatusRequestEntityTooLarge, "request_too_large")
}

func TestPutDocumentRejectsNonFrontendSnapshotData(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnv(t, config.DeploymentModeSelfHosted)
	rec := performJSONRequest(t, env.handler, env.cookie, http.MethodPut, "/api/document", map[string]any{
		"appId": "demo",
		"data": map[string]any{
			"tabs": []any{
				map[string]any{"id": "tab-1"},
			},
		},
	})

	assertErrorResponse(t, rec, http.StatusBadRequest, "invalid_request")
}

func TestGetDocumentReturnsNotFound(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnv(t, config.DeploymentModeSelfHosted)
	req := httptest.NewRequest(http.MethodGet, "/api/document?appId=missing", nil)
	req.AddCookie(env.cookie)
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	assertErrorResponse(t, rec, http.StatusNotFound, "document_not_found")
}

func TestPutDocumentCreatesDocument(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnv(t, config.DeploymentModeSelfHosted)
	rec := performJSONRequest(t, env.handler, env.cookie, http.MethodPut, "/api/document", map[string]any{
		"appId": "demo",
		"data":  snapshot("tab-1", "[]"),
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp struct {
		OK      bool   `json:"ok"`
		AppID   string `json:"appId"`
		Version int64  `json:"version"`
	}
	decodeResponse(t, rec, &resp)
	if !resp.OK || resp.AppID != "demo" || resp.Version != 1 {
		t.Fatalf("unexpected response: %+v", resp)
	}
	assertNoStore(t, rec)
}

func TestPutDocumentRejectsReplacingExistingDocumentWithoutVersion(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnv(t, config.DeploymentModeSelfHosted)
	performJSONRequest(t, env.handler, env.cookie, http.MethodPut, "/api/document", map[string]any{
		"appId": "demo",
		"data":  snapshot("tab-1", "[]"),
	})

	rec := performJSONRequest(t, env.handler, env.cookie, http.MethodPut, "/api/document", map[string]any{
		"appId": "demo",
		"data":  snapshot("tab-2", "[]"),
	})

	assertErrorResponse(t, rec, http.StatusBadRequest, "invalid_request")
	if !strings.Contains(rec.Body.String(), "version is required") {
		t.Fatalf("expected missing version error, got %q", rec.Body.String())
	}
}

func TestPutDocumentSupportsOptimisticLocking(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnv(t, config.DeploymentModeSelfHosted)
	first := performJSONRequest(t, env.handler, env.cookie, http.MethodPut, "/api/document", map[string]any{
		"appId": "demo",
		"data":  snapshot("tab-1", "[]"),
	})

	var firstResp struct {
		Version int64 `json:"version"`
	}
	decodeResponse(t, first, &firstResp)

	second := performJSONRequest(t, env.handler, env.cookie, http.MethodPut, "/api/document", map[string]any{
		"appId":              "demo",
		"data":               snapshot("tab-2", "[]"),
		"version":            firstResp.Version,
		"baseServerRevision": firstResp.Version,
	})

	var secondResp struct {
		Version int64 `json:"version"`
	}
	decodeResponse(t, second, &secondResp)
	if secondResp.Version != 2 {
		t.Fatalf("expected version 2, got %d", secondResp.Version)
	}
}

func TestPutDocumentRejectsMismatchedVersionAndBaseServerRevision(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnv(t, config.DeploymentModeSelfHosted)
	rec := performJSONRequest(t, env.handler, env.cookie, http.MethodPut, "/api/document", map[string]any{
		"appId":              "demo",
		"data":               snapshot("tab-1", "[]"),
		"version":            1,
		"baseServerRevision": 2,
	})

	assertErrorResponse(t, rec, http.StatusBadRequest, "invalid_request")
	if !strings.Contains(rec.Body.String(), "version and baseServerRevision must match") {
		t.Fatalf("expected base revision mismatch error, got %q", rec.Body.String())
	}
}

func TestPutDocumentReturnsConflictForMismatchedVersion(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnv(t, config.DeploymentModeSelfHosted)
	first := performJSONRequest(t, env.handler, env.cookie, http.MethodPut, "/api/document", map[string]any{
		"appId": "demo",
		"data":  snapshot("tab-1", "[]"),
	})
	var firstResp struct {
		Version int64 `json:"version"`
	}
	decodeResponse(t, first, &firstResp)

	second := performJSONRequest(t, env.handler, env.cookie, http.MethodPut, "/api/document", map[string]any{
		"appId":   "demo",
		"data":    snapshot("tab-2", "[]"),
		"version": firstResp.Version,
	})
	var secondResp struct {
		Version int64 `json:"version"`
	}
	decodeResponse(t, second, &secondResp)

	rec := performJSONRequest(t, env.handler, env.cookie, http.MethodPut, "/api/document", map[string]any{
		"appId":   "demo",
		"data":    snapshot("tab-3", "[]"),
		"version": firstResp.Version,
	})

	assertVersionConflictResponse(t, rec, secondResp.Version)
}

func TestSyncMutationsRequiresAuthentication(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t, config.DeploymentModeSelfHosted)
	rec := performJSONRequest(t, env.handler, nil, http.MethodPost, "/api/sync/mutations", map[string]any{
		"appId":     "postbaby-web",
		"mutations": []map[string]any{buildSyncMutationEnvelope("mut-1", "CreateNode", map[string]any{"tabId": "tab-1"})},
	})

	assertErrorResponse(t, rec, http.StatusUnauthorized, "unauthorized")
	assertNoStore(t, rec)
}

func TestSyncMutationsStoresReceiptsAndReturnsAcceptedResults(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnv(t, config.DeploymentModeSelfHosted)
	user := authenticatedUserFromCookie(t, env, env.cookie)

	rec := performJSONRequest(t, env.handler, env.cookie, http.MethodPost, "/api/sync/mutations", map[string]any{
		"appId": "postbaby-web",
		"mutations": []map[string]any{
			buildSyncMutationEnvelope("mut-1", "CreateNode", map[string]any{"tabId": "tab-1", "name": "Inbox"}),
			buildSyncMutationEnvelope("mut-2", "MoveNode", map[string]any{"tabId": "tab-1", "position": map[string]any{"top": "10px", "left": "20px"}}),
		},
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%q", rec.Code, rec.Body.String())
	}

	var resp struct {
		OK      bool   `json:"ok"`
		AppID   string `json:"appId"`
		Results []struct {
			MutationID string `json:"mutationId"`
			Status     string `json:"status"`
			Duplicate  bool   `json:"duplicate"`
			AcceptedAt string `json:"acceptedAt"`
		} `json:"results"`
	}
	decodeResponse(t, rec, &resp)

	if !resp.OK || resp.AppID != "postbaby-web" || len(resp.Results) != 2 {
		t.Fatalf("unexpected response: %+v", resp)
	}
	for _, result := range resp.Results {
		if result.Status != store.SyncMutationReceiptStatusAccepted || result.Duplicate || result.AcceptedAt == "" {
			t.Fatalf("unexpected mutation ack result: %+v", result)
		}
	}

	count, err := env.store.CountSyncMutationReceipts(context.Background(), user.OwnerKey, "postbaby-web")
	if err != nil {
		t.Fatalf("count sync mutation receipts: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected two stored mutation receipts, got %d", count)
	}
}

func TestSyncMutationsReturnsDuplicateAckWithoutDuplicateRows(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnv(t, config.DeploymentModeSelfHosted)
	user := authenticatedUserFromCookie(t, env, env.cookie)
	requestBody := map[string]any{
		"appId":     "postbaby-web",
		"mutations": []map[string]any{buildSyncMutationEnvelope("mut-1", "CreateNode", map[string]any{"tabId": "tab-1"})},
	}

	first := performJSONRequest(t, env.handler, env.cookie, http.MethodPost, "/api/sync/mutations", requestBody)
	second := performJSONRequest(t, env.handler, env.cookie, http.MethodPost, "/api/sync/mutations", requestBody)

	var firstResp struct {
		Results []struct {
			MutationID string `json:"mutationId"`
			Status     string `json:"status"`
			Duplicate  bool   `json:"duplicate"`
			AcceptedAt string `json:"acceptedAt"`
		} `json:"results"`
	}
	var secondResp struct {
		Results []struct {
			MutationID string `json:"mutationId"`
			Status     string `json:"status"`
			Duplicate  bool   `json:"duplicate"`
			AcceptedAt string `json:"acceptedAt"`
		} `json:"results"`
	}
	decodeResponse(t, first, &firstResp)
	decodeResponse(t, second, &secondResp)

	if len(firstResp.Results) != 1 || len(secondResp.Results) != 1 {
		t.Fatalf("unexpected duplicate ack responses: first=%+v second=%+v", firstResp, secondResp)
	}
	if firstResp.Results[0].Status != store.SyncMutationReceiptStatusAccepted || firstResp.Results[0].Duplicate {
		t.Fatalf("unexpected first ack result: %+v", firstResp.Results[0])
	}
	if secondResp.Results[0].Status != store.SyncMutationReceiptStatusAccepted || !secondResp.Results[0].Duplicate {
		t.Fatalf("unexpected duplicate ack result: %+v", secondResp.Results[0])
	}
	if firstResp.Results[0].AcceptedAt != secondResp.Results[0].AcceptedAt {
		t.Fatalf("expected duplicate ack to preserve acceptedAt %q, got %q", firstResp.Results[0].AcceptedAt, secondResp.Results[0].AcceptedAt)
	}

	count, err := env.store.CountSyncMutationReceipts(context.Background(), user.OwnerKey, "postbaby-web")
	if err != nil {
		t.Fatalf("count sync mutation receipts after duplicate request: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one stored receipt after duplicate request, got %d", count)
	}
}

func TestSyncMutationsRejectsInvalidOperationType(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnv(t, config.DeploymentModeSelfHosted)
	user := authenticatedUserFromCookie(t, env, env.cookie)

	rec := performJSONRequest(t, env.handler, env.cookie, http.MethodPost, "/api/sync/mutations", map[string]any{
		"appId":     "postbaby-web",
		"mutations": []map[string]any{buildSyncMutationEnvelope("mut-1", "ImportGraph", map[string]any{"tabId": "tab-1"})},
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%q", rec.Code, rec.Body.String())
	}

	var resp struct {
		Results []struct {
			MutationID string `json:"mutationId"`
			Status     string `json:"status"`
			Error      struct {
				Code string `json:"code"`
			} `json:"error"`
		} `json:"results"`
	}
	decodeResponse(t, rec, &resp)

	if len(resp.Results) != 1 || resp.Results[0].MutationID != "mut-1" || resp.Results[0].Status != "rejected" || resp.Results[0].Error.Code != "invalid_operation_type" {
		t.Fatalf("unexpected rejection response: %+v", resp)
	}

	count, err := env.store.CountSyncMutationReceipts(context.Background(), user.OwnerKey, "postbaby-web")
	if err != nil {
		t.Fatalf("count sync mutation receipts after rejection: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected no stored receipts for rejected mutation, got %d", count)
	}
}

func TestSyncMutationsScopePerOwnerAndApp(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t, config.DeploymentModeSelfHosted)
	firstCookie := createAuthenticatedUserSession(t, env, "owner-one")
	secondCookie := createAuthenticatedUserSession(t, env, "owner-two")
	firstUser := authenticatedUserFromCookie(t, env, firstCookie)
	secondUser := authenticatedUserFromCookie(t, env, secondCookie)

	body := map[string]any{
		"appId":     "postbaby-web",
		"mutations": []map[string]any{buildSyncMutationEnvelope("shared-mut", "CreateNode", map[string]any{"tabId": "tab-1"})},
	}
	otherAppBody := map[string]any{
		"appId":     "postbaby-mobile",
		"mutations": []map[string]any{buildSyncMutationEnvelope("shared-mut", "CreateNode", map[string]any{"tabId": "tab-1"})},
	}

	firstRec := performJSONRequest(t, env.handler, firstCookie, http.MethodPost, "/api/sync/mutations", body)
	secondRec := performJSONRequest(t, env.handler, secondCookie, http.MethodPost, "/api/sync/mutations", body)
	thirdRec := performJSONRequest(t, env.handler, firstCookie, http.MethodPost, "/api/sync/mutations", otherAppBody)

	if firstRec.Code != http.StatusOK || secondRec.Code != http.StatusOK || thirdRec.Code != http.StatusOK {
		t.Fatalf("expected scoped mutation receipts to succeed, got statuses %d %d %d", firstRec.Code, secondRec.Code, thirdRec.Code)
	}

	firstCount, err := env.store.CountSyncMutationReceipts(context.Background(), firstUser.OwnerKey, "postbaby-web")
	if err != nil {
		t.Fatalf("count first owner receipts: %v", err)
	}
	secondCount, err := env.store.CountSyncMutationReceipts(context.Background(), secondUser.OwnerKey, "postbaby-web")
	if err != nil {
		t.Fatalf("count second owner receipts: %v", err)
	}
	otherAppCount, err := env.store.CountSyncMutationReceipts(context.Background(), firstUser.OwnerKey, "postbaby-mobile")
	if err != nil {
		t.Fatalf("count first owner other-app receipts: %v", err)
	}

	if firstCount != 1 || secondCount != 1 || otherAppCount != 1 {
		t.Fatalf("expected scoped counts 1/1/1, got %d/%d/%d", firstCount, secondCount, otherAppCount)
	}
}

func TestSyncMutationsRejectMalformedPayloadSafely(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnv(t, config.DeploymentModeSelfHosted)
	user := authenticatedUserFromCookie(t, env, env.cookie)

	rec := performJSONRequest(t, env.handler, env.cookie, http.MethodPost, "/api/sync/mutations", map[string]any{
		"appId": "postbaby-web",
		"mutations": []map[string]any{
			buildSyncMutationEnvelope("mut-1", "CreateNode", []any{"bad-payload"}),
		},
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%q", rec.Code, rec.Body.String())
	}

	var resp struct {
		Results []struct {
			Status string `json:"status"`
			Error  struct {
				Code string `json:"code"`
			} `json:"error"`
		} `json:"results"`
	}
	decodeResponse(t, rec, &resp)
	if len(resp.Results) != 1 || resp.Results[0].Status != "rejected" || resp.Results[0].Error.Code != "invalid_payload" {
		t.Fatalf("unexpected malformed payload response: %+v", resp)
	}

	count, err := env.store.CountSyncMutationReceipts(context.Background(), user.OwnerKey, "postbaby-web")
	if err != nil {
		t.Fatalf("count sync mutation receipts after malformed payload: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected no stored receipts for malformed payload, got %d", count)
	}
}

func TestSyncMutationsRejectLargePayload(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnv(t, config.DeploymentModeSelfHosted)
	user := authenticatedUserFromCookie(t, env, env.cookie)

	rec := performJSONRequest(t, env.handler, env.cookie, http.MethodPost, "/api/sync/mutations", map[string]any{
		"appId": "postbaby-web",
		"mutations": []map[string]any{
			buildSyncMutationEnvelope("mut-1", "CreateNode", map[string]any{
				"tabId": "tab-1",
				"blob":  strings.Repeat("a", maxSyncMutationPayloadBytes+1),
			}),
		},
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%q", rec.Code, rec.Body.String())
	}

	var resp struct {
		Results []struct {
			Status string `json:"status"`
			Error  struct {
				Code string `json:"code"`
			} `json:"error"`
		} `json:"results"`
	}
	decodeResponse(t, rec, &resp)
	if len(resp.Results) != 1 || resp.Results[0].Status != "rejected" || resp.Results[0].Error.Code != "payload_too_large" {
		t.Fatalf("unexpected large payload response: %+v", resp)
	}

	count, err := env.store.CountSyncMutationReceipts(context.Background(), user.OwnerKey, "postbaby-web")
	if err != nil {
		t.Fatalf("count sync mutation receipts after large payload: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected no stored receipts for large payload, got %d", count)
	}
}

func TestDocumentLoadAfterSavePreservesJSON(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnv(t, config.DeploymentModeSelfHosted)
	payload := map[string]any{
		"appId": "demo",
		"data": map[string]string{
			"tabs":                `[{"id":"tab-1","name":"1","items":[{"id":"item-1","name":"Inbox","color":"#FFFFFF","position":{"top":"0px","left":"0px"}}],"colorIndex":0,"gridSetting":"none","edges":[]}]`,
			"activeTabId":         "tab-1",
			"theme":               "light",
			"corporateMode":       "false",
			"hideInstructions":    "false",
			"defaultColorEnabled": "true",
			"defaultColor":        "#FFFFFF",
			"hasRunBefore":        "true",
		},
	}

	performJSONRequest(t, env.handler, env.cookie, http.MethodPut, "/api/document", payload)

	req := httptest.NewRequest(http.MethodGet, "/api/document?appId=demo", nil)
	req.AddCookie(env.cookie)
	rec := httptest.NewRecorder()
	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp struct {
		OK      bool            `json:"ok"`
		AppID   string          `json:"appId"`
		Data    json.RawMessage `json:"data"`
		Version int64           `json:"version"`
	}
	decodeResponse(t, rec, &resp)

	if !resp.OK || resp.AppID != "demo" || resp.Version != 1 {
		t.Fatalf("unexpected response: %+v", resp)
	}

	assertJSONEqual(t, payload["data"], resp.Data)
}

func TestCloudMultiUserDocumentRoutesRequireAuthentication(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t, config.DeploymentModeCloud)

	for _, target := range []string{"/api/document?appId=demo", "/api/document/meta?appId=demo"} {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		rec := httptest.NewRecorder()
		env.handler.ServeHTTP(rec, req)

		assertErrorResponse(t, rec, http.StatusUnauthorized, "unauthorized")
		assertNoStore(t, rec)
	}
}

func TestCloudMultiUserDocumentRoutesRequireHostedSyncEntitlement(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t, config.DeploymentModeCloud)
	cookie := createAuthenticatedUserSession(t, env, "cloud-user")

	getReq := httptest.NewRequest(http.MethodGet, "/api/document?appId=demo", nil)
	getReq.AddCookie(cookie)
	getRec := httptest.NewRecorder()
	env.handler.ServeHTTP(getRec, getReq)
	assertErrorResponse(t, getRec, http.StatusForbidden, "entitlement_required")
	assertNoStore(t, getRec)

	metaReq := httptest.NewRequest(http.MethodGet, "/api/document/meta?appId=demo", nil)
	metaReq.AddCookie(cookie)
	metaRec := httptest.NewRecorder()
	env.handler.ServeHTTP(metaRec, metaReq)
	assertErrorResponse(t, metaRec, http.StatusForbidden, "entitlement_required")
	assertNoStore(t, metaRec)

	putRec := performJSONRequest(t, env.handler, cookie, http.MethodPut, "/api/document", map[string]any{
		"appId": "demo",
		"data":  snapshot("tab-1", "[]"),
	})
	assertErrorResponse(t, putRec, http.StatusForbidden, "entitlement_required")
	assertNoStore(t, putRec)
}

func TestCloudMultiUserInactiveEntitlementAllowsReadButBlocksWrite(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t, config.DeploymentModeCloud)
	cookie := createAuthenticatedUserSession(t, env, "inactive-cloud-user")
	grantHostedSyncEntitlement(t, env, cookie)

	firstPut := performJSONRequest(t, env.handler, cookie, http.MethodPut, "/api/document", map[string]any{
		"appId": "demo",
		"data":  snapshot("tab-1", `[{"id":"tab-1","name":"1"}]`),
	})
	if firstPut.Code != http.StatusOK {
		t.Fatalf("expected initial active save status 200, got %d body=%q", firstPut.Code, firstPut.Body.String())
	}

	setHostedSyncEntitlementStatus(t, env, cookie, store.EntitlementStatusCanceled)

	metaReq := httptest.NewRequest(http.MethodGet, "/api/document/meta?appId=demo", nil)
	metaReq.AddCookie(cookie)
	metaRec := httptest.NewRecorder()
	env.handler.ServeHTTP(metaRec, metaReq)
	if metaRec.Code != http.StatusOK {
		t.Fatalf("expected inactive entitlement to read metadata, got %d body=%q", metaRec.Code, metaRec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/document?appId=demo", nil)
	getReq.AddCookie(cookie)
	getRec := httptest.NewRecorder()
	env.handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected inactive entitlement to read document, got %d body=%q", getRec.Code, getRec.Body.String())
	}

	putRec := performJSONRequest(t, env.handler, cookie, http.MethodPut, "/api/document", map[string]any{
		"appId":              "demo",
		"data":               snapshot("tab-2", `[{"id":"tab-2","name":"2"}]`),
		"version":            1,
		"baseServerRevision": 1,
	})
	assertErrorResponse(t, putRec, http.StatusForbidden, "entitlement_required")
	assertNoStore(t, putRec)
}

func TestCloudMultiUserDocumentRoutesUseAuthenticatedOwnerNamespace(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t, config.DeploymentModeCloud)
	firstCookie := createAuthenticatedUserSession(t, env, "cloud-user-one")
	secondCookie := createAuthenticatedUserSession(t, env, "cloud-user-two")
	grantHostedSyncEntitlement(t, env, firstCookie)
	grantHostedSyncEntitlement(t, env, secondCookie)

	firstPut := performJSONRequest(t, env.handler, firstCookie, http.MethodPut, "/api/document", map[string]any{
		"appId": "demo",
		"data":  snapshot("tab-1", `[{"id":"tab-1","name":"1"}]`),
	})
	if firstPut.Code != http.StatusOK {
		t.Fatalf("expected first user save status 200, got %d", firstPut.Code)
	}

	secondGetBeforeSaveReq := httptest.NewRequest(http.MethodGet, "/api/document?appId=demo", nil)
	secondGetBeforeSaveReq.AddCookie(secondCookie)
	secondGetBeforeSaveRec := httptest.NewRecorder()
	env.handler.ServeHTTP(secondGetBeforeSaveRec, secondGetBeforeSaveReq)
	assertErrorResponse(t, secondGetBeforeSaveRec, http.StatusNotFound, "document_not_found")

	secondMetaBeforeSaveReq := httptest.NewRequest(http.MethodGet, "/api/document/meta?appId=demo", nil)
	secondMetaBeforeSaveReq.AddCookie(secondCookie)
	secondMetaBeforeSaveRec := httptest.NewRecorder()
	env.handler.ServeHTTP(secondMetaBeforeSaveRec, secondMetaBeforeSaveReq)

	if secondMetaBeforeSaveRec.Code != http.StatusOK {
		t.Fatalf("expected second user document meta status 200 before save, got %d", secondMetaBeforeSaveRec.Code)
	}

	var secondMetaBeforeSave struct {
		OK     bool   `json:"ok"`
		AppID  string `json:"appId"`
		Exists bool   `json:"exists"`
	}
	decodeResponse(t, secondMetaBeforeSaveRec, &secondMetaBeforeSave)
	if !secondMetaBeforeSave.OK || secondMetaBeforeSave.AppID != "demo" || secondMetaBeforeSave.Exists {
		t.Fatalf("expected second user meta to report no existing document before save, got %+v", secondMetaBeforeSave)
	}

	secondPut := performJSONRequest(t, env.handler, secondCookie, http.MethodPut, "/api/document", map[string]any{
		"appId": "demo",
		"data":  snapshot("tab-2", `[{"id":"tab-2","name":"2"}]`),
	})
	if secondPut.Code != http.StatusOK {
		t.Fatalf("expected second user save status 200, got %d", secondPut.Code)
	}

	firstGetReq := httptest.NewRequest(http.MethodGet, "/api/document?appId=demo", nil)
	firstGetReq.AddCookie(firstCookie)
	firstGetRec := httptest.NewRecorder()
	env.handler.ServeHTTP(firstGetRec, firstGetReq)

	secondGetReq := httptest.NewRequest(http.MethodGet, "/api/document?appId=demo", nil)
	secondGetReq.AddCookie(secondCookie)
	secondGetRec := httptest.NewRecorder()
	env.handler.ServeHTTP(secondGetRec, secondGetReq)

	var firstDoc struct {
		OK      bool              `json:"ok"`
		AppID   string            `json:"appId"`
		Data    map[string]string `json:"data"`
		Version int64             `json:"version"`
	}
	decodeResponse(t, firstGetRec, &firstDoc)
	if !firstDoc.OK || firstDoc.AppID != "demo" || firstDoc.Version != 1 || firstDoc.Data["activeTabId"] != "tab-1" {
		t.Fatalf("unexpected first user document response: %+v", firstDoc)
	}

	var secondDoc struct {
		OK      bool              `json:"ok"`
		AppID   string            `json:"appId"`
		Data    map[string]string `json:"data"`
		Version int64             `json:"version"`
	}
	decodeResponse(t, secondGetRec, &secondDoc)
	if !secondDoc.OK || secondDoc.AppID != "demo" || secondDoc.Version != 1 || secondDoc.Data["activeTabId"] != "tab-2" {
		t.Fatalf("unexpected second user document response: %+v", secondDoc)
	}

	firstMetaReq := httptest.NewRequest(http.MethodGet, "/api/document/meta?appId=demo", nil)
	firstMetaReq.AddCookie(firstCookie)
	firstMetaRec := httptest.NewRecorder()
	env.handler.ServeHTTP(firstMetaRec, firstMetaReq)

	var firstMeta struct {
		OK      bool   `json:"ok"`
		AppID   string `json:"appId"`
		Exists  bool   `json:"exists"`
		Version int64  `json:"version"`
	}
	decodeResponse(t, firstMetaRec, &firstMeta)
	if !firstMeta.OK || firstMeta.AppID != "demo" || !firstMeta.Exists || firstMeta.Version != 1 {
		t.Fatalf("unexpected first user document meta response: %+v", firstMeta)
	}
}

type testEnv struct {
	handler     http.Handler
	store       *store.Store
	authManager *auth.Manager
	cookie      *http.Cookie
	dbPath      string
}

func TestDocumentRoutesReturnNotFoundWhenSyncDisabled(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t, config.DeploymentModeStatic)

	for _, target := range []string{"/api/document?appId=demo", "/api/document/meta?appId=demo"} {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		rec := httptest.NewRecorder()
		env.handler.ServeHTTP(rec, req)

		assertErrorResponse(t, rec, http.StatusNotFound, "not_found")
		assertNoStore(t, rec)
	}
}

func TestSyncDeltaReturnsNotFoundWhenDisabledByDefault(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnv(t, config.DeploymentModeSelfHosted)
	req := httptest.NewRequest(http.MethodGet, "/api/sync/delta?appId=postbaby-web&sinceVersion=0", nil)
	req.AddCookie(env.cookie)
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	assertErrorResponse(t, rec, http.StatusNotFound, "not_found")
	assertNoStore(t, rec)
}

func TestSyncDeltaReturnsNotFoundBeforeAuthWhenDisabled(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t, config.DeploymentModeSelfHosted)
	req := httptest.NewRequest(http.MethodGet, "/api/sync/delta?appId=postbaby-web&sinceVersion=0", nil)
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	assertErrorResponse(t, rec, http.StatusNotFound, "not_found")
	assertNoStore(t, rec)
}

func TestSyncDeltaReturnsNotFoundInCloudWhenEnabled(t *testing.T) {
	t.Parallel()

	env := newTestEnvWithOptions(t, config.DeploymentModeCloud, true)
	req := httptest.NewRequest(http.MethodGet, "/api/sync/delta?appId=postbaby-web&sinceVersion=0", nil)
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	assertErrorResponse(t, rec, http.StatusNotFound, "not_found")
	assertNoStore(t, rec)
}

func TestSyncDeltaReturnsNotFoundInStaticWhenEnabled(t *testing.T) {
	t.Parallel()

	env := newTestEnvWithOptions(t, config.DeploymentModeStatic, true)
	req := httptest.NewRequest(http.MethodGet, "/api/sync/delta?appId=postbaby-web&sinceVersion=0", nil)
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	assertErrorResponse(t, rec, http.StatusNotFound, "not_found")
	assertNoStore(t, rec)
}

func TestSyncDeltaReturnsNotFoundInCloudMultiUserModeWhenEnabled(t *testing.T) {
	t.Setenv("POSTBABY_DEPLOYMENT_MODE", "cloud_multi_user")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	env := newTestEnvWithOptions(t, cfg.DeploymentMode, true)
	req := httptest.NewRequest(http.MethodGet, "/api/sync/delta?appId=postbaby-web&sinceVersion=0", nil)
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	assertErrorResponse(t, rec, http.StatusNotFound, "not_found")
	assertNoStore(t, rec)
}

func TestSyncDeltaReturnsNotFoundInCloudEvenForAuthenticatedEntitledUser(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnvWithOptions(t, config.DeploymentModeCloud, true)
	grantHostedSyncEntitlement(t, env, env.cookie)
	req := httptest.NewRequest(http.MethodGet, "/api/sync/delta?appId=postbaby-web&sinceVersion=0", nil)
	req.AddCookie(env.cookie)
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	assertErrorResponse(t, rec, http.StatusNotFound, "not_found")
	assertNoStore(t, rec)
}

func TestSyncDeltaRejectsNonGetMethodWithAllowHeader(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnvWithOptions(t, config.DeploymentModeSelfHosted, true)
	req := httptest.NewRequest(http.MethodPost, "/api/sync/delta?appId=postbaby-web&sinceVersion=0", nil)
	req.AddCookie(env.cookie)
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	assertErrorResponse(t, rec, http.StatusMethodNotAllowed, "method_not_allowed")
	if allow := rec.Header().Get("Allow"); allow != http.MethodGet {
		t.Fatalf("expected Allow header %q, got %q", http.MethodGet, allow)
	}
	assertNoStore(t, rec)
}

func TestSyncDeltaRequiresAuthenticationWhenEnabled(t *testing.T) {
	t.Parallel()

	env := newTestEnvWithOptions(t, config.DeploymentModeSelfHosted, true)
	req := httptest.NewRequest(http.MethodGet, "/api/sync/delta?appId=postbaby-web&sinceVersion=0", nil)
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	assertErrorResponse(t, rec, http.StatusUnauthorized, "unauthorized")
	assertNoStore(t, rec)
}

func TestSyncDeltaRequiresAppID(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnvWithOptions(t, config.DeploymentModeSelfHosted, true)
	req := httptest.NewRequest(http.MethodGet, "/api/sync/delta?sinceVersion=0", nil)
	req.AddCookie(env.cookie)
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	assertErrorResponse(t, rec, http.StatusBadRequest, "invalid_request")
	assertNoStore(t, rec)
}

func TestSyncDeltaRequiresSinceVersion(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnvWithOptions(t, config.DeploymentModeSelfHosted, true)
	req := httptest.NewRequest(http.MethodGet, "/api/sync/delta?appId=postbaby-web", nil)
	req.AddCookie(env.cookie)
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	assertErrorResponse(t, rec, http.StatusBadRequest, "invalid_request")
	assertNoStore(t, rec)
}

func TestSyncDeltaRejectsInvalidSinceVersion(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnvWithOptions(t, config.DeploymentModeSelfHosted, true)
	req := httptest.NewRequest(http.MethodGet, "/api/sync/delta?appId=postbaby-web&sinceVersion=abc", nil)
	req.AddCookie(env.cookie)
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	assertErrorResponse(t, rec, http.StatusBadRequest, "invalid_request")
	assertNoStore(t, rec)
}

func TestSyncDeltaRejectsNegativeSinceVersion(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnvWithOptions(t, config.DeploymentModeSelfHosted, true)
	req := httptest.NewRequest(http.MethodGet, "/api/sync/delta?appId=postbaby-web&sinceVersion=-1", nil)
	req.AddCookie(env.cookie)
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	assertErrorResponse(t, rec, http.StatusBadRequest, "invalid_request")
	assertNoStore(t, rec)
}

func TestSyncDeltaRejectsNegativeApplicationWatermark(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnvWithOptions(t, config.DeploymentModeSelfHosted, true)
	req := httptest.NewRequest(http.MethodGet, "/api/sync/delta?appId=postbaby-web&sinceVersion=0&applicationWatermark=-1", nil)
	req.AddCookie(env.cookie)
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	assertErrorResponse(t, rec, http.StatusBadRequest, "invalid_request")
	assertNoStore(t, rec)
}

func TestSyncDeltaRejectsNonIntegerApplicationWatermark(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnvWithOptions(t, config.DeploymentModeSelfHosted, true)
	req := httptest.NewRequest(http.MethodGet, "/api/sync/delta?appId=postbaby-web&sinceVersion=0&applicationWatermark=abc", nil)
	req.AddCookie(env.cookie)
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	assertErrorResponse(t, rec, http.StatusBadRequest, "invalid_request")
	assertNoStore(t, rec)
}

func TestSyncDeltaRejectsInvalidIncludeApplications(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnvWithOptions(t, config.DeploymentModeSelfHosted, true)
	req := httptest.NewRequest(http.MethodGet, "/api/sync/delta?appId=postbaby-web&sinceVersion=0&includeApplications=maybe", nil)
	req.AddCookie(env.cookie)
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	assertErrorResponse(t, rec, http.StatusBadRequest, "invalid_request")
	assertNoStore(t, rec)
}

func TestSyncDeltaRejectsNonIntegerLimit(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnvWithOptions(t, config.DeploymentModeSelfHosted, true)
	req := httptest.NewRequest(http.MethodGet, "/api/sync/delta?appId=postbaby-web&sinceVersion=0&limit=abc", nil)
	req.AddCookie(env.cookie)
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	assertErrorResponse(t, rec, http.StatusBadRequest, "invalid_request")
	assertNoStore(t, rec)
}

func TestSyncDeltaRejectsInvalidLimit(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnvWithOptions(t, config.DeploymentModeSelfHosted, true)
	req := httptest.NewRequest(http.MethodGet, "/api/sync/delta?appId=postbaby-web&sinceVersion=0&limit=0", nil)
	req.AddCookie(env.cookie)
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	assertErrorResponse(t, rec, http.StatusBadRequest, "invalid_request")
	assertNoStore(t, rec)
}

func TestSyncDeltaReturnsClientVersionAheadReason(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnvWithOptions(t, config.DeploymentModeSelfHosted, true)
	user := authenticatedUserFromCookie(t, env, env.cookie)
	appID := "postbaby-web"
	body := mustMarshalSyncDeltaSnapshotBody(t, snapshot("tab-1", "[]"))
	doc, err := env.store.PutDocument(context.Background(), user.OwnerKey, appID, body, nil)
	if err != nil {
		t.Fatalf("seed document: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/sync/delta?appId="+appID+"&sinceVersion=2", nil)
	req.AddCookie(env.cookie)
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp syncDeltaResponse
	decodeResponse(t, rec, &resp)

	if !resp.OK || !resp.RequiresSnapshotRefresh || resp.Reason != store.SyncDeltaMetadataReasonSnapshotRequiredClientVersionAhead {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if resp.CurrentDocumentVersion != doc.Version || resp.CurrentDocumentHash != hashSyncDeltaTestBody(body) {
		t.Fatalf("unexpected current document metadata: %+v", resp)
	}
	if len(resp.Applications) != 0 {
		t.Fatalf("expected no application metadata, got %+v", resp.Applications)
	}
	assertNoStore(t, rec)
}

func TestSyncDeltaReturnsUpToDateWhenEnabled(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnvWithOptions(t, config.DeploymentModeSelfHosted, true)
	user := authenticatedUserFromCookie(t, env, env.cookie)
	appID := "postbaby-web"
	body := mustMarshalSyncDeltaSnapshotBody(t, snapshot("tab-1", "[]"))
	doc, err := env.store.PutDocument(context.Background(), user.OwnerKey, appID, body, nil)
	if err != nil {
		t.Fatalf("seed document: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/sync/delta?appId="+appID+"&sinceVersion=1", nil)
	req.AddCookie(env.cookie)
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp syncDeltaResponse
	decodeResponse(t, rec, &resp)

	if !resp.OK || resp.RequiresSnapshotRefresh || resp.Reason != store.SyncDeltaMetadataReasonUpToDate {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if resp.CurrentDocumentVersion != doc.Version || resp.ClientVersion != doc.Version || resp.CurrentDocumentHash != hashSyncDeltaTestBody(body) {
		t.Fatalf("unexpected document metadata response: %+v", resp)
	}
	if len(resp.Applications) != 0 {
		t.Fatalf("expected no applications, got %+v", resp.Applications)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode raw sync delta response: %v", err)
	}
	if string(raw["applications"]) != "[]" || string(raw["warnings"]) != "[]" {
		t.Fatalf("expected empty applications and warnings arrays, got %s", rec.Body.String())
	}
	assertNoStore(t, rec)
}

func TestSyncDeltaReturnsDocumentVersionChanged(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnvWithOptions(t, config.DeploymentModeSelfHosted, true)
	user := authenticatedUserFromCookie(t, env, env.cookie)
	appID := "postbaby-web"
	bodyBefore := mustMarshalSyncDeltaSnapshotBody(t, snapshot("tab-1", "[]"))
	docBefore, err := env.store.PutDocument(context.Background(), user.OwnerKey, appID, bodyBefore, nil)
	if err != nil {
		t.Fatalf("seed initial document: %v", err)
	}
	bodyAfter := mustMarshalSyncDeltaSnapshotBody(t, snapshot("tab-2", "[]"))
	expectedVersion := docBefore.Version
	docAfter, err := env.store.PutDocument(context.Background(), user.OwnerKey, appID, bodyAfter, &expectedVersion)
	if err != nil {
		t.Fatalf("update document: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/sync/delta?appId="+appID+"&sinceVersion=1", nil)
	req.AddCookie(env.cookie)
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp syncDeltaResponse
	decodeResponse(t, rec, &resp)

	if !resp.OK || resp.RequiresSnapshotRefresh || resp.Reason != store.SyncDeltaMetadataReasonDocumentVersionChanged {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if resp.CurrentDocumentVersion != docAfter.Version || resp.ClientVersion != docBefore.Version || resp.CurrentDocumentHash != hashSyncDeltaTestBody(bodyAfter) {
		t.Fatalf("unexpected document metadata response: %+v", resp)
	}
	assertNoStore(t, rec)
}

func TestSyncDeltaReturnsCommittedAppliedAndSkippedApplications(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnvWithOptions(t, config.DeploymentModeSelfHosted, true)
	fixture := seedCommittedSyncDeltaMetadataFixture(t, env)

	req := httptest.NewRequest(http.MethodGet, "/api/sync/delta?appId="+fixture.AppID+"&sinceVersion=1&includeApplications=true", nil)
	req.AddCookie(env.cookie)
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp syncDeltaResponse
	decodeResponse(t, rec, &resp)

	if !resp.OK || resp.RequiresSnapshotRefresh || resp.Reason != store.SyncDeltaMetadataReasonApplicationsAvailable {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if resp.CurrentDocumentVersion != fixture.CurrentDocument.Version || resp.CurrentDocumentHash != hashSyncDeltaTestBody(fixture.BodyAfter) {
		t.Fatalf("unexpected current document metadata: %+v", resp)
	}
	if len(resp.Applications) != 2 {
		t.Fatalf("expected 2 committed applications, got %+v", resp.Applications)
	}
	if resp.Applications[0].MutationID != fixture.AppliedMutationID || resp.Applications[0].ApplicationStatus != store.SyncMutationReplayApplicationStatusApplied {
		t.Fatalf("unexpected applied application entry: %+v", resp.Applications[0])
	}
	if resp.Applications[1].MutationID != fixture.SkippedMutationID || resp.Applications[1].ApplicationStatus != store.SyncMutationReplayApplicationStatusSkipped {
		t.Fatalf("unexpected skipped application entry: %+v", resp.Applications[1])
	}
	if resp.NextApplicationWatermark == nil || *resp.NextApplicationWatermark != fixture.SkippedApplication.ID {
		t.Fatalf("unexpected next application watermark: %+v", resp)
	}
	if !containsSyncDeltaString(resp.Warnings, "document_version_advanced_without_body_change") {
		t.Fatalf("expected version-advance warning, got %+v", resp.Warnings)
	}
	assertNoStore(t, rec)
}

func TestSyncDeltaUsesVersionBasedFilteringWhenNoWatermarkIsProvided(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnvWithOptions(t, config.DeploymentModeSelfHosted, true)
	fixture := seedCommittedSyncDeltaMetadataFixture(t, env)

	req := httptest.NewRequest(http.MethodGet, "/api/sync/delta?appId="+fixture.AppID+"&sinceVersion=2&includeApplications=true", nil)
	req.AddCookie(env.cookie)
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp syncDeltaResponse
	decodeResponse(t, rec, &resp)

	if !resp.OK || resp.RequiresSnapshotRefresh || resp.Reason != store.SyncDeltaMetadataReasonApplicationsAvailable {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if len(resp.Applications) != 1 || resp.Applications[0].MutationID != fixture.SkippedMutationID {
		t.Fatalf("expected only skipped application after version-based filtering, got %+v", resp.Applications)
	}
	if resp.ApplicationWatermark != nil {
		t.Fatalf("expected nil applicationWatermark when none supplied, got %+v", resp.ApplicationWatermark)
	}
	if resp.NextApplicationWatermark == nil || *resp.NextApplicationWatermark != fixture.SkippedApplication.ID {
		t.Fatalf("unexpected next application watermark: %+v", resp)
	}
	assertNoStore(t, rec)
}

func TestSyncDeltaApplicationWatermarkFiltersCommittedRows(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnvWithOptions(t, config.DeploymentModeSelfHosted, true)
	fixture := seedCommittedSyncDeltaMetadataFixture(t, env)

	req := httptest.NewRequest(http.MethodGet, "/api/sync/delta?appId="+fixture.AppID+"&sinceVersion=1&includeApplications=true&applicationWatermark="+strconv.FormatInt(fixture.AppliedApplication.ID, 10), nil)
	req.AddCookie(env.cookie)
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp syncDeltaResponse
	decodeResponse(t, rec, &resp)

	if !resp.OK || resp.RequiresSnapshotRefresh || resp.Reason != store.SyncDeltaMetadataReasonApplicationsAvailable {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if resp.ApplicationWatermark == nil || *resp.ApplicationWatermark != fixture.AppliedApplication.ID {
		t.Fatalf("unexpected echoed application watermark: %+v", resp)
	}
	if len(resp.Applications) != 1 || resp.Applications[0].MutationID != fixture.SkippedMutationID {
		t.Fatalf("expected only skipped application after watermark filtering, got %+v", resp.Applications)
	}
	if resp.NextApplicationWatermark == nil || *resp.NextApplicationWatermark != fixture.SkippedApplication.ID {
		t.Fatalf("unexpected next application watermark: %+v", resp)
	}
	assertNoStore(t, rec)
}

func TestSyncDeltaNoRowsAfterApplicationWatermarkReturnsUpToDate(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnvWithOptions(t, config.DeploymentModeSelfHosted, true)
	fixture := seedCommittedSyncDeltaMetadataFixture(t, env)

	req := httptest.NewRequest(http.MethodGet, "/api/sync/delta?appId="+fixture.AppID+"&sinceVersion=3&includeApplications=true&applicationWatermark="+strconv.FormatInt(fixture.SkippedApplication.ID, 10), nil)
	req.AddCookie(env.cookie)
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp syncDeltaResponse
	decodeResponse(t, rec, &resp)

	if !resp.OK || resp.RequiresSnapshotRefresh || resp.Reason != store.SyncDeltaMetadataReasonUpToDate {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if len(resp.Applications) != 0 {
		t.Fatalf("expected no applications after watermark, got %+v", resp.Applications)
	}
	if resp.ApplicationWatermark == nil || *resp.ApplicationWatermark != fixture.SkippedApplication.ID {
		t.Fatalf("unexpected echoed application watermark: %+v", resp)
	}
	if resp.NextApplicationWatermark == nil || *resp.NextApplicationWatermark != fixture.SkippedApplication.ID {
		t.Fatalf("unexpected next application watermark: %+v", resp)
	}
	assertNoStore(t, rec)
}

func TestSyncDeltaOmitsApplicationsWhenIncludeFalse(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnvWithOptions(t, config.DeploymentModeSelfHosted, true)
	fixture := seedCommittedSyncDeltaMetadataFixture(t, env)

	req := httptest.NewRequest(http.MethodGet, "/api/sync/delta?appId="+fixture.AppID+"&sinceVersion=3&includeApplications=false", nil)
	req.AddCookie(env.cookie)
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp syncDeltaResponse
	decodeResponse(t, rec, &resp)

	if !resp.OK || resp.RequiresSnapshotRefresh || resp.Reason != store.SyncDeltaMetadataReasonUpToDate {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if len(resp.Applications) != 0 {
		t.Fatalf("expected no application metadata when includeApplications=false, got %+v", resp.Applications)
	}
	if resp.NextApplicationWatermark == nil || *resp.NextApplicationWatermark != fixture.SkippedApplication.ID {
		t.Fatalf("expected next application watermark even when applications are omitted, got %+v", resp)
	}
	assertNoStore(t, rec)
}

func TestSyncDeltaTooManyApplicationsRequiresSnapshotRefresh(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnvWithOptions(t, config.DeploymentModeSelfHosted, true)
	fixture := seedCommittedSyncDeltaMetadataFixture(t, env)

	req := httptest.NewRequest(http.MethodGet, "/api/sync/delta?appId="+fixture.AppID+"&sinceVersion=1&includeApplications=true&limit=1", nil)
	req.AddCookie(env.cookie)
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp syncDeltaResponse
	decodeResponse(t, rec, &resp)

	if !resp.OK || !resp.RequiresSnapshotRefresh || resp.Reason != store.SyncDeltaMetadataReasonSnapshotRequiredTooManyApplications {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if len(resp.Applications) != 0 {
		t.Fatalf("expected no applications in snapshot refresh response, got %+v", resp.Applications)
	}
	if resp.NextApplicationWatermark == nil || *resp.NextApplicationWatermark != fixture.SkippedApplication.ID {
		t.Fatalf("unexpected next application watermark: %+v", resp)
	}
	assertNoStore(t, rec)
}

func TestSyncDeltaAcceptsLargeLimitAndReliesOnStoreClamp(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnvWithOptions(t, config.DeploymentModeSelfHosted, true)
	fixture := seedCommittedSyncDeltaMetadataFixture(t, env)

	req := httptest.NewRequest(http.MethodGet, "/api/sync/delta?appId="+fixture.AppID+"&sinceVersion=1&includeApplications=true&limit=999999", nil)
	req.AddCookie(env.cookie)
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp syncDeltaResponse
	decodeResponse(t, rec, &resp)

	if !resp.OK || resp.RequiresSnapshotRefresh || resp.Reason != store.SyncDeltaMetadataReasonApplicationsAvailable {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if len(resp.Applications) != 2 {
		t.Fatalf("expected committed applications with large limit, got %+v", resp.Applications)
	}
	assertNoStore(t, rec)
}

func TestSyncDeltaRepeatedSinceVersionUsesFirstValue(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnvWithOptions(t, config.DeploymentModeSelfHosted, true)
	fixture := seedCommittedSyncDeltaMetadataFixture(t, env)

	req := httptest.NewRequest(http.MethodGet, "/api/sync/delta?appId="+fixture.AppID+"&sinceVersion=3&sinceVersion=1&includeApplications=true", nil)
	req.AddCookie(env.cookie)
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp syncDeltaResponse
	decodeResponse(t, rec, &resp)

	if !resp.OK || resp.RequiresSnapshotRefresh || resp.Reason != store.SyncDeltaMetadataReasonUpToDate {
		t.Fatalf("expected first sinceVersion value to be used, got %+v", resp)
	}
	if len(resp.Applications) != 0 {
		t.Fatalf("expected no applications when first sinceVersion=3 is used, got %+v", resp.Applications)
	}
	assertNoStore(t, rec)
}

func TestSyncDeltaIgnoresUnsupportedExtraParameters(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnvWithOptions(t, config.DeploymentModeSelfHosted, true)
	fixture := seedCommittedSyncDeltaMetadataFixture(t, env)

	req := httptest.NewRequest(http.MethodGet, "/api/sync/delta?appId="+fixture.AppID+"&sinceVersion=3&ignored=value", nil)
	req.AddCookie(env.cookie)
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp syncDeltaResponse
	decodeResponse(t, rec, &resp)

	if !resp.OK || resp.RequiresSnapshotRefresh || resp.Reason != store.SyncDeltaMetadataReasonUpToDate {
		t.Fatalf("unexpected response with ignored parameter: %+v", resp)
	}
	assertNoStore(t, rec)
}

func TestSyncDeltaResponseExcludesCanonicalBodyAndCameraState(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnvWithOptions(t, config.DeploymentModeSelfHosted, true)
	fixture := seedCommittedSyncDeltaMetadataFixture(t, env)

	req := httptest.NewRequest(http.MethodGet, "/api/sync/delta?appId="+fixture.AppID+"&sinceVersion=3&includeApplications=true", nil)
	req.AddCookie(env.cookie)
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var raw map[string]json.RawMessage
	decodeResponse(t, rec, &raw)
	for _, forbiddenField := range []string{"data", "body", "camera", "cameraState"} {
		if _, exists := raw[forbiddenField]; exists {
			t.Fatalf("expected field %q to be absent from delta metadata response: %s", forbiddenField, rec.Body.String())
		}
	}
	assertNoStore(t, rec)
}

func TestSyncDeltaResponseShapeUsesArraysNullWatermarkAndRFCTimestamps(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnvWithOptions(t, config.DeploymentModeSelfHosted, true)
	fixture := seedCommittedSyncDeltaMetadataFixture(t, env)

	req := httptest.NewRequest(http.MethodGet, "/api/sync/delta?appId="+fixture.AppID+"&sinceVersion=1&includeApplications=true", nil)
	req.AddCookie(env.cookie)
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var raw map[string]json.RawMessage
	decodeResponse(t, rec, &raw)
	if string(raw["applicationWatermark"]) != "null" {
		t.Fatalf("expected applicationWatermark to serialize as null when omitted, got %s", string(raw["applicationWatermark"]))
	}
	if string(raw["applications"]) == "null" || string(raw["warnings"]) == "null" {
		t.Fatalf("expected applications and warnings arrays, got %s", rec.Body.String())
	}

	var resp syncDeltaResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode sync delta response: %v", err)
	}
	if len(resp.Applications) != 2 {
		t.Fatalf("expected 2 applications, got %+v", resp.Applications)
	}
	for _, application := range resp.Applications {
		parsedCreatedAt, err := time.Parse(time.RFC3339, application.CreatedAt)
		if err != nil {
			t.Fatalf("expected RFC3339 createdAt, got %q: %v", application.CreatedAt, err)
		}
		if !strings.HasSuffix(application.CreatedAt, "Z") || parsedCreatedAt.UTC() != parsedCreatedAt {
			t.Fatalf("expected UTC createdAt, got %q", application.CreatedAt)
		}
	}
}

func TestSyncDeltaOwnerIsolation(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnvWithOptions(t, config.DeploymentModeSelfHosted, true)
	firstFixture := seedCommittedSyncDeltaMetadataFixture(t, env)
	secondCookie := createAuthenticatedUserSession(t, env, "second-owner")
	secondUser := authenticatedUserFromCookie(t, env, secondCookie)
	secondFixture := seedCommittedSyncDeltaMetadataFixtureForScope(t, env, secondUser.OwnerKey, firstFixture.AppID, "second-owner", "owner-2-tab-1", "owner-2-tab-2")

	req := httptest.NewRequest(http.MethodGet, "/api/sync/delta?appId="+firstFixture.AppID+"&sinceVersion=1&includeApplications=true", nil)
	req.AddCookie(env.cookie)
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp syncDeltaResponse
	decodeResponse(t, rec, &resp)

	if resp.CurrentDocumentVersion != firstFixture.CurrentDocument.Version || resp.CurrentDocumentHash != hashSyncDeltaTestBody(firstFixture.BodyAfter) {
		t.Fatalf("expected first owner document metadata, got %+v", resp)
	}
	if resp.CurrentDocumentHash == hashSyncDeltaTestBody(secondFixture.BodyAfter) {
		t.Fatalf("unexpected second owner document hash exposed: %+v", resp)
	}
	for _, application := range resp.Applications {
		if strings.HasPrefix(application.MutationID, "second-owner") {
			t.Fatalf("unexpected second owner application metadata exposed: %+v", resp.Applications)
		}
	}
	assertNoStore(t, rec)
}

func TestSyncDeltaAppIsolation(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnvWithOptions(t, config.DeploymentModeSelfHosted, true)
	user := authenticatedUserFromCookie(t, env, env.cookie)
	firstFixture := seedCommittedSyncDeltaMetadataFixtureForScope(t, env, user.OwnerKey, "postbaby-web", "app-one", "app-1-tab-1", "app-1-tab-2")
	secondFixture := seedCommittedSyncDeltaMetadataFixtureForScope(t, env, user.OwnerKey, "postbaby-mobile", "app-two", "app-2-tab-1", "app-2-tab-2")

	req := httptest.NewRequest(http.MethodGet, "/api/sync/delta?appId=postbaby-web&sinceVersion=1&includeApplications=true", nil)
	req.AddCookie(env.cookie)
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	var resp syncDeltaResponse
	decodeResponse(t, rec, &resp)

	if resp.CurrentDocumentVersion != firstFixture.CurrentDocument.Version || resp.CurrentDocumentHash != hashSyncDeltaTestBody(firstFixture.BodyAfter) {
		t.Fatalf("expected first app document metadata, got %+v", resp)
	}
	if resp.CurrentDocumentHash == hashSyncDeltaTestBody(secondFixture.BodyAfter) {
		t.Fatalf("unexpected second app document hash exposed: %+v", resp)
	}
	for _, application := range resp.Applications {
		if strings.HasPrefix(application.MutationID, "app-two") {
			t.Fatalf("unexpected second app application metadata exposed: %+v", resp.Applications)
		}
	}
	assertNoStore(t, rec)
}

func TestSyncDeltaUnknownAppReturnsDocumentNotFound(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnvWithOptions(t, config.DeploymentModeSelfHosted, true)
	req := httptest.NewRequest(http.MethodGet, "/api/sync/delta?appId=unknown-app&sinceVersion=0", nil)
	req.AddCookie(env.cookie)
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	assertErrorResponse(t, rec, http.StatusNotFound, "document_not_found")
	assertNoStore(t, rec)
}

func TestSyncDeltaIsReadOnlyAcrossDocumentReceiptsApplicationsAndObservations(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnvWithOptions(t, config.DeploymentModeSelfHosted, true)
	fixture := seedCommittedSyncDeltaMetadataFixture(t, env)
	beforeState := snapshotSyncDeltaAPIState(t, env, fixture.OwnerKey, fixture.AppID)

	req := httptest.NewRequest(http.MethodGet, "/api/sync/delta?appId="+fixture.AppID+"&sinceVersion=3&includeApplications=true", nil)
	req.AddCookie(env.cookie)
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	afterState := snapshotSyncDeltaAPIState(t, env, fixture.OwnerKey, fixture.AppID)
	assertSyncDeltaAPIStateEqual(t, beforeState, afterState)
	assertNoStore(t, rec)
}

func TestSyncDeltaClientAheadIsReadOnly(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnvWithOptions(t, config.DeploymentModeSelfHosted, true)
	user := authenticatedUserFromCookie(t, env, env.cookie)
	appID := "postbaby-web"
	body := mustMarshalSyncDeltaSnapshotBody(t, snapshot("tab-1", "[]"))
	if _, err := env.store.PutDocument(context.Background(), user.OwnerKey, appID, body, nil); err != nil {
		t.Fatalf("seed document: %v", err)
	}
	beforeState := snapshotSyncDeltaAPIState(t, env, user.OwnerKey, appID)

	req := httptest.NewRequest(http.MethodGet, "/api/sync/delta?appId="+appID+"&sinceVersion=2", nil)
	req.AddCookie(env.cookie)
	rec := httptest.NewRecorder()
	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	afterState := snapshotSyncDeltaAPIState(t, env, user.OwnerKey, appID)
	assertSyncDeltaAPIStateEqual(t, beforeState, afterState)
}

func TestSyncDeltaDocumentVersionChangedIsReadOnly(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnvWithOptions(t, config.DeploymentModeSelfHosted, true)
	user := authenticatedUserFromCookie(t, env, env.cookie)
	appID := "postbaby-web"
	bodyBefore := mustMarshalSyncDeltaSnapshotBody(t, snapshot("tab-1", "[]"))
	docBefore, err := env.store.PutDocument(context.Background(), user.OwnerKey, appID, bodyBefore, nil)
	if err != nil {
		t.Fatalf("seed initial document: %v", err)
	}
	bodyAfter := mustMarshalSyncDeltaSnapshotBody(t, snapshot("tab-2", "[]"))
	expectedVersion := docBefore.Version
	if _, err := env.store.PutDocument(context.Background(), user.OwnerKey, appID, bodyAfter, &expectedVersion); err != nil {
		t.Fatalf("update document: %v", err)
	}
	beforeState := snapshotSyncDeltaAPIState(t, env, user.OwnerKey, appID)

	req := httptest.NewRequest(http.MethodGet, "/api/sync/delta?appId="+appID+"&sinceVersion=1", nil)
	req.AddCookie(env.cookie)
	rec := httptest.NewRecorder()
	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	afterState := snapshotSyncDeltaAPIState(t, env, user.OwnerKey, appID)
	assertSyncDeltaAPIStateEqual(t, beforeState, afterState)
}

func TestSyncDeltaTooManyApplicationsIsReadOnly(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnvWithOptions(t, config.DeploymentModeSelfHosted, true)
	fixture := seedCommittedSyncDeltaMetadataFixture(t, env)
	beforeState := snapshotSyncDeltaAPIState(t, env, fixture.OwnerKey, fixture.AppID)

	req := httptest.NewRequest(http.MethodGet, "/api/sync/delta?appId="+fixture.AppID+"&sinceVersion=1&includeApplications=true&limit=1", nil)
	req.AddCookie(env.cookie)
	rec := httptest.NewRecorder()
	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	afterState := snapshotSyncDeltaAPIState(t, env, fixture.OwnerKey, fixture.AppID)
	assertSyncDeltaAPIStateEqual(t, beforeState, afterState)
}

func TestSyncDeltaValidationFailureIsReadOnly(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnvWithOptions(t, config.DeploymentModeSelfHosted, true)
	fixture := seedCommittedSyncDeltaMetadataFixture(t, env)
	beforeState := snapshotSyncDeltaAPIState(t, env, fixture.OwnerKey, fixture.AppID)

	req := httptest.NewRequest(http.MethodGet, "/api/sync/delta?appId="+fixture.AppID+"&sinceVersion=1&limit=abc", nil)
	req.AddCookie(env.cookie)
	rec := httptest.NewRecorder()
	env.handler.ServeHTTP(rec, req)

	assertErrorResponse(t, rec, http.StatusBadRequest, "invalid_request")

	afterState := snapshotSyncDeltaAPIState(t, env, fixture.OwnerKey, fixture.AppID)
	assertSyncDeltaAPIStateEqual(t, beforeState, afterState)
}

func newTestEnv(t *testing.T, deploymentMode config.DeploymentMode) *testEnv {
	return newTestEnvWithOptions(t, deploymentMode, false)
}

func newTestEnvWithOptions(t *testing.T, deploymentMode config.DeploymentMode, enableSyncDeltaMetadata bool) *testEnv {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "postbaby-api-test.db")
	docStore, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open test store: %v", err)
	}

	t.Cleanup(func() {
		if err := docStore.Close(); err != nil {
			t.Fatalf("close test store: %v", err)
		}
	})

	authManager := auth.NewManager(docStore, auth.Options{})
	entitlementManager := entitlement.NewManager(docStore)
	return &testEnv{
		handler:     NewHandler(docStore, authManager, entitlementManager, deploymentMode, enableSyncDeltaMetadata),
		store:       docStore,
		authManager: authManager,
		dbPath:      dbPath,
	}
}

func newAuthenticatedTestEnv(t *testing.T, deploymentMode config.DeploymentMode) *testEnv {
	return newAuthenticatedTestEnvWithOptions(t, deploymentMode, false)
}

func newAuthenticatedTestEnvWithOptions(t *testing.T, deploymentMode config.DeploymentMode, enableSyncDeltaMetadata bool) *testEnv {
	t.Helper()

	env := newTestEnvWithOptions(t, deploymentMode, enableSyncDeltaMetadata)
	user, err := env.authManager.CreateInitialUser(context.Background(), "owner", testPassword)
	if err != nil {
		t.Fatalf("create initial user: %v", err)
	}

	rec := httptest.NewRecorder()
	if err := env.authManager.CreateSession(context.Background(), rec, user.ID); err != nil {
		t.Fatalf("create session: %v", err)
	}

	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected session cookie")
	}

	env.cookie = cookies[0]
	return env
}

func createAuthenticatedUserSession(t *testing.T, env *testEnv, username string) *http.Cookie {
	t.Helper()

	user, err := env.authManager.CreateUser(context.Background(), username, testPassword)
	if err != nil {
		t.Fatalf("create hosted user: %v", err)
	}

	rec := httptest.NewRecorder()
	if err := env.authManager.CreateSession(context.Background(), rec, user.ID); err != nil {
		t.Fatalf("create session: %v", err)
	}

	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected session cookie")
	}

	return cookies[0]
}

func authenticatedUserFromCookie(t *testing.T, env *testEnv, cookie *http.Cookie) *store.User {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	user, err := env.authManager.AuthenticateRequest(context.Background(), rec, req)
	if err != nil {
		t.Fatalf("authenticate user from cookie: %v", err)
	}

	return user
}

func grantHostedSyncEntitlement(t *testing.T, env *testEnv, cookie *http.Cookie) {
	t.Helper()
	setHostedSyncEntitlementStatus(t, env, cookie, store.EntitlementStatusActive)
}

func setHostedSyncEntitlementStatus(t *testing.T, env *testEnv, cookie *http.Cookie, status string) {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	authenticatedUser, err := env.authManager.AuthenticateRequest(context.Background(), rec, req)
	if err != nil {
		t.Fatalf("authenticate user from session cookie: %v", err)
	}

	if _, err := env.store.PutAccountEntitlement(
		context.Background(),
		authenticatedUser.ID,
		store.EntitlementKeyHostedSync,
		status,
		store.EntitlementSourceManual,
		nil,
	); err != nil {
		t.Fatalf("set hosted sync entitlement: %v", err)
	}
}

func performJSONRequest(t *testing.T, handler http.Handler, cookie *http.Cookie, method, target string, payload any) *httptest.ResponseRecorder {
	t.Helper()

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	req := httptest.NewRequest(method, target, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		req.AddCookie(cookie)
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func decodeResponse(t *testing.T, rec *httptest.ResponseRecorder, dest any) {
	t.Helper()

	body, err := io.ReadAll(rec.Result().Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}

	if err := json.Unmarshal(body, dest); err != nil {
		t.Fatalf("decode response %q: %v", string(body), err)
	}
}

func assertErrorResponse(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantCode string) {
	t.Helper()

	if rec.Code != wantStatus {
		t.Fatalf("expected status %d, got %d", wantStatus, rec.Code)
	}

	var resp struct {
		OK    bool `json:"ok"`
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	decodeResponse(t, rec, &resp)

	if resp.OK {
		t.Fatal("expected ok=false")
	}

	if resp.Error.Code != wantCode {
		t.Fatalf("expected error code %q, got %q", wantCode, resp.Error.Code)
	}
}

func assertVersionConflictResponse(t *testing.T, rec *httptest.ResponseRecorder, wantCurrentVersion int64) {
	t.Helper()

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected status %d, got %d", http.StatusConflict, rec.Code)
	}

	var resp struct {
		OK    bool `json:"ok"`
		Error struct {
			Code           string `json:"code"`
			CurrentVersion *int64 `json:"currentVersion"`
		} `json:"error"`
	}
	decodeResponse(t, rec, &resp)

	if resp.OK {
		t.Fatal("expected ok=false")
	}
	if resp.Error.Code != "version_conflict" {
		t.Fatalf("expected error code %q, got %q", "version_conflict", resp.Error.Code)
	}
	if resp.Error.CurrentVersion == nil {
		t.Fatal("expected currentVersion in conflict response")
	}
	if *resp.Error.CurrentVersion != wantCurrentVersion {
		t.Fatalf("expected currentVersion %d, got %d", wantCurrentVersion, *resp.Error.CurrentVersion)
	}
}

func assertNoStore(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()

	if cacheControl := rec.Header().Get("Cache-Control"); cacheControl != "no-store" {
		t.Fatalf("expected Cache-Control no-store, got %q", cacheControl)
	}
}

func assertJSONEqual(t *testing.T, want any, got json.RawMessage) {
	t.Helper()

	wantBody, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal expected JSON: %v", err)
	}

	var wantValue any
	if err := json.Unmarshal(wantBody, &wantValue); err != nil {
		t.Fatalf("unmarshal expected JSON: %v", err)
	}

	var gotValue any
	if err := json.Unmarshal(got, &gotValue); err != nil {
		t.Fatalf("unmarshal actual JSON: %v", err)
	}

	if !deepEqualJSON(wantValue, gotValue) {
		t.Fatalf("expected JSON %s, got %s", string(wantBody), string(got))
	}
}

func deepEqualJSON(left, right any) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	if leftErr != nil || rightErr != nil {
		return false
	}

	return bytes.Equal(leftJSON, rightJSON)
}

func snapshot(activeTabID, tabs string) map[string]string {
	return map[string]string{
		"tabs":        tabs,
		"activeTabId": activeTabID,
	}
}

func buildSyncMutationEnvelope(mutationID, operationType string, payload any) map[string]any {
	return map[string]any{
		"protocol":      "PB-SYNC/1",
		"clientId":      "client-1",
		"deviceId":      "device-1",
		"mutationId":    mutationID,
		"baseRevision":  6,
		"entityType":    "Node",
		"entityId":      "item-1",
		"operationType": operationType,
		"payload":       payload,
	}
}

type syncDeltaResponse struct {
	OK                       bool                        `json:"ok"`
	AppID                    string                      `json:"appId"`
	CurrentDocumentVersion   int64                       `json:"currentDocumentVersion"`
	CurrentDocumentHash      string                      `json:"currentDocumentHash"`
	ClientVersion            int64                       `json:"clientVersion"`
	RequiresSnapshotRefresh  bool                        `json:"requiresSnapshotRefresh"`
	Reason                   string                      `json:"reason"`
	ApplicationWatermark     *int64                      `json:"applicationWatermark"`
	NextApplicationWatermark *int64                      `json:"nextApplicationWatermark"`
	Applications             []syncDeltaApplicationEntry `json:"applications"`
	Warnings                 []string                    `json:"warnings"`
}

type syncDeltaApplicationEntry struct {
	MutationID                     string  `json:"mutationId"`
	ApplicationStatus              string  `json:"applicationStatus"`
	ApplicationReason              string  `json:"applicationReason"`
	CanonicalDocumentVersionBefore int64   `json:"canonicalDocumentVersionBefore"`
	CanonicalDocumentVersionAfter  *int64  `json:"canonicalDocumentVersionAfter"`
	CanonicalDocumentHashBefore    string  `json:"canonicalDocumentHashBefore"`
	CanonicalDocumentHashAfter     *string `json:"canonicalDocumentHashAfter"`
	ReplayObservationID            *int64  `json:"replayObservationId"`
	CreatedAt                      string  `json:"createdAt"`
}

type syncDeltaCommittedMetadataFixture struct {
	OwnerKey           string
	AppID              string
	BodyBefore         json.RawMessage
	BodyAfter          json.RawMessage
	CurrentDocument    store.Document
	Observation        store.SyncMutationReplayDryRunObservation
	AppliedMutationID  string
	SkippedMutationID  string
	ConflictMutationID string
	AppliedApplication store.SyncMutationReplayApplication
	SkippedApplication store.SyncMutationReplayApplication
}

type syncDeltaAPIStateSnapshot struct {
	Document     store.Document
	Applications []store.SyncMutationReplayApplication
	Receipts     []syncDeltaAPIReceiptRow
	Observations []store.SyncMutationReplayDryRunObservation
}

type syncDeltaAPIReceiptRow struct {
	ID            int64
	OwnerKey      string
	AppID         string
	MutationID    string
	ClientID      string
	DeviceID      string
	Protocol      string
	EntityType    string
	EntityID      string
	OperationType string
	PayloadJSON   string
	BaseRevision  *int64
	Status        string
	CreatedAt     string
	AcceptedAt    string
}

func mustMarshalSyncDeltaSnapshotBody(t *testing.T, value any) json.RawMessage {
	t.Helper()

	body, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal sync delta snapshot body: %v", err)
	}
	return json.RawMessage(body)
}

func hashSyncDeltaTestBody(body json.RawMessage) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func buildSyncDeltaReceiptInput(mutationID string) store.SyncMutationReceiptInput {
	baseRevision := int64(6)
	return store.SyncMutationReceiptInput{
		MutationID:    mutationID,
		ClientID:      "client-1",
		DeviceID:      "device-1",
		Protocol:      "PB-SYNC/1",
		EntityType:    "Node",
		EntityID:      "item-" + mutationID,
		OperationType: "CreateNode",
		Payload:       json.RawMessage(`{"tabId":"tab-1","name":"Seeded Node","position":{"top":"20px","left":"20px"}}`),
		BaseRevision:  &baseRevision,
	}
}

func seedCommittedSyncDeltaMetadataFixture(t *testing.T, env *testEnv) syncDeltaCommittedMetadataFixture {
	t.Helper()

	user := authenticatedUserFromCookie(t, env, env.cookie)
	return seedCommittedSyncDeltaMetadataFixtureForScope(t, env, user.OwnerKey, "postbaby-web", "default", "tab-1", "tab-2")
}

func seedCommittedSyncDeltaMetadataFixtureForScope(t *testing.T, env *testEnv, ownerKey, appID, mutationPrefix, activeTabBefore, activeTabAfter string) syncDeltaCommittedMetadataFixture {
	t.Helper()

	ctx := context.Background()
	bodyBefore := mustMarshalSyncDeltaSnapshotBody(t, snapshot(activeTabBefore, "[]"))
	docBefore, err := env.store.PutDocument(ctx, ownerKey, appID, bodyBefore, nil)
	if err != nil {
		t.Fatalf("seed initial document: %v", err)
	}

	bodyAfter := mustMarshalSyncDeltaSnapshotBody(t, snapshot(activeTabAfter, "[]"))
	expectedVersion := docBefore.Version
	docAfter, err := env.store.PutDocument(ctx, ownerKey, appID, bodyAfter, &expectedVersion)
	if err != nil {
		t.Fatalf("seed changed document: %v", err)
	}

	expectedUnchangedVersion := docAfter.Version
	currentDoc, err := env.store.PutDocument(ctx, ownerKey, appID, bodyAfter, &expectedUnchangedVersion)
	if err != nil {
		t.Fatalf("seed body-unchanged version bump: %v", err)
	}

	appliedMutationID := mutationPrefix + "-mut-applied"
	skippedMutationID := mutationPrefix + "-mut-skipped"
	conflictMutationID := mutationPrefix + "-mut-conflict"
	receiptInputs := []store.SyncMutationReceiptInput{
		buildSyncDeltaReceiptInput(appliedMutationID),
		buildSyncDeltaReceiptInput(skippedMutationID),
		buildSyncDeltaReceiptInput(conflictMutationID),
	}
	if _, err := env.store.AcceptSyncMutationReceipts(ctx, ownerKey, appID, receiptInputs); err != nil {
		t.Fatalf("seed sync mutation receipts: %v", err)
	}

	observation, err := env.store.RecordSyncMutationReplayDryRunObservation(ctx, ownerKey, appID)
	if err != nil {
		t.Fatalf("record sync delta dry-run observation: %v", err)
	}

	hashBefore := hashSyncDeltaTestBody(bodyBefore)
	hashAfter := hashSyncDeltaTestBody(bodyAfter)
	observationID := observation.ID
	docAfterVersion := docAfter.Version
	currentDocVersion := currentDoc.Version

	appliedResult, err := env.store.RecordSyncMutationReplayApplicationInert(ctx, ownerKey, appID, store.SyncMutationReplayApplicationInput{
		MutationID:                     appliedMutationID,
		ApplicationStatus:              store.SyncMutationReplayApplicationStatusApplied,
		ApplicationReason:              "policy_allowed",
		CanonicalDocumentVersionBefore: docBefore.Version,
		CanonicalDocumentHashBefore:    hashBefore,
		CanonicalDocumentVersionAfter:  &docAfterVersion,
		CanonicalDocumentHashAfter:     stringPointer(hashAfter),
		ReplayObservationID:            &observationID,
	})
	if err != nil {
		t.Fatalf("record applied replay application: %v", err)
	}

	skippedResult, err := env.store.RecordSyncMutationReplayApplicationInert(ctx, ownerKey, appID, store.SyncMutationReplayApplicationInput{
		MutationID:                     skippedMutationID,
		ApplicationStatus:              store.SyncMutationReplayApplicationStatusSkipped,
		ApplicationReason:              "policy_skip_already_reflected",
		CanonicalDocumentVersionBefore: docAfter.Version,
		CanonicalDocumentHashBefore:    hashAfter,
		CanonicalDocumentVersionAfter:  &currentDocVersion,
		CanonicalDocumentHashAfter:     stringPointer(hashAfter),
		ReplayObservationID:            &observationID,
	})
	if err != nil {
		t.Fatalf("record skipped replay application: %v", err)
	}

	if _, err := env.store.RecordSyncMutationReplayApplicationInert(ctx, ownerKey, appID, store.SyncMutationReplayApplicationInput{
		MutationID:                     conflictMutationID,
		ApplicationStatus:              store.SyncMutationReplayApplicationStatusConflict,
		ApplicationReason:              "diagnostic_conflict_only",
		CanonicalDocumentVersionBefore: currentDoc.Version,
		CanonicalDocumentHashBefore:    hashAfter,
		ReplayObservationID:            &observationID,
	}); err != nil {
		t.Fatalf("record diagnostic-only replay application: %v", err)
	}

	return syncDeltaCommittedMetadataFixture{
		OwnerKey:           ownerKey,
		AppID:              appID,
		BodyBefore:         bodyBefore,
		BodyAfter:          bodyAfter,
		CurrentDocument:    currentDoc,
		Observation:        observation,
		AppliedMutationID:  appliedMutationID,
		SkippedMutationID:  skippedMutationID,
		ConflictMutationID: conflictMutationID,
		AppliedApplication: appliedResult.Application,
		SkippedApplication: skippedResult.Application,
	}
}

func snapshotSyncDeltaAPIState(t *testing.T, env *testEnv, ownerKey, appID string) syncDeltaAPIStateSnapshot {
	t.Helper()

	document, err := env.store.GetDocument(context.Background(), ownerKey, appID)
	if err != nil {
		t.Fatalf("load sync delta API document snapshot: %v", err)
	}
	applications, err := env.store.ListSyncMutationReplayApplications(context.Background(), ownerKey, appID)
	if err != nil {
		t.Fatalf("list replay applications snapshot: %v", err)
	}

	return syncDeltaAPIStateSnapshot{
		Document:     document,
		Applications: applications,
		Receipts:     loadSyncDeltaAPIReceiptRows(t, env.dbPath, ownerKey, appID),
		Observations: loadSyncDeltaAPIObservationRows(t, env.dbPath, ownerKey, appID),
	}
}

func assertSyncDeltaAPIStateEqual(t *testing.T, before, after syncDeltaAPIStateSnapshot) {
	t.Helper()

	if before.Document.ID != after.Document.ID ||
		before.Document.OwnerKey != after.Document.OwnerKey ||
		before.Document.AppID != after.Document.AppID ||
		string(before.Document.Body) != string(after.Document.Body) ||
		before.Document.Version != after.Document.Version ||
		!before.Document.UpdatedAt.Equal(after.Document.UpdatedAt) {
		t.Fatalf("document changed during sync delta API read\nbefore=%+v\nafter=%+v", before.Document, after.Document)
	}
	if !reflect.DeepEqual(before.Applications, after.Applications) {
		t.Fatalf("replay application rows changed during sync delta API read\nbefore=%+v\nafter=%+v", before.Applications, after.Applications)
	}
	if !reflect.DeepEqual(before.Receipts, after.Receipts) {
		t.Fatalf("receipt rows changed during sync delta API read\nbefore=%+v\nafter=%+v", before.Receipts, after.Receipts)
	}
	if !reflect.DeepEqual(before.Observations, after.Observations) {
		t.Fatalf("observation rows changed during sync delta API read\nbefore=%+v\nafter=%+v", before.Observations, after.Observations)
	}
}

func loadSyncDeltaAPIReceiptRows(t *testing.T, dbPath, ownerKey, appID string) []syncDeltaAPIReceiptRow {
	t.Helper()

	db := openSyncDeltaAPIRawDB(t, dbPath)
	defer func() {
		if err := db.Close(); err != nil {
			t.Fatalf("close sync delta API raw sqlite connection: %v", err)
		}
	}()

	rows, err := db.Query(
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
		t.Fatalf("query sync delta API receipt rows: %v", err)
	}
	defer rows.Close()

	result := make([]syncDeltaAPIReceiptRow, 0)
	for rows.Next() {
		var row syncDeltaAPIReceiptRow
		var baseRevision sql.NullInt64
		if err := rows.Scan(
			&row.ID,
			&row.OwnerKey,
			&row.AppID,
			&row.MutationID,
			&row.ClientID,
			&row.DeviceID,
			&row.Protocol,
			&row.EntityType,
			&row.EntityID,
			&row.OperationType,
			&row.PayloadJSON,
			&baseRevision,
			&row.Status,
			&row.CreatedAt,
			&row.AcceptedAt,
		); err != nil {
			t.Fatalf("scan sync delta API receipt row: %v", err)
		}
		if baseRevision.Valid {
			row.BaseRevision = &baseRevision.Int64
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate sync delta API receipt rows: %v", err)
	}

	return result
}

func loadSyncDeltaAPIObservationRows(t *testing.T, dbPath, ownerKey, appID string) []store.SyncMutationReplayDryRunObservation {
	t.Helper()

	db := openSyncDeltaAPIRawDB(t, dbPath)
	defer func() {
		if err := db.Close(); err != nil {
			t.Fatalf("close sync delta API raw sqlite connection: %v", err)
		}
	}()

	rows, err := db.Query(
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
		ORDER BY id ASC`,
		ownerKey,
		appID,
	)
	if err != nil {
		t.Fatalf("query sync delta API observation rows: %v", err)
	}
	defer rows.Close()

	result := make([]store.SyncMutationReplayDryRunObservation, 0)
	for rows.Next() {
		var row store.SyncMutationReplayDryRunObservation
		var createdAt string
		if err := rows.Scan(
			&row.ID,
			&row.OwnerKey,
			&row.AppID,
			&row.CanonicalDocumentVersionObserved,
			&row.CanonicalDocumentHashObserved,
			&row.ReceiptCountConsidered,
			&row.FirstOrderedMutationID,
			&row.LastOrderedMutationID,
			&row.OrderedReceiptHighWatermark,
			&row.AppliedCount,
			&row.SkippedCount,
			&row.WarningCount,
			&row.PreviewHash,
			&createdAt,
		); err != nil {
			t.Fatalf("scan sync delta API observation row: %v", err)
		}
		parsedCreatedAt, err := time.Parse(time.RFC3339, createdAt)
		if err != nil {
			t.Fatalf("parse sync delta API observation created_at %q: %v", createdAt, err)
		}
		row.CreatedAt = parsedCreatedAt.UTC()
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate sync delta API observation rows: %v", err)
	}

	return result
}

func openSyncDeltaAPIRawDB(t *testing.T, dbPath string) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sync delta API raw sqlite connection: %v", err)
	}
	return db
}

func stringPointer(value string) *string {
	return &value
}

func containsSyncDeltaString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
