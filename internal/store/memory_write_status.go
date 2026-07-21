package store

import (
	"context"
	"time"

	"github.com/jholhewres/anchored_oss/internal/model"
)

// MemoryWriteStatusStore is an additive write capability that reports whether
// an upsert inserted a new row or updated an existing memory.
type MemoryWriteStatusStore interface {
	UpsertMemoryWithStatus(ctx context.Context, memory *model.Memory) (created bool, err error)
}

// MemoryIdempotencyStatusStore extends idempotent writes with the original
// insert/update result so replays can reproduce the same response.
type MemoryIdempotencyStatusStore interface {
	MemoryIdempotencyStore
	UpsertMemoryIdempotentWithStatus(
		ctx context.Context,
		orgID, actorID, operationID, payloadHash string,
		memory *model.Memory,
	) (result *model.Memory, replayed, created bool, err error)
	GetMemoryIdempotencyWithStatus(
		ctx context.Context,
		orgID, actorID, operationID, payloadHash string,
	) (result *model.Memory, created, found bool, err error)
}

// MemoryIdempotencyRetentionStore bounds the replay ledger. Implementations
// delete only records older than the supplied cutoff.
type MemoryIdempotencyRetentionStore interface {
	PurgeMemoryIdempotencyOlderThan(ctx context.Context, before time.Time) (int64, error)
}
