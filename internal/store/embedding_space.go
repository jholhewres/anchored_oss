package store

import (
	"context"
	"fmt"

	"github.com/jholhewres/anchored_oss/internal/model"
	"github.com/jholhewres/anchored_oss/internal/semanticspace"
)

// PostgresEmbeddingDimensions is fixed by migration 011's vector(384)
// column. Changing it requires a schema migration, not only configuration.
const PostgresEmbeddingDimensions = 384

// SemanticSpaceSearcher is the model-aware vector retrieval extension. Store
// remains source-compatible for existing implementations, while the server
// requires this extension before it performs semantic comparisons.
type SemanticSpaceSearcher interface {
	SearchMemoriesByVectorSpace(ctx context.Context, projectID string, vec []float32, model string, dims, limit int) ([]*model.Memory, error)
}

// SemanticSpaceBackfiller is the model-aware stale-generation extension used
// by startup checks and reindex.
type SemanticSpaceBackfiller interface {
	MemoriesStaleEmbeddingSpace(ctx context.Context, model string, dims int, afterID string, limit int) ([]*model.Memory, error)
}

// CompleteSemanticSpaceWriter stores a vector with its full provider/model
// identity. It is additive so existing Store implementations keep compiling.
type CompleteSemanticSpaceWriter interface {
	UpdateMemoryEmbeddingInSpace(ctx context.Context, memoryID string, vec []float32, space semanticspace.Identity) error
}

// ContentBoundCompleteSemanticSpaceWriter prevents a vector computed from an
// older revision of a memory from being attached after the content changed.
// The boolean is false when the expected content hash is no longer current.
type ContentBoundCompleteSemanticSpaceWriter interface {
	UpdateMemoryEmbeddingInSpaceIfContent(
		ctx context.Context,
		memoryID string,
		expectedContentHash string,
		vec []float32,
		space semanticspace.Identity,
	) (bool, error)
}

// CompleteSemanticSpaceSearcher excludes vectors from providers, revisions, or
// normalization schemes that happen to share a model name and width.
type CompleteSemanticSpaceSearcher interface {
	SearchMemoriesByCompleteSemanticSpace(ctx context.Context, projectID string, vec []float32, space semanticspace.Identity, limit int) ([]*model.Memory, error)
}

// CompleteSemanticSpaceBackfiller identifies every row outside the complete
// active space, including legacy rows that predate semantic_space_id.
type CompleteSemanticSpaceBackfiller interface {
	MemoriesStaleInCompleteSemanticSpace(ctx context.Context, space semanticspace.Identity, afterID string, limit int) ([]*model.Memory, error)
}

// CompleteSemanticSpaceCoverage reports whether a project still contains rows
// that cannot safely participate in the active semantic space.
type CompleteSemanticSpaceCoverage interface {
	ProjectHasStaleSemanticSpace(
		ctx context.Context,
		projectID string,
		space semanticspace.Identity,
	) (bool, error)
}

// EmbeddingDimensionStore declares a backend's physical vector-width
// constraint. Postgres is fixed at vector(384); SQLite is intentionally
// unconstrained because vectors are JSON and test/dev models may use any size.
type EmbeddingDimensionStore interface {
	EmbeddingDimensionConstraint() (dimensions int, fixed bool)
}

// ValidateEmbeddingDimensions checks a provider width against the physical
// backend before any query, backfill, or HTTP listener starts.
func ValidateEmbeddingDimensions(st Store, dimensions int) error {
	if dimensions <= 0 {
		return fmt.Errorf("embedding dimensions must be > 0, got %d", dimensions)
	}
	constraint, ok := st.(EmbeddingDimensionStore)
	if !ok {
		return nil
	}
	required, fixed := constraint.EmbeddingDimensionConstraint()
	if fixed && dimensions != required {
		return fmt.Errorf(
			"embedding dimensions=%d are incompatible with Postgres vector(%d)",
			dimensions,
			required,
		)
	}
	return nil
}

func SearchMemoriesBySemanticSpace(
	ctx context.Context,
	st Store,
	projectID string,
	vec []float32,
	model string,
	dims, limit int,
) ([]*model.Memory, error) {
	spaceStore, ok := st.(SemanticSpaceSearcher)
	if !ok {
		return nil, fmt.Errorf("semantic-space search is unsupported by %T", st)
	}
	return spaceStore.SearchMemoriesByVectorSpace(ctx, projectID, vec, model, dims, limit)
}

func MemoriesStaleInSemanticSpace(
	ctx context.Context,
	st Store,
	model string,
	dims int,
	afterID string,
	limit int,
) ([]*model.Memory, error) {
	spaceStore, ok := st.(SemanticSpaceBackfiller)
	if !ok {
		return nil, fmt.Errorf("semantic-space backfill is unsupported by %T", st)
	}
	return spaceStore.MemoriesStaleEmbeddingSpace(ctx, model, dims, afterID, limit)
}

// UpdateMemoryEmbeddingForSpace uses the complete additive contract when
// available and falls back to the legacy model-only Store method for old test
// doubles and third-party implementations.
func UpdateMemoryEmbeddingForSpace(
	ctx context.Context,
	st Store,
	memoryID string,
	vec []float32,
	space semanticspace.Identity,
) error {
	if complete, ok := st.(CompleteSemanticSpaceWriter); ok {
		return complete.UpdateMemoryEmbeddingInSpace(ctx, memoryID, vec, space)
	}
	return st.UpdateMemoryEmbedding(ctx, memoryID, vec, space.Model)
}

// UpdateMemoryEmbeddingForContentSpace uses an optimistic content guard when
// the backend supports it. The compatibility fallback keeps older Store
// implementations working, but cannot protect against a concurrent rewrite.
func UpdateMemoryEmbeddingForContentSpace(
	ctx context.Context,
	st Store,
	memoryID string,
	expectedContentHash string,
	vec []float32,
	space semanticspace.Identity,
) (bool, error) {
	if expectedContentHash != "" {
		if guarded, ok := st.(ContentBoundCompleteSemanticSpaceWriter); ok {
			return guarded.UpdateMemoryEmbeddingInSpaceIfContent(
				ctx,
				memoryID,
				expectedContentHash,
				vec,
				space,
			)
		}
	}
	if err := UpdateMemoryEmbeddingForSpace(ctx, st, memoryID, vec, space); err != nil {
		return false, err
	}
	return true, nil
}

// SearchMemoriesByCompleteSpace prefers full semantic identity while retaining
// compatibility with existing Store implementations.
func SearchMemoriesByCompleteSpace(
	ctx context.Context,
	st Store,
	projectID string,
	vec []float32,
	space semanticspace.Identity,
	limit int,
) ([]*model.Memory, error) {
	if complete, ok := st.(CompleteSemanticSpaceSearcher); ok {
		return complete.SearchMemoriesByCompleteSemanticSpace(ctx, projectID, vec, space, limit)
	}
	return SearchMemoriesBySemanticSpace(
		ctx,
		st,
		projectID,
		vec,
		space.Model,
		space.Dimensions,
		limit,
	)
}

// MemoriesStaleInCompleteSpace prefers full semantic identity while retaining
// the previous model+dimensions extension as a compatibility fallback.
func MemoriesStaleInCompleteSpace(
	ctx context.Context,
	st Store,
	space semanticspace.Identity,
	afterID string,
	limit int,
) ([]*model.Memory, error) {
	if complete, ok := st.(CompleteSemanticSpaceBackfiller); ok {
		return complete.MemoriesStaleInCompleteSemanticSpace(ctx, space, afterID, limit)
	}
	return MemoriesStaleInSemanticSpace(
		ctx,
		st,
		space.Model,
		space.Dimensions,
		afterID,
		limit,
	)
}

// ProjectHasStaleCompleteSpace scopes semantic readiness to the requested
// project. Compatibility stores fall back to a global one-row stale scan.
func ProjectHasStaleCompleteSpace(
	ctx context.Context,
	st Store,
	projectID string,
	space semanticspace.Identity,
) (bool, error) {
	if coverage, ok := st.(CompleteSemanticSpaceCoverage); ok {
		return coverage.ProjectHasStaleSemanticSpace(ctx, projectID, space)
	}
	stale, err := MemoriesStaleInCompleteSpace(ctx, st, space, "", 1)
	if err != nil {
		return false, err
	}
	return len(stale) > 0, nil
}
