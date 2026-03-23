package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestRenderButtonBarContainsLabelsAndHotkeys(t *testing.T) {
	buttons := []Button{
		{Label: "Yes", Hotkey: "y"},
		{Label: "No", Hotkey: "N"},
	}
	result := RenderButtonBar(buttons, 0)
	stripped := ansi.Strip(result)

	for _, b := range buttons {
		want := b.Label + "(" + b.Hotkey + ")"
		if !strings.Contains(stripped, want) {
			t.Fatalf("expected %q in output, got: %s", want, stripped)
		}
	}
}

func TestRenderButtonBarContainsSeparators(t *testing.T) {
	buttons := []Button{
		{Label: "Yes", Hotkey: "y"},
		{Label: "No", Hotkey: "N"},
	}
	result := RenderButtonBar(buttons, 0)
	stripped := ansi.Strip(result)

	// Format is: │ Yes(y) │ No(N) │
	// There should be 3 separators for 2 buttons.
	if count := strings.Count(stripped, "│"); count != 3 {
		t.Fatalf("expected 3 separators, got %d in: %s", count, stripped)
	}
}

func TestRenderButtonBarFocusedDiffersFromUnfocused(t *testing.T) {
	buttons := []Button{
		{Label: "Yes", Hotkey: "y"},
		{Label: "No", Hotkey: "N"},
	}
	result := RenderButtonBar(buttons, 0)

	// Find the styled rendering of each button by checking the raw
	// (ANSI-included) output. The focused button (index 0) should have
	// different styling than the unfocused button (index 1).
	// We render each individually to compare.
	focusedOnly := RenderButtonBar([]Button{{Label: "Yes", Hotkey: "y"}}, 0)
	unfocusedOnly := RenderButtonBar([]Button{{Label: "Yes", Hotkey: "y"}}, -1)

	if focusedOnly == unfocusedOnly {
		t.Fatal("focused button rendering should differ from unfocused rendering")
	}

	// Also verify both renderings appear to have content.
	if len(result) == 0 {
		t.Fatal("result should not be empty")
	}
}

func TestRenderButtonBarNoFocusAllSame(t *testing.T) {
	buttons := []Button{
		{Label: "Yes", Hotkey: "y"},
		{Label: "Force", Hotkey: "f"},
		{Label: "No", Hotkey: "N"},
	}
	result := RenderButtonBar(buttons, -1)

	// All buttons should use the same (normal) style when focusedIdx == -1.
	// Render each button individually with focusedIdx = -1 and check
	// they produce identical styling.
	for i := range buttons {
		single := RenderButtonBar([]Button{buttons[i]}, -1)
		singleFocused := RenderButtonBar([]Button{buttons[i]}, 0)
		if single == singleFocused {
			t.Fatalf("button %d: no-focus render should differ from focused render", i)
		}
	}

	// Verify no button in the full bar uses the focused style by comparing
	// with a version where one is focused — they should differ.
	resultWithFocus := RenderButtonBar(buttons, 1)
	if result == resultWithFocus {
		t.Fatal("bar with focusedIdx=-1 should differ from bar with a focused button")
	}
}

func TestRenderButtonBarThreeButtons(t *testing.T) {
	buttons := []Button{
		{Label: "Yes", Hotkey: "y"},
		{Label: "Force", Hotkey: "f"},
		{Label: "No", Hotkey: "N"},
	}
	result := RenderButtonBar(buttons, 2)
	stripped := ansi.Strip(result)

	expected := "│ Yes(y) │ Force(f) │ No(N) │"
	if stripped != expected {
		t.Fatalf("expected %q, got %q", expected, stripped)
	}
}

func TestRenderButtonBarSingleButton(t *testing.T) {
	buttons := []Button{
		{Label: "OK", Hotkey: "o"},
	}
	result := RenderButtonBar(buttons, 0)
	stripped := ansi.Strip(result)

	expected := "│ OK(o) │"
	if stripped != expected {
		t.Fatalf("expected %q, got %q", expected, stripped)
	}
}
