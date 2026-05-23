package handler

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/jholhewres/anchored_oss/internal/middleware"
	"github.com/jholhewres/anchored_oss/internal/store"
)

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
}

type createKeyResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Key       string `json:"key"`
	Scope     string `json:"scope"`
	CreatedAt string `json:"created_at"`
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

	orgID := middleware.GetOrgID(r.Context())

	fullKey, prefix, hash, err := generateAPIKey()
	if err != nil {
		h.logger.Error("failed to generate API key", "error", err)
		jsonError(w, http.StatusInternalServerError, "internal error")
		return
	}

	apiKey, err := h.store.CreateAPIKey(r.Context(), orgID, req.AccountID, req.Name, prefix, hash, req.Scope, nil)
	if err != nil {
		h.logger.Error("failed to store API key", "error", err)
		jsonError(w, http.StatusInternalServerError, "internal error")
		return
	}

	jsonResponse(w, http.StatusCreated, createKeyResponse{
		ID:        apiKey.ID,
		Name:      apiKey.Name,
		Key:       fullKey,
		Scope:     apiKey.Scope,
		CreatedAt: apiKey.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}

func (h *APIKeyHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		jsonError(w, http.StatusBadRequest, "missing key id")
		return
	}

	if err := h.store.RevokeAPIKey(r.Context(), id); err != nil {
		h.logger.Error("failed to revoke API key", "error", err)
		jsonError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func generateAPIKey() (full, prefix, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", "", err
	}
	hexBytes := hex.EncodeToString(b)
	full = "anc_live_" + hexBytes
	prefix = full[:12]

	h256 := sha256.Sum256([]byte(full))
	hash = hex.EncodeToString(h256[:])

	return full, prefix, hash, nil
}
