package highlight

import (
	"regexp"
	"strings"
)

// keyValueRe matches key=value patterns where the key is a word character sequence
// preceded by the start of line or whitespace, and the separator is '='.
// Groups: 1=key, 2='=' separator.
var keyValueRe = regexp.MustCompile(`(?:^|\s)(\w+)(=)`)

// KeyValueHighlighter colorizes logfmt-style key=value pairs found within a line.
// Only the key and the '=' separator are painted; the value is left untouched
// for downstream highlighters (numbers, quotes, keywords, etc.).
type KeyValueHighlighter struct {
	re       *regexp.Regexp
	keyPaint Painter
	sepPaint Painter
}

// NewKeyValueHighlighter creates a KeyValueHighlighter with painters for the
// key and the '=' separator.
func NewKeyValueHighlighter(key, sep Painter) *KeyValueHighlighter {
	return &KeyValueHighlighter{
		re:       keyValueRe,
		keyPaint: key,
		sepPaint: sep,
	}
}

// Highlight scans line for key=value patterns and wraps each key and '='
// with the appropriate ANSI color. If no matches are found, the original
// string (same pointer) is returned for the zero-alloc fast path.
func (h *KeyValueHighlighter) Highlight(line string) string {
	// Early-exit guard: no '=' means no key=value pairs.
	if strings.IndexByte(line, '=') < 0 {
		return line
	}

	matches := h.re.FindAllStringSubmatchIndex(line, -1)
	if len(matches) == 0 {
		return line
	}

	var sb strings.Builder
	sb.Grow(len(line) + len(matches)*24)

	prev := 0
	for _, loc := range matches {
		// loc layout:
		// [0:1] = full match start/end (includes optional leading whitespace)
		// [2:3] = capture group 1: key
		// [4:5] = capture group 2: '='

		fullStart := loc[0]
		keyStart, keyEnd := loc[2], loc[3]
		sepStart, sepEnd := loc[4], loc[5]

		// Copy text before the full match.
		if prev < fullStart {
			sb.WriteString(line[prev:fullStart])
		}

		// Copy leading whitespace (between full match start and key start) as-is.
		if fullStart < keyStart {
			sb.WriteString(line[fullStart:keyStart])
		}

		// Paint the key.
		h.keyPaint.WriteTo(&sb, line[keyStart:keyEnd])

		// Paint the '=' separator.
		h.sepPaint.WriteTo(&sb, line[sepStart:sepEnd])

		prev = sepEnd
	}

	// Copy remaining text after the last match.
	if prev < len(line) {
		sb.WriteString(line[prev:])
	}

	return sb.String()
}
