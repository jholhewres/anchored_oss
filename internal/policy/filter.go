package policy

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

// ContentFilter validates incoming sync content against security rules.
type ContentFilter struct {
	blockedCategories map[string]bool
}

// NewContentFilter creates a ContentFilter with default blocked categories.
func NewContentFilter() *ContentFilter {
	return &ContentFilter{
		blockedCategories: map[string]bool{
			"event":      true,
			"preference": true,
		},
	}
}

// Filter validates each item and returns a FilterResult per item.
// Items are checked in order: blocked category → lifecycle metadata → local paths → secrets.
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

		if found, pattern := containsLocalPath(item.Content); found {
			results = append(results, FilterResult{
				ID:       item.ID,
				Accepted: false,
				Rule:     "local_path_detected",
				Detail:   "local path pattern detected: " + pattern,
			})
			continue
		}

		if found, pattern := containsSecret(item.Content); found {
			results = append(results, FilterResult{
				ID:       item.ID,
				Accepted: false,
				Rule:     "secret_detected",
				Detail:   "secret pattern detected: " + pattern,
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

func (f *ContentFilter) checkLifecycle(meta map[string]any) (rule string, detail string) {
	if meta == nil {
		return "", ""
	}

	if scope, _ := meta["scope"].(string); scope == "user" {
		return "lifecycle_user_scope", "user-scoped memories are not allowed on the server"
	}

	if memoryType, _ := meta["memory_type"].(string); memoryType == "operational" {
		return "lifecycle_operational", "operational memories are local-only and should not be synced"
	}

	if origin, _ := meta["origin"].(string); origin == "precompact" || origin == "handoff" {
		return "lifecycle_local_origin", origin + " memories are local session context and should not be synced"
	}

	return "", ""
}
