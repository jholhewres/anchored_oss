package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/mail"
	"strings"

	"github.com/jholhewres/anchored_oss/internal/middleware"
	"github.com/jholhewres/anchored_oss/internal/model"
	"github.com/jholhewres/anchored_oss/internal/store"
	"golang.org/x/crypto/bcrypt"
)

type AccountHandler struct {
	store  store.Store
	logger *slog.Logger
}

func NewAccountHandler(st store.Store, logger *slog.Logger) *AccountHandler {
	return &AccountHandler{store: st, logger: logger}
}

type createAccountRequest struct {
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Password    string `json:"password"`
}

type createAccountResponse struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Created     bool   `json:"created"`
}

func (h *AccountHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	if orgID == "" {
		jsonError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	accounts, err := h.store.ListAccountsByOrg(r.Context(), orgID)
	if err != nil {
		h.logger.Error("list accounts failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "list accounts failed")
		return
	}
	jsonResponse(w, http.StatusOK, accounts)
}

func (h *AccountHandler) Create(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	if orgID == "" {
		jsonError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req createAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Email = strings.TrimSpace(req.Email)
	req.DisplayName = strings.TrimSpace(req.DisplayName)
	req.Password = strings.TrimSpace(req.Password)
	if req.DisplayName == "" {
		jsonError(w, http.StatusBadRequest, "display_name is required")
		return
	}
	if _, err := mail.ParseAddress(req.Email); err != nil {
		jsonError(w, http.StatusBadRequest, "email is not a valid address")
		return
	}

	var acc *model.Account
	var created bool
	var err error

	if req.Password != "" {
		passwordHash, hashErr := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if hashErr != nil {
			h.logger.Error("password hashing failed", "error", hashErr)
			jsonError(w, http.StatusInternalServerError, "password hashing failed")
			return
		}
		acc, created, err = h.store.GetOrCreateAccountByEmail(r.Context(), orgID, req.Email, req.DisplayName)
		if err != nil {
			h.logger.Error("create account failed", "error", err)
			jsonError(w, http.StatusInternalServerError, "create account failed")
			return
		}
		if acc.PasswordHash == "" {
			if err := h.store.SetAccountPassword(r.Context(), acc.ID, string(passwordHash)); err != nil {
				h.logger.Error("set password failed", "error", err)
				jsonError(w, http.StatusInternalServerError, "set password failed")
				return
			}
		}
	} else {
		acc, created, err = h.store.GetOrCreateAccountByEmail(r.Context(), orgID, req.Email, req.DisplayName)
		if err != nil {
			h.logger.Error("create account failed", "error", err)
			jsonError(w, http.StatusInternalServerError, "create account failed")
			return
		}
	}

	if created {
		if err := h.store.AddOrgMember(r.Context(), orgID, acc.ID, "member"); err != nil {
			h.logger.Error("add org member failed", "error", err)
			jsonError(w, http.StatusInternalServerError, "add org member failed")
			return
		}
		if err := h.store.EnsureDefaultTeamMembership(r.Context(), orgID, acc.ID); err != nil {
			h.logger.Error("ensure default team membership failed", "error", err)
			jsonError(w, http.StatusInternalServerError, "wire default team failed")
			return
		}
	}

	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	jsonResponse(w, status, createAccountResponse{
		ID: acc.ID, Email: acc.Email, DisplayName: acc.DisplayName, Created: created,
	})
}
