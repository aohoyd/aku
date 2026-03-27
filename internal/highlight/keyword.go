package highlight

import (
	"regexp"
	"strings"
)

// KeywordGroup is a public config type for constructing the KeywordHighlighter.
// Words are matched case-sensitively at word boundaries.
type KeywordGroup struct {
	Words   []string
	Painter Painter
}

// keywordGroup is the compiled internal representation of a KeywordGroup.
type keywordGroup struct {
	re      *regexp.Regexp
	painter Painter
}

// KeywordHighlighter colorizes predefined keyword groups (HTTP methods,
// booleans, null variants, etc.) found within a line. Each group has its own
// compiled regex and painter. Keywords are matched case-sensitively.
type KeywordHighlighter struct {
	groups []keywordGroup
}

// NewKeywordHighlighter creates a KeywordHighlighter from the given groups.
// Each group's words are compiled into a single case-sensitive word-boundary
// alternation regex: \b(GET|POST|PUT|...)\b.
func NewKeywordHighlighter(groups []KeywordGroup) *KeywordHighlighter {
	compiled := make([]keywordGroup, 0, len(groups))
	for _, g := range groups {
		if len(g.Words) == 0 {
			continue
		}
		escaped := make([]string, len(g.Words))
		for i, w := range g.Words {
			escaped[i] = regexp.QuoteMeta(w)
		}
		pattern := `\b(` + strings.Join(escaped, "|") + `)\b`
		compiled = append(compiled, keywordGroup{
			re:      regexp.MustCompile(pattern),
			painter: g.Painter,
		})
	}
	return &KeywordHighlighter{groups: compiled}
}

// Highlight scans line for keyword matches across all groups and wraps each
// match with the appropriate ANSI color. Groups are applied sequentially.
// If no matches are found across all groups, the original string (same pointer)
// is returned for the zero-alloc fast path.
func (h *KeywordHighlighter) Highlight(line string) string {
	if len(h.groups) == 0 {
		return line
	}

	current := line
	modified := false

	for _, g := range h.groups {
		locs := g.re.FindAllStringIndex(current, -1)
		if len(locs) == 0 {
			continue
		}

		modified = true
		var sb strings.Builder
		sb.Grow(len(current) + len(locs)*23)

		prev := 0
		for _, loc := range locs {
			start, end := loc[0], loc[1]
			if prev < start {
				sb.WriteString(current[prev:start])
			}
			g.painter.WriteTo(&sb, current[start:end])
			prev = end
		}
		if prev < len(current) {
			sb.WriteString(current[prev:])
		}
		current = sb.String()
	}

	if !modified {
		return line
	}
	return current
}
