package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
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

// splitWrappedVisible splits a single (potentially ANSI-decorated) line into
// wrapped segments of vpWidth display columns, returning only the segments
// visible in the window [startRow, startRow+numRows).
//
// Each returned segment is self-contained: it starts with the accumulated SGR
// state (so colours carry across wrap boundaries) and ends with \x1b[m if any
// SGR was active.
//
// This replaces the O(N^2) pattern of calling ansi.Cut in a loop with a single
// O(N) pass over the input.
func splitWrappedVisible(line string, vpWidth, startRow, numRows int) (segments []string, widths []int) {
	if line == "" || numRows <= 0 || vpWidth <= 0 {
		return nil, nil
	}

	endRow := startRow + numRows

	var (
		row      int             // current wrap row
		col      int             // current display column within row
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
		// row even if it exceeds vpWidth (avoids spurious empty segments).
		if col+width > vpWidth && col > 0 {
			// Close current segment (row boundary).
			if capturing {
				closeSegment()
			}
			row++
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
		if col >= vpWidth {
			if capturing {
				closeSegment()
			}
			row++
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
