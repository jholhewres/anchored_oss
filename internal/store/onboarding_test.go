package store

import (
	"context"
	"errors"
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

func onboardingTableCount(t *testing.T, st Store, table string) int {
	t.Helper()
	var count int
	if err := rawDB(t, st).QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return count
}

func TestSQLiteBootstrapOnboardingCreatesAllResources(t *testing.T) {
	checkBootstrapOnboardingCreatesAllResources(t, newSQLiteTestStore(t))
}

// checkBootstrapOnboardingCreatesAllResources needs a store with no
// organization yet (a first run).
func checkBootstrapOnboardingCreatesAllResources(t *testing.T, st Store) {
	t.Helper()
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
		ok, err := st.HasProjectAccess(context.Background(), result.Organization.ID, result.Admin.ID, project.ID)
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
	checkBootstrapOnboardingRollsBack(t, st)
}

// checkBootstrapOnboardingRollsBack expects a store whose project inserts fail
// (a trigger the caller installed) and no organization yet.
func checkBootstrapOnboardingRollsBack(t *testing.T, st Store) {
	t.Helper()

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

// TestSQLiteBootstrapOnboardingRefusesASecondRun: once an organization exists,
// the store itself answers ErrConflict, whatever the handler checked.
func TestSQLiteBootstrapOnboardingRefusesASecondRun(t *testing.T) {
	checkBootstrapOnboardingRefusesASecondRun(t, newSQLiteTestStore(t))
}

func checkBootstrapOnboardingRefusesASecondRun(t *testing.T, st Store) {
	t.Helper()
	if _, err := st.BootstrapOnboarding(context.Background(), onboardingBootstrapInput()); err != nil {
		t.Fatalf("first bootstrap: %v", err)
	}
	second := onboardingBootstrapInput()
	second.OrgSlug, second.AdminEmail, second.APIKeyHash = "acme-2", "admin2@acme.test", "key-hash-2"
	for i := range second.Projects {
		second.Projects[i].Slug += "-2"
		second.Projects[i].RemoteKey += "-2"
	}
	if _, err := st.BootstrapOnboarding(context.Background(), second); !errors.Is(err, ErrConflict) {
		t.Fatalf("second bootstrap = %v, want ErrConflict", err)
	}
	if n := onboardingTableCount(t, st, "organizations"); n != 1 {
		t.Fatalf("organizations after a second run = %d, want 1", n)
	}
}
