package policy

func (f *ContentFilter) isBlockedCategory(category string) bool {
	return f.blockedCategories[category]
}
