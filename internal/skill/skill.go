// Package skill owns the canonical SKILL.md representation and the small
// built-in template registry. Templates deliberately live in Go so deployment
// never depends on mutable files beside the server binary.
package skill

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	maxFrontmatterBytes = 32 << 10
)

var skillNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// Header returns the complete SKILL.md frontmatter. Codex skills support only
// these two discovery fields; organization/catalog metadata lives outside the
// instruction document.
func Header(name, description string) map[string]any {
	return map[string]any{"name": name, "description": description}
}

// ValidateFrontmatter enforces the actual SKILL.md header contract: name and
// description are required, and no unrelated metadata is allowed.
func ValidateFrontmatter(frontmatter map[string]any) error {
	if frontmatter == nil {
		return fmt.Errorf("frontmatter is required")
	}
	encoded, err := json.Marshal(frontmatter)
	if err != nil {
		return fmt.Errorf("frontmatter must be JSON-compatible: %w", err)
	}
	if len(encoded) > maxFrontmatterBytes {
		return fmt.Errorf("frontmatter exceeds 32 KiB")
	}
	if len(frontmatter) != 2 {
		return fmt.Errorf("frontmatter must contain only name and description")
	}
	name, ok := frontmatter["name"].(string)
	if !ok || !skillNameRe.MatchString(name) {
		return fmt.Errorf("frontmatter name must match ^[a-z0-9][a-z0-9-]{0,63}$")
	}
	description, ok := frontmatter["description"].(string)
	if !ok || strings.TrimSpace(description) == "" {
		return fmt.Errorf("frontmatter description must be a non-empty string")
	}
	return nil
}

// Render produces the one canonical runtime representation for a new skill.
// It normalizes line endings and gives every document exactly one final LF.
func Render(slug, purpose string, frontmatter map[string]any, body string) (string, error) {
	if frontmatter == nil {
		return "", fmt.Errorf("frontmatter is required for canonical rendering")
	}
	if err := ValidateFrontmatter(frontmatter); err != nil {
		return "", err
	}
	if frontmatter["name"] != slug {
		return "", fmt.Errorf("frontmatter name must match skill slug")
	}
	if frontmatter["description"] != purpose {
		return "", fmt.Errorf("frontmatter description must match skill purpose")
	}
	root := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	root.Content = append(root.Content,
		stringNode("name"), stringNode(slug),
		stringNode("description"), stringNode(purpose),
	)
	encoded, err := yaml.Marshal(&yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}})
	if err != nil {
		return "", fmt.Errorf("marshal frontmatter: %w", err)
	}
	body = strings.ReplaceAll(strings.ReplaceAll(body, "\r\n", "\n"), "\r", "\n")
	body = strings.TrimRight(body, "\n")
	return "---\n" + string(encoded) + "---\n" + body + "\n", nil
}

func stringNode(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
}

// ParseJSONFrontmatter converts a request or database JSON value into the
// validated header used by Render.
func ParseJSONFrontmatter(raw []byte) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	var value map[string]any
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("frontmatter must be a JSON object: %w", err)
	}
	if value == nil {
		return nil, fmt.Errorf("frontmatter must be a JSON object")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("frontmatter must contain one JSON object")
	}
	if err := ValidateFrontmatter(value); err != nil {
		return nil, err
	}
	return value, nil
}

// JSON returns a stable JSON representation suitable for SQLite TEXT and
// Postgres JSONB writes.
func JSON(frontmatter map[string]any) ([]byte, error) {
	if err := ValidateFrontmatter(frontmatter); err != nil {
		return nil, err
	}
	return json.Marshal(frontmatter)
}

// Template is a built-in starting point. It has no persistence identity;
// creation copies its values into an organization-owned skill.
type Template struct {
	ID          string         `json:"id"`
	Revision    string         `json:"revision"`
	Name        string         `json:"name"`
	Slug        string         `json:"slug"`
	Purpose     string         `json:"purpose"`
	Frontmatter map[string]any `json:"frontmatter"`
	Body        string         `json:"body"`
}

var templates = []Template{
	{ID: "code-review/v1", Revision: "v1", Name: "Code review", Slug: "code-review", Purpose: "Review changes for correctness, security, and maintainability.", Frontmatter: Header("code-review", "Review changes for correctness, security, and maintainability."), Body: "# Review\n\nIdentify concrete risks, explain their impact, and propose focused fixes."},
	{ID: "test-strategy/v1", Revision: "v1", Name: "Test strategy", Slug: "test-strategy", Purpose: "Design focused tests that prove the requested behavior.", Frontmatter: Header("test-strategy", "Design focused tests that prove the requested behavior."), Body: "# Test strategy\n\nCover the happy path, failure cases, and regressions before declaring completion."},
	{ID: "business-rules/v1", Revision: "v1", Name: "Business rules", Slug: "business-rules", Purpose: "Apply product rules consistently while implementing changes.", Frontmatter: Header("business-rules", "Apply product rules consistently while implementing changes."), Body: "# Business rules\n\nState the relevant rule, enforce it at the boundary, and keep exceptions explicit."},
}

// Templates returns independent copies in stable ID/revision order.
func Templates() []Template {
	out := make([]Template, len(templates))
	for i, template := range templates {
		out[i] = template
		out[i].Frontmatter = cloneMap(template.Frontmatter)
	}
	return out
}

func FindTemplate(id, revision string) (Template, bool) {
	for _, template := range templates {
		if template.ID == id && (revision == "" || template.Revision == revision) {
			template.Frontmatter = cloneMap(template.Frontmatter)
			return template, true
		}
	}
	return Template{}, false
}

func cloneMap(source map[string]any) map[string]any {
	clone := make(map[string]any, len(source))
	for key, value := range source {
		clone[key] = cloneValue(value)
	}
	return clone
}

func cloneValue(value any) any {
	switch value := value.(type) {
	case map[string]any:
		return cloneMap(value)
	case []any:
		clone := make([]any, len(value))
		for i, item := range value {
			clone[i] = cloneValue(item)
		}
		return clone
	default:
		return value
	}
}
