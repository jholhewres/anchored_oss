package curation

import (
	"testing"
	"time"

	"github.com/jholhewres/anchored_oss/internal/model"
)

func mem(id string, content string, kw []string, age time.Duration) *model.Memory {
	return &model.Memory{
		ID:        id,
		Content:   content,
		Keywords:  kw,
		CreatedAt: time.Date(2026, 6, 8, 12, 0, 0, 0, time.UTC).Add(-age),
	}
}

func TestFindContradiction(t *testing.T) {
	// Subject: package manager / monorepo tooling. The newer memory reverses the
	// earlier one ("no longer", "switched"), sharing >=2 keywords.
	pnpm := mem("m-pnpm", "we use pnpm for the monorepo tooling workspace",
		[]string{"pnpm", "monorepo", "tooling"}, 60*24*time.Hour)
	npm := mem("m-npm", "we no longer use pnpm; switched the monorepo tooling to npm",
		[]string{"pnpm", "monorepo", "tooling", "npm"}, 1*24*time.Hour)

	t.Run("reversal over shared subject -> candidate", func(t *testing.T) {
		got, ok := findContradiction(npm, []*model.Memory{pnpm})
		if !ok || got != "m-pnpm" {
			t.Fatalf("want contradiction with m-pnpm, got (%q, %v)", got, ok)
		}
	})

	t.Run("unrelated peer -> no candidate", func(t *testing.T) {
		deploy := mem("m-deploy", "we deploy the server with pm2 and systemd",
			[]string{"deploy", "pm2", "systemd"}, 30*24*time.Hour)
		if _, ok := findContradiction(npm, []*model.Memory{deploy}); ok {
			t.Fatal("unrelated peer must not be a contradiction candidate")
		}
	})

	t.Run("same time window -> skipped", func(t *testing.T) {
		// Both written within contradictionMinGap of each other.
		a := mem("a", "we use pnpm for monorepo tooling", []string{"pnpm", "monorepo", "tooling"}, 0)
		b := mem("b", "we no longer use pnpm for monorepo tooling", []string{"pnpm", "monorepo", "tooling"}, 30*time.Minute)
		if _, ok := findContradiction(a, []*model.Memory{b}); ok {
			t.Fatal("memories in the same time window must not contradict")
		}
	})

	t.Run("both assert, no reversal -> no candidate", func(t *testing.T) {
		a := mem("a", "we use pnpm for monorepo tooling", []string{"pnpm", "monorepo", "tooling"}, 60*24*time.Hour)
		b := mem("b", "we use pnpm for monorepo tooling workspace builds", []string{"pnpm", "monorepo", "tooling"}, 1*24*time.Hour)
		if _, ok := findContradiction(a, []*model.Memory{b}); ok {
			t.Fatal("two non-negated memories on the same subject are not a contradiction")
		}
	})

	t.Run("single shared keyword -> below overlap threshold", func(t *testing.T) {
		a := mem("a", "we use pnpm", []string{"pnpm"}, 60*24*time.Hour)
		b := mem("b", "we no longer use pnpm", []string{"pnpm"}, 1*24*time.Hour)
		if _, ok := findContradiction(a, []*model.Memory{b}); ok {
			t.Fatal("a single shared keyword is too weak a subject anchor")
		}
	})

	t.Run("token fallback when keywords empty", func(t *testing.T) {
		a := mem("a", "production database runs postgres replicas", nil, 60*24*time.Hour)
		b := mem("b", "production database no longer runs postgres replicas", nil, 1*24*time.Hour)
		got, ok := findContradiction(b, []*model.Memory{a})
		if !ok || got != "a" {
			t.Fatalf("want contradiction via token fallback, got (%q, %v)", got, ok)
		}
	})
}
