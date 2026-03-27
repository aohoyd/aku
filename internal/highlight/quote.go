package highlight

import "strings"

// QuoteHighlighter colorizes quoted strings using a character-by-character
// state machine. It runs in "raw" mode (last in the pipeline) and handles
// ANSI resets from previous highlighters by re-injecting its own color after
// each reset sequence found inside a quoted region.
type QuoteHighlighter struct {
	quoteChar byte
	painter   Painter
}

// NewQuoteHighlighter creates a QuoteHighlighter for the given quote character
// and painter.
func NewQuoteHighlighter(quoteChar byte, painter Painter) *QuoteHighlighter {
	return &QuoteHighlighter{
		quoteChar: quoteChar,
		painter:   painter,
	}
}

// Highlight scans the line for balanced pairs of quoteChar and wraps each
// quoted region (including the quote characters) with ANSI color. If the
// number of quote characters is odd (unbalanced), the original string is
// returned unchanged.
//
// Inside a quoted region, if an ANSI reset (\x1b[0m) is detected (from a
// previous highlighter), the reset is written through and the quote color
// is re-injected so that the remaining quoted text stays colored.
func (h *QuoteHighlighter) Highlight(line string) string {
	// Fast path: no quote character at all.
	if strings.IndexByte(line, h.quoteChar) < 0 {
		return line
	}

	// Count quote characters. If odd, return unchanged (unbalanced).
	count := 0
	for i := 0; i < len(line); i++ {
		if line[i] == h.quoteChar {
			count++
		}
	}
	if count%2 != 0 {
		return line
	}

	prefix := h.painter.Prefix()

	var sb strings.Builder
	sb.Grow(len(line) + count*len(prefix+AnsiReset))

	insideQuote := false

	for i := 0; i < len(line); i++ {
		ch := line[i]

		if ch == h.quoteChar {
			if !insideQuote {
				// Entering a quoted region: write painter prefix, then the quote char.
				insideQuote = true
				sb.WriteString(prefix)
				sb.WriteByte(ch)
			} else {
				// Leaving a quoted region: write the closing quote char, then reset.
				insideQuote = false
				sb.WriteByte(ch)
				sb.WriteString(AnsiReset)
			}
			continue
		}

		// Inside a quote, detect ANSI reset sequences:
		// \x1b[0m (4 bytes) or short-form \x1b[m (3 bytes).
		if insideQuote && ch == '\x1b' && i+2 < len(line) && line[i+1] == '[' {
			if line[i+2] == 'm' {
				// Short-form reset \x1b[m
				sb.WriteString("\x1b[m")
				sb.WriteString(prefix)
				i += 2
				continue
			}
			if i+3 < len(line) && line[i+2] == '0' && line[i+3] == 'm' {
				// Standard reset \x1b[0m
				sb.WriteString(AnsiReset)
				sb.WriteString(prefix)
				i += 3
				continue
			}
		}

		// Default: copy character through.
		sb.WriteByte(ch)
	}

	return sb.String()
}
