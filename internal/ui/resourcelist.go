package ui

import (
	"fmt"
	"regexp"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/aohoyd/aku/internal/msgs"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/table"
	"github.com/charmbracelet/x/ansi"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

// ResourceList wraps bubbles/table with plugin-driven columns.
type ResourceList struct {
	plugin         plugin.ResourcePlugin
	table          table.Model
	styles         table.Styles
	allObjects     []*unstructured.Unstructured
	displayObjects []*unstructured.Unstructured
	filterState    SearchState
	searchState    SearchState
	sortState      SortState
	namespace      string
	focused        bool
	width          int
	height         int
	contentWidth   int
	xOffset        int
	navStack       NavStack
	inlineSearch     string
	lastSearchCursor int
	selected         map[types.UID]struct{} // multi-select set
}

// SetInlineSearch sets the inline search input text for rendering in the title.
func (r *ResourceList) SetInlineSearch(s string) { r.inlineSearch = s }

// NewResourceList creates a new resource list for the given plugin.
func NewResourceList(p plugin.ResourcePlugin, width, height int) ResourceList {
	sortState := SortStateForPlugin(p)
	cols, contentWidth := pluginColumnsToTableColumns(p.Columns(), width-2, sortState, nil)
	t := table.New(
		table.WithColumns(cols),
		table.WithFocused(true),
		table.WithWidth(width-2),
		table.WithHeight(height-3),
	)

	s := table.DefaultStyles()
	s.Header = TableHeaderStyle
	s.Selected = TableSelectedStyle
	t.SetStyles(s)
	t.SetContentWidth(contentWidth)

	return ResourceList{
		plugin:       p,
		table:        t,
		styles:       s,
		sortState:    sortState,
		focused:      true,
		width:        width,
		height:       height,
		contentWidth: contentWidth,
	}
}

// SetObjects replaces the displayed items with cursor position preservation.
func (r *ResourceList) SetObjects(objs []*unstructured.Unstructured) {
	var selectedKey string
	cursor := r.table.Cursor()
	if len(r.displayObjects) > 0 && cursor >= 0 && cursor < len(r.displayObjects) {
		obj := r.displayObjects[cursor]
		if r.namespace == "" {
			selectedKey = obj.GetKind() + "/" + obj.GetNamespace() + "/" + obj.GetName()
		} else {
			selectedKey = obj.GetKind() + "/" + obj.GetName()
		}
	}

	// Sort a copy — do not mutate the slice from the store
	sorted := make([]*unstructured.Unstructured, len(objs))
	copy(sorted, objs)
	sortObjects(sorted, r.sortState, r.plugin)

	// Prune stale UIDs from selection
	if len(r.selected) > 0 {
		valid := make(map[types.UID]struct{}, len(sorted))
		for _, obj := range sorted {
			valid[obj.GetUID()] = struct{}{}
		}
		for uid := range r.selected {
			if _, ok := valid[uid]; !ok {
				delete(r.selected, uid)
			}
		}
		if len(r.selected) == 0 {
			r.selected = nil
		}
	}

	r.allObjects = sorted
	r.rebuildDisplay()

	// Restore cursor by key
	if selectedKey != "" {
		for i, obj := range r.displayObjects {
			key := obj.GetKind() + "/" + obj.GetName()
			if r.namespace == "" {
				key = obj.GetKind() + "/" + obj.GetNamespace() + "/" + obj.GetName()
			}
			if key == selectedKey {
				r.table.SetCursor(i)
				return
			}
		}
	}
	if len(r.displayObjects) > 0 {
		// Ensure cursor is valid and viewport is updated
		cursor = r.table.Cursor()
		if cursor >= len(r.displayObjects) {
			cursor = len(r.displayObjects) - 1
		}
		r.table.SetCursor(cursor)
	}
}

// SetSort toggles or changes the sort column and re-renders.
func (r *ResourceList) SetSort(column string) {
	r.sortState = r.sortState.Toggle(column)
	r.SetObjects(r.allObjects)
}

// SortState returns the current sort state.
func (r *ResourceList) SortState() SortState {
	return r.sortState
}

// Selected returns the currently highlighted unstructured object.
func (r *ResourceList) Selected() *unstructured.Unstructured {
	idx := r.table.Cursor()
	if idx < 0 || idx >= len(r.displayObjects) {
		return nil
	}
	return r.displayObjects[idx]
}

// Len returns the number of displayed items.
func (r *ResourceList) Len() int {
	return len(r.displayObjects)
}

// GotoTop moves the cursor to the first row.
func (r *ResourceList) GotoTop() { r.table.GotoTop(); r.refreshSearchHighlights() }

// GotoBottom moves the cursor to the last row.
func (r *ResourceList) GotoBottom() { r.table.GotoBottom(); r.refreshSearchHighlights() }

// PageUp moves the cursor up by one page.
func (r *ResourceList) PageUp() { r.table.MoveUp(r.table.Height()); r.refreshSearchHighlights() }

// PageDown moves the cursor down by one page.
func (r *ResourceList) PageDown() { r.table.MoveDown(r.table.Height()); r.refreshSearchHighlights() }

// CursorUp moves the table cursor up by one row.
func (r *ResourceList) CursorUp() { r.table.MoveUp(1); r.refreshSearchHighlights() }

// CursorDown moves the table cursor down by one row.
func (r *ResourceList) CursorDown() { r.table.MoveDown(1); r.refreshSearchHighlights() }

const listScrollStep = 8

// ScrollRight scrolls the resource list right by listScrollStep characters.
func (r *ResourceList) ScrollRight() {
	maxOffset := max(0, r.contentWidth-(r.width-2))
	r.xOffset = min(r.xOffset+listScrollStep, maxOffset)
	r.table.SetXOffset(r.xOffset)
}

// ScrollLeft scrolls the resource list left by listScrollStep characters.
func (r *ResourceList) ScrollLeft() {
	r.xOffset = max(r.xOffset-listScrollStep, 0)
	r.table.SetXOffset(r.xOffset)
}

// Cursor returns selected element.
func (r *ResourceList) Cursor() int { return r.table.Cursor() }

// Plugin returns the current plugin.
func (r *ResourceList) Plugin() plugin.ResourcePlugin {
	return r.plugin
}

// Namespace returns the namespace this list is scoped to.
func (r *ResourceList) Namespace() string {
	return r.namespace
}

// SetNamespace updates the namespace this list is scoped to.
func (r *ResourceList) SetNamespace(ns string) {
	r.namespace = ns
}

// EffectiveNamespace returns the namespace used for store operations.
// Returns "" for cluster-scoped plugins (they watch all namespaces).
func (r *ResourceList) EffectiveNamespace() string {
	if r.plugin.IsClusterScoped() {
		return ""
	}
	return r.namespace
}

// SetPlugin changes the resource plugin and clears the list.
func (r *ResourceList) SetPlugin(p plugin.ResourcePlugin) {
	r.plugin = p
	r.sortState = SortStateForPlugin(p)
	r.filterState.Clear()
	r.searchState.Clear()
	r.selected = nil
	r.allObjects = nil
	r.displayObjects = nil
	r.xOffset = 0
	r.table.SetXOffset(0)
	cols, cw := pluginColumnsToTableColumns(r.effectiveColumns(), r.width-2, r.sortState, nil)
	r.contentWidth = cw
	r.table.SetColumnsAndRows(cols, nil)
	r.table.SetContentWidth(r.contentWidth)
}

// ResetNav clears the drill-down nav stack.
func (r *ResourceList) ResetNav() {
	r.navStack = NavStack{}
	r.selected = nil
}

// ResetForReload unwinds the nav stack to root, restoring the root plugin
// and namespace. Sort/filter/search state is preserved. Objects are cleared
// and must be repopulated by the caller.
func (r *ResourceList) ResetForReload() {
	// Unwind to root — the bottom-most snapshot has the root state
	var root NavSnapshot
	var hasRoot bool
	for r.navStack.Depth() > 0 {
		root, hasRoot = r.navStack.Pop()
	}
	if hasRoot {
		r.plugin = root.Plugin
		r.namespace = root.Namespace
		r.sortState = root.SortState
		cols, cw := pluginColumnsToTableColumns(r.effectiveColumns(), r.width-2, r.sortState, nil)
		r.contentWidth = cw
		r.table.SetColumnsAndRows(cols, nil)
		r.table.SetContentWidth(r.contentWidth)
	} else {
		r.table.SetRows(nil)
	}
	// Clear filter/search so reload gives a clean view
	r.filterState.Clear()
	r.searchState.Clear()
	r.selected = nil
	// Clear objects — caller will repopulate via SetObjects
	r.allObjects = nil
	r.displayObjects = nil
}

// Width returns the current width.
func (r ResourceList) Width() int { return r.width }

// Height returns the current height.
func (r ResourceList) Height() int { return r.height }

// SetSize updates the dimensions.
func (r *ResourceList) SetSize(w, h int) {
	r.width = w
	r.height = h
	r.xOffset = 0
	r.table.SetXOffset(0)
	cols, cw := pluginColumnsToTableColumns(r.effectiveColumns(), w-2, r.sortState, r.table.Rows())
	r.contentWidth = cw
	r.table.SetLayout(cols, w-2, h-3)
	r.table.SetContentWidth(r.contentWidth)
}

// Focus marks this list as focused.
func (r *ResourceList) Focus() {
	r.focused = true
	r.table.Focus()
	r.styles.Selected = TableSelectedStyle
	r.table.SetStyles(r.styles)
	r.EnsureCursorVisible()
}

// EnsureCursorVisible adjusts cursor location.
func (r *ResourceList) EnsureCursorVisible() {
	r.table.EnsureCursorVisible()
}

// Blur marks this list as unfocused.
func (r *ResourceList) Blur() {
	r.focused = false
	r.table.Blur()
	r.styles.Selected = TableSelectedDimStyle
	r.table.SetStyles(r.styles)
}

// BlurBorder dims only the border, keeping the selected row style.
// Used when focus moves to the detail panel so the active selection remains visible.
func (r *ResourceList) BlurBorder() {
	r.focused = false
}

// FocusBorder restores the focused border without changing the selected row style.
// Pairs with BlurBorder when returning from detail-scroll mode.
func (r *ResourceList) FocusBorder() {
	r.focused = true
	r.table.EnsureCursorVisible()
}

// Focused returns whether this list has focus.
func (r *ResourceList) Focused() bool {
	return r.focused
}

// PushNav saves current state and switches to a drill-down child view.
func (r *ResourceList) PushNav(childPlugin plugin.ResourcePlugin, children []*unstructured.Unstructured, parentName string, parentUID string, parentAPIVersion string, parentKind string) {
	r.selected = nil
	r.navStack.Push(NavSnapshot{
		Plugin:           r.plugin,
		Namespace:        r.namespace,
		Objects:          r.allObjects,
		Cursor:           r.table.Cursor(),
		SortState:        r.sortState,
		FilterState:      r.filterState,
		SearchState:      r.searchState,
		ParentUID:        parentUID,
		ParentName:       parentName,
		ParentAPIVersion: parentAPIVersion,
		ParentKind:       parentKind,
	})
	r.SetPlugin(childPlugin)
	r.SetObjects(children)
}

// PopNav restores the previous pane state. Returns false if stack is empty.
func (r *ResourceList) PopNav() bool {
	snap, ok := r.navStack.Pop()
	if !ok {
		return false
	}
	r.plugin = snap.Plugin
	r.namespace = snap.Namespace
	r.sortState = snap.SortState
	r.filterState = snap.FilterState
	r.searchState = snap.SearchState
	// Rebuild columns for restored plugin
	cols, cw := pluginColumnsToTableColumns(r.effectiveColumns(), r.width-2, r.sortState, nil)
	r.contentWidth = cw
	r.table.SetColumnsAndRows(cols, nil)
	r.table.SetContentWidth(r.contentWidth)

	r.selected = nil

	// Restore objects and cursor
	r.allObjects = snap.Objects
	r.rebuildDisplay()
	if snap.Cursor < len(r.displayObjects) {
		r.table.SetCursor(snap.Cursor)
		r.table.EnsureCursorVisible()
	}
	return true
}

// InDrillDown reports whether this pane is in a drill-down child view.
func (r *ResourceList) InDrillDown() bool {
	return r.navStack.Depth() > 0
}

// ParentContext returns the parent resource name shown in the title during drill-down.
func (r *ResourceList) ParentContext() string {
	if snap, ok := r.navStack.Peek(); ok {
		label := snap.Plugin.ShortName()
		if label == "" {
			label = snap.Plugin.Name()
		}
		return label + "/" + snap.ParentName
	}
	return ""
}

// ParentSnap returns a pointer to the top nav snapshot, or nil if not in drill-down.
func (r *ResourceList) ParentSnap() *NavSnapshot {
	if snap, ok := r.navStack.Peek(); ok {
		return &snap
	}
	return nil
}

// NavStackHasGVR reports whether this pane's nav stack contains a snapshot
// referencing the given GVR and namespace.
func (r *ResourceList) NavStackHasGVR(gvr schema.GroupVersionResource, namespace string) bool {
	return r.navStack.HasGVR(gvr, namespace)
}

// effectiveColumns returns plugin columns, prepending NAMESPACE when in all-namespaces mode.
func (r *ResourceList) effectiveColumns() []plugin.Column {
	cols := r.plugin.Columns()
	if r.namespace != "" {
		return cols
	}
	if r.plugin.IsClusterScoped() {
		return cols
	}
	result := make([]plugin.Column, 0, len(cols)+1)
	result = append(result, plugin.Column{Title: "NAMESPACE", Flex: true})
	result = append(result, cols...)
	return result
}

// effectiveRow returns plugin row cells, prepending the object's namespace when in all-namespaces mode.
func (r *ResourceList) effectiveRow(obj *unstructured.Unstructured) []string {
	row := r.plugin.Row(obj)
	if r.namespace != "" {
		return row
	}
	if r.plugin.IsClusterScoped() {
		return row
	}
	result := make([]string, 0, len(row)+1)
	result = append(result, obj.GetNamespace())
	result = append(result, row...)
	return result
}

// Update handles key messages for table navigation.
func (r ResourceList) Update(msg tea.Msg) (ResourceList, tea.Cmd) {
	var cmd tea.Cmd
	r.table, cmd = r.table.Update(msg)
	return r, cmd
}

// View renders the resource list with border and title.
func (r ResourceList) View() string {
	ns := r.namespace
	if ns == "" {
		ns = "All Namespaces"
	}

	nsPrefix := fmt.Sprintf(" %s >", ns)
	if r.plugin.IsClusterScoped() {
		nsPrefix = ""
	}

	if pc := r.ParentContext(); pc != "" {
		nsPrefix += " " + pc + " >"
	}

	pluginLabel := r.plugin.Name()
	var baseTitle string
	if r.HasSelection() {
		if r.filterState.Active() && len(r.allObjects) != len(r.displayObjects) {
			baseTitle = fmt.Sprintf("%s (%d/%d) [%d sel]", pluginLabel, len(r.displayObjects), len(r.allObjects), r.SelectionCount())
		} else {
			baseTitle = fmt.Sprintf("%s (%d) [%d sel]", pluginLabel, len(r.displayObjects), r.SelectionCount())
		}
	} else if r.filterState.Active() && len(r.allObjects) != len(r.displayObjects) {
		baseTitle = fmt.Sprintf("%s (%d/%d)", pluginLabel, len(r.displayObjects), len(r.allObjects))
	} else {
		baseTitle = fmt.Sprintf("%s (%d)", pluginLabel, len(r.displayObjects))
	}

	borderStyle := UnfocusedBorderStyle
	if r.focused {
		borderStyle = FocusedBorderStyle
	}

	content := r.table.View()
	styled := borderStyle.Width(r.width).Height(r.height).Render(content)

	titleRendered := BuildPanelTitleWithPrefix(nsPrefix, baseTitle, r.filterState.DisplayPattern(), r.searchState.DisplayPattern(), r.width, r.inlineSearch)
	return injectBorderTitle(styled, titleRendered, r.focused)
}

// --- Selection methods ---

// ToggleSelect toggles the selection of the row under the cursor and advances the cursor.
func (r *ResourceList) ToggleSelect() {
	idx := r.table.Cursor()
	if idx < 0 || idx >= len(r.displayObjects) {
		return
	}
	uid := r.displayObjects[idx].GetUID()
	if uid == "" {
		return
	}
	if r.selected == nil {
		r.selected = make(map[types.UID]struct{})
	}
	if _, ok := r.selected[uid]; ok {
		delete(r.selected, uid)
		if len(r.selected) == 0 {
			r.selected = nil
		}
	} else {
		r.selected[uid] = struct{}{}
	}
	r.rebindRowStyle()
	r.table.UpdateViewport()
}

// SelectAll selects all display objects. If all are already selected, deselects all.
func (r *ResourceList) SelectAll() {
	if len(r.displayObjects) == 0 {
		return
	}
	if r.SelectionCount() == len(r.displayObjects) {
		r.selected = nil
		r.rebindRowStyle()
		r.table.UpdateViewport()
		return
	}
	r.selected = make(map[types.UID]struct{}, len(r.displayObjects))
	for _, obj := range r.displayObjects {
		if uid := obj.GetUID(); uid != "" {
			r.selected[uid] = struct{}{}
		}
	}
	r.rebindRowStyle()
	r.table.UpdateViewport()
}

// ClearSelection removes all selections.
func (r *ResourceList) ClearSelection() {
	if r.selected == nil {
		return
	}
	r.selected = nil
	r.rebindRowStyle()
	r.table.UpdateViewport()
}

// HasSelection reports whether any rows are selected.
func (r ResourceList) HasSelection() bool {
	return len(r.selected) > 0
}

// SelectionCount returns the number of selected rows visible in the current display.
func (r ResourceList) SelectionCount() int {
	if len(r.selected) == 0 {
		return 0
	}
	count := 0
	for _, obj := range r.displayObjects {
		if _, ok := r.selected[obj.GetUID()]; ok {
			count++
		}
	}
	return count
}

// SelectedObjects returns all selected objects in display order.
func (r *ResourceList) SelectedObjects() []*unstructured.Unstructured {
	if len(r.selected) == 0 {
		return nil
	}
	result := make([]*unstructured.Unstructured, 0, len(r.selected))
	for _, obj := range r.displayObjects {
		if _, ok := r.selected[obj.GetUID()]; ok {
			result = append(result, obj)
		}
	}
	return result
}

// rebindRowStyle rebuilds the RowStyleFunc closure with current selected/displayObjects state.
// Must be called whenever selected or displayObjects changes.
func (r *ResourceList) rebindRowStyle() {
	if len(r.selected) == 0 {
		r.table.RowStyleFunc = nil
		return
	}
	selected := r.selected
	display := r.displayObjects
	r.table.RowStyleFunc = func(index int, isCursor bool) *lipgloss.Style {
		if index < 0 || index >= len(display) {
			return nil
		}
		uid := display[index].GetUID()
		if _, ok := selected[uid]; !ok {
			return nil
		}
		if isCursor {
			return &TableMarkedSelectedStyle
		}
		return &TableMarkedStyle
	}
}

// --- Search / Filter methods ---

// ApplySearch compiles the regex pattern and applies search or filter mode.
// In search mode, all rows remain visible but matches are highlighted and cursor
// jumps to the first match. In filter mode, non-matching rows are hidden.
func (r *ResourceList) ApplySearch(pattern string, mode msgs.SearchMode) error {
	if mode == msgs.SearchModeFilter {
		if err := r.filterState.Compile(pattern, mode); err != nil {
			return err
		}
	} else {
		if err := r.searchState.Compile(pattern, mode); err != nil {
			return err
		}
	}
	r.rebuildDisplay()
	if mode == msgs.SearchModeSearch && r.searchState.Active() {
		indices := r.matchingRowIndices()
		if len(indices) > 0 {
			r.table.SetCursor(indices[0])
			r.searchState.CurrentIdx = 0
			r.searchState.MatchCount = len(indices)
		}
	}
	return nil
}

// ClearSearch removes the active search and restores highlighting.
func (r *ResourceList) ClearSearch() {
	r.searchState.Clear()
	r.rebuildDisplay()
}

// ClearFilter removes the active filter and restores all rows.
func (r *ResourceList) ClearFilter() {
	r.filterState.Clear()
	r.rebuildDisplay()
}

// SearchActive reports whether a search is currently active.
func (r *ResourceList) SearchActive() bool {
	return r.searchState.Active()
}

// FilterActive reports whether a filter is currently active.
func (r *ResourceList) FilterActive() bool {
	return r.filterState.Active()
}

// AnyActive reports whether either search or filter is active.
func (r *ResourceList) AnyActive() bool {
	return r.searchState.Active() || r.filterState.Active()
}

// SearchNext moves the cursor to the next matching row after the current cursor position.
func (r *ResourceList) SearchNext() {
	indices := r.matchingRowIndices()
	if len(indices) == 0 {
		return
	}
	defer r.refreshSearchHighlights()
	cur := r.table.Cursor()
	// Find first match strictly after current cursor
	for i, idx := range indices {
		if idx > cur {
			r.searchState.CurrentIdx = i
			r.searchState.MatchCount = len(indices)
			r.table.SetCursor(idx)
			return
		}
	}
	// Wrap around to first match
	r.searchState.CurrentIdx = 0
	r.searchState.MatchCount = len(indices)
	r.table.SetCursor(indices[0])
}

// SearchPrev moves the cursor to the previous matching row before the current cursor position.
func (r *ResourceList) SearchPrev() {
	indices := r.matchingRowIndices()
	if len(indices) == 0 {
		return
	}
	defer r.refreshSearchHighlights()
	cur := r.table.Cursor()
	// Find last match strictly before current cursor
	for i := len(indices) - 1; i >= 0; i-- {
		if indices[i] < cur {
			r.searchState.CurrentIdx = i
			r.searchState.MatchCount = len(indices)
			r.table.SetCursor(indices[i])
			return
		}
	}
	// Wrap around to last match
	r.searchState.CurrentIdx = len(indices) - 1
	r.searchState.MatchCount = len(indices)
	r.table.SetCursor(indices[len(indices)-1])
}

// --- Private helpers ---

// rebuildDisplay applies filter if active, then renders rows into the table.
func (r *ResourceList) rebuildDisplay() {
	if r.filterState.Active() {
		var filtered []*unstructured.Unstructured
		for _, obj := range r.allObjects {
			row := r.effectiveRow(obj)
			if rowMatchesRegex(row, r.filterState.Re) {
				filtered = append(filtered, obj)
			}
		}
		r.displayObjects = filtered
	} else {
		r.displayObjects = r.allObjects
	}
	r.rebindRowStyle()
	r.renderRows(r.searchHighlightRe())
}

// renderRows builds table rows from displayObjects, optionally highlighting matches.
func (r *ResourceList) renderRows(re *regexp.Regexp) {
	cursor := r.table.Cursor()
	r.lastSearchCursor = cursor
	rows := make([]table.Row, len(r.displayObjects))
	for i, obj := range r.displayObjects {
		row := r.effectiveRow(obj)
		if re != nil {
			for j, cell := range row {
				if i == cursor {
					row[j] = HighlightMatches(cell, re)
				} else {
					row[j] = HighlightMatchesColor(cell, re)
				}
			}
		}
		rows[i] = row
	}

	cols, cw := pluginColumnsToTableColumns(r.effectiveColumns(), r.width-2, r.sortState, rows)
	r.contentWidth = cw
	r.table.SetColumnsAndRows(cols, rows)
	r.table.SetContentWidth(r.contentWidth)
}

// refreshSearchHighlights updates only the old and new cursor rows when
// search highlighting is active. This avoids rebuilding all rows on every
// cursor movement.
func (r *ResourceList) refreshSearchHighlights() {
	re := r.searchHighlightRe()
	if re == nil {
		return
	}

	oldCursor := r.lastSearchCursor
	newCursor := r.table.Cursor()
	r.lastSearchCursor = newCursor

	rows := r.table.Rows()
	if len(rows) == 0 {
		return
	}

	// Re-render old cursor row (demote from reverse-video to color highlight)
	if oldCursor >= 0 && oldCursor < len(r.displayObjects) && oldCursor < len(rows) {
		row := r.effectiveRow(r.displayObjects[oldCursor])
		for j, cell := range row {
			row[j] = HighlightMatchesColor(cell, re)
		}
		rows[oldCursor] = row
	}

	// Re-render new cursor row (promote from color to reverse-video highlight)
	if newCursor >= 0 && newCursor < len(r.displayObjects) && newCursor < len(rows) {
		row := r.effectiveRow(r.displayObjects[newCursor])
		for j, cell := range row {
			row[j] = HighlightMatches(cell, re)
		}
		rows[newCursor] = row
	}

	r.table.SetRows(rows)
	r.table.EnsureCursorVisible()
}

// searchHighlightRe returns the compiled regex if search (not filter) mode is active,
// nil otherwise. Filter mode does not need inline highlighting since non-matching
// rows are already hidden.
func (r *ResourceList) searchHighlightRe() *regexp.Regexp {
	if r.searchState.Active() {
		return r.searchState.Re
	}
	return nil
}

// matchingRowIndices returns the indices of rows in displayObjects that match the
// current search regex.
func (r *ResourceList) matchingRowIndices() []int {
	if !r.searchState.Active() || r.searchState.Re == nil {
		return nil
	}
	var indices []int
	for i, obj := range r.displayObjects {
		row := r.effectiveRow(obj)
		if rowMatchesRegex(row, r.searchState.Re) {
			indices = append(indices, i)
		}
	}
	return indices
}

// rowMatchesRegex checks if any cell in the row matches the regex.
// It strips ANSI from cells before matching.
func rowMatchesRegex(row []string, re *regexp.Regexp) bool {
	for _, cell := range row {
		stripped := ansi.Strip(cell)
		if re.MatchString(stripped) {
			return true
		}
	}
	return false
}

// injectBorderTitle replaces the top border line of a rendered lipgloss box
// with a custom title string. titleRendered must already be styled.
func injectBorderTitle(styled, titleRendered string, focused bool) string {
	lines := strings.Split(styled, "\n")
	if len(lines) == 0 {
		return styled
	}
	lineWidth := lipgloss.Width(lines[0])
	titleWidth := lipgloss.Width(titleRendered)
	if titleWidth+2 >= lineWidth {
		return styled
	}
	border := lipgloss.RoundedBorder()
	borderColor := UnfocusedBorderColor
	if focused {
		borderColor = FocusedBorderColor
	}
	bc := lipgloss.NewStyle().Foreground(borderColor)
	dashCount := max(lineWidth-1-titleWidth-1, 0)
	lines[0] = bc.Render(string(border.TopLeft)) +
		titleRendered +
		bc.Render(strings.Repeat(string(border.Top), dashCount)) +
		bc.Render(string(border.TopRight))
	return strings.Join(lines, "\n")
}

// pluginColumnsToTableColumns converts plugin columns to bubbles/table columns.
// Flex columns are sized to fit content (max cell width) rather than filling
// all remaining space, similar to k9s.
func pluginColumnsToTableColumns(cols []plugin.Column, totalWidth int, sortState SortState, rows []table.Row) ([]table.Column, int) {
	cellPadding := 2 * len(cols)

	result := make([]table.Column, len(cols))
	used := 0
	for i, c := range cols {
		title := c.Title
		if ind := sortState.Indicator(c.Title); ind != "" {
			title = c.Title + " " + ind
		}

		w := c.Width
		if c.Flex {
			w = contentWidth(i, len(title), rows)
		}
		result[i] = table.Column{Title: title, Width: w}
		used += w
	}

	contentWidth := used + cellPadding
	return result, contentWidth
}

// contentWidth returns the minimum width needed for column col: max of header
// width and the widest cell value, plus padding.
func contentWidth(col int, headerWidth int, rows []table.Row) int {
	w := headerWidth
	for _, row := range rows {
		if col < len(row) {
			if vw := ansi.StringWidth(row[col]); vw > w {
				w = vw
			}
		}
	}
	return w + 3
}
