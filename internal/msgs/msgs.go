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

// GlobalContextSelectedMsg signals the context picker selected a context. It
// retargets the focused pane's context group (the `gx` binding /
// context-picker command).
type GlobalContextSelectedMsg struct {
	Context string
}

// ClusterReadyMsg is returned by the async cluster-connect command once the
// blocking dial for Context has completed (or failed) off the Bubble Tea Update
// goroutine.
//
// Client carries the freshly-dialed *k8s.Client as an opaque handle (typed
// `any` to keep the msgs package free of a k8s import, which would form an
// import cycle). It is an immutable connection handle, not shared mutable
// Manager state, so passing it across the goroutine boundary is safe. On the
// Update goroutine the handler type-asserts it to *k8s.Client and installs it
// via Manager.RegisterConnected — so ALL Manager map/refCount mutation stays on
// the Update goroutine. On a dial failure Client is nil and Err is set; no
// cluster is registered and no ref is taken (so a failed connect leaks nothing).
//
// The handler applies the connected cluster to whichever pane(s) have
// Context()==Context and are awaiting data — identified by pane context, NOT by
// focus, so a focus change between dispatch and arrival cannot drop the update.
type ClusterReadyMsg struct {
	Context string
	Client  any
	Err     error
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

// TermBytesMsg delivers a chunk of shell stdout from a terminal session's
// background read loop. ID identifies the session/pane the bytes belong to.
type TermBytesMsg struct {
	ID   string
	Data []byte
}

// TermExitMsg signals a terminal session ended (shell exit, stream error, or
// cancellation). ID identifies the session/pane; Code is the shell exit code
// and Err the terminal error (nil on clean exit).
type TermExitMsg struct {
	ID   string
	Code int
	Err  error
}

// DebugReadyMsg reports the result of the async debug pre-flight (creating the
// ephemeral container / node-debug pod and waiting for it to be Running). ID is
// the pane/session id allocated up front, so the handler can bind the attach
// session to the placeholder pane already on screen. On Err, the handler
// surfaces the error and tears the placeholder pane down.
type DebugReadyMsg struct {
	ID            string
	PodName       string
	Namespace     string
	ContainerName string
	NodeMode      bool
	Err           error
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
//
// Context names the cluster the check ran against. The status bar shows a single
// online indicator, so only the health of the FOCUSED pane's cluster is
// reflected there; ticks for other (background) clusters are still re-armed so
// every cluster a pane uses keeps being probed, but they do not clobber the
// displayed indicator unless they match the focused pane's context. An empty
// Context is treated as the global cluster (legacy/global-tick).
type ClusterHealthMsg struct {
	Context string
	Online  bool
}
