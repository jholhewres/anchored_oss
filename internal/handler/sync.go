package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jholhewres/anchored_oss/internal/middleware"
	"github.com/jholhewres/anchored_oss/internal/model"
	"github.com/jholhewres/anchored_oss/internal/store"
	syncpkg "github.com/jholhewres/anchored_oss/internal/sync"
)

type SyncHandler struct {
	engine *syncpkg.SyncEngine
	store  store.Store
	logger *slog.Logger
}

func NewSyncHandler(engine *syncpkg.SyncEngine, st store.Store, logger *slog.Logger) *SyncHandler {
	return &SyncHandler{engine: engine, store: st, logger: logger}
}

// ServeHTTP handles POST /v1/sync — the canonical bidirectional protocol.
func (h *SyncHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req model.SyncRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "INVALID_REQUEST")
		return
	}

	if req.ProjectID == "" && req.ProjectClaim == nil {
		jsonError(w, http.StatusBadRequest, "INVALID_REQUEST")
		return
	}

	scope := middleware.GetScope(r.Context())
	if scope == "readonly" && (len(req.Pushes) > 0 || len(req.Tombstones) > 0) {
		jsonError(w, http.StatusForbidden, "FORBIDDEN")
		return
	}

	accountID := middleware.GetAccountID(r.Context())
	orgID := middleware.GetOrgID(r.Context())

	resp, err := h.engine.Sync(r.Context(), accountID, orgID, &req)
	if err != nil {
		h.writeSyncError(w, err)
		return
	}

	jsonResponse(w, http.StatusOK, resp)
}

// --- Compat adapters for clients that speak the simpler split protocol -----

// compatPushRequest mirrors the client's SyncPushRequest shape.
type compatPushRequest struct {
	ClientID  string              `json:"client_id"`
	ProjectID string              `json:"project_id"`
	Memories  []compatPushMemory  `json:"memories"`
}

type compatPushMemory struct {
	ID               string         `json:"id"`
	Category         string         `json:"category"`
	Content          string         `json:"content"`
	Source           string         `json:"source"`
	PreferenceScope  string         `json:"preference_scope,omitempty"`
	RemoteProjectKey string         `json:"remote_project_key,omitempty"`
	Metadata         map[string]any `json:"metadata,omitempty"`
}

type compatPushResponse struct {
	Accepted int      `json:"accepted"`
	Rejected int      `json:"rejected"`
	Errors   []string `json:"errors,omitempty"`
}

type compatPullRequest struct {
	ClientID  string `json:"client_id"`
	ProjectID string `json:"project_id"`
	Watermark string `json:"watermark,omitempty"`
}

type compatPullResponse struct {
	Memories  []compatPullMemory `json:"memories"`
	Watermark string             `json:"watermark"`
}

type compatPullMemory struct {
	ID         string         `json:"id"`
	Category   string         `json:"category"`
	Content    string         `json:"content"`
	Source     string         `json:"source,omitempty"`
	AuthorName string         `json:"author_name,omitempty"`
	UpdatedAt  time.Time      `json:"updated_at"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// CompatPush adapts POST /api/v1/sync/push to the bidirectional engine.
//
// Routing rules:
//   - When `project_id` is set, all memories go to that project (legacy path).
//   - When `project_id` is empty, memories are grouped by `remote_project_key`
//     and routed via a per-group ProjectClaim so the engine can auto-create
//     the project on the first push. Memories without a remote_project_key
//     are rejected in the response with `routing: no project_id and no
//     remote_project_key` so the client can surface them.
func (h *SyncHandler) CompatPush(w http.ResponseWriter, r *http.Request) {
	var req compatPushRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Warn("compat push decode failed", "error", err)
		jsonError(w, http.StatusBadRequest, "INVALID_REQUEST")
		return
	}
	if middleware.GetScope(r.Context()) == "readonly" {
		jsonError(w, http.StatusForbidden, "FORBIDDEN")
		return
	}
	if req.ProjectID == "" && len(req.Memories) == 0 {
		jsonError(w, http.StatusBadRequest, "project_id or memories with remote_project_key required")
		return
	}

	accountID := middleware.GetAccountID(r.Context())
	orgID := middleware.GetOrgID(r.Context())

	authorName := "" // fall back to empty; engine treats empty as anonymous
	if accountID != "" {
		if acc, err := h.store.GetAccountByID(r.Context(), accountID); err == nil && acc != nil {
			authorName = acc.DisplayName
		} else if err != nil && !errors.Is(err, store.ErrNotFound) {
			h.logger.Warn("compat push: account lookup failed", "error", err)
		}
	}

	out := compatPushResponse{}

	// Group memories so each batch lands in exactly one project.
	// When ProjectID is set we keep the legacy single-group behaviour; otherwise
	// we partition by RemoteProjectKey so the engine can claim/auto-create
	// projects per repository.
	type pushGroup struct {
		projectID string
		claimKey  string
		memories  []compatPushMemory
	}
	groups := make(map[string]*pushGroup)
	unroutable := make([]string, 0)

	for _, m := range req.Memories {
		if m.PreferenceScope == "user" {
			continue
		}
		var key string
		switch {
		case req.ProjectID != "":
			key = "id:" + req.ProjectID
		case m.RemoteProjectKey != "":
			key = "key:" + m.RemoteProjectKey
		default:
			unroutable = append(unroutable, m.ID)
			continue
		}
		g, ok := groups[key]
		if !ok {
			g = &pushGroup{}
			if req.ProjectID != "" {
				g.projectID = req.ProjectID
			} else {
				g.claimKey = m.RemoteProjectKey
			}
			groups[key] = g
		}
		g.memories = append(g.memories, m)
	}

	for _, id := range unroutable {
		out.Rejected++
		out.Errors = append(out.Errors, "memory "+id+" blocked: routing: no project_id and no remote_project_key")
	}

	now := time.Now().UTC()
	for _, g := range groups {
		pushes := make([]model.SyncMemory, 0, len(g.memories))
		for _, m := range g.memories {
			hash := "sha256:" + sha256Hex(m.Content)
			pushes = append(pushes, model.SyncMemory{
				ID:          m.ID,
				Category:    m.Category,
				Content:     m.Content,
				ContentHash: hash,
				Source:      m.Source,
				AuthorName:  authorName,
				CreatedAt:   now,
				UpdatedAt:   now,
				Metadata:    m.Metadata,
			})
		}

		sr := &model.SyncRequest{ClientID: req.ClientID, Pushes: pushes}
		if g.projectID != "" {
			sr.ProjectID = g.projectID
		} else {
			sr.ProjectClaim = &model.ProjectClaim{
				RemoteKey: g.claimKey,
				Name:      autoProjectName(g.claimKey),
			}
		}

		resp, err := h.engine.Sync(r.Context(), accountID, orgID, sr)
		if err != nil {
			// Record the failure for this group but keep processing the
			// remaining groups so a single bad project doesn't abort the
			// entire batch.
			detail := err.Error()
			for _, m := range g.memories {
				out.Rejected++
				out.Errors = append(out.Errors, "memory "+m.ID+" blocked: "+detail)
			}
			h.logger.Warn("compat push: group sync failed", "claim_key", g.claimKey, "project_id", g.projectID, "error", err)
			continue
		}

		for _, rr := range resp.Results {
			switch rr.Status {
			case "accepted":
				out.Accepted++
			default:
				out.Rejected++
				detail := rr.Detail
				if detail == "" {
					detail = rr.Rule
				}
				out.Errors = append(out.Errors, "memory "+rr.ID+" blocked: "+detail)
			}
		}
	}

	jsonResponse(w, http.StatusOK, out)
}

// autoProjectName derives a stable, human-readable project name from a
// remote_project_key (typically SHA-256 of the git remote URL). The client
// doesn't send a friendly name on the legacy push endpoint; this keeps the
// dashboard usable until an admin renames the project.
func autoProjectName(key string) string {
	short := key
	if len(short) > 12 {
		short = short[:12]
	}
	return "auto-" + short
}

// CompatPull adapts POST /api/v1/sync/pull to the bidirectional engine.
func (h *SyncHandler) CompatPull(w http.ResponseWriter, r *http.Request) {
	var req compatPullRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "INVALID_REQUEST")
		return
	}
	if req.ProjectID == "" {
		jsonError(w, http.StatusBadRequest, "project_id is required")
		return
	}

	accountID := middleware.GetAccountID(r.Context())
	orgID := middleware.GetOrgID(r.Context())

	sr := &model.SyncRequest{
		ProjectID: req.ProjectID,
		ClientID:  req.ClientID,
	}
	// Default watermark: epoch when client omits it, so the first pull
	// returns everything in the project.
	wm := time.Time{}
	if strings.TrimSpace(req.Watermark) != "" {
		parsed, err := time.Parse(time.RFC3339Nano, req.Watermark)
		if err != nil {
			parsed, err = time.Parse(time.RFC3339, req.Watermark)
			if err != nil {
				jsonError(w, http.StatusBadRequest, "INVALID_REQUEST: watermark must be RFC3339")
				return
			}
		}
		wm = parsed
	}
	sr.Watermark = &wm

	resp, err := h.engine.Sync(r.Context(), accountID, orgID, sr)
	if err != nil {
		h.writeSyncError(w, err)
		return
	}

	out := compatPullResponse{
		Memories:  make([]compatPullMemory, 0, len(resp.Pulls)),
		Watermark: resp.Watermark.Format(time.RFC3339Nano),
	}
	for _, m := range resp.Pulls {
		var meta map[string]any
		if m.Metadata != nil {
			if raw, ok := m.Metadata.(map[string]any); ok {
				meta = raw
			}
		}
		out.Memories = append(out.Memories, compatPullMemory{
			ID:         m.ID,
			Category:   m.Category,
			Content:    m.Content,
			Source:     m.Source,
			AuthorName: m.AuthorName,
			UpdatedAt:  m.UpdatedAt,
			Metadata:   meta,
		})
	}
	jsonResponse(w, http.StatusOK, out)
}

func (h *SyncHandler) writeSyncError(w http.ResponseWriter, err error) {
	var se *syncpkg.SyncError
	if errors.As(err, &se) {
		jsonError(w, se.Status, se.Code)
		return
	}
	h.logger.Error("sync failed", "error", err)
	jsonError(w, http.StatusInternalServerError, "INTERNAL_ERROR")
}

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
