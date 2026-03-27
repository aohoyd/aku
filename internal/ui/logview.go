package ui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/aohoyd/aku/internal/highlight"
	"github.com/aohoyd/aku/internal/msgs"
	"github.com/aohoyd/aku/internal/theme"
	"github.com/charmbracelet/x/ansi"
)

// LogView is a dedicated component for streaming pod logs with autoscroll,
// filtering, search, and built-in syntax highlighting.
type LogView struct {
	buffer     *DualRingBuffer
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
	pipeline *highlight.Pipeline

	// Time range
	timeRangeLabel      string
	defaultTimeRange    string
	defaultSinceSeconds int64

	// Virtual scroll
	scrollOffset    int   // top-of-viewport line index in display list
	filteredIndices []int // when filter active, logical buffer indices of matching lines

	// Custom viewport for non-wrap rendering
	logVP                  logViewport
	highlightStyle         lipgloss.Style
	selectedHighlightStyle lipgloss.Style
	softWrap               bool

	// Wrap-mode scroll tracking
	wrapYOffset      int // visual row offset for wrap mode
	totalWrappedRows int // total visual rows (sum of all wrapped heights)
}

// NewLogView creates a new log view with the given dimensions and ring buffer capacity.
func NewLogView(width, height, bufCapacity int, defaultTimeRange string, defaultSinceSeconds int64) LogView {
	var logVP logViewport
	logVP.SetSize(width-2, height-2)

	return LogView{
		buffer:                 NewDualRingBuffer(bufCapacity),
		width:                  width,
		height:                 height,
		autoscroll:             true,
		pipeline:               highlight.DefaultPipeline(),
		timeRangeLabel:         defaultTimeRange,
		defaultTimeRange:       defaultTimeRange,
		defaultSinceSeconds:    defaultSinceSeconds,
		logVP:                  logVP,
		highlightStyle:         lipgloss.NewStyle().Background(theme.SearchMatch).Foreground(theme.SearchFg),
		selectedHighlightStyle: lipgloss.NewStyle().Background(theme.SearchSelected).Foreground(theme.SearchFg).Bold(true),
	}
}

// viewportHeight returns the number of visible lines in the viewport.
func (lv *LogView) viewportHeight() int {
	if lv.borderless {
		return lv.height
	}
	return lv.height - 2
}

// totalDisplayLines returns the total number of display lines (with indicator if present).
func (lv *LogView) totalDisplayLines() int {
	n := lv.buffer.Len()
	if lv.filterState.Active() {
		n = len(lv.filteredIndices)
	}
	if lv.buffer.Dropped() > 0 {
		n++
	}
	return n
}

// clampScrollOffset ensures scrollOffset is within valid bounds.
func (lv *LogView) clampScrollOffset() {
	total := lv.totalDisplayLines()
	h := lv.viewportHeight()
	maxOffset := total - h
	if maxOffset < 0 {
		maxOffset = 0
	}
	if lv.scrollOffset > maxOffset {
		lv.scrollOffset = maxOffset
	}
	if lv.scrollOffset < 0 {
		lv.scrollOffset = 0
	}
}

// updateViewport renders the visible window of lines into the viewport.
// This is the fast path — O(H) instead of O(N).
func (lv *LogView) updateViewport() {
	if lv.softWrap {
		lv.updateViewportWrapped()
		return
	}
	lv.clampScrollOffset()
	h := lv.viewportHeight()
	hasIndicator := lv.buffer.Dropped() > 0

	// Compute the visible line range
	var total int
	if lv.filterState.Active() {
		total = len(lv.filteredIndices)
	} else {
		total = lv.buffer.Len()
	}
	indicatorOffset := 0
	if hasIndicator {
		indicatorOffset = 1
	}
	start := lv.scrollOffset - indicatorOffset
	if start < 0 {
		start = 0
	}
	end := start + h
	if hasIndicator && lv.scrollOffset == 0 {
		end--
	}
	if end > total {
		end = total
	}
	if start > total {
		start = total
	}

	// Extract the visible window with widths
	var window []string
	var widths []int
	if lv.filterState.Active() {
		for _, idx := range lv.filteredIndices[start:end] {
			window = append(window, lv.buffer.ColoredGet(idx))
			widths = append(widths, lv.buffer.WidthGet(idx))
		}
	} else {
		window = lv.buffer.ColoredSlice(start, end)
		widths = lv.buffer.WidthSlice(start, end)
	}

	// Prepend indicator if visible
	if hasIndicator && lv.scrollOffset == 0 {
		indicator := fmt.Sprintf("~%d lines dropped", lv.buffer.Dropped())
		window = append([]string{indicator}, window...)
		widths = append([]int{len(indicator)}, widths...) // plain ASCII, len is fine
	}

	// Bake search highlights into window if search active
	if lv.searchState.Active() && len(lv.matchPositions) > 0 {
		windowStart := lv.scrollOffset
		windowEnd := lv.scrollOffset + len(window)
		var windowPositions []matchPosition
		for _, pos := range lv.matchPositions {
			if pos.line >= windowStart && pos.line < windowEnd {
				shifted := pos
				shifted.line -= windowStart
				windowPositions = append(windowPositions, shifted)
			}
		}
		if len(windowPositions) > 0 {
			selectedInWindow := -1
			if lv.matchIndex >= 0 {
				sel := lv.matchPositions[lv.matchIndex]
				if sel.line >= windowStart && sel.line < windowEnd {
					for i, wp := range windowPositions {
						if wp.line == sel.line-windowStart && wp.colStart == sel.colStart {
							selectedInWindow = i
							break
						}
					}
				}
			}
			joined := strings.Join(window, "\n")
			highlighted := buildHighlightedDisplay(
				joined, windowPositions, selectedInWindow,
				lv.highlightStyle, lv.selectedHighlightStyle,
			)
			window = strings.Split(highlighted, "\n")
		}
	}

	lv.logVP.SetLines(window, widths)
}

// updateViewportWrapped computes the visible window of wrapped lines and feeds
// them to logVP. Instead of delegating scrolling to the bubbles viewport, it
// tracks visual row offsets (wrapYOffset / totalWrappedRows) so that the bubbles
// viewport is no longer needed for wrap-mode rendering.
func (lv *LogView) updateViewportWrapped() {
	vpWidth := lv.logVP.width
	vpHeight := lv.viewportHeight()

	// Build full list of colored lines and widths
	var allLines []string
	var allWidths []int
	if lv.filterState.Active() {
		for _, idx := range lv.filteredIndices {
			allLines = append(allLines, lv.buffer.ColoredGet(idx))
			allWidths = append(allWidths, lv.buffer.WidthGet(idx))
		}
	} else {
		allLines = lv.buffer.ColoredAll()
		allWidths = lv.buffer.WidthSlice(0, lv.buffer.Len())
	}

	// Prepend indicator if applicable
	if lv.buffer.Dropped() > 0 {
		indicator := fmt.Sprintf("~%d lines dropped", lv.buffer.Dropped())
		allLines = append([]string{indicator}, allLines...)
		allWidths = append([]int{len(indicator)}, allWidths...)
	}

	// Bake search highlights
	if lv.searchState.Active() && len(lv.matchPositions) > 0 {
		selectedInWindow := -1
		if lv.matchIndex >= 0 && lv.matchIndex < len(lv.matchPositions) {
			selectedInWindow = lv.matchIndex
		}
		joined := strings.Join(allLines, "\n")
		highlighted := buildHighlightedDisplay(
			joined, lv.matchPositions, selectedInWindow,
			lv.highlightStyle, lv.selectedHighlightStyle,
		)
		allLines = strings.Split(highlighted, "\n")
	}

	// Compute wrapped heights and total
	if vpWidth <= 0 {
		vpWidth = 1
	}
	lv.totalWrappedRows = 0
	wrappedHeights := make([]int, len(allLines))
	for i, w := range allWidths {
		if w <= vpWidth {
			wrappedHeights[i] = 1
		} else {
			wrappedHeights[i] = (w + vpWidth - 1) / vpWidth // ceiling division
		}
		lv.totalWrappedRows += wrappedHeights[i]
	}

	// Clamp wrapYOffset
	maxOffset := lv.totalWrappedRows - vpHeight
	if maxOffset < 0 {
		maxOffset = 0
	}
	if lv.wrapYOffset > maxOffset {
		lv.wrapYOffset = maxOffset
	}
	if lv.wrapYOffset < 0 {
		lv.wrapYOffset = 0
	}

	// Autoscroll: snap to bottom
	if lv.autoscroll {
		lv.wrapYOffset = maxOffset
	}

	// Find the first visible logical line
	row := 0
	firstLine := 0
	vOffset := 0
	for i, h := range wrappedHeights {
		if row+h > lv.wrapYOffset {
			firstLine = i
			vOffset = lv.wrapYOffset - row
			break
		}
		row += h
		if i == len(wrappedHeights)-1 {
			firstLine = i
			vOffset = 0
		}
	}

	// Build visual rows for the visible window
	var visRows []string
	var visWidths []int
	for i := firstLine; i < len(allLines) && len(visRows) < vpHeight+vOffset; i++ {
		line := allLines[i]
		w := 0
		if i < len(allWidths) {
			w = allWidths[i]
		}
		if w <= vpWidth {
			// Line fits in one row
			visRows = append(visRows, line)
			visWidths = append(visWidths, w)
		} else {
			// Wrap: use ansi.Cut to split into vpWidth-wide segments
			offset := 0
			for offset < w {
				end := offset + vpWidth
				if end > w {
					end = w
				}
				segment := ansi.Cut(line, offset, end)
				segWidth := end - offset
				visRows = append(visRows, segment)
				visWidths = append(visWidths, segWidth)
				offset += vpWidth
			}
		}
	}

	// Trim to viewport: skip vOffset rows from top, take vpHeight rows
	if vOffset > 0 && vOffset < len(visRows) {
		visRows = visRows[vOffset:]
		visWidths = visWidths[vOffset:]
	}
	if len(visRows) > vpHeight {
		visRows = visRows[:vpHeight]
		visWidths = visWidths[:vpHeight]
	}

	lv.logVP.SetLines(visRows, visWidths)
}

// AppendLine appends a log line to the ring buffer and updates the viewport.
func (lv *LogView) AppendLine(line string) {
	droppedBefore := lv.buffer.Dropped()
	colored := lv.pipeline.Highlight(line)
	lv.buffer.Append(line, colored, ansi.StringWidth(colored))

	if lv.filterState.Active() {
		evicted := lv.buffer.Dropped() > droppedBefore
		// On eviction, decrement existing indices first since logical index 0 shifted
		if evicted && len(lv.filteredIndices) > 0 {
			for i := range lv.filteredIndices {
				lv.filteredIndices[i]--
			}
			for len(lv.filteredIndices) > 0 && lv.filteredIndices[0] < 0 {
				lv.filteredIndices = lv.filteredIndices[1:]
			}
		}
		// Then add the new line if it matches
		matched := lv.filterState.Re.MatchString(line)
		if matched {
			lv.filteredIndices = append(lv.filteredIndices, lv.buffer.Len()-1)
		}
		if !matched && !evicted {
			return // no viewport change needed
		}
	}

	if lv.autoscroll && !lv.softWrap {
		lv.scrollOffset = lv.totalDisplayLines() - lv.viewportHeight()
	}
	lv.updateViewport()
}


// rebuildViewportContent recomputes search match positions and updates the viewport.
// Called on search/filter activation (user action), not on every append.
func (lv *LogView) rebuildViewportContent() {
	coloredLines := lv.buffer.ColoredAll()

	var displayLines []string
	if lv.filterState.Active() {
		for _, idx := range lv.filteredIndices {
			if idx >= 0 && idx < len(coloredLines) {
				displayLines = append(displayLines, coloredLines[idx])
			}
		}
	} else {
		displayLines = coloredLines
	}

	hasIndicator := lv.buffer.Dropped() > 0
	if hasIndicator {
		displayLines = append([]string{fmt.Sprintf("~%d lines dropped", lv.buffer.Dropped())}, displayLines...)
	}

	lv.matchPositions = nil
	lv.matchIndex = -1

	if lv.searchState.Active() {
		searchLines := displayLines
		if hasIndicator {
			searchLines = displayLines[1:]
		}
		strippedLines := make([]string, len(searchLines))
		for i, hl := range searchLines {
			strippedLines[i] = ansi.Strip(hl)
		}
		strippedContent := strings.Join(strippedLines, "\n")

		rawMatches := lv.searchState.Re.FindAllStringIndex(strippedContent, -1)
		positions := computeMatchPositions(strippedContent, rawMatches)

		if hasIndicator {
			for i := range positions {
				positions[i].line++
			}
		}

		lv.matchPositions = positions
		if len(positions) > 0 {
			lv.matchIndex = 0
		}
	}

	lv.updateViewport()
}

// View renders the log view with border and title.
func (lv LogView) View() string {
	if lv.borderless {
		return lv.logVP.View()
	}

	borderStyle := UnfocusedBorderStyle
	if lv.focused {
		borderStyle = FocusedBorderStyle
	}

	content := lv.logVP.View() // now used for BOTH modes
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
	if lv.timeRangeLabel != "" {
		sb.WriteString(" " + lv.timeRangeLabel)
	}
	if lv.autoscroll {
		sb.WriteString(" [A]")
	}
	if lv.softWrap {
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
		// Rebuild filtered indices
		lv.filteredIndices = nil
		for i := range lv.buffer.Len() {
			if lv.filterState.Re.MatchString(lv.buffer.RawGet(i)) {
				lv.filteredIndices = append(lv.filteredIndices, i)
			}
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
	lv.updateViewport()
}

// ClearFilter removes the active filter and restores all lines.
func (lv *LogView) ClearFilter() {
	lv.filterState.Clear()
	lv.filteredIndices = nil
	lv.updateViewport()
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

// ensureMatchVisible adjusts scrollOffset so the current match is visible.
func (lv *LogView) ensureMatchVisible() {
	if lv.matchIndex < 0 || lv.matchIndex >= len(lv.matchPositions) {
		return
	}
	pos := lv.matchPositions[lv.matchIndex]
	if lv.softWrap {
		// Compute visual row for the match line.
		// pos.line is in display-line coordinates (indicator at 0 if present).
		// Convert to buffer-line index for the width walk.
		vpWidth := lv.logVP.width
		if vpWidth <= 0 {
			vpWidth = 1
		}
		visualRow := 0
		bufferLine := pos.line
		if lv.buffer.Dropped() > 0 {
			bufferLine-- // subtract indicator row
			visualRow++  // indicator occupies 1 visual row
		}
		total := lv.buffer.Len()
		if lv.filterState.Active() {
			total = len(lv.filteredIndices)
		}
		for i := 0; i < total && i < bufferLine; i++ {
			var w int
			if lv.filterState.Active() {
				w = lv.buffer.WidthGet(lv.filteredIndices[i])
			} else {
				w = lv.buffer.WidthGet(i)
			}
			if w <= vpWidth {
				visualRow++
			} else {
				visualRow += (w + vpWidth - 1) / vpWidth
			}
		}

		vpHeight := lv.viewportHeight()
		if visualRow < lv.wrapYOffset {
			lv.wrapYOffset = visualRow
		} else if visualRow >= lv.wrapYOffset+vpHeight {
			lv.wrapYOffset = visualRow - vpHeight + 1
		}
		lv.autoscroll = false
		lv.updateViewportWrapped()
		return
	}
	h := lv.viewportHeight()
	if pos.line < lv.scrollOffset {
		lv.scrollOffset = pos.line
	} else if pos.line >= lv.scrollOffset+h {
		lv.scrollOffset = pos.line - h + 1
	}
	lv.autoscroll = false
}

// SearchNext navigates to the next search match.
func (lv *LogView) SearchNext() {
	if !lv.searchState.Active() || len(lv.matchPositions) == 0 {
		return
	}
	lv.matchIndex = (lv.matchIndex + 1) % len(lv.matchPositions)
	lv.ensureMatchVisible()
	lv.updateViewport()
}

// SearchPrev navigates to the previous search match.
func (lv *LogView) SearchPrev() {
	if !lv.searchState.Active() || len(lv.matchPositions) == 0 {
		return
	}
	lv.matchIndex--
	if lv.matchIndex < 0 {
		lv.matchIndex = len(lv.matchPositions) - 1
	}
	lv.ensureMatchVisible()
	lv.updateViewport()
}


// SetSize updates the dimensions of the log view.
func (lv *LogView) SetSize(w, h int) {
	lv.width = w
	lv.height = h
	if lv.borderless {
		lv.logVP.SetSize(w, h)
	} else {
		lv.logVP.SetSize(w-2, h-2)
	}
	lv.updateViewport()
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

// SyntaxEnabled reports whether built-in syntax highlighting is enabled.
func (lv LogView) SyntaxEnabled() bool {
	return lv.pipeline != nil
}

// ToggleSyntax toggles built-in syntax highlighting on/off and re-renders.
func (lv *LogView) ToggleSyntax() {
	if lv.pipeline != nil {
		lv.pipeline = nil
	} else {
		lv.pipeline = highlight.DefaultPipeline()
	}
	for i := range lv.buffer.Len() {
		colored := lv.pipeline.Highlight(lv.buffer.RawGet(i))
		lv.buffer.SetColored(i, colored, ansi.StringWidth(colored))
	}
	if lv.searchState.Active() {
		lv.rebuildViewportContent()
	} else {
		lv.updateViewport()
	}
}

// ToggleAutoscroll toggles autoscroll on/off.
func (lv *LogView) ToggleAutoscroll() {
	lv.autoscroll = !lv.autoscroll
	if lv.autoscroll {
		if lv.softWrap {
			maxOff := lv.totalWrappedRows - lv.viewportHeight()
			if maxOff < 0 {
				maxOff = 0
			}
			lv.wrapYOffset = maxOff
			lv.updateViewportWrapped()
		} else {
			lv.scrollOffset = lv.totalDisplayLines() - lv.viewportHeight()
			lv.updateViewport()
		}
	}
}

// InsertMarker inserts a dimmed timestamp marker line into the log buffer.
func (lv *LogView) InsertMarker() {
	raw := fmt.Sprintf("--- %s ---", time.Now().Format("15:04:05"))
	styled := lipgloss.NewStyle().Foreground(theme.Muted).Faint(true).Render(raw)
	lv.buffer.Append(raw, styled, ansi.StringWidth(raw))
	if lv.filterState.Active() {
		lv.filteredIndices = append(lv.filteredIndices, lv.buffer.Len()-1)
	}
	if lv.autoscroll && !lv.softWrap {
		lv.scrollOffset = lv.totalDisplayLines() - lv.viewportHeight()
	}
	lv.updateViewport()
}

// BufferLen returns the number of lines in the log buffer.
func (lv *LogView) BufferLen() int {
	return lv.buffer.Len()
}

// SetInlineSearch sets the inline search input text for rendering in the title.
func (lv *LogView) SetInlineSearch(s string) { lv.inlineSearch = s }

// Update handles messages for viewport scrolling.
func (lv LogView) Update(msg tea.Msg) (LogView, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.MouseWheelMsg:
		switch msg.Button {
		case tea.MouseWheelDown:
			lv.ScrollDown()
		case tea.MouseWheelUp:
			lv.ScrollUp()
		case tea.MouseWheelLeft:
			lv.ScrollLeft()
		case tea.MouseWheelRight:
			lv.ScrollRight()
		}
	}
	return lv, nil
}

// ScrollUp scrolls the viewport up by one line.
func (lv *LogView) ScrollUp() {
	if lv.softWrap {
		lv.wrapYOffset--
		if lv.wrapYOffset < 0 {
			lv.wrapYOffset = 0
		}
		lv.autoscroll = false
		lv.updateViewportWrapped()
		return
	}
	lv.scrollOffset--
	lv.autoscroll = false
	lv.updateViewport()
}

// ScrollDown scrolls the viewport down by one line.
func (lv *LogView) ScrollDown() {
	if lv.softWrap {
		lv.wrapYOffset++
		lv.updateViewportWrapped() // this will clamp
		maxOff := lv.totalWrappedRows - lv.viewportHeight()
		if maxOff < 0 {
			maxOff = 0
		}
		if lv.wrapYOffset >= maxOff {
			lv.autoscroll = true
		}
		return
	}
	lv.scrollOffset++
	total := lv.totalDisplayLines()
	h := lv.viewportHeight()
	if lv.scrollOffset >= total-h {
		lv.autoscroll = true
	}
	lv.updateViewport()
}

// ScrollLeft scrolls the viewport left by hScrollStep columns.
func (lv *LogView) ScrollLeft() {
	if !lv.softWrap {
		lv.logVP.xOffset = max(0, lv.logVP.xOffset-hScrollStep)
	}
}

// ScrollRight scrolls the viewport right by hScrollStep columns.
func (lv *LogView) ScrollRight() {
	if !lv.softWrap {
		lv.logVP.xOffset += hScrollStep
	}
}

// PageUp scrolls the viewport up by one page.
func (lv *LogView) PageUp() {
	if lv.softWrap {
		lv.wrapYOffset -= lv.viewportHeight()
		if lv.wrapYOffset < 0 {
			lv.wrapYOffset = 0
		}
		lv.autoscroll = false
		lv.updateViewportWrapped()
		return
	}
	lv.scrollOffset -= lv.viewportHeight()
	lv.autoscroll = false
	lv.updateViewport()
}

// PageDown scrolls the viewport down by one page.
func (lv *LogView) PageDown() {
	if lv.softWrap {
		lv.wrapYOffset += lv.viewportHeight()
		lv.updateViewportWrapped() // this will clamp
		maxOff := lv.totalWrappedRows - lv.viewportHeight()
		if maxOff < 0 {
			maxOff = 0
		}
		if lv.wrapYOffset >= maxOff {
			lv.autoscroll = true
		}
		return
	}
	lv.scrollOffset += lv.viewportHeight()
	total := lv.totalDisplayLines()
	h := lv.viewportHeight()
	if lv.scrollOffset >= total-h {
		lv.autoscroll = true
	}
	lv.updateViewport()
}

// GotoTop scrolls to the top of the content.
func (lv *LogView) GotoTop() {
	if lv.softWrap {
		lv.wrapYOffset = 0
		lv.autoscroll = false
		lv.updateViewportWrapped()
		return
	}
	lv.scrollOffset = 0
	lv.autoscroll = false
	lv.updateViewport()
}

// GotoBottom scrolls to the bottom of the content.
func (lv *LogView) GotoBottom() {
	if lv.softWrap {
		maxOff := lv.totalWrappedRows - lv.viewportHeight()
		if maxOff < 0 {
			maxOff = 0
		}
		lv.wrapYOffset = maxOff
		lv.autoscroll = true
		lv.updateViewportWrapped()
		return
	}
	lv.scrollOffset = lv.totalDisplayLines() - lv.viewportHeight()
	lv.autoscroll = true
	lv.updateViewport()
}

// ToggleWrap toggles soft-wrap on the viewport. When enabling wrap,
// the horizontal offset is reset since horizontal scroll is a no-op.
func (lv *LogView) ToggleWrap() {
	lv.softWrap = !lv.softWrap
	if lv.softWrap {
		lv.logVP.xOffset = 0
		lv.wrapYOffset = 0 // reset wrap scroll
	} else if lv.autoscroll {
		lv.scrollOffset = lv.totalDisplayLines() - lv.viewportHeight()
	}
	lv.updateViewport()
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

// SetTimeRangeLabel sets the displayed time range label in the title.
func (lv *LogView) SetTimeRangeLabel(label string) {
	lv.timeRangeLabel = label
}

// DefaultSinceSeconds returns the configured default sinceSeconds value for log streams.
func (lv *LogView) DefaultSinceSeconds() int64 {
	return lv.defaultSinceSeconds
}

// ClearAndRestart resets the ring buffer, clears viewport content,
// and re-enables autoscroll for a fresh log stream.
func (lv *LogView) ClearAndRestart() {
	lv.buffer.Reset()
	lv.logVP.SetLines(nil, nil)
	lv.matchPositions = nil
	lv.matchIndex = -1
	lv.scrollOffset = 0
	lv.wrapYOffset = 0
	lv.totalWrappedRows = 0
	lv.filteredIndices = nil
	lv.autoscroll = true
	lv.timeRangeLabel = lv.defaultTimeRange
	lv.filterState.Clear()
	lv.searchState.Clear()
}
