package msgs

import (
	"github.com/aohoyd/aku/internal/render"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// DetailMode is which view is active in the right panel.
type DetailMode int

const (
	DetailYAML DetailMode = iota
	DetailDescribe
	DetailLogs
	DetailValues    // helm get values <release>
	DetailValuesAll // helm get values <release> --all
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

// PortForwardHandle abstracts a running port-forward session so that
// msgs can reference it without importing the k8s package.
type PortForwardHandle interface {
	Stop()
	Ready() <-chan struct{}
	Done() <-chan struct{}
	Err() <-chan error
}

// PortForwardStartedMsg is the async result after starting a port-forward.
type PortForwardStartedMsg struct {
	ID        string
	LocalPort int
	Err       error
	Handle    PortForwardHandle
}

// PortForwardStatusMsg carries a status update for a port-forward entry.
type PortForwardStatusMsg struct {
	ID     string
	Status string
	Err    error
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

// DescribeLoadedMsg carries the result of an async describe operation.
type DescribeLoadedMsg struct {
	Content render.Content
	Events  render.Content
	Gen     uint64
	Err     error
}

// HelmHistoryEntry represents a single Helm release revision.
// Defined here to avoid circular imports with the ui package.
type HelmHistoryEntry struct {
	Revision int
	Display  string
}

// HelmHistoryLoadedMsg carries the result of an async Helm history lookup.
type HelmHistoryLoadedMsg struct {
	ReleaseName string
	Namespace   string
	Entries     []HelmHistoryEntry
	Err         error
}

// HelmReleasesLoadedMsg carries the result of an async Helm releases listing.
type HelmReleasesLoadedMsg struct {
	Namespace string
	Objects   []*unstructured.Unstructured
	Err       error
}

// HelmValuesLoadedMsg carries the result of an async Helm GetValues fetch.
// Mode is DetailValues or DetailValuesAll, identifying which fetch produced this result.
type HelmValuesLoadedMsg struct {
	ReleaseName string
	Namespace   string
	Mode        DetailMode
	Content     render.Content
	Err         error
}

// LogStreamReadyMsg carries the result of an async log stream connection.
type LogStreamReadyMsg struct {
	Ch  <-chan string
	Gen uint64
	Err error
}

// DescribeDebounceFiredMsg is sent after the debounce interval elapses
// following a cursor move in describe mode. Seq is compared against the
// app's counter to discard stale fires.
type DescribeDebounceFiredMsg struct {
	Seq uint64
}

// SearchDebounceFiredMsg is sent after the debounce interval elapses
// following a search input change. Seq is compared against the app's
// counter to discard stale fires.
type SearchDebounceFiredMsg struct {
	Seq     uint64
	Pattern string
	Mode    SearchMode
}

// StatusBarClearErrorMsg signals the status bar to hide the error message.
type StatusBarClearErrorMsg struct{}

// StatusBarClearWarningMsg signals the status bar to hide the warning message.
type StatusBarClearWarningMsg struct{}

// StatusBarShowSpinnerMsg signals the status bar to show the spinner after a delay.
type StatusBarShowSpinnerMsg struct{}

// ClusterHealthMsg carries the result of a cluster health check.
type ClusterHealthMsg struct {
	Online bool
}
