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
	"time"

	"postbaby-backend/internal/auth"
	"postbaby-backend/internal/billing"
	"postbaby-backend/internal/config"
	"postbaby-backend/internal/entitlement"
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
		`"billingAvailable":false`,
		`"syncAvailable":false`,
		`"syncRequiresAuth":false`,
		`"syncUsable":false`,
		`"entitlement":{"hostedSync":false}`,
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
		`"billingAvailable":false`,
		`"syncAvailable":true`,
		`"syncRequiresAuth":true`,
		`"syncUsable":false`,
		`"entitlement":{"hostedSync":false}`,
		`"setupAvailable":false`,
		`"apiBase":""`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected runtime config body to contain %q, got %q", want, body)
		}
	}

	assertNoStore(t, rec)
}

func TestRuntimeConfigReflectsBillingAvailabilityInCloudMultiUserMode(t *testing.T) {
	t.Parallel()

	env := newServerTestEnvWithOptions(t, config.DeploymentModeCloudMultiUser, serverTestOptions{
		billingProvider: &serverFakeBillingProvider{
			available: true,
			name:      "stripe",
		},
		publicBaseURL: "http://127.0.0.1:8080",
	})

	req := httptest.NewRequest(http.MethodGet, "/runtime-config.js", nil)
	rec := httptest.NewRecorder()
	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected runtime config status 200, got %d", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, `"billingAvailable":true`) {
		t.Fatalf("expected billingAvailable true, got %q", body)
	}
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
		`"billingAvailable":false`,
		`"syncAvailable":true`,
		`"syncRequiresAuth":true`,
		`"syncUsable":false`,
		`"entitlement":{"hostedSync":false}`,
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
	for _, want := range []string{
		`"isAuthenticated":true`,
		`"billingAvailable":false`,
		`"syncUsable":false`,
		`"entitlement":{"hostedSync":false}`,
	} {
		if body := authRec.Body.String(); !strings.Contains(body, want) {
			t.Fatalf("expected authenticated cloud runtime config body to contain %q, got %q", want, body)
		}
	}
	assertNoStore(t, unauthRec)
	assertNoStore(t, authRec)
}

func TestRuntimeConfigReflectsHostedSyncEntitlementInCloudMultiUserMode(t *testing.T) {
	t.Parallel()

	env := newServerTestEnv(t, config.DeploymentModeCloudMultiUser)
	user := createHostedUser(t, env, "cloud-entitled-user")
	grantHostedSyncEntitlement(t, env, user.ID)

	authReq := httptest.NewRequest(http.MethodGet, "/runtime-config.js", nil)
	authReq.AddCookie(createSessionCookie(t, env, user.ID))
	authRec := httptest.NewRecorder()
	env.handler.ServeHTTP(authRec, authReq)

	if authRec.Code != http.StatusOK {
		t.Fatalf("expected authenticated runtime config status 200, got %d", authRec.Code)
	}

	body := authRec.Body.String()
	for _, want := range []string{
		`"deploymentMode":"cloud_multi_user"`,
		`"isAuthenticated":true`,
		`"billingAvailable":false`,
		`"syncAvailable":true`,
		`"syncRequiresAuth":true`,
		`"syncUsable":true`,
		`"entitlement":{"hostedSync":true}`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected entitled cloud runtime config body to contain %q, got %q", want, body)
		}
	}
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
		`"billingAvailable":false`,
		`"syncAvailable":true`,
		`"syncRequiresAuth":true`,
		`"syncUsable":true`,
		`"entitlement":{"hostedSync":false}`,
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
	for _, want := range []string{
		`"isAuthenticated":true`,
		`"billingAvailable":false`,
		`"syncUsable":true`,
		`"entitlement":{"hostedSync":false}`,
	} {
		if body := authRec.Body.String(); !strings.Contains(body, want) {
			t.Fatalf("expected authenticated runtime config body to contain %q, got %q", want, body)
		}
	}
	assertNoStore(t, unauthRec)
	assertNoStore(t, authRec)
}

func TestStaticLocalModeDisablesAuthRoutes(t *testing.T) {
	t.Parallel()

	env := newServerTestEnv(t, config.DeploymentModeStaticLocal)

	for _, target := range []string{"/setup", "/signup", "/login", "/logout"} {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		rec := httptest.NewRecorder()
		env.handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected %s to return 404 in static mode, got %d", target, rec.Code)
		}
	}
}

func TestSelfHostedSingleUserModeDisablesSignupRoute(t *testing.T) {
	t.Parallel()

	env := newServerTestEnv(t, config.DeploymentModeSelfHostedSingleUser)
	req := httptest.NewRequest(http.MethodGet, "/signup", nil)
	rec := httptest.NewRecorder()
	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected /signup to return 404 in self-hosted mode, got %d", rec.Code)
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

func TestCloudMultiUserSignupRejectsUnsupportedMethods(t *testing.T) {
	t.Parallel()

	env := newServerTestEnv(t, config.DeploymentModeCloudMultiUser)
	req := httptest.NewRequest(http.MethodPut, "/signup", nil)
	rec := httptest.NewRecorder()
	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected signup unsupported method status 405, got %d", rec.Code)
	}
	if allow := rec.Header().Get("Allow"); allow != "GET, POST" {
		t.Fatalf("expected Allow header %q, got %q", "GET, POST", allow)
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
	if !strings.Contains(runtimeRec.Body.String(), `"syncUsable":false`) {
		t.Fatalf("expected authenticated runtime config after signup to keep sync unusable without entitlement, got %q", runtimeRec.Body.String())
	}
}

func TestCloudMultiUserSignupValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		form        url.Values
		wantStatus  int
		wantMessage string
	}{
		{
			name: "empty username",
			form: url.Values{
				"username":        {"   "},
				"password":        {serverTestPassword},
				"confirmPassword": {serverTestPassword},
			},
			wantStatus:  http.StatusBadRequest,
			wantMessage: "Enter a username.",
		},
		{
			name: "empty password",
			form: url.Values{
				"username":        {"cloud-user"},
				"password":        {""},
				"confirmPassword": {""},
			},
			wantStatus:  http.StatusBadRequest,
			wantMessage: "Enter a password.",
		},
		{
			name: "mismatched confirmation",
			form: url.Values{
				"username":        {"cloud-user"},
				"password":        {serverTestPassword},
				"confirmPassword": {"not-the-same"},
			},
			wantStatus:  http.StatusBadRequest,
			wantMessage: "Passwords do not match.",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			env := newServerTestEnv(t, config.DeploymentModeCloudMultiUser)
			req := newFormRequest(http.MethodPost, "/signup", tc.form, "http://example.com", "example.com")
			rec := httptest.NewRecorder()

			env.handler.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("expected status %d, got %d", tc.wantStatus, rec.Code)
			}
			if body := rec.Body.String(); !strings.Contains(body, tc.wantMessage) {
				t.Fatalf("expected response body to contain %q, got %q", tc.wantMessage, body)
			}
			if len(rec.Result().Cookies()) != 0 {
				t.Fatal("expected signup validation failure to avoid creating a session")
			}
			assertNoStore(t, rec)
		})
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

func TestSetupRejectsCrossOriginPost(t *testing.T) {
	t.Parallel()

	env := newServerTestEnv(t, config.DeploymentModeSelfHostedSingleUser)
	req := newFormRequest(http.MethodPost, "/setup", url.Values{
		"username":        {"owner"},
		"password":        {serverTestPassword},
		"confirmPassword": {serverTestPassword},
	}, "http://evil.example", "example.com")
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected cross-origin setup POST to return 403, got %d", rec.Code)
	}
	assertNoStore(t, rec)
}

func TestSelfHostedLoginRejectsCrossOriginPost(t *testing.T) {
	t.Parallel()

	env := newServerTestEnv(t, config.DeploymentModeSelfHostedSingleUser)
	createInitialUser(t, env)

	req := newFormRequest(http.MethodPost, "/login", url.Values{
		"username": {"owner"},
		"password": {serverTestPassword},
	}, "http://evil.example", "example.com")
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected cross-origin self-hosted login POST to return 403, got %d", rec.Code)
	}
	assertNoStore(t, rec)
}

func TestSelfHostedLogoutRejectsCrossOriginPost(t *testing.T) {
	t.Parallel()

	env := newServerTestEnv(t, config.DeploymentModeSelfHostedSingleUser)
	user := createInitialUser(t, env)

	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	req.Header.Set("Origin", "http://evil.example")
	req.Host = "example.com"
	req.AddCookie(createSessionCookie(t, env, user.ID))
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected cross-origin self-hosted logout POST to return 403, got %d", rec.Code)
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

func TestCloudMultiUserLoginRejectsCrossOriginPost(t *testing.T) {
	t.Parallel()

	env := newServerTestEnv(t, config.DeploymentModeCloudMultiUser)
	createHostedUser(t, env, "cloud-user")

	req := newFormRequest(http.MethodPost, "/login", url.Values{
		"username": {"cloud-user"},
		"password": {serverTestPassword},
	}, "http://evil.example", "example.com")
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected cross-origin cloud login POST to return 403, got %d", rec.Code)
	}
	assertNoStore(t, rec)
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

func TestCloudMultiUserSignupRejectsCrossOriginPost(t *testing.T) {
	t.Parallel()

	env := newServerTestEnv(t, config.DeploymentModeCloudMultiUser)
	req := newFormRequest(http.MethodPost, "/signup", url.Values{
		"username":        {"cloud-user"},
		"password":        {serverTestPassword},
		"confirmPassword": {serverTestPassword},
	}, "http://evil.example", "example.com")
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected cross-origin cloud signup POST to return 403, got %d", rec.Code)
	}
	assertNoStore(t, rec)
}

func TestCloudMultiUserLogoutRejectsCrossOriginPost(t *testing.T) {
	t.Parallel()

	env := newServerTestEnv(t, config.DeploymentModeCloudMultiUser)
	user := createHostedUser(t, env, "cloud-user")

	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	req.Header.Set("Origin", "http://evil.example")
	req.Host = "example.com"
	req.AddCookie(createSessionCookie(t, env, user.ID))
	rec := httptest.NewRecorder()

	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected cross-origin cloud logout POST to return 403, got %d", rec.Code)
	}
	assertNoStore(t, rec)
}

func TestBillingRoutesDisabledOutsideConfiguredCloudMode(t *testing.T) {
	t.Parallel()

	for _, mode := range []config.DeploymentMode{
		config.DeploymentModeStaticLocal,
		config.DeploymentModeSelfHostedSingleUser,
	} {
		t.Run(string(mode), func(t *testing.T) {
			t.Parallel()

			env := newServerTestEnvWithOptions(t, mode, serverTestOptions{
				billingProvider: &serverFakeBillingProvider{
					available: true,
					name:      "stripe",
				},
				publicBaseURL: "http://127.0.0.1:8080",
			})

			for _, target := range []string{"/billing/checkout", "/billing/portal", "/billing/webhook"} {
				req := httptest.NewRequest(http.MethodPost, target, nil)
				rec := httptest.NewRecorder()
				env.handler.ServeHTTP(rec, req)

				if rec.Code != http.StatusNotFound {
					t.Fatalf("expected %s to return 404 in %s mode, got %d", target, mode, rec.Code)
				}
			}
		})
	}
}

func TestBillingRoutesReturnNotFoundWhenCloudBillingIsNotConfigured(t *testing.T) {
	t.Parallel()

	env := newServerTestEnv(t, config.DeploymentModeCloudMultiUser)

	for _, target := range []string{"/billing/checkout", "/billing/portal", "/billing/webhook"} {
		req := httptest.NewRequest(http.MethodPost, target, nil)
		rec := httptest.NewRecorder()
		env.handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected %s to return 404 without configured billing, got %d", target, rec.Code)
		}
	}
}

func TestBillingCheckoutRequiresAuthenticationAndRedirectsToProvider(t *testing.T) {
	t.Parallel()

	provider := &serverFakeBillingProvider{
		available:        true,
		name:             "stripe",
		createCustomerID: "cus_checkout",
		checkoutURL:      "https://checkout.stripe.test/session",
	}
	env := newServerTestEnvWithOptions(t, config.DeploymentModeCloudMultiUser, serverTestOptions{
		billingProvider: provider,
		publicBaseURL:   "http://127.0.0.1:8080",
	})

	unauthReq := httptest.NewRequest(http.MethodPost, "/billing/checkout", nil)
	unauthReq.Header.Set("Origin", "http://example.com")
	unauthReq.Host = "example.com"
	unauthRec := httptest.NewRecorder()
	env.handler.ServeHTTP(unauthRec, unauthReq)

	if unauthRec.Code != http.StatusSeeOther || unauthRec.Header().Get("Location") != "/login" {
		t.Fatalf("expected unauthenticated checkout to redirect to /login, got status=%d location=%q", unauthRec.Code, unauthRec.Header().Get("Location"))
	}
	assertNoStore(t, unauthRec)

	user := createHostedUser(t, env, "billing-user")
	authReq := httptest.NewRequest(http.MethodPost, "/billing/checkout", nil)
	authReq.Header.Set("Origin", "http://example.com")
	authReq.Host = "example.com"
	authReq.AddCookie(createSessionCookie(t, env, user.ID))
	authRec := httptest.NewRecorder()
	env.handler.ServeHTTP(authRec, authReq)

	if authRec.Code != http.StatusSeeOther || authRec.Header().Get("Location") != "https://checkout.stripe.test/session" {
		t.Fatalf("expected checkout redirect to provider, got status=%d location=%q", authRec.Code, authRec.Header().Get("Location"))
	}
	if provider.checkoutInput == nil || provider.checkoutInput.ProviderCustomerID != "cus_checkout" {
		t.Fatalf("unexpected checkout provider input: %+v", provider.checkoutInput)
	}
	assertNoStore(t, authRec)
}

func TestBillingPortalRequiresAuthenticationAndRedirectsToProvider(t *testing.T) {
	t.Parallel()

	provider := &serverFakeBillingProvider{
		available: true,
		name:      "stripe",
		portalURL: "https://billing.stripe.test/session",
	}
	env := newServerTestEnvWithOptions(t, config.DeploymentModeCloudMultiUser, serverTestOptions{
		billingProvider: provider,
		publicBaseURL:   "http://127.0.0.1:8080",
	})

	user := createHostedUser(t, env, "portal-user")
	if _, err := env.store.PutBillingCustomer(context.Background(), user.ID, "stripe", "cus_portal"); err != nil {
		t.Fatalf("seed billing customer: %v", err)
	}

	unauthReq := httptest.NewRequest(http.MethodPost, "/billing/portal", nil)
	unauthReq.Header.Set("Origin", "http://example.com")
	unauthReq.Host = "example.com"
	unauthRec := httptest.NewRecorder()
	env.handler.ServeHTTP(unauthRec, unauthReq)

	if unauthRec.Code != http.StatusSeeOther || unauthRec.Header().Get("Location") != "/login" {
		t.Fatalf("expected unauthenticated portal to redirect to /login, got status=%d location=%q", unauthRec.Code, unauthRec.Header().Get("Location"))
	}

	authReq := httptest.NewRequest(http.MethodPost, "/billing/portal", nil)
	authReq.Header.Set("Origin", "http://example.com")
	authReq.Host = "example.com"
	authReq.AddCookie(createSessionCookie(t, env, user.ID))
	authRec := httptest.NewRecorder()
	env.handler.ServeHTTP(authRec, authReq)

	if authRec.Code != http.StatusSeeOther || authRec.Header().Get("Location") != "https://billing.stripe.test/session" {
		t.Fatalf("expected portal redirect to provider, got status=%d location=%q", authRec.Code, authRec.Header().Get("Location"))
	}
	if provider.portalInput == nil || provider.portalInput.ProviderCustomerID != "cus_portal" {
		t.Fatalf("unexpected portal provider input: %+v", provider.portalInput)
	}
	assertNoStore(t, authRec)
}

func TestBillingCheckoutRejectsCrossOriginPost(t *testing.T) {
	t.Parallel()

	env := newServerTestEnvWithOptions(t, config.DeploymentModeCloudMultiUser, serverTestOptions{
		billingProvider: &serverFakeBillingProvider{
			available: true,
			name:      "stripe",
		},
		publicBaseURL: "http://127.0.0.1:8080",
	})
	user := createHostedUser(t, env, "billing-user")

	req := httptest.NewRequest(http.MethodPost, "/billing/checkout", nil)
	req.Header.Set("Origin", "http://evil.example")
	req.Host = "example.com"
	req.AddCookie(createSessionCookie(t, env, user.ID))
	rec := httptest.NewRecorder()
	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected cross-origin checkout POST to return 403, got %d", rec.Code)
	}
	assertNoStore(t, rec)
}

func TestBillingPortalRejectsCrossOriginPost(t *testing.T) {
	t.Parallel()

	env := newServerTestEnvWithOptions(t, config.DeploymentModeCloudMultiUser, serverTestOptions{
		billingProvider: &serverFakeBillingProvider{
			available: true,
			name:      "stripe",
		},
		publicBaseURL: "http://127.0.0.1:8080",
	})
	user := createHostedUser(t, env, "billing-user")
	if _, err := env.store.PutBillingCustomer(context.Background(), user.ID, "stripe", "cus_portal"); err != nil {
		t.Fatalf("seed billing customer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/billing/portal", nil)
	req.Header.Set("Origin", "http://evil.example")
	req.Host = "example.com"
	req.AddCookie(createSessionCookie(t, env, user.ID))
	rec := httptest.NewRecorder()
	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected cross-origin portal POST to return 403, got %d", rec.Code)
	}
	assertNoStore(t, rec)
}

func TestBillingWebhookRejectsInvalidSignature(t *testing.T) {
	t.Parallel()

	env := newServerTestEnvWithOptions(t, config.DeploymentModeCloudMultiUser, serverTestOptions{
		billingProvider: &serverFakeBillingProvider{
			available:  true,
			name:       "stripe",
			webhookErr: billing.ErrInvalidWebhookSignature,
		},
		publicBaseURL: "http://127.0.0.1:8080",
	})

	req := httptest.NewRequest(http.MethodPost, "/billing/webhook", strings.NewReader(`{"id":"evt_bad"}`))
	req.Header.Set("Stripe-Signature", "bad")
	rec := httptest.NewRecorder()
	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid signature webhook to return 400, got %d", rec.Code)
	}
	assertNoStore(t, rec)
}

func TestBillingWebhookIgnoresUnknownValidEvent(t *testing.T) {
	t.Parallel()

	env := newServerTestEnvWithOptions(t, config.DeploymentModeCloudMultiUser, serverTestOptions{
		billingProvider: &serverFakeBillingProvider{
			available: true,
			name:      "stripe",
			webhookEvent: billing.WebhookEvent{
				ID:   "evt_unknown",
				Type: "ping.unknown",
			},
		},
		publicBaseURL: "http://127.0.0.1:8080",
	})

	req := httptest.NewRequest(http.MethodPost, "/billing/webhook", strings.NewReader(`{"id":"evt_unknown"}`))
	req.Header.Set("Stripe-Signature", "valid")
	rec := httptest.NewRecorder()
	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected unknown webhook event to return 200, got %d", rec.Code)
	}
	assertNoStore(t, rec)
}

func TestBillingWebhookUpdatesEntitlementAndSyncSourceOfTruth(t *testing.T) {
	t.Parallel()

	provider := &serverFakeBillingProvider{
		available: true,
		name:      "stripe",
	}
	env := newServerTestEnvWithOptions(t, config.DeploymentModeCloudMultiUser, serverTestOptions{
		billingProvider: provider,
		publicBaseURL:   "http://127.0.0.1:8080",
	})
	user := createHostedUser(t, env, "billed-user")
	cookie := createSessionCookie(t, env, user.ID)

	runtimeBeforeReq := httptest.NewRequest(http.MethodGet, "/runtime-config.js", nil)
	runtimeBeforeReq.AddCookie(cookie)
	runtimeBeforeRec := httptest.NewRecorder()
	env.handler.ServeHTTP(runtimeBeforeRec, runtimeBeforeReq)
	if !strings.Contains(runtimeBeforeRec.Body.String(), `"billingAvailable":true`) || !strings.Contains(runtimeBeforeRec.Body.String(), `"syncUsable":false`) {
		t.Fatalf("expected billing enabled but sync unusable before webhook, got %q", runtimeBeforeRec.Body.String())
	}

	apiBeforeReq := httptest.NewRequest(http.MethodGet, "/api/document?appId=postbaby-web", nil)
	apiBeforeReq.AddCookie(cookie)
	apiBeforeRec := httptest.NewRecorder()
	env.handler.ServeHTTP(apiBeforeRec, apiBeforeReq)
	if apiBeforeRec.Code != http.StatusForbidden || !strings.Contains(apiBeforeRec.Body.String(), `"code":"entitlement_required"`) {
		t.Fatalf("expected entitlement_required before webhook, got status=%d body=%q", apiBeforeRec.Code, apiBeforeRec.Body.String())
	}

	provider.webhookEvent = billing.WebhookEvent{
		ID:                     "evt_checkout",
		Type:                   "checkout.session.completed",
		UserID:                 user.ID,
		ProviderCustomerID:     "cus_billed",
		ProviderSubscriptionID: "sub_billed",
	}
	checkoutReq := httptest.NewRequest(http.MethodPost, "/billing/webhook", strings.NewReader(`{"id":"evt_checkout"}`))
	checkoutReq.Header.Set("Stripe-Signature", "valid")
	checkoutRec := httptest.NewRecorder()
	env.handler.ServeHTTP(checkoutRec, checkoutReq)
	if checkoutRec.Code != http.StatusOK {
		t.Fatalf("expected checkout webhook status 200, got %d", checkoutRec.Code)
	}

	provider.webhookEvent = billing.WebhookEvent{
		ID:                     "evt_subscription",
		Type:                   "customer.subscription.updated",
		ProviderCustomerID:     "cus_billed",
		ProviderSubscriptionID: "sub_billed",
		Status:                 "active",
		ValidUntil:             timePointer(time.Date(2026, time.May, 25, 12, 0, 0, 0, time.UTC)),
	}
	subscriptionReq := httptest.NewRequest(http.MethodPost, "/billing/webhook", strings.NewReader(`{"id":"evt_subscription"}`))
	subscriptionReq.Header.Set("Stripe-Signature", "valid")
	subscriptionRec := httptest.NewRecorder()
	env.handler.ServeHTTP(subscriptionRec, subscriptionReq)
	if subscriptionRec.Code != http.StatusOK {
		t.Fatalf("expected subscription webhook status 200, got %d", subscriptionRec.Code)
	}

	subscriptionRepeatReq := httptest.NewRequest(http.MethodPost, "/billing/webhook", strings.NewReader(`{"id":"evt_subscription"}`))
	subscriptionRepeatReq.Header.Set("Stripe-Signature", "valid")
	subscriptionRepeatRec := httptest.NewRecorder()
	env.handler.ServeHTTP(subscriptionRepeatRec, subscriptionRepeatReq)
	if subscriptionRepeatRec.Code != http.StatusOK {
		t.Fatalf("expected repeated subscription webhook status 200, got %d", subscriptionRepeatRec.Code)
	}

	runtimeAfterReq := httptest.NewRequest(http.MethodGet, "/runtime-config.js", nil)
	runtimeAfterReq.AddCookie(cookie)
	runtimeAfterRec := httptest.NewRecorder()
	env.handler.ServeHTTP(runtimeAfterRec, runtimeAfterReq)
	body := runtimeAfterRec.Body.String()
	if !strings.Contains(body, `"billingAvailable":true`) || !strings.Contains(body, `"syncUsable":true`) || !strings.Contains(body, `"entitlement":{"hostedSync":true}`) {
		t.Fatalf("expected entitled runtime config after webhook, got %q", body)
	}

	apiAfterReq := httptest.NewRequest(http.MethodGet, "/api/document/meta?appId=postbaby-web", nil)
	apiAfterReq.AddCookie(cookie)
	apiAfterRec := httptest.NewRecorder()
	env.handler.ServeHTTP(apiAfterRec, apiAfterReq)
	if apiAfterRec.Code != http.StatusOK {
		t.Fatalf("expected entitled sync metadata access after webhook, got %d body=%q", apiAfterRec.Code, apiAfterRec.Body.String())
	}
}

func TestBillingWebhookCheckoutCompletionAloneDoesNotGrantHostedSync(t *testing.T) {
	t.Parallel()

	provider := &serverFakeBillingProvider{
		available: true,
		name:      "stripe",
	}
	env := newServerTestEnvWithOptions(t, config.DeploymentModeCloudMultiUser, serverTestOptions{
		billingProvider: provider,
		publicBaseURL:   "http://127.0.0.1:8080",
	})
	user := createHostedUser(t, env, "checkout-only-user")
	cookie := createSessionCookie(t, env, user.ID)

	provider.webhookEvent = billing.WebhookEvent{
		ID:                     "evt_checkout_only",
		Type:                   "checkout.session.completed",
		UserID:                 user.ID,
		ProviderCustomerID:     "cus_checkout_only",
		ProviderSubscriptionID: "sub_checkout_only",
	}
	webhookReq := httptest.NewRequest(http.MethodPost, "/billing/webhook", strings.NewReader(`{"id":"evt_checkout_only"}`))
	webhookReq.Header.Set("Stripe-Signature", "valid")
	webhookRec := httptest.NewRecorder()
	env.handler.ServeHTTP(webhookRec, webhookReq)

	if webhookRec.Code != http.StatusOK {
		t.Fatalf("expected checkout completion webhook status 200, got %d", webhookRec.Code)
	}

	customer, err := env.store.GetBillingCustomer(context.Background(), user.ID, "stripe")
	if err != nil {
		t.Fatalf("expected checkout completion to persist customer mapping, got %v", err)
	}
	if customer.ProviderCustomerID != "cus_checkout_only" {
		t.Fatalf("unexpected billing customer mapping after checkout completion: %+v", customer)
	}

	runtimeReq := httptest.NewRequest(http.MethodGet, "/runtime-config.js", nil)
	runtimeReq.AddCookie(cookie)
	runtimeRec := httptest.NewRecorder()
	env.handler.ServeHTTP(runtimeRec, runtimeReq)

	if runtimeRec.Code != http.StatusOK {
		t.Fatalf("expected runtime config status 200, got %d", runtimeRec.Code)
	}
	runtimeBody := runtimeRec.Body.String()
	for _, want := range []string{
		`"billingAvailable":true`,
		`"isAuthenticated":true`,
		`"syncUsable":false`,
		`"entitlement":{"hostedSync":false}`,
	} {
		if !strings.Contains(runtimeBody, want) {
			t.Fatalf("expected runtime config body to contain %q after checkout completion only, got %q", want, runtimeBody)
		}
	}

	apiReq := httptest.NewRequest(http.MethodGet, "/api/document/meta?appId=postbaby-web", nil)
	apiReq.AddCookie(cookie)
	apiRec := httptest.NewRecorder()
	env.handler.ServeHTTP(apiRec, apiReq)

	if apiRec.Code != http.StatusForbidden {
		t.Fatalf("expected checkout completion alone to leave sync gated, got %d body=%q", apiRec.Code, apiRec.Body.String())
	}
	if !strings.Contains(apiRec.Body.String(), `"code":"entitlement_required"`) {
		t.Fatalf("expected entitlement_required after checkout completion only, got %q", apiRec.Body.String())
	}
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

type serverTestOptions struct {
	billingProvider billing.Provider
	publicBaseURL   string
}

func newServerTestEnv(t *testing.T, deploymentMode config.DeploymentMode) *serverTestEnv {
	return newServerTestEnvWithOptions(t, deploymentMode, serverTestOptions{})
}

func newServerTestEnvWithOptions(t *testing.T, deploymentMode config.DeploymentMode, options serverTestOptions) *serverTestEnv {
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
	entitlementManager := entitlement.NewManager(docStore)
	billingService := billing.NewServiceWithProvider(docStore, options.billingProvider, options.publicBaseURL)
	apiHandler := httpapi.NewHandler(docStore, authManager, entitlementManager, deploymentMode)
	return &serverTestEnv{
		handler:     NewHandler(apiHandler, authManager, entitlementManager, billingService, staticDir, deploymentMode),
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

func grantHostedSyncEntitlement(t *testing.T, env *serverTestEnv, userID int64) {
	t.Helper()

	if _, err := env.store.PutAccountEntitlement(
		context.Background(),
		userID,
		store.EntitlementKeyHostedSync,
		store.EntitlementStatusActive,
		store.EntitlementSourceManual,
		nil,
	); err != nil {
		t.Fatalf("grant hosted sync entitlement: %v", err)
	}
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

func newFormRequest(method, target string, form url.Values, origin, host string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if host != "" {
		req.Host = host
	}
	return req
}

type serverFakeBillingProvider struct {
	available        bool
	name             string
	createCustomerID string
	checkoutURL      string
	portalURL        string
	webhookEvent     billing.WebhookEvent
	webhookErr       error
	checkoutInput    *billing.CheckoutSessionInput
	portalInput      *billing.PortalSessionInput
}

func (p *serverFakeBillingProvider) Name() string {
	return p.name
}

func (p *serverFakeBillingProvider) Available() bool {
	return p.available
}

func (p *serverFakeBillingProvider) CreateCustomer(ctx context.Context, user *store.User) (string, error) {
	return p.createCustomerID, nil
}

func (p *serverFakeBillingProvider) CreateCheckoutSession(ctx context.Context, input billing.CheckoutSessionInput) (string, error) {
	copied := input
	p.checkoutInput = &copied
	return p.checkoutURL, nil
}

func (p *serverFakeBillingProvider) CreatePortalSession(ctx context.Context, input billing.PortalSessionInput) (string, error) {
	copied := input
	p.portalInput = &copied
	return p.portalURL, nil
}

func (p *serverFakeBillingProvider) ParseWebhook(ctx context.Context, rawBody []byte, signatureHeader string) (billing.WebhookEvent, error) {
	if p.webhookErr != nil {
		return billing.WebhookEvent{}, p.webhookErr
	}
	if signatureHeader != "valid" {
		return billing.WebhookEvent{}, billing.ErrInvalidWebhookSignature
	}
	return p.webhookEvent, nil
}

func timePointer(value time.Time) *time.Time {
	utcValue := value.UTC()
	return &utcValue
}
