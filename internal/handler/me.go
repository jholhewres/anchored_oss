package handler

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/jholhewres/anchored_oss/internal/middleware"
	"github.com/jholhewres/anchored_oss/internal/store"
)

// MeHandler returns the caller's profile. Any authenticated scope may use it.
type MeHandler struct {
	store  store.Store
	logger *slog.Logger
}

func NewMeHandler(st store.Store, logger *slog.Logger) *MeHandler {
	return &MeHandler{store: st, logger: logger}
}

type meResponse struct {
	AccountID   string `json:"account_id"`
	OrgID       string `json:"org_id"`
	Scope       string `json:"scope"`
	Email       string `json:"email,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
}

func (h *MeHandler) Get(w http.ResponseWriter, r *http.Request) {
	accountID := middleware.GetAccountID(r.Context())
	orgID := middleware.GetOrgID(r.Context())
	scope := middleware.GetScope(r.Context())
	if accountID == "" || orgID == "" {
		jsonError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	resp := meResponse{AccountID: accountID, OrgID: orgID, Scope: scope}
	if acc, err := h.store.GetAccountByID(r.Context(), accountID); err == nil && acc != nil {
		resp.Email = acc.Email
		resp.DisplayName = acc.DisplayName
	} else if err != nil && !errors.Is(err, store.ErrNotFound) {
		h.logger.Warn("me: account lookup failed", "error", err)
	}

	jsonResponse(w, http.StatusOK, resp)
}
