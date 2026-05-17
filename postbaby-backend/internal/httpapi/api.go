package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"postbaby-backend/internal/auth"
	"postbaby-backend/internal/config"
	"postbaby-backend/internal/httpcache"
	"postbaby-backend/internal/store"
)

const MaxDocumentBodyBytes int64 = 4 << 20

type API struct {
	store          store.DocumentStore
	authManager    *auth.Manager
	deploymentMode config.DeploymentMode
}

type errorResponse struct {
	OK    bool      `json:"ok"`
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code           string `json:"code"`
	Message        string `json:"message"`
	CurrentVersion *int64 `json:"currentVersion,omitempty"`
}

type healthResponse struct {
	OK bool `json:"ok"`
}

type getDocumentResponse struct {
	OK        bool            `json:"ok"`
	AppID     string          `json:"appId"`
	Data      json.RawMessage `json:"data"`
	Version   int64           `json:"version"`
	UpdatedAt string          `json:"updatedAt"`
}

type getDocumentMetaResponse struct {
	OK        bool   `json:"ok"`
	AppID     string `json:"appId"`
	Exists    bool   `json:"exists"`
	Version   int64  `json:"version,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

type putDocumentRequest struct {
	AppID   string          `json:"appId"`
	Data    json.RawMessage `json:"data"`
	Version *int64          `json:"version"`
}

type putDocumentResponse struct {
	OK        bool   `json:"ok"`
	AppID     string `json:"appId"`
	Version   int64  `json:"version"`
	UpdatedAt string `json:"updatedAt"`
}

type frontendSnapshot map[string]string

func NewHandler(docStore store.DocumentStore, authManager *auth.Manager, deploymentMode config.DeploymentMode) http.Handler {
	api := &API{
		store:          docStore,
		authManager:    authManager,
		deploymentMode: deploymentMode,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", api.handleHealth)
	mux.HandleFunc("/api/document/meta", api.handleDocumentMeta)
	mux.HandleFunc("/api/document", api.handleDocument)
	return mux
}

func (a *API) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}

	if err := a.store.Health(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "unhealthy", "database health check failed")
		return
	}

	writeJSON(w, http.StatusOK, healthResponse{OK: true})
}

func (a *API) handleDocument(w http.ResponseWriter, r *http.Request) {
	httpcache.SetNoStore(w)

	if !a.syncEnabled() {
		writeError(w, http.StatusNotFound, "not_found", "sync is not enabled for this deployment")
		return
	}

	switch r.Method {
	case http.MethodGet:
		a.handleGetDocument(w, r)
	case http.MethodPut:
		a.handlePutDocument(w, r)
	default:
		w.Header().Set("Allow", strings.Join([]string{http.MethodGet, http.MethodPut}, ", "))
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
	}
}

func (a *API) handleDocumentMeta(w http.ResponseWriter, r *http.Request) {
	httpcache.SetNoStore(w)

	if !a.syncEnabled() {
		writeError(w, http.StatusNotFound, "not_found", "sync is not enabled for this deployment")
		return
	}

	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}

	user, err := a.requireUser(w, r)
	if err != nil {
		return
	}

	appID := strings.TrimSpace(r.URL.Query().Get("appId"))
	if appID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "appId is required")
		return
	}

	meta, err := a.store.GetDocumentMeta(r.Context(), user.OwnerKey, appID)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrDocumentNotFound):
			writeJSON(w, http.StatusOK, getDocumentMetaResponse{
				OK:     true,
				AppID:  appID,
				Exists: false,
			})
		default:
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to load document metadata")
		}
		return
	}

	writeJSON(w, http.StatusOK, getDocumentMetaResponse{
		OK:        true,
		AppID:     meta.AppID,
		Exists:    true,
		Version:   meta.Version,
		UpdatedAt: meta.UpdatedAt.UTC().Format(time.RFC3339),
	})
}

func (a *API) handleGetDocument(w http.ResponseWriter, r *http.Request) {
	user, err := a.requireUser(w, r)
	if err != nil {
		return
	}

	appID := strings.TrimSpace(r.URL.Query().Get("appId"))
	if appID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "appId is required")
		return
	}

	doc, err := a.store.GetDocument(r.Context(), user.OwnerKey, appID)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrDocumentNotFound):
			writeError(w, http.StatusNotFound, "document_not_found", "document not found")
		default:
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to load document")
		}
		return
	}

	writeJSON(w, http.StatusOK, getDocumentResponse{
		OK:        true,
		AppID:     doc.AppID,
		Data:      doc.Body,
		Version:   doc.Version,
		UpdatedAt: doc.UpdatedAt.UTC().Format(time.RFC3339),
	})
}

func (a *API) handlePutDocument(w http.ResponseWriter, r *http.Request) {
	user, err := a.requireUser(w, r)
	if err != nil {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, MaxDocumentBodyBytes)
	defer r.Body.Close()

	var req putDocumentRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeDecodeError(w, err)
		return
	}

	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must contain a single JSON object")
		return
	}

	req.AppID = strings.TrimSpace(req.AppID)
	if req.AppID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "appId is required")
		return
	}

	if req.Data == nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "data is required")
		return
	}

	if err := validateFrontendSnapshot(req.Data); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "data must be the current frontend localStorage snapshot object with string values")
		return
	}

	if req.Version != nil && *req.Version < 0 {
		writeError(w, http.StatusBadRequest, "invalid_request", "version must be zero or greater")
		return
	}

	doc, err := a.store.PutDocument(r.Context(), user.OwnerKey, req.AppID, cloneJSON(req.Data), req.Version)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrVersionConflict):
			currentVersion, _ := store.CurrentVersionFromConflict(err)
			writeVersionConflict(w, currentVersion)
		default:
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to save document")
		}
		return
	}

	writeJSON(w, http.StatusOK, putDocumentResponse{
		OK:        true,
		AppID:     doc.AppID,
		Version:   doc.Version,
		UpdatedAt: doc.UpdatedAt.UTC().Format(time.RFC3339),
	})
}

func writeDecodeError(w http.ResponseWriter, err error) {
	var maxBytesErr *http.MaxBytesError
	switch {
	case errors.As(err, &maxBytesErr):
		writeError(w, http.StatusRequestEntityTooLarge, "request_too_large", "request body exceeds 4 MiB limit")
	case errors.Is(err, io.EOF):
		writeError(w, http.StatusBadRequest, "invalid_json", "request body is required")
	default:
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must be valid JSON")
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeErrorBody(w, status, errorBody{
		Code:    code,
		Message: message,
	})
}

func writeVersionConflict(w http.ResponseWriter, currentVersion *int64) {
	writeErrorBody(w, http.StatusConflict, errorBody{
		Code:           "version_conflict",
		Message:        "document version conflict",
		CurrentVersion: currentVersion,
	})
}

func writeErrorBody(w http.ResponseWriter, status int, body errorBody) {
	writeJSON(w, status, errorResponse{
		OK:    false,
		Error: body,
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func cloneJSON(value json.RawMessage) json.RawMessage {
	return append(json.RawMessage(nil), value...)
}

func validateFrontendSnapshot(value json.RawMessage) error {
	var snapshot frontendSnapshot
	if err := json.Unmarshal(value, &snapshot); err != nil {
		return err
	}

	if snapshot == nil {
		return errors.New("snapshot must be an object")
	}

	trimmed := bytes.TrimSpace(value)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return errors.New("snapshot must be an object")
	}

	return nil
}

func (a *API) requireUser(w http.ResponseWriter, r *http.Request) (*store.User, error) {
	user, err := a.authManager.AuthenticateRequest(r.Context(), w, r)
	if err != nil {
		if errors.Is(err, auth.ErrUnauthorized) {
			writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return nil, err
		}

		writeError(w, http.StatusInternalServerError, "internal_error", "failed to authenticate request")
		return nil, err
	}

	return user, nil
}

func (a *API) syncEnabled() bool {
	return a.deploymentMode == config.DeploymentModeSelfHostedSingleUser ||
		a.deploymentMode == config.DeploymentModeCloudMultiUser
}
