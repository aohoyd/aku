package highlight

import "strings"

// ApplyToUnhighlighted applies a Highlighter only to the portions of line that
// are not already wrapped in ANSI escape sequences. It performs a single-pass
// scan, splitting the line into "styled" regions (between \x1b[...m and
// \x1b[0m) and "plain" regions, applying h.Highlight only to plain regions.
//
// If no segment was modified (pointer equality), the original line is returned.
func ApplyToUnhighlighted(line string, h Highlighter) string {
	// Fast path: no ANSI escapes at all — highlight the whole line.
	if strings.IndexByte(line, '\x1b') == -1 {
		return h.Highlight(line)
	}

	// Collect segments: each segment is a slice of the original line,
	// tagged as either plain or styled.
	type segment struct {
		start, end int
		styled     bool
	}

	var segs []segment
	inStyled := false
	segStart := 0
	i := 0

	for i < len(line) {
		if line[i] != '\x1b' {
			i++
			continue
		}

		// Found ESC — try to parse an SGR sequence \x1b[...m
		seqStart := i
		i++ // skip \x1b
		if i >= len(line) || line[i] != '[' {
			// Not a CSI sequence, treat as plain text
			i++
			continue
		}
		i++ // skip '['

		// Scan parameter bytes until we hit a letter
		for i < len(line) && ((line[i] >= '0' && line[i] <= '9') || line[i] == ';') {
			i++
		}

		if i >= len(line) || line[i] != 'm' {
			// Not an SGR sequence (wrong terminator or truncated), skip
			i++
			continue
		}
		i++ // skip 'm'
		seqEnd := i

		seq := line[seqStart:seqEnd]
		isReset := seq == "\x1b[0m" || seq == "\x1b[m"

		if !inStyled {
			if isReset {
				// Reset outside styled — include it as part of the plain segment
				continue
			}
			// Entering styled region.
			// Flush plain text before the SGR as a plain segment.
			if seqStart > segStart {
				segs = append(segs, segment{segStart, seqStart, false})
			}
			// The SGR + styled content starts here.
			segStart = seqStart
			inStyled = true
		} else {
			if isReset {
				// End of styled region — flush styled segment including the reset.
				segs = append(segs, segment{segStart, seqEnd, true})
				segStart = seqEnd
				inStyled = false
			}
			// Non-reset SGR inside styled — keep scanning
		}
	}

	// Flush trailing segment.
	if segStart < len(line) {
		segs = append(segs, segment{segStart, len(line), inStyled})
	}

	// Now process segments: apply highlighter to plain segments only.
	var sb strings.Builder
	modified := false

	for _, seg := range segs {
		text := line[seg.start:seg.end]
		if seg.styled {
			if modified {
				sb.WriteString(text)
			}
			continue
		}

		// Plain segment — apply highlighter.
		highlighted := h.Highlight(text)
		if highlighted != text {
			if !modified {
				// Lazy init: copy everything before this segment.
				modified = true
				sb.Grow(len(line) + 64)
				sb.WriteString(line[:seg.start])
			}
			sb.WriteString(highlighted)
		} else {
			if modified {
				sb.WriteString(text)
			}
		}
	}

	if !modified {
		return line
	}
	return sb.String()
}
