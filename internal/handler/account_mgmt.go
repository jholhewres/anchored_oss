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

// accountRoles are the org roles an admin may assign. "member" is what
// POST /v1/accounts gives new accounts; its login session gets key scope sync.
var accountRoles = map[string]bool{"admin": true, "sync": true, "readonly": true, "member": true}

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
	if req.Role != "" && !accountRoles[req.Role] {
		jsonError(w, http.StatusBadRequest, "role must be one of: admin, sync, readonly, member")
		return
	}
	orgID, ok := h.requireOrgMember(w, r, id)
	if !ok {
		return
	}

	if err := h.store.UpdateAccount(r.Context(), orgID, id, req.DisplayName, req.Role); err != nil {
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

	orgID, ok := h.requireOrgMember(w, r, id)
	if !ok {
		return
	}

	if err := h.store.SoftDeleteAccount(r.Context(), orgID, id); err != nil {
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

	orgID, ok := h.requireOrgMember(w, r, id)
	if !ok {
		return
	}

	projects, err := h.store.ListAccountProjects(r.Context(), orgID, id)
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
	// Only a member of the caller's org can be put on its default team: an
	// account of another org answers 404 and is never touched.
	orgID, ok := h.requireOrgMember(w, r, id)
	if !ok {
		return
	}
	if req.ProjectIDs == nil {
		req.ProjectIDs = []string{}
	}

	if err := h.store.SetAccountProjects(r.Context(), orgID, id, req.ProjectIDs); errors.Is(err, store.ErrNotFound) {
		jsonError(w, http.StatusNotFound, "account not found")
		return
	} else if err != nil {
		h.logger.Error("set account projects failed", "error", err, "account_id", id)
		jsonError(w, http.StatusInternalServerError, "set account projects failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// requireOrgMember resolves the caller's org and answers 404 when the target
// account is not a member of it, so admins cannot read or change accounts of
// other orgs (or learn that they exist).
func (h *AccountHandler) requireOrgMember(w http.ResponseWriter, r *http.Request, accountID string) (string, bool) {
	orgID := middleware.GetOrgID(r.Context())
	if orgID == "" {
		jsonError(w, http.StatusUnauthorized, "unauthorized")
		return "", false
	}
	member, err := h.store.IsOrgMember(r.Context(), orgID, accountID)
	if err != nil {
		h.logger.Error("check account org membership failed", "error", err, "account_id", accountID)
		jsonError(w, http.StatusInternalServerError, "internal error")
		return "", false
	}
	if !member {
		jsonError(w, http.StatusNotFound, "account not found")
		return "", false
	}
	return orgID, true
}
