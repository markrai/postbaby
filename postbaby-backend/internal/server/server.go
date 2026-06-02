package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"postbaby-backend/internal/auth"
	"postbaby-backend/internal/billing"
	"postbaby-backend/internal/config"
	"postbaby-backend/internal/entitlement"
	"postbaby-backend/internal/httpcache"
	"postbaby-backend/internal/store"
)

type Server struct {
	authManager        *auth.Manager
	billingService     *billing.Service
	entitlementManager *entitlement.Manager
	staticDir          string
	staticHandler      http.Handler
	apiHandler         http.Handler
	deploymentMode     config.DeploymentMode
}

type authPageData struct {
	Title             string
	Heading           string
	Body              string
	Action            string
	SubmitLabel       string
	Error             string
	Username          string
	ShowConfirm       bool
	PasswordMinLength int
}

type runtimeAccount struct {
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	Email       string `json:"email"`
	AvatarURL   string `json:"avatarUrl"`
	IsAdmin     bool   `json:"isAdmin"`
	StorageKey  string `json:"storageKey"`
	Status      string `json:"status"`
}

type runtimeConfig struct {
	DeploymentMode   string `json:"deploymentMode"`
	AuthorityModel   string `json:"authorityModel"`
	AuthAvailable    bool   `json:"authAvailable"`
	AuthRequired     bool   `json:"authRequired"`
	IsAuthenticated  bool   `json:"isAuthenticated"`
	BillingAvailable bool   `json:"billingAvailable"`
	SyncAvailable    bool   `json:"syncAvailable"`
	SyncRequiresAuth bool   `json:"syncRequiresAuth"`
	SyncUsable       bool   `json:"syncUsable"`
	SyncPausedReason string `json:"syncPausedReason"`
	Entitlement      struct {
		HostedSync bool   `json:"hostedSync"`
		Status     string `json:"status"`
		ValidUntil string `json:"validUntil,omitempty"`
	} `json:"entitlement"`
	SetupAvailable bool            `json:"setupAvailable"`
	APIBase        string          `json:"apiBase"`
	Account        *runtimeAccount `json:"account"`
}

func NewHandler(apiHandler http.Handler, authManager *auth.Manager, entitlementManager *entitlement.Manager, billingService *billing.Service, staticDir string, deploymentMode config.DeploymentMode) http.Handler {
	server := &Server{
		authManager:        authManager,
		billingService:     billingService,
		entitlementManager: entitlementManager,
		staticDir:          staticDir,
		staticHandler:      http.FileServer(http.Dir(staticDir)),
		apiHandler:         apiHandler,
		deploymentMode:     deploymentMode,
	}

	mux := http.NewServeMux()
	mux.Handle("/api/", server.apiHandler)
	mux.HandleFunc("/runtime-config.js", server.handleRuntimeConfig)
	if server.setupEnabled() {
		mux.HandleFunc("/setup", server.handleSetup)
		mux.HandleFunc("/admin/accounts", server.handleAdminAccounts)
	}
	if server.signupEnabled() {
		mux.HandleFunc("/signup", server.handleSignup)
	}
	if server.loginEnabled() {
		mux.HandleFunc("/login", server.handleLogin)
	}
	if server.logoutEnabled() {
		mux.HandleFunc("/logout", server.handleLogout)
	}
	if server.billingRoutesEnabled() {
		mux.HandleFunc("/billing/checkout", server.handleBillingCheckout)
		mux.HandleFunc("/billing/portal", server.handleBillingPortal)
		mux.HandleFunc("/billing/webhook", server.handleBillingWebhook)
	}
	mux.HandleFunc("/", server.handleRoot)
	return mux
}

func (s *Server) handleRuntimeConfig(w http.ResponseWriter, r *http.Request) {
	httpcache.SetNoStore(w)
	s.cleanupExpiredProvisionalUsers(r)

	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", strings.Join([]string{http.MethodGet, http.MethodHead}, ", "))
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	config, err := s.currentRuntimeConfig(w, r)
	if err != nil {
		http.Error(w, "failed to build runtime config", http.StatusInternalServerError)
		return
	}

	payload, err := json.Marshal(config)
	if err != nil {
		http.Error(w, "failed to encode runtime config", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	if r.Method == http.MethodHead {
		return
	}

	_, _ = w.Write([]byte("window.POSTBABY_RUNTIME = "))
	_, _ = w.Write(payload)
	_, _ = w.Write([]byte(";\n"))
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/", "/index.html":
		s.handleAppShell(w, r)
	default:
		s.staticHandler.ServeHTTP(w, r)
	}
}

func (s *Server) handleAppShell(w http.ResponseWriter, r *http.Request) {
	httpcache.SetNoStore(w)

	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", strings.Join([]string{http.MethodGet, http.MethodHead}, ", "))
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !s.appShellAuthRequired() {
		s.serveAppShellFile(w, r)
		return
	}

	if s.setupEnabled() {
		setupRequired, err := s.authManager.SetupRequired(r.Context())
		if err != nil {
			http.Error(w, "failed to check setup status", http.StatusInternalServerError)
			return
		}
		if setupRequired {
			http.Redirect(w, r, "/setup", http.StatusSeeOther)
			return
		}
	}

	if _, err := s.authManager.AuthenticateRequest(r.Context(), w, r); err != nil {
		if errors.Is(err, auth.ErrUnauthorized) {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		http.Error(w, "failed to authenticate request", http.StatusInternalServerError)
		return
	}

	s.serveAppShellFile(w, r)
}

func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	httpcache.SetNoStore(w)

	if !s.setupEnabled() {
		http.NotFound(w, r)
		return
	}

	setupRequired, err := s.authManager.SetupRequired(r.Context())
	if err != nil {
		http.Error(w, "failed to check setup status", http.StatusInternalServerError)
		return
	}

	if !setupRequired {
		s.redirectAuthenticatedOrLogin(w, r)
		return
	}

	switch r.Method {
	case http.MethodGet:
		renderAuthPage(w, http.StatusOK, authPageData{
			Title:             "Postbaby Setup",
			Heading:           "Create the first Postbaby account",
			Body:              "This account becomes the initial owner and admin for this server.",
			Action:            "/setup",
			SubmitLabel:       "Create account",
			ShowConfirm:       true,
			PasswordMinLength: auth.PasswordMinLength(),
		})
	case http.MethodPost:
		s.handleSetupPost(w, r)
	default:
		w.Header().Set("Allow", strings.Join([]string{http.MethodGet, http.MethodPost}, ", "))
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleSetupPost(w http.ResponseWriter, r *http.Request) {
	if !isTrustedFormPost(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form submission", http.StatusBadRequest)
		return
	}

	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")
	confirmPassword := r.FormValue("confirmPassword")

	page := authPageData{
		Title:             "Postbaby Setup",
		Heading:           "Create the first Postbaby account",
		Body:              "This account becomes the initial owner and admin for this server.",
		Action:            "/setup",
		SubmitLabel:       "Create account",
		Username:          username,
		ShowConfirm:       true,
		PasswordMinLength: auth.PasswordMinLength(),
	}

	if username == "" {
		page.Error = "Enter a username."
		renderAuthPage(w, http.StatusBadRequest, page)
		return
	}
	if password == "" {
		page.Error = "Enter a password."
		renderAuthPage(w, http.StatusBadRequest, page)
		return
	}
	if len(password) < auth.PasswordMinLength() {
		page.Error = "Choose a longer password."
		renderAuthPage(w, http.StatusBadRequest, page)
		return
	}
	if confirmPassword == "" {
		page.Error = "Confirm the password."
		renderAuthPage(w, http.StatusBadRequest, page)
		return
	}
	if password != confirmPassword {
		page.Error = "Passwords do not match."
		renderAuthPage(w, http.StatusBadRequest, page)
		return
	}

	user, err := s.authManager.CreateInitialUser(r.Context(), username, password)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrSetupAlreadyComplete):
			http.Redirect(w, r, "/login", http.StatusSeeOther)
		case errors.Is(err, store.ErrUsernameTaken):
			page.Error = "That username is already in use."
			renderAuthPage(w, http.StatusConflict, page)
		case errors.Is(err, store.ErrBootstrapOwnerKeyConflict):
			page.Error = "This database has multiple document owners. Manual migration is required before setup can continue."
			renderAuthPage(w, http.StatusConflict, page)
		default:
			http.Error(w, "failed to create account", http.StatusInternalServerError)
		}
		return
	}

	if err := s.authManager.CreateSession(r.Context(), w, user.ID); err != nil {
		http.Error(w, "failed to start session", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	httpcache.SetNoStore(w)
	s.cleanupExpiredProvisionalUsers(r)

	if !s.loginEnabled() {
		http.NotFound(w, r)
		return
	}

	if s.setupEnabled() {
		setupRequired, err := s.authManager.SetupRequired(r.Context())
		if err != nil {
			http.Error(w, "failed to check setup status", http.StatusInternalServerError)
			return
		}
		if setupRequired {
			http.Redirect(w, r, "/setup", http.StatusSeeOther)
			return
		}
	}

	if r.Method == http.MethodGet {
		if _, err := s.authManager.AuthenticateRequest(r.Context(), w, r); err == nil {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		} else if err != nil && !errors.Is(err, auth.ErrUnauthorized) {
			http.Error(w, "failed to authenticate request", http.StatusInternalServerError)
			return
		}
		renderAuthPage(w, http.StatusOK, s.loginPageData(""))
		return
	}

	if r.Method != http.MethodPost {
		w.Header().Set("Allow", strings.Join([]string{http.MethodGet, http.MethodPost}, ", "))
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !isTrustedFormPost(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form submission", http.StatusBadRequest)
		return
	}

	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")
	page := s.loginPageData(username)

	if username == "" || password == "" {
		page.Error = "Enter both username and password."
		renderAuthPage(w, http.StatusBadRequest, page)
		return
	}

	if _, err := s.authManager.Login(r.Context(), w, username, password); err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			page.Error = "Username or password was not recognized."
			renderAuthPage(w, http.StatusUnauthorized, page)
			return
		}

		http.Error(w, "failed to sign in", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleSignup(w http.ResponseWriter, r *http.Request) {
	httpcache.SetNoStore(w)
	s.cleanupExpiredProvisionalUsers(r)

	if !s.signupEnabled() {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodGet:
		if _, err := s.authManager.AuthenticateRequest(r.Context(), w, r); err == nil {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		} else if err != nil && !errors.Is(err, auth.ErrUnauthorized) {
			http.Error(w, "failed to authenticate request", http.StatusInternalServerError)
			return
		}
		renderAuthPage(w, http.StatusOK, s.signupPageData(""))
	case http.MethodPost:
		s.handleSignupPost(w, r)
	default:
		w.Header().Set("Allow", strings.Join([]string{http.MethodGet, http.MethodPost}, ", "))
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleSignupPost(w http.ResponseWriter, r *http.Request) {
	if !isTrustedFormPost(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form submission", http.StatusBadRequest)
		return
	}

	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")
	confirmPassword := r.FormValue("confirmPassword")
	page := s.signupPageData(username)

	if username == "" {
		page.Error = "Enter a username."
		renderAuthPage(w, http.StatusBadRequest, page)
		return
	}
	if password == "" {
		page.Error = "Enter a password."
		renderAuthPage(w, http.StatusBadRequest, page)
		return
	}
	if len(password) < auth.PasswordMinLength() {
		page.Error = "Choose a longer password."
		renderAuthPage(w, http.StatusBadRequest, page)
		return
	}
	if confirmPassword == "" {
		page.Error = "Confirm the password."
		renderAuthPage(w, http.StatusBadRequest, page)
		return
	}
	if password != confirmPassword {
		page.Error = "Passwords do not match."
		renderAuthPage(w, http.StatusBadRequest, page)
		return
	}

	if !s.billingAvailable() {
		page.Error = "Account sync upgrades are unavailable right now."
		renderAuthPage(w, http.StatusServiceUnavailable, page)
		return
	}

	user, err := s.authManager.CreateProvisionalUser(r.Context(), username, password, time.Now().UTC().Add(24*time.Hour))
	if err != nil {
		switch {
		case errors.Is(err, store.ErrUsernameTaken):
			page.Error = "Could not create that account. Try signing in or choose a different username."
			renderAuthPage(w, http.StatusConflict, page)
		default:
			http.Error(w, "failed to create account", http.StatusInternalServerError)
		}
		return
	}

	if err := s.authManager.CreateSession(r.Context(), w, user.ID); err != nil {
		http.Error(w, "failed to start session", http.StatusInternalServerError)
		return
	}

	redirectURL, err := s.billingService.CreateCheckoutSession(r.Context(), &user)
	if err != nil {
		http.Error(w, "failed to start billing checkout", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, redirectURL, http.StatusSeeOther)
}

func (s *Server) handleAdminAccounts(w http.ResponseWriter, r *http.Request) {
	httpcache.SetNoStore(w)

	if !s.setupEnabled() {
		http.NotFound(w, r)
		return
	}

	user, err := s.requireAuthenticatedUserOrLogin(w, r)
	if err != nil {
		return
	}
	if !user.IsAdmin {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	switch r.Method {
	case http.MethodGet:
		renderAuthPage(w, http.StatusOK, authPageData{
			Title:             "Postbaby Accounts",
			Heading:           "Create a Postbaby account",
			Body:              "Create another local account for this server.",
			Action:            "/admin/accounts",
			SubmitLabel:       "Create account",
			ShowConfirm:       true,
			PasswordMinLength: auth.PasswordMinLength(),
		})
	case http.MethodPost:
		s.handleAdminAccountsPost(w, r)
	default:
		w.Header().Set("Allow", strings.Join([]string{http.MethodGet, http.MethodPost}, ", "))
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAdminAccountsPost(w http.ResponseWriter, r *http.Request) {
	if !isTrustedFormPost(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form submission", http.StatusBadRequest)
		return
	}

	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")
	confirmPassword := r.FormValue("confirmPassword")
	page := authPageData{
		Title:             "Postbaby Accounts",
		Heading:           "Create a Postbaby account",
		Body:              "Create another local account for this server.",
		Action:            "/admin/accounts",
		SubmitLabel:       "Create account",
		Username:          username,
		ShowConfirm:       true,
		PasswordMinLength: auth.PasswordMinLength(),
	}

	if username == "" || password == "" {
		page.Error = "Enter both username and password."
		renderAuthPage(w, http.StatusBadRequest, page)
		return
	}
	if len(password) < auth.PasswordMinLength() {
		page.Error = "Choose a longer password."
		renderAuthPage(w, http.StatusBadRequest, page)
		return
	}
	if password != confirmPassword {
		page.Error = "Passwords do not match."
		renderAuthPage(w, http.StatusBadRequest, page)
		return
	}

	if _, err := s.authManager.CreateUser(r.Context(), username, password); err != nil {
		if errors.Is(err, store.ErrUsernameTaken) {
			page.Error = "That username is already in use."
			renderAuthPage(w, http.StatusConflict, page)
			return
		}
		http.Error(w, "failed to create account", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/admin/accounts", http.StatusSeeOther)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	httpcache.SetNoStore(w)

	if !s.logoutEnabled() {
		http.NotFound(w, r)
		return
	}

	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !isTrustedFormPost(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	if err := s.authManager.EndSession(r.Context(), w, r); err != nil {
		http.Error(w, "failed to sign out", http.StatusInternalServerError)
		return
	}

	if s.setupEnabled() {
		setupRequired, err := s.authManager.SetupRequired(r.Context())
		if err != nil {
			http.Error(w, "failed to check setup status", http.StatusInternalServerError)
			return
		}
		if setupRequired {
			http.Redirect(w, r, "/setup", http.StatusSeeOther)
			return
		}
	}

	if s.appShellAuthRequired() {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleBillingCheckout(w http.ResponseWriter, r *http.Request) {
	httpcache.SetNoStore(w)

	if !s.billingRoutesEnabled() {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !isTrustedFormPost(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	user, err := s.requireAuthenticatedUserOrLogin(w, r)
	if err != nil {
		return
	}

	redirectURL, err := s.billingService.CreateCheckoutSession(r.Context(), user)
	if err != nil {
		if errors.Is(err, billing.ErrBillingUnavailable) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "failed to start billing checkout", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, redirectURL, http.StatusSeeOther)
}

func (s *Server) handleBillingPortal(w http.ResponseWriter, r *http.Request) {
	httpcache.SetNoStore(w)

	if !s.billingRoutesEnabled() {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !isTrustedFormPost(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	user, err := s.requireAuthenticatedUserOrLogin(w, r)
	if err != nil {
		return
	}

	redirectURL, err := s.billingService.CreatePortalSession(r.Context(), user)
	if err != nil {
		switch {
		case errors.Is(err, billing.ErrBillingUnavailable):
			http.NotFound(w, r)
		case errors.Is(err, billing.ErrBillingCustomerNotFound):
			http.Error(w, "billing customer not found", http.StatusConflict)
		default:
			http.Error(w, "failed to open billing portal", http.StatusInternalServerError)
		}
		return
	}

	http.Redirect(w, r, redirectURL, http.StatusSeeOther)
}

func (s *Server) handleBillingWebhook(w http.ResponseWriter, r *http.Request) {
	httpcache.SetNoStore(w)

	if !s.billingRoutesEnabled() {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	defer r.Body.Close()

	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "invalid webhook request", http.StatusBadRequest)
		return
	}

	if err := s.billingService.HandleWebhook(r.Context(), rawBody, strings.TrimSpace(r.Header.Get("Stripe-Signature"))); err != nil {
		switch {
		case errors.Is(err, billing.ErrInvalidWebhookSignature):
			http.Error(w, "invalid webhook signature", http.StatusBadRequest)
		case errors.Is(err, billing.ErrBillingUnavailable):
			http.NotFound(w, r)
		default:
			log.Printf("billing webhook processing failed: %v", err)
			http.Error(w, "failed to process billing webhook", http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (s *Server) redirectAuthenticatedOrLogin(w http.ResponseWriter, r *http.Request) {
	if _, err := s.authManager.AuthenticateRequest(r.Context(), w, r); err == nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	} else if err != nil && !errors.Is(err, auth.ErrUnauthorized) {
		http.Error(w, "failed to authenticate request", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) currentRuntimeConfig(w http.ResponseWriter, r *http.Request) (runtimeConfig, error) {
	runtime := runtimeConfig{
		DeploymentMode:   string(s.deploymentMode),
		AuthorityModel:   s.authorityModel(),
		AuthAvailable:    s.authAvailable(),
		AuthRequired:     s.appShellAuthRequired(),
		IsAuthenticated:  false,
		BillingAvailable: s.billingAvailable(),
		SyncAvailable:    s.syncEnabled(),
		SyncRequiresAuth: s.syncRequiresAuth(),
		SyncUsable:       s.syncUsableWithoutAuthentication(),
		SyncPausedReason: s.defaultSyncPausedReason(),
		SetupAvailable:   s.setupEnabled(),
		APIBase:          "",
	}
	runtime.Entitlement.Status = store.EntitlementStatusNone

	if !s.authAvailable() {
		return runtime, nil
	}

	user, err := s.authManager.AuthenticateRequest(r.Context(), w, r)
	if err != nil {
		if errors.Is(err, auth.ErrUnauthorized) {
			return runtime, nil
		}
		return runtimeConfig{}, err
	}

	runtime.IsAuthenticated = true
	runtime.Account = &runtimeAccount{
		Username:    user.Username,
		DisplayName: user.Username,
		Email:       "",
		AvatarURL:   "",
		IsAdmin:     user.IsAdmin,
		StorageKey:  storageKeyForUser(s.deploymentMode, user),
		Status:      user.AccountStatus,
	}
	if s.deploymentMode == config.DeploymentModeCloudMultiUser {
		ent, exists, err := s.entitlementManager.HostedSyncEntitlement(r.Context(), user.ID)
		if err != nil {
			return runtimeConfig{}, err
		}
		if exists {
			runtime.Entitlement.Status = ent.Status
			if ent.ValidUntil != nil {
				runtime.Entitlement.ValidUntil = ent.ValidUntil.UTC().Format(time.RFC3339)
			}
		}
		hostedSyncGranted := exists && ent.Status == store.EntitlementStatusActive
		runtime.SyncUsable = hostedSyncGranted
		runtime.Entitlement.HostedSync = hostedSyncGranted
		if hostedSyncGranted {
			runtime.SyncPausedReason = ""
		} else if user.AccountStatus == store.AccountStatusCheckoutPending {
			runtime.SyncPausedReason = "checkout_pending"
		} else if exists {
			runtime.SyncPausedReason = "subscription_inactive"
		} else {
			runtime.SyncPausedReason = "subscription_required"
		}
	} else if s.deploymentMode == config.DeploymentModeSelfHostedSingleUser {
		runtime.SyncUsable = true
		runtime.SyncPausedReason = ""
	}
	return runtime, nil
}

func (s *Server) serveAppShellFile(w http.ResponseWriter, r *http.Request) {
	req := r.Clone(r.Context())
	if r.URL != nil {
		clonedURL := *r.URL
		clonedURL.Path = "/"
		clonedURL.RawPath = ""
		req.URL = &clonedURL
	}

	http.ServeFile(w, req, filepath.Join(s.staticDir, "index.html"))
}

func (s *Server) authAvailable() bool {
	return s.deploymentMode == config.DeploymentModeSelfHostedSingleUser ||
		s.deploymentMode == config.DeploymentModeCloudMultiUser
}

func (s *Server) appShellAuthRequired() bool {
	return s.deploymentMode == config.DeploymentModeSelfHostedSingleUser
}

func (s *Server) setupEnabled() bool {
	return s.deploymentMode == config.DeploymentModeSelfHostedSingleUser
}

func (s *Server) signupEnabled() bool {
	return s.deploymentMode == config.DeploymentModeCloudMultiUser && s.billingAvailable()
}

func (s *Server) loginEnabled() bool {
	return s.authAvailable()
}

func (s *Server) logoutEnabled() bool {
	return s.authAvailable()
}

func (s *Server) syncEnabled() bool {
	return s.deploymentMode == config.DeploymentModeSelfHostedSingleUser ||
		s.deploymentMode == config.DeploymentModeCloudMultiUser
}

func (s *Server) syncRequiresAuth() bool {
	return s.syncEnabled()
}

func (s *Server) billingAvailable() bool {
	return s.deploymentMode == config.DeploymentModeCloudMultiUser &&
		s.billingService != nil &&
		s.billingService.Available()
}

func (s *Server) syncUsableWithoutAuthentication() bool {
	return false
}

func (s *Server) billingRoutesEnabled() bool {
	return s.deploymentMode == config.DeploymentModeCloudMultiUser && s.billingAvailable()
}

func (s *Server) requireAuthenticatedUserOrLogin(w http.ResponseWriter, r *http.Request) (*store.User, error) {
	user, err := s.authManager.AuthenticateRequest(r.Context(), w, r)
	if err == nil {
		return user, nil
	}
	if errors.Is(err, auth.ErrUnauthorized) {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return nil, err
	}

	http.Error(w, "failed to authenticate request", http.StatusInternalServerError)
	return nil, err
}

func (s *Server) loginPageData(username string) authPageData {
	body := "Use the local account configured on this server."
	if s.deploymentMode == config.DeploymentModeCloudMultiUser {
		body = "Sign in to manage billing, reactivate sync, or access your paid account."
	}

	return authPageData{
		Title:             "Postbaby Login",
		Heading:           "Sign in to Postbaby",
		Body:              body,
		Action:            "/login",
		SubmitLabel:       "Sign in",
		Username:          username,
		PasswordMinLength: auth.PasswordMinLength(),
	}
}

func (s *Server) signupPageData(username string) authPageData {
	return authPageData{
		Title:             "Postbaby Upgrade",
		Heading:           "Upgrade Postbaby",
		Body:              "Create your account and continue to checkout to sync this board across your devices.",
		Action:            "/signup",
		SubmitLabel:       "Continue to checkout",
		Username:          username,
		ShowConfirm:       true,
		PasswordMinLength: auth.PasswordMinLength(),
	}
}

func (s *Server) authorityModel() string {
	switch s.deploymentMode {
	case config.DeploymentModeSelfHostedSingleUser:
		return "server_authoritative"
	case config.DeploymentModeCloudMultiUser:
		return "subscription_sync"
	default:
		return "browser_only"
	}
}

func (s *Server) defaultSyncPausedReason() string {
	if s.deploymentMode == config.DeploymentModeCloudMultiUser ||
		s.deploymentMode == config.DeploymentModeSelfHostedSingleUser {
		return "auth_required"
	}
	return ""
}

func storageKeyForUser(deploymentMode config.DeploymentMode, user *store.User) string {
	sum := sha256.Sum256([]byte(string(deploymentMode) + ":" + user.OwnerKey))
	return hex.EncodeToString(sum[:])
}

func (s *Server) cleanupExpiredProvisionalUsers(r *http.Request) {
	if s.deploymentMode != config.DeploymentModeCloudMultiUser {
		return
	}
	if count, err := s.authManager.CleanupExpiredProvisionalUsers(r.Context(), time.Now().UTC()); err != nil {
		log.Printf("cleanup expired provisional users failed: %v", err)
	} else if count > 0 {
		log.Printf("cleanup expired provisional users deleted=%d", count)
	}
}

func isTrustedFormPost(r *http.Request) bool {
	if r.Method != http.MethodPost {
		return false
	}

	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin != "" {
		return originMatchesHost(origin, r.Host)
	}

	referer := strings.TrimSpace(r.Header.Get("Referer"))
	if referer != "" {
		return originMatchesHost(referer, r.Host)
	}

	return true
}

func originMatchesHost(rawURL, host string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}

	return strings.EqualFold(parsed.Host, host)
}

func renderAuthPage(w http.ResponseWriter, status int, data authPageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_ = authPageTemplate.Execute(w, data)
}
