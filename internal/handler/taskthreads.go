package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jholhewres/anchored_oss/internal/middleware"
	"github.com/jholhewres/anchored_oss/internal/model"
	"github.com/jholhewres/anchored_oss/internal/store"
)

// taskThreadMaxBatch caps one PUT so a runaway client can't bulk-insert
// unbounded rows.
const taskThreadMaxBatch = 100

var taskThreadStatuses = map[string]bool{
	"active": true, "paused": true, "done": true, "cancelled": true,
}

// TaskThreadsHandler serves the personal kanban: the caller's own task
// threads, synced from their client. Threads are PRIVATE — both endpoints
// derive the account from the token and there is deliberately no parameter
// to read another account's threads (and no admin bypass).
type TaskThreadsHandler struct {
	store  store.Store
	logger *slog.Logger
}

func NewTaskThreadsHandler(st store.Store, logger *slog.Logger) *TaskThreadsHandler {
	return &TaskThreadsHandler{store: st, logger: logger}
}

// List handles GET /v1/me/task-threads.
func (h *TaskThreadsHandler) List(w http.ResponseWriter, r *http.Request) {
	accountID := middleware.GetAccountID(r.Context())
	if accountID == "" {
		jsonError(w, http.StatusUnauthorized, "task threads require an account-scoped key")
		return
	}
	threads, err := h.store.ListAccountTaskThreads(r.Context(), accountID)
	if err != nil {
		h.logger.Error("list task threads failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "failed to list task threads")
		return
	}
	if threads == nil {
		threads = []*model.AccountTaskThread{}
	}
	jsonResponse(w, http.StatusOK, map[string]any{"threads": threads})
}

// Put handles PUT /v1/me/task-threads: bulk upsert of the caller's threads.
func (h *TaskThreadsHandler) Put(w http.ResponseWriter, r *http.Request) {
	accountID := middleware.GetAccountID(r.Context())
	if accountID == "" {
		jsonError(w, http.StatusUnauthorized, "task threads require an account-scoped key")
		return
	}

	var body struct {
		Threads []model.AccountTaskThread `json:"threads"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&body); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if len(body.Threads) == 0 {
		jsonError(w, http.StatusBadRequest, "threads is required")
		return
	}
	if len(body.Threads) > taskThreadMaxBatch {
		jsonError(w, http.StatusBadRequest, "too many threads in one request")
		return
	}

	saved := 0
	for i := range body.Threads {
		t := &body.Threads[i]
		t.AccountID = accountID // never trust a client-supplied account
		t.TaskKey = strings.ToUpper(strings.TrimSpace(t.TaskKey))
		if t.TaskKey == "" || len(t.TaskKey) > 64 {
			continue
		}
		if !taskThreadStatuses[t.Status] {
			t.Status = "active"
		}
		// external_ref is rendered as a link in the dashboard: only http(s)
		// survives (javascript:/data: would be a stored XSS vector).
		if t.ExternalRef != "" && !strings.HasPrefix(t.ExternalRef, "http://") && !strings.HasPrefix(t.ExternalRef, "https://") {
			t.ExternalRef = ""
		}
		if len(t.ExternalRef) > 512 {
			t.ExternalRef = t.ExternalRef[:512]
		}
		if len(t.Journal) > 100 {
			t.Journal = t.Journal[:100]
		}
		for j, entry := range t.Journal {
			if len(entry) > 2048 {
				t.Journal[j] = entry[:2048]
			}
		}
		if len(t.Projects) > 50 {
			t.Projects = t.Projects[:50]
		}
		for j, p := range t.Projects {
			if len(p) > 128 {
				t.Projects[j] = p[:128]
			}
		}
		if err := h.store.UpsertAccountTaskThread(r.Context(), t); err != nil {
			h.logger.Error("upsert task thread failed", "task_key", t.TaskKey, "error", err)
			continue
		}
		saved++
	}
	jsonResponse(w, http.StatusOK, map[string]any{"saved": saved})
}
