package highlight

import (
	"net"
	"regexp"
	"strings"
)

// ipv6Re broadly matches potential IPv6 candidates: sequences of hex digits and
// colons with at least two colons, optionally followed by a CIDR prefix (/NNN).
// Word boundaries (\b) don't work well with leading "::" so we use explicit
// non-hex/colon character boundaries instead.
var ipv6Re = regexp.MustCompile(`(?:^|[^0-9a-fA-F:])([0-9a-fA-F]*(?::[0-9a-fA-F]*){2,7})(?:/(\d{1,3}))?(?:[^0-9a-fA-F:/]|$)`)

// IPv6Highlighter colorizes valid IPv6 addresses found within a line.
// Each candidate is validated with net.ParseIP before painting.
type IPv6Highlighter struct {
	re      *regexp.Regexp
	painter Painter
}

// NewIPv6Highlighter creates an IPv6Highlighter with the given painter.
func NewIPv6Highlighter(painter Painter) *IPv6Highlighter {
	return &IPv6Highlighter{
		re:      ipv6Re,
		painter: painter,
	}
}

// Highlight scans line for IPv6 addresses and paints them.
// Returns the original string (same pointer) when no valid match is found.
func (h *IPv6Highlighter) Highlight(line string) string {
	// Early-exit guard: IPv6 always contains colons.
	if strings.IndexByte(line, ':') < 0 {
		return line
	}

	matches := h.re.FindAllStringSubmatchIndex(line, -1)
	if len(matches) == 0 {
		return line
	}

	// Collect valid match regions (start, end) including optional /prefix.
	type region struct {
		start, end int
	}
	var regions []region

	for _, loc := range matches {
		// loc[0], loc[1] = full match
		// loc[2], loc[3] = group 1 (hex:colon part)
		// loc[4], loc[5] = group 2 (prefix length, or -1 if absent)
		if loc[2] < 0 {
			continue
		}
		candidate := line[loc[2]:loc[3]]

		ip := net.ParseIP(candidate)
		if ip == nil {
			continue
		}
		// Exclude IPv4 addresses that happen to parse (e.g., mapped addresses).
		// A true IPv6 has To4() == nil.
		if ip.To4() != nil {
			continue
		}

		// The painted region includes the IP and optional /prefix.
		end := loc[3]
		if loc[4] >= 0 && loc[5] >= 0 {
			// Include the "/" and prefix digits.
			end = loc[5]
		}
		regions = append(regions, region{start: loc[2], end: end})
	}

	if len(regions) == 0 {
		return line
	}

	var sb strings.Builder
	sb.Grow(len(line) + len(regions)*20)

	prev := 0
	for _, r := range regions {
		if prev < r.start {
			sb.WriteString(line[prev:r.start])
		}
		h.painter.WriteTo(&sb, line[r.start:r.end])
		prev = r.end
	}
	if prev < len(line) {
		sb.WriteString(line[prev:])
	}

	return sb.String()
}
