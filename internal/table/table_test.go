package table

import (
	"fmt"
	"testing"

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
	// Should not panic
	m.EnsureCursorVisible()
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

	m.RowStyleFunc = func(index int, isCursor bool) *lipgloss.Style {
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

func makeRows(n int) []Row {
	rows := make([]Row, n)
	for i := range n {
		rows[i] = Row{fmt.Sprintf("row-%02d", i)}
	}
	return rows
}
