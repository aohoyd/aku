package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/aohoyd/aku/internal/msgs"
)

func TestConfirmDialogRender(t *testing.T) {
	cd := NewConfirmDialog("Delete pods default/nginx?", 80)
	view := cd.View()
	if !strings.Contains(view, "nginx") {
		t.Fatal("confirm dialog should show the resource name")
	}
	if !strings.Contains(view, "Yes") || !strings.Contains(view, "Force") || !strings.Contains(view, "No") {
		t.Fatal("confirm dialog should show all three buttons")
	}
}

func TestConfirmDialogAutoWidth(t *testing.T) {
	short := NewConfirmDialog("Delete pods x?", 80)
	long := NewConfirmDialog("Delete pods very-long-namespace/very-long-pod-name-here?", 80)
	shortView := short.View()
	longView := long.View()
	if shortView == "" || longView == "" {
		t.Fatal("both dialogs should render non-empty views")
	}
}

func TestConfirmDialogYes(t *testing.T) {
	cd := NewConfirmDialog("Delete?", 60)
	_, cmd := cd.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if cmd == nil {
		t.Fatal("pressing 'y' should return a command")
	}
	result := cmd().(msgs.ConfirmResultMsg)
	if result.Action != msgs.ConfirmYes {
		t.Fatalf("expected ConfirmYes, got %v", result.Action)
	}
}

func TestConfirmDialogForce(t *testing.T) {
	cd := NewConfirmDialog("Delete?", 60)
	_, cmd := cd.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
	if cmd == nil {
		t.Fatal("pressing 'f' should return a command")
	}
	result := cmd().(msgs.ConfirmResultMsg)
	if result.Action != msgs.ConfirmForce {
		t.Fatalf("expected ConfirmForce, got %v", result.Action)
	}
}

func TestConfirmDialogNo(t *testing.T) {
	cd := NewConfirmDialog("Delete?", 60)
	_, cmd := cd.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	if cmd == nil {
		t.Fatal("pressing 'n' should return a command")
	}
	result := cmd().(msgs.ConfirmResultMsg)
	if result.Action != msgs.ConfirmCancel {
		t.Fatalf("expected ConfirmCancel, got %v", result.Action)
	}
}

func TestConfirmDialogEsc(t *testing.T) {
	cd := NewConfirmDialog("Delete?", 60)
	_, cmd := cd.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd == nil {
		t.Fatal("pressing Esc should return a command")
	}
	result := cmd().(msgs.ConfirmResultMsg)
	if result.Action != msgs.ConfirmCancel {
		t.Fatalf("expected ConfirmCancel, got %v", result.Action)
	}
}

func TestConfirmDialogDefaultFocusNo(t *testing.T) {
	cd := NewConfirmDialog("Delete?", 60)
	_, cmd := cd.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("pressing Enter should return a command")
	}
	result := cmd().(msgs.ConfirmResultMsg)
	if result.Action != msgs.ConfirmCancel {
		t.Fatalf("default Enter should cancel, got %v", result.Action)
	}
}

func TestConfirmDialogArrowNavigation(t *testing.T) {
	cd := NewConfirmDialog("Delete?", 60)
	// Default focus: No (index 2)
	// Press left -> Force (index 1)
	cd, _ = cd.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	_, cmd := cd.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter after left should return a command")
	}
	result := cmd().(msgs.ConfirmResultMsg)
	if result.Action != msgs.ConfirmForce {
		t.Fatalf("expected ConfirmForce after left+enter, got %v", result.Action)
	}
}

func TestConfirmDialogTabNavigation(t *testing.T) {
	cd := NewConfirmDialog("Delete?", 60)
	// Default focus: No (index 2)
	// Press tab -> wraps to Yes (index 0)
	cd, _ = cd.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	_, cmd := cd.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter after tab should return a command")
	}
	result := cmd().(msgs.ConfirmResultMsg)
	if result.Action != msgs.ConfirmYes {
		t.Fatalf("expected ConfirmYes after tab+enter, got %v", result.Action)
	}
}

func TestConfirmDialogWrapLeft(t *testing.T) {
	cd := NewConfirmDialog("Delete?", 60)
	// Default focus: No (index 2)
	// Press left 3 times -> wraps to No (index 2)
	cd, _ = cd.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	cd, _ = cd.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	cd, _ = cd.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	_, cmd := cd.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	result := cmd().(msgs.ConfirmResultMsg)
	if result.Action != msgs.ConfirmCancel {
		t.Fatalf("expected ConfirmCancel after wrapping left, got %v", result.Action)
	}
}

func TestConfirmDialogMessageInsideBox(t *testing.T) {
	cd := NewConfirmDialog("Delete pods?", 80)
	view := cd.View()
	lines := strings.Split(view, "\n")
	// Message should NOT be in the border line (line 0)
	if strings.Contains(lines[0], "Delete") {
		t.Fatal("confirm message should be inside the box, not in the border")
	}
	// Message should still be somewhere in the view
	if !strings.Contains(view, "Delete pods?") {
		t.Fatal("confirm message should appear in the view")
	}
}
