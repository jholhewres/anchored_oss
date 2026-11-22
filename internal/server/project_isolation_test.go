package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/jholhewres/anchored_oss/internal/model"
)

// registerForeignAdmin self-registers Mallory, which makes her the admin of a
// new org "Evil": the attacker every isolation test starts from.
func registerForeignAdmin(t *testing.T, ts *httptest.Server) (key, orgID, accountID string) {
	t.Helper()
	code, reg := call(t, ts, http.MethodPost, "/v1/auth/register", "",
		`{"email":"mallory@example.test","password":"password123","display_name":"Mallory","org_name":"Evil"}`)
	key, _ = reg["api_key"].(string)
	orgID, _ = reg["org_id"].(string)
	accountID, _ = reg["account_id"].(string)
	if code != http.StatusCreated || key == "" || orgID == "" || accountID == "" {
		t.Fatalf("register = %d %v", code, reg)
	}
	return key, orgID, accountID
}

// TestAdminOfAnotherOrgCannotReachProjectData: an admin key only reaches the
// projects of its own org. Self-registration hands out admin of a new org, so
// "admin somewhere" must never open another org's project by its UUID.
func TestAdminOfAnotherOrgCannotReachProjectData(t *testing.T) {
	ts, st, ids := accountLifecycleServer(t, "owner@example.test")
	ctx := context.Background()

	orgID, _, err := st.GetAccountOrgMembership(ctx, ids["owner@example.test"])
	if err != nil {
		t.Fatal(err)
	}
	project, err := st.CreateProject(ctx, orgID, "Secret", "secret", "0123456789abcdef", "", "", ids["owner@example.test"], "other")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := st.UpsertMemory(ctx, &model.Memory{
		ID: "secret-memory", ProjectID: project.ID, Category: "decision",
		Content:     "The launch date for the secret project is the first of December, keep it internal.",
		ContentHash: "sha256:secret-memory", AuthorID: ids["owner@example.test"], AuthorName: "Owner",
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	foreignAdmin, _, _ := registerForeignAdmin(t, ts)

	pid := project.ID
	for _, tc := range []struct{ method, path, body string }{
		{http.MethodGet, "/v1/projects/" + pid, ""},
		{http.MethodGet, "/v1/projects/" + pid + "/memories", ""},
		{http.MethodGet, "/v1/projects/" + pid + "/graph", ""},
		{http.MethodPost, "/v1/projects/" + pid + "/triples", `{"triples":[{"subject":"a","predicate":"b","object":"c"}]}`},
		{http.MethodGet, "/v1/projects/" + pid + "/memory-health", ""},
		{http.MethodGet, "/v1/memories/search?mode=text&q=launch&project_id=" + url.QueryEscape(pid), ""},
	} {
		if code, body := call(t, ts, tc.method, tc.path, foreignAdmin, tc.body); code != http.StatusNotFound {
			t.Fatalf("foreign admin %s %s = %d %v, want 404", tc.method, tc.path, code, body)
		}
	}

	// DELETE has no team check of its own: the org filter is what keeps a
	// foreign admin from erasing the project.
	if code, body := call(t, ts, http.MethodDelete, "/v1/projects/"+pid, foreignAdmin, ""); code != http.StatusNotFound {
		t.Fatalf("foreign admin DELETE project = %d %v, want 404", code, body)
	}
	if _, err := st.GetActiveProjectByID(ctx, pid); err != nil {
		t.Fatalf("project must survive a foreign delete: %v", err)
	}

	_, ownerKey, _ := loginKey(t, ts, "owner@example.test")
	if code, body := call(t, ts, http.MethodGet, "/v1/projects/"+pid+"/memories", ownerKey, ""); code != http.StatusOK {
		t.Fatalf("own admin lists memories = %d %v, want 200", code, body)
	}
	if code, body := call(t, ts, http.MethodDelete, "/v1/projects/"+pid, ownerKey, ""); code != http.StatusNoContent {
		t.Fatalf("own admin DELETE project = %d %v, want 204", code, body)
	}
}

// TestAdminOfAnotherOrgCannotSetAccountProjects: PUT /v1/accounts/{id}/projects
// only reaches members of the caller's org. Otherwise a foreign admin could put
// any account on its own org's default team and plant projects in that
// account's project list.
func TestAdminOfAnotherOrgCannotSetAccountProjects(t *testing.T) {
	ts, st, ids := accountLifecycleServer(t, "owner@example.test")
	ctx := context.Background()
	victim := ids["owner@example.test"]

	foreignAdmin, foreignOrg, foreignAccount := registerForeignAdmin(t, ts)
	bait, err := st.CreateProject(ctx, foreignOrg, "Bait", "bait", "fedcba9876543210", "", "", foreignAccount, "other")
	if err != nil {
		t.Fatal(err)
	}

	body := `{"project_ids":["` + bait.ID + `"]}`
	if code, out := call(t, ts, http.MethodPut, "/v1/accounts/"+victim+"/projects", foreignAdmin, body); code != http.StatusNotFound {
		t.Fatalf("foreign admin PUT account projects = %d %v, want 404", code, out)
	}
	if ok, err := st.HasProjectAccess(ctx, foreignOrg, victim, bait.ID); err != nil || ok {
		t.Fatalf("victim reaches the bait project = %t, %v; want false", ok, err)
	}
	if member, err := st.IsOrgMember(ctx, foreignOrg, victim); err != nil || member {
		t.Fatalf("victim became a member of the foreign org = %t, %v", member, err)
	}

	// The org's own admin still manages its own accounts.
	_, ownerKey, _ := loginKey(t, ts, "owner@example.test")
	if code, out := call(t, ts, http.MethodPut, "/v1/accounts/"+victim+"/projects", ownerKey, `{"project_ids":[]}`); code != http.StatusNoContent {
		t.Fatalf("own admin PUT account projects = %d %v, want 204", code, out)
	}
}

// TestMemoryWritesUseTheProjectGate: POST /v1/memories and POST /v1/sync name
// a project in their body. Another org's project answers 404 there too (no
// existence oracle), and inside the org a non-admin key needs team access to
// write, as on every other project route.
func TestMemoryWritesUseTheProjectGate(t *testing.T) {
	ts, st, ids := accountLifecycleServer(t, "owner@example.test", "member@example.test")
	ctx := context.Background()
	orgID, _, err := st.GetAccountOrgMembership(ctx, ids["owner@example.test"])
	if err != nil {
		t.Fatal(err)
	}
	project, err := st.CreateProject(ctx, orgID, "Team only", "team-only", "0123456789abcdef", "", "", ids["owner@example.test"], "other")
	if err != nil {
		t.Fatal(err)
	}
	pid := project.ID
	memory := `{"project_id":"` + pid + `","category":"decision","content":"Releases are cut from the main branch after the matrix passes."}`

	_, ownerKey, _ := loginKey(t, ts, "owner@example.test")
	code, minted := call(t, ts, http.MethodPost, "/v1/api-keys", ownerKey,
		`{"name":"laptop","scope":"sync","account_id":"`+ids["member@example.test"]+`"}`)
	memberSync, _ := minted["key"].(string)
	if code != http.StatusCreated || memberSync == "" {
		t.Fatalf("mint member sync key = %d %v", code, minted)
	}
	if code, body := call(t, ts, http.MethodPost, "/v1/memories", memberSync, memory); code != http.StatusForbidden {
		t.Fatalf("sync key without team access writes a memory = %d %v, want 403", code, body)
	}
	// A claim naming the existing project takes the same gate.
	claim := `{"project_claim":{"name":"Team only","remote_key":"0123456789abcdef"},"category":"decision","content":"Claims reach an existing project only with team access to it."}`
	if code, body := call(t, ts, http.MethodPost, "/v1/memories", memberSync, claim); code != http.StatusForbidden {
		t.Fatalf("sync key without team access claims the project = %d %v, want 403", code, body)
	}

	foreignAdmin, _, _ := registerForeignAdmin(t, ts)
	if code, body := call(t, ts, http.MethodPost, "/v1/memories", foreignAdmin, memory); code != http.StatusNotFound {
		t.Fatalf("foreign admin writes a memory = %d %v, want 404", code, body)
	}
	if code, body := call(t, ts, http.MethodPost, "/v1/sync", foreignAdmin, `{"project_id":"`+pid+`","client_id":"c1"}`); code != http.StatusNotFound {
		t.Fatalf("foreign admin syncs the project = %d %v, want 404", code, body)
	}

	if err := st.EnsureCreatorProjectAccess(ctx, orgID, ids["member@example.test"], pid); err != nil {
		t.Fatal(err)
	}
	if code, body := call(t, ts, http.MethodPost, "/v1/memories", memberSync, memory); code != http.StatusCreated {
		t.Fatalf("sync key with team access writes a memory = %d %v, want 201", code, body)
	}
	if code, body := call(t, ts, http.MethodPost, "/v1/memories", memberSync, claim); code != http.StatusCreated {
		t.Fatalf("sync key with team access claims the project = %d %v, want 201", code, body)
	}
}

// TestTeamRoutesStayInsideTheOrg: team detail and team membership routes take
// a team id from the path. Another org's team answers 404 and is never read or
// changed, and a team only takes members of its own org.
func TestTeamRoutesStayInsideTheOrg(t *testing.T) {
	ts, st, ids := accountLifecycleServer(t, "owner@example.test", "member@example.test")
	ctx := context.Background()
	orgID, _, err := st.GetAccountOrgMembership(ctx, ids["owner@example.test"])
	if err != nil {
		t.Fatal(err)
	}
	team, err := st.CreateTeam(ctx, orgID, "Core", "core")
	if err != nil {
		t.Fatal(err)
	}
	member := ids["member@example.test"]
	if err := st.AddTeamMember(ctx, orgID, team.ID, member); err != nil {
		t.Fatal(err)
	}

	foreignAdmin, _, foreignAccount := registerForeignAdmin(t, ts)

	for _, tc := range []struct{ method, path, body string }{
		{http.MethodGet, "/v1/teams/" + team.ID, ""},
		{http.MethodPost, "/v1/teams/" + team.ID + "/members", `{"account_id":"` + foreignAccount + `"}`},
		{http.MethodDelete, "/v1/teams/" + team.ID + "/members/" + member, ""},
	} {
		if code, body := call(t, ts, tc.method, tc.path, foreignAdmin, tc.body); code != http.StatusNotFound {
			t.Fatalf("foreign admin %s %s = %d %v, want 404", tc.method, tc.path, code, body)
		}
	}
	detail, err := st.GetTeamDetail(ctx, orgID, team.ID)
	if err != nil || len(detail.Members) != 1 || detail.Members[0].AccountID != member {
		t.Fatalf("team after foreign calls = %+v, %v; want only the member", detail, err)
	}

	_, ownerKey, _ := loginKey(t, ts, "owner@example.test")
	if code, body := call(t, ts, http.MethodPost, "/v1/teams/"+team.ID+"/members", ownerKey, `{"account_id":"`+foreignAccount+`"}`); code != http.StatusNotFound {
		t.Fatalf("own admin adds an account of another org = %d %v, want 404", code, body)
	}
	if code, body := call(t, ts, http.MethodGet, "/v1/teams/"+team.ID, ownerKey, ""); code != http.StatusOK {
		t.Fatalf("own admin reads its team = %d %v, want 200", code, body)
	}
	if code, body := call(t, ts, http.MethodDelete, "/v1/teams/"+team.ID+"/members/"+member, ownerKey, ""); code != http.StatusNoContent {
		t.Fatalf("own admin removes its member = %d %v, want 204", code, body)
	}
}

// TestSyncRefusesADeletedProject: a deleted project is PROJECT_NOT_FOUND on
// sync for every scope, admin included, as on the HTTP project routes.
func TestSyncRefusesADeletedProject(t *testing.T) {
	ts, st, ids := accountLifecycleServer(t, "owner@example.test")
	ctx := context.Background()
	orgID, _, err := st.GetAccountOrgMembership(ctx, ids["owner@example.test"])
	if err != nil {
		t.Fatal(err)
	}
	project, err := st.CreateProject(ctx, orgID, "Gone", "gone", "0123456789abcdef", "", "", ids["owner@example.test"], "other")
	if err != nil {
		t.Fatal(err)
	}
	_, ownerKey, _ := loginKey(t, ts, "owner@example.test")
	syncBody := `{"project_id":"` + project.ID + `","client_id":"c1"}`
	if code, body := call(t, ts, http.MethodPost, "/v1/sync", ownerKey, syncBody); code != http.StatusOK {
		t.Fatalf("admin sync on a live project = %d %v, want 200", code, body)
	}
	if err := st.SoftDeleteProject(ctx, orgID, project.ID); err != nil {
		t.Fatal(err)
	}
	if code, body := call(t, ts, http.MethodPost, "/v1/sync", ownerKey, syncBody); code != http.StatusNotFound {
		t.Fatalf("admin sync on a deleted project = %d %v, want 404", code, body)
	}
}

// TestInviteRevokeStaysInsideTheOrg: DELETE /v1/invites/{id} only removes the
// caller's org's invites; another org's answers 404 and stays valid.
func TestInviteRevokeStaysInsideTheOrg(t *testing.T) {
	ts, st, ids := accountLifecycleServer(t, "owner@example.test")
	ctx := context.Background()
	orgID, _, err := st.GetAccountOrgMembership(ctx, ids["owner@example.test"])
	if err != nil {
		t.Fatal(err)
	}
	invite, err := st.CreateInvite(ctx, orgID, "new@example.test", "New", "member", "invite-token-hash", time.Now().UTC().Add(24*time.Hour), ids["owner@example.test"])
	if err != nil {
		t.Fatal(err)
	}

	foreignAdmin, _, _ := registerForeignAdmin(t, ts)
	if code, body := call(t, ts, http.MethodDelete, "/v1/invites/"+invite.ID, foreignAdmin, ""); code != http.StatusNotFound {
		t.Fatalf("foreign admin revokes the invite = %d %v, want 404", code, body)
	}
	invites, err := st.ListInvitesByOrg(ctx, orgID)
	if err != nil || len(invites) != 1 {
		t.Fatalf("invites after a foreign revoke = %d, %v; want 1", len(invites), err)
	}

	_, ownerKey, _ := loginKey(t, ts, "owner@example.test")
	if code, body := call(t, ts, http.MethodDelete, "/v1/invites/"+invite.ID, ownerKey, ""); code != http.StatusNoContent {
		t.Fatalf("own admin revokes the invite = %d %v, want 204", code, body)
	}
}

// TestClaimCreateDoesNotWidenTeamAccess (attack h1, run 20261122T194933Z): a
// sync key kept off the default team must not reach that team's projects by
// creating a fresh project through project_claim on POST /v1/memories.
func TestClaimCreateDoesNotWidenTeamAccess(t *testing.T) {
	ts, st, ids := accountLifecycleServer(t, "owner@example.test", "member@example.test")
	ctx := context.Background()
	owner, member := ids["owner@example.test"], ids["member@example.test"]
	orgID, _, err := st.GetAccountOrgMembership(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	restricted, err := st.CreateProject(ctx, orgID, "Restricted", "restricted", "0123456789abcdef", "", "", owner, "other")
	if err != nil {
		t.Fatal(err)
	}
	// As POST /v1/projects does: the project is granted to the default team.
	if err := st.EnsureCreatorProjectAccess(ctx, orgID, owner, restricted.ID); err != nil {
		t.Fatal(err)
	}

	_, ownerKey, _ := loginKey(t, ts, "owner@example.test")
	code, minted := call(t, ts, http.MethodPost, "/v1/api-keys", ownerKey,
		`{"name":"laptop","scope":"sync","account_id":"`+member+`"}`)
	memberSync, _ := minted["key"].(string)
	if code != http.StatusCreated || memberSync == "" {
		t.Fatalf("mint member sync key = %d %v", code, minted)
	}
	path := "/v1/projects/" + restricted.ID + "/memories"
	if code, body := call(t, ts, http.MethodGet, path, memberSync, ""); code != http.StatusForbidden {
		t.Fatalf("precondition: member off the default team lists memories = %d %v, want 403", code, body)
	}

	claim := `{"project_claim":{"name":"Side project","remote_key":"fedcba9876543210"},"category":"fact","content":"The side project cuts releases from the main branch after the matrix passes."}`
	code, newBody := call(t, ts, http.MethodPost, "/v1/memories", memberSync, claim)
	if code != http.StatusForbidden {
		t.Fatalf("member off the default team creates a project by claim = %d %v, want 403", code, newBody)
	}
	// Claiming the existing project answers the very same 403, so remote
	// keys cannot be probed.
	existing := `{"project_claim":{"name":"Restricted","remote_key":"0123456789abcdef"},"category":"fact","content":"Probing whether this repository already has a project."}`
	if code, body := call(t, ts, http.MethodPost, "/v1/memories", memberSync, existing); code != http.StatusForbidden || body["error"] != newBody["error"] {
		t.Fatalf("claim of an existing project = %d %v, want the same 403 as %v", code, body, newBody)
	}
	if code, body := call(t, ts, http.MethodGet, path, memberSync, ""); code != http.StatusForbidden {
		t.Fatalf("member reaches a default-team project after a claim = %d %v, want 403", code, body)
	}

	// On the default team, the claim still creates the project (released
	// clients rely on it).
	if err := st.EnsureDefaultTeamMembership(ctx, orgID, member); err != nil {
		t.Fatal(err)
	}
	if code, body := call(t, ts, http.MethodPost, "/v1/memories", memberSync, claim); code != http.StatusCreated {
		t.Fatalf("default-team member creates a project by claim = %d %v, want 201", code, body)
	}
}

// TestTeamDetailIsAdminOnly (attack h2): GET /v1/teams/{id} lists members'
// emails and every project the team grants, so a non-admin key gets 403, as
// on GET /v1/accounts.
func TestTeamDetailIsAdminOnly(t *testing.T) {
	ts, st, ids := accountLifecycleServer(t, "owner@example.test", "member@example.test")
	ctx := context.Background()
	owner, member := ids["owner@example.test"], ids["member@example.test"]
	orgID, _, err := st.GetAccountOrgMembership(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureDefaultTeamMembership(ctx, orgID, owner); err != nil {
		t.Fatal(err)
	}
	teams, err := st.ListTeamsByOrg(ctx, orgID)
	if err != nil || len(teams) != 1 {
		t.Fatalf("teams = %d, %v; want the default team only", len(teams), err)
	}

	_, ownerKey, _ := loginKey(t, ts, "owner@example.test")
	code, minted := call(t, ts, http.MethodPost, "/v1/api-keys", ownerKey,
		`{"name":"reader","scope":"readonly","account_id":"`+member+`"}`)
	memberKey, _ := minted["key"].(string)
	if code != http.StatusCreated || memberKey == "" {
		t.Fatalf("mint member readonly key = %d %v", code, minted)
	}
	if code, body := call(t, ts, http.MethodGet, "/v1/teams/"+teams[0].ID, memberKey, ""); code != http.StatusForbidden {
		t.Fatalf("readonly key reads team detail = %d %v, want 403", code, body)
	}
	if code, body := call(t, ts, http.MethodGet, "/v1/teams/"+teams[0].ID, ownerKey, ""); code != http.StatusOK {
		t.Fatalf("admin reads team detail = %d %v, want 200", code, body)
	}
}

// TestStaleCrossOrgMemberIsNotListed (attack h3): the row the old
// PUT /v1/accounts/{id}/projects could write (an account of org A on a team of
// org B) is neither written any more nor shown to org B's admin.
func TestStaleCrossOrgMemberIsNotListed(t *testing.T) {
	ts, st, ids := accountLifecycleServer(t, "owner@example.test")
	ctx := context.Background()
	victim := ids["owner@example.test"]
	foreignAdmin, foreignOrg, _ := registerForeignAdmin(t, ts)

	if err := st.EnsureDefaultTeamMembership(ctx, foreignOrg, victim); err != nil {
		t.Fatal(err)
	}
	teams, err := st.ListTeamsByOrg(ctx, foreignOrg)
	if err != nil || len(teams) == 0 {
		t.Fatalf("foreign teams = %d, %v", len(teams), err)
	}
	for _, team := range teams {
		code, body := call(t, ts, http.MethodGet, "/v1/teams/"+team.ID, foreignAdmin, "")
		if code != http.StatusOK {
			t.Fatalf("foreign admin reads its own team = %d %v, want 200", code, body)
		}
		members, _ := body["members"].([]any)
		for _, m := range members {
			if mm, _ := m.(map[string]any); mm["account_id"] == victim {
				t.Fatalf("foreign org's team lists an account of another org: %v", mm)
			}
		}
	}
}
