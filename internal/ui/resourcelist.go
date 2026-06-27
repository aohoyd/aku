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

// maxBadgeContext caps the width of the context name shown in the top-border
// badge; longer names are truncated with an ellipsis (see truncateContext).
const maxBadgeContext = 20

// ResourceList wraps bubbles/table with plugin-driven columns.
type ResourceList struct {
	plugin         plugin.ResourcePlugin
	table          table.Model
	allObjects     []*unstructured.Unstructured
	displayObjects []*unstructured.Unstructured
	filterState    SearchState
	searchState    SearchState
	sortState      SortState
	namespace      string
	context        string // kube-context this pane is scoped to ("" until app/layer sets it)
	focused        bool
	// selectionActive (true under Focus and under BlurBorder — the detail-focused
	// state) lives solely on table.Model; applyFocus mirrors it there and
	// SelectionActive() reads it back, so there is no second copy to drift.
	borderless   bool
	width        int
	height       int
	contentWidth int
	xOffset      int
	navStack     NavStack
	// navFloor is the minimum nav-stack depth Escape's pop guard may unwind to.
	// Default 0 (no floor). Used by split-opened drills so a split can't unwind
	// to a root it never showed: the split's home drill sets navFloor=1 so
	// Escape can pop frames pushed inside the split, but not the home frame.
	navFloor           int
	inlineSearch       string
	contextLabel       string                 // when non-empty, rendered as a right-aligned top-border badge; "" hides it
	offline            bool                   // when true, the context badge is colored red (offline) instead of green
	selected           map[types.UID]struct{} // multi-select set
	cachedColumnWidths []table.Column
	cachedContentWidth int
	cachedRowCount     int
}

// Compile-time assertion that *ResourceList satisfies the Pane interface.
var _ Pane = (*ResourceList)(nil)

// Title returns a short label identifying this pane's content, derived from the
// plugin name. This is the pane-interface accessor; View() builds the richer
// bordered title (with counts, namespace prefix, search/filter state).
func (r *ResourceList) Title() string { return r.plugin.Name() }

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
	// A new pane is focused with an active selection; push that into the table so
	// the cursor renders without waiting for the first focus transition.
	t.SetSelectionActive(true)

	return ResourceList{
		plugin:       p,
		table:        t,
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
				// Re-overlay highlights so the cursor-row highlight follows the
				// restored cursor when a search is active (no-op otherwise).
				r.applyVisibleHighlights()
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
		// Re-overlay highlights so the cursor-row highlight follows the
		// restored cursor when a search is active (no-op otherwise).
		r.applyVisibleHighlights()
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
func (r *ResourceList) GotoTop() { r.table.GotoTop(); r.applyVisibleHighlights() }

// GotoBottom moves the cursor to the last row.
func (r *ResourceList) GotoBottom() { r.table.GotoBottom(); r.applyVisibleHighlights() }

// PageUp moves the cursor up by one page.
func (r *ResourceList) PageUp() { r.table.MoveUp(r.table.Height()); r.applyVisibleHighlights() }

// PageDown moves the cursor down by one page.
func (r *ResourceList) PageDown() { r.table.MoveDown(r.table.Height()); r.applyVisibleHighlights() }

// CursorUp moves the table cursor up by one row.
func (r *ResourceList) CursorUp() { r.table.MoveUp(1); r.applyVisibleHighlights() }

// CursorDown moves the table cursor down by one row.
func (r *ResourceList) CursorDown() { r.table.MoveDown(1); r.applyVisibleHighlights() }

// SetCursor sets the table cursor to the given row index. Out-of-range values
// are clamped by the underlying table.
func (r *ResourceList) SetCursor(row int) {
	r.table.SetCursor(row)
	r.applyVisibleHighlights()
}

// ScrollWheel advances the cursor by one row in response to a mouse wheel
// event. Up/down reuse CursorUp/CursorDown (same as k/j). Left/right wheel
// and any other button are dropped.
func (r *ResourceList) ScrollWheel(btn tea.MouseButton) {
	switch btn {
	case tea.MouseWheelUp:
		r.CursorUp()
	case tea.MouseWheelDown:
		r.CursorDown()
	}
}

const listScrollStep = 8

// ScrollRight scrolls the resource list right by listScrollStep characters.
func (r *ResourceList) ScrollRight() {
	maxOffset := max(0, r.contentWidth-r.tableWidth())
	r.xOffset = min(r.xOffset+listScrollStep, maxOffset)
	r.table.SetXOffset(r.xOffset)
}

// ScrollLeft scrolls the resource list left by listScrollStep characters.
func (r *ResourceList) ScrollLeft() {
	r.xOffset = max(r.xOffset-listScrollStep, 0)
	r.table.SetXOffset(r.xOffset)
}

// ScrollHome resets horizontal scroll to the beginning.
func (r *ResourceList) ScrollHome() {
	r.xOffset = 0
	r.table.SetXOffset(0)
}

// ScrollEnd scrolls horizontally to show the end of the content.
func (r *ResourceList) ScrollEnd() {
	r.xOffset = max(0, r.contentWidth-r.tableWidth())
	r.table.SetXOffset(r.xOffset)
}

// Cursor returns selected element.
func (r *ResourceList) Cursor() int { return r.table.Cursor() }

// RowAtY maps a y-coordinate (relative to the ResourceList pane's top-left,
// y=0 = top border line that carries the injected title) to an index into the
// underlying display objects. Returns -1 for the border/header area or any y
// past the last data row. Chrome accounted for: 1 line for the top border
// (the title is injected into that border line via injectBorderTitle, so it
// shares the same row); the table's own header row is subtracted inside
// table.RowAtY.
func (r *ResourceList) RowAtY(y int) int {
	// 1 line: top border (bordered) or header (borderless). In both modes a
	// single chrome line sits above the first data row, so the value is the
	// same — the table's own header row is subtracted inside table.RowAtY.
	const topChromeLines = 1
	adjusted := y - topChromeLines
	if adjusted < 0 {
		return -1
	}
	return r.table.RowAtY(adjusted)
}

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

// Context returns the kube-context this list is scoped to. An empty string
// means the pane has no explicitly resolved context yet (set later by the
// app/layout layer).
func (r *ResourceList) Context() string {
	return r.context
}

// SetContext updates the kube-context this list is scoped to.
func (r *ResourceList) SetContext(ctx string) {
	r.context = ctx
}

// ContextLabel returns the context name shown as the pane's top-border badge.
// An empty string means no badge is rendered.
func (r *ResourceList) ContextLabel() string {
	return r.contextLabel
}

// SetContextLabel sets the context name drawn as a right-aligned badge on the
// pane's top border. Passing "" hides the badge. The app owns the "panes use
// more than one context" decision and passes the context name to show (or "" to
// hide); View() stays dumb and renders the badge iff this text is non-empty.
func (r *ResourceList) SetContextLabel(text string) {
	r.contextLabel = text
}

// Offline reports whether the pane's cluster is currently unreachable, which
// colors the context badge red instead of green.
func (r *ResourceList) Offline() bool {
	return r.offline
}

// SetOffline marks the pane's cluster as unreachable (true) or reachable
// (false). When offline, View() colors the context badge red (muted) instead of
// green. The app owns the connectivity decision (driven from the same
// per-context connection state the contexts plugin STATUS uses) and clears it
// automatically once the cluster reconnects; View() stays dumb.
func (r *ResourceList) SetOffline(offline bool) {
	r.offline = offline
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
	r.cachedColumnWidths = nil
	r.cachedRowCount = 0
	r.cachedContentWidth = 0
	cols, cw := pluginColumnsToTableColumns(r.effectiveColumns(), r.tableWidth(), r.sortState, nil)
	r.contentWidth = cw
	r.table.SetColumnsAndRows(cols, nil)
	r.table.SetContentWidth(r.contentWidth)
}

// ResetNav clears the drill-down nav stack.
func (r *ResourceList) ResetNav() {
	r.navStack = NavStack{}
	r.navFloor = 0
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
		// Preserve the pane's cluster across reload. The root snapshot carries the
		// context the pane was bound to; only adopt it when non-empty so a pre-
		// context snapshot can never blank out a resolved context.
		if root.Context != "" {
			r.context = root.Context
		}
		r.sortState = root.SortState
		cols, cw := pluginColumnsToTableColumns(r.effectiveColumns(), r.tableWidth(), r.sortState, nil)
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
	// Reset the Escape pop guard: a fresh root has no split-imposed floor.
	r.navFloor = 0
	// Clear objects — caller will repopulate via SetObjects
	r.allObjects = nil
	r.displayObjects = nil
}

// Width returns the current width.
func (r ResourceList) Width() int { return r.width }

// Height returns the current height.
func (r ResourceList) Height() int { return r.height }

// tableWidth returns the width available to the table given the current mode:
// the full pane width when borderless (fullscreen zoom), or width-2 when
// bordered to reserve the left/right border columns. This is the source of
// truth for all POST-CONSTRUCTION sizing (SetSize/SetBorderless and the
// horizontal-scroll clamp). The constructor hardcodes the equivalent width-2
// literal because the receiver does not yet exist; a new pane is always
// bordered, so the two agree.
func (r *ResourceList) tableWidth() int {
	if r.borderless {
		return r.width
	}
	return r.width - 2
}

// SetSize updates the dimensions.
func (r *ResourceList) SetSize(w, h int) {
	r.width = w
	r.height = h
	r.xOffset = 0
	r.table.SetXOffset(0)
	r.cachedColumnWidths = nil
	r.cachedRowCount = 0
	r.cachedContentWidth = 0
	// Borderless (fullscreen zoom) lays the table out at the full width with a
	// single header line above it; bordered mode reserves the border chrome
	// (w-2 wide, h-3 tall to leave room for the box + bottom border).
	tableW, tableH := r.tableWidth(), h-3
	if r.borderless {
		tableH = h - 1
	}
	cols, cw := pluginColumnsToTableColumns(r.effectiveColumns(), tableW, r.sortState, r.table.Rows())
	r.contentWidth = cw
	r.table.SetLayout(cols, tableW, tableH)
	r.table.SetContentWidth(r.contentWidth)
}

// SetBorderless toggles fullscreen borderless rendering and re-runs the layout
// sizing so the table reflows for the new mode.
func (r *ResourceList) SetBorderless(b bool) {
	r.borderless = b
	r.SetSize(r.width, r.height)
}

// applyFocus is the single setter of the two orthogonal focus bits: `border`
// drives border color + keyboard input (r.focused → table.Focus/Blur), and
// `selection` drives the cursor highlight (pushed into table.Model's live
// selectionActive flag, the single source of truth read by renderRow). It
// re-renders; the RowStyleFunc closure reads `active` live, so no per-focus
// rebind is needed.
func (r *ResourceList) applyFocus(border, selection bool) {
	r.focused = border
	if border {
		r.table.Focus()
	} else {
		r.table.Blur()
	}
	r.table.SetSelectionActive(selection)
	// Snap the viewport to the cursor only on border-taking transitions
	// (Focus); Blur/BlurBorder leave the viewport untouched, so the
	// border-only gate here is intentional, not a missing branch.
	if border {
		r.table.EnsureCursorVisible()
	}
}

// Focus marks this list as focused with an active selection.
func (r *ResourceList) Focus() { r.applyFocus(true, true) }

// EnsureCursorVisible adjusts cursor location.
func (r *ResourceList) EnsureCursorVisible() {
	r.table.EnsureCursorVisible()
}

// Blur marks this list as unfocused, clearing the active selection so the cursor
// row renders identically to its neighbors.
func (r *ResourceList) Blur() { r.applyFocus(false, false) }

// BlurBorder dims only the border, keeping the active selection so the cursor
// stays visible. Used when focus moves to the detail panel.
func (r *ResourceList) BlurBorder() { r.applyFocus(false, true) }

// Focused returns whether this list has focus.
func (r *ResourceList) Focused() bool {
	return r.focused
}

// SelectionActive reports the selection state mirrored into table.Model by
// applyFocus: true under Focus and BlurBorder, false only under
// Blur. When active the selected row keeps its cursor-style fill (accent for
// healthy rows, health-colored fill for warning/error rows). This is distinct
// from Focused(), which reports only border focus: under BlurBorder
// SelectionActive() is true while Focused() is false.
func (r *ResourceList) SelectionActive() bool { return r.table.SelectionActive() }

// PushNav saves current state and switches to a drill-down child view.
func (r *ResourceList) PushNav(childPlugin plugin.ResourcePlugin, children []*unstructured.Unstructured, parentName string, parentUID string, parentAPIVersion string, parentKind string, dir NavDirection) {
	r.selected = nil
	r.navStack.Push(NavSnapshot{
		Plugin:           r.plugin,
		Namespace:        r.namespace,
		Context:          r.context,
		Objects:          r.allObjects,
		Cursor:           r.table.Cursor(),
		SortState:        r.sortState,
		FilterState:      r.filterState,
		SearchState:      r.searchState,
		ParentUID:        parentUID,
		ParentName:       parentName,
		ParentAPIVersion: parentAPIVersion,
		ParentKind:       parentKind,
		Direction:        dir,
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
	r.context = snap.Context
	r.sortState = snap.SortState
	r.filterState = snap.FilterState
	r.searchState = snap.SearchState
	// Rebuild columns for restored plugin
	cols, cw := pluginColumnsToTableColumns(r.effectiveColumns(), r.tableWidth(), r.sortState, nil)
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
		// Re-overlay highlights so the cursor-row highlight follows the
		// restored cursor when a search is active (no-op otherwise).
		r.applyVisibleHighlights()
	}
	return true
}

// InDrillDown reports whether this pane is in a drill-down child view.
func (r *ResourceList) InDrillDown() bool {
	return r.navStack.Depth() > 0
}

// Depth returns the number of frames on this pane's nav stack. Exposed so the
// clear-overlay (Escape) handler can compare against NavFloor to decide whether
// a pop is permitted (Depth > NavFloor) — distinct from InDrillDown, which keeps
// its Depth>0 semantics for the live-refresh path.
func (r *ResourceList) Depth() int {
	return r.navStack.Depth()
}

// NavFloor returns the minimum nav-stack depth Escape may unwind to.
func (r *ResourceList) NavFloor() int {
	return r.navFloor
}

// SetNavFloor sets the minimum nav-stack depth Escape may unwind to. Split-opened
// drills set this to 1 so Escape can pop frames pushed inside the split but not
// the split's home drill.
func (r *ResourceList) SetNavFloor(n int) {
	r.navFloor = n
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

// ParentSnap returns a value copy of the top nav snapshot and an ok flag.
// The returned snapshot is a one-time copy — callers that mutate the nav
// stack (PushNav/PopNav) must re-fetch to see the new top.
func (r *ResourceList) ParentSnap() (NavSnapshot, bool) {
	return r.navStack.Peek()
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

	if r.borderless {
		// Append the context badge into the base title so it stays visible without
		// a border line to host it.
		headerBase := baseTitle
		if r.contextLabel != "" {
			headerBase += " [" + truncateContext(r.contextLabel, maxBadgeContext) + "]"
		}
		titleRendered := BuildPanelTitleWithPrefix(nsPrefix, headerBase, r.filterState.DisplayPattern(), r.searchState.DisplayPattern(), r.width, r.inlineSearch)
		headerLine := DetailHeaderStyle.Width(r.width).Render(titleRendered)
		return lipgloss.JoinVertical(lipgloss.Left, headerLine, r.table.View())
	}

	borderStyle := UnfocusedBorderStyle
	if r.focused {
		borderStyle = FocusedBorderStyle
	}

	content := r.table.View()
	styled := borderStyle.Width(r.width).Height(r.height).Render(content)

	titleRendered := BuildPanelTitleWithPrefix(nsPrefix, baseTitle, r.filterState.DisplayPattern(), r.searchState.DisplayPattern(), r.width, r.inlineSearch)

	// Context badge: when panes use more than one context the app sets
	// contextLabel to this pane's context name (it stays "" otherwise). It is
	// rendered as a right-aligned segment on the top border, colored muted green
	// when the pane's cluster is reachable and muted red when offline. Reusing the
	// top border line means it does not consume a content row, so the table height
	// is unchanged. injectBorderTitle drops the badge on a too-narrow pane rather
	// than truncating the title.
	var rightRendered string
	if r.contextLabel != "" {
		style := PaneContextOnlineStyle
		if r.offline {
			style = PaneContextOfflineStyle
		}
		rightRendered = style.Render(" " + truncateContext(r.contextLabel, maxBadgeContext) + " ")
	}

	return injectBorderTitle(styled, titleRendered, rightRendered, r.focused)
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

// rebindRowStyle rebuilds the RowStyleFunc closure over the current `selected`
// and `displayObjects`. Must be called whenever those (data / marks / display)
// change. Focus / selection-active changes do NOT require a rebind: the closure
// is parameterized on the live `active` argument that renderRow passes in, so
// applyFocus deliberately does not call this.
func (r *ResourceList) rebindRowStyle() {
	selected := r.selected
	display := r.displayObjects
	reporter, _ := r.plugin.(plugin.HealthReporter)
	// Fast path: when nothing is selected AND reporter == nil, the closure can only
	// ever return nil, so we skip installing it to avoid a per-row call.
	// The fast path may ignore `active` only because BOTH conditions hold together:
	// `len(selected) == 0` makes the marks branch (which reads `active` directly) a
	// no-op, and `reporter == nil` makes the active-cursor health branch (which also
	// reads `active`) a no-op. With every `active`-reading branch dead, selection-
	// active state cannot affect the result. Do NOT skip on `reporter == nil && !active`
	// alone: with marks present that would drop the marked-cursor style in active panes.
	if len(selected) == 0 && reporter == nil {
		r.table.RowStyleFunc = nil
		return
	}
	r.table.RowStyleFunc = func(index int, isCursor, active bool) *lipgloss.Style {
		if index < 0 || index >= len(display) {
			return nil
		}
		// Marks win over selection and health. The cursor variant only applies in
		// an active pane; an inactive pane shows the plain mark (no cursor highlight).
		if _, ok := selected[display[index].GetUID()]; ok {
			if isCursor && active {
				return &TableMarkedSelectedStyle
			}
			return &TableMarkedStyle
		}
		// Cursor row in the ACTIVE pane (list or detail focus): k9s-style status fill.
		if isCursor && active && reporter != nil {
			switch reporter.RowHealth(display[index]) {
			case plugin.Warning:
				return &TableHealthWarnCursorStyle
			case plugin.Error:
				return &TableHealthErrorCursorStyle
			default:
				return nil // healthy → table applies Selected
			}
		}
		// Unmarked non-cursor row, OR unmarked cursor row in an INACTIVE pane: standard
		// health tint (renders the cursor row identically to a normal row — no cursor
		// highlight). Marked rows returned earlier.
		if reporter == nil {
			return nil
		}
		switch reporter.RowHealth(display[index]) {
		case plugin.Warning:
			return &TableHealthWarnStyle
		case plugin.Error:
			return &TableHealthErrorStyle
		default:
			return nil
		}
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
	// In search mode all rows stay visible (no filtering), so displayObjects is
	// already the full set and matchingRowIndices is valid before rebuildDisplay.
	// Position the cursor on the first match up front so the single highlight pass
	// inside rebuildDisplay paints the correct (post-jump) cursor row, avoiding a
	// wasted pass that would otherwise highlight the stale cursor position.
	if mode == msgs.SearchModeSearch && r.searchState.Active() {
		indices := r.matchingRowIndices()
		if len(indices) > 0 {
			r.table.SetCursor(indices[0])
			r.searchState.CurrentIdx = 0
			r.searchState.MatchCount = len(indices)
		}
	}
	r.rebuildDisplay()
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
	defer r.applyVisibleHighlights()
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
	defer r.applyVisibleHighlights()
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
	r.renderRows()
	r.applyVisibleHighlights()
}

// renderRows builds plain table rows from displayObjects. Highlighting is
// overlaid separately onto only the visible window by applyVisibleHighlights, so
// per-keystroke cost stays O(visible) rather than O(all rows).
func (r *ResourceList) renderRows() {
	rows := make([]table.Row, len(r.displayObjects))
	for i, obj := range r.displayObjects {
		rows[i] = r.effectiveRow(obj)
	}

	if len(rows) == r.cachedRowCount && r.cachedColumnWidths != nil {
		r.contentWidth = r.cachedContentWidth
		r.table.SetColumnsAndRows(r.cachedColumnWidths, rows)
		r.table.SetContentWidth(r.contentWidth)
	} else {
		cols, cw := pluginColumnsToTableColumns(r.effectiveColumns(), r.tableWidth(), r.sortState, rows)
		r.cachedColumnWidths = cols
		r.cachedContentWidth = cw
		r.cachedRowCount = len(rows)
		r.contentWidth = cw
		r.table.SetColumnsAndRows(cols, rows)
		r.table.SetContentWidth(r.contentWidth)
	}
}

// applyVisibleHighlights overlays search highlighting onto only the rows in the
// table's rendered window [start, end) — VisibleRange returns up to ~2*viewport
// height rows around the cursor, not just the on-screen rows. Rows outside this
// window are left plain (or stale); they are repainted fresh the next time they
// enter the window. This keeps per-keystroke and per-cursor-move highlight cost
// proportional to the rendered window (~2*height) rather than O(all rows). The
// cursor row gets reverse-video highlighting; other windowed rows get the themed
// color. Early-returns when search highlighting is not active.
func (r *ResourceList) applyVisibleHighlights() {
	re := r.searchHighlightRe()
	if re == nil {
		return
	}

	// Settle the viewport around the (already-positioned) cursor before reading
	// the window, so the range reflects the rows that will actually render.
	r.table.EnsureCursorVisible()
	start, end := r.table.VisibleRange()
	rows := r.table.Rows()
	cursor := r.table.Cursor()

	for i := start; i < end && i < len(r.displayObjects) && i < len(rows); i++ {
		row := r.effectiveRow(r.displayObjects[i])
		// Rows the table will tint with a whole-row override (mark / health) have
		// their per-cell color stripped at render time, which would also erase a
		// themed-color match marker. Use reverse-video for those rows so the match
		// stays visible on the tint; reverse-video survives the strip.
		reverse := i == cursor
		// The probe asks "would this non-cursor row get a whole-row tint?". Every
		// branch in the closure that reads `active` (the marked-cursor and
		// active-cursor-health branches) is guarded by isCursor; this probe passes
		// isCursor=false, so none of them fire and the `active` arg is dead. Pass a
		// constant false to make that independence explicit.
		if !reverse && r.table.RowStyleFunc != nil && r.table.RowStyleFunc(i, false, false) != nil {
			reverse = true
		}
		for j, cell := range row {
			if reverse {
				row[j] = HighlightMatches(cell, re)
			} else {
				row[j] = HighlightMatchesColor(cell, re)
			}
		}
		rows[i] = row
	}

	r.table.SetRows(rows)
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
// with a custom title string and an optional right-aligned segment. Both
// titleRendered and rightRendered must already be styled; pass "" for
// rightRendered when there is no right segment.
//
// Layout: TopLeft + title + dashes + right + TopRight. The right segment is
// included only when it fits with at least one dash between it and the title;
// otherwise it is dropped (the title is never truncated to make room — matching
// the graceful-skip the title itself already uses when it does not fit).
func injectBorderTitle(styled, titleRendered, rightRendered string, focused bool) string {
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

	// Drop the right segment unless it fits with at least one dash separating it
	// from the title (corners + title + 1 dash + right <= line width).
	rightWidth := lipgloss.Width(rightRendered)
	if rightRendered != "" && titleWidth+2+1+rightWidth > lineWidth {
		rightRendered = ""
		rightWidth = 0
	}

	dashCount := max(lineWidth-1-titleWidth-rightWidth-1, 0)
	lines[0] = bc.Render(string(border.TopLeft)) +
		titleRendered +
		bc.Render(strings.Repeat(string(border.Top), dashCount)) +
		rightRendered +
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
