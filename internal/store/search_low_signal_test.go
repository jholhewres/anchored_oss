package store

import (
	"context"
	"testing"
	"time"

	"github.com/jholhewres/anchored_oss/internal/model"
)

// TestSearchMemories_LowSignalVisible locks the relaxed read-path filter: a
// memory the client marked curation_status=low_signal must still appear in
// search results (it was hidden before v0.5.8). low_quality< threshold and
// scope=user stay hidden.
func TestSearchMemories_LowSignalVisible(t *testing.T) {
	st := newSQLiteTestStore(t)
	ctx := context.Background()
	orgID, acctID := projectTestOrg(t, st)
	proj, err := st.CreateProject(ctx, orgID, "Repo", "repo", "klow", "", "", acctID, "service")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	now := time.Now().UTC()
	low := &model.Memory{
		ID: "low", ProjectID: proj.ID, Category: "fact",
		Content:     "the release pipeline ships the binary to the staging cluster",
		ContentHash: "h-low", AuthorID: acctID,
		CreatedAt: now, UpdatedAt: now,
		Metadata: map[string]any{"curation_status": "low_signal"},
	}
	// A clean sibling with identical content confirms the filter change did not
	// break normal matches and that both rows surface.
	clean := &model.Memory{
		ID: "clean", ProjectID: proj.ID, Category: "fact",
		Content:     "the release pipeline ships the binary to the staging cluster",
		ContentHash: "h-clean", AuthorID: acctID,
		CreatedAt: now, UpdatedAt: now,
	}
	for _, m := range []*model.Memory{low, clean} {
		if err := st.UpsertMemory(ctx, m); err != nil {
			t.Fatalf("seed %s: %v", m.ID, err)
		}
	}

	got, err := st.SearchMemories(ctx, proj.ID, "release pipeline staging", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	ids := map[string]bool{}
	for _, m := range got {
		ids[m.ID] = true
	}
	if !ids["low"] {
		t.Fatalf("low_signal memory hidden from search; got ids=%v", ids)
	}
	if !ids["clean"] {
		t.Fatalf("clean memory missing from search; got ids=%v", ids)
	}

	// Sanity: low_quality below threshold still stays hidden (filter relaxed
	// only for low_signal, not for low_quality).
	lq := &model.Memory{
		ID: "lq", ProjectID: proj.ID, Category: "fact",
		Content:     "the release pipeline ships the binary to the staging cluster",
		ContentHash: "h-lq", AuthorID: acctID,
		CreatedAt: now, UpdatedAt: now,
		Metadata: map[string]any{"quality_score": 0.01},
	}
	if err := st.UpsertMemory(ctx, lq); err != nil {
		t.Fatalf("seed lq: %v", err)
	}
	got, err = st.SearchMemories(ctx, proj.ID, "release pipeline staging", 10)
	if err != nil {
		t.Fatalf("search lq: %v", err)
	}
	for _, m := range got {
		if m.ID == "lq" {
			t.Fatalf("low_quality memory should be hidden, but was returned")
		}
	}
}
