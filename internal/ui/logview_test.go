package ui

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/aohoyd/aku/internal/msgs"
	"github.com/charmbracelet/x/ansi"
)

func TestLogView_AppendLine(t *testing.T) {
	lv := NewLogView(80, 24, 100, "15m", 900)
	lv.AppendLine("hello")
	lv.AppendLine("world")
	if lv.buffer.Len() != 2 {
		t.Fatalf("expected 2 lines, got %d", lv.buffer.Len())
	}
}

func TestLogView_Autoscroll(t *testing.T) {
	lv := NewLogView(80, 24, 100, "15m", 900)
	if !lv.Autoscroll() {
		t.Fatal("autoscroll should be on by default")
	}
	lv.ToggleAutoscroll()
	if lv.Autoscroll() {
		t.Fatal("autoscroll should be off after toggle")
	}
	lv.ToggleAutoscroll()
	if !lv.Autoscroll() {
		t.Fatal("autoscroll should be on after second toggle")
	}
}

func TestLogView_Searchable(t *testing.T) {
	lv := NewLogView(80, 24, 100, "15m", 900)
	var _ Searchable = &lv
}

func TestLogView_FilterHidesLines(t *testing.T) {
	lv := NewLogView(80, 24, 100, "15m", 900)
	lv.AppendLine("error: something failed")
	lv.AppendLine("info: all good")
	lv.AppendLine("error: another failure")
	if err := lv.ApplySearch("error", msgs.SearchModeFilter); err != nil {
		t.Fatalf("ApplySearch: %v", err)
	}
	if !lv.FilterActive() {
		t.Fatal("filter should be active")
	}
}

func TestLogView_SearchMatchPositions(t *testing.T) {
	lv := NewLogView(80, 24, 100, "15m", 900)
	lv.AppendLine("foo bar")
	lv.AppendLine("foo baz")
	lv.AppendLine("foo qux")

	if err := lv.ApplySearch("foo", msgs.SearchModeSearch); err != nil {
		t.Fatalf("ApplySearch: %v", err)
	}
	if len(lv.matchPositions) != 3 {
		t.Fatalf("expected 3 matches, got %d", len(lv.matchPositions))
	}
	if lv.matchIndex != 0 {
		t.Fatalf("expected initial matchIndex 0, got %d", lv.matchIndex)
	}
}

func TestLogView_SearchNavigationWraps(t *testing.T) {
	lv := NewLogView(80, 24, 100, "15m", 900)
	lv.AppendLine("foo bar")
	lv.AppendLine("foo baz")

	if err := lv.ApplySearch("foo", msgs.SearchModeSearch); err != nil {
		t.Fatalf("ApplySearch: %v", err)
	}
	if len(lv.matchPositions) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(lv.matchPositions))
	}

	// Forward navigation
	lv.SearchNext()
	if lv.matchIndex != 1 {
		t.Fatalf("expected matchIndex 1 after SearchNext, got %d", lv.matchIndex)
	}
	lv.SearchNext()
	if lv.matchIndex != 0 {
		t.Fatalf("expected matchIndex 0 after wrap, got %d", lv.matchIndex)
	}

	// Backward navigation
	lv.SearchPrev()
	if lv.matchIndex != 1 {
		t.Fatalf("expected matchIndex 1 after SearchPrev wrap, got %d", lv.matchIndex)
	}
}

func TestLogView_ClearSearchResetsMatchState(t *testing.T) {
	lv := NewLogView(80, 24, 100, "15m", 900)
	lv.AppendLine("foo bar")

	if err := lv.ApplySearch("foo", msgs.SearchModeSearch); err != nil {
		t.Fatalf("ApplySearch: %v", err)
	}
	if len(lv.matchPositions) == 0 {
		t.Fatal("expected match positions before clear")
	}

	lv.ClearSearch()
	if lv.matchPositions != nil {
		t.Fatal("expected nil matchPositions after ClearSearch")
	}
	if lv.matchIndex != -1 {
		t.Fatalf("expected matchIndex -1 after ClearSearch, got %d", lv.matchIndex)
	}
}

func TestLogView_FilterEvictionTriggersRebuild(t *testing.T) {
	// Capacity 5 buffer, filter active for "match"
	// Uses "match" instead of "error" to avoid collision with builtin log level highlighting.
	lv := NewLogView(80, 24, 5, "15m", 900)
	if err := lv.ApplySearch("match", msgs.SearchModeFilter); err != nil {
		t.Fatalf("ApplySearch: %v", err)
	}

	// Fill buffer: 3 matching, 2 non-matching
	lv.AppendLine("match: one")
	lv.AppendLine("other: two")
	lv.AppendLine("match: three")
	lv.AppendLine("other: four")
	lv.AppendLine("match: five")

	// Buffer is now full (5/5). Viewport should show 3 matching lines.
	content := lv.View()
	if !strings.Contains(content, "match: one") {
		t.Fatal("expected 'match: one' in viewport before eviction")
	}

	// Append non-matching line -> evicts "match: one" (oldest, which matched)
	lv.AppendLine("other: six")

	// After eviction, "match: one" should be gone from viewport
	content = lv.View()
	if strings.Contains(content, "match: one") {
		t.Fatal("evicted line 'match: one' should not appear in viewport after eviction")
	}
	// Remaining matches should still be visible
	if !strings.Contains(content, "match: three") {
		t.Fatal("expected 'match: three' to remain visible")
	}
	if !strings.Contains(content, "match: five") {
		t.Fatal("expected 'match: five' to remain visible")
	}
}

func TestLogView_FilterEvictionWithMatchingLine(t *testing.T) {
	lv := NewLogView(80, 24, 3, "15m", 900)
	if err := lv.ApplySearch("match", msgs.SearchModeFilter); err != nil {
		t.Fatalf("ApplySearch: %v", err)
	}
	lv.AppendLine("match: one")
	lv.AppendLine("other: two")
	lv.AppendLine("match: three")
	// Buffer full: ["match: one", "other: two", "match: three"]

	// Append matching line that also triggers eviction
	lv.AppendLine("match: four") // evicts "match: one"
	// Buffer: ["other: two", "match: three", "match: four"]

	content := lv.View()
	if !strings.Contains(content, "match: four") {
		t.Fatal("new matching line 'match: four' should be visible after eviction")
	}
	if !strings.Contains(content, "match: three") {
		t.Fatal("existing matching line 'match: three' should remain visible")
	}
	if strings.Contains(content, "match: one") {
		t.Fatal("evicted line 'match: one' should not appear")
	}
}

func TestLogView_SearchDoesNotMatchDroppedIndicator(t *testing.T) {
	// Capacity 3 buffer — fill with 4 lines to trigger dropped indicator
	lv := NewLogView(80, 24, 3, "15m", 900)
	lv.AppendLine("alpha")
	lv.AppendLine("beta")
	lv.AppendLine("gamma")
	lv.AppendLine("delta") // evicts "alpha", dropped=1

	// Search for "dropped" — should NOT match the indicator text
	if err := lv.ApplySearch("dropped", msgs.SearchModeSearch); err != nil {
		t.Fatalf("ApplySearch: %v", err)
	}
	if len(lv.matchPositions) != 0 {
		t.Fatalf("expected 0 matches for 'dropped' (indicator should not be searchable), got %d", len(lv.matchPositions))
	}
}

func TestLogView_SearchMatchIndicesWithIndicator(t *testing.T) {
	// Capacity 3 buffer — fill with 4 lines to trigger dropped indicator
	lv := NewLogView(80, 24, 3, "15m", 900)
	lv.AppendLine("alpha")
	lv.AppendLine("foo bar")
	lv.AppendLine("baz")
	lv.AppendLine("foo qux") // evicts "alpha", dropped=1

	// Search for "foo" — should match lines but not indicator
	if err := lv.ApplySearch("foo", msgs.SearchModeSearch); err != nil {
		t.Fatalf("ApplySearch: %v", err)
	}
	if len(lv.matchPositions) != 2 {
		t.Fatalf("expected 2 matches for 'foo', got %d", len(lv.matchPositions))
	}
	// With indicator at display line 0, real lines start at 1.
	// "foo bar" is the first real line (display line 1),
	// "foo qux" is the third real line (display line 3).
	if lv.matchPositions[0].line != 1 {
		t.Errorf("expected first match on display line 1, got %d", lv.matchPositions[0].line)
	}
	if lv.matchPositions[1].line != 3 {
		t.Errorf("expected second match on display line 3, got %d", lv.matchPositions[1].line)
	}
}

func TestLogView_Unavailable(t *testing.T) {
	lv := NewLogView(80, 24, 100, "15m", 900)

	// Initially available
	if lv.IsUnavailable() {
		t.Fatal("expected available by default")
	}
	title := lv.buildTitle()
	if title != "Logs 15m [A]" {
		t.Fatalf("expected default title 'Logs 15m [A]', got %q", title)
	}

	// Set unavailable
	lv.SetUnavailable(true)
	if !lv.IsUnavailable() {
		t.Fatal("expected unavailable after SetUnavailable(true)")
	}
	title = lv.buildTitle()
	if title != "Logs unavailable" {
		t.Fatalf("expected 'Logs unavailable', got %q", title)
	}

	// Set available again
	lv.SetUnavailable(false)
	if lv.IsUnavailable() {
		t.Fatal("expected available after SetUnavailable(false)")
	}
	title = lv.buildTitle()
	if title != "Logs 15m [A]" {
		t.Fatalf("expected default title after clearing unavailable, got %q", title)
	}
}

func TestLogView_ClearAndRestartResetsFilterAndSearch(t *testing.T) {
	lv := NewLogView(80, 24, 100, "15m", 900)
	lv.AppendLine("error: something failed")
	lv.AppendLine("info: all good")

	// Activate a filter
	if err := lv.ApplySearch("error", msgs.SearchModeFilter); err != nil {
		t.Fatalf("ApplySearch filter: %v", err)
	}
	if !lv.FilterActive() {
		t.Fatal("filter should be active before ClearAndRestart")
	}

	// Also activate a search
	if err := lv.ApplySearch("fail", msgs.SearchModeSearch); err != nil {
		t.Fatalf("ApplySearch search: %v", err)
	}
	if !lv.SearchActive() {
		t.Fatal("search should be active before ClearAndRestart")
	}

	// ClearAndRestart should reset both
	lv.ClearAndRestart()

	if lv.FilterActive() {
		t.Fatal("filter should be cleared after ClearAndRestart")
	}
	if lv.SearchActive() {
		t.Fatal("search should be cleared after ClearAndRestart")
	}
	if lv.buffer.Len() != 0 {
		t.Fatalf("buffer should be empty after ClearAndRestart, got %d", lv.buffer.Len())
	}
}

func TestLogView_BuiltinLogLevelHighlighting(t *testing.T) {
	lv := NewLogView(80, 24, 100, "15m", 900) // no user rules

	// ERROR should be highlighted
	line := "2024-01-01 ERROR something failed"
	result := lv.pipeline.Highlight(line)
	if result == line {
		t.Fatal("expected ERROR to be highlighted")
	}
	if !strings.Contains(result, "ERROR") {
		t.Fatal("ERROR text should still be present")
	}
}

func TestLogView_BuiltinHighlightAllLevels(t *testing.T) {
	lv := NewLogView(80, 24, 100, "15m", 900)
	levels := []string{
		"ERROR", "FATAL", "WARN", "WARNING", "INFO", "DEBUG", "TRACE",
		"error", "fatal", "warn", "warning", "info", "debug", "trace",
		"Error", "Fatal", "Warn", "Warning", "Info", "Debug", "Trace",
		"ERR", "err", "Err",
		"DBG", "dbg", "Dbg",
	}
	for _, level := range levels {
		line := "prefix " + level + " suffix"
		result := lv.pipeline.Highlight(line)
		if result == line {
			t.Fatalf("expected %s to be highlighted", level)
		}
	}
}

func TestLogView_BuiltinHighlightWordBoundary(t *testing.T) {
	lv := NewLogView(80, 24, 100, "15m", 900)
	// "INFORMATIONAL" should NOT match "INFO" as a word
	line := "INFORMATIONAL message"
	result := lv.pipeline.Highlight(line)
	if result != line {
		t.Fatal("INFORMATIONAL should not trigger INFO highlight (word boundary)")
	}
	// "informational" should also NOT match
	line = "informational message"
	result = lv.pipeline.Highlight(line)
	if result != line {
		t.Fatal("informational should not trigger info highlight (word boundary)")
	}
	// "errors" should NOT match "error"
	line = "errors happened"
	result = lv.pipeline.Highlight(line)
	if result != line {
		t.Fatal("errors should not trigger error highlight (word boundary)")
	}
}

func TestLogView_BuiltinTimestampHighlighting(t *testing.T) {
	lv := NewLogView(80, 24, 100, "15m", 900)

	// ISO 8601 timestamp
	line := "2024-03-22T14:30:00.123Z INFO starting server"
	result := lv.pipeline.Highlight(line)
	if result == line {
		t.Fatal("expected timestamp to be highlighted")
	}
	// Both date and time parts should be present
	if !strings.Contains(result, "2024-03-22") {
		t.Fatal("date part should be present")
	}
	if !strings.Contains(result, "14:30:00.123") {
		t.Fatal("time part should be present")
	}
}

func TestLogView_BuiltinTimestampVariants(t *testing.T) {
	lv := NewLogView(80, 24, 100, "15m", 900)

	variants := []string{
		"2024-03-22T14:30:00Z message",       // Z timezone
		"2024-03-22T14:30:00+05:30 message",  // offset timezone
		"2024-03-22 14:30:00.123456 message", // space separator, microseconds
		"2024-03-22T14:30:00 message",        // no fractional, no tz
	}
	for _, line := range variants {
		result := lv.pipeline.Highlight(line)
		if result == line {
			t.Fatalf("expected timestamp to be highlighted in: %s", line)
		}
	}
}

func TestLogView_BuiltinIPHighlighting(t *testing.T) {
	lv := NewLogView(80, 24, 100, "15m", 900)

	line := "connection from 192.168.1.100:8080 accepted"
	result := lv.pipeline.Highlight(line)
	if result == line {
		t.Fatal("expected IP address to be highlighted")
	}
	if !strings.Contains(result, "192.168.1.100:8080") {
		t.Fatal("IP:port should be present in output")
	}
}

func TestLogView_BuiltinIPWithoutPort(t *testing.T) {
	lv := NewLogView(80, 24, 100, "15m", 900)

	line := "resolved to 10.0.0.1"
	result := lv.pipeline.Highlight(line)
	if result == line {
		t.Fatal("expected IP without port to be highlighted")
	}
}

func TestLogView_BuiltinJSONReformat(t *testing.T) {
	lv := NewLogView(80, 24, 100, "15m", 900)

	line := `prefix {"message":"hello","count":42} suffix`
	result := lv.pipeline.Highlight(line)

	// Should be reformatted with spaces
	if !strings.Contains(ansi.Strip(result), `"message": "hello"`) {
		t.Fatalf("expected JSON to be reformatted with spaces, got: %s", ansi.Strip(result))
	}
}

func TestLogView_BuiltinJSONColoring(t *testing.T) {
	lv := NewLogView(80, 24, 100, "15m", 900)

	line := `{"level":"info"}`
	result := lv.pipeline.Highlight(line)

	// Should have ANSI codes (colorized)
	if result == line {
		t.Fatal("expected JSON to be colorized")
	}
	// Plain text content should still be readable
	stripped := ansi.Strip(result)
	if !strings.Contains(stripped, "level") {
		t.Fatal("JSON key should be present in stripped output")
	}
}

func TestLogView_BuiltinJSONNestedObject(t *testing.T) {
	lv := NewLogView(80, 24, 100, "15m", 900)

	line := `log {"a":{"b":1}} end`
	result := lv.pipeline.Highlight(line)
	stripped := ansi.Strip(result)
	if !strings.Contains(stripped, `"a"`) {
		t.Fatal("nested JSON should be reformatted")
	}
	// Verify colon separator between key and nested object value
	if !strings.Contains(stripped, `"a": { "b": 1 }`) {
		t.Fatalf("expected nested object with colon separator, got: %s", stripped)
	}
}

func TestLogView_BuiltinJSONNestedArrayInObject(t *testing.T) {
	lv := NewLogView(80, 24, 100, "15m", 900)

	line := `{"items":[1,2]}`
	result := lv.pipeline.Highlight(line)
	stripped := ansi.Strip(result)
	if !strings.Contains(stripped, `"items": [1, 2]`) {
		t.Fatalf("expected colon before nested array, got: %s", stripped)
	}
}

func TestLogView_BuiltinJSONArray(t *testing.T) {
	lv := NewLogView(80, 24, 100, "15m", 900)

	line := `items: [1,2,3]`
	result := lv.pipeline.Highlight(line)
	stripped := ansi.Strip(result)
	if !strings.Contains(stripped, "[1, 2, 3]") {
		t.Fatalf("expected JSON array to be reformatted, got: %s", stripped)
	}
}

func TestLogView_BuiltinJSONInvalidIgnored(t *testing.T) {
	lv := NewLogView(80, 24, 100, "15m", 900)

	// Looks like JSON start but isn't valid
	line := `not json {invalid`
	result := lv.pipeline.Highlight(line)
	// Should not crash; invalid JSON is left as-is (though log level/timestamp/IP highlighting may still apply)
	if !strings.Contains(ansi.Strip(result), "{invalid") {
		t.Fatal("invalid JSON should be left unchanged")
	}
}

func TestLogView_BuiltinJSONPreservesPrefix(t *testing.T) {
	lv := NewLogView(80, 24, 100, "15m", 900)

	line := `2024-01-01T00:00:00Z INFO {"msg":"hello"}`
	result := lv.pipeline.Highlight(line)
	stripped := ansi.Strip(result)
	// Prefix text should still be there
	if !strings.Contains(stripped, "INFO") {
		t.Fatal("prefix before JSON should be preserved")
	}
}

func TestLogView_ToggleSyntaxHighlighting(t *testing.T) {
	lv := NewLogView(80, 24, 100, "15m", 900)

	// Built-ins should be enabled by default
	if !lv.SyntaxEnabled() {
		t.Fatal("syntax highlighting should be enabled by default")
	}

	// With builtins enabled, ERROR should be highlighted
	line := "ERROR something failed"
	result := lv.pipeline.Highlight(line)
	if result == line {
		t.Fatal("ERROR should be highlighted when syntax is enabled")
	}

	// Toggle off
	lv.ToggleSyntax()
	if lv.SyntaxEnabled() {
		t.Fatal("syntax highlighting should be disabled after toggle")
	}

	// With builtins disabled, ERROR should NOT be highlighted (no user rules)
	result = lv.pipeline.Highlight(line)
	if result != line {
		t.Fatal("ERROR should not be highlighted when syntax is disabled")
	}

	// Toggle back on
	lv.ToggleSyntax()
	if !lv.SyntaxEnabled() {
		t.Fatal("syntax highlighting should be re-enabled after second toggle")
	}
}

func TestLogView_ToggleSyntaxRecomputesSearchPositions(t *testing.T) {
	lv := NewLogView(80, 24, 100, "15m", 900)
	lv.AppendLine("2024-01-01T00:00:00Z ERROR something failed")

	// Search while syntax is ON
	if err := lv.ApplySearch("failed", msgs.SearchModeSearch); err != nil {
		t.Fatalf("ApplySearch: %v", err)
	}
	if len(lv.matchPositions) != 1 {
		t.Fatalf("expected 1 match, got %d", len(lv.matchPositions))
	}
	posBefore := lv.matchPositions[0]

	// Toggle syntax OFF — match positions should be recomputed but stable
	// (highlighting only adds ANSI codes, not text, so stripped positions are the same)
	lv.ToggleSyntax()
	if len(lv.matchPositions) != 1 {
		t.Fatalf("expected 1 match after toggle, got %d", len(lv.matchPositions))
	}
	posAfter := lv.matchPositions[0]

	if posBefore.colStart != posAfter.colStart || posBefore.colEnd != posAfter.colEnd {
		t.Fatalf("expected stable match positions after toggle: before=(%d,%d) after=(%d,%d)",
			posBefore.colStart, posBefore.colEnd, posAfter.colStart, posAfter.colEnd)
	}
}

func TestLogView_TimeRangeInTitle(t *testing.T) {
	lv := NewLogView(80, 24, 100, "15m", 900)

	// Default title should show "15m" time range, no [S]
	title := lv.buildTitle()
	if strings.Contains(title, "[S]") {
		t.Fatalf("expected no [S] in title, got: %s", title)
	}
	if !strings.Contains(title, "15m") {
		t.Fatalf("expected '15m' in title, got: %s", title)
	}

	// Set a different time range
	lv.SetTimeRangeLabel("5m")
	title = lv.buildTitle()
	if !strings.Contains(title, "5m") {
		t.Fatalf("expected '5m' in title, got: %s", title)
	}

	// ClearAndRestart should reset to default "15m"
	lv.ClearAndRestart()
	title = lv.buildTitle()
	if !strings.Contains(title, "15m") {
		t.Fatalf("expected '15m' after ClearAndRestart, got: %s", title)
	}
}

func TestLogView_BuiltinJSONWithLogLevel(t *testing.T) {
	lv := NewLogView(80, 24, 100, "15m", 900)

	// JSON containing "ERROR" as a value — ERROR inside JSON should be colored
	// by JSON coloring, not double-colored by log level regex
	line := `2024-01-01T00:00:00Z {"level":"ERROR","msg":"failed"}`
	result := lv.pipeline.Highlight(line)

	// Should not crash; timestamp and JSON should both be handled
	stripped := ansi.Strip(result)
	if !strings.Contains(stripped, "ERROR") {
		t.Fatal("ERROR should be present in output")
	}
	if !strings.Contains(stripped, "2024-01-01") {
		t.Fatal("timestamp should be present in output")
	}
}

func TestLogView_BuiltinJSONNoDuplication(t *testing.T) {
	lv := NewLogView(80, 24, 100, "15m", 900)

	// This line triggered text duplication: the timestamp and level were
	// doubled when regex highlighting ran on already-JSON-colorized text.
	line := `{"time":"2026-03-27T12:15:35.707422909Z","level":"info","message":"HTTP API","module":"http"}`
	result := lv.pipeline.Highlight(line)
	stripped := ansi.Strip(result)

	// Each value must appear exactly once
	for _, needle := range []string{"2026-03-27", "12:15:35", "info", "HTTP API", "http"} {
		count := strings.Count(stripped, needle)
		if count != 1 {
			t.Errorf("%q appears %d times (want 1) in: %s", needle, count, stripped)
		}
	}
}

func TestLogView_BuiltinMixedPrefixJSONNoDuplication(t *testing.T) {
	lv := NewLogView(80, 24, 100, "15m", 900)

	line := `2024-01-01T00:00:00Z INFO {"level":"error","msg":"failed","ip":"10.0.0.1"}`
	result := lv.pipeline.Highlight(line)
	stripped := ansi.Strip(result)

	// Prefix parts should be present
	if !strings.Contains(stripped, "2024-01-01") {
		t.Fatal("timestamp in prefix should be present")
	}
	if !strings.Contains(stripped, "INFO") {
		t.Fatal("log level in prefix should be present")
	}
	// JSON parts should not be duplicated
	if strings.Count(stripped, "error") != 1 {
		t.Errorf("'error' duplicated in: %s", stripped)
	}
	if strings.Count(stripped, "10.0.0.1") != 1 {
		t.Errorf("IP duplicated in: %s", stripped)
	}
}

func TestLogView_BuiltinHighlightsWithSearch(t *testing.T) {
	lv := NewLogView(80, 24, 100, "15m", 900)
	lv.AppendLine("2024-01-01T00:00:00Z ERROR something failed")
	lv.AppendLine("2024-01-01T00:00:01Z INFO all good")

	if err := lv.ApplySearch("failed", msgs.SearchModeSearch); err != nil {
		t.Fatalf("ApplySearch: %v", err)
	}
	// Should find the match despite built-in highlights
	if len(lv.matchPositions) != 1 {
		t.Fatalf("expected 1 match, got %d", len(lv.matchPositions))
	}
}

func TestLogView_BuiltinHighlightsWithFilter(t *testing.T) {
	lv := NewLogView(80, 24, 100, "15m", 900)
	lv.AppendLine("ERROR: failure one")
	lv.AppendLine("INFO: all good")
	lv.AppendLine("ERROR: failure two")

	if err := lv.ApplySearch("ERROR", msgs.SearchModeFilter); err != nil {
		t.Fatalf("ApplySearch: %v", err)
	}
	// Filter should work correctly with built-in highlights
	if !lv.FilterActive() {
		t.Fatal("filter should be active")
	}
}

func TestLogView_BuiltinEmptyLine(t *testing.T) {
	lv := NewLogView(80, 24, 100, "15m", 900)

	result := lv.pipeline.Highlight("")
	if result != "" {
		t.Fatal("empty line should remain empty")
	}
}

func BenchmarkApplyHighlights_PlainText(b *testing.B) {
	lv := NewLogView(80, 24, 100, "15m", 900)
	line := "2024-03-22T14:30:00.123Z INFO processing request from 192.168.1.100:8080"
	b.ResetTimer()
	for range b.N {
		_ = lv.pipeline.Highlight(line)
	}
}

func BenchmarkApplyHighlights_JSON(b *testing.B) {
	lv := NewLogView(80, 24, 100, "15m", 900)
	line := `2024-03-22T14:30:00Z INFO {"level":"info","msg":"request processed","duration":0.042,"status":200,"ip":"10.0.0.1"}`
	b.ResetTimer()
	for range b.N {
		_ = lv.pipeline.Highlight(line)
	}
}

func BenchmarkAppendLine_WithHighlights(b *testing.B) {
	lines := []string{
		"2024-03-22T14:30:00.123Z INFO processing request",
		`{"level":"error","msg":"connection failed","host":"10.0.0.5:3306"}`,
		"2024-03-22T14:30:01Z DEBUG cache hit ratio: 0.95",
		"plain text log line with no special patterns",
		"2024-03-22T14:30:02.456Z WARN high latency from 172.16.0.1:443",
	}
	b.ResetTimer()
	for range b.N {
		lv := NewLogView(80, 24, 1000, "15m", 900)
		for i := range 1000 {
			lv.AppendLine(lines[i%len(lines)])
		}
	}
}

func BenchmarkAppendLine_WithoutHighlights(b *testing.B) {
	lines := []string{
		"2024-03-22T14:30:00.123Z INFO processing request",
		`{"level":"error","msg":"connection failed","host":"10.0.0.5:3306"}`,
		"2024-03-22T14:30:01Z DEBUG cache hit ratio: 0.95",
		"plain text log line with no special patterns",
		"2024-03-22T14:30:02.456Z WARN high latency from 172.16.0.1:443",
	}
	b.ResetTimer()
	for range b.N {
		lv := NewLogView(80, 24, 1000, "15m", 900)
		lv.ToggleSyntax() // disable highlighting
		for i := range 1000 {
			lv.AppendLine(lines[i%len(lines)])
		}
	}
}

func TestLogView_WrapTotalWrappedRows100Lines(t *testing.T) {
	// Viewport is 40 chars wide (42 - 2 for border), 10 rows tall (12 - 2)
	lv := NewLogView(42, 12, 200, "15m", 900)
	lv.softWrap = true
	vpWidth := lv.logVP.width // 40

	// Append 100 lines: mix of short and long lines
	expectedTotal := 0
	for i := range 100 {
		var line string
		if i%3 == 0 {
			// Long line that wraps: 100 chars ~ 3 rows at width 40
			line = strings.Repeat("x", 100)
		} else {
			// Short line: 1 row
			line = fmt.Sprintf("short line %d", i)
		}
		lv.AppendLine(line)

		// Compute expected wrapped height for this line
		w := ansi.StringWidth(lv.buffer.ColoredGet(lv.buffer.Len() - 1))
		expectedTotal += wrapHeight(w, vpWidth)
	}

	if lv.totalWrappedRows != expectedTotal {
		t.Fatalf("totalWrappedRows: got %d, want %d", lv.totalWrappedRows, expectedTotal)
	}

	// Cross-check with a full recompute
	lv.recomputeTotalWrappedRows()
	if lv.totalWrappedRows != expectedTotal {
		t.Fatalf("after recompute, totalWrappedRows: got %d, want %d", lv.totalWrappedRows, expectedTotal)
	}
}

func TestLogView_WrapTotalWrappedRowsAfterEviction(t *testing.T) {
	// Small buffer (capacity 10) so evictions happen
	lv := NewLogView(42, 12, 10, "15m", 900)
	lv.softWrap = true
	vpWidth := lv.logVP.width // 40

	// Append 50 lines to force evictions (buffer wraps around multiple times)
	for i := range 50 {
		var line string
		if i%4 == 0 {
			line = strings.Repeat("y", 80) // wraps to 2 rows at width 40
		} else {
			line = fmt.Sprintf("line %d", i)
		}
		lv.AppendLine(line)
	}

	// After 50 appends with capacity 10, 40 lines should have been evicted
	if lv.buffer.Dropped() != 40 {
		t.Fatalf("expected 40 dropped, got %d", lv.buffer.Dropped())
	}

	// Compute expected totalWrappedRows from the current buffer contents
	expected := 0
	for i := range lv.buffer.Len() {
		w := lv.buffer.WidthGet(i)
		expected += wrapHeight(w, vpWidth)
	}
	// Plus indicator row for dropped lines
	expected++

	if lv.totalWrappedRows != expected {
		t.Fatalf("totalWrappedRows: got %d, want %d", lv.totalWrappedRows, expected)
	}

	// Cross-check with a full recompute
	lv.recomputeTotalWrappedRows()
	if lv.totalWrappedRows != expected {
		t.Fatalf("after recompute, totalWrappedRows: got %d, want %d", lv.totalWrappedRows, expected)
	}
}

func TestLogView_UpdateViewportWindowSize(t *testing.T) {
	lv := NewLogView(80, 24, 100, "15m", 900)
	for i := range 50 {
		lv.AppendLine(fmt.Sprintf("line %d", i))
	}
	view := ansi.Strip(lv.View())
	if !strings.Contains(view, "line 49") {
		t.Fatal("expected last line visible with autoscroll")
	}
}

func BenchmarkAppendLine_Windowed(b *testing.B) {
	lines := []string{
		"2024-03-22T14:30:00.123Z INFO processing request",
		`{"level":"error","msg":"connection failed","host":"10.0.0.5:3306"}`,
		"2024-03-22T14:30:01Z DEBUG cache hit ratio: 0.95",
		"plain text log line with no special patterns",
		"2024-03-22T14:30:02.456Z WARN high latency from 172.16.0.1:443",
	}
	b.ResetTimer()
	for range b.N {
		lv := NewLogView(80, 24, 10000, "15m", 900)
		for i := range 10000 {
			lv.AppendLine(lines[i%len(lines)])
		}
	}
}

func TestLogView_ScrollOffset_Bounds(t *testing.T) {
	lv := NewLogView(80, 24, 100, "15m", 900)
	for i := range 50 {
		lv.AppendLine(fmt.Sprintf("line %d", i))
	}
	lv.scrollOffset = -5
	lv.clampScrollOffset()
	if lv.scrollOffset != 0 {
		t.Fatalf("expected scrollOffset 0, got %d", lv.scrollOffset)
	}
	lv.scrollOffset = 1000
	lv.clampScrollOffset()
	h := lv.viewportHeight()
	maxOff := 50 - h
	if lv.scrollOffset != maxOff {
		t.Fatalf("expected scrollOffset %d, got %d", maxOff, lv.scrollOffset)
	}
}

func TestLogView_InsertMarker(t *testing.T) {
	lv := NewLogView(80, 24, 100, "15m", 900)
	lv.AppendLine("line one")
	lv.InsertMarker()
	lv.AppendLine("line two")

	if lv.buffer.Len() != 3 {
		t.Fatalf("expected 3 lines, got %d", lv.buffer.Len())
	}
	raw := lv.buffer.RawGet(1)
	if !strings.Contains(raw, "---") {
		t.Fatalf("expected marker to contain '---', got: %s", raw)
	}
	// Should contain a time pattern HH:MM:SS
	if !regexp.MustCompile(`\d{2}:\d{2}:\d{2}`).MatchString(raw) {
		t.Fatalf("expected marker to contain timestamp, got: %s", raw)
	}
}

func TestLogView_WrappedModeAllLinesReachViewport(t *testing.T) {
	// Viewport is 40 chars wide, 10 rows tall
	lv := NewLogView(42, 12, 100, "15m", 900) // +2 for border
	lv.softWrap = true

	// Add 5 short lines and 3 long lines that will wrap
	for i := range 5 {
		lv.AppendLine(fmt.Sprintf("short line %d", i))
	}
	for range 3 {
		lv.AppendLine(strings.Repeat("long ", 20)) // 100 chars, wraps to ~3 rows each
	}

	// totalWrappedRows should account for all 8 logical lines (5 short + 3 wrapped)
	if lv.totalWrappedRows < 8 {
		t.Fatalf("expected at least 8 total wrapped rows, got %d", lv.totalWrappedRows)
	}
	// Should be scrolled to bottom (autoscroll)
	vpHeight := lv.viewportHeight()
	maxOff := max(lv.totalWrappedRows-vpHeight, 0)
	if lv.wrapYOffset != maxOff {
		t.Fatalf("expected wrapYOffset at bottom (%d), got %d", maxOff, lv.wrapYOffset)
	}
}

func TestLogView_NonWrappedModeUnchanged(t *testing.T) {
	lv := NewLogView(80, 12, 100, "15m", 900)
	// SoftWrap is off by default

	for i := range 20 {
		lv.AppendLine(fmt.Sprintf("line %d", i))
	}

	// In non-wrapped mode, logVP should receive only viewportHeight lines
	// (the O(H) window), not all 20
	vpLines := len(lv.logVP.lines)
	h := lv.viewportHeight()
	if vpLines > h+1 { // +1 for possible indicator
		t.Fatalf("expected at most %d lines in viewport (non-wrapped), got %d", h+1, vpLines)
	}
}

func TestLogView_InsertMarkerNonMatchingFilter(t *testing.T) {
	lv := NewLogView(80, 24, 100, "15m", 900)
	lv.AppendLine("ERROR: something broke")
	lv.AppendLine("INFO: all good")

	// Apply a filter for ERROR lines only
	if err := lv.ApplySearch("ERROR", msgs.SearchModeFilter); err != nil {
		t.Fatalf("ApplySearch: %v", err)
	}
	// Only 1 line should match the filter
	if len(lv.filteredIndices) != 1 {
		t.Fatalf("expected 1 filtered line, got %d", len(lv.filteredIndices))
	}

	// Insert a marker — its text "--- HH:MM:SS ---" does NOT match "ERROR",
	// but markers are ALWAYS included in the filtered view for visibility.
	lv.InsertMarker()
	if len(lv.filteredIndices) != 2 {
		t.Fatalf("expected 2 filtered lines (1 ERROR match + 1 marker always visible), got %d", len(lv.filteredIndices))
	}
}

func TestLogView_InsertMarkerMatchingFilter(t *testing.T) {
	lv := NewLogView(80, 24, 100, "15m", 900)
	lv.AppendLine("line with ---")
	lv.AppendLine("other line")

	// Apply a filter that matches the marker text pattern "---"
	if err := lv.ApplySearch("---", msgs.SearchModeFilter); err != nil {
		t.Fatalf("ApplySearch: %v", err)
	}
	// 1 line should match the filter ("line with ---")
	if len(lv.filteredIndices) != 1 {
		t.Fatalf("expected 1 filtered line before marker, got %d", len(lv.filteredIndices))
	}

	// Insert a marker — markers are ALWAYS included in the filtered view
	// regardless of whether their text matches the filter pattern.
	lv.InsertMarker()
	if len(lv.filteredIndices) != 2 {
		t.Fatalf("expected 2 filtered lines (1 match + 1 marker always visible), got %d", len(lv.filteredIndices))
	}
}

// TestLogView_InsertMarkerAtCapacityWithFilterAndSoftWrap verifies that
// InsertMarker correctly handles eviction when the ring buffer is full.
// totalWrappedRows must stay consistent, and filteredIndices must not retain
// stale absolute indices that would translate to negative buffer positions.
func TestLogView_InsertMarkerAtCapacityWithFilterAndSoftWrap(t *testing.T) {
	// Small capacity + soft wrap + filter all active.
	lv := NewLogView(80, 24, 3, "15m", 900)
	lv.softWrap = true
	if err := lv.ApplySearch("match", msgs.SearchModeFilter); err != nil {
		t.Fatalf("ApplySearch: %v", err)
	}

	// Fill to capacity with matching lines so filteredIndices has 3 entries.
	lv.AppendLine("match one")
	lv.AppendLine("match two")
	lv.AppendLine("match three")
	// Buffer now at capacity.
	if lv.buffer.Len() != lv.buffer.Cap() {
		t.Fatalf("expected buffer full, got len=%d cap=%d", lv.buffer.Len(), lv.buffer.Cap())
	}
	if len(lv.filteredIndices) != 3 {
		t.Fatalf("expected 3 filtered indices, got %d", len(lv.filteredIndices))
	}

	// Insert a marker — this should evict "match one".
	lv.InsertMarker()

	// Buffer still at capacity; one line dropped.
	if lv.buffer.Len() != lv.buffer.Cap() {
		t.Fatalf("expected buffer full after InsertMarker, got len=%d", lv.buffer.Len())
	}
	if lv.buffer.Dropped() != 1 {
		t.Fatalf("expected 1 dropped line, got %d", lv.buffer.Dropped())
	}

	// Every entry in filteredIndices must reference a still-live line:
	// i.e. filteredIndices[i] - filteredIndexOffset must be in [0, Len()).
	for i, abs := range lv.filteredIndices {
		local := abs - lv.filteredIndexOffset
		if local < 0 || local >= lv.buffer.Len() {
			t.Fatalf("filteredIndices[%d]=%d out of buffer range [0,%d) after offset=%d",
				i, abs, lv.buffer.Len(), lv.filteredIndexOffset)
		}
	}

	// totalWrappedRows should equal the sum of wrapped heights of filtered
	// entries plus the indicator row (now present because Dropped>0).
	vpWidth := lv.logVP.width
	expected := 0
	for _, abs := range lv.filteredIndices {
		local := abs - lv.filteredIndexOffset
		expected += wrapHeight(lv.buffer.WidthGet(local), vpWidth)
	}
	expected++ // indicator row
	if lv.totalWrappedRows != expected {
		t.Fatalf("totalWrappedRows=%d, expected %d", lv.totalWrappedRows, expected)
	}
}

// TestLogView_InsertMarkerAtCapacityNoFilterSoftWrap confirms that
// totalWrappedRows stays consistent when InsertMarker evicts in soft-wrap
// mode without a filter (no filteredIndices mutations needed).
func TestLogView_InsertMarkerAtCapacityNoFilterSoftWrap(t *testing.T) {
	lv := NewLogView(80, 24, 3, "15m", 900)
	lv.softWrap = true

	// Fill the buffer with lines of known width.
	lv.AppendLine("alpha")
	lv.AppendLine("beta")
	lv.AppendLine("gamma")
	rowsBefore := lv.totalWrappedRows

	// InsertMarker should evict "alpha" — totalWrappedRows bookkeeping must
	// subtract alpha's wrapped height, add the marker's wrapped height, and
	// add 1 for the indicator row that now appears.
	lv.InsertMarker()

	if lv.buffer.Dropped() != 1 {
		t.Fatalf("expected 1 dropped line, got %d", lv.buffer.Dropped())
	}

	// Recompute totalWrappedRows from scratch and compare.
	vpWidth := lv.logVP.width
	expected := 0
	for i := range lv.buffer.Len() {
		expected += wrapHeight(lv.buffer.WidthGet(i), vpWidth)
	}
	expected++ // indicator row
	if lv.totalWrappedRows != expected {
		t.Fatalf("totalWrappedRows=%d, expected %d (rowsBefore=%d)",
			lv.totalWrappedRows, expected, rowsBefore)
	}
}

func TestLogView_ApplySearchPreservesExistingMarkers(t *testing.T) {
	lv := NewLogView(80, 24, 100, "15m", 900)
	lv.AppendLine("ERROR: something broke")
	lv.AppendLine("INFO: all good")

	// Insert a marker before applying the filter
	lv.InsertMarker()

	// Apply a filter for ERROR — the marker text does NOT match "ERROR",
	// but markers are ALWAYS included in the filtered view.
	if err := lv.ApplySearch("ERROR", msgs.SearchModeFilter); err != nil {
		t.Fatalf("ApplySearch: %v", err)
	}
	// Expect 2 entries: 1 ERROR line + 1 marker (always visible)
	if len(lv.filteredIndices) != 2 {
		t.Fatalf("expected 2 filtered entries (1 ERROR match + 1 marker always visible), got %d", len(lv.filteredIndices))
	}
}

func TestLogView_MouseWheelScroll(t *testing.T) {
	lv := NewLogView(80, 24, 100, "15m", 900)
	for i := range 50 {
		lv.AppendLine(fmt.Sprintf("line %d", i))
	}
	// Should be at bottom with autoscroll
	initialOffset := lv.scrollOffset

	// Mouse wheel up should scroll up
	lv, _ = lv.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	if lv.scrollOffset >= initialOffset {
		t.Fatal("expected scrollOffset to decrease after mouse wheel up")
	}
	if lv.autoscroll {
		t.Fatal("expected autoscroll off after mouse wheel up")
	}

	// Mouse wheel down should scroll down
	prevOffset := lv.scrollOffset
	lv, _ = lv.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if lv.scrollOffset <= prevOffset {
		t.Fatal("expected scrollOffset to increase after mouse wheel down")
	}
}

func TestLogView_WrappedScrollDown(t *testing.T) {
	lv := NewLogView(42, 12, 100, "15m", 900)
	lv.softWrap = true

	// Add enough lines to need scrolling
	for i := range 30 {
		lv.AppendLine(fmt.Sprintf("line %d", i))
	}

	// Autoscroll should have us at bottom
	vpHeight := lv.viewportHeight()
	maxOff := max(lv.totalWrappedRows-vpHeight, 0)
	atBottom := lv.wrapYOffset >= maxOff
	if !atBottom {
		t.Fatal("expected at bottom with autoscroll")
	}

	// Scroll up, then check we're no longer at bottom
	lv.ScrollUp()
	atBottom = lv.wrapYOffset >= lv.totalWrappedRows-vpHeight
	if atBottom {
		t.Fatal("expected not at bottom after ScrollUp")
	}
	if lv.autoscroll {
		t.Fatal("expected autoscroll off after ScrollUp")
	}

	// GotoBottom should restore autoscroll
	lv.GotoBottom()
	maxOff = max(lv.totalWrappedRows-vpHeight, 0)
	atBottom = lv.wrapYOffset >= maxOff
	if !atBottom {
		t.Fatal("expected at bottom after GotoBottom")
	}
	if !lv.autoscroll {
		t.Fatal("expected autoscroll on after GotoBottom")
	}
}

func TestLogView_ToggleSyntaxOffWithBufferedLines(t *testing.T) {
	lv := NewLogView(80, 24, 100, "15m", 900)
	// Append lines that will be syntax-highlighted (e.g., log level coloring)
	lv.AppendLine("ERROR something failed")
	lv.AppendLine("INFO all good")
	lv.AppendLine("plain text line")

	// Verify lines are highlighted (colored != raw)
	for i := range lv.buffer.Len() {
		raw := lv.buffer.RawGet(i)
		colored := lv.buffer.ColoredGet(i)
		if raw == colored && (strings.Contains(raw, "ERROR") || strings.Contains(raw, "INFO")) {
			t.Fatalf("expected line %d to be highlighted before toggle, raw=%q colored=%q", i, raw, colored)
		}
	}

	// Toggle syntax off — should not panic and lines should revert to raw content
	lv.ToggleSyntax()

	if lv.SyntaxEnabled() {
		t.Fatal("syntax highlighting should be disabled after toggle")
	}
	for i := range lv.buffer.Len() {
		raw := lv.buffer.RawGet(i)
		colored := lv.buffer.ColoredGet(i)
		if raw != colored {
			t.Fatalf("expected line %d colored to equal raw after toggle off, raw=%q colored=%q", i, raw, colored)
		}
	}
}

func TestLogView_WrapModeSearchHighlightsOnlyVisibleLines(t *testing.T) {
	// Viewport: 42 wide (40 inner), 7 tall (5 inner rows).
	// This ensures only a small window of lines is visible at a time.
	lv := NewLogView(42, 7, 100, "15m", 900)
	lv.softWrap = true
	lv.autoscroll = false
	lv.wrapYOffset = 0

	// Disable syntax highlighting so colored lines == raw lines (no ANSI noise).
	lv.ToggleSyntax()

	// Add 20 lines. Every 3rd line contains "needle".
	for i := range 20 {
		if i%3 == 0 {
			lv.AppendLine(fmt.Sprintf("line %02d needle here", i))
		} else {
			lv.AppendLine(fmt.Sprintf("line %02d other text", i))
		}
	}

	// Activate search for "needle".
	if err := lv.ApplySearch("needle", msgs.SearchModeSearch); err != nil {
		t.Fatalf("ApplySearch: %v", err)
	}

	// There should be 7 matches (lines 0,3,6,9,12,15,18).
	if len(lv.matchPositions) != 7 {
		t.Fatalf("expected 7 matches, got %d", len(lv.matchPositions))
	}

	// Scroll to top so lines 0..4 are visible.
	lv.wrapYOffset = 0
	lv.autoscroll = false
	lv.updateViewport()

	view := lv.logVP.View()
	vpHeight := lv.viewportHeight()

	// The visible window should contain search highlights for "needle"
	// on visible matching lines.
	// Line 0 matches ("line 00 needle here") — should be highlighted.
	if !strings.Contains(view, "line 00") {
		t.Fatal("expected 'line 00' to be visible at top")
	}
	// Line 3 matches ("line 03 needle here") — should be highlighted if visible.
	if vpHeight >= 4 && !strings.Contains(view, "line 03") {
		t.Fatal("expected 'line 03' to be visible in viewport")
	}

	// Lines beyond the viewport (e.g., line 18) should NOT be in the rendered output.
	if strings.Contains(view, "line 18") {
		t.Fatal("line 18 should not be visible when scrolled to top")
	}

	// The logVP should receive at most vpHeight lines (windowed, not all 20).
	if len(lv.logVP.lines) > vpHeight {
		t.Fatalf("expected at most %d lines in logVP, got %d (wrap-mode should be windowed)",
			vpHeight, len(lv.logVP.lines))
	}

	// Now scroll to bottom and verify the last matching line is highlighted.
	lv.GotoBottom()
	view = lv.logVP.View()

	if !strings.Contains(view, "line 19") {
		t.Fatal("expected 'line 19' visible at bottom")
	}
	// Line 18 matches; it should be in the rendered output when scrolled to bottom.
	if !strings.Contains(view, "line 18") {
		t.Fatal("expected 'line 18' visible near bottom")
	}
	// Line 0 should no longer be visible.
	if strings.Contains(view, "line 00") {
		t.Fatal("line 00 should not be visible when scrolled to bottom")
	}

	// Again, logVP should have at most vpHeight lines.
	if len(lv.logVP.lines) > vpHeight {
		t.Fatalf("expected at most %d lines in logVP at bottom, got %d",
			vpHeight, len(lv.logVP.lines))
	}
}

func TestLogView_SearchUsesStrippedColoredLines(t *testing.T) {
	// Verify that search matching uses stripped colored lines (visible text)
	// so that match positions correspond to display columns.
	lv := NewLogView(80, 24, 100, "15m", 900)

	// Append lines that will be syntax-highlighted (contain log levels,
	// timestamps, IPs — all of which produce ANSI codes in colored lines).
	lv.AppendLine("2024-01-01T00:00:00Z ERROR connection refused from 10.0.0.1")
	lv.AppendLine("2024-01-01T00:00:01Z INFO request processed successfully")
	lv.AppendLine("2024-01-01T00:00:02Z WARN high latency detected")

	// Confirm colored lines differ from raw lines (syntax highlighting is active).
	for i := range lv.buffer.Len() {
		raw := lv.buffer.RawGet(i)
		colored := lv.buffer.ColoredGet(i)
		if raw == colored {
			t.Fatalf("line %d: expected colored != raw (syntax highlighting should be active)", i)
		}
	}

	// Search for a term that spans highlighted regions.
	if err := lv.ApplySearch("connection", msgs.SearchModeSearch); err != nil {
		t.Fatalf("ApplySearch: %v", err)
	}
	if len(lv.matchPositions) != 1 {
		t.Fatalf("expected 1 match for 'connection', got %d", len(lv.matchPositions))
	}

	// Verify match position corresponds to the visible (stripped colored) text.
	pos := lv.matchPositions[0]
	visible := ansi.Strip(lv.buffer.ColoredGet(0))
	matched := ansi.Strip(ansi.Cut(visible, pos.colStart, pos.colEnd))
	if matched != "connection" {
		t.Fatalf("expected matched text 'connection', got %q (colStart=%d, colEnd=%d)",
			matched, pos.colStart, pos.colEnd)
	}

	// Search for a term that appears in multiple highlighted lines.
	if err := lv.ApplySearch("2024-01-01", msgs.SearchModeSearch); err != nil {
		t.Fatalf("ApplySearch: %v", err)
	}
	if len(lv.matchPositions) != 3 {
		t.Fatalf("expected 3 matches for '2024-01-01', got %d", len(lv.matchPositions))
	}

	// Verify each match is on a different display line.
	for i, p := range lv.matchPositions {
		if p.line != i {
			t.Errorf("match %d: expected line %d, got %d", i, i, p.line)
		}
	}
}

func TestLogView_SearchHighlightsCorrectlyInJSON(t *testing.T) {
	// Regression test: when log lines contain compact JSON, the JSON highlighter
	// reformats them (adds spaces), making the display text longer than the raw
	// text. Search positions must match display columns, not raw byte offsets.
	lv := NewLogView(120, 24, 100, "15m", 900)

	// Compact JSON line — the JSON highlighter will add spaces.
	lv.AppendLine(`{"level":"debug","code":200,"headers":{"Accept":"application/json"}}`)

	// Confirm the JSON highlighter expanded the visible text.
	raw := lv.buffer.RawGet(0)
	visible := ansi.Strip(lv.buffer.ColoredGet(0))
	if raw == visible {
		t.Fatal("expected JSON highlighter to reformat compact JSON (visible != raw)")
	}

	// Case-insensitive search for "accept".
	if err := lv.ApplySearch("accept", msgs.SearchModeSearch); err != nil {
		t.Fatalf("ApplySearch: %v", err)
	}
	if len(lv.matchPositions) != 1 {
		t.Fatalf("expected 1 match for 'accept', got %d", len(lv.matchPositions))
	}

	// The match position must point to "Accept" in the visible (display) text.
	pos := lv.matchPositions[0]
	matched := ansi.Strip(ansi.Cut(visible, pos.colStart, pos.colEnd))
	if !strings.EqualFold(matched, "accept") {
		t.Fatalf("expected highlight on 'Accept', got %q at cols [%d:%d]\n  visible: %q",
			matched, pos.colStart, pos.colEnd, visible)
	}
}

func TestLogView_SearchWithFilterUsesRawLines(t *testing.T) {
	// Verify that search + filter combo uses raw lines correctly.
	lv := NewLogView(80, 24, 100, "15m", 900)

	lv.AppendLine("ERROR: first failure")
	lv.AppendLine("INFO: all good")
	lv.AppendLine("ERROR: second failure")
	lv.AppendLine("INFO: still good")
	lv.AppendLine("ERROR: third failure")

	// Apply filter for ERROR lines.
	if err := lv.ApplySearch("ERROR", msgs.SearchModeFilter); err != nil {
		t.Fatalf("ApplySearch filter: %v", err)
	}
	if len(lv.filteredIndices) != 3 {
		t.Fatalf("expected 3 filtered lines, got %d", len(lv.filteredIndices))
	}

	// Now search within filtered lines.
	if err := lv.ApplySearch("failure", msgs.SearchModeSearch); err != nil {
		t.Fatalf("ApplySearch search: %v", err)
	}
	// All 3 filtered lines contain "failure".
	if len(lv.matchPositions) != 3 {
		t.Fatalf("expected 3 matches for 'failure' in filtered set, got %d", len(lv.matchPositions))
	}

	// Search for something that only appears in one filtered line.
	if err := lv.ApplySearch("second", msgs.SearchModeSearch); err != nil {
		t.Fatalf("ApplySearch search: %v", err)
	}
	if len(lv.matchPositions) != 1 {
		t.Fatalf("expected 1 match for 'second' in filtered set, got %d", len(lv.matchPositions))
	}
}

func TestLogView_ToggleSyntaxOffThenOn(t *testing.T) {
	lv := NewLogView(80, 24, 100, "15m", 900)
	lv.AppendLine("ERROR something failed")
	lv.AppendLine("INFO all good")

	// Save the original highlighted versions
	origColored := make([]string, lv.buffer.Len())
	for i := range lv.buffer.Len() {
		origColored[i] = lv.buffer.ColoredGet(i)
	}

	// Toggle off
	lv.ToggleSyntax()
	if lv.SyntaxEnabled() {
		t.Fatal("syntax should be disabled after first toggle")
	}
	// Verify lines are raw
	for i := range lv.buffer.Len() {
		if lv.buffer.RawGet(i) != lv.buffer.ColoredGet(i) {
			t.Fatalf("line %d should be raw after toggle off", i)
		}
	}

	// Toggle back on — pipeline should be restored and highlights reapplied
	lv.ToggleSyntax()
	if !lv.SyntaxEnabled() {
		t.Fatal("syntax should be re-enabled after second toggle")
	}
	for i := range lv.buffer.Len() {
		raw := lv.buffer.RawGet(i)
		colored := lv.buffer.ColoredGet(i)
		if raw == colored && (strings.Contains(raw, "ERROR") || strings.Contains(raw, "INFO")) {
			t.Fatalf("expected line %d to be highlighted after toggle on, raw=%q colored=%q", i, raw, colored)
		}
		// The re-highlighted version should match the original
		if colored != origColored[i] {
			t.Fatalf("line %d highlight mismatch after toggle on: got=%q want=%q", i, colored, origColored[i])
		}
	}
}

func TestLogView_FilterEvictionOffsetBased(t *testing.T) {
	// Use a small buffer (capacity 20) so evictions happen quickly,
	// and verify that the offset-based filtered indices produce the
	// correct filtered view after many evictions.
	const cap = 20
	lv := NewLogView(80, 24, cap, "15m", 900)

	// Disable syntax highlighting so we can compare raw content easily.
	lv.ToggleSyntax()

	// Apply a filter for lines containing "match".
	if err := lv.ApplySearch("match", msgs.SearchModeFilter); err != nil {
		t.Fatalf("ApplySearch: %v", err)
	}

	// Append 100 lines: every 3rd line matches the filter.
	// This triggers 80 evictions (100 - 20 = 80).
	for i := range 100 {
		if i%3 == 0 {
			lv.AppendLine(fmt.Sprintf("match line %d", i))
		} else {
			lv.AppendLine(fmt.Sprintf("other line %d", i))
		}
	}

	// After 100 appends with capacity 20, exactly 80 lines were evicted.
	if lv.buffer.Dropped() != 80 {
		t.Fatalf("expected 80 dropped, got %d", lv.buffer.Dropped())
	}

	// The buffer now holds lines 80..99. Among those, matching lines are
	// 81 (81%3==0), 84, 87, 90, 93, 96, 99.
	var expectedMatches []string
	for i := 80; i < 100; i++ {
		if i%3 == 0 {
			expectedMatches = append(expectedMatches, fmt.Sprintf("match line %d", i))
		}
	}

	// Verify filtered count equals expected matches.
	if len(lv.filteredIndices) != len(expectedMatches) {
		t.Fatalf("expected %d filtered indices, got %d", len(expectedMatches), len(lv.filteredIndices))
	}

	// Verify each filtered line is correct by reading from the buffer.
	for i, idx := range lv.filteredIndices {
		bufIdx := idx - lv.filteredIndexOffset
		raw := lv.buffer.RawGet(bufIdx)
		if raw != expectedMatches[i] {
			t.Fatalf("filtered line %d: got %q, want %q (absIdx=%d, offset=%d, bufIdx=%d)",
				i, raw, expectedMatches[i], idx, lv.filteredIndexOffset, bufIdx)
		}
	}

	// Verify the rendered view contains only matching lines (no "other" lines).
	view := ansi.Strip(lv.View())
	for _, expected := range expectedMatches {
		if !strings.Contains(view, expected) {
			t.Fatalf("expected %q in viewport, got:\n%s", expected, view)
		}
	}
	if strings.Contains(view, "other line") {
		t.Fatal("viewport should not contain 'other line' when filter is active")
	}

	// Verify the offset is consistent: offset should equal dropped count.
	if lv.filteredIndexOffset != 80 {
		t.Fatalf("expected filteredIndexOffset 80, got %d", lv.filteredIndexOffset)
	}

	// Now clear the filter and verify offset resets.
	lv.ClearFilter()
	if lv.filteredIndexOffset != 0 {
		t.Fatalf("expected filteredIndexOffset 0 after ClearFilter, got %d", lv.filteredIndexOffset)
	}

	// Re-apply filter and verify it still works correctly.
	if err := lv.ApplySearch("match", msgs.SearchModeFilter); err != nil {
		t.Fatalf("re-ApplySearch: %v", err)
	}
	if len(lv.filteredIndices) != len(expectedMatches) {
		t.Fatalf("after re-apply: expected %d filtered indices, got %d", len(expectedMatches), len(lv.filteredIndices))
	}
}

func TestLogView_ShowHeaderDefaultTrue(t *testing.T) {
	lv := NewLogView(80, 24, 100, "15m", 900)
	if !lv.ShowHeader() {
		t.Fatal("expected ShowHeader to be true by default")
	}
}

func TestLogView_ToggleHeaderFlips(t *testing.T) {
	lv := NewLogView(80, 24, 100, "15m", 900)
	lv.SetBorderless(true)
	lv.SetSize(80, 24)

	if !lv.ShowHeader() {
		t.Fatal("expected ShowHeader true initially")
	}

	lv.ToggleHeader()
	if lv.ShowHeader() {
		t.Fatal("expected ShowHeader false after first toggle")
	}

	lv.ToggleHeader()
	if !lv.ShowHeader() {
		t.Fatal("expected ShowHeader true after second toggle")
	}
}

func TestLogView_HeaderShownInBorderlessMode(t *testing.T) {
	lv := NewLogView(80, 24, 100, "15m", 900)
	lv.SetBorderless(true)
	lv.SetSize(80, 24)
	lv.AppendLine("hello world")

	view := lv.View()
	// The header should contain the title from buildTitle()
	if !strings.Contains(view, "Logs") {
		t.Fatal("expected header with 'Logs' title in borderless mode with showHeader")
	}
}

func TestLogView_HeaderHiddenWhenToggledOff(t *testing.T) {
	lv := NewLogView(80, 24, 100, "15m", 900)
	lv.SetBorderless(true)
	lv.SetSize(80, 24)
	lv.AppendLine("hello world")

	lv.ToggleHeader()
	view := lv.View()

	// In borderless mode with header off, View should just return logVP.View()
	// which should not contain a styled title line
	expected := lv.logVP.View()
	if view != expected {
		t.Fatalf("expected View() to equal logVP.View() when header is off, got diff")
	}
}

func TestLogView_SetSizeViewportHeightWithHeader(t *testing.T) {
	lv := NewLogView(80, 24, 100, "15m", 900)
	lv.SetBorderless(true)
	lv.SetSize(80, 20)

	// With showHeader=true and borderless, logVP height should be h-1
	if lv.logVP.height != 19 {
		t.Fatalf("expected logVP height 19 (20-1 for header), got %d", lv.logVP.height)
	}

	// Toggle header off — logVP height should be full h
	lv.ToggleHeader()
	if lv.logVP.height != 20 {
		t.Fatalf("expected logVP height 20 (no header), got %d", lv.logVP.height)
	}

	// Toggle header back on — logVP height should be h-1 again
	lv.ToggleHeader()
	if lv.logVP.height != 19 {
		t.Fatalf("expected logVP height 19 (header back on), got %d", lv.logVP.height)
	}
}

func TestLogView_ViewportHeightAccountsForHeader(t *testing.T) {
	lv := NewLogView(80, 24, 100, "15m", 900)

	// Non-borderless: viewportHeight = height - 2 (borders)
	if lv.viewportHeight() != 22 {
		t.Fatalf("expected viewportHeight 22 in bordered mode, got %d", lv.viewportHeight())
	}

	lv.SetBorderless(true)
	lv.SetSize(80, 24)

	// Borderless with header: viewportHeight = height - 1
	if lv.viewportHeight() != 23 {
		t.Fatalf("expected viewportHeight 23 in borderless+header mode, got %d", lv.viewportHeight())
	}

	lv.ToggleHeader()
	// Borderless without header: viewportHeight = height
	if lv.viewportHeight() != 24 {
		t.Fatalf("expected viewportHeight 24 in borderless no-header mode, got %d", lv.viewportHeight())
	}
}

func TestLogView_HeaderNotShownInBorderedMode(t *testing.T) {
	lv := NewLogView(80, 24, 100, "15m", 900)
	// Not borderless — header should not appear even though showHeader is true
	lv.AppendLine("hello world")
	view := lv.View()

	// In bordered mode, the title is injected into the border, not as a header line
	// The view should use injectBorderTitle, not JoinVertical with header
	if !strings.Contains(view, "Logs") {
		t.Fatal("expected border title with 'Logs' in bordered mode")
	}
}

func TestLogView_SearchUsesCachedStrippedLines(t *testing.T) {
	// Verify that rebuildViewportContent uses the cached stripped lines
	// (StrippedGet) rather than calling ansi.Strip at search time.
	// The cached stripped value should equal ansi.Strip(colored) and
	// produce correct match positions.
	lv := NewLogView(120, 24, 100, "15m", 900)

	// Append lines with syntax highlighting active (JSON, log levels, timestamps).
	lv.AppendLine(`2024-01-01T00:00:00Z ERROR {"msg":"connection refused","host":"10.0.0.1"}`)
	lv.AppendLine("2024-01-01T00:00:01Z INFO request processed successfully")
	lv.AppendLine(`2024-01-01T00:00:02Z WARN {"latency":500,"unit":"ms"}`)

	// Verify that cached stripped lines equal ansi.Strip(colored).
	for i := range lv.buffer.Len() {
		cached := lv.buffer.StrippedGet(i)
		expected := ansi.Strip(lv.buffer.ColoredGet(i))
		if cached != expected {
			t.Fatalf("line %d: StrippedGet=%q != ansi.Strip(colored)=%q", i, cached, expected)
		}
	}

	// Search and verify match positions are correct using cached stripped lines.
	if err := lv.ApplySearch("connection", msgs.SearchModeSearch); err != nil {
		t.Fatalf("ApplySearch: %v", err)
	}
	if len(lv.matchPositions) != 1 {
		t.Fatalf("expected 1 match, got %d", len(lv.matchPositions))
	}
	pos := lv.matchPositions[0]
	visible := lv.buffer.StrippedGet(0)
	if visible[pos.colStart:pos.colEnd] != "connection" {
		t.Fatalf("expected 'connection' at [%d:%d], got %q", pos.colStart, pos.colEnd, visible[pos.colStart:pos.colEnd])
	}
}

func TestLogView_ToggleSyntaxUpdatesCachedStrippedLines(t *testing.T) {
	// After ToggleSyntax, the stripped cache must be updated so that
	// subsequent searches use the correct (post-toggle) stripped text.
	lv := NewLogView(120, 24, 100, "15m", 900)

	// Append a line with JSON (the JSON highlighter reformats compact JSON).
	lv.AppendLine(`{"level":"error","msg":"connection failed","code":500}`)

	// With syntax ON, stripped text includes reformatted JSON (spaces added).
	strippedOn := lv.buffer.StrippedGet(0)
	if strippedOn == lv.buffer.RawGet(0) {
		t.Fatal("with syntax ON, stripped should differ from raw (JSON reformat adds spaces)")
	}

	// Toggle syntax OFF.
	lv.ToggleSyntax()

	// With syntax OFF, colored == raw, so stripped should == raw.
	strippedOff := lv.buffer.StrippedGet(0)
	raw := lv.buffer.RawGet(0)
	if strippedOff != raw {
		t.Fatalf("with syntax OFF, StrippedGet should equal raw.\n  stripped=%q\n  raw=%q", strippedOff, raw)
	}

	// Search should find matches using the updated stripped cache.
	if err := lv.ApplySearch("connection", msgs.SearchModeSearch); err != nil {
		t.Fatalf("ApplySearch: %v", err)
	}
	if len(lv.matchPositions) != 1 {
		t.Fatalf("expected 1 match, got %d", len(lv.matchPositions))
	}
	pos := lv.matchPositions[0]
	matched := strippedOff[pos.colStart:pos.colEnd]
	if matched != "connection" {
		t.Fatalf("expected 'connection' at [%d:%d], got %q", pos.colStart, pos.colEnd, matched)
	}

	// Toggle syntax back ON and verify stripped cache is updated again.
	lv.ToggleSyntax()
	strippedOnAgain := lv.buffer.StrippedGet(0)
	if strippedOnAgain != strippedOn {
		t.Fatalf("after toggling syntax back ON, stripped should match original.\n  got=%q\n  want=%q", strippedOnAgain, strippedOn)
	}

	// Search again — positions should correspond to the reformatted text.
	if err := lv.ApplySearch("connection", msgs.SearchModeSearch); err != nil {
		t.Fatalf("ApplySearch: %v", err)
	}
	if len(lv.matchPositions) != 1 {
		t.Fatalf("expected 1 match after toggle back on, got %d", len(lv.matchPositions))
	}
	pos = lv.matchPositions[0]
	matched = strippedOnAgain[pos.colStart:pos.colEnd]
	if matched != "connection" {
		t.Fatalf("expected 'connection' at [%d:%d] after toggle on, got %q", pos.colStart, pos.colEnd, matched)
	}
}

func TestLogView_SearchWithFilterUsesCachedStrippedLines(t *testing.T) {
	// Verify that the filtered search path also uses cached stripped lines.
	lv := NewLogView(80, 24, 100, "15m", 900)

	lv.AppendLine("ERROR: first failure")
	lv.AppendLine("INFO: all good")
	lv.AppendLine("ERROR: second failure")

	// Apply filter for ERROR lines.
	if err := lv.ApplySearch("ERROR", msgs.SearchModeFilter); err != nil {
		t.Fatalf("ApplySearch filter: %v", err)
	}
	if len(lv.filteredIndices) != 2 {
		t.Fatalf("expected 2 filtered lines, got %d", len(lv.filteredIndices))
	}

	// Verify cached stripped lines match ansi.Strip(colored) for filtered lines.
	for _, idx := range lv.filteredIndices {
		bufIdx := idx - lv.filteredIndexOffset
		cached := lv.buffer.StrippedGet(bufIdx)
		expected := ansi.Strip(lv.buffer.ColoredGet(bufIdx))
		if cached != expected {
			t.Fatalf("bufIdx %d: StrippedGet=%q != ansi.Strip(colored)=%q", bufIdx, cached, expected)
		}
	}

	// Search within filtered lines.
	if err := lv.ApplySearch("failure", msgs.SearchModeSearch); err != nil {
		t.Fatalf("ApplySearch search: %v", err)
	}
	if len(lv.matchPositions) != 2 {
		t.Fatalf("expected 2 matches for 'failure', got %d", len(lv.matchPositions))
	}
}

func TestLogView_BinarySearchMatchWindow_NoMatchesInWindow(t *testing.T) {
	// All matches are outside the visible window.
	lv := NewLogView(80, 7, 100, "15m", 900) // viewport height = 5
	lv.ToggleSyntax()                         // disable syntax highlighting for predictable text

	// Add 20 lines; matches on lines 0, 1, 2 only.
	for i := range 20 {
		if i < 3 {
			lv.AppendLine(fmt.Sprintf("line %02d needle", i))
		} else {
			lv.AppendLine(fmt.Sprintf("line %02d other", i))
		}
	}

	if err := lv.ApplySearch("needle", msgs.SearchModeSearch); err != nil {
		t.Fatalf("ApplySearch: %v", err)
	}
	if len(lv.matchPositions) != 3 {
		t.Fatalf("expected 3 matches, got %d", len(lv.matchPositions))
	}

	// Scroll to the bottom where no matches exist (lines 15-19 visible).
	lv.autoscroll = false
	lv.scrollOffset = 15
	lv.updateViewport()

	// The viewport should render correctly without any highlighted matches.
	view := ansi.Strip(lv.logVP.View())
	if strings.Contains(view, "needle") {
		t.Fatal("no match lines should be visible when scrolled past them")
	}
	if !strings.Contains(view, "line 15") {
		t.Fatal("expected line 15 to be visible")
	}
}

func TestLogView_BinarySearchMatchWindow_AllMatchesInWindow(t *testing.T) {
	// All matches fall within the visible window.
	lv := NewLogView(80, 7, 100, "15m", 900) // viewport height = 5
	lv.ToggleSyntax()

	for i := range 5 {
		lv.AppendLine(fmt.Sprintf("line %d needle", i))
	}

	if err := lv.ApplySearch("needle", msgs.SearchModeSearch); err != nil {
		t.Fatalf("ApplySearch: %v", err)
	}
	if len(lv.matchPositions) != 5 {
		t.Fatalf("expected 5 matches, got %d", len(lv.matchPositions))
	}

	// Scroll to top; all 5 lines fit in the viewport.
	lv.autoscroll = false
	lv.scrollOffset = 0
	lv.updateViewport()

	view := ansi.Strip(lv.logVP.View())
	for i := range 5 {
		needle := fmt.Sprintf("line %d", i)
		if !strings.Contains(view, needle) {
			t.Fatalf("expected %q to be visible", needle)
		}
	}
}

func TestLogView_BinarySearchMatchWindow_SpanningBoundary(t *testing.T) {
	// Matches span the window boundary: some inside, some outside.
	lv := NewLogView(80, 7, 100, "15m", 900) // viewport height = 5
	lv.ToggleSyntax()

	// 10 lines, every other line has a match.
	for i := range 10 {
		if i%2 == 0 {
			lv.AppendLine(fmt.Sprintf("line %02d needle", i))
		} else {
			lv.AppendLine(fmt.Sprintf("line %02d other", i))
		}
	}

	if err := lv.ApplySearch("needle", msgs.SearchModeSearch); err != nil {
		t.Fatalf("ApplySearch: %v", err)
	}
	// Matches on lines 0, 2, 4, 6, 8.
	if len(lv.matchPositions) != 5 {
		t.Fatalf("expected 5 matches, got %d", len(lv.matchPositions))
	}

	// Window shows lines 3..7 (scrollOffset=3, height=5).
	// Matches inside window: line 4 and line 6.
	lv.autoscroll = false
	lv.scrollOffset = 3
	lv.updateViewport()

	view := ansi.Strip(lv.logVP.View())
	// Lines 3..7 should be visible.
	if !strings.Contains(view, "line 03") {
		t.Fatal("expected line 03 to be visible")
	}
	if !strings.Contains(view, "line 07") {
		t.Fatal("expected line 07 to be visible")
	}
	// Matches outside window should not be present.
	if strings.Contains(view, "line 00") {
		t.Fatal("line 00 should not be visible")
	}
	if strings.Contains(view, "line 08") {
		t.Fatal("line 08 should not be visible")
	}
}

func TestLogView_BinarySearchMatchWindow_SingleMatchAtEdge(t *testing.T) {
	// A single match at the very first and very last position of the window.
	lv := NewLogView(80, 7, 100, "15m", 900) // viewport height = 5
	lv.ToggleSyntax()

	for i := range 10 {
		lv.AppendLine(fmt.Sprintf("line %02d other", i))
	}

	// Overwrite buffer entry at line 3 and 7 to contain "needle".
	// Instead, let's just set up a scenario with matches at window edges.
	lv2 := NewLogView(80, 7, 100, "15m", 900)
	lv2.ToggleSyntax()

	// 10 lines; matches only on lines 3 (first in window) and 7 (last in window).
	for i := range 10 {
		if i == 3 || i == 7 {
			lv2.AppendLine(fmt.Sprintf("line %02d needle", i))
		} else {
			lv2.AppendLine(fmt.Sprintf("line %02d other", i))
		}
	}

	if err := lv2.ApplySearch("needle", msgs.SearchModeSearch); err != nil {
		t.Fatalf("ApplySearch: %v", err)
	}
	if len(lv2.matchPositions) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(lv2.matchPositions))
	}

	// Window shows lines 3..7 (scrollOffset=3, height=5).
	// Match at line 3 = first line of window.
	// Match at line 7 = last line of window.
	lv2.autoscroll = false
	lv2.scrollOffset = 3
	lv2.updateViewport()

	view := ansi.Strip(lv2.logVP.View())
	if !strings.Contains(view, "line 03 needle") {
		t.Fatal("expected match at first window edge (line 03) to be visible")
	}
	if !strings.Contains(view, "line 07 needle") {
		t.Fatal("expected match at last window edge (line 07) to be visible")
	}
}

func TestLogView_BinarySearchMatchWindow_WrappedMode(t *testing.T) {
	// Verify binary search works correctly in wrapped mode too.
	lv := NewLogView(42, 7, 100, "15m", 900) // viewport width=40, height=5
	lv.softWrap = true
	lv.ToggleSyntax()

	// 20 lines; every 5th has "needle".
	for i := range 20 {
		if i%5 == 0 {
			lv.AppendLine(fmt.Sprintf("line %02d needle", i))
		} else {
			lv.AppendLine(fmt.Sprintf("line %02d other text", i))
		}
	}

	if err := lv.ApplySearch("needle", msgs.SearchModeSearch); err != nil {
		t.Fatalf("ApplySearch: %v", err)
	}
	// Matches on lines 0, 5, 10, 15.
	if len(lv.matchPositions) != 4 {
		t.Fatalf("expected 4 matches, got %d", len(lv.matchPositions))
	}

	// Scroll to top.
	lv.autoscroll = false
	lv.wrapYOffset = 0
	lv.updateViewport()

	view := ansi.Strip(lv.logVP.View())
	// Line 0 should be visible and contain "needle".
	if !strings.Contains(view, "line 00 needle") {
		t.Fatal("expected line 00 with needle at top of wrapped viewport")
	}
	// Lines beyond the viewport (e.g., 15) should not be visible.
	if strings.Contains(view, "line 15") {
		t.Fatal("line 15 should not be visible when scrolled to top")
	}
}

func TestLogView_ApplySearchEmptyFilterNoPanic(t *testing.T) {
	lv := NewLogView(80, 20, 100, "15m", 900)
	lv.AppendLine("line one")
	lv.AppendLine("line two")

	// Empty pattern in filter mode should not panic (was nil deref on filterState.Re)
	err := lv.ApplySearch("", msgs.SearchModeFilter)
	if err != nil {
		t.Fatalf("ApplySearch with empty filter should not error: %v", err)
	}
	if lv.FilterActive() {
		t.Fatal("filter should not be active after empty pattern")
	}
}

func TestLogView_WrappedVOffsetSkipFirstLine(t *testing.T) {
	// A long line at vpWidth=10 (contWidth=8) wraps into multiple rows.
	// Scroll to the middle and verify the visible segments show correct content.
	const vpWidth = 10
	const contWidth = vpWidth - wrapIndicatorWidth // 8
	// LogView border takes 2 from each dimension, so inner width = vpWidth.
	lv := NewLogView(vpWidth+2, 12, 100, "15m", 900) // inner: 10 wide, 10 tall
	lv.softWrap = true
	lv.autoscroll = false
	lv.ToggleSyntax() // disable syntax highlighting for predictable text

	// Build a long line with identifiable content. Use alphabetic markers
	// at predictable offsets so we can verify which portion is displayed.
	// Row 0 holds 10 chars, each continuation row holds 8 chars.
	// We need enough content to produce many rows.
	// Total chars for N rows: 10 + (N-1)*8. For 63 rows: 10 + 62*8 = 506.
	var sb strings.Builder
	for i := range 506 {
		sb.WriteByte('A' + byte(i%26))
	}
	longLine := sb.String()

	lv.AppendLine(longLine)

	// Verify the line wraps to the expected number of rows.
	w := lv.buffer.WidthGet(0)
	expectedRows := wrapHeight(w, vpWidth)
	if expectedRows != 63 {
		t.Fatalf("expected 63 wrapped rows, got %d (width=%d, vpWidth=%d)", expectedRows, w, vpWidth)
	}

	// Scroll to the middle: wrapYOffset=25 means we skip 25 visual rows.
	lv.wrapYOffset = 25
	lv.updateViewport()

	vpHeight := lv.viewportHeight()
	lines := lv.logVP.lines

	// Should have vpHeight lines (or fewer if content runs out).
	if len(lines) > vpHeight {
		t.Fatalf("expected at most %d lines, got %d", vpHeight, len(lines))
	}

	// All visible segments are continuation rows (startRow > 0) so each has
	// the "↪ " prefix prepended.

	// Row 0: chars 0-9 (10 chars). Row k (k>=1): chars 10+(k-1)*8 .. 10+k*8-1.
	// Row 25: chars 10 + 24*8 = 202 .. 209.
	expectedStartChar := 10 + 24*contWidth // 202
	expectedContent := longLine[expectedStartChar : expectedStartChar+contWidth]

	firstStripped := ansi.Strip(lines[0])
	if !strings.HasPrefix(firstStripped, "↪ ") {
		t.Fatalf("expected first visible segment to start with '↪ ', got %q", firstStripped)
	}
	// Content after prefix should be the 8 chars from the original line.
	firstContent := firstStripped[len("↪ "):]
	if firstContent != expectedContent {
		t.Fatalf("expected first visible content %q, got %q", expectedContent, firstContent)
	}

	// Verify the second visible segment comes from row 26.
	if len(lines) > 1 {
		secondStripped := ansi.Strip(lines[1])
		if !strings.HasPrefix(secondStripped, "↪ ") {
			t.Fatalf("expected second visible segment to start with '↪ ', got %q", secondStripped)
		}
		expectedStart2 := expectedStartChar + contWidth
		expectedContent2 := longLine[expectedStart2 : expectedStart2+contWidth]
		secondContent := secondStripped[len("↪ "):]
		if secondContent != expectedContent2 {
			t.Fatalf("expected second visible content %q, got %q", expectedContent2, secondContent)
		}
	}

	// Verify the last visible row.
	lastRow := 25 + vpHeight - 1
	if lastRow >= expectedRows {
		lastRow = expectedRows - 1
	}
	lastStripped := ansi.Strip(lines[len(lines)-1])
	if !strings.HasPrefix(lastStripped, "↪ ") {
		t.Fatalf("expected last visible segment to start with '↪ ', got %q", lastStripped)
	}
	lastStartChar := 10 + (lastRow-1)*contWidth
	lastEndChar := lastStartChar + contWidth
	if lastEndChar > len(longLine) {
		lastEndChar = len(longLine)
	}
	expectedLastContent := longLine[lastStartChar:lastEndChar]
	lastContent := lastStripped[len("↪ "):]
	if lastContent != expectedLastContent {
		t.Fatalf("expected last visible content %q, got %q", expectedLastContent, lastContent)
	}
}

func TestLogView_WrapContinuationIndicator(t *testing.T) {
	// Append a long line that wraps, enable wrap, and verify that
	// continuation rows in logVP.lines contain the "↪" indicator
	// while the first row does not. Also verify actual row widths.
	const vpWidth = 20
	contWidth := vpWidth - wrapIndicatorWidth // 18
	lv := NewLogView(vpWidth+2, 12, 100, "15m", 900) // inner: 20 wide, 10 tall
	lv.softWrap = true
	lv.autoscroll = true
	lv.ToggleSyntax() // disable syntax highlighting for predictable text

	// 60 chars wraps: row 0 = 20, row 1 = 18+2(indicator) = 20, row 2 = 18+2 = 20,
	// row 3 = remaining 4 + 2(indicator) = 6.
	longLine := strings.Repeat("A", 60)
	lv.AppendLine(longLine)

	lines := lv.logVP.lines
	// Expected rows: row 0 (20 chars), then continuation rows of 18 content chars each.
	// 60 - 20 = 40 remaining, 40/18 = 2 full + 4 leftover => 4 rows total.
	if len(lines) < 4 {
		t.Fatalf("expected at least 4 wrapped rows, got %d", len(lines))
	}

	// First row should NOT have the continuation indicator and should be vpWidth wide.
	firstStripped := ansi.Strip(lines[0])
	if strings.Contains(firstStripped, "↪") {
		t.Fatalf("first row should not contain ↪, got %q", firstStripped)
	}
	firstW := ansi.StringWidth(lines[0])
	if firstW != vpWidth {
		t.Fatalf("first row width = %d, want %d (vpWidth)", firstW, vpWidth)
	}

	// Continuation rows (index 1+) should start with "↪ " and have correct widths.
	for i := 1; i < len(lines); i++ {
		stripped := ansi.Strip(lines[i])
		if !strings.HasPrefix(stripped, "↪ ") {
			t.Fatalf("continuation row %d should start with '↪ ', got %q", i, stripped)
		}
		rowW := ansi.StringWidth(lines[i])
		// Content after removing the indicator prefix.
		contentPart := stripped[len("↪ "):]
		contentW := ansi.StringWidth(contentPart)
		// Full continuation rows should have contWidth content columns.
		// The last row may be shorter.
		if i < len(lines)-1 {
			if contentW != contWidth {
				t.Fatalf("continuation row %d content width = %d, want %d (contWidth)", i, contentW, contWidth)
			}
			if rowW != vpWidth {
				t.Fatalf("continuation row %d total width = %d, want %d (vpWidth)", i, rowW, vpWidth)
			}
		} else {
			// Last row: content width should be the remainder.
			remaining := 60 - vpWidth - contWidth*(len(lines)-2)
			if contentW != remaining {
				t.Fatalf("last continuation row content width = %d, want %d", contentW, remaining)
			}
		}
	}
}

func TestComputeLineMatchPositions_MatchesJoinAllApproach(t *testing.T) {
	// Verify that the per-line computeLineMatchPositions approach produces
	// identical match positions to the old join-all approach (join lines,
	// run FindAllStringIndex, call computeMatchPositions).
	tests := []struct {
		name  string
		lines []string
		pattern string
	}{
		{
			name:    "simple matches across lines",
			lines:   []string{"foo bar", "baz foo", "no match here", "foo end"},
			pattern: "foo",
		},
		{
			name:    "match at line boundaries",
			lines:   []string{"endfoo", "foostart", "midfoomid"},
			pattern: "foo",
		},
		{
			name:    "multiple matches per line",
			lines:   []string{"foo foo foo", "bar", "foo bar foo"},
			pattern: "foo",
		},
		{
			name:    "no matches",
			lines:   []string{"abc", "def", "ghi"},
			pattern: "xyz",
		},
		{
			name:    "empty lines mixed in",
			lines:   []string{"foo", "", "foo", "", ""},
			pattern: "foo",
		},
		{
			name:    "single line",
			lines:   []string{"hello foo world"},
			pattern: "foo",
		},
		{
			name:    "regex pattern",
			lines:   []string{"error: 123", "warn: 456", "error: 789"},
			pattern: `error: \d+`,
		},
		{
			name:    "match spanning would cross newline in join-all but not per-line",
			lines:   []string{"abc", "def"},
			pattern: "c\nd",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			re := regexp.MustCompile(tt.pattern)

			// Per-line approach (new)
			var perLinePositions []matchPosition
			for lineIdx, line := range tt.lines {
				perLinePositions = append(perLinePositions,
					computeLineMatchPositions(line, re, lineIdx)...)
			}

			// Join-all approach (old) — but only counting non-cross-line matches
			// The per-line approach by definition cannot find cross-line matches,
			// so we compare only within-line matches from the old approach.
			joined := strings.Join(tt.lines, "\n")
			rawMatches := re.FindAllStringIndex(joined, -1)
			oldPositions := computeMatchPositions(joined, rawMatches)

			// Filter old positions to exclude cross-newline matches
			// (those where the match spans more than one line).
			// Actually, computeMatchPositions already handles this by clamping
			// lineByteEnd, so a cross-line match would have a truncated colEnd.
			// The per-line approach simply won't find cross-line matches at all.
			// For fair comparison, skip the cross-line test case and compare directly.

			if tt.pattern == "c\nd" {
				// Cross-line pattern: per-line should find 0 matches
				// (since no single line contains "c\nd").
				if len(perLinePositions) != 0 {
					t.Fatalf("expected 0 per-line matches for cross-line pattern, got %d", len(perLinePositions))
				}
				return
			}

			// For all non-cross-line patterns, results should be identical.
			if len(perLinePositions) != len(oldPositions) {
				t.Fatalf("match count mismatch: per-line=%d, join-all=%d",
					len(perLinePositions), len(oldPositions))
			}
			for i := range perLinePositions {
				pl := perLinePositions[i]
				ol := oldPositions[i]
				if pl.line != ol.line || pl.colStart != ol.colStart || pl.colEnd != ol.colEnd {
					t.Errorf("match %d mismatch: per-line={line:%d, col:%d-%d} join-all={line:%d, col:%d-%d}",
						i, pl.line, pl.colStart, pl.colEnd, ol.line, ol.colStart, ol.colEnd)
				}
			}
		})
	}
}

func TestLogView_IncrementalSearchAppendNewLines(t *testing.T) {
	// Verify that appending lines while search is active incrementally
	// updates matchPositions without a full rebuild.
	lv := NewLogView(80, 24, 100, "15m", 900)
	lv.ToggleSyntax() // disable syntax highlighting for predictable text

	// Append initial lines and activate search.
	lv.AppendLine("foo bar")
	lv.AppendLine("baz qux")
	if err := lv.ApplySearch("foo", msgs.SearchModeSearch); err != nil {
		t.Fatalf("ApplySearch: %v", err)
	}
	if len(lv.matchPositions) != 1 {
		t.Fatalf("expected 1 match after initial search, got %d", len(lv.matchPositions))
	}
	if lv.searchState.MatchCount != 1 {
		t.Fatalf("expected MatchCount 1, got %d", lv.searchState.MatchCount)
	}

	// Append a line that matches while search is active.
	lv.AppendLine("foo end")
	if len(lv.matchPositions) != 2 {
		t.Fatalf("expected 2 matches after appending matching line, got %d", len(lv.matchPositions))
	}
	if lv.searchState.MatchCount != 2 {
		t.Fatalf("expected MatchCount 2, got %d", lv.searchState.MatchCount)
	}

	// Append a non-matching line — count should stay the same.
	lv.AppendLine("no match here")
	if len(lv.matchPositions) != 2 {
		t.Fatalf("expected 2 matches after appending non-matching line, got %d", len(lv.matchPositions))
	}
	if lv.searchState.MatchCount != 2 {
		t.Fatalf("expected MatchCount 2, got %d", lv.searchState.MatchCount)
	}

	// Append another matching line.
	lv.AppendLine("foo again")
	if len(lv.matchPositions) != 3 {
		t.Fatalf("expected 3 matches after second matching append, got %d", len(lv.matchPositions))
	}
	if lv.searchState.MatchCount != 3 {
		t.Fatalf("expected MatchCount 3, got %d", lv.searchState.MatchCount)
	}

	// Verify the new match is on the correct display line.
	// Lines: 0="foo bar", 1="baz qux", 2="foo end", 3="no match here", 4="foo again"
	// No indicator (no eviction), so display indices == buffer indices.
	lastMatch := lv.matchPositions[len(lv.matchPositions)-1]
	if lastMatch.line != 4 {
		t.Fatalf("expected last match on display line 4, got %d", lastMatch.line)
	}
}

func TestLogView_IncrementalSearchAppendWithEviction(t *testing.T) {
	// Verify that appending lines past buffer capacity (eviction)
	// triggers a full rebuild of matchPositions.
	lv := NewLogView(80, 24, 5, "15m", 900) // capacity 5
	lv.ToggleSyntax()

	// Activate search before filling buffer.
	if err := lv.ApplySearch("foo", msgs.SearchModeSearch); err != nil {
		t.Fatalf("ApplySearch: %v", err)
	}

	// Fill the buffer with matching lines.
	lv.AppendLine("foo one")
	lv.AppendLine("foo two")
	lv.AppendLine("bar three")
	lv.AppendLine("foo four")
	lv.AppendLine("bar five")
	// Buffer: [foo one, foo two, bar three, foo four, bar five]
	// Matches: foo one (0), foo two (1), foo four (3) = 3 matches.
	if len(lv.matchPositions) != 3 {
		t.Fatalf("expected 3 matches before eviction, got %d", len(lv.matchPositions))
	}
	if lv.searchState.MatchCount != 3 {
		t.Fatalf("expected MatchCount 3, got %d", lv.searchState.MatchCount)
	}

	// Append a matching line — triggers eviction of "foo one".
	lv.AppendLine("foo six")
	// Buffer: [foo two, bar three, foo four, bar five, foo six]
	// Dropped=1, indicator present. Matches: foo two, foo four, foo six = 3 matches.
	if len(lv.matchPositions) != 3 {
		t.Fatalf("expected 3 matches after eviction, got %d", len(lv.matchPositions))
	}
	if lv.searchState.MatchCount != 3 {
		t.Fatalf("expected MatchCount 3 after eviction, got %d", lv.searchState.MatchCount)
	}

	// With indicator at display 0, buffer lines start at display 1.
	// Buffer indices: 0=foo two, 1=bar three, 2=foo four, 3=bar five, 4=foo six
	// Display indices: 1=foo two, 2=bar three, 3=foo four, 4=bar five, 5=foo six
	// Matches at display lines: 1 (foo two), 3 (foo four), 5 (foo six)
	expectedDisplayLines := []int{1, 3, 5}
	for i, pos := range lv.matchPositions {
		if pos.line != expectedDisplayLines[i] {
			t.Errorf("match %d: expected display line %d, got %d", i, expectedDisplayLines[i], pos.line)
		}
	}

	// Append more lines to trigger additional evictions.
	lv.AppendLine("bar seven")
	lv.AppendLine("foo eight")
	// Buffer: [foo four, bar five, foo six, bar seven, foo eight]
	// Dropped=3, indicator present. Matches: foo four, foo six, foo eight = 3 matches.
	if len(lv.matchPositions) != 3 {
		t.Fatalf("expected 3 matches after multiple evictions, got %d", len(lv.matchPositions))
	}
	if lv.searchState.MatchCount != 3 {
		t.Fatalf("expected MatchCount 3 after multiple evictions, got %d", lv.searchState.MatchCount)
	}
}

func TestLogView_IncrementalSearchMatchCountAccuracy(t *testing.T) {
	// Verify that searchState.MatchCount stays perfectly in sync with
	// len(matchPositions) across many appends, with and without eviction.
	lv := NewLogView(80, 24, 10, "15m", 900) // capacity 10
	lv.ToggleSyntax()

	if err := lv.ApplySearch("needle", msgs.SearchModeSearch); err != nil {
		t.Fatalf("ApplySearch: %v", err)
	}

	// Append 30 lines: every 3rd line matches.
	for i := range 30 {
		if i%3 == 0 {
			lv.AppendLine(fmt.Sprintf("needle line %d", i))
		} else {
			lv.AppendLine(fmt.Sprintf("other line %d", i))
		}
		// After every append, MatchCount must equal len(matchPositions).
		if lv.searchState.MatchCount != len(lv.matchPositions) {
			t.Fatalf("after append %d: MatchCount=%d != len(matchPositions)=%d",
				i, lv.searchState.MatchCount, len(lv.matchPositions))
		}
	}

	// After 30 appends with capacity 10, buffer holds lines 20..29.
	// Matching lines among 20..29: 21 (21%3==0), 24, 27 = 3 matches.
	// Wait — 21%3==0 is true, 24%3==0 is true, 27%3==0 is true. So 3 matches.
	if lv.buffer.Dropped() != 20 {
		t.Fatalf("expected 20 dropped, got %d", lv.buffer.Dropped())
	}

	// Cross-check: do a full rebuild and compare.
	savedCount := lv.searchState.MatchCount
	savedPositions := make([]matchPosition, len(lv.matchPositions))
	copy(savedPositions, lv.matchPositions)

	lv.rebuildMatchPositions()

	if lv.searchState.MatchCount != savedCount {
		t.Fatalf("MatchCount mismatch: incremental=%d, full rebuild=%d",
			savedCount, lv.searchState.MatchCount)
	}
	if len(lv.matchPositions) != len(savedPositions) {
		t.Fatalf("matchPositions length mismatch: incremental=%d, full rebuild=%d",
			len(savedPositions), len(lv.matchPositions))
	}
	for i := range lv.matchPositions {
		if lv.matchPositions[i] != savedPositions[i] {
			t.Errorf("matchPositions[%d] mismatch: incremental=%+v, full rebuild=%+v",
				i, savedPositions[i], lv.matchPositions[i])
		}
	}
}

func TestLogView_IncrementalSearchDoesNotResetMatchIndex(t *testing.T) {
	// Verify that appending lines while search is active does NOT reset
	// matchIndex (user's current navigation position should be preserved).
	lv := NewLogView(80, 24, 100, "15m", 900)
	lv.ToggleSyntax()

	lv.AppendLine("foo one")
	lv.AppendLine("foo two")
	lv.AppendLine("foo three")

	if err := lv.ApplySearch("foo", msgs.SearchModeSearch); err != nil {
		t.Fatalf("ApplySearch: %v", err)
	}
	// matchIndex starts at 0 after ApplySearch.
	if lv.matchIndex != 0 {
		t.Fatalf("expected matchIndex 0 after ApplySearch, got %d", lv.matchIndex)
	}

	// Navigate to the second match.
	lv.SearchNext()
	if lv.matchIndex != 1 {
		t.Fatalf("expected matchIndex 1 after SearchNext, got %d", lv.matchIndex)
	}

	// Append a new matching line — matchIndex should stay at 1.
	lv.AppendLine("foo four")
	if lv.matchIndex != 1 {
		t.Fatalf("expected matchIndex 1 preserved after append, got %d", lv.matchIndex)
	}
	if len(lv.matchPositions) != 4 {
		t.Fatalf("expected 4 matches, got %d", len(lv.matchPositions))
	}
}

func TestLogView_IncrementalSearchWithFilterActive(t *testing.T) {
	// Verify incremental search update works when both filter and search are active.
	lv := NewLogView(80, 24, 100, "15m", 900)
	lv.ToggleSyntax()

	lv.AppendLine("ERROR: first failure")
	lv.AppendLine("INFO: all good")
	lv.AppendLine("ERROR: second failure")

	// Apply filter for ERROR lines.
	if err := lv.ApplySearch("ERROR", msgs.SearchModeFilter); err != nil {
		t.Fatalf("ApplySearch filter: %v", err)
	}
	// Apply search within filtered lines.
	if err := lv.ApplySearch("failure", msgs.SearchModeSearch); err != nil {
		t.Fatalf("ApplySearch search: %v", err)
	}
	if len(lv.matchPositions) != 2 {
		t.Fatalf("expected 2 matches in filtered set, got %d", len(lv.matchPositions))
	}

	// Append a line that matches both filter and search.
	lv.AppendLine("ERROR: third failure")
	if len(lv.matchPositions) != 3 {
		t.Fatalf("expected 3 matches after appending matching line, got %d", len(lv.matchPositions))
	}
	if lv.searchState.MatchCount != 3 {
		t.Fatalf("expected MatchCount 3, got %d", lv.searchState.MatchCount)
	}

	// Append a line that matches filter but NOT search.
	lv.AppendLine("ERROR: no match keyword here")
	if len(lv.matchPositions) != 3 {
		t.Fatalf("expected 3 matches (new ERROR line doesn't contain 'failure'), got %d", len(lv.matchPositions))
	}

	// Append a line that matches neither filter nor search.
	lv.AppendLine("INFO: irrelevant")
	if len(lv.matchPositions) != 3 {
		t.Fatalf("expected 3 matches (non-ERROR line should not change matches), got %d", len(lv.matchPositions))
	}
}

func TestComputeLineMatchPositions_IntegrationWithLogView(t *testing.T) {
	// Integration test: verify that rebuildViewportContent using the per-line
	// approach produces correct match positions for a LogView with real data.
	lv := NewLogView(80, 24, 100, "15m", 900)

	lines := []string{
		"2024-01-01 ERROR connection refused",
		"2024-01-01 INFO request completed",
		"2024-01-01 ERROR timeout occurred",
		"2024-01-01 DEBUG all clear",
		"2024-01-01 ERROR disk full",
	}
	for _, line := range lines {
		lv.AppendLine(line)
	}

	// Search for "ERROR" — should find 3 matches, one per ERROR line.
	if err := lv.ApplySearch("ERROR", msgs.SearchModeSearch); err != nil {
		t.Fatalf("ApplySearch: %v", err)
	}
	if len(lv.matchPositions) != 3 {
		t.Fatalf("expected 3 matches for 'ERROR', got %d", len(lv.matchPositions))
	}

	// Verify each match is on the correct line (lines 0, 2, 4 in stripped content).
	expectedLines := []int{0, 2, 4}
	for i, pos := range lv.matchPositions {
		if pos.line != expectedLines[i] {
			t.Errorf("match %d: expected line %d, got %d", i, expectedLines[i], pos.line)
		}
		// Verify the match columns point to "ERROR" in the stripped text.
		stripped := lv.buffer.StrippedGet(pos.line)
		matchedText := stripped[pos.colStart:pos.colEnd]
		if matchedText != "ERROR" {
			t.Errorf("match %d: expected 'ERROR' at [%d:%d], got %q",
				i, pos.colStart, pos.colEnd, matchedText)
		}
	}
}

func TestLogView_EnsureMatchVisible_HScrollMatchAtCol0(t *testing.T) {
	// Match at column 0 with xOffset 0 — no horizontal scroll change needed.
	lv := NewLogView(80, 24, 100, "15m", 900)
	lv.ToggleSyntax()
	lv.AppendLine("needle at the start")

	if err := lv.ApplySearch("needle", msgs.SearchModeSearch); err != nil {
		t.Fatalf("ApplySearch: %v", err)
	}
	if len(lv.matchPositions) != 1 {
		t.Fatalf("expected 1 match, got %d", len(lv.matchPositions))
	}
	// colStart should be 0
	if lv.matchPositions[0].colStart != 0 {
		t.Fatalf("expected colStart 0, got %d", lv.matchPositions[0].colStart)
	}

	lv.logVP.xOffset = 0
	lv.ensureMatchVisible()

	if lv.logVP.xOffset != 0 {
		t.Fatalf("expected xOffset to remain 0 for match at col 0, got %d", lv.logVP.xOffset)
	}
}

func TestLogView_EnsureMatchVisible_HScrollMatchAtHighCol(t *testing.T) {
	// Match far to the right — xOffset should adjust so the match is visible.
	lv := NewLogView(82, 24, 100, "15m", 900) // logVP.width = 80
	lv.ToggleSyntax()

	// Create a line with "needle" starting at column 200.
	line := strings.Repeat("x", 200) + "needle"
	lv.AppendLine(line)

	if err := lv.ApplySearch("needle", msgs.SearchModeSearch); err != nil {
		t.Fatalf("ApplySearch: %v", err)
	}
	if len(lv.matchPositions) != 1 {
		t.Fatalf("expected 1 match, got %d", len(lv.matchPositions))
	}
	if lv.matchPositions[0].colStart != 200 {
		t.Fatalf("expected colStart 200, got %d", lv.matchPositions[0].colStart)
	}

	lv.logVP.xOffset = 0
	lv.ensureMatchVisible()

	// xOffset should be colStart - hScrollPadding = 200 - 15 = 185
	expected := 200 - hScrollPadding
	if lv.logVP.xOffset != expected {
		t.Fatalf("expected xOffset %d, got %d", expected, lv.logVP.xOffset)
	}
}

func TestLogView_EnsureMatchVisible_HScrollMatchAlreadyVisible(t *testing.T) {
	// Match is already within the visible horizontal range — no change.
	lv := NewLogView(82, 24, 100, "15m", 900) // logVP.width = 80
	lv.ToggleSyntax()

	// "needle" at column 50
	line := strings.Repeat("x", 50) + "needle" + strings.Repeat("x", 100)
	lv.AppendLine(line)

	if err := lv.ApplySearch("needle", msgs.SearchModeSearch); err != nil {
		t.Fatalf("ApplySearch: %v", err)
	}
	if len(lv.matchPositions) != 1 {
		t.Fatalf("expected 1 match, got %d", len(lv.matchPositions))
	}

	// Set xOffset so column 50 is visible (xOffset=10, viewport shows cols 10..89).
	lv.logVP.xOffset = 10
	lv.ensureMatchVisible()

	// Match at col 50 is in [10, 90), so xOffset should not change.
	if lv.logVP.xOffset != 10 {
		t.Fatalf("expected xOffset to remain 10 (match already visible), got %d", lv.logVP.xOffset)
	}
}

func TestLogView_EnsureMatchVisible_HScrollMatchLeftOfViewport(t *testing.T) {
	// Match is to the left of the current viewport — xOffset should scroll back.
	lv := NewLogView(82, 24, 100, "15m", 900) // logVP.width = 80
	lv.ToggleSyntax()

	// "needle" at column 5
	line := strings.Repeat("x", 5) + "needle" + strings.Repeat("x", 200)
	lv.AppendLine(line)

	if err := lv.ApplySearch("needle", msgs.SearchModeSearch); err != nil {
		t.Fatalf("ApplySearch: %v", err)
	}
	if len(lv.matchPositions) != 1 {
		t.Fatalf("expected 1 match, got %d", len(lv.matchPositions))
	}

	// xOffset at 100: viewport shows cols 100..179, match at col 5 is off-screen left.
	lv.logVP.xOffset = 100
	lv.ensureMatchVisible()

	// xOffset should be max(0, 5-15) = 0 (clamped to 0).
	if lv.logVP.xOffset != 0 {
		t.Fatalf("expected xOffset 0 (clamped), got %d", lv.logVP.xOffset)
	}
}

func TestLogView_EnsureMatchVisible_HScrollNearStartClampToZero(t *testing.T) {
	// Match near start of line where padding would go negative — should clamp to 0.
	lv := NewLogView(82, 24, 100, "15m", 900) // logVP.width = 80
	lv.ToggleSyntax()

	// "needle" at column 5 (less than hScrollPadding=15)
	line := strings.Repeat("x", 5) + "needle"
	lv.AppendLine(line)

	if err := lv.ApplySearch("needle", msgs.SearchModeSearch); err != nil {
		t.Fatalf("ApplySearch: %v", err)
	}
	if lv.matchPositions[0].colStart != 5 {
		t.Fatalf("expected colStart 5, got %d", lv.matchPositions[0].colStart)
	}

	// Force xOffset past the match so it triggers adjustment.
	lv.logVP.xOffset = 90
	lv.ensureMatchVisible()

	// max(0, 5-15) = 0
	if lv.logVP.xOffset != 0 {
		t.Fatalf("expected xOffset 0 (clamped to 0 since colStart-padding < 0), got %d", lv.logVP.xOffset)
	}
}

func TestLogView_EnsureMatchVisible_WrapFirstRow(t *testing.T) {
	// Match on the first wrapped row of a long line — no extra offset needed.
	// vpWidth = 20, vpHeight = 10
	lv := NewLogView(22, 12, 100, "15m", 900)
	lv.ToggleSyntax() // disable highlighting so widths = len
	lv.ToggleWrap()

	// Line 0: 60 chars, wraps into 3 visual rows.
	// "needle" starts at column 5 (first wrapped row: cols 0-19).
	line := strings.Repeat("x", 5) + "needle" + strings.Repeat("x", 49)
	lv.AppendLine(line)

	if err := lv.ApplySearch("needle", msgs.SearchModeSearch); err != nil {
		t.Fatalf("ApplySearch: %v", err)
	}
	if len(lv.matchPositions) != 1 {
		t.Fatalf("expected 1 match, got %d", len(lv.matchPositions))
	}
	if lv.matchPositions[0].colStart != 5 {
		t.Fatalf("expected colStart 5, got %d", lv.matchPositions[0].colStart)
	}

	// visualRow = 0 (no lines before) + 5/20 = 0. Already visible.
	lv.wrapYOffset = 0
	lv.ensureMatchVisible()

	if lv.wrapYOffset != 0 {
		t.Fatalf("expected wrapYOffset 0 for match on first wrapped row, got %d", lv.wrapYOffset)
	}
}

func TestLogView_EnsureMatchVisible_WrapSecondRow(t *testing.T) {
	// Match on the second wrapped row of a long line — viewport must scroll down.
	// vpWidth = 20, vpHeight = 4
	lv := NewLogView(22, 6, 100, "15m", 900)
	lv.ToggleSyntax()
	lv.ToggleWrap()

	// Line 0: short line (1 visual row)
	lv.AppendLine("short")
	// Line 1: short line (1 visual row)
	lv.AppendLine("short")
	// Line 2: 60 chars with "needle" at column 25 (second wrapped row: cols 20-39).
	line2 := strings.Repeat("x", 25) + "needle" + strings.Repeat("x", 29)
	lv.AppendLine(line2)
	// Line 3: another long line to push total visual rows beyond viewport.
	// 60 chars -> 3 visual rows. Total: 1+1+3+3 = 8 visual rows.
	lv.AppendLine(strings.Repeat("y", 60))

	if err := lv.ApplySearch("needle", msgs.SearchModeSearch); err != nil {
		t.Fatalf("ApplySearch: %v", err)
	}
	if len(lv.matchPositions) != 1 {
		t.Fatalf("expected 1 match, got %d", len(lv.matchPositions))
	}
	if lv.matchPositions[0].colStart != 25 {
		t.Fatalf("expected colStart 25, got %d", lv.matchPositions[0].colStart)
	}

	// visualRow for lines before match: line0=1, line1=1 -> 2.
	// Plus wrapped row offset: 25/20 = 1 -> visualRow = 3.
	// With wrapYOffset=0, vpHeight=4: visible rows 0-3. Row 3 is the last visible.
	lv.wrapYOffset = 0
	lv.ensureMatchVisible()
	if lv.wrapYOffset != 0 {
		t.Fatalf("expected wrapYOffset 0 (match at row 3 visible in viewport 0-3), got %d", lv.wrapYOffset)
	}

	// Scroll viewport past the match: wrapYOffset=4 means visible rows 4-7.
	// maxOffset = max(8-4,0) = 4, so wrapYOffset=4 is valid.
	// Match at visualRow=3 < wrapYOffset=4 -> scroll up to 3.
	lv.wrapYOffset = 4
	lv.ensureMatchVisible()
	if lv.wrapYOffset != 3 {
		t.Fatalf("expected wrapYOffset 3 after scrolling up to match, got %d", lv.wrapYOffset)
	}
}

func TestLogView_EnsureMatchVisible_WrapLastRow(t *testing.T) {
	// Match on the last wrapped row of a very long line.
	// vpWidth = 20, vpHeight = 4
	lv := NewLogView(22, 6, 100, "15m", 900)
	lv.ToggleSyntax()
	lv.ToggleWrap()

	// Line 0: short (1 visual row)
	lv.AppendLine("short")
	// Line 1: 100 chars with "needle" at column 85 (wrapped row 85/20=4).
	// The line wraps into ceil(100/20)=5 visual rows.
	line := strings.Repeat("x", 85) + "needle" + strings.Repeat("x", 9)
	lv.AppendLine(line)

	if err := lv.ApplySearch("needle", msgs.SearchModeSearch); err != nil {
		t.Fatalf("ApplySearch: %v", err)
	}
	if len(lv.matchPositions) != 1 {
		t.Fatalf("expected 1 match, got %d", len(lv.matchPositions))
	}
	if lv.matchPositions[0].colStart != 85 {
		t.Fatalf("expected colStart 85, got %d", lv.matchPositions[0].colStart)
	}

	// visualRow: line0 = 1 row -> 1. Plus wrapped row offset: 85/20 = 4 -> visualRow = 5.
	// vpHeight = 4, wrapYOffset = 0: visible rows 0-3. Match at row 5 is off-screen.
	lv.wrapYOffset = 0
	lv.ensureMatchVisible()
	// visualRow(5) >= wrapYOffset(0)+vpHeight(4) -> wrapYOffset = 5-4+1 = 2
	if lv.wrapYOffset != 2 {
		t.Fatalf("expected wrapYOffset 2 after scrolling to match on last wrapped row, got %d", lv.wrapYOffset)
	}
}

func TestLogView_ScrollHome_ResetsXOffset(t *testing.T) {
	lv := NewLogView(80, 24, 100, "15m", 900)
	lv.ToggleSyntax()
	// Manually set xOffset to simulate scrolled state.
	lv.logVP.xOffset = 50
	lv.ScrollHome()
	if lv.logVP.xOffset != 0 {
		t.Fatalf("expected xOffset 0 after ScrollHome, got %d", lv.logVP.xOffset)
	}
}

func TestLogView_ScrollHome_NoOpInWrapMode(t *testing.T) {
	lv := NewLogView(80, 24, 100, "15m", 900)
	lv.ToggleSyntax()
	lv.ToggleWrap()
	lv.logVP.xOffset = 50
	lv.ScrollHome()
	// In wrap mode, xOffset should remain unchanged.
	if lv.logVP.xOffset != 50 {
		t.Fatalf("expected xOffset 50 (unchanged) in wrap mode, got %d", lv.logVP.xOffset)
	}
}

func TestLogView_ScrollEnd_ScrollsToLongestLine(t *testing.T) {
	lv := NewLogView(22, 6, 100, "15m", 900) // vpWidth = 20
	lv.ToggleSyntax()
	// Append lines of varying widths.
	lv.AppendLine("short")                                       // width 5
	lv.AppendLine(strings.Repeat("a", 60))                       // width 60
	lv.AppendLine(strings.Repeat("b", 40))                       // width 40

	lv.ScrollEnd()
	// maxWidth = 60, vpWidth = 20, expected xOffset = 60 - 20 = 40
	if lv.logVP.xOffset != 40 {
		t.Fatalf("expected xOffset 40 after ScrollEnd, got %d", lv.logVP.xOffset)
	}
}

func TestLogView_ScrollEnd_NoOpInWrapMode(t *testing.T) {
	lv := NewLogView(22, 6, 100, "15m", 900)
	lv.ToggleSyntax()
	lv.ToggleWrap()
	lv.AppendLine(strings.Repeat("a", 60))
	lv.ScrollEnd()
	// In wrap mode, xOffset should remain 0.
	if lv.logVP.xOffset != 0 {
		t.Fatalf("expected xOffset 0 in wrap mode, got %d", lv.logVP.xOffset)
	}
}

func TestLogView_ScrollEnd_AllLinesFitInViewport(t *testing.T) {
	lv := NewLogView(82, 6, 100, "15m", 900) // vpWidth = 80
	lv.ToggleSyntax()
	lv.AppendLine("short")     // width 5
	lv.AppendLine("also short") // width 10

	lv.ScrollEnd()
	// maxWidth = 10, vpWidth = 80 -> max(0, 10-80) = 0
	if lv.logVP.xOffset != 0 {
		t.Fatalf("expected xOffset 0 when all lines fit, got %d", lv.logVP.xOffset)
	}
}
