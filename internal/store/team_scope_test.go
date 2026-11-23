package store

import (
	"context"
	"errors"
	"testing"
)

// TestTeams_ScopedToOrg: team detail and membership changes only act on teams
// of the org given. Its Postgres twin is TestTeams_ScopedToOrg_Postgres.
func TestTeams_ScopedToOrg(t *testing.T) {
	checkTeamsScopedToOrg(t, newSQLiteTestStore(t), "")
}

// checkTeamsScopedToOrg runs the team org-scope assertions on any backend. u
// keeps names unique on a shared database.
func checkTeamsScopedToOrg(t *testing.T, st Store, u string) {
	t.Helper()
	ctx := context.Background()
	orgA, err := st.CreateOrganization(ctx, "Acme", "acme-teams"+u)
	if err != nil {
		t.Fatalf("create org a: %v", err)
	}
	orgB, err := st.CreateOrganization(ctx, "Other", "other-teams"+u)
	if err != nil {
		t.Fatalf("create org b: %v", err)
	}
	member, err := st.CreateAccount(ctx, "member"+u+"@acme.test", "Member", "hash")
	if err != nil {
		t.Fatalf("create member: %v", err)
	}
	if err := st.AddOrgMember(ctx, orgA.ID, member.ID, "member"); err != nil {
		t.Fatalf("add org member: %v", err)
	}
	stranger, err := st.CreateAccount(ctx, "stranger"+u+"@other.test", "Stranger", "hash")
	if err != nil {
		t.Fatalf("create stranger: %v", err)
	}
	if err := st.AddOrgMember(ctx, orgB.ID, stranger.ID, "admin"); err != nil {
		t.Fatalf("add stranger to org b: %v", err)
	}
	team, err := st.CreateTeam(ctx, orgA.ID, "Core", "core"+u)
	if err != nil {
		t.Fatalf("create team: %v", err)
	}

	if err := st.AddTeamMember(ctx, orgB.ID, team.ID, member.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("AddTeamMember naming org b = %v, want ErrNotFound", err)
	}
	if err := st.AddTeamMember(ctx, orgA.ID, team.ID, stranger.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("AddTeamMember of an account outside org a = %v, want ErrNotFound", err)
	}
	if err := st.AddTeamMember(ctx, orgA.ID, team.ID, member.ID); err != nil {
		t.Fatalf("AddTeamMember in its own org: %v", err)
	}
	if err := st.AddTeamMember(ctx, orgA.ID, team.ID, member.ID); err != nil {
		t.Fatalf("AddTeamMember again must stay idempotent: %v", err)
	}

	if _, err := st.GetTeamDetail(ctx, orgB.ID, team.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetTeamDetail from org b = %v, want ErrNotFound", err)
	}
	detail, err := st.GetTeamDetail(ctx, orgA.ID, team.ID)
	if err != nil || len(detail.Members) != 1 {
		t.Fatalf("GetTeamDetail from org a = %v, %v; want one member", detail, err)
	}

	// The default-team joins refuse an account of another org, whoever calls.
	project, err := st.CreateProject(ctx, orgA.ID, "Teams", "teams"+u, "key-teams"+u, "", "", member.ID, "other")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if err := st.EnsureCreatorProjectAccess(ctx, orgA.ID, stranger.ID, project.ID); err != nil {
		t.Fatalf("EnsureCreatorProjectAccess for a stranger: %v", err)
	}
	if err := st.EnsureDefaultTeamMembership(ctx, orgA.ID, stranger.ID); err != nil {
		t.Fatalf("EnsureDefaultTeamMembership for a stranger: %v", err)
	}
	if on, err := st.IsDefaultTeamMember(ctx, orgA.ID, stranger.ID); err != nil || on {
		t.Fatalf("stranger on org a's default team = %t, %v; want false", on, err)
	}
	if err := st.EnsureDefaultTeamMembership(ctx, orgA.ID, member.ID); err != nil {
		t.Fatalf("EnsureDefaultTeamMembership for a member: %v", err)
	}
	if on, err := st.IsDefaultTeamMember(ctx, orgA.ID, member.ID); err != nil || !on {
		t.Fatalf("member on org a's default team = %t, %v; want true", on, err)
	}
	if err := st.SetAccountProjects(ctx, orgA.ID, stranger.ID, []string{project.ID}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("SetAccountProjects for an account of another org = %v, want ErrNotFound", err)
	}
	if on, err := st.IsDefaultTeamMember(ctx, orgA.ID, stranger.ID); err != nil || on {
		t.Fatalf("stranger joined org a's default team through SetAccountProjects = %t, %v", on, err)
	}
	// A grant without a join: the default team holds the project, nobody new
	// joins it.
	solo, err := st.CreateProject(ctx, orgA.ID, "Solo", "solo"+u, "key-solo"+u, "", "", member.ID, "other")
	if err != nil {
		t.Fatalf("create solo project: %v", err)
	}
	if err := st.GrantProjectToDefaultTeam(ctx, orgA.ID, solo.ID); err != nil {
		t.Fatalf("GrantProjectToDefaultTeam: %v", err)
	}
	if ok, err := st.HasProjectAccess(ctx, orgA.ID, member.ID, solo.ID); err != nil || !ok {
		t.Fatalf("default-team member reaches the granted project = %t, %v; want true", ok, err)
	}
	if err := st.GrantProjectToDefaultTeam(ctx, orgB.ID, solo.ID); err != nil {
		t.Fatalf("GrantProjectToDefaultTeam across orgs: %v", err)
	}
	if teams, err := st.ListTeamsByOrg(ctx, orgB.ID); err == nil {
		for _, tm := range teams {
			if d, err := st.GetTeamDetail(ctx, orgB.ID, tm.ID); err == nil && len(d.ProjectGrants) != 0 {
				t.Fatalf("org b's team got a grant on org a's project: %+v", d.ProjectGrants)
			}
		}
	}

	// A row planted before v0.7 (an account of another org on a team of org
	// A) is never listed.
	plantTeamMember(t, st, team.ID, stranger.ID)
	detail, err = st.GetTeamDetail(ctx, orgA.ID, team.ID)
	if err != nil {
		t.Fatalf("team detail with a stray row: %v", err)
	}
	for _, m := range detail.Members {
		if m.AccountID == stranger.ID {
			t.Fatalf("team lists an account of another org: %+v", m)
		}
	}

	if err := st.RemoveTeamMember(ctx, orgB.ID, team.ID, member.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("RemoveTeamMember naming org b = %v, want ErrNotFound", err)
	}
	if detail, err := st.GetTeamDetail(ctx, orgA.ID, team.ID); err != nil || len(detail.Members) != 1 {
		t.Fatalf("member must survive a foreign removal: %v, %v", detail, err)
	}
	if err := st.RemoveTeamMember(ctx, orgA.ID, team.ID, member.ID); err != nil {
		t.Fatalf("RemoveTeamMember in its own org: %v", err)
	}
}

// plantTeamMember writes a team_members row directly, the way the pre-v0.7
// cross-org holes could, which no store method allows any more.
func plantTeamMember(t *testing.T, st Store, teamID, accountID string) {
	t.Helper()
	query := `INSERT INTO team_members (team_id, account_id) VALUES (?, ?)`
	if isPostgres(st) {
		query = `INSERT INTO team_members (team_id, account_id) VALUES ($1, $2)`
	}
	if _, err := rawDB(t, st).Exec(query, teamID, accountID); err != nil {
		t.Fatalf("plant team member: %v", err)
	}
}
