package curation

import (
	"strings"
	"time"
	"unicode"

	"github.com/jholhewres/anchored_oss/internal/model"
)

// contradictionMinOverlap is the number of shared significant keywords two
// memories must have to be considered "about the same thing" for contradiction
// purposes. Two is enough to anchor a shared subject without flooding on a
// single common word.
const contradictionMinOverlap = 2

// contradictionMinGap is the minimum age difference between two memories for
// them to count as written in "distinct time windows". A reversal that evolved
// over time is the signal we want; two memories saved seconds apart in the same
// batch are not a contradiction candidate.
const contradictionMinGap = time.Hour

// negationMarkers are reversal/negation tokens in English and Portuguese. A
// contradiction candidate is one where exactly one of the two memories carries
// such a marker over a shared subject — i.e. one asserts and the other negates
// or reverses the earlier statement.
var negationMarkers = map[string]bool{
	// English
	"no": true, "not": true, "never": true, "dont": true, "doesnt": true,
	"isnt": true, "arent": true, "wont": true, "cant": true, "stop": true,
	"stopped": true, "drop": true, "dropped": true, "deprecate": true,
	"deprecated": true, "instead": true, "replace": true, "replaced": true,
	"migrate": true, "migrated": true, "switch": true, "switched": true,
	"abandon": true, "abandoned": true, "remove": true, "removed": true,
	"longer": true, "avoid": true, "discontinue": true, "discontinued": true,
	// Portuguese
	"nao": true, "não": true, "nunca": true, "deixamos": true, "deixar": true,
	"parar": true, "paramos": true, "substituir": true, "substituimos": true,
	"substituímos": true, "trocar": true, "trocamos": true, "remover": true,
	"removemos": true, "abandonar": true, "abandonamos": true, "evitar": true,
	"descontinuar": true, "descontinuado": true,
}

// findContradiction scans peers for the most likely contradiction of mem. It
// returns the peer's ID and true when a candidate is found.
//
// Heuristic (no LLM, no embeddings):
//  1. shared subject: at least contradictionMinOverlap significant keywords in
//     common (keywords if present, otherwise content tokens);
//  2. reversal: exactly one of the pair contains a negation/reversal marker;
//  3. distinct windows: the two were written at least contradictionMinGap apart.
//
// The window is bounded by whatever peer set the caller passes (the worker
// reuses the near-duplicate window), so this catches contradictions that
// emerged within that window — a deliberately conservative MVP that never
// auto-resolves, only flags for human review.
func findContradiction(mem *model.Memory, peers []*model.Memory) (string, bool) {
	selfKW := significantTokens(mem)
	if len(selfKW) == 0 {
		return "", false
	}
	selfNeg := hasNegation(mem.Content)

	for _, peer := range peers {
		if peer.ID == mem.ID {
			continue
		}
		if abs(mem.CreatedAt.Sub(peer.CreatedAt)) < contradictionMinGap {
			continue
		}
		if overlapCount(selfKW, significantTokens(peer)) < contradictionMinOverlap {
			continue
		}
		// Exactly one side expresses a reversal over the shared subject.
		if selfNeg != hasNegation(peer.Content) {
			return peer.ID, true
		}
	}
	return "", false
}

// significantTokens returns the set of lower-cased significant tokens for a
// memory: its keywords when populated, otherwise content words of length >= 4
// that are not stopwords.
func significantTokens(m *model.Memory) map[string]bool {
	set := make(map[string]bool)
	if len(m.Keywords) > 0 {
		for _, k := range m.Keywords {
			k = strings.ToLower(strings.TrimSpace(k))
			if k != "" {
				set[k] = true
			}
		}
		return set
	}
	for _, tok := range tokenize(m.Content) {
		if len([]rune(tok)) >= 4 && !stopwords[tok] {
			set[tok] = true
		}
	}
	return set
}

// hasNegation reports whether the content contains any negation/reversal marker.
func hasNegation(content string) bool {
	for _, tok := range tokenize(content) {
		if negationMarkers[tok] {
			return true
		}
	}
	return false
}

// tokenize lower-cases and splits on any non-letter/digit rune.
func tokenize(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

func overlapCount(a, b map[string]bool) int {
	n := 0
	for k := range a {
		if b[k] {
			n++
		}
	}
	return n
}

func abs(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

// stopwords are common EN/PT words excluded from token-derived subjects so the
// overlap signal reflects real topic words rather than glue.
var stopwords = map[string]bool{
	"the": true, "and": true, "for": true, "with": true, "this": true,
	"that": true, "from": true, "have": true, "will": true, "should": true,
	"must": true, "uses": true, "using": true, "used": true, "into": true,
	"para": true, "como": true, "isso": true, "esta": true, "este": true,
	"dos": true, "das": true, "uma": true, "que": true, "por": true,
}
