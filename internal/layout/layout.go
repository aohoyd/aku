package layout

import (
	"slices"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/ui"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	statusBarHeight = 1
	leftPanelRatio  = 0.5
)

// ZoomMode describes which component, if any, is zoomed.
type ZoomMode int

const (
	ZoomNone   ZoomMode = iota // all splits visible
	ZoomSplit                  // focused split fills the entire screen, borderless
	ZoomDetail                 // detail panel fills entire screen
)

// Orientation describes the layout direction.
type Orientation int

const (
	OrientationVertical   Orientation = iota // resources left, details right
	OrientationHorizontal                    // resources top, details bottom
)

// FocusTarget describes which component has input focus.
type FocusTarget int

const (
	FocusTargetResources FocusTarget = iota // resource list has focus
	FocusTargetDetails                      // detail panel has focus
)

// PaneKind identifies the kind of pane at a given screen rect.
type PaneKind int

const (
	PaneSplit  PaneKind = iota // a resource-list split pane
	PaneDetail                 // the detail (describe/yaml) panel
	PaneLog                    // the log view panel
)

// PaneRect is the screen-space rectangle occupied by a pane. It is cached
// after each geometry recompute so app-level hit-testing (mouse) can map
// (x, y) back to a pane without re-deriving the layout math.
type PaneRect struct {
	X, Y, W, H int
	Kind       PaneKind
	SplitIdx   int // only valid when Kind == PaneSplit
}

// Layout manages the left panel splits and optional right panel.
type Layout struct {
	// splits is heterogeneous: each element is a ui.Pane, concretely either a
	// *ui.ResourceList or a *ui.TerminalPane (both live in this slice). Panes are
	// stored as pointers so in-place mutation works and the typed accessors can
	// hand back a live *ui.ResourceList / *ui.TerminalPane.
	splits       []ui.Pane
	focusIdx     int
	rightPanel   *ui.DetailView
	rightVisible bool
	focusTarget  FocusTarget
	splitZoomed  bool
	detailZoomed bool
	orientation  Orientation
	width        int
	height       int
	logView      *ui.LogView
	logMode      bool
	paneRects    []PaneRect
}

// New creates a new layout with the given terminal dimensions.
func New(width, height, logBufSize int, defaultTimeRange string, defaultSinceSeconds int64) Layout {
	contentH := height - statusBarHeight
	dp := ui.NewDetailView(0, contentH)
	lv := ui.NewLogView(0, contentH, logBufSize, defaultTimeRange, defaultSinceSeconds)
	return Layout{
		width:      width,
		height:     contentH,
		rightPanel: &dp,
		logView:    &lv,
	}
}

// AddSplit adds a new split pane for the given plugin, seeded with the given
// namespace and kube-context. The context is set at creation so the pane is
// never observed with an empty context (which would otherwise flicker the
// wrong context badge before a later SetContext landed). The new split is
// inserted directly after the focused pane (at focusIdx+1), not appended.
func (l *Layout) AddSplit(p plugin.ResourcePlugin, namespace, context string) {
	w, h := l.splitDimensions(l.SplitCount() + 1)
	rl := ui.NewResourceList(p, w, h)
	rl.SetNamespace(namespace)
	rl.SetContext(context)

	// Blur all existing splits
	for i := range l.splits {
		l.splits[i].Blur()
	}

	// Under zoom the previously-focused pane carries borderless=true; clear it
	// before inserting so the borderless flag follows the new focus (mirrors
	// FocusNext/FocusPrev/FocusSplitAt) and never desyncs from splitZoomed.
	if l.splitZoomed && len(l.splits) > 0 {
		l.splits[l.focusIdx].SetBorderless(false)
	}

	// Insert the new split directly after the focused pane (not at the end).
	// Stored as a pointer so it is a stable ui.Pane element that can be mutated
	// in place via the typed accessors.
	insertIdx := 0
	if len(l.splits) > 0 {
		insertIdx = l.focusIdx + 1
	}
	l.splits = slices.Insert(l.splits, insertIdx, ui.Pane(&rl))
	l.focusIdx = insertIdx
	l.splits[l.focusIdx].Focus()
	if l.splitZoomed {
		l.splits[l.focusIdx].SetBorderless(true)
	}
	l.recalcSizes()
}

// AddTerminalSplit inserts an already-constructed terminal pane directly after
// the focused pane (mirroring AddSplit's insert-adjacent + focus + recalc
// behavior), blurs the others, focuses the new pane, and recomputes sizes. The
// pane is stored as a ui.Pane element in the heterogeneous splits slice. Sizes
// are corrected by recalcSizes; the App pushes the resulting inner size to the
// session via syncTerminalSizes.
func (l *Layout) AddTerminalSplit(p ui.Pane) {
	for i := range l.splits {
		l.splits[i].Blur()
	}
	// Under zoom the previously-focused pane carries borderless=true; clear it
	// before inserting so the borderless flag follows the new focus (mirrors
	// FocusNext/FocusPrev/FocusSplitAt) and never desyncs from splitZoomed.
	if l.splitZoomed && len(l.splits) > 0 {
		l.splits[l.focusIdx].SetBorderless(false)
	}
	insertIdx := 0
	if len(l.splits) > 0 {
		insertIdx = l.focusIdx + 1
	}
	l.splits = slices.Insert(l.splits, insertIdx, p)
	l.focusIdx = insertIdx
	l.splits[l.focusIdx].Focus()
	if l.splitZoomed {
		l.splits[l.focusIdx].SetBorderless(true)
	}
	l.recalcSizes()
}

// FocusedPane returns the focused pane as a ui.Pane, or nil when there are no
// splits. Unlike FocusedSplit it does not narrow to a resource pane, so callers
// can detect a *ui.TerminalPane via a type assertion.
func (l *Layout) FocusedPane() ui.Pane {
	if len(l.splits) == 0 {
		return nil
	}
	return l.splits[l.focusIdx]
}

// TerminalPaneByID scans the splits for a terminal pane whose ID matches id.
// The layout's splits slice is the single source of truth for live panes, so a
// pane that has been closed (removed from splits) is correctly not found.
func (l *Layout) TerminalPaneByID(id string) (*ui.TerminalPane, bool) {
	for i := range l.splits {
		if tp, ok := l.splits[i].(*ui.TerminalPane); ok && tp.ID() == id {
			return tp, true
		}
	}
	return nil, false
}

// TerminalPaneInnerSize returns the inner (emulator content) size of the
// terminal pane with the given id, derived from the size the layout assigned it
// during recalcSizes. ok is false when no such terminal pane exists, or when the
// pane is hidden (a non-focused split under zoom gets a degenerate 0×0 outer
// size). Hidden panes are reported as not-ok so callers do not forward the
// clamped 1×1 inner size to the remote shell and reflow a background full-screen
// program (vim/less).
func (l *Layout) TerminalPaneInnerSize(id string) (w, h int, ok bool) {
	tp, found := l.TerminalPaneByID(id)
	if !found || tp.IsHidden() {
		return 0, 0, false
	}
	iw, ih := tp.InnerSize()
	return iw, ih, true
}

// CloseCurrentSplit removes the focused split. Returns true if it was the last one (app should quit).
func (l *Layout) CloseCurrentSplit() bool {
	if len(l.splits) <= 1 {
		return true // signal to quit
	}

	l.splits = append(l.splits[:l.focusIdx], l.splits[l.focusIdx+1:]...)
	if l.focusIdx >= len(l.splits) {
		l.focusIdx = len(l.splits) - 1
	}

	// Focus the new current split
	for i := range l.splits {
		if i == l.focusIdx {
			l.splits[i].Focus()
		} else {
			l.splits[i].Blur()
		}
	}
	// The removed pane carried the borderless flag while zoomed, but it is now
	// discarded. If we remain zoomed (a lone pane stays zoomed under the
	// fullscreen-borderless model), move the borderless flag onto the new
	// focused split so splitZoomed and borderless never desync.
	if l.splitZoomed {
		l.splits[l.focusIdx].SetBorderless(true)
	}
	l.recalcSizes()
	return false
}

// SplitCount returns the number of splits.
func (l *Layout) SplitCount() int {
	return len(l.splits)
}

// FocusIndex returns the currently focused split index.
func (l *Layout) FocusIndex() int {
	return l.focusIdx
}

// moveBorderlessFlag keeps the fullscreen-borderless flag in lockstep with split
// focus while zoomed: it clears borderless on the split at `from`, sets it on the
// split at `to`, and recomputes geometry so the newly-focused split renders
// fullscreen. It is a no-op unless splitZoomed is set, so the "zoom follows
// focus" invariant lives in one place instead of being duplicated across every
// focus-move path (FocusNext/FocusPrev/FocusSplitAt). Indices must be valid.
func (l *Layout) moveBorderlessFlag(from, to int) {
	if !l.splitZoomed {
		return
	}
	l.splits[from].SetBorderless(false)
	l.splits[to].SetBorderless(true)
	l.recalcSizes()
}

// FocusNext cycles focus to the next split (wraps around).
func (l *Layout) FocusNext() {
	if len(l.splits) == 0 {
		return
	}
	// If the detail panel currently holds focus, release it before moving the
	// split focus — otherwise both the detail border and the new split border
	// would highlight at once.
	if l.focusTarget == FocusTargetDetails {
		l.ActiveDetailPanel().Blur()
		l.focusTarget = FocusTargetResources
	}
	oldIdx := l.focusIdx
	l.splits[l.focusIdx].Blur()
	l.focusIdx = (l.focusIdx + 1) % len(l.splits)
	l.splits[l.focusIdx].Focus()
	// Zoom follows focus: move the borderless flag to the newly-focused split so
	// it never desyncs from splitZoomed (no-op when not zoomed).
	l.moveBorderlessFlag(oldIdx, l.focusIdx)
}

// FocusPrev cycles focus to the previous split (wraps around).
func (l *Layout) FocusPrev() {
	if len(l.splits) == 0 {
		return
	}
	// If the detail panel currently holds focus, release it before moving the
	// split focus — otherwise both the detail border and the new split border
	// would highlight at once.
	if l.focusTarget == FocusTargetDetails {
		l.ActiveDetailPanel().Blur()
		l.focusTarget = FocusTargetResources
	}
	oldIdx := l.focusIdx
	l.splits[l.focusIdx].Blur()
	l.focusIdx = (l.focusIdx - 1 + len(l.splits)) % len(l.splits)
	l.splits[l.focusIdx].Focus()
	// Zoom follows focus: move the borderless flag to the newly-focused split so
	// it never desyncs from splitZoomed (no-op when not zoomed).
	l.moveBorderlessFlag(oldIdx, l.focusIdx)
}

// FocusSplitAt moves focus to the split at the given index.
func (l *Layout) FocusSplitAt(idx int) {
	if idx < 0 || idx >= len(l.splits) {
		return
	}
	oldIdx := l.focusIdx
	l.splits[l.focusIdx].Blur()
	l.focusIdx = idx
	l.splits[l.focusIdx].Focus()
	// Zoom follows focus: move the borderless flag to the newly-focused split and
	// recompute geometry so the new split renders fullscreen (no-op when not
	// zoomed).
	l.moveBorderlessFlag(oldIdx, l.focusIdx)
}

// MoveFocusedSplit moves the focused split by delta positions in the split
// order (delta -1 = toward the start, +1 = toward the end). No-op at the edges.
func (l *Layout) MoveFocusedSplit(delta int) {
	target := l.focusIdx + delta
	if target < 0 || target >= len(l.splits) {
		return
	}
	// If the detail panel currently holds focus, release it before reordering —
	// move-pane operates on resource-list splits, and leaving the detail focused
	// would highlight both the detail border and the moved split border at once.
	// Matches the release pattern in FocusNext/FocusPrev.
	if l.focusTarget == FocusTargetDetails {
		l.ActiveDetailPanel().Blur()
		l.focusTarget = FocusTargetResources
		if rl, ok := l.splits[l.focusIdx].(*ui.ResourceList); ok {
			rl.FocusBorder()
		}
	}
	l.splits[l.focusIdx], l.splits[target] = l.splits[target], l.splits[l.focusIdx]
	l.focusIdx = target
	l.recalcSizes()
}

// FocusedSplit returns a pointer to the focused split's ResourceList, or nil if
// there are no splits or the focused pane is not a resource pane (e.g. a
// terminal pane). Resource-only operations are gated on a non-nil result.
func (l *Layout) FocusedSplit() *ui.ResourceList {
	if len(l.splits) == 0 {
		return nil
	}
	rl, _ := l.splits[l.focusIdx].(*ui.ResourceList)
	return rl
}

// SplitAt returns a pointer to the split at the given index, or nil when the
// index is out of range or the pane is not a resource pane.
func (l *Layout) SplitAt(idx int) *ui.ResourceList {
	if idx < 0 || idx >= len(l.splits) {
		return nil
	}
	rl, _ := l.splits[idx].(*ui.ResourceList)
	return rl
}

// PaneAtIdx returns the raw ui.Pane at the given split index (any kind:
// resource or terminal), or nil when the index is out of range. Unlike SplitAt
// it does not narrow to a resource pane, so callers can type-assert a
// *ui.TerminalPane.
func (l *Layout) PaneAtIdx(idx int) ui.Pane {
	if idx < 0 || idx >= len(l.splits) {
		return nil
	}
	return l.splits[idx]
}

// ShowRightPanel makes the right panel visible.
func (l *Layout) ShowRightPanel() {
	l.rightVisible = true
	l.recalcSizes()
}

// HideRightPanel hides the right panel.
func (l *Layout) HideRightPanel() {
	if l.detailZoomed {
		l.ActiveDetailPanel().SetBorderless(false)
	}
	l.detailZoomed = false
	l.rightVisible = false
	l.recalcSizes()
}

// RightPanelVisible returns whether the right panel is shown.
func (l *Layout) RightPanelVisible() bool {
	return l.rightVisible
}

// RightPanel returns a pointer to the detail view.
func (l *Layout) RightPanel() *ui.DetailView {
	return l.rightPanel
}

// LogView returns a pointer to the log view.
func (l *Layout) LogView() *ui.LogView { return l.logView }

// ActiveDetailPanel returns the currently active detail panel (log or describe/yaml).
func (l *Layout) ActiveDetailPanel() ui.DetailPanel {
	if l.logMode {
		return l.logView
	}
	return l.rightPanel
}

// IsLogMode reports whether the layout is in log mode.
func (l *Layout) IsLogMode() bool { return l.logMode }

// SetLogMode enables or disables log mode.
func (l *Layout) SetLogMode(on bool) {
	l.logMode = on
	// Pane kind depends on logMode; refresh the cache so PaneAt reflects it.
	l.rebuildPaneRects()
}

// FocusedDetails returns whether the detail panel has input focus.
func (l *Layout) FocusedDetails() bool {
	return l.focusTarget == FocusTargetDetails
}

// FocusedResources returns whether the resource list has input focus.
func (l *Layout) FocusedResources() bool {
	return l.focusTarget == FocusTargetResources
}

// FocusDetails sets input focus to the detail panel.
// No-op if the right panel is not visible or no splits exist.
func (l *Layout) FocusDetails() {
	if !l.rightVisible || len(l.splits) == 0 {
		return
	}
	l.focusTarget = FocusTargetDetails
	if rl, ok := l.splits[l.focusIdx].(*ui.ResourceList); ok {
		rl.BlurBorder()
	}
	l.ActiveDetailPanel().Focus()
}

// FocusResources sets input focus back to the resource list.
func (l *Layout) FocusResources() {
	l.focusTarget = FocusTargetResources
	l.ActiveDetailPanel().Blur()
	if len(l.splits) > 0 {
		if rl, ok := l.splits[l.focusIdx].(*ui.ResourceList); ok {
			rl.FocusBorder()
		}
	}
}

// ToggleZoomSplit toggles fullscreen-borderless zoom on the focused split.
// The focused split fills the entire screen; all other splits and the right
// panel are hidden. A single split can be zoomed. No-op when there are no
// splits. The borderless flag on the focused pane is kept in lockstep with
// splitZoomed (mirrors ToggleZoomDetail).
func (l *Layout) ToggleZoomSplit() {
	if l.splitZoomed {
		l.splitZoomed = false
		if p := l.FocusedPane(); p != nil {
			p.SetBorderless(false)
		}
	} else if len(l.splits) >= 1 {
		l.splitZoomed = true
		l.FocusedPane().SetBorderless(true)
	} else {
		return
	}
	l.recalcSizes()
}

// ToggleZoomDetail toggles fullscreen zoom on the detail panel.
// No-op when the right panel is not visible.
func (l *Layout) ToggleZoomDetail() {
	if l.detailZoomed {
		l.detailZoomed = false
		l.ActiveDetailPanel().SetBorderless(false)
	} else if l.rightVisible {
		l.detailZoomed = true
		l.ActiveDetailPanel().SetBorderless(true)
	} else {
		return
	}
	l.recalcSizes()
}

// EffectiveZoom returns the visual zoom mode (detail > split > none).
func (l *Layout) EffectiveZoom() ZoomMode {
	if l.detailZoomed && l.rightVisible {
		return ZoomDetail
	}
	if l.splitZoomed {
		return ZoomSplit
	}
	return ZoomNone
}

// SplitZoomed returns whether the split zoom flag is set.
func (l *Layout) SplitZoomed() bool { return l.splitZoomed }

// DetailZoomed returns whether the detail zoom flag is set.
func (l *Layout) DetailZoomed() bool { return l.detailZoomed }

// AnyZoomed returns whether any zoom flag is set.
func (l *Layout) AnyZoomed() bool { return l.splitZoomed || l.detailZoomed }

// Orientation returns the current layout orientation.
func (l *Layout) Orientation() Orientation { return l.orientation }

// ToggleOrientation flips between vertical and horizontal layout.
func (l *Layout) ToggleOrientation() {
	if l.orientation == OrientationVertical {
		l.orientation = OrientationHorizontal
	} else {
		l.orientation = OrientationVertical
	}
	l.recalcSizes()
}

// UnzoomAll clears both zoom flags.
func (l *Layout) UnzoomAll() {
	if l.detailZoomed {
		l.ActiveDetailPanel().SetBorderless(false)
	}
	if l.splitZoomed {
		if p := l.FocusedPane(); p != nil {
			p.SetBorderless(false)
		}
	}
	l.splitZoomed = false
	l.detailZoomed = false
	l.recalcSizes()
}

// Resize updates the layout for new terminal dimensions.
func (l *Layout) Resize(width, height int) {
	l.width = width
	l.height = height - statusBarHeight
	l.recalcSizes()
}

// UpdateFocusedSplit sends a message to the focused split and returns its
// command. No-op (returns nil) when there are no splits or the focused pane is
// not a resource pane.
func (l *Layout) UpdateFocusedSplit(msg tea.Msg) tea.Cmd {
	if len(l.splits) == 0 {
		return nil
	}
	rl, ok := l.splits[l.focusIdx].(*ui.ResourceList)
	if !ok {
		return nil
	}
	updated, cmd := rl.Update(msg)
	*rl = updated
	return cmd
}

// UpdateRightPanel sends a message to the right panel and returns its command.
func (l *Layout) UpdateRightPanel(msg tea.Msg) tea.Cmd {
	updated, cmd := l.rightPanel.Update(msg)
	*l.rightPanel = updated
	return cmd
}

// UpdateLogView sends a message to the log view and returns its command.
func (l *Layout) UpdateLogView(msg tea.Msg) tea.Cmd {
	updated, cmd := l.logView.Update(msg)
	*l.logView = updated
	return cmd
}

// UpdateSplitObjects updates all splits that match the given GVR + namespace +
// context with new objects. ctxName is the kube-context the update originated
// from (stamped on k8s.ResourceUpdatedMsg).
//
// Context matching is strict equality: a pane only repaints from an update whose
// originating context equals the pane's own Context(). This is the core
// multi-cluster correctness guarantee — a prod informer tick can never repaint a
// staging-pinned pane. Every split carries a resolved, non-empty context: the
// initial split and global-retargeted panes get the global context, drill-down
// children inherit their parent's context (NavSnapshot.Context), and a pane
// pinned via gX gets the chosen context. A pane with an empty context (which
// should not occur in practice) matches nothing and is left untouched.
func (l *Layout) UpdateSplitObjects(p plugin.ResourcePlugin, namespace, ctxName string, objs []*unstructured.Unstructured) {
	for i := range l.splits {
		rl, ok := l.splits[i].(*ui.ResourceList)
		if !ok {
			continue
		}
		if rl.InDrillDown() {
			continue
		}
		if rl.Plugin().GVR() != p.GVR() || rl.EffectiveNamespace() != namespace {
			continue
		}
		if rl.Context() != ctxName {
			continue
		}
		rl.SetObjects(objs)
	}
}

// rightPanelView returns the rendered right panel, choosing between log view and detail view.
func (l Layout) rightPanelView() string {
	return l.ActiveDetailPanel().View()
}

// View renders the full layout (left splits + optional right panel).
func (l Layout) View() string {
	if len(l.splits) == 0 {
		return ""
	}

	switch l.EffectiveZoom() {
	case ZoomDetail:
		if l.rightVisible {
			return l.rightPanelView()
		}
		return l.splits[l.focusIdx].View()

	case ZoomSplit:
		// Fullscreen-borderless: only the focused split is rendered; the right
		// panel and all other splits are hidden. Orientation-independent.
		return l.splits[l.focusIdx].View()

	default: // ZoomNone
		var splitViews []string
		for _, s := range l.splits {
			splitViews = append(splitViews, s.View())
		}
		if l.orientation == OrientationHorizontal {
			top := lipgloss.JoinHorizontal(lipgloss.Top, splitViews...)
			if !l.rightVisible {
				return top
			}
			right := l.rightPanelView()
			return lipgloss.JoinVertical(lipgloss.Left, top, right)
		}
		left := lipgloss.JoinVertical(lipgloss.Left, splitViews...)
		if !l.rightVisible {
			return left
		}
		right := l.rightPanelView()
		return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	}
}

// PaneAt returns the pane rect that contains the given screen coordinate.
// Coordinates are in the same space used by Resize / the layout's View output;
// the status-bar row is never part of a pane rect, so clicks there yield
// (PaneRect{}, false). Also returns false for out-of-bounds clicks.
func (l *Layout) PaneAt(x, y int) (PaneRect, bool) {
	for _, r := range l.paneRects {
		if x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H {
			return r, true
		}
	}
	return PaneRect{}, false
}

// FocusedSplitRect returns the cached screen rect of the focused split pane
// (resource or terminal — both are stored with Kind == PaneSplit), or
// (PaneRect{}, false) when there are no splits. X,Y is the outer border-corner
// top-left; the inner content begins one cell in on each axis.
func (l *Layout) FocusedSplitRect() (PaneRect, bool) {
	for _, r := range l.paneRects {
		if r.Kind == PaneSplit && r.SplitIdx == l.focusIdx {
			return r, true
		}
	}
	return PaneRect{}, false
}

// detailPaneKind returns PaneLog when the right panel is showing logs,
// PaneDetail otherwise.
func (l *Layout) detailPaneKind() PaneKind {
	if l.logMode {
		return PaneLog
	}
	return PaneDetail
}

// rebuildPaneRects recomputes the cached pane rectangles from current geometry.
// Called at the end of recalcSizes() so every geometry change refreshes the cache.
// The status-bar row (owned by the app, at y == l.height) is never included.
func (l *Layout) rebuildPaneRects() {
	l.paneRects = l.paneRects[:0]
	n := len(l.splits)
	if n == 0 || l.width <= 0 || l.height <= 0 {
		return
	}

	switch l.EffectiveZoom() {
	case ZoomDetail:
		// EffectiveZoom() only returns ZoomDetail when detailZoomed &&
		// rightVisible, so this branch always has rightVisible==true. Detail/log
		// fills the pane area. The panel itself is sized to cover the
		// status-bar row on screen, but the rect intentionally stops at
		// l.height per "status-bar row is not a pane" rule.
		l.paneRects = append(l.paneRects, PaneRect{
			X: 0, Y: 0, W: l.width, H: l.height,
			Kind: l.detailPaneKind(),
		})
		return

	case ZoomSplit:
		// Fullscreen-borderless: a single full-area rect for the focused split.
		// The right panel and other splits are hidden, so no other rects.
		// Orientation-independent. Like ZoomDetail, the rect stops at l.height
		// (the status-bar row is not a pane) even though the pane is sized to
		// cover that row on screen.
		l.paneRects = append(l.paneRects, PaneRect{
			X: 0, Y: 0, W: l.width, H: l.height,
			Kind: PaneSplit, SplitIdx: l.focusIdx,
		})
		return

	default: // ZoomNone
		if l.orientation == OrientationHorizontal {
			topHeight, bottomHeight := l.panelSizes()
			splitWidth := l.width / n
			for i := range l.splits {
				w := splitWidth
				x := splitWidth * i
				if i == n-1 {
					w = l.width - splitWidth*(n-1)
				}
				l.paneRects = append(l.paneRects, PaneRect{
					X: x, Y: 0, W: w, H: topHeight,
					Kind: PaneSplit, SplitIdx: i,
				})
			}
			if l.rightVisible {
				l.paneRects = append(l.paneRects, PaneRect{
					X: 0, Y: topHeight, W: l.width, H: bottomHeight,
					Kind: l.detailPaneKind(),
				})
			}
		} else {
			leftWidth, rightWidth := l.panelSizes()
			splitHeight := l.height / n
			for i := range l.splits {
				h := splitHeight
				y := splitHeight * i
				if i == n-1 {
					h = l.height - splitHeight*(n-1)
				}
				l.paneRects = append(l.paneRects, PaneRect{
					X: 0, Y: y, W: leftWidth, H: h,
					Kind: PaneSplit, SplitIdx: i,
				})
			}
			if l.rightVisible {
				l.paneRects = append(l.paneRects, PaneRect{
					X: leftWidth, Y: 0, W: rightWidth, H: l.height,
					Kind: l.detailPaneKind(),
				})
			}
		}
	}
}

// recalcSizes recomputes sizes for all components.
func (l *Layout) recalcSizes() {
	n := len(l.splits)
	if n == 0 {
		// No splits: rebuildPaneRects clears the cached rect slice. No
		// sizes to assign below.
		l.rebuildPaneRects()
		return
	}

	dp := l.ActiveDetailPanel()

	switch l.EffectiveZoom() {
	case ZoomDetail:
		if l.rightVisible {
			dp.SetSize(l.width, l.height+statusBarHeight)
		}
		for i := range l.splits {
			l.splits[i].SetSize(0, 0)
		}

	case ZoomSplit:
		// Fullscreen-borderless: the focused split fills the entire screen
		// (including the status-bar row, like ZoomDetail) and every other
		// split is zero-sized. The right panel is hidden, so it is not sized.
		// Orientation-independent — one pane fills everything.
		for i := range l.splits {
			if i == l.focusIdx {
				l.splits[i].SetSize(l.width, l.height+statusBarHeight)
			} else {
				l.splits[i].SetSize(0, 0)
			}
		}

	default: // ZoomNone
		if l.orientation == OrientationHorizontal {
			topHeight, bottomHeight := l.panelSizes()
			splitWidth := l.width / n
			for i := range l.splits {
				w := splitWidth
				if i == n-1 {
					w = l.width - splitWidth*(n-1)
				}
				l.splits[i].SetSize(w, topHeight)
			}
			if l.rightVisible {
				dp.SetSize(l.width, bottomHeight)
			}
		} else {
			leftWidth, rightWidth := l.panelSizes()
			splitHeight := l.height / n
			for i := range l.splits {
				h := splitHeight
				if i == n-1 {
					h = l.height - splitHeight*(n-1)
				}
				l.splits[i].SetSize(leftWidth, h)
			}
			if l.rightVisible {
				dp.SetSize(rightWidth, l.height)
			}
		}
		if f := l.FocusedSplit(); f != nil {
			f.EnsureCursorVisible()
		}
	}

	l.rebuildPaneRects()
}

// panelSizes computes the primary and secondary panel dimensions.
// In vertical orientation: returns (leftWidth, rightWidth).
// In horizontal orientation: returns (topHeight, bottomHeight).
func (l *Layout) panelSizes() (int, int) {
	if l.orientation == OrientationHorizontal {
		if !l.rightVisible {
			return l.height, 0
		}
		topHeight := int(float64(l.height) * leftPanelRatio)
		bottomHeight := l.height - topHeight
		return topHeight, bottomHeight
	}
	if !l.rightVisible {
		return l.width, 0
	}
	leftWidth := int(float64(l.width) * leftPanelRatio)
	rightWidth := l.width - leftWidth
	return leftWidth, rightWidth
}

// SplitSeedSize returns a reasonable outer size to seed a freshly-created pane
// with before it is inserted. recalcSizes corrects the size immediately on
// insert; this only avoids constructing the pane at a degenerate 0×0. It mirrors
// splitDimensions for one additional split.
func (l *Layout) SplitSeedSize() (int, int) {
	return l.splitDimensions(l.SplitCount() + 1)
}

// splitDimensions returns the width and height for a new split given the count.
func (l *Layout) splitDimensions(totalSplits int) (int, int) {
	if totalSplits == 0 {
		totalSplits = 1
	}
	if l.orientation == OrientationHorizontal {
		topHeight, _ := l.panelSizes()
		return l.width / totalSplits, topHeight
	}
	leftWidth, _ := l.panelSizes()
	return leftWidth, l.height / totalSplits
}
