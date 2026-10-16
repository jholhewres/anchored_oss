package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/jholhewres/anchored_oss/internal/model"
	anchoredskill "github.com/jholhewres/anchored_oss/internal/skill"
	_ "modernc.org/sqlite"
)

func TestSkillFrontmatterPreservesLegacyAndRendersCanonicalSQLite(t *testing.T) {
	st := newSQLiteTestStore(t)
	ctx := context.Background()
	orgID, accountID := projectTestOrg(t, st)

	legacyContent := "---\r\nname: hand-written\r\n---\r\n# Legacy\r\n"
	legacy := &model.Skill{OrgID: orgID, Name: "Legacy", Slug: "legacy", Purpose: "Legacy bytes", Status: model.SkillStatusActive, CreatedBy: accountID}
	if err := st.CreateSkill(ctx, legacy, legacyContent); err != nil {
		t.Fatal(err)
	}
	if legacy.CurrentVersion.Content != legacyContent || legacy.CurrentVersion.ContentHash != skillHash(legacyContent) || legacy.CurrentVersion.Frontmatter != nil {
		t.Fatalf("legacy version changed: %#v", legacy.CurrentVersion)
	}
	var nullFrontmatter any
	if err := st.db.QueryRowContext(ctx, `SELECT frontmatter FROM skill_versions WHERE skill_id = ? AND version = 1`, legacy.ID).Scan(&nullFrontmatter); err != nil {
		t.Fatal(err)
	}
	if nullFrontmatter != nil {
		t.Fatalf("legacy frontmatter = %#v, want NULL", nullFrontmatter)
	}

	frontmatter := anchoredskill.Header("release", "Prepare releases")
	canonical := &model.Skill{
		OrgID: orgID, Name: "Release", Slug: "release", Purpose: "Prepare releases", Status: model.SkillStatusActive, CreatedBy: accountID,
		CurrentVersion: &model.SkillVersion{Frontmatter: frontmatter, Body: "# Release\r\n\r\nShip safely"},
	}
	if err := st.CreateSkill(ctx, canonical, "ignored legacy alias"); err != nil {
		t.Fatal(err)
	}
	want, err := anchoredskill.Render(canonical.Slug, canonical.Purpose, frontmatter, "# Release\r\n\r\nShip safely")
	if err != nil {
		t.Fatal(err)
	}
	if canonical.CurrentVersion.Content != want || canonical.CurrentVersion.ContentHash != skillHash(want) || canonical.CurrentVersion.Body != "# Release\r\n\r\nShip safely" {
		t.Fatalf("canonical version = %#v\nwant content=%q", canonical.CurrentVersion, want)
	}
	var stored string
	if err := st.db.QueryRowContext(ctx, `SELECT frontmatter FROM skill_versions WHERE skill_id = ? AND version = 1`, canonical.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if !json.Valid([]byte(stored)) {
		t.Fatalf("SQLite frontmatter is not valid JSON: %q", stored)
	}
}

func TestSQLiteMigration024KeepsLegacySkillRowsByteExact(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`
		CREATE TABLE schema_version (version INTEGER PRIMARY KEY);
		CREATE TABLE organizations (id TEXT PRIMARY KEY, name TEXT NOT NULL, slug TEXT NOT NULL);
		CREATE TABLE accounts (id TEXT PRIMARY KEY, email TEXT NOT NULL, display_name TEXT NOT NULL);
		CREATE TABLE api_keys (id TEXT PRIMARY KEY, name TEXT NOT NULL, revoked_at TEXT);
		CREATE TABLE skills (id TEXT PRIMARY KEY, org_id TEXT NOT NULL, name TEXT NOT NULL, slug TEXT NOT NULL, purpose TEXT NOT NULL, status TEXT NOT NULL, current_version INTEGER NOT NULL, created_by TEXT);
		CREATE TABLE skill_versions (skill_id TEXT NOT NULL, version INTEGER NOT NULL, purpose TEXT NOT NULL, body TEXT NOT NULL, content_hash TEXT NOT NULL, created_by TEXT, PRIMARY KEY (skill_id, version));
		INSERT INTO schema_version(version) VALUES (23);
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO organizations(id, name, slug) VALUES ('org', 'Org', 'org'); INSERT INTO accounts(id, email, display_name) VALUES ('actor', 'a@example.test', 'Actor'); INSERT INTO skills(id, org_id, name, slug, purpose, status, current_version, created_by) VALUES ('skill', 'org', 'Legacy', 'legacy', 'legacy bytes', 'active', 1, 'actor'); INSERT INTO skill_versions(skill_id, version, purpose, body, content_hash, created_by) VALUES ('skill', 1, 'legacy bytes', ?, ?, 'actor')`, "# Legacy\r\n", skillHash("# Legacy\r\n")); err != nil {
		t.Fatal(err)
	}
	if err := MigrateSQLite(db); err != nil {
		t.Fatal(err)
	}
	var body, hash string
	var frontmatter any
	if err := db.QueryRow(`SELECT body, content_hash, frontmatter FROM skill_versions WHERE skill_id = 'skill'`).Scan(&body, &hash, &frontmatter); err != nil {
		t.Fatal(err)
	}
	if body != "# Legacy\r\n" || hash != skillHash(body) || frontmatter != nil {
		t.Fatalf("legacy row changed body=%q hash=%q frontmatter=%#v", body, hash, frontmatter)
	}
	if _, err := db.Exec(`UPDATE skill_versions SET frontmatter = 'not-json' WHERE skill_id = 'skill'`); err == nil {
		t.Fatal("SQLite accepted invalid frontmatter JSON")
	}
}

func TestSkillProjectCountReflectsSharedAttachmentsSQLite(t *testing.T) {
	st := newSQLiteTestStore(t)
	ctx := context.Background()
	orgID, accountID := projectTestOrg(t, st)
	first, err := st.CreateProject(ctx, orgID, "First", "first", "first", "", "", accountID, "other")
	if err != nil {
		t.Fatal(err)
	}
	second, err := st.CreateProject(ctx, orgID, "Second", "second", "second", "", "", accountID, "other")
	if err != nil {
		t.Fatal(err)
	}
	skill := &model.Skill{OrgID: orgID, Name: "Shared", Slug: "shared", Purpose: "shared", Status: model.SkillStatusActive, CreatedBy: accountID}
	if err := st.CreateSkill(ctx, skill, "# shared"); err != nil {
		t.Fatal(err)
	}
	for _, project := range []*model.Project{first, second} {
		if _, err := st.ReplaceProjectSkills(ctx, orgID, project.ID, []string{skill.ID}); err != nil {
			t.Fatal(err)
		}
	}
	loaded, err := st.GetSkill(ctx, orgID, skill.ID)
	if err != nil || loaded.ProjectCount != 2 {
		t.Fatalf("project count = %#v, err=%v", loaded, err)
	}
}
