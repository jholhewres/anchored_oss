package sync

import (
	"context"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/jholhewres/anchored_oss/internal/model"
	"github.com/jholhewres/anchored_oss/internal/store"
)

// TestHandlePushes_RejectionStatsIncrement verifies that every rejected push
// bumps the per-day (org, project, rule) counter feeding the memory health
// view, and that accepted pushes do not.
func TestHandlePushes_RejectionStatsIncrement(t *testing.T) {
	st, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "rs.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	ctx := context.Background()

	org, err := st.CreateOrganization(ctx, "Acme", "acme") // seeds default guardrails
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	acct, err := st.CreateAccount(ctx, "dev@example.com", "Dev", "x")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	proj, err := st.CreateProject(ctx, org.ID, "Repo", "repo", "k1", "k1", "https://github.example.com/org/repo.git", acct.ID, "service")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	eng := NewSyncEngine(st, slog.Default())
	now := time.Now().UTC()
	push := func() []model.SyncResult {
		res, err := eng.handlePushes(ctx, acct.ID, org.ID, proj.ID, []model.SyncMemory{
			{ID: "", Category: "event", Content: "deployed v2 today", CreatedAt: now, UpdatedAt: now},
			{ID: "", Category: "decision", Content: "we standardized on Postgres", CreatedAt: now, UpdatedAt: now},
		})
		if err != nil {
			t.Fatalf("handlePushes: %v", err)
		}
		return res
	}

	res := push()
	if res[0].Status != "rejected" || res[1].Status != "accepted" {
		t.Fatalf("unexpected results: %+v", res)
	}

	day := now.Format("2006-01-02")
	stats, err := st.ListRejectionStats(ctx, org.ID, proj.ID, day)
	if err != nil {
		t.Fatalf("list stats: %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("want exactly 1 stat row (only the rejected rule), got %d: %+v", len(stats), stats)
	}
	if stats[0].Count != 1 || stats[0].Day != day || stats[0].ProjectID != proj.ID {
		t.Fatalf("unexpected stat row: %+v", stats[0])
	}

	// A repeated rejection on the same day grows the same counter row.
	push()
	stats, err = st.ListRejectionStats(ctx, org.ID, proj.ID, day)
	if err != nil {
		t.Fatalf("list stats after second push: %v", err)
	}
	if len(stats) != 1 || stats[0].Count != 2 {
		t.Fatalf("want single row with count 2, got %+v", stats)
	}

	// Org-wide listing (empty projectID) sees the same row.
	orgStats, err := st.ListRejectionStats(ctx, org.ID, "", day)
	if err != nil {
		t.Fatalf("org-wide list: %v", err)
	}
	if len(orgStats) != 1 || orgStats[0].Count != 2 {
		t.Fatalf("org-wide want single row count 2, got %+v", orgStats)
	}
}
