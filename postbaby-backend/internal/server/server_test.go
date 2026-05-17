package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"postbaby-backend/internal/auth"
	"postbaby-backend/internal/config"
	"postbaby-backend/internal/httpapi"
	"postbaby-backend/internal/store"
)

const serverTestPassword = "correct-horse-battery"

func TestNewHandlerServesRootWithoutAuthInStaticLocalMode(t *testing.T) {
	t.Parallel()

	env := newServerTestEnv(t, config.DeploymentModeStaticLocal)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected / to serve app shell, got %d", rec.Code)
	}
	assertNoStore(t, rec)
}

func TestNewHandlerServesAppShellWithoutAuthInCloudMultiUserMode(t *testing.T) {
	t.Parallel()

	env := newServerTestEnv(t, config.DeploymentModeCloudMultiUser)

	for _, target := range []string{"/", "/index.html"} {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		rec := httptest.NewRecorder()

		env.handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected %s to serve app shell, got %d", target, rec.Code)
		}
		assertNoStore(t, rec)
	}
}

func TestNewHandlerServesStaticAssetsWithoutAuth(t *testing.T) {
	t.Parallel()

	env := newServerTestEnv(t, config.DeploymentModeStaticLocal)
	req := httptest.NewRequest(http.MethodGet, "/css/style.css", nil)
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected static asset to return 200, got %d", rec.Code)
	}
}

func TestRuntimeConfigServedWithoutAuth(t *testing.T) {
	t.Parallel()

	env := newServerTestEnv(t, config.DeploymentModeStaticLocal)
	req := httptest.NewRequest(http.MethodGet, "/runtime-config.js", nil)
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected runtime config status 200, got %d", rec.Code)
	}
	if contentType := rec.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "application/javascript") {
		t.Fatalf("expected javascript content type, got %q", contentType)
	}

	body := rec.Body.String()
	for _, want := range []string{
		`"deploymentMode":"static_local"`,
		`"authAvailable":false`,
		`"authRequired":false`,
		`"isAuthenticated":false`,
		`"syncAvailable":false`,
		`"syncRequiresAuth":false`,
		`"setupAvailable":false`,
		`"apiBase":""`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected runtime config body to contain %q, got %q", want, body)
		}
	}

	assertNoStore(t, rec)
}

func TestRuntimeConfigServedWithoutAuthInCloudMultiUserMode(t *testing.T) {
	t.Parallel()

	env := newServerTestEnv(t, config.DeploymentModeCloudMultiUser)
	req := httptest.NewRequest(http.MethodGet, "/runtime-config.js", nil)
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected runtime config status 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	for _, want := range []string{
		`"deploymentMode":"cloud_multi_user"`,
		`"authAvailable":true`,
		`"authRequired":false`,
		`"isAuthenticated":false`,
		`"syncAvailable":true`,
		`"syncRequiresAuth":true`,
		`"setupAvailable":false`,
		`"apiBase":""`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected runtime config body to contain %q, got %q", want, body)
		}
	}

	assertNoStore(t, rec)
}

func TestRuntimeConfigReflectsAuthenticationStateInCloudMultiUserMode(t *testing.T) {
	t.Parallel()

	env := newServerTestEnv(t, config.DeploymentModeCloudMultiUser)
	user := createHostedUser(t, env, "cloud-user")

	unauthReq := httptest.NewRequest(http.MethodGet, "/runtime-config.js", nil)
	unauthRec := httptest.NewRecorder()
	env.handler.ServeHTTP(unauthRec, unauthReq)

	if unauthRec.Code != http.StatusOK {
		t.Fatalf("expected unauthenticated runtime config status 200, got %d", unauthRec.Code)
	}

	unauthBody := unauthRec.Body.String()
	for _, want := range []string{
		`"deploymentMode":"cloud_multi_user"`,
		`"authAvailable":true`,
		`"authRequired":false`,
		`"isAuthenticated":false`,
		`"syncAvailable":true`,
		`"syncRequiresAuth":true`,
		`"setupAvailable":false`,
		`"apiBase":""`,
	} {
		if !strings.Contains(unauthBody, want) {
			t.Fatalf("expected unauthenticated cloud runtime config body to contain %q, got %q", want, unauthBody)
		}
	}

	authReq := httptest.NewRequest(http.MethodGet, "/runtime-config.js", nil)
	authReq.AddCookie(createSessionCookie(t, env, user.ID))
	authRec := httptest.NewRecorder()
	env.handler.ServeHTTP(authRec, authReq)

	if authRec.Code != http.StatusOK {
		t.Fatalf("expected authenticated cloud runtime config status 200, got %d", authRec.Code)
	}
	if body := authRec.Body.String(); !strings.Contains(body, `"isAuthenticated":true`) {
		t.Fatalf("expected authenticated cloud runtime config body to contain isAuthenticated true, got %q", body)
	}
	assertNoStore(t, unauthRec)
	assertNoStore(t, authRec)
}

func TestRuntimeConfigReflectsAuthenticationStateInSelfHostedMode(t *testing.T) {
	t.Parallel()

	env := newServerTestEnv(t, config.DeploymentModeSelfHostedSingleUser)
	user := createInitialUser(t, env)

	unauthReq := httptest.NewRequest(http.MethodGet, "/runtime-config.js", nil)
	unauthRec := httptest.NewRecorder()
	env.handler.ServeHTTP(unauthRec, unauthReq)

	if unauthRec.Code != http.StatusOK {
		t.Fatalf("expected unauthenticated runtime config status 200, got %d", unauthRec.Code)
	}

	unauthBody := unauthRec.Body.String()
	for _, want := range []string{
		`"deploymentMode":"selfhosted_single_user"`,
		`"authAvailable":true`,
		`"authRequired":true`,
		`"isAuthenticated":false`,
		`"syncAvailable":true`,
		`"syncRequiresAuth":true`,
		`"setupAvailable":true`,
		`"apiBase":""`,
	} {
		if !strings.Contains(unauthBody, want) {
			t.Fatalf("expected unauthenticated runtime config body to contain %q, got %q", want, unauthBody)
		}
	}

	authReq := httptest.NewRequest(http.MethodGet, "/runtime-config.js", nil)
	authReq.AddCookie(createSessionCookie(t, env, user.ID))
	authRec := httptest.NewRecorder()
	env.handler.ServeHTTP(authRec, authReq)

	if authRec.Code != http.StatusOK {
		t.Fatalf("expected authenticated runtime config status 200, got %d", authRec.Code)
	}
	if body := authRec.Body.String(); !strings.Contains(body, `"isAuthenticated":true`) {
		t.Fatalf("expected authenticated runtime config body to contain isAuthenticated true, got %q", body)
	}
	assertNoStore(t, unauthRec)
	assertNoStore(t, authRec)
}

func TestStaticLocalModeDisablesAuthRoutes(t *testing.T) {
	t.Parallel()

	env := newServerTestEnv(t, config.DeploymentModeStaticLocal)

	for _, target := range []string{"/setup", "/login", "/logout"} {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		rec := httptest.NewRecorder()
		env.handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected %s to return 404 in static mode, got %d", target, rec.Code)
		}
	}
}

func TestCloudMultiUserModeDisablesSetupRoute(t *testing.T) {
	t.Parallel()

	env := newServerTestEnv(t, config.DeploymentModeCloudMultiUser)

	req := httptest.NewRequest(http.MethodGet, "/setup", nil)
	rec := httptest.NewRecorder()
	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected /setup to return 404 in cloud mode, got %d", rec.Code)
	}
}

func TestCloudMultiUserSignupPageAndFlow(t *testing.T) {
	t.Parallel()

	env := newServerTestEnv(t, config.DeploymentModeCloudMultiUser)

	getReq := httptest.NewRequest(http.MethodGet, "/signup", nil)
	getRec := httptest.NewRecorder()
	env.handler.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("expected signup page status 200, got %d", getRec.Code)
	}
	assertNoStore(t, getRec)

	form := url.Values{
		"username":        {"cloud-user"},
		"password":        {serverTestPassword},
		"confirmPassword": {serverTestPassword},
	}
	postReq := httptest.NewRequest(http.MethodPost, "/signup", strings.NewReader(form.Encode()))
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postReq.Header.Set("Origin", "http://example.com")
	postReq.Host = "example.com"
	postRec := httptest.NewRecorder()
	env.handler.ServeHTTP(postRec, postReq)

	if postRec.Code != http.StatusSeeOther {
		t.Fatalf("expected signup redirect, got %d", postRec.Code)
	}
	if location := postRec.Header().Get("Location"); location != "/" {
		t.Fatalf("expected signup redirect to /, got %q", location)
	}
	assertNoStore(t, postRec)

	cookies := postRec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected session cookie after signup")
	}

	runtimeReq := httptest.NewRequest(http.MethodGet, "/runtime-config.js", nil)
	runtimeReq.AddCookie(cookies[0])
	runtimeRec := httptest.NewRecorder()
	env.handler.ServeHTTP(runtimeRec, runtimeReq)
	if !strings.Contains(runtimeRec.Body.String(), `"isAuthenticated":true`) {
		t.Fatalf("expected authenticated runtime config after signup, got %q", runtimeRec.Body.String())
	}
}

func TestCloudMultiUserDuplicateSignupIsHandledSafely(t *testing.T) {
	t.Parallel()

	env := newServerTestEnv(t, config.DeploymentModeCloudMultiUser)
	createHostedUser(t, env, "cloud-user")

	form := url.Values{
		"username":        {"cloud-user"},
		"password":        {serverTestPassword},
		"confirmPassword": {serverTestPassword},
	}
	req := httptest.NewRequest(http.MethodPost, "/signup", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "http://example.com")
	req.Host = "example.com"
	rec := httptest.NewRecorder()
	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected duplicate signup status 409, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Could not create that account. Try signing in or choose a different username.") {
		t.Fatalf("expected generic duplicate signup error, got %q", body)
	}
	if strings.Contains(body, "already in use") {
		t.Fatalf("expected duplicate signup to avoid username leak, got %q", body)
	}
	assertNoStore(t, rec)
}

func TestCloudMultiUserLoginPageAndFlow(t *testing.T) {
	t.Parallel()

	env := newServerTestEnv(t, config.DeploymentModeCloudMultiUser)
	createHostedUser(t, env, "cloud-user")

	getReq := httptest.NewRequest(http.MethodGet, "/login", nil)
	getRec := httptest.NewRecorder()
	env.handler.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("expected login page status 200, got %d", getRec.Code)
	}
	assertNoStore(t, getRec)

	form := url.Values{
		"username": {"cloud-user"},
		"password": {serverTestPassword},
	}
	postReq := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postReq.Header.Set("Origin", "http://example.com")
	postReq.Host = "example.com"
	postRec := httptest.NewRecorder()
	env.handler.ServeHTTP(postRec, postReq)

	if postRec.Code != http.StatusSeeOther {
		t.Fatalf("expected login redirect, got %d", postRec.Code)
	}
	if location := postRec.Header().Get("Location"); location != "/" {
		t.Fatalf("expected login redirect to /, got %q", location)
	}
	if len(postRec.Result().Cookies()) == 0 {
		t.Fatal("expected session cookie after login")
	}
	assertNoStore(t, postRec)
}

func TestCloudMultiUserLogoutClearsSessionAndRedirectsHome(t *testing.T) {
	t.Parallel()

	env := newServerTestEnv(t, config.DeploymentModeCloudMultiUser)
	user := createHostedUser(t, env, "cloud-user")
	cookie := createSessionCookie(t, env, user.ID)

	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	req.Header.Set("Origin", "http://example.com")
	req.Host = "example.com"
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected logout redirect, got %d", rec.Code)
	}
	if location := rec.Header().Get("Location"); location != "/" {
		t.Fatalf("expected logout redirect to /, got %q", location)
	}
	logoutCookies := rec.Result().Cookies()
	if len(logoutCookies) == 0 || logoutCookies[0].MaxAge != -1 {
		t.Fatal("expected cleared session cookie on logout")
	}
	assertNoStore(t, rec)
}

func TestNewHandlerRedirectsRootToSetupWhenNoUsers(t *testing.T) {
	t.Parallel()

	env := newServerTestEnv(t, config.DeploymentModeSelfHostedSingleUser)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected / to redirect, got %d", rec.Code)
	}
	if location := rec.Header().Get("Location"); location != "/setup" {
		t.Fatalf("expected redirect to /setup, got %q", location)
	}
	assertNoStore(t, rec)
}

func TestSetupCreatesFirstUserAndServesApp(t *testing.T) {
	t.Parallel()

	env := newServerTestEnv(t, config.DeploymentModeSelfHostedSingleUser)
	form := url.Values{
		"username":        {"owner"},
		"password":        {serverTestPassword},
		"confirmPassword": {serverTestPassword},
	}
	req := httptest.NewRequest(http.MethodPost, "/setup", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", "http://example.com")
	req.Host = "example.com"
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected setup redirect, got %d", rec.Code)
	}
	if location := rec.Header().Get("Location"); location != "/" {
		t.Fatalf("expected setup redirect to /, got %q", location)
	}
	assertNoStore(t, rec)

	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected session cookie after setup")
	}

	appReq := httptest.NewRequest(http.MethodGet, "/", nil)
	appReq.AddCookie(cookies[0])
	appRec := httptest.NewRecorder()
	env.handler.ServeHTTP(appRec, appReq)

	if appRec.Code != http.StatusOK {
		t.Fatalf("expected authenticated app shell, got %d", appRec.Code)
	}
	assertNoStore(t, appRec)
}

func TestProtectedAppRedirectsToLoginWhenSessionMissing(t *testing.T) {
	t.Parallel()

	env := newServerTestEnv(t, config.DeploymentModeSelfHostedSingleUser)
	createInitialUser(t, env)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect to login, got %d", rec.Code)
	}
	if location := rec.Header().Get("Location"); location != "/login" {
		t.Fatalf("expected redirect to /login, got %q", location)
	}
}

func TestLoginAndLogoutFlow(t *testing.T) {
	t.Parallel()

	env := newServerTestEnv(t, config.DeploymentModeSelfHostedSingleUser)
	createInitialUser(t, env)

	loginForm := url.Values{
		"username": {"owner"},
		"password": {serverTestPassword},
	}
	loginReq := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(loginForm.Encode()))
	loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginReq.Header.Set("Origin", "http://example.com")
	loginReq.Host = "example.com"
	loginRec := httptest.NewRecorder()

	env.handler.ServeHTTP(loginRec, loginReq)

	if loginRec.Code != http.StatusSeeOther {
		t.Fatalf("expected login redirect, got %d", loginRec.Code)
	}
	if location := loginRec.Header().Get("Location"); location != "/" {
		t.Fatalf("expected login redirect to /, got %q", location)
	}
	assertNoStore(t, loginRec)

	cookies := loginRec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected session cookie after login")
	}

	logoutReq := httptest.NewRequest(http.MethodPost, "/logout", nil)
	logoutReq.Header.Set("Origin", "http://example.com")
	logoutReq.Host = "example.com"
	logoutReq.AddCookie(cookies[0])
	logoutRec := httptest.NewRecorder()

	env.handler.ServeHTTP(logoutRec, logoutReq)

	if logoutRec.Code != http.StatusSeeOther {
		t.Fatalf("expected logout redirect, got %d", logoutRec.Code)
	}
	if location := logoutRec.Header().Get("Location"); location != "/login" {
		t.Fatalf("expected logout redirect to /login, got %q", location)
	}
	assertNoStore(t, logoutRec)

	logoutCookies := logoutRec.Result().Cookies()
	if len(logoutCookies) == 0 || logoutCookies[0].MaxAge != -1 {
		t.Fatal("expected cleared session cookie on logout")
	}
}

func TestSetupUnavailableAfterFirstUserExists(t *testing.T) {
	t.Parallel()

	env := newServerTestEnv(t, config.DeploymentModeSelfHostedSingleUser)
	createInitialUser(t, env)

	req := httptest.NewRequest(http.MethodGet, "/setup", nil)
	rec := httptest.NewRecorder()
	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect, got %d", rec.Code)
	}
	if location := rec.Header().Get("Location"); location != "/login" {
		t.Fatalf("expected redirect to /login, got %q", location)
	}
	assertNoStore(t, rec)
}

func TestLoginPageBypassesForExistingSession(t *testing.T) {
	t.Parallel()

	env := newServerTestEnv(t, config.DeploymentModeSelfHostedSingleUser)
	user := createInitialUser(t, env)
	cookie := createSessionCookie(t, env, user.ID)

	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect, got %d", rec.Code)
	}
	if location := rec.Header().Get("Location"); location != "/" {
		t.Fatalf("expected redirect to /, got %q", location)
	}
	assertNoStore(t, rec)
}

func TestSetupPageSetsNoStore(t *testing.T) {
	t.Parallel()

	env := newServerTestEnv(t, config.DeploymentModeSelfHostedSingleUser)
	req := httptest.NewRequest(http.MethodGet, "/setup", nil)
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected setup page status 200, got %d", rec.Code)
	}
	assertNoStore(t, rec)
}

func TestLoginPageSetsNoStore(t *testing.T) {
	t.Parallel()

	env := newServerTestEnv(t, config.DeploymentModeSelfHostedSingleUser)
	createInitialUser(t, env)

	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected login page status 200, got %d", rec.Code)
	}
	assertNoStore(t, rec)
}

type serverTestEnv struct {
	handler     http.Handler
	store       *store.Store
	authManager *auth.Manager
}

func newServerTestEnv(t *testing.T, deploymentMode config.DeploymentMode) *serverTestEnv {
	t.Helper()

	staticDir := t.TempDir()
	writeTestFile(t, filepath.Join(staticDir, "index.html"), "<html><body>postbaby</body></html>")
	writeTestFile(t, filepath.Join(staticDir, "manifest.json"), `{"name":"postbaby"}`)
	writeTestFile(t, filepath.Join(staticDir, "css", "style.css"), "body { color: black; }")

	dbPath := filepath.Join(t.TempDir(), "postbaby-server-test.db")
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
	apiHandler := httpapi.NewHandler(docStore, authManager, deploymentMode)
	return &serverTestEnv{
		handler:     NewHandler(apiHandler, authManager, staticDir, deploymentMode),
		store:       docStore,
		authManager: authManager,
	}
}

func createInitialUser(t *testing.T, env *serverTestEnv) store.User {
	t.Helper()

	user, err := env.authManager.CreateInitialUser(context.Background(), "owner", serverTestPassword)
	if err != nil {
		t.Fatalf("create initial user: %v", err)
	}

	return user
}

func createHostedUser(t *testing.T, env *serverTestEnv, username string) store.User {
	t.Helper()

	user, err := env.authManager.CreateUser(context.Background(), username, serverTestPassword)
	if err != nil {
		t.Fatalf("create hosted user: %v", err)
	}

	return user
}

func createSessionCookie(t *testing.T, env *serverTestEnv, userID int64) *http.Cookie {
	t.Helper()

	rec := httptest.NewRecorder()
	if err := env.authManager.CreateSession(context.Background(), rec, userID); err != nil {
		t.Fatalf("create session: %v", err)
	}

	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected session cookie")
	}

	return cookies[0]
}

func assertNoStore(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()

	if cacheControl := rec.Header().Get("Cache-Control"); cacheControl != "no-store" {
		t.Fatalf("expected Cache-Control no-store, got %q", cacheControl)
	}
}

func writeTestFile(t *testing.T, path, contents string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create test directory: %v", err)
	}

	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}
}
