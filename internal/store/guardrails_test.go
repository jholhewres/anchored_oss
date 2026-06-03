package store

import (
	"context"
	"testing"

	"github.com/jholhewres/anchored_oss/internal/model"
)

func TestSQLiteCreateOrganization_SeedsDefaultGuardrails(t *testing.T) {
	st := newSQLiteTestStore(t)
	ctx := context.Background()

	org, err := st.CreateOrganization(ctx, "Acme", "acme")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	guards, err := st.ListGuardrails(ctx, org.ID)
	if err != nil {
		t.Fatalf("list guardrails: %v", err)
	}
	if len(guards) != len(DefaultGuardrails(org.ID)) {
		t.Fatalf("expected %d seeded guardrails, got %d", len(DefaultGuardrails(org.ID)), len(guards))
	}

	var builtins, categories int
	for _, g := range guards {
		if g.CreatedAt.IsZero() {
			t.Errorf("guardrail %q has zero CreatedAt (scanTime missing?)", g.Kind)
		}
		if !g.Enabled {
			t.Errorf("seeded guardrail %q should start enabled", g.Kind)
		}
		if g.Builtin {
			builtins++
		}
		if g.Kind == model.GuardrailCategory {
			categories++
		}
	}
	if builtins != 3 {
		t.Errorf("expected 3 builtin security guardrails, got %d", builtins)
	}
	if categories != 2 {
		t.Errorf("expected 2 default category guardrails, got %d", categories)
	}
}

func TestSQLiteGuardrailCRUD(t *testing.T) {
	st := newSQLiteTestStore(t)
	ctx := context.Background()
	org, err := st.CreateOrganization(ctx, "Acme", "acme")
	if err != nil {
		t.Fatalf("create org: %v", err)
	}

	// Create a custom keyword guardrail.
	g := &model.Guardrail{
		OrgID: org.ID, Kind: model.GuardrailKeyword, Value: "Project Falcon",
		Label: "no codename", Enabled: true,
	}
	if err := st.CreateGuardrail(ctx, g); err != nil {
		t.Fatalf("create guardrail: %v", err)
	}
	if g.ID == "" || g.CreatedAt.IsZero() {
		t.Fatalf("create should populate ID and CreatedAt, got id=%q created=%v", g.ID, g.CreatedAt)
	}

	// Get it back.
	got, err := st.GetGuardrail(ctx, org.ID, g.ID)
	if err != nil {
		t.Fatalf("get guardrail: %v", err)
	}
	if got.Value != "Project Falcon" || got.Builtin {
		t.Fatalf("unexpected guardrail round-trip: %+v", got)
	}

	// Update: disable + change value.
	got.Enabled = false
	got.Value = "Project Condor"
	if err := st.UpdateGuardrail(ctx, got); err != nil {
		t.Fatalf("update guardrail: %v", err)
	}
	reloaded, err := st.GetGuardrail(ctx, org.ID, g.ID)
	if err != nil {
		t.Fatalf("get after update: %v", err)
	}
	if reloaded.Enabled || reloaded.Value != "Project Condor" {
		t.Fatalf("update not persisted: %+v", reloaded)
	}

	// Delete.
	if err := st.DeleteGuardrail(ctx, org.ID, g.ID); err != nil {
		t.Fatalf("delete guardrail: %v", err)
	}
	if _, err := st.GetGuardrail(ctx, org.ID, g.ID); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}

	// Cross-org isolation: another org cannot see/delete this org's rows.
	other, err := st.CreateOrganization(ctx, "Other", "other")
	if err != nil {
		t.Fatalf("create other org: %v", err)
	}
	if err := st.DeleteGuardrail(ctx, other.ID, "nonexistent"); err != ErrNotFound {
		t.Fatalf("delete of missing id should be ErrNotFound, got %v", err)
	}
}
