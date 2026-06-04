package sync

import (
	"context"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/jholhewres/anchored_oss/internal/middleware"
	"github.com/jholhewres/anchored_oss/internal/model"
	projectpkg "github.com/jholhewres/anchored_oss/internal/project"
	"github.com/jholhewres/anchored_oss/internal/store"
)

// TestSync_AcceptsLegacyRemoteKey proves a client that stamps a push with the
// legacy (v1) key still resolves to the project created under the v2 canonical
// key, because GetProjectByRemoteKey matches either column. This is the whole
// point of recording remote_key_v1 during the v2 normalization rollout.
func TestSync_AcceptsLegacyRemoteKey(t *testing.T) {
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

	url := "ssh://git@bitbucket.example.com:7999/proj/repo.git"
	canonical := projectpkg.DeriveRemoteKey(url)
	legacy := projectpkg.DeriveLegacyRemoteKey(url)
	if canonical == legacy {
		t.Fatal("test URL must produce distinct keys")
	}
	proj, err := st.CreateProject(ctx, org.ID, "Repo", "repo", canonical, legacy, url, acc.ID, "service")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	eng := NewSyncEngine(st, slog.Default())

	// A push that claims the LEGACY key must land in the same project.
	resp, err := eng.Sync(ctx, acc.ID, org.ID, &model.SyncRequest{
		ClientID:     "test",
		ProjectClaim: &model.ProjectClaim{Name: "Repo", RemoteKey: legacy},
	})
	if err != nil {
		t.Fatalf("sync by legacy key: %v", err)
	}
	if resp.ProjectID != proj.ID {
		t.Errorf("legacy-key claim resolved to %q, want %q", resp.ProjectID, proj.ID)
	}
}
