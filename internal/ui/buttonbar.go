package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/aohoyd/aku/internal/theme"
)

// Button defines a single footer button with a label and hotkey hint.
type Button struct {
	Label  string
	Hotkey string
}

// RenderButtonBar renders a styled button bar for overlay footers.
// Pass focusedIdx = -1 to render all buttons without highlight.
func RenderButtonBar(buttons []Button, focusedIdx int) string {
	focusedStyle := lipgloss.NewStyle().Bold(true).Foreground(theme.Highlight)
	normalStyle := lipgloss.NewStyle().Foreground(theme.Subtle)
	sepStyle := lipgloss.NewStyle().Foreground(theme.Muted)

	sep := sepStyle.Render("│")
	var parts []string
	for i, b := range buttons {
		text := b.Label + "(" + b.Hotkey + ")"
		if i == focusedIdx {
			parts = append(parts, focusedStyle.Render(text))
		} else {
			parts = append(parts, normalStyle.Render(text))
		}
	}
	return sep + " " + strings.Join(parts, " "+sep+" ") + " " + sep
}
