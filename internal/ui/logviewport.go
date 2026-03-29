package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// logViewport is a minimal viewport renderer for pre-styled log lines.
// It avoids the redundant ANSI parsing that a general-purpose viewport performs
// by accepting pre-computed display widths and doing at most one ansi.Cut per
// visible line (only when horizontally scrolled).
type logViewport struct {
	lines     []string
	rawWidths []int
	width     int
	height    int
	xOffset   int
}

// SetLines stores lines and their pre-computed display widths verbatim.
// No processing is performed on the content.
func (v *logViewport) SetLines(lines []string, rawWidths []int) {
	v.lines = lines
	v.rawWidths = rawWidths
}

// SetSize updates the viewport dimensions.
func (v *logViewport) SetSize(w, h int) {
	v.width = w
	v.height = h
}

// View renders the visible content as a single string.
//
// For each stored line the visible portion is extracted (via ansi.Cut only when
// xOffset > 0) and right-padded with spaces so every output line has exactly
// `width` display columns. If there are fewer lines than `height`, blank lines
// (width spaces) are appended. The result is newline-joined.
func (v *logViewport) View() string {
	if v.width <= 0 || v.height <= 0 {
		return ""
	}

	// Pre-compute the empty-line padding string once.
	emptyLine := strings.Repeat(" ", v.width)

	n := len(v.lines)
	if n > v.height {
		n = v.height
	}

	outputs := make([]string, v.height)

	for i := range n {
		line := v.lines[i]
		rw := 0
		if i < len(v.rawWidths) {
			rw = v.rawWidths[i]
		}

		var visibleWidth int
		if v.xOffset > 0 {
			if rw <= v.xOffset {
				// Scrolled past the end of this line — emit empty padding.
				line = ""
				visibleWidth = 0
			} else {
				line = ansi.Cut(line, v.xOffset, v.xOffset+v.width)
				visibleWidth = rw - v.xOffset
				if visibleWidth > v.width {
					visibleWidth = v.width
				}
			}
		} else {
			if rw > v.width {
				line = ansi.Cut(line, 0, v.width)
			}
			visibleWidth = rw
			if visibleWidth > v.width {
				visibleWidth = v.width
			}
		}

		padding := v.width - visibleWidth
		if padding < 0 {
			padding = 0
		}
		if padding > 0 {
			outputs[i] = line + emptyLine[:padding]
		} else {
			outputs[i] = line
		}
	}

	// Fill remaining rows with blank lines.
	for i := n; i < v.height; i++ {
		outputs[i] = emptyLine
	}

	return strings.Join(outputs, "\n")
}
