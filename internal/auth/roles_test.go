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
