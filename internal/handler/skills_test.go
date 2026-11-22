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

// skillCall runs one skill handler method as an admin of orgID.
func skillCall(h *SkillHandler, method func(http.ResponseWriter, *http.Request), verb, path, body, orgID, accountID string, pathValues ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(verb, path, strings.NewReader(body)).WithContext(adminCtx(orgID, accountID))
	for i := 0; i+1 < len(pathValues); i += 2 {
		req.SetPathValue(pathValues[i], pathValues[i+1])
	}
	rec := httptest.NewRecorder()
	method(rec, req)
	return rec
}

// TestSkillContentHeaderIsHeldToTheContract (attack h2, run 20261122T183109Z):
// a header inside the legacy content field passes the same check as
// frontmatter (name and description only, matching slug and purpose) and is
// stored canonically, so no other header key ever reaches an agent.
func TestSkillContentHeaderIsHeldToTheContract(t *testing.T) {
	st := newProjectTestStore(t)
	orgID, accountID, _ := seedProject(t, st)
	h := NewSkillHandler(st, slog.Default())

	for name, content := range map[string]string{
		"extra key":       `---\nname: deploy\ndescription: Deploy safely\nallowed-tools: Bash\n---\n# Deploy\n`,
		"spaced opening":  `--- \nname: deploy\ndescription: Deploy safely\nallowed-tools: Bash\n---\n# Deploy\n`,
		"unterminated":    `---\nname: deploy\ndescription: Deploy safely\n# Deploy\n`,
		"name mismatch":   `---\nname: other\ndescription: Deploy safely\n---\n# Deploy\n`,
		"header, no body": `---\nname: deploy\ndescription: Deploy safely\n---\n`,
	} {
		body := `{"name":"Deploy","slug":"deploy","purpose":"Deploy safely","status":"active","content":"` + content + `"}`
		if rec := skillCall(h, h.Create, http.MethodPost, "/v1/skills", body, orgID, accountID); rec.Code != http.StatusBadRequest {
			t.Fatalf("create with content header (%s) = %d, want 400; body=%s", name, rec.Code, rec.Body.String())
		}
	}

	rec := skillCall(h, h.Create, http.MethodPost, "/v1/skills",
		`{"name":"Deploy","slug":"deploy","purpose":"Deploy safely","status":"active","content":"---\nname: deploy\ndescription: Deploy safely\n---\n# Deploy\n"}`, orgID, accountID)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create with a valid content header = %d; body=%s", rec.Code, rec.Body.String())
	}
	var skill runtimeSkillResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &skill); err != nil {
		t.Fatal(err)
	}
	if skill.Frontmatter == nil || skill.Body != "# Deploy\n" || skill.Content != "---\nname: deploy\ndescription: Deploy safely\n---\n# Deploy\n" {
		t.Fatalf("content with a header is not stored canonically: %#v", skill)
	}

	// Header-less content still makes a legacy row, byte for byte.
	rec = skillCall(h, h.Create, http.MethodPost, "/v1/skills",
		`{"name":"Notes","slug":"notes","purpose":"Keep notes","status":"active","content":"# Notes\n\nPlain."}`, orgID, accountID)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create with header-less content = %d; body=%s", rec.Code, rec.Body.String())
	}
	var legacy runtimeSkillResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &legacy); err != nil {
		t.Fatal(err)
	}
	if legacy.Frontmatter != nil || legacy.Content != "# Notes\n\nPlain." {
		t.Fatalf("header-less content changed: %#v", legacy)
	}

	// On that legacy row, a PATCH with a bad header is refused and a good
	// one makes the row canonical.
	if rec := skillCall(h, h.Update, http.MethodPatch, "/v1/skills/"+legacy.ID,
		`{"content":"---\nname: notes\ndescription: Keep notes\nallowed-tools: Bash\n---\n# Notes\n"}`, orgID, accountID, "id", legacy.ID); rec.Code != http.StatusBadRequest {
		t.Fatalf("PATCH content with an extra header key = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	rec = skillCall(h, h.Update, http.MethodPatch, "/v1/skills/"+legacy.ID,
		`{"content":"---\nname: notes\ndescription: Keep notes\n---\n# Notes v2\n"}`, orgID, accountID, "id", legacy.ID)
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH content with a valid header = %d; body=%s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &legacy); err != nil {
		t.Fatal(err)
	}
	if legacy.Frontmatter == nil || legacy.Content != "---\nname: notes\ndescription: Keep notes\n---\n# Notes v2\n" {
		t.Fatalf("PATCH with a header did not make the row canonical: %#v", legacy)
	}
}

// TestSkillLegacyRowNeedsABodyForFrontmatter: frontmatter alone on a legacy
// row would render a second header over its raw SKILL.md, so it is refused;
// with a body it converts the row.
func TestSkillLegacyRowNeedsABodyForFrontmatter(t *testing.T) {
	st := newProjectTestStore(t)
	orgID, accountID, _ := seedProject(t, st)
	h := NewSkillHandler(st, slog.Default())
	rec := skillCall(h, h.Create, http.MethodPost, "/v1/skills",
		`{"name":"Notes","slug":"notes","purpose":"Keep notes","status":"active","content":"# Notes"}`, orgID, accountID)
	if rec.Code != http.StatusCreated {
		t.Fatalf("legacy create = %d; body=%s", rec.Code, rec.Body.String())
	}
	var legacy runtimeSkillResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &legacy); err != nil {
		t.Fatal(err)
	}
	fm := `"frontmatter":{"name":"notes","description":"Keep notes"}`
	if rec := skillCall(h, h.Update, http.MethodPatch, "/v1/skills/"+legacy.ID, `{`+fm+`}`, orgID, accountID, "id", legacy.ID); rec.Code != http.StatusBadRequest {
		t.Fatalf("frontmatter-only PATCH on a legacy row = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
	if rec := skillCall(h, h.Update, http.MethodPatch, "/v1/skills/"+legacy.ID, `{`+fm+`,"body":"# Notes"}`, orgID, accountID, "id", legacy.ID); rec.Code != http.StatusOK {
		t.Fatalf("frontmatter and body PATCH on a legacy row = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
}

// TestSkillAttachmentsNeedUUIDs: skill_ids that are not UUIDs are a 400, not a
// database error.
func TestSkillAttachmentsNeedUUIDs(t *testing.T) {
	st := newProjectTestStore(t)
	orgID, accountID, projectID := seedProject(t, st)
	h := NewSkillHandler(st, slog.Default())
	rec := skillCall(h, h.ReplaceProject, http.MethodPut, "/v1/projects/"+projectID+"/skills",
		`{"skill_ids":["not-a-uuid"]}`, orgID, accountID, "id", projectID)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("attach a non-UUID skill id = %d, want 400; body=%s", rec.Code, rec.Body.String())
	}
}

// TestSkillRuntimeWithholdsAnInvalidLegacyHeader: a legacy row written before
// headers were checked keeps its bytes for admins, but the runtime endpoint
// never serves it while its header breaks the contract; a valid legacy row
// is served byte for byte.
func TestSkillRuntimeWithholdsAnInvalidLegacyHeader(t *testing.T) {
	st := newProjectTestStore(t)
	orgID, accountID, projectID := seedProject(t, st)
	h := NewSkillHandler(st, slog.Default())
	ctx := context.Background()
	var attached []string
	for slug, content := range map[string]string{
		"planted": "---\nname: planted\ndescription: Planted\nallowed-tools: Bash\n---\n# Planted\n",
		"spaced":  "--- \nname: spaced\ndescription: Spaced\nallowed-tools: Bash\n---\n# Spaced\n",
		"plain":   "# Plain\n\nNo header.",
	} {
		skill := &model.Skill{OrgID: orgID, Name: slug, Slug: slug, Purpose: slug, Status: model.SkillStatusActive, CreatedBy: accountID}
		if err := st.CreateSkill(ctx, skill, content); err != nil {
			t.Fatal(err)
		}
		attached = append(attached, skill.ID)
		if _, err := st.ReplaceProjectSkills(ctx, orgID, projectID, attached); err != nil {
			t.Fatal(err)
		}
	}
	for slug, want := range map[string]int{"planted": http.StatusNotFound, "spaced": http.StatusNotFound, "plain": http.StatusOK} {
		rec := skillCall(h, h.GetProject, http.MethodGet, "/v1/projects/"+projectID+"/skills/"+slug, "", orgID, accountID, "id", projectID, "slug", slug)
		if rec.Code != want {
			t.Fatalf("runtime GET %s = %d, want %d; body=%s", slug, rec.Code, want, rec.Body.String())
		}
	}
}
