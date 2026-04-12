package ui

import (
	"fmt"
	"regexp"
	"sort"
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
	scrollOffset        int   // top-of-viewport line index in display list
	filteredIndices     []int // when filter active, absolute buffer indices of matching lines
	filteredIndexOffset int   // number of evictions; subtract from filteredIndices entries to get logical buffer index

	// Header toggle (borderless title bar)
	showHeader bool

	// Custom viewport for non-wrap rendering
	logVP                  logViewport
	highlightStyle         lipgloss.Style
	selectedHighlightStyle lipgloss.Style
	highlightSGR           string
	selectedHighlightSGR   string
	softWrap               bool

	// Wrap-mode scroll tracking
	wrapYOffset      int // visual row offset for wrap mode
	totalWrappedRows int // total visual rows (sum of all wrapped heights)
}

// NewLogView creates a new log view with the given dimensions and ring buffer capacity.
func NewLogView(width, height, bufCapacity int, defaultTimeRange string, defaultSinceSeconds int64) LogView {
	var logVP logViewport
	logVP.SetSize(width-2, height-2)

	hiStyle := lipgloss.NewStyle().Background(theme.SearchMatch).Foreground(theme.SearchFg)
	selStyle := lipgloss.NewStyle().Background(theme.SearchSelected).Foreground(theme.SearchFg).Bold(true)

	return LogView{
		buffer:                 NewDualRingBuffer(bufCapacity),
		width:                  width,
		height:                 height,
		autoscroll:             true,
		showHeader:             true,
		pipeline:               highlight.DefaultPipeline(),
		timeRangeLabel:         defaultTimeRange,
		defaultTimeRange:       defaultTimeRange,
		defaultSinceSeconds:    defaultSinceSeconds,
		logVP:                  logVP,
		highlightStyle:         hiStyle,
		selectedHighlightStyle: selStyle,
		highlightSGR:           styleToSGR(hiStyle),
		selectedHighlightSGR:   styleToSGR(selStyle),
	}
}

// viewportHeight returns the number of visible lines in the viewport.
func (lv *LogView) viewportHeight() int {
	if lv.borderless {
		if lv.showHeader {
			return lv.height - 1
		}
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
	maxOffset := max(total-h, 0)
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
	start := max(lv.scrollOffset-indicatorOffset, 0)
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
			bufIdx := idx - lv.filteredIndexOffset
			window = append(window, lv.buffer.ColoredGet(bufIdx))
			widths = append(widths, lv.buffer.WidthGet(bufIdx))
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
		startIdx := sort.Search(len(lv.matchPositions), func(i int) bool {
			return lv.matchPositions[i].line >= windowStart
		})
		var windowPositions []matchPosition
		for i := startIdx; i < len(lv.matchPositions); i++ {
			pos := lv.matchPositions[i]
			if pos.line >= windowEnd {
				break
			}
			shifted := pos
			shifted.line -= windowStart
			windowPositions = append(windowPositions, shifted)
		}
		if len(windowPositions) > 0 {
			// Determine which windowPosition is the globally selected match.
			globalSelectedWP := -1
			if lv.matchIndex >= 0 {
				sel := lv.matchPositions[lv.matchIndex]
				if sel.line >= windowStart && sel.line < windowEnd {
					for i, wp := range windowPositions {
						if wp.line == sel.line-windowStart && wp.colStart == sel.colStart {
							globalSelectedWP = i
							break
						}
					}
				}
			}

			// Group windowPositions by line and call injectHighlights per line.
			wpIdx := 0
			for wpIdx < len(windowPositions) {
				lineIdx := windowPositions[wpIdx].line
				lineStart := wpIdx
				var ranges []highlightRange
				for wpIdx < len(windowPositions) && windowPositions[wpIdx].line == lineIdx {
					ranges = append(ranges, highlightRange{
						start: windowPositions[wpIdx].colStart,
						end:   windowPositions[wpIdx].colEnd,
					})
					wpIdx++
				}
				// Compute selectedInLine: which range within this line is selected.
				selectedInLine := -1
				if globalSelectedWP >= lineStart && globalSelectedWP < wpIdx {
					selectedInLine = globalSelectedWP - lineStart
				}
				window[lineIdx] = injectHighlights(
					window[lineIdx], ranges, selectedInLine,
					lv.highlightSGR, lv.selectedHighlightSGR,
				)
			}
		}
	}

	lv.logVP.SetLines(window, widths)
}

// wrapHeight returns the number of visual rows a line of the given display
// width occupies when soft-wrapped to vpWidth columns.
func wrapHeight(displayWidth, vpWidth int) int {
	if vpWidth <= 0 {
		vpWidth = 1
	}
	if displayWidth <= vpWidth {
		return 1
	}
	return (displayWidth + vpWidth - 1) / vpWidth // ceiling division
}

// recomputeTotalWrappedRows recalculates totalWrappedRows from scratch.
// Called after resize, toggle wrap, filter change, or any bulk mutation —
// NOT on every AppendLine (that path uses incremental updates).
func (lv *LogView) recomputeTotalWrappedRows() {
	vpWidth := lv.logVP.width
	total := 0
	if lv.filterState.Active() {
		for _, idx := range lv.filteredIndices {
			total += wrapHeight(lv.buffer.WidthGet(idx-lv.filteredIndexOffset), vpWidth)
		}
	} else {
		for i := range lv.buffer.Len() {
			total += wrapHeight(lv.buffer.WidthGet(i), vpWidth)
		}
	}
	if lv.buffer.Dropped() > 0 {
		total++ // indicator row
	}
	lv.totalWrappedRows = total
}

// updateViewportWrapped computes the visible window of wrapped lines and feeds
// them to logVP. Instead of copying the entire buffer, it uses the
// incrementally maintained totalWrappedRows for scroll math and only fetches
// the lines visible in the current viewport.
func (lv *LogView) updateViewportWrapped() {
	vpWidth := lv.logVP.width
	vpHeight := lv.viewportHeight()
	if vpWidth <= 0 {
		vpWidth = 1
	}

	hasIndicator := lv.buffer.Dropped() > 0

	// Determine the logical line count (filtered or full buffer).
	var lineCount int
	if lv.filterState.Active() {
		lineCount = len(lv.filteredIndices)
	} else {
		lineCount = lv.buffer.Len()
	}

	// Clamp wrapYOffset
	maxOffset := max(lv.totalWrappedRows-vpHeight, 0)
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

	// --- Find the first visible logical line ---
	// We need to walk wrapped heights to find which logical line corresponds
	// to wrapYOffset.  The "display list" has an optional indicator at index 0
	// followed by lineCount buffer lines.  We track a running visual-row counter.
	row := 0
	firstDisplayIdx := 0 // index into the display list (0 may be indicator)
	vOffset := 0         // visual rows to skip inside the first visible line
	displayCount := lineCount
	if hasIndicator {
		displayCount++
	}

	for di := 0; di < displayCount; di++ {
		var h int
		if hasIndicator && di == 0 {
			h = 1 // indicator is always 1 row
		} else {
			bufIdx := di
			if hasIndicator {
				bufIdx = di - 1
			}
			var w int
			if lv.filterState.Active() {
				w = lv.buffer.WidthGet(lv.filteredIndices[bufIdx] - lv.filteredIndexOffset)
			} else {
				w = lv.buffer.WidthGet(bufIdx)
			}
			h = wrapHeight(w, vpWidth)
		}
		if row+h > lv.wrapYOffset {
			firstDisplayIdx = di
			vOffset = lv.wrapYOffset - row
			break
		}
		row += h
		if di == displayCount-1 {
			firstDisplayIdx = di
			vOffset = 0
		}
	}

	// --- Build visual rows for the visible window ---
	// We only fetch lines from firstDisplayIdx onward, enough to fill the
	// viewport. The first long line's vOffset skip is handled inside the
	// splitter (startRow parameter), so no post-hoc trimming is needed.
	var visRows []string
	var visWidths []int

	for di := firstDisplayIdx; di < displayCount && len(visRows) < vpHeight; di++ {
		var line string
		var w int
		if hasIndicator && di == 0 {
			line = fmt.Sprintf("~%d lines dropped", lv.buffer.Dropped())
			w = len(line) // plain ASCII
		} else {
			bufIdx := di
			if hasIndicator {
				bufIdx = di - 1
			}
			if lv.filterState.Active() {
				idx := lv.filteredIndices[bufIdx] - lv.filteredIndexOffset
				line = lv.buffer.ColoredGet(idx)
				w = lv.buffer.WidthGet(idx)
			} else {
				line = lv.buffer.ColoredGet(bufIdx)
				w = lv.buffer.WidthGet(bufIdx)
			}
		}

		// Bake search highlights for this single line if search is active.
		if lv.searchState.Active() && len(lv.matchPositions) > 0 {
			startIdx := sort.Search(len(lv.matchPositions), func(i int) bool {
				return lv.matchPositions[i].line >= di
			})
			var linePositions []matchPosition
			for i := startIdx; i < len(lv.matchPositions); i++ {
				pos := lv.matchPositions[i]
				if pos.line > di {
					break
				}
				shifted := pos
				shifted.line = 0
				linePositions = append(linePositions, shifted)
			}
			if len(linePositions) > 0 {
				selectedInLine := -1
				if lv.matchIndex >= 0 && lv.matchIndex < len(lv.matchPositions) {
					sel := lv.matchPositions[lv.matchIndex]
					if sel.line == di {
						for i, lp := range linePositions {
							if lp.colStart == sel.colStart && lp.colEnd == sel.colEnd {
								selectedInLine = i
								break
							}
						}
					}
				}
				ranges := make([]highlightRange, len(linePositions))
				for i, pos := range linePositions {
					ranges[i] = highlightRange{start: pos.colStart, end: pos.colEnd}
				}
				line = injectHighlights(
					line, ranges, selectedInLine,
					lv.highlightSGR, lv.selectedHighlightSGR,
				)
			}
		}

		if w <= vpWidth {
			visRows = append(visRows, line)
			visWidths = append(visWidths, w)
		} else {
			// For the first visible long line, skip vOffset rows inside the
			// splitter instead of generating them and trimming post-hoc.
			startRow := 0
			if di == firstDisplayIdx {
				startRow = vOffset
			}
			segs, segWidths := splitWrappedVisible(line, vpWidth, startRow, vpHeight-len(visRows))
			visRows = append(visRows, segs...)
			visWidths = append(visWidths, segWidths...)
		}
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
	coloredWidth := ansi.StringWidth(colored)

	// Before the buffer mutates, capture the evicted line's width for
	// incremental totalWrappedRows bookkeeping (wrap mode only).
	var evictedWidth int
	willEvict := lv.buffer.Len() == lv.buffer.Cap()
	if willEvict && lv.softWrap {
		evictedWidth = lv.buffer.WidthGet(0) // logical index 0 = oldest
	}

	lv.buffer.Append(line, colored, ansi.Strip(colored), coloredWidth)
	evicted := lv.buffer.Dropped() > droppedBefore

	// Incremental totalWrappedRows update (wrap mode, no filter).
	if lv.softWrap && !lv.filterState.Active() {
		vpWidth := lv.logVP.width
		// Add the new line's wrapped height.
		lv.totalWrappedRows += wrapHeight(coloredWidth, vpWidth)
		if evicted {
			// Subtract the evicted line's wrapped height.
			lv.totalWrappedRows -= wrapHeight(evictedWidth, vpWidth)
		}
		// Account for the indicator row appearing for the first time.
		if evicted && droppedBefore == 0 {
			lv.totalWrappedRows++ // indicator row now present
		}
	}

	if lv.filterState.Active() {
		// On eviction, increment offset instead of decrementing all indices (O(1) vs O(K))
		if evicted {
			lv.filteredIndexOffset++
			// Trim entries that refer to the evicted line
			for len(lv.filteredIndices) > 0 && lv.filteredIndices[0] < lv.filteredIndexOffset {
				lv.filteredIndices = lv.filteredIndices[1:]
			}
		}
		// Then add the new line if it matches (store absolute index = offset + logical)
		matched := lv.filterState.Re.MatchString(line)
		if matched {
			lv.filteredIndices = append(lv.filteredIndices, lv.filteredIndexOffset+lv.buffer.Len()-1)
		}
		if !matched && !evicted {
			return // no viewport change needed
		}
		// Recompute totalWrappedRows for the filtered set.
		if lv.softWrap {
			lv.recomputeTotalWrappedRows()
		}
	}

	// Incremental search match update.
	if lv.searchState.Active() {
		hasIndicator := lv.buffer.Dropped() > 0
		if evicted {
			// Display indices shifted — full per-line rebuild.
			lv.rebuildMatchPositions()
		} else {
			// Just append the new line's matches.
			stripped := lv.buffer.StrippedGet(lv.buffer.Len() - 1)
			var displayIdx int
			if lv.filterState.Active() {
				// When filter is active, display index is position in filteredIndices.
				displayIdx = len(lv.filteredIndices) - 1
			} else {
				displayIdx = lv.buffer.Len() - 1
			}
			if hasIndicator {
				displayIdx++ // indicator occupies index 0
			}
			newPositions := computeLineMatchPositions(stripped, lv.searchState.Re, displayIdx)
			lv.matchPositions = append(lv.matchPositions, newPositions...)
			lv.searchState.MatchCount = len(lv.matchPositions)
		}
	}

	if lv.autoscroll && !lv.softWrap {
		lv.scrollOffset = lv.totalDisplayLines() - lv.viewportHeight()
	}
	lv.updateViewport()
}

// rebuildMatchPositions recomputes all search match positions from scratch.
// It clears matchPositions, iterates all lines (or filtered indices), applies
// the indicator offset, and updates searchState.MatchCount.
// It does NOT reset matchIndex — callers decide whether to reset navigation.
func (lv *LogView) rebuildMatchPositions() {
	lv.matchPositions = nil

	if !lv.searchState.Active() {
		lv.searchState.MatchCount = 0
		return
	}

	hasIndicator := lv.buffer.Dropped() > 0
	var positions []matchPosition

	if lv.filterState.Active() {
		for lineIdx, idx := range lv.filteredIndices {
			bufIdx := idx - lv.filteredIndexOffset
			if bufIdx >= 0 && bufIdx < lv.buffer.Len() {
				positions = append(positions, computeLineMatchPositions(
					lv.buffer.StrippedGet(bufIdx), lv.searchState.Re, lineIdx)...)
			}
		}
	} else {
		for i := range lv.buffer.Len() {
			positions = append(positions, computeLineMatchPositions(
				lv.buffer.StrippedGet(i), lv.searchState.Re, i)...)
		}
	}

	if hasIndicator {
		for i := range positions {
			positions[i].line++
		}
	}

	lv.matchPositions = positions
	lv.searchState.MatchCount = len(positions)
}

// rebuildViewportContent recomputes search match positions and updates the viewport.
// Called on search/filter activation (user action), not on every append.
func (lv *LogView) rebuildViewportContent() {
	lv.matchIndex = -1
	lv.rebuildMatchPositions()

	if len(lv.matchPositions) > 0 {
		lv.matchIndex = 0
	}

	if lv.softWrap {
		lv.recomputeTotalWrappedRows()
	}
	lv.updateViewport()
}

// computeLineMatchPositions runs FindAllStringIndex on a single stripped line
// and converts byte offsets to grapheme-width columns using ansi.StringWidth.
func computeLineMatchPositions(stripped string, re *regexp.Regexp, lineIdx int) []matchPosition {
	byteMatches := re.FindAllStringIndex(stripped, -1)
	if len(byteMatches) == 0 {
		return nil
	}
	positions := make([]matchPosition, 0, len(byteMatches))
	for _, match := range byteMatches {
		colStart := ansi.StringWidth(stripped[:match[0]])
		colEnd := ansi.StringWidth(stripped[:match[1]])
		positions = append(positions, matchPosition{
			line:     lineIdx,
			colStart: colStart,
			colEnd:   colEnd,
		})
	}
	return positions
}

// View renders the log view with border and title.
func (lv LogView) View() string {
	if lv.borderless && lv.showHeader {
		baseTitle := lv.buildTitle()
		titleRendered := BuildPanelTitle(baseTitle, lv.filterState.DisplayPattern(),
			lv.searchState.DisplayPattern(), lv.width, lv.inlineSearch)
		headerLine := DetailHeaderStyle.Width(lv.width).Render(titleRendered)
		return lipgloss.JoinVertical(lipgloss.Left, headerLine, lv.logVP.View())
	}
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
		// Rebuild filtered indices with current offset
		lv.filteredIndices = nil
		if lv.filterState.Active() {
			for i := range lv.buffer.Len() {
				if lv.filterState.Re.MatchString(lv.buffer.RawGet(i)) {
					lv.filteredIndices = append(lv.filteredIndices, lv.filteredIndexOffset+i)
				}
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
	lv.filteredIndexOffset = 0
	if lv.softWrap {
		lv.recomputeTotalWrappedRows()
	}
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
				w = lv.buffer.WidthGet(lv.filteredIndices[i] - lv.filteredIndexOffset)
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
		vpH := h
		if lv.showHeader {
			vpH = h - 1
		}
		lv.logVP.SetSize(w, vpH)
	} else {
		lv.logVP.SetSize(w-2, h-2)
	}
	if lv.softWrap {
		lv.recomputeTotalWrappedRows()
	}
	lv.updateViewport()
}

// SetBorderless enables or disables borderless rendering.
func (lv *LogView) SetBorderless(b bool) {
	lv.borderless = b
}

// ToggleHeader flips the header visibility and recalculates the viewport size.
func (lv *LogView) ToggleHeader() {
	lv.showHeader = !lv.showHeader
	lv.SetSize(lv.width, lv.height)
}

// ShowHeader reports whether the header bar is visible.
func (lv LogView) ShowHeader() bool { return lv.showHeader }

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
		var colored string
		if lv.pipeline != nil {
			colored = lv.pipeline.Highlight(lv.buffer.RawGet(i))
		} else {
			colored = lv.buffer.RawGet(i)
		}
		lv.buffer.SetColored(i, colored, ansi.StringWidth(colored))
		lv.buffer.SetStripped(i, ansi.Strip(colored))
	}
	if lv.softWrap {
		lv.recomputeTotalWrappedRows()
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
			maxOff := max(lv.totalWrappedRows-lv.viewportHeight(), 0)
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
	rawWidth := ansi.StringWidth(raw)
	lv.buffer.Append(raw, styled, raw, rawWidth)
	if lv.softWrap && !lv.filterState.Active() {
		lv.totalWrappedRows += wrapHeight(rawWidth, lv.logVP.width)
	}
	if lv.filterState.Active() && lv.filterState.Re.MatchString(raw) {
		lv.filteredIndices = append(lv.filteredIndices, lv.filteredIndexOffset+lv.buffer.Len()-1)
		if lv.softWrap {
			lv.recomputeTotalWrappedRows()
		}
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
		maxOff := max(lv.totalWrappedRows-lv.viewportHeight(), 0)
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
		maxOff := max(lv.totalWrappedRows-lv.viewportHeight(), 0)
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
		maxOff := max(lv.totalWrappedRows-lv.viewportHeight(), 0)
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
		lv.recomputeTotalWrappedRows()
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
	lv.filteredIndexOffset = 0
	lv.autoscroll = true
	lv.timeRangeLabel = lv.defaultTimeRange
	lv.filterState.Clear()
	lv.searchState.Clear()
}
