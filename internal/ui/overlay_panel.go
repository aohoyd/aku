package ui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/aohoyd/aku/internal/theme"
)

// Overlay is a composable building block for modal overlay panels.
// It manages optional title, text inputs, scrollable item list, and footer.
type Overlay struct {
	active bool
	width  int
	height int

	// Optional sections
	title      string
	content    string
	footer     string
	fixedWidth int

	// Text inputs (multi-field form support)
	inputs   []textinput.Model
	focusIdx int // which input has focus (-1 = list)

	// Scrollable item list
	items            []string
	itemsSet         bool // distinguishes "not configured" from "empty slice"
	cursor           int
	offset           int
	maxVisible       int
	noItemsMsg       string
	postListInputIdx int // inputs >= this index render after the list section
	promptStyle      *lipgloss.Style
}

// overlayChrome returns the total horizontal columns consumed by
// OverlayStyle borders and padding.
func overlayChrome() int {
	return OverlayStyle.GetBorderLeftSize() + OverlayStyle.GetBorderRightSize() +
		OverlayStyle.GetPaddingLeft() + OverlayStyle.GetPaddingRight()
}

// NewOverlay creates a new inactive overlay with sensible defaults.
func NewOverlay() Overlay {
	ps := lipgloss.NewStyle().Foreground(theme.Prompt)
	return Overlay{
		noItemsMsg:  "(no items)",
		focusIdx:    -1,
		promptStyle: &ps,
	}
}

// Active returns whether the overlay is currently active.
func (o Overlay) Active() bool { return o.active }

// SetActive sets the overlay's active state.
func (o *Overlay) SetActive(v bool) { o.active = v }

// Reset clears all dynamic state (inputs, cursor, offset) and deactivates.
func (o *Overlay) Reset() {
	o.active = false
	o.inputs = nil
	o.focusIdx = -1
	o.content = ""
	o.items = nil
	o.itemsSet = false
	o.cursor = 0
	o.offset = 0
	o.postListInputIdx = 0
	ps := lipgloss.NewStyle().Foreground(theme.Prompt)
	o.promptStyle = &ps
}

// SetTitle sets the overlay title.
func (o *Overlay) SetTitle(t string) { o.title = t }

// SetContent sets the overlay content text rendered inside the box.
func (o *Overlay) SetContent(s string) { o.content = s }

// SetFooter sets the overlay footer text.
func (o *Overlay) SetFooter(f string) { o.footer = f }

// SetFixedWidth sets a fixed inner width. 0 means auto-size.
func (o *Overlay) SetFixedWidth(w int) { o.fixedWidth = w }

// SetSize updates the terminal dimensions available to the overlay.
func (o *Overlay) SetSize(w, h int) { o.width = w; o.height = h }

// SetMaxVisible sets the maximum number of visible items in the list.
func (o *Overlay) SetMaxVisible(n int) { o.maxVisible = n }

// SetNoItemsMsg sets the message shown when the item list is empty.
func (o *Overlay) SetNoItemsMsg(msg string) { o.noItemsMsg = msg }

// SetPostListInputIdx configures which inputs render after the list.
// Inputs with index >= idx render after the list, others render before.
// Default 0 means all inputs render before the list (backward compatible).
func (o *Overlay) SetPostListInputIdx(idx int) { o.postListInputIdx = idx }

// SetPromptStyle sets a style applied to all subsequently added input prompts.
func (o *Overlay) SetPromptStyle(s lipgloss.Style) { o.promptStyle = &s }

// FocusedInput returns the index of the currently focused input (-1 = list).
func (o Overlay) FocusedInput() int { return o.focusIdx }

// FocusList sets focus to the item list, blurring all inputs.
func (o *Overlay) FocusList() {
	o.blurAll()
	o.focusIdx = -1
}

// SetItems sets the item list, marks items as configured, and resets the cursor.
func (o *Overlay) SetItems(items []string) {
	o.items = items
	o.itemsSet = true
	o.cursor = 0
	o.offset = 0
	o.clampCursor()
}

// Items returns the current item list.
func (o Overlay) Items() []string { return o.items }

// SetItemsVisible controls whether the item list section is rendered.
func (o *Overlay) SetItemsVisible(v bool) { o.itemsSet = v }

// Cursor returns the current cursor position.
func (o Overlay) Cursor() int { return o.cursor }

// SetCursor sets the cursor position, clamping to valid range.
func (o *Overlay) SetCursor(pos int) {
	o.cursor = pos
	o.clampCursor()
}

// SelectedItem returns the item at the cursor, or "" if no items.
func (o Overlay) SelectedItem() string {
	if len(o.items) == 0 {
		return ""
	}
	return o.items[o.cursor]
}

// HandleListKeys handles Up/Down key presses for list navigation.
// Returns true if the key was handled.
func (o *Overlay) HandleListKeys(msg tea.KeyPressMsg) bool {
	switch msg.Code {
	case tea.KeyDown:
		o.cursor++
		o.clampCursor()
		return true
	case tea.KeyUp:
		o.cursor--
		o.clampCursor()
		return true
	}
	return false
}

// clampCursor ensures cursor and viewport offset stay within valid bounds.
func (o *Overlay) clampCursor() {
	if len(o.items) == 0 {
		o.cursor = 0
		o.offset = 0
		return
	}
	if o.cursor < 0 {
		o.cursor = 0
	}
	if o.cursor >= len(o.items) {
		o.cursor = len(o.items) - 1
	}
	mv := o.effectiveMaxVisible()
	if o.cursor < o.offset {
		o.offset = o.cursor
	}
	if o.cursor >= o.offset+mv {
		o.offset = o.cursor - mv + 1
	}
}

// AddInput appends a text input and returns its index.
// If a prompt style has been set, it is applied to the input.
func (o *Overlay) AddInput(ti textinput.Model) int {
	if o.promptStyle != nil {
		styles := ti.Styles()
		styles.Focused.Prompt = *o.promptStyle
		styles.Blurred.Prompt = *o.promptStyle
		ti.SetStyles(styles)
	}
	o.inputs = append(o.inputs, ti)
	return len(o.inputs) - 1
}

// Input returns a pointer to the input at index i.
func (o *Overlay) Input(i int) *textinput.Model {
	return &o.inputs[i]
}

// InputValue returns the current value of the input at index i.
func (o Overlay) InputValue(i int) string {
	return o.inputs[i].Value()
}

// InputCount returns the number of text inputs.
func (o Overlay) InputCount() int {
	return len(o.inputs)
}

// FocusInput sets focus to the input at index i, blurring all others.
func (o *Overlay) FocusInput(i int) {
	o.focusIdx = i
	for j := range o.inputs {
		if j == i {
			o.inputs[j].Focus()
		} else {
			o.inputs[j].Blur()
		}
	}
}

// FocusNextInput cycles focus forward: inputs -> list (-1) -> first input.
func (o *Overlay) FocusNextInput() {
	if len(o.inputs) == 0 {
		return
	}
	if o.focusIdx == -1 {
		// list -> first input
		o.FocusInput(0)
		return
	}
	if o.focusIdx >= len(o.inputs)-1 {
		// last input -> list (if items exist) or wrap to first input
		if len(o.items) > 0 {
			o.blurAll()
			o.focusIdx = -1
		} else {
			o.FocusInput(0)
		}
		return
	}
	o.FocusInput(o.focusIdx + 1)
}

// FocusPrevInput cycles focus backward: first input -> list (-1) -> last input.
func (o *Overlay) FocusPrevInput() {
	if len(o.inputs) == 0 {
		return
	}
	if o.focusIdx == -1 {
		// list -> last input
		o.FocusInput(len(o.inputs) - 1)
		return
	}
	if o.focusIdx <= 0 {
		// first input -> list (if items exist) or wrap to last input
		if len(o.items) > 0 {
			o.blurAll()
			o.focusIdx = -1
		} else {
			o.FocusInput(len(o.inputs) - 1)
		}
		return
	}
	o.FocusInput(o.focusIdx - 1)
}

// UpdateInputs forwards a message to the focused input and returns any command.
func (o *Overlay) UpdateInputs(msg tea.Msg) tea.Cmd {
	if o.focusIdx >= 0 && o.focusIdx < len(o.inputs) {
		var cmd tea.Cmd
		o.inputs[o.focusIdx], cmd = o.inputs[o.focusIdx].Update(msg)
		return cmd
	}
	return nil
}

// blurAll blurs all text inputs.
func (o *Overlay) blurAll() {
	for i := range o.inputs {
		o.inputs[i].Blur()
	}
}

// effectiveMaxVisible returns maxVisible if set, otherwise a computed default.
func (o Overlay) effectiveMaxVisible() int {
	if o.maxVisible > 0 {
		return o.maxVisible
	}
	v := max(o.height-8, 1)
	return v
}

// View renders the overlay panel. Returns "" if inactive.
func (o Overlay) View() string {
	if !o.active {
		return ""
	}

	// Compute inner width first so we can constrain input widths.
	innerWidth := o.computeInnerWidth()
	o.syncInputWidths(innerWidth)

	var lines []string

	// Content (rendered inside the box)
	if o.content != "" {
		lines = append(lines, TitleStyle.Render(o.content))
		lines = append(lines, "")
	}

	// Pre-list inputs
	preEnd := len(o.inputs)
	if o.postListInputIdx > 0 && o.postListInputIdx < len(o.inputs) {
		preEnd = o.postListInputIdx
	}
	if preEnd > 0 {
		for i := 0; i < preEnd; i++ {
			lines = append(lines, o.inputs[i].View())
		}
		lines = append(lines, "")
	}

	// List
	if o.itemsSet {
		mv := o.effectiveMaxVisible()
		var listLines []string

		if len(o.items) == 0 {
			listLines = append(listLines, "  "+o.noItemsMsg)
		} else {
			start := o.offset
			end := min(start+mv, len(o.items))

			cursorStyle := lipgloss.NewStyle().Bold(true).Foreground(theme.Highlight)
			for i := start; i < end; i++ {
				if i == o.cursor {
					listLines = append(listLines, cursorStyle.Render("> "+o.items[i]))
				} else {
					listLines = append(listLines, "  "+o.items[i])
				}
			}

			remaining := len(o.items) - end
			if remaining > 0 {
				listLines = append(listLines, fmt.Sprintf("  ... and %d more", remaining))
			}
		}

		// Pad to exactly mv lines for stable height (allow mv+1 for overflow indicator)
		for len(listLines) < mv {
			listLines = append(listLines, "")
		}
		if len(listLines) > mv+1 {
			listLines = listLines[:mv+1]
		}

		lines = append(lines, listLines...)
	}

	// Post-list inputs
	if o.postListInputIdx > 0 && o.postListInputIdx < len(o.inputs) {
		lines = append(lines, "")
		for i := o.postListInputIdx; i < len(o.inputs); i++ {
			lines = append(lines, o.inputs[i].View())
		}
	}

	// Footer
	if o.footer != "" {
		lines = append(lines, "")
		lines = append(lines, lipgloss.NewStyle().Width(innerWidth).Align(lipgloss.Center).Render(o.footer))
	}

	content := strings.Join(lines, "\n")
	box := OverlayStyle.Width(innerWidth).Render(content)
	return injectCenteredBorderTitle(box, o.title)
}

// syncInputWidths sets each input's visible width so text scrolls horizontally
// instead of overflowing into the overlay's padding/border chrome.
func (o *Overlay) syncInputWidths(innerWidth int) {
	cw := innerWidth - overlayChrome()
	for i := range o.inputs {
		pw := lipgloss.Width(o.inputs[i].Prompt)
		w := max(
			// -1 for cursor column
			cw-pw-1, 1)
		o.inputs[i].SetWidth(w)
		o.inputs[i].SetCursor(o.inputs[i].Position())
	}
}

// computeInnerWidth determines the total width for the overlay (content + chrome).
// lipgloss Width() includes padding and border, so we add chrome to content measurements.
func (o Overlay) computeInnerWidth() int {
	if o.fixedWidth > 0 {
		return o.fixedWidth
	}

	chrome := overlayChrome()

	// Auto-measure: find widest content
	maxW := max(len(o.footer), len(o.title))
	if len(o.content) > maxW {
		maxW = len(o.content)
	}
	for _, item := range o.items {
		w := len(item) + 4 // account for cursor prefix and padding
		if w > maxW {
			maxW = w
		}
	}
	for _, inp := range o.inputs {
		w := len(inp.Prompt) + 20 // prompt + reasonable input space
		if w > maxW {
			maxW = w
		}
	}

	// Convert content width to total width for lipgloss Width()
	maxW += chrome

	// Clamp
	upper := max(o.width, 26)
	if upper > 80 {
		upper = 80
	}
	if maxW < 26 {
		maxW = 26
	}
	if maxW > upper {
		maxW = upper
	}
	return maxW
}

// injectCenteredBorderTitle replaces the top border line with a centered title.
// Returns the original string unchanged if title is empty or too wide.
func injectCenteredBorderTitle(rendered, title string) string {
	if title == "" {
		return rendered
	}
	lines := strings.Split(rendered, "\n")
	if len(lines) == 0 {
		return rendered
	}
	lineWidth := lipgloss.Width(lines[0])
	titleRendered := TitleStyle.Render(title)
	titleWidth := lipgloss.Width(titleRendered)
	if titleWidth+2 >= lineWidth {
		return rendered
	}
	border := lipgloss.RoundedBorder()
	bc := lipgloss.NewStyle().Foreground(theme.Accent)
	available := lineWidth - 2 - titleWidth // subtract corners
	leftDashes := available / 2
	rightDashes := available - leftDashes
	lines[0] = bc.Render(string(border.TopLeft)) +
		bc.Render(strings.Repeat(string(border.Top), leftDashes)) +
		titleRendered +
		bc.Render(strings.Repeat(string(border.Top), rightDashes)) +
		bc.Render(string(border.TopRight))
	return strings.Join(lines, "\n")
}
