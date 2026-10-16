package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jholhewres/anchored_oss/internal/middleware"
	"github.com/jholhewres/anchored_oss/internal/model"
)

func TestSkillHandlerCreateAndRuntimeWireContract(t *testing.T) {
	st := newProjectTestStore(t)
	orgID, accountID, projectID := seedProject(t, st)
	h := NewSkillHandler(st, slog.Default())

	create := httptest.NewRequest(http.MethodPost, "/v1/skills", strings.NewReader(`{"name":"Release guide","slug":"release-guide","purpose":"prepare releases","content":"\n# release\n","status":"active"}`)).WithContext(adminCtx(orgID, accountID))
	created := httptest.NewRecorder()
	h.Create(created, create)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status = %d body=%s", created.Code, created.Body.String())
	}
	var skill runtimeSkillResponse
	if err := json.Unmarshal(created.Body.Bytes(), &skill); err != nil {
		t.Fatal(err)
	}
	if skill.Version != 1 || skill.Content != "\n# release\n" || skill.ContentHash == "" || skill.Status != "active" {
		t.Fatalf("created wire response = %#v", skill)
	}

	replace := httptest.NewRequest(http.MethodPut, "/v1/projects/"+projectID+"/skills", strings.NewReader(`{"skill_ids":["`+skill.ID+`"]}`)).WithContext(adminCtx(orgID, accountID))
	replace.SetPathValue("id", projectID)
	replaced := httptest.NewRecorder()
	h.ReplaceProject(replaced, replace)
	if replaced.Code != http.StatusNoContent {
		t.Fatalf("replace status = %d body=%s", replaced.Code, replaced.Body.String())
	}
	second := &model.Skill{OrgID: orgID, Name: "Deploy guide", Slug: "deploy-guide", Purpose: "prepare deploys", Status: model.SkillStatusActive, CreatedBy: accountID}
	if err := st.CreateSkill(context.Background(), second, "# deploy"); err != nil {
		t.Fatalf("create second skill: %v", err)
	}
	replace = httptest.NewRequest(http.MethodPut, "/v1/projects/"+projectID+"/skills", strings.NewReader(`{"skill_ids":["`+second.ID+`"]}`)).WithContext(adminCtx(orgID, accountID))
	replace.SetPathValue("id", projectID)
	replaced = httptest.NewRecorder()
	h.ReplaceProject(replaced, replace)
	if replaced.Code != http.StatusNoContent {
		t.Fatalf("second replace status = %d body=%s", replaced.Code, replaced.Body.String())
	}
	audits, _, err := st.ListAuditEntries(context.Background(), orgID, model.AuditFilters{})
	if err != nil {
		t.Fatalf("list audits: %v", err)
	}
	foundReplaceAudit := false
	for _, audit := range audits {
		if audit.Action != "skill.attachments.replace" {
			continue
		}
		metadata, ok := audit.Metadata.(map[string]any)
		if !ok {
			continue
		}
		attached, detached := metadata["attached_ids"], metadata["detached_ids"]
		if audit.TargetType == "project" && audit.TargetID == projectID && audit.ProjectID == projectID &&
			containsAttachmentID(attached, second.ID) && containsAttachmentID(detached, skill.ID) && metadata["total_count"] == float64(1) {
			foundReplaceAudit = true
		}
	}
	if !foundReplaceAudit {
		t.Fatalf("missing exact attachment replacement audit: %#v", audits)
	}

	readCtx := context.WithValue(context.Background(), middleware.OrgIDKey, orgID)
	readCtx = context.WithValue(readCtx, middleware.AccountIDKey, accountID)
	readCtx = context.WithValue(readCtx, middleware.ScopeKey, "admin")
	list := httptest.NewRequest(http.MethodGet, "/v1/projects/"+projectID+"/skills?q=deploy", nil).WithContext(readCtx)
	list.SetPathValue("id", projectID)
	listed := httptest.NewRecorder()
	h.ListProject(listed, list)
	if listed.Code != http.StatusOK {
		t.Fatalf("list status = %d body=%s", listed.Code, listed.Body.String())
	}
	var descriptors []runtimeSkillResponse
	if err := json.Unmarshal(listed.Body.Bytes(), &descriptors); err != nil {
		t.Fatal(err)
	}
	if len(descriptors) != 1 || descriptors[0].Content != "" || descriptors[0].AttachedAt == nil || descriptors[0].Version != 1 {
		t.Fatalf("descriptor wire response = %#v", descriptors)
	}

	load := httptest.NewRequest(http.MethodGet, "/v1/projects/"+projectID+"/skills/deploy-guide", nil).WithContext(readCtx)
	load.SetPathValue("id", projectID)
	load.SetPathValue("slug", "deploy-guide")
	loaded := httptest.NewRecorder()
	h.GetProject(loaded, load)
	if loaded.Code != http.StatusOK {
		t.Fatalf("load status = %d body=%s", loaded.Code, loaded.Body.String())
	}
	if err := json.Unmarshal(loaded.Body.Bytes(), &skill); err != nil {
		t.Fatal(err)
	}
	if skill.Content != "# deploy" || skill.Version != 1 {
		t.Fatalf("load wire response = %#v", skill)
	}
}

func containsAttachmentID(value any, id string) bool {
	ids, ok := value.([]any)
	if !ok {
		return false
	}
	for _, candidate := range ids {
		if candidate == id {
			return true
		}
	}
	return false
}

func TestAttachmentDiffSortsAuditIDs(t *testing.T) {
	attached, detached := attachmentDiff([]string{"c", "a"}, []string{"d", "b", "d"})
	if strings.Join(attached, ",") != "b,d" || strings.Join(detached, ",") != "a,c" {
		t.Fatalf("attachment diff attached=%v detached=%v", attached, detached)
	}
}

func TestSkillHandlerListsAllAdminAttachments(t *testing.T) {
	st := newProjectTestStore(t)
	orgID, accountID, projectID := seedProject(t, st)
	h := NewSkillHandler(st, slog.Default())
	draft := &model.Skill{OrgID: orgID, Name: "Draft", Slug: "draft", Purpose: "draft", Status: model.SkillStatusDraft, CreatedBy: accountID}
	if err := st.CreateSkill(context.Background(), draft, "# draft"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.ReplaceProjectSkills(context.Background(), orgID, projectID, []string{draft.ID}); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/projects/"+projectID+"/skill-attachments", nil).WithContext(adminCtx(orgID, accountID))
	req.SetPathValue("id", projectID)
	rec := httptest.NewRecorder()
	h.ListAttachments(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("attachments status = %d body=%s", rec.Code, rec.Body.String())
	}
	var attachments []runtimeSkillResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &attachments); err != nil {
		t.Fatal(err)
	}
	if len(attachments) != 1 || attachments[0].ID != draft.ID || attachments[0].Status != model.SkillStatusDraft || attachments[0].Content != "" {
		t.Fatalf("attachment response = %#v", attachments)
	}
}

func TestSkillHandlerRejectsOversizedRuntimeQuery(t *testing.T) {
	st := newProjectTestStore(t)
	orgID, accountID, projectID := seedProject(t, st)
	h := NewSkillHandler(st, slog.Default())
	req := httptest.NewRequest(http.MethodGet, "/v1/projects/"+projectID+"/skills?q="+strings.Repeat("a", 501), nil).WithContext(adminCtx(orgID, accountID))
	req.SetPathValue("id", projectID)
	rec := httptest.NewRecorder()
	h.ListProject(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("oversized query status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestSkillHandlerRejectsTerminalStatusWrites(t *testing.T) {
	st := newProjectTestStore(t)
	orgID, accountID, _ := seedProject(t, st)
	h := NewSkillHandler(st, slog.Default())
	create := httptest.NewRequest(http.MethodPost, "/v1/skills", strings.NewReader(`{"name":"Invalid","slug":"invalid","purpose":"invalid","content":"# invalid","status":"revoked"}`)).WithContext(adminCtx(orgID, accountID))
	created := httptest.NewRecorder()
	h.Create(created, create)
	if created.Code != http.StatusBadRequest {
		t.Fatalf("create revoked status = %d body=%s", created.Code, created.Body.String())
	}
	skill := &model.Skill{OrgID: orgID, Name: "Valid", Slug: "valid", Purpose: "valid", Status: model.SkillStatusDraft, CreatedBy: accountID}
	if err := st.CreateSkill(context.Background(), skill, "# valid"); err != nil {
		t.Fatal(err)
	}
	patch := httptest.NewRequest(http.MethodPatch, "/v1/skills/"+skill.ID, strings.NewReader(`{"status":"revoked"}`)).WithContext(adminCtx(orgID, accountID))
	patch.SetPathValue("id", skill.ID)
	patched := httptest.NewRecorder()
	h.Update(patched, patch)
	if patched.Code != http.StatusBadRequest {
		t.Fatalf("PATCH revoked status = %d body=%s", patched.Code, patched.Body.String())
	}
}

func TestSkillHandlerRuntimeDoesNotLeakCrossOrgProject(t *testing.T) {
	st := newProjectTestStore(t)
	_, accountID, projectID := seedProject(t, st)
	h := NewSkillHandler(st, slog.Default())
	ctx := adminCtx("00000000-0000-0000-0000-000000000000", accountID)
	req := httptest.NewRequest(http.MethodGet, "/v1/projects/"+projectID+"/skills", nil).WithContext(ctx)
	req.SetPathValue("id", projectID)
	rec := httptest.NewRecorder()
	h.ListProject(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross-org project status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestSkillHandlerCanonicalFrontmatterAndTemplateContract(t *testing.T) {
	st := newProjectTestStore(t)
	orgID, accountID, _ := seedProject(t, st)
	h := NewSkillHandler(st, slog.Default())

	templates := httptest.NewRequest(http.MethodGet, "/v1/skill-templates", nil).WithContext(adminCtx(orgID, accountID))
	templateRec := httptest.NewRecorder()
	h.ListTemplates(templateRec, templates)
	if templateRec.Code != http.StatusOK {
		t.Fatalf("template list status = %d body=%s", templateRec.Code, templateRec.Body.String())
	}
	var listed []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(templateRec.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed) != 3 || listed[0].ID != "code-review/v1" || listed[1].ID != "test-strategy/v1" || listed[2].ID != "business-rules/v1" {
		t.Fatalf("templates = %#v", listed)
	}

	create := httptest.NewRequest(http.MethodPost, "/v1/skills", strings.NewReader(`{"name":"Release guide","slug":"release-guide","purpose":"prepare releases","frontmatter":{"name":"release-guide","description":"prepare releases"},"body":"# Release\r\n\r\nShip safely"}`)).WithContext(adminCtx(orgID, accountID))
	created := httptest.NewRecorder()
	h.Create(created, create)
	if created.Code != http.StatusCreated {
		t.Fatalf("canonical create status = %d body=%s", created.Code, created.Body.String())
	}
	var skill runtimeSkillResponse
	if err := json.Unmarshal(created.Body.Bytes(), &skill); err != nil {
		t.Fatal(err)
	}
	if skill.Frontmatter == nil || (*skill.Frontmatter)["name"] != "release-guide" || (*skill.Frontmatter)["description"] != "prepare releases" || skill.Body != "# Release\r\n\r\nShip safely" || !strings.HasPrefix(skill.Content, "---\nname: release-guide\ndescription: prepare releases\n---\n") || !strings.HasSuffix(skill.Content, "Ship safely\n") || skill.ContentHash == "" {
		t.Fatalf("canonical skill response = %#v", skill)
	}
	if skill.ContentHash != sha256Hex(skill.Content) {
		t.Fatalf("content hash = %q, want sha256(%q)", skill.ContentHash, skill.Content)
	}
	legacyAlias := httptest.NewRequest(http.MethodPatch, "/v1/skills/"+skill.ID, strings.NewReader(`{"content":"# nested header"}`)).WithContext(adminCtx(orgID, accountID))
	legacyAlias.SetPathValue("id", skill.ID)
	legacyAliasRec := httptest.NewRecorder()
	h.Update(legacyAliasRec, legacyAlias)
	if legacyAliasRec.Code != http.StatusBadRequest {
		t.Fatalf("canonical legacy alias status = %d body=%s", legacyAliasRec.Code, legacyAliasRec.Body.String())
	}
	patch := httptest.NewRequest(http.MethodPatch, "/v1/skills/"+skill.ID, strings.NewReader(`{"body":"# Release\n\nShip even safer"}`)).WithContext(adminCtx(orgID, accountID))
	patch.SetPathValue("id", skill.ID)
	patched := httptest.NewRecorder()
	h.Update(patched, patch)
	if patched.Code != http.StatusOK {
		t.Fatalf("canonical body patch status = %d body=%s", patched.Code, patched.Body.String())
	}
	if err := json.Unmarshal(patched.Body.Bytes(), &skill); err != nil {
		t.Fatal(err)
	}
	if skill.Version != 2 || skill.Body != "# Release\n\nShip even safer" || skill.ContentHash != sha256Hex(skill.Content) {
		t.Fatalf("canonical patch response = %#v", skill)
	}

	bad := httptest.NewRequest(http.MethodPost, "/v1/skills", strings.NewReader(`{"name":"Bad","slug":"bad","purpose":"bad","frontmatter":{"name":"bad","description":"bad","tools":["tests"]},"body":"# bad"}`)).WithContext(adminCtx(orgID, accountID))
	badRec := httptest.NewRecorder()
	h.Create(badRec, bad)
	if badRec.Code != http.StatusBadRequest {
		t.Fatalf("reserved frontmatter status = %d body=%s", badRec.Code, badRec.Body.String())
	}

	fromTemplate := httptest.NewRequest(http.MethodPost, "/v1/skills", strings.NewReader(`{"template_id":"test-strategy/v1","slug":"project-test-strategy"}`)).WithContext(adminCtx(orgID, accountID))
	templateCreated := httptest.NewRecorder()
	h.Create(templateCreated, fromTemplate)
	if templateCreated.Code != http.StatusCreated {
		t.Fatalf("template create status = %d body=%s", templateCreated.Code, templateCreated.Body.String())
	}
	var copied runtimeSkillResponse
	if err := json.Unmarshal(templateCreated.Body.Bytes(), &copied); err != nil {
		t.Fatal(err)
	}
	if copied.Name != "Test strategy" || copied.Frontmatter == nil || copied.Body == "" || copied.ContentHash != sha256Hex(copied.Content) {
		t.Fatalf("template copy = %#v", copied)
	}
}

func TestSkillPublishAuditIncludesAffectedProjectCount(t *testing.T) {
	st := newProjectTestStore(t)
	orgID, accountID, projectID := seedProject(t, st)
	h := NewSkillHandler(st, slog.Default())
	skill := &model.Skill{OrgID: orgID, Name: "Shared", Slug: "shared", Purpose: "shared", Status: model.SkillStatusActive, CreatedBy: accountID}
	if err := st.CreateSkill(context.Background(), skill, "# shared"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.ReplaceProjectSkills(context.Background(), orgID, projectID, []string{skill.ID}); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPatch, "/v1/skills/"+skill.ID, strings.NewReader(`{"content":"# updated"}`)).WithContext(adminCtx(orgID, accountID))
	req.SetPathValue("id", skill.ID)
	rec := httptest.NewRecorder()
	h.Update(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("publish status = %d body=%s", rec.Code, rec.Body.String())
	}
	audits, _, err := st.ListAuditEntries(context.Background(), orgID, model.AuditFilters{})
	if err != nil {
		t.Fatal(err)
	}
	for _, audit := range audits {
		if audit.Action != "skill.publish" {
			continue
		}
		metadata, ok := audit.Metadata.(map[string]any)
		if ok && metadata["affected_projects"] == float64(1) {
			return
		}
	}
	t.Fatalf("missing affected_projects publish audit: %#v", audits)
}
