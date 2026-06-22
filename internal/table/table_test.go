package table

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
)

func TestEnsureCursorVisibleScrollsWhenOffScreen(t *testing.T) {
	m := New(
		WithColumns([]Column{{Title: "Name", Width: 20}}),
		WithRows(makeRows(30)),
		WithHeight(7), // viewport height ~6 after header
	)

	// Move cursor to row 25 via MoveDown (properly sets yOffset)
	m.MoveDown(25)
	if m.Cursor() != 25 {
		t.Fatalf("expected cursor at 25, got %d", m.Cursor())
	}

	// Simulate stale viewport by resetting yOffset to 0
	// (this is what happens after Focus/Blur/SetCursor don't manage yOffset)
	m.viewport.SetYOffset(0)

	// Now cursor should be off-screen. EnsureCursorVisible should fix it.
	m.EnsureCursorVisible()

	// Cursor must still be at 25
	if m.Cursor() != 25 {
		t.Fatalf("cursor should remain at 25, got %d", m.Cursor())
	}

	// Verify yOffset was adjusted to center the cursor
	cursorLine := m.cursor - m.start
	yOffset := m.viewport.YOffset()
	height := m.viewport.Height()
	if cursorLine < yOffset || cursorLine >= yOffset+height {
		t.Fatalf("cursor line %d not in visible range [%d, %d)", cursorLine, yOffset, yOffset+height)
	}
	// Cursor should be near the middle of the viewport, not at the top
	expectedOffset := cursorLine - height/2
	if yOffset != expectedOffset {
		t.Fatalf("expected yOffset %d (centered), got %d", expectedOffset, yOffset)
	}
}

func TestEnsureCursorVisibleNoOpWhenVisible(t *testing.T) {
	m := New(
		WithColumns([]Column{{Title: "Name", Width: 20}}),
		WithRows(makeRows(5)),
		WithHeight(10), // viewport bigger than rows
	)

	m.SetCursor(2)
	origOffset := m.viewport.YOffset()

	m.EnsureCursorVisible()

	if m.viewport.YOffset() != origOffset {
		t.Fatalf("yOffset should not change when cursor visible, was %d now %d",
			origOffset, m.viewport.YOffset())
	}
}

func TestEnsureCursorVisibleEmptyTable(t *testing.T) {
	m := New(
		WithColumns([]Column{{Title: "Name", Width: 20}}),
		WithHeight(10),
	)

	startBefore := m.start
	cursorBefore := m.cursor
	offsetBefore := m.viewport.YOffset()

	// Should not panic and must leave scroll/cursor state untouched on an empty table.
	m.EnsureCursorVisible()

	if m.start != startBefore {
		t.Fatalf("start should not change on empty table, was %d now %d", startBefore, m.start)
	}
	if m.cursor != cursorBefore {
		t.Fatalf("cursor should not change on empty table, was %d now %d", cursorBefore, m.cursor)
	}
	if m.viewport.YOffset() != offsetBefore {
		t.Fatalf("yOffset should not change on empty table, was %d now %d", offsetBefore, m.viewport.YOffset())
	}
}

func TestSetColumnsAndRows(t *testing.T) {
	m := New(
		WithColumns([]Column{{Title: "Name", Width: 20}}),
		WithHeight(10),
	)

	newCols := []Column{{Title: "ID", Width: 10}, {Title: "Value", Width: 30}}
	newRows := []Row{{"1", "alpha"}, {"2", "bravo"}}
	m.SetColumnsAndRows(newCols, newRows)

	if len(m.Columns()) != 2 {
		t.Fatalf("expected 2 columns, got %d", len(m.Columns()))
	}
	if len(m.Rows()) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(m.Rows()))
	}
	if m.Columns()[0].Title != "ID" {
		t.Fatalf("expected column title 'ID', got %q", m.Columns()[0].Title)
	}
}

func TestSetColumnsAndRowsCursorClamp(t *testing.T) {
	m := New(
		WithColumns([]Column{{Title: "Name", Width: 20}}),
		WithRows(makeRows(10)),
		WithHeight(10),
	)
	m.SetCursor(9)

	// Replace with fewer rows — cursor must clamp
	m.SetColumnsAndRows(
		[]Column{{Title: "Name", Width: 20}},
		makeRows(3),
	)
	if m.Cursor() != 2 {
		t.Fatalf("expected cursor clamped to 2, got %d", m.Cursor())
	}
}

func TestSetRowsEmptyCursorClamp(t *testing.T) {
	m := New(
		WithColumns([]Column{{Title: "Name", Width: 20}}),
		WithRows(makeRows(5)),
		WithHeight(10),
	)
	m.SetCursor(3)

	// Set empty rows — cursor must clamp to 0, not -1
	m.SetRows(nil)
	if m.Cursor() != 0 {
		t.Fatalf("expected cursor 0 on empty table, got %d", m.Cursor())
	}
}

func TestSetRowsEmptyThenRepopulate(t *testing.T) {
	m := New(
		WithColumns([]Column{{Title: "Name", Width: 20}}),
		WithRows(makeRows(5)),
		WithHeight(10),
	)
	m.SetCursor(3)

	// Empty then repopulate
	m.SetRows(nil)
	m.SetRows(makeRows(5))

	// Cursor should be valid (0), not stuck at -1
	if m.Cursor() < 0 || m.Cursor() >= 5 {
		t.Fatalf("expected valid cursor after repopulate, got %d", m.Cursor())
	}
	row := m.SelectedRow()
	if row == nil {
		t.Fatal("expected non-nil SelectedRow after repopulate")
	}
}

func TestSetCursorOnEmptyTable(t *testing.T) {
	m := New(
		WithColumns([]Column{{Title: "Name", Width: 20}}),
		WithHeight(10),
	)
	m.SetCursor(0)
	if m.Cursor() != 0 {
		t.Fatalf("expected cursor 0 on empty table, got %d", m.Cursor())
	}
}

func TestSetColumnsAndRowsEmptyCursorClamp(t *testing.T) {
	m := New(
		WithColumns([]Column{{Title: "Name", Width: 20}}),
		WithRows(makeRows(5)),
		WithHeight(10),
	)
	m.SetCursor(3)

	m.SetColumnsAndRows([]Column{{Title: "Name", Width: 20}}, nil)
	if m.Cursor() != 0 {
		t.Fatalf("expected cursor 0 on empty table, got %d", m.Cursor())
	}
}

func TestRowStyleFunc(t *testing.T) {
	markedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000"))
	m := New(
		WithColumns([]Column{{Title: "Name", Width: 20}}),
		WithRows(makeRows(5)),
		WithHeight(10),
	)
	m.SetCursor(2)

	m.RowStyleFunc = func(index int, isCursor, active bool) *lipgloss.Style {
		if index == 1 {
			return &markedStyle
		}
		return nil
	}
	m.UpdateViewport()

	view := m.View()
	if view == "" {
		t.Fatal("expected non-empty view")
	}
}

// stylePrefix returns the leading SGR escape sequence a style emits before its
// content, derived from the style object itself (no hardcoded escape codes).
func stylePrefix(s lipgloss.Style) string {
	rendered := s.Render("X")
	i := strings.Index(rendered, "X")
	if i <= 0 {
		return ""
	}
	return rendered[:i]
}

// TestSelectionActiveGatesCursor verifies that the cursor highlight in renderRow
// is gated purely on the live selectionActive flag, for both an override-returning
// RowStyleFunc (Case A) and a nil RowStyleFunc that relies on styles.Selected
// (Case B). When selectionActive is false the cursor row must render plain — no
// override, no Selected style. The cursor row is inspected via renderRow (the same
// path UpdateViewport feeds View), matching the existing renderRow-level tests.
func TestSelectionActiveGatesCursor(t *testing.T) {
	// Case A: an override-returning RowStyleFunc that only fires for the cursor
	// row when its `active` arg is true.
	t.Run("override RowStyleFunc", func(t *testing.T) {
		overrideStyle := lipgloss.NewStyle().Background(lipgloss.Color("#00FF00")).Foreground(lipgloss.Color("#000000"))
		m := New(
			WithColumns([]Column{{Title: "Name", Width: 20}}),
			WithRows(makeRows(3)),
			WithHeight(10),
		)
		m.SetCursor(1)
		m.RowStyleFunc = func(index int, isCursor, active bool) *lipgloss.Style {
			if index == 1 && isCursor && active {
				return &overrideStyle
			}
			return nil
		}

		want := stylePrefix(overrideStyle)
		if want == "" {
			t.Fatal("test setup: override style produced no SGR prefix")
		}
		// The default Selected style must not leak in either: with the override
		// returning nil while inactive, the row should render truly plain.
		selPrefix := stylePrefix(m.styles.Selected)
		if selPrefix == "" {
			t.Fatal("test setup: default Selected style produced no SGR prefix")
		}

		m.SetSelectionActive(true)
		if got := m.renderRow(1); !strings.Contains(got, want) {
			t.Fatalf("active cursor: expected override SGR %q in cursor row, got %q", want, got)
		}

		m.SetSelectionActive(false)
		if got := m.renderRow(1); strings.Contains(got, want) {
			t.Fatalf("inactive cursor: override SGR %q must be absent (plain row), got %q", want, got)
		}
		if got := m.renderRow(1); strings.Contains(got, selPrefix) {
			t.Fatalf("inactive cursor: Selected SGR %q must be absent (plain row), got %q", selPrefix, got)
		}
	})

	// Case B: no RowStyleFunc; the cursor highlight rides on styles.Selected.
	t.Run("nil RowStyleFunc relies on styles.Selected", func(t *testing.T) {
		m := New(
			WithColumns([]Column{{Title: "Name", Width: 20}}),
			WithRows(makeRows(3)),
			WithHeight(10),
		)
		m.SetCursor(1)
		// A distinctive, non-empty Selected style so its SGR is detectable.
		sel := lipgloss.NewStyle().Background(lipgloss.Color("#FF00FF")).Foreground(lipgloss.Color("#FFFFFF")).Bold(true)
		m.SetStyles(Styles{
			Header:   DefaultStyles().Header,
			Cell:     DefaultStyles().Cell,
			Selected: sel,
		})

		want := stylePrefix(sel)
		if want == "" {
			t.Fatal("test setup: Selected style produced no SGR prefix")
		}

		m.SetSelectionActive(true)
		if got := m.renderRow(1); !strings.Contains(got, want) {
			t.Fatalf("active cursor: expected Selected SGR %q in cursor row, got %q", want, got)
		}

		m.SetSelectionActive(false)
		if got := m.renderRow(1); strings.Contains(got, want) {
			t.Fatalf("inactive cursor: Selected SGR %q must be absent (plain row), got %q", want, got)
		}
	})
}

func TestRowStyleFuncNil(t *testing.T) {
	m := New(
		WithColumns([]Column{{Title: "Name", Width: 20}}),
		WithRows(makeRows(5)),
		WithHeight(10),
	)
	m.UpdateViewport()
	view := m.View()
	if view == "" {
		t.Fatal("expected non-empty view")
	}
}

func TestSetLayout(t *testing.T) {
	m := New(
		WithColumns([]Column{{Title: "Name", Width: 20}}),
		WithRows(makeRows(10)),
		WithHeight(10),
		WithWidth(80),
	)

	newCols := []Column{{Title: "ID", Width: 10}, {Title: "Value", Width: 30}}
	m.SetLayout(newCols, 60, 8)

	if len(m.Columns()) != 2 {
		t.Fatalf("expected 2 columns, got %d", len(m.Columns()))
	}
	if m.Columns()[0].Title != "ID" {
		t.Fatalf("expected column title 'ID', got %q", m.Columns()[0].Title)
	}
	if m.Width() != 60 {
		t.Fatalf("expected width 60, got %d", m.Width())
	}
	// Height accounts for header subtraction
	view := m.View()
	if view == "" {
		t.Fatal("expected non-empty view after SetLayout")
	}
}

func TestSetLayoutCursorClamp(t *testing.T) {
	m := New(
		WithColumns([]Column{{Title: "Name", Width: 20}}),
		WithRows(makeRows(10)),
		WithHeight(10),
	)
	m.SetCursor(9)

	// SetLayout doesn't change rows, cursor should stay
	m.SetLayout([]Column{{Title: "Name", Width: 20}}, 80, 10)
	if m.Cursor() != 9 {
		t.Fatalf("expected cursor 9, got %d", m.Cursor())
	}
}

func TestTableXOffset(t *testing.T) {
	cols := []Column{
		{Title: "NAME", Width: 20},
		{Title: "STATUS", Width: 10},
	}
	rows := []Row{
		{"my-pod-name", "Running"},
	}
	m := New(
		WithColumns(cols),
		WithRows(rows),
		WithWidth(15),
		WithHeight(5),
	)
	m.SetContentWidth(34) // 20+10 + 2*2 padding

	if m.XOffset() != 0 {
		t.Fatalf("expected initial xOffset 0, got %d", m.XOffset())
	}

	m.SetXOffset(5)
	if m.XOffset() != 5 {
		t.Fatalf("expected xOffset 5, got %d", m.XOffset())
	}

	// Clamp to max
	m.SetXOffset(100)
	maxX := 34 - 15
	if m.XOffset() != maxX {
		t.Fatalf("expected xOffset clamped to %d, got %d", maxX, m.XOffset())
	}

	// Clamp to 0
	m.SetXOffset(-5)
	if m.XOffset() != 0 {
		t.Fatalf("expected xOffset clamped to 0, got %d", m.XOffset())
	}
}

func TestTableXOffsetNoScrollNeeded(t *testing.T) {
	m := New(WithWidth(80))
	m.SetContentWidth(40) // content fits
	m.SetXOffset(10)
	if m.XOffset() != 0 {
		t.Fatalf("expected xOffset 0 when content fits, got %d", m.XOffset())
	}
}

func TestRowAtYWithHeaderZeroScroll(t *testing.T) {
	m := New(
		WithColumns([]Column{{Title: "Name", Width: 20}}),
		WithRows(makeRows(10)),
		WithHeight(10),
	)
	// cursor=0 so m.start=0
	if got := m.RowAtY(0); got != -1 {
		t.Fatalf("y=0 (header): expected -1, got %d", got)
	}
	if got := m.RowAtY(1); got != 0 {
		t.Fatalf("y=1: expected 0, got %d", got)
	}
	if got := m.RowAtY(2); got != 1 {
		t.Fatalf("y=2: expected 1, got %d", got)
	}
	if got := m.RowAtY(5); got != 4 {
		t.Fatalf("y=5: expected 4, got %d", got)
	}
}

func TestRowAtYWithNonZeroScroll(t *testing.T) {
	// Small viewport, many rows, cursor far down — forces m.start > 0
	// AND viewport.YOffset() > 0 because MoveDown adjusts the viewport
	// scroll position to keep the cursor visible.
	m := New(
		WithColumns([]Column{{Title: "Name", Width: 20}}),
		WithRows(makeRows(50)),
		WithHeight(6), // viewport height 5 after header
	)
	m.MoveDown(20) // cursor = 20

	// Verify m.start is non-zero
	if m.start == 0 {
		t.Fatalf("test setup: expected m.start > 0, got %d", m.start)
	}

	// y=0 is header row → -1
	if got := m.RowAtY(0); got != -1 {
		t.Fatalf("y=0 (header): expected -1, got %d", got)
	}
	// y=1 is the first rendered body row, which corresponds to data
	// index m.start + viewport.YOffset().
	want := m.start + m.viewport.YOffset()
	if got := m.RowAtY(1); got != want {
		t.Fatalf("y=1: expected m.start+YOffset=%d, got %d", want, got)
	}
	if got := m.RowAtY(2); got != want+1 {
		t.Fatalf("y=2: expected %d, got %d", want+1, got)
	}
}

// TestRowAtYWithStartOnlyNoYOffset exercises the m.start > 0 branch in
// isolation — the rendered window starts at m.start but the viewport itself
// has not been scrolled (YOffset == 0). RowAtY(1) must return m.start.
func TestRowAtYWithStartOnlyNoYOffset(t *testing.T) {
	m := New(
		WithColumns([]Column{{Title: "Name", Width: 20}}),
		WithRows(makeRows(50)),
		WithHeight(11), // viewport height 10 after header
	)
	// Cursor at index 15 — UpdateViewport sets m.start = clamp(15-10, 0, 15) = 5.
	// EnsureCursorVisible centers the cursor and SetYOffset would normally
	// move it; we then explicitly reset YOffset to 0 so we exercise pure
	// m.start contribution.
	m.SetCursor(15)
	m.viewport.SetYOffset(0)

	if m.start == 0 {
		t.Fatalf("test setup: expected m.start > 0, got %d", m.start)
	}
	if m.viewport.YOffset() != 0 {
		t.Fatalf("test setup: expected YOffset == 0, got %d", m.viewport.YOffset())
	}
	if got := m.RowAtY(1); got != m.start {
		t.Fatalf("y=1: expected m.start=%d, got %d", m.start, got)
	}
	if got := m.RowAtY(2); got != m.start+1 {
		t.Fatalf("y=2: expected m.start+1=%d, got %d", m.start+1, got)
	}
}

func TestRowAtYOutOfRange(t *testing.T) {
	m := New(
		WithColumns([]Column{{Title: "Name", Width: 20}}),
		WithRows(makeRows(3)),
		WithHeight(10),
	)
	// 3 rows: valid data at y=1,2,3. y=4 → past last row → -1.
	if got := m.RowAtY(4); got != -1 {
		t.Fatalf("y past last row: expected -1, got %d", got)
	}
	if got := m.RowAtY(100); got != -1 {
		t.Fatalf("y=100: expected -1, got %d", got)
	}
}

func TestRowAtYPastViewportWithScroll(t *testing.T) {
	// Many rows, small viewport, cursor far down so m.start > 0. A y that
	// is valid in terms of header+data count but past the viewport bottom
	// must return -1 (no row is actually rendered there).
	m := New(
		WithColumns([]Column{{Title: "Name", Width: 20}}),
		WithRows(makeRows(50)),
		WithHeight(6), // viewport height 5 after header
	)
	m.MoveDown(20)
	if m.start == 0 {
		t.Fatalf("test setup: expected m.start > 0, got %d", m.start)
	}
	vh := 5 // viewport height matches WithHeight(6) - 1 header

	// y=1+vh (== 6) is one past the last rendered body row — must be -1.
	if got := m.RowAtY(1 + vh); got != -1 {
		t.Fatalf("y=1+vh: expected -1 (off-bottom), got %d", got)
	}
	// Within viewport still works. The data row visible at the last body
	// line is m.start + viewport.YOffset() + (vh-1).
	wantLast := m.start + m.viewport.YOffset() + vh - 1
	if got := m.RowAtY(vh); got != wantLast {
		t.Fatalf("y=vh (last visible row): expected %d, got %d", wantLast, got)
	}
}

func TestRowAtYEmptyRows(t *testing.T) {
	m := New(
		WithColumns([]Column{{Title: "Name", Width: 20}}),
		WithHeight(10),
	)
	for _, y := range []int{-1, 0, 1, 2, 5} {
		if got := m.RowAtY(y); got != -1 {
			t.Fatalf("empty table, y=%d: expected -1, got %d", y, got)
		}
	}
}

func TestRowAtYNegativeY(t *testing.T) {
	m := New(
		WithColumns([]Column{{Title: "Name", Width: 20}}),
		WithRows(makeRows(5)),
		WithHeight(10),
	)
	if got := m.RowAtY(-1); got != -1 {
		t.Fatalf("y=-1: expected -1, got %d", got)
	}
}

func TestRowAtYDoesNotMutate(t *testing.T) {
	m := New(
		WithColumns([]Column{{Title: "Name", Width: 20}}),
		WithRows(makeRows(10)),
		WithHeight(10),
	)
	m.SetCursor(3)
	startBefore := m.start
	cursorBefore := m.Cursor()

	_ = m.RowAtY(2)
	_ = m.RowAtY(100)
	_ = m.RowAtY(-5)

	if m.start != startBefore {
		t.Fatalf("RowAtY mutated m.start: before=%d after=%d", startBefore, m.start)
	}
	if m.Cursor() != cursorBefore {
		t.Fatalf("RowAtY mutated cursor: before=%d after=%d", cursorBefore, m.Cursor())
	}
}

// firstVisibleRow returns the data index of the first row currently visible in
// the viewport: m.start + viewport.YOffset().
func firstVisibleRow(m *Model) int {
	return m.start + m.viewport.YOffset()
}

// TestMoveUpFromLastVisibleRowNoScroll verifies that pressing up while the
// cursor sits on the last visible row only moves the cursor — it must NOT
// scroll the viewport (which would hide that row).
func TestMoveUpFromLastVisibleRowNoScroll(t *testing.T) {
	m := New(
		WithColumns([]Column{{Title: "Name", Width: 20}}),
		WithRows(makeRows(50)),
		WithHeight(6), // viewport height 5 after header
	)
	height := m.viewport.Height() // 5

	// Construct a window with a NON-TRIVIAL scroll offset (yoffset=3) whose
	// first visible data row is 23, then place the cursor on the LAST visible
	// row of that window. A yoffset >= 2 is what makes this a real guard: with
	// the old offset+n heuristic, MoveUp would bump yoffset and scroll the
	// window down (first visible row 23 -> 25), whereas the minimal-scroll code
	// must leave the first visible row unchanged.
	//
	// SetCursor first so the viewport renders a wide enough window for the
	// offset to be valid, then lower m.start and set the offset directly.
	m.SetCursor(27)
	m.start = 20
	m.viewport.SetYOffset(3)              // first visible row = 23
	top := m.start + m.viewport.YOffset() // 23
	m.cursor = top + height - 1           // 27, last visible row

	topBefore := firstVisibleRow(&m)
	m.MoveUp(1)

	if got := firstVisibleRow(&m); got != topBefore {
		t.Fatalf("first visible row changed on MoveUp from last visible row: before=%d after=%d", topBefore, got)
	}
	if m.Cursor() != top+height-2 {
		t.Fatalf("expected cursor at %d, got %d", top+height-2, m.Cursor())
	}
}

// TestMoveDownFromFirstVisibleRowNoScroll verifies that pressing down while the
// cursor sits on the first visible row only moves the cursor — no scroll.
func TestMoveDownFromFirstVisibleRowNoScroll(t *testing.T) {
	m := New(
		WithColumns([]Column{{Title: "Name", Width: 20}}),
		WithRows(makeRows(50)),
		WithHeight(6), // viewport height 5 after header
	)

	// Mirror of the MoveUp guard: a window with a NON-TRIVIAL offset (yoffset=3,
	// first visible row 23) with the cursor on the FIRST visible row. The old
	// offset-n heuristic would shrink the offset and scroll the window up (first
	// visible row 23 -> 21); the minimal-scroll code must not move it.
	m.SetCursor(23)
	m.start = 20
	m.viewport.SetYOffset(3)              // first visible row = 23
	top := m.start + m.viewport.YOffset() // 23
	m.cursor = top                        // first visible row

	topBefore := firstVisibleRow(&m)
	m.MoveDown(1)

	if got := firstVisibleRow(&m); got != topBefore {
		t.Fatalf("first visible row changed on MoveDown from first visible row: before=%d after=%d", topBefore, got)
	}
	if m.Cursor() != top+1 {
		t.Fatalf("expected cursor at %d, got %d", top+1, m.Cursor())
	}
}

// TestMoveUpFromTopOfWindowScrollsByOne verifies that pressing up while the
// cursor is on the first visible row scrolls up by exactly one and keeps the
// cursor pinned to the top edge.
func TestMoveUpFromTopOfWindowScrollsByOne(t *testing.T) {
	m := New(
		WithColumns([]Column{{Title: "Name", Width: 20}}),
		WithRows(makeRows(50)),
		WithHeight(6), // viewport height 5 after header
	)

	m.MoveDown(20)
	top := firstVisibleRow(&m)
	m.SetCursor(top) // cursor at top of window
	m.viewport.SetYOffset(top - m.start)

	topBefore := firstVisibleRow(&m)
	m.MoveUp(1)

	if got := firstVisibleRow(&m); got != topBefore-1 {
		t.Fatalf("expected scroll up by 1 to %d, got %d", topBefore-1, got)
	}
	if m.Cursor() != firstVisibleRow(&m) {
		t.Fatalf("cursor should stay at top edge %d, got %d", firstVisibleRow(&m), m.Cursor())
	}
}

// TestMoveDownFromBottomOfWindowScrollsByOne verifies that pressing down while
// the cursor is on the last visible row scrolls down by exactly one and keeps
// the cursor pinned to the bottom edge.
func TestMoveDownFromBottomOfWindowScrollsByOne(t *testing.T) {
	m := New(
		WithColumns([]Column{{Title: "Name", Width: 20}}),
		WithRows(makeRows(50)),
		WithHeight(6), // viewport height 5 after header
	)
	height := m.viewport.Height()

	m.MoveDown(20)
	top := firstVisibleRow(&m)
	m.SetCursor(top + height - 1) // cursor at bottom of window
	m.viewport.SetYOffset(top - m.start)

	topBefore := firstVisibleRow(&m)
	m.MoveDown(1)

	if got := firstVisibleRow(&m); got != topBefore+1 {
		t.Fatalf("expected scroll down by 1 to %d, got %d", topBefore+1, got)
	}
	// cursor stays at bottom edge
	if m.Cursor() != firstVisibleRow(&m)+height-1 {
		t.Fatalf("cursor should stay at bottom edge %d, got %d", firstVisibleRow(&m)+height-1, m.Cursor())
	}
}

// TestMoveDownLargeJumpLandsAtEdge verifies a jump of one full page lands the
// cursor at the bottom viewport edge and keeps it visible.
func TestMoveDownLargeJumpLandsAtEdge(t *testing.T) {
	m := New(
		WithColumns([]Column{{Title: "Name", Width: 20}}),
		WithRows(makeRows(50)),
		WithHeight(6), // viewport height 5 after header
	)
	height := m.viewport.Height()

	m.MoveDown(height) // cursor = 5

	top := firstVisibleRow(&m)
	if m.Cursor() < top || m.Cursor() > top+height-1 {
		t.Fatalf("cursor %d not visible in window [%d,%d]", m.Cursor(), top, top+height-1)
	}
	if m.Cursor() != top+height-1 {
		t.Fatalf("expected cursor at bottom edge %d, got %d", top+height-1, m.Cursor())
	}
}

// TestMoveUpLargeJumpLandsAtEdge verifies a page-up jump lands the cursor at
// the top viewport edge and keeps it visible.
func TestMoveUpLargeJumpLandsAtEdge(t *testing.T) {
	m := New(
		WithColumns([]Column{{Title: "Name", Width: 20}}),
		WithRows(makeRows(50)),
		WithHeight(6), // viewport height 5 after header
	)
	height := m.viewport.Height()

	m.MoveDown(30) // somewhere in the middle
	m.MoveUp(height)

	top := firstVisibleRow(&m)
	if m.Cursor() < top || m.Cursor() > top+height-1 {
		t.Fatalf("cursor %d not visible in window [%d,%d]", m.Cursor(), top, top+height-1)
	}
	if m.Cursor() != top {
		t.Fatalf("expected cursor at top edge %d, got %d", top, m.Cursor())
	}
}

// TestGotoTopBottomPlaceCursor verifies GotoTop/GotoBottom still position the
// cursor correctly and keep it visible.
func TestGotoTopBottomPlaceCursor(t *testing.T) {
	m := New(
		WithColumns([]Column{{Title: "Name", Width: 20}}),
		WithRows(makeRows(50)),
		WithHeight(6), // viewport height 5 after header
	)
	height := m.viewport.Height()

	m.GotoBottom()
	if m.Cursor() != 49 {
		t.Fatalf("GotoBottom: expected cursor 49, got %d", m.Cursor())
	}
	top := firstVisibleRow(&m)
	if m.Cursor() < top || m.Cursor() > top+height-1 {
		t.Fatalf("GotoBottom: cursor %d not visible in window [%d,%d]", m.Cursor(), top, top+height-1)
	}
	// The viewport must actually be scrolled to the bottom edge: the last row is
	// pinned to the bottom, so the first visible row is len(rows)-height.
	if wantTop := len(makeRows(50)) - height; top != wantTop {
		t.Fatalf("GotoBottom: expected first visible row %d (bottom edge), got %d", wantTop, top)
	}

	m.GotoTop()
	if m.Cursor() != 0 {
		t.Fatalf("GotoTop: expected cursor 0, got %d", m.Cursor())
	}
	if firstVisibleRow(&m) != 0 {
		t.Fatalf("GotoTop: expected first visible row 0, got %d", firstVisibleRow(&m))
	}
}

// TestMoveOnListShorterThanViewport verifies that when the entire list fits
// within the viewport (rows < height) MoveUp/MoveDown never scroll: the first
// visible row and the raw YOffset both stay 0 while the cursor tracks each move.
func TestMoveOnListShorterThanViewport(t *testing.T) {
	m := New(
		WithColumns([]Column{{Title: "Name", Width: 20}}),
		WithRows(makeRows(3)),
		WithHeight(11), // viewport height 10 after header; 3 rows < 10
	)

	assertNoScroll := func(stage string) {
		if got := firstVisibleRow(&m); got != 0 {
			t.Fatalf("%s: expected first visible row 0, got %d", stage, got)
		}
		if got := m.viewport.YOffset(); got != 0 {
			t.Fatalf("%s: expected YOffset 0, got %d", stage, got)
		}
	}

	assertNoScroll("initial")

	// Move down across every row.
	for i := 1; i < 3; i++ {
		m.MoveDown(1)
		if m.Cursor() != i {
			t.Fatalf("MoveDown: expected cursor %d, got %d", i, m.Cursor())
		}
		assertNoScroll(fmt.Sprintf("after MoveDown to %d", i))
	}

	// Move back up across every row.
	for i := 1; i >= 0; i-- {
		m.MoveUp(1)
		if m.Cursor() != i {
			t.Fatalf("MoveUp: expected cursor %d, got %d", i, m.Cursor())
		}
		assertNoScroll(fmt.Sprintf("after MoveUp to %d", i))
	}
}

// TestVisibleRangeBoundedAndContainsCursor verifies that VisibleRange returns a
// bounded window around the cursor when the list is taller than the viewport.
func TestVisibleRangeBoundedAndContainsCursor(t *testing.T) {
	const rowCount = 100
	m := New(
		WithColumns([]Column{{Title: "Name", Width: 20}}),
		WithRows(makeRows(rowCount)),
		WithHeight(6), // viewport height 5 after header — far smaller than rowCount
	)
	height := m.viewport.Height()

	m.SetCursor(50) // middle row, well inside a windowed list

	start, end := m.VisibleRange()

	if start < 0 {
		t.Fatalf("expected start >= 0, got %d", start)
	}
	if end > len(m.Rows()) {
		t.Fatalf("expected end <= len(rows)=%d, got %d", len(m.Rows()), end)
	}
	if !(start <= m.Cursor() && m.Cursor() < end) {
		t.Fatalf("expected start <= cursor < end, got start=%d cursor=%d end=%d",
			start, m.Cursor(), end)
	}
	if size := end - start; size > 2*height+1 {
		t.Fatalf("expected window size <= 2*height+1=%d, got %d", 2*height+1, size)
	}

	// Pin the exact window with hand-computed literals so a change to the
	// production windowing formula breaks this test (instead of a tautology
	// that re-derives the window with the same clamp() call as production).
	//
	// Setup: WithHeight(6) minus the 1-line header => viewport height 5.
	// Cursor is 50, rowCount is 100. The production formula yields:
	//   start = clamp(50-5, 0, 50) = 45
	//   end   = clamp(50+5, 50, 100) = 55
	const (
		wantStart = 45
		wantEnd   = 55
	)
	if height != 5 {
		t.Fatalf("test assumes viewport height 5 (WithHeight(6) - header); got %d", height)
	}
	if start != wantStart || end != wantEnd {
		t.Fatalf("expected exact window [%d,%d), got [%d,%d)", wantStart, wantEnd, start, end)
	}
}

// TestVisibleRangeShorterThanViewport verifies that when the list is shorter than
// the viewport the window spans the entire list (no windowing): [0, len(rows)).
func TestVisibleRangeShorterThanViewport(t *testing.T) {
	const rowCount = 3
	m := New(
		WithColumns([]Column{{Title: "Name", Width: 20}}),
		WithRows(makeRows(rowCount)),
		WithHeight(10), // viewport taller than the list
	)

	m.SetCursor(1)

	start, end := m.VisibleRange()
	if start != 0 || end != rowCount {
		t.Fatalf("short list should yield full-list window [0,%d); got [%d,%d)", rowCount, start, end)
	}
}

// TestRowStyleFuncStripsInnerANSI verifies that when a whole-row foreground
// override is applied, any embedded per-cell SGR (including a \x1b[39m
// foreground-only reset) is stripped from the row first, so the override's
// foreground tint spans the entire row instead of being cut short mid-row.
func TestRowStyleFuncStripsInnerANSI(t *testing.T) {
	const innerReset = "\x1b[39m"
	cellValue := "\x1b[31mERR\x1b[39m"

	overrideStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("11")) // yellow
	m := New(
		WithColumns([]Column{{Title: "Status", Width: 20}}),
		WithRows([]Row{
			{"row-00"},
			{cellValue},
		}),
		WithHeight(10),
	)
	// Cursor on row 0 so row 1 exercises the NON-cursor override branch.
	m.SetCursor(0)
	m.RowStyleFunc = func(index int, isCursor, active bool) *lipgloss.Style {
		if index == 1 {
			return &overrideStyle
		}
		return nil
	}

	out := m.renderRow(1)

	if strings.Contains(out, innerReset) {
		t.Fatalf("expected inner foreground reset %q to be stripped, got %q", innerReset, out)
	}
	// The override's foreground SGR must be present (the row is tinted).
	want := overrideStyle.Render("ERR")
	prefix := want[:strings.Index(want, "ERR")] // leading SGR of the override
	if prefix == "" {
		t.Fatalf("test setup: override produced no SGR prefix: %q", want)
	}
	if !strings.Contains(out, prefix) {
		t.Fatalf("expected override SGR prefix %q in output, got %q", prefix, out)
	}
}

// TestRowStyleFuncStripsInnerANSIOnCursorRow is the cursor-branch counterpart of
// TestRowStyleFuncStripsInnerANSI: the cursor is ON the row under test and the
// RowStyleFunc returns a non-nil override for the cursor row. That override must
// win over the default Selected style and the inner per-cell color SGRs must be
// stripped (via stripStyleKeepReverse) so the override tint spans the whole row.
func TestRowStyleFuncStripsInnerANSIOnCursorRow(t *testing.T) {
	const innerReset = "\x1b[39m"
	cellValue := "\x1b[31mERR\x1b[39m"

	overrideStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("11")) // yellow
	m := New(
		WithColumns([]Column{{Title: "Status", Width: 20}}),
		WithRows([]Row{
			{"row-00"},
			{cellValue},
		}),
		WithHeight(10),
	)
	// Cursor ON row 1 so renderRow takes the r == m.cursor override branch.
	m.SetCursor(1)
	m.RowStyleFunc = func(index int, isCursor, active bool) *lipgloss.Style {
		if index == 1 && isCursor {
			return &overrideStyle
		}
		return nil
	}

	out := m.renderRow(1)

	if strings.Contains(out, innerReset) {
		t.Fatalf("expected inner foreground reset %q to be stripped on cursor row, got %q", innerReset, out)
	}
	if strings.Contains(out, "\x1b[31m") {
		t.Fatalf("expected inner foreground color SGR to be stripped on cursor row, got %q", out)
	}
	// The override's foreground SGR must be present (the cursor row is tinted by
	// the override, not by the default Selected style).
	want := overrideStyle.Render("ERR")
	prefix := want[:strings.Index(want, "ERR")] // leading SGR of the override
	if prefix == "" {
		t.Fatalf("test setup: override produced no SGR prefix: %q", want)
	}
	if !strings.Contains(out, prefix) {
		t.Fatalf("expected override SGR prefix %q in cursor-row output, got %q", prefix, out)
	}
}

// TestRowStyleFuncNilPreservesInnerANSI is the control: with no override for a
// row, the embedded per-cell ANSI (the colored status cell) is preserved so
// healthy rows keep their colored status cell.
func TestRowStyleFuncNilPreservesInnerANSI(t *testing.T) {
	const innerReset = "\x1b[39m"
	cellValue := "\x1b[31mERR\x1b[39m"

	m := New(
		WithColumns([]Column{{Title: "Status", Width: 20}}),
		WithRows([]Row{
			{"row-00"},
			{cellValue},
		}),
		WithHeight(10),
	)
	m.SetCursor(0)
	// No RowStyleFunc => non-override return path keeps embedded ANSI.

	out := m.renderRow(1)

	if !strings.Contains(out, innerReset) {
		t.Fatalf("expected embedded ANSI %q to be preserved without override, got %q", innerReset, out)
	}
	if !strings.Contains(out, "\x1b[31m") {
		t.Fatalf("expected embedded foreground SGR to be preserved, got %q", out)
	}
}

func makeRows(n int) []Row {
	rows := make([]Row, n)
	for i := range n {
		rows[i] = Row{fmt.Sprintf("row-%02d", i)}
	}
	return rows
}

// TestRenderRowWithMoreCellsThanColumns confirms renderRow does not panic
// when a plugin supplies a row with more cells than declared columns. The
// extra cells are silently dropped.
func TestRenderRowWithMoreCellsThanColumns(t *testing.T) {
	m := New(
		WithColumns([]Column{
			{Title: "A", Width: 4},
			{Title: "B", Width: 4},
		}),
		WithRows([]Row{
			{"a1", "b1", "extra", "another"},
		}),
	)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("renderRow panicked on overflow row: %v", r)
		}
	}()

	// Exercise View() which calls renderRow for each row.
	out := m.View()
	if len(out) == 0 {
		t.Fatal("expected non-empty render output")
	}
}

func TestStripStyleKeepReverse(t *testing.T) {
	const (
		revOn  = "\x1b[7m"
		revOff = "\x1b[27m"
	)
	// A cell with a foreground color and a reverse-video search match inside it.
	in := "\x1b[38;2;80;250;123mRun" + revOn + "ning" + revOff + "\x1b[39m pod"
	got := stripStyleKeepReverse(in)

	// Color SGRs must be gone; reverse-video markers must remain, around "ning".
	if strings.Contains(got, "\x1b[38;2;80;250;123m") || strings.Contains(got, "\x1b[39m") {
		t.Fatalf("color SGRs should be stripped, got %q", got)
	}
	want := "Run" + revOn + "ning" + revOff + " pod"
	if got != want {
		t.Fatalf("stripStyleKeepReverse = %q, want %q", got, want)
	}
}

func TestStripStyleKeepReverseNoEscape(t *testing.T) {
	if got := stripStyleKeepReverse("plain row"); got != "plain row" {
		t.Fatalf("plain text should pass through unchanged, got %q", got)
	}
}

// TestStripStyleKeepReverseMultiByteUTF8 guards against the multi-byte UTF-8 bug
// where the visual-position counter is mistaken for a rune index: a reverse
// marker placed after non-ASCII content must land on a valid byte boundary so
// the output stays valid UTF-8 and the marker wraps the right run.
func TestStripStyleKeepReverseMultiByteUTF8(t *testing.T) {
	const (
		revOn  = "\x1b[7m"
		revOff = "\x1b[27m"
	)
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "accented latin before marker",
			// "café-pod" with a colored prefix; the search match wraps "pod".
			in:   "\x1b[38;2;80;250;123mcafé-" + revOn + "pod" + revOff + "\x1b[39m",
			want: "café-" + revOn + "pod" + revOff,
		},
		{
			name: "cjk before and inside marker",
			// 名前 = two 3-byte runes; marker wraps the second rune.
			in:   "\x1b[31m名" + revOn + "前" + revOff + "\x1b[39m",
			want: "名" + revOn + "前" + revOff,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := stripStyleKeepReverse(tc.in)
			if !utf8.ValidString(got) {
				t.Fatalf("output is not valid UTF-8: %q", got)
			}
			if got != tc.want {
				t.Fatalf("stripStyleKeepReverse = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestStripStyleKeepReverseEdgeCases covers multiple disjoint reverse spans, an
// unmatched revOn, and a marker at the end-of-string boundary.
func TestStripStyleKeepReverseEdgeCases(t *testing.T) {
	const (
		revOn  = "\x1b[7m"
		revOff = "\x1b[27m"
		color  = "\x1b[31m"
		colOff = "\x1b[39m"
	)
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "two disjoint spans",
			in:   color + revOn + "ab" + revOff + "cd" + revOn + "ef" + revOff + colOff,
			want: revOn + "ab" + revOff + "cd" + revOn + "ef" + revOff,
		},
		{
			name: "revOn without matching revOff",
			in:   color + "ab" + revOn + "cd" + colOff,
			want: "ab" + revOn + "cd",
		},
		{
			name: "marker at end-of-string boundary",
			// revOn appears after all visible text (visualPos == len(stripped)).
			in:   color + "abc" + colOff + revOn,
			want: "abc" + revOn,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := stripStyleKeepReverse(tc.in); got != tc.want {
				t.Fatalf("stripStyleKeepReverse = %q, want %q", got, tc.want)
			}
		})
	}
}
