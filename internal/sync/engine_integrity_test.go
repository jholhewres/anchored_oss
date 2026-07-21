package sync

import (
	"testing"
	"time"

	"github.com/jholhewres/anchored_oss/internal/model"
)

func TestSyncCanonicalizesUntrustedContentHash(t *testing.T) {
	eng, st, ctx, orgID, accID, projectID := capTestEngine(t)
	now := time.Now().UTC()
	const content = "The semantic index uses a server-owned content identity."

	response, err := eng.Sync(ctx, accID, orgID, &model.SyncRequest{
		ProjectID: projectID,
		ClientID:  "untrusted-hash-client",
		Pushes: []model.SyncMemory{{
			ID:          "canonical-hash-memory",
			Category:    "decision",
			Content:     content,
			ContentHash: "sha256:client-controlled",
			CreatedAt:   now,
			UpdatedAt:   now,
		}},
	})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if len(response.Results) != 1 || response.Results[0].Status != "accepted" {
		t.Fatalf("sync results = %+v, want accepted", response.Results)
	}
	stored, err := st.GetMemoryByID(ctx, "canonical-hash-memory")
	if err != nil {
		t.Fatalf("load stored memory: %v", err)
	}
	if stored.ContentHash != canonicalContentHash(content) {
		t.Fatalf(
			"stored content hash = %q, want canonical %q",
			stored.ContentHash,
			canonicalContentHash(content),
		)
	}
	if stored.ContentHash == "sha256:client-controlled" {
		t.Fatal("server trusted the client-controlled content hash")
	}
}

func TestSyncRejectsMemoryIDAcrossProjectsWithoutMutation(t *testing.T) {
	eng, st, ctx, orgID, accID, projectA := capTestEngine(t)
	projectB, err := st.CreateProject(
		ctx,
		orgID,
		"Other Repo",
		"other-repo",
		"k2",
		"k2",
		"https://github.example.com/org/other-repo.git",
		accID,
		"service",
	)
	if err != nil {
		t.Fatalf("create second project: %v", err)
	}
	now := time.Now().UTC()
	original := model.SyncMemory{
		ID:        "cross-project-memory",
		Category:  "decision",
		Content:   "The original project owns this globally unique memory ID.",
		CreatedAt: now,
		UpdatedAt: now,
	}
	first, err := eng.Sync(ctx, accID, orgID, &model.SyncRequest{
		ProjectID: projectA,
		ClientID:  "project-a-client",
		Pushes:    []model.SyncMemory{original},
	})
	if err != nil {
		t.Fatalf("seed sync: %v", err)
	}
	if len(first.Results) != 1 || first.Results[0].Status != "accepted" {
		t.Fatalf("seed results = %+v, want accepted", first.Results)
	}

	conflicting := original
	conflicting.Content = "A different project must not overwrite or rehome it."
	conflicting.UpdatedAt = now.Add(time.Minute)
	second, err := eng.Sync(ctx, accID, orgID, &model.SyncRequest{
		ProjectID: projectB.ID,
		ClientID:  "project-b-client",
		Pushes:    []model.SyncMemory{conflicting},
	})
	if err != nil {
		t.Fatalf("conflicting sync: %v", err)
	}
	if len(second.Results) != 1 ||
		second.Results[0].Status != "rejected" ||
		second.Results[0].Rule != "memory_id_conflict" {
		t.Fatalf("conflicting results = %+v, want memory_id_conflict", second.Results)
	}

	stored, err := st.GetMemoryByID(ctx, original.ID)
	if err != nil {
		t.Fatalf("load original memory: %v", err)
	}
	if stored.ProjectID != projectA || stored.Content != original.Content {
		t.Fatalf(
			"cross-project conflict mutated original: project=%q content=%q",
			stored.ProjectID,
			stored.Content,
		)
	}
}
