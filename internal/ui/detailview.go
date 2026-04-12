package ui

import (
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
	d.viewport.SetContent(content.Display)
	if refresh {
		d.viewport.GotoTop()
		d.viewport.SetXOffset(0)
	}
	d.reapplySearch()
}

// ClearContent clears all content.
func (d *DetailView) ClearContent() {
	d.rawContent = ""
	d.displayContent = ""
	d.viewport.SetContent("")
}

// ScrollLeft scrolls the viewport left by hScrollStep columns.
// No-op when soft-wrap is enabled.
func (d *DetailView) ScrollLeft() {
	if !d.viewport.SoftWrap {
		d.viewport.ScrollLeft(hScrollStep)
	}
}

// ScrollRight scrolls the viewport right by hScrollStep columns.
// No-op when soft-wrap is enabled.
func (d *DetailView) ScrollRight() {
	if !d.viewport.SoftWrap {
		d.viewport.ScrollRight(hScrollStep)
	}
}

// ScrollHome resets horizontal scroll to the beginning of the line.
// No-op when soft-wrap is enabled.
func (d *DetailView) ScrollHome() {
	if !d.viewport.SoftWrap {
		d.viewport.SetXOffset(0)
	}
}

// ScrollEnd scrolls horizontally to show the end of the longest visible line.
// No-op when soft-wrap is enabled.
func (d *DetailView) ScrollEnd() {
	if !d.viewport.SoftWrap {
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

// ToggleWrap toggles soft-wrap on the viewport. When enabling wrap,
// the horizontal offset is reset since horizontal scroll is a no-op.
func (d *DetailView) ToggleWrap() {
	d.viewport.SoftWrap = !d.viewport.SoftWrap
	if d.viewport.SoftWrap {
		d.viewport.SetXOffset(0)
	}
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
	default:
		return ""
	}
}

// buildTitle constructs the title string for the detail view header.
func (d DetailView) buildTitle() string {
	title := d.modeName()
	if d.loading {
		title += " " + d.spinner.View()
	}
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

// SearchNext navigates to the next search match.
func (d *DetailView) SearchNext() {
	if !d.searchState.Active() {
		return
	}
	if len(d.matchPositions) > 0 {
		d.matchIndex = (d.matchIndex + 1) % len(d.matchPositions)
		d.rebuildHighlightedDisplay()
		pos := d.matchPositions[d.matchIndex]
		d.viewport.EnsureVisible(pos.line, pos.colStart, pos.colEnd)
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
		d.viewport.EnsureVisible(pos.line, pos.colStart, pos.colEnd)
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
		d.viewport.SetContent(displayForViewport)
		return
	}

	rawMatches := d.searchState.Re.FindAllStringIndex(rawForSearch, -1)

	if rawForSearch == displayForViewport {
		// Plain text (Describe/Logs): viewport handles highlighting correctly.
		d.viewport.SetContent(displayForViewport)
		d.viewport.SetHighlights(rawMatches)
		return
	}

	// ANSI-colored content (YAML): the viewport's parseMatches cannot handle
	// ANSI content correctly (it tracks byte positions in stripped text but
	// checks newlines in the original ANSI content). Bake highlights into the
	// display text ourselves using lipgloss.StyleRanges.
	positions := computeMatchPositions(rawForSearch, rawMatches)
	d.matchPositions = positions
	if len(positions) > 0 {
		d.matchIndex = 0
	}
	highlighted := buildHighlightedDisplay(
		displayForViewport, positions, d.matchIndex,
		d.viewport.HighlightStyle, d.viewport.SelectedHighlightStyle,
	)
	d.viewport.SetContent(highlighted)
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
	d.viewport.SetContent(highlighted)
	d.viewport.SetYOffset(y)
	d.viewport.SetXOffset(x)
}

// computeMatchPositions converts raw byte offsets to per-line grapheme-width positions.
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
		lineByteStart := start - lineStart
		lineByteEnd := min(end-lineStart, len(rawLines[lineIdx]))
		line := rawLines[lineIdx]
		colStart := ansi.StringWidth(line[:lineByteStart])
		colEnd := ansi.StringWidth(line[:lineByteEnd])
		positions = append(positions, matchPosition{
			line:     lineIdx,
			colStart: colStart,
			colEnd:   colEnd,
		})
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
