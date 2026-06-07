package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"postbaby-backend/internal/auth"
	"postbaby-backend/internal/config"
	"postbaby-backend/internal/entitlement"
	"postbaby-backend/internal/httpcache"
	"postbaby-backend/internal/store"
)

const (
	MaxDocumentBodyBytes            int64 = 4 << 20
	MaxSyncMutationsBodyBytes       int64 = 1 << 20
	maxSyncMutationBatchSize              = 100
	maxSyncMutationPayloadBytes           = 32 << 10
	maxSyncMutationIDLength               = 255
	maxSyncMutationReplicaIDLength        = 255
	maxSyncMutationEntityTypeLength       = 64
	maxSyncMutationEntityIDLength         = 255
	maxSyncMutationOperationLength        = 64
	syncMutationProtocol                  = "PB-SYNC/1"
)

var allowedSyncMutationOperations = map[string]struct{}{
	"CreateNode": {},
	"UpdateNode": {},
	"MoveNode":   {},
	"DeleteNode": {},
	"CreateEdge": {},
	"DeleteEdge": {},
}

var errEntitlementRequired = errors.New("entitlement required")

type API struct {
	store                   store.DocumentStore
	authManager             *auth.Manager
	entitlementManager      *entitlement.Manager
	deploymentMode          config.DeploymentMode
	enableSyncDeltaMetadata bool
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
	AppID              string          `json:"appId"`
	Data               json.RawMessage `json:"data"`
	Version            *int64          `json:"version"`
	BaseServerRevision *int64          `json:"baseServerRevision"`
}

type putDocumentResponse struct {
	OK        bool   `json:"ok"`
	AppID     string `json:"appId"`
	Version   int64  `json:"version"`
	UpdatedAt string `json:"updatedAt"`
}

type postSyncMutationsRequest struct {
	AppID     string                    `json:"appId"`
	Mutations []postSyncMutationRequest `json:"mutations"`
}

type postSyncMutationRequest struct {
	Protocol      string          `json:"protocol"`
	ClientID      string          `json:"clientId"`
	DeviceID      string          `json:"deviceId"`
	MutationID    string          `json:"mutationId"`
	BaseRevision  *int64          `json:"baseRevision"`
	EntityType    string          `json:"entityType"`
	EntityID      string          `json:"entityId"`
	OperationType string          `json:"operationType"`
	Payload       json.RawMessage `json:"payload"`
}

type postSyncMutationsResponse struct {
	OK      bool                    `json:"ok"`
	AppID   string                  `json:"appId"`
	Results []syncMutationAckResult `json:"results"`
}

type getSyncDeltaResponse struct {
	OK                       bool                           `json:"ok"`
	AppID                    string                         `json:"appId"`
	CurrentDocumentVersion   int64                          `json:"currentDocumentVersion"`
	CurrentDocumentHash      string                         `json:"currentDocumentHash"`
	ClientVersion            int64                          `json:"clientVersion"`
	RequiresSnapshotRefresh  bool                           `json:"requiresSnapshotRefresh"`
	Reason                   string                         `json:"reason"`
	ApplicationWatermark     *int64                         `json:"applicationWatermark"`
	NextApplicationWatermark *int64                         `json:"nextApplicationWatermark"`
	Applications             []getSyncDeltaApplicationEntry `json:"applications"`
	Warnings                 []string                       `json:"warnings"`
}

type getSyncDeltaApplicationEntry struct {
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

type syncMutationAckResult struct {
	MutationID string                `json:"mutationId"`
	Status     string                `json:"status"`
	Duplicate  bool                  `json:"duplicate,omitempty"`
	AcceptedAt string                `json:"acceptedAt,omitempty"`
	Error      *syncMutationAckError `json:"error,omitempty"`
}

type syncMutationAckError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type frontendSnapshot map[string]string

func NewHandler(docStore store.DocumentStore, authManager *auth.Manager, entitlementManager *entitlement.Manager, deploymentMode config.DeploymentMode, enableSyncDeltaMetadata bool) http.Handler {
	api := &API{
		store:                   docStore,
		authManager:             authManager,
		entitlementManager:      entitlementManager,
		deploymentMode:          deploymentMode,
		enableSyncDeltaMetadata: enableSyncDeltaMetadata,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", api.handleHealth)
	mux.HandleFunc("/api/document/meta", api.handleDocumentMeta)
	mux.HandleFunc("/api/document", api.handleDocument)
	mux.HandleFunc("/api/sync/mutations", api.handleSyncMutations)
	mux.HandleFunc("/api/sync/delta", api.handleSyncDelta)
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

	user, err := a.requireDocumentReadUser(w, r)
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
	user, err := a.requireDocumentReadUser(w, r)
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
	user, err := a.requireDocumentWriteUser(w, r)
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

	expectedVersion, ok := normalizeExpectedVersion(w, req)
	if !ok {
		return
	}
	if expectedVersion != nil && *expectedVersion < 0 {
		writeError(w, http.StatusBadRequest, "invalid_request", "version must be zero or greater")
		return
	}

	if expectedVersion == nil {
		if _, err := a.store.GetDocumentMeta(r.Context(), user.OwnerKey, req.AppID); err == nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "version is required when replacing an existing document")
			return
		} else if err != nil && !errors.Is(err, store.ErrDocumentNotFound) {
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to load document metadata")
			return
		}
	}

	doc, err := a.store.PutDocument(r.Context(), user.OwnerKey, req.AppID, cloneJSON(req.Data), expectedVersion)
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

func (a *API) handleSyncDelta(w http.ResponseWriter, r *http.Request) {
	httpcache.SetNoStore(w)

	if !a.syncDeltaMetadataEnabled() {
		writeError(w, http.StatusNotFound, "not_found", "sync delta metadata is not enabled for this deployment")
		return
	}

	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}

	user, err := a.requireDocumentReadUser(w, r)
	if err != nil {
		return
	}

	appID := strings.TrimSpace(r.URL.Query().Get("appId"))
	if appID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "appId is required")
		return
	}

	sinceVersionRaw := strings.TrimSpace(r.URL.Query().Get("sinceVersion"))
	if sinceVersionRaw == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "sinceVersion is required")
		return
	}
	sinceVersion, err := strconv.ParseInt(sinceVersionRaw, 10, 64)
	if err != nil || sinceVersion < 0 {
		writeError(w, http.StatusBadRequest, "invalid_request", "sinceVersion must be a non-negative integer")
		return
	}

	var applicationWatermark *int64
	applicationWatermarkRaw := strings.TrimSpace(r.URL.Query().Get("applicationWatermark"))
	if applicationWatermarkRaw != "" {
		parsedApplicationWatermark, parseErr := strconv.ParseInt(applicationWatermarkRaw, 10, 64)
		if parseErr != nil || parsedApplicationWatermark < 0 {
			writeError(w, http.StatusBadRequest, "invalid_request", "applicationWatermark must be a non-negative integer")
			return
		}
		applicationWatermark = &parsedApplicationWatermark
	}

	includeApplications := false
	includeApplicationsRaw := strings.TrimSpace(r.URL.Query().Get("includeApplications"))
	if includeApplicationsRaw != "" {
		parsedIncludeApplications, parseErr := strconv.ParseBool(includeApplicationsRaw)
		if parseErr != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "includeApplications must be a boolean")
			return
		}
		includeApplications = parsedIncludeApplications
	}

	limit := 0
	limitRaw := strings.TrimSpace(r.URL.Query().Get("limit"))
	if limitRaw != "" {
		parsedLimit, parseErr := strconv.Atoi(limitRaw)
		if parseErr != nil || parsedLimit <= 0 {
			writeError(w, http.StatusBadRequest, "invalid_request", "limit must be a positive integer")
			return
		}
		limit = parsedLimit
	}

	result, err := a.store.GetSyncDeltaMetadata(r.Context(), user.OwnerKey, appID, store.SyncDeltaMetadataOptions{
		SinceVersion:         sinceVersion,
		ApplicationWatermark: applicationWatermark,
		IncludeApplications:  includeApplications,
		Limit:                limit,
	})
	if err != nil {
		switch {
		case errors.Is(err, store.ErrDocumentNotFound):
			writeError(w, http.StatusNotFound, "document_not_found", "document not found")
		default:
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to load sync delta metadata")
		}
		return
	}

	writeJSON(w, http.StatusOK, buildSyncDeltaResponse(result))
}

func (a *API) handleSyncMutations(w http.ResponseWriter, r *http.Request) {
	httpcache.SetNoStore(w)

	if !a.syncEnabled() {
		writeError(w, http.StatusNotFound, "not_found", "sync is not enabled for this deployment")
		return
	}

	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}

	user, err := a.requireDocumentWriteUser(w, r)
	if err != nil {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, MaxSyncMutationsBodyBytes)
	defer r.Body.Close()

	var req postSyncMutationsRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeSyncMutationsDecodeError(w, err)
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

	if len(req.Mutations) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_request", "mutations must contain at least one envelope")
		return
	}
	if len(req.Mutations) > maxSyncMutationBatchSize {
		writeError(w, http.StatusBadRequest, "invalid_request", "mutation batch exceeds limit")
		return
	}

	responseResults := make([]syncMutationAckResult, len(req.Mutations))
	acceptedInputs := make([]store.SyncMutationReceiptInput, 0, len(req.Mutations))
	acceptedIndexes := make([]int, 0, len(req.Mutations))

	for index, mutationReq := range req.Mutations {
		acceptedInput, rejectedResult := validateSyncMutationEnvelope(mutationReq)
		if rejectedResult != nil {
			responseResults[index] = *rejectedResult
			continue
		}

		acceptedInputs = append(acceptedInputs, acceptedInput)
		acceptedIndexes = append(acceptedIndexes, index)
	}

	if len(acceptedInputs) > 0 {
		acceptedResults, err := a.store.AcceptSyncMutationReceipts(r.Context(), user.OwnerKey, req.AppID, acceptedInputs)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to store mutation receipts")
			return
		}
		if len(acceptedResults) != len(acceptedInputs) {
			writeError(w, http.StatusInternalServerError, "internal_error", "failed to reconcile mutation receipts")
			return
		}

		for resultIndex, acceptedResult := range acceptedResults {
			responseResults[acceptedIndexes[resultIndex]] = syncMutationAckResult{
				MutationID: acceptedResult.Receipt.MutationID,
				Status:     store.SyncMutationReceiptStatusAccepted,
				Duplicate:  acceptedResult.Duplicate,
				AcceptedAt: acceptedResult.Receipt.AcceptedAt.UTC().Format(time.RFC3339),
			}
		}
	}

	writeJSON(w, http.StatusOK, postSyncMutationsResponse{
		OK:      true,
		AppID:   req.AppID,
		Results: responseResults,
	})
}

func normalizeExpectedVersion(w http.ResponseWriter, req putDocumentRequest) (*int64, bool) {
	if req.Version != nil && req.BaseServerRevision != nil && *req.Version != *req.BaseServerRevision {
		writeError(w, http.StatusBadRequest, "invalid_request", "version and baseServerRevision must match")
		return nil, false
	}
	if req.Version != nil {
		return req.Version, true
	}
	return req.BaseServerRevision, true
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

func writeSyncMutationsDecodeError(w http.ResponseWriter, err error) {
	var maxBytesErr *http.MaxBytesError
	switch {
	case errors.As(err, &maxBytesErr):
		writeError(w, http.StatusRequestEntityTooLarge, "request_too_large", "request body exceeds 1 MiB limit")
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

func validateSyncMutationEnvelope(req postSyncMutationRequest) (store.SyncMutationReceiptInput, *syncMutationAckResult) {
	mutationID := strings.TrimSpace(req.MutationID)
	if mutationID == "" || len(mutationID) > maxSyncMutationIDLength {
		return store.SyncMutationReceiptInput{}, rejectedSyncMutationAckResult(mutationID, "invalid_mutation_id", "mutationId is required and must be bounded")
	}

	protocol := strings.TrimSpace(req.Protocol)
	if protocol != syncMutationProtocol {
		return store.SyncMutationReceiptInput{}, rejectedSyncMutationAckResult(mutationID, "invalid_protocol", "protocol must be PB-SYNC/1")
	}

	clientID := strings.TrimSpace(req.ClientID)
	if len(clientID) > maxSyncMutationReplicaIDLength {
		return store.SyncMutationReceiptInput{}, rejectedSyncMutationAckResult(mutationID, "invalid_client_id", "clientId must be bounded")
	}

	deviceID := strings.TrimSpace(req.DeviceID)
	if len(deviceID) > maxSyncMutationReplicaIDLength {
		return store.SyncMutationReceiptInput{}, rejectedSyncMutationAckResult(mutationID, "invalid_device_id", "deviceId must be bounded")
	}

	entityType := strings.TrimSpace(req.EntityType)
	if entityType == "" || len(entityType) > maxSyncMutationEntityTypeLength {
		return store.SyncMutationReceiptInput{}, rejectedSyncMutationAckResult(mutationID, "invalid_entity_type", "entityType is required and must be bounded")
	}

	entityID := strings.TrimSpace(req.EntityID)
	if entityID == "" || len(entityID) > maxSyncMutationEntityIDLength {
		return store.SyncMutationReceiptInput{}, rejectedSyncMutationAckResult(mutationID, "invalid_entity_id", "entityId is required and must be bounded")
	}

	operationType := strings.TrimSpace(req.OperationType)
	if operationType == "" || len(operationType) > maxSyncMutationOperationLength {
		return store.SyncMutationReceiptInput{}, rejectedSyncMutationAckResult(mutationID, "invalid_operation_type", "operationType is required and must be bounded")
	}
	if _, ok := allowedSyncMutationOperations[operationType]; !ok {
		return store.SyncMutationReceiptInput{}, rejectedSyncMutationAckResult(mutationID, "invalid_operation_type", "operationType is not supported")
	}

	if req.BaseRevision != nil && *req.BaseRevision < 0 {
		return store.SyncMutationReceiptInput{}, rejectedSyncMutationAckResult(mutationID, "invalid_base_revision", "baseRevision must be zero or greater")
	}

	if err := validateSyncMutationPayload(req.Payload); err != nil {
		if errors.Is(err, errSyncMutationPayloadTooLarge) {
			return store.SyncMutationReceiptInput{}, rejectedSyncMutationAckResult(mutationID, "payload_too_large", "payload exceeds size limit")
		}
		return store.SyncMutationReceiptInput{}, rejectedSyncMutationAckResult(mutationID, "invalid_payload", "payload must be a JSON object within the allowed size limit")
	}

	return store.SyncMutationReceiptInput{
		MutationID:    mutationID,
		ClientID:      clientID,
		DeviceID:      deviceID,
		Protocol:      protocol,
		EntityType:    entityType,
		EntityID:      entityID,
		OperationType: operationType,
		Payload:       cloneJSON(req.Payload),
		BaseRevision:  req.BaseRevision,
	}, nil
}

var errSyncMutationPayloadTooLarge = errors.New("sync mutation payload too large")

func validateSyncMutationPayload(value json.RawMessage) error {
	if value == nil {
		return errors.New("payload is required")
	}

	trimmed := bytes.TrimSpace(value)
	if len(trimmed) == 0 {
		return errors.New("payload is required")
	}
	if len(trimmed) > maxSyncMutationPayloadBytes {
		return errSyncMutationPayloadTooLarge
	}
	if trimmed[0] != '{' {
		return errors.New("payload must be an object")
	}
	if !json.Valid(trimmed) {
		return errors.New("payload must be valid JSON")
	}

	return nil
}

func rejectedSyncMutationAckResult(mutationID, code, message string) *syncMutationAckResult {
	return &syncMutationAckResult{
		MutationID: mutationID,
		Status:     "rejected",
		Error: &syncMutationAckError{
			Code:    code,
			Message: message,
		},
	}
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

func buildSyncDeltaResponse(metadata store.SyncDeltaMetadata) getSyncDeltaResponse {
	applications := make([]getSyncDeltaApplicationEntry, 0, len(metadata.Applications))
	for _, application := range metadata.Applications {
		applications = append(applications, getSyncDeltaApplicationEntry{
			MutationID:                     application.MutationID,
			ApplicationStatus:              application.ApplicationStatus,
			ApplicationReason:              application.ApplicationReason,
			CanonicalDocumentVersionBefore: application.CanonicalDocumentVersionBefore,
			CanonicalDocumentVersionAfter:  application.CanonicalDocumentVersionAfter,
			CanonicalDocumentHashBefore:    application.CanonicalDocumentHashBefore,
			CanonicalDocumentHashAfter:     application.CanonicalDocumentHashAfter,
			ReplayObservationID:            application.ReplayObservationID,
			CreatedAt:                      application.CreatedAt.UTC().Format(time.RFC3339),
		})
	}

	warnings := append([]string{}, metadata.Warnings...)
	if warnings == nil {
		warnings = []string{}
	}

	return getSyncDeltaResponse{
		OK:                       true,
		AppID:                    metadata.AppID,
		CurrentDocumentVersion:   metadata.CurrentDocumentVersion,
		CurrentDocumentHash:      metadata.CurrentDocumentHash,
		ClientVersion:            metadata.ClientVersion,
		RequiresSnapshotRefresh:  metadata.RequiresSnapshotRefresh,
		Reason:                   metadata.Reason,
		ApplicationWatermark:     metadata.ApplicationWatermark,
		NextApplicationWatermark: metadata.NextApplicationWatermark,
		Applications:             applications,
		Warnings:                 warnings,
	}
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

func (a *API) requireDocumentReadUser(w http.ResponseWriter, r *http.Request) (*store.User, error) {
	user, err := a.requireUser(w, r)
	if err != nil {
		return nil, err
	}

	readAllowed, err := a.documentReadAllowedForUser(r.Context(), user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to evaluate account entitlement")
		return nil, err
	}
	if !readAllowed {
		writeError(w, http.StatusForbidden, "entitlement_required", "hosted sync is not enabled for this account")
		return nil, errEntitlementRequired
	}

	return user, nil
}

func (a *API) requireDocumentWriteUser(w http.ResponseWriter, r *http.Request) (*store.User, error) {
	user, err := a.requireUser(w, r)
	if err != nil {
		return nil, err
	}

	syncUsable, err := a.syncUsableForUser(r.Context(), user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to evaluate account entitlement")
		return nil, err
	}
	if !syncUsable {
		writeError(w, http.StatusForbidden, "entitlement_required", "hosted sync is not enabled for this account")
		return nil, errEntitlementRequired
	}

	return user, nil
}

func (a *API) syncEnabled() bool {
	return a.deploymentMode == config.DeploymentModeSelfHosted ||
		a.deploymentMode == config.DeploymentModeCloud
}

func (a *API) syncDeltaMetadataEnabled() bool {
	return a.enableSyncDeltaMetadata && a.deploymentMode == config.DeploymentModeSelfHosted
}

func (a *API) syncUsableForUser(ctx context.Context, user *store.User) (bool, error) {
	if a.deploymentMode == config.DeploymentModeSelfHosted {
		return true, nil
	}
	if a.deploymentMode != config.DeploymentModeCloud {
		return false, nil
	}

	return a.entitlementManager.HostedSyncGranted(ctx, user.ID)
}

func (a *API) documentReadAllowedForUser(ctx context.Context, user *store.User) (bool, error) {
	if a.deploymentMode == config.DeploymentModeSelfHosted {
		return true, nil
	}
	if a.deploymentMode != config.DeploymentModeCloud {
		return false, nil
	}
	if user.AccountStatus == store.AccountStatusCheckoutPending {
		return false, nil
	}

	entitlement, exists, err := a.entitlementManager.HostedSyncEntitlement(ctx, user.ID)
	if err != nil {
		return false, err
	}
	if !exists {
		return false, nil
	}

	switch entitlement.Status {
	case store.EntitlementStatusActive,
		store.EntitlementStatusPastDue,
		store.EntitlementStatusCanceled,
		store.EntitlementStatusExpired:
		return true, nil
	default:
		return false, nil
	}
}
