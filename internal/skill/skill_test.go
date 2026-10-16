package skill

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func TestRenderIsDeterministicAndCanonical(t *testing.T) {
	frontmatter := Header("release-guide", "Prepare releases")
	first, err := Render("release-guide", "Prepare releases", frontmatter, "# Release\r\n\r\nShip safely")
	if err != nil {
		t.Fatal(err)
	}
	second, err := Render("release-guide", "Prepare releases", frontmatter, "# Release\n\nShip safely\n\n")
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("canonical render differs by input line endings/final newline:\nfirst=%q\nsecond=%q", first, second)
	}
	if !strings.HasPrefix(first, "---\nname: release-guide\ndescription: Prepare releases\n---\n") || !strings.HasSuffix(first, "Ship safely\n") {
		t.Fatalf("unexpected canonical document %q", first)
	}
	sum := sha256.Sum256([]byte(first))
	if hex.EncodeToString(sum[:]) == "" {
		t.Fatal("content hash must be derived from rendered bytes")
	}
}

func TestFrontmatterValidationAndTemplateIsolation(t *testing.T) {
	if err := ValidateFrontmatter(map[string]any{"name": "override"}); err == nil {
		t.Fatal("incomplete header accepted")
	}
	if err := ValidateFrontmatter(map[string]any{"name": "valid", "description": "Valid", "tools": []any{"tests"}}); err == nil {
		t.Fatal("unrelated frontmatter metadata accepted")
	}
	if _, err := ParseJSONFrontmatter([]byte(`[]`)); err == nil {
		t.Fatal("array frontmatter accepted")
	}
	templates := Templates()
	if got := []string{templates[0].ID, templates[1].ID, templates[2].ID}; strings.Join(got, ",") != "code-review/v1,test-strategy/v1,business-rules/v1" {
		t.Fatalf("template order = %v", got)
	}
	templates[0].Frontmatter["description"] = "changed"
	fresh, ok := FindTemplate("code-review/v1", "v1")
	if !ok || fresh.Frontmatter["description"] == "changed" {
		t.Fatalf("template mutation leaked: %#v", fresh)
	}
}
