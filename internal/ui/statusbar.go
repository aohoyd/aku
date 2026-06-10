package ui

import (
	"fmt"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/aohoyd/aku/internal/config"
	"github.com/aohoyd/aku/internal/msgs"
	"github.com/aohoyd/aku/internal/theme"
	"github.com/charmbracelet/x/ansi"
)

const maxContextWidth = 24

// StatusBar displays context-sensitive key help at the bottom.
type StatusBar struct {
	hints       []config.KeyHint
	indicator   string
	width       int
	spinner     spinner.Model
	online      bool
	inflight    int
	showSpinner bool
	contextName string
}

func NewStatusBar(width int) StatusBar {
	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	sp.Style = lipgloss.NewStyle().Foreground(theme.TextOnAccent).Background(theme.StatusRunning)
	return StatusBar{width: width, spinner: sp, contextName: "default"}
}

func (s *StatusBar) SetHints(hints []config.KeyHint) {
	s.hints = hints
}

func (s *StatusBar) ClearHints() {
	s.hints = nil
}

func (s *StatusBar) SetIndicator(ind string) {
	s.indicator = ind
}

func (s *StatusBar) SetWidth(w int) {
	s.width = w
}

func (s *StatusBar) SetContextName(name string) {
	if name == "" {
		name = "default"
	}
	s.contextName = name
}

// ContextName returns the current context name displayed in the health badge.
// Intended for tests that need to assert a context switch updated the bar.
func (s *StatusBar) ContextName() string {
	return s.contextName
}

// Online reports whether the health badge is currently in its online (green)
// state. Intended for tests asserting the badge tracks the focused pane.
func (s *StatusBar) Online() bool {
	return s.online
}

func truncateContext(name string, max int) string {
	if ansi.StringWidth(name) <= max {
		return name
	}
	runes := []rune(name)
	for i := len(runes) - 1; i > 0; i-- {
		candidate := string(runes[:i]) + "…"
		if ansi.StringWidth(candidate) <= max {
			return candidate
		}
	}
	return "…"
}

func (s *StatusBar) SetOnline(v bool) {
	s.online = v
	if v {
		s.spinner.Style = lipgloss.NewStyle().Foreground(theme.TextOnAccent).Background(theme.StatusRunning)
	} else {
		s.spinner.Style = lipgloss.NewStyle().Foreground(theme.TextOnAccent).Background(theme.Error)
	}
}

func (s *StatusBar) StartOperation() tea.Cmd {
	s.inflight++
	if s.inflight == 1 {
		return tea.Tick(1*time.Second, func(time.Time) tea.Msg {
			return msgs.StatusBarShowSpinnerMsg{}
		})
	}
	return nil
}

func (s *StatusBar) EndOperation() {
	if s.inflight > 0 {
		s.inflight--
	}
	if s.inflight == 0 {
		s.showSpinner = false
	}
}

func (s *StatusBar) Busy() bool {
	return s.inflight > 0
}

func (s *StatusBar) Update(msg tea.Msg) tea.Cmd {
	switch msg.(type) {
	case msgs.StatusBarShowSpinnerMsg:
		if s.inflight > 0 {
			s.showSpinner = true
			return s.spinner.Tick
		}
	}
	return nil
}

func (s *StatusBar) UpdateSpinner(msg tea.Msg) tea.Cmd {
	if !s.showSpinner {
		return nil
	}
	var cmd tea.Cmd
	s.spinner, cmd = s.spinner.Update(msg)
	return cmd
}

func (s StatusBar) View() string {
	if s.width < 2 {
		return ""
	}

	var line string

	// Health indicator slot
	var healthSlot string
	name := truncateContext(s.contextName, maxContextWidth)
	badgeStyle := ContextBadgeOfflineStyle
	if s.online {
		badgeStyle = ContextBadgeOnlineStyle
	}
	if s.showSpinner {
		healthSlot = badgeStyle.Render(name + " " + s.spinner.View())
	} else {
		healthSlot = badgeStyle.Render(name)
	}

	line = healthSlot
	if s.indicator != "" {
		line += s.indicator
	}

	// Build hints — fill available width, then "? more" if truncated
	moreSuffix := fmt.Sprintf("  %s %s", StatusKeyStyle.Render("?"), StatusHelpStyle.Render("more"))
	moreWidth := ansi.StringWidth(moreSuffix)
	usedWidth := ansi.StringWidth(line)

	for i, h := range s.hints {
		rendered := fmt.Sprintf("%s %s", StatusKeyStyle.Render(h.Key), StatusHelpStyle.Render(h.Help))
		gap := " "
		if line == "" {
			gap = ""
		}
		hintWidth := ansi.StringWidth(gap + rendered)
		isLast := i == len(s.hints)-1

		if isLast {
			// Last hint — only needs to fit itself, no "? more" needed
			if usedWidth+hintWidth <= s.width {
				line += gap + rendered
			} else {
				line += moreSuffix
			}
		} else {
			// Not last — reserve space for "? more" in case we stop here
			if usedWidth+hintWidth+moreWidth <= s.width {
				line += gap + rendered
				usedWidth += hintWidth
			} else {
				line += moreSuffix
				break
			}
		}
	}

	return StatusBarStyle.Width(s.width).Render(line)
}
