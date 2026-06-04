package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jholhewres/anchored_oss/internal/middleware"
	"github.com/jholhewres/anchored_oss/internal/model"
	projectpkg "github.com/jholhewres/anchored_oss/internal/project"
	"github.com/jholhewres/anchored_oss/internal/store"
)

func newProjectTestStore(t *testing.T) *store.SQLiteStore {
	t.Helper()
	st, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "p.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

// adminCtx returns a context carrying admin scope + the given org/account, as
// the auth middleware would populate it.
func adminCtx(orgID, acctID string) context.Context {
	ctx := context.WithValue(context.Background(), middleware.OrgIDKey, orgID)
	ctx = context.WithValue(ctx, middleware.AccountIDKey, acctID)
	ctx = context.WithValue(ctx, middleware.ScopeKey, "admin")
	return ctx
}

func seedProject(t *testing.T, st *store.SQLiteStore) (orgID, acctID, projectID string) {
	t.Helper()
	ctx := context.Background()
	org, err := st.CreateOrganization(ctx, "Acme", "acme")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	acct, err := st.CreateAccount(ctx, "a@acme.test", "A", "h")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	p, err := st.CreateProject(ctx, org.ID, "Repo", "repo", "k", "", "", acct.ID, "other")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	return org.ID, acct.ID, p.ID
}

func TestProjectUpdate_Success(t *testing.T) {
	st := newProjectTestStore(t)
	orgID, acctID, projectID := seedProject(t, st)
	h := NewProjectHandler(st, slog.Default())

	body := `{"name":"Renamed","repo_url":"https://github.com/user/repo.git","category":"service"}`
	req := httptest.NewRequest(http.MethodPatch, "/v1/projects/"+projectID, strings.NewReader(body)).
		WithContext(adminCtx(orgID, acctID))
	req.SetPathValue("id", projectID)
	rec := httptest.NewRecorder()

	h.Update(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	var got model.Project
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Name != "Renamed" {
		t.Errorf("name = %q", got.Name)
	}
	if got.RemoteKey != projectpkg.DeriveRemoteKey("https://github.com/user/repo.git") {
		t.Errorf("remote_key not recomputed: %q", got.RemoteKey)
	}
	if got.RemoteKeyV1 != projectpkg.DeriveLegacyRemoteKey("https://github.com/user/repo.git") {
		t.Errorf("remote_key_v1 not recomputed: %q", got.RemoteKeyV1)
	}
	if got.RepoURL != "https://github.com/user/repo.git" {
		t.Errorf("repo_url = %q", got.RepoURL)
	}
}

func TestProjectUpdate_InvalidSlug(t *testing.T) {
	st := newProjectTestStore(t)
	orgID, acctID, projectID := seedProject(t, st)
	h := NewProjectHandler(st, slog.Default())

	req := httptest.NewRequest(http.MethodPatch, "/v1/projects/"+projectID, strings.NewReader(`{"slug":"Invalid Slug!"}`)).
		WithContext(adminCtx(orgID, acctID))
	req.SetPathValue("id", projectID)
	rec := httptest.NewRecorder()

	h.Update(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
}

func TestProjectUpdate_NotFoundCrossOrg(t *testing.T) {
	st := newProjectTestStore(t)
	_, _, projectID := seedProject(t, st)
	h := NewProjectHandler(st, slog.Default())

	// Different org in context: must 404, not leak the project.
	req := httptest.NewRequest(http.MethodPatch, "/v1/projects/"+projectID, strings.NewReader(`{"name":"X"}`)).
		WithContext(adminCtx("11111111-1111-1111-1111-111111111111", "acct"))
	req.SetPathValue("id", projectID)
	rec := httptest.NewRecorder()

	h.Update(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", rec.Code, rec.Body.String())
	}
}

// TestProjectUpdate_DuplicateSlug renames a project onto a sibling's live slug
// and expects a 409 (mapped from store.ErrConflict), not a 500.
func TestProjectUpdate_DuplicateSlug(t *testing.T) {
	st := newProjectTestStore(t)
	orgID, acctID, _ := seedProject(t, st)
	h := NewProjectHandler(st, slog.Default())

	// "repo" already exists from seedProject; create a second project, then try
	// to rename it onto "repo".
	second, err := st.CreateProject(context.Background(), orgID, "Other", "other", "k2", "", "", acctID, "other")
	if err != nil {
		t.Fatalf("create second: %v", err)
	}

	req := httptest.NewRequest(http.MethodPatch, "/v1/projects/"+second.ID, strings.NewReader(`{"slug":"repo"}`)).
		WithContext(adminCtx(orgID, acctID))
	req.SetPathValue("id", second.ID)
	rec := httptest.NewRecorder()

	h.Update(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body=%s", rec.Code, rec.Body.String())
	}
}

// TestProjectUpdate_ForbiddenNonAdmin exercises the requireAdmin guard the
// route wraps the handler with: a non-admin scope must never reach Update.
func TestProjectUpdate_ForbiddenNonAdmin(t *testing.T) {
	st := newProjectTestStore(t)
	orgID, acctID, projectID := seedProject(t, st)
	h := NewProjectHandler(st, slog.Default())

	guarded := middleware.RequireScope("admin")(http.HandlerFunc(h.Update))

	ctx := context.WithValue(context.Background(), middleware.OrgIDKey, orgID)
	ctx = context.WithValue(ctx, middleware.AccountIDKey, acctID)
	ctx = context.WithValue(ctx, middleware.ScopeKey, "sync") // non-admin
	req := httptest.NewRequest(http.MethodPatch, "/v1/projects/"+projectID, strings.NewReader(`{"name":"X"}`)).
		WithContext(ctx)
	req.SetPathValue("id", projectID)
	rec := httptest.NewRecorder()

	guarded.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}
