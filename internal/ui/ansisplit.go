package ui

import (
	"strings"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/aohoyd/aku/internal/theme"
)

// isSGR returns true if seq is a CSI SGR sequence (ends with 'm').
func isSGR(seq string) bool {
	// Minimum valid SGR: \x1b[m (3 bytes)
	if len(seq) < 3 {
		return false
	}
	return seq[0] == '\x1b' && seq[1] == '[' && seq[len(seq)-1] == 'm'
}

// isResetSGR returns true if seq is an SGR reset: \x1b[m or \x1b[0m.
func isResetSGR(seq string) bool {
	return seq == "\x1b[m" || seq == "\x1b[0m"
}

// highlightRange represents a display-column range [start, end) for a single
// search match on a line.
type highlightRange struct {
	start, end int
}

// styleToSGR extracts the SGR prefix from a lipgloss.Style by rendering a
// dummy character and taking everything before it. This is called once per
// style, not per frame.
func styleToSGR(s lipgloss.Style) string {
	rendered := s.Render("X")
	idx := strings.Index(rendered, "X")
	if idx <= 0 {
		return ""
	}
	return rendered[:idx]
}

// injectHighlights applies highlight styling to the given display-column ranges
// within an ANSI-decorated line in a single O(L) pass.
//
// matches must be sorted by start column and non-overlapping. selectedIdx
// identifies which match (by index into matches) should use selSGR; all others
// use hiSGR. Both hiSGR and selSGR should be pre-computed SGR sequences
// (e.g. via styleToSGR).
//
// Original SGR sequences within a highlighted range are suppressed from the
// output (to keep the highlight clean) but tracked so they can be restored
// after the match ends.
func injectHighlights(line string, matches []highlightRange, selectedIdx int, hiSGR, selSGR string) string {
	if len(matches) == 0 {
		return line
	}

	var (
		out      strings.Builder
		sgrBuf   strings.Builder // accumulated original SGR state
		matchIdx int             // index into sorted matches
		inMatch  bool
		col      int // current display column
		p        = ansi.NewParser()
		state    byte
	)

	out.Grow(len(line) + len(matches)*40) // pre-allocate with headroom for SGR injections

	b := line
	for len(b) > 0 {
		seq, width, n, newState := ansi.DecodeSequence(b, state, p)
		state = newState

		if width == 0 {
			// Non-printable: control/escape sequence.
			if isSGR(seq) {
				if isResetSGR(seq) {
					sgrBuf.Reset()
				} else {
					sgrBuf.WriteString(seq)
				}
				// When inside a match, suppress original SGR from output
				// (it would override highlight colors) but still track it.
				if !inMatch {
					out.WriteString(seq)
				}
			} else {
				// Non-SGR sequences (OSC, cursor moves, etc.) pass through always.
				out.WriteString(seq)
			}
			b = b[n:]
			continue
		}

		// Printable character with display width > 0.

		// Check if we should start a match at this column.
		if !inMatch && matchIdx < len(matches) && col >= matches[matchIdx].start {
			// Clamp: if col already past start (wide char boundary), still start here.
			out.WriteString("\x1b[m") // reset any prior styling
			if matchIdx == selectedIdx {
				out.WriteString(selSGR)
			} else {
				out.WriteString(hiSGR)
			}
			inMatch = true
		}

		out.WriteString(seq)
		col += width

		// Check if the match ends at or before this column.
		if inMatch && col >= matches[matchIdx].end {
			out.WriteString("\x1b[m") // reset highlight
			if sgrBuf.Len() > 0 {
				out.WriteString(sgrBuf.String()) // restore original styling
			}
			inMatch = false
			matchIdx++

			// After advancing, check if the next match starts at this same column.
			// (Adjacent matches.)
			if matchIdx < len(matches) && col >= matches[matchIdx].start {
				out.WriteString("\x1b[m")
				if matchIdx == selectedIdx {
					out.WriteString(selSGR)
				} else {
					out.WriteString(hiSGR)
				}
				inMatch = true
			}
		}

		b = b[n:]
	}

	// If still inside a match at end of line, close the highlight.
	if inMatch {
		out.WriteString("\x1b[m")
	}

	return out.String()
}

// splitWrappedVisible splits a single (potentially ANSI-decorated) line into
// wrapped segments, returning only the segments visible in the window
// [startRow, startRow+numRows).
//
// The first row (row 0) is vpWidth display columns wide. Continuation rows
// (row 1+) are contWidth columns wide, leaving room for the wrap indicator
// prefix to be prepended later without losing content.
//
// Each returned segment is self-contained: it starts with the accumulated SGR
// state (so colours carry across wrap boundaries) and ends with \x1b[m if any
// SGR was active.
//
// This replaces the O(N^2) pattern of calling ansi.Cut in a loop with a single
// O(N) pass over the input.
func splitWrappedVisible(line string, vpWidth, contWidth, startRow, numRows int) (segments []string, widths []int) {
	if line == "" || numRows <= 0 || vpWidth <= 0 {
		return nil, nil
	}
	if contWidth <= 0 {
		contWidth = 1
	}

	endRow := startRow + numRows

	var (
		row      int             // current wrap row
		col      int             // current display column within row
		rowWidth = vpWidth       // current row's width limit (vpWidth for row 0, contWidth for rows 1+)
		sgrBuf   strings.Builder // accumulated SGR sequences for state tracking
		segBuf   strings.Builder // current segment being built
		segStart bool            // whether segBuf has been initialized for this row
		p        = ansi.NewParser()
		state    byte
	)

	capturing := row >= startRow && row < endRow

	// startSegment initializes segBuf for a new captured row, prepending SGR state.
	startSegment := func() {
		segBuf.Reset()
		if sgrBuf.Len() > 0 {
			segBuf.WriteString(sgrBuf.String())
		}
		segStart = true
	}

	// closeSegment finalizes the current segment and appends it to results.
	closeSegment := func() {
		if !segStart {
			return
		}
		if sgrBuf.Len() > 0 {
			segBuf.WriteString("\x1b[m")
		}
		segments = append(segments, segBuf.String())
		widths = append(widths, col)
		segStart = false
	}

	if capturing {
		startSegment()
	}

	b := line
	for len(b) > 0 {
		seq, width, n, newState := ansi.DecodeSequence(b, state, p)
		state = newState

		if width == 0 {
			// Non-printable: control/escape sequence
			if isSGR(seq) {
				if isResetSGR(seq) {
					sgrBuf.Reset()
				} else {
					sgrBuf.WriteString(seq)
				}
			}
			if capturing && segStart {
				segBuf.WriteString(seq)
			}
			b = b[n:]
			continue
		}

		// Printable character with display width > 0.

		// Check if this character would exceed the row width.
		// Guard col > 0 so a wide char at column 0 is placed on the current
		// row even if it exceeds rowWidth (avoids spurious empty segments).
		if col+width > rowWidth && col > 0 {
			// Close current segment (row boundary).
			if capturing {
				closeSegment()
			}
			row++
			if row == 1 {
				rowWidth = contWidth
			}
			col = 0
			if row >= endRow {
				break
			}
			capturing = row >= startRow && row < endRow
			if capturing {
				startSegment()
			}
		}

		// Write character to segment.
		if capturing && segStart {
			segBuf.WriteString(seq)
		}
		col += width

		// Check if we've exactly filled the row.
		if col >= rowWidth {
			if capturing {
				closeSegment()
			}
			row++
			if row == 1 {
				rowWidth = contWidth
			}
			col = 0
			if row >= endRow {
				b = b[n:]
				break
			}
			capturing = row >= startRow && row < endRow
			if capturing {
				startSegment()
			}
		}

		b = b[n:]
	}

	// Flush remaining content in the last segment.
	if capturing && segStart && col > 0 {
		closeSegment()
	}

	return segments, widths
}

// wrapIndicatorWidth is the display-column width of the wrap indicator prefix
// (↪ + space = 2 columns).
const wrapIndicatorWidth = 2

// cachedWrapIndicatorPrefix is the pre-rendered styled "↪ " string used as a
// soft-wrap continuation indicator. Computed once at package init time to avoid
// allocating a new lipgloss.Style on every wrapped row in the hot render path.
var cachedWrapIndicatorPrefix = lipgloss.NewStyle().Foreground(theme.Subtle).Faint(true).Render("↪") + " "

// lineMap maps a visual (wrapped) row back to its logical source line and the
// display-column offset within that line where the visual row begins.
type lineMap struct {
	logicalLine int
	colOffset   int
}

// wrapLines pre-wraps content lines at the given display-column width,
// producing a flat slice of wrapped visual lines and a parallel mapping from
// each visual row back to the logical source line and column offset.
//
// Continuation rows (all segments after the first for a given logical line)
// receive the wrap indicator prefix via prefixContinuationRows.
//
// If width <= 0, the input lines are returned as-is with identity mapping.
func wrapLines(lines []string, width int) (wrapped []string, mapping []lineMap) {
	if width <= 0 {
		wrapped = make([]string, len(lines))
		mapping = make([]lineMap, len(lines))
		copy(wrapped, lines)
		for i := range lines {
			mapping[i] = lineMap{logicalLine: i, colOffset: 0}
		}
		return wrapped, mapping
	}

	for i, line := range lines {
		lineWidth := ansi.StringWidth(line)

		if lineWidth <= width {
			// Line fits in a single visual row.
			wrapped = append(wrapped, line)
			mapping = append(mapping, lineMap{logicalLine: i, colOffset: 0})
			continue
		}

		// Line needs wrapping: first segment at full width, continuation
		// segments at contWidth (narrower to make room for ↪ prefix).
		contWidth := width - wrapIndicatorWidth
		if contWidth <= 0 {
			contWidth = 1
		}
		var segs []string
		var segWidths []int
		col := 0
		for col < lineWidth {
			var segW int
			if col == 0 {
				segW = width
			} else {
				segW = contWidth
			}
			end := col + segW
			if end > lineWidth {
				end = lineWidth
			}
			seg := ansi.Cut(line, col, end)
			actualW := ansi.StringWidth(seg)
			if actualW == 0 {
				// Wide character doesn't fit in contWidth — skip to end to
				// avoid an infinite loop (mirrors the col > 0 guard in
				// splitWrappedVisible).
				col = end
				continue
			}
			segs = append(segs, seg)
			segWidths = append(segWidths, actualW)
			mapping = append(mapping, lineMap{logicalLine: i, colOffset: col})
			col += actualW
		}

		// Add wrap indicator prefix to continuation rows.
		prefixContinuationRows(segs, segWidths, cachedWrapIndicatorPrefix, wrapIndicatorWidth, false)

		wrapped = append(wrapped, segs...)
	}

	return wrapped, mapping
}

// prefixContinuationRows prepends prefix to continuation segments in place.
// Segments are expected to already be split at the narrower continuation width,
// so the prefix is simply prepended and widths are increased by prefixWidth.
//
// By default the first segment (index 0) is the original first row and is
// skipped. When firstIsCont is true, all segments including index 0 are treated
// as continuation rows.
func prefixContinuationRows(segs []string, widths []int, prefix string, prefixWidth int, firstIsCont bool) {
	start := 1
	if firstIsCont {
		start = 0
	}
	for i := start; i < len(segs); i++ {
		segs[i] = prefix + segs[i]
		widths[i] += prefixWidth
	}
}
