package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jholhewres/anchored_oss/internal/model"
)

func TestPurgeAuditOlderThan_SQLite(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "audit.db")
	st, err := NewSQLiteStore(dsn)
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	defer st.Close()

	ctx := context.Background()
	org, err := st.CreateOrganization(ctx, "Acme", "acme")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	if err := st.AppendAudit(ctx, &model.AuditEntry{
		OrgID:  org.ID,
		Action: "test.event",
	}); err != nil {
		t.Fatalf("append audit: %v", err)
	}

	// Cutoff in the past: the just-created entry is newer, so nothing is purged.
	removed, err := st.PurgeAuditOlderThan(ctx, time.Now().UTC().Add(-time.Hour))
	if err != nil {
		t.Fatalf("purge (past cutoff): %v", err)
	}
	if removed != 0 {
		t.Fatalf("expected 0 removed with past cutoff, got %d", removed)
	}

	// Cutoff in the future: the entry is older than the cutoff and is purged.
	removed, err = st.PurgeAuditOlderThan(ctx, time.Now().UTC().Add(time.Hour))
	if err != nil {
		t.Fatalf("purge (future cutoff): %v", err)
	}
	if removed != 1 {
		t.Fatalf("expected 1 removed with future cutoff, got %d", removed)
	}

	entries, total, err := st.ListAuditEntries(ctx, org.ID, model.AuditFilters{})
	if err != nil {
		t.Fatalf("list audit: %v", err)
	}
	if total != 0 || len(entries) != 0 {
		t.Fatalf("expected empty audit log after purge, got total=%d len=%d", total, len(entries))
	}
}
