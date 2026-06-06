package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jholhewres/anchored_oss/internal/middleware"
	"github.com/jholhewres/anchored_oss/internal/model"
)

// memberCtx returns a context for a non-admin key (sync/readonly scope).
func memberCtx(orgID, acctID, scope string) context.Context {
	ctx := context.WithValue(context.Background(), middleware.OrgIDKey, orgID)
	ctx = context.WithValue(ctx, middleware.AccountIDKey, acctID)
	ctx = context.WithValue(ctx, middleware.ScopeKey, scope)
	return ctx
}

func TestMemoryHealth_ProjectAccessMatrix(t *testing.T) {
	st := newProjectTestStore(t)
	orgID, acctID, projectID := seedProject(t, st)
	h := NewMemoryHealthHandler(st, slog.Default())

	get := func(ctx context.Context) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/v1/projects/"+projectID+"/memory-health", nil).
			WithContext(ctx)
		req.SetPathValue("id", projectID)
		rec := httptest.NewRecorder()
		h.Project(rec, req)
		return rec
	}

	// Admin always passes.
	if rec := get(adminCtx(orgID, acctID)); rec.Code != http.StatusOK {
		t.Fatalf("admin status = %d, body=%s", rec.Code, rec.Body.String())
	}

	// Member without team access is forbidden.
	ctx := context.Background()
	outsider, err := st.CreateAccount(ctx, "out@acme.test", "Out", "h")
	if err != nil {
		t.Fatalf("create outsider: %v", err)
	}
	if rec := get(memberCtx(orgID, outsider.ID, "readonly")); rec.Code != http.StatusForbidden {
		t.Fatalf("outsider status = %d, want 403", rec.Code)
	}

	// Member with project access (creator) passes.
	if err := st.EnsureCreatorProjectAccess(ctx, orgID, acctID, projectID); err != nil {
		t.Fatalf("grant access: %v", err)
	}
	if rec := get(memberCtx(orgID, acctID, "readonly")); rec.Code != http.StatusOK {
		t.Fatalf("member status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestMemoryHealth_ResponseShape(t *testing.T) {
	st := newProjectTestStore(t)
	orgID, acctID, projectID := seedProject(t, st)
	h := NewMemoryHealthHandler(st, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/v1/projects/"+projectID+"/memory-health", nil).
		WithContext(adminCtx(orgID, acctID))
	req.SetPathValue("id", projectID)
	rec := httptest.NewRecorder()
	h.Project(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var health model.MemoryHealth
	if err := json.Unmarshal(rec.Body.Bytes(), &health); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// Empty project: perfect score, zeroed counts, contract fields present.
	if health.Score != 1.0 || health.Counts.Live != 0 {
		t.Fatalf("empty project health: %+v", health)
	}
	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode raw: %v", err)
	}
	for _, key := range []string{"score", "counts", "by_source", "by_category",
		"age_histogram", "sync_rejections", "anomalies", "recommendations"} {
		if _, ok := raw[key]; !ok {
			t.Errorf("response missing contract key %q", key)
		}
	}
	counts, _ := raw["counts"].(map[string]any)
	for _, key := range []string{"live", "low_signal", "near_duplicate", "stale",
		"contradictions", "missing_embeddings"} {
		if _, ok := counts[key]; !ok {
			t.Errorf("counts missing contract key %q", key)
		}
	}
}

func TestMemoryHealth_OrgEndpoint(t *testing.T) {
	st := newProjectTestStore(t)
	orgID, acctID, _ := seedProject(t, st)
	h := NewMemoryHealthHandler(st, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/v1/orgs/memory-health", nil).
		WithContext(adminCtx(orgID, acctID))
	rec := httptest.NewRecorder()
	h.Org(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("org health status = %d, body=%s", rec.Code, rec.Body.String())
	}
	// Note: admin-only enforcement for the org endpoint lives in the route
	// wiring (requireAdmin middleware), exercised by the server route table.
}
