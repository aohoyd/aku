package ui

import (
	"charm.land/lipgloss/v2"
	"github.com/aohoyd/aku/internal/theme"
)

var (
	// Border colors
	FocusedBorderColor   = theme.Accent
	UnfocusedBorderColor = theme.Muted

	// Borders
	FocusedBorderStyle   = lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(FocusedBorderColor)
	UnfocusedBorderStyle = lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(UnfocusedBorderColor)

	// Namespace badge in status bar
	NamespaceStyle = lipgloss.NewStyle().Foreground(theme.TextOnAccent).Background(theme.Accent).Bold(true).Padding(0, 1)

	// Status bar
	StatusBarStyle     = lipgloss.NewStyle().Foreground(theme.Muted).Padding(0, 1)
	StatusKeyStyle     = lipgloss.NewStyle().Bold(true).Foreground(theme.Highlight)
	StatusHelpStyle    = lipgloss.NewStyle().Foreground(theme.Muted)
	StatusErrorStyle   = lipgloss.NewStyle().Foreground(theme.Error).Bold(true)
	StatusWarningStyle = lipgloss.NewStyle().Foreground(theme.Warning).Bold(true)
	StatusOnlineStyle  = lipgloss.NewStyle().Foreground(theme.StatusRunning)
	StatusOfflineStyle = lipgloss.NewStyle().Foreground(theme.Error)

	// Zoom indicator
	ZoomIndicatorStyle = lipgloss.NewStyle().
				Foreground(theme.TextOnAccent).
				Background(theme.Accent).
				Bold(true)

	// Port-forward indicator
	PortForwardIndicatorStyle = lipgloss.NewStyle().
					Foreground(theme.TextOnAccent).
					Background(theme.Highlight).
					Bold(true)

	// Table
	TableHeaderStyle      = lipgloss.NewStyle().Bold(true).Foreground(theme.Highlight).Padding(0, 1)
	TableSelectedStyle    = lipgloss.NewStyle().Background(theme.Accent).Foreground(theme.TextOnAccent).Bold(true)
	TableSelectedDimStyle = lipgloss.NewStyle()

	// Table multi-select
	TableMarkedStyle         = lipgloss.NewStyle().Foreground(theme.Selection).Bold(true)
	TableMarkedSelectedStyle = lipgloss.NewStyle().Background(theme.Accent).Foreground(theme.Selection).Bold(true)

	// Detail panel
	DetailHeaderStyle = lipgloss.NewStyle().Bold(true).Padding(0, 1)

	// Overlays
	OverlayStyle = lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(theme.Accent).Padding(1, 2)

	// Title
	TitleStyle          = lipgloss.NewStyle().Bold(true).Foreground(theme.Highlight).Padding(0, 1)
	TitleIndicatorStyle = lipgloss.NewStyle().Foreground(theme.Subtle)
)
