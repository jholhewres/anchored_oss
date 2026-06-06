package handler

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jholhewres/anchored_oss/internal/model"
	"github.com/jholhewres/anchored_oss/internal/store"
)

func auditCount(t *testing.T, st *store.SQLiteStore, orgID, action string) int {
	t.Helper()
	_, total, err := st.ListAuditEntries(context.Background(), orgID, model.AuditFilters{Action: action})
	if err != nil {
		t.Fatalf("list audit %s: %v", action, err)
	}
	return total
}

func TestProjectCreate_AppendsAudit(t *testing.T) {
	st := newProjectTestStore(t)
	orgID, acctID, _ := seedProject(t, st)
	h := NewProjectHandler(st, slog.Default())

	body := `{"name":"Svc","slug":"svc","repo_url":"https://github.example.com/org/svc.git"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/projects", strings.NewReader(body)).
		WithContext(adminCtx(orgID, acctID))
	rec := httptest.NewRecorder()
	h.Create(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if n := auditCount(t, st, orgID, "project.created"); n != 1 {
		t.Fatalf("project.created audit entries = %d, want 1", n)
	}
}

func TestProjectSoftDelete_AppendsAudit(t *testing.T) {
	st := newProjectTestStore(t)
	orgID, acctID, projectID := seedProject(t, st)
	h := NewProjectHandler(st, slog.Default())

	req := httptest.NewRequest(http.MethodDelete, "/v1/projects/"+projectID, nil).
		WithContext(adminCtx(orgID, acctID))
	req.SetPathValue("id", projectID)
	rec := httptest.NewRecorder()
	h.SoftDelete(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if n := auditCount(t, st, orgID, "project.deleted"); n != 1 {
		t.Fatalf("project.deleted audit entries = %d, want 1", n)
	}
}

func TestUpdateApply_AppendsServerUpdatedAudit(t *testing.T) {
	st := newProjectTestStore(t)
	orgID, acctID, _ := seedProject(t, st)

	h := newUpdateHandler(&fakeUpdater{available: true})
	h.store = st

	req := httptest.NewRequest(http.MethodPost, "/v1/admin/update/apply", nil).
		WithContext(adminCtx(orgID, acctID))
	rec := httptest.NewRecorder()
	h.Apply(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("apply status = %d, body=%s", rec.Code, rec.Body.String())
	}
	entries, _, err := st.ListAuditEntries(context.Background(), orgID, model.AuditFilters{Action: "server.updated"})
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("server.updated audit entries = %d, want 1", len(entries))
	}
	meta, ok := entries[0].Metadata.(map[string]any)
	if !ok {
		t.Fatalf("metadata type %T", entries[0].Metadata)
	}
	if meta["from_version"] != "v0.4.6" || meta["to_version"] != "v0.5.0" {
		t.Fatalf("metadata versions: %+v", meta)
	}
}
