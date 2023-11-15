package render

import (
	"charm.land/lipgloss/v2"
	"github.com/aohoyd/aku/internal/theme"
)

// YAML syntax-highlighting styles (exported).
var (
	YAMLKeyStyle    = lipgloss.NewStyle().Foreground(theme.SyntaxKey)
	YAMLStringStyle = lipgloss.NewStyle().Foreground(theme.SyntaxString)
	YAMLNumberStyle = lipgloss.NewStyle().Foreground(theme.SyntaxNumber)
	YAMLBoolStyle   = lipgloss.NewStyle().Foreground(theme.SyntaxBool)
	YAMLNullStyle   = lipgloss.NewStyle().Foreground(theme.SyntaxNull)
	YAMLMarkerStyle = lipgloss.NewStyle().Foreground(theme.SyntaxMarker)
)

// Describe writer styles (unexported).
var (
	writerKeyStyle        = lipgloss.NewStyle().Foreground(theme.SyntaxKey)
	writerValueStyle      = lipgloss.NewStyle().Foreground(theme.SyntaxValue)
	writerStatusOKStyle   = lipgloss.NewStyle().Foreground(theme.StatusRunning)
	writerStatusWarnStyle = lipgloss.NewStyle().Foreground(theme.StatusPending)
	writerStatusErrStyle  = lipgloss.NewStyle().Foreground(theme.StatusFailed).Bold(true)
	writerStatusGrayStyle = lipgloss.NewStyle().Foreground(theme.StatusSucceeded)
	writerNumberStyle     = lipgloss.NewStyle().Foreground(theme.SyntaxNumber)
)
