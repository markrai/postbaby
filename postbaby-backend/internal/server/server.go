package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"postbaby-backend/internal/auth"
	"postbaby-backend/internal/config"
	"postbaby-backend/internal/httpcache"
	"postbaby-backend/internal/store"
)

type Server struct {
	authManager    *auth.Manager
	staticDir      string
	staticHandler  http.Handler
	apiHandler     http.Handler
	deploymentMode config.DeploymentMode
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

type runtimeConfig struct {
	DeploymentMode   string `json:"deploymentMode"`
	AuthAvailable    bool   `json:"authAvailable"`
	AuthRequired     bool   `json:"authRequired"`
	IsAuthenticated  bool   `json:"isAuthenticated"`
	SyncAvailable    bool   `json:"syncAvailable"`
	SyncRequiresAuth bool   `json:"syncRequiresAuth"`
	SetupAvailable   bool   `json:"setupAvailable"`
	APIBase          string `json:"apiBase"`
}

func NewHandler(apiHandler http.Handler, authManager *auth.Manager, staticDir string, deploymentMode config.DeploymentMode) http.Handler {
	server := &Server{
		authManager:    authManager,
		staticDir:      staticDir,
		staticHandler:  http.FileServer(http.Dir(staticDir)),
		apiHandler:     apiHandler,
		deploymentMode: deploymentMode,
	}

	mux := http.NewServeMux()
	mux.Handle("/api/", server.apiHandler)
	mux.HandleFunc("/runtime-config.js", server.handleRuntimeConfig)
	if server.setupEnabled() {
		mux.HandleFunc("/setup", server.handleSetup)
	}
	if server.loginEnabled() {
		mux.HandleFunc("/login", server.handleLogin)
	}
	if server.logoutEnabled() {
		mux.HandleFunc("/logout", server.handleLogout)
	}
	mux.HandleFunc("/", server.handleRoot)
	return mux
}

func (s *Server) handleRuntimeConfig(w http.ResponseWriter, r *http.Request) {
	httpcache.SetNoStore(w)

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
		renderAuthPage(w, http.StatusOK, authPageData{
			Title:             "Postbaby Login",
			Heading:           "Sign in to Postbaby",
			Body:              "Use the local account configured on this server.",
			Action:            "/login",
			SubmitLabel:       "Sign in",
			PasswordMinLength: auth.PasswordMinLength(),
		})
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
	page := authPageData{
		Title:             "Postbaby Login",
		Heading:           "Sign in to Postbaby",
		Body:              "Use the local account configured on this server.",
		Action:            "/login",
		SubmitLabel:       "Sign in",
		Username:          username,
		PasswordMinLength: auth.PasswordMinLength(),
	}

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

	http.Redirect(w, r, "/login", http.StatusSeeOther)
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
	config := runtimeConfig{
		DeploymentMode:   string(s.deploymentMode),
		AuthAvailable:    s.authAvailable(),
		AuthRequired:     s.appShellAuthRequired(),
		IsAuthenticated:  false,
		SyncAvailable:    s.syncEnabled(),
		SyncRequiresAuth: s.syncRequiresAuth(),
		SetupAvailable:   s.setupEnabled(),
		APIBase:          "",
	}

	if !s.authAvailable() {
		return config, nil
	}

	if _, err := s.authManager.AuthenticateRequest(r.Context(), w, r); err != nil {
		if errors.Is(err, auth.ErrUnauthorized) {
			return config, nil
		}
		return runtimeConfig{}, err
	}

	config.IsAuthenticated = true
	return config, nil
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
	return s.deploymentMode == config.DeploymentModeSelfHostedSingleUser
}

func (s *Server) appShellAuthRequired() bool {
	return s.deploymentMode == config.DeploymentModeSelfHostedSingleUser
}

func (s *Server) setupEnabled() bool {
	return s.deploymentMode == config.DeploymentModeSelfHostedSingleUser
}

func (s *Server) loginEnabled() bool {
	return s.deploymentMode == config.DeploymentModeSelfHostedSingleUser
}

func (s *Server) logoutEnabled() bool {
	return s.deploymentMode == config.DeploymentModeSelfHostedSingleUser
}

func (s *Server) syncEnabled() bool {
	return s.deploymentMode == config.DeploymentModeSelfHostedSingleUser
}

func (s *Server) syncRequiresAuth() bool {
	return s.deploymentMode == config.DeploymentModeSelfHostedSingleUser
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
