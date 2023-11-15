package ui

import (
	"strings"
	"testing"

	"github.com/aohoyd/aku/internal/msgs"
	"github.com/aohoyd/aku/internal/theme"
)

func TestLogView_AppendLine(t *testing.T) {
	lv := NewLogView(80, 24, 100, nil)
	lv.AppendLine("hello")
	lv.AppendLine("world")
	if lv.buffer.Len() != 2 {
		t.Fatalf("expected 2 lines, got %d", lv.buffer.Len())
	}
}

func TestLogView_Autoscroll(t *testing.T) {
	lv := NewLogView(80, 24, 100, nil)
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
	lv := NewLogView(80, 24, 100, nil)
	var _ Searchable = &lv
}

func TestLogView_FilterHidesLines(t *testing.T) {
	lv := NewLogView(80, 24, 100, nil)
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
	lv := NewLogView(80, 24, 100, nil)
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
	lv := NewLogView(80, 24, 100, nil)
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

func TestLogView_SearchWithHighlightRules(t *testing.T) {
	rules := []theme.LogHighlightRule{
		{Pattern: "ERROR", Fg: "#ff0000", Bold: true},
	}
	lv := NewLogView(80, 24, 100, rules)
	lv.AppendLine("INFO: all good")
	lv.AppendLine("ERROR: something failed")
	lv.AppendLine("INFO: recovered")

	if err := lv.ApplySearch("something", msgs.SearchModeSearch); err != nil {
		t.Fatalf("ApplySearch: %v", err)
	}
	// With highlight rules active, matchPositions should be populated
	// (ANSI-aware path)
	if len(lv.matchPositions) != 1 {
		t.Fatalf("expected 1 match, got %d", len(lv.matchPositions))
	}
	pos := lv.matchPositions[0]
	if pos.line != 1 {
		t.Errorf("expected match on line 1, got %d", pos.line)
	}
}

func TestLogView_ClearSearchResetsMatchState(t *testing.T) {
	lv := NewLogView(80, 24, 100, nil)
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
	// Capacity 5 buffer, filter active for "error"
	lv := NewLogView(80, 24, 5, nil)
	if err := lv.ApplySearch("error", msgs.SearchModeFilter); err != nil {
		t.Fatalf("ApplySearch: %v", err)
	}

	// Fill buffer: 3 matching, 2 non-matching
	lv.AppendLine("error: one")
	lv.AppendLine("info: two")
	lv.AppendLine("error: three")
	lv.AppendLine("info: four")
	lv.AppendLine("error: five")

	// Buffer is now full (5/5). Viewport should show 3 error lines.
	content := lv.viewport.View()
	if !strings.Contains(content, "error: one") {
		t.Fatal("expected 'error: one' in viewport before eviction")
	}

	// Append non-matching line -> evicts "error: one" (oldest, which matched)
	lv.AppendLine("info: six")

	// After eviction, "error: one" should be gone from viewport
	content = lv.viewport.View()
	if strings.Contains(content, "error: one") {
		t.Fatal("evicted line 'error: one' should not appear in viewport after eviction")
	}
	// Remaining matches should still be visible
	if !strings.Contains(content, "error: three") {
		t.Fatal("expected 'error: three' to remain visible")
	}
	if !strings.Contains(content, "error: five") {
		t.Fatal("expected 'error: five' to remain visible")
	}
}

func TestLogView_SearchDoesNotMatchDroppedIndicator(t *testing.T) {
	// Capacity 3 buffer — fill with 4 lines to trigger dropped indicator
	lv := NewLogView(80, 24, 3, nil)
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
	lv := NewLogView(80, 24, 3, nil)
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
	lv := NewLogView(80, 24, 100, nil)

	// Initially available
	if lv.IsUnavailable() {
		t.Fatal("expected available by default")
	}
	title := lv.buildTitle()
	if title != "Logs [A]" {
		t.Fatalf("expected default title 'Logs [A]', got %q", title)
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
	if title != "Logs [A]" {
		t.Fatalf("expected default title after clearing unavailable, got %q", title)
	}
}

func TestLogView_ClearAndRestartResetsFilterAndSearch(t *testing.T) {
	lv := NewLogView(80, 24, 100, nil)
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

func TestLogView_ApplyHighlightsMultiByte(t *testing.T) {
	rules := []theme.LogHighlightRule{
		{Pattern: "ready", Fg: "#00ff00"},
	}
	lv := NewLogView(80, 24, 100, rules)

	// Line with multi-byte prefix: "✓" is 3 bytes (E2 9C 93) but 1 grapheme column
	line := "✓ pod ready"
	result := lv.applyHighlights(line)

	// The result should contain ANSI codes highlighting "ready"
	if result == line {
		t.Fatal("expected highlights to be applied")
	}
	// "ready" should still be present in the output (not garbled)
	if !strings.Contains(result, "ready") {
		t.Fatalf("expected 'ready' in highlighted output, got %q", result)
	}
}
