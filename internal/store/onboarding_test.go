package store

import (
	"context"
	"testing"
)

func onboardingBootstrapInput() OnboardingBootstrapInput {
	return OnboardingBootstrapInput{
		OrgName:           "Acme",
		OrgSlug:           "acme",
		AdminEmail:        "admin@acme.test",
		AdminDisplayName:  "Admin",
		AdminPasswordHash: "bcrypt-hash",
		APIKeyName:        "default",
		APIKeyPrefix:      "anc_live_test",
		APIKeyHash:        "key-hash",
		APIKeyScope:       "admin",
		Projects: []OnboardingProject{
			{Name: "API", Slug: "api", Category: "service", RepoURL: "https://github.com/acme/api.git", RemoteKey: "api-key", RemoteKeyV1: "api-key-v1"},
			{Name: "Web", Slug: "web", Category: "service", RepoURL: "git@github.com:acme/web.git", RemoteKey: "web-key", RemoteKeyV1: "web-key-v1"},
		},
	}
}

func onboardingTableCount(t *testing.T, st *SQLiteStore, table string) int {
	t.Helper()
	var count int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return count
}

func TestSQLiteBootstrapOnboardingCreatesAllResources(t *testing.T) {
	st := newSQLiteTestStore(t)
	result, err := st.BootstrapOnboarding(context.Background(), onboardingBootstrapInput())
	if err != nil {
		t.Fatalf("bootstrap onboarding: %v", err)
	}
	if result.Organization == nil || result.Admin == nil || result.APIKey == nil || len(result.Projects) != 2 {
		t.Fatalf("incomplete bootstrap result: %#v", result)
	}
	for table, want := range map[string]int{
		"organizations": 1,
		"accounts":      1,
		"org_members":   1,
		"teams":         1,
		"team_members":  1,
		"api_keys":      1,
		"projects":      2,
	} {
		if got := onboardingTableCount(t, st, table); got != want {
			t.Errorf("%s rows = %d, want %d", table, got, want)
		}
	}
	for _, project := range result.Projects {
		ok, err := st.HasProjectAccess(context.Background(), result.Admin.ID, project.ID)
		if err != nil || !ok {
			t.Errorf("admin access to %s = %t, %v", project.ID, ok, err)
		}
	}
}

func TestSQLiteBootstrapOnboardingRollsBackInjectedProjectFailure(t *testing.T) {
	st := newSQLiteTestStore(t)
	if _, err := st.db.Exec(`CREATE TRIGGER fail_onboarding_project
		BEFORE INSERT ON projects BEGIN SELECT RAISE(ABORT, 'injected project failure'); END`); err != nil {
		t.Fatalf("create failure trigger: %v", err)
	}

	if _, err := st.BootstrapOnboarding(context.Background(), onboardingBootstrapInput()); err == nil {
		t.Fatal("bootstrap unexpectedly succeeded")
	}
	for _, table := range []string{
		"organizations", "accounts", "org_members", "teams", "team_members", "api_keys", "projects", "team_project_access", "org_guardrails",
	} {
		if got := onboardingTableCount(t, st, table); got != 0 {
			t.Errorf("%s rows after rollback = %d, want 0", table, got)
		}
	}
}
