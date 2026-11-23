package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"github.com/jholhewres/anchored_oss/internal/auth"
	"github.com/jholhewres/anchored_oss/internal/middleware"
	"github.com/jholhewres/anchored_oss/internal/model"
	"github.com/jholhewres/anchored_oss/internal/store"
)

// uuidRe matches canonical lowercase UUIDs. Tight enough for path params.
var uuidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type APIKeyHandler struct {
	store  store.Store
	logger *slog.Logger
}

func NewAPIKeyHandler(st store.Store, logger *slog.Logger) *APIKeyHandler {
	return &APIKeyHandler{store: st, logger: logger}
}

type createKeyRequest struct {
	Name      string `json:"name"`
	Scope     string `json:"scope"`
	AccountID string `json:"account_id"`
	ExpiresIn string `json:"expires_in,omitempty"`
}

type createKeyResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Key       string `json:"key"`
	Scope     string `json:"scope"`
	CreatedAt string `json:"created_at"`
	ExpiresAt string `json:"expires_at,omitempty"`
}

var allowedScopes = map[string]bool{"sync": true, "readonly": true, "admin": true}

// expiresInWindows maps the UI selector values to a duration. Empty string
// means never expires. Anything else is rejected.
var expiresInWindows = map[string]time.Duration{
	"":    0,
	"7d":  7 * 24 * time.Hour,
	"30d": 30 * 24 * time.Hour,
	"90d": 90 * 24 * time.Hour,
}

func (h *APIKeyHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Name == "" || req.Scope == "" || req.AccountID == "" {
		jsonError(w, http.StatusBadRequest, "name, scope, and account_id are required")
		return
	}
	if req.Name == auth.SessionKeyName {
		jsonError(w, http.StatusBadRequest, "name session is reserved for login credentials")
		return
	}
	if !allowedScopes[req.Scope] {
		jsonError(w, http.StatusBadRequest, "scope must be one of: sync, readonly, admin")
		return
	}
	window, ok := expiresInWindows[req.ExpiresIn]
	if !ok {
		jsonError(w, http.StatusBadRequest, "expires_in must be empty, 7d, 30d, or 90d")
		return
	}

	orgID := middleware.GetOrgID(r.Context())
	if orgID == "" {
		jsonError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	role, err := h.store.GetOrgMemberRole(r.Context(), orgID, req.AccountID)
	if errors.Is(err, store.ErrNotFound) {
		jsonError(w, http.StatusNotFound, "account not found")
		return
	}
	if err != nil {
		h.logger.Error("check api key account membership failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "internal error")
		return
	}
	// A key never outranks its account: a role change revokes the keys above
	// the new role, and minting one above the current role would undo that.
	if !auth.ScopeAllowedForRole(role, req.Scope) {
		jsonError(w, http.StatusBadRequest, "scope exceeds the account's role")
		return
	}

	fullKey, prefix, hash, err := auth.GenerateAPIKey()
	if err != nil {
		h.logger.Error("failed to generate API key", "error", err)
		jsonError(w, http.StatusInternalServerError, "internal error")
		return
	}

	var expiresAt *time.Time
	if window > 0 {
		t := time.Now().UTC().Add(window)
		expiresAt = &t
	}

	apiKey, err := h.store.CreateAPIKey(r.Context(), orgID, req.AccountID, req.Name, prefix, hash, req.Scope, expiresAt)
	if err != nil {
		h.logger.Error("failed to store API key", "error", err)
		jsonError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// An admin minting a key in another account's name leaves a trace. Only
	// metadata: never the secret or its hash.
	if err := h.store.AppendAudit(r.Context(), &model.AuditEntry{
		OrgID:      orgID,
		ActorID:    middleware.GetAccountID(r.Context()),
		Action:     "api_key.created",
		TargetType: "api_key",
		TargetID:   apiKey.ID,
		Metadata:   map[string]string{"account_id": req.AccountID, "scope": apiKey.Scope, "name": apiKey.Name},
	}); err != nil {
		h.logger.Error("audit api key create failed", "error", err)
	}

	resp := createKeyResponse{
		ID:        apiKey.ID,
		Name:      apiKey.Name,
		Key:       fullKey,
		Scope:     apiKey.Scope,
		CreatedAt: apiKey.CreatedAt.UTC().Format(time.RFC3339),
	}
	if apiKey.ExpiresAt != nil {
		resp.ExpiresAt = apiKey.ExpiresAt.UTC().Format(time.RFC3339)
	}
	jsonResponse(w, http.StatusCreated, resp)
}

func (h *APIKeyHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" || !uuidRe.MatchString(id) {
		jsonError(w, http.StatusBadRequest, "id must be a UUID")
		return
	}

	orgID := middleware.GetOrgID(r.Context())
	if orgID == "" {
		jsonError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if err := h.store.RevokeAPIKey(r.Context(), orgID, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			jsonError(w, http.StatusNotFound, "api key not found")
			return
		}
		h.logger.Error("failed to revoke API key", "error", err)
		jsonError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *APIKeyHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	if orgID == "" {
		jsonError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	keys, err := h.store.ListAPIKeysByOrg(r.Context(), orgID)
	if err != nil {
		h.logger.Error("list api keys failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "list api keys failed")
		return
	}
	if keys == nil {
		keys = []*model.APIKey{}
	}
	jsonResponse(w, http.StatusOK, keys)
}
