package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/aohoyd/aku/internal/msgs"
)

func TestTimeRangePickerOpen(t *testing.T) {
	tp := NewTimeRangePicker(80, 30)
	tp.OpenPresets()
	if !tp.Active() {
		t.Fatal("expected picker to be active after OpenPresets")
	}
}

func TestTimeRangePickerInactiveByDefault(t *testing.T) {
	tp := NewTimeRangePicker(80, 30)
	if tp.Active() {
		t.Fatal("expected picker to be inactive by default")
	}
}

func TestTimeRangePickerEscapeDismisses(t *testing.T) {
	tp := NewTimeRangePicker(80, 30)
	tp.OpenPresets()
	tp, _ = tp.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if tp.Active() {
		t.Fatal("expected picker to be inactive after Escape")
	}
}

func TestTimeRangePickerEscapeReturnsNilCmd(t *testing.T) {
	tp := NewTimeRangePicker(80, 30)
	tp.OpenPresets()
	_, cmd := tp.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd != nil {
		t.Fatal("expected nil command from Escape")
	}
}

func TestTimeRangePickerEnterSelectsFirstItem(t *testing.T) {
	tp := NewTimeRangePicker(80, 30)
	tp.OpenPresets()

	tp, cmd := tp.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if tp.Active() {
		t.Fatal("expected picker to close after Enter")
	}
	if cmd == nil {
		t.Fatal("Enter should produce a command")
	}
	msg := cmd()
	trMsg, ok := msg.(msgs.LogTimeRangeSelectedMsg)
	if !ok {
		t.Fatalf("expected LogTimeRangeSelectedMsg, got %T", msg)
	}
	// First preset is "tail 200" (Seconds == -1), which means SinceSeconds is nil
	if trMsg.SinceSeconds != nil {
		t.Fatalf("expected nil SinceSeconds for 'tail 200', got %d", *trMsg.SinceSeconds)
	}
	if trMsg.Label != "tail 200" {
		t.Fatalf("expected label 'tail 200', got %q", trMsg.Label)
	}
}

func TestTimeRangePickerNavigateAndSelect(t *testing.T) {
	tp := NewTimeRangePicker(80, 30)
	tp.OpenPresets()

	// Navigate down to "1m" (second item, index 1)
	tp, _ = tp.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	tp, cmd := tp.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if tp.Active() {
		t.Fatal("expected picker to close after Enter")
	}
	if cmd == nil {
		t.Fatal("Enter should produce a command")
	}
	msg := cmd()
	trMsg, ok := msg.(msgs.LogTimeRangeSelectedMsg)
	if !ok {
		t.Fatalf("expected LogTimeRangeSelectedMsg, got %T", msg)
	}
	if trMsg.SinceSeconds == nil {
		t.Fatal("expected non-nil SinceSeconds for '1m'")
	}
	if *trMsg.SinceSeconds != 60 {
		t.Fatalf("expected 60 seconds for '1m', got %d", *trMsg.SinceSeconds)
	}
	if trMsg.Label != "1m" {
		t.Fatalf("expected label '1m', got %q", trMsg.Label)
	}
}

func TestTimeRangePickerView(t *testing.T) {
	tp := NewTimeRangePicker(80, 30)
	tp.OpenPresets()
	view := tp.View()
	if view == "" {
		t.Fatal("expected non-empty view")
	}
}

func TestTimeRangePickerFilter(t *testing.T) {
	tp := NewTimeRangePicker(80, 30)
	tp.OpenPresets()

	// Type "1" to filter — should match "1m", "1h", "15m", "12h"
	tp, _ = tp.Update(tea.KeyPressMsg{Code: -1, Text: "1"})
	filtered := tp.Filtered()
	if len(filtered) == 0 {
		t.Fatal("expected at least one filtered result for '1'")
	}
	for _, item := range filtered {
		if item.Label[0] != '1' && item.Label != "tail 200" {
			// "12h", "15m", "1m", "1h" all start with 1; but filter checks contains
			// so this is a loose check
		}
	}
}

func TestTimeRangePickerSelectAllPreset(t *testing.T) {
	tp := NewTimeRangePicker(80, 30)
	tp.OpenPresets()

	// Navigate to the last item ("all", index 9)
	for i := 0; i < 9; i++ {
		tp, _ = tp.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	tp, cmd := tp.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter should produce a command")
	}
	msg := cmd()
	trMsg, ok := msg.(msgs.LogTimeRangeSelectedMsg)
	if !ok {
		t.Fatalf("expected LogTimeRangeSelectedMsg, got %T", msg)
	}
	// "all" has Seconds == 0, which is > 0 is false, so SinceSeconds should be nil
	if trMsg.SinceSeconds != nil {
		t.Fatalf("expected nil SinceSeconds for 'all', got %d", *trMsg.SinceSeconds)
	}
	if trMsg.Label != "all" {
		t.Fatalf("expected label 'all', got %q", trMsg.Label)
	}
}

func TestLookupTimePreset(t *testing.T) {
	sec, ok := LookupTimePreset("5m")
	if !ok {
		t.Fatal("expected '5m' to be a known preset")
	}
	if sec != 300 {
		t.Fatalf("expected 300 seconds for '5m', got %d", sec)
	}

	_, ok = LookupTimePreset("unknown")
	if ok {
		t.Fatal("expected 'unknown' to not be a known preset")
	}
}
