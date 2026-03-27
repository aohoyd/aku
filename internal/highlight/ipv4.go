package highlight

import (
	"regexp"
	"strconv"
	"strings"
)

// IPv4Highlighter colorizes IPv4 addresses (with optional port) found within a line.
type IPv4Highlighter struct {
	re      *regexp.Regexp
	painter Painter
}

// NewIPv4Highlighter creates an IPv4Highlighter with the given painter.
func NewIPv4Highlighter(painter Painter) *IPv4Highlighter {
	return &IPv4Highlighter{
		re:      regexp.MustCompile(`\b(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})(:\d+)?\b`),
		painter: painter,
	}
}

// Highlight scans the line for valid IPv4 addresses and colorizes them.
// Returns the original string (same pointer) when no match is found.
func (h *IPv4Highlighter) Highlight(line string) string {
	// Early-exit guard: need at least 3 dots for an IPv4 address.
	if strings.Count(line, ".") < 3 {
		return line
	}

	matches := h.re.FindAllStringSubmatchIndex(line, -1)
	if len(matches) == 0 {
		return line
	}

	var sb strings.Builder
	sb.Grow(len(line) + 32)
	prev := 0
	painted := false

	for _, loc := range matches {
		// loc layout:
		// [0:1] = full match start/end
		// [2:3] = octet 1
		// [4:5] = octet 2
		// [6:7] = octet 3
		// [8:9] = octet 4
		// [10:11] = optional port group (-1 if not present)

		// Validate each octet is 0-255.
		valid := true
		for i := 2; i <= 8; i += 2 {
			octet := line[loc[i]:loc[i+1]]
			n, err := strconv.Atoi(octet)
			if err != nil || n < 0 || n > 255 {
				valid = false
				break
			}
		}
		if !valid {
			continue
		}

		// Write text before this match.
		sb.WriteString(line[prev:loc[0]])
		// Paint the entire match (IP + optional port).
		h.painter.WriteTo(&sb, line[loc[0]:loc[1]])
		prev = loc[1]
		painted = true
	}

	if !painted {
		return line
	}

	// Write remaining text after the last match.
	if prev < len(line) {
		sb.WriteString(line[prev:])
	}
	return sb.String()
}
