package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"postbaby-backend/internal/auth"
	"postbaby-backend/internal/config"
	"postbaby-backend/internal/entitlement"
	"postbaby-backend/internal/store"
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

func newTestEnv(t *testing.T, deploymentMode config.DeploymentMode) *testEnv {
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
		handler:     NewHandler(docStore, authManager, entitlementManager, deploymentMode),
		store:       docStore,
		authManager: authManager,
	}
}

func newAuthenticatedTestEnv(t *testing.T, deploymentMode config.DeploymentMode) *testEnv {
	t.Helper()

	env := newTestEnv(t, deploymentMode)
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
