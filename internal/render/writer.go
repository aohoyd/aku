package render

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// ValueKind tags a field value for semantic styling.
type ValueKind int

const (
	ValueDefault ValueKind = iota
	ValueStatusOK
	ValueStatusWarn
	ValueStatusError
	ValueStatusGray
	ValueNumber
)

// FormatDuration formats a duration as a human-readable age string (Xs/Xm/Xh/Xd).
func FormatDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

// FormatAge returns a human-readable age string for an Unstructured object.
func FormatAge(obj *unstructured.Unstructured) string {
	created := obj.GetCreationTimestamp()
	if created.IsZero() {
		return "?"
	}
	return FormatDuration(time.Since(created.Time))
}

// StatusKind maps a pod phase string to a ValueKind for styling.
func StatusKind(phase string) ValueKind {
	switch phase {
	case "Running":
		return ValueStatusOK
	case "Succeeded", "Completed":
		return ValueStatusGray
	case "Pending", "Waiting", "ContainerCreating":
		return ValueStatusWarn
	case "Failed", "CrashLoopBackOff", "Error",
		"ImagePullBackOff", "ErrImagePull",
		"CreateContainerError", "CreateContainerConfigError",
		"InvalidImageName", "RunContainerError",
		"OOMKilled", "Terminating":
		return ValueStatusError
	default:
		if strings.HasPrefix(phase, "Init:") ||
			strings.HasPrefix(phase, "Signal:") ||
			strings.HasPrefix(phase, "ExitCode:") {
			return ValueStatusError
		}
		if phase == "NotReady" {
			return ValueStatusWarn
		}
		return ValueDefault
	}
}

// ConditionKind maps a condition status string to a ValueKind for styling.
func ConditionKind(status string) ValueKind {
	switch status {
	case "True":
		return ValueStatusOK
	case "False":
		return ValueStatusError
	case "Unknown":
		return ValueStatusWarn
	default:
		return ValueDefault
	}
}

// Level constants (each level = 2 spaces indentation).
const (
	LEVEL_0 = iota
	LEVEL_1
	LEVEL_2
	LEVEL_3
	LEVEL_4
)

func writerValueStyleForKind(kind ValueKind) lipgloss.Style {
	switch kind {
	case ValueStatusOK:
		return writerStatusOKStyle
	case ValueStatusWarn:
		return writerStatusWarnStyle
	case ValueStatusError:
		return writerStatusErrStyle
	case ValueStatusGray:
		return writerStatusGrayStyle
	case ValueNumber:
		return writerNumberStyle
	default:
		return writerValueStyle
	}
}
