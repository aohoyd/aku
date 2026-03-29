package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/aohoyd/aku/internal/msgs"
)

func TestContainerPickerOpen(t *testing.T) {
	cp := NewContainerPicker(80, 30)
	cp.SetContainers([]string{"nginx", "sidecar", "init"})
	cp.Open()
	if !cp.Active() {
		t.Fatal("expected picker to be active after Open")
	}
}

func TestContainerPickerInactiveByDefault(t *testing.T) {
	cp := NewContainerPicker(80, 30)
	if cp.Active() {
		t.Fatal("expected picker to be inactive by default")
	}
}

func TestContainerPickerEscapeDismisses(t *testing.T) {
	cp := NewContainerPicker(80, 30)
	cp.SetContainers([]string{"nginx", "sidecar"})
	cp.Open()
	cp, _ = cp.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if cp.Active() {
		t.Fatal("expected picker to be inactive after Escape")
	}
}

func TestContainerPickerEscapeReturnsNilCmd(t *testing.T) {
	cp := NewContainerPicker(80, 30)
	cp.SetContainers([]string{"nginx", "sidecar"})
	cp.Open()
	_, cmd := cp.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd != nil {
		t.Fatal("expected nil command from Escape")
	}
}

func TestContainerPickerEnterSelectsFirstItem(t *testing.T) {
	cp := NewContainerPicker(80, 30)
	cp.SetContainers([]string{"nginx", "sidecar", "init"})
	cp.Open()

	cp, cmd := cp.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cp.Active() {
		t.Fatal("expected picker to close after Enter")
	}
	if cmd == nil {
		t.Fatal("Enter should produce a command")
	}
	msg := cmd()
	cMsg, ok := msg.(msgs.LogContainerSelectedMsg)
	if !ok {
		t.Fatalf("expected LogContainerSelectedMsg, got %T", msg)
	}
	if cMsg.Container != "nginx" {
		t.Fatalf("expected container 'nginx', got %q", cMsg.Container)
	}
}

func TestContainerPickerNavigateAndSelect(t *testing.T) {
	cp := NewContainerPicker(80, 30)
	cp.SetContainers([]string{"nginx", "sidecar", "init"})
	cp.Open()

	// Navigate down to "sidecar" (index 1)
	cp, _ = cp.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	cp, cmd := cp.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cp.Active() {
		t.Fatal("expected picker to close after Enter")
	}
	if cmd == nil {
		t.Fatal("Enter should produce a command")
	}
	msg := cmd()
	cMsg, ok := msg.(msgs.LogContainerSelectedMsg)
	if !ok {
		t.Fatalf("expected LogContainerSelectedMsg, got %T", msg)
	}
	if cMsg.Container != "sidecar" {
		t.Fatalf("expected container 'sidecar', got %q", cMsg.Container)
	}
}

func TestContainerPickerView(t *testing.T) {
	cp := NewContainerPicker(80, 30)
	cp.SetContainers([]string{"nginx", "sidecar"})
	cp.Open()
	view := cp.View()
	if view == "" {
		t.Fatal("expected non-empty view")
	}
}

func TestContainerPickerFilter(t *testing.T) {
	cp := NewContainerPicker(80, 30)
	cp.SetContainers([]string{"nginx", "sidecar", "init-container"})
	cp.Open()

	// Type "side" to filter
	cp, _ = cp.Update(tea.KeyPressMsg{Code: -1, Text: "s"})
	cp, _ = cp.Update(tea.KeyPressMsg{Code: -1, Text: "i"})
	cp, _ = cp.Update(tea.KeyPressMsg{Code: -1, Text: "d"})
	cp, _ = cp.Update(tea.KeyPressMsg{Code: -1, Text: "e"})

	filtered := cp.Filtered()
	if len(filtered) != 1 {
		t.Fatalf("expected 1 filtered item, got %d", len(filtered))
	}
	if filtered[0] != "sidecar" {
		t.Fatalf("expected 'sidecar', got %q", filtered[0])
	}
}

func TestContainerPickerEnterOnEmptyList(t *testing.T) {
	cp := NewContainerPicker(80, 30)
	cp.SetContainers([]string{})
	cp.Open()

	cp, cmd := cp.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cp.Active() {
		t.Fatal("expected picker to close on Enter with empty list")
	}
	if cmd != nil {
		t.Fatal("Enter on empty list should return nil cmd")
	}
}

func TestContainerPickerFilterThenSelect(t *testing.T) {
	cp := NewContainerPicker(80, 30)
	cp.SetContainers([]string{"nginx", "sidecar", "init-container"})
	cp.Open()

	// Type "init" to filter
	cp, _ = cp.Update(tea.KeyPressMsg{Code: -1, Text: "i"})
	cp, _ = cp.Update(tea.KeyPressMsg{Code: -1, Text: "n"})
	cp, _ = cp.Update(tea.KeyPressMsg{Code: -1, Text: "i"})
	cp, _ = cp.Update(tea.KeyPressMsg{Code: -1, Text: "t"})

	filtered := cp.Filtered()
	if len(filtered) != 1 {
		t.Fatalf("expected 1 filtered item, got %d", len(filtered))
	}

	cp, cmd := cp.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cp.Active() {
		t.Fatal("expected picker to close after Enter")
	}
	if cmd == nil {
		t.Fatal("Enter should produce a command")
	}
	msg := cmd()
	cMsg, ok := msg.(msgs.LogContainerSelectedMsg)
	if !ok {
		t.Fatalf("expected LogContainerSelectedMsg, got %T", msg)
	}
	if cMsg.Container != "init-container" {
		t.Fatalf("expected container 'init-container', got %q", cMsg.Container)
	}
}
