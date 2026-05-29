package policy

import "testing"

func TestNewContentFilterWithConfig_CustomBlocked(t *testing.T) {
	// Override defaults: block "fact", allow the default "event".
	f := NewContentFilterWithConfig([]string{"fact"}, 0.5)
	res := f.Filter([]Filterable{
		{ID: "1", Category: "fact", Content: "anything"},
		{ID: "2", Category: "event", Content: "a longer event description that is articulate"},
	})
	if res[0].Accepted {
		t.Fatal("custom-blocked category 'fact' should be rejected")
	}
	if res[0].Rule != "blocked_category" {
		t.Fatalf("expected blocked_category, got %q", res[0].Rule)
	}
	if !res[1].Accepted {
		t.Fatal("'event' should be accepted when not in the custom blocked set")
	}
}

func TestNewContentFilterWithConfig_Defaults(t *testing.T) {
	// Empty config falls back to defaults (event/preference blocked, 0.55).
	f := NewContentFilterWithConfig(nil, 0)
	res := f.Filter([]Filterable{{ID: "1", Category: "event", Content: "x"}})
	if res[0].Accepted {
		t.Fatal("default config should still block 'event'")
	}
}

func TestNewContentFilterWithConfig_QualityThreshold(t *testing.T) {
	// A strict threshold rejects a mid-quality memory that the default accepts.
	strict := NewContentFilterWithConfig([]string{"event", "preference"}, 0.9)
	lenient := NewContentFilterWithConfig([]string{"event", "preference"}, 0.3)
	item := Filterable{ID: "1", Category: "fact", Content: "some durable fact",
		Metadata: map[string]any{"quality_score": 0.6}}
	if strict.Filter([]Filterable{item})[0].Accepted {
		t.Fatal("strict threshold (0.9) should reject score 0.6")
	}
	if !lenient.Filter([]Filterable{item})[0].Accepted {
		t.Fatal("lenient threshold (0.3) should accept score 0.6")
	}
}
