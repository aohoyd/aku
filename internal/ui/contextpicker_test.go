package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/aohoyd/aku/internal/msgs"
	"github.com/charmbracelet/x/ansi"
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

func TestContextPickerEmitsGlobalMsg(t *testing.T) {
	cp := NewContextPicker(40, 20)
	cp.SetContexts([]string{"prod", "staging"})
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

func TestContextPickerAnnotationsMarkInUse(t *testing.T) {
	cp := NewContextPicker(60, 20)
	cp.SetContexts([]string{"ctxA", "ctxB", "ctxC"})
	cp.SetAnnotations(map[string]int{"ctxA": 2, "ctxB": 1}, "")

	// ctxA: in use by 2 panes.
	a := ansi.Strip(cp.DisplayOf("ctxA"))
	if !strings.HasPrefix(a, inUseMarker+" ctxA") {
		t.Fatalf("ctxA should start with in-use marker and name, got %q", a)
	}
	if !strings.Contains(a, "(2)") {
		t.Fatalf("ctxA should show pane count (2), got %q", a)
	}

	// ctxB: in use by 1 pane.
	b := ansi.Strip(cp.DisplayOf("ctxB"))
	if !strings.HasPrefix(b, inUseMarker+" ctxB") {
		t.Fatalf("ctxB should start with in-use marker and name, got %q", b)
	}
	if !strings.Contains(b, "(1)") {
		t.Fatalf("ctxB should show pane count (1), got %q", b)
	}

	// ctxC: not in use — no marker, no count.
	c := ansi.Strip(cp.DisplayOf("ctxC"))
	if strings.Contains(c, inUseMarker) {
		t.Fatalf("ctxC should not be marked in-use, got %q", c)
	}
	if strings.Contains(c, "(") {
		t.Fatalf("ctxC should not show a pane count, got %q", c)
	}
	if c != "ctxC" {
		t.Fatalf("ctxC should render as bare name, got %q", c)
	}
}

func TestContextPickerAnnotationsHighlightFocused(t *testing.T) {
	cp := NewContextPicker(60, 20)
	cp.SetContexts([]string{"ctxA", "ctxB"})
	cp.SetAnnotations(map[string]int{"ctxA": 1}, "ctxB")

	// The focused context's row must carry the focused style (raw output
	// differs from a plain render of the same name).
	focusedRaw := cp.DisplayOf("ctxB")
	if focusedRaw == ansi.Strip(focusedRaw) {
		t.Fatalf("focused context row should be styled, got unstyled %q", focusedRaw)
	}
	if !strings.Contains(focusedRaw, ContextFocusedStyle.Render("ctxB")) {
		t.Fatalf("focused context row should use ContextFocusedStyle, got %q", focusedRaw)
	}

	// A non-focused, non-in-use context renders as the bare name.
	cp2 := NewContextPicker(60, 20)
	cp2.SetContexts([]string{"ctxB"})
	cp2.SetAnnotations(nil, "other")
	if got := cp2.DisplayOf("ctxB"); got != "ctxB" {
		t.Fatalf("non-focused unmarked context should be bare name, got %q", got)
	}
}

func TestContextPickerAnnotationsSelectionValueIsBare(t *testing.T) {
	cp := NewContextPicker(60, 20)
	cp.SetContexts([]string{"ctxA", "ctxB"})
	cp.SetAnnotations(map[string]int{"ctxA": 3}, "ctxA")
	cp.Open()

	// Filtered values must remain bare context names regardless of annotations.
	filtered := cp.Filtered()
	if len(filtered) != 2 || filtered[0] != "ctxA" {
		t.Fatalf("filtered values should be bare names, got %v", filtered)
	}

	// Selecting the first (annotated, focused) row yields the bare name.
	_, cmd := cp.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter should produce a command")
	}
	gm, ok := cmd().(msgs.GlobalContextSelectedMsg)
	if !ok {
		t.Fatalf("expected GlobalContextSelectedMsg, got %T", cmd())
	}
	if gm.Context != "ctxA" {
		t.Fatalf("selected value should be bare context name, got %q", gm.Context)
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
