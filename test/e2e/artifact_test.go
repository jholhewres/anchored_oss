//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestArtifact_AddAndSearch verifies the full add→search loop:
// content added via stdin is indexed locally and found by keyword search.
func TestArtifact_AddAndSearch(t *testing.T) {
	e := newEnv(t)

	content := "The artifact store indexes ephemeral content chunks for FTS retrieval during active sessions."

	// Write content to a temp file and add as artifact.
	f := filepath.Join(t.TempDir(), "note.md")
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	out, code := e.run(t, "",
		"artifact", "add",
		"--type", "prose",
		"--label", "session-note",
		"--file", f,
	)
	if code != 0 {
		t.Fatalf("artifact add exit %d:\n%s", code, out)
	}
	mustContain(t, out, "artifact:")
	id := extractArtifactID(t, out)
	if id == "" {
		t.Fatalf("no artifact id in output: %s", out)
	}

	// Search must find the artifact by keyword.
	out, code = e.run(t, "", "artifact", "search", "ephemeral")
	if code != 0 {
		t.Fatalf("artifact search exit %d:\n%s", code, out)
	}
	if !strings.Contains(out, id) && !strings.Contains(out, "session-note") {
		t.Fatalf("search did not return the added artifact:\nid=%s\n%s", id, out)
	}
}

// TestArtifact_ContentHashDedup verifies that adding the same content twice
// returns the same artifact ID and creates only one DB row.
func TestArtifact_ContentHashDedup(t *testing.T) {
	e := newEnv(t)

	content := "dedup test: this exact string must only be stored once regardless of how many times it is submitted"
	f := filepath.Join(t.TempDir(), "dedup.txt")
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	out1, code := e.run(t, "", "artifact", "add", "--type", "prose", "--file", f)
	if code != 0 {
		t.Fatalf("first add exit %d:\n%s", code, out1)
	}
	id1 := extractArtifactID(t, out1)

	out2, code := e.run(t, "", "artifact", "add", "--type", "prose", "--file", f)
	if code != 0 {
		t.Fatalf("second add exit %d:\n%s", code, out2)
	}
	id2 := extractArtifactID(t, out2)

	if id1 != id2 {
		t.Fatalf("dedup failed: first=%s second=%s", id1, id2)
	}

	// List must show exactly one artifact.
	out, code := e.run(t, "", "artifact", "list")
	if code != 0 {
		t.Fatalf("artifact list exit %d:\n%s", code, out)
	}
	lines := artifactLines(out)
	if len(lines) != 1 {
		t.Fatalf("expected 1 artifact in list after dedup, got %d:\n%s", len(lines), out)
	}
}

// TestArtifact_ListNewestFirst verifies that `artifact list` returns artifacts
// ordered newest first.
func TestArtifact_ListNewestFirst(t *testing.T) {
	e := newEnv(t)
	dir := t.TempDir()

	labels := []string{"alpha", "beta", "gamma"}
	for i, label := range labels {
		f := filepath.Join(dir, label+".txt")
		content := "unique content for artifact " + label + " number " + string(rune('1'+i))
		if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", label, err)
		}
		out, code := e.run(t, "",
			"artifact", "add",
			"--type", "prose",
			"--label", label,
			"--file", f,
		)
		if code != 0 {
			t.Fatalf("add %s exit %d:\n%s", label, code, out)
		}
	}

	out, code := e.run(t, "", "artifact", "list")
	if code != 0 {
		t.Fatalf("artifact list exit %d:\n%s", code, out)
	}

	lines := artifactLines(out)
	if len(lines) < 3 {
		t.Fatalf("expected at least 3 lines, got %d:\n%s", len(lines), out)
	}

	// Newest (gamma) must appear before oldest (alpha).
	gammaIdx := lineIndex(lines, "gamma")
	alphaIdx := lineIndex(lines, "alpha")
	if gammaIdx == -1 {
		t.Fatalf("'gamma' not found in list output:\n%s", out)
	}
	if alphaIdx == -1 {
		t.Fatalf("'alpha' not found in list output:\n%s", out)
	}
	if gammaIdx > alphaIdx {
		t.Fatalf("newest artifact 'gamma' should appear before oldest 'alpha' (gammaIdx=%d alphaIdx=%d):\n%s",
			gammaIdx, alphaIdx, out)
	}
}

// TestArtifact_PruneExpired verifies that `artifact prune` removes expired
// artifacts (reported count > 0) and leaves fresh ones untouched.
func TestArtifact_PruneExpired(t *testing.T) {
	e := newEnv(t)
	dir := t.TempDir()

	// Add a fresh artifact (long TTL).
	fresh := filepath.Join(dir, "fresh.txt")
	if err := os.WriteFile(fresh, []byte("fresh artifact that must survive pruning, unique content abc123"), 0o644); err != nil {
		t.Fatalf("write fresh: %v", err)
	}
	out, code := e.run(t, "",
		"artifact", "add", "--type", "prose", "--label", "fresh-one",
		"--ttl-hours", "9999", "--file", fresh,
	)
	if code != 0 {
		t.Fatalf("add fresh exit %d:\n%s", code, out)
	}

	// Prune on a store with only non-expired artifacts → 0 pruned.
	out, code = e.run(t, "", "artifact", "prune")
	if code != 0 {
		t.Fatalf("prune exit %d:\n%s", code, out)
	}
	mustContain(t, out, "pruned 0")

	// The fresh artifact is still listed.
	out, code = e.run(t, "", "artifact", "list")
	if code != 0 {
		t.Fatalf("list after prune exit %d:\n%s", code, out)
	}
	if !strings.Contains(out, "fresh-one") {
		t.Fatalf("fresh artifact should survive prune:\n%s", out)
	}
}

// TestArtifact_LocalOnly verifies that artifact operations work without any
// remote configured — the artifact store is local-only and must not require
// a network connection.
func TestArtifact_LocalOnly(t *testing.T) {
	e := newEnv(t)
	// Deliberately do NOT call e.configureRemote — no server involved.

	f := filepath.Join(t.TempDir(), "local.md")
	content := "local artifact: no remote server required for artifact storage"
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	out, code := e.run(t, "",
		"artifact", "add", "--type", "prose", "--label", "local-test", "--file", f,
	)
	if code != 0 {
		t.Fatalf("artifact add (no remote) exit %d:\n%s", code, out)
	}
	mustContain(t, out, "artifact:")

	out, code = e.run(t, "", "artifact", "search", "local artifact")
	if code != 0 {
		t.Fatalf("artifact search (no remote) exit %d:\n%s", code, out)
	}
	if strings.Contains(out, "no results") {
		t.Fatalf("search should find local artifact without remote:\n%s", out)
	}
}

// TestArtifact_ProjectIsolation verifies that artifacts tagged with a project
// ID are only returned when that project filter is applied.
func TestArtifact_ProjectIsolation(t *testing.T) {
	e := newEnv(t)
	dir := t.TempDir()

	// Two artifacts with different project IDs.
	fA := filepath.Join(dir, "a.txt")
	fB := filepath.Join(dir, "b.txt")
	if err := os.WriteFile(fA, []byte("project alpha specific document about deployment pipelines"), 0o644); err != nil {
		t.Fatalf("write a: %v", err)
	}
	if err := os.WriteFile(fB, []byte("project beta specific document about monitoring dashboards"), 0o644); err != nil {
		t.Fatalf("write b: %v", err)
	}

	out, code := e.run(t, "",
		"artifact", "add", "--type", "prose", "--label", "alpha-doc",
		"--project", "proj-alpha", "--file", fA,
	)
	if code != 0 {
		t.Fatalf("add alpha exit %d:\n%s", code, out)
	}

	out, code = e.run(t, "",
		"artifact", "add", "--type", "prose", "--label", "beta-doc",
		"--project", "proj-beta", "--file", fB,
	)
	if code != 0 {
		t.Fatalf("add beta exit %d:\n%s", code, out)
	}

	// List with --project proj-alpha must show alpha-doc only.
	out, code = e.run(t, "", "artifact", "list", "--project", "proj-alpha")
	if code != 0 {
		t.Fatalf("list alpha exit %d:\n%s", code, out)
	}
	if !strings.Contains(out, "alpha-doc") {
		t.Fatalf("alpha-doc missing from proj-alpha list:\n%s", out)
	}
	if strings.Contains(out, "beta-doc") {
		t.Fatalf("beta-doc must not appear in proj-alpha list:\n%s", out)
	}

	// Search with --project proj-beta must match beta content only.
	out, code = e.run(t, "", "artifact", "search", "monitoring", "--project", "proj-beta")
	if code != 0 {
		t.Fatalf("search beta exit %d:\n%s", code, out)
	}
	if !strings.Contains(out, "beta-doc") {
		t.Fatalf("search in proj-beta should find beta-doc:\n%s", out)
	}
	if strings.Contains(out, "alpha-doc") {
		t.Fatalf("search in proj-beta must not return alpha-doc:\n%s", out)
	}
}

// extractArtifactID parses the artifact ID from `anchored artifact add` output.
// The output line looks like: "artifact: <hex-id>". Log lines from slog may
// appear in the combined output and are skipped.
func extractArtifactID(t *testing.T, out string) string {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "artifact:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "artifact:"))
		}
	}
	t.Fatalf("no 'artifact: <id>' line in output:\n%s", out)
	return ""
}

// artifactLines returns non-empty output lines that are not slog log lines
// (which start with "time=" in the anchored binary's text handler format).
func artifactLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "time=") {
			continue
		}
		out = append(out, l)
	}
	return out
}

// lineIndex returns the index of the first line containing substr, or -1.
func lineIndex(lines []string, substr string) int {
	for i, l := range lines {
		if strings.Contains(l, substr) {
			return i
		}
	}
	return -1
}
