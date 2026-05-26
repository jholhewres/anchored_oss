package handler

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/jholhewres/anchored_oss/internal/auth"
	"github.com/jholhewres/anchored_oss/internal/middleware"
	"github.com/jholhewres/anchored_oss/internal/model"
	"github.com/jholhewres/anchored_oss/internal/store"
	"golang.org/x/crypto/bcrypt"
)

const inviteTokenExpiry = 7 * 24 * time.Hour

type InviteHandler struct {
	store  store.Store
	logger *slog.Logger
}

func NewInviteHandler(st store.Store, logger *slog.Logger) *InviteHandler {
	return &InviteHandler{store: st, logger: logger}
}

type createInviteRequest struct {
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
}

type createInviteResponse struct {
	ID        string    `json:"id"`
	InviteURL string    `json:"invite_url"`
	ExpiresAt time.Time `json:"expires_at"`
}

type acceptInviteRequest struct {
	Password string `json:"password"`
}

type acceptInviteResponse struct {
	APIKey    string `json:"api_key"`
	AccountID string `json:"account_id"`
}

func generateInviteToken() (token, tokenHash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	token = hex.EncodeToString(b)
	h := sha256.Sum256([]byte(token))
	tokenHash = hex.EncodeToString(h[:])
	return token, tokenHash, nil
}

func hashInviteToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func (h *InviteHandler) Create(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	accountID := middleware.GetAccountID(r.Context())
	if orgID == "" {
		jsonError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req createInviteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Email == "" || req.DisplayName == "" {
		jsonError(w, http.StatusBadRequest, "email and display_name are required")
		return
	}
	role := req.Role
	if role == "" {
		role = "sync"
	}

	token, tokenHash, err := generateInviteToken()
	if err != nil {
		h.logger.Error("invite: generate token failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "internal error")
		return
	}

	expiresAt := time.Now().UTC().Add(inviteTokenExpiry)
	inv, err := h.store.CreateInvite(r.Context(), orgID, req.Email, req.DisplayName, role, tokenHash, expiresAt, accountID)
	if err != nil {
		h.logger.Error("invite: create failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "failed to create invite")
		return
	}

	scheme := "https"
	if r.TLS == nil {
		scheme = "http"
	}
	host := r.Host
	if host == "" {
		host = "localhost"
	}
	inviteURL := scheme + "://" + host + "/invite/" + token

	jsonResponse(w, http.StatusCreated, createInviteResponse{
		ID:        inv.ID,
		InviteURL: inviteURL,
		ExpiresAt: expiresAt,
	})
}

func (h *InviteHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	if orgID == "" {
		jsonError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	invites, err := h.store.ListInvitesByOrg(r.Context(), orgID)
	if err != nil {
		h.logger.Error("invite: list failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "failed to list invites")
		return
	}
	if invites == nil {
		invites = make([]*model.Invite, 0)
	}
	jsonResponse(w, http.StatusOK, invites)
}

func (h *InviteHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !uuidRe.MatchString(id) {
		jsonError(w, http.StatusBadRequest, "id must be a UUID")
		return
	}
	if err := h.store.DeleteInvite(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			jsonError(w, http.StatusNotFound, "invite not found")
			return
		}
		h.logger.Error("invite: revoke failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "failed to revoke invite")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type getInviteResponse struct {
	Valid       bool   `json:"valid"`
	Email       string `json:"email,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
}

func (h *InviteHandler) Get(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	if token == "" {
		jsonError(w, http.StatusBadRequest, "token is required")
		return
	}
	tokenHash := hashInviteToken(token)
	inv, err := h.store.GetInviteByTokenHash(r.Context(), tokenHash)
	if errors.Is(err, store.ErrNotFound) {
		jsonResponse(w, http.StatusOK, getInviteResponse{Valid: false})
		return
	}
	if err != nil {
		h.logger.Error("invite: get failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if inv.AcceptedAt != nil || time.Now().UTC().After(inv.ExpiresAt) {
		jsonResponse(w, http.StatusOK, getInviteResponse{Valid: false})
		return
	}
	jsonResponse(w, http.StatusOK, getInviteResponse{
		Valid:       true,
		Email:       inv.Email,
		DisplayName: inv.DisplayName,
	})
}

func (h *InviteHandler) Accept(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	if token == "" {
		jsonError(w, http.StatusBadRequest, "token is required")
		return
	}
	tokenHash := hashInviteToken(token)
	inv, err := h.store.GetInviteByTokenHash(r.Context(), tokenHash)
	if errors.Is(err, store.ErrNotFound) {
		jsonError(w, http.StatusNotFound, "invite not found or already used")
		return
	}
	if err != nil {
		h.logger.Error("invite: accept lookup failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if inv.AcceptedAt != nil {
		jsonError(w, http.StatusConflict, "invite already accepted")
		return
	}
	if time.Now().UTC().After(inv.ExpiresAt) {
		jsonError(w, http.StatusGone, "invite has expired")
		return
	}

	var req acceptInviteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Password) < 8 {
		jsonError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		h.logger.Error("invite: hash password failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "internal error")
		return
	}

	acc, err := h.store.CreateAccount(r.Context(), inv.Email, inv.DisplayName, string(passwordHash))
	if err != nil {
		h.logger.Error("invite: create account failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "failed to create account")
		return
	}

	if err := h.store.AddOrgMember(r.Context(), inv.OrgID, acc.ID, inv.Role); err != nil {
		h.logger.Error("invite: add org member failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := h.store.EnsureDefaultTeamMembership(r.Context(), inv.OrgID, acc.ID); err != nil {
		h.logger.Error("invite: ensure default team failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "internal error")
		return
	}

	fullKey, prefix, keyHash, err := auth.GenerateAPIKey()
	if err != nil {
		h.logger.Error("invite: generate api key failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if _, err := h.store.CreateAPIKey(r.Context(), inv.OrgID, acc.ID, "default", prefix, keyHash, inv.Role, nil); err != nil {
		h.logger.Error("invite: create api key failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := h.store.MarkInviteAccepted(r.Context(), inv.ID); err != nil {
		h.logger.Warn("invite: mark accepted failed", "invite_id", inv.ID, "error", err)
	}

	jsonResponse(w, http.StatusCreated, acceptInviteResponse{
		APIKey:    fullKey,
		AccountID: acc.ID,
	})
}
