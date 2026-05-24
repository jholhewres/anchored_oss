package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	"github.com/jholhewres/anchored_oss/internal/auth"
	"github.com/jholhewres/anchored_oss/internal/store"
	"golang.org/x/crypto/bcrypt"
)

type RegisterHandler struct {
	store  store.Store
	logger *slog.Logger
}

func NewRegisterHandler(st store.Store, logger *slog.Logger) *RegisterHandler {
	return &RegisterHandler{store: st, logger: logger}
}

type registerRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"display_name"`
	OrgName     string `json:"org_name"`
	OrgSlug     string `json:"org_slug"`
}

type registerResponse struct {
	APIKey    string `json:"api_key"`
	AccountID string `json:"account_id"`
	OrgID     string `json:"org_id"`
	Scope     string `json:"scope"`
}

func (h *RegisterHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Email == "" || req.Password == "" || req.DisplayName == "" || req.OrgName == "" {
		jsonError(w, http.StatusBadRequest, "email, password, display_name, and org_name are required")
		return
	}

	if len(req.Password) < 8 {
		jsonError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}

	if !strings.Contains(req.Email, "@") || !strings.Contains(req.Email, ".") {
		jsonError(w, http.StatusBadRequest, "invalid email format")
		return
	}

	orgSlug := req.OrgSlug
	if orgSlug == "" {
		orgSlug = slugify(req.OrgName)
	}

	existing, err := h.store.GetAccountByEmail(r.Context(), req.Email)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		h.logger.Error("register: failed to check email", "error", err)
		jsonError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if existing != nil {
		jsonError(w, http.StatusConflict, "email already registered")
		return
	}

	org, err := h.store.CreateOrganization(r.Context(), req.OrgName, orgSlug)
	if err != nil {
		h.logger.Error("register: failed to create organization", "slug", orgSlug, "error", err)
		jsonError(w, http.StatusConflict, "organization slug already taken")
		return
	}

	passwordHash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		h.logger.Error("register: failed to hash password", "error", err)
		jsonError(w, http.StatusInternalServerError, "internal error")
		return
	}

	account, err := h.store.CreateAccount(r.Context(), req.Email, req.DisplayName, string(passwordHash))
	if err != nil {
		h.logger.Error("register: failed to create account", "error", err)
		jsonError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := h.store.AddOrgMember(r.Context(), org.ID, account.ID, "admin"); err != nil {
		h.logger.Error("register: failed to add org member", "error", err)
		jsonError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if err := h.store.EnsureDefaultTeamMembership(r.Context(), org.ID, account.ID); err != nil {
		h.logger.Error("register: failed to ensure default team membership", "error", err)
		jsonError(w, http.StatusInternalServerError, "internal error")
		return
	}

	fullKey, prefix, hash, err := auth.GenerateAPIKey()
	if err != nil {
		h.logger.Error("register: failed to generate api key", "error", err)
		jsonError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if _, err := h.store.CreateAPIKey(r.Context(), org.ID, account.ID, "default", prefix, hash, "admin", nil); err != nil {
		h.logger.Error("register: failed to store api key", "error", err)
		jsonError(w, http.StatusInternalServerError, "internal error")
		return
	}

	jsonResponse(w, http.StatusCreated, registerResponse{
		APIKey:    fullKey,
		AccountID: account.ID,
		OrgID:     org.ID,
		Scope:     "admin",
	})
}

var nonAlphaNum = regexp.MustCompile(`[^a-z0-9-]+`)
var multiHyphen = regexp.MustCompile(`-{2,}`)

func slugify(name string) string {
	s := strings.ToLower(name)
	s = strings.ReplaceAll(s, " ", "-")
	s = nonAlphaNum.ReplaceAllString(s, "")
	s = multiHyphen.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}
