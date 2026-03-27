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
		"2024-03-22T14:30:00Z message",        // Z timezone
		"2024-03-22T14:30:00+05:30 message",   // offset timezone
		"2024-03-22 14:30:00.123456 message",   // space separator, microseconds
		"2024-03-22T14:30:00 message",          // no fractional, no tz
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
	for i := 0; i < 5; i++ {
		lv.AppendLine(fmt.Sprintf("short line %d", i))
	}
	for i := 0; i < 3; i++ {
		lv.AppendLine(strings.Repeat("long ", 20)) // 100 chars, wraps to ~3 rows each
	}

	// totalWrappedRows should account for all 8 logical lines (5 short + 3 wrapped)
	if lv.totalWrappedRows < 8 {
		t.Fatalf("expected at least 8 total wrapped rows, got %d", lv.totalWrappedRows)
	}
	// Should be scrolled to bottom (autoscroll)
	vpHeight := lv.viewportHeight()
	maxOff := lv.totalWrappedRows - vpHeight
	if maxOff < 0 {
		maxOff = 0
	}
	if lv.wrapYOffset != maxOff {
		t.Fatalf("expected wrapYOffset at bottom (%d), got %d", maxOff, lv.wrapYOffset)
	}
}

func TestLogView_NonWrappedModeUnchanged(t *testing.T) {
	lv := NewLogView(80, 12, 100, "15m", 900)
	// SoftWrap is off by default

	for i := 0; i < 20; i++ {
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

func TestLogView_InsertMarkerPassesFilter(t *testing.T) {
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

	// Insert a marker — it should pass the filter even though it doesn't match "ERROR"
	lv.InsertMarker()
	if len(lv.filteredIndices) != 2 {
		t.Fatalf("expected 2 filtered lines (1 match + 1 marker), got %d", len(lv.filteredIndices))
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
	for i := 0; i < 30; i++ {
		lv.AppendLine(fmt.Sprintf("line %d", i))
	}

	// Autoscroll should have us at bottom
	vpHeight := lv.viewportHeight()
	maxOff := lv.totalWrappedRows - vpHeight
	if maxOff < 0 {
		maxOff = 0
	}
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
	maxOff = lv.totalWrappedRows - vpHeight
	if maxOff < 0 {
		maxOff = 0
	}
	atBottom = lv.wrapYOffset >= maxOff
	if !atBottom {
		t.Fatal("expected at bottom after GotoBottom")
	}
	if !lv.autoscroll {
		t.Fatal("expected autoscroll on after GotoBottom")
	}
}
