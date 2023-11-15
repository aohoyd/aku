package ui

import (
	"fmt"
	"time"

	"github.com/aohoyd/aku/internal/config"
	"github.com/charmbracelet/x/ansi"
)

// StatusBar displays context-sensitive key help at the bottom.
type StatusBar struct {
	hints       []config.KeyHint
	indicator   string
	error_      string
	errorTime   time.Time
	warning     string
	warningTime time.Time
	width       int
}

func NewStatusBar(width int) StatusBar {
	return StatusBar{width: width}
}

func (s *StatusBar) SetHints(hints []config.KeyHint) {
	s.hints = hints
}

func (s *StatusBar) ClearHints() {
	s.hints = nil
}

func (s *StatusBar) SetError(err string) {
	s.error_ = err
	s.errorTime = time.Now()
}

func (s *StatusBar) SetWarning(w string) {
	s.warning = w
	s.warningTime = time.Now()
}

func (s *StatusBar) SetIndicator(ind string) {
	s.indicator = ind
}

func (s *StatusBar) SetWidth(w int) {
	s.width = w
}

func (s StatusBar) View() string {
	if s.width < 2 {
		return ""
	}

	var line string

	if s.indicator != "" {
		line = s.indicator
	}

	// Show error if recent (< 3 seconds)
	if s.error_ != "" && time.Since(s.errorTime) < 3*time.Second {
		if line != "" {
			line += " "
		}
		line += StatusErrorStyle.Render(s.error_)
		return StatusBarStyle.Width(s.width).Render(line)
	}

	// Show warning if recent (< 5 seconds)
	if s.warning != "" && time.Since(s.warningTime) < 5*time.Second {
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
