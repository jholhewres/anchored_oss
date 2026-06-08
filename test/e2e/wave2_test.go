//go:build e2e

package e2e

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jholhewres/anchored_oss/internal/model"
	"github.com/jholhewres/anchored_oss/internal/project"
)

// jsonStr returns s encoded as a JSON string literal (quoted, escaped).
func jsonStr(t *testing.T, s string) string {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// seedProjectForRepo creates a server-side project matching the repo's origin
// and grants the seeded account access, mirroring the dashboard "create with
// repository URL" flow.
func seedProjectForRepo(t *testing.T, e *env, name, slug, origin string) *model.Project {
	t.Helper()
	ctx := context.Background()
	p, err := e.store.CreateProject(ctx, e.orgID, name, slug,
		project.DeriveRemoteKey(origin), project.DeriveLegacyRemoteKey(origin),
		origin, e.acctID, "service")
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if err := e.store.EnsureCreatorProjectAccess(ctx, e.orgID, e.acctID, p.ID); err != nil {
		t.Fatalf("grant access: %v", err)
	}
	return p
}

// TestCapabilityMatrix_NewClientGetsPolicyHint verifies the new client (HEAD,
// which always advertises capabilities) surfaces the server's policy hint
// after a sync against the new server.
func TestCapabilityMatrix_NewClientGetsPolicyHint(t *testing.T) {
	e := newEnv(t)
	repo := newRepo(t, testOrigin)
	seedProjectForRepo(t, e, "API Service", "api-service", testOrigin)
	e.configureRemote(t, e.syncKey)

	if out, code := e.run(t, repo, "save",
		"decision: the sync protocol negotiates capabilities so the client can warn before pushing blocked categories",
		"--category", "decision"); code != 0 {
		t.Fatalf("save: %d\n%s", code, out)
	}
	out, code := e.run(t, repo, "remote", "sync")
	if code != 0 {
		t.Fatalf("sync exit %d:\n%s", code, out)
	}
	// The new client always advertises capabilities, so the server returns
	// policy hints and the CLI surfaces them.
	mustContain(t, out, "server policy:", "max 500 per sync")
}

// TestMaxMemoriesPerSync_RejectedWholesale lowers the org cap and verifies a
// cap+1 push is rejected wholesale with nothing persisted. Driven server-side
// (the client batches differently), this exercises the engine guarantee end to
// end via the compat push endpoint.
func TestMaxMemoriesPerSync_RejectedWholesale(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	proj := seedProjectForRepo(t, e, "Capped", "capped", "git@github.example.com:org/capped.git")

	// Lower the cap to 3 for a fast test.
	if err := e.store.UpsertOrgPolicy(ctx, &model.OrgPolicy{
		OrgID: e.orgID, QualityThreshold: 0.55, NearDupThreshold: 0.85, MaxMemoriesPerSync: 3,
	}); err != nil {
		t.Fatalf("set policy: %v", err)
	}

	// Push 4 memories (cap+1) directly through the engine to assert the
	// wholesale rejection + zero persistence guarantee.
	now := time.Now().UTC()
	pushes := make([]model.SyncMemory, 4)
	for i := range pushes {
		pushes[i] = model.SyncMemory{
			Category: "decision", CreatedAt: now, UpdatedAt: now,
			Content: "decision " + string(rune('a'+i)) + " about the durable storage architecture and sync engine",
		}
	}
	resp := e.engineSync(t, proj.ID, &model.SyncRequest{
		ProjectID: proj.ID, ClientID: "e2e", Pushes: pushes,
		ClientCapabilities: &model.ClientCapabilities{},
	})
	for _, r := range resp.Results {
		if r.Status != "rejected" || r.Rule != "max_memories_per_sync" {
			t.Fatalf("every result must be cap-rejected, got %+v", r)
		}
	}
	if n := countProjectMemories(t, e.store, proj.ID); n != 0 {
		t.Fatalf("over-cap push persisted %d memories, want 0", n)
	}
	if resp.Policy == nil || resp.Policy.MaxMemoriesPerSync != 3 {
		t.Fatalf("policy hint should report the cap: %+v", resp.Policy)
	}
}

// TestHookAutoRecall_InjectsRelevantHit drives the real userpromptsubmit hook
// binary: a prompt relevant to a seeded memory injects a recall block; an
// irrelevant prompt injects only the reminder (no recall block).
func TestHookAutoRecall_InjectsRelevantHit(t *testing.T) {
	e := newEnv(t)
	repo := newRepo(t, testOrigin)

	// Seed a memory locally via the client CLI so it lands in the client DB the
	// hook reads.
	if out, code := e.run(t, repo, "save",
		"decision: we standardized on Reciprocal Rank Fusion for hybrid search ranking in the sync engine",
		"--category", "decision"); code != 0 {
		t.Fatalf("save: %d\n%s", code, out)
	}

	// Relevant prompt -> recall block present.
	relevant := `{"session_id":"s1","cwd":"` + repo + `","prompt":"how did we decide on hybrid search ranking fusion?"}`
	out := e.runStdin(t, repo, relevant, "hook", "userpromptsubmit")
	if !strings.Contains(out, "anchored_recall") {
		t.Fatalf("relevant prompt should inject a recall block:\n%s", out)
	}

	// Trivial prompt -> no recall block (reminder only).
	trivial := `{"session_id":"s1","cwd":"` + repo + `","prompt":"oi"}`
	out = e.runStdin(t, repo, trivial, "hook", "userpromptsubmit")
	if strings.Contains(out, "anchored_recall") {
		t.Fatalf("trivial prompt must not inject a recall block:\n%s", out)
	}
}

// TestHookPostToolUse_LargeOutputCaptured drives the REAL posttooluse hook
// binary with a >8KB tool output and verifies it is captured as a searchable
// artifact — exercising the production AddArtifact path (prepared statements),
// which a stubbed unit test cannot. A regression guard for the capture wiring.
func TestHookPostToolUse_LargeOutputCaptured(t *testing.T) {
	e := newEnv(t)
	repo := newRepo(t, testOrigin)

	// Initialize the client DB schema (the lightweight hook context does not
	// run migrations — production relies on a prior save/serve). A throwaway
	// save is the cheapest way to migrate before driving the hook cold.
	if out, code := e.run(t, repo, "save", "fact: initialize the local store schema for this scenario", "--category", "fact"); code != 0 {
		t.Fatalf("seed save: %d\n%s", code, out)
	}

	big := strings.Repeat("--- FAIL: TestAlpha (0.01s)\n    alpha_test.go:42: boom\n", 400) // >8KB
	resp := jsonStr(t, big)
	payload := `{"session_id":"s1","cwd":"` + repo + `","tool_name":"Bash",` +
		`"tool_input":{"command":"go test ./..."},"tool_response":` + resp + `}`

	out := e.runStdin(t, repo, payload, "hook", "posttooluse")
	if !strings.Contains(out, `"artifact_id"`) || !strings.Contains(out, "test_report") {
		t.Fatalf("large output should be captured as a test_report artifact:\n%s", out)
	}

	// The artifact is findable via the client CLI.
	found, code := e.run(t, repo, "artifact", "search", "boom")
	if code != 0 {
		t.Fatalf("artifact search exit %d:\n%s", code, found)
	}
	if !strings.Contains(found, "test_report") {
		t.Fatalf("captured artifact not found by search:\n%s", found)
	}
}
