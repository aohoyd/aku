package highlight

import (
	"regexp"
	"strings"
)

// UUIDHighlighter colorizes UUID strings (8-4-4-4-12 hex format) found within a line.
type UUIDHighlighter struct {
	re      *regexp.Regexp
	painter Painter
}

// NewUUIDHighlighter creates a UUIDHighlighter with the given painter.
func NewUUIDHighlighter(painter Painter) *UUIDHighlighter {
	return &UUIDHighlighter{
		re:      regexp.MustCompile(`\b[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}\b`),
		painter: painter,
	}
}

// Highlight scans the line for valid UUIDs and colorizes them.
// Returns the original string (same pointer) when no match is found.
func (h *UUIDHighlighter) Highlight(line string) string {
	// Early-exit guard: a UUID has at least 4 dashes.
	if strings.Count(line, "-") < 4 {
		return line
	}

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
