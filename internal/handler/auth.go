package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/jholhewres/anchored_oss/internal/auth"
	"github.com/jholhewres/anchored_oss/internal/store"
	"golang.org/x/crypto/bcrypt"
)

type AuthHandler struct {
	store  store.Store
	logger *slog.Logger
}

func NewAuthHandler(st store.Store, logger *slog.Logger) *AuthHandler {
	return &AuthHandler{store: st, logger: logger}
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginResponse struct {
	APIKey    string `json:"api_key"`
	AccountID string `json:"account_id"`
	OrgID     string `json:"org_id"`
	Scope     string `json:"scope"`
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Email == "" || req.Password == "" {
		jsonError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	account, err := h.store.GetAccountByEmail(r.Context(), req.Email)
	if err != nil {
		jsonError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	if account.PasswordHash == "" {
		jsonError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(account.PasswordHash), []byte(req.Password)); err != nil {
		jsonError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	orgID, err := h.store.GetAccountOrgID(r.Context(), account.ID)
	if err != nil {
		h.logger.Error("login: failed to resolve org for account", "account_id", account.ID, "error", err)
		jsonError(w, http.StatusInternalServerError, "internal error")
		return
	}

	fullKey, prefix, hash, err := auth.GenerateAPIKey()
	if err != nil {
		h.logger.Error("login: failed to generate api key", "error", err)
		jsonError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if _, err := h.store.CreateAPIKey(r.Context(), orgID, account.ID, "session", prefix, hash, "admin", nil); err != nil {
		h.logger.Error("login: failed to store session key", "error", err)
		jsonError(w, http.StatusInternalServerError, "internal error")
		return
	}

	jsonResponse(w, http.StatusOK, loginResponse{
		APIKey:    fullKey,
		AccountID: account.ID,
		OrgID:     orgID,
		Scope:     "admin",
	})
}
