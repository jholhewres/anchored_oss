// Package project derives the stable remote_key used to identify a repository
// across machines and clones. The logic here MUST stay byte-identical to the
// anchored CLI's pkg/project/detector.go (normalizeRemoteURL + deriveRemoteKey):
// the CLI stamps memories with a key derived from `git remote get-url origin`,
// and this package derives the same key from a URL pasted in onboarding / the
// dashboard, so a repo's sync resolves to the project the user created by name.
//
// Parity is enforced by remotekey_test.go using the same vectors as the CLI.
package project

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
)

var (
	reSSHGitPrefix = regexp.MustCompile(`^git@`)
	reWWWPrefix    = regexp.MustCompile(`^www\.`)
)

// NormalizeRemoteURL reduces various git remote URL formats to a canonical form:
//
//	https://github.com/user/repo.git → github.com/user/repo
//	git@github.com:user/repo.git     → github.com/user/repo
//	ssh://git@github.com/user/repo   → github.com/user/repo
//
// Keep this identical to the CLI's normalizeRemoteURL.
func NormalizeRemoteURL(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.TrimRight(s, "/")
	s = strings.TrimSuffix(s, ".git")

	// ssh://git@host/path → host/path
	if strings.HasPrefix(s, "ssh://") {
		s = strings.TrimPrefix(s, "ssh://")
		s = reSSHGitPrefix.ReplaceAllString(s, "")
	}

	// git@host:path → host/path
	if strings.Contains(s, "@") && strings.Contains(s, ":") {
		parts := strings.SplitN(s, "@", 2)
		if len(parts) == 2 {
			rest := parts[1]
			idx := strings.Index(rest, ":")
			if idx >= 0 {
				s = rest[:idx] + "/" + rest[idx+1:]
			} else {
				s = rest
			}
		}
	}

	// https:// or http:// host/path → host/path
	if strings.HasPrefix(s, "https://") {
		s = strings.TrimPrefix(s, "https://")
	} else if strings.HasPrefix(s, "http://") {
		s = strings.TrimPrefix(s, "http://")
	}

	s = reWWWPrefix.ReplaceAllString(s, "")
	s = strings.TrimRight(s, "/")
	s = strings.ToLower(s)

	return s
}

// DeriveRemoteKey returns a stable 16-hex-char SHA-256 prefix from a git remote
// URL, or "" when the URL is empty/normalizes to nothing. Identical to the
// CLI's deriveRemoteKey (which hashes the normalized origin URL).
func DeriveRemoteKey(rawURL string) string {
	normalized := NormalizeRemoteURL(rawURL)
	if normalized == "" {
		return ""
	}
	hash := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(hash[:8])
}
