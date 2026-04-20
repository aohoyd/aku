package layout

import (
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
	ZoomSplit                  // focused split fills left column
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
	splits       []ui.ResourceList
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

// AddSplit adds a new split pane for the given plugin.
func (l *Layout) AddSplit(p plugin.ResourcePlugin, namespace string) {
	w, h := l.splitDimensions(l.SplitCount() + 1)
	rl := ui.NewResourceList(p, w, h)
	rl.SetNamespace(namespace)

	// Blur all existing splits
	for i := range l.splits {
		l.splits[i].Blur()
	}

	l.splits = append(l.splits, rl)
	l.focusIdx = len(l.splits) - 1
	l.splits[l.focusIdx].Focus()
	l.recalcSizes()
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
	if len(l.splits) == 1 && l.splitZoomed {
		l.splitZoomed = false
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

// FocusNext cycles focus to the next split (wraps around).
func (l *Layout) FocusNext() {
	if len(l.splits) == 0 {
		return
	}
	l.splits[l.focusIdx].Blur()
	l.focusIdx = (l.focusIdx + 1) % len(l.splits)
	l.splits[l.focusIdx].Focus()
	if l.splitZoomed {
		l.recalcSizes()
	}
}

// FocusPrev cycles focus to the previous split (wraps around).
func (l *Layout) FocusPrev() {
	if len(l.splits) == 0 {
		return
	}
	l.splits[l.focusIdx].Blur()
	l.focusIdx = (l.focusIdx - 1 + len(l.splits)) % len(l.splits)
	l.splits[l.focusIdx].Focus()
	if l.splitZoomed {
		l.recalcSizes()
	}
}

// FocusSplitAt moves focus to the split at the given index.
func (l *Layout) FocusSplitAt(idx int) {
	if idx < 0 || idx >= len(l.splits) {
		return
	}
	l.splits[l.focusIdx].Blur()
	l.focusIdx = idx
	l.splits[l.focusIdx].Focus()
}

// FocusedSplit returns a pointer to the focused split's ResourceList.
func (l *Layout) FocusedSplit() *ui.ResourceList {
	if len(l.splits) == 0 {
		return nil
	}
	return &l.splits[l.focusIdx]
}

// SplitAt returns a pointer to the split at the given index.
func (l *Layout) SplitAt(idx int) *ui.ResourceList {
	if idx < 0 || idx >= len(l.splits) {
		return nil
	}
	return &l.splits[idx]
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
	l.splits[l.focusIdx].BlurBorder()
	l.ActiveDetailPanel().Focus()
}

// FocusResources sets input focus back to the resource list.
func (l *Layout) FocusResources() {
	l.focusTarget = FocusTargetResources
	l.ActiveDetailPanel().Blur()
	if len(l.splits) > 0 {
		l.splits[l.focusIdx].FocusBorder()
	}
}

// ToggleZoomSplit toggles zoom on the focused resource split.
// No-op when there is only one split (nothing to zoom past).
func (l *Layout) ToggleZoomSplit() {
	if l.splitZoomed {
		l.splitZoomed = false
	} else if len(l.splits) > 1 {
		l.splitZoomed = true
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
	if l.splitZoomed && len(l.splits) > 1 {
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

// UpdateFocusedSplit sends a message to the focused split and returns its command.
func (l *Layout) UpdateFocusedSplit(msg tea.Msg) tea.Cmd {
	if len(l.splits) == 0 {
		return nil
	}
	updated, cmd := l.splits[l.focusIdx].Update(msg)
	l.splits[l.focusIdx] = updated
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

// UpdateSplitObjects updates all splits that match the given GVR with new objects.
func (l *Layout) UpdateSplitObjects(p plugin.ResourcePlugin, namespace string, objs []*unstructured.Unstructured) {
	for i := range l.splits {
		if l.splits[i].InDrillDown() {
			continue
		}
		if l.splits[i].Plugin().GVR() == p.GVR() && l.splits[i].EffectiveNamespace() == namespace {
			l.splits[i].SetObjects(objs)
		}
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
		left := l.splits[l.focusIdx].View()
		if !l.rightVisible {
			return left
		}
		right := l.rightPanelView()
		if l.orientation == OrientationHorizontal {
			return lipgloss.JoinVertical(lipgloss.Left, left, right)
		}
		return lipgloss.JoinHorizontal(lipgloss.Top, left, right)

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
		if l.rightVisible {
			// Detail/log fills the pane area. The panel itself is sized to
			// cover the status-bar row on screen, but the rect intentionally
			// stops at l.height per "status-bar row is not a pane" rule.
			l.paneRects = append(l.paneRects, PaneRect{
				X: 0, Y: 0, W: l.width, H: l.height,
				Kind: l.detailPaneKind(),
			})
			return
		}
		// No right panel: zoomed focused split fills the area.
		l.paneRects = append(l.paneRects, PaneRect{
			X: 0, Y: 0, W: l.width, H: l.height,
			Kind: PaneSplit, SplitIdx: l.focusIdx,
		})

	case ZoomSplit:
		if l.orientation == OrientationHorizontal {
			topHeight, bottomHeight := l.panelSizes()
			l.paneRects = append(l.paneRects, PaneRect{
				X: 0, Y: 0, W: l.width, H: topHeight,
				Kind: PaneSplit, SplitIdx: l.focusIdx,
			})
			if l.rightVisible {
				l.paneRects = append(l.paneRects, PaneRect{
					X: 0, Y: topHeight, W: l.width, H: bottomHeight,
					Kind: l.detailPaneKind(),
				})
			}
		} else {
			leftWidth, rightWidth := l.panelSizes()
			l.paneRects = append(l.paneRects, PaneRect{
				X: 0, Y: 0, W: leftWidth, H: l.height,
				Kind: PaneSplit, SplitIdx: l.focusIdx,
			})
			if l.rightVisible {
				l.paneRects = append(l.paneRects, PaneRect{
					X: leftWidth, Y: 0, W: rightWidth, H: l.height,
					Kind: l.detailPaneKind(),
				})
			}
		}

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
		if l.orientation == OrientationHorizontal {
			topHeight, bottomHeight := l.panelSizes()
			for i := range l.splits {
				if i == l.focusIdx {
					l.splits[i].SetSize(l.width, topHeight)
				} else {
					l.splits[i].SetSize(0, 0)
				}
			}
			if l.rightVisible {
				dp.SetSize(l.width, bottomHeight)
			}
		} else {
			leftWidth, rightWidth := l.panelSizes()
			for i := range l.splits {
				if i == l.focusIdx {
					l.splits[i].SetSize(leftWidth, l.height)
				} else {
					l.splits[i].SetSize(0, 0)
				}
			}
			if l.rightVisible {
				dp.SetSize(rightWidth, l.height)
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
