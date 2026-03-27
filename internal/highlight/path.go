package highlight

import (
	"regexp"
	"strings"
)

// PathHighlighter colorizes Unix file paths found within a line.
// Each path segment is painted with segmentPainter and each "/" separator
// is painted with sepPainter.
type PathHighlighter struct {
	re             *regexp.Regexp
	segmentPainter Painter
	sepPainter     Painter
}

// NewPathHighlighter creates a PathHighlighter that paints path segments and
// separators independently. Requires at least 2 path components to match
// (e.g., /usr/bin matches, /usr alone does not). Allows ./relative, ~/home,
// //network, or absolute /paths.
func NewPathHighlighter(segment, separator Painter) *PathHighlighter {
	return &PathHighlighter{
		re:             regexp.MustCompile(`(?:^|\s)((?:\./|~/|//)[\w.\-]+(?:/[\w.\-]+)*|/[\w.\-]+(?:/[\w.\-]+)+)`),
		segmentPainter: segment,
		sepPainter:     separator,
	}
}

// Highlight scans the line for Unix paths and colorizes them.
// Returns the original string (same pointer) when no match is found.
func (h *PathHighlighter) Highlight(line string) string {
	// Early-exit guard: paths always contain a slash.
	if strings.IndexByte(line, '/') < 0 {
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
		// loc[0], loc[1] = full match (including possible leading whitespace)
		// loc[2], loc[3] = group 1 (the path itself)
		if loc[2] < 0 {
			continue
		}

		pathStart := loc[2]
		pathEnd := loc[3]
		path := line[pathStart:pathEnd]

		// Write text before this match (including any leading whitespace
		// that is part of the full match but not the capture group).
		sb.WriteString(line[prev:pathStart])

		// Paint each character: segments vs separators.
		h.paintPath(&sb, path)

		prev = pathEnd
	}

	// Write remaining text after the last match.
	if prev < len(line) {
		sb.WriteString(line[prev:])
	}
	return sb.String()
}

// paintPath writes the path to the builder, painting segments and separators.
func (h *PathHighlighter) paintPath(sb *strings.Builder, path string) {
	segStart := -1

	for i := 0; i < len(path); i++ {
		if path[i] == '/' {
			// Flush any accumulated segment.
			if segStart >= 0 {
				h.segmentPainter.WriteTo(sb, path[segStart:i])
				segStart = -1
			}
			// Paint the separator.
			h.sepPainter.WriteTo(sb, "/")
		} else {
			if segStart < 0 {
				segStart = i
			}
		}
	}

	// Flush trailing segment.
	if segStart >= 0 {
		h.segmentPainter.WriteTo(sb, path[segStart:])
	}
}
