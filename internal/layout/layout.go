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

// FocusTarget describes which component has input focus.
type FocusTarget int

const (
	FocusTargetResources FocusTarget = iota // resource list has focus
	FocusTargetDetails                      // detail panel has focus
)

// Layout manages the left panel splits and optional right panel.
type Layout struct {
	splits       []ui.ResourceList
	focusIdx     int
	rightPanel   *ui.DetailView
	rightVisible bool
	focusTarget  FocusTarget
	splitZoomed  bool
	detailZoomed bool
	width        int
	height       int
	logView      *ui.LogView
	logMode      bool
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
func (l *Layout) SetLogMode(on bool) { l.logMode = on }

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
		return lipgloss.JoinHorizontal(lipgloss.Top, left, right)

	default: // ZoomNone
		var splitViews []string
		for _, s := range l.splits {
			splitViews = append(splitViews, s.View())
		}
		left := lipgloss.JoinVertical(lipgloss.Left, splitViews...)
		if !l.rightVisible {
			return left
		}
		right := l.rightPanelView()
		return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	}
}

// recalcSizes recomputes sizes for all components.
func (l *Layout) recalcSizes() {
	n := len(l.splits)
	if n == 0 {
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
		leftWidth, rightWidth := l.panelWidths()
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

	default: // ZoomNone
		leftWidth, rightWidth := l.panelWidths()
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
		if f := l.FocusedSplit(); f != nil {
			f.EnsureCursorVisible()
		}
	}
}

// panelWidths computes left and right panel widths.
func (l *Layout) panelWidths() (int, int) {
	if !l.rightVisible {
		return l.width, 0
	}
	leftWidth := int(float64(l.width) * leftPanelRatio)
	rightWidth := l.width - leftWidth
	return leftWidth, rightWidth
}

// splitDimensions returns the width and height for a new split given the count.
func (l *Layout) splitDimensions(totalSplits int) (int, int) {
	leftWidth, _ := l.panelWidths()
	if totalSplits == 0 {
		totalSplits = 1
	}
	return leftWidth, l.height / totalSplits
}
