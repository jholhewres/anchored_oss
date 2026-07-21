//go:build integration

package store

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"
)

func TestMemoryIdempotency_PostgresConcurrentReplay(t *testing.T) {
	dsn := os.Getenv("ANCHORED_TEST_DSN")
	if dsn == "" {
		t.Skip("ANCHORED_TEST_DSN not set; skipping Postgres idempotency integration test")
	}
	st, err := NewPostgresStore(PoolConfig{DSN: dsn})
	if err != nil {
		t.Fatalf("open postgres store: %v", err)
	}
	defer st.Close()

	ctx := context.Background()
	suffix := uniq()
	org, err := st.CreateOrganization(ctx, "Idempotency "+suffix, "idempotency-"+suffix)
	if err != nil {
		t.Fatalf("create organization: %v", err)
	}
	defer func() {
		_, _ = st.db.ExecContext(ctx, `DELETE FROM memory_write_idempotency WHERE org_scope = $1`, org.ID)
		_, _ = st.db.ExecContext(ctx, `DELETE FROM organizations WHERE id = $1`, org.ID)
	}()
	account, err := st.CreateAccount(ctx, "idempotency-"+suffix+"@example.test", "Idempotency", "hash")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	project, err := st.CreateProject(
		ctx,
		org.ID,
		"Idempotency",
		"idempotency-"+suffix,
		"idempotency-"+suffix,
		"",
		"",
		account.ID,
		"other",
	)
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	const callers = 12
	type result struct {
		id       string
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
				fmt.Sprintf("pg-memory-%s-%02d", suffix, i),
				project.ID,
				account.ID,
				"postgres concurrent replay",
			)
			got, replayed, err := st.UpsertMemoryIdempotent(
				ctx,
				org.ID,
				account.ID,
				"pg-operation-"+suffix,
				"sha256:pg-one-payload",
				memory,
			)
			id := ""
			if got != nil {
				id = got.ID
			}
			results <- result{id: id, replayed: replayed, err: err}
		}(i)
	}
	start.Done()

	ids := make(map[string]struct{})
	writes := 0
	for i := 0; i < callers; i++ {
		result := <-results
		if result.err != nil {
			t.Fatalf("concurrent postgres write: %v", result.err)
		}
		ids[result.id] = struct{}{}
		if !result.replayed {
			writes++
		}
	}
	if writes != 1 || len(ids) != 1 {
		t.Fatalf("writes=%d distinct response IDs=%v, want one of each", writes, ids)
	}

	var memories, operations int
	if err := st.db.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM memories WHERE project_id = $1`,
		project.ID,
	).Scan(&memories); err != nil {
		t.Fatalf("count memories: %v", err)
	}
	if err := st.db.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM memory_write_idempotency
		 WHERE org_scope = $1 AND actor_scope = $2 AND operation_id = $3`,
		org.ID,
		account.ID,
		"pg-operation-"+suffix,
	).Scan(&operations); err != nil {
		t.Fatalf("count idempotency operations: %v", err)
	}
	if memories != 1 || operations != 1 {
		t.Fatalf("memories=%d operations=%d, want 1/1", memories, operations)
	}
}

func TestMemoryIdempotency_PostgresSnapshotsAuthoritativeNoopRow(t *testing.T) {
	dsn := os.Getenv("ANCHORED_TEST_DSN")
	if dsn == "" {
		t.Skip("ANCHORED_TEST_DSN not set; skipping Postgres idempotency integration test")
	}
	st, err := NewPostgresStore(PoolConfig{DSN: dsn})
	if err != nil {
		t.Fatalf("open postgres store: %v", err)
	}
	defer st.Close()

	ctx := context.Background()
	suffix := uniq()
	org, err := st.CreateOrganization(ctx, "Idempotency Future "+suffix, "idempotency-future-"+suffix)
	if err != nil {
		t.Fatalf("create organization: %v", err)
	}
	defer func() {
		_, _ = st.db.ExecContext(ctx, `DELETE FROM memory_write_idempotency WHERE org_scope = $1`, org.ID)
		_, _ = st.db.ExecContext(ctx, `DELETE FROM organizations WHERE id = $1`, org.ID)
	}()
	account, err := st.CreateAccount(ctx, "idempotency-future-"+suffix+"@example.test", "Idempotency", "hash")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	project, err := st.CreateProject(
		ctx, org.ID, "Idempotency Future", "idempotency-future-"+suffix,
		"idempotency-future-"+suffix, "", "", account.ID, "other",
	)
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	existing := idempotencyTestMemory(
		"pg-memory-future-"+suffix,
		project.ID,
		account.ID,
		"authoritative future content",
	)
	existing.UpdatedAt = existing.UpdatedAt.Add(time.Hour)
	if err := st.UpsertMemory(ctx, existing); err != nil {
		t.Fatalf("seed future memory: %v", err)
	}
	incoming := idempotencyTestMemory(
		existing.ID,
		project.ID,
		account.ID,
		"incoming stale content",
	)
	got, replayed, err := st.UpsertMemoryIdempotent(
		ctx, org.ID, account.ID, "pg-operation-future-"+suffix,
		"sha256:future", incoming,
	)
	if err != nil || replayed {
		t.Fatalf("first result = (%+v, %v, %v)", got, replayed, err)
	}
	if got.Content != existing.Content || !got.UpdatedAt.Equal(existing.UpdatedAt) {
		t.Fatalf("first response = %+v, want authoritative %+v", got, existing)
	}
	replay, replayed, err := st.UpsertMemoryIdempotent(
		ctx, org.ID, account.ID, "pg-operation-future-"+suffix,
		"sha256:future", incoming,
	)
	if err != nil || !replayed {
		t.Fatalf("replay = (%+v, %v, %v)", replay, replayed, err)
	}
	if replay.Content != existing.Content || !replay.UpdatedAt.Equal(existing.UpdatedAt) {
		t.Fatalf("replay response = %+v, want authoritative %+v", replay, existing)
	}
}
