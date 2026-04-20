package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func newTestPicker(items []string) Picker[string] {
	p := NewPicker(PickerConfig[string]{
		Title:      "Test Picker",
		NoItemsMsg: "(no matches)",
		Display:    func(s string) string { return s },
		Filter: func(query string, items []string) []string {
			if query == "" {
				return items
			}
			lower := strings.ToLower(query)
			var out []string
			for _, s := range items {
				if strings.Contains(strings.ToLower(s), lower) {
					out = append(out, s)
				}
			}
			return out
		},
		OnSelect: func(s string) tea.Cmd {
			return func() tea.Msg { return s }
		},
	}, 80, 24)
	p.SetItems(items)
	return p
}

func TestPickerOpenClose(t *testing.T) {
	p := newTestPicker([]string{"a", "b", "c"})
	p.Open()
	if !p.Active() {
		t.Fatal("picker should be active after Open")
	}
	p.Close()
	if p.Active() {
		t.Fatal("picker should be inactive after Close")
	}
}

func TestPickerViewRendersWhenActive(t *testing.T) {
	p := newTestPicker([]string{"alpha", "beta"})
	p.Open()
	view := p.View()
	if view == "" {
		t.Fatal("active picker should render something")
	}
	if !strings.Contains(view, "Test Picker") {
		t.Fatal("view should contain the title")
	}
}

func TestPickerEnterSelectsItem(t *testing.T) {
	p := newTestPicker([]string{"alpha", "beta", "gamma"})
	p.Open()
	p, _ = p.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	p, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if p.Active() {
		t.Fatal("picker should close after Enter")
	}
	if cmd == nil {
		t.Fatal("Enter should produce a command")
	}
	msg := cmd()
	if msg != "beta" {
		t.Fatalf("expected 'beta', got %v", msg)
	}
}

func TestPickerEscapeCloses(t *testing.T) {
	p := newTestPicker([]string{"alpha", "beta"})
	p.Open()
	p, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if p.Active() {
		t.Fatal("picker should close after Escape")
	}
	if cmd != nil {
		t.Fatal("Escape should return nil cmd")
	}
}

func TestPickerFilterNarrowsList(t *testing.T) {
	p := newTestPicker([]string{"apple", "banana", "avocado"})
	p.Open()
	// Type "av" — should match only "avocado"
	p, _ = p.Update(tea.KeyPressMsg{Code: -1, Text: "a"})
	p, _ = p.Update(tea.KeyPressMsg{Code: -1, Text: "v"})
	filtered := p.Filtered()
	if len(filtered) != 1 {
		t.Fatalf("expected 1 filtered item, got %d", len(filtered))
	}
	if filtered[0] != "avocado" {
		t.Fatalf("expected 'avocado', got %q", filtered[0])
	}
}

func TestPickerArrowNavigation(t *testing.T) {
	p := newTestPicker([]string{"a", "b", "c"})
	p.Open()
	if p.Cursor() != 0 {
		t.Fatalf("initial cursor should be 0, got %d", p.Cursor())
	}
	p, _ = p.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if p.Cursor() != 1 {
		t.Fatalf("cursor should be 1 after down, got %d", p.Cursor())
	}
	p, _ = p.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if p.Cursor() != 0 {
		t.Fatalf("cursor should be 0 after up, got %d", p.Cursor())
	}
	p, _ = p.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if p.Cursor() != 0 {
		t.Fatalf("cursor should clamp at 0, got %d", p.Cursor())
	}
}

func TestPickerEnterOnEmptyListClosesWithNil(t *testing.T) {
	p := newTestPicker([]string{})
	p.Open()
	p, cmd := p.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if p.Active() {
		t.Fatal("picker should close on Enter even with empty list")
	}
	if cmd != nil {
		t.Fatal("Enter on empty list should return nil cmd")
	}
}

func TestPickerScrollWheelDownAdvancesCursor(t *testing.T) {
	p := newTestPicker([]string{"a", "b", "c"})
	p.Open()
	if p.Cursor() != 0 {
		t.Fatalf("initial cursor should be 0, got %d", p.Cursor())
	}
	p.ScrollWheel(tea.MouseWheelDown)
	if p.Cursor() != 1 {
		t.Fatalf("cursor should be 1 after wheel down, got %d", p.Cursor())
	}
	p.ScrollWheel(tea.MouseWheelDown)
	if p.Cursor() != 2 {
		t.Fatalf("cursor should be 2 after second wheel down, got %d", p.Cursor())
	}
}

func TestPickerScrollWheelDownAtLastStays(t *testing.T) {
	p := newTestPicker([]string{"a", "b", "c"})
	p.Open()
	p.ScrollWheel(tea.MouseWheelDown)
	p.ScrollWheel(tea.MouseWheelDown)
	if p.Cursor() != 2 {
		t.Fatalf("cursor should be 2 after two wheel downs, got %d", p.Cursor())
	}
	p.ScrollWheel(tea.MouseWheelDown)
	if p.Cursor() != 2 {
		t.Fatalf("cursor should clamp at 2 (last), got %d", p.Cursor())
	}
}

func TestPickerScrollWheelUpAtFirstStays(t *testing.T) {
	p := newTestPicker([]string{"a", "b", "c"})
	p.Open()
	if p.Cursor() != 0 {
		t.Fatalf("initial cursor should be 0, got %d", p.Cursor())
	}
	p.ScrollWheel(tea.MouseWheelUp)
	if p.Cursor() != 0 {
		t.Fatalf("cursor should clamp at 0, got %d", p.Cursor())
	}
}

func TestPickerScrollWheelUpMovesCursorBack(t *testing.T) {
	p := newTestPicker([]string{"a", "b", "c"})
	p.Open()
	p.ScrollWheel(tea.MouseWheelDown)
	p.ScrollWheel(tea.MouseWheelDown)
	if p.Cursor() != 2 {
		t.Fatalf("setup: expected cursor 2, got %d", p.Cursor())
	}
	p.ScrollWheel(tea.MouseWheelUp)
	if p.Cursor() != 1 {
		t.Fatalf("cursor should be 1 after wheel up, got %d", p.Cursor())
	}
}

func TestPickerScrollWheelLeftRightNoOp(t *testing.T) {
	p := newTestPicker([]string{"a", "b", "c"})
	p.Open()
	p.ScrollWheel(tea.MouseWheelDown)
	if p.Cursor() != 1 {
		t.Fatalf("setup: expected cursor 1, got %d", p.Cursor())
	}
	p.ScrollWheel(tea.MouseWheelLeft)
	if p.Cursor() != 1 {
		t.Fatalf("wheel left should not move cursor, got %d", p.Cursor())
	}
	p.ScrollWheel(tea.MouseWheelRight)
	if p.Cursor() != 1 {
		t.Fatalf("wheel right should not move cursor, got %d", p.Cursor())
	}
}

// TestPickerScrollWheelOnInactivePickerDoesNothing verifies that wheel events
// on a picker that was never opened (or was closed) do not panic and do not
// move the cursor off its initial 0 value. Defensive: the app dispatcher
// should never route wheel events to an inactive overlay, but we guarantee
// the component tolerates it regardless.
func TestPickerScrollWheelOnInactivePickerDoesNothing(t *testing.T) {
	p := newTestPicker([]string{"a", "b", "c"})
	// Picker is NOT opened.
	if p.Active() {
		t.Fatal("precondition: picker should be inactive")
	}
	before := p.Cursor()
	p.ScrollWheel(tea.MouseWheelDown)
	p.ScrollWheel(tea.MouseWheelUp)
	if p.Cursor() != before {
		t.Fatalf("cursor moved on inactive picker: before=%d after=%d", before, p.Cursor())
	}
}

func TestPickerFilterResetsSelection(t *testing.T) {
	p := newTestPicker([]string{"alpha", "beta", "gamma"})
	p.Open()
	p, _ = p.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	p, _ = p.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if p.Cursor() != 2 {
		t.Fatalf("expected cursor at 2, got %d", p.Cursor())
	}
	p, _ = p.Update(tea.KeyPressMsg{Code: -1, Text: "a"})
	if p.Cursor() != 0 {
		t.Fatalf("cursor should reset to 0 after filter, got %d", p.Cursor())
	}
}
