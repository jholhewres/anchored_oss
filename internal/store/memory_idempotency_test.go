package store

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jholhewres/anchored_oss/internal/model"
)

func TestSQLiteMemoryIdempotency_ReplayAndConflict(t *testing.T) {
	st := newSQLiteTestStore(t)
	orgID, actorID, projectID := seedIdempotencyProject(t, st)
	ctx := context.Background()
	first := idempotencyTestMemory("memory-first", projectID, actorID, "first")

	got, replayed, err := st.UpsertMemoryIdempotent(
		ctx, orgID, actorID, "operation-1", "sha256:payload-a", first,
	)
	if err != nil {
		t.Fatalf("initial write: %v", err)
	}
	if replayed || got.ID != first.ID {
		t.Fatalf("initial result = %+v, replayed=%v", got, replayed)
	}

	lookedUp, found, err := st.GetMemoryIdempotency(
		ctx, orgID, actorID, "operation-1", "sha256:payload-a",
	)
	if err != nil || !found || lookedUp.ID != first.ID {
		t.Fatalf("lookup = %+v, found=%v, err=%v", lookedUp, found, err)
	}

	retry := idempotencyTestMemory("memory-retry", projectID, actorID, "first")
	got, replayed, err = st.UpsertMemoryIdempotent(
		ctx, orgID, actorID, "operation-1", "sha256:payload-a", retry,
	)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if !replayed || got.ID != first.ID {
		t.Fatalf("replay result = %+v, replayed=%v; want original %s", got, replayed, first.ID)
	}

	_, _, err = st.UpsertMemoryIdempotent(
		ctx, orgID, actorID, "operation-1", "sha256:payload-b", retry,
	)
	if !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("conflicting replay error = %v, want ErrIdempotencyConflict", err)
	}
	_, found, err = st.GetMemoryIdempotency(
		ctx, orgID, actorID, "operation-1", "sha256:payload-b",
	)
	if found || !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("conflicting lookup found=%v err=%v, want conflict", found, err)
	}

	_, found, err = st.GetMemoryIdempotency(
		ctx, orgID, actorID, "operation-missing", "sha256:payload-a",
	)
	if err != nil || found {
		t.Fatalf("missing lookup found=%v err=%v", found, err)
	}

	_, total, err := st.ListMemoriesPaginated(ctx, projectID, 10, 0, "")
	if err != nil {
		t.Fatalf("list memories: %v", err)
	}
	if total != 1 {
		t.Fatalf("memory total = %d, want 1", total)
	}
}

func TestSQLiteMemoryIdempotency_ConcurrentReplay(t *testing.T) {
	st := newSQLiteTestStore(t)
	orgID, actorID, projectID := seedIdempotencyProject(t, st)
	ctx := context.Background()

	const callers = 16
	type result struct {
		memory   *model.Memory
		replayed bool
		err      error
	}
	results := make(chan result, callers)
	var start sync.WaitGroup
	start.Add(1)
	for i := 0; i < callers; i++ {
		go func(i int) {
			start.Wait()
			memory := idempotencyTestMemory(
				fmt.Sprintf("memory-%02d", i),
				projectID,
				actorID,
				"concurrent",
			)
			got, replayed, err := st.UpsertMemoryIdempotent(
				ctx,
				orgID,
				actorID,
				"operation-concurrent",
				"sha256:one-payload",
				memory,
			)
			results <- result{memory: got, replayed: replayed, err: err}
		}(i)
	}
	start.Done()

	var (
		originalID string
		writes     int
	)
	seenIDs := make(map[string]struct{})
	for i := 0; i < callers; i++ {
		result := <-results
		if result.err != nil {
			t.Fatalf("concurrent write: %v", result.err)
		}
		if !result.replayed {
			writes++
			originalID = result.memory.ID
		}
		if result.memory == nil {
			t.Fatal("concurrent result memory is nil")
		}
		seenIDs[result.memory.ID] = struct{}{}
	}
	if writes != 1 {
		t.Fatalf("non-replayed writes = %d, want 1", writes)
	}
	if len(seenIDs) != 1 {
		t.Fatalf("distinct response memory IDs = %v, want one original result", seenIDs)
	}

	_, total, err := st.ListMemoriesPaginated(ctx, projectID, 20, 0, "")
	if err != nil {
		t.Fatalf("list memories: %v", err)
	}
	if total != 1 {
		t.Fatalf("memory total = %d, want 1", total)
	}
	stored, err := st.GetMemoryByID(ctx, originalID)
	if err != nil {
		t.Fatalf("get original memory: %v", err)
	}
	if stored.ID != originalID {
		t.Fatalf("stored ID = %q, want %q", stored.ID, originalID)
	}
}

func TestSQLiteMemoryIdempotency_RollsBackReservationWithWrite(t *testing.T) {
	st := newSQLiteTestStore(t)
	orgID, actorID, projectID := seedIdempotencyProject(t, st)
	ctx := context.Background()

	invalid := idempotencyTestMemory("memory-invalid", "missing-project", actorID, "retryable")
	_, _, err := st.UpsertMemoryIdempotent(
		ctx, orgID, actorID, "operation-retry", "sha256:payload", invalid,
	)
	if err == nil {
		t.Fatal("invalid project write unexpectedly succeeded")
	}

	var reservations int
	if err := st.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM memory_write_idempotency
		 WHERE org_scope = ? AND actor_scope = ? AND operation_id = ?`,
		orgID, actorID, "operation-retry",
	).Scan(&reservations); err != nil {
		t.Fatalf("count reservations: %v", err)
	}
	if reservations != 0 {
		t.Fatalf("reservations after rollback = %d, want 0", reservations)
	}

	valid := idempotencyTestMemory("memory-valid", projectID, actorID, "retryable")
	got, replayed, err := st.UpsertMemoryIdempotent(
		ctx, orgID, actorID, "operation-retry", "sha256:payload", valid,
	)
	if err != nil {
		t.Fatalf("retry after rollback: %v", err)
	}
	if replayed || got.ID != valid.ID {
		t.Fatalf("retry result = %+v, replayed=%v", got, replayed)
	}
}

func TestSQLiteMemoryIdempotency_DeleteRedactsSnapshotAndKeepsTombstone(t *testing.T) {
	st := newSQLiteTestStore(t)
	orgID, actorID, projectID := seedIdempotencyProject(t, st)
	ctx := context.Background()
	const secret = "private content that must not survive deletion"
	memory := idempotencyTestMemory("memory-redacted", projectID, actorID, secret)

	if _, replayed, err := st.UpsertMemoryIdempotent(
		ctx,
		orgID,
		actorID,
		"operation-redacted",
		"sha256:redacted",
		memory,
	); err != nil || replayed {
		t.Fatalf("initial idempotent write replayed=%v err=%v", replayed, err)
	}
	if err := st.SoftDeleteMemory(ctx, memory.ID, projectID); err != nil {
		t.Fatalf("soft delete memory: %v", err)
	}

	var responseJSON string
	if err := st.db.QueryRowContext(ctx,
		`SELECT response_json
		 FROM memory_write_idempotency
		 WHERE org_scope = ? AND actor_scope = ? AND operation_id = ?`,
		orgID,
		actorID,
		"operation-redacted",
	).Scan(&responseJSON); err != nil {
		t.Fatalf("load redacted idempotency row: %v", err)
	}
	if strings.Contains(responseJSON, secret) {
		t.Fatalf("redacted idempotency snapshot leaked deleted content: %s", responseJSON)
	}
	if !strings.Contains(responseJSON, memory.ID) {
		t.Fatalf("redacted idempotency snapshot lost tombstone ID: %s", responseJSON)
	}

	replayed, found, err := st.GetMemoryIdempotency(
		ctx,
		orgID,
		actorID,
		"operation-redacted",
		"sha256:redacted",
	)
	if err != nil || !found || replayed == nil || replayed.ID != memory.ID {
		t.Fatalf("redacted replay = %+v, found=%v err=%v", replayed, found, err)
	}
}

func TestSQLiteOrgStorageCountsRetainedIdempotencySnapshots(t *testing.T) {
	st := newSQLiteTestStore(t)
	orgID, actorID, projectID := seedIdempotencyProject(t, st)
	ctx := context.Background()
	memory := idempotencyTestMemory(
		"memory-quota-ledger",
		projectID,
		actorID,
		"quota-accounted-content",
	)

	before, err := st.GetOrgStorageBytes(ctx, orgID)
	if err != nil {
		t.Fatalf("storage before idempotent write: %v", err)
	}
	if _, _, err := st.UpsertMemoryIdempotent(
		ctx,
		orgID,
		actorID,
		"operation-quota-ledger",
		"sha256:quota-ledger",
		memory,
	); err != nil {
		t.Fatalf("idempotent write: %v", err)
	}
	after, err := st.GetOrgStorageBytes(ctx, orgID)
	if err != nil {
		t.Fatalf("storage after idempotent write: %v", err)
	}
	if after <= before+int64(len(memory.Content)) {
		t.Fatalf(
			"storage delta = %d, want memory content plus retained snapshot",
			after-before,
		)
	}
}

func TestSQLiteMemoryIdempotency_SnapshotsAuthoritativeNoopRow(t *testing.T) {
	st := newSQLiteTestStore(t)
	orgID, actorID, projectID := seedIdempotencyProject(t, st)
	ctx := context.Background()

	existing := idempotencyTestMemory(
		"memory-future",
		projectID,
		actorID,
		"authoritative future content",
	)
	existing.UpdatedAt = existing.UpdatedAt.Add(time.Hour)
	if err := st.UpsertMemory(ctx, existing); err != nil {
		t.Fatalf("seed future memory: %v", err)
	}

	incoming := idempotencyTestMemory(
		existing.ID,
		projectID,
		actorID,
		"incoming stale content",
	)
	got, replayed, err := st.UpsertMemoryIdempotent(
		ctx,
		orgID,
		actorID,
		"operation-future",
		"sha256:future",
		incoming,
	)
	if err != nil {
		t.Fatalf("idempotent no-op: %v", err)
	}
	if replayed {
		t.Fatal("first request unexpectedly reported replay")
	}
	if got.Content != existing.Content || !got.UpdatedAt.Equal(existing.UpdatedAt) {
		t.Fatalf("first response = %+v, want authoritative %+v", got, existing)
	}

	replayedMemory, replayed, err := st.UpsertMemoryIdempotent(
		ctx,
		orgID,
		actorID,
		"operation-future",
		"sha256:future",
		incoming,
	)
	if err != nil || !replayed {
		t.Fatalf("replay = (%+v, %v, %v)", replayedMemory, replayed, err)
	}
	if replayedMemory.Content != existing.Content ||
		!replayedMemory.UpdatedAt.Equal(existing.UpdatedAt) {
		t.Fatalf("replay response = %+v, want authoritative %+v",
			replayedMemory, existing)
	}
	stored, err := st.GetMemoryByID(ctx, existing.ID)
	if err != nil {
		t.Fatalf("load stored memory: %v", err)
	}
	if stored.Content != existing.Content {
		t.Fatalf("stored content = %q, want %q", stored.Content, existing.Content)
	}
}

func TestSQLiteMigration019_UpgradesLegacyDatabaseWithoutDataLoss(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-v18.db")
	st, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("create current store: %v", err)
	}
	orgID, actorID, projectID := seedIdempotencyProject(t, st)
	memory := idempotencyTestMemory("legacy-memory", projectID, actorID, "preserved")
	if err := st.UpsertMemory(context.Background(), memory); err != nil {
		t.Fatalf("seed legacy memory: %v", err)
	}

	if _, err := st.db.Exec(`DROP TABLE memory_write_idempotency`); err != nil {
		t.Fatalf("drop migration 019 table: %v", err)
	}
	// Simulate a database whose last applied migration is 018. Migration 020
	// may already exist in the fixture because NewSQLiteStore always migrates
	// to the current schema, so remove it as well; leaving version 020 behind
	// would describe an impossible migration history and MAX(version) would
	// correctly skip 019.
	if _, err := st.db.Exec(`DELETE FROM schema_version WHERE version >= 19`); err != nil {
		t.Fatalf("rewind schema version: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("close legacy store: %v", err)
	}

	upgraded, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("upgrade legacy store: %v", err)
	}
	defer upgraded.Close()

	var version int
	if err := upgraded.db.QueryRow(`SELECT MAX(version) FROM schema_version`).Scan(&version); err != nil {
		t.Fatalf("read upgraded version: %v", err)
	}
	if version != sqliteSchemaVersion {
		t.Fatalf("schema version = %d, want %d", version, sqliteSchemaVersion)
	}
	if _, err := upgraded.GetMemoryByID(context.Background(), memory.ID); err != nil {
		t.Fatalf("legacy memory after migration: %v", err)
	}

	got, replayed, err := upgraded.UpsertMemoryIdempotent(
		context.Background(),
		orgID,
		actorID,
		"post-upgrade-operation",
		"sha256:post-upgrade",
		idempotencyTestMemory("post-upgrade-memory", projectID, actorID, "new"),
	)
	if err != nil || replayed || got.ID != "post-upgrade-memory" {
		t.Fatalf("post-upgrade idempotent write = %+v, replayed=%v, err=%v", got, replayed, err)
	}
}

func TestSQLiteMemoryIdempotencyRetention(t *testing.T) {
	st := newSQLiteTestStore(t)
	ctx := context.Background()
	if _, err := st.db.Exec(
		`INSERT INTO memory_write_idempotency
		   (org_scope, actor_scope, operation_id, payload_hash, memory_id, response_json, created_at)
		 VALUES
		   ('org', 'actor', 'old', 'old-hash', 'old-memory', '{}', ?),
		   ('org', 'actor', 'new', 'new-hash', 'new-memory', '{}', ?)`,
		time.Now().UTC().Add(-48*time.Hour),
		time.Now().UTC(),
	); err != nil {
		t.Fatalf("seed idempotency ledger: %v", err)
	}

	removed, err := st.PurgeMemoryIdempotencyOlderThan(
		ctx,
		time.Now().UTC().Add(-24*time.Hour),
	)
	if err != nil {
		t.Fatalf("purge idempotency ledger: %v", err)
	}
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}
	var remaining int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM memory_write_idempotency`).Scan(&remaining); err != nil {
		t.Fatalf("count idempotency ledger: %v", err)
	}
	if remaining != 1 {
		t.Fatalf("remaining = %d, want 1", remaining)
	}
}

func seedIdempotencyProject(t *testing.T, st *SQLiteStore) (orgID, actorID, projectID string) {
	t.Helper()
	orgID, actorID = projectTestOrg(t, st)
	project, err := st.CreateProject(
		context.Background(),
		orgID,
		"Idempotency",
		"idempotency",
		"idempotency-key",
		"",
		"",
		actorID,
		"other",
	)
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	return orgID, actorID, project.ID
}

func idempotencyTestMemory(id, projectID, actorID, content string) *model.Memory {
	now := time.Date(2026, time.July, 20, 12, 0, 0, 0, time.UTC)
	return &model.Memory{
		ID:          id,
		ProjectID:   projectID,
		Category:    "fact",
		Content:     content,
		ContentHash: "sha256:" + content,
		Keywords:    []string{"idempotency"},
		Source:      "test",
		AuthorID:    actorID,
		AuthorName:  "Test",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}
