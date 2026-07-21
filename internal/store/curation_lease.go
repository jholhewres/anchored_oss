package store

import (
	"context"
	"time"
)

// LeasedCurationStore is an additive capability over the base curation queue: a
// backend that stamps an owner and a lease expiry on claimed rows so work
// orphaned by a crashed worker can be reclaimed instead of stranding forever in
// the 'processing' state. It is a separate interface (not part of Store) so the
// base contract and its mocks stay source-compatible.
//
// Semantics:
//   - ClaimCurationBatchLeased claims pending rows for owner, honoring the claim
//     until leaseTTL elapses.
//   - SetCurationDoneLeased / SetCurationFailedLeased complete a row only if the
//     caller still owns it; a worker that lost its lease (reclaimed after a
//     stall) cannot overwrite the row a new owner is now processing.
//   - ReclaimExpiredCuration returns rows whose lease expired to 'pending' and
//     reports how many were reclaimed.
type LeasedCurationStore interface {
	ClaimCurationBatchLeased(ctx context.Context, batchSize int, owner string, leaseTTL time.Duration) ([]string, error)
	SetCurationDoneLeased(ctx context.Context, memoryID, owner string) error
	SetCurationFailedLeased(ctx context.Context, memoryID, owner, errMsg string) error
	ReclaimExpiredCuration(ctx context.Context) (int, error)
}

// leaseSeconds normalizes a lease TTL to a positive whole number of seconds,
// falling back to defaultLeaseSeconds when the configured value is non-positive.
func leaseSeconds(leaseTTL time.Duration) int {
	secs := int(leaseTTL / time.Second)
	if secs <= 0 {
		return defaultLeaseSeconds
	}
	return secs
}

// defaultLeaseSeconds bounds a claim when the caller passes a non-positive TTL.
const defaultLeaseSeconds = 300
