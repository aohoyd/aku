package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// firstLine returns the top rendered line of a multi-line box view (the border
// line that carries the title and the context badge).
func firstLine(view string) string {
	return strings.Split(view, "\n")[0]
}

// TestResourceListContextBadgeShown verifies that a non-empty context label is
// drawn on the pane's TOP border and contains the context name.
func TestResourceListContextBadgeShown(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 40, 10)
	rl.SetObjects([]*unstructured.Unstructured{makeObj("pod-a")})
	rl.SetContextLabel("staging")

	top := ansi.Strip(firstLine(rl.View()))
	if !strings.Contains(top, "staging") {
		t.Fatalf("expected top border to contain context name, got %q", top)
	}
}

// TestResourceListContextBadgeHidden verifies that an empty label leaves the top
// border free of any context name.
func TestResourceListContextBadgeHidden(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 40, 10)
	rl.SetObjects([]*unstructured.Unstructured{makeObj("pod-a")})
	rl.SetContextLabel("")

	top := ansi.Strip(firstLine(rl.View()))
	if strings.Contains(top, "staging") {
		t.Fatalf("expected no context badge on top border, got %q", top)
	}
}

// TestResourceListContextBadgeColorReflectsOffline verifies the badge is colored
// with the offline (red) style when the pane is offline and the online (green)
// style otherwise. Both render the SAME name string through different styles, so
// the rendered ANSI substring is an unambiguous color check.
func TestResourceListContextBadgeColorReflectsOffline(t *testing.T) {
	online := NewResourceList(&testPlugin{}, 40, 10)
	online.SetObjects([]*unstructured.Unstructured{makeObj("pod-a")})
	online.SetContextLabel("staging")
	online.SetOffline(false)

	offline := NewResourceList(&testPlugin{}, 40, 10)
	offline.SetObjects([]*unstructured.Unstructured{makeObj("pod-a")})
	offline.SetContextLabel("staging")
	offline.SetOffline(true)

	wantOnline := PaneContextOnlineStyle.Render(" staging ")
	wantOffline := PaneContextOfflineStyle.Render(" staging ")

	if got := firstLine(online.View()); !strings.Contains(got, wantOnline) {
		t.Fatalf("online pane: top border missing online-styled badge")
	}
	if got := firstLine(offline.View()); !strings.Contains(got, wantOffline) {
		t.Fatalf("offline pane: top border missing offline-styled badge")
	}
	// The two styles must be visually distinct (different ANSI), or the offline
	// state would be invisible.
	if wantOnline == wantOffline {
		t.Fatalf("online and offline badge styles render identically")
	}
}

// TestResourceListContextBadgeDoesNotChangeHeight verifies the badge reuses the
// top border line: the rendered view has the same number of lines with or
// without a badge (no content row consumed, no table-height change).
func TestResourceListContextBadgeDoesNotChangeHeight(t *testing.T) {
	objs := []*unstructured.Unstructured{makeObj("pod-a"), makeObj("pod-b")}

	noBadge := NewResourceList(&testPlugin{}, 40, 10)
	noBadge.SetObjects(objs)
	withBadge := NewResourceList(&testPlugin{}, 40, 10)
	withBadge.SetObjects(objs)
	withBadge.SetContextLabel("staging")

	if gotNo, gotWith := strings.Count(noBadge.View(), "\n"), strings.Count(withBadge.View(), "\n"); gotNo != gotWith {
		t.Fatalf("badge changed line count: without=%d with=%d", gotNo, gotWith)
	}
}

// TestResourceListContextBadgeWidthDoesNotOverflow verifies that a very long
// context name does not break the box: every rendered line keeps the same width
// (the badge is dropped gracefully when it cannot fit, mirroring
// injectBorderTitle's width guard).
func TestResourceListContextBadgeWidthDoesNotOverflow(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 40, 10)
	rl.SetObjects([]*unstructured.Unstructured{makeObj("pod-a")})
	rl.SetContextLabel(strings.Repeat("very-long-context-name", 10))

	lines := strings.Split(rl.View(), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected a multi-line box, got %d lines", len(lines))
	}
	want := lipgloss.Width(lines[0])
	for i, ln := range lines {
		if w := lipgloss.Width(ln); w != want {
			t.Fatalf("line %d width %d != expected box width %d (badge overflowed)", i, w, want)
		}
	}
}

// TestContextBadgeNameTruncatedToCap verifies a long context name is capped to
// maxBadgeContext columns with an ellipsis before it reaches the badge.
func TestContextBadgeNameTruncatedToCap(t *testing.T) {
	long := strings.Repeat("ctx-", 20) // 80 cols, well over the cap
	got := truncateContext(long, maxBadgeContext)
	if w := ansi.StringWidth(got); w > maxBadgeContext {
		t.Fatalf("truncated name width %d exceeds cap %d", w, maxBadgeContext)
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("expected truncated name to end with ellipsis, got %q", got)
	}
}

// TestInjectBorderTitleRightSegment is a focused unit test on the helper: the
// right segment is placed before the right corner when it fits, dropped when the
// box is too narrow (the title is never truncated), and absent when "".
func TestInjectBorderTitleRightSegment(t *testing.T) {
	box := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		Width(40).Height(4).
		Render("content")

	title := TitleStyle.Render("pods (3)")
	right := PaneContextOnlineStyle.Render(" staging ")

	// Wide box: both title and right segment present, width unchanged.
	out := injectBorderTitle(box, title, right, true)
	top := ansi.Strip(firstLine(out))
	if !strings.Contains(top, "pods (3)") || !strings.Contains(top, "staging") {
		t.Fatalf("wide box: expected both title and right segment, got %q", top)
	}
	if lipgloss.Width(firstLine(out)) != lipgloss.Width(firstLine(box)) {
		t.Fatalf("wide box: top border width changed")
	}

	// No right segment: title only.
	out = injectBorderTitle(box, title, "", true)
	if top := ansi.Strip(firstLine(out)); strings.Contains(top, "staging") {
		t.Fatalf("empty right: expected no right segment, got %q", top)
	}

	// Narrow box: wide enough for the title (~10 cols padded) but not for the
	// title plus the ~9-col right segment, so the badge is dropped while the title
	// survives.
	narrow := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		Width(18).Height(4).
		Render("content")
	out = injectBorderTitle(narrow, title, right, true)
	top = ansi.Strip(firstLine(out))
	if strings.Contains(top, "staging") {
		t.Fatalf("narrow box: expected right segment dropped, got %q", top)
	}
	if !strings.Contains(top, "pods (3)") {
		t.Fatalf("narrow box: title should survive, got %q", top)
	}
	if lipgloss.Width(firstLine(out)) != lipgloss.Width(firstLine(narrow)) {
		t.Fatalf("narrow box: top border width changed")
	}
}
