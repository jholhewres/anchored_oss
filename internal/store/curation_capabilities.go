package store

import "context"

// ContentBoundMetadataStore prevents curation derived from an older content
// revision from being merged into a memory after that memory was rewritten.
// It is additive so existing Store implementations remain source-compatible.
type ContentBoundMetadataStore interface {
	UpdateMemoryMetadataIfContent(
		ctx context.Context,
		memoryID string,
		expectedContentHash string,
		metadata any,
	) (bool, error)
}

// UpdateMemoryMetadataForContent uses optimistic content matching when the
// backend supports it and falls back to the legacy metadata update otherwise.
func UpdateMemoryMetadataForContent(
	ctx context.Context,
	st Store,
	memoryID string,
	expectedContentHash string,
	metadata any,
) (bool, error) {
	if expectedContentHash != "" {
		if guarded, ok := st.(ContentBoundMetadataStore); ok {
			return guarded.UpdateMemoryMetadataIfContent(
				ctx,
				memoryID,
				expectedContentHash,
				metadata,
			)
		}
	}
	if err := st.UpdateMemoryMetadata(ctx, memoryID, metadata); err != nil {
		return false, err
	}
	return true, nil
}
