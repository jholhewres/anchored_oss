package store

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jholhewres/anchored_oss/internal/model"
	projectpkg "github.com/jholhewres/anchored_oss/internal/project"
)

// projectTestOrg creates an org + account and returns their IDs.
func projectTestOrg(t *testing.T, st *SQLiteStore) (orgID, acctID string) {
	t.Helper()
	ctx := context.Background()
	org, err := st.CreateOrganization(ctx, "Acme", "acme")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	acct, err := st.CreateAccount(ctx, "u@acme.test", "U", "hash")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	return org.ID, acct.ID
}

// TestProject_DualKeyLookup proves a project created with both a canonical and
// a legacy key resolves when queried by EITHER key — the core of the v2
// migration's backward compatibility.
func TestProject_DualKeyLookup(t *testing.T) {
	st := newSQLiteTestStore(t)
	ctx := context.Background()
	orgID, acctID := projectTestOrg(t, st)

	url := "ssh://git@bitbucket.example.com:7999/proj/repo.git"
	canonical := projectpkg.DeriveRemoteKey(url)
	legacy := projectpkg.DeriveLegacyRemoteKey(url)
	if canonical == legacy {
		t.Fatal("test URL must produce distinct canonical and legacy keys")
	}

	created, err := st.CreateProject(ctx, orgID, "Repo", "repo", canonical, legacy, url, acctID, "service")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if created.RemoteKeyV1 != legacy || created.RepoURL != url {
		t.Fatalf("created project did not persist v1 key/url: %+v", created)
	}

	byCanonical, err := st.GetProjectByRemoteKey(ctx, orgID, canonical)
	if err != nil {
		t.Fatalf("lookup by canonical: %v", err)
	}
	byLegacy, err := st.GetProjectByRemoteKey(ctx, orgID, legacy)
	if err != nil {
		t.Fatalf("lookup by legacy: %v", err)
	}
	if byCanonical.ID != created.ID || byLegacy.ID != created.ID {
		t.Errorf("both lookups must resolve to %s, got %s / %s", created.ID, byCanonical.ID, byLegacy.ID)
	}
}

// TestUpdateProject_RecomputesKeys verifies a repo_url change recomputes both
// keys and that clearing it unlinks the repo.
func TestUpdateProject_RecomputesKeys(t *testing.T) {
	st := newSQLiteTestStore(t)
	ctx := context.Background()
	orgID, acctID := projectTestOrg(t, st)

	p, err := st.CreateProject(ctx, orgID, "Repo", "repo", "manual-key", "", "", acctID, "other")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	newURL := "https://github.com/user/repo.git"
	updated, err := st.UpdateProject(ctx, orgID, p.ID, model.ProjectUpdate{RepoURL: strPtr(newURL)})
	if err != nil {
		t.Fatalf("update with repo_url: %v", err)
	}
	if updated.RemoteKey != projectpkg.DeriveRemoteKey(newURL) {
		t.Errorf("remote_key not recomputed: %q", updated.RemoteKey)
	}
	if updated.RemoteKeyV1 != projectpkg.DeriveLegacyRemoteKey(newURL) {
		t.Errorf("remote_key_v1 not recomputed: %q", updated.RemoteKeyV1)
	}
	if updated.RepoURL != newURL {
		t.Errorf("repo_url not stored: %q", updated.RepoURL)
	}

	// Name-only update leaves keys untouched.
	renamed, err := st.UpdateProject(ctx, orgID, p.ID, model.ProjectUpdate{Name: strPtr("Renamed")})
	if err != nil {
		t.Fatalf("rename update: %v", err)
	}
	if renamed.Name != "Renamed" || renamed.RemoteKey != updated.RemoteKey {
		t.Errorf("name-only update should not touch keys: %+v", renamed)
	}

	// Clearing repo_url unlinks the repo: repo_url and the legacy key go empty,
	// and the canonical key is parked on a per-row "norepo-" sentinel so it can
	// never resolve to a real repo while staying unique within the org.
	cleared, err := st.UpdateProject(ctx, orgID, p.ID, model.ProjectUpdate{RepoURL: strPtr("")})
	if err != nil {
		t.Fatalf("clear repo_url: %v", err)
	}
	if cleared.RepoURL != "" || cleared.RemoteKeyV1 != "" {
		t.Errorf("clearing repo_url should empty url + legacy key: %+v", cleared)
	}
	if cleared.RemoteKey != "norepo-"+p.ID {
		t.Errorf("cleared remote_key = %q, want sentinel norepo-%s", cleared.RemoteKey, p.ID)
	}
}

// TestUpdateProject_ClearRepoURL_TwoInSameOrg proves clearing repo_url on two
// projects in the same org both succeed: the per-row "norepo-" sentinel keeps
// UNIQUE(org_id, remote_key) collision-free.
func TestUpdateProject_ClearRepoURL_TwoInSameOrg(t *testing.T) {
	st := newSQLiteTestStore(t)
	ctx := context.Background()
	orgID, acctID := projectTestOrg(t, st)

	a, err := st.CreateProject(ctx, orgID, "A", "a", projectpkg.DeriveRemoteKey("https://github.com/u/a.git"), "", "https://github.com/u/a.git", acctID, "service")
	if err != nil {
		t.Fatalf("create a: %v", err)
	}
	b, err := st.CreateProject(ctx, orgID, "B", "b", projectpkg.DeriveRemoteKey("https://github.com/u/b.git"), "", "https://github.com/u/b.git", acctID, "service")
	if err != nil {
		t.Fatalf("create b: %v", err)
	}

	if _, err := st.UpdateProject(ctx, orgID, a.ID, model.ProjectUpdate{RepoURL: strPtr("")}); err != nil {
		t.Fatalf("clear repo_url on a: %v", err)
	}
	if _, err := st.UpdateProject(ctx, orgID, b.ID, model.ProjectUpdate{RepoURL: strPtr("")}); err != nil {
		t.Fatalf("clear repo_url on b must also succeed: %v", err)
	}
}

// TestSoftDelete_TwoInSameOrg proves soft-deleting two projects in the same org
// both succeed: the per-row "deleted-" sentinel keeps UNIQUE(org_id,
// remote_key) collision-free even though both rows are deleted.
func TestSoftDelete_TwoInSameOrg(t *testing.T) {
	st := newSQLiteTestStore(t)
	ctx := context.Background()
	orgID, acctID := projectTestOrg(t, st)

	a, err := st.CreateProject(ctx, orgID, "A", "a", "key-a", "", "", acctID, "other")
	if err != nil {
		t.Fatalf("create a: %v", err)
	}
	b, err := st.CreateProject(ctx, orgID, "B", "b", "key-b", "", "", acctID, "other")
	if err != nil {
		t.Fatalf("create b: %v", err)
	}

	if err := st.SoftDeleteProject(ctx, a.ID); err != nil {
		t.Fatalf("soft delete a: %v", err)
	}
	if err := st.SoftDeleteProject(ctx, b.ID); err != nil {
		t.Fatalf("soft delete b must also succeed: %v", err)
	}
}

// TestUpdateProject_DuplicateSlug proves renaming a project onto a live sibling's
// slug surfaces ErrConflict rather than an opaque DB error.
func TestUpdateProject_DuplicateSlug(t *testing.T) {
	st := newSQLiteTestStore(t)
	ctx := context.Background()
	orgID, acctID := projectTestOrg(t, st)

	if _, err := st.CreateProject(ctx, orgID, "A", "taken", "key-a", "", "", acctID, "other"); err != nil {
		t.Fatalf("create a: %v", err)
	}
	b, err := st.CreateProject(ctx, orgID, "B", "free", "key-b", "", "", acctID, "other")
	if err != nil {
		t.Fatalf("create b: %v", err)
	}

	_, err = st.UpdateProject(ctx, orgID, b.ID, model.ProjectUpdate{Slug: strPtr("taken")})
	if err != ErrConflict {
		t.Fatalf("rename onto live slug = %v, want ErrConflict", err)
	}
}

// TestUpdateProject_OrgScoped ensures cross-org updates 404 and don't leak.
func TestUpdateProject_OrgScoped(t *testing.T) {
	st := newSQLiteTestStore(t)
	ctx := context.Background()
	orgID, acctID := projectTestOrg(t, st)
	other, err := st.CreateOrganization(ctx, "Other", "other")
	if err != nil {
		t.Fatalf("create other org: %v", err)
	}

	p, err := st.CreateProject(ctx, orgID, "Repo", "repo", "k", "", "", acctID, "other")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := st.UpdateProject(ctx, other.ID, p.ID, model.ProjectUpdate{Name: strPtr("X")}); err != ErrNotFound {
		t.Errorf("cross-org update = %v, want ErrNotFound", err)
	}
}

// TestSoftDelete_FreesSlugForReuse proves the mangling lets a new project take
// the deleted project's slug immediately (UNIQUE(org_id, slug) is freed).
func TestSoftDelete_FreesSlugForReuse(t *testing.T) {
	st := newSQLiteTestStore(t)
	ctx := context.Background()
	orgID, acctID := projectTestOrg(t, st)

	first, err := st.CreateProject(ctx, orgID, "Repo", "repo", "key-a", "", "", acctID, "other")
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	if err := st.SoftDeleteProject(ctx, first.ID); err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	// Same slug + same remote_key must now be free.
	second, err := st.CreateProject(ctx, orgID, "Repo", "repo", "key-a", "", "", acctID, "other")
	if err != nil {
		t.Fatalf("recreate with same slug/key must succeed after soft delete: %v", err)
	}
	if second.ID == first.ID {
		t.Error("expected a new project id")
	}

	// The deleted project is no longer resolvable by its old remote_key.
	if _, err := st.GetProjectByRemoteKey(ctx, orgID, "key-a"); err != nil {
		// Resolves to the NEW project (key-a), which is expected; only assert it
		// is the second one.
		t.Fatalf("lookup after recreate: %v", err)
	}
	got, _ := st.GetProjectByRemoteKey(ctx, orgID, "key-a")
	if got.ID != second.ID {
		t.Errorf("remote_key should resolve to the live project, got %s want %s", got.ID, second.ID)
	}
}

func strPtr(s string) *string { return &s }

// TestMigration015_FreesLegacyDeletedIdentity reproduces the pre-v0.4.7 state
// where soft-delete left slug/remote_key intact on the dead row (blocking any
// recreate with the same identity) and proves migration 015 retroactively
// parks those values so creation succeeds again.
func TestMigration015_FreesLegacyDeletedIdentity(t *testing.T) {
	st := newSQLiteTestStore(t)
	ctx := context.Background()
	orgID, acctID := projectTestOrg(t, st)

	url := "git@github.com:example/repo.git"
	key := projectpkg.DeriveRemoteKey(url)
	legacy := projectpkg.DeriveLegacyRemoteKey(url)
	created, err := st.CreateProject(ctx, orgID, "Repo", "repo", key, legacy, url, acctID, "service")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if err := st.SoftDeleteProject(ctx, created.ID); err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	// Rewind to the legacy on-disk shape: original slug/key on the deleted row,
	// and schema_version back to 14 so the runner re-applies 015.
	if _, err := st.db.Exec(
		`UPDATE projects SET slug = ?, remote_key = ?, remote_key_v1 = ?, repo_url = ? WHERE id = ?`,
		"repo", key, legacy, url, created.ID,
	); err != nil {
		t.Fatalf("rewind row: %v", err)
	}
	if _, err := st.db.Exec(`DELETE FROM schema_version WHERE version >= 15`); err != nil {
		t.Fatalf("rewind schema_version: %v", err)
	}

	if err := MigrateSQLite(st.db); err != nil {
		t.Fatalf("re-run migrations: %v", err)
	}

	var slug, rk string
	if err := st.db.QueryRow(`SELECT slug, remote_key FROM projects WHERE id = ?`, created.ID).Scan(&slug, &rk); err != nil {
		t.Fatalf("read parked row: %v", err)
	}
	if slug != mangleDeletedSlug("repo", created.ID) || rk != deletedRemoteKey(created.ID) {
		t.Fatalf("row not parked: slug=%q remote_key=%q", slug, rk)
	}

	// The original identity is free again.
	if _, err := st.CreateProject(ctx, orgID, "Repo", "repo", key, legacy, url, acctID, "service"); err != nil {
		t.Fatalf("recreate with freed identity: %v", err)
	}
}

// TestSoftDeleteMemoriesByWindow proves the admin bulk-undo only tombstones
// memories created inside the window and leaves the rest of the project alone.
func TestSoftDeleteMemoriesByWindow(t *testing.T) {
	st := newSQLiteTestStore(t)
	ctx := context.Background()
	orgID, acctID := projectTestOrg(t, st)
	proj, err := st.CreateProject(ctx, orgID, "Repo", "repo", "k1", "", "", acctID, "service")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	for i, created := range []string{"2026-06-04 10:00:00", "2026-06-04 16:57:38", "2026-06-04 16:58:02"} {
		m := &model.Memory{
			ID: fmt.Sprintf("m%d", i), ProjectID: proj.ID, Category: "fact",
			Content: fmt.Sprintf("memory %d", i), ContentHash: fmt.Sprintf("h%d", i),
			CreatedAt: time.Now(), UpdatedAt: time.Now(),
		}
		if err := st.UpsertMemory(ctx, m); err != nil {
			t.Fatalf("upsert %d: %v", i, err)
		}
		if _, err := st.db.Exec(`UPDATE memories SET created_at = ? WHERE id = ?`, created, m.ID); err != nil {
			t.Fatalf("set created_at %d: %v", i, err)
		}
	}

	since := time.Date(2026, 6, 4, 16, 55, 0, 0, time.UTC)
	deleted, err := st.SoftDeleteMemoriesByWindow(ctx, proj.ID, &since, nil)
	if err != nil {
		t.Fatalf("window delete: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("want 2 deleted, got %d", deleted)
	}
	var live int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM memories WHERE project_id = ? AND deleted_at IS NULL`, proj.ID).Scan(&live); err != nil {
		t.Fatalf("count: %v", err)
	}
	if live != 1 {
		t.Fatalf("want 1 live memory, got %d", live)
	}
}

// TestBatchUpsertRehomesProject locks the recreate-project recovery path: a
// batched re-push of an existing memory ID under a NEW project must move the
// row (the single-row upsert always did; the batched variant used to drop
// project_id from its conflict set, stranding memories in the old project).
func TestBatchUpsertRehomesProject(t *testing.T) {
	st := newSQLiteTestStore(t)
	ctx := context.Background()
	orgID, acctID := projectTestOrg(t, st)
	a, err := st.CreateProject(ctx, orgID, "A", "proj-a", "ka", "", "", acctID, "service")
	if err != nil {
		t.Fatalf("create A: %v", err)
	}
	b, err := st.CreateProject(ctx, orgID, "B", "proj-b", "kb", "", "", acctID, "service")
	if err != nil {
		t.Fatalf("create B: %v", err)
	}

	mk := func(projectID string, ts time.Time) *model.Memory {
		return &model.Memory{
			ID: "rehome-1", ProjectID: projectID, Category: "fact",
			Content: "same content", ContentHash: "h-rehome",
			CreatedAt: ts, UpdatedAt: ts,
		}
	}
	t0 := time.Now().UTC().Add(-time.Hour)
	if err := st.UpsertMemories(ctx, []*model.Memory{mk(a.ID, t0)}); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	if err := st.UpsertMemories(ctx, []*model.Memory{mk(b.ID, time.Now().UTC())}); err != nil {
		t.Fatalf("re-push upsert: %v", err)
	}

	var got string
	if err := st.db.QueryRow(`SELECT project_id FROM memories WHERE id = 'rehome-1'`).Scan(&got); err != nil {
		t.Fatalf("read row: %v", err)
	}
	if got != b.ID {
		t.Fatalf("memory not rehomed: project_id=%s want %s", got, b.ID)
	}
}
