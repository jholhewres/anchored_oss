package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/jholhewres/anchored_oss/internal/middleware"
	"github.com/jholhewres/anchored_oss/internal/model"
	"github.com/jholhewres/anchored_oss/internal/policy"
	"github.com/jholhewres/anchored_oss/internal/store"
)

// PolicyHandler exposes an org's customizable guardrails. Reads return the
// effective values (with defaults filled in) plus the always-on rules so the
// dashboard can show what cannot be disabled.
type PolicyHandler struct {
	store  store.Store
	logger *slog.Logger
}

func NewPolicyHandler(st store.Store, logger *slog.Logger) *PolicyHandler {
	return &PolicyHandler{store: st, logger: logger}
}

type policyResponse struct {
	BlockedCategories []string `json:"blocked_categories"`
	QualityThreshold  float64  `json:"quality_threshold"`
	NearDupThreshold  float64  `json:"near_dup_threshold"`
	// Always-on guardrails, surfaced read-only for the UI.
	AlwaysOn []string `json:"always_on"`
}

func (h *PolicyHandler) toResponse(p *model.OrgPolicy) policyResponse {
	blocked := p.BlockedCategories
	if len(blocked) == 0 {
		blocked = policy.DefaultBlockedCategories
	}
	return policyResponse{
		BlockedCategories: blocked,
		QualityThreshold:  p.QualityThreshold,
		NearDupThreshold:  p.NearDupThreshold,
		AlwaysOn:          []string{"secret_detection", "local_path_redaction", "user_scope_block"},
	}
}

func (h *PolicyHandler) Get(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	if orgID == "" {
		jsonError(w, http.StatusUnauthorized, "UNAUTHORIZED")
		return
	}
	p, err := h.store.GetOrgPolicy(r.Context(), orgID)
	if err != nil {
		h.logger.Error("get org policy failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "policy lookup failed")
		return
	}
	jsonResponse(w, http.StatusOK, h.toResponse(p))
}

type updatePolicyRequest struct {
	BlockedCategories []string `json:"blocked_categories"`
	QualityThreshold  float64  `json:"quality_threshold"`
	NearDupThreshold  float64  `json:"near_dup_threshold"`
}

func (h *PolicyHandler) Update(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	if orgID == "" {
		jsonError(w, http.StatusUnauthorized, "UNAUTHORIZED")
		return
	}
	var req updatePolicyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	// Clamp thresholds to sane ranges; 0 means "use default" on read.
	if req.QualityThreshold < 0 || req.QualityThreshold > 1 {
		jsonError(w, http.StatusBadRequest, "quality_threshold must be between 0 and 1")
		return
	}
	if req.NearDupThreshold < 0 || req.NearDupThreshold > 1 {
		jsonError(w, http.StatusBadRequest, "near_dup_threshold must be between 0 and 1")
		return
	}
	// Normalize "0 means default" so the persisted/returned value matches what
	// the sync engine actually enforces (it treats <=0 as the default). Without
	// this, the dashboard would show 0 while the engine silently uses 0.55.
	if req.QualityThreshold == 0 {
		req.QualityThreshold = store.DefaultQualityThreshold
	}
	if req.NearDupThreshold == 0 {
		req.NearDupThreshold = store.DefaultNearDupThreshold
	}
	p := &model.OrgPolicy{
		OrgID:             orgID,
		BlockedCategories: req.BlockedCategories,
		QualityThreshold:  req.QualityThreshold,
		NearDupThreshold:  req.NearDupThreshold,
	}
	if err := h.store.UpsertOrgPolicy(r.Context(), p); err != nil {
		h.logger.Error("upsert org policy failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "policy update failed")
		return
	}
	jsonResponse(w, http.StatusOK, h.toResponse(p))
}
