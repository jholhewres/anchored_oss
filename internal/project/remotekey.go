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
	// reHostPort matches a host that carries an explicit numeric port, capturing
	// the host and the optional path so the port can be dropped.
	reHostPort = regexp.MustCompile(`^([^/:]+):\d+(/.*)?$`)
)

// normalizeRemoteURLLegacy is the v1 normalization, frozen verbatim. It is the
// shared core of both the legacy and the canonical (v2) algorithms; v2 layers
// two extra reductions on top (see NormalizeRemoteURL). Keeping the legacy form
// available lets the server resolve repos created before v2 by their old key.
func normalizeRemoteURLLegacy(raw string) string {
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

// NormalizeRemoteURL reduces various git remote URL formats to a canonical (v2)
// form:
//
//	https://github.com/user/repo.git                   → github.com/user/repo
//	git@github.com:user/repo.git                       → github.com/user/repo
//	ssh://git@github.com/user/repo                     → github.com/user/repo
//	ssh://git@bitbucket.example.com:7999/proj/repo.git → bitbucket.example.com/proj/repo
//	https://bitbucket.example.com/scm/proj/repo.git    → bitbucket.example.com/proj/repo
//
// v2 = the legacy pipeline plus two reductions applied before the final return:
// (a) strip a numeric host port, and (b) strip a leading "scm/" path segment
// (self-hosted git servers embed both). Keep this identical to the CLI's
// normalizeRemoteURL.
func NormalizeRemoteURL(raw string) string {
	s := normalizeRemoteURLLegacy(raw)

	// (a) strip numeric host port: host:1234/path → host/path
	if m := reHostPort.FindStringSubmatch(s); m != nil {
		s = m[1] + m[2]
	}

	// (b) strip a leading "scm/" path segment (first segment only):
	// host/scm/rest → host/rest. Deeper "scm" segments are left untouched.
	if slash := strings.Index(s, "/"); slash >= 0 {
		host := s[:slash]
		rest := s[slash+1:]
		if rest == "scm" {
			s = host
		} else if strings.HasPrefix(rest, "scm/") {
			s = host + "/" + strings.TrimPrefix(rest, "scm/")
		}
	}

	return s
}

// DeriveRemoteKey returns a stable 16-hex-char SHA-256 prefix from a git remote
// URL, or "" when the URL is empty/normalizes to nothing. Identical to the
// CLI's deriveRemoteKey (which hashes the canonical normalized origin URL).
func DeriveRemoteKey(rawURL string) string {
	return deriveKey(NormalizeRemoteURL(rawURL))
}

// DeriveLegacyRemoteKey returns the key a pre-v2 server/CLI would have derived
// for the same URL (legacy normalization). Stored as remote_key_v1 so repos
// created before the v2 change still resolve by their old key.
func DeriveLegacyRemoteKey(rawURL string) string {
	return deriveKey(normalizeRemoteURLLegacy(rawURL))
}

func deriveKey(normalized string) string {
	if normalized == "" {
		return ""
	}
	hash := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(hash[:8])
}
