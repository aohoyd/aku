package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"
)

func TestOverlayLifecycle(t *testing.T) {
	o := NewOverlay()
	if o.Active() {
		t.Fatal("new overlay should be inactive")
	}
	o.SetActive(true)
	if !o.Active() {
		t.Fatal("overlay should be active after SetActive(true)")
	}
	o.Reset()
	if o.Active() {
		t.Fatal("overlay should be inactive after Reset")
	}
}

func TestOverlayConfiguration(t *testing.T) {
	o := NewOverlay()
	o.SetTitle("Test Title")
	o.SetFooter("Press Enter")
	o.SetFixedWidth(40)
	o.SetSize(80, 24)
	o.SetMaxVisible(10)
	o.SetNoItemsMsg("(empty)")
	if o.title != "Test Title" {
		t.Fatalf("expected title 'Test Title', got %q", o.title)
	}
	if o.footer != "Press Enter" {
		t.Fatalf("expected footer 'Press Enter', got %q", o.footer)
	}
}

func TestOverlaySetItemsAndCursor(t *testing.T) {
	o := NewOverlay()
	o.SetItems([]string{"alpha", "beta", "gamma"})
	if o.Cursor() != 0 {
		t.Fatalf("initial cursor should be 0, got %d", o.Cursor())
	}
	if o.SelectedItem() != "alpha" {
		t.Fatalf("expected 'alpha', got %q", o.SelectedItem())
	}
	o.SetCursor(2)
	if o.SelectedItem() != "gamma" {
		t.Fatalf("expected 'gamma', got %q", o.SelectedItem())
	}
}

func TestOverlayCursorClamp(t *testing.T) {
	o := NewOverlay()
	o.SetItems([]string{"a", "b", "c"})
	o.SetCursor(10)
	if o.Cursor() != 2 {
		t.Fatalf("cursor should clamp to 2, got %d", o.Cursor())
	}
	o.SetCursor(-5)
	if o.Cursor() != 0 {
		t.Fatalf("cursor should clamp to 0, got %d", o.Cursor())
	}
}

func TestOverlayEmptyItemsSelectedItem(t *testing.T) {
	o := NewOverlay()
	if o.SelectedItem() != "" {
		t.Fatalf("expected empty string for no items, got %q", o.SelectedItem())
	}
}

func TestOverlayHandleListKeys(t *testing.T) {
	o := NewOverlay()
	o.SetItems([]string{"a", "b", "c"})

	handled := o.HandleListKeys(tea.KeyPressMsg{Code: tea.KeyDown})
	if !handled {
		t.Fatal("Down should be handled")
	}
	if o.Cursor() != 1 {
		t.Fatalf("cursor should be 1 after Down, got %d", o.Cursor())
	}

	handled = o.HandleListKeys(tea.KeyPressMsg{Code: tea.KeyUp})
	if !handled {
		t.Fatal("Up should be handled")
	}
	if o.Cursor() != 0 {
		t.Fatalf("cursor should be 0 after Up, got %d", o.Cursor())
	}

	handled = o.HandleListKeys(tea.KeyPressMsg{Code: tea.KeyEnter})
	if handled {
		t.Fatal("Enter should not be handled by HandleListKeys")
	}
}

func TestOverlayViewportScrolling(t *testing.T) {
	o := NewOverlay()
	o.SetMaxVisible(3)
	o.SetItems([]string{"a", "b", "c", "d", "e"})

	for range 4 {
		o.HandleListKeys(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	if o.Cursor() != 4 {
		t.Fatalf("cursor should be 4, got %d", o.Cursor())
	}
	if o.offset == 0 {
		t.Fatal("offset should have scrolled from 0")
	}
	if o.Cursor() < o.offset || o.Cursor() >= o.offset+3 {
		t.Fatalf("cursor %d should be within viewport [%d, %d)", o.Cursor(), o.offset, o.offset+3)
	}
}

func TestOverlayAddInput(t *testing.T) {
	o := NewOverlay()
	ti := textinput.New()
	ti.Prompt = "Filter: "
	idx := o.AddInput(ti)
	if idx != 0 {
		t.Fatalf("first input index should be 0, got %d", idx)
	}
	if o.InputCount() != 1 {
		t.Fatalf("expected 1 input, got %d", o.InputCount())
	}
	if o.InputValue(0) != "" {
		t.Fatalf("expected empty value, got %q", o.InputValue(0))
	}
}

func TestOverlayMultiInput(t *testing.T) {
	o := NewOverlay()
	ti1 := textinput.New()
	ti1.Prompt = "Local: "
	ti2 := textinput.New()
	ti2.Prompt = "Remote: "
	o.AddInput(ti1)
	o.AddInput(ti2)
	if o.InputCount() != 2 {
		t.Fatalf("expected 2 inputs, got %d", o.InputCount())
	}
}

func TestOverlayFocusInput(t *testing.T) {
	o := NewOverlay()
	ti1 := textinput.New()
	ti2 := textinput.New()
	o.AddInput(ti1)
	o.AddInput(ti2)
	o.FocusInput(0)
	if o.focusIdx != 0 {
		t.Fatalf("expected focusIdx 0, got %d", o.focusIdx)
	}
	o.FocusInput(1)
	if o.focusIdx != 1 {
		t.Fatalf("expected focusIdx 1, got %d", o.focusIdx)
	}
}

func TestOverlayFocusCycling(t *testing.T) {
	o := NewOverlay()
	ti1 := textinput.New()
	ti2 := textinput.New()
	o.AddInput(ti1)
	o.AddInput(ti2)
	o.SetItems([]string{"item1"})
	o.FocusInput(0)

	// Tab: 0 -> 1 -> list(-1) -> 0
	o.FocusNextInput()
	if o.focusIdx != 1 {
		t.Fatalf("expected focusIdx 1, got %d", o.focusIdx)
	}
	o.FocusNextInput()
	if o.focusIdx != -1 {
		t.Fatalf("expected focusIdx -1 (list), got %d", o.focusIdx)
	}
	o.FocusNextInput()
	if o.focusIdx != 0 {
		t.Fatalf("expected focusIdx 0 (wrap), got %d", o.focusIdx)
	}

	// Shift-Tab: 0 -> list(-1) -> 1 -> 0
	o.FocusPrevInput()
	if o.focusIdx != -1 {
		t.Fatalf("expected focusIdx -1, got %d", o.focusIdx)
	}
	o.FocusPrevInput()
	if o.focusIdx != 1 {
		t.Fatalf("expected focusIdx 1, got %d", o.focusIdx)
	}
}

func TestOverlayUpdateInputs(t *testing.T) {
	o := NewOverlay()
	ti := textinput.New()
	ti.Prompt = "> "
	ti.Focus()
	o.AddInput(ti)
	o.FocusInput(0)

	o.UpdateInputs(tea.KeyPressMsg{Code: -1, Text: "a"})
	if o.InputValue(0) != "a" {
		t.Fatalf("expected 'a', got %q", o.InputValue(0))
	}
}

func TestOverlayViewInactive(t *testing.T) {
	o := NewOverlay()
	if o.View() != "" {
		t.Fatal("inactive overlay should render empty string")
	}
}

func TestOverlayViewTitleOnly(t *testing.T) {
	o := NewOverlay()
	o.SetActive(true)
	o.SetTitle("My Title")
	o.SetSize(80, 24)
	view := o.View()
	if !strings.Contains(view, "My Title") {
		t.Fatal("view should contain title")
	}
}

func TestOverlayViewFooterOnly(t *testing.T) {
	o := NewOverlay()
	o.SetActive(true)
	o.SetFooter("[y/N]")
	o.SetSize(80, 24)
	view := o.View()
	if !strings.Contains(view, "[y/N]") {
		t.Fatal("view should contain footer")
	}
}

func TestOverlayViewWithItems(t *testing.T) {
	o := NewOverlay()
	o.SetActive(true)
	o.SetTitle("Pick One")
	o.SetSize(80, 24)
	o.SetItems([]string{"alpha", "beta", "gamma"})
	view := o.View()
	if !strings.Contains(view, "alpha") {
		t.Fatal("view should contain first item")
	}
	if !strings.Contains(view, "beta") {
		t.Fatal("view should contain second item")
	}
}

func TestOverlayViewCursorHighlight(t *testing.T) {
	o := NewOverlay()
	o.SetActive(true)
	o.SetSize(80, 24)
	o.SetItems([]string{"alpha", "beta"})
	view := o.View()
	if !strings.Contains(view, "> alpha") {
		t.Fatalf("view should show cursor on alpha, got:\n%s", view)
	}
}

func TestOverlayViewStableHeight(t *testing.T) {
	o := NewOverlay()
	o.SetActive(true)
	o.SetTitle("Test")
	o.SetSize(80, 24)
	o.SetMaxVisible(5)
	o.SetItems([]string{"a", "b", "c", "d", "e"})
	fullView := o.View()
	fullLines := strings.Count(fullView, "\n")

	o.SetItems([]string{"a"})
	reducedView := o.View()
	reducedLines := strings.Count(reducedView, "\n")

	if fullLines != reducedLines {
		t.Fatalf("height should be stable: full=%d, reduced=%d", fullLines, reducedLines)
	}
}

func TestOverlayViewNoItemsMsg(t *testing.T) {
	o := NewOverlay()
	o.SetActive(true)
	o.SetSize(80, 24)
	o.SetMaxVisible(5)
	o.SetNoItemsMsg("(no matches)")
	o.SetItems([]string{})
	view := o.View()
	if !strings.Contains(view, "(no matches)") {
		t.Fatalf("view should contain no-items message, got:\n%s", view)
	}
}

func TestOverlayViewOverflowIndicator(t *testing.T) {
	o := NewOverlay()
	o.SetActive(true)
	o.SetSize(80, 24)
	o.SetMaxVisible(2)
	o.SetItems([]string{"a", "b", "c", "d"})
	view := o.View()
	if !strings.Contains(view, "... and") {
		t.Fatalf("view should show overflow indicator, got:\n%s", view)
	}
}

func TestOverlayViewContentAdaptiveWidth(t *testing.T) {
	o := NewOverlay()
	o.SetActive(true)
	o.SetSize(80, 24)
	o.SetTitle("Short")
	view := o.View()
	lines := strings.Split(view, "\n")
	if len(lines) > 0 {
		w := lipgloss.Width(lines[0])
		if w >= 78 {
			t.Fatalf("expected adaptive width less than terminal, got %d", w)
		}
	}
}

func TestOverlayPostListInputRendering(t *testing.T) {
	o := NewOverlay()
	o.SetActive(true)
	o.SetSize(80, 24)
	o.SetTitle("Test")

	ti1 := textinput.New()
	ti1.Prompt = "Filter: "
	ti1.Focus()
	o.AddInput(ti1)

	ti2 := textinput.New()
	ti2.Prompt = "Local: "
	o.AddInput(ti2)

	o.SetPostListInputIdx(1)
	o.SetItems([]string{"item-alpha", "item-beta"})

	view := o.View()

	// Filter input should appear before list items
	filterPos := strings.Index(view, "Filter:")
	itemPos := strings.Index(view, "item-alpha")
	localPos := strings.Index(view, "Local:")

	if filterPos == -1 || itemPos == -1 || localPos == -1 {
		t.Fatalf("expected all three sections in view, got:\n%s", view)
	}
	if filterPos >= itemPos {
		t.Fatalf("filter input should appear before list items: filter=%d, item=%d", filterPos, itemPos)
	}
	if itemPos >= localPos {
		t.Fatalf("list items should appear before local input: item=%d, local=%d", itemPos, localPos)
	}
}

func TestOverlayFocusedInput(t *testing.T) {
	o := NewOverlay()
	if o.FocusedInput() != -1 {
		t.Fatalf("expected -1, got %d", o.FocusedInput())
	}
	ti := textinput.New()
	o.AddInput(ti)
	o.FocusInput(0)
	if o.FocusedInput() != 0 {
		t.Fatalf("expected 0, got %d", o.FocusedInput())
	}
}

func TestOverlayChromeMatchesStyle(t *testing.T) {
	c := overlayChrome()
	// OverlayStyle has RoundedBorder (1+1) + Padding(1, 2) means vertical=1, horizontal=2 → (2+2)
	// Total horizontal chrome = 6
	if c != 6 {
		t.Fatalf("expected chrome=6 for current OverlayStyle, got %d", c)
	}
}

func TestOverlayInputWidthConstrainedFixedWidth(t *testing.T) {
	o := NewOverlay()
	o.SetActive(true)
	o.SetFixedWidth(40)
	o.SetSize(80, 24)

	ti := textinput.New()
	ti.Prompt = "Image: "
	ti.Focus()
	o.AddInput(ti)

	// Before View(), width is unconstrained (0)
	if o.inputs[0].Width() != 0 {
		t.Fatalf("expected initial width 0, got %d", o.inputs[0].Width())
	}

	_ = o.View()

	// After View(), width should be contentWidth - promptWidth - 1 (cursor column)
	// fixedWidth=40, chrome=6, content=34, prompt="Image: " (7 chars), cursor=1
	chrome := overlayChrome()
	expectedInputW := (40 - chrome) - lipgloss.Width("Image: ") - 1
	if o.inputs[0].Width() != expectedInputW {
		t.Fatalf("expected input width %d, got %d", expectedInputW, o.inputs[0].Width())
	}
}

func TestOverlayInputWidthConstrainedAutoWidth(t *testing.T) {
	o := NewOverlay()
	o.SetActive(true)
	o.SetSize(80, 24)
	o.SetTitle("Pick")

	ti := textinput.New()
	ti.Prompt = "> "
	ti.Focus()
	o.AddInput(ti)

	_ = o.View()

	w := o.inputs[0].Width()
	if w <= 0 {
		t.Fatalf("expected positive input width, got %d", w)
	}
	if w >= 80 {
		t.Fatalf("expected input width less than terminal width, got %d", w)
	}
}

func TestOverlayInputWidthMinimumGuard(t *testing.T) {
	o := NewOverlay()
	o.SetActive(true)
	o.SetFixedWidth(12) // very narrow: content = 12 - 6 = 6
	o.SetSize(80, 24)

	ti := textinput.New()
	ti.Prompt = "Very long prompt: " // 18 chars, wider than content area
	ti.Focus()
	o.AddInput(ti)

	// Should not panic
	_ = o.View()

	w := o.inputs[0].Width()
	if w < 1 {
		t.Fatalf("expected minimum width of 1, got %d", w)
	}
}

func TestOverlayBorderTitleCentered(t *testing.T) {
	o := NewOverlay()
	o.SetActive(true)
	o.SetTitle("Test Title")
	o.SetSize(80, 24)
	view := o.View()
	lines := strings.Split(view, "\n")
	// Title should be in the top border line (line 0), not in the body
	if !strings.Contains(lines[0], "Test Title") {
		t.Fatalf("title should appear in top border line, got:\n%s", lines[0])
	}
}

func TestOverlayEmptyTitleNoBorderTitle(t *testing.T) {
	o := NewOverlay()
	o.SetActive(true)
	o.SetSize(80, 24)
	o.SetItems([]string{"a", "b"})
	view := o.View()
	lines := strings.Split(view, "\n")
	// With no title, the top border should be plain (just border chars)
	if strings.Contains(lines[0], "Title") {
		t.Fatal("empty title should produce plain border")
	}
}

func TestOverlayContentField(t *testing.T) {
	o := NewOverlay()
	o.SetActive(true)
	o.SetContent("Some message")
	o.SetSize(80, 24)
	view := o.View()
	if !strings.Contains(view, "Some message") {
		t.Fatal("view should contain content text")
	}
	// Content should NOT be in the border line
	lines := strings.Split(view, "\n")
	if strings.Contains(lines[0], "Some message") {
		t.Fatal("content should not appear in border line")
	}
}

func TestOverlayPostListInputIdxReset(t *testing.T) {
	o := NewOverlay()
	o.SetPostListInputIdx(1)
	o.Reset()
	// After reset, postListInputIdx should be 0 (default)
	o.SetActive(true)
	o.SetSize(80, 24)
	ti1 := textinput.New()
	ti1.Prompt = "A: "
	ti2 := textinput.New()
	ti2.Prompt = "B: "
	o.AddInput(ti1)
	o.AddInput(ti2)
	o.SetItems([]string{"x"})

	view := o.View()
	aPos := strings.Index(view, "A:")
	bPos := strings.Index(view, "B:")
	xPos := strings.Index(view, "x")
	// Both inputs should appear before the list item (default behavior)
	if aPos >= xPos || bPos >= xPos {
		t.Fatalf("after reset, all inputs should render before list: a=%d, b=%d, x=%d", aPos, bPos, xPos)
	}
}
