package highlight

import (
	"regexp"
	"strings"
)

// timestampPattern matches ISO 8601 timestamps with optional timezone.
// Group 1: date (2024-03-22)
// Group 2: time (14:30:00 or 14:30:00.123456)
// Group 3: timezone (Z or +05:30 or +0530) — optional
var timestampPattern = regexp.MustCompile(
	`(\d{4}-\d{2}-\d{2})[T ](\d{2}:\d{2}:\d{2}(?:\.\d+)?)(Z|[+-]\d{2}:?\d{2})?`,
)

// TimestampHighlighter colorizes timestamp parts (date, time, timezone)
// found within a line. Each part gets its own Painter.
type TimestampHighlighter struct {
	re          *regexp.Regexp
	datePainter Painter
	timePainter Painter
	tzPainter   Painter
}

// NewTimestampHighlighter creates a TimestampHighlighter with separate painters
// for the date, time, and timezone portions of matched timestamps.
func NewTimestampHighlighter(date, time_, tz Painter) *TimestampHighlighter {
	return &TimestampHighlighter{
		re:          timestampPattern,
		datePainter: date,
		timePainter: time_,
		tzPainter:   tz,
	}
}

// Highlight scans the line for timestamps and colorizes matched parts.
// Returns the original string (same pointer) when no match is found.
func (h *TimestampHighlighter) Highlight(line string) string {
	// Early-exit guard: timestamps require both '-' and ':'
	if strings.IndexByte(line, '-') < 0 || strings.IndexByte(line, ':') < 0 {
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
		// loc[0], loc[1] = full match
		// loc[2], loc[3] = group 1 (date)
		// loc[4], loc[5] = group 2 (time)
		// loc[6], loc[7] = group 3 (timezone, may be -1 if absent)

		// Write text before this match
		if prev < loc[0] {
			sb.WriteString(line[prev:loc[0]])
		}

		// Paint date part (group 1)
		h.datePainter.WriteTo(&sb, line[loc[2]:loc[3]])

		// Write the separator (T or space) between date and time
		// It's the character right after the date group and before the time group
		if loc[3] < loc[4] {
			sb.WriteString(line[loc[3]:loc[4]])
		}

		// Paint time part (group 2)
		h.timePainter.WriteTo(&sb, line[loc[4]:loc[5]])

		// Paint timezone part (group 3) if present
		if loc[6] >= 0 && loc[7] >= 0 {
			h.tzPainter.WriteTo(&sb, line[loc[6]:loc[7]])
		}

		prev = loc[1]
	}

	// Write any remaining text after the last match
	if prev < len(line) {
		sb.WriteString(line[prev:])
	}

	return sb.String()
}
