//go:build integration

package store

import (
	"context"
	"errors"
	"strconv"
	"testing"
)

// Postgres twins of the SQLite onboarding tests, each on a database of its own
// since a first-run bootstrap needs zero organizations.

func TestBootstrapOnboardingCreatesAllResources_Postgres(t *testing.T) {
	checkBootstrapOnboardingCreatesAllResources(t, freshPostgresStore(t))
}

func TestBootstrapOnboardingRollsBackInjectedProjectFailure_Postgres(t *testing.T) {
	st := freshPostgresStore(t)
	if _, err := st.db.Exec(`CREATE FUNCTION fail_onboarding_project() RETURNS trigger
		LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected project failure'; END $$`); err != nil {
		t.Fatalf("create failure function: %v", err)
	}
	if _, err := st.db.Exec(`CREATE TRIGGER fail_onboarding_project BEFORE INSERT ON projects
		FOR EACH ROW EXECUTE FUNCTION fail_onboarding_project()`); err != nil {
		t.Fatalf("create failure trigger: %v", err)
	}
	checkBootstrapOnboardingRollsBack(t, st)
}

func TestBootstrapOnboardingRefusesASecondRun_Postgres(t *testing.T) {
	checkBootstrapOnboardingRefusesASecondRun(t, freshPostgresStore(t))
}

// TestBootstrapOnboardingSerializesRacingFirstRuns_Postgres: racing first runs, with
// distinct orgs and admins so no unique index decides, end with exactly one
// organization: the in-transaction count under the advisory lock answers
// ErrConflict to every other one.
func TestBootstrapOnboardingSerializesRacingFirstRuns_Postgres(t *testing.T) {
	st := freshPostgresStore(t)
	const racers = 8
	errs := make(chan error, racers)
	start := make(chan struct{})
	for i := 0; i < racers; i++ {
		in := onboardingBootstrapInput()
		suffix := strconv.Itoa(i)
		in.OrgSlug, in.AdminEmail, in.APIKeyHash = "acme-"+suffix, "admin"+suffix+"@acme.test", "key-hash-"+suffix
		for j := range in.Projects {
			in.Projects[j].Slug += "-" + suffix
			in.Projects[j].RemoteKey += "-" + suffix
		}
		go func() {
			<-start
			_, err := st.BootstrapOnboarding(context.Background(), in)
			errs <- err
		}()
	}
	close(start)
	won := 0
	for i := 0; i < racers; i++ {
		switch err := <-errs; {
		case err == nil:
			won++
		case !errors.Is(err, ErrConflict):
			t.Errorf("racing bootstrap = %v, want nil or ErrConflict", err)
		}
	}
	if won != 1 {
		t.Fatalf("%d racing bootstraps succeeded, want exactly 1", won)
	}
	if n := onboardingTableCount(t, st, "organizations"); n != 1 {
		t.Fatalf("organizations after the race = %d, want 1", n)
	}
}
