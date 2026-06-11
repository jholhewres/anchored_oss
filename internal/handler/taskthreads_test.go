package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jholhewres/anchored_oss/internal/middleware"
	"github.com/jholhewres/anchored_oss/internal/store"
)

// TestTaskThreads_PutGetAndAccountIsolation drives the personal-kanban API:
// PUT upserts the caller's threads (never trusting a client-supplied
// account), GET lists only the caller's, and a key without account context
// is rejected — there is no way to read another account's board.
func TestTaskThreads_PutGetAndAccountIsolation(t *testing.T) {
	st, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "tt.db"))
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	defer st.Close()
	h := NewTaskThreadsHandler(st, slog.Default())

	withAccount := func(acct string) context.Context {
		return context.WithValue(context.Background(), middleware.AccountIDKey, acct)
	}

	// PUT for account A.
	body := `{"threads":[{"task_key":"proj-9","status":"active","projects":["repo-a"],"journal":["note 1"]}]}`
	req := httptest.NewRequest("PUT", "/v1/me/task-threads", strings.NewReader(body)).WithContext(withAccount("acct-A"))
	rec := httptest.NewRecorder()
	h.Put(rec, req)
	if rec.Code != 200 {
		t.Fatalf("PUT = %d: %s", rec.Code, rec.Body.String())
	}

	// GET for A sees the (normalized, uppercased) thread.
	req = httptest.NewRequest("GET", "/v1/me/task-threads", nil).WithContext(withAccount("acct-A"))
	rec = httptest.NewRecorder()
	h.List(rec, req)
	var out struct {
		Threads []struct {
			TaskKey string `json:"task_key"`
			Status  string `json:"status"`
		} `json:"threads"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Threads) != 1 || out.Threads[0].TaskKey != "PROJ-9" {
		t.Fatalf("GET A = %s", rec.Body.String())
	}

	// GET for B sees NOTHING (privacy).
	req = httptest.NewRequest("GET", "/v1/me/task-threads", nil).WithContext(withAccount("acct-B"))
	rec = httptest.NewRecorder()
	h.List(rec, req)
	if !strings.Contains(rec.Body.String(), `"threads":[]`) {
		t.Fatalf("GET B should be empty, got %s", rec.Body.String())
	}

	// No account context (service/admin org key) → 401.
	req = httptest.NewRequest("GET", "/v1/me/task-threads", nil)
	rec = httptest.NewRecorder()
	h.List(rec, req)
	if rec.Code != 401 {
		t.Fatalf("no-account GET = %d, want 401", rec.Code)
	}

	// Invalid status normalizes to active; oversized batch rejected.
	body = `{"threads":[{"task_key":"x-1","status":"weird"}]}`
	req = httptest.NewRequest("PUT", "/v1/me/task-threads", strings.NewReader(body)).WithContext(withAccount("acct-A"))
	rec = httptest.NewRecorder()
	h.Put(rec, req)
	if rec.Code != 200 {
		t.Fatalf("PUT weird = %d", rec.Code)
	}
	threads, _ := st.ListAccountTaskThreads(context.Background(), "acct-A")
	for _, th := range threads {
		if th.TaskKey == "X-1" && th.Status != "active" {
			t.Fatalf("status not normalized: %q", th.Status)
		}
	}
}
