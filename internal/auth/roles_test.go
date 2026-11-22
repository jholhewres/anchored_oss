package auth

import "testing"

func TestScopeAllowedForRole(t *testing.T) {
	for _, tc := range []struct {
		role, scope string
		want        bool
	}{
		{"admin", "admin", true}, {"admin", "sync", true}, {"admin", "readonly", true},
		{"sync", "admin", false}, {"sync", "sync", true}, {"sync", "readonly", true},
		{"member", "admin", false}, {"member", "sync", true}, {"member", "readonly", true},
		{"readonly", "admin", false}, {"readonly", "sync", false}, {"readonly", "readonly", true},
		{"unknown", "sync", false}, {"unknown", "readonly", true},
		{"admin", "", false},
		{"admin", "superuser", false}, {"sync", "owner", false},
	} {
		if got := ScopeAllowedForRole(tc.role, tc.scope); got != tc.want {
			t.Errorf("ScopeAllowedForRole(%q, %q) = %t, want %t", tc.role, tc.scope, got, tc.want)
		}
	}
}

// TestSessionScopeAgreesWithTheRoleCeiling: a login session never holds a
// scope its role may not, unknown roles included.
func TestSessionScopeAgreesWithTheRoleCeiling(t *testing.T) {
	for role, want := range map[string]string{"admin": "admin", "sync": "sync", "member": "sync", "readonly": "readonly", "owner": "readonly", "": "readonly"} {
		got := SessionScopeForRole(role)
		if got != want {
			t.Errorf("SessionScopeForRole(%q) = %q, want %q", role, got, want)
		}
		if !ScopeAllowedForRole(role, got) {
			t.Errorf("session scope %q is above role %q", got, role)
		}
	}
	for _, role := range []string{"admin", "sync", "readonly", "member"} {
		if !KnownRole(role) {
			t.Errorf("KnownRole(%q) = false", role)
		}
	}
	if KnownRole("owner") {
		t.Error("KnownRole(owner) = true")
	}
}
