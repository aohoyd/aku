package highlight

import (
	"regexp"
	"strings"
)

// NumberHighlighter colorizes integer and decimal numbers found within a line.
type NumberHighlighter struct {
	re      *regexp.Regexp
	painter Painter
}

// NewNumberHighlighter creates a NumberHighlighter with the given painter.
func NewNumberHighlighter(painter Painter) *NumberHighlighter {
	return &NumberHighlighter{
		re:      regexp.MustCompile(`\b\d+(?:\.\d+)?\b`),
		painter: painter,
	}
}

// Highlight scans the line for numbers and colorizes them.
// Returns the original string (same pointer) when no match is found.
func (h *NumberHighlighter) Highlight(line string) string {
	matches := h.re.FindAllStringIndex(line, -1)
	if len(matches) == 0 {
		return line
	}

	var sb strings.Builder
	sb.Grow(len(line) + 32*len(matches))
	prev := 0

	for _, loc := range matches {
		sb.WriteString(line[prev:loc[0]])
		h.painter.WriteTo(&sb, line[loc[0]:loc[1]])
		prev = loc[1]
	}

	if prev < len(line) {
		sb.WriteString(line[prev:])
	}

	return sb.String()
}
