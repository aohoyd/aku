package ui

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/aohoyd/aku/internal/msgs"
	"github.com/aohoyd/aku/internal/theme"
)

// SearchBar is a bottom-anchored overlay for entering search/filter patterns.
type SearchBar struct {
	input         textinput.Model
	defaultStyles textinput.Styles
	mode          msgs.SearchMode
	active        bool
	width         int
	errMsg        string
}

// NewSearchBar creates a new search bar with the given width.
func NewSearchBar(width int) SearchBar {
	ti := textinput.New()
	ti.CharLimit = 256
	ti.SetWidth(width - 4)
	return SearchBar{input: ti, defaultStyles: ti.Styles(), width: width}
}

// Open activates the search bar in the given mode.
func (s *SearchBar) Open(mode msgs.SearchMode) {
	s.mode = mode
	s.active = true
	s.errMsg = ""
	s.input.SetValue("")
	if mode == msgs.SearchModeSearch {
		s.input.Prompt = "/"
	} else {
		s.input.Prompt = "|"
	}
	s.input.Focus()
}

// Close deactivates the search bar.
func (s *SearchBar) Close() {
	s.active = false
	s.input.Blur()
}

// Active reports whether the search bar is open.
func (s SearchBar) Active() bool { return s.active }

// SetValue sets the text input value (for testing).
func (s *SearchBar) SetValue(v string) { s.input.SetValue(v) }

// SetError sets an error message and toggles the input styling to indicate
// invalid regex. When err is non-empty, the prompt, text, and cursor turn red.
func (s *SearchBar) SetError(err string) {
	s.errMsg = err
	if err != "" {
		styles := s.defaultStyles
		errStyle := lipgloss.NewStyle().Foreground(theme.Error)
		styles.Focused.Text = errStyle
		styles.Focused.Prompt = errStyle
		styles.Cursor.Color = theme.Error
		s.input.SetStyles(styles)
	} else {
		s.input.SetStyles(s.defaultStyles)
	}
}

// SetWidth updates the width of the search bar.
func (s *SearchBar) SetWidth(w int) {
	s.width = w
	s.input.SetWidth(w - 4)
}

// Update handles key messages for the search bar.
func (s SearchBar) Update(msg tea.Msg) (SearchBar, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.Code {
		case tea.KeyEnter:
			pattern := s.input.Value()
			mode := s.mode
			s.Close()
			return s, func() tea.Msg {
				return msgs.SearchSubmittedMsg{Pattern: pattern, Mode: mode}
			}
		case tea.KeyEscape:
			mode := s.mode
			s.Close()
			return s, func() tea.Msg {
				return msgs.SearchClearedMsg{Mode: mode}
			}
		}
	}

	var cmd tea.Cmd
	prevVal := s.input.Value()
	s.input, cmd = s.input.Update(msg)

	// Emit live change for both search and filter modes
	if s.input.Value() != prevVal {
		pattern := s.input.Value()
		mode := s.mode
		liveCmd := func() tea.Msg {
			return msgs.SearchChangedMsg{Pattern: pattern, Mode: mode}
		}
		return s, tea.Batch(cmd, liveCmd)
	}

	return s, cmd
}

// InlineView returns a styled string fragment for embedding in a title border.
// Returns "" when inactive. Includes prompt, text, and cursor.
// Error styling is applied via textinput styles set in SetError().
func (s SearchBar) InlineView() string {
	if !s.active {
		return ""
	}
	return s.input.View()
}

// View renders the search bar. Returns empty string when not active.
func (s SearchBar) View() string {
	if !s.active {
		return ""
	}
	w := max(min(s.width/2, 60), 20)

	content := s.input.View()
	if s.errMsg != "" {
		errLine := lipgloss.NewStyle().Foreground(theme.Error).Render(s.errMsg)
		content = content + "\n" + errLine
	}
	return OverlayStyle.Width(w).Render(content)
}
