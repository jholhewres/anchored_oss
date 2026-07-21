package store

import (
	"context"
	"testing"
	"time"

	"github.com/jholhewres/anchored_oss/internal/model"
)

// seedLeaseTestMemory creates a project + one queued memory and returns its ID.
func seedLeaseTestMemory(t *testing.T, st *SQLiteStore) string {
	t.Helper()
	ctx := context.Background()
	orgID, actorID := projectTestOrg(t, st)
	project, err := st.CreateProject(ctx, orgID, "Lease", "lease", "lease", "", "", actorID, "other")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	now := time.Date(2026, time.July, 20, 12, 0, 0, 0, time.UTC)
	mem := &model.Memory{
		ID:          "lease-mem",
		ProjectID:   project.ID,
		Category:    "fact",
		Content:     "leased content",
		ContentHash: "sha256:lease",
		AuthorID:    actorID,
		AuthorName:  "Test",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := st.UpsertMemory(ctx, mem); err != nil {
		t.Fatalf("insert memory: %v", err)
	}
	return mem.ID
}

func leaseOwner(t *testing.T, st *SQLiteStore, memoryID string) (string, bool) {
	t.Helper()
	var owner *string
	if err := st.db.QueryRow(`SELECT owner_id FROM curation_queue WHERE memory_id = ?`, memoryID).Scan(&owner); err != nil {
		t.Fatalf("read owner: %v", err)
	}
	if owner == nil {
		return "", false
	}
	return *owner, true
}

func TestSQLiteLeasedClaimStampsOwnerAndCompletes(t *testing.T) {
	st := newSQLiteTestStore(t)
	ctx := context.Background()
	memID := seedLeaseTestMemory(t, st)

	claimed, err := st.ClaimCurationBatchLeased(ctx, 10, "owner-A", time.Minute)
	if err != nil || len(claimed) != 1 || claimed[0] != memID {
		t.Fatalf("leased claim = %v, err=%v", claimed, err)
	}
	assertSQLiteCurationStatus(t, st, memID, "processing")
	if owner, ok := leaseOwner(t, st, memID); !ok || owner != "owner-A" {
		t.Fatalf("owner = %q ok=%v, want owner-A", owner, ok)
	}

	// A different owner must not be able to complete a row it does not hold.
	if err := st.SetCurationDoneLeased(ctx, memID, "owner-B"); err != nil {
		t.Fatalf("done by wrong owner: %v", err)
	}
	assertSQLiteCurationStatus(t, st, memID, "processing")

	// The holding owner completes it and the lease is cleared.
	if err := st.SetCurationDoneLeased(ctx, memID, "owner-A"); err != nil {
		t.Fatalf("done by owner: %v", err)
	}
	assertSQLiteCurationStatus(t, st, memID, "done")
	if _, ok := leaseOwner(t, st, memID); ok {
		t.Fatalf("lease owner not cleared after completion")
	}
}

func TestSQLiteReclaimExpiredLeaseRequeues(t *testing.T) {
	st := newSQLiteTestStore(t)
	ctx := context.Background()
	memID := seedLeaseTestMemory(t, st)

	if _, err := st.ClaimCurationBatchLeased(ctx, 10, "owner-A", time.Minute); err != nil {
		t.Fatalf("leased claim: %v", err)
	}

	// A live lease is not reclaimed.
	if n, err := st.ReclaimExpiredCuration(ctx); err != nil || n != 0 {
		t.Fatalf("reclaim with live lease = %d, err=%v; want 0", n, err)
	}
	assertSQLiteCurationStatus(t, st, memID, "processing")

	// Force the lease into the past to simulate a crashed worker.
	if _, err := st.db.ExecContext(ctx,
		`UPDATE curation_queue SET lease_expires_at = datetime('now', '-1 hour') WHERE memory_id = ?`, memID,
	); err != nil {
		t.Fatalf("expire lease: %v", err)
	}

	n, err := st.ReclaimExpiredCuration(ctx)
	if err != nil || n != 1 {
		t.Fatalf("reclaim expired = %d, err=%v; want 1", n, err)
	}
	assertSQLiteCurationStatus(t, st, memID, "pending")
	if _, ok := leaseOwner(t, st, memID); ok {
		t.Fatalf("owner not cleared after reclaim")
	}

	// The reclaimed row is claimable again by a new owner.
	claimed, err := st.ClaimCurationBatchLeased(ctx, 10, "owner-B", time.Minute)
	if err != nil || len(claimed) != 1 || claimed[0] != memID {
		t.Fatalf("reclaim then claim = %v, err=%v", claimed, err)
	}

	// The stalled original owner can no longer complete the row it lost.
	if err := st.SetCurationDoneLeased(ctx, memID, "owner-A"); err != nil {
		t.Fatalf("stale owner completion: %v", err)
	}
	assertSQLiteCurationStatus(t, st, memID, "processing")
}
