package sync

import (
	"context"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/jholhewres/anchored_oss/internal/middleware"
	"github.com/jholhewres/anchored_oss/internal/model"
	"github.com/jholhewres/anchored_oss/internal/store"
)

// TestSync_ResponseCarriesResolvedProjectID locks the contract that the sync
// response always reports which project the batch landed in — both when the
// client targets an explicit project_id and when it routes by project_claim
// (git-origin remote_key). Clients need this ID for follow-up per-project
// calls such as knowledge-graph triple ingest.
func TestSync_ResponseCarriesResolvedProjectID(t *testing.T) {
	st, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	ctx := context.WithValue(context.Background(), middleware.ScopeKey, "admin")

	org, err := st.CreateOrganization(ctx, "Acme", "acme")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	acc, err := st.CreateAccount(ctx, "dev@acme.test", "Dev", "x")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	if err := st.AddOrgMember(ctx, org.ID, acc.ID, "admin"); err != nil {
		t.Fatalf("add org member: %v", err)
	}
	proj, err := st.CreateProject(ctx, org.ID, "Repo", "repo", "github.com/acme/repo=abc123", acc.ID, "code")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	eng := NewSyncEngine(st, slog.Default())

	// Explicit project_id route.
	resp, err := eng.Sync(ctx, acc.ID, org.ID, &model.SyncRequest{
		ProjectID: proj.ID,
		ClientID:  "test",
	})
	if err != nil {
		t.Fatalf("sync by project_id: %v", err)
	}
	if resp.ProjectID != proj.ID {
		t.Errorf("project_id route: resp.ProjectID = %q, want %q", resp.ProjectID, proj.ID)
	}

	// project_claim (git-origin remote_key) route.
	resp, err = eng.Sync(ctx, acc.ID, org.ID, &model.SyncRequest{
		ClientID:     "test",
		ProjectClaim: &model.ProjectClaim{Name: "Repo", RemoteKey: "github.com/acme/repo=abc123"},
	})
	if err != nil {
		t.Fatalf("sync by project_claim: %v", err)
	}
	if resp.ProjectID != proj.ID {
		t.Errorf("project_claim route: resp.ProjectID = %q, want %q", resp.ProjectID, proj.ID)
	}
}
