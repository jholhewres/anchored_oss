package policy

import (
	"regexp"
	"strings"
)

// RemoteQualityThreshold is the minimum quality_score a memory must reach to
// be accepted by the remote server. Kept in sync with the client-side
// constant (anchored/pkg/memory.RemoteQualityThreshold). The client is the
// authoritative scorer; this is the server's safety-net.
const RemoteQualityThreshold = 0.55

// Regex patterns ported from anchored/pkg/memory/quality.go so the server can
// reject obviously low-signal "operational" memories that get mis-categorized
// as facts (test output, progress updates, terminal traces). The client
// already filters most of these, but old data and misbehaving clients still
// reach the server.
var (
	testOutputRe    = regexp.MustCompile(`(?i)\b(\d+\s+(passed|failed|skipped)|0\s+failures?|testes?\s+passando|suite\s+completa|rodando\s+suite|go\s+test|pytest|npm\s+test)\b`)
	progressRe      = regexp.MustCompile(`(?i)\b(corrigido|rodando|testando|retestar)\b`)
	terminalTraceRe = regexp.MustCompile(`(?i)(?:^|\n)\s*(?:error:|warning:|panic:|traceback\b|stack trace\b|expected\b|actual\b|assert\b)`)
)

// FilterResult represents the outcome of filtering a single item.
type FilterResult struct {
	ID       string `json:"id"`
	Accepted bool   `json:"accepted"`
	Rule     string `json:"rule,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

// Filterable is a content item to be validated by the filter.
type Filterable struct {
	ID       string
	Content  string
	Category string
	Metadata map[string]any
}

// DefaultBlockedCategories are the categories rejected at sync time unless an
// org overrides them. event/preference are local-only by default.
var DefaultBlockedCategories = []string{"event", "preference"}

// CustomRule is an admin-defined content rejection (a compiled regex). Keyword
// guardrails are compiled into a case-insensitive literal regex by the caller.
type CustomRule struct {
	Label string
	Re    *regexp.Regexp
}

// Config drives a ContentFilter from an org's guardrail set. The three security
// toggles default to true via NewContentFilterWithConfig (legacy callers); the
// guardrail manager passes explicit values so admins can disable any of them.
type Config struct {
	BlockedCategories []string
	QualityThreshold  float64
	SecretDetection   bool
	PathRedaction     bool
	UserScopeBlock    bool
	CustomRules       []CustomRule
}

// ContentFilter validates incoming sync content against the org's guardrails.
// Which rules run is controlled by the per-org guardrail set (see Config); the
// operational/local-origin lifecycle checks are always on (transient session
// artifacts should never sync regardless of configuration).
type ContentFilter struct {
	blockedCategories map[string]bool
	qualityThreshold  float64
	secretDetection   bool
	pathRedaction     bool
	userScopeBlock    bool
	customRules       []CustomRule
}

// NewContentFilter creates a ContentFilter with default blocked categories,
// the default quality threshold, and all security guardrails enabled.
func NewContentFilter() *ContentFilter {
	return NewContentFilterWithConfig(DefaultBlockedCategories, RemoteQualityThreshold)
}

// NewContentFilterWithConfig builds a filter from a blocked-category list and
// quality threshold, with all security guardrails enabled (legacy default).
// An empty blockedCategories falls back to the defaults; a non-positive
// threshold falls back to RemoteQualityThreshold.
func NewContentFilterWithConfig(blockedCategories []string, qualityThreshold float64) *ContentFilter {
	if len(blockedCategories) == 0 {
		blockedCategories = DefaultBlockedCategories
	}
	return NewContentFilterFromConfig(Config{
		BlockedCategories: blockedCategories,
		QualityThreshold:  qualityThreshold,
		SecretDetection:   true,
		PathRedaction:     true,
		UserScopeBlock:    true,
	})
}

// NewContentFilterFromConfig builds a filter from an explicit guardrail config.
// Unlike NewContentFilterWithConfig, an empty BlockedCategories means "block no
// categories" (the admin disabled them all) — it does NOT fall back to defaults.
func NewContentFilterFromConfig(cfg Config) *ContentFilter {
	blockedCategories := cfg.BlockedCategories
	threshold := cfg.QualityThreshold
	if threshold <= 0 {
		threshold = RemoteQualityThreshold
	}
	blocked := make(map[string]bool, len(blockedCategories))
	for _, c := range blockedCategories {
		blocked[strings.ToLower(strings.TrimSpace(c))] = true
	}
	return &ContentFilter{
		blockedCategories: blocked,
		qualityThreshold:  threshold,
		secretDetection:   cfg.SecretDetection,
		pathRedaction:     cfg.PathRedaction,
		userScopeBlock:    cfg.UserScopeBlock,
		customRules:       cfg.CustomRules,
	}
}

// Filter validates each item and returns a FilterResult per item.
// Items are checked in order: blocked category → lifecycle metadata → local
// paths → secrets → quality.
func (f *ContentFilter) Filter(items []Filterable) []FilterResult {
	results := make([]FilterResult, 0, len(items))
	for _, item := range items {
		if f.isBlockedCategory(item.Category) {
			results = append(results, FilterResult{
				ID:       item.ID,
				Accepted: false,
				Rule:     "blocked_category",
				Detail:   "category \"" + item.Category + "\" is not allowed",
			})
			continue
		}

		if rule, detail := f.checkLifecycle(item.Metadata); rule != "" {
			results = append(results, FilterResult{
				ID:       item.ID,
				Accepted: false,
				Rule:     rule,
				Detail:   detail,
			})
			continue
		}

		if f.pathRedaction {
			if found, pattern := containsLocalPath(item.Content); found {
				results = append(results, FilterResult{
					ID:       item.ID,
					Accepted: false,
					Rule:     "local_path_detected",
					Detail:   "local path pattern detected: " + pattern,
				})
				continue
			}
		}

		if f.secretDetection {
			if found, pattern := containsSecret(item.Content); found {
				results = append(results, FilterResult{
					ID:       item.ID,
					Accepted: false,
					Rule:     "secret_detected",
					Detail:   "secret pattern detected: " + pattern,
				})
				continue
			}
		}

		if rule, detail := f.matchCustomRule(item.Content); rule != "" {
			results = append(results, FilterResult{
				ID:       item.ID,
				Accepted: false,
				Rule:     "custom_rule",
				Detail:   detail,
			})
			continue
		}

		if rule, detail := f.checkQuality(item); rule != "" {
			results = append(results, FilterResult{
				ID:       item.ID,
				Accepted: false,
				Rule:     rule,
				Detail:   detail,
			})
			continue
		}

		results = append(results, FilterResult{
			ID:       item.ID,
			Accepted: true,
		})
	}
	return results
}

// matchCustomRule rejects content matching any admin-defined custom guardrail.
func (f *ContentFilter) matchCustomRule(content string) (rule string, detail string) {
	for _, r := range f.customRules {
		if r.Re != nil && r.Re.MatchString(content) {
			return "custom_rule", "blocked by custom guardrail: " + r.Label
		}
	}
	return "", ""
}

func (f *ContentFilter) checkQuality(item Filterable) (rule string, detail string) {
	pinned := false
	if item.Metadata != nil {
		if status, _ := item.Metadata["curation_status"].(string); status == "low_signal" {
			return "low_signal", "memory is marked low_signal by client curation"
		}
		if v, ok := item.Metadata["pinned"].(bool); ok {
			pinned = v
		}
		if score, ok := numberFromMetadata(item.Metadata["quality_score"]); ok && score > 0 && score < f.qualityThreshold && !pinned {
			return "low_quality", "quality_score below remote threshold"
		}
	}

	if pinned {
		return "", ""
	}

	content := strings.TrimSpace(item.Content)
	chars := len([]rune(content))

	// Short "fact" entries that are really test output or progress chatter
	// fail the heuristic. We're intentionally narrow: long, articulate facts
	// pass even if they contain trigger words.
	if item.Category == "fact" && chars < 180 {
		if testOutputRe.MatchString(content) {
			return "low_quality", "test output is not durable project knowledge"
		}
		if progressRe.MatchString(content) {
			return "low_quality", "progress chatter is not durable project knowledge"
		}
		if terminalTraceRe.MatchString(content) {
			return "low_quality", "terminal trace is not durable project knowledge"
		}
	}
	return "", ""
}

func numberFromMetadata(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}

func (f *ContentFilter) checkLifecycle(meta map[string]any) (rule string, detail string) {
	if meta == nil {
		return "", ""
	}

	if f.userScopeBlock {
		if scope, _ := meta["scope"].(string); scope == "user" {
			return "lifecycle_user_scope", "user-scoped memories are not allowed on the server"
		}
	}

	if memoryType, _ := meta["memory_type"].(string); memoryType == "operational" {
		// "handoff" kind is a recognized cross-tool bridge, allow it through
		// even when memory_type=operational.
		if kind, _ := meta["kind"].(string); kind != "handoff" {
			return "lifecycle_operational", "operational memories are local-only and should not be synced"
		}
	}

	if origin, _ := meta["origin"].(string); origin == "precompact" || origin == "handoff" {
		return "lifecycle_local_origin", origin + " memories are local session context and should not be synced"
	}

	return "", ""
}
