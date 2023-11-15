package ui

import (
	"cmp"
	"slices"
	"strings"
)

// PluginEntry holds the minimal info needed for fuzzy matching.
type PluginEntry struct {
	Name      string
	ShortName string
}

// FuzzyMatch represents a scored match result.
type FuzzyMatch struct {
	Name    string // plugin Name() — the goto target
	Display string // formatted: "name (short)"
	Score   int    // higher = better match
}

// FuzzyMatchPlugins scores all entries against query, returning non-zero
// matches sorted best-first. Empty query returns all entries.
func FuzzyMatchPlugins(query string, entries []PluginEntry) []FuzzyMatch {
	query = strings.ToLower(strings.TrimSpace(query))

	var results []FuzzyMatch
	for _, e := range entries {
		score := scoreEntry(query, e)
		if score > 0 || query == "" {
			display := e.Name
			if e.ShortName != "" && e.ShortName != e.Name {
				display = e.Name + " (" + e.ShortName + ")"
			}
			if query == "" {
				score = 1
			}
			results = append(results, FuzzyMatch{
				Name:    e.Name,
				Display: display,
				Score:   score,
			})
		}
	}

	slices.SortStableFunc(results, func(a, b FuzzyMatch) int {
		if c := cmp.Compare(b.Score, a.Score); c != 0 {
			return c
		}
		return cmp.Compare(len(a.Name), len(b.Name))
	})

	return results
}

func scoreEntry(query string, e PluginEntry) int {
	if query == "" {
		return 1
	}
	name := strings.ToLower(e.Name)
	short := strings.ToLower(e.ShortName)

	best := 0
	for _, candidate := range []string{name, short} {
		if candidate == "" {
			continue
		}
		if s := scoreCandidate(candidate, query); s > best {
			best = s
		}
	}
	return best
}

func scoreCandidate(candidate, query string) int {
	candidate = strings.ToLower(candidate)
	query = strings.ToLower(query)

	if candidate == query {
		return 1000
	}
	if strings.HasPrefix(candidate, query) {
		return 800 + (100 * len(query) / len(candidate))
	}
	if strings.Contains(candidate, query) {
		return 400
	}
	if matchAbbreviation(query, candidate) {
		return 200
	}
	return 0
}

// FilterPlugins returns PluginEntries matching query, sorted by score (best first).
func FilterPlugins(query string, entries []PluginEntry) []PluginEntry {
	matches := FuzzyMatchPlugins(query, entries)
	byName := make(map[string]PluginEntry, len(entries))
	for _, e := range entries {
		byName[e.Name] = e
	}
	result := make([]PluginEntry, len(matches))
	for i, m := range matches {
		result[i] = byName[m.Name]
	}
	return result
}

// matchAbbreviation checks if each character in query appears in candidate
// in order. For example, "hpa" matches "horizontalpodautoscalers".
func matchAbbreviation(query, candidate string) bool {
	runes := []rune(candidate)
	ci := 0
	for _, qc := range query {
		found := false
		for ci < len(runes) {
			if runes[ci] == qc {
				ci++
				found = true
				break
			}
			ci++
		}
		if !found {
			return false
		}
	}
	return true
}
