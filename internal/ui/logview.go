package ui

import (
	"fmt"
	"regexp"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/aohoyd/aku/internal/msgs"
	"github.com/aohoyd/aku/internal/theme"
	"github.com/charmbracelet/x/ansi"
)

// compiledHighlight is a pre-compiled highlight rule ready for fast matching.
type compiledHighlight struct {
	re    *regexp.Regexp
	style lipgloss.Style
}

// LogView is a dedicated component for streaming pod logs with autoscroll,
// filtering, search, and theme-configured highlights.
type LogView struct {
	viewport   viewport.Model
	buffer     *RingBuffer
	focused    bool
	borderless bool
	width      int
	height     int

	// Stream metadata
	activeContainer string
	containers      []string

	// UI toggles
	autoscroll  bool // default true
	unavailable bool

	// Filter/Search
	filterState  SearchState
	searchState  SearchState
	inlineSearch string

	// Search match tracking for ANSI-aware highlighting
	matchPositions []matchPosition
	matchIndex     int

	// Highlights
	highlights []compiledHighlight

}

// NewLogView creates a new log view with the given dimensions and ring buffer capacity.
// Highlight rules are compiled once; invalid regexps are silently skipped.
func NewLogView(width, height, bufCapacity int, rules []theme.LogHighlightRule) LogView {
	vp := viewport.New(viewport.WithWidth(width-2), viewport.WithHeight(height-2))
	vp.KeyMap.Left = key.NewBinding()
	vp.KeyMap.Right = key.NewBinding()
	vp.HighlightStyle = lipgloss.NewStyle().Background(theme.SearchMatch).Foreground(theme.SearchFg)
	vp.SelectedHighlightStyle = lipgloss.NewStyle().Background(theme.SearchSelected).Foreground(theme.SearchFg).Bold(true)

	var compiled []compiledHighlight
	for _, rule := range rules {
		re, err := regexp.Compile(rule.Pattern)
		if err != nil {
			continue
		}
		s := lipgloss.NewStyle()
		if rule.Fg != "" {
			s = s.Foreground(lipgloss.Color(rule.Fg))
		}
		if rule.Bg != "" {
			s = s.Background(lipgloss.Color(rule.Bg))
		}
		if rule.Bold {
			s = s.Bold(true)
		}
		compiled = append(compiled, compiledHighlight{re: re, style: s})
	}

	return LogView{
		viewport:   vp,
		buffer:     NewRingBuffer(bufCapacity),
		width:      width,
		height:     height,
		autoscroll: true,
		highlights: compiled,
	}
}

// AppendLine appends a log line to the ring buffer and updates the viewport.
func (lv *LogView) AppendLine(line string) {
	droppedBefore := lv.buffer.Dropped()
	lv.buffer.Append(line)

	if lv.filterState.Active() {
		evicted := lv.buffer.Dropped() > droppedBefore
		if lv.filterState.Re.MatchString(line) || evicted {
			lv.rebuildViewportContent()
		}
		return
	}

	lv.rebuildViewportContent()
}

// rebuildViewportContent rebuilds the viewport content from the ring buffer,
// applying filter, highlights, and search as needed.
func (lv *LogView) rebuildViewportContent() {
	lines := lv.buffer.All()

	// Apply filter if active
	if lv.filterState.Active() {
		var filtered []string
		for _, line := range lines {
			if lv.filterState.Re.MatchString(line) {
				filtered = append(filtered, line)
			}
		}
		lines = filtered
	}

	// Build raw content for search matching (real lines only, no indicator)
	rawContent := strings.Join(lines, "\n")

	// Prepend dropped lines indicator for display only
	hasIndicator := lv.buffer.Dropped() > 0
	if hasIndicator {
		indicator := fmt.Sprintf("~%d lines dropped", lv.buffer.Dropped())
		lines = append([]string{indicator}, lines...)
	}

	// Apply theme highlight rules per line for display
	displayLines := make([]string, len(lines))
	for i, line := range lines {
		displayLines[i] = lv.applyHighlights(line)
	}
	displayContent := strings.Join(displayLines, "\n")

	// Apply search highlights — use ANSI-aware path when display differs from raw
	lv.matchPositions = nil
	lv.matchIndex = -1

	if lv.searchState.Active() {
		rawMatches := lv.searchState.Re.FindAllStringIndex(rawContent, -1)
		positions := computeMatchPositions(rawContent, rawMatches)

		// Shift line indices to account for indicator in display
		if hasIndicator {
			for i := range positions {
				positions[i].line++
			}
		}

		lv.matchPositions = positions
		if len(positions) > 0 {
			lv.matchIndex = 0
		}

		if rawContent == displayContent {
			// No theme highlights and no indicator: viewport can handle plain-text offsets
			lv.viewport.SetContent(displayContent)
			lv.viewport.SetHighlights(rawMatches)
		} else {
			// Theme highlights or indicator present: bake search highlights ourselves
			highlighted := buildHighlightedDisplay(
				displayContent, positions, lv.matchIndex,
				lv.viewport.HighlightStyle, lv.viewport.SelectedHighlightStyle,
			)
			lv.viewport.SetContent(highlighted)
		}
	} else {
		lv.viewport.SetContent(displayContent)
	}

	if lv.autoscroll {
		lv.viewport.GotoBottom()
	}
}

// applyHighlights applies compiled highlight rules to a single line using lipgloss.StyleRanges.
func (lv *LogView) applyHighlights(line string) string {
	if len(lv.highlights) == 0 {
		return line
	}

	var ranges []lipgloss.Range
	for _, h := range lv.highlights {
		matches := h.re.FindAllStringIndex(line, -1)
		for _, m := range matches {
			colStart := ansi.StringWidth(line[:m[0]])
			colEnd := ansi.StringWidth(line[:m[1]])
			ranges = append(ranges, lipgloss.NewRange(colStart, colEnd, h.style))
		}
	}

	if len(ranges) == 0 {
		return line
	}
	return lipgloss.StyleRanges(line, ranges...)
}

// View renders the log view with border and title.
func (lv LogView) View() string {
	if lv.borderless {
		return lv.viewport.View()
	}

	borderStyle := UnfocusedBorderStyle
	if lv.focused {
		borderStyle = FocusedBorderStyle
	}

	content := lv.viewport.View()
	styled := borderStyle.Width(lv.width).Height(lv.height).Render(content)

	baseTitle := lv.buildTitle()
	titleRendered := BuildPanelTitle(baseTitle, lv.filterState.DisplayPattern(), lv.searchState.DisplayPattern(), lv.width, lv.inlineSearch)
	return injectBorderTitle(styled, titleRendered, lv.focused)
}

// buildTitle constructs the title string for the log view border.
func (lv LogView) buildTitle() string {
	if lv.unavailable {
		return "Logs unavailable"
	}

	var sb strings.Builder
	sb.WriteString("Logs")

	if lv.activeContainer != "" {
		fmt.Fprintf(&sb, " [%s]", lv.activeContainer)
	}
	if lv.autoscroll {
		sb.WriteString(" [A]")
	}
	if lv.viewport.SoftWrap {
		sb.WriteString(" [W]")
	}
	if lv.buffer.Dropped() > 0 {
		fmt.Fprintf(&sb, " ~%d", lv.buffer.Dropped())
	}

	return sb.String()
}

// ApplySearch compiles the pattern and applies the given search mode.
func (lv *LogView) ApplySearch(pattern string, mode msgs.SearchMode) error {
	if mode == msgs.SearchModeFilter {
		if err := lv.filterState.Compile(pattern, mode); err != nil {
			return err
		}
	} else {
		if err := lv.searchState.Compile(pattern, mode); err != nil {
			return err
		}
	}
	lv.rebuildViewportContent()
	return nil
}

// ClearSearch removes the active search highlights.
func (lv *LogView) ClearSearch() {
	lv.searchState.Clear()
	lv.matchPositions = nil
	lv.matchIndex = -1
	lv.viewport.ClearHighlights()
	lv.rebuildViewportContent()
}

// ClearFilter removes the active filter and restores all lines.
func (lv *LogView) ClearFilter() {
	lv.filterState.Clear()
	lv.rebuildViewportContent()
}

// SearchActive reports whether a search is active (highlights only).
func (lv LogView) SearchActive() bool {
	return lv.searchState.Active()
}

// FilterActive reports whether a filter is currently active.
func (lv LogView) FilterActive() bool {
	return lv.filterState.Active()
}

// AnyActive reports whether either search or filter is active.
func (lv LogView) AnyActive() bool {
	return lv.searchState.Active() || lv.filterState.Active()
}

// SearchNext navigates to the next search match.
func (lv *LogView) SearchNext() {
	if !lv.searchState.Active() {
		return
	}
	if len(lv.matchPositions) > 0 {
		lv.matchIndex = (lv.matchIndex + 1) % len(lv.matchPositions)
		lv.rebuildSearchHighlights()
		pos := lv.matchPositions[lv.matchIndex]
		lv.viewport.EnsureVisible(pos.line, pos.colStart, pos.colEnd)
	} else {
		lv.viewport.HighlightNext()
	}
}

// SearchPrev navigates to the previous search match.
func (lv *LogView) SearchPrev() {
	if !lv.searchState.Active() {
		return
	}
	if len(lv.matchPositions) > 0 {
		lv.matchIndex--
		if lv.matchIndex < 0 {
			lv.matchIndex = len(lv.matchPositions) - 1
		}
		lv.rebuildSearchHighlights()
		pos := lv.matchPositions[lv.matchIndex]
		lv.viewport.EnsureVisible(pos.line, pos.colStart, pos.colEnd)
	} else {
		lv.viewport.HighlightPrevious()
	}
}

// rebuildSearchHighlights re-applies search highlights with the current matchIndex,
// preserving the viewport scroll position. Unlike rebuildViewportContent, this
// directly builds highlighted display without resetting match state.
func (lv *LogView) rebuildSearchHighlights() {
	y := lv.viewport.YOffset()
	x := lv.viewport.XOffset()

	// Rebuild display lines with theme highlights
	lines := lv.buffer.All()
	if lv.filterState.Active() {
		var filtered []string
		for _, line := range lines {
			if lv.filterState.Re.MatchString(line) {
				filtered = append(filtered, line)
			}
		}
		lines = filtered
	}
	if lv.buffer.Dropped() > 0 {
		indicator := fmt.Sprintf("~%d lines dropped", lv.buffer.Dropped())
		lines = append([]string{indicator}, lines...)
	}
	displayLines := make([]string, len(lines))
	for i, line := range lines {
		displayLines[i] = lv.applyHighlights(line)
	}
	displayContent := strings.Join(displayLines, "\n")

	highlighted := buildHighlightedDisplay(
		displayContent, lv.matchPositions, lv.matchIndex,
		lv.viewport.HighlightStyle, lv.viewport.SelectedHighlightStyle,
	)
	lv.viewport.SetContent(highlighted)
	lv.viewport.SetYOffset(y)
	lv.viewport.SetXOffset(x)
}

// SetSize updates the dimensions of the log view.
func (lv *LogView) SetSize(w, h int) {
	lv.width = w
	lv.height = h
	if lv.borderless {
		lv.viewport.SetWidth(w)
		lv.viewport.SetHeight(h)
	} else {
		lv.viewport.SetWidth(w - 2)
		lv.viewport.SetHeight(h - 2)
	}
}

// SetBorderless enables or disables borderless rendering.
func (lv *LogView) SetBorderless(b bool) {
	lv.borderless = b
}

// Focus marks this log view as focused.
func (lv *LogView) Focus() { lv.focused = true }

// Blur marks this log view as unfocused.
func (lv *LogView) Blur() { lv.focused = false }

// Mode returns DetailLogs, identifying this as a log view component.
func (lv LogView) Mode() msgs.DetailMode {
	return msgs.DetailLogs
}

// SetUnavailable marks the log view as unavailable (no loggable resource focused).
func (lv *LogView) SetUnavailable(v bool) { lv.unavailable = v }

// IsUnavailable reports whether the log view is in the unavailable state.
func (lv LogView) IsUnavailable() bool { return lv.unavailable }

// Autoscroll reports whether autoscroll is enabled.
func (lv LogView) Autoscroll() bool {
	return lv.autoscroll
}

// ToggleAutoscroll toggles autoscroll on/off.
func (lv *LogView) ToggleAutoscroll() {
	lv.autoscroll = !lv.autoscroll
	if lv.autoscroll {
		lv.rebuildViewportContent()
	}
}

// SetInlineSearch sets the inline search input text for rendering in the title.
func (lv *LogView) SetInlineSearch(s string) { lv.inlineSearch = s }

// Update handles messages for viewport scrolling.
func (lv LogView) Update(msg tea.Msg) (LogView, tea.Cmd) {
	var cmd tea.Cmd
	lv.viewport, cmd = lv.viewport.Update(msg)
	return lv, cmd
}

// ScrollUp scrolls the viewport up by one line.
func (lv *LogView) ScrollUp() { lv.viewport.ScrollUp(1) }

// ScrollDown scrolls the viewport down by one line.
func (lv *LogView) ScrollDown() { lv.viewport.ScrollDown(1) }

// ScrollLeft scrolls the viewport left by hScrollStep columns.
func (lv *LogView) ScrollLeft() {
	if !lv.viewport.SoftWrap {
		lv.viewport.ScrollLeft(hScrollStep)
	}
}

// ScrollRight scrolls the viewport right by hScrollStep columns.
func (lv *LogView) ScrollRight() {
	if !lv.viewport.SoftWrap {
		lv.viewport.ScrollRight(hScrollStep)
	}
}

// PageUp scrolls the viewport up by one page.
func (lv *LogView) PageUp() { lv.viewport.PageUp() }

// PageDown scrolls the viewport down by one page.
func (lv *LogView) PageDown() { lv.viewport.PageDown() }

// GotoTop scrolls to the top of the content.
func (lv *LogView) GotoTop() { lv.viewport.GotoTop() }

// GotoBottom scrolls to the bottom of the content.
func (lv *LogView) GotoBottom() { lv.viewport.GotoBottom() }

// ToggleWrap toggles soft-wrap on the viewport. When enabling wrap,
// the horizontal offset is reset since horizontal scroll is a no-op.
func (lv *LogView) ToggleWrap() {
	lv.viewport.SoftWrap = !lv.viewport.SoftWrap
	if lv.viewport.SoftWrap {
		lv.viewport.SetXOffset(0)
	}
}

// SetContainers sets the list of available containers for the log view.
func (lv *LogView) SetContainers(containers []string) {
	lv.containers = containers
}

// Containers returns the list of available containers.
func (lv LogView) Containers() []string {
	return lv.containers
}

// ActiveContainer returns the currently active container name.
func (lv LogView) ActiveContainer() string {
	return lv.activeContainer
}

// SetActiveContainer sets the active container for log streaming.
func (lv *LogView) SetActiveContainer(name string) {
	lv.activeContainer = name
}

// ClearAndRestart resets the ring buffer, clears viewport content,
// and re-enables autoscroll for a fresh log stream.
func (lv *LogView) ClearAndRestart() {
	lv.buffer.Reset()
	lv.viewport.SetContent("")
	lv.viewport.ClearHighlights()
	lv.matchPositions = nil
	lv.matchIndex = -1
	lv.autoscroll = true
	lv.filterState.Clear()
	lv.searchState.Clear()
}
