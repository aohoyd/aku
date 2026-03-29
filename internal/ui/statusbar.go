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

// StatusBar displays context-sensitive key help at the bottom.
type StatusBar struct {
	hints          []config.KeyHint
	indicator      string
	errText         string
	errorVisible   bool
	warning        string
	warningVisible bool
	width          int
	spinner        spinner.Model
	online         bool
	inflight       int
}

func NewStatusBar(width int) StatusBar {
	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	sp.Style = lipgloss.NewStyle().Foreground(theme.StatusRunning)
	return StatusBar{width: width, spinner: sp}
}

func (s *StatusBar) SetHints(hints []config.KeyHint) {
	s.hints = hints
}

func (s *StatusBar) ClearHints() {
	s.hints = nil
}

func (s *StatusBar) SetError(err string) tea.Cmd {
	s.errText = err
	if err == "" {
		s.errorVisible = false
		return nil
	}
	s.errorVisible = true
	return tea.Tick(3*time.Second, func(time.Time) tea.Msg {
		return msgs.StatusBarClearErrorMsg{}
	})
}

func (s *StatusBar) SetWarning(w string) tea.Cmd {
	s.warning = w
	if w == "" {
		s.warningVisible = false
		return nil
	}
	s.warningVisible = true
	return tea.Tick(5*time.Second, func(time.Time) tea.Msg {
		return msgs.StatusBarClearWarningMsg{}
	})
}

func (s *StatusBar) SetIndicator(ind string) {
	s.indicator = ind
}

func (s *StatusBar) SetWidth(w int) {
	s.width = w
}

func (s *StatusBar) SetOnline(v bool) {
	s.online = v
	if v {
		s.spinner.Style = lipgloss.NewStyle().Foreground(theme.StatusRunning)
	} else {
		s.spinner.Style = lipgloss.NewStyle().Foreground(theme.Error)
	}
}

func (s *StatusBar) StartOperation() tea.Cmd {
	s.inflight++
	if s.inflight == 1 {
		return s.spinner.Tick
	}
	return nil
}

func (s *StatusBar) EndOperation() {
	if s.inflight > 0 {
		s.inflight--
	}
}

func (s *StatusBar) Busy() bool {
	return s.inflight > 0
}

func (s *StatusBar) Update(msg tea.Msg) {
	switch msg.(type) {
	case msgs.StatusBarClearErrorMsg:
		s.errorVisible = false
	case msgs.StatusBarClearWarningMsg:
		s.warningVisible = false
	}
}

func (s *StatusBar) UpdateSpinner(msg tea.Msg) tea.Cmd {
	if s.inflight <= 0 {
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
	if s.inflight > 0 {
		healthSlot = s.spinner.View() + " "
	} else if s.online {
		healthSlot = StatusOnlineStyle.Render("●") + " "
	} else {
		healthSlot = StatusOfflineStyle.Render("●") + " "
	}

	line = healthSlot
	if s.indicator != "" {
		line += s.indicator
	}

	// Show error if visible
	if s.errText != "" && s.errorVisible {
		if line != "" {
			line += " "
		}
		line += StatusErrorStyle.Render(s.errText)
		return StatusBarStyle.Width(s.width).Render(line)
	}

	// Show warning if visible
	if s.warning != "" && s.warningVisible {
		if line != "" {
			line += " "
		}
		line += StatusWarningStyle.Render(s.warning)
		return StatusBarStyle.Width(s.width).Render(line)
	}

	// Build hints — fill available width, then "? more" if truncated
	moreSuffix := fmt.Sprintf("  %s %s", StatusKeyStyle.Render("?"), StatusHelpStyle.Render("more"))
	moreWidth := ansi.StringWidth(moreSuffix)
	usedWidth := ansi.StringWidth(line)

	for i, h := range s.hints {
		rendered := fmt.Sprintf("%s %s", StatusKeyStyle.Render(h.Key), StatusHelpStyle.Render(h.Help))
		gap := "  "
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
