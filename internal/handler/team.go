package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/jholhewres/anchored_oss/internal/middleware"
	"github.com/jholhewres/anchored_oss/internal/model"
	"github.com/jholhewres/anchored_oss/internal/store"
)

type TeamHandler struct {
	store  store.Store
	logger *slog.Logger
}

func NewTeamHandler(st store.Store, logger *slog.Logger) *TeamHandler {
	return &TeamHandler{store: st, logger: logger}
}

type createTeamRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type teamMemberRequest struct {
	AccountID string `json:"account_id"`
}

func (h *TeamHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	if orgID == "" {
		jsonError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	teams, err := h.store.ListTeamsByOrg(r.Context(), orgID)
	if err != nil {
		h.logger.Error("list teams failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "list teams failed")
		return
	}
	if teams == nil {
		teams = []*model.Team{}
	}
	jsonResponse(w, http.StatusOK, teams)
}

func (h *TeamHandler) Create(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	if orgID == "" {
		jsonError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req createTeamRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		jsonError(w, http.StatusBadRequest, "name is required")
		return
	}
	if !slugRe.MatchString(req.Slug) {
		jsonError(w, http.StatusBadRequest, "slug must match ^[a-z0-9][a-z0-9-]{0,63}$")
		return
	}

	team, err := h.store.CreateTeam(r.Context(), orgID, req.Name, req.Slug)
	if err != nil {
		h.logger.Error("create team failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "create team failed")
		return
	}
	jsonResponse(w, http.StatusCreated, team)
}

func (h *TeamHandler) GetDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !uuidRe.MatchString(id) {
		jsonError(w, http.StatusBadRequest, "id must be a UUID")
		return
	}
	detail, err := h.store.GetTeamDetail(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		jsonError(w, http.StatusNotFound, "team not found")
		return
	}
	if err != nil {
		h.logger.Error("get team detail failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "get team detail failed")
		return
	}
	jsonResponse(w, http.StatusOK, detail)
}

func (h *TeamHandler) AddMember(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !uuidRe.MatchString(id) {
		jsonError(w, http.StatusBadRequest, "id must be a UUID")
		return
	}
	var req teamMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !uuidRe.MatchString(req.AccountID) {
		jsonError(w, http.StatusBadRequest, "account_id must be a UUID")
		return
	}
	if err := h.store.AddTeamMember(r.Context(), id, req.AccountID); err != nil {
		h.logger.Error("add team member failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "add team member failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *TeamHandler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	accountID := r.PathValue("account_id")
	if !uuidRe.MatchString(id) || !uuidRe.MatchString(accountID) {
		jsonError(w, http.StatusBadRequest, "id and account_id must be UUIDs")
		return
	}
	if err := h.store.RemoveTeamMember(r.Context(), id, accountID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			jsonError(w, http.StatusNotFound, "membership not found")
			return
		}
		h.logger.Error("remove team member failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "remove team member failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
