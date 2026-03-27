package highlight

import (
	"regexp"
	"strings"
)

// urlPattern matches URLs with http or https schemes.
// Groups: 1=protocol, 2="://", 3=host, 4=path (optional), 5=query string (optional)
var urlPattern = regexp.MustCompile(`(https?)(://)([^/\s?#]+)(/[^\s?#]*)?(\?[^\s#]*)?`)

// URLHighlighter colorizes URLs found within a line, painting each part
// (protocol, host, path, query) with a separate Painter.
type URLHighlighter struct {
	re              *regexp.Regexp
	protocolPainter Painter
	hostPainter     Painter
	pathPainter     Painter
	queryPainter    Painter
	symbolPainter   Painter
}

// NewURLHighlighter creates a URLHighlighter with separate painters for each
// URL component: protocol, host, path, query string, and symbols (like "://").
func NewURLHighlighter(protocol, host, path, query, symbol Painter) *URLHighlighter {
	return &URLHighlighter{
		re:              urlPattern,
		protocolPainter: protocol,
		hostPainter:     host,
		pathPainter:     path,
		queryPainter:    query,
		symbolPainter:   symbol,
	}
}

// Highlight scans the line for URLs and colorizes matched parts.
// Returns the original string (same pointer) when no match is found.
func (h *URLHighlighter) Highlight(line string) string {
	// Early-exit guard: URLs always contain "://"
	if !strings.Contains(line, "://") {
		return line
	}

	matches := h.re.FindAllStringSubmatchIndex(line, -1)
	if len(matches) == 0 {
		return line
	}

	var sb strings.Builder
	sb.Grow(len(line) + 64)
	prev := 0

	for _, loc := range matches {
		// loc layout:
		// [0:1]   = full match start/end
		// [2:3]   = group 1 (protocol: "http" or "https")
		// [4:5]   = group 2 ("://")
		// [6:7]   = group 3 (host)
		// [8:9]   = group 4 (path, optional — -1 if absent)
		// [10:11]  = group 5 (query string, optional — may be empty string)

		// Write text before this match.
		if prev < loc[0] {
			sb.WriteString(line[prev:loc[0]])
		}

		// Paint protocol (group 1)
		h.protocolPainter.WriteTo(&sb, line[loc[2]:loc[3]])

		// Paint "://" symbol (group 2)
		h.symbolPainter.WriteTo(&sb, line[loc[4]:loc[5]])

		// Paint host (group 3)
		h.hostPainter.WriteTo(&sb, line[loc[6]:loc[7]])

		// Paint path (group 4) if present
		if loc[8] >= 0 && loc[9] >= 0 {
			h.pathPainter.WriteTo(&sb, line[loc[8]:loc[9]])
		}

		// Paint query string (group 5) if present and non-empty
		if loc[10] >= 0 && loc[11] >= 0 && loc[10] < loc[11] {
			h.queryPainter.WriteTo(&sb, line[loc[10]:loc[11]])
		}

		prev = loc[1]
	}

	// Write remaining text after the last match.
	if prev < len(line) {
		sb.WriteString(line[prev:])
	}

	return sb.String()
}
