package policy

import "strings"

func containsSecret(content string) (bool, string) {
	secretPrefixes := []string{
		"sk_live_",
		"sk_test_",
		"rk_live_",
		"ghp_",
		"gho_",
		"ghu_",
		"ghs_",
		"xoxb-",
		"xoxp-",
		"hooks.slack.com/services/T",
		"AMAZONS3ACCESSKEY",
		"-----BEGIN PRIVATE KEY-----",
		"-----BEGIN RSA PRIVATE KEY-----",
	}

	for _, prefix := range secretPrefixes {
		if strings.Contains(content, prefix) {
			return true, prefix
		}
	}

	if strings.Contains(content, "AKIA") {
		if idx := strings.Index(content, "AKIA"); idx+20 <= len(content) {
			candidate := content[idx : idx+20]
			if isAlphanumeric(candidate[4:]) {
				return true, "AKIA... (AWS access key)"
			}
		}
	}

	if strings.Contains(content, "AIza") {
		if idx := strings.Index(content, "AIza"); idx+39 <= len(content) {
			candidate := content[idx : idx+39]
			if isAlphanumeric(candidate[4:]) {
				return true, "AIza... (Google API key)"
			}
		}
	}

	connStringChecks := []struct {
		prefix    string
		indicator string
		name      string
	}{
		{"mongodb://", ":", "mongodb:// with credentials"},
		{"postgres://", "@", "postgres:// with credentials"},
		{"postgresql://", "@", "postgresql:// with credentials"},
		{"mysql://", ":", "mysql:// with credentials"},
	}
	for _, check := range connStringChecks {
		if idx := strings.Index(content, check.prefix); idx >= 0 {
			rest := content[idx:]
			if strings.Contains(rest, check.indicator) {
				return true, check.name
			}
		}
	}

	if strings.Contains(content, "redis://:") {
		return true, "redis://: (Redis with password)"
	}

	return false, ""
}

func isAlphanumeric(s string) bool {
	for _, r := range s {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}
