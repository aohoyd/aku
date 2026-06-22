package ui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/aohoyd/aku/internal/msgs"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/render"
	"github.com/aohoyd/aku/internal/table"
	"github.com/charmbracelet/x/ansi"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

func TestContentWidthWithANSI(t *testing.T) {
	// Simulate a row with ANSI-colored cell (like colored status)
	ansiCell := "\x1b[38;2;80;250;123mRunning\x1b[0m" // green "Running"
	rows := []table.Row{
		{"my-pod", ansiCell},
	}
	// Column 1 is the ANSI cell; visual width of "Running" is 7
	w := contentWidth(1, 6, rows) // headerWidth=6 ("STATUS")
	// Should be 7 + 3 = 10, NOT inflated by ANSI byte length
	if w != 10 {
		t.Fatalf("expected contentWidth 10 (visual), got %d", w)
	}
}

// testPlugin implements plugin.ResourcePlugin for testing
type testPlugin struct{}

func (p *testPlugin) Name() string      { return "pods" }
func (p *testPlugin) ShortName() string { return "po" }
func (p *testPlugin) GVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{Version: "v1", Resource: "pods"}
}
func (p *testPlugin) IsClusterScoped() bool { return false }
func (p *testPlugin) Columns() []plugin.Column {
	return []plugin.Column{
		{Title: "NAME", Flex: true},
		{Title: "STATUS", Width: 10},
	}
}
func (p *testPlugin) Row(obj *unstructured.Unstructured) []string {
	return []string{obj.GetName(), "Running"}
}
func (p *testPlugin) YAML(_ *unstructured.Unstructured) (render.Content, error) {
	return render.Content{}, nil
}
func (p *testPlugin) Describe(_ context.Context, _ *unstructured.Unstructured) (render.Content, error) {
	return render.Content{}, nil
}

func makeObj(name string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetName(name)
	return obj
}

func makeNsObj(name, namespace string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetName(name)
	obj.SetNamespace(namespace)
	return obj
}

func makeKindObj(name, kind string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetName(name)
	obj.SetKind(kind)
	return obj
}

func TestResourceListSetObjects(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 40, 10)
	objs := []*unstructured.Unstructured{makeObj("pod-a"), makeObj("pod-b")}
	rl.SetObjects(objs)
	if rl.Len() != 2 {
		t.Fatalf("expected 2 objects, got %d", rl.Len())
	}
}

func TestResourceListSelected(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 40, 10)
	objs := []*unstructured.Unstructured{makeObj("pod-a"), makeObj("pod-b")}
	rl.SetObjects(objs)
	sel := rl.Selected()
	if sel == nil || sel.GetName() != "pod-a" {
		t.Fatal("expected first object selected by default")
	}
}

func TestResourceListSelectedEmpty(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 40, 10)
	sel := rl.Selected()
	if sel != nil {
		t.Fatal("expected nil for empty list")
	}
}

func TestResourceListSetPlugin(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 40, 10)
	objs := []*unstructured.Unstructured{makeObj("pod-a")}
	rl.SetObjects(objs)

	newPlugin := &testPlugin{}
	rl.SetPlugin(newPlugin)
	if rl.Len() != 0 {
		t.Fatal("changing plugin should clear the list")
	}
	if rl.Plugin().Name() != "pods" {
		t.Fatal("plugin should be updated")
	}
}

func TestResourceListFocusBlur(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 40, 10)
	if !rl.Focused() {
		t.Fatal("should start focused")
	}
	rl.Blur()
	if rl.Focused() {
		t.Fatal("should be blurred")
	}
	rl.Focus()
	if !rl.Focused() {
		t.Fatal("should be focused again")
	}
}

// TestResourceListSelectionActiveAccessor exercises the public SelectionActive()
// accessor across every transition reconcileFocus drives. It covers the
// Focus()→BlurBorder() path specifically (Focus then BlurBorder with no
// intervening Blur), which is the actual production path reconcileFocus uses when
// handing input to the detail panel — distinct from the Blur()→BlurBorder()
// recovery path.
func TestResourceListSelectionActiveAccessor(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 40, 10)
	rl.Focus()
	if !rl.SelectionActive() {
		t.Fatal("after Focus(), SelectionActive() should be true")
	}
	rl.Blur()
	if rl.SelectionActive() {
		t.Fatal("after Blur(), SelectionActive() should be false")
	}
	// Blur()→BlurBorder() recovery path: BlurBorder restores the active cursor.
	rl.BlurBorder()
	if !rl.SelectionActive() {
		t.Fatal("after Blur→BlurBorder, SelectionActive() should be true")
	}
	// Focus()→BlurBorder() production path: detail panel takes input directly from
	// a focused list, without an intervening Blur. SelectionActive must stay true.
	rl.Focus()
	rl.BlurBorder()
	if !rl.SelectionActive() {
		t.Fatal("after Focus→BlurBorder, SelectionActive() should be true")
	}
	rl.Focus()
	if !rl.SelectionActive() {
		t.Fatal("after Focus(), SelectionActive() should be true")
	}
}

func TestPluginColumnsToTableColumnsNoRows(t *testing.T) {
	cols := []plugin.Column{
		{Title: "NAME", Flex: true},
		{Title: "STATUS", Width: 10},
	}
	result, _ := pluginColumnsToTableColumns(cols, 50, SortState{}, nil)
	if len(result) != 2 {
		t.Fatalf("expected 2 columns, got %d", len(result))
	}
	// With no rows, flex column width = len("NAME") + 3 = 7
	if result[0].Width != 7 {
		t.Fatalf("expected flex column width 7 (header only), got %d", result[0].Width)
	}
	if result[1].Width != 10 {
		t.Fatalf("expected fixed column width 10, got %d", result[1].Width)
	}
}

func TestPluginColumnsToTableColumnsWithRows(t *testing.T) {
	cols := []plugin.Column{
		{Title: "NAME", Flex: true},
		{Title: "STATUS", Width: 10},
	}
	rows := []table.Row{
		{"my-long-pod-name", "Running"},
		{"short", "Pending"},
	}
	result, _ := pluginColumnsToTableColumns(cols, 80, SortState{}, rows)
	// Flex width = max(len("NAME"), len("my-long-pod-name")) + 3 = 19
	if result[0].Width != 19 {
		t.Fatalf("expected flex column width 19 (content-based), got %d", result[0].Width)
	}
}

func TestPluginColumnsFlexUncapped(t *testing.T) {
	cols := []plugin.Column{
		{Title: "NAME", Flex: true},
		{Title: "AGE", Width: 8},
	}
	rows := []table.Row{
		{"a-very-very-very-very-very-very-very-very-long-name", "5d"},
	}
	// totalWidth=40, but flex should NOT be capped
	result, contentWidth := pluginColumnsToTableColumns(cols, 40, SortState{}, rows)
	// content width = len("a-very-...") + 3 = 54
	if result[0].Width != 54 {
		t.Fatalf("expected uncapped flex width 54, got %d", result[0].Width)
	}
	// contentWidth = sum of widths + padding (2 per col)
	expectedCW := 54 + 8 + 2*2
	if contentWidth != expectedCW {
		t.Fatalf("expected contentWidth %d, got %d", expectedCW, contentWidth)
	}
}

func TestPluginColumnsToTableColumnsReturnsContentWidth(t *testing.T) {
	cols := []plugin.Column{
		{Title: "NAME", Flex: true},
		{Title: "STATUS", Width: 10},
	}
	rows := []table.Row{
		{"my-pod", "Running"},
	}
	result, contentWidth := pluginColumnsToTableColumns(cols, 80, SortState{}, rows)
	// NAME flex = max(4, 6) + 3 = 9, STATUS = 10, padding = 2*2 = 4
	expectedCW := result[0].Width + result[1].Width + 2*len(cols)
	if contentWidth != expectedCW {
		t.Fatalf("expected contentWidth %d, got %d", expectedCW, contentWidth)
	}
}

func TestPluginColumnsWithSortIndicator(t *testing.T) {
	cols := []plugin.Column{
		{Title: "NAME", Flex: true},
		{Title: "STATUS", Width: 10},
	}
	state := SortState{Column: "NAME", Ascending: true}
	result, _ := pluginColumnsToTableColumns(cols, 50, state, nil)
	if result[0].Title != "NAME ▲" {
		t.Fatalf("expected 'NAME ▲', got %q", result[0].Title)
	}
	if result[1].Title != "STATUS" {
		t.Fatalf("expected 'STATUS' (no indicator), got %q", result[1].Title)
	}
}

func TestResourceListDefaultSort(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 40, 10)
	state := rl.SortState()
	if state.Column != "NAME" || !state.Ascending {
		t.Fatalf("expected default sort NAME ascending, got %+v", state)
	}
}

func TestResourceListSetSort(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 40, 10)
	objs := []*unstructured.Unstructured{makeObj("bravo"), makeObj("alpha")}
	rl.SetObjects(objs)

	// Default sort is NAME ascending, so objects should be reordered
	if rl.Selected().GetName() != "alpha" {
		t.Fatalf("expected 'alpha' first after default NAME sort, got %q", rl.Selected().GetName())
	}

	// Toggle to descending
	rl.SetSort("NAME")
	if rl.SortState().Ascending {
		t.Fatal("second press should toggle to descending")
	}
	if rl.Selected() == nil {
		t.Fatal("selected should not be nil after sort change")
	}
}

func TestResourceListSetSortNewColumn(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 40, 10)
	rl.SetSort("STATUS")
	state := rl.SortState()
	if state.Column != "STATUS" || !state.Ascending {
		t.Fatalf("expected STATUS ascending, got %+v", state)
	}
}

func TestResourceListSetPluginResetsSort(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 40, 10)
	rl.SetSort("AGE")
	rl.SetPlugin(&testPlugin{})
	state := rl.SortState()
	if state.Column != "NAME" || !state.Ascending {
		t.Fatalf("SetPlugin should reset sort to default, got %+v", state)
	}
}

func TestResourceListApplySearch(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 15)
	objs := []*unstructured.Unstructured{makeObj("nginx-pod"), makeObj("redis-pod"), makeObj("mysql-pod")}
	rl.SetObjects(objs)

	err := rl.ApplySearch("nginx", msgs.SearchModeSearch)
	if err != nil {
		t.Fatalf("ApplySearch should not error: %v", err)
	}
	if !rl.SearchActive() {
		t.Fatal("search should be active")
	}
	// All 3 rows should still be visible (search highlights, doesn't filter)
	if rl.Len() != 3 {
		t.Fatalf("search should not filter rows, expected 3, got %d", rl.Len())
	}
	// At least one visible row must carry highlight ANSI for the matched substring.
	rows := rl.table.Rows()
	highlighted := false
	for _, row := range rows {
		if rowHighlighted(row) {
			highlighted = true
			break
		}
	}
	if !highlighted {
		t.Fatalf("expected at least one highlighted row after ApplySearch; rows=%q", rows)
	}
}

// cellHighlighted reports whether a rendered cell carries either of the search
// highlight markers: reverse-video (cursor row) or themed color (other rows).
func cellHighlighted(cell string) bool {
	return strings.Contains(cell, highlightOn) || strings.Contains(cell, highlightMatchOn)
}

// cellCursorHighlighted reports whether a cell carries the cursor-row variant
// (reverse-video) specifically, and not merely the themed-color match variant.
func cellCursorHighlighted(cell string) bool {
	return strings.Contains(cell, highlightOn)
}

// cellMatchHighlighted reports whether a cell carries the non-cursor themed
// match variant specifically. highlightMatchOn is a distinct ANSI sequence from
// highlightOn, so this does not also match the cursor variant.
func cellMatchHighlighted(cell string) bool {
	return strings.Contains(cell, highlightMatchOn)
}

// rowHighlighted reports whether any cell in the row carries a highlight marker.
func rowHighlighted(row table.Row) bool {
	for _, cell := range row {
		if cellHighlighted(cell) {
			return true
		}
	}
	return false
}

// rowCursorHighlighted reports whether any cell uses the reverse-video variant.
func rowCursorHighlighted(row table.Row) bool {
	for _, cell := range row {
		if cellCursorHighlighted(cell) {
			return true
		}
	}
	return false
}

// rowMatchHighlighted reports whether any cell uses the themed-color variant.
func rowMatchHighlighted(row table.Row) bool {
	for _, cell := range row {
		if cellMatchHighlighted(cell) {
			return true
		}
	}
	return false
}

func makeSearchObjs(n int) []*unstructured.Unstructured {
	objs := make([]*unstructured.Unstructured, n)
	for i := range objs {
		objs[i] = makeObj(fmt.Sprintf("match-pod-%02d", i))
	}
	return objs
}

// TestResourceListSearchHighlightsWholeShortList covers the non-windowing path:
// when the list is shorter than the viewport, VisibleRange returns the full list
// and every matching row is highlighted (no row is left out of the window).
func TestResourceListSearchHighlightsWholeShortList(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 40, 20) // tall viewport, only 3 rows
	objs := []*unstructured.Unstructured{makeObj("match-a"), makeObj("match-b"), makeObj("match-c")}
	rl.SetObjects(objs)

	if err := rl.ApplySearch("match", msgs.SearchModeSearch); err != nil {
		t.Fatalf("ApplySearch should not error: %v", err)
	}

	start, end := rl.table.VisibleRange()
	if start != 0 || end != rl.Len() {
		t.Fatalf("short list should yield a full-list window [0,%d); got [%d,%d)", rl.Len(), start, end)
	}

	rows := rl.table.Rows()
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	for i, row := range rows {
		if !rowHighlighted(row) {
			t.Fatalf("matching row %d should be highlighted in non-windowing path; got %q", i, row)
		}
	}

	// The cursor row (row 0 by default) must use the reverse-video variant, while
	// the non-cursor matching rows must use the themed-color variant. Asserting the
	// specific variant catches a regression that applies the wrong highlight kind.
	if cursor := rl.table.Cursor(); cursor != 0 {
		t.Fatalf("expected cursor on row 0, got %d", cursor)
	}
	if !rowCursorHighlighted(rows[0]) {
		t.Fatalf("cursor row should use reverse-video variant; got %q", rows[0])
	}
	if rowMatchHighlighted(rows[0]) {
		t.Fatalf("cursor row should not use the themed-color variant; got %q", rows[0])
	}
	for i := 1; i < len(rows); i++ {
		if !rowMatchHighlighted(rows[i]) {
			t.Fatalf("non-cursor matching row %d should use themed-color variant; got %q", i, rows[i])
		}
		if rowCursorHighlighted(rows[i]) {
			t.Fatalf("non-cursor row %d should not use reverse-video variant; got %q", i, rows[i])
		}
	}
}

// TestResourceListSearchHighlightsOnlyVisibleWindow verifies that with an active
// search on a list taller than the viewport, rows inside the visible window carry
// highlight ANSI for the matched substring while a row known to be outside the
// window stays plain.
func TestResourceListSearchHighlightsOnlyVisibleWindow(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 40, 10) // table height ~7, far smaller than 50 rows
	rl.SetObjects(makeSearchObjs(50))

	if err := rl.ApplySearch("match", msgs.SearchModeSearch); err != nil {
		t.Fatalf("ApplySearch should not error: %v", err)
	}

	start, end := rl.table.VisibleRange()
	if end-start >= rl.Len() {
		t.Fatalf("expected a window smaller than the full list; window [%d,%d) of %d rows", start, end, rl.Len())
	}

	rows := rl.table.Rows()
	// Every row inside the visible window matches "match" and must be highlighted.
	for i := start; i < end && i < len(rows); i++ {
		if !rowHighlighted(rows[i]) {
			t.Fatalf("visible row %d should be highlighted; got %q", i, rows[i])
		}
	}

	// A row outside the window must be plain.
	outside := end
	if outside >= len(rows) {
		outside = start - 1
	}
	if outside < 0 || outside >= len(rows) {
		t.Fatalf("could not pick an out-of-window row (window [%d,%d), %d rows)", start, end, len(rows))
	}
	if rowHighlighted(rows[outside]) {
		t.Fatalf("out-of-window row %d should be plain; got %q", outside, rows[outside])
	}

	// Windowing invariant: even though EVERY row matches "match", only the rows
	// inside [start,end) may be highlighted. If applyVisibleHighlights ignored the
	// window and painted all rows, this count would be rl.Len() (50) instead of
	// the window size, and this assertion would fail.
	highlighted := 0
	for i := range rows {
		if rowHighlighted(rows[i]) {
			highlighted++
		}
	}
	if highlighted != end-start {
		t.Fatalf("expected exactly %d highlighted rows (window [%d,%d)), got %d of %d total",
			end-start, start, end, highlighted, len(rows))
	}
}

// TestResourceListSearchHighlightsAfterScroll verifies that paging/scrolling the
// window re-highlights the newly visible rows (no plain rows showing inside the
// window for cells that contain the match).
func TestResourceListSearchHighlightsAfterScroll(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 40, 10)
	rl.SetObjects(makeSearchObjs(50))

	if err := rl.ApplySearch("match", msgs.SearchModeSearch); err != nil {
		t.Fatalf("ApplySearch should not error: %v", err)
	}

	// Record the initial window (top of the list) before scrolling.
	initStart, initEnd := rl.table.VisibleRange()

	// Jump far down the list so the visible window shifts away from the top.
	rl.SetCursor(45)

	start, end := rl.table.VisibleRange()
	if start == 0 {
		t.Fatalf("expected the window to have scrolled off the top; window [%d,%d)", start, end)
	}

	rows := rl.table.Rows()
	for i := start; i < end && i < len(rows); i++ {
		if !rowHighlighted(rows[i]) {
			t.Fatalf("newly visible row %d should be highlighted after scroll; got %q", i, rows[i])
		}
	}

	// Re-highlight contract: a row that was OUTSIDE the initial window but is now
	// INSIDE the post-scroll window must have been freshly highlighted by the
	// scroll. This proves newly-scrolled-in rows get painted, not just rows that
	// happened to be highlighted at the top before the jump.
	freshRow := -1
	for i := start; i < end && i < len(rows); i++ {
		if i < initStart || i >= initEnd {
			freshRow = i
			break
		}
	}
	if freshRow == -1 {
		t.Fatalf("post-scroll window [%d,%d) did not move outside initial window [%d,%d); cannot test re-highlight",
			start, end, initStart, initEnd)
	}
	if !rowHighlighted(rows[freshRow]) {
		t.Fatalf("row %d (outside initial window [%d,%d), inside post-scroll window [%d,%d)) should be freshly highlighted; got %q",
			freshRow, initStart, initEnd, start, end, rows[freshRow])
	}
}

// TestResourceListFilterHidesNonMatching verifies filter mode hides non-matching
// rows. Filter mode does NOT inline-highlight (that is search-only), so the test
// also asserts the surviving visible rows carry no spurious highlight ANSI.
func TestResourceListFilterHidesNonMatching(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 15)
	objs := []*unstructured.Unstructured{makeObj("nginx-pod"), makeObj("redis-pod"), makeObj("mysql-pod")}
	rl.SetObjects(objs)

	if err := rl.ApplySearch("nginx", msgs.SearchModeFilter); err != nil {
		t.Fatalf("ApplySearch filter should not error: %v", err)
	}
	if rl.Len() != 1 {
		t.Fatalf("filter should hide non-matching rows, expected 1, got %d", rl.Len())
	}
	if rl.Selected().GetName() != "nginx-pod" {
		t.Fatalf("expected nginx-pod visible, got %q", rl.Selected().GetName())
	}
	// Filter mode hides non-matches but must not inline-highlight visible rows.
	for i, row := range rl.table.Rows() {
		if rowHighlighted(row) {
			t.Fatalf("filter-mode row %d should not carry inline highlight ANSI; got %q", i, row)
		}
	}
}

func TestResourceListApplyFilter(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 15)
	objs := []*unstructured.Unstructured{makeObj("nginx-pod"), makeObj("redis-pod"), makeObj("mysql-pod")}
	rl.SetObjects(objs)

	err := rl.ApplySearch("nginx", msgs.SearchModeFilter)
	if err != nil {
		t.Fatalf("ApplySearch filter should not error: %v", err)
	}
	// Only nginx-pod should be visible
	if rl.Len() != 1 {
		t.Fatalf("filter should leave 1 row, got %d", rl.Len())
	}
	if rl.Selected().GetName() != "nginx-pod" {
		t.Fatalf("expected nginx-pod, got %q", rl.Selected().GetName())
	}
}

func TestResourceListClearSearch(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 15)
	objs := []*unstructured.Unstructured{makeObj("nginx-pod"), makeObj("redis-pod")}
	rl.SetObjects(objs)

	rl.ApplySearch("nginx", msgs.SearchModeFilter)
	if rl.Len() != 1 {
		t.Fatalf("expected 1, got %d", rl.Len())
	}

	rl.ClearFilter()
	if rl.AnyActive() {
		t.Fatal("should be inactive after clear")
	}
	if rl.Len() != 2 {
		t.Fatalf("all rows should be restored, expected 2, got %d", rl.Len())
	}
}

func TestResourceListSearchInvalidRegex(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 15)
	objs := []*unstructured.Unstructured{makeObj("pod-a")}
	rl.SetObjects(objs)

	err := rl.ApplySearch("[invalid", msgs.SearchModeSearch)
	if err == nil {
		t.Fatal("invalid regex should return error")
	}
	if rl.SearchActive() {
		t.Fatal("search should not be active after invalid regex")
	}
}

func TestResourceListSearchNext(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 15)
	objs := []*unstructured.Unstructured{makeObj("alpha"), makeObj("beta-pod"), makeObj("gamma"), makeObj("delta-pod")}
	rl.SetObjects(objs)

	rl.ApplySearch("pod", msgs.SearchModeSearch)
	// Cursor should jump to first match
	sel := rl.Selected()
	if sel == nil || sel.GetName() != "beta-pod" {
		name := ""
		if sel != nil {
			name = sel.GetName()
		}
		t.Fatalf("expected cursor on first match 'beta-pod', got %q", name)
	}

	rl.SearchNext()
	sel = rl.Selected()
	if sel == nil || sel.GetName() != "delta-pod" {
		name := ""
		if sel != nil {
			name = sel.GetName()
		}
		t.Fatalf("expected cursor on next match 'delta-pod', got %q", name)
	}

	// Wrap around
	rl.SearchNext()
	sel = rl.Selected()
	if sel == nil || sel.GetName() != "beta-pod" {
		name := ""
		if sel != nil {
			name = sel.GetName()
		}
		t.Fatalf("expected cursor to wrap to 'beta-pod', got %q", name)
	}
}

func TestResourceListSearchPrev(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 15)
	objs := []*unstructured.Unstructured{makeObj("alpha"), makeObj("beta-pod"), makeObj("gamma"), makeObj("delta-pod")}
	rl.SetObjects(objs)

	rl.ApplySearch("pod", msgs.SearchModeSearch)
	// Should be on first match: beta-pod
	rl.SearchPrev()
	sel := rl.Selected()
	if sel == nil || sel.GetName() != "delta-pod" {
		name := ""
		if sel != nil {
			name = sel.GetName()
		}
		t.Fatalf("expected cursor to wrap backwards to 'delta-pod', got %q", name)
	}
}

func TestResourceListSearchNextAfterManualCursorMove(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 15)
	// Matches at indices 1, 3, 5
	objs := []*unstructured.Unstructured{
		makeObj("alpha"), makeObj("beta-pod"), makeObj("gamma"),
		makeObj("delta-pod"), makeObj("epsilon"), makeObj("zeta-pod"),
	}
	rl.SetObjects(objs)

	rl.ApplySearch("pod", msgs.SearchModeSearch)
	// Cursor on beta-pod (index 1)

	// Manually move cursor to index 4 (epsilon), simulating j/k navigation
	rl.table.SetCursor(4)

	// SearchNext should find next match after cursor pos 4 → zeta-pod (index 5)
	rl.SearchNext()
	sel := rl.Selected()
	if sel == nil || sel.GetName() != "zeta-pod" {
		name := ""
		if sel != nil {
			name = sel.GetName()
		}
		t.Fatalf("expected 'zeta-pod' after manual move, got %q", name)
	}

	// Move cursor back to index 0, SearchPrev should wrap to zeta-pod (index 5)
	rl.table.SetCursor(0)
	rl.SearchPrev()
	sel = rl.Selected()
	if sel == nil || sel.GetName() != "zeta-pod" {
		name := ""
		if sel != nil {
			name = sel.GetName()
		}
		t.Fatalf("expected 'zeta-pod' wrapping backwards from 0, got %q", name)
	}
}

func TestResourceListFilterReappliedOnSetObjects(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 15)
	objs := []*unstructured.Unstructured{makeObj("nginx-pod"), makeObj("redis-pod")}
	rl.SetObjects(objs)

	rl.ApplySearch("nginx", msgs.SearchModeFilter)
	if rl.Len() != 1 {
		t.Fatalf("expected 1, got %d", rl.Len())
	}

	// Simulate k8s data update with a new object that matches
	newObjs := []*unstructured.Unstructured{makeObj("nginx-pod"), makeObj("redis-pod"), makeObj("nginx-deploy")}
	rl.SetObjects(newObjs)
	if rl.Len() != 2 {
		t.Fatalf("filter should include new matching object, expected 2, got %d", rl.Len())
	}
}

func TestResourceListSetPluginClearsSearch(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 15)
	objs := []*unstructured.Unstructured{makeObj("nginx-pod")}
	rl.SetObjects(objs)

	rl.ApplySearch("nginx", msgs.SearchModeFilter)
	rl.SetPlugin(&testPlugin{})
	if rl.AnyActive() {
		t.Fatal("SetPlugin should clear all search state")
	}
}

func TestResourceListFilterAndSearchIndependent(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 15)
	objs := []*unstructured.Unstructured{
		makeObj("nginx-pod"), makeObj("redis-pod"), makeObj("nginx-deploy"),
	}
	rl.SetObjects(objs)

	// Apply filter first
	rl.ApplySearch("nginx", msgs.SearchModeFilter)
	if rl.Len() != 2 {
		t.Fatalf("filter should show 2 nginx objects, got %d", rl.Len())
	}

	// Apply search on top of filter
	err := rl.ApplySearch("pod", msgs.SearchModeSearch)
	if err != nil {
		t.Fatalf("search should not error: %v", err)
	}

	// Both should be active
	if !rl.SearchActive() {
		t.Fatal("search should be active")
	}
	if !rl.FilterActive() {
		t.Fatal("filter should be active")
	}
	if !rl.AnyActive() {
		t.Fatal("AnyActive should be true")
	}

	// Filter should still show only 2 rows (search doesn't change row count)
	if rl.Len() != 2 {
		t.Fatalf("search should not change filtered row count, expected 2, got %d", rl.Len())
	}
}

func TestResourceListLayeredClear(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 15)
	objs := []*unstructured.Unstructured{makeObj("nginx-pod"), makeObj("redis-pod")}
	rl.SetObjects(objs)

	rl.ApplySearch("nginx", msgs.SearchModeFilter)
	rl.ApplySearch("pod", msgs.SearchModeSearch)

	// Clear search first
	rl.ClearSearch()
	if rl.SearchActive() {
		t.Fatal("search should be cleared")
	}
	if !rl.FilterActive() {
		t.Fatal("filter should still be active")
	}
	if rl.Len() != 1 {
		t.Fatalf("filter should still apply, expected 1, got %d", rl.Len())
	}

	// Clear filter
	rl.ClearFilter()
	if rl.FilterActive() {
		t.Fatal("filter should be cleared")
	}
	if rl.AnyActive() {
		t.Fatal("nothing should be active")
	}
	if rl.Len() != 2 {
		t.Fatalf("all rows should be restored, expected 2, got %d", rl.Len())
	}
}

func TestResourceListAllNamespacesColumn(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 15)
	rl.SetNamespace("")

	objs := []*unstructured.Unstructured{
		makeNsObj("pod-a", "staging"),
		makeNsObj("pod-b", "production"),
	}
	rl.SetObjects(objs)

	// The table should have 3 columns: NAMESPACE, NAME, STATUS
	rows := rl.table.Rows()
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	// First cell of each row should be the namespace
	if rows[0][0] != "staging" {
		t.Fatalf("expected first cell 'staging', got %q", rows[0][0])
	}
	if rows[1][0] != "production" {
		t.Fatalf("expected first cell 'production', got %q", rows[1][0])
	}
}

func TestResourceListNoNamespaceColumnForSpecificNs(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 15)
	rl.SetNamespace("default")

	objs := []*unstructured.Unstructured{makeNsObj("pod-a", "default")}
	rl.SetObjects(objs)

	rows := rl.table.Rows()
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	// First cell should be the pod name (no namespace column)
	if rows[0][0] != "pod-a" {
		t.Fatalf("expected first cell 'pod-a', got %q", rows[0][0])
	}
}

func TestResourceListAllNamespacesFilterMatchesNamespace(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 15)
	rl.SetNamespace("")

	objs := []*unstructured.Unstructured{
		makeNsObj("pod-a", "staging"),
		makeNsObj("pod-b", "production"),
	}
	rl.SetObjects(objs)

	// Filter by namespace name "staging"
	err := rl.ApplySearch("staging", msgs.SearchModeFilter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rl.Len() != 1 {
		t.Fatalf("expected 1 filtered result, got %d", rl.Len())
	}
	if rl.Selected().GetNamespace() != "staging" {
		t.Fatalf("expected staging pod, got %q", rl.Selected().GetNamespace())
	}
}

func TestResourceListAllNamespacesCursorRestore(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 15)
	rl.SetNamespace("")

	// Two pods with same name in different namespaces
	objs := []*unstructured.Unstructured{
		makeNsObj("nginx", "production"),
		makeNsObj("nginx", "staging"),
	}
	rl.SetObjects(objs)

	// Move cursor to second item (staging/nginx)
	rl.table.SetCursor(1)

	// Re-set objects (simulating informer update)
	rl.SetObjects(objs)

	// Cursor should restore to staging/nginx (index 1), not production/nginx (index 0)
	sel := rl.Selected()
	if sel == nil {
		t.Fatal("expected a selected object")
	}
	if sel.GetNamespace() != "staging" {
		t.Fatalf("expected cursor restored to staging/nginx, got %s/%s", sel.GetNamespace(), sel.GetName())
	}
}

func TestResourceListCursorRestoreByKind(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 15)
	rl.SetNamespace("default")

	// Two objects with the same name but different kinds
	objs := []*unstructured.Unstructured{
		makeKindObj("my-app", "Deployment"),
		makeKindObj("my-app", "Service"),
	}
	rl.SetObjects(objs)

	// Move cursor to second item (Service/my-app)
	rl.table.SetCursor(1)

	// Re-set objects (simulating informer update or post-pop refresh)
	rl.SetObjects(objs)

	// Cursor should restore to Service/my-app (index 1), not Deployment/my-app (index 0)
	sel := rl.Selected()
	if sel == nil {
		t.Fatal("expected a selected object")
	}
	if sel.GetKind() != "Service" {
		t.Fatalf("expected cursor restored to Service/my-app, got %s/%s", sel.GetKind(), sel.GetName())
	}
}

func TestResourceListSwitchFromAllNamespacesToSpecific(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 15)
	rl.SetNamespace("")

	objs := []*unstructured.Unstructured{
		makeNsObj("pod-a", "staging"),
		makeNsObj("pod-b", "production"),
	}
	rl.SetObjects(objs)

	// Verify NAMESPACE column is present (3 cells per row)
	if len(rl.table.Rows()[0]) != 3 {
		t.Fatalf("expected 3 cells in all-ns mode, got %d", len(rl.table.Rows()[0]))
	}

	// Switch to specific namespace — must not panic when column count changes
	rl.SetNamespace("default")
	rl.SetObjects(nil)

	specificObjs := []*unstructured.Unstructured{makeNsObj("pod-c", "default")}
	rl.SetObjects(specificObjs)

	// Should now have 2 cells per row (no NAMESPACE column)
	if len(rl.table.Rows()) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rl.table.Rows()))
	}
	if len(rl.table.Rows()[0]) != 2 {
		t.Fatalf("expected 2 cells in specific-ns mode, got %d", len(rl.table.Rows()[0]))
	}
}

func TestFocusMakesCursorVisible(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 10)
	objs := make([]*unstructured.Unstructured, 50)
	for i := range objs {
		objs[i] = makeObj(fmt.Sprintf("pod-%03d", i))
	}
	rl.SetObjects(objs)

	// Navigate cursor deep into the list
	for range 30 {
		rl.CursorDown()
	}
	if rl.Cursor() != 30 {
		t.Fatalf("expected cursor at 30, got %d", rl.Cursor())
	}

	// Blur and re-focus — cursor position must be preserved
	rl.Blur()
	rl.Focus()

	if rl.Cursor() != 30 {
		t.Fatalf("expected cursor at 30 after blur/focus, got %d", rl.Cursor())
	}
	sel := rl.Selected()
	if sel == nil || sel.GetName() != "pod-030" {
		name := ""
		if sel != nil {
			name = sel.GetName()
		}
		t.Fatalf("expected pod-030 selected, got %q", name)
	}
}

func TestParentSnap(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 20)
	// Before drilldown, ParentSnap should report ok=false
	if snap, ok := rl.ParentSnap(); ok {
		t.Fatalf("expected no ParentSnap before drilldown, got %+v", snap)
	}
	// Push a nav entry
	child := &testPlugin{}
	rl.PushNav(child, nil, "my-deployment", "uid-123", "", "")
	snap, ok := rl.ParentSnap()
	if !ok {
		t.Fatal("expected ParentSnap after drilldown")
	}
	if snap.ParentName != "my-deployment" {
		t.Fatalf("expected ParentName 'my-deployment', got %q", snap.ParentName)
	}
	if snap.ParentUID != "uid-123" {
		t.Fatalf("expected ParentUID 'uid-123', got %q", snap.ParentUID)
	}
}

func TestParentSnapKindAndAPIVersion(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 20)
	if snap, ok := rl.ParentSnap(); ok {
		t.Fatalf("expected no ParentSnap before drilldown, got %+v", snap)
	}
	child := &testPlugin{}
	rl.PushNav(child, nil, "my-app", "", "v1", "Secret")
	snap, ok := rl.ParentSnap()
	if !ok {
		t.Fatal("expected ParentSnap after drilldown")
	}
	if snap.ParentKind != "Secret" {
		t.Fatalf("expected ParentKind 'Secret', got %q", snap.ParentKind)
	}
	if snap.ParentAPIVersion != "v1" {
		t.Fatalf("expected ParentAPIVersion 'v1', got %q", snap.ParentAPIVersion)
	}
}

func TestFocusAfterBlurBorderKeepsCursorVisible(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 10)
	objs := make([]*unstructured.Unstructured, 50)
	for i := range objs {
		objs[i] = makeObj(fmt.Sprintf("pod-%03d", i))
	}
	rl.SetObjects(objs)

	for range 25 {
		rl.CursorDown()
	}

	rl.BlurBorder()
	rl.Focus()

	if rl.Cursor() != 25 {
		t.Fatalf("expected cursor at 25 after BlurBorder/Focus, got %d", rl.Cursor())
	}
}

// --- Selection tests ---

func makeUIDObj(name, uid string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetName(name)
	obj.SetUID(types.UID(uid))
	return obj
}

func TestToggleSelect(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 15)
	objs := []*unstructured.Unstructured{
		makeUIDObj("pod-a", "uid-a"),
		makeUIDObj("pod-b", "uid-b"),
		makeUIDObj("pod-c", "uid-c"),
	}
	rl.SetObjects(objs)

	rl.ToggleSelect()
	if !rl.HasSelection() {
		t.Fatal("expected selection after toggle")
	}
	if rl.SelectionCount() != 1 {
		t.Fatalf("expected 1 selected, got %d", rl.SelectionCount())
	}

	// Toggle same row again should deselect
	rl.table.SetCursor(0)
	rl.ToggleSelect()
	if rl.HasSelection() {
		t.Fatal("expected no selection after second toggle")
	}
}

func TestSelectAll(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 15)
	objs := []*unstructured.Unstructured{
		makeUIDObj("pod-a", "uid-a"),
		makeUIDObj("pod-b", "uid-b"),
	}
	rl.SetObjects(objs)

	rl.SelectAll()
	if rl.SelectionCount() != 2 {
		t.Fatalf("expected 2 selected, got %d", rl.SelectionCount())
	}

	// Second call should deselect all
	rl.SelectAll()
	if rl.HasSelection() {
		t.Fatal("expected deselect all on second call")
	}
}

func TestClearSelection(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 15)
	objs := []*unstructured.Unstructured{makeUIDObj("pod-a", "uid-a")}
	rl.SetObjects(objs)
	rl.ToggleSelect()
	rl.ClearSelection()
	if rl.HasSelection() {
		t.Fatal("expected no selection after clear")
	}
}

func TestSelectedObjects(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 15)
	objs := []*unstructured.Unstructured{
		makeUIDObj("pod-a", "uid-a"),
		makeUIDObj("pod-b", "uid-b"),
		makeUIDObj("pod-c", "uid-c"),
	}
	rl.SetObjects(objs)

	// Select first and third
	rl.ToggleSelect() // selects pod-a, cursor stays at 0
	rl.table.SetCursor(2)
	rl.ToggleSelect() // selects pod-c

	selected := rl.SelectedObjects()
	if len(selected) != 2 {
		t.Fatalf("expected 2 selected objects, got %d", len(selected))
	}
	if selected[0].GetName() != "pod-a" {
		t.Fatalf("expected pod-a first, got %s", selected[0].GetName())
	}
	if selected[1].GetName() != "pod-c" {
		t.Fatalf("expected pod-c second, got %s", selected[1].GetName())
	}
}

func TestSelectionClearedOnSetPlugin(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 15)
	objs := []*unstructured.Unstructured{makeUIDObj("pod-a", "uid-a")}
	rl.SetObjects(objs)
	rl.ToggleSelect()
	rl.SetPlugin(&testPlugin{})
	if rl.HasSelection() {
		t.Fatal("SetPlugin should clear selection")
	}
}

func TestSelectionPrunedOnSetObjects(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 15)
	objs := []*unstructured.Unstructured{
		makeUIDObj("pod-a", "uid-a"),
		makeUIDObj("pod-b", "uid-b"),
	}
	rl.SetObjects(objs)

	rl.SelectAll()
	if rl.SelectionCount() != 2 {
		t.Fatalf("expected 2, got %d", rl.SelectionCount())
	}

	// Simulate informer update removing pod-b
	newObjs := []*unstructured.Unstructured{makeUIDObj("pod-a", "uid-a")}
	rl.SetObjects(newObjs)

	if rl.SelectionCount() != 1 {
		t.Fatalf("expected 1 after prune, got %d", rl.SelectionCount())
	}
	selected := rl.SelectedObjects()
	if len(selected) != 1 || selected[0].GetName() != "pod-a" {
		t.Fatal("expected only pod-a after prune")
	}
}

func TestSelectionClearedOnPushNav(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 15)
	objs := []*unstructured.Unstructured{makeUIDObj("pod-a", "uid-a")}
	rl.SetObjects(objs)
	rl.ToggleSelect()
	rl.PushNav(&testPlugin{}, nil, "parent", "uid", "", "")
	if rl.HasSelection() {
		t.Fatal("PushNav should clear selection")
	}
}

func TestSelectionClearedOnPopNav(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 15)
	objs := []*unstructured.Unstructured{makeUIDObj("pod-a", "uid-a")}
	rl.SetObjects(objs)
	rl.PushNav(&testPlugin{}, []*unstructured.Unstructured{makeUIDObj("child", "uid-c")}, "parent", "uid", "", "")
	rl.ToggleSelect() // select child
	rl.PopNav()
	if rl.HasSelection() {
		t.Fatal("PopNav should clear selection")
	}
}

func TestResourceListScrollLeftRight(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 40, 10)
	// Use long names to force content wider than viewport
	objs := []*unstructured.Unstructured{
		makeObj("a-very-very-very-very-long-pod-name"),
	}
	rl.SetObjects(objs)

	// Content should exceed viewport width (40-2=38)
	rl.ScrollRight()
	if rl.xOffset != 8 {
		t.Fatalf("expected xOffset 8 after ScrollRight, got %d", rl.xOffset)
	}

	rl.ScrollLeft()
	if rl.xOffset != 0 {
		t.Fatalf("expected xOffset 0 after ScrollLeft, got %d", rl.xOffset)
	}

	// Should not go below 0
	rl.ScrollLeft()
	if rl.xOffset != 0 {
		t.Fatalf("expected xOffset clamped to 0, got %d", rl.xOffset)
	}
}

func TestResourceListScrollPreservedOnSetObjects(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 40, 10)
	objs := []*unstructured.Unstructured{
		makeObj("a-very-very-very-very-long-pod-name"),
	}
	rl.SetObjects(objs)
	rl.ScrollRight()
	if rl.xOffset == 0 {
		t.Fatal("xOffset should be non-zero after scroll")
	}

	// SetObjects should preserve xOffset
	saved := rl.xOffset
	rl.SetObjects(objs)
	if rl.xOffset != saved {
		t.Fatalf("expected xOffset preserved at %d on SetObjects, got %d", saved, rl.xOffset)
	}
}

func TestResourceListScrollResetOnSetPlugin(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 40, 10)
	objs := []*unstructured.Unstructured{
		makeObj("a-very-very-very-very-long-pod-name"),
	}
	rl.SetObjects(objs)
	rl.ScrollRight()

	rl.SetPlugin(&testPlugin{})
	if rl.xOffset != 0 {
		t.Fatalf("expected xOffset reset to 0 on SetPlugin, got %d", rl.xOffset)
	}
}

func TestResourceListScrollResetOnSetSize(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 40, 10)
	objs := []*unstructured.Unstructured{
		makeObj("a-very-very-very-very-long-pod-name"),
	}
	rl.SetObjects(objs)
	rl.ScrollRight()

	rl.SetSize(80, 20)
	if rl.xOffset != 0 {
		t.Fatalf("expected xOffset reset to 0 on SetSize, got %d", rl.xOffset)
	}
}

// TestResourceListScrollClampedAcrossBorderlessToggle asserts horizontal scroll
// stays within the valid clamp across a SetBorderless toggle. The maxOffset
// anchor is contentWidth-tableWidth(), and tableWidth() differs by 2 between
// bordered (width-2) and borderless (width) modes — so a stale xOffset could in
// principle point past the right edge for the active width. SetBorderless re-runs
// the layout (via SetSize), which re-clamps xOffset to 0; after toggling back to
// bordered, a fresh scroll must still respect the bordered-mode maxOffset and
// never exceed it.
func TestResourceListScrollClampedAcrossBorderlessToggle(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 40, 10)
	objs := []*unstructured.Unstructured{
		makeObj("a-very-very-very-very-long-pod-name"),
	}
	rl.SetObjects(objs)

	// Scroll right in bordered mode to a non-zero offset.
	rl.ScrollRight()
	if rl.xOffset == 0 {
		t.Fatal("precondition: xOffset should be non-zero after ScrollRight in bordered mode")
	}

	maxBordered := max(0, rl.contentWidth-rl.tableWidth())
	if rl.xOffset > maxBordered {
		t.Fatalf("bordered xOffset %d exceeds maxOffset %d", rl.xOffset, maxBordered)
	}

	// Toggle to borderless and back; each toggle re-runs layout and re-clamps.
	rl.SetBorderless(true)
	if rl.xOffset != 0 {
		t.Fatalf("xOffset should re-clamp to 0 on SetBorderless(true), got %d", rl.xOffset)
	}
	maxBorderless := max(0, rl.contentWidth-rl.tableWidth())
	if rl.xOffset > maxBorderless {
		t.Fatalf("borderless xOffset %d exceeds maxOffset %d", rl.xOffset, maxBorderless)
	}

	rl.SetBorderless(false)
	if rl.xOffset != 0 {
		t.Fatalf("xOffset should re-clamp to 0 on SetBorderless(false), got %d", rl.xOffset)
	}

	// A fresh scroll in the restored bordered mode must respect the bordered
	// maxOffset and never run past the right edge.
	for i := 0; i < 20; i++ {
		rl.ScrollRight()
	}
	maxAfter := max(0, rl.contentWidth-rl.tableWidth())
	if rl.xOffset > maxAfter {
		t.Fatalf("xOffset %d scrolled past the right edge (maxOffset %d) after toggle", rl.xOffset, maxAfter)
	}
}

func TestColumnWidthsCachedWhenRowCountSame(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 15)
	objs := []*unstructured.Unstructured{makeObj("pod-a"), makeObj("pod-b")}
	rl.SetObjects(objs)

	// After SetObjects, the cache should be populated
	if rl.cachedColumnWidths == nil {
		t.Fatal("expected cachedColumnWidths to be populated after SetObjects")
	}
	if rl.cachedRowCount != 2 {
		t.Fatalf("expected cachedRowCount 2, got %d", rl.cachedRowCount)
	}
	savedCols := rl.cachedColumnWidths
	savedCW := rl.cachedContentWidth

	// Set objects with same count — cache should be reused (same slice pointer)
	objs2 := []*unstructured.Unstructured{makeObj("pod-c"), makeObj("pod-d")}
	rl.SetObjects(objs2)

	if rl.cachedRowCount != 2 {
		t.Fatalf("expected cachedRowCount still 2, got %d", rl.cachedRowCount)
	}
	// Column widths should be the same cached slice (pointer equality)
	if &rl.cachedColumnWidths[0] != &savedCols[0] {
		t.Fatal("expected cachedColumnWidths to be reused when row count is unchanged")
	}
	if rl.cachedContentWidth != savedCW {
		t.Fatalf("expected cachedContentWidth %d, got %d", savedCW, rl.cachedContentWidth)
	}
}

func TestColumnWidthsRecomputedWhenRowCountChanges(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 15)
	objs := []*unstructured.Unstructured{makeObj("pod-a"), makeObj("pod-b")}
	rl.SetObjects(objs)

	if rl.cachedRowCount != 2 {
		t.Fatalf("expected cachedRowCount 2, got %d", rl.cachedRowCount)
	}
	savedCols := rl.cachedColumnWidths

	// Set objects with different count — cache should be invalidated and recomputed
	objs2 := []*unstructured.Unstructured{makeObj("pod-a"), makeObj("pod-b"), makeObj("pod-c-with-a-longer-name")}
	rl.SetObjects(objs2)

	if rl.cachedRowCount != 3 {
		t.Fatalf("expected cachedRowCount 3, got %d", rl.cachedRowCount)
	}
	// Column widths should have been recomputed (different slice)
	if &rl.cachedColumnWidths[0] == &savedCols[0] {
		t.Fatal("expected cachedColumnWidths to be recomputed when row count changes")
	}
}

func TestResourceListRowAtYAboveChrome(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 40, 20)
	rl.SetObjects([]*unstructured.Unstructured{
		makeObj("pod-a"), makeObj("pod-b"), makeObj("pod-c"),
	})

	// y=0 is the top border (with injected title) → -1
	if got := rl.RowAtY(0); got != -1 {
		t.Fatalf("y=0 (border): expected -1, got %d", got)
	}
	// y=1 is the table's header row (border chrome accounts for y=0, then
	// table.RowAtY sees its own y=0 which is its header) → -1
	if got := rl.RowAtY(1); got != -1 {
		t.Fatalf("y=1 (table header): expected -1, got %d", got)
	}
}

func TestResourceListRowAtYFirstDataRow(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 40, 20)
	rl.SetObjects([]*unstructured.Unstructured{
		makeObj("pod-a"), makeObj("pod-b"), makeObj("pod-c"),
	})

	// y=2 is just past top-border (y=0) + table header (y=1) → first data row
	if got := rl.RowAtY(2); got != 0 {
		t.Fatalf("y=2 (first data row): expected 0, got %d", got)
	}
	if got := rl.RowAtY(3); got != 1 {
		t.Fatalf("y=3: expected 1, got %d", got)
	}
	if got := rl.RowAtY(4); got != 2 {
		t.Fatalf("y=4: expected 2, got %d", got)
	}
}

func TestResourceListRowAtYPastLastRow(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 40, 20)
	rl.SetObjects([]*unstructured.Unstructured{
		makeObj("pod-a"), makeObj("pod-b"),
	})

	// 2 rows: valid at y=2,3. y=4 → past last → -1
	if got := rl.RowAtY(4); got != -1 {
		t.Fatalf("y past last row: expected -1, got %d", got)
	}
	if got := rl.RowAtY(100); got != -1 {
		t.Fatalf("y=100: expected -1, got %d", got)
	}
}

func TestResourceListRowAtYNegative(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 40, 20)
	rl.SetObjects([]*unstructured.Unstructured{makeObj("pod-a")})
	if got := rl.RowAtY(-1); got != -1 {
		t.Fatalf("y=-1: expected -1, got %d", got)
	}
}

func TestResourceListScrollWheelDownAdvancesCursor(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 40, 20)
	rl.SetObjects([]*unstructured.Unstructured{
		makeObj("pod-a"), makeObj("pod-b"), makeObj("pod-c"),
	})
	before := rl.Cursor()
	rl.ScrollWheel(tea.MouseWheelDown)
	if got := rl.Cursor(); got != before+1 {
		t.Fatalf("cursor after wheel down: expected %d, got %d", before+1, got)
	}
}

func TestResourceListScrollWheelDownAtBottomStays(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 40, 20)
	rl.SetObjects([]*unstructured.Unstructured{
		makeObj("pod-a"), makeObj("pod-b"),
	})
	rl.GotoBottom()
	bottom := rl.Cursor()
	rl.ScrollWheel(tea.MouseWheelDown)
	if got := rl.Cursor(); got != bottom {
		t.Fatalf("cursor at bottom after wheel down: expected %d, got %d", bottom, got)
	}
}

func TestResourceListScrollWheelUpAtTopStays(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 40, 20)
	rl.SetObjects([]*unstructured.Unstructured{
		makeObj("pod-a"), makeObj("pod-b"),
	})
	rl.GotoTop()
	if rl.Cursor() != 0 {
		t.Fatalf("expected cursor at 0 before wheel up, got %d", rl.Cursor())
	}
	rl.ScrollWheel(tea.MouseWheelUp)
	if got := rl.Cursor(); got != 0 {
		t.Fatalf("cursor at top after wheel up: expected 0, got %d", got)
	}
}

func TestResourceListScrollWheelLeftRightNoOp(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 40, 20)
	rl.SetObjects([]*unstructured.Unstructured{
		makeObj("pod-a"), makeObj("pod-b"), makeObj("pod-c"),
	})
	// Advance once so we're not at top (to prove left/right don't move).
	rl.CursorDown()
	before := rl.Cursor()
	rl.ScrollWheel(tea.MouseWheelLeft)
	if got := rl.Cursor(); got != before {
		t.Fatalf("wheel left changed cursor: before=%d after=%d", before, got)
	}
	rl.ScrollWheel(tea.MouseWheelRight)
	if got := rl.Cursor(); got != before {
		t.Fatalf("wheel right changed cursor: before=%d after=%d", before, got)
	}
}

// rlContainsBorderRune reports whether s contains any of the rounded-border box
// drawing runes used by the bordered render path.
func rlContainsBorderRune(s string) bool {
	for _, r := range []string{"│", "╭", "╮", "╰", "╯", "─"} {
		if strings.Contains(s, r) {
			return true
		}
	}
	return false
}

// TestResourceListSetBorderlessChangesTableHeight asserts that toggling
// borderless re-lays-out the table at h-1 (one header line) rather than the
// bordered h-3, so the table grows by two rows for the same outer size.
func TestResourceListSetBorderlessChangesTableHeight(t *testing.T) {
	const (
		paneH = 12
		// table.Height() reports the viewport height = the SetLayout height minus
		// the table's internal header row. Derive the expectations from the
		// documented chrome relationship so a constant change fails loudly here:
		//   bordered    lays out at paneH-3 (top border + bottom border + ...),
		//   borderless  lays out at paneH-1 (single title/header chrome line),
		// then both subtract the one internal header row.
		tableHeaderRow   = 1
		borderedChrome   = 3
		borderlessChrome = 1
		wantBordered     = paneH - borderedChrome - tableHeaderRow   // 8
		wantBorderless   = paneH - borderlessChrome - tableHeaderRow // 10
	)
	rl := NewResourceList(&testPlugin{}, 40, paneH)
	borderedH := rl.table.Height()
	rl.SetBorderless(true)
	borderlessH := rl.table.Height()
	if borderlessH <= borderedH {
		t.Fatalf("borderless table height = %d, want greater than bordered %d", borderlessH, borderedH)
	}
	if borderlessH != wantBorderless {
		t.Fatalf("borderless table height = %d, want %d (paneH-%d minus header)", borderlessH, wantBorderless, borderlessChrome)
	}
	if borderedH != wantBordered {
		t.Fatalf("bordered table height = %d, want %d (paneH-%d minus header)", borderedH, wantBordered, borderedChrome)
	}
}

// TestResourceListBorderlessViewNoBorder asserts the borderless View renders no
// box-border runes and includes the title header line, while the bordered View
// does include the border.
func TestResourceListBorderlessViewNoBorder(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 40, 12)
	objs := []*unstructured.Unstructured{makeObj("pod-a")}
	rl.SetObjects(objs)

	bordered := rl.View()
	if !rlContainsBorderRune(bordered) {
		t.Fatalf("bordered View should contain border runes; got:\n%s", ansi.Strip(bordered))
	}

	rl.SetBorderless(true)
	out := rl.View()
	if rlContainsBorderRune(out) {
		t.Fatalf("borderless View should contain no border runes; got:\n%s", ansi.Strip(out))
	}
	if !strings.Contains(ansi.Strip(out), "pods") {
		t.Fatalf("borderless View missing title header; got:\n%s", ansi.Strip(out))
	}
	// The (alt+z: exit zoom) hint is intentionally terminal-only: resource lists
	// must never render it, even in borderless (zoom) mode. Lock that contract.
	if strings.Contains(ansi.Strip(out), "alt+z") {
		t.Fatalf("borderless ResourceList header must not show the exit-zoom hint; got:\n%s", ansi.Strip(out))
	}
}

// --- Health row-tint tests ---

// bgSGR returns the background-color SGR body (e.g. "48;2;142;106;217") a style's
// background color emits, derived from the style itself — no hardcoded escape
// codes. It returns the inner color parameters (without the leading "\x1b[" or
// trailing "m") because lipgloss merges foreground+background+bold into a single
// SGR escape on a styled row, so the background sequence is never standalone.
// Matching this body as a substring distinguishes a cursor-row background FILL
// (which sets a background) from a foreground-only health TINT (which sets none).
func bgSGR(t *testing.T, s lipgloss.Style) string {
	t.Helper()
	bg := s.GetBackground()
	rendered := lipgloss.NewStyle().Background(bg).Render("X")
	i := strings.Index(rendered, "X")
	if i <= 0 {
		t.Fatalf("test setup: style produced no background SGR prefix (rendered %q)", rendered)
	}
	body := strings.TrimSuffix(strings.TrimPrefix(rendered[:i], "\x1b["), "m")
	if body == "" {
		t.Fatalf("test setup: empty background SGR body from %q", rendered[:i])
	}
	return body
}

// lineContaining returns the rendered line whose stripped text contains name, or
// "" if no such line exists.
func lineContaining(out, name string) string {
	for line := range strings.SplitSeq(out, "\n") {
		if strings.Contains(ansi.Strip(line), name) {
			return line
		}
	}
	return ""
}

// TestViewCursorFillAcrossHealthWhenFocused encodes bug 1: when the pane is
// focused (Focus → selectionActive=true), the cursor row carries a background
// FILL on EVERY health level — accent fill (TableSelectedStyle) on a healthy
// cursor row and the red error fill (TableHealthErrorCursorStyle) on an unhealthy
// cursor row. A foreground-only tint is NOT a fill, so we assert the background
// SGR is present on the cursor line.
func TestViewCursorFillAcrossHealthWhenFocused(t *testing.T) {
	accentBG := bgSGR(t, TableSelectedStyle)
	errorBG := bgSGR(t, TableHealthErrorCursorStyle)
	warnBG := bgSGR(t, TableHealthWarnCursorStyle)

	// Cursor on the HEALTHY row → accent fill.
	t.Run("healthy cursor row gets accent fill", func(t *testing.T) {
		hp := &healthTestPlugin{health: map[types.UID]plugin.Health{
			"uid-healthy": plugin.Healthy,
			"uid-err":     plugin.Error,
		}}
		rl := NewResourceList(hp, 80, 15)
		// NAME-ascending: row 0 = pod-a-healthy (cursor), row 1 = pod-b-err.
		rl.SetObjects([]*unstructured.Unstructured{
			makeUIDObj("pod-a-healthy", "uid-healthy"),
			makeUIDObj("pod-b-err", "uid-err"),
		})
		rl.Focus()

		out := rl.table.View()
		cursorLine := lineContaining(out, "pod-a-healthy")
		if cursorLine == "" {
			t.Fatalf("could not find healthy cursor row in output:\n%q", out)
		}
		if !strings.Contains(cursorLine, accentBG) {
			t.Fatalf("focused healthy cursor row must carry accent background fill %q, got %q", accentBG, cursorLine)
		}
	})

	// Cursor on the UNHEALTHY row → red error fill (the bug-1 case).
	t.Run("unhealthy cursor row gets error fill", func(t *testing.T) {
		hp := &healthTestPlugin{health: map[types.UID]plugin.Health{
			"uid-healthy": plugin.Healthy,
			"uid-err":     plugin.Error,
		}}
		rl := NewResourceList(hp, 80, 15)
		// NAME-ascending: row 0 = pod-a-err (cursor), row 1 = pod-b-healthy.
		rl.SetObjects([]*unstructured.Unstructured{
			makeUIDObj("pod-a-err", "uid-err"),
			makeUIDObj("pod-b-healthy", "uid-healthy"),
		})
		rl.Focus()

		out := rl.table.View()
		cursorLine := lineContaining(out, "pod-a-err")
		if cursorLine == "" {
			t.Fatalf("could not find unhealthy cursor row in output:\n%q", out)
		}
		if !strings.Contains(cursorLine, errorBG) {
			t.Fatalf("focused unhealthy cursor row must carry error background fill %q, got %q", errorBG, cursorLine)
		}
	})

	// Cursor on the WARNING row → yellow warn fill.
	t.Run("warning cursor row gets warn fill", func(t *testing.T) {
		hp := &healthTestPlugin{health: map[types.UID]plugin.Health{
			"uid-warn":    plugin.Warning,
			"uid-healthy": plugin.Healthy,
		}}
		rl := NewResourceList(hp, 80, 15)
		// NAME-ascending: row 0 = pod-a-warn (cursor), row 1 = pod-b-healthy.
		rl.SetObjects([]*unstructured.Unstructured{
			makeUIDObj("pod-a-warn", "uid-warn"),
			makeUIDObj("pod-b-healthy", "uid-healthy"),
		})
		rl.Focus()

		out := rl.table.View()
		cursorLine := lineContaining(out, "pod-a-warn")
		if cursorLine == "" {
			t.Fatalf("could not find warning cursor row in output:\n%q", out)
		}
		if !strings.Contains(cursorLine, warnBG) {
			t.Fatalf("focused warning cursor row must carry warn background fill %q, got %q", warnBG, cursorLine)
		}
	})
}

// TestViewNoCursorFillAcrossHealthWhenBlurred encodes bug 2: when the pane is
// unfocused (Blur → selectionActive=false), NEITHER health level shows a cursor
// background FILL. The healthy cursor row renders plain (no accent fill); the
// unhealthy cursor row shows only its foreground tint (TableHealthErrorStyle, no
// background fill), rendering identically to a non-cursor unhealthy row.
func TestViewNoCursorFillAcrossHealthWhenBlurred(t *testing.T) {
	accentBG := bgSGR(t, TableSelectedStyle)
	errorBG := bgSGR(t, TableHealthErrorCursorStyle)
	warnBG := bgSGR(t, TableHealthWarnCursorStyle)
	// The plain tints set a FOREGROUND only; these prefixes should still appear
	// on the blurred unhealthy cursor row (it is tinted, just not filled).
	fgPrefix := func(s lipgloss.Style, name string) string {
		rendered := s.Render("X")
		i := strings.Index(rendered, "X")
		if i <= 0 {
			t.Fatalf("test setup: %s produced no SGR prefix", name)
		}
		return rendered[:i]
	}
	errorTintFG := fgPrefix(TableHealthErrorStyle, "TableHealthErrorStyle")
	warnTintFG := fgPrefix(TableHealthWarnStyle, "TableHealthWarnStyle")

	// Cursor on the HEALTHY row → plain, no accent fill.
	//
	// This sub-test does not on its own discriminate against the old buggy code:
	// the old dim-style cursor also rendered the healthy row without an accent
	// fill. The UNHEALTHY sub-test below is the true regression discriminator,
	// asserting no health-colored fill leaks onto the blurred cursor row.
	t.Run("healthy cursor row renders plain", func(t *testing.T) {
		hp := &healthTestPlugin{health: map[types.UID]plugin.Health{
			"uid-healthy": plugin.Healthy,
			"uid-err":     plugin.Error,
		}}
		rl := NewResourceList(hp, 80, 15)
		rl.SetObjects([]*unstructured.Unstructured{
			makeUIDObj("pod-a-healthy", "uid-healthy"),
			makeUIDObj("pod-b-err", "uid-err"),
		})
		rl.Blur()

		out := rl.table.View()
		cursorLine := lineContaining(out, "pod-a-healthy")
		if cursorLine == "" {
			t.Fatalf("could not find healthy cursor row in output:\n%q", out)
		}
		if strings.Contains(cursorLine, accentBG) {
			t.Fatalf("blurred healthy cursor row must NOT carry accent background fill %q, got %q", accentBG, cursorLine)
		}
	})

	// Cursor on the UNHEALTHY row → fg tint only, no error fill (the bug-2 case).
	t.Run("unhealthy cursor row shows tint, no fill", func(t *testing.T) {
		hp := &healthTestPlugin{health: map[types.UID]plugin.Health{
			"uid-healthy": plugin.Healthy,
			"uid-err":     plugin.Error,
		}}
		rl := NewResourceList(hp, 80, 15)
		rl.SetObjects([]*unstructured.Unstructured{
			makeUIDObj("pod-a-err", "uid-err"),
			makeUIDObj("pod-b-healthy", "uid-healthy"),
		})
		rl.Blur()

		out := rl.table.View()
		cursorLine := lineContaining(out, "pod-a-err")
		if cursorLine == "" {
			t.Fatalf("could not find unhealthy cursor row in output:\n%q", out)
		}
		// No background fill (neither the accent nor the error cursor fill).
		if strings.Contains(cursorLine, errorBG) {
			t.Fatalf("blurred unhealthy cursor row must NOT carry error background fill %q, got %q", errorBG, cursorLine)
		}
		if strings.Contains(cursorLine, accentBG) {
			t.Fatalf("blurred unhealthy cursor row must NOT carry accent background fill %q, got %q", accentBG, cursorLine)
		}
		// But it must still carry the foreground health tint — proving it renders
		// identically to a normal unhealthy row, not stripped of color entirely.
		if !strings.Contains(cursorLine, errorTintFG) {
			t.Fatalf("blurred unhealthy cursor row must carry the foreground tint %q, got %q", errorTintFG, cursorLine)
		}
	})

	// Cursor on the WARNING row → fg tint only, no warn fill.
	t.Run("warning cursor row shows tint, no fill", func(t *testing.T) {
		hp := &healthTestPlugin{health: map[types.UID]plugin.Health{
			"uid-warn":    plugin.Warning,
			"uid-healthy": plugin.Healthy,
		}}
		rl := NewResourceList(hp, 80, 15)
		rl.SetObjects([]*unstructured.Unstructured{
			makeUIDObj("pod-a-warn", "uid-warn"),
			makeUIDObj("pod-b-healthy", "uid-healthy"),
		})
		rl.Blur()

		out := rl.table.View()
		cursorLine := lineContaining(out, "pod-a-warn")
		if cursorLine == "" {
			t.Fatalf("could not find warning cursor row in output:\n%q", out)
		}
		// No background fill (neither the accent nor the warn cursor fill).
		if strings.Contains(cursorLine, warnBG) {
			t.Fatalf("blurred warning cursor row must NOT carry warn background fill %q, got %q", warnBG, cursorLine)
		}
		if strings.Contains(cursorLine, accentBG) {
			t.Fatalf("blurred warning cursor row must NOT carry accent background fill %q, got %q", accentBG, cursorLine)
		}
		// But it must still carry the foreground warn tint — identical to a normal
		// warning row.
		if !strings.Contains(cursorLine, warnTintFG) {
			t.Fatalf("blurred warning cursor row must carry the foreground tint %q, got %q", warnTintFG, cursorLine)
		}
	})
}

// TestViewCursorFillUnderBlurBorder covers the detail-focused production path
// (BlurBorder → selectionActive=true, focused=false): even though the border is
// dimmed, the cursor row must STILL carry its cursor fill — accent on a healthy
// row, the status fill on an unhealthy row — exactly as under Focus().
func TestViewCursorFillUnderBlurBorder(t *testing.T) {
	accentBG := bgSGR(t, TableSelectedStyle)
	warnBG := bgSGR(t, TableHealthWarnCursorStyle)
	errorBG := bgSGR(t, TableHealthErrorCursorStyle)

	// Healthy cursor row → accent fill under BlurBorder.
	t.Run("healthy cursor row keeps accent fill", func(t *testing.T) {
		hp := &healthTestPlugin{health: map[types.UID]plugin.Health{
			"uid-healthy": plugin.Healthy,
			"uid-err":     plugin.Error,
		}}
		rl := NewResourceList(hp, 80, 15)
		rl.SetObjects([]*unstructured.Unstructured{
			makeUIDObj("pod-a-healthy", "uid-healthy"),
			makeUIDObj("pod-b-err", "uid-err"),
		})
		// Exercise a genuine false→true transition into BlurBorder.
		rl.Blur()
		rl.BlurBorder()
		if rl.Focused() {
			t.Fatal("BlurBorder must leave the border unfocused")
		}
		if !rl.SelectionActive() {
			t.Fatal("BlurBorder must keep the selection active")
		}

		out := rl.table.View()
		cursorLine := lineContaining(out, "pod-a-healthy")
		if cursorLine == "" {
			t.Fatalf("could not find healthy cursor row in output:\n%q", out)
		}
		if !strings.Contains(cursorLine, accentBG) {
			t.Fatalf("BlurBorder healthy cursor row must keep accent background fill %q, got %q", accentBG, cursorLine)
		}
	})

	// Warning cursor row → warn fill under BlurBorder.
	t.Run("warning cursor row keeps warn fill", func(t *testing.T) {
		hp := &healthTestPlugin{health: map[types.UID]plugin.Health{
			"uid-warn":    plugin.Warning,
			"uid-healthy": plugin.Healthy,
		}}
		rl := NewResourceList(hp, 80, 15)
		rl.SetObjects([]*unstructured.Unstructured{
			makeUIDObj("pod-a-warn", "uid-warn"),
			makeUIDObj("pod-b-healthy", "uid-healthy"),
		})
		// Exercise a genuine false→true transition into BlurBorder.
		rl.Blur()
		rl.BlurBorder()
		if rl.Focused() {
			t.Fatal("BlurBorder must leave the border unfocused")
		}
		if !rl.SelectionActive() {
			t.Fatal("BlurBorder must keep the selection active")
		}

		out := rl.table.View()
		cursorLine := lineContaining(out, "pod-a-warn")
		if cursorLine == "" {
			t.Fatalf("could not find warning cursor row in output:\n%q", out)
		}
		if !strings.Contains(cursorLine, warnBG) {
			t.Fatalf("BlurBorder warning cursor row must keep warn background fill %q, got %q", warnBG, cursorLine)
		}
	})

	// Unhealthy cursor row → error fill under BlurBorder.
	t.Run("unhealthy cursor row keeps error fill", func(t *testing.T) {
		hp := &healthTestPlugin{health: map[types.UID]plugin.Health{
			"uid-err":     plugin.Error,
			"uid-healthy": plugin.Healthy,
		}}
		rl := NewResourceList(hp, 80, 15)
		rl.SetObjects([]*unstructured.Unstructured{
			makeUIDObj("pod-a-err", "uid-err"),
			makeUIDObj("pod-b-healthy", "uid-healthy"),
		})
		// Exercise a genuine false→true transition into BlurBorder.
		rl.Blur()
		rl.BlurBorder()

		out := rl.table.View()
		cursorLine := lineContaining(out, "pod-a-err")
		if cursorLine == "" {
			t.Fatalf("could not find unhealthy cursor row in output:\n%q", out)
		}
		if !strings.Contains(cursorLine, errorBG) {
			t.Fatalf("BlurBorder unhealthy cursor row must keep error background fill %q, got %q", errorBG, cursorLine)
		}
	})
}

// healthTestPlugin is a testPlugin that also implements plugin.HealthReporter,
// reporting per-object health from a UID-keyed map.
type healthTestPlugin struct {
	testPlugin
	health map[types.UID]plugin.Health
}

func (p *healthTestPlugin) RowHealth(obj *unstructured.Unstructured) plugin.Health {
	return p.health[obj.GetUID()]
}

func TestRebindRowStyleHealthNonCursor(t *testing.T) {
	hp := &healthTestPlugin{health: map[types.UID]plugin.Health{
		"uid-warn":    plugin.Warning,
		"uid-err":     plugin.Error,
		"uid-healthy": plugin.Healthy,
	}}
	rl := NewResourceList(hp, 80, 15)
	// Names chosen so the default NAME-ascending sort yields:
	//   row 0 = pod-a-warn, row 1 = pod-b-err, row 2 = pod-c-healthy.
	objs := []*unstructured.Unstructured{
		makeUIDObj("pod-a-warn", "uid-warn"),
		makeUIDObj("pod-b-err", "uid-err"),
		makeUIDObj("pod-c-healthy", "uid-healthy"),
	}
	rl.SetObjects(objs)

	if got := rl.table.RowStyleFunc(0, false, true); got != &TableHealthWarnStyle {
		t.Fatalf("warning non-cursor row: expected &TableHealthWarnStyle, got %v", got)
	}
	if got := rl.table.RowStyleFunc(1, false, true); got != &TableHealthErrorStyle {
		t.Fatalf("error non-cursor row: expected &TableHealthErrorStyle, got %v", got)
	}
	if got := rl.table.RowStyleFunc(2, false, true); got != nil {
		t.Fatalf("healthy non-cursor row: expected nil, got %v", got)
	}

	// The non-cursor health tint must be independent of selection-active state:
	// flipping to inactive (Blur) and back to active (Focus) must not change it.
	rl.Blur()
	if got := rl.table.RowStyleFunc(0, false, false); got != &TableHealthWarnStyle {
		t.Fatalf("blurred warning non-cursor row: expected &TableHealthWarnStyle, got %v", got)
	}
	if got := rl.table.RowStyleFunc(1, false, false); got != &TableHealthErrorStyle {
		t.Fatalf("blurred error non-cursor row: expected &TableHealthErrorStyle, got %v", got)
	}
	rl.Focus()
	if got := rl.table.RowStyleFunc(0, false, true); got != &TableHealthWarnStyle {
		t.Fatalf("refocused warning non-cursor row: expected &TableHealthWarnStyle, got %v", got)
	}
}

func TestRebindRowStyleUnfocusedCursorRendersAsNormalRow(t *testing.T) {
	hp := &healthTestPlugin{health: map[types.UID]plugin.Health{
		"uid-err": plugin.Error,
	}}
	rl := NewResourceList(hp, 80, 15)
	// One Error object lives at display index 0, so RowStyleFunc(0, true) lands on
	// a real unhealthy row — this guards against the closure's `index >= len(display)`
	// bounds check returning nil for an EMPTY display and the test passing for the
	// wrong reason (never reaching the inactive branch).
	objs := []*unstructured.Unstructured{makeUIDObj("pod-err", "uid-err")}
	rl.SetObjects(objs)

	// Positive baseline: while still ACTIVE, the cursor row over the unhealthy
	// object must take the status-colored cursor fill. If Blur() below failed to
	// rebind the closure, this assertion would still pass but the next one would
	// fail — proving the inactive branch (not a stale closure) changes the tint.
	if got := rl.table.RowStyleFunc(0, true, true); got != &TableHealthErrorCursorStyle {
		t.Fatalf("active baseline: expected &TableHealthErrorCursorStyle at index 0, got %v", got)
	}

	// After Blur() the pane is no longer active: the cursor row falls through to
	// the standard non-cursor health tint, rendering identically to a normal
	// unready row (so it matches its red siblings instead of dimming to Selected).
	rl.Blur()
	if got := rl.table.RowStyleFunc(0, true, false); got != &TableHealthErrorStyle {
		t.Fatalf("blurred cursor row over unhealthy object: expected &TableHealthErrorStyle (normal row), got %v", got)
	}
}

func TestRebindRowStyleCursorHealthWhenFocused(t *testing.T) {
	hp := &healthTestPlugin{health: map[types.UID]plugin.Health{
		"uid-warn":    plugin.Warning,
		"uid-err":     plugin.Error,
		"uid-healthy": plugin.Healthy,
	}}
	rl := NewResourceList(hp, 80, 15)
	// Default NAME-ascending sort yields:
	//   row 0 = pod-a-warn, row 1 = pod-b-err, row 2 = pod-c-healthy.
	objs := []*unstructured.Unstructured{
		makeUIDObj("pod-a-warn", "uid-warn"),
		makeUIDObj("pod-b-err", "uid-err"),
		makeUIDObj("pod-c-healthy", "uid-healthy"),
	}
	rl.SetObjects(objs)

	// Start blurred so Focus() exercises a real blurred→focused transition. The
	// closure is NOT rebound on focus change (applyFocus does not call
	// rebindRowStyle); the per-state behavior comes entirely from the live `active`
	// arg passed below, so a single binding suffices for both focus states.
	rl.Blur()
	rl.Focus()
	styleFunc := rl.table.RowStyleFunc

	if got := styleFunc(0, true, true); got != &TableHealthWarnCursorStyle {
		t.Fatalf("focused warning cursor row: expected &TableHealthWarnCursorStyle, got %v", got)
	}
	if got := styleFunc(1, true, true); got != &TableHealthErrorCursorStyle {
		t.Fatalf("focused error cursor row: expected &TableHealthErrorCursorStyle, got %v", got)
	}
	// Healthy row under a focused cursor falls through to the table's Selected style.
	if got := styleFunc(2, true, true); got != nil {
		t.Fatalf("focused healthy cursor row: expected nil, got %v", got)
	}
	// Non-cursor unready rows keep the plain health tint (regression guard).
	if got := styleFunc(0, false, true); got != &TableHealthWarnStyle {
		t.Fatalf("focused warning non-cursor row: expected &TableHealthWarnStyle, got %v", got)
	}
	if got := styleFunc(1, false, true); got != &TableHealthErrorStyle {
		t.Fatalf("focused error non-cursor row: expected &TableHealthErrorStyle, got %v", got)
	}

	// With active=false the unhealthy cursor rows fall through to the plain health
	// tint (no cursor variant), while a healthy cursor row returns nil — and with
	// active=false a nil return makes renderRow render a PLAIN row (Selected is
	// gated off), not the Selected style. Blur() does not rebind the closure;
	// styleFunc is the same object, just probed with active=false here.
	rl.Blur()
	if got := styleFunc(0, true, false); got != &TableHealthWarnStyle {
		t.Fatalf("blurred warning cursor row: expected &TableHealthWarnStyle, got %v", got)
	}
	if got := styleFunc(1, true, false); got != &TableHealthErrorStyle {
		t.Fatalf("blurred error cursor row: expected &TableHealthErrorStyle, got %v", got)
	}
	if got := styleFunc(2, true, false); got != nil {
		t.Fatalf("blurred healthy cursor row: expected nil, got %v", got)
	}
}

// TestRowStyleFuncHealthMappingIsPure proves the cursor-row health style is a pure
// function of the live `active` arg, independent of which focus method last ran.
// For both the warn and error rows: RowStyleFunc(i,true,true) returns the cursor
// fill, RowStyleFunc(i,true,false) returns the plain tint — and the result does not
// change across Focus/Blur/BlurBorder, because active is now a render
// parameter, not captured focus state. (Replaces the deleted BorderFocusToggle
// tests, which only asserted that BlurBorder/Focus re-ran rebindRowStyle.)
func TestRowStyleFuncHealthMappingIsPure(t *testing.T) {
	hp := &healthTestPlugin{health: map[types.UID]plugin.Health{
		"uid-warn": plugin.Warning,
		"uid-err":  plugin.Error,
	}}
	rl := NewResourceList(hp, 80, 15)
	// Default NAME-ascending sort: row 0 = pod-a-warn, row 1 = pod-b-err.
	objs := []*unstructured.Unstructured{
		makeUIDObj("pod-a-warn", "uid-warn"),
		makeUIDObj("pod-b-err", "uid-err"),
	}
	rl.SetObjects(objs)

	const (
		warnRow = 0
		errRow  = 1
	)

	assertMapping := func(t *testing.T, label string) {
		t.Helper()
		// active=true → cursor fill.
		if got := rl.table.RowStyleFunc(warnRow, true, true); got != &TableHealthWarnCursorStyle {
			t.Fatalf("%s: warn cursor row active: expected &TableHealthWarnCursorStyle, got %v", label, got)
		}
		if got := rl.table.RowStyleFunc(errRow, true, true); got != &TableHealthErrorCursorStyle {
			t.Fatalf("%s: error cursor row active: expected &TableHealthErrorCursorStyle, got %v", label, got)
		}
		// active=false → plain tint (no cursor variant).
		if got := rl.table.RowStyleFunc(warnRow, true, false); got != &TableHealthWarnStyle {
			t.Fatalf("%s: warn cursor row inactive: expected &TableHealthWarnStyle, got %v", label, got)
		}
		if got := rl.table.RowStyleFunc(errRow, true, false); got != &TableHealthErrorStyle {
			t.Fatalf("%s: error cursor row inactive: expected &TableHealthErrorStyle, got %v", label, got)
		}
	}

	// The mapping is identical no matter which focus method last ran.
	rl.Focus()
	assertMapping(t, "after Focus")
	rl.Blur()
	assertMapping(t, "after Blur")
	rl.BlurBorder()
	assertMapping(t, "after BlurBorder")
	rl.Focus()
	assertMapping(t, "after Focus")
}

func TestRebindRowStyleMarkWinsOverCursorHealthWhenFocused(t *testing.T) {
	hp := &healthTestPlugin{health: map[types.UID]plugin.Health{
		"uid-err": plugin.Error,
	}}
	rl := NewResourceList(hp, 80, 15)
	objs := []*unstructured.Unstructured{makeUIDObj("pod-err", "uid-err")}
	rl.SetObjects(objs)

	// Start blurred so Focus() exercises a real active=false → active=true
	// transition, guarding against a Focus() that fails to rebind the closure
	// after a Blur().
	rl.Blur()
	rl.Focus()
	rl.ToggleSelect() // rebinds the closure itself

	// Marks win even on a focused unready cursor row.
	if got := rl.table.RowStyleFunc(0, true, true); got != &TableMarkedSelectedStyle {
		t.Fatalf("focused marked unready cursor row: expected &TableMarkedSelectedStyle, got %v", got)
	}
}

func TestRebindRowStyleMarkWinsOverCursorHealthWhenBlurred(t *testing.T) {
	hp := &healthTestPlugin{health: map[types.UID]plugin.Health{
		"uid-err": plugin.Error,
	}}
	rl := NewResourceList(hp, 80, 15)
	objs := []*unstructured.Unstructured{makeUIDObj("pod-err", "uid-err")}
	rl.SetObjects(objs)

	rl.Blur()
	rl.ToggleSelect() // rebinds the closure itself

	// The mark check precedes the health branch, but the cursor variant only
	// applies in an active pane. A blurred (inactive) marked cursor row shows the
	// plain mark style — no cursor highlight.
	if got := rl.table.RowStyleFunc(0, true, false); got != &TableMarkedStyle {
		t.Fatalf("blurred marked unready cursor row: expected &TableMarkedStyle, got %v", got)
	}
}

func TestRebindRowStyleMarkWinsOverHealth(t *testing.T) {
	hp := &healthTestPlugin{health: map[types.UID]plugin.Health{
		"uid-err": plugin.Error,
	}}
	rl := NewResourceList(hp, 80, 15)
	objs := []*unstructured.Unstructured{makeUIDObj("pod-err", "uid-err")}
	rl.SetObjects(objs)

	// Drive a real blurred→focused transition so the focused cursor assertion below
	// is non-vacuous (distinguishes the active-cursor contract from the old one).
	// The blurred marked-cursor case (→ &TableMarkedStyle) is covered separately by
	// TestRebindRowStyleMarkWinsOverCursorHealthWhenBlurred.
	rl.Blur()
	rl.Focus()

	// Mark the row. ToggleSelect rebinds the RowStyleFunc itself.
	rl.ToggleSelect()

	// Marked + non-cursor → mark style (not health).
	if got := rl.table.RowStyleFunc(0, false, true); got != &TableMarkedStyle {
		t.Fatalf("marked non-cursor row: expected &TableMarkedStyle, got %v", got)
	}
	// Marked + cursor → marked-selected style (not health, not plain selection).
	if got := rl.table.RowStyleFunc(0, true, true); got != &TableMarkedSelectedStyle {
		t.Fatalf("marked cursor row: expected &TableMarkedSelectedStyle, got %v", got)
	}
}

func TestRebindRowStyleNoHealthReporter(t *testing.T) {
	// testPlugin does not implement plugin.HealthReporter.
	rl := NewResourceList(&testPlugin{}, 80, 15)
	objs := []*unstructured.Unstructured{makeUIDObj("pod-a", "uid-a")}
	rl.SetObjects(objs)

	// Fast path: with no HealthReporter and no selection the closure can only
	// ever return nil, so it is not installed at all.
	if rl.table.RowStyleFunc != nil {
		t.Fatalf("no HealthReporter and no selection: expected nil RowStyleFunc, got non-nil")
	}
}

func TestRebindRowStyleOutOfRange(t *testing.T) {
	hp := &healthTestPlugin{health: map[types.UID]plugin.Health{
		"uid-err": plugin.Error,
	}}
	rl := NewResourceList(hp, 80, 15)
	objs := []*unstructured.Unstructured{makeUIDObj("pod-err", "uid-err")}
	rl.SetObjects(objs)

	if got := rl.table.RowStyleFunc(-1, false, true); got != nil {
		t.Fatalf("index -1: expected nil, got %v", got)
	}
	if got := rl.table.RowStyleFunc(5, false, true); got != nil {
		t.Fatalf("out-of-range index: expected nil, got %v", got)
	}
}

// coloredHealthPlugin is a HealthReporter whose Row emits a status cell with an
// embedded foreground color SGR, so the strip path is exercised end-to-end.
type coloredHealthPlugin struct {
	testPlugin
	health map[types.UID]plugin.Health
}

func (p *coloredHealthPlugin) RowHealth(obj *unstructured.Unstructured) plugin.Health {
	return p.health[obj.GetUID()]
}

func (p *coloredHealthPlugin) Row(obj *unstructured.Unstructured) []string {
	// Status cell carries a red foreground + fg-only reset, like RenderStatus.
	return []string{obj.GetName(), "\x1b[31mERR\x1b[39m"}
}

// TestHealthTintStripsInnerANSIEndToEnd drives the real wiring: SetObjects calls
// rebindRowStyle, and the rendered table runs renderRow, which must apply the
// health override foreground across the unhealthy row AND strip the inner
// per-cell color SGR so the tint is not cut short mid-row.
func TestHealthTintStripsInnerANSIEndToEnd(t *testing.T) {
	hp := &coloredHealthPlugin{health: map[types.UID]plugin.Health{
		"uid-err": plugin.Error,
	}}
	rl := NewResourceList(hp, 80, 15)
	// Two rows: cursor lands on row 0 by default, so row 1 (the unhealthy one)
	// takes the non-cursor health-override branch.
	objs := []*unstructured.Unstructured{
		makeUIDObj("pod-a-healthy", "uid-healthy"),
		makeUIDObj("pod-b-err", "uid-err"),
	}
	rl.SetObjects(objs)

	// The RowStyleFunc must return the error health style for the unhealthy row.
	if got := rl.table.RowStyleFunc(1, false, true); got != &TableHealthErrorStyle {
		t.Fatalf("unhealthy non-cursor row: expected &TableHealthErrorStyle, got %v", got)
	}

	out := rl.table.View()

	// The override SGR (error foreground) must appear in the rendered output.
	overridePrefix := func() string {
		rendered := TableHealthErrorStyle.Render("X")
		return rendered[:strings.Index(rendered, "X")]
	}()
	if overridePrefix == "" {
		t.Fatalf("test setup: TableHealthErrorStyle produced no SGR prefix")
	}
	if !strings.Contains(out, overridePrefix) {
		t.Fatalf("expected health override SGR %q in rendered table, got:\n%q", overridePrefix, out)
	}

	// Locate the unhealthy row specifically by its unique NAME, not by the
	// override prefix alone — the prefix could otherwise match a header, border,
	// or padding line and pass vacuously. The same line must carry the health
	// override SGR AND have its inner per-cell color (\x1b[31m) stripped.
	var tinted string
	for line := range strings.SplitSeq(out, "\n") {
		if strings.Contains(ansi.Strip(line), "pod-b-err") {
			tinted = line
			break
		}
	}
	if tinted == "" {
		t.Fatalf("could not find the unhealthy row (containing 'pod-b-err') in output:\n%q", out)
	}
	if !strings.Contains(tinted, overridePrefix) {
		t.Fatalf("unhealthy row must carry the health override SGR %q, got %q", overridePrefix, tinted)
	}
	if strings.Contains(tinted, "\x1b[31m") {
		t.Fatalf("inner per-cell color SGR should be stripped on the tinted row, got %q", tinted)
	}
}

// TestRebindRowStyleKeepsHealthAfterDeselect guards the nil fast-path boundary:
// the closure may only be dropped when reporter == nil. With a HealthReporter
// present, selecting then deselecting the last marked item must leave a NON-nil
// RowStyleFunc that still returns health styles — the empty-selection state must
// not be mistaken for "nothing to style".
func TestRebindRowStyleKeepsHealthAfterDeselect(t *testing.T) {
	hp := &healthTestPlugin{health: map[types.UID]plugin.Health{
		"uid-err": plugin.Error,
	}}
	rl := NewResourceList(hp, 80, 15)
	objs := []*unstructured.Unstructured{makeUIDObj("pod-err", "uid-err")}
	rl.SetObjects(objs)

	// Mark the only row, then deselect it (back to empty selection).
	rl.ToggleSelect()
	if !rl.HasSelection() {
		t.Fatal("expected selection after ToggleSelect")
	}
	rl.ToggleSelect()
	if rl.HasSelection() {
		t.Fatal("expected no selection after second ToggleSelect")
	}

	// Even with an empty selection, the reporter is present, so the closure must
	// survive and keep reporting health for non-cursor rows.
	if rl.table.RowStyleFunc == nil {
		t.Fatal("RowStyleFunc must stay non-nil while a HealthReporter is present")
	}
	if got := rl.table.RowStyleFunc(0, false, true); got != &TableHealthErrorStyle {
		t.Fatalf("unhealthy non-cursor row after deselect: expected &TableHealthErrorStyle, got %v", got)
	}

	// ClearSelection takes the same empty-selection path and must also preserve it.
	rl.ToggleSelect()
	rl.ClearSelection()
	if rl.table.RowStyleFunc == nil {
		t.Fatal("RowStyleFunc must stay non-nil after ClearSelection with a HealthReporter")
	}
	if got := rl.table.RowStyleFunc(0, false, true); got != &TableHealthErrorStyle {
		t.Fatalf("unhealthy non-cursor row after ClearSelection: expected &TableHealthErrorStyle, got %v", got)
	}
}

// TestSearchHighlightUsesReverseOnTintedRows covers the search-highlight +
// health-tint interaction: tinted rows (mark/health) have their per-cell color
// stripped at render time, which would also erase a themed-color match marker.
// applyVisibleHighlights must switch those rows to reverse-video so the match
// survives stripStyleKeepReverse. The cursor row also uses reverse-video.
func TestSearchHighlightUsesReverseOnTintedRows(t *testing.T) {
	hp := &healthTestPlugin{health: map[types.UID]plugin.Health{
		"uid-warn": plugin.Warning,
		"uid-err":  plugin.Error,
	}}
	rl := NewResourceList(hp, 80, 20) // tall viewport: full-list window, no scrolling
	// Default NAME-ascending sort: row 0 = match-a-cursor (healthy, the cursor),
	// row 1 = match-b-warn, row 2 = match-c-err.
	objs := []*unstructured.Unstructured{
		makeUIDObj("match-a-cursor", "uid-healthy"),
		makeUIDObj("match-b-warn", "uid-warn"),
		makeUIDObj("match-c-err", "uid-err"),
	}
	rl.SetObjects(objs)

	if err := rl.ApplySearch("match", msgs.SearchModeSearch); err != nil {
		t.Fatalf("ApplySearch should not error: %v", err)
	}
	if cursor := rl.table.Cursor(); cursor != 0 {
		t.Fatalf("expected cursor on row 0, got %d", cursor)
	}

	rows := rl.table.Rows()
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}

	// Cursor row uses reverse-video.
	if !rowCursorHighlighted(rows[0]) {
		t.Fatalf("cursor row should use reverse-video variant; got %q", rows[0])
	}
	// Tinted (warning/error) rows must use reverse-video, not the themed-color
	// variant, so the match survives the whole-row strip at render time.
	for i := 1; i < len(rows); i++ {
		if !rowCursorHighlighted(rows[i]) {
			t.Fatalf("tinted row %d should use reverse-video variant; got %q", i, rows[i])
		}
		if rowMatchHighlighted(rows[i]) {
			t.Fatalf("tinted row %d should not use themed-color variant; got %q", i, rows[i])
		}
	}

	// End-to-end: the rendered tinted row must still carry the reverse-video
	// marker after renderRow applies the health override and strips cell color.
	out := rl.table.View()
	var tinted string
	for line := range strings.SplitSeq(out, "\n") {
		if strings.Contains(ansi.Strip(line), "match-c-err") {
			tinted = line
			break
		}
	}
	if tinted == "" {
		t.Fatalf("could not find the error-tinted row in output:\n%q", out)
	}
	if !strings.Contains(tinted, highlightOn) {
		t.Fatalf("rendered tinted row must retain the reverse-video match marker %q, got %q", highlightOn, tinted)
	}
}
