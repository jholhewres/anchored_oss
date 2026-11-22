package auth

// Key scopes an org role may hold. A role's login session and every key minted
// for the account stay at or below it: admin holds every scope; sync, and
// "member" (whose login scope is sync), hold sync and readonly; readonly, or
// any role this table does not know, holds readonly only.

// ScopesAboveRole lists the key scopes role may not hold, padded to two
// entries so callers can bind them as query parameters.
func ScopesAboveRole(role string) [2]string {
	switch role {
	case "admin":
		return [2]string{"", ""}
	case "sync", "member":
		return [2]string{"admin", ""}
	default:
		return [2]string{"admin", "sync"}
	}
}

// ScopeAllowedForRole reports whether a key of scope may belong to an account
// whose org role is role. It is an allow-list: a scope it does not know is
// never allowed.
func ScopeAllowedForRole(role, scope string) bool {
	switch scope {
	case "readonly":
		return true
	case "sync":
		return role == "admin" || role == "sync" || role == "member"
	case "admin":
		return role == "admin"
	default:
		return false
	}
}
