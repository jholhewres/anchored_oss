package project

import "testing"

// TestNormalizeRemoteURLCanonical locks the v2 normalization vectors. These
// MUST stay byte-identical to the anchored CLI's normalizeRemoteURL: the CLI
// stamps memories with a key derived from this form, and the server derives the
// same key from a pasted URL, so a repo's sync resolves to the right project.
// Hosts here are generic on purpose so the vectors never embed a real vendor.
func TestNormalizeRemoteURLCanonical(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"https://github.com/user/repo.git", "github.com/user/repo"},
		{"git@github.com:user/repo.git", "github.com/user/repo"},
		{"ssh://git@github.com/user/repo", "github.com/user/repo"},
		{"ssh://git@bitbucket.example.com:7999/proj/repo.git", "bitbucket.example.com/proj/repo"},
		{"https://bitbucket.example.com/scm/proj/repo.git", "bitbucket.example.com/proj/repo"},
		{"ssh://git@gitlab.example.com:2222/group/sub/repo.git", "gitlab.example.com/group/sub/repo"},
		{"http://www.example.com/team/repo/", "example.com/team/repo"},
		// "scm" is only stripped as the FIRST path segment; a deeper "scm" stays.
		{"https://example.com/x/scm/y.git", "example.com/x/scm/y"},
	}
	for _, c := range cases {
		if got := NormalizeRemoteURL(c.in); got != c.want {
			t.Errorf("NormalizeRemoteURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestNormalizeRemoteURLLegacy locks the frozen v1 normalization. The legacy
// form keeps the host port and the "scm/" segment, so repos keyed before the v2
// change still resolve via remote_key_v1.
func TestNormalizeRemoteURLLegacy(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"ssh://git@bitbucket.example.com:7999/proj/repo.git", "bitbucket.example.com:7999/proj/repo"},
		{"https://bitbucket.example.com/scm/proj/repo.git", "bitbucket.example.com/scm/proj/repo"},
	}
	for _, c := range cases {
		if got := normalizeRemoteURLLegacy(c.in); got != c.want {
			t.Errorf("normalizeRemoteURLLegacy(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestDeriveRemoteKeyCanonicalEquivalence proves the v2 key collapses the ssh
// and the https /scm/ forms of the same Bitbucket Server repo to a single key,
// which is the whole point of the port + scm reductions.
func TestDeriveRemoteKeyCanonicalEquivalence(t *testing.T) {
	ssh := DeriveRemoteKey("ssh://git@bitbucket.example.com:7999/proj/repo.git")
	https := DeriveRemoteKey("https://bitbucket.example.com/scm/proj/repo.git")
	if ssh == "" {
		t.Fatal("ssh key is empty")
	}
	if ssh != https {
		t.Errorf("canonical keys must match: ssh=%q https=%q", ssh, https)
	}
	// The key is hex(sha256(normalized))[:16] = 16 hex chars.
	if len(ssh) != 16 {
		t.Errorf("key length = %d, want 16", len(ssh))
	}
}

// TestDeriveRemoteKeyParity locks this server-side derivation to the anchored
// CLI's. The expected keys are produced by the CLI's deriveRemoteKey for the
// same URLs (see anchored/pkg/project/detector.go). If either side changes its
// normalization, these vectors break and the repo<->project match silently
// drifts — so the constants are intentionally hard-coded, not computed.
func TestDeriveRemoteKeyParity(t *testing.T) {
	cases := []struct {
		urls []string
		want string
	}{
		{
			want: "7b355865a2042946",
			urls: []string{
				"git@github.com:jholhewres/anchored.git",
				"https://github.com/jholhewres/anchored.git",
				"https://github.com/jholhewres/anchored",
				"ssh://git@github.com/jholhewres/anchored.git",
				"git@github.com:jholhewres/anchored",
				"https://github.com/jholhewres/anchored/",
				"https://github.com/Jholhewres/Anchored.git",
			},
		},
		{
			want: "65c462e45b4e9426",
			urls: []string{
				"git@gitlab.com:group/sub/proj.git",
				"https://gitlab.com/group/sub/proj.git",
				"ssh://git@gitlab.com/group/sub/proj",
			},
		},
	}

	for _, c := range cases {
		for _, u := range c.urls {
			if got := DeriveRemoteKey(u); got != c.want {
				t.Errorf("DeriveRemoteKey(%q) = %q, want %q (ssh/https must match the CLI)", u, got, c.want)
			}
		}
	}
}

func TestDeriveRemoteKeyEmpty(t *testing.T) {
	for _, u := range []string{"", "   ", ".git", "/"} {
		if got := DeriveRemoteKey(u); got != "" {
			t.Errorf("DeriveRemoteKey(%q) = %q, want empty", u, got)
		}
		if got := DeriveLegacyRemoteKey(u); got != "" {
			t.Errorf("DeriveLegacyRemoteKey(%q) = %q, want empty", u, got)
		}
	}
}

func TestIsValidGitRemote(t *testing.T) {
	tests := []struct {
		remote string
		valid  bool
	}{
		{"https://github.com/acme/repo.git", true},
		{"git@github.com:acme/repo.git", true},
		{"ssh://git@github.com/acme/repo.git", true},
		{"ssh://git@bitbucket.example.com:7999/acme/repo.git", true},
		{"", false},
		{"   ", false},
		{"github.com/acme/repo", false},
		{"https://github.com", false},
		{"http://github.com/acme/repo", false},
		{"https://token@example.com/acme/repo", true},
		{"https://user:secret@example.com/acme/repo", false},
		{"https://@example.com/acme/repo", false},
		{"HTTPS://github.com/acme/repo.git", false},
		{"git@github.com:acme/repo\x00.git", false},
		{"git:secret@github.com:acme/repo.git", false},
		{"ssh://alice@github.com/acme/repo.git", false},
	}
	for _, tt := range tests {
		if got := IsValidGitRemote(tt.remote); got != tt.valid {
			t.Errorf("IsValidGitRemote(%q) = %t, want %t", tt.remote, got, tt.valid)
		}
	}
}

// TestCanonicalGitRemote_DropsTheHTTPSUsername: a clone URL with a bare
// username is stored without it and derives the same key as the plain one.
func TestCanonicalGitRemote_DropsTheHTTPSUsername(t *testing.T) {
	got, ok := CanonicalGitRemote(" https://jdoe@bitbucket.org/acme/repo.git ")
	if !ok || got != "https://bitbucket.org/acme/repo.git" {
		t.Fatalf("CanonicalGitRemote = %q, %t; want the URL without its username", got, ok)
	}
	if DeriveRemoteKey(got) != DeriveRemoteKey("git@bitbucket.org:acme/repo.git") {
		t.Fatalf("stored remote derives a different key than the SSH form")
	}
	if got, ok := CanonicalGitRemote("git@github.com:acme/repo.git"); !ok || got != "git@github.com:acme/repo.git" {
		t.Fatalf("scp remote = %q, %t; want it unchanged", got, ok)
	}
}

// TestCanonicalGitRemote_RefusesInvisibleCharactersAndPersonalSSHUsers
// (attack h2 and h3, run 20261123T002801Z): a stored remote reads as the
// repository it names, and keeps no personal SSH username.
func TestCanonicalGitRemote_RefusesInvisibleCharactersAndPersonalSSHUsers(t *testing.T) {
	for _, remote := range []string{
		"git@github.com:acme/re\u0085po.git",
		"git@github.com:acme/re\u202epo.git",
		"https://github.com/acme/re\u200bpo.git",
		"https://github.com/acme/re\u00a0po.git",
		"jdoe@bitbucket.org:acme/repo.git",
		"git:secret@github.com:acme/repo.git",
	} {
		if got, ok := CanonicalGitRemote(remote); ok {
			t.Errorf("CanonicalGitRemote(%q) = %q, true; want refused", remote, got)
		}
	}
	if got, ok := CanonicalGitRemote("git@bitbucket.org:acme/réalisé.git"); !ok || got != "git@bitbucket.org:acme/réalisé.git" {
		t.Errorf("a printable non-ASCII path = %q, %t; want accepted", got, ok)
	}
}
