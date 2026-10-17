package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGeneratedComposeRunsPgvectorOnLoopback: the wizard's compose file must
// run an image with the vector extension (migration 011 needs it) and keep
// the fixed-password database off other hosts.
func TestGeneratedComposeRunsPgvectorOnLoopback(t *testing.T) {
	t.Chdir(t.TempDir())

	dsn, err := GenerateDockerCompose()
	if err != nil {
		t.Fatalf("generate compose: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(".", "docker-compose.yml"))
	if err != nil {
		t.Fatal(err)
	}
	compose := string(raw)
	if !strings.Contains(compose, "image: pgvector/pgvector:pg16") {
		t.Fatalf("compose image lacks pgvector:\n%s", compose)
	}
	if !strings.Contains(compose, `"127.0.0.1:5432:5432"`) {
		t.Fatalf("compose publishes Postgres beyond loopback:\n%s", compose)
	}
	if !strings.Contains(dsn, "@localhost:5432/") {
		t.Fatalf("dsn = %q, want the published loopback port", dsn)
	}
}
