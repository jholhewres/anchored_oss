//go:build integration

package store

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jholhewres/anchored_oss/internal/model"
	anchoredskill "github.com/jholhewres/anchored_oss/internal/skill"
)

func TestSkills_PostgresCRUDVersioningAndAttachments(t *testing.T) {
	dsn := os.Getenv("ANCHORED_TEST_DSN")
	if dsn == "" {
		t.Skip("ANCHORED_TEST_DSN not set; skipping Postgres skills integration test")
	}
	st, err := NewPostgresStore(PoolConfig{DSN: dsn})
	if err != nil {
		t.Fatalf("open postgres store: %v", err)
	}
	defer st.Close()

	ctx := context.Background()
	suffix := uniq()
	org, err := st.CreateOrganization(ctx, "Skills "+suffix, "skills-"+suffix)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = st.db.ExecContext(ctx, `DELETE FROM organizations WHERE id = $1`, org.ID) }()
	account, err := st.CreateAccount(ctx, "skills-"+suffix+"@example.test", "Skills", "hash")
	if err != nil {
		t.Fatal(err)
	}
	project, err := st.CreateProject(ctx, org.ID, "Skills", "skills-"+suffix, "skills-"+suffix, "", "", account.ID, "other")
	if err != nil {
		t.Fatal(err)
	}
	skill := &model.Skill{OrgID: org.ID, Name: "Release", Slug: "release-" + suffix, Purpose: "prepare", Status: model.SkillStatusActive, CreatedBy: account.ID,
		CurrentVersion: &model.SkillVersion{Frontmatter: anchoredskill.Header("release-"+suffix, "prepare"), Body: "# one"}}
	if err := st.CreateSkill(ctx, skill, "ignored legacy alias"); err != nil {
		t.Fatal(err)
	}
	content, purpose := "# two", "prepare safely"
	updated, err := st.UpdateSkill(ctx, org.ID, skill.ID, account.ID, model.SkillUpdate{Body: &content, Purpose: &purpose})
	want, renderErr := anchoredskill.Render(skill.Slug, purpose, anchoredskill.Header(skill.Slug, purpose), content)
	if renderErr != nil || err != nil || updated.CurrentVersion.Version != 2 || updated.CurrentVersion.Content != want || updated.CurrentVersion.ContentHash != skillHash(want) {
		t.Fatalf("publish = %#v, err=%v", updated, err)
	}
	var frontmatterType string
	if err := st.db.QueryRowContext(ctx, `SELECT pg_typeof(frontmatter)::text FROM skill_versions WHERE skill_id = $1 AND version = 2`, skill.ID).Scan(&frontmatterType); err != nil || frontmatterType != "jsonb" {
		t.Fatalf("frontmatter type = %q, err=%v", frontmatterType, err)
	}
	oldIDs, err := st.ReplaceProjectSkills(ctx, org.ID, project.ID, []string{skill.ID})
	if err != nil || len(oldIDs) != 0 {
		t.Fatalf("attach = old=%v err=%v", oldIDs, err)
	}
	runtime, err := st.ListProjectSkills(ctx, project.ID, "safe", 20)
	if err != nil || len(runtime) != 1 || runtime[0].Version != 2 {
		t.Fatalf("runtime = %#v, err=%v", runtime, err)
	}
	if err := st.RevokeSkill(ctx, org.ID, skill.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetProjectSkill(ctx, project.ID, skill.Slug); err != ErrNotFound {
		t.Fatalf("runtime load after revoke = %v, want ErrNotFound", err)
	}
	attachments, err := st.ListProjectSkillAttachments(ctx, org.ID, project.ID)
	if err != nil || len(attachments) != 1 || attachments[0].Status != model.SkillStatusRevoked {
		t.Fatalf("admin attachments after revoke = %#v, err=%v", attachments, err)
	}
}

func TestReplaceProjectSkills_PostgresSerializesAgainstRevocation(t *testing.T) {
	dsn := os.Getenv("ANCHORED_TEST_DSN")
	if dsn == "" {
		t.Skip("ANCHORED_TEST_DSN not set; skipping Postgres skills concurrency test")
	}
	st, err := NewPostgresStore(PoolConfig{DSN: dsn})
	if err != nil {
		t.Fatalf("open postgres store: %v", err)
	}
	defer st.Close()

	ctx := context.Background()
	suffix := uniq()
	org, err := st.CreateOrganization(ctx, "Skill lock "+suffix, "skill-lock-"+suffix)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = st.db.ExecContext(ctx, `DELETE FROM organizations WHERE id = $1`, org.ID) }()
	account, err := st.CreateAccount(ctx, "skill-lock-"+suffix+"@example.test", "Skill lock", "hash")
	if err != nil {
		t.Fatal(err)
	}
	project, err := st.CreateProject(ctx, org.ID, "Skill lock", "skill-lock-"+suffix, "skill-lock-"+suffix, "", "", account.ID, "other")
	if err != nil {
		t.Fatal(err)
	}
	skill := &model.Skill{OrgID: org.ID, Name: "Concurrent", Slug: "concurrent-" + suffix, Purpose: "lock", Status: model.SkillStatusActive, CreatedBy: account.ID}
	if err := st.CreateSkill(ctx, skill, "# lock"); err != nil {
		t.Fatal(err)
	}

	// Hold the row before replacement starts. A correct replacement must wait
	// for this transaction, then observe the committed revocation and reject the
	// new attachment. A non-locking validation would read the old active row.
	blocker, err := st.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = blocker.Rollback() }()
	var lockedID string
	if err := blocker.QueryRowContext(ctx, `SELECT id::text FROM skills WHERE id = $1 FOR UPDATE`, skill.ID).Scan(&lockedID); err != nil {
		t.Fatal(err)
	}

	replaceDone := make(chan error, 1)
	go func() {
		_, replaceErr := st.ReplaceProjectSkills(ctx, org.ID, project.ID, []string{skill.ID})
		replaceDone <- replaceErr
	}()
	select {
	case replaceErr := <-replaceDone:
		t.Fatalf("replacement returned before skill lock released: %v", replaceErr)
	case <-time.After(150 * time.Millisecond):
	}
	if _, err := blocker.ExecContext(ctx, `UPDATE skills SET status = 'revoked', deleted_at = now(), updated_at = now() WHERE id = $1`, skill.ID); err != nil {
		t.Fatal(err)
	}
	if err := blocker.Commit(); err != nil {
		t.Fatal(err)
	}

	select {
	case replaceErr := <-replaceDone:
		if !errors.Is(replaceErr, ErrNotFound) {
			t.Fatalf("replacement after concurrent revoke = %v, want ErrNotFound", replaceErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("replacement remained blocked after revocation committed")
	}
	var attachments int
	if err := st.db.QueryRowContext(ctx, `SELECT count(*) FROM project_skills WHERE project_id = $1 AND skill_id = $2`, project.ID, skill.ID).Scan(&attachments); err != nil {
		t.Fatal(err)
	}
	if attachments != 0 {
		t.Fatalf("concurrently revoked skill was newly attached: count=%d", attachments)
	}
}

// TestUpdateSkill_LegacyRowNeedsABodyForFrontmatter_Postgres is the Postgres
// twin of TestUpdateSkill_LegacyRowNeedsABodyForFrontmatter.
func TestUpdateSkill_LegacyRowNeedsABodyForFrontmatter_Postgres(t *testing.T) {
	dsn := os.Getenv("ANCHORED_TEST_DSN")
	if dsn == "" {
		t.Skip("ANCHORED_TEST_DSN not set; skipping Postgres legacy skill test")
	}
	st, err := NewPostgresStore(PoolConfig{DSN: dsn})
	if err != nil {
		t.Fatalf("open postgres store: %v", err)
	}
	defer st.Close()
	checkLegacySkillNeedsBody(t, st, "-"+uniq())
}
