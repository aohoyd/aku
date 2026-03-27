// Package highlight provides a composable pipeline for log line highlighting
// using raw ANSI escape codes (no lipgloss) for performance.
package highlight

// AnsiReset is the SGR reset sequence that clears all attributes.
const AnsiReset = "\x1b[0m"

// Highlighter transforms a line by wrapping matched regions with ANSI codes.
// Implementations MUST return the original string (same pointer) when no match
// is found — this is the zero-alloc fast path.
type Highlighter interface {
	Highlight(line string) string
}
