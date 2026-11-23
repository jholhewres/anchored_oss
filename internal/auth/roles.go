package auth

// Org roles and the key scopes each may hold, in one place. A role's login
// session and every key minted for the account stay at or below it: admin
// holds every scope; sync, and "member" (what POST /v1/accounts gives new
// accounts), hold sync and readonly; readonly, or any role this table does not
// know, holds readonly only. Unknown roles fail closed everywhere.

// SessionKeyName names the bearer key a password sign-in mints (the
// dashboard's "session key", not the cookie-based web session). It is
// reserved: no API route creates a key with this name.
const SessionKeyName = "session"

// KnownRole reports whether role is one an admin may assign.
func KnownRole(role string) bool {
	switch role {
	case "admin", "sync", "readonly", "member":
		return true
	default:
		return false
	}
}

// InvitableRole reports whether role may be given through an invite: the
// roles whose key scope is the role itself.
func InvitableRole(role string) bool {
	switch role {
	case "admin", "sync", "readonly":
		return true
	default:
		return false
	}
}

// SessionScopeForRole is the key scope of a login session for role.
func SessionScopeForRole(role string) string {
	switch role {
	case "admin":
		return "admin"
	case "sync", "member":
		return "sync"
	default:
		return "readonly"
	}
}

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
