package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jholhewres/anchored_oss/internal/model"
)

// TestAccountTaskThreads_PrivacyAndRoundTrip is the Feature C correctness
// gate: threads round-trip with their JSON fields, upserts converge on
// (account_id, task_key), and — the privacy requirement — one account NEVER
// sees another's threads.
func TestAccountTaskThreads_PrivacyAndRoundTrip(t *testing.T) {
	st, err := NewSQLiteStore(filepath.Join(t.TempDir(), "tt.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	ctx := context.Background()

	a := &model.AccountTaskThread{
		AccountID: "acct-A", TaskKey: "PROJ-1", Status: "active",
		Projects: []string{"repo-a", "repo-b"},
		Journal:  []string{"decided the wire format"},
		Details:  map[string]any{"files": []any{"a.go"}},
	}
	if err := st.UpsertAccountTaskThread(ctx, a); err != nil {
		t.Fatal(err)
	}
	b := &model.AccountTaskThread{AccountID: "acct-B", TaskKey: "PROJ-1", Status: "done"}
	if err := st.UpsertAccountTaskThread(ctx, b); err != nil {
		t.Fatal(err)
	}

	// Round-trip for A.
	got, err := st.ListAccountTaskThreads(ctx, "acct-A")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].TaskKey != "PROJ-1" || got[0].Status != "active" {
		t.Fatalf("acct-A list wrong: %+v", got)
	}
	if len(got[0].Projects) != 2 || got[0].Journal[0] != "decided the wire format" {
		t.Fatalf("JSON fields lost: %+v", got[0])
	}

	// PRIVACY: A's listing never contains B's state and vice versa.
	gotB, _ := st.ListAccountTaskThreads(ctx, "acct-B")
	if len(gotB) != 1 || gotB[0].Status != "done" {
		t.Fatalf("acct-B list wrong: %+v", gotB)
	}

	// Upsert converges (same account+key updates, no duplicate).
	a.Status = "paused"
	if err := st.UpsertAccountTaskThread(ctx, a); err != nil {
		t.Fatal(err)
	}
	got, _ = st.ListAccountTaskThreads(ctx, "acct-A")
	if len(got) != 1 || got[0].Status != "paused" {
		t.Fatalf("upsert did not converge: %+v", got)
	}
}
