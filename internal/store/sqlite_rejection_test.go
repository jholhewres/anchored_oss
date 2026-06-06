package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

// TestMigration016_IdempotentReopen ensures a DB migrated to v16 reopens
// cleanly (the migration loop must not re-run or fail on existing tables).
func TestMigration016_IdempotentReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "m16.db")
	st, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	ctx := context.Background()
	if err := st.IncrementRejectionStat(ctx, "org1", "proj1", "blocked_category", 3); err != nil {
		t.Fatalf("increment: %v", err)
	}
	st.Close()

	st2, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("reopen migrated db: %v", err)
	}
	defer st2.Close()

	day := time.Now().UTC().Format("2006-01-02")
	stats, err := st2.ListRejectionStats(ctx, "org1", "proj1", day)
	if err != nil {
		t.Fatalf("list after reopen: %v", err)
	}
	if len(stats) != 1 || stats[0].Count != 3 {
		t.Fatalf("counter must survive reopen, got %+v", stats)
	}
}

// TestPurgeRejectionStatsOlderThan removes only rows older than the cutoff day.
func TestPurgeRejectionStatsOlderThan(t *testing.T) {
	st, err := NewSQLiteStore(filepath.Join(t.TempDir(), "purge.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()
	ctx := context.Background()

	// Today's counter via the public API; an old row inserted directly.
	if err := st.IncrementRejectionStat(ctx, "org1", "proj1", "secret_detection", 1); err != nil {
		t.Fatalf("increment: %v", err)
	}
	if _, err := st.db.ExecContext(ctx, `
		INSERT INTO sync_rejection_stats (org_id, project_id, rule, day, count)
		VALUES ('org1', 'proj1', 'secret_detection', '2020-01-01', 9)
	`); err != nil {
		t.Fatalf("seed old row: %v", err)
	}

	cutoff := time.Now().UTC().AddDate(0, 0, -90).Format("2006-01-02")
	n, err := st.PurgeRejectionStatsOlderThan(ctx, cutoff)
	if err != nil {
		t.Fatalf("purge: %v", err)
	}
	if n != 1 {
		t.Fatalf("want 1 purged row, got %d", n)
	}

	stats, err := st.ListRejectionStats(ctx, "org1", "", "2019-01-01")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("today's row must survive, got %+v", stats)
	}
}
