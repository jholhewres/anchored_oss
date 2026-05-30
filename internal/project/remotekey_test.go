package project

import "testing"

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
	}
}
