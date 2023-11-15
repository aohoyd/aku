package ui

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/aohoyd/aku/internal/theme"
	"github.com/charmbracelet/x/ansi"
)

const (
	// Reverse video for search highlights. SGR 7 swaps fg/bg, SGR 27
	// restores the original fg/bg relationship without resetting any
	// colors — so the table's selected-row background is preserved.
	highlightOn  = "\x1b[7m"
	highlightOff = "\x1b[27m"
)

var (
	highlightMatchOn  string
	highlightMatchOff = "\x1b[49m\x1b[39m"
)

func init() {
	highlightMatchOn = fmt.Sprintf("\x1b[%sm\x1b[%sm",
		colorToSGR(string(theme.SearchMatch), true),
		colorToSGR(string(theme.SearchFg), false))
}

// colorToSGR converts a theme color string to an SGR parameter.
// Accepts ANSI-256 index ("43") or hex ("#FF5555").
// bg selects background (48) vs foreground (38).
func colorToSGR(c string, bg bool) string {
	base := 38
	if bg {
		base = 48
	}
	if strings.HasPrefix(c, "#") && len(c) == 7 {
		r, err1 := strconv.ParseUint(c[1:3], 16, 8)
		g, err2 := strconv.ParseUint(c[3:5], 16, 8)
		b, err3 := strconv.ParseUint(c[5:7], 16, 8)
		if err1 != nil || err2 != nil || err3 != nil {
			return fmt.Sprintf("%d;5;1", base) // fallback to red
		}
		return fmt.Sprintf("%d;2;%d;%d;%d", base, r, g, b)
	}
	return fmt.Sprintf("%d;5;%s", base, c)
}

// HighlightMatches wraps regex matches with reverse-video ANSI codes (for the active/cursor row).
func HighlightMatches(cell string, re *regexp.Regexp) string {
	return highlightWith(cell, re, highlightOn, highlightOff)
}

// HighlightMatchesColor wraps regex matches with themed color ANSI codes (for inactive rows).
func HighlightMatchesColor(cell string, re *regexp.Regexp) string {
	return highlightWith(cell, re, highlightMatchOn, highlightMatchOff)
}

// highlightWith wraps regex matches in a cell string with the given on/off ANSI codes.
// It strips ANSI for matching, then walks the original string to inject highlights
// at correct positions, preserving existing ANSI escape sequences.
func highlightWith(cell string, re *regexp.Regexp, on, off string) string {
	if re == nil || cell == "" {
		return cell
	}

	stripped := ansi.Strip(cell)
	allLocs := re.FindAllStringIndex(stripped, -1)
	// Filter out zero-width matches that would only insert empty highlight markers.
	var locs [][]int
	for _, loc := range allLocs {
		if loc[0] != loc[1] {
			locs = append(locs, loc)
		}
	}
	if len(locs) == 0 {
		return cell
	}

	// Build mapping from visual (stripped) byte offset to original byte offset.
	// Walk both strings: skip ANSI escapes in original without advancing visual pos.
	visualToOrig := make([]int, len(stripped)+1)
	oi, vi := 0, 0
	for oi < len(cell) && vi <= len(stripped) {
		if cell[oi] == '\x1b' {
			// Skip entire escape sequence
			oi++
			for oi < len(cell) && !isEscTerminator(cell[oi]) {
				oi++
			}
			if oi < len(cell) {
				oi++ // consume terminator
			}
			continue
		}
		if vi < len(stripped) {
			visualToOrig[vi] = oi
		}
		oi++
		vi++
	}
	visualToOrig[len(stripped)] = oi // sentinel

	var sb strings.Builder
	sb.Grow(len(cell) + len(locs)*(len(on)+len(off)))
	lastOrig := 0

	for _, loc := range locs {
		if loc[0] > len(stripped) || loc[1] > len(stripped) {
			continue
		}
		origStart := visualToOrig[loc[0]]
		origEnd := visualToOrig[loc[1]]

		chunk := cell[lastOrig:origStart]
		sb.WriteString(chunk)
		sb.WriteString(on)
		sb.WriteString(cell[origStart:origEnd])
		sb.WriteString(off)
		// Re-emit ANSI escapes that were active before the match so that
		// the off sequence (which resets colors) doesn't clobber cell styling.
		sb.WriteString(collectSGR(cell[:origStart]))
		lastOrig = origEnd
	}
	sb.WriteString(cell[lastOrig:])
	return sb.String()
}

// collectSGR scans s for SGR escape sequences (\x1b[...m) and returns
// the concatenation of all found. This captures the active fg/bg color
// state so it can be re-emitted after a highlight-off reset.
func collectSGR(s string) string {
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\x1b' {
			continue
		}
		start := i
		i++
		for i < len(s) && !isEscTerminator(s[i]) {
			i++
		}
		if i < len(s) && s[i] == 'm' {
			sb.WriteString(s[start : i+1])
		}
	}
	return sb.String()
}

// isEscTerminator reports whether b is the final byte of an ANSI escape sequence.
func isEscTerminator(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
}
