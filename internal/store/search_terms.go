package store

import (
	"strings"
	"unicode/utf8"
)

// maxSearchTerms caps how many tokens a free-text query contributes to the
// term-wise search SQL, bounding placeholder growth on long prompts.
const maxSearchTerms = 8

// searchTerms tokenizes a free-text query for term-wise matching. Natural
// queries (the kind LLM clients send: "ASC-API sistema gestão condominial")
// almost never occur verbatim inside a memory, so matching the whole query as
// one substring returns nothing — each whitespace-separated token becomes its
// own LIKE term instead. Tokens are lowercased, LIKE-escaped, and single-rune
// tokens are dropped.
func searchTerms(query string) []string {
	fields := strings.Fields(strings.ToLower(query))
	terms := make([]string, 0, len(fields))
	for _, f := range fields {
		if utf8.RuneCountInString(f) < 2 {
			continue
		}
		f = strings.ReplaceAll(f, `\`, `\\`)
		f = strings.ReplaceAll(f, "%", `\%`)
		f = strings.ReplaceAll(f, "_", `\_`)
		terms = append(terms, f)
		if len(terms) == maxSearchTerms {
			break
		}
	}
	return terms
}
