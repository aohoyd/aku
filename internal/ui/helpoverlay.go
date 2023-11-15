package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/aohoyd/aku/internal/config"
	"github.com/aohoyd/aku/internal/theme"
)

// HelpOverlay is a full-screen modal displaying all keybindings grouped by scope.
type HelpOverlay struct {
	groups    []config.HintGroup
	scroll    int
	active    bool
	searching bool
	query     string
	width     int
	height    int
}

// NewHelpOverlay creates a new (inactive) help overlay.
func NewHelpOverlay(width, height int) HelpOverlay {
	return HelpOverlay{width: width, height: height}
}

// Open activates the overlay with the provided hint groups.
func (h *HelpOverlay) Open(groups []config.HintGroup) {
	h.groups = groups
	h.scroll = 0
	h.searching = false
	h.query = ""
	h.active = true
}

// Active reports whether the overlay is currently shown.
func (h HelpOverlay) Active() bool { return h.active }

// SetSize updates the terminal dimensions.
func (h *HelpOverlay) SetSize(w, height int) {
	h.width = w
	h.height = height
}

// Update handles key events for the help overlay.
func (h HelpOverlay) Update(msg tea.Msg) (HelpOverlay, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if h.searching {
			switch msg.String() {
			case "esc":
				h.query = ""
				h.searching = false
				h.scroll = 0
			case "enter":
				h.searching = false
			case "backspace":
				if len(h.query) > 0 {
					runes := []rune(h.query)
					h.query = string(runes[:len(runes)-1])
					h.scroll = 0
				}
				if h.query == "" {
					h.searching = false
				}
			default:
				if msg.Text != "" {
					h.query += msg.Text
					h.scroll = 0
				}
			}
		} else {
			switch msg.String() {
			case "/":
				h.searching = true
			case "esc", "?", "q":
				h.active = false
				h.query = ""
				h.searching = false
			case "j", "down":
				h.scroll++
				if max := h.maxScroll(); h.scroll > max {
					h.scroll = max
				}
			case "k", "up":
				if h.scroll > 0 {
					h.scroll--
				}
			}
		}
	}
	return h, nil
}

// filteredGroups returns hint groups filtered by the current query.
func (h HelpOverlay) filteredGroups() []config.HintGroup {
	if h.query == "" {
		return h.groups
	}
	q := strings.ToLower(h.query)
	var result []config.HintGroup
	for _, g := range h.groups {
		var filtered []config.KeyHint
		for _, hint := range g.Hints {
			if strings.Contains(strings.ToLower(hint.Key+" "+hint.Help), q) {
				filtered = append(filtered, hint)
			}
		}
		if len(filtered) > 0 {
			result = append(result, config.HintGroup{Scope: g.Scope, Hints: filtered})
		}
	}
	return result
}

// tallestGroup returns the number of lines in the tallest group (header + hints).
func (h HelpOverlay) tallestGroup() int {
	max := 0
	for _, g := range h.filteredGroups() {
		n := 1 + len(g.Hints) // header + hints
		if n > max {
			max = n
		}
	}
	return max
}

func (h HelpOverlay) maxScroll() int {
	// In horizontal layout the content height is the tallest column
	contentH := h.tallestGroup()
	visibleH := h.visibleHeight()
	max := contentH - visibleH
	if max < 0 {
		return 0
	}
	return max
}

// visibleHeight returns the number of content lines that fit inside the overlay.
// Accounts for border+padding (6), title+gap (2), footer+gap (2), optional search line (1).
func (h HelpOverlay) visibleHeight() int {
	overhead := 10
	if h.searching || h.query != "" {
		overhead++ // search input line
	}
	v := max(h.height-overhead, 5)
	return v
}

// View renders the help overlay. Returns empty string when inactive.
func (h HelpOverlay) View() string {
	if !h.active {
		return ""
	}

	groups := h.filteredGroups()
	nGroups := len(groups)
	if nGroups == 0 {
		footerStyle := lipgloss.NewStyle().Foreground(theme.Muted)
		title := TitleStyle.Render("Keybindings")
		searchLine := h.renderSearchLine(footerStyle)
		noMatch := footerStyle.Render("No matching keybindings")
		footer := h.renderFooter(footerStyle, 0)
		content := title + searchLine + "\n\n" + noMatch + "\n\n" + footer
		return OverlayStyle.Render(content)
	}

	keyStyle := lipgloss.NewStyle().Bold(true).Foreground(theme.Highlight)
	scopeStyle := lipgloss.NewStyle().Bold(true).Foreground(theme.Accent).
		Underline(true)
	helpStyle := lipgloss.NewStyle().Foreground(theme.Muted)
	footerStyle := lipgloss.NewStyle().Foreground(theme.Muted)

	gap := "  "

	// Measure each column: widest key and widest (key+help) entry
	keyWidths := make([]int, nGroups)
	colWidths := make([]int, nGroups) // in raw characters, pre-styling
	for i, g := range groups {
		for _, hint := range g.Hints {
			if len(hint.Key) > keyWidths[i] {
				keyWidths[i] = len(hint.Key)
			}
		}
		keyWidths[i] += 2 // padding after key
		for _, hint := range g.Hints {
			w := keyWidths[i] + len(hint.Help)
			if w > colWidths[i] {
				colWidths[i] = w
			}
		}
		if len(g.Scope) > colWidths[i] {
			colWidths[i] = len(g.Scope)
		}
	}

	visibleH := h.visibleHeight()

	// Clamp scroll
	scroll := h.scroll
	tallest := 0
	for _, g := range groups {
		n := 1 + len(g.Hints)
		if n > tallest {
			tallest = n
		}
	}
	maxScroll := max(tallest-visibleH, 0)
	if scroll > maxScroll {
		scroll = maxScroll
	}

	// Build each column as a slice of fixed-width lines (no lipgloss Width wrapping)
	colLines := make([][]string, nGroups)
	for i, group := range groups {
		kw := keyWidths[i]
		cw := colWidths[i]

		// Header padded to column width
		header := scopeStyle.Render(group.Scope) + strings.Repeat(" ", cw-len(group.Scope))

		var body []string
		for _, hint := range group.Hints {
			keyPart := keyStyle.Render(fmt.Sprintf("%-*s", kw, hint.Key))
			helpPart := helpStyle.Render(hint.Help)
			// Pad to column width
			used := kw + len(hint.Help)
			pad := ""
			if used < cw {
				pad = strings.Repeat(" ", cw-used)
			}
			body = append(body, keyPart+helpPart+pad)
		}

		// Apply scroll
		if scroll < len(body) {
			body = body[scroll:]
		} else {
			body = nil
		}
		if len(body) > visibleH-1 {
			body = body[:visibleH-1]
		}

		colLines[i] = append([]string{header}, body...)
	}

	// Pad shorter columns with blank lines so all have equal height
	maxLines := 0
	for _, lines := range colLines {
		if len(lines) > maxLines {
			maxLines = len(lines)
		}
	}
	for i := range colLines {
		for len(colLines[i]) < maxLines {
			colLines[i] = append(colLines[i], strings.Repeat(" ", colWidths[i]))
		}
	}

	// Join columns row by row
	var rows []string
	for row := range maxLines {
		var parts []string
		for i := range colLines {
			parts = append(parts, colLines[i][row])
		}
		rows = append(rows, strings.Join(parts, gap))
	}
	body := strings.Join(rows, "\n")

	title := TitleStyle.Render("Keybindings")
	searchLine := h.renderSearchLine(footerStyle)
	footer := h.renderFooter(footerStyle, maxScroll)
	content := title + searchLine + "\n\n" + body + "\n\n" + footer

	return OverlayStyle.Render(content)
}

// renderSearchLine returns the search input line (with leading newline) or empty string.
func (h HelpOverlay) renderSearchLine(footerStyle lipgloss.Style) string {
	if h.searching {
		return "\n" + footerStyle.Render("/") + " " + h.query + "█"
	}
	if h.query != "" {
		return "\n" + footerStyle.Render("/") + " " + h.query
	}
	return ""
}

// renderFooter returns the appropriate footer text based on current state.
func (h HelpOverlay) renderFooter(footerStyle lipgloss.Style, maxScroll int) string {
	if h.searching {
		return footerStyle.Render("enter apply  esc cancel")
	}
	parts := []string{"/ search"}
	if maxScroll > 0 {
		parts = append(parts, "j/k scroll")
	}
	parts = append(parts, "esc/? close")
	return footerStyle.Render(strings.Join(parts, "  "))
}
