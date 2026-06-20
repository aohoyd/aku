package plugin

import (
	"image/color"
	"strings"

	"github.com/aohoyd/aku/internal/theme"
	"github.com/charmbracelet/x/ansi"
)

// Status foreground colors shared by all plugins.
var (
	FgRunning   = theme.StatusRunning
	FgSucceeded = theme.StatusSucceeded
	FgPending   = theme.StatusPending
	FgFailed    = theme.StatusFailed
)

// StyledFg applies a foreground color and resets only the foreground (SGR 39)
// instead of a full reset (SGR 0). This preserves background colors set by
// outer styles such as the table selection highlight.
func StyledFg(text string, c color.Color) string {
	var s ansi.Style
	s = s.ForegroundColor(c)
	return s.String() + text + "\x1b[39m"
}

// IsFailedPhase reports whether phase belongs to the red ("broken") set used by
// both cell coloring and row tinting.
func IsFailedPhase(phase string) bool {
	switch phase {
	case "Failed", "CrashLoopBackOff", "Error",
		"ImagePullBackOff", "ErrImagePull",
		"CreateContainerError", "CreateContainerConfigError",
		"InvalidImageName", "RunContainerError",
		"OOMKilled", "Terminating":
		return true
	}
	return strings.HasPrefix(phase, "Init:") ||
		strings.HasPrefix(phase, "Signal:") ||
		strings.HasPrefix(phase, "ExitCode:")
}

// IsPendingPhase reports whether phase belongs to the yellow ("transitional")
// set used by both cell coloring and row tinting.
func IsPendingPhase(phase string) bool {
	switch phase {
	case "Pending", "Waiting", "ContainerCreating", "NotReady":
		return true
	}
	return false
}

// RenderStatus returns the phase string with foreground color that is
// compatible with the table selection highlight.
func RenderStatus(phase string) string {
	switch {
	case phase == "Running":
		return StyledFg(phase, FgRunning)
	case phase == "Succeeded", phase == "Completed":
		return StyledFg(phase, FgSucceeded)
	case IsPendingPhase(phase):
		return StyledFg(phase, FgPending)
	case IsFailedPhase(phase):
		return StyledFg(phase, FgFailed)
	default:
		return phase
	}
}
