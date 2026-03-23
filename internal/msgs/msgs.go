package msgs

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// DetailMode is which view is active in the right panel.
type DetailMode int

const (
	DetailYAML     DetailMode = iota
	DetailDescribe
	DetailLogs
)

// NamespaceSelectedMsg signals the namespace picker selected a namespace.
type NamespaceSelectedMsg struct {
	Namespace string
}

// NamespacesLoadedMsg carries the result of async namespace listing.
type NamespacesLoadedMsg struct {
	Namespaces []string
	Err        error
}

// ActionResultMsg is returned after an action completes.
type ActionResultMsg struct {
	ActionID string
	Err      error
}

// ResourcePickedMsg is returned when resource picker submits a command.
type ResourcePickedMsg struct {
	Command string
}

// ConfirmAction represents the user's choice in the confirm dialog.
type ConfirmAction int

const (
	ConfirmCancel ConfirmAction = iota
	ConfirmYes
	ConfirmForce
)

// ConfirmResultMsg is returned from the confirm dialog.
type ConfirmResultMsg struct {
	Action ConfirmAction
}

// ErrMsg wraps any async error for display in the status bar.
type ErrMsg struct {
	Err error
}

func (e ErrMsg) Error() string { return e.Err.Error() }

// WarningMsg carries a Kubernetes API warning for display in the status bar.
type WarningMsg struct {
	Text string
}

// SearchMode controls whether the search bar searches or filters.
type SearchMode int

const (
	SearchModeSearch SearchMode = iota
	SearchModeFilter
)

// SearchChangedMsg fires on each keystroke while the search bar is open.
type SearchChangedMsg struct {
	Pattern string
	Mode    SearchMode
}

// SearchSubmittedMsg fires when the user confirms with Enter.
type SearchSubmittedMsg struct {
	Pattern string
	Mode    SearchMode
}

// SearchClearedMsg fires when the user cancels with Esc.
// Mode indicates which layer to clear.
type SearchClearedMsg struct {
	Mode SearchMode
}

// PortForwardRequestedMsg is emitted by the port-forward overlay when the user confirms.
type PortForwardRequestedMsg struct {
	PodName       string
	PodNamespace  string
	ContainerName string
	LocalPort     int
	RemotePort    int
	Protocol      string
}

// PortForwardStartedMsg is the async result after starting a port-forward.
type PortForwardStartedMsg struct {
	ID        string
	LocalPort int
	Err       error
}

// PortForwardStoppedMsg is emitted when a port-forward is stopped.
type PortForwardStoppedMsg struct {
	ID string
}

// ContainerImageChange represents a container image to change.
type ContainerImageChange struct {
	Name  string
	Image string
	Init  bool
}

// SetImageRequestedMsg is emitted by the set-image overlay when the user confirms.
type SetImageRequestedMsg struct {
	ResourceName string
	Namespace    string
	GVR          schema.GroupVersionResource
	PluginName   string
	Images       []ContainerImageChange
}

// ScaleRequestedMsg is emitted by the scale overlay when the user confirms.
type ScaleRequestedMsg struct {
	ResourceName string
	Namespace    string
	GVR          schema.GroupVersionResource
	Replicas     int32
}

// HelmRollbackRequestedMsg is emitted by the rollback overlay.
type HelmRollbackRequestedMsg struct {
	ReleaseName string
	Namespace   string
	Revision    int
}

// HelmReleasesRefreshMsg signals helm releases splits to refresh.
type HelmReleasesRefreshMsg struct{}

// HelmChartRefSetMsg is emitted when the user sets a chart source for a release.
type HelmChartRefSetMsg struct {
	ReleaseName string
	Namespace   string
	ChartRef    string
}

// LogLineMsg delivers a single log line from the streaming goroutine.
type LogLineMsg struct {
	Line string
	Gen  uint64
}

// LogStreamEndedMsg signals the log stream closed (pod terminated, error, etc.)
type LogStreamEndedMsg struct {
	Err error
	Gen uint64
}

// LogContainerSelectedMsg signals a container was chosen from the picker.
type LogContainerSelectedMsg struct {
	Container string
}

// LogTimeRangeSelectedMsg signals a time range was chosen.
type LogTimeRangeSelectedMsg struct {
	SinceSeconds *int64 // nil means "tail 200 lines" (default)
	Label        string // display label like "5m", "1h"
}

// LogDebounceFiredMsg is sent after the debounce interval elapses
// following a cursor move in log mode. Seq is compared against the
// app's counter to discard stale fires.
type LogDebounceFiredMsg struct {
	Seq uint64
}

