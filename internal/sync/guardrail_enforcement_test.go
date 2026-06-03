package sync

import (
	"context"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/jholhewres/anchored_oss/internal/model"
	"github.com/jholhewres/anchored_oss/internal/policy"
	"github.com/jholhewres/anchored_oss/internal/store"
)

// accepted runs the org's effective filter on one item.
func accepted(t *testing.T, e *SyncEngine, orgID string, item policy.Filterable) bool {
	t.Helper()
	return e.filterForOrg(context.Background(), orgID).Filter([]policy.Filterable{item})[0].Accepted
}

// TestFilterForOrg_GuardrailEnforcement verifies that the sync engine builds its
// content filter from the org's guardrail set: seeded defaults block the right
// things, custom rules reject matching content, and disabling a builtin relaxes it.
func TestFilterForOrg_GuardrailEnforcement(t *testing.T) {
	st, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "g.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	ctx := context.Background()

	org, err := st.CreateOrganization(ctx, "Acme", "acme") // seeds default guardrails
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	eng := NewSyncEngine(st, slog.Default())

	// Seeded defaults: event category, secrets, and user-scope are blocked.
	if accepted(t, eng, org.ID, policy.Filterable{ID: "e", Category: "event", Content: "deployed v2 today"}) {
		t.Error("event must be blocked by the seeded category guardrail")
	}
	if accepted(t, eng, org.ID, policy.Filterable{ID: "s", Category: "decision", Content: "leaked AKIA1234567890ABCDEF"}) {
		t.Error("secret must be blocked by the seeded secret_detection guardrail")
	}
	if accepted(t, eng, org.ID, policy.Filterable{ID: "ok", Category: "decision", Content: "we standardized on Postgres for durability"}) == false {
		t.Error("clean content must be accepted")
	}

	// Add a custom keyword guardrail; matching content is now rejected.
	if err := st.CreateGuardrail(ctx, &model.Guardrail{
		OrgID: org.ID, Kind: model.GuardrailKeyword, Value: "Project Falcon", Label: "codename", Enabled: true,
	}); err != nil {
		t.Fatalf("create guardrail: %v", err)
	}
	if accepted(t, eng, org.ID, policy.Filterable{ID: "k", Category: "decision", Content: "the project falcon launch is set"}) {
		t.Error("custom keyword guardrail must reject matching content (case-insensitive)")
	}

	// Disable the secret_detection builtin; secrets now pass (admin opted out).
	guards, err := st.ListGuardrails(ctx, org.ID)
	if err != nil {
		t.Fatalf("list guardrails: %v", err)
	}
	for _, g := range guards {
		if g.Kind == model.GuardrailSecretDetection {
			g.Enabled = false
			if err := st.UpdateGuardrail(ctx, g); err != nil {
				t.Fatalf("disable secret_detection: %v", err)
			}
		}
	}
	if !accepted(t, eng, org.ID, policy.Filterable{ID: "s2", Category: "decision", Content: "key AKIA1234567890ABCDEF noted"}) {
		t.Error("secret must pass once secret_detection is disabled")
	}
	// The keyword guardrail still applies (independent of the security toggle).
	if accepted(t, eng, org.ID, policy.Filterable{ID: "k2", Category: "decision", Content: "project falcon again"}) {
		t.Error("custom keyword guardrail must still reject after toggling a builtin")
	}
}
