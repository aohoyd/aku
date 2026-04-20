package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/aohoyd/aku/internal/config"
)

func testHintGroups() []config.HintGroup {
	return []config.HintGroup{
		{
			Scope: "Global",
			Hints: []config.KeyHint{
				{Key: "?", Help: "Toggle help"},
				{Key: "q", Help: "Quit"},
			},
		},
		{
			Scope: "Navigation",
			Hints: []config.KeyHint{
				{Key: "j/k", Help: "Move up/down"},
				{Key: "enter", Help: "Select"},
			},
		},
	}
}

func TestHelpOverlayOpen(t *testing.T) {
	h := NewHelpOverlay(80, 30)
	h.Open(testHintGroups())
	if !h.Active() {
		t.Fatal("expected overlay to be active after Open")
	}
}

func TestHelpOverlayInactiveByDefault(t *testing.T) {
	h := NewHelpOverlay(80, 30)
	if h.Active() {
		t.Fatal("expected overlay to be inactive by default")
	}
}

func TestHelpOverlayEscapeDismisses(t *testing.T) {
	h := NewHelpOverlay(80, 30)
	h.Open(testHintGroups())
	h, _ = h.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if h.Active() {
		t.Fatal("expected overlay to be inactive after Escape")
	}
}

func TestHelpOverlayQuestionMarkDismisses(t *testing.T) {
	h := NewHelpOverlay(80, 30)
	h.Open(testHintGroups())
	h, _ = h.Update(tea.KeyPressMsg{Code: -1, Text: "?"})
	if h.Active() {
		t.Fatal("expected overlay to be inactive after ?")
	}
}

func TestHelpOverlayQDismisses(t *testing.T) {
	h := NewHelpOverlay(80, 30)
	h.Open(testHintGroups())
	h, _ = h.Update(tea.KeyPressMsg{Code: -1, Text: "q"})
	if h.Active() {
		t.Fatal("expected overlay to be inactive after q")
	}
}

func TestHelpOverlayScrollDown(t *testing.T) {
	h := NewHelpOverlay(80, 10) // small height to enable scrolling
	h.Open(testHintGroups())
	before := h.scroll
	h, _ = h.Update(tea.KeyPressMsg{Code: -1, Text: "j"})
	// scroll may not advance if content fits; just verify no panic
	_ = before
	_ = h.scroll
}

func TestHelpOverlayScrollDownArrow(t *testing.T) {
	h := NewHelpOverlay(80, 10)
	h.Open(testHintGroups())
	h, _ = h.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	// just ensure no panic; scroll behavior depends on content height
}

func TestHelpOverlayScrollUp(t *testing.T) {
	h := NewHelpOverlay(80, 10)
	h.Open(testHintGroups())
	// scroll down first, then up
	h, _ = h.Update(tea.KeyPressMsg{Code: -1, Text: "j"})
	h, _ = h.Update(tea.KeyPressMsg{Code: -1, Text: "k"})
	if h.scroll != 0 {
		t.Fatalf("expected scroll 0 after up, got %d", h.scroll)
	}
}

func TestHelpOverlayScrollUpClampsAtZero(t *testing.T) {
	h := NewHelpOverlay(80, 30)
	h.Open(testHintGroups())
	h, _ = h.Update(tea.KeyPressMsg{Code: -1, Text: "k"})
	if h.scroll != 0 {
		t.Fatalf("expected scroll clamped at 0, got %d", h.scroll)
	}
}

// Build a group tall enough to force scrolling at height=10.
func tallHintGroups() []config.HintGroup {
	hints := make([]config.KeyHint, 0, 30)
	for i := range 30 {
		hints = append(hints, config.KeyHint{Key: "k", Help: "move"})
		_ = i
	}
	return []config.HintGroup{{Scope: "Big", Hints: hints}}
}

func TestHelpOverlayScrollWheelDown(t *testing.T) {
	h := NewHelpOverlay(80, 10) // small height forces maxScroll > 0
	h.Open(tallHintGroups())
	if h.maxScroll() == 0 {
		t.Skip("tall hint group did not exceed visibleHeight; cannot test scroll")
	}
	before := h.scroll
	h.ScrollWheel(tea.MouseWheelDown)
	if h.scroll != before+1 {
		t.Fatalf("expected scroll to advance to %d after wheel down, got %d",
			before+1, h.scroll)
	}
}

func TestHelpOverlayScrollWheelUp(t *testing.T) {
	h := NewHelpOverlay(80, 10)
	h.Open(tallHintGroups())
	if h.maxScroll() == 0 {
		t.Skip("tall hint group did not exceed visibleHeight")
	}
	// Advance then roll back.
	h.ScrollWheel(tea.MouseWheelDown)
	h.ScrollWheel(tea.MouseWheelDown)
	h.ScrollWheel(tea.MouseWheelUp)
	if h.scroll != 1 {
		t.Fatalf("expected scroll=1 after up, got %d", h.scroll)
	}
}

func TestHelpOverlayScrollWheelClampsAtBounds(t *testing.T) {
	h := NewHelpOverlay(80, 10)
	h.Open(tallHintGroups())

	// Wheel up on scroll==0 stays at 0.
	h.ScrollWheel(tea.MouseWheelUp)
	if h.scroll != 0 {
		t.Fatalf("expected scroll=0 clamp after wheel up at 0, got %d", h.scroll)
	}

	// Wheel down beyond maxScroll stays at maxScroll.
	max := h.maxScroll()
	if max == 0 {
		t.Skip("no scroll range to clamp")
	}
	for range max + 5 {
		h.ScrollWheel(tea.MouseWheelDown)
	}
	if h.scroll != max {
		t.Fatalf("expected scroll clamped to maxScroll=%d, got %d", max, h.scroll)
	}
}

func TestHelpOverlayView(t *testing.T) {
	h := NewHelpOverlay(80, 30)
	h.Open(testHintGroups())
	view := h.View()
	if view == "" {
		t.Fatal("expected non-empty view")
	}
}

func TestHelpOverlayViewEmptyWhenInactive(t *testing.T) {
	h := NewHelpOverlay(80, 30)
	view := h.View()
	if view != "" {
		t.Fatal("expected empty view when inactive")
	}
}

func TestHelpOverlaySearchMode(t *testing.T) {
	h := NewHelpOverlay(80, 30)
	h.Open(testHintGroups())

	// Enter search mode with /
	h, _ = h.Update(tea.KeyPressMsg{Code: -1, Text: "/"})
	if !h.searching {
		t.Fatal("expected searching to be true after /")
	}

	// Type a query
	h, _ = h.Update(tea.KeyPressMsg{Code: -1, Text: "q"})
	if h.query != "q" {
		t.Fatalf("expected query 'q', got %q", h.query)
	}

	// Escape in search mode clears query, exits search
	h, _ = h.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if h.searching {
		t.Fatal("expected searching to be false after Escape in search mode")
	}
	if h.query != "" {
		t.Fatalf("expected empty query after Escape in search mode, got %q", h.query)
	}
	// Overlay should still be active (Escape in search only exits search)
	if !h.Active() {
		t.Fatal("expected overlay to remain active after Escape in search mode")
	}
}

func TestHelpOverlaySearchEnterConfirms(t *testing.T) {
	h := NewHelpOverlay(80, 30)
	h.Open(testHintGroups())

	h, _ = h.Update(tea.KeyPressMsg{Code: -1, Text: "/"})
	h, _ = h.Update(tea.KeyPressMsg{Code: -1, Text: "q"})
	h, _ = h.Update(tea.KeyPressMsg{Code: -1, Text: "u"})

	// Enter confirms the search (exits search mode but keeps query)
	h, _ = h.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if h.searching {
		t.Fatal("expected searching to be false after Enter")
	}
	if h.query != "qu" {
		t.Fatalf("expected query 'qu' preserved after Enter, got %q", h.query)
	}
}

func TestHelpOverlaySearchBackspace(t *testing.T) {
	h := NewHelpOverlay(80, 30)
	h.Open(testHintGroups())

	h, _ = h.Update(tea.KeyPressMsg{Code: -1, Text: "/"})
	h, _ = h.Update(tea.KeyPressMsg{Code: -1, Text: "a"})
	h, _ = h.Update(tea.KeyPressMsg{Code: -1, Text: "b"})
	if h.query != "ab" {
		t.Fatalf("expected query 'ab', got %q", h.query)
	}

	h, _ = h.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if h.query != "a" {
		t.Fatalf("expected query 'a' after backspace, got %q", h.query)
	}

	// Backspace on single char exits search mode
	h, _ = h.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if h.query != "" {
		t.Fatalf("expected empty query after backspace, got %q", h.query)
	}
	if h.searching {
		t.Fatal("expected searching to be false after query becomes empty")
	}
}

func TestHelpOverlayUpdateReturnsNilCmd(t *testing.T) {
	h := NewHelpOverlay(80, 30)
	h.Open(testHintGroups())
	_, cmd := h.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd != nil {
		t.Fatal("expected nil command from help overlay Update")
	}
}
