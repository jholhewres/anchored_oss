package policy

import "testing"

func TestContainsLocalPath(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{"linux home", "/home/alice/project", true},
		{"macos home", "/Users/bob/.config", true},
		{"windows home backslash", `C:\Users\bob\file`, true},
		{"windows home forward", "C:/Users/bob/file", true},
		{"home relative", "~/projects/app", true},
		{"linux tmp", "/tmp/build-output", true},
		{"macos cache", "/var/folders/xyz/cache", true},
		{"windows temp", "%TEMP%\\build", true},
		{"windows tmp env", "%TMP%\\build", true},
		{"linux var tmp", "/var/tmp/something", true},
		{"macos private tmp", "/private/tmp/launchd.sock", true},
		{"windows system backslash", `C:\Windows\System32`, true},
		{"windows system forward", "C:/Windows/System32", true},
		{"safe relative", "pkg/memory/service.go", false},
		{"safe package", "internal/config/config.go", false},
		{"safe decision text", "We decided to use Postgres for storage", false},
		{"safe empty", "", false},
		{"home in word", "homepage is at example.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			found, _ := containsLocalPath(tt.content)
			if found != tt.expected {
				t.Errorf("containsLocalPath(%q) = %v, want %v", tt.content, found, tt.expected)
			}
		})
	}
}

func TestContainsSecret(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{"stripe live key", "sk_live_abc123def456", true},
		{"stripe test key", "sk_test_abc123def456", true},
		{"stripe restricted key", "rk_live_abc123def456", true},
		{"aws key", "AKIAIOSFODNN7EXAMPLE", true},
		{"private key", "-----BEGIN PRIVATE KEY-----\nMIIE...", true},
		{"rsa key", "-----BEGIN RSA PRIVATE KEY-----\nMIIE...", true},
		{"github pat", "ghp_ABCDEFGHIJKLMNOP", true},
		{"github oauth", "gho_ABCDEFGHIJKLMNOP", true},
		{"github user token", "ghu_ABCDEFGHIJKLMNOP", true},
		{"github server token", "ghs_ABCDEFGHIJKLMNOP", true},
		{"slack bot", "xoxb-1234567890-abcdef", true},
		{"slack user", "xoxp-1234567890-abcdef", true},
		{"slack webhook", "hooks.slack.com/services/T12345/B1234/xyz", true},
		{"mongo uri", "mongodb://user:pass@host:27017/db", true},
		{"postgres uri", "postgres://admin:secret@localhost:5432/db", true},
		{"postgresql uri", "postgresql://admin:secret@localhost:5432/db", true},
		{"mysql uri", "mysql://root:password@127.0.0.1:3306/db", true},
		{"redis with password", "redis://:mypassword@localhost:6379/0", true},
		{"amazon s3 key", "AMAZONS3ACCESSKEY", true},
		{"google api key", "AIzaSyA1234567890abcdefghijklmnopqrstuv", true},
		{"safe text about postgres", "We use Postgres with pgx driver", false},
		{"safe port config", "The server runs on port 8080", false},
		{"safe empty", "", false},
		{"safe akia without enough chars", "AKIA", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			found, _ := containsSecret(tt.content)
			if found != tt.expected {
				t.Errorf("containsSecret(%q) = %v, want %v", tt.content, found, tt.expected)
			}
		})
	}
}

func TestFilter_Accepted(t *testing.T) {
	filter := NewContentFilter()
	items := []Filterable{
		{ID: "1", Content: "Postgres uses MVCC for concurrency", Category: "fact"},
		{ID: "2", Content: "We chose API keys over JWT", Category: "decision"},
		{ID: "3", Content: "The team prefers Go over Rust for this service", Category: "learning"},
	}
	results := filter.Filter(items)

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	for i, r := range results {
		if !r.Accepted {
			t.Errorf("result[%d] (id=%s): expected accepted=true, got rejected (rule=%s, detail=%s)", i, r.ID, r.Rule, r.Detail)
		}
	}
}

func TestFilter_BlockedCategory(t *testing.T) {
	filter := NewContentFilter()

	tests := []struct {
		name     string
		category string
		blocked  bool
	}{
		{"event blocked", "event", true},
		{"preference blocked", "preference", true},
		{"fact allowed", "fact", false},
		{"decision allowed", "decision", false},
		{"learning allowed", "learning", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items := []Filterable{
				{ID: "1", Content: "some safe content", Category: tt.category},
			}
			results := filter.Filter(items)
			if len(results) != 1 {
				t.Fatalf("expected 1 result, got %d", len(results))
			}
			if results[0].Accepted == tt.blocked {
				t.Errorf("category=%q: accepted=%v, want blocked=%v", tt.category, results[0].Accepted, tt.blocked)
			}
			if tt.blocked && results[0].Rule != "blocked_category" {
				t.Errorf("category=%q: expected rule=blocked_category, got %s", tt.category, results[0].Rule)
			}
		})
	}
}

func TestFilter_LocalPathRejected(t *testing.T) {
	filter := NewContentFilter()
	items := []Filterable{
		{ID: "1", Content: "/home/alice/project/main.go", Category: "fact"},
	}
	results := filter.Filter(items)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Accepted {
		t.Error("expected item with local path to be rejected")
	}
	if results[0].Rule != "local_path_detected" {
		t.Errorf("expected rule=local_path_detected, got %s", results[0].Rule)
	}
}

func TestFilter_SecretRejected(t *testing.T) {
	filter := NewContentFilter()
	items := []Filterable{
		{ID: "1", Content: "password is sk_live_abc123", Category: "fact"},
	}
	results := filter.Filter(items)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Accepted {
		t.Error("expected item with secret to be rejected")
	}
	if results[0].Rule != "secret_detected" {
		t.Errorf("expected rule=secret_detected, got %s", results[0].Rule)
	}
}

func TestFilter_CategoryTakesPrecedence(t *testing.T) {
	filter := NewContentFilter()
	items := []Filterable{
		{ID: "1", Content: "sk_live_super_secret_key_here", Category: "event"},
	}
	results := filter.Filter(items)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Accepted {
		t.Error("expected blocked category item to be rejected")
	}
	if results[0].Rule != "blocked_category" {
		t.Errorf("expected blocked_category to take precedence, got %s", results[0].Rule)
	}
}

func TestFilter_EmptyInput(t *testing.T) {
	filter := NewContentFilter()
	results := filter.Filter([]Filterable{})

	if len(results) != 0 {
		t.Errorf("expected 0 results for empty input, got %d", len(results))
	}
}

func TestFilter_MixedItems(t *testing.T) {
	filter := NewContentFilter()
	items := []Filterable{
		{ID: "1", Content: "Safe fact content", Category: "fact"},
		{ID: "2", Content: "/home/user/secret.txt", Category: "fact"},
		{ID: "3", Content: "sk_live_abc123", Category: "fact"},
		{ID: "4", Content: "User logged in", Category: "event"},
		{ID: "5", Content: "Another safe decision", Category: "decision"},
	}
	results := filter.Filter(items)

	if len(results) != 5 {
		t.Fatalf("expected 5 results, got %d", len(results))
	}

	expected := []struct {
		accepted bool
		rule     string
	}{
		{true, ""},
		{false, "local_path_detected"},
		{false, "secret_detected"},
		{false, "blocked_category"},
		{true, ""},
	}

	for i, exp := range expected {
		if results[i].Accepted != exp.accepted {
			t.Errorf("item[%d] (id=%s): accepted=%v, want %v", i, results[i].ID, results[i].Accepted, exp.accepted)
		}
		if !exp.accepted && results[i].Rule != exp.rule {
			t.Errorf("item[%d] (id=%s): rule=%q, want %q", i, results[i].ID, results[i].Rule, exp.rule)
		}
	}
}
