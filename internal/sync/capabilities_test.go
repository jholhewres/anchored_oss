package sync

import (
	"context"
	"encoding/json"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/jholhewres/anchored_oss/internal/middleware"
	"github.com/jholhewres/anchored_oss/internal/model"
	"github.com/jholhewres/anchored_oss/internal/store"
)

func capTestEngine(t *testing.T) (*SyncEngine, *store.SQLiteStore, context.Context, string, string, string) {
	t.Helper()
	st, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "cap.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	ctx := context.WithValue(context.Background(), middleware.ScopeKey, "admin")
	org, err := st.CreateOrganization(ctx, "Acme", "acme")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	acc, err := st.CreateAccount(ctx, "dev@acme.test", "Dev", "x")
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	if err := st.AddOrgMember(ctx, org.ID, acc.ID, "admin"); err != nil {
		t.Fatalf("add org member: %v", err)
	}
	proj, err := st.CreateProject(ctx, org.ID, "Repo", "repo", "k1", "k1",
		"https://github.example.com/org/repo.git", acc.ID, "service")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	return NewSyncEngine(st, slog.Default()), st, ctx, org.ID, acc.ID, proj.ID
}

// TestSync_NoCapabilities_NoPolicy locks backward compatibility: a request
// without ClientCapabilities gets a response whose JSON has no "policy" key —
// byte-identical to the pre-negotiation protocol.
func TestSync_NoCapabilities_NoPolicy(t *testing.T) {
	eng, _, ctx, orgID, accID, projID := capTestEngine(t)

	resp, err := eng.Sync(ctx, accID, orgID, &model.SyncRequest{ProjectID: projID, ClientID: "old"})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if resp.Policy != nil {
		t.Fatalf("capability-less request must get nil Policy, got %+v", resp.Policy)
	}
	blob, _ := json.Marshal(resp)
	if got := string(blob); contains(got, "\"policy\"") {
		t.Fatalf("response JSON must not contain a policy key: %s", got)
	}
}

// TestSync_WithCapabilities_PolicyPresent: a capability-aware request gets the
// effective policy hints.
func TestSync_WithCapabilities_PolicyPresent(t *testing.T) {
	eng, _, ctx, orgID, accID, projID := capTestEngine(t)

	resp, err := eng.Sync(ctx, accID, orgID, &model.SyncRequest{
		ProjectID:          projID,
		ClientID:           "new",
		ClientCapabilities: &model.ClientCapabilities{PromotionQueue: true},
	})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if resp.Policy == nil {
		t.Fatal("capability-aware request must get Policy hints")
	}
	if resp.Policy.MaxMemoriesPerSync != store.DefaultMaxMemoriesPerSync {
		t.Errorf("max per sync = %d, want %d", resp.Policy.MaxMemoriesPerSync, store.DefaultMaxMemoriesPerSync)
	}
	if resp.Policy.QualityThreshold != store.DefaultQualityThreshold {
		t.Errorf("quality threshold = %v, want %v", resp.Policy.QualityThreshold, store.DefaultQualityThreshold)
	}
	// Seeded org blocks event + preference by default.
	if !containsStr(resp.Policy.BlockedCategories, "event") || !containsStr(resp.Policy.BlockedCategories, "preference") {
		t.Errorf("blocked categories = %v, want event+preference", resp.Policy.BlockedCategories)
	}
}

// TestSync_OverCap_WholeBatchRejected: a push beyond the cap is rejected
// wholesale and nothing is persisted.
func TestSync_OverCap_WholeBatchRejected(t *testing.T) {
	eng, st, ctx, orgID, accID, projID := capTestEngine(t)

	// Lower the cap to keep the test fast.
	if err := st.UpsertOrgPolicy(ctx, &model.OrgPolicy{
		OrgID: orgID, QualityThreshold: store.DefaultQualityThreshold,
		NearDupThreshold: store.DefaultNearDupThreshold, MaxMemoriesPerSync: 5,
	}); err != nil {
		t.Fatalf("set policy: %v", err)
	}

	now := time.Now().UTC()
	pushes := make([]model.SyncMemory, 6) // cap+1
	for i := range pushes {
		pushes[i] = model.SyncMemory{
			ID: "", Category: "decision",
			Content:   "decision number " + string(rune('a'+i)) + " about the persistence layer architecture",
			CreatedAt: now, UpdatedAt: now,
		}
	}

	resp, err := eng.Sync(ctx, accID, orgID, &model.SyncRequest{ProjectID: projID, ClientID: "new", Pushes: pushes})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if len(resp.Results) != 6 {
		t.Fatalf("want 6 results, got %d", len(resp.Results))
	}
	for _, r := range resp.Results {
		if r.Status != "rejected" || r.Rule != "max_memories_per_sync" {
			t.Fatalf("every result must be rejected by the cap, got %+v", r)
		}
	}
	// Nothing persisted.
	_, total, err := st.ListMemoriesPaginated(ctx, projID, 1, 0, "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 0 {
		t.Fatalf("over-cap push must persist nothing, project has %d", total)
	}

	// The over-cap rejection must be visible to memory health (rejection
	// counter bumped by the full batch size) — the cap defends against exactly
	// the dump this counter surfaces.
	stats, err := st.ListRejectionStats(ctx, orgID, projID, time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02"))
	if err != nil {
		t.Fatalf("list rejection stats: %v", err)
	}
	var capCount int64
	for _, s := range stats {
		if s.Rule == "max_memories_per_sync" {
			capCount += s.Count
		}
	}
	if capCount != 6 {
		t.Fatalf("over-cap rejection stat = %d, want 6", capCount)
	}

	// At the cap, the batch is accepted.
	resp, err = eng.Sync(ctx, accID, orgID, &model.SyncRequest{ProjectID: projID, ClientID: "new", Pushes: pushes[:5]})
	if err != nil {
		t.Fatalf("sync at cap: %v", err)
	}
	for _, r := range resp.Results {
		if r.Status != "accepted" {
			t.Fatalf("at-cap push must be accepted, got %+v", r)
		}
	}
}

// TestSync_ArtifactSummaries_PopulatedWhenCapable verifies that artifact IDs
// from accepted memory metadata are returned in ArtifactSummaries when the
// client advertises ArtifactSummaries capability, and are absent otherwise.
func TestSync_ArtifactSummaries_PopulatedWhenCapable(t *testing.T) {
	eng, _, ctx, orgID, accID, projID := capTestEngine(t)

	now := time.Now().UTC()
	pushes := []model.SyncMemory{
		{
			ID: "m1", Category: "decision",
			Content:   "use postgres for the primary data store in production",
			CreatedAt: now, UpdatedAt: now,
			Metadata: map[string]any{"artifact_id": "art-abc"},
		},
		{
			ID: "m2", Category: "decision",
			Content:   "use redis for the session cache layer in production",
			CreatedAt: now, UpdatedAt: now,
			Metadata: map[string]any{"artifact_id": "art-abc"}, // same artifact, deduped
		},
		{
			ID: "m3", Category: "decision",
			Content:   "use s3 for object storage in production environments",
			CreatedAt: now, UpdatedAt: now,
			Metadata: map[string]any{"artifact_id": "art-xyz"},
		},
		{
			ID: "m4", Category: "decision",
			Content:   "memory without artifact id for coverage of the no-artifact path",
			CreatedAt: now, UpdatedAt: now,
			// no artifact_id in metadata
		},
	}

	// Without ArtifactSummaries capability: field must be absent.
	respNoArt, err := eng.Sync(ctx, accID, orgID, &model.SyncRequest{
		ProjectID:          projID,
		ClientID:           "new",
		Pushes:             pushes,
		ClientCapabilities: &model.ClientCapabilities{PromotionQueue: true},
	})
	if err != nil {
		t.Fatalf("sync (no artifact cap): %v", err)
	}
	if len(respNoArt.ArtifactSummaries) != 0 {
		t.Fatalf("ArtifactSummaries must be empty when capability not set, got %v", respNoArt.ArtifactSummaries)
	}

	// With ArtifactSummaries capability: two unique IDs returned (art-abc, art-xyz).
	respWithArt, err := eng.Sync(ctx, accID, orgID, &model.SyncRequest{
		ProjectID:          projID,
		ClientID:           "new",
		Pushes:             pushes,
		ClientCapabilities: &model.ClientCapabilities{ArtifactSummaries: true},
	})
	if err != nil {
		t.Fatalf("sync (artifact cap): %v", err)
	}
	if len(respWithArt.ArtifactSummaries) != 2 {
		t.Fatalf("expected 2 ArtifactSummaries (deduped), got %d: %v", len(respWithArt.ArtifactSummaries), respWithArt.ArtifactSummaries)
	}
	ids := make(map[string]bool)
	for _, s := range respWithArt.ArtifactSummaries {
		ids[s.ArtifactID] = true
	}
	if !ids["art-abc"] {
		t.Errorf("expected art-abc in ArtifactSummaries, got %v", respWithArt.ArtifactSummaries)
	}
	if !ids["art-xyz"] {
		t.Errorf("expected art-xyz in ArtifactSummaries, got %v", respWithArt.ArtifactSummaries)
	}
}

// TestSync_ArtifactSummaries_RejectedMemoriesExcluded verifies that artifact IDs
// from rejected memories are not included in ArtifactSummaries.
func TestSync_ArtifactSummaries_RejectedMemoriesExcluded(t *testing.T) {
	eng, _, ctx, orgID, accID, projID := capTestEngine(t)

	now := time.Now().UTC()
	pushes := []model.SyncMemory{
		{
			ID: "r1", Category: "event", // blocked by default guardrails
			Content:   "ci pipeline run event that should be rejected by the default blocked categories",
			CreatedAt: now, UpdatedAt: now,
			Metadata: map[string]any{"artifact_id": "art-rejected"},
		},
		{
			ID: "r2", Category: "decision",
			Content:   "accepted memory with its own artifact id for the test",
			CreatedAt: now, UpdatedAt: now,
			Metadata: map[string]any{"artifact_id": "art-accepted"},
		},
	}

	resp, err := eng.Sync(ctx, accID, orgID, &model.SyncRequest{
		ProjectID:          projID,
		ClientID:           "new",
		Pushes:             pushes,
		ClientCapabilities: &model.ClientCapabilities{ArtifactSummaries: true},
	})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	for _, s := range resp.ArtifactSummaries {
		if s.ArtifactID == "art-rejected" {
			t.Errorf("rejected memory's artifact_id must not appear in ArtifactSummaries")
		}
	}
	found := false
	for _, s := range resp.ArtifactSummaries {
		if s.ArtifactID == "art-accepted" {
			found = true
		}
	}
	if !found {
		t.Errorf("accepted memory's artifact_id must appear in ArtifactSummaries, got %v", resp.ArtifactSummaries)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func containsStr(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
