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

// RenderStatus returns the phase string with foreground color that is
// compatible with the table selection highlight.
func RenderStatus(phase string) string {
	switch phase {
	case "Running":
		return StyledFg(phase, FgRunning)
	case "Succeeded", "Completed":
		return StyledFg(phase, FgSucceeded)
	case "Pending", "Waiting", "ContainerCreating":
		return StyledFg(phase, FgPending)
	case "Failed", "CrashLoopBackOff", "Error",
		"ImagePullBackOff", "ErrImagePull",
		"CreateContainerError", "CreateContainerConfigError",
		"InvalidImageName", "RunContainerError",
		"OOMKilled", "Terminating":
		return StyledFg(phase, FgFailed)
	default:
		if strings.HasPrefix(phase, "Init:") ||
			strings.HasPrefix(phase, "Signal:") ||
			strings.HasPrefix(phase, "ExitCode:") {
			return StyledFg(phase, FgFailed)
		}
		if phase == "NotReady" {
			return StyledFg(phase, FgPending)
		}
		return phase
	}
}
