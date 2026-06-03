package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jholhewres/anchored_oss/internal/model"
)

// These tests pin the SQLite time-scanning contract. The modernc.org/sqlite
// driver returns DATETIME columns as strings, so scanning a timestamp column
// directly into a *time.Time fails with "unsupported Scan". Every read path
// that selects a timestamp must wrap the destination with scanTime/scanNullTime.
// Regression guard for the dashboard-stats, org-policy, and KG-triple paths,
// which were scanning time.Time directly and only failed at runtime on SQLite.

func newSQLiteTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	st, err := NewSQLiteStore(filepath.Join(t.TempDir(), "scan.db"))
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestSQLiteDashboardStats_ScansLastPush(t *testing.T) {
	st := newSQLiteTestStore(t)
	ctx := context.Background()

	org, err := st.CreateOrganization(ctx, "Acme", "acme")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	acct, err := st.CreateAccount(ctx, "u@acme.test", "U", "hash")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	proj, err := st.CreateProject(ctx, org.ID, "repo", "repo", "key-1", acct.ID, "")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	// A recent accepted push is what GetDashboardStats aggregates into
	// RecentPushes (MAX(created_at) AS last_push).
	if err := st.AppendAudit(ctx, &model.AuditEntry{
		OrgID:     org.ID,
		ProjectID: proj.ID,
		Action:    "sync.push.accepted",
	}); err != nil {
		t.Fatalf("append audit: %v", err)
	}

	stats, err := st.GetDashboardStats(ctx, org.ID)
	if err != nil {
		t.Fatalf("GetDashboardStats must not fail scanning last_push: %v", err)
	}
	if len(stats.RecentPushes) != 1 {
		t.Fatalf("expected 1 recent push, got %d", len(stats.RecentPushes))
	}
	if stats.RecentPushes[0].LastPush.IsZero() {
		t.Error("LastPush should be a parsed timestamp, not zero")
	}
}

func TestSQLiteOrgPolicy_ScansUpdatedAt(t *testing.T) {
	st := newSQLiteTestStore(t)
	ctx := context.Background()

	org, err := st.CreateOrganization(ctx, "Acme", "acme")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	if err := st.UpsertOrgPolicy(ctx, &model.OrgPolicy{
		OrgID:             org.ID,
		BlockedCategories: []string{"secret"},
		QualityThreshold:  0.3,
		NearDupThreshold:  0.9,
	}); err != nil {
		t.Fatalf("upsert policy: %v", err)
	}

	// After a row exists, GetOrgPolicy scans updated_at (a real timestamp),
	// which is the path that previously failed on SQLite.
	got, err := st.GetOrgPolicy(ctx, org.ID)
	if err != nil {
		t.Fatalf("GetOrgPolicy must not fail scanning updated_at: %v", err)
	}
	if got.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should be a parsed timestamp, not zero")
	}
	if len(got.BlockedCategories) != 1 || got.BlockedCategories[0] != "secret" {
		t.Errorf("blocked categories round-trip failed: %v", got.BlockedCategories)
	}
}

func TestSQLiteTeamDetail_ScansMemberAddedAt(t *testing.T) {
	st := newSQLiteTestStore(t)
	ctx := context.Background()

	org, err := st.CreateOrganization(ctx, "Acme", "acme")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	acct, err := st.CreateAccount(ctx, "u@acme.test", "Member U", "hash")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	team, err := st.CreateTeam(ctx, org.ID, "Core", "core")
	if err != nil {
		t.Fatalf("create team: %v", err)
	}
	if err := st.AddTeamMember(ctx, team.ID, acct.ID); err != nil {
		t.Fatalf("add team member: %v", err)
	}

	// GetTeamDetail scans tm.created_at into TeamMember.AddedAt; this is the
	// member-row path that List does not exercise.
	detail, err := st.GetTeamDetail(ctx, team.ID)
	if err != nil {
		t.Fatalf("GetTeamDetail must not fail scanning member added_at: %v", err)
	}
	if len(detail.Members) != 1 {
		t.Fatalf("expected 1 member, got %d", len(detail.Members))
	}
	if detail.Members[0].AddedAt.IsZero() {
		t.Error("member AddedAt should be a parsed timestamp, not zero")
	}
}

func TestSQLiteListTriples_ScansCreatedAt(t *testing.T) {
	st := newSQLiteTestStore(t)
	ctx := context.Background()

	org, err := st.CreateOrganization(ctx, "Acme", "acme")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	acct, err := st.CreateAccount(ctx, "u@acme.test", "U", "hash")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	proj, err := st.CreateProject(ctx, org.ID, "repo", "repo", "key-1", acct.ID, "")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if err := st.UpsertTriple(ctx, &model.Triple{
		Subject:    "anchored_oss",
		Predicate:  "depends_on",
		Object:     "Postgres",
		Confidence: 0.95,
		ProjectID:  proj.ID,
	}); err != nil {
		t.Fatalf("upsert triple: %v", err)
	}

	triples, total, err := st.ListTriplesByProject(ctx, proj.ID, 10, 0)
	if err != nil {
		t.Fatalf("ListTriplesByProject must not fail scanning created_at: %v", err)
	}
	if total != 1 || len(triples) != 1 {
		t.Fatalf("expected 1 triple, got total=%d len=%d", total, len(triples))
	}
	if triples[0].CreatedAt.IsZero() {
		t.Error("triple CreatedAt should be a parsed timestamp, not zero")
	}
	if triples[0].Predicate != "depends_on" {
		t.Errorf("predicate round-trip failed: %q", triples[0].Predicate)
	}
}
