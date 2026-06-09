package ui

import (
	"charm.land/lipgloss/v2"
	"github.com/aohoyd/aku/internal/theme"
)

// pullPolicyDefault is the option representing an unset imagePullPolicy.
const pullPolicyDefault = "(default)"

// CycleField is a self-contained widget that cycles through a fixed list of
// options in place. It carries no bubbletea dependency: the owning overlay
// drives it by calling Next/Prev and reads the selection via Value.
// Construct one with NewPullPolicyField.
type CycleField struct {
	prompt  string
	options []string
	idx     int
	focused bool
}

// NewPullPolicyField builds a CycleField for editing a container's
// imagePullPolicy. An empty current value selects "(default)" (index 0).
func NewPullPolicyField(current string) CycleField {
	f := CycleField{
		prompt:  "pull",
		options: []string{pullPolicyDefault, "Always", "IfNotPresent", "Never"},
	}
	f.SetValue(current)
	return f
}

// Next advances the selection, wrapping around to the first option.
func (f *CycleField) Next() {
	if len(f.options) == 0 {
		return
	}
	f.idx = (f.idx + 1) % len(f.options)
}

// Prev moves the selection back, wrapping around to the last option.
func (f *CycleField) Prev() {
	if len(f.options) == 0 {
		return
	}
	f.idx = (f.idx - 1 + len(f.options)) % len(f.options)
}

// Focus marks the field as focused.
func (f *CycleField) Focus() { f.focused = true }

// Blur marks the field as unfocused.
func (f *CycleField) Blur() { f.focused = false }

// Focused reports whether the field currently has focus.
func (f CycleField) Focused() bool { return f.focused }

// Value returns the selected option, or "" when "(default)" is selected.
func (f CycleField) Value() string {
	if f.idx <= 0 || f.idx >= len(f.options) {
		return ""
	}
	return f.options[f.idx]
}

// SetValue selects the option matching s. An empty string selects "(default)"
// (index 0); an unknown value also falls back to index 0.
func (f *CycleField) SetValue(s string) {
	if s == "" {
		f.idx = 0
		return
	}
	for i, opt := range f.options {
		if opt == s {
			f.idx = i
			return
		}
	}
	f.idx = 0
}

// View renders the field like "  pull:  ‹ IfNotPresent ›". When focused the
// value and arrows are highlighted; when blurred the arrows are dimmed.
func (f CycleField) View() string {
	if len(f.options) == 0 {
		return ""
	}

	promptStyle := lipgloss.NewStyle().Foreground(theme.Prompt)

	var valueStyle, arrowStyle lipgloss.Style
	if f.focused {
		valueStyle = lipgloss.NewStyle().Bold(true).Foreground(theme.Highlight)
		arrowStyle = lipgloss.NewStyle().Foreground(theme.Accent)
	} else {
		valueStyle = lipgloss.NewStyle().Foreground(theme.Muted)
		arrowStyle = lipgloss.NewStyle().Foreground(theme.Subtle)
	}

	left := arrowStyle.Render("‹")
	right := arrowStyle.Render("›")
	value := valueStyle.Render(f.options[f.idx])

	return "  " + promptStyle.Render(f.prompt+":") + "  " + left + " " + value + " " + right
}
