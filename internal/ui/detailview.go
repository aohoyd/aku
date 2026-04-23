package ui

import (
	"sort"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/aohoyd/aku/internal/msgs"
	"github.com/aohoyd/aku/internal/render"
	"github.com/aohoyd/aku/internal/theme"
	"github.com/charmbracelet/x/ansi"
)

const hScrollStep = 8

// matchPosition tracks a search match's location in terms of line and grapheme column.
type matchPosition struct {
	line     int
	colStart int
	colEnd   int
}

// DetailView wraps a viewport for displaying YAML, describe, or log content.
type DetailView struct {
	viewport       viewport.Model
	spinner        spinner.Model
	mode           msgs.DetailMode
	loading        bool
	loadErr        string
	focused        bool
	borderless     bool
	showHeader     bool
	width          int
	height         int
	filterState    SearchState
	searchState    SearchState
	rawContent     string
	displayContent string
	matchPositions []matchPosition
	matchIndex     int
	inlineSearch   string
	envResolved    bool
	softWrap       bool
	wrapMapping    []lineMap
}

// SetInlineSearch sets the inline search input text for rendering in the title.
func (d *DetailView) SetInlineSearch(s string) { d.inlineSearch = s }

// SetEnvResolved sets whether secrets are currently revealed, shown as [S] in the header.
func (d *DetailView) SetEnvResolved(v bool) { d.envResolved = v }

// SetLoading enables or disables the loading state.
// When true, resets any load error but preserves existing viewport content
// so that scroll position is maintained across async reloads.
// Returns a tea.Cmd to start the spinner animation (nil when disabling).
func (d *DetailView) SetLoading(v bool) tea.Cmd {
	if v {
		d.loading = true
		d.loadErr = ""
		return d.spinner.Tick
	}
	d.loading = false
	return nil
}

// SetLoadError sets a load error message and clears the loading state.
func (d *DetailView) SetLoadError(msg string) {
	d.loading = false
	d.loadErr = msg
	d.viewport.SetContent(msg)
}

// Loading reports whether the detail view is in a loading state.
func (d DetailView) Loading() bool { return d.loading }

// LoadErr returns the current load error message, if any.
func (d DetailView) LoadErr() string { return d.loadErr }

// NewDetailView creates a new detail view with the given dimensions.
func NewDetailView(width, height int) DetailView {
	vp := viewport.New(viewport.WithWidth(width-2), viewport.WithHeight(height-2)) // -2 border
	vp.KeyMap.Left = key.NewBinding()
	vp.KeyMap.Right = key.NewBinding()
	vp.HighlightStyle = lipgloss.NewStyle().Background(theme.SearchMatch).Foreground(theme.SearchFg)
	vp.SelectedHighlightStyle = lipgloss.NewStyle().Background(theme.SearchSelected).Foreground(theme.SearchFg).Bold(true)
	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	return DetailView{
		viewport:   vp,
		spinner:    sp,
		showHeader: true,
		width:      width,
		height:     height,
	}
}

// SetMode sets the active detail mode.
func (d *DetailView) SetMode(mode msgs.DetailMode) {
	d.mode = mode
	d.rawContent = ""
	d.displayContent = ""
	d.filterState.Clear()
	d.searchState.Clear()
	d.matchPositions = nil
	d.matchIndex = -1
	d.viewport.ClearHighlights()
	d.viewport.SetContent("")
	d.viewport.GotoTop()
	d.viewport.SetXOffset(0)
}

// SetContent sets the detail view content from pre-rendered render.Content.
// When refresh is true, scroll resets to top; when false, scroll position is preserved.
// Setting content also clears the loading and error states.
func (d *DetailView) SetContent(content render.Content, refresh bool) {
	d.loading = false
	d.loadErr = ""
	if !refresh {
		y := d.viewport.YOffset()
		x := d.viewport.XOffset()
		defer func() {
			d.viewport.SetYOffset(y)
			d.viewport.SetXOffset(x)
		}()
	}
	d.rawContent = content.Raw
	d.displayContent = content.Display
	if refresh {
		d.viewport.GotoTop()
		d.viewport.SetXOffset(0)
	}
	d.reapplySearch()
}

// setViewportContent wraps content through wrapLines when softWrap is active,
// then sets the result on the viewport. When softWrap is off, content is set
// directly and the wrap mapping is cleared.
func (d *DetailView) setViewportContent(content string) {
	if d.softWrap {
		lines := strings.Split(content, "\n")
		wrapped, mapping := wrapLines(lines, d.viewport.Width())
		d.wrapMapping = mapping
		d.viewport.SetContent(strings.Join(wrapped, "\n"))
	} else {
		d.wrapMapping = nil
		d.viewport.SetContent(content)
	}
}

// ClearContent clears all content.
func (d *DetailView) ClearContent() {
	d.rawContent = ""
	d.displayContent = ""
	d.wrapMapping = nil
	d.viewport.SetContent("")
}

// ScrollLeft scrolls the viewport left by hScrollStep columns.
// No-op when soft-wrap is enabled.
func (d *DetailView) ScrollLeft() {
	if !d.softWrap {
		d.viewport.ScrollLeft(hScrollStep)
	}
}

// ScrollRight scrolls the viewport right by hScrollStep columns.
// No-op when soft-wrap is enabled.
func (d *DetailView) ScrollRight() {
	if !d.softWrap {
		d.viewport.ScrollRight(hScrollStep)
	}
}

// ScrollHome resets horizontal scroll to the beginning of the line.
// No-op when soft-wrap is enabled.
func (d *DetailView) ScrollHome() {
	if !d.softWrap {
		d.viewport.SetXOffset(0)
	}
}

// ScrollEnd scrolls horizontally to show the end of the longest visible line.
// No-op when soft-wrap is enabled.
func (d *DetailView) ScrollEnd() {
	if !d.softWrap {
		content := d.viewport.GetContent()
		lines := strings.Split(content, "\n")
		yOff := d.viewport.YOffset()
		visCount := d.viewport.VisibleLineCount()
		end := yOff + visCount
		if end > len(lines) {
			end = len(lines)
		}
		maxW := 0
		for i := yOff; i < end; i++ {
			w := ansi.StringWidth(lines[i])
			if w > maxW {
				maxW = w
			}
		}
		off := maxW - d.viewport.Width()
		if off < 0 {
			off = 0
		}
		d.viewport.SetXOffset(off)
	}
}

// PageDown scrolls the viewport down by one page.
func (d *DetailView) PageDown() { d.viewport.PageDown() }

// PageUp scrolls the viewport up by one page.
func (d *DetailView) PageUp() { d.viewport.PageUp() }

// GotoTop scrolls to the top of the content.
func (d *DetailView) GotoTop() { d.viewport.GotoTop() }

// GotoBottom scrolls to the bottom of the content.
func (d *DetailView) GotoBottom() { d.viewport.GotoBottom() }

// ScrollUp scrolls the viewport up by one line.
func (d *DetailView) ScrollUp() { d.viewport.ScrollUp(1) }

// ScrollDown scrolls the viewport down by one line.
func (d *DetailView) ScrollDown() { d.viewport.ScrollDown(1) }

// ScrollWheel nudges the viewport by one line in response to a mouse wheel
// event. Up/down delegate to the embedded bubbles/viewport's scroll helpers.
// Left/right wheel and any other button are dropped.
func (d *DetailView) ScrollWheel(btn tea.MouseButton) {
	switch btn {
	case tea.MouseWheelUp:
		d.viewport.ScrollUp(1)
	case tea.MouseWheelDown:
		d.viewport.ScrollDown(1)
	}
}

// ToggleWrap toggles soft-wrap mode. When enabling wrap, content is pre-wrapped
// via wrapLines so that continuation rows receive the "↪ " indicator. The
// viewport's own SoftWrap is never used — we handle wrapping ourselves.
// When enabling wrap, the horizontal offset is reset since horizontal scroll
// is a no-op.
func (d *DetailView) ToggleWrap() {
	d.softWrap = !d.softWrap
	if d.softWrap {
		d.viewport.SetXOffset(0)
	}
	d.reapplySearch()
}

// ToggleHeader flips the header visibility and recalculates the viewport size.
func (d *DetailView) ToggleHeader() {
	d.showHeader = !d.showHeader
	d.SetSize(d.width, d.height)
}

// ShowHeader reports whether the header bar is visible.
func (d DetailView) ShowHeader() bool { return d.showHeader }

// Mode returns the current detail mode.
func (d *DetailView) Mode() msgs.DetailMode {
	return d.mode
}

// Focus marks this detail view as focused.
func (d *DetailView) Focus() { d.focused = true }

// Blur marks this detail view as unfocused.
func (d *DetailView) Blur() { d.focused = false }

// Width returns the current width.
func (d DetailView) Width() int { return d.width }

// Height returns the current height.
func (d DetailView) Height() int { return d.height }

// SetBorderless enables or disables borderless rendering.
func (d *DetailView) SetBorderless(b bool) {
	d.borderless = b
}

// SetSize updates the dimensions.
func (d *DetailView) SetSize(w, h int) {
	d.width = w
	d.height = h
	if d.borderless {
		vpH := h
		if d.showHeader {
			vpH = h - 1
		}
		d.viewport.SetWidth(w)
		d.viewport.SetHeight(vpH)
	} else {
		d.viewport.SetWidth(w - 2)
		d.viewport.SetHeight(h - 2)
	}
	if d.softWrap {
		d.reapplySearch()
	}
}

// Update handles key messages for viewport scrolling.
func (d DetailView) Update(msg tea.Msg) (DetailView, tea.Cmd) {
	if d.loading {
		if _, ok := msg.(spinner.TickMsg); ok {
			var cmd tea.Cmd
			d.spinner, cmd = d.spinner.Update(msg)
			return d, cmd
		}
	}
	var cmd tea.Cmd
	d.viewport, cmd = d.viewport.Update(msg)
	return d, cmd
}

// modeName returns the display label for the current detail mode.
func (d DetailView) modeName() string {
	switch d.mode {
	case msgs.DetailYAML:
		return "YAML"
	case msgs.DetailDescribe:
		return "Describe"
	case msgs.DetailLogs:
		return "Logs"
	case msgs.DetailValues:
		return "Values (user)"
	case msgs.DetailValuesAll:
		return "Values (all)"
	default:
		return ""
	}
}

// buildTitle constructs the title string for the detail view header.
func (d DetailView) buildTitle() string {
	title := d.modeName()
	if d.envResolved {
		title += " [S]"
	}
	return title
}

// View renders the detail view with mode label in the border.
func (d DetailView) View() string {
	if d.borderless && d.showHeader {
		baseTitle := d.buildTitle()
		titleRendered := BuildPanelTitle(baseTitle, d.filterState.DisplayPattern(),
			d.searchState.DisplayPattern(), d.width, d.inlineSearch)
		headerLine := DetailHeaderStyle.Width(d.width).Render(titleRendered)
		return lipgloss.JoinVertical(lipgloss.Left, headerLine, d.viewport.View())
	}
	if d.borderless {
		return d.viewport.View()
	}

	borderStyle := UnfocusedBorderStyle
	if d.focused {
		borderStyle = FocusedBorderStyle
	}

	content := d.viewport.View()
	styled := borderStyle.Width(d.width).Height(d.height).Render(content)

	baseTitle := d.buildTitle()
	titleRendered := BuildPanelTitle(baseTitle, d.filterState.DisplayPattern(), d.searchState.DisplayPattern(), d.width, d.inlineSearch)
	return injectBorderTitle(styled, titleRendered, d.focused)
}

// ApplySearch compiles the pattern and applies the given mode.
// In search mode, all content remains visible with matches highlighted.
// In filter mode, non-matching lines are hidden.
func (d *DetailView) ApplySearch(pattern string, mode msgs.SearchMode) error {
	if mode == msgs.SearchModeFilter {
		if err := d.filterState.Compile(pattern, mode); err != nil {
			return err
		}
	} else {
		if err := d.searchState.Compile(pattern, mode); err != nil {
			return err
		}
	}
	d.reapplySearch()
	return nil
}

// ClearSearch removes the active search highlights.
func (d *DetailView) ClearSearch() {
	d.searchState.Clear()
	d.reapplySearch()
}

// ClearFilter removes the active filter and restores all lines.
func (d *DetailView) ClearFilter() {
	d.filterState.Clear()
	d.reapplySearch()
}

// SearchActive reports whether a search is active (highlights only).
func (d DetailView) SearchActive() bool {
	return d.searchState.Active()
}

// FilterActive reports whether a filter is currently active.
func (d DetailView) FilterActive() bool {
	return d.filterState.Active()
}

// AnyActive reports whether either search or filter is active.
func (d DetailView) AnyActive() bool {
	return d.searchState.Active() || d.filterState.Active()
}

// logicalToVisual converts a logical match position to visual (wrapped)
// coordinates using d.wrapMapping. It finds the last visual row whose
// logicalLine matches and whose colOffset is <= the match start column,
// then adjusts the column range by subtracting that row's colOffset.
// On continuation rows, columns are shifted right by wrapIndicatorWidth
// to account for the prepended ↪ prefix.
func (d *DetailView) logicalToVisual(pos matchPosition) (line, colStart, colEnd int) {
	// wrapMapping is sorted by (logicalLine, colOffset) since wrapLines
	// emits segments in input order, each line's continuation rows following
	// its first row. Use a binary search to find the first row with
	// logicalLine > pos.line OR (logicalLine == pos.line && colOffset >
	// pos.colStart) — the best matching row is the one immediately before.
	n := len(d.wrapMapping)
	idx := sort.Search(n, func(i int) bool {
		m := d.wrapMapping[i]
		if m.logicalLine != pos.line {
			return m.logicalLine > pos.line
		}
		return m.colOffset > pos.colStart
	})
	bestRow := -1
	if idx > 0 && d.wrapMapping[idx-1].logicalLine == pos.line {
		bestRow = idx - 1
	}
	if bestRow < 0 {
		// Fallback: return logical coordinates unchanged.
		return pos.line, pos.colStart, pos.colEnd
	}
	// Invariant: the scan above picks the last row whose colOffset <=
	// pos.colStart, so pos.colStart - colOffset is guaranteed >= 0. And
	// pos.colEnd >= pos.colStart by construction of matchPosition (end is
	// strictly after start), so colEnd - colOffset is also >= 0. Matches do
	// not span row boundaries — wrapping is a visual concern and each
	// logical match lives inside a single wrap segment; so no clamp against
	// the row width is needed here.
	colStart = pos.colStart - d.wrapMapping[bestRow].colOffset
	colEnd = pos.colEnd - d.wrapMapping[bestRow].colOffset
	// Continuation rows have ↪ prepended — shift visual position right.
	if d.wrapMapping[bestRow].colOffset > 0 {
		colStart += wrapIndicatorWidth
		colEnd += wrapIndicatorWidth
	}
	return bestRow, colStart, colEnd
}

// SearchNext navigates to the next search match.
func (d *DetailView) SearchNext() {
	if !d.searchState.Active() {
		return
	}
	if len(d.matchPositions) > 0 {
		d.matchIndex = (d.matchIndex + 1) % len(d.matchPositions)
		d.rebuildHighlightedDisplay()
		pos := d.matchPositions[d.matchIndex]
		if d.softWrap && d.wrapMapping != nil {
			vLine, vColStart, vColEnd := d.logicalToVisual(pos)
			d.viewport.EnsureVisible(vLine, vColStart, vColEnd)
		} else {
			d.viewport.EnsureVisible(pos.line, pos.colStart, pos.colEnd)
		}
	} else {
		d.viewport.HighlightNext()
	}
}

// SearchPrev navigates to the previous search match.
func (d *DetailView) SearchPrev() {
	if !d.searchState.Active() {
		return
	}
	if len(d.matchPositions) > 0 {
		d.matchIndex--
		if d.matchIndex < 0 {
			d.matchIndex = len(d.matchPositions) - 1
		}
		d.rebuildHighlightedDisplay()
		pos := d.matchPositions[d.matchIndex]
		if d.softWrap && d.wrapMapping != nil {
			vLine, vColStart, vColEnd := d.logicalToVisual(pos)
			d.viewport.EnsureVisible(vLine, vColStart, vColEnd)
		} else {
			d.viewport.EnsureVisible(pos.line, pos.colStart, pos.colEnd)
		}
	} else {
		d.viewport.HighlightPrevious()
	}
}

// reapplySearch re-applies the current search and filter after content changes.
func (d *DetailView) reapplySearch() {
	var rawForSearch string
	var displayForViewport string
	if d.filterState.Active() {
		rawForSearch = d.filteredContent()
		displayForViewport = d.filterDisplayContent()
	} else {
		rawForSearch = d.rawContent
		displayForViewport = d.displayContent
	}

	d.matchPositions = nil
	d.matchIndex = -1

	if !d.searchState.Active() {
		d.setViewportContent(displayForViewport)
		return
	}

	rawMatches := d.searchState.Re.FindAllStringIndex(rawForSearch, -1)

	// When soft-wrap is off AND content is plain text (no ANSI), the
	// viewport's built-in highlight machinery works correctly.
	if !d.softWrap && rawForSearch == displayForViewport {
		d.setViewportContent(displayForViewport)
		d.viewport.SetHighlights(rawMatches)
		return
	}

	// Otherwise (soft-wrap active OR ANSI-colored content), bake highlights
	// into the display text ourselves. The viewport's parseMatches cannot
	// handle ANSI content correctly, and its byte ranges don't align with
	// wrapped line positions.
	positions := computeMatchPositions(rawForSearch, rawMatches)
	d.matchPositions = positions
	if len(positions) > 0 {
		d.matchIndex = 0
	}
	highlighted := buildHighlightedDisplay(
		displayForViewport, positions, d.matchIndex,
		d.viewport.HighlightStyle, d.viewport.SelectedHighlightStyle,
	)
	d.setViewportContent(highlighted)
}

// rebuildHighlightedDisplay re-applies highlights with the current matchIndex,
// preserving the viewport scroll position.
func (d *DetailView) rebuildHighlightedDisplay() {
	y := d.viewport.YOffset()
	x := d.viewport.XOffset()
	var displayBase string
	if d.filterState.Active() {
		displayBase = d.filterDisplayContent()
	} else {
		displayBase = d.displayContent
	}
	highlighted := buildHighlightedDisplay(
		displayBase, d.matchPositions, d.matchIndex,
		d.viewport.HighlightStyle, d.viewport.SelectedHighlightStyle,
	)
	d.setViewportContent(highlighted)
	d.viewport.SetYOffset(y)
	d.viewport.SetXOffset(x)
}

// computeMatchPositions converts raw byte offsets to per-line grapheme-width positions.
// A single regex match spanning multiple lines (e.g. via (?s) or literal \n in the
// pattern) is expanded into one matchPosition per affected line so downstream
// visual mapping scrolls to the correct location on SearchNext/Prev.
func computeMatchPositions(raw string, rawMatches [][]int) []matchPosition {
	if len(rawMatches) == 0 {
		return nil
	}
	rawLines := strings.Split(raw, "\n")
	positions := make([]matchPosition, 0, len(rawMatches))
	lineStart := 0
	lineIdx := 0
	for _, match := range rawMatches {
		start, end := match[0], match[1]
		for lineIdx < len(rawLines)-1 && lineStart+len(rawLines[lineIdx])+1 <= start {
			lineStart += len(rawLines[lineIdx]) + 1
			lineIdx++
		}
		// Walk the lines the match spans, emitting a position per line.
		// Preserve order; use a local cursor that does not perturb the outer
		// (lineIdx, lineStart) tracking.
		curIdx := lineIdx
		curLineStart := lineStart
		firstSegment := true
		for curIdx < len(rawLines) {
			line := rawLines[curIdx]
			lineEndByte := curLineStart + len(line)
			// Byte offsets for the portion of the match on this line.
			var segStart, segEnd int
			if firstSegment {
				segStart = start - curLineStart
			} else {
				segStart = 0
			}
			if end <= lineEndByte {
				segEnd = end - curLineStart
			} else {
				segEnd = len(line)
			}
			colStart := ansi.StringWidth(line[:segStart])
			colEnd := ansi.StringWidth(line[:segEnd])
			positions = append(positions, matchPosition{
				line:     curIdx,
				colStart: colStart,
				colEnd:   colEnd,
			})
			if end <= lineEndByte {
				break
			}
			// Advance to next line; +1 accounts for the consumed newline.
			curLineStart = lineEndByte + 1
			curIdx++
			firstSegment = false
		}
	}
	return positions
}

// buildHighlightedDisplay overlays search highlight styles onto ANSI-colored display lines.
func buildHighlightedDisplay(display string, positions []matchPosition, selectedIdx int, hiStyle, selStyle lipgloss.Style) string {
	if len(positions) == 0 {
		return display
	}
	displayLines := strings.Split(display, "\n")
	lineRanges := make(map[int][]lipgloss.Range)
	for i, pos := range positions {
		style := hiStyle
		if i == selectedIdx {
			style = selStyle
		}
		lineRanges[pos.line] = append(lineRanges[pos.line],
			lipgloss.NewRange(pos.colStart, pos.colEnd, style))
	}
	for lineIdx, ranges := range lineRanges {
		if lineIdx < len(displayLines) {
			displayLines[lineIdx] = lipgloss.StyleRanges(displayLines[lineIdx], ranges...)
		}
	}
	return strings.Join(displayLines, "\n")
}

// filteredContent returns only the lines from rawContent that match the filter regex.
func (d *DetailView) filteredContent() string {
	if d.filterState.Re == nil {
		return d.rawContent
	}
	var kept []string
	for line := range strings.SplitSeq(d.rawContent, "\n") {
		if d.filterState.Re.MatchString(line) {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}

// filterDisplayContent returns the display-rendered lines corresponding to
// the raw lines that match the filter regex.
func (d *DetailView) filterDisplayContent() string {
	if d.filterState.Re == nil {
		return d.displayContent
	}
	rawLines := strings.Split(d.rawContent, "\n")
	displayLines := strings.Split(d.displayContent, "\n")
	if len(rawLines) != len(displayLines) {
		return d.filteredContent()
	}
	var kept []string
	for i, line := range rawLines {
		if d.filterState.Re.MatchString(line) {
			kept = append(kept, displayLines[i])
		}
	}
	return strings.Join(kept, "\n")
}
