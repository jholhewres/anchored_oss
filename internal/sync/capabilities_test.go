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
