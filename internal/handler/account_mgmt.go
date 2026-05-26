package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jholhewres/anchored_oss/internal/middleware"
	"github.com/jholhewres/anchored_oss/internal/model"
	"github.com/jholhewres/anchored_oss/internal/store"
)

type updateAccountRequest struct {
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
}

type setAccountProjectsRequest struct {
	ProjectIDs []string `json:"project_ids"`
}

func (h *AccountHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !uuidRe.MatchString(id) {
		jsonError(w, http.StatusBadRequest, "id must be a UUID")
		return
	}

	var req updateAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.DisplayName == "" && req.Role == "" {
		jsonError(w, http.StatusBadRequest, "at least one of display_name or role is required")
		return
	}

	if err := h.store.UpdateAccount(r.Context(), id, req.DisplayName, req.Role); err != nil {
		h.logger.Error("update account failed", "error", err, "account_id", id)
		jsonError(w, http.StatusInternalServerError, "update account failed")
		return
	}

	acc, err := h.store.GetAccountByID(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		jsonError(w, http.StatusNotFound, "account not found")
		return
	}
	if err != nil {
		h.logger.Error("get account after update failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "internal error")
		return
	}
	jsonResponse(w, http.StatusOK, acc)
}

func (h *AccountHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !uuidRe.MatchString(id) {
		jsonError(w, http.StatusBadRequest, "id must be a UUID")
		return
	}

	// Prevent self-deletion.
	callerID := middleware.GetAccountID(r.Context())
	if callerID == id {
		jsonError(w, http.StatusBadRequest, "cannot delete your own account")
		return
	}

	if err := h.store.SoftDeleteAccount(r.Context(), id); err != nil {
		h.logger.Error("delete account failed", "error", err, "account_id", id)
		jsonError(w, http.StatusInternalServerError, "delete account failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AccountHandler) ListProjects(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !uuidRe.MatchString(id) {
		jsonError(w, http.StatusBadRequest, "id must be a UUID")
		return
	}

	projects, err := h.store.ListAccountProjects(r.Context(), id)
	if err != nil {
		h.logger.Error("list account projects failed", "error", err, "account_id", id)
		jsonError(w, http.StatusInternalServerError, "list account projects failed")
		return
	}
	if projects == nil {
		projects = make([]*model.Project, 0)
	}
	jsonResponse(w, http.StatusOK, projects)
}

func (h *AccountHandler) SetProjects(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	if orgID == "" {
		jsonError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	id := r.PathValue("id")
	if !uuidRe.MatchString(id) {
		jsonError(w, http.StatusBadRequest, "id must be a UUID")
		return
	}

	var req setAccountProjectsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.ProjectIDs == nil {
		req.ProjectIDs = []string{}
	}

	if err := h.store.SetAccountProjects(r.Context(), orgID, id, req.ProjectIDs); err != nil {
		h.logger.Error("set account projects failed", "error", err, "account_id", id)
		jsonError(w, http.StatusInternalServerError, "set account projects failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
