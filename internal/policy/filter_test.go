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

func TestFilter_LifecycleUserScopeRejected(t *testing.T) {
	filter := NewContentFilter()
	items := []Filterable{
		{ID: "1", Content: "safe content", Category: "fact", Metadata: map[string]any{"scope": "user"}},
	}
	results := filter.Filter(items)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Accepted {
		t.Error("user-scope memory should be rejected")
	}
	if results[0].Rule != "lifecycle_user_scope" {
		t.Errorf("rule: got %q, want lifecycle_user_scope", results[0].Rule)
	}
}

func TestFilter_LifecycleOperationalRejected(t *testing.T) {
	filter := NewContentFilter()
	items := []Filterable{
		{ID: "1", Content: "safe content", Category: "fact", Metadata: map[string]any{"memory_type": "operational"}},
	}
	results := filter.Filter(items)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Accepted {
		t.Error("operational memory should be rejected")
	}
	if results[0].Rule != "lifecycle_operational" {
		t.Errorf("rule: got %q, want lifecycle_operational", results[0].Rule)
	}
}

func TestFilter_LifecyclePrecompactRejected(t *testing.T) {
	filter := NewContentFilter()
	items := []Filterable{
		{ID: "1", Content: "safe content", Category: "fact", Metadata: map[string]any{"origin": "precompact"}},
	}
	results := filter.Filter(items)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Accepted {
		t.Error("precompact origin should be rejected")
	}
	if results[0].Rule != "lifecycle_local_origin" {
		t.Errorf("rule: got %q, want lifecycle_local_origin", results[0].Rule)
	}
}

func TestFilter_LifecycleHandoffRejected(t *testing.T) {
	filter := NewContentFilter()
	items := []Filterable{
		{ID: "1", Content: "safe content", Category: "fact", Metadata: map[string]any{"origin": "handoff"}},
	}
	results := filter.Filter(items)
	if results[0].Accepted {
		t.Error("handoff origin should be rejected")
	}
}

func TestFilter_LifecycleSemanticProjectAccepted(t *testing.T) {
	filter := NewContentFilter()
	items := []Filterable{
		{ID: "1", Content: "safe content", Category: "decision",
			Metadata: map[string]any{"memory_type": "semantic", "scope": "project", "origin": "dream"}},
	}
	results := filter.Filter(items)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Accepted {
		t.Errorf("semantic project memory should be accepted, got rule=%s", results[0].Rule)
	}
}

func TestFilter_LifecycleNilMetadataAccepted(t *testing.T) {
	filter := NewContentFilter()
	items := []Filterable{
		{ID: "1", Content: "safe content", Category: "fact", Metadata: nil},
	}
	results := filter.Filter(items)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Accepted {
		t.Errorf("nil metadata should be accepted, got rule=%s", results[0].Rule)
	}
}

func TestFilter_QualityLowSignalCurationRejected(t *testing.T) {
	filter := NewContentFilter()
	items := []Filterable{
		{ID: "1", Content: "Postgres has MVCC", Category: "fact",
			Metadata: map[string]any{"curation_status": "low_signal"}},
	}
	results := filter.Filter(items)
	if results[0].Accepted {
		t.Fatal("expected low_signal curation to be rejected")
	}
	if results[0].Rule != "low_signal" {
		t.Errorf("rule: got %q, want low_signal", results[0].Rule)
	}
}

func TestFilter_QualityLowScoreRejected(t *testing.T) {
	filter := NewContentFilter()
	items := []Filterable{
		{ID: "1", Content: "Postgres has MVCC", Category: "fact",
			Metadata: map[string]any{"quality_score": 0.40}},
	}
	results := filter.Filter(items)
	if results[0].Accepted {
		t.Fatal("expected low quality_score to be rejected")
	}
	if results[0].Rule != "low_quality" {
		t.Errorf("rule: got %q, want low_quality", results[0].Rule)
	}
}

func TestFilter_QualityPinnedBypassesScore(t *testing.T) {
	filter := NewContentFilter()
	items := []Filterable{
		{ID: "1", Content: "Postgres has MVCC", Category: "fact",
			Metadata: map[string]any{"quality_score": 0.20, "pinned": true}},
	}
	results := filter.Filter(items)
	if !results[0].Accepted {
		t.Errorf("pinned memory should bypass quality, got rule=%s", results[0].Rule)
	}
}

func TestFilter_QualityZeroScoreAccepted(t *testing.T) {
	// score=0 means "not scored yet" — should pass through.
	filter := NewContentFilter()
	items := []Filterable{
		{ID: "1", Content: "Postgres has MVCC", Category: "fact",
			Metadata: map[string]any{"quality_score": 0.0}},
	}
	results := filter.Filter(items)
	if !results[0].Accepted {
		t.Errorf("zero score should pass (not scored), got rule=%s", results[0].Rule)
	}
}

func TestFilter_QualityTestOutputFactRejected(t *testing.T) {
	filter := NewContentFilter()
	items := []Filterable{
		{ID: "1", Content: "23 passed, 0 failed", Category: "fact"},
	}
	results := filter.Filter(items)
	if results[0].Accepted {
		t.Fatal("expected test output to be rejected as low_quality")
	}
	if results[0].Rule != "low_quality" {
		t.Errorf("rule: got %q, want low_quality", results[0].Rule)
	}
}

func TestFilter_QualityProgressFactRejected(t *testing.T) {
	filter := NewContentFilter()
	items := []Filterable{
		{ID: "1", Content: "Corrigido o bug do scheduler", Category: "fact"},
	}
	results := filter.Filter(items)
	if results[0].Accepted {
		t.Fatal("expected progress chatter to be rejected")
	}
}

func TestFilter_QualityTerminalTraceFactRejected(t *testing.T) {
	filter := NewContentFilter()
	items := []Filterable{
		{ID: "1", Content: "panic: nil pointer dereference at line 42", Category: "fact"},
	}
	results := filter.Filter(items)
	if results[0].Accepted {
		t.Fatal("expected terminal trace to be rejected")
	}
}

func TestFilter_QualityLongFactWithTriggerWordsAccepted(t *testing.T) {
	// Long, articulate facts are NOT operational chatter even if they
	// mention "passed", "rodando", etc.
	filter := NewContentFilter()
	long := "When testing the new sync path, our suite went from 200 to 245 tests with 0 failures, " +
		"validating that the throttling fix in the engine prevents the regression we saw in v1.2 " +
		"where concurrent batches were rodando over each other and double-counting tombstones."
	items := []Filterable{
		{ID: "1", Content: long, Category: "fact"},
	}
	results := filter.Filter(items)
	if !results[0].Accepted {
		t.Errorf("long fact with trigger words should pass, got rule=%s", results[0].Rule)
	}
}

func TestFilter_QualityDecisionWithTriggerWordsAccepted(t *testing.T) {
	// The short-content heuristic only applies to category=fact.
	filter := NewContentFilter()
	items := []Filterable{
		{ID: "1", Content: "23 passed, 0 failed — ship it", Category: "decision"},
	}
	results := filter.Filter(items)
	if !results[0].Accepted {
		t.Errorf("decision with trigger words should pass, got rule=%s", results[0].Rule)
	}
}

func TestFilter_LifecycleHandoffKindWithOperationalAccepted(t *testing.T) {
	filter := NewContentFilter()
	items := []Filterable{
		{ID: "1", Content: "context bridge for the next session about the auth refactor",
			Category: "summary",
			Metadata: map[string]any{"memory_type": "operational", "kind": "handoff"}},
	}
	results := filter.Filter(items)
	// Origin=handoff still blocks. memory_type=operational + kind=handoff
	// passes operational check, but if origin is also handoff it would
	// block via lifecycle_local_origin. With only memory_type/kind set,
	// it passes. Validate that.
	if !results[0].Accepted {
		t.Errorf("operational+handoff should pass when origin isn't set, got rule=%s", results[0].Rule)
	}
}
