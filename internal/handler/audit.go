package handler

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/jholhewres/anchored_oss/internal/middleware"
	"github.com/jholhewres/anchored_oss/internal/model"
	"github.com/jholhewres/anchored_oss/internal/store"
)

type AuditHandler struct {
	store  store.Store
	logger *slog.Logger
}

func NewAuditHandler(st store.Store, logger *slog.Logger) *AuditHandler {
	return &AuditHandler{store: st, logger: logger}
}

type auditResponse struct {
	Entries []*model.AuditEntry `json:"entries"`
	Total   int                 `json:"total"`
	Limit   int                 `json:"limit"`
	Offset  int                 `json:"offset"`
}

func (h *AuditHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID := middleware.GetOrgID(r.Context())
	if orgID == "" {
		jsonError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	q := r.URL.Query()
	filters := model.AuditFilters{
		ProjectID:  q.Get("project"),
		ActorID:    q.Get("actor"),
		Action:     q.Get("action"),
		TargetType: q.Get("target_type"),
	}

	if v := q.Get("from"); v != "" {
		t, err := parseRFC3339(v)
		if err != nil {
			jsonError(w, http.StatusBadRequest, "from must be RFC3339")
			return
		}
		filters.From = &t
	}
	if v := q.Get("to"); v != "" {
		t, err := parseRFC3339(v)
		if err != nil {
			jsonError(w, http.StatusBadRequest, "to must be RFC3339")
			return
		}
		filters.To = &t
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			jsonError(w, http.StatusBadRequest, "limit must be an integer")
			return
		}
		filters.Limit = n
	}
	if v := q.Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			jsonError(w, http.StatusBadRequest, "offset must be an integer")
			return
		}
		filters.Offset = n
	}

	entries, total, err := h.store.ListAuditEntries(r.Context(), orgID, filters)
	if err != nil {
		h.logger.Error("list audit failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "list audit failed")
		return
	}
	if entries == nil {
		entries = []*model.AuditEntry{}
	}

	// Echo the effective limit/offset so the client can drive pagination.
	limit := filters.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	offset := filters.Offset
	if offset < 0 {
		offset = 0
	}

	jsonResponse(w, http.StatusOK, auditResponse{
		Entries: entries, Total: total, Limit: limit, Offset: offset,
	})
}

func parseRFC3339(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339, s)
}
