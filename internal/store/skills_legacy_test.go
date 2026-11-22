package store

import (
	"context"
	"errors"
	"testing"

	"github.com/jholhewres/anchored_oss/internal/model"
)

// TestUpdateSkill_LegacyRowNeedsABodyForFrontmatter: the store refuses
// frontmatter without a body on a legacy row, whoever calls. Its Postgres twin
// is TestUpdateSkill_LegacyRowNeedsABodyForFrontmatter_Postgres.
func TestUpdateSkill_LegacyRowNeedsABodyForFrontmatter(t *testing.T) {
	checkLegacySkillNeedsBody(t, newSQLiteTestStore(t), "")
}

func checkLegacySkillNeedsBody(t *testing.T, st Store, u string) {
	t.Helper()
	ctx := context.Background()
	org, err := st.CreateOrganization(ctx, "Legacy skills", "legacy-skills"+u)
	if err != nil {
		t.Fatal(err)
	}
	skill := &model.Skill{OrgID: org.ID, Name: "Notes", Slug: "notes" + u, Purpose: "Keep notes", Status: "active"}
	if err := st.CreateSkill(ctx, skill, "---\nname: notes\n---\n# Notes"); err != nil {
		t.Fatalf("create legacy skill: %v", err)
	}
	fm := map[string]any{"name": skill.Slug, "description": "Keep notes"}
	if _, err := st.UpdateSkill(ctx, org.ID, skill.ID, "", model.SkillUpdate{Frontmatter: &fm}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("frontmatter-only update of a legacy skill = %v, want ErrInvalid", err)
	}
	body := "# Notes"
	updated, err := st.UpdateSkill(ctx, org.ID, skill.ID, "", model.SkillUpdate{Frontmatter: &fm, Body: &body})
	if err != nil {
		t.Fatalf("frontmatter and body update of a legacy skill: %v", err)
	}
	if updated.CurrentVersion == nil || updated.CurrentVersion.Frontmatter == nil || updated.CurrentVersion.Body != body {
		t.Fatalf("legacy skill after a full update = %#v", updated.CurrentVersion)
	}
}
