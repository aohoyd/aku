package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/aohoyd/aku/internal/msgs"
)

func TestContextPickerOpenShowsItems(t *testing.T) {
	cp := NewContextPicker(40, 20)
	cp.SetContexts([]string{"prod", "staging", "dev"})
	cp.Open()
	if !cp.Active() {
		t.Fatal("context picker should be active after Open")
	}
	filtered := cp.Filtered()
	if len(filtered) != 3 {
		t.Fatalf("expected 3 items, got %d: %v", len(filtered), filtered)
	}
}

func TestContextPickerNoSentinel(t *testing.T) {
	cp := NewContextPicker(40, 20)
	cp.SetContexts([]string{"prod", "staging"})
	cp.Open()
	filtered := cp.Filtered()
	if len(filtered) != 2 {
		t.Fatalf("expected exactly 2 items (no sentinel), got %d: %v", len(filtered), filtered)
	}
	if filtered[0] != "prod" {
		t.Fatalf("expected first item 'prod', got %q", filtered[0])
	}
}

func TestContextPickerFilterNarrows(t *testing.T) {
	cp := NewContextPicker(40, 20)
	cp.SetContexts([]string{"prod-us", "prod-eu", "staging"})
	cp.Open()

	cp, _ = cp.Update(tea.KeyPressMsg{Code: -1, Text: "s"})
	cp, _ = cp.Update(tea.KeyPressMsg{Code: -1, Text: "t"})

	filtered := cp.Filtered()
	if len(filtered) != 1 {
		t.Fatalf("expected 1 filtered item, got %d: %v", len(filtered), filtered)
	}
	if filtered[0] != "staging" {
		t.Fatalf("expected 'staging', got %q", filtered[0])
	}
}

func TestContextPickerEscCancels(t *testing.T) {
	cp := NewContextPicker(40, 20)
	cp.SetContexts([]string{"prod", "staging"})
	cp.Open()
	updated, _ := cp.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if updated.Active() {
		t.Fatal("context picker should close after Esc")
	}
}

func TestContextPickerGlobalScopeEmitsGlobalMsg(t *testing.T) {
	cp := NewContextPicker(40, 20)
	cp.SetContexts([]string{"prod", "staging"})
	cp.SetScope(true)
	cp.Open()

	updated, cmd := cp.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if updated.Active() {
		t.Fatal("context picker should close after selection")
	}
	if cmd == nil {
		t.Fatal("Enter should produce a command")
	}
	msg := cmd()
	gm, ok := msg.(msgs.GlobalContextSelectedMsg)
	if !ok {
		t.Fatalf("expected GlobalContextSelectedMsg, got %T", msg)
	}
	if gm.Context != "prod" {
		t.Fatalf("expected context 'prod', got %q", gm.Context)
	}
}

func TestContextPickerPaneScopeEmitsPaneMsg(t *testing.T) {
	cp := NewContextPicker(40, 20)
	cp.SetContexts([]string{"prod", "staging"})
	cp.SetScope(false)
	cp.Open()

	updated, cmd := cp.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if updated.Active() {
		t.Fatal("context picker should close after selection")
	}
	if cmd == nil {
		t.Fatal("Enter should produce a command")
	}
	msg := cmd()
	pm, ok := msg.(msgs.PaneContextSelectedMsg)
	if !ok {
		t.Fatalf("expected PaneContextSelectedMsg, got %T", msg)
	}
	if pm.Context != "prod" {
		t.Fatalf("expected context 'prod', got %q", pm.Context)
	}
}

func TestContextPickerScopeSwitchableAcrossOpens(t *testing.T) {
	cp := NewContextPicker(40, 20)
	cp.SetContexts([]string{"prod"})

	// First open in global scope.
	cp.SetScope(true)
	cp.Open()
	_, cmd := cp.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if _, ok := cmd().(msgs.GlobalContextSelectedMsg); !ok {
		t.Fatalf("first selection should be global")
	}

	// Reopen in pane scope; the same picker must now emit a pane message.
	cp.SetScope(false)
	cp.Open()
	_, cmd = cp.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if _, ok := cmd().(msgs.PaneContextSelectedMsg); !ok {
		t.Fatalf("second selection should be pane-scoped")
	}
}

func TestContextPickerFixedHeight(t *testing.T) {
	cp := NewContextPicker(50, 20)
	cp.SetContexts([]string{"prod", "staging", "dev", "qa"})
	cp.Open()

	fullView := cp.View()
	fullLines := strings.Count(fullView, "\n")

	cp, _ = cp.Update(tea.KeyPressMsg{Code: -1, Text: "p"})
	cp, _ = cp.Update(tea.KeyPressMsg{Code: -1, Text: "r"})

	filteredView := cp.View()
	filteredLines := strings.Count(filteredView, "\n")

	if fullLines != filteredLines {
		t.Fatalf("picker height should be stable: full=%d lines, filtered=%d lines", fullLines, filteredLines)
	}
}
