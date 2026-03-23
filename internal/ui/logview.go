package ui

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/aohoyd/aku/internal/msgs"
	"github.com/aohoyd/aku/internal/theme"
	"github.com/charmbracelet/x/ansi"
	"github.com/valyala/fastjson"
)

// compiledHighlight is a pre-compiled highlight rule ready for fast matching.
type compiledHighlight struct {
	re    *regexp.Regexp
	style lipgloss.Style
}

// Package-level compiled regexes for built-in log level highlighting.
var (
	builtinError = regexp.MustCompile(`(?i)\b(ERROR|ERR|FATAL)\b`)
	builtinWarn  = regexp.MustCompile(`(?i)\b(WARN|WARNING)\b`)
	builtinInfo  = regexp.MustCompile(`(?i)\bINFO\b`)
	builtinDebug = regexp.MustCompile(`(?i)\b(DEBUG|DBG)\b`)
	builtinTrace = regexp.MustCompile(`(?i)\bTRACE\b`)

	// Timestamp regex with submatch groups: (date)(time)(timezone?)
	builtinTimestamp = regexp.MustCompile(`(\d{4}-\d{2}-\d{2})[T ](\d{2}:\d{2}:\d{2}(?:\.\d+)?)(Z|[+-]\d{2}:?\d{2})?`)

	builtinIPv4 = regexp.MustCompile(`\b(\d{1,3})\.(\d{1,3})\.(\d{1,3})\.(\d{1,3})(:\d+)?\b`)
)

// Lipgloss styles for timestamp components.
var (
	timestampDateStyle = lipgloss.NewStyle().Foreground(theme.LogTimestamp)
	timestampTimeStyle = lipgloss.NewStyle().Foreground(theme.LogTime)
	timestampTZStyle   = lipgloss.NewStyle().Foreground(theme.LogTimezone)
	builtinIPStyle     = lipgloss.NewStyle().Foreground(theme.LogIP)
)

// Lipgloss styles for JSON syntax coloring (Kanagawa theme colors).
var (
	jsonKeyStyle    = lipgloss.NewStyle().Foreground(theme.SyntaxKey)
	jsonStringStyle = lipgloss.NewStyle().Foreground(theme.SyntaxString)
	jsonNumberStyle = lipgloss.NewStyle().Foreground(theme.SyntaxNumber)
	jsonBoolStyle   = lipgloss.NewStyle().Foreground(theme.SyntaxBool)
	jsonNullStyle   = lipgloss.NewStyle().Foreground(theme.SyntaxNull)
	jsonMarkerStyle = lipgloss.NewStyle().Foreground(theme.SyntaxMarker)
)


var fjParser fastjson.Parser

// builtinLogLevels contains pre-compiled highlight rules for standard log levels
// using Kanagawa theme colors.
var builtinLogLevels = []compiledHighlight{
	{re: builtinError, style: lipgloss.NewStyle().Foreground(theme.Error).Bold(true)},
	{re: builtinWarn, style: lipgloss.NewStyle().Foreground(theme.Warning)},
	{re: builtinInfo, style: lipgloss.NewStyle().Foreground(theme.SyntaxValue)},
	{re: builtinDebug, style: lipgloss.NewStyle().Foreground(theme.StatusRunning)},
	{re: builtinTrace, style: lipgloss.NewStyle().Foreground(theme.Muted).Faint(true)},
}

// jsonFragment records the byte offsets of a valid JSON fragment within a line.
type jsonFragment struct {
	start, end int // byte offsets in the line
}

// findJSONFragments scans a line for valid JSON objects ({...}) and arrays ([...]).
// It tracks nesting depth and handles quoted strings (including escaped quotes).
// Only candidates that pass fastjson.Validate are returned.
func findJSONFragments(line string) []jsonFragment {
	var fragments []jsonFragment
	i := 0
	for i < len(line) {
		ch := line[i]
		if ch != '{' && ch != '[' {
			i++
			continue
		}
		// Found a potential JSON start
		open := ch
		var close byte
		if open == '{' {
			close = '}'
		} else {
			close = ']'
		}
		depth := 1
		j := i + 1
		for j < len(line) && depth > 0 {
			c := line[j]
			if c == '"' {
				// Skip quoted string
				j++
				for j < len(line) {
					if line[j] == '\\' {
						j += 2 // skip escaped character
						continue
					}
					if line[j] == '"' {
						j++
						break
					}
					j++
				}
				continue
			}
			if c == open || (open == '{' && c == '[') || (open == '[' && c == '{') {
				depth++
			} else if c == close || (open == '{' && c == ']') || (open == '[' && c == '}') {
				depth--
			}
			j++
		}
		if depth == 0 {
			candidate := line[i:j]
			if err := fastjson.Validate(candidate); err == nil {
				fragments = append(fragments, jsonFragment{start: i, end: j})
				i = j
				continue
			}
		}
		i++
	}
	return fragments
}

// colorizeJSON tokenizes a raw JSON string and returns it with ANSI color codes
// and improved spacing (spaces after colons, commas, and inside braces).
func colorizeJSON(raw string) string {
	v, err := fjParser.Parse(raw)
	if err != nil {
		return raw
	}

	var sb strings.Builder
	writeJSONValue(&sb, v)
	return sb.String()
}

func writeJSONValue(sb *strings.Builder, v *fastjson.Value) {
	switch v.Type() {
	case fastjson.TypeObject:
		sb.WriteString(jsonMarkerStyle.Render("{"))
		sb.WriteString(" ")
		obj, _ := v.Object()
		i := 0
		obj.Visit(func(key []byte, val *fastjson.Value) {
			if i > 0 {
				sb.WriteString(jsonMarkerStyle.Render(","))
				sb.WriteString(" ")
			}
			sb.WriteString(jsonKeyStyle.Render(quoteString(string(key))))
			sb.WriteString(jsonMarkerStyle.Render(":"))
			sb.WriteString(" ")
			writeJSONValue(sb, val)
			i++
		})
		sb.WriteString(" ")
		sb.WriteString(jsonMarkerStyle.Render("}"))

	case fastjson.TypeArray:
		sb.WriteString(jsonMarkerStyle.Render("["))
		arr, _ := v.Array()
		for i, elem := range arr {
			if i > 0 {
				sb.WriteString(jsonMarkerStyle.Render(","))
				sb.WriteString(" ")
			}
			writeJSONValue(sb, elem)
		}
		sb.WriteString(jsonMarkerStyle.Render("]"))

	case fastjson.TypeString:
		s := v.GetStringBytes()
		sb.WriteString(jsonStringStyle.Render(quoteString(string(s))))

	case fastjson.TypeNumber:
		sb.WriteString(jsonNumberStyle.Render(string(v.MarshalTo(nil))))

	case fastjson.TypeTrue:
		sb.WriteString(jsonBoolStyle.Render("true"))
	case fastjson.TypeFalse:
		sb.WriteString(jsonBoolStyle.Render("false"))

	case fastjson.TypeNull:
		sb.WriteString(jsonNullStyle.Render("null"))
	}
}

// quoteString wraps s in double quotes with JSON escaping.
func quoteString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// applyBuiltinJSON finds and replaces JSON fragments in a line with colorized versions.
// Non-JSON text is left unchanged for subsequent regex-based highlighting.
func applyBuiltinJSON(line string) string {
	fragments := findJSONFragments(line)
	if len(fragments) == 0 {
		return line
	}

	var sb strings.Builder
	prev := 0
	for _, frag := range fragments {
		sb.WriteString(line[prev:frag.start])
		sb.WriteString(colorizeJSON(line[frag.start:frag.end]))
		prev = frag.end
	}
	sb.WriteString(line[prev:])
	return sb.String()
}

// byteOffsetsToColumns converts byte offsets to column (display width) offsets
// using incremental ansi.StringWidth calls on successive segments.
// Offsets can be in any order; results correspond to input order.
func byteOffsetsToColumns(line string, offsets []int) []int {
	if len(offsets) == 0 {
		return nil
	}

	type offsetEntry struct {
		byteOff int
		origIdx int
	}
	sorted := make([]offsetEntry, len(offsets))
	for i, o := range offsets {
		sorted[i] = offsetEntry{byteOff: o, origIdx: i}
	}
	slices.SortFunc(sorted, func(a, b offsetEntry) int {
		return a.byteOff - b.byteOff
	})

	// Clamp all offsets to len(line) upfront to avoid mismatches
	for i := range sorted {
		if sorted[i].byteOff > len(line) {
			sorted[i].byteOff = len(line)
		}
	}

	result := make([]int, len(offsets))
	lastByte := 0
	lastCol := 0
	nextIdx := 0

	// Handle offset 0
	for nextIdx < len(sorted) && sorted[nextIdx].byteOff == 0 {
		result[sorted[nextIdx].origIdx] = 0
		nextIdx++
	}

	for nextIdx < len(sorted) {
		off := sorted[nextIdx].byteOff
		// Add width of segment [lastByte:off]
		lastCol += ansi.StringWidth(line[lastByte:off])
		lastByte = off

		for nextIdx < len(sorted) && sorted[nextIdx].byteOff == off {
			result[sorted[nextIdx].origIdx] = lastCol
			nextIdx++
		}
	}

	return result
}

// LogView is a dedicated component for streaming pod logs with autoscroll,
// filtering, search, and built-in syntax highlighting.
type LogView struct {
	viewport   viewport.Model
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
	builtinsEnabled bool

	// Time range
	timeRangeLabel      string
	defaultTimeRange    string
	defaultSinceSeconds int64

	// Virtual scroll
	scrollOffset    int   // top-of-viewport line index in display list
	filteredIndices []int // when filter active, logical buffer indices of matching lines
}

// NewLogView creates a new log view with the given dimensions and ring buffer capacity.
func NewLogView(width, height, bufCapacity int, defaultTimeRange string, defaultSinceSeconds int64) LogView {
	vp := viewport.New(viewport.WithWidth(width-2), viewport.WithHeight(height-2))
	vp.KeyMap.Left = key.NewBinding()
	vp.KeyMap.Right = key.NewBinding()
	vp.HighlightStyle = lipgloss.NewStyle().Background(theme.SearchMatch).Foreground(theme.SearchFg)
	vp.SelectedHighlightStyle = lipgloss.NewStyle().Background(theme.SearchSelected).Foreground(theme.SearchFg).Bold(true)

	return LogView{
		viewport:            vp,
		buffer:              NewDualRingBuffer(bufCapacity),
		width:               width,
		height:              height,
		autoscroll:          true,
		builtinsEnabled:     true,
		timeRangeLabel:      defaultTimeRange,
		defaultTimeRange:    defaultTimeRange,
		defaultSinceSeconds: defaultSinceSeconds,
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
	if lv.viewport.SoftWrap {
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

	// Extract the visible window
	var window []string
	if lv.filterState.Active() {
		for _, idx := range lv.filteredIndices[start:end] {
			window = append(window, lv.buffer.ColoredGet(idx))
		}
	} else {
		window = lv.buffer.ColoredSlice(start, end)
	}

	// Prepend indicator if visible
	if hasIndicator && lv.scrollOffset == 0 {
		indicator := fmt.Sprintf("~%d lines dropped", lv.buffer.Dropped())
		window = append([]string{indicator}, window...)
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
				lv.viewport.HighlightStyle, lv.viewport.SelectedHighlightStyle,
			)
			window = strings.Split(highlighted, "\n")
		}
	}

	lv.viewport.SetContentLines(window)
}

// updateViewportWrapped feeds all display lines to the viewport and delegates
// scrolling to the viewport's native wrap-aware scroll. This is used when
// SoftWrap is enabled because the virtual scroll's logical-line math doesn't
// account for wrapped visual rows.
func (lv *LogView) updateViewportWrapped() {
	// Build full display list
	var lines []string
	if lv.filterState.Active() {
		for _, idx := range lv.filteredIndices {
			lines = append(lines, lv.buffer.ColoredGet(idx))
		}
	} else {
		lines = lv.buffer.ColoredAll()
	}

	// Prepend indicator if applicable
	if lv.buffer.Dropped() > 0 {
		indicator := fmt.Sprintf("~%d lines dropped", lv.buffer.Dropped())
		lines = append([]string{indicator}, lines...)
	}

	// Bake search highlights
	if lv.searchState.Active() && len(lv.matchPositions) > 0 {
		selectedInWindow := -1
		if lv.matchIndex >= 0 && lv.matchIndex < len(lv.matchPositions) {
			selectedInWindow = lv.matchIndex
		}
		joined := strings.Join(lines, "\n")
		highlighted := buildHighlightedDisplay(
			joined, lv.matchPositions, selectedInWindow,
			lv.viewport.HighlightStyle, lv.viewport.SelectedHighlightStyle,
		)
		lines = strings.Split(highlighted, "\n")
	}

	lv.viewport.SetContentLines(lines)
	if lv.autoscroll {
		lv.viewport.GotoBottom()
	}
}

// AppendLine appends a log line to the ring buffer and updates the viewport.
func (lv *LogView) AppendLine(line string) {
	droppedBefore := lv.buffer.Dropped()
	lv.buffer.Append(line, lv.applyHighlights(line))

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

	if lv.autoscroll && !lv.viewport.SoftWrap {
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

// applyHighlights applies built-in highlight rules to a single line using lipgloss.StyleRanges.
func (lv *LogView) applyHighlights(line string) string {
	if !lv.builtinsEnabled {
		return line
	}

	line = applyBuiltinJSON(line)

	type matchInfo struct {
		startByte, endByte int
		style              lipgloss.Style
	}
	var matches []matchInfo

	// Log levels
	for _, h := range builtinLogLevels {
		for _, m := range h.re.FindAllStringIndex(line, -1) {
			matches = append(matches, matchInfo{m[0], m[1], h.style})
		}
	}

	// Timestamps (submatch groups)
	for _, sm := range builtinTimestamp.FindAllStringSubmatchIndex(line, -1) {
		if sm[2] >= 0 && sm[3] >= 0 {
			matches = append(matches, matchInfo{sm[2], sm[3], timestampDateStyle})
		}
		if sm[4] >= 0 && sm[5] >= 0 {
			matches = append(matches, matchInfo{sm[4], sm[5], timestampTimeStyle})
		}
		if sm[6] >= 0 && sm[7] >= 0 {
			matches = append(matches, matchInfo{sm[6], sm[7], timestampTZStyle})
		}
	}

	// IPv4 with validation
	for _, sm := range builtinIPv4.FindAllStringSubmatchIndex(line, -1) {
		valid := true
		for g := 1; g <= 4; g++ {
			octetStr := line[sm[2*g]:sm[2*g+1]]
			v, err := strconv.Atoi(octetStr)
			if err != nil || v < 0 || v > 255 {
				valid = false
				break
			}
		}
		if valid {
			matches = append(matches, matchInfo{sm[0], sm[1], builtinIPStyle})
		}
	}

	if len(matches) == 0 {
		return line
	}

	// Collect all byte offsets
	offsets := make([]int, len(matches)*2)
	for i, m := range matches {
		offsets[i*2] = m.startByte
		offsets[i*2+1] = m.endByte
	}

	// Single-pass conversion
	cols := byteOffsetsToColumns(line, offsets)

	// Build ranges using converted columns
	ranges := make([]lipgloss.Range, len(matches))
	for i, m := range matches {
		ranges[i] = lipgloss.NewRange(cols[i*2], cols[i*2+1], m.style)
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
	if lv.timeRangeLabel != "" {
		sb.WriteString(" " + lv.timeRangeLabel)
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
	lv.viewport.ClearHighlights()
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
	if lv.viewport.SoftWrap {
		lv.viewport.SetYOffset(pos.line)
		lv.autoscroll = false
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
		lv.viewport.SetWidth(w)
		lv.viewport.SetHeight(h)
	} else {
		lv.viewport.SetWidth(w - 2)
		lv.viewport.SetHeight(h - 2)
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
	return lv.builtinsEnabled
}

// ToggleSyntax toggles built-in syntax highlighting on/off and re-renders.
func (lv *LogView) ToggleSyntax() {
	lv.builtinsEnabled = !lv.builtinsEnabled
	for i := range lv.buffer.Len() {
		lv.buffer.SetColored(i, lv.applyHighlights(lv.buffer.RawGet(i)))
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
		if lv.viewport.SoftWrap {
			lv.viewport.GotoBottom()
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
	lv.buffer.Append(raw, styled)
	if lv.filterState.Active() {
		lv.filteredIndices = append(lv.filteredIndices, lv.buffer.Len()-1)
	}
	if lv.autoscroll && !lv.viewport.SoftWrap {
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
	var cmd tea.Cmd
	lv.viewport, cmd = lv.viewport.Update(msg)
	return lv, cmd
}

// ScrollUp scrolls the viewport up by one line.
func (lv *LogView) ScrollUp() {
	if lv.viewport.SoftWrap {
		lv.viewport.ScrollUp(1)
		lv.autoscroll = false
		return
	}
	lv.scrollOffset--
	lv.autoscroll = false
	lv.updateViewport()
}

// ScrollDown scrolls the viewport down by one line.
func (lv *LogView) ScrollDown() {
	if lv.viewport.SoftWrap {
		lv.viewport.ScrollDown(1)
		if lv.viewport.AtBottom() {
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
func (lv *LogView) PageUp() {
	if lv.viewport.SoftWrap {
		lv.viewport.HalfPageUp()
		lv.autoscroll = false
		return
	}
	lv.scrollOffset -= lv.viewportHeight()
	lv.autoscroll = false
	lv.updateViewport()
}

// PageDown scrolls the viewport down by one page.
func (lv *LogView) PageDown() {
	if lv.viewport.SoftWrap {
		lv.viewport.HalfPageDown()
		if lv.viewport.AtBottom() {
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
	if lv.viewport.SoftWrap {
		lv.viewport.GotoTop()
		lv.autoscroll = false
		return
	}
	lv.scrollOffset = 0
	lv.autoscroll = false
	lv.updateViewport()
}

// GotoBottom scrolls to the bottom of the content.
func (lv *LogView) GotoBottom() {
	if lv.viewport.SoftWrap {
		lv.viewport.GotoBottom()
		lv.autoscroll = true
		return
	}
	lv.scrollOffset = lv.totalDisplayLines() - lv.viewportHeight()
	lv.autoscroll = true
	lv.updateViewport()
}

// ToggleWrap toggles soft-wrap on the viewport. When enabling wrap,
// the horizontal offset is reset since horizontal scroll is a no-op.
func (lv *LogView) ToggleWrap() {
	lv.viewport.SoftWrap = !lv.viewport.SoftWrap
	if lv.viewport.SoftWrap {
		lv.viewport.SetXOffset(0)
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
	lv.viewport.SetContentLines(nil)
	lv.viewport.ClearHighlights()
	lv.matchPositions = nil
	lv.matchIndex = -1
	lv.scrollOffset = 0
	lv.filteredIndices = nil
	lv.autoscroll = true
	lv.timeRangeLabel = lv.defaultTimeRange
	lv.filterState.Clear()
	lv.searchState.Clear()
}
