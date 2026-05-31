package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// lastLine returns the final rendered line of a multi-line box view.
func lastLine(view string) string {
	lines := strings.Split(view, "\n")
	return lines[len(lines)-1]
}

// TestResourceListFooterShown verifies that a non-empty context footer is drawn
// on the pane's bottom border and contains the context name.
func TestResourceListFooterShown(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 40, 10)
	rl.SetObjects([]*unstructured.Unstructured{makeObj("pod-a")})
	rl.SetContextFooter("staging")

	out := rl.View()
	bottom := ansi.Strip(lastLine(out))
	if !strings.Contains(bottom, "staging") {
		t.Fatalf("expected bottom border to contain context name, got %q", bottom)
	}
}

// TestResourceListFooterHidden verifies that an empty footer leaves the bottom
// border as a plain border with no context name.
func TestResourceListFooterHidden(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 40, 10)
	rl.SetObjects([]*unstructured.Unstructured{makeObj("pod-a")})
	rl.SetContextFooter("")

	out := rl.View()
	bottom := ansi.Strip(lastLine(out))
	if strings.Contains(bottom, "staging") {
		t.Fatalf("expected no footer text on bottom border, got %q", bottom)
	}
	// A plain bottom border carries no letters at all.
	for _, r := range bottom {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			t.Fatalf("expected plain border with no letters, got %q", bottom)
		}
	}
}

// TestResourceListFooterDoesNotChangeHeight verifies the footer reuses the
// bottom border line: the rendered view has the same number of lines with or
// without a footer (no content row consumed, no table-height change).
func TestResourceListFooterDoesNotChangeHeight(t *testing.T) {
	objs := []*unstructured.Unstructured{makeObj("pod-a"), makeObj("pod-b")}

	noFooter := NewResourceList(&testPlugin{}, 40, 10)
	noFooter.SetObjects(objs)
	withFooter := NewResourceList(&testPlugin{}, 40, 10)
	withFooter.SetObjects(objs)
	withFooter.SetContextFooter("staging")

	gotNo := strings.Count(noFooter.View(), "\n")
	gotWith := strings.Count(withFooter.View(), "\n")
	if gotNo != gotWith {
		t.Fatalf("footer changed line count: without=%d with=%d", gotNo, gotWith)
	}
}

// TestResourceListFooterWidthDoesNotOverflow verifies that a very long context
// name does not break the box: every rendered line keeps the same width and the
// line count is unchanged (the footer is skipped/truncated gracefully when it
// cannot fit, mirroring injectBorderTitle's width guard).
func TestResourceListFooterWidthDoesNotOverflow(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 40, 10)
	rl.SetObjects([]*unstructured.Unstructured{makeObj("pod-a")})
	rl.SetContextFooter(strings.Repeat("very-long-context-name", 10))

	out := rl.View()
	lines := strings.Split(out, "\n")
	if len(lines) < 2 {
		t.Fatalf("expected a multi-line box, got %d lines", len(lines))
	}
	want := lipgloss.Width(lines[0])
	for i, ln := range lines {
		if w := lipgloss.Width(ln); w != want {
			t.Fatalf("line %d width %d != expected box width %d (footer overflowed)", i, w, want)
		}
	}
}

// TestInjectBorderFooterMirrorsTitle is a focused unit test on the helper: it
// rewrites the last line of a box with the footer and leaves the first line
// (title border) untouched, keeping the box width consistent.
func TestInjectBorderFooterMirrorsTitle(t *testing.T) {
	box := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		Width(30).Height(4).
		Render("content")

	footer := TitleIndicatorStyle.Render(" prod ")
	out := injectBorderFooter(box, footer, true)

	origLines := strings.Split(box, "\n")
	outLines := strings.Split(out, "\n")
	if len(origLines) != len(outLines) {
		t.Fatalf("injectBorderFooter changed line count: %d -> %d", len(origLines), len(outLines))
	}
	if got := lipgloss.Width(outLines[len(outLines)-1]); got != lipgloss.Width(origLines[0]) {
		t.Fatalf("footer line width %d != box width %d", got, lipgloss.Width(origLines[0]))
	}
	if !strings.Contains(ansi.Strip(outLines[len(outLines)-1]), "prod") {
		t.Fatalf("expected footer text on last line, got %q", ansi.Strip(outLines[len(outLines)-1]))
	}
}
