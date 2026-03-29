package highlight

import "github.com/aohoyd/aku/internal/theme"

// DefaultPipeline constructs the standard log highlighting pipeline with all
// highlighters wired to Kanagawa theme colors.
//
// Pipeline order:
//  1. JSON (raw)          — structural tokens + keys
//  2. Log levels          — ERROR, WARN, INFO, DEBUG, TRACE
//  3. Timestamps          — date / time / timezone
//  4. IPv4                — dotted-quad addresses
//  5. IPv6                — colon-hex addresses
//  6. URLs                — protocol / host / path / query / symbols
//  7. Unix paths          — segments / separators
//  8. Key-value (logfmt)  — key / '=' separator
//  9. UUIDs               — 8-4-4-4-12 hex
//  10. Numbers            — integer and decimal
//  11. Keywords           — HTTP methods, booleans, null
//  12. Quotes (raw)       — double-quoted strings
func DefaultPipeline() *Pipeline {
	return NewPipelineBuilder().
		// 1. JSON — raw mode (needs to see structure before any ANSI is injected)
		AddRaw(NewJSONHighlighter(
			FgPainter(theme.SyntaxKey),    // key names
			FgPainter(theme.SyntaxMarker), // structural tokens: {}[],:
		)).
		// 2. Log levels
		Add(NewLogLevelHighlighter(
			BoldFgPainter(theme.Error),     // ERROR, ERR, FATAL
			FgPainter(theme.Warning),       // WARN, WARNING
			FgPainter(theme.SyntaxValue),   // INFO
			FgPainter(theme.StatusRunning), // DEBUG, DBG
			FaintFgPainter(theme.Muted),    // TRACE
		)).
		// 3. Timestamps
		Add(NewTimestampHighlighter(
			FgPainter(theme.LogTimestamp), // date part
			FgPainter(theme.LogTime),      // time part
			FgPainter(theme.LogTimezone),  // timezone part
		)).
		// 4. IPv4
		Add(NewIPv4Highlighter(
			FgPainter(theme.LogIP),
		)).
		// 5. IPv6
		Add(NewIPv6Highlighter(
			FgPainter(theme.LogIP),
		)).
		// 6. URLs
		Add(NewURLHighlighter(
			FgPainter(theme.SyntaxKey),    // protocol (http/https)
			FgPainter(theme.SyntaxString), // host
			FgPainter(theme.SyntaxKey),    // path
			FgPainter(theme.Muted),        // query string
			FgPainter(theme.Muted),        // "://" symbol
		)).
		// 7. Unix paths
		Add(NewPathHighlighter(
			FgPainter(theme.SyntaxString), // path segments
			FgPainter(theme.Muted),        // "/" separators
		)).
		// 8. Key-value (logfmt)
		Add(NewKeyValueHighlighter(
			FaintFgPainter(theme.Highlight), // key (faint)
			FgPainter(theme.Muted),          // "=" separator
		)).
		// 9. UUIDs
		Add(NewUUIDHighlighter(
			FgPainter(theme.Accent),
		)).
		// 10. Numbers
		Add(NewNumberHighlighter(
			FgPainter(theme.SyntaxNumber),
		)).
		// 11. Keywords — HTTP methods, booleans, null
		Add(NewKeywordHighlighter([]KeywordGroup{
			{
				Words:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},
				Painter: BgFgPainter(theme.Accent, theme.TextOnAccent),
			},
			{
				Words:   []string{"true", "false"},
				Painter: FgPainter(theme.SyntaxBool),
			},
			{
				Words:   []string{"null", "nil", "None"},
				Painter: FgPainter(theme.SyntaxNull),
			},
		})).
		// 12. Quotes — raw mode (needs to see ANSI from previous steps)
		AddRaw(NewQuoteHighlighter('"', FgPainter(theme.SyntaxString))).
		Build()
}
