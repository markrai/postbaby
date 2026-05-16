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
	"postbaby-backend/internal/store"
)

const testPassword = "correct-horse-battery"

func TestHealthReturnsOK(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t, config.DeploymentModeSelfHostedSingleUser)
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

	env := newTestEnv(t, config.DeploymentModeSelfHostedSingleUser)
	req := httptest.NewRequest(http.MethodGet, "/api/document?appId=demo", nil)
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	assertErrorResponse(t, rec, http.StatusUnauthorized, "unauthorized")
	assertNoStore(t, rec)
}

func TestDocumentRejectsInvalidSessionCookie(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t, config.DeploymentModeSelfHostedSingleUser)
	req := httptest.NewRequest(http.MethodGet, "/api/document?appId=demo", nil)
	req.AddCookie(&http.Cookie{Name: "postbaby_session", Value: "bogus"})
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	assertErrorResponse(t, rec, http.StatusUnauthorized, "unauthorized")
}

func TestDocumentMetaRequiresAuthentication(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t, config.DeploymentModeSelfHostedSingleUser)
	req := httptest.NewRequest(http.MethodGet, "/api/document/meta?appId=demo", nil)
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	assertErrorResponse(t, rec, http.StatusUnauthorized, "unauthorized")
	assertNoStore(t, rec)
}

func TestDocumentMetaRequiresAppID(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnv(t, config.DeploymentModeSelfHostedSingleUser)
	req := httptest.NewRequest(http.MethodGet, "/api/document/meta", nil)
	req.AddCookie(env.cookie)
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	assertErrorResponse(t, rec, http.StatusBadRequest, "invalid_request")
}

func TestDocumentMetaRejectsWrongMethod(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnv(t, config.DeploymentModeSelfHostedSingleUser)
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

	env := newAuthenticatedTestEnv(t, config.DeploymentModeSelfHostedSingleUser)
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

	env := newAuthenticatedTestEnv(t, config.DeploymentModeSelfHostedSingleUser)
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

	env := newAuthenticatedTestEnv(t, config.DeploymentModeSelfHostedSingleUser)
	req := httptest.NewRequest(http.MethodGet, "/api/document", nil)
	req.AddCookie(env.cookie)
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	assertErrorResponse(t, rec, http.StatusBadRequest, "invalid_request")
}

func TestPutDocumentRejectsInvalidJSON(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnv(t, config.DeploymentModeSelfHostedSingleUser)
	req := httptest.NewRequest(http.MethodPut, "/api/document", strings.NewReader(`{"appId":"demo","data":`))
	req.AddCookie(env.cookie)
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	assertErrorResponse(t, rec, http.StatusBadRequest, "invalid_json")
}

func TestPutDocumentRejectsOversizedBody(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnv(t, config.DeploymentModeSelfHostedSingleUser)
	oversized := `{"appId":"demo","data":"` + strings.Repeat("a", int(MaxDocumentBodyBytes)) + `"}`
	req := httptest.NewRequest(http.MethodPut, "/api/document", strings.NewReader(oversized))
	req.AddCookie(env.cookie)
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	assertErrorResponse(t, rec, http.StatusRequestEntityTooLarge, "request_too_large")
}

func TestPutDocumentRejectsNonFrontendSnapshotData(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnv(t, config.DeploymentModeSelfHostedSingleUser)
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

	env := newAuthenticatedTestEnv(t, config.DeploymentModeSelfHostedSingleUser)
	req := httptest.NewRequest(http.MethodGet, "/api/document?appId=missing", nil)
	req.AddCookie(env.cookie)
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	assertErrorResponse(t, rec, http.StatusNotFound, "document_not_found")
}

func TestPutDocumentCreatesDocument(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnv(t, config.DeploymentModeSelfHostedSingleUser)
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

func TestPutDocumentOverwritesWithoutVersion(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnv(t, config.DeploymentModeSelfHostedSingleUser)
	performJSONRequest(t, env.handler, env.cookie, http.MethodPut, "/api/document", map[string]any{
		"appId": "demo",
		"data":  snapshot("tab-1", "[]"),
	})

	rec := performJSONRequest(t, env.handler, env.cookie, http.MethodPut, "/api/document", map[string]any{
		"appId": "demo",
		"data":  snapshot("tab-2", "[]"),
	})

	var resp struct {
		Version int64 `json:"version"`
	}
	decodeResponse(t, rec, &resp)
	if resp.Version != 2 {
		t.Fatalf("expected version 2, got %d", resp.Version)
	}
}

func TestPutDocumentSupportsOptimisticLocking(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnv(t, config.DeploymentModeSelfHostedSingleUser)
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
	if secondResp.Version != 2 {
		t.Fatalf("expected version 2, got %d", secondResp.Version)
	}
}

func TestPutDocumentReturnsConflictForMismatchedVersion(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnv(t, config.DeploymentModeSelfHostedSingleUser)
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

func TestDocumentLoadAfterSavePreservesJSON(t *testing.T) {
	t.Parallel()

	env := newAuthenticatedTestEnv(t, config.DeploymentModeSelfHostedSingleUser)
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

type testEnv struct {
	handler     http.Handler
	store       *store.Store
	authManager *auth.Manager
	cookie      *http.Cookie
}

func TestDocumentRoutesReturnNotFoundWhenSyncDisabled(t *testing.T) {
	t.Parallel()

	env := newTestEnv(t, config.DeploymentModeStaticLocal)

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
	return &testEnv{
		handler:     NewHandler(docStore, authManager, deploymentMode),
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
