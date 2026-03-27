package highlight

import (
	"regexp"
	"strings"
)

// logLevelRe matches log level keywords at word boundaries, case-insensitively.
var logLevelRe = regexp.MustCompile(`(?i)\b(ERROR|ERR|FATAL|WARN|WARNING|INFO|DEBUG|DBG|TRACE)\b`)

// LogLevelHighlighter colorizes log level keywords (ERROR, WARN, INFO, etc.)
// found within a line. Each severity maps to a distinct Painter.
type LogLevelHighlighter struct {
	re           *regexp.Regexp
	errorPainter Painter // ERROR, ERR, FATAL
	warnPainter  Painter // WARN, WARNING
	infoPainter  Painter // INFO
	debugPainter Painter // DEBUG, DBG
	tracePainter Painter // TRACE
}

// NewLogLevelHighlighter creates a LogLevelHighlighter with painters for each
// severity: error (ERROR/ERR/FATAL), warn (WARN/WARNING), info (INFO),
// debug (DEBUG/DBG), and trace (TRACE).
func NewLogLevelHighlighter(error_, warn, info, debug, trace Painter) *LogLevelHighlighter {
	return &LogLevelHighlighter{
		re:           logLevelRe,
		errorPainter: error_,
		warnPainter:  warn,
		infoPainter:  info,
		debugPainter: debug,
		tracePainter: trace,
	}
}

// Highlight scans line for log level keywords and wraps each match with the
// appropriate ANSI color. If no matches are found, the original string (same
// pointer) is returned for the zero-alloc fast path.
func (h *LogLevelHighlighter) Highlight(line string) string {
	matches := h.re.FindAllStringSubmatchIndex(line, -1)
	if len(matches) == 0 {
		return line
	}

	var sb strings.Builder
	sb.Grow(len(line) + len(matches)*23)

	prev := 0
	for _, loc := range matches {
		// loc[0]:loc[1] is the full match, loc[2]:loc[3] is the capture group.
		// They are the same for this regex but we use the full match offsets.
		start, end := loc[0], loc[1]

		// Append text before the match.
		if prev < start {
			sb.WriteString(line[prev:start])
		}

		matched := line[start:end]
		p := h.painterFor(matched)
		p.WriteTo(&sb, matched)

		prev = end
	}

	// Append any remaining text after the last match.
	if prev < len(line) {
		sb.WriteString(line[prev:])
	}

	return sb.String()
}

// painterFor returns the appropriate Painter for the given matched keyword.
func (h *LogLevelHighlighter) painterFor(matched string) Painter {
	switch strings.ToUpper(matched) {
	case "ERROR", "ERR", "FATAL":
		return h.errorPainter
	case "WARN", "WARNING":
		return h.warnPainter
	case "INFO":
		return h.infoPainter
	case "DEBUG", "DBG":
		return h.debugPainter
	case "TRACE":
		return h.tracePainter
	default:
		return Painter{}
	}
}
