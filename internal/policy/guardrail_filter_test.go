package policy

import (
	"regexp"
	"testing"
)

// one runs the filter on a single item and returns its result.
func one(f *ContentFilter, item Filterable) FilterResult {
	return f.Filter([]Filterable{item})[0]
}

// TestGuardrailToggles_DisableSecurityRules verifies that the three security
// guardrails are skipped when disabled in the config (admins can turn them off).
func TestGuardrailToggles_DisableSecurityRules(t *testing.T) {
	secret := Filterable{ID: "s", Category: "fact", Content: "key AKIA1234567890ABCDEF here"}
	path := Filterable{ID: "p", Category: "fact", Content: "see /home/jhol/notes.txt"}
	userScoped := Filterable{ID: "u", Category: "fact", Content: "a stable team fact", Metadata: map[string]any{"scope": "user"}}

	// All enabled (legacy default): each is rejected.
	on := NewContentFilterFromConfig(Config{SecretDetection: true, PathRedaction: true, UserScopeBlock: true})
	if one(on, secret).Accepted {
		t.Error("secret should be rejected when secret_detection on")
	}
	if one(on, path).Accepted {
		t.Error("local path should be rejected when path_redaction on")
	}
	if one(on, userScoped).Accepted {
		t.Error("user-scoped should be rejected when user_scope_block on")
	}

	// All disabled: each is accepted (admin turned the guardrails off).
	off := NewContentFilterFromConfig(Config{SecretDetection: false, PathRedaction: false, UserScopeBlock: false})
	if !one(off, secret).Accepted {
		t.Error("secret should pass when secret_detection disabled")
	}
	if !one(off, path).Accepted {
		t.Error("local path should pass when path_redaction disabled")
	}
	if !one(off, userScoped).Accepted {
		t.Error("user-scoped should pass when user_scope_block disabled")
	}
}

// TestGuardrailCategories_EmptyBlocksNothing verifies that an explicit empty
// category list (admin removed all category guardrails) blocks no categories,
// rather than falling back to the event/preference defaults.
func TestGuardrailCategories_EmptyBlocksNothing(t *testing.T) {
	cfg := Config{BlockedCategories: nil, SecretDetection: true, PathRedaction: true, UserScopeBlock: true}
	f := NewContentFilterFromConfig(cfg)
	if !one(f, Filterable{ID: "e", Category: "event", Content: "deployed v2 today and it went fine"}).Accepted {
		t.Error("event must be accepted when no category guardrails are configured")
	}

	// With "event" listed, it is blocked.
	f2 := NewContentFilterFromConfig(Config{BlockedCategories: []string{"event"}})
	if one(f2, Filterable{ID: "e", Category: "event", Content: "deployed v2 today"}).Accepted {
		t.Error("event must be blocked when listed as a category guardrail")
	}
}

// TestGuardrailCustomRules rejects content matching admin-defined regex/keyword.
func TestGuardrailCustomRules(t *testing.T) {
	cfg := Config{
		SecretDetection: true, PathRedaction: true, UserScopeBlock: true,
		CustomRules: []CustomRule{
			{Label: "no jira ids", Re: regexp.MustCompile(`PROJ-\d+`)},
			{Label: "no codename", Re: regexp.MustCompile("(?i)" + regexp.QuoteMeta("Project Falcon"))},
		},
	}
	f := NewContentFilterFromConfig(cfg)

	r := one(f, Filterable{ID: "j", Category: "fact", Content: "fixed in PROJ-1421 last week"})
	if r.Accepted || r.Rule != "custom_rule" {
		t.Errorf("regex custom rule should reject; got accepted=%v rule=%q", r.Accepted, r.Rule)
	}
	r2 := one(f, Filterable{ID: "k", Category: "fact", Content: "the project falcon rollout is on track"})
	if r2.Accepted || r2.Rule != "custom_rule" {
		t.Errorf("keyword custom rule should reject (case-insensitive); got accepted=%v rule=%q", r2.Accepted, r2.Rule)
	}
	// Unrelated content passes.
	if !one(f, Filterable{ID: "ok", Category: "fact", Content: "we standardized on Postgres for durability"}).Accepted {
		t.Error("content not matching any custom rule should be accepted")
	}
}

// TestLegacyFilterDefaults confirms the back-compat constructor keeps all
// security rules on and applies the default blocked categories.
func TestLegacyFilterDefaults(t *testing.T) {
	f := NewContentFilterWithConfig(nil, 0)
	if one(f, Filterable{ID: "e", Category: "event", Content: "deployed today"}).Accepted {
		t.Error("legacy default must still block event")
	}
	if one(f, Filterable{ID: "s", Category: "fact", Content: "AKIA1234567890ABCDEF"}).Accepted {
		t.Error("legacy default must still detect secrets")
	}
}
