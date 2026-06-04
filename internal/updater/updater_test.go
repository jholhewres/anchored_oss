package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/jholhewres/anchored_oss/internal/version"
)

// versionServer serves a one-line VERSION.md body.
func versionServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
}

func newUpdater(versionURL string) *Updater {
	return &Updater{
		VersionURL: versionURL,
		HTTPClient: http.DefaultClient,
	}
}

func TestCheckLatest_NewerAvailable(t *testing.T) {
	orig := version.Version
	version.Version = "v0.4.6"
	defer func() { version.Version = orig }()

	srv := versionServer(t, "v0.5.0\nrelease notes\n")
	defer srv.Close()

	cur, latest, available, err := newUpdater(srv.URL).CheckLatest(context.Background())
	if err != nil {
		t.Fatalf("CheckLatest: %v", err)
	}
	if cur != "v0.4.6" || latest != "v0.5.0" {
		t.Fatalf("versions = %q/%q, want v0.4.6/v0.5.0", cur, latest)
	}
	if !available {
		t.Error("expected update available")
	}
}

func TestCheckLatest_SameVersion(t *testing.T) {
	orig := version.Version
	version.Version = "v0.5.0"
	defer func() { version.Version = orig }()

	srv := versionServer(t, "v0.5.0\n")
	defer srv.Close()

	_, _, available, err := newUpdater(srv.URL).CheckLatest(context.Background())
	if err != nil {
		t.Fatalf("CheckLatest: %v", err)
	}
	if available {
		t.Error("expected no update for identical version")
	}
}

func TestCheckLatest_OlderPublished(t *testing.T) {
	orig := version.Version
	version.Version = "v0.6.0"
	defer func() { version.Version = orig }()

	srv := versionServer(t, "v0.5.0\n")
	defer srv.Close()

	_, _, available, err := newUpdater(srv.URL).CheckLatest(context.Background())
	if err != nil {
		t.Fatalf("CheckLatest: %v", err)
	}
	if available {
		t.Error("expected no update when published version is older")
	}
}

func TestCheckLatest_UnparseableCurrent(t *testing.T) {
	orig := version.Version
	version.Version = "dev"
	defer func() { version.Version = orig }()

	srv := versionServer(t, "v0.5.0\n")
	defer srv.Close()

	cur, latest, available, err := newUpdater(srv.URL).CheckLatest(context.Background())
	if err != nil {
		t.Fatalf("CheckLatest: %v", err)
	}
	if cur != "dev" || latest != "v0.5.0" {
		t.Fatalf("versions = %q/%q", cur, latest)
	}
	if available {
		t.Error("dev (unparseable current) must conservatively report no update")
	}
}

// TestApply_NoUpdateReturnsSentinel proves Apply early-returns ErrNoUpdate
// (touching no binary) when CheckLatest reports nothing newer is available, so
// the handler can map it to a benign 409 instead of attempting a swap.
func TestApply_NoUpdateReturnsSentinel(t *testing.T) {
	orig := version.Version
	version.Version = "v0.6.0" // newer than the published v0.5.0 -> not available
	defer func() { version.Version = orig }()

	srv := versionServer(t, "v0.5.0\n")
	defer srv.Close()

	err := newUpdater(srv.URL).Apply(context.Background())
	if !errors.Is(err, ErrNoUpdate) {
		t.Fatalf("Apply = %v, want ErrNoUpdate", err)
	}
}

func TestVerifySHA256(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "asset.bin")
	content := []byte("anchored-oss release payload")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	sum := sha256.Sum256(content)
	good := hex.EncodeToString(sum[:])

	if err := verifySHA256(path, good); err != nil {
		t.Errorf("verifySHA256 should pass for correct digest: %v", err)
	}
	// Case-insensitive match.
	if err := verifySHA256(path, "DEADBEEF"); err == nil {
		t.Error("verifySHA256 should fail for wrong digest")
	}
}

func TestChecksumFor(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "checksums-sha256.txt")
	body := "abc123  anchored-oss-selfhosted-linux-amd64\n" +
		"def456 *anchored-oss-selfhosted-darwin-arm64\n"
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatalf("write sums: %v", err)
	}

	got, err := checksumFor(path, "anchored-oss-selfhosted-linux-amd64")
	if err != nil {
		t.Fatalf("checksumFor: %v", err)
	}
	if got != "abc123" {
		t.Errorf("digest = %q, want abc123", got)
	}
	// The "*" binary-mode marker must be tolerated.
	got, err = checksumFor(path, "anchored-oss-selfhosted-darwin-arm64")
	if err != nil {
		t.Fatalf("checksumFor (star): %v", err)
	}
	if got != "def456" {
		t.Errorf("digest = %q, want def456", got)
	}
	if _, err := checksumFor(path, "anchored-oss-selfhosted-windows-amd64.exe"); err == nil {
		t.Error("expected error for missing entry")
	}
}

func TestParseSemver(t *testing.T) {
	cases := []struct {
		in    string
		ok    bool
		major int
	}{
		{"v0.4.6", true, 0},
		{"1.2.3", true, 1},
		{"v0.5.0-rc1", true, 0},
		{"v0.5.0+build7", true, 0},
		{"dev", false, 0},
		{"v1.2", false, 0},
		{"", false, 0},
	}
	for _, c := range cases {
		sv, ok := parseSemver(c.in)
		if ok != c.ok {
			t.Errorf("parseSemver(%q) ok = %v, want %v", c.in, ok, c.ok)
			continue
		}
		if ok && sv.major != c.major {
			t.Errorf("parseSemver(%q) major = %d, want %d", c.in, sv.major, c.major)
		}
	}
}
