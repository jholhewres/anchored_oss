package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	"github.com/jholhewres/anchored_oss/internal/middleware"
	"github.com/jholhewres/anchored_oss/internal/model"
	"github.com/jholhewres/anchored_oss/internal/store"
)

// GuardrailHandler exposes the per-org guardrail manager. All routes are
// admin-only (wired with requireAdmin in the server). Builtins (the security
// rules) can be toggled but not deleted, and their kind/value are immutable.
type GuardrailHandler struct {
	store  store.Store
	logger *slog.Logger
}

func NewGuardrailHandler(st store.Store, logger *slog.Logger) *GuardrailHandler {
	return &GuardrailHandler{store: st, logger: logger}
}

func (h *GuardrailHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	if orgID == "" {
		jsonError(w, http.StatusUnauthorized, "UNAUTHORIZED")
		return
	}
	guards, err := h.store.ListGuardrails(r.Context(), orgID)
	if err != nil {
		h.logger.Error("list guardrails failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "guardrail lookup failed")
		return
	}
	jsonResponse(w, http.StatusOK, guards)
}

type createGuardrailRequest struct {
	Kind        string `json:"kind"`
	Value       string `json:"value"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

func (h *GuardrailHandler) Create(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	if orgID == "" {
		jsonError(w, http.StatusUnauthorized, "UNAUTHORIZED")
		return
	}
	var req createGuardrailRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	req.Kind = strings.TrimSpace(req.Kind)
	req.Value = strings.TrimSpace(req.Value)
	req.Label = strings.TrimSpace(req.Label)

	// Only admin-managed kinds can be created; the security builtins are seeded.
	switch req.Kind {
	case model.GuardrailCategory, model.GuardrailRegex, model.GuardrailKeyword:
	default:
		jsonError(w, http.StatusBadRequest, "kind must be one of: category, regex, keyword")
		return
	}
	if req.Value == "" {
		jsonError(w, http.StatusBadRequest, "value is required")
		return
	}
	if req.Kind == model.GuardrailRegex {
		if _, err := regexp.Compile(req.Value); err != nil {
			jsonError(w, http.StatusBadRequest, "invalid regex: "+err.Error())
			return
		}
	}
	if req.Label == "" {
		req.Label = defaultGuardrailLabel(req.Kind, req.Value)
	}

	g := &model.Guardrail{
		OrgID:       orgID,
		Kind:        req.Kind,
		Value:       req.Value,
		Label:       req.Label,
		Description: req.Description,
		Enabled:     true,
		Builtin:     false,
	}
	if err := h.store.CreateGuardrail(r.Context(), g); err != nil {
		h.logger.Error("create guardrail failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "guardrail create failed")
		return
	}
	jsonResponse(w, http.StatusCreated, g)
}

type updateGuardrailRequest struct {
	Enabled     *bool   `json:"enabled"`
	Value       *string `json:"value"`
	Label       *string `json:"label"`
	Description *string `json:"description"`
}

func (h *GuardrailHandler) Update(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	if orgID == "" {
		jsonError(w, http.StatusUnauthorized, "UNAUTHORIZED")
		return
	}
	id := r.PathValue("id")
	if id == "" {
		jsonError(w, http.StatusBadRequest, "id is required")
		return
	}

	g, err := h.store.GetGuardrail(r.Context(), orgID, id)
	if errors.Is(err, store.ErrNotFound) {
		jsonError(w, http.StatusNotFound, "guardrail not found")
		return
	}
	if err != nil {
		h.logger.Error("get guardrail failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "guardrail lookup failed")
		return
	}

	var req updateGuardrailRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	// enabled is always toggleable, including on builtins.
	if req.Enabled != nil {
		g.Enabled = *req.Enabled
	}
	// Builtins are security rules: their value/label/description are fixed, only
	// the enabled toggle applies. Reject attempts to edit those fields.
	if g.Builtin {
		if req.Value != nil || req.Label != nil || req.Description != nil {
			jsonError(w, http.StatusBadRequest, "builtin guardrails can only be enabled/disabled, not edited")
			return
		}
	} else {
		if req.Label != nil {
			g.Label = strings.TrimSpace(*req.Label)
		}
		if req.Description != nil {
			g.Description = *req.Description
		}
		if req.Value != nil {
			v := strings.TrimSpace(*req.Value)
			if v == "" {
				jsonError(w, http.StatusBadRequest, "value cannot be empty")
				return
			}
			if g.Kind == model.GuardrailRegex {
				if _, cerr := regexp.Compile(v); cerr != nil {
					jsonError(w, http.StatusBadRequest, "invalid regex: "+cerr.Error())
					return
				}
			}
			g.Value = v
		}
	}

	if err := h.store.UpdateGuardrail(r.Context(), g); err != nil {
		h.logger.Error("update guardrail failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "guardrail update failed")
		return
	}
	jsonResponse(w, http.StatusOK, g)
}

func (h *GuardrailHandler) Delete(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	if orgID == "" {
		jsonError(w, http.StatusUnauthorized, "UNAUTHORIZED")
		return
	}
	id := r.PathValue("id")
	if id == "" {
		jsonError(w, http.StatusBadRequest, "id is required")
		return
	}

	g, err := h.store.GetGuardrail(r.Context(), orgID, id)
	if errors.Is(err, store.ErrNotFound) {
		jsonError(w, http.StatusNotFound, "guardrail not found")
		return
	}
	if err != nil {
		h.logger.Error("get guardrail failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "guardrail lookup failed")
		return
	}
	if g.Builtin {
		jsonError(w, http.StatusForbidden, "builtin guardrails cannot be deleted; disable it instead")
		return
	}

	if err := h.store.DeleteGuardrail(r.Context(), orgID, id); err != nil {
		h.logger.Error("delete guardrail failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "guardrail delete failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func defaultGuardrailLabel(kind, value string) string {
	switch kind {
	case model.GuardrailCategory:
		return "Block category: " + value
	case model.GuardrailRegex:
		return "Regex: " + value
	case model.GuardrailKeyword:
		return "Keyword: " + value
	default:
		return value
	}
}
