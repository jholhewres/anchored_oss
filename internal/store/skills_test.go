package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jholhewres/anchored_oss/internal/model"
)

func TestSkillsVersioningAttachmentsAndRuntimeFiltering(t *testing.T) {
	checkSkillsVersioningAttachmentsAndRuntimeFiltering(t, newSQLiteTestStore(t))
}

func checkSkillsVersioningAttachmentsAndRuntimeFiltering(t *testing.T, st Store) {
	t.Helper()
	ctx := context.Background()
	orgID, accountID := projectTestOrg(t, st)
	project, err := st.CreateProject(ctx, orgID, "Repo", "repo", "repo-key", "", "", accountID, "other")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	skill := &model.Skill{OrgID: orgID, Name: "Release guide", Slug: "release-guide", Purpose: "Prepare releases", Status: model.SkillStatusActive, CreatedBy: accountID}
	if err := st.CreateSkill(ctx, skill, "# Version one"); err != nil {
		t.Fatalf("create skill: %v", err)
	}
	if skill.CurrentVersion == nil || skill.CurrentVersion.Version != 1 {
		t.Fatalf("initial version = %#v", skill.CurrentVersion)
	}
	content, purpose := "# Version two", "Prepare safe releases"
	updated, err := st.UpdateSkill(ctx, orgID, skill.ID, accountID, model.SkillUpdate{Content: &content, Purpose: &purpose})
	if err != nil {
		t.Fatalf("publish version: %v", err)
	}
	if updated.CurrentVersion.Version != 2 || updated.CurrentVersion.Content != content || updated.CurrentVersion.ContentHash == skillHash("# Version one") {
		t.Fatalf("updated version = %#v", updated.CurrentVersion)
	}
	if _, err := st.ReplaceProjectSkills(ctx, orgID, project.ID, []string{skill.ID}); err != nil {
		t.Fatalf("attach skill: %v", err)
	}
	descriptors, err := st.ListProjectSkills(ctx, project.ID, "safe", 20)
	if err != nil || len(descriptors) != 1 {
		t.Fatalf("runtime descriptors = %#v, err=%v", descriptors, err)
	}
	if descriptors[0].Version != 2 || descriptors[0].ContentHash != updated.CurrentVersion.ContentHash || descriptors[0].AttachedAt == nil {
		t.Fatalf("descriptor = %#v", descriptors[0])
	}
	loaded, err := st.GetProjectSkill(ctx, project.ID, skill.Slug)
	if err != nil || loaded.CurrentVersion.Content != content {
		t.Fatalf("load current = %#v, err=%v", loaded, err)
	}
	status := model.SkillStatusDeprecated
	if _, err := st.UpdateSkill(ctx, orgID, skill.ID, accountID, model.SkillUpdate{Status: &status}); err != nil {
		t.Fatalf("deprecate: %v", err)
	}
	descriptors, err = st.ListProjectSkills(ctx, project.ID, "", 20)
	if err != nil || len(descriptors) != 0 {
		t.Fatalf("deprecated runtime descriptors = %#v, err=%v", descriptors, err)
	}
}

func TestSkillsAttachmentRejectsCrossOrgSkillAndRevokeHidesIt(t *testing.T) {
	checkSkillsAttachmentRejectsCrossOrgSkillAndRevokeHidesIt(t, newSQLiteTestStore(t))
}

func checkSkillsAttachmentRejectsCrossOrgSkillAndRevokeHidesIt(t *testing.T, st Store) {
	t.Helper()
	ctx := context.Background()
	orgID, accountID := projectTestOrg(t, st)
	project, err := st.CreateProject(ctx, orgID, "Repo", "repo", "repo-key", "", "", accountID, "other")
	if err != nil {
		t.Fatal(err)
	}
	other, err := st.CreateOrganization(ctx, "Else", "else")
	if err != nil {
		t.Fatal(err)
	}
	skill := &model.Skill{OrgID: other.ID, Name: "Other", Slug: "other", Purpose: "Other org", Status: model.SkillStatusActive}
	if err := st.CreateSkill(ctx, skill, "# Other"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.ReplaceProjectSkills(ctx, orgID, project.ID, []string{skill.ID}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-org attachment error = %v, want ErrNotFound", err)
	}
	local := &model.Skill{OrgID: orgID, Name: "Local", Slug: "local", Purpose: "Local org", Status: model.SkillStatusActive, CreatedBy: accountID}
	if err := st.CreateSkill(ctx, local, "# Local"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.ReplaceProjectSkills(ctx, orgID, project.ID, []string{local.ID}); err != nil {
		t.Fatal(err)
	}
	if err := st.RevokeSkill(ctx, orgID, local.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetProjectSkill(ctx, project.ID, local.Slug); !errors.Is(err, ErrNotFound) {
		t.Fatalf("revoked runtime load error = %v, want ErrNotFound", err)
	}
}

func TestProjectSkillAttachmentsIncludeInactiveAndRevokedButStayOrgScoped(t *testing.T) {
	checkProjectSkillAttachmentsIncludeInactiveAndRevokedButStayOrgScoped(t, newSQLiteTestStore(t))
}

func checkProjectSkillAttachmentsIncludeInactiveAndRevokedButStayOrgScoped(t *testing.T, st Store) {
	t.Helper()
	ctx := context.Background()
	orgID, accountID := projectTestOrg(t, st)
	project, err := st.CreateProject(ctx, orgID, "Repo", "repo", "repo-key", "", "", accountID, "other")
	if err != nil {
		t.Fatal(err)
	}
	active := &model.Skill{OrgID: orgID, Name: "Active", Slug: "active", Purpose: "active", Status: model.SkillStatusActive, CreatedBy: accountID}
	draft := &model.Skill{OrgID: orgID, Name: "Draft", Slug: "draft", Purpose: "draft", Status: model.SkillStatusDraft, CreatedBy: accountID}
	deprecated := &model.Skill{OrgID: orgID, Name: "Deprecated", Slug: "deprecated", Purpose: "deprecated", Status: model.SkillStatusDeprecated, CreatedBy: accountID}
	for _, skill := range []*model.Skill{active, draft, deprecated} {
		if err := st.CreateSkill(ctx, skill, "# "+skill.Name); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := st.ReplaceProjectSkills(ctx, orgID, project.ID, []string{active.ID, draft.ID, deprecated.ID}); err != nil {
		t.Fatal(err)
	}
	if err := st.RevokeSkill(ctx, orgID, deprecated.ID); err != nil {
		t.Fatal(err)
	}

	attachments, err := st.ListProjectSkillAttachments(ctx, orgID, project.ID)
	if err != nil || len(attachments) != 3 {
		t.Fatalf("attachments = %#v, err=%v", attachments, err)
	}
	statuses := map[string]string{}
	for _, attachment := range attachments {
		statuses[attachment.ID] = attachment.Status
		if attachment.AttachedAt == nil {
			t.Fatalf("attachment missing timestamp: %#v", attachment)
		}
	}
	if statuses[active.ID] != model.SkillStatusActive || statuses[draft.ID] != model.SkillStatusDraft || statuses[deprecated.ID] != model.SkillStatusRevoked {
		t.Fatalf("attachment statuses = %#v", statuses)
	}
	runtime, err := st.ListProjectSkills(ctx, project.ID, "", 20)
	if err != nil || len(runtime) != 1 || runtime[0].ID != active.ID {
		t.Fatalf("runtime skills = %#v, err=%v", runtime, err)
	}
	other, err := st.CreateOrganization(ctx, "Else", "else")
	if err != nil {
		t.Fatal(err)
	}
	otherView, err := st.ListProjectSkillAttachments(ctx, other.ID, project.ID)
	if err != nil || len(otherView) != 0 {
		t.Fatalf("cross-org attachments = %#v, err=%v", otherView, err)
	}
}

func TestReplaceProjectSkillsPreservesExistingRevokedButRejectsNewRevoked(t *testing.T) {
	checkReplaceProjectSkillsPreservesExistingRevokedButRejectsNewRevoked(t, newSQLiteTestStore(t))
}

func checkReplaceProjectSkillsPreservesExistingRevokedButRejectsNewRevoked(t *testing.T, st Store) {
	t.Helper()
	ctx := context.Background()
	orgID, accountID := projectTestOrg(t, st)
	project, err := st.CreateProject(ctx, orgID, "Repo", "repo", "repo-key", "", "", accountID, "other")
	if err != nil {
		t.Fatal(err)
	}
	revoked := &model.Skill{OrgID: orgID, Name: "Old", Slug: "old", Purpose: "old", Status: model.SkillStatusActive, CreatedBy: accountID}
	active := &model.Skill{OrgID: orgID, Name: "Current", Slug: "current", Purpose: "current", Status: model.SkillStatusActive, CreatedBy: accountID}
	for _, skill := range []*model.Skill{revoked, active} {
		if err := st.CreateSkill(ctx, skill, "# "+skill.Name); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := st.ReplaceProjectSkills(ctx, orgID, project.ID, []string{revoked.ID}); err != nil {
		t.Fatalf("initial attachment: %v", err)
	}
	if err := st.RevokeSkill(ctx, orgID, revoked.ID); err != nil {
		t.Fatal(err)
	}
	oldIDs, err := st.ReplaceProjectSkills(ctx, orgID, project.ID, []string{revoked.ID, active.ID})
	if err != nil {
		t.Fatalf("preserve revoked while attaching active: %v", err)
	}
	if len(oldIDs) != 1 || oldIDs[0] != revoked.ID {
		t.Fatalf("pre-replacement IDs = %v, want [%s]", oldIDs, revoked.ID)
	}
	newRevoked := &model.Skill{OrgID: orgID, Name: "Never attach", Slug: "never-attach", Purpose: "never", Status: model.SkillStatusActive, CreatedBy: accountID}
	if err := st.CreateSkill(ctx, newRevoked, "# never"); err != nil {
		t.Fatal(err)
	}
	if err := st.RevokeSkill(ctx, orgID, newRevoked.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.ReplaceProjectSkills(ctx, orgID, project.ID, []string{revoked.ID, active.ID, newRevoked.ID}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("new revoked attachment error = %v, want ErrNotFound", err)
	}
	if _, err := st.ReplaceProjectSkills(ctx, orgID, project.ID, []string{active.ID}); err != nil {
		t.Fatalf("detach revoked attachment: %v", err)
	}
	attachments, err := st.ListProjectSkillAttachments(ctx, orgID, project.ID)
	if err != nil || len(attachments) != 1 || attachments[0].ID != active.ID {
		t.Fatalf("attachments after detach = %#v, err=%v", attachments, err)
	}
}

func TestSkillStoreRejectsTerminalStatusOutsideRevoke(t *testing.T) {
	checkSkillStoreRejectsTerminalStatusOutsideRevoke(t, newSQLiteTestStore(t))
}

func checkSkillStoreRejectsTerminalStatusOutsideRevoke(t *testing.T, st Store) {
	t.Helper()
	ctx := context.Background()
	orgID, accountID := projectTestOrg(t, st)
	revoked := &model.Skill{OrgID: orgID, Name: "Invalid", Slug: "invalid", Purpose: "invalid", Status: model.SkillStatusRevoked, CreatedBy: accountID}
	if err := st.CreateSkill(ctx, revoked, "# invalid"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("create revoked error = %v, want ErrInvalid", err)
	}
	skill := &model.Skill{OrgID: orgID, Name: "Valid", Slug: "valid", Purpose: "valid", Status: model.SkillStatusDraft, CreatedBy: accountID}
	if err := st.CreateSkill(ctx, skill, "# valid"); err != nil {
		t.Fatal(err)
	}
	status := model.SkillStatusRevoked
	if _, err := st.UpdateSkill(ctx, orgID, skill.ID, accountID, model.SkillUpdate{Status: &status}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("PATCH revoked error = %v, want ErrInvalid", err)
	}
}

func TestReplaceProjectSkillsPreservesUnchangedAttachmentTimestamp(t *testing.T) {
	checkReplaceProjectSkillsPreservesUnchangedAttachmentTimestamp(t, newSQLiteTestStore(t))
}

func checkReplaceProjectSkillsPreservesUnchangedAttachmentTimestamp(t *testing.T, st Store) {
	t.Helper()
	ctx := context.Background()
	orgID, accountID := projectTestOrg(t, st)
	project, err := st.CreateProject(ctx, orgID, "Repo", "repo", "repo-key", "", "", accountID, "other")
	if err != nil {
		t.Fatal(err)
	}
	skill := &model.Skill{OrgID: orgID, Name: "Keep", Slug: "keep", Purpose: "keep", Status: model.SkillStatusActive, CreatedBy: accountID}
	if err := st.CreateSkill(ctx, skill, "# keep"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.ReplaceProjectSkills(ctx, orgID, project.ID, []string{skill.ID}); err != nil {
		t.Fatal(err)
	}
	const attachedAt = "2001-02-03 04:05:06"
	execRaw(t, st,
		`UPDATE project_skills SET created_at = ? WHERE project_id = ? AND skill_id = ?`,
		`UPDATE project_skills SET created_at = $1::timestamp AT TIME ZONE 'UTC' WHERE project_id = $2 AND skill_id = $3`,
		attachedAt, project.ID, skill.ID)
	if _, err := st.ReplaceProjectSkills(ctx, orgID, project.ID, []string{skill.ID}); err != nil {
		t.Fatalf("keep unchanged attachment: %v", err)
	}
	attachments, err := st.ListProjectSkillAttachments(ctx, orgID, project.ID)
	if err != nil || len(attachments) != 1 || attachments[0].AttachedAt == nil {
		t.Fatalf("attachments = %#v, err=%v", attachments, err)
	}
	want, err := time.Parse("2006-01-02 15:04:05", attachedAt)
	if err != nil {
		t.Fatal(err)
	}
	if !attachments[0].AttachedAt.Equal(want) {
		t.Fatalf("attached_at = %s, want %s", attachments[0].AttachedAt, want)
	}
}

// execRaw runs one statement directly on either backend, for test setup no
// store method offers.
func execRaw(t *testing.T, st Store, sqliteQuery, pgQuery string, args ...any) {
	t.Helper()
	query := sqliteQuery
	if isPostgres(st) {
		query = pgQuery
	}
	if _, err := rawDB(t, st).Exec(query, args...); err != nil {
		t.Fatalf("exec raw: %v", err)
	}
}
