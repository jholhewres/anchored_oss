package handler

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jholhewres/anchored_oss/internal/auth"
	"github.com/jholhewres/anchored_oss/internal/middleware"
	"github.com/jholhewres/anchored_oss/internal/model"
	"github.com/jholhewres/anchored_oss/internal/store"
)

const (
	personalAPIKeyDefaultLifetime = 90 * 24 * time.Hour
	personalAPIKeyMaxDays         = 365
	personalAPIKeyMaxNameLength   = 128
)

var personalAPIKeyScopes = map[string]bool{"sync": true, "readonly": true}

// PersonalAPIKeyHandler serves API keys owned by the authenticated account.
// It deliberately has no admin bypass: ownership always comes from context.
type PersonalAPIKeyHandler struct {
	store  store.Store
	logger *slog.Logger
}

func NewPersonalAPIKeyHandler(st store.Store, logger *slog.Logger) *PersonalAPIKeyHandler {
	return &PersonalAPIKeyHandler{store: st, logger: logger}
}

type createPersonalAPIKeyRequest struct {
	Name      string          `json:"name"`
	Scope     string          `json:"scope"`
	ExpiresIn string          `json:"expires_in"`
	AccountID json.RawMessage `json:"account_id"`
}

// List handles GET /v1/me/api-keys.
func (h *PersonalAPIKeyHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID, accountID, ok := personalAPIKeyOwner(w, r)
	if !ok {
		return
	}
	keys, err := h.store.ListAPIKeysByOwner(r.Context(), orgID, accountID)
	if err != nil {
		h.logger.Error("list personal api keys failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "list api keys failed")
		return
	}
	if keys == nil {
		keys = []*model.APIKey{}
	}
	jsonResponse(w, http.StatusOK, keys)
}

// Create handles POST /v1/me/api-keys. The generated plaintext key is only
// included in this response; the store receives its SHA-256 hash instead.
func (h *PersonalAPIKeyHandler) Create(w http.ResponseWriter, r *http.Request) {
	orgID, accountID, ok := personalAPIKeyOwner(w, r)
	if !ok {
		return
	}

	var req createPersonalAPIKeyRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	if err := decoder.Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.AccountID) != 0 {
		jsonError(w, http.StatusBadRequest, "account_id is not allowed")
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len(req.Name) > personalAPIKeyMaxNameLength {
		jsonError(w, http.StatusBadRequest, "name is required and must be at most 128 characters")
		return
	}
	if req.Name == auth.SessionKeyName {
		jsonError(w, http.StatusBadRequest, "name session is reserved for login credentials")
		return
	}
	if !personalAPIKeyScopes[req.Scope] {
		jsonError(w, http.StatusBadRequest, "scope must be one of: sync, readonly")
		return
	}
	if !canCreatePersonalAPIKey(middleware.GetScope(r.Context()), req.Scope) {
		jsonError(w, http.StatusForbidden, "requested scope exceeds session scope")
		return
	}
	lifetime, err := personalAPIKeyLifetime(req.ExpiresIn)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "expires_in must be between 1d and 365d")
		return
	}

	fullKey, prefix, hash, err := auth.GenerateAPIKey()
	if err != nil {
		h.logger.Error("generate personal api key failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "internal error")
		return
	}
	expiresAt := time.Now().UTC().Add(lifetime)
	apiKey, err := h.store.CreateAPIKey(r.Context(), orgID, accountID, req.Name, prefix, hash, req.Scope, &expiresAt)
	if err != nil {
		h.logger.Error("store personal api key failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "internal error")
		return
	}
	metadata := map[string]string{
		"name":       apiKey.Name,
		"scope":      apiKey.Scope,
		"expires_at": apiKey.ExpiresAt.UTC().Format(time.RFC3339),
	}
	h.appendAudit(r, "api_key.personal_created", apiKey.ID, metadata)

	jsonResponse(w, http.StatusCreated, createKeyResponse{
		ID:        apiKey.ID,
		Name:      apiKey.Name,
		Key:       fullKey,
		Scope:     apiKey.Scope,
		CreatedAt: apiKey.CreatedAt.UTC().Format(time.RFC3339),
		ExpiresAt: apiKey.ExpiresAt.UTC().Format(time.RFC3339),
	})
}

// canCreatePersonalAPIKey prevents session credentials from minting a more
// capable persistent key. Admin and sync sessions may mint sync or readonly;
// readonly sessions may mint readonly only.
func canCreatePersonalAPIKey(sessionScope, requestedScope string) bool {
	switch sessionScope {
	case "admin", "sync":
		return requestedScope == "sync" || requestedScope == "readonly"
	case "readonly":
		return requestedScope == "readonly"
	default:
		return false
	}
}

// Revoke handles DELETE /v1/me/api-keys/{id}. A foreign ID is intentionally
// indistinguishable from a missing one.
func (h *PersonalAPIKeyHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	orgID, accountID, ok := personalAPIKeyOwner(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if id == "" || !uuidRe.MatchString(id) {
		jsonError(w, http.StatusBadRequest, "id must be a UUID")
		return
	}
	if err := h.store.RevokeAPIKeyByOwner(r.Context(), orgID, accountID, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			jsonError(w, http.StatusNotFound, "api key not found")
			return
		}
		h.logger.Error("revoke personal api key failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "internal error")
		return
	}
	h.appendAudit(r, "api_key.personal_revoked", id, map[string]string{})
	w.WriteHeader(http.StatusNoContent)
}

// appendAudit records only management metadata. It intentionally never
// receives the generated plaintext secret or its stored hash.
func (h *PersonalAPIKeyHandler) appendAudit(r *http.Request, action, keyID string, metadata map[string]string) {
	if err := h.store.AppendAudit(r.Context(), &model.AuditEntry{
		OrgID:      middleware.GetOrgID(r.Context()),
		ActorID:    middleware.GetAccountID(r.Context()),
		Action:     action,
		TargetType: "api_key",
		TargetID:   keyID,
		Metadata:   metadata,
	}); err != nil {
		h.logger.Error("audit personal api key failed", "action", action, "error", err)
	}
}

func personalAPIKeyOwner(w http.ResponseWriter, r *http.Request) (orgID, accountID string, ok bool) {
	orgID = middleware.GetOrgID(r.Context())
	accountID = middleware.GetAccountID(r.Context())
	if orgID == "" || accountID == "" {
		jsonError(w, http.StatusUnauthorized, "personal api keys require an account-scoped key")
		return "", "", false
	}
	return orgID, accountID, true
}

func personalAPIKeyLifetime(value string) (time.Duration, error) {
	if value == "" {
		return personalAPIKeyDefaultLifetime, nil
	}
	if !strings.HasSuffix(value, "d") {
		return 0, errors.New("invalid expiry unit")
	}
	days, err := strconv.Atoi(strings.TrimSuffix(value, "d"))
	if err != nil || days < 1 || days > personalAPIKeyMaxDays {
		return 0, errors.New("invalid expiry days")
	}
	lifetime := time.Duration(days) * 24 * time.Hour
	return lifetime, nil
}
