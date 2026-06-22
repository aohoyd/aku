package app

import (
	"context"
	"fmt"
	"image/color"
	"strings"
	"sync"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/aohoyd/aku/internal/cluster"
	"github.com/aohoyd/aku/internal/config"
	"github.com/aohoyd/aku/internal/helm"
	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/k8s/session"
	"github.com/aohoyd/aku/internal/layout"
	"github.com/aohoyd/aku/internal/msgs"
	"github.com/aohoyd/aku/internal/notify"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/plugins/apiresources"
	"github.com/aohoyd/aku/internal/plugins/generic"
	"github.com/aohoyd/aku/internal/plugins/helmreleases"
	"github.com/aohoyd/aku/internal/portforward"
	"github.com/aohoyd/aku/internal/render"
	"github.com/aohoyd/aku/internal/theme"
	"github.com/aohoyd/aku/internal/ui"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	eventsGVR     = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "events"}
	configMapsGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}
	secretsGVR    = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "secrets"}
)

type overlay int

const (
	overlayNone overlay = iota
	overlayResourcePicker
	overlayNsPicker
	overlayConfirm
	overlaySearchBar
	overlayHelp
	overlayPortForward
	overlaySetImage
	overlayHelmRollback
	overlayChartInput
	overlayContainerPicker
	overlayTimeRange
	overlayScale
	overlayContextPicker
)

// pendingDebugAction stores the parameters for a debug command waiting for
// user confirmation. Both node debug and privileged ephemeral container debug
// are dangerous operations that require explicit confirmation.
type pendingDebugAction struct {
	nodeMode      bool     // true for node debug
	privileged    bool     // true for privileged ephemeral container
	nodeName      string   // node name (node mode)
	podName       string   // pod name (pod/container mode)
	containerName string   // container name (pod/container mode)
	namespace     string   // namespace (pod/container mode)
	image         string   // debug image
	command       []string // debug command
}

// terminalMeta records cleanup metadata for an embedded terminal session,
// keyed by session id in App.termCleanup.
//
//   - ephemeral marks a pod-debug (ephemeral container) session. Ephemeral
//     containers cannot be removed from a pod (a k8s limitation), so close/exit
//     only surfaces a one-line note rather than deleting anything.
//   - nodeDebug marks a node-debug session whose created pod (podName/namespace
//     on client) must be deleted on close and on quit (best-effort).
type terminalMeta struct {
	ephemeral bool
	nodeDebug bool
	client    *k8s.Client
	podName   string
	namespace string

	// preflightCancel cancels the in-flight debug pre-flight (GET/patch/create +
	// wait-Running) tied to this placeholder pane. It is set while the pre-flight
	// runs and nil once it lands (handleDebugReady). Closing the placeholder mid-
	// flight calls it so the API calls do not leak up to the 60s wait — and, for
	// node-debug, so PrepareNodeDebug's own ctx-cancel cleanup deletes any pod it
	// already created. nil for exec sessions (no pre-flight).
	preflightCancel context.CancelFunc
}

// nodeDebugDeleteTimeout bounds the best-effort delete of a node-debug pod so a
// slow API server never hangs the UI/quit path.
const nodeDebugDeleteTimeout = 3 * time.Second

// deleteNodeDebugPodAsync fires a best-effort, bounded delete of a node-debug
// pod in its own goroutine so a slow API server never blocks the UI goroutine.
// It is a no-op when client or pod is empty (e.g. a pre-flight that never named
// a pod). Used by closeTerminalSession/handleDebugReady/shutdownTerminals.
func deleteNodeDebugPodAsync(client *k8s.Client, pod, ns string) {
	if client == nil || pod == "" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), nodeDebugDeleteTimeout)
		defer cancel()
		_ = k8s.DeleteNodeDebugPod(ctx, client, pod, ns)
	}()
}

// App is the root Bubbletea model.
type App struct {
	// mgr is the single source of truth for clusters. All client/store/
	// discovery access flows through it via clusterFor / clusterForFocused. Each
	// pane carries a context; the focused pane is the source of truth. Clusters
	// are resolved per pane by the pane's context.
	mgr *cluster.Manager

	// startupContext is the explicit kube-context the initial pane(s) were seeded
	// with at New() (the kubeconfig current-context, resolved by the Manager).
	// clusterFor falls back to it for a pane (or the very first pane) that has no
	// context yet, replacing the removed Manager "global" notion.
	startupContext string

	bindingSet       *config.BindingSet
	keyTrie          *config.KeyTrie
	trieContextType  string
	trieResourceName string
	layout           layout.Layout
	statusBar        ui.StatusBar
	width            int
	height           int

	// envResolved toggles resolved environment variable display in describe view
	envResolved bool

	// Overlay components
	activeOverlay overlay
	// overlayRect is the screen area occupied by the currently active overlay,
	// populated on each render and used by the mouse dispatcher to hit-test
	// wheel/click events.
	//
	// WARNING / intentional pattern: this is a *pointer* field even though App
	// is otherwise a value type. View() is a value-receiver by bubbletea
	// contract, but the rect it computes must survive back to the next Update()
	// call; returning a new App from View() is not an option. The pointer lets
	// setOverlayRect/clearOverlayRect mutate the single shared cell through a
	// value receiver. This is the only side-effecting field in View(); see the
	// setOverlayRect/clearOverlayRect helpers. New() always initializes the
	// pointer to a fresh zero rect — nothing reassigns it at runtime, so
	// nil-checks are not needed on the read path.
	overlayRect         *ui.OverlayRect
	resourcePicker      ui.ResourcePicker
	nsPicker            ui.NsPicker
	contextPicker       ui.ContextPicker
	confirmDialog       ui.ConfirmDialog
	searchBar           ui.SearchBar
	helpOverlay         ui.HelpOverlay
	portForwardOverlay  ui.PortForwardOverlay
	setImageOverlay     ui.SetImageOverlay
	helmRollbackOverlay ui.HelmRollbackOverlay
	chartInputOverlay   ui.ChartInputOverlay
	containerPicker     ui.ContainerPicker
	timeRangePicker     ui.TimeRangePicker
	scaleOverlay        ui.ScaleOverlay

	// Helm
	//
	// chartResolver is the per-app resolver used when building per-cluster helm
	// clients. The helm client itself is built lazily from the focused cluster's
	// rest.Config (see helmClientFor) — helm.NewClient is cheap (it only wraps a
	// config), so it is rebuilt per call rather than cached.
	//
	// helmClient, when non-nil, overrides that lazy build. It exists so tests
	// can inject a stub helm.Client without bringing up a real rest.Config.
	// Production leaves it nil and relies on helmClientFor.
	chartResolver helm.ChartResolver
	helmClient    helm.Client

	// Port-forward
	pfRegistry *portforward.Registry
	pfHandles  map[string]msgs.PortForwardHandle

	// Embedded terminals. terminals maps a session id to its live background
	// SPDY session; the matching *ui.TerminalPane lives in layout.splits (the
	// single source of truth for panes, looked up via layout.TerminalPaneByID).
	// termSeq generates unique session ids.
	terminals map[string]*session.Terminal
	termSeq   uint64

	// attachExecutorFn builds the SPDY executor a ready debug session attaches
	// to. It is a seam: production wires it to k8s.NewAttachExecutor; tests
	// substitute a fake so the handleDebugReady success/error paths can be
	// exercised without a real cluster Config. Never nil after New.
	attachExecutorFn func(client *k8s.Client, podName, containerName, namespace string) (session.Executor, error)

	// execExecutorFn builds the SPDY executor an exec terminal pane streams
	// through. Like attachExecutorFn it is a seam: production wires it to
	// k8s.NewExecExecutor; tests substitute a fake so openExecTerminal can be
	// driven end-to-end without a real cluster Config. Never nil after New.
	execExecutorFn func(client *k8s.Client, podName, containerName, namespace string, command []string) (session.Executor, error)

	// termCleanup carries per-terminal lifecycle metadata needed to clean up
	// remote resources when the pane closes or the app quits. Keyed by the same
	// session id as terminals. Exec/pod-debug entries are mostly informational
	// (ephemeral set); node-debug entries carry the created pod so it can be
	// deleted. Entries are removed alongside the session in closeTerminalSession.
	termCleanup       map[string]terminalMeta
	config            *config.Config

	// notify is the shared store of aku's own info/warning/error messages. It
	// backs both the toast overlay (rendered each frame in View via Live) and the
	// aku-messages synthetic resource. Injected at New; may be nil on some
	// construction paths (notably tests), so every read path guards against nil.
	notify *notify.Store
	// toasts is the pure renderer for the top-right toast stack. It holds no
	// state: View feeds it the currently-visible messages each frame.
	toasts ui.ToastStack
	// dismissed records message IDs that have expired (per-level TTL tick) or been
	// explicitly cleared (clear-notifications). The visible toast set is
	// notify.Live(now, toastTTL) minus these IDs. The store's history is never
	// mutated — dismissal is purely an overlay concern.
	dismissed map[uint64]bool
	pendingRun        *config.RunConfig            // external command waiting for confirm
	pendingDelete     []*unstructured.Unstructured // delete targets (always-list) waiting for confirm
	pendingDebug      *pendingDebugAction          // debug action waiting for confirm
	pendingRestart    []k8s.RestartTarget          // rollout restart targets waiting for confirm
	pendingRestartGVR schema.GroupVersionResource  // GVR matching pendingRestart

	// Log stream
	logStreamCancel context.CancelFunc
	logCh           <-chan string
	logDebounceSeq  uint64
	logStreamGen    uint64
	searchApplySeq  uint64

	describeGen         uint64
	describeDebounceSeq uint64
	lastDetailKey       string

	// Mouse double-click detection. `now` is injected so tests can advance a
	// virtual clock; defaults to time.Now in New(). `lastClick*` record the
	// previous click used to decide whether the current click is a double.
	now            func() time.Time
	lastClickTime  time.Time
	lastClickKind  layout.PaneKind
	lastClickRow   int // body-row index under the last click (-1 if chrome)
	lastClickSplit int // split index for the last click (-1 if not a split)

	// executeCommandFn is a test seam for intercepting command dispatch from
	// the mouse double-click handler. When nil, (App).executeCommand is
	// used. Tests that supply a spy receive the current App so state
	// mutations made by handleMouseClick (e.g. resetting lastClickTime)
	// propagate back into the returned model.
	//
	// Why a seam rather than observing the real side effects: the
	// "enter-detail" command branches on split kind (resources, details,
	// logs) and for some plugins triggers async drill-down work that
	// depends on a live k8s client or store subscription. Mocking those
	// dependencies just to observe "did Enter fire" would require wiring
	// a full fake store per double-click test. The seam narrows the
	// assertion to what's actually under test — the double-click decision
	// — leaving the command-dispatch path covered by its own tests.
	executeCommandFn func(a App, cmd string) (tea.Model, tea.Cmd)
}

// ResourceSpec describes a resource pane to open at startup.
type ResourceSpec struct {
	Plugin    plugin.ResourcePlugin
	Namespace string
}

// New creates a new App with all dependencies. The Manager is the single source
// of client/store/discovery; the App resolves them per pane via clusterFor /
// clusterForFocused. chartResolver is used to build per-cluster helm clients
// lazily (see helmClientFor).
func New(mgr *cluster.Manager, keymap *config.Keymap, cfg *config.Config, notifyStore *notify.Store, pfRegistry *portforward.Registry, chartResolver helm.ChartResolver, specs []ResourceSpec, initialDetail *msgs.DetailMode, initialOrientation layout.Orientation, startupContext string) App {
	bs := keymap.BindingSet()
	defaultTimeRange := cfg.LogDefaultTimeRange()
	defaultSinceSeconds, ok := ui.LookupTimePreset(defaultTimeRange)
	if !ok {
		defaultTimeRange = "15m"
		defaultSinceSeconds = 900
	}

	// Resolve the startup cluster up front (may be nil/degraded if connect
	// failed at startup). Everything below treats a nil client the same way the
	// old single-client path did.
	startup, _ := mgr.Get(startupContext)
	var client *k8s.Client
	var startupStore *k8s.Store
	if startup != nil {
		client = startup.Client()
		startupStore = startup.Store()
	}

	// Every error/warning/info now flows through the notify store (it backs both
	// the toast overlay and the aku-messages resource). Default a nil store to a
	// fresh one so call sites can append unconditionally; the send callback stays
	// nil until root wires it, so Add records history without dispatching toasts.
	if notifyStore == nil {
		notifyStore = notify.NewStore(0)
	}

	a := App{
		mgr:                 mgr,
		startupContext:      startupContext,
		bindingSet:          bs,
		keyTrie:             bs.TrieFor("resources", ""),
		layout:              layout.New(80, 24, cfg.LogBufferSize(), defaultTimeRange, defaultSinceSeconds),
		statusBar:           ui.NewStatusBar(80),
		resourcePicker:      ui.NewResourcePicker(40, 20),
		nsPicker:            ui.NewNsPicker(40, 20),
		contextPicker:       ui.NewContextPicker(40, 20),
		searchBar:           ui.NewSearchBar(80),
		helpOverlay:         ui.NewHelpOverlay(80, 24),
		chartResolver:       chartResolver,
		pfRegistry:          pfRegistry,
		pfHandles:           make(map[string]msgs.PortForwardHandle),
		terminals:           make(map[string]*session.Terminal),
		termCleanup:         make(map[string]terminalMeta),
		portForwardOverlay:  ui.NewPortForwardOverlay(40, 20),
		setImageOverlay:     ui.NewSetImageOverlay(40, 20),
		helmRollbackOverlay: ui.NewHelmRollbackOverlay(40, 20),
		config:              cfg,
		notify:              notifyStore,
		toasts:              ui.NewToastStack(cfg.NotifyMaxVisible()),
		dismissed:           make(map[uint64]bool),
		chartInputOverlay:   ui.NewChartInputOverlay(40, 20),
		containerPicker:     ui.NewContainerPicker(40, 20),
		timeRangePicker:     ui.NewTimeRangePicker(40, 20),
		scaleOverlay:        ui.NewScaleOverlay(40, 20),
		overlayRect:         &ui.OverlayRect{},
		now:                 time.Now,
		lastClickRow:        -1,
		lastClickSplit:      -1,
		// Executor seams default to the real SPDY factories. Tests override these
		// to drive handleDebugReady / openExecTerminal without a live cluster. They
		// reference only package functions (no post-construction App state), so they
		// belong in the struct literal alongside the other field defaults.
		attachExecutorFn: func(client *k8s.Client, podName, containerName, namespace string) (session.Executor, error) {
			return k8s.NewAttachExecutor(client, podName, containerName, namespace)
		},
		execExecutorFn: func(client *k8s.Client, podName, containerName, namespace string, command []string) (session.Executor, error) {
			return k8s.NewExecExecutor(client, podName, containerName, namespace, command)
		},
	}

	if initialOrientation == layout.OrientationHorizontal {
		a.layout.ToggleOrientation()
	}

	if client != nil {
		a.statusBar.SetContextName(client.Context)
	}

	// Populate fuzzy picker with all registered plugins
	a.resourcePicker.SetPlugins(buildPickerEntries())

	// Determine the default namespace
	defaultNs := "default"
	if client != nil {
		defaultNs = client.Namespace
	}

	// Add initial splits from specs. Each split is stamped with the explicit
	// startup context so informer updates (cluster-tagged via msg.Context) match
	// the pane, and so layout.UpdateSplitObjects can match on context.

	// If no specs provided, default to a single pods pane
	if len(specs) == 0 {
		if p, ok := plugin.ByName("pods"); ok {
			specs = []ResourceSpec{{Plugin: p, Namespace: defaultNs}}
		}
	}

	for i, spec := range specs {
		ns := spec.Namespace
		if ns == "" {
			ns = defaultNs
		}
		a.layout.AddSplit(spec.Plugin, ns, startupContext)
		if startupStore != nil {
			startupStore.Subscribe(spec.Plugin.GVR(), ns)
		}
		if i == 0 {
			a.keyTrie = bs.TrieFor("resources", spec.Plugin.Name())
		}
	}

	// Re-focus first split (AddSplit focuses each newly added split)
	if len(specs) > 1 {
		a.layout.FocusSplitAt(0)
	}

	// Open detail panel if requested via --details flag
	if initialDetail != nil {
		if *initialDetail == msgs.DetailLogs {
			a.layout.SetLogMode(true)
			a.layout.ShowRightPanel()
		} else {
			a.layout.ShowRightPanel()
			a.layout.RightPanel().SetMode(*initialDetail)
		}
	}

	// Set initial status bar hints
	a.statusBar.SetHints(a.currentHints())

	return a
}

// clusterFor resolves the cluster backing a given pane. The pane's context is
// authoritative (panes are seeded with a concrete context at creation); a nil
// pane resolves to the focused pane's context, and ultimately to the startup
// context (the kubeconfig current-context the first pane was seeded with). If
// the resolved context has no created cluster yet (e.g. an async connect that
// has not landed) the lookup returns a typed-nil *Cluster, which is
// nil-receiver-safe; callers still guard with Connected()/nil-client checks for
// degraded clusters.
func (a App) clusterFor(rl *ui.ResourceList) *cluster.Cluster {
	ctx := a.contextFor(rl)
	c, _ := a.mgr.Get(ctx)
	return c
}

// contextFor resolves the kube-context for a pane. Panes are seeded with a
// concrete context at creation (layout.AddSplit), so a non-nil pane is
// authoritative. A nil pane means "the current context": the focused pane's,
// else the startup context (when no pane is focused yet).
func (a App) contextFor(rl *ui.ResourceList) string {
	if rl != nil {
		return rl.Context()
	}
	if f := a.layout.FocusedSplit(); f != nil {
		return f.Context()
	}
	return a.startupContext
}

// clusterForFocused resolves the cluster backing the focused split, falling back
// to the startup context when there is no focused split.
func (a App) clusterForFocused() *cluster.Cluster {
	return a.clusterFor(a.layout.FocusedSplit())
}

// storeFor returns the informer store for the cluster backing rl, or nil if that
// cluster is absent/degraded.
func (a App) storeFor(rl *ui.ResourceList) *k8s.Store {
	cl := a.clusterFor(rl)
	if cl == nil {
		return nil
	}
	return cl.Store()
}

// storeForFocused returns the informer store for the focused split's cluster.
func (a App) storeForFocused() *k8s.Store {
	return a.storeFor(a.layout.FocusedSplit())
}

// clientForFocused returns the k8s client for the focused split's cluster, or
// nil if that cluster is absent/degraded.
func (a App) clientForFocused() *k8s.Client {
	cl := a.clusterForFocused()
	if cl == nil {
		return nil
	}
	return cl.Client()
}

// helmClientFor returns the helm client for the given cluster. When a.helmClient
// is set (test override) it is returned directly. Otherwise a client is built
// lazily from the cluster's rest.Config — helm.NewClient merely wraps the
// config, so rebuilding per call is cheap and keeps the client pinned to the
// right cluster. Returns nil when the cluster is degraded or has no config.
func (a App) helmClientFor(cl *cluster.Cluster) helm.Client {
	if a.helmClient != nil {
		return a.helmClient
	}
	if cl == nil || !cl.Connected() {
		return nil
	}
	client := cl.Client()
	if client == nil || client.Config == nil {
		return nil
	}
	return helm.NewClient(client.Config, a.chartResolver)
}

// helmClientForFocused returns the helm client for the focused split's cluster.
func (a App) helmClientForFocused() helm.Client {
	return a.helmClientFor(a.clusterForFocused())
}

func (a App) Init() tea.Cmd {
	startup, _ := a.mgr.Get(a.startupContext)
	if startup == nil || !startup.Connected() {
		return nil
	}
	disc := startup.Discovery()
	client := startup.Client()
	typed := client.Typed
	// disc.Refresh populates the startup cluster's own Discovery index directly;
	// the message is tagged with the startup context so handleAPIResourcesDiscovered
	// only feeds the plugin registry from the startup cluster's results.
	startupCtx := startup.Context()
	return tea.Batch(
		func() tea.Msg {
			resources, err := disc.Refresh(typed)
			return k8s.APIResourcesDiscoveredMsg{Context: startupCtx, Resources: resources, Err: err}
		},
		initialHeartbeatCmd(startupCtx, client),
	)
}

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	model, cmd := a.update(msg)
	app := model.(App)
	app.syncInlineSearch()
	return app, cmd
}

func (a App) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height
		a.statusBar.SetWidth(msg.Width)
		a.layout.Resize(msg.Width, msg.Height)
		overlayW := min(msg.Width-10, 80)
		overlayH := min(msg.Height-6, 30)
		a.resourcePicker.SetSize(overlayW, overlayH)
		a.searchBar.SetWidth(msg.Width)
		a.nsPicker.SetSize(overlayW, overlayH)
		a.contextPicker.SetSize(overlayW, overlayH)
		a.portForwardOverlay.SetSize(overlayW, overlayH)
		a.setImageOverlay.SetSize(overlayW, overlayH)
		a.helmRollbackOverlay.SetSize(overlayW, overlayH)
		a.chartInputOverlay.SetSize(overlayW, overlayH)
		a.containerPicker.SetSize(overlayW, overlayH)
		a.timeRangePicker.SetSize(overlayW, overlayH)
		a.scaleOverlay.SetSize(overlayW, overlayH)
		a.helpOverlay.SetSize(msg.Width, msg.Height)
		if a.activeOverlay == overlayConfirm {
			a.confirmDialog.SetWidth(msg.Width)
		}
		a.syncTerminalSizes()
		return a, nil

	case tea.KeyPressMsg:
		// Terminal interception: when a live (non-exited) terminal pane is
		// focused and no overlay is open, the pane's prefix input machine owns
		// most keys — including ctrl+c, which must reach the shell rather than
		// quit the app (the prefix is the escape hatch). Captured keys like
		// alt+z / shift+arrows fall through to the trie. An exited terminal pane
		// is a normal closeable split and falls through to the trie. This is
		// placed before the global ctrl+c/ctrl+w handling on purpose.
		if a.activeOverlay == overlayNone {
			if tp, ok := a.layout.FocusedPane().(*ui.TerminalPane); ok && !tp.Exited() {
				if handled, model, cmd := a.routeTerminalKey(tp, msg); handled {
					return model, cmd
				}
			}
		}

		// Ctrl-c always quits immediately. Sweep terminal sessions / node-debug
		// pods first (best-effort, bounded) so quit does not leak remote state.
		if msg.String() == "ctrl+c" {
			a.shutdownTerminals()
			return a, tea.Quit
		}

		// ctrl+w closes current panel / overlay
		if msg.String() == "ctrl+w" {
			if a.activeOverlay != overlayNone {
				a.activeOverlay = overlayNone
				a.pendingRun = nil
				a.pendingDelete = nil
				a.pendingDebug = nil
				a.pendingRestart = nil
				a.pendingRestartGVR = schema.GroupVersionResource{}
				return a, nil
			}
			// Fall through to handleKey → executeCommand for structural panels
		}

		switch a.activeOverlay {
		case overlayHelp:
			updated, cmd := a.helpOverlay.Update(msg)
			a.helpOverlay = updated
			if !a.helpOverlay.Active() {
				a.activeOverlay = overlayNone
			}
			return a, cmd
		case overlaySearchBar:
			updated, cmd := a.searchBar.Update(msg)
			a.searchBar = updated
			if !a.searchBar.Active() {
				a.activeOverlay = overlayNone
			}
			return a, cmd
		case overlayPortForward:
			updated, cmd := a.portForwardOverlay.Update(msg)
			a.portForwardOverlay = updated
			if !a.portForwardOverlay.Active() {
				a.activeOverlay = overlayNone
			}
			return a, cmd
		case overlaySetImage:
			updated, cmd := a.setImageOverlay.Update(msg)
			a.setImageOverlay = updated
			if !a.setImageOverlay.Active() {
				a.activeOverlay = overlayNone
			}
			return a, cmd
		case overlayHelmRollback:
			updated, cmd := a.helmRollbackOverlay.Update(msg)
			a.helmRollbackOverlay = updated
			if !a.helmRollbackOverlay.Active() {
				a.activeOverlay = overlayNone
			}
			return a, cmd
		case overlayChartInput:
			updated, cmd := a.chartInputOverlay.Update(msg)
			a.chartInputOverlay = updated
			if !a.chartInputOverlay.Active() {
				a.activeOverlay = overlayNone
			}
			return a, cmd
		case overlayResourcePicker:
			updated, cmd := a.resourcePicker.Update(msg)
			a.resourcePicker = updated
			if !a.resourcePicker.Active() {
				a.activeOverlay = overlayNone
			}
			return a, cmd
		case overlayNsPicker:
			updated, cmd := a.nsPicker.Update(msg)
			a.nsPicker = updated
			if !a.nsPicker.Active() {
				a.activeOverlay = overlayNone
			}
			return a, cmd
		case overlayContextPicker:
			updated, cmd := a.contextPicker.Update(msg)
			a.contextPicker = updated
			if !a.contextPicker.Active() {
				a.activeOverlay = overlayNone
			}
			return a, cmd
		case overlayConfirm:
			updated, cmd := a.confirmDialog.Update(msg)
			a.confirmDialog = updated
			if !a.confirmDialog.Active() {
				a.activeOverlay = overlayNone
			}
			return a, cmd
		case overlayContainerPicker:
			updated, cmd := a.containerPicker.Update(msg)
			a.containerPicker = updated
			if !a.containerPicker.Active() {
				a.activeOverlay = overlayNone
			}
			return a, cmd
		case overlayTimeRange:
			updated, cmd := a.timeRangePicker.Update(msg)
			a.timeRangePicker = updated
			if !a.timeRangePicker.Active() {
				a.activeOverlay = overlayNone
			}
			return a, cmd
		case overlayScale:
			updated, cmd := a.scaleOverlay.Update(msg)
			a.scaleOverlay = updated
			if !a.scaleOverlay.Active() {
				a.activeOverlay = overlayNone
			}
			return a, cmd
		}
		return a.handleKey(msg)

	case tea.MouseClickMsg:
		return a.handleMouseClick(msg)

	case tea.MouseWheelMsg:
		return a.handleMouseWheel(msg)

	case msgs.ResourcePickedMsg:
		// Handle resource picker submissions
		return a.handleResourcePickerCommand(msg.Command)

	case msgs.NamespaceSelectedMsg:
		return a.handleNamespaceSwitch(msg.Namespace)

	case msgs.GlobalContextSelectedMsg:
		m, cmd := a.handleGroupContextSwitch(msg.Context)
		// The global changed: panes that now match it lose their footer; panes
		// still on a different context keep/gain it. Sync on the returned model.
		if app, ok := m.(App); ok {
			app.syncPaneFooters()
			return app, cmd
		}
		return m, cmd

	case msgs.ClusterReadyMsg:
		return a.handleClusterReady(msg)

	case msgs.NamespacesLoadedMsg:
		a.statusBar.EndOperation()
		if msg.Err != nil {
			a.notify.Add(notify.LevelError, "namespaces: "+msg.Err.Error(), a.contextFor(nil), "namespaces")
			return a, nil
		} else if a.activeOverlay == overlayNsPicker {
			a.nsPicker.SetNamespaces(msg.Namespaces)
		}
		return a, nil

	case msgs.ConfirmResultMsg:
		a.activeOverlay = overlayNone
		if a.pendingRestart != nil {
			targets := a.pendingRestart
			gvr := a.pendingRestartGVR
			a.pendingRestart = nil
			a.pendingRestartGVR = schema.GroupVersionResource{}
			if msg.Action == msgs.ConfirmYes || msg.Action == msgs.ConfirmForce {
				if focused := a.layout.FocusedSplit(); focused != nil {
					focused.ClearSelection()
				}
				if cl := a.clusterForFocused(); cl != nil && cl.Connected() {
					return a, k8s.RestartCmd(cl.Client().Dynamic, gvr, targets)
				}
			}
			return a, nil
		}
		if a.pendingDelete != nil {
			objs := a.pendingDelete
			a.pendingDelete = nil
			if msg.Action == msgs.ConfirmYes || msg.Action == msgs.ConfirmForce {
				if focused := a.layout.FocusedSplit(); focused != nil {
					focused.ClearSelection()
				}
				return a.executeDelete(objs, msg.Action == msgs.ConfirmForce)
			}
			return a, nil
		}
		if a.pendingRun != nil {
			run := a.pendingRun
			a.pendingRun = nil
			if msg.Action == msgs.ConfirmYes || msg.Action == msgs.ConfirmForce {
				return a.executeRun(run)
			}
			return a, nil
		}
		if a.pendingDebug != nil {
			dbg := a.pendingDebug
			a.pendingDebug = nil
			if msg.Action == msgs.ConfirmYes || msg.Action == msgs.ConfirmForce {
				return a.executePendingDebug(dbg)
			}
			return a, nil
		}
		return a, nil

	case msgs.ActionResultMsg:
		if msg.Err != nil {
			a.notify.Add(notify.LevelError, msg.Err.Error(), a.contextFor(nil), actionSource(msg.ActionID))
		} else if note := actionSuccessNote(msg.ActionID); note != "" {
			a.notify.Add(notify.LevelInfo, note, a.contextFor(nil), actionSource(msg.ActionID))
		}
		if strings.HasPrefix(msg.ActionID, "helm-") {
			model, helmCmd := a.refreshHelmSplits()
			// After an `e`-edit upgrade (helm-edit-values:<release>), the right
			// panel may still be showing the previous values. If we're focused
			// on a helmrelease and the panel is in YAML mode with a non-Manifest
			// variant, also re-fetch values so the panel reflects the upgrade.
			if strings.HasPrefix(msg.ActionID, "helm-edit-values:") {
				app := model.(App)
				return app.maybeRefetchValuesAfterEdit(nil, helmCmd)
			}
			return model, helmCmd
		}
		return a, nil

	case k8s.ResourceUpdatedMsg:
		// Find plugin by GVR and update matching splits. List from the store of
		// the cluster that produced the update (identified by msg.Context),
		// falling back to the startup cluster when the message carries no context.
		// Pass msg.Context so only panes on that cluster are repainted.
		if p, ok := plugin.ByGVR(msg.GVR); ok {
			var srcStore *k8s.Store
			lookup := msg.Context
			if lookup == "" {
				lookup = a.startupContext
			}
			if c, ok := a.mgr.Get(lookup); ok {
				srcStore = c.Store()
			}
			if srcStore != nil {
				objs := srcStore.List(msg.GVR, msg.Namespace)
				a.layout.UpdateSplitObjects(p, msg.Namespace, msg.Context, objs)
			}
		}
		// Refresh drill-down child views when relevant resources update
		a.refreshDrillDownSplits(msg.GVR, msg.Namespace)
		// Detect resource identity change at cursor and refresh immediately.
		// Gated on a focused resource list: when a terminal pane is focused
		// FocusedSplit() is nil and detailKey() is "", which would otherwise be
		// read as "selection went away" and blank the panel on the next informer
		// tick. Skipping the block leaves the detail panel frozen on the last
		// resource (and lastDetailKey intact) until focus returns to a resource.
		if a.layout.RightPanelVisible() && a.layout.FocusedSplit() != nil {
			newKey := a.detailKey()
			if newKey != a.lastDetailKey {
				if newKey == "" {
					if a.layout.IsLogMode() {
						a = a.stopLogStream()
						if lv := a.layout.LogView(); lv != nil {
							lv.ClearAndRestart()
						}
					} else if panel := a.layout.RightPanel(); panel != nil {
						panel.ClearContent()
					}
					a.lastDetailKey = ""
					return a, nil
				}
				a.lastDetailKey = newKey
				if a.layout.IsLogMode() {
					return a.refreshDetailPanelOrLog()
				}
				a, cmd := a.refreshDetailPanel()
				return a, cmd
			}
		}
		// Freeze automatic detail panel updates while a search/filter is
		// active on either the resource list or the detail panel.
		// Manual refresh (ctrl+r) still works.
		listSearchActive := false
		if focused := a.layout.FocusedSplit(); focused != nil {
			listSearchActive = focused.AnyActive()
		}
		detailSearchActive := false
		if a.layout.RightPanelVisible() {
			if a.layout.IsLogMode() {
				detailSearchActive = a.layout.LogView().AnyActive()
			} else {
				detailSearchActive = a.layout.RightPanel().AnyActive()
			}
		}
		if !listSearchActive && !detailSearchActive {
			if !a.layout.IsLogMode() {
				if panel := a.layout.RightPanel(); panel != nil {
					if panel.Mode() == msgs.DetailDescribe {
						if focused := a.layout.FocusedSplit(); focused != nil {
							gvrMatch := msg.GVR == focused.Plugin().GVR()
							evtMatch := msg.GVR == eventsGVR
							envMatch := a.envResolved && (msg.GVR == configMapsGVR || msg.GVR == secretsGVR)
							if gvrMatch || evtMatch || envMatch {
								a.describeDebounceSeq++
								return a, a.describeDebounceCmd()
							}
						}
						return a, nil // unrelated GVR — skip describe reload
					}
					// YAML mode: only reload for matching GVR
					if focused := a.layout.FocusedSplit(); focused != nil && msg.GVR == focused.Plugin().GVR() {
						var descCmd tea.Cmd
						a, descCmd = a.reloadDetailPanel()
						return a, descCmd
					}
					return a, nil // unrelated GVR — skip YAML reload
				}
			}
			// Log mode or no panel: existing path
			var descCmd tea.Cmd
			a, descCmd = a.reloadDetailPanel()
			// Start log stream if log mode is waiting for objects
			if a.layout.IsLogMode() && a.logCh == nil {
				var logCmd tea.Cmd
				a, logCmd = a.syncLogPanel()
				return a, tea.Batch(descCmd, logCmd)
			}
			return a, descCmd
		}
		// Start log stream if log mode is waiting for objects
		if a.layout.IsLogMode() && a.logCh == nil {
			var cmd tea.Cmd
			a, cmd = a.syncLogPanel()
			if cmd != nil {
				return a, cmd
			}
		}
		return a, nil

	case k8s.APIResourcesDiscoveredMsg:
		return a.handleAPIResourcesDiscovered(msg)

	case msgs.SearchSubmittedMsg:
		return a.handleSearchSubmitted(msg)
	case msgs.SearchChangedMsg:
		return a.handleSearchChanged(msg)
	case msgs.SearchApplyMsg:
		return a.handleSearchApply(msg)
	case msgs.SearchClearedMsg:
		return a.handleSearchCleared(msg)

	case msgs.PortForwardRequestedMsg:
		return a.handlePortForwardRequested(msg)

	case msgs.SetImageRequestedMsg:
		return a.handleSetImageRequested(msg)

	case msgs.ScaleRequestedMsg:
		return a.handleScaleRequested(msg)

	case msgs.HelmRollbackRequestedMsg:
		return a.handleHelmRollback(msg)

	case msgs.HelmHistoryLoadedMsg:
		a.statusBar.EndOperation()
		if a.activeOverlay != overlayHelmRollback {
			return a, nil // overlay was dismissed, discard
		}
		if msg.Err != nil {
			a.helmRollbackOverlay.SetError("helm history: " + msg.Err.Error())
			return a, nil
		}
		entries := make([]ui.HelmRevisionEntry, len(msg.Entries))
		for i, e := range msg.Entries {
			entries[i] = ui.HelmRevisionEntry{Revision: e.Revision, Display: e.Display}
		}
		a.helmRollbackOverlay.SetRevisions(entries)
		return a, nil

	case msgs.HelmReleasesLoadedMsg:
		a.statusBar.EndOperation()
		if msg.Err != nil {
			a.notify.Add(notify.LevelError, "helm releases: "+msg.Err.Error(), a.contextFor(nil), "helm")
			return a, nil
		}
		for i := range a.layout.SplitCount() {
			split := a.layout.SplitAt(i)
			if split != nil && split.Plugin().Name() == "helmreleases" && split.Namespace() == msg.Namespace {
				split.SetObjects(msg.Objects)
			}
		}
		return a, nil

	case msgs.HelmValuesLoadedMsg:
		return a.handleHelmValuesLoaded(msg)

	case msgs.HelmChartRefSetMsg:
		return a.handleHelmChartRefSet(msg)

	case msgs.HelmReleasesRefreshMsg:
		return a.refreshHelmSplits()

	case msgs.PortForwardStartedMsg:
		if msg.Err != nil {
			a.notify.Add(notify.LevelError, "port-forward failed: "+msg.Err.Error(), a.contextFor(nil), "port-forward")
			return a, nil
		}
		a.notify.Add(notify.LevelInfo, fmt.Sprintf("port-forward starting: localhost:%d", msg.LocalPort), a.contextFor(nil), "port-forward")
		a = a.syncIndicators()
		a = a.refreshSelfPopulatingSplits("portforwards")
		// Store the handle in the main Update loop (single-threaded) to avoid data race.
		if msg.Handle != nil {
			a.pfHandles[msg.ID] = msg.Handle
			return a, watchPortForwardReady(msg.ID, msg.Handle)
		}
		return a, nil

	case msgs.PortForwardStatusMsg:
		a.pfRegistry.UpdateStatus(msg.ID, msg.Status)
		switch msg.Status {
		case portforward.StatusReady:
			a.notify.Add(notify.LevelInfo, fmt.Sprintf("port-forward ready: localhost:%d", a.localPortForPF(msg.ID)), a.contextFor(nil), "port-forward")
			var cmd tea.Cmd
			if apf, ok := a.pfHandles[msg.ID]; ok {
				cmd = watchPortForwardDone(msg.ID, apf)
			}
			a = a.syncIndicators()
			a = a.refreshSelfPopulatingSplits("portforwards")
			return a, cmd
		case portforward.StatusError:
			errMsg := "port-forward error"
			if msg.Err != nil {
				errMsg = "port-forward error: " + msg.Err.Error()
			}
			a.notify.Add(notify.LevelError, errMsg, a.contextFor(nil), "port-forward")
			a.pfRegistry.Remove(msg.ID)
			delete(a.pfHandles, msg.ID)
			a = a.syncIndicators()
			a = a.refreshSelfPopulatingSplits("portforwards")
			return a, nil
		case portforward.StatusStopped:
			a.pfRegistry.Remove(msg.ID)
			delete(a.pfHandles, msg.ID)
		}
		a = a.syncIndicators()
		a = a.refreshSelfPopulatingSplits("portforwards")
		return a, nil

	case msgs.MessageAddedMsg:
		// A new message landed in the notify store. The toast appears
		// automatically because View reads notify.Live each frame and this msg
		// forces a re-render; we additionally (1) re-poll any open aku-messages
		// splits so the synthetic resource shows the new row, and (2) arm a
		// per-toast auto-expiry tick for non-sticky levels. A sticky level
		// (ttl == 0) gets no tick and lingers until clear-notifications.
		a = a.refreshSelfPopulatingSplits("aku-messages")
		ttl := a.toastTTL(notify.Level(msg.Level))
		if ttl == 0 {
			// Sticky: no auto-expiry tick.
			return a, nil
		}
		// The message was stamped with its creation time when Add ran, which is
		// slightly before this handler runs. Arm the tick for the time remaining
		// from CREATION, not from arrival, so a backlog of queued MessageAddedMsg
		// does not extend each toast's lifetime. Look the message up by ID in the
		// store's current window.
		created, found := a.messageCreatedAt(msg.ID)
		if !found {
			// Already evicted from the ring before we could arm a tick: treat as
			// expired so it never lingers in the visible set.
			if a.dismissed != nil {
				a.dismissed[msg.ID] = true
			}
			a.pruneDismissed()
			return a, nil
		}
		remaining := ttl - a.now().Sub(created)
		if remaining <= 0 {
			// Created longer than ttl ago (e.g. a delayed MessageAddedMsg): it is
			// already past its lifetime, so dismiss immediately with no tick.
			if a.dismissed != nil {
				a.dismissed[msg.ID] = true
			}
			a.pruneDismissed()
			return a, nil
		}
		id := msg.ID
		return a, tea.Tick(remaining, func(time.Time) tea.Msg {
			return msgs.ToastExpiredMsg{ID: id}
		})

	case msgs.ToastExpiredMsg:
		// The level's TTL elapsed for this toast: drop it from the visible set.
		// The store's history is untouched (it still shows in aku-messages).
		if a.dismissed != nil {
			a.dismissed[msg.ID] = true
		}
		a.pruneDismissed()
		return a, nil

	case msgs.ClearNotificationsMsg:
		// Dismiss every currently-live toast at once (e.g. user keybinding). Mark
		// each live ID dismissed; history in the store is left intact.
		if a.notify != nil && a.dismissed != nil {
			for _, m := range a.notify.Live(a.now(), a.toastTTL) {
				a.dismissed[m.ID] = true
			}
		}
		a.pruneDismissed()
		return a, nil

	case msgs.WarningMsg:
		// WarningMsg carries the originating cluster context (set by the per-client
		// k8s warning handler), so warnings land tagged with the cluster that
		// produced them. Context may be "" when the producing client could not
		// resolve a context name.
		a.notify.Add(notify.LevelWarning, msg.Text, msg.Context, "k8s")
		return a, nil

	case msgs.ErrMsg:
		a.notify.Add(notify.LevelError, msg.Error(), a.contextFor(nil), "k8s")
		return a, nil

	case msgs.LogLineMsg:
		if msg.Gen != a.logStreamGen {
			return a, nil
		}
		if panel := a.layout.LogView(); panel != nil && a.layout.IsLogMode() {
			panel.AppendLine(msg.Line)
		}
		if a.logCh != nil {
			return a, readLogLine(a.logCh, a.logStreamGen)
		}
		return a, nil

	case msgs.LogStreamEndedMsg:
		if msg.Gen != a.logStreamGen {
			return a, nil
		}
		if msg.Err != nil {
			a.notify.Add(notify.LevelError, "log stream: "+msg.Err.Error(), a.contextFor(nil), "logs")
		}
		a.logCh = nil
		return a, nil

	case msgs.LogStreamReadyMsg:
		a.statusBar.EndOperation()
		if msg.Gen != a.logStreamGen {
			return a, nil // stale, discard
		}
		if msg.Err != nil {
			a.notify.Add(notify.LevelError, "logs: "+msg.Err.Error(), a.contextFor(nil), "logs")
			a.logStreamCancel = nil
			return a, nil
		}
		a.logCh = msg.Ch
		return a, readLogLine(msg.Ch, msg.Gen)

	case msgs.TermBytesMsg:
		// Feed shell stdout into the emulator and reschedule the next read.
		// A missing session/pane (closed between dispatch and arrival) is a
		// no-op: drop the bytes and do not reschedule.
		sess, ok := a.terminals[msg.ID]
		if !ok {
			return a, nil
		}
		if tp, found := a.layout.TerminalPaneByID(msg.ID); found {
			_, _ = tp.Write(msg.Data)
		}
		return a, readTermBytes(sess)

	case msgs.TermExitMsg:
		// The session ended: freeze the pane (it becomes a normal closeable
		// split) and stop pumping. The session stays in the registry until the
		// pane is closed so a late lookup still resolves; closing removes both.
		if tp, found := a.layout.TerminalPaneByID(msg.ID); found {
			code := msg.Code
			// A mid-session stream/network failure carries a non-nil Err but may
			// report Code 0 (no shell exit status). Surface the error rather than
			// letting it masquerade as a clean "[exited — status 0]": force a
			// non-zero synthetic code when one was not provided and embed the error
			// text in the persistent exit banner.
			var note string
			if msg.Err != nil {
				if code == 0 {
					code = 1
				}
				note = "stream error: " + msg.Err.Error()
			}
			tp.MarkExited(code)
			// For an ephemeral pod-debug session, embed the "container can't be
			// removed" note in the pane's [exited] banner. Unlike the transient
			// status-bar flash, it persists with the frozen pane so the user still
			// sees it after the exit. A stream-error note takes precedence (it
			// explains why the session died); the ephemeral note is appended.
			if eph := a.ephemeralCloseNote(msg.ID); eph != "" {
				if note != "" {
					note += " — " + eph
				} else {
					note = eph
				}
			}
			if note != "" {
				tp.SetExitNote(note)
			}
		}
		return a, nil

	case msgs.DebugReadyMsg:
		return a.handleDebugReady(msg)

	case msgs.LogDebounceFiredMsg:
		if msg.Seq != a.logDebounceSeq {
			return a, nil
		}
		if !a.layout.IsLogMode() {
			return a, nil
		}
		focused := a.layout.FocusedSplit()
		if focused == nil || !isLoggablePlugin(focused.Plugin().Name()) {
			return a, nil
		}
		return a.restartLogForCursor()

	case msgs.LogContainerSelectedMsg:
		if !a.layout.IsLogMode() {
			return a, nil
		}
		lv := a.layout.LogView()
		lv.ClearAndRestart()
		lv.SetActiveContainer(msg.Container)
		// Resolve pod name
		focused, selected, ok := a.focusedSelection()
		if !ok {
			return a, nil
		}
		podName := resolvePodName(focused, selected)
		ns := selected.GetNamespace()
		if ns == "" {
			ns = focused.Namespace()
		}
		a, cmd := a.startLogStream(podName, msg.Container, ns, a.defaultLogOptions())
		return a, cmd

	case msgs.LogTimeRangeSelectedMsg:
		if !a.layout.IsLogMode() {
			return a, nil
		}
		lv := a.layout.LogView()
		lv.ClearAndRestart()
		lv.SetTimeRangeLabel(msg.Label)
		// Build LogOptions from selection
		opts := k8s.LogOptions{Follow: true}
		if msg.SinceSeconds != nil {
			opts.SinceSeconds = msg.SinceSeconds
		} else if msg.Label == "tail 200" || msg.Label == "" {
			tail := int64(200)
			opts.TailLines = &tail
		}
		// Restart stream
		focused, selected, ok := a.focusedSelection()
		if !ok {
			return a, nil
		}
		podName := resolvePodName(focused, selected)
		ns := selected.GetNamespace()
		if ns == "" {
			ns = focused.Namespace()
		}
		containerName := lv.ActiveContainer()
		a, cmd := a.startLogStream(podName, containerName, ns, opts)
		return a, cmd

	case msgs.DescribeLoadedMsg:
		a.statusBar.EndOperation()
		if msg.Gen != a.describeGen {
			return a, nil // stale, discard
		}
		panel := a.layout.RightPanel()
		if panel == nil {
			return a, nil
		}
		if panel.Mode() != msgs.DetailDescribe {
			return a, nil // mode changed, discard stale describe result
		}
		if msg.Err != nil {
			panel.SetLoadError(msg.Err.Error())
			return a, nil
		}
		content := msg.Content
		if msg.Events.Raw != "" {
			content = content.Append(msg.Events)
		}
		panel.SetContent(content, false) // preserve scroll position
		return a, nil

	case msgs.DescribeDebounceFiredMsg:
		if msg.Seq != a.describeDebounceSeq {
			return a, nil
		}
		if !a.layout.RightPanelVisible() || a.layout.IsLogMode() {
			return a, nil
		}
		a, cmd := a.reloadDetailPanel()
		return a, cmd

	case msgs.StatusBarShowSpinnerMsg:
		cmd := a.statusBar.Update(msg)
		return a, cmd

	case spinner.TickMsg:
		var cmds []tea.Cmd
		// Forward to detail panel spinner
		if panel := a.layout.RightPanel(); panel != nil && panel.Loading() {
			updated, cmd := panel.Update(msg)
			*panel = updated
			cmds = append(cmds, cmd)
		}
		// Forward to status bar spinner
		if a.statusBar.Busy() {
			cmds = append(cmds, a.statusBar.UpdateSpinner(msg))
		}
		return a, tea.Batch(cmds...)

	case msgs.ClusterHealthMsg:
		return a.handleClusterHealth(msg)
	}

	return a, nil
}

// routeTerminalKey feeds a key to the focused terminal pane's input machine and
// translates the result into app actions. It returns handled=false (and the
// app unchanged) when the pane did not consume the key, so the caller falls
// through to the normal trie. When handled, it returns the updated model and
// any command.
//
// Two outputs from HandleKey are acted on:
//   - toShell bytes are written to the pane's session (best-effort; a write to
//     a torn-down session is ignored).
//   - a PaneCommand is translated to the same layout actions the app already
//     exposes: Focus{Left,Up}→FocusPrev, Focus{Right,Down}→FocusNext,
//     Close→closeFocusedTerminal, Scroll{Up,Down}→pane scrollback by one page.
func (a App) routeTerminalKey(tp *ui.TerminalPane, msg tea.KeyPressMsg) (handled bool, model tea.Model, cmd tea.Cmd) {
	consumed, toShell, paneCmd := tp.HandleKey(msg, a.config.TerminalPrefix(), a.bindingSet.IsCaptured)
	if !consumed {
		return false, a, nil
	}

	if toShell != nil {
		if sess, ok := a.terminals[tp.ID()]; ok {
			_, _ = sess.Write(toShell)
		}
	}

	var outCmd tea.Cmd
	switch paneCmd {
	case ui.PaneCmdFocusLeft, ui.PaneCmdFocusUp:
		a.layout.FocusPrev()
		a.syncTerminalSizes()
		// Keep the status bar context badge in sync after a prefix-nav focus move,
		// mirroring the trie focus path (executeCommand's focus-left/up).
		a = a.syncStatusBarContext()
	case ui.PaneCmdFocusRight, ui.PaneCmdFocusDown:
		a.layout.FocusNext()
		a.syncTerminalSizes()
		// Keep the status bar context badge in sync after a prefix-nav focus move,
		// mirroring the trie focus path (executeCommand's focus-right/down).
		a = a.syncStatusBarContext()
	case ui.PaneCmdClose:
		a, outCmd = a.closeFocusedTerminal()
	case ui.PaneCmdScrollUp:
		tp.ScrollUp(terminalScrollPageLines(tp))
	case ui.PaneCmdScrollDown:
		tp.ScrollDown(terminalScrollPageLines(tp))
	case ui.PaneCmdNone:
		// Key consumed by the shell or a no-op nav key; nothing to do.
	}

	return true, a, outCmd
}

// terminalScrollPageLines is the number of lines a prefix-pgup/pgdown scrolls a
// terminal pane's scrollback — one near-full page (the pane's inner content
// height minus a one-line overlap so the user keeps a row of context). Clamped
// to at least 1.
func terminalScrollPageLines(tp *ui.TerminalPane) int {
	_, ih := tp.InnerSize()
	n := ih - 1
	if n < 1 {
		n = 1
	}
	return n
}

// closeFocusedTerminal closes the focused terminal pane via the shared
// close-split path (which is terminal-aware: it tears down the SPDY session,
// drops it from the registry, and removes the pane). Closing the last remaining
// split is a no-op here — CloseCurrentSplit signals quit without removing it, so
// the layout is never left empty; the explicit ctrl+w/q path owns the quit.
func (a App) closeFocusedTerminal() (App, tea.Cmd) {
	if a.layout.SplitCount() <= 1 {
		// Still tear down the session so a dangling SPDY stream does not leak,
		// but leave the pane in place (quit is owned by the ctrl+w/q path).
		if tp, ok := a.layout.FocusedPane().(*ui.TerminalPane); ok {
			note := a.ephemeralCloseNote(tp.ID())
			a.closeTerminalSession(tp.ID())
			if note != "" {
				a.notify.Add(notify.LevelWarning, note, a.contextFor(nil), "debug")
			}
		}
		return a, nil
	}
	return a.closeFocusedSplit()
}

func (a App) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Refresh active trie when at root (context may have changed)
	if a.keyTrie.AtRoot() {
		ct, rn := a.currentContext()
		if ct != a.trieContextType || rn != a.trieResourceName {
			a.keyTrie = a.bindingSet.TrieFor(ct, rn)
			a.trieContextType = ct
			a.trieResourceName = rn
		}
	}

	// Walk the key trie
	command, run, resolved := a.keyTrie.Press(key)
	if !resolved {
		// Mid-sequence: update status bar with available next keys
		a.statusBar.SetHints(a.currentHints())
		return a, nil
	}

	// Resolved: execute command
	a.statusBar.SetHints(a.currentHints())

	if command == "" && run == nil {
		// Invalid key — already reset by trie
		return a, nil
	}

	if run != nil {
		return a.handleRunCommand(run)
	}

	return a.executeCommand(command)
}

// doubleClickWindow is the maximum elapsed time between two clicks for them
// to be considered a double-click. Hardcoded per the mouse-support plan.
const doubleClickWindow = 500 * time.Millisecond

// terminalWheelLines is the number of scrollback lines a single mouse-wheel
// notch scrolls a terminal pane (a common terminal default).
const terminalWheelLines = 3

// handleMouseClick routes a left-click event to the pane under the cursor,
// moving focus (and the split's row cursor when applicable). Non-left buttons
// and clicks outside any pane are dropped. When an overlay is active the
// click is dropped entirely — the overlay is not dismissed and no background
// focus is stolen.
//
// A second left-click on the same split cell within doubleClickWindow is
// treated as a double-click and dispatches the "enter-detail" command (the
// same action Enter binds to in the resources scope).
func (a App) handleMouseClick(msg tea.MouseClickMsg) (tea.Model, tea.Cmd) {
	// Defense in depth: bubbletea should not deliver mouse events when
	// MouseMode is None, but tests bypass that path and a misbehaving
	// terminal could too.
	if !a.config.Mouse.Enabled {
		return a, nil
	}
	if msg.Button != tea.MouseLeft {
		return a, nil
	}
	if a.activeOverlay != overlayNone {
		return a, nil
	}
	rect, ok := a.layout.PaneAt(msg.X, msg.Y)
	if !ok {
		return a, nil
	}
	// row is the body-row index under the click (or -1 for chrome / non-split).
	row := -1
	var refreshCmd tea.Cmd
	switch rect.Kind {
	case layout.PaneSplit:
		prevFocusIdx := a.layout.FocusIndex()
		split := a.layout.SplitAt(rect.SplitIdx)
		prevCursor := -1
		if split != nil {
			prevCursor = split.Cursor()
		}
		// FocusSplitAt self-reconciles to resources (sets focusTarget=Resources
		// and runs reconcileFocus), so no follow-up FocusResources is needed.
		a.layout.FocusSplitAt(rect.SplitIdx)
		crossSplit := rect.SplitIdx != prevFocusIdx
		if split != nil {
			if r := split.RowAtY(msg.Y - rect.Y); r >= 0 {
				split.SetCursor(r)
				row = r
				if crossSplit || r != prevCursor {
					a, refreshCmd = a.refreshDetailPanelOrLog()
				}
			}
		}
	case layout.PaneDetail, layout.PaneLog:
		a.layout.FocusDetails()
	}

	// Double-click detection: only splits can drill down, and the second
	// click must land on the same data row as the first. Comparing row
	// indices (not raw screen coords) protects against false doubles when
	// the viewport has scrolled between the two clicks. Clicks on the
	// detail/log pane update lastClick* (so a follow-up click on a split at
	// the same coords cannot spuriously count as a double) but never trigger
	// a double themselves.
	now := a.now()
	isDouble := rect.Kind == layout.PaneSplit &&
		a.lastClickKind == layout.PaneSplit &&
		row >= 0 &&
		row == a.lastClickRow &&
		rect.SplitIdx == a.lastClickSplit &&
		now.Sub(a.lastClickTime) <= doubleClickWindow

	a.lastClickTime = now
	a.lastClickKind = rect.Kind
	a.lastClickRow = row
	if rect.Kind == layout.PaneSplit {
		a.lastClickSplit = rect.SplitIdx
	} else {
		a.lastClickSplit = -1
	}

	if isDouble {
		// Reset so a third click in quick succession does not chain into
		// another double-click. The updated `a` (with lastClick state and
		// reset lastClickTime) is passed by value into executeCommand, so
		// the returned model preserves these fields.
		a.lastClickTime = time.Time{}
		if a.executeCommandFn != nil {
			return a.executeCommandFn(a, "enter-detail")
		}
		return a.executeCommand("enter-detail")
	}
	return a, refreshCmd
}

// handleMouseWheel routes a wheel event to the pane under the cursor (or the
// active overlay) without changing focus. Wheel events outside any pane or
// outside the active overlay rect are dropped.
func (a App) handleMouseWheel(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	// Defense in depth: bubbletea should not deliver mouse events when
	// MouseMode is None, but tests bypass that path and a misbehaving
	// terminal could too.
	if !a.config.Mouse.Enabled {
		return a, nil
	}
	if a.activeOverlay != overlayNone {
		// Drop events if we have no populated rect (View() has not yet run)
		// or the pointer is outside the overlay bounds.
		if !a.OverlayRect().Contains(msg.X, msg.Y) {
			return a, nil
		}
		// Inline the active-overlay routing. The pointer expressions take
		// the address of fields on the local value receiver `a`; ScrollWheel
		// mutates that local copy, which is then returned to bubbletea so
		// the mutation persists.
		//
		// Overlays not listed here intentionally swallow the wheel:
		//   - overlayPortForward, overlaySetImage, overlayHelmRollback,
		//     overlayChartInput, overlayScale: text-input forms, no
		//     scrollable content.
		//   - overlayConfirm: single-screen prompt, no scroll.
		//   - overlaySearchBar: inline single-line input.
		switch a.activeOverlay {
		case overlayResourcePicker:
			(&a.resourcePicker).ScrollWheel(msg.Button)
		case overlayNsPicker:
			(&a.nsPicker).ScrollWheel(msg.Button)
		case overlayContextPicker:
			(&a.contextPicker).ScrollWheel(msg.Button)
		case overlayContainerPicker:
			(&a.containerPicker).ScrollWheel(msg.Button)
		case overlayTimeRange:
			(&a.timeRangePicker).ScrollWheel(msg.Button)
		case overlayHelp:
			(&a.helpOverlay).ScrollWheel(msg.Button)
		}
		return a, nil
	}

	rect, ok := a.layout.PaneAt(msg.X, msg.Y)
	if !ok {
		return a, nil
	}
	switch rect.Kind {
	case layout.PaneSplit:
		// Terminal panes and resource panes both live in the splits region and
		// both report PaneSplit. A terminal pane scrolls its scrollback buffer;
		// a resource pane moves its row cursor. SplitAt narrows to resource panes
		// (nil for terminals), so dispatch on the raw pane kind here.
		if tp, ok := a.layout.PaneAtIdx(rect.SplitIdx).(*ui.TerminalPane); ok {
			switch msg.Button {
			case tea.MouseWheelUp:
				tp.ScrollUp(terminalWheelLines)
			case tea.MouseWheelDown:
				tp.ScrollDown(terminalWheelLines)
			}
		} else if split := a.layout.SplitAt(rect.SplitIdx); split != nil {
			split.ScrollWheel(msg.Button)
		}
	case layout.PaneDetail:
		if panel := a.layout.RightPanel(); panel != nil {
			panel.ScrollWheel(msg.Button)
		}
	case layout.PaneLog:
		if lv := a.layout.LogView(); lv != nil {
			updated, cmd := lv.Update(msg)
			*lv = updated
			return a, cmd
		}
	}
	return a, nil
}

func (a App) refreshDetailPanelOpts(preserve bool) (App, tea.Cmd) {
	if a.layout.IsLogMode() {
		// In log mode, refreshes are handled by the log stream, not the detail panel
		return a, nil
	}
	if !a.layout.RightPanelVisible() {
		return a, nil
	}
	focused := a.layout.FocusedSplit()
	if focused == nil {
		return a, nil
	}
	sel := focused.Selected()
	if sel == nil {
		return a, nil
	}
	// When the selected resource changes (cursor move, mouse click, identity
	// change from store update, etc.), clear uncover state so the [S] header
	// indicator does not stick on resources whose plugin may not support it.
	if newKey := a.detailKey(); newKey != "" && a.lastDetailKey != "" && newKey != a.lastDetailKey {
		a.envResolved = false
	}
	panel := a.layout.RightPanel()
	panel.SetEnvResolved(a.envResolved)
	refresh := !preserve
	switch panel.Mode() {
	case msgs.DetailYAML:
		a.lastDetailKey = a.detailKey()
		content, err := focused.Plugin().YAML(sel)
		if err == nil {
			panel.SetContent(content, refresh)
		}
	case msgs.DetailValues, msgs.DetailValuesAll:
		a.lastDetailKey = a.detailKey()
		hp, ok := focused.Plugin().(*helmreleases.Plugin)
		if !ok {
			// Defensive fallback: chord is helmreleases-scoped so this shouldn't fire.
			content, err := focused.Plugin().YAML(sel)
			if err == nil {
				panel.SetContent(content, refresh)
			}
			return a, nil
		}
		placeholder := "# loading values...\n"
		panel.SetContent(render.Content{Raw: placeholder, Display: placeholder}, refresh)
		mode := panel.Mode()
		selCopy := sel.DeepCopy()
		// Use the FOCUSED pane's cluster helm client, not the plugin's baked
		// startup one, so values are read from the pane's own cluster.
		hc := a.helmClientFor(a.clusterFor(focused))
		fetchCmd := func() tea.Msg {
			var (
				content render.Content
				err     error
			)
			if mode == msgs.DetailValuesAll {
				content, err = hp.ValuesAllWith(hc, selCopy)
			} else {
				content, err = hp.ValuesWith(hc, selCopy)
			}
			return msgs.HelmValuesLoadedMsg{
				ReleaseName: selCopy.GetName(),
				Namespace:   selCopy.GetNamespace(),
				Mode:        mode,
				Content:     content,
				Err:         err,
			}
		}
		return a, fetchCmd
	case msgs.DetailDescribe:
		a.lastDetailKey = a.detailKey()
		spinCmd := panel.SetLoading(true)
		opCmd := a.statusBar.StartOperation()
		a.describeGen++
		gen := a.describeGen
		p := focused.Plugin()
		selCopy := sel.DeepCopy()
		envResolved := a.envResolved
		ns := sel.GetNamespace()
		if ns == "" {
			ns = focused.Namespace()
		}
		gvr := p.GVR()
		cl := a.clusterFor(focused)
		store := plugin.StoreOf(cl)
		discovery := plugin.DiscoveryOf(cl)
		timeout := a.config.APITimeout()
		describeCmd := func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			var content render.Content
			var err error
			if envResolved {
				if unc, ok := p.(plugin.Uncoverable); ok {
					content, err = unc.DescribeUncovered(ctx, cl, selCopy)
					if err != nil {
						content, err = p.Describe(ctx, selCopy)
					}
				}
			}
			if content.Raw == "" && err == nil {
				content, err = p.Describe(ctx, selCopy)
			}
			var events render.Content
			if err == nil && content.Raw != "" && store != nil && discovery != nil && gvr != eventsGVR {
				if kind, ok := discovery.KindForGVR(gvr); ok {
					store.Subscribe(eventsGVR, ns)
					allEvents := store.List(eventsGVR, ns)
					events = render.RenderEvents(allEvents, kind, selCopy.GetName(), ns)
				}
			}
			return msgs.DescribeLoadedMsg{Content: content, Events: events, Gen: gen, Err: err}
		}
		return a, tea.Batch(spinCmd, opCmd, describeCmd)
	case msgs.DetailLogs:
		// Log mode: handled by LogView + streaming, not by detail panel
		if a.layout.IsLogMode() {
			return a, nil
		}
		msg := "Log streaming not yet implemented"
		panel.SetContent(render.Content{Raw: msg, Display: msg}, refresh)
	}
	return a, nil
}

// refreshDetailPanel resets scroll to top — used after cursor navigation, tab switch, focus change.
func (a App) refreshDetailPanel() (App, tea.Cmd) {
	return a.refreshDetailPanelOpts(false)
}

// reloadDetailPanel preserves scroll position — used for background auto-reload and manual ctrl+r.
func (a App) reloadDetailPanel() (App, tea.Cmd) {
	return a.refreshDetailPanelOpts(true)
}

// refreshDetailPanelOrLog wraps refreshDetailPanel for cursor navigation.
// In log mode it debounces a stream restart; otherwise it delegates to
// refreshDetailPanel.
func (a App) refreshDetailPanelOrLog() (App, tea.Cmd) {
	if !a.layout.IsLogMode() {
		return a.refreshDetailPanel()
	}
	a = a.stopLogStream()
	if lv := a.layout.LogView(); lv != nil {
		lv.ClearAndRestart()
	}
	a.logDebounceSeq++
	return a, a.logDebounceCmd()
}

// describeDebounceCmd returns a tea.Cmd that fires DescribeDebounceFiredMsg
// after a short delay. Used to coalesce rapid resource updates.
func (a App) describeDebounceCmd() tea.Cmd {
	seq := a.describeDebounceSeq
	return func() tea.Msg {
		time.Sleep(100 * time.Millisecond)
		return msgs.DescribeDebounceFiredMsg{Seq: seq}
	}
}

// restartLogForCursor resolves the pod/container from the current cursor
// and starts a log stream. Always defaults to the first container.
// Used by both the debounce handler and startLogViewForSelected.
func (a App) restartLogForCursor() (tea.Model, tea.Cmd) {
	focused, selected, ok := a.focusedSelection()
	if !ok {
		return a, nil
	}
	a.lastDetailKey = a.detailKey()
	podName := resolvePodName(focused, selected)
	ns := selected.GetNamespace()
	if ns == "" {
		ns = focused.Namespace()
	}

	var containerName string
	var containers []string

	if focused.Plugin().Name() == "containers" {
		containerName = selected.GetName()
		containers = []string{containerName}
	} else {
		containers = extractContainerNames(selected)
		if len(containers) == 0 {
			a.notify.Add(notify.LevelError, "no containers found", a.contextFor(nil), "logs")
			return a, nil
		}
		containerName = containers[0]
	}

	lv := a.layout.LogView()
	lv.SetContainers(containers)
	lv.SetActiveContainer(containerName)
	lv.SetPodName(podName)
	lv.SetNamespace(ns)

	a, cmd := a.startLogStream(podName, containerName, ns, a.defaultLogOptions())
	return a, cmd
}

// defaultLogOptions builds LogOptions from the configured default time range,
// correctly handling sentinel values like "tail 200" (-1) and "all" (0).
func (a App) defaultLogOptions() k8s.LogOptions {
	lv := a.layout.LogView()
	s := lv.DefaultSinceSeconds()
	opts := k8s.LogOptions{Follow: true}
	if s > 0 {
		opts.SinceSeconds = &s
	} else if s == -1 {
		tail := int64(200)
		opts.TailLines = &tail
	}
	// s == 0 ("all") leaves both nil, streaming everything
	return opts
}

// searchTarget returns the component that should receive search operations
// based on the current focus mode, or nil if no valid target exists.
func (a App) searchTarget() ui.Searchable {
	if a.layout.FocusedDetails() && a.layout.RightPanelVisible() {
		return a.layout.ActiveDetailPanel()
	}
	if focused := a.layout.FocusedSplit(); focused != nil {
		return focused
	}
	return nil
}

// clearInlineSearch resets the inline search text on all panes.
func (a App) clearInlineSearch() {
	for i := range a.layout.SplitCount() {
		if s := a.layout.SplitAt(i); s != nil {
			s.SetInlineSearch("")
		}
	}
	if a.layout.RightPanelVisible() {
		a.layout.ActiveDetailPanel().SetInlineSearch("")
	}
}

// syncInlineSearch updates inline search display state.
// Called after every Update to keep View() pure.
// NOTE: value receiver is intentional — mutations propagate through pointer
// indirection (Layout returns *ResourceList / *DetailView into shared backing).
func (a App) syncInlineSearch() {
	a.clearInlineSearch()
	if a.activeOverlay == overlaySearchBar {
		if iv := a.searchBar.InlineView(); iv != "" {
			if a.layout.FocusedDetails() && a.layout.RightPanelVisible() {
				a.layout.ActiveDetailPanel().SetInlineSearch(iv)
			} else if focused := a.layout.FocusedSplit(); focused != nil {
				focused.SetInlineSearch(iv)
			}
		}
	}
}

// currentContext returns the scope names for the current focus state.
//
// A focused terminal pane (resource focus, not the detail panel) reports the
// "terminal" context. This drives the statusbar/help hints and the keybinding
// trie for that pane. Note the trie only governs an EXITED terminal pane: a
// live (non-exited) terminal's keys are intercepted earlier by routeTerminalKey
// (see Update's KeyPressMsg branch) and never reach the trie — except captured
// keys (alt+z / shift+arrows), which routeTerminalKey deliberately lets fall
// through. For ordinary keys the two mechanisms cannot double-handle. The
// terminal trie's nav bindings (focus moves, close, scroll) intentionally
// mirror the prefix-machine's nav commands so an exited pane stays navigable
// with the same intents. Zoom is NOT part of that mirror: it was removed from
// the prefix machine and is now reached only via the captured alt+z → trie path.
func (a App) currentContext() (componentType, resourceName string) {
	componentType = "resources"
	if a.layout.FocusedDetails() {
		if a.layout.IsLogMode() {
			componentType = "logs"
		} else {
			switch a.layout.RightPanel().Mode() {
			case msgs.DetailYAML:
				componentType = "yaml"
			case msgs.DetailDescribe:
				componentType = "describe"
			default:
				componentType = "details"
			}
		}
		return
	}
	// Resource focus: a terminal pane reports the "terminal" context (no
	// resource name); a resource pane reports "resources" + its plugin name.
	if _, ok := a.layout.FocusedPane().(*ui.TerminalPane); ok {
		componentType = "terminal"
		return
	}
	if focused := a.layout.FocusedSplit(); focused != nil {
		resourceName = focused.Plugin().Name()
	}
	return
}

// currentHints returns context-aware key hints for the status bar.
func (a App) currentHints() []config.KeyHint {
	if a.keyTrie.AtRoot() {
		ct, rn := a.currentContext()
		return a.bindingSet.StatusHints(ct, rn)
	}
	return a.keyTrie.CurrentHints()
}

// syncIndicators updates the status bar indicators (zoom + port-forward count).
func (a App) syncIndicators() App {
	indicator := ""
	if a.layout.AnyZoomed() {
		indicator += ui.ZoomIndicatorStyle.Render(" ⤢ ")
	}
	if a.pfRegistry != nil && a.pfRegistry.Count() > 0 {
		indicator += ui.PortForwardIndicatorStyle.Render(fmt.Sprintf(" PF:%d ", a.pfRegistry.Count()))
	}
	a.statusBar.SetIndicator(indicator)
	return a
}

// syncStatusBarContext makes the status bar reflect the focused pane's context
// and its cluster connectivity. The focused pane is the source of truth, so this
// is called after any focus change to keep the badge name and its online/offline
// color correct immediately (rather than waiting for the next heartbeat tick).
func (a App) syncStatusBarContext() App {
	// FocusedSplit() narrows to a resource pane and returns nil for a terminal
	// pane, which would fall back to the startup context and show the wrong
	// cluster when a terminal pane is focused. Resolve through paneContext (which
	// is pane-kind aware: terminal panes expose their context directly) so the
	// badge tracks whatever pane actually holds focus. A nil focused pane (no
	// splits) keeps the original contextFor(nil) fallback to the startup context.
	if focused := a.layout.FocusedPane(); focused != nil {
		return a.setStatusBarContext(a.paneContext(focused))
	}
	return a.setStatusBarContext(a.contextFor(nil))
}

// setStatusBarContext points the status bar at an explicit context, deriving the
// online/offline color from that context's current Manager connection state. Use
// this when the focused pane is about to move to ctx but hasn't been restamped
// yet (e.g. an optimistic group switch); use syncStatusBarContext when the
// focused pane already carries its final context.
func (a App) setStatusBarContext(ctx string) App {
	a.statusBar.SetContextName(ctx)
	online := false
	if cl, ok := a.mgr.Get(ctx); ok && cl != nil {
		online = cl.Connected()
	}
	a.statusBar.SetOnline(online)
	return a
}

// refreshSelfPopulatingSplits re-polls every open split whose plugin name
// matches resource and is SelfPopulating, pushing its current Objects() into the
// pane. Used to force synthetic resources (portforwards, aku-messages, …) to
// re-render when their backing model changes outside the informer path.
func (a App) refreshSelfPopulatingSplits(resource string) App {
	for i := range a.layout.SplitCount() {
		split := a.layout.SplitAt(i)
		if split != nil && split.Plugin().Name() == resource {
			if sp, ok := split.Plugin().(plugin.SelfPopulating); ok {
				split.SetObjects(sp.Objects())
			}
		}
	}
	return a
}

// actionSource extracts the short op label (the verb before the first ':') from
// an ActionResultMsg.ActionID, used as the notify message Source. ActionIDs are
// formatted "<verb>:<detail>" (e.g. "scale:nginx", "delete:3-resources").
func actionSource(actionID string) string {
	if i := strings.IndexByte(actionID, ':'); i >= 0 {
		return actionID[:i]
	}
	if actionID == "" {
		return "action"
	}
	return actionID
}

// actionSuccessNote returns a short human note for a successful action, derived
// from the ActionID's "<verb>:<detail>" shape. Returns "" for verbs that have
// no useful success text (the caller then records nothing on success).
func actionSuccessNote(actionID string) string {
	verb, detail, found := strings.Cut(actionID, ":")
	if !found {
		return ""
	}
	switch verb {
	case "scale":
		return "scaled " + detail
	case "delete":
		return "deleted " + detail
	case "restart":
		return "rollout restart " + detail
	case "set-image":
		return "updated image for " + detail
	case "helm-rollback":
		return "rolled back " + detail
	case "helm-uninstall":
		return "uninstalled " + detail
	case "edit":
		return "applied edit to " + detail
	default:
		return ""
	}
}

// toastTTL resolves the auto-hide duration for a toast of level l from config. A
// return of 0 means sticky (no auto-expiry tick). Used both as the TTL predicate
// passed to notify.Store.Live and to choose each toast's tea.Tick duration.
func (a App) toastTTL(l notify.Level) time.Duration {
	return a.config.ToastTTL(int(l))
}

// pruneDismissed drops any ID from the dismissed set that no longer exists in
// the store's current window. The notify ring buffer evicts old messages, so
// without this the dismissed map would grow without bound for the session
// lifetime. An evicted ID is simply forgotten — it cannot reappear because it is
// gone from the store — while a dismissed ID still in the window is preserved so
// it stays hidden. Called after every mutation of dismissed; O(n) over the
// store window, which is bounded by the ring capacity. Deleting keys during a
// range is safe in Go. The map is a reference, so mutation on the value receiver
// persists.
func (a App) pruneDismissed() {
	if a.dismissed == nil || a.notify == nil {
		return
	}
	live := make(map[uint64]bool, len(a.dismissed))
	for _, m := range a.notify.List() {
		live[m.ID] = true
	}
	for id := range a.dismissed {
		if !live[id] {
			delete(a.dismissed, id)
		}
	}
}

// messageCreatedAt returns the creation timestamp of the message with the given
// ID from the store's current window, and whether it was found. A miss means the
// message was evicted from the ring buffer (the caller treats it as expired).
func (a App) messageCreatedAt(id uint64) (time.Time, bool) {
	if a.notify == nil {
		return time.Time{}, false
	}
	for _, m := range a.notify.List() {
		if m.ID == id {
			return m.Time, true
		}
	}
	return time.Time{}, false
}

// visibleToasts returns the messages that should currently be drawn as toasts:
// notify.Live(now, toastTTL) minus any IDs in dismissed (expired or cleared). It
// is nil-safe for the store so View never panics on a construction path that did
// not inject one (e.g. tests).
func visibleToasts(store *notify.Store, dismissed map[uint64]bool, now time.Time, ttl func(notify.Level) time.Duration) []notify.Message {
	if store == nil {
		return nil
	}
	live := store.Live(now, ttl)
	out := live[:0]
	for _, m := range live {
		if dismissed[m.ID] {
			continue
		}
		out = append(out, m)
	}
	return out
}

func (a App) localPortForPF(id string) int {
	for _, e := range a.pfRegistry.List() {
		if e.ID == id {
			return e.LocalPort
		}
	}
	return 0
}

// handleAPIResourcesDiscovered consumes a discovery result.
//
// Per-cluster routing: the per-cluster *k8s.Discovery index is populated by the
// Refresh call that produced this message (disc.Refresh runs against the
// cluster's OWN Discovery in Init / handleGroupContextSwitch / handleClusterReady),
// so the per-cluster missing-resource check (cl.Discovery().KindForGVR) is already
// accurate for every cluster. THIS handler additionally populates the
// process-global, shared plugin registry (the picker/registry is a superset by
// design). To avoid a CRD that exists only on cluster B becoming navigable (and
// mis-subscribing) from cluster A's panes, the shared registry is fed ONLY from
// the result whose Context matches the focused pane's resolved context
// (contextFor(FocusedSplit())). Results tagged with any other context still
// populated their own cluster's Discovery (above) but are skipped here.
func (a App) handleAPIResourcesDiscovered(msg k8s.APIResourcesDiscoveredMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil && len(msg.Resources) == 0 {
		a.notify.Add(notify.LevelError, "api discovery: "+msg.Err.Error(), a.contextFor(nil), "discovery")
		return a, nil
	}
	if msg.Err != nil {
		a.notify.Add(notify.LevelWarning, "api discovery partial: "+msg.Err.Error(), a.contextFor(nil), "discovery")
	}

	// Only the focused pane's context feeds the shared plugin registry / picker.
	// A result from another cluster has already populated its own cluster's
	// Discovery index (so its panes resolve correctly) and must not leak
	// resources into the shared superset. An empty Context resolves to the
	// focused pane's context (startup at boot).
	//
	// Open question: it is not yet settled which cluster's discovery should feed
	// the shared registry once async group switching is finalized — today it is
	// the focused pane's context, which can change between dispatch and arrival.
	registryCtx := a.contextFor(a.layout.FocusedSplit())
	tagged := msg.Context
	if tagged == "" {
		tagged = registryCtx
	}
	if tagged != registryCtx {
		return a, nil
	}

	// Register generic plugins for undiscovered resources
	for _, r := range msg.Resources {
		shortName := ""
		if len(r.ShortNames) > 0 {
			shortName = r.ShortNames[0]
		}
		plugin.RegisterIfAbsent(generic.NewDiscovered(r.GVR, r.Name, shortName, !r.Namespaced))
	}

	// Update api-resources plugin with discovery data
	if p, ok := plugin.ByName("api-resources"); ok {
		if arPlugin, ok := p.(*apiresources.Plugin); ok {
			arPlugin.SetResources(msg.Resources)
		}
	}

	// Refresh resource picker entries with all plugins (including newly discovered)
	a.resourcePicker.SetPlugins(buildPickerEntries())

	// If any split is currently showing api-resources, update its objects
	a = a.refreshSelfPopulatingSplits("api-resources")

	return a, nil
}

// toggleZoomDetailAndSync toggles detail zoom and updates the indicator.
func (a App) toggleZoomDetailAndSync() App {
	a.layout.ToggleZoomDetail()
	return a.syncIndicators()
}

// toggleZoomSplitAndSync toggles split zoom and updates the indicator. Zooming a
// split changes every pane's render geometry, so push the new sizes to any live
// terminal sessions too.
func (a App) toggleZoomSplitAndSync() App {
	a.layout.ToggleZoomSplit()
	a = a.syncIndicators()
	a.syncTerminalSizes()
	return a
}

func (a App) View() tea.View {
	body := a.layout.View()

	var main string
	if a.layout.EffectiveZoom() != layout.ZoomNone {
		main = body
	} else {
		statusBar := a.statusBar.View()
		main = lipgloss.JoinVertical(lipgloss.Left, body, statusBar)
	}

	// Capture the overlay rect (for mouse hit-testing) and composite the
	// overlay onto main in a single pass — PlaceOverlayWithRect measures
	// and anchors the overlay once and returns both the rendered string
	// and the rect.
	overlayView := a.currentOverlayView()
	if overlayView != "" {
		rendered, rect, ok := ui.PlaceOverlayWithRect(a.width, a.height, main, overlayView)
		if ok {
			a.setOverlayRect(rect)
		} else {
			a.clearOverlayRect()
		}
		main = rendered
	} else {
		a.clearOverlayRect()
	}

	// Toasts float above everything, including modals: composite them LAST,
	// anchored to the top-right with no backdrop dim. visibleToasts is nil-safe
	// for a missing store and excludes expired/cleared IDs. Sample the clock once
	// so the whole render pass uses a single "now".
	now := a.now()
	if visible := visibleToasts(a.notify, a.dismissed, now, a.toastTTL); len(visible) > 0 {
		if tv := a.toasts.View(visible, a.width, a.height); tv != "" {
			main = ui.PlaceOverlay(a.width, a.height, main, tv, ui.WithOverlayPosition(1.0, 0.0), ui.WithDim(false))
		}
	}

	return tea.View{
		Content:         main,
		AltScreen:       true,
		MouseMode:       mouseMode(a.config.Mouse.Enabled),
		Cursor:          a.terminalCursor(),
		BackgroundColor: themeColorOrNil(theme.Background),
		ForegroundColor: themeColorOrNil(theme.Foreground),
	}
}

// themeColorOrNil returns c as a color.Color, or nil when c is empty so the
// terminal keeps its default (an empty theme.Color would resolve to black).
func themeColorOrNil(c theme.Color) color.Color {
	if c == "" {
		return nil
	}
	return c
}

// terminalCursor returns the real terminal cursor positioned at the focused
// terminal pane's live cursor, or nil when no cursor should be shown (an overlay
// owns input, focus is on a non-terminal pane, the pane has exited, or it is
// scrolled into history). The emulator reports an inner-grid (x, y); the pane's
// cached rect gives the outer top-left, and the pane's own ContentOffset adds the
// gap to the first content cell — (1,1) for a bordered pane, (0,1) for a
// borderless (zoomed) pane whose left border is gone but which still has a header
// line. paneRects share the frame's coordinate space (the body sits at the top-
// left and the status bar is appended below), so these coords are what
// tea.View.Cursor expects.
func (a App) terminalCursor() *tea.Cursor {
	if a.activeOverlay != overlayNone {
		return nil
	}
	tp, ok := a.layout.FocusedPane().(*ui.TerminalPane)
	if !ok {
		return nil
	}
	cx, cy, visible := tp.CursorPos()
	if !visible {
		return nil
	}
	rect, ok := a.layout.FocusedSplitRect()
	dx, dy := tp.ContentOffset()
	iw, ih := tp.InnerSize()
	// The rect must hold at least one content cell past the offset.
	if !ok || rect.W <= dx || rect.H <= dy {
		return nil
	}
	// Clamp into the content box so a cursor at the wrap column can't land on or
	// past the frame edge. The horizontal extent is the smaller of the emulator's
	// inner width and what the rect can fit after the offset. The vertical extent
	// is the emulator's inner height alone: under ZoomSplit the pane is sized one
	// row taller than rect.H (the rect stops at l.height to exclude the status-bar
	// row from mouse hit-testing, but the borderless pane renders over that row
	// since the status bar is hidden when zoomed), so clamping cy by rect.H-dy
	// would lose the last emulator row. In bordered mode ih <= rect.H-dy always
	// holds, so using ih is behaviorally identical there.
	maxX := min(iw, rect.W-dx) - 1
	maxY := ih - 1
	if maxX < 0 || maxY < 0 {
		return nil
	}
	if cx > maxX {
		cx = maxX
	}
	if cy > maxY {
		cy = maxY
	}
	cur := tea.NewCursor(rect.X+dx+cx, rect.Y+dy+cy)
	cur.Shape = cursorShape(tp.CursorShape())
	return cur
}

// cursorShape maps the ui-package cursor shape (tracked from the terminal's
// DECSCUSR state) to the bubbletea cursor shape.
func cursorShape(s ui.CursorShape) tea.CursorShape {
	switch s {
	case ui.CursorShapeUnderline:
		return tea.CursorUnderline
	case ui.CursorShapeBar:
		return tea.CursorBar
	default:
		return tea.CursorBlock
	}
}

// mouseMode returns the tea.MouseMode enum for the configured mouse.enabled
// flag. Kept as a tiny helper so tea.View{} construction stays expression-form.
func mouseMode(enabled bool) tea.MouseMode {
	if enabled {
		return tea.MouseModeCellMotion
	}
	return tea.MouseModeNone
}

// currentOverlayView returns the rendered view string of the currently active
// overlay, or "" if no overlay is active or its view is empty. The search bar
// and confirm dialog returns are handled consistently with the render switch
// in View().
func (a App) currentOverlayView() string {
	switch a.activeOverlay {
	case overlayResourcePicker:
		return a.resourcePicker.View()
	case overlayNsPicker:
		return a.nsPicker.View()
	case overlayContextPicker:
		return a.contextPicker.View()
	case overlayPortForward:
		return a.portForwardOverlay.View()
	case overlaySetImage:
		return a.setImageOverlay.View()
	case overlayHelmRollback:
		return a.helmRollbackOverlay.View()
	case overlayChartInput:
		return a.chartInputOverlay.View()
	case overlayConfirm:
		return a.confirmDialog.View()
	case overlayHelp:
		return a.helpOverlay.View()
	case overlayContainerPicker:
		return a.containerPicker.View()
	case overlayTimeRange:
		return a.timeRangePicker.View()
	case overlayScale:
		return a.scaleOverlay.View()
	}
	return ""
}

// setOverlayRect stores the current overlay's screen rectangle. Uses a pointer
// field so the mutation persists across the value-receiver View() call. The
// pointer is always non-nil — New() initializes it and nothing reassigns it.
func (a App) setOverlayRect(r ui.OverlayRect) {
	*a.overlayRect = r
}

// clearOverlayRect zeroes the stored overlay rect (no active overlay).
func (a App) clearOverlayRect() {
	*a.overlayRect = ui.OverlayRect{}
}

// OverlayRect returns the screen rectangle occupied by the currently active
// overlay. The zero value (all fields 0) indicates no active overlay.
func (a App) OverlayRect() ui.OverlayRect {
	return *a.overlayRect
}

// refreshDrillDownSplit re-runs the parent's DrillDown to get fresh filtered
// children for a single split that is currently in a drill-down view. The
// DrillDown reads from the split's own cluster (its store/discovery), keeping
// drill-downs scoped to the pane's context.
func (a App) refreshDrillDownSplit(split *ui.ResourceList) {
	snap, ok := split.ParentSnap()
	if !ok {
		return
	}
	drillable, ok := snap.Plugin.(plugin.DrillDowner)
	if !ok {
		return
	}

	cl := a.clusterFor(split)

	var parentObj *unstructured.Unstructured

	if sp, ok := snap.Plugin.(plugin.SelfPopulating); ok {
		for _, obj := range sp.Objects() {
			if obj.GetName() == snap.ParentName &&
				(snap.ParentKind == "" || obj.GetKind() == snap.ParentKind) &&
				(snap.ParentAPIVersion == "" || obj.GetAPIVersion() == snap.ParentAPIVersion) {
				parentObj = obj
				break
			}
		}
	} else if snap.ParentUID != "" {
		store := plugin.StoreOf(cl)
		if store == nil {
			return
		}
		parentNs := split.Namespace()
		if snap.Plugin.IsClusterScoped() {
			parentNs = ""
		}
		for _, obj := range store.List(snap.Plugin.GVR(), parentNs) {
			if string(obj.GetUID()) == snap.ParentUID {
				parentObj = obj
				break
			}
		}
	}

	if parentObj == nil {
		return
	}
	_, children := drillable.DrillDown(cl, parentObj)
	split.SetObjects(children)
}

// refreshDrillDownSplits re-extracts child data for any split that is
// currently drilled down and showing the given GVR.
func (a App) refreshDrillDownSplits(updatedGVR schema.GroupVersionResource, namespace string) {
	for i := range a.layout.SplitCount() {
		split := a.layout.SplitAt(i)
		if split == nil || !split.InDrillDown() {
			continue
		}
		snap, ok := split.ParentSnap()
		if !ok {
			continue
		}
		// Refresh if the update matches this split's current child view OR its
		// drill-down parent. Container drill-downs derive children from the parent
		// pod, whose GVR (v1/pods) differs from the synthetic child GVR — so a
		// parent-GVR match is what makes them refresh live. GVRs are unique per
		// resource kind, so the parent-GVR clause cannot match an unrelated update.
		if split.Plugin().GVR() != updatedGVR && snap.Plugin.GVR() != updatedGVR {
			continue
		}
		// Match exact namespace, or refresh all-namespace drill-downs (namespace="")
		if split.EffectiveNamespace() != namespace && split.EffectiveNamespace() != "" {
			continue
		}
		a.refreshDrillDownSplit(split)
	}
}

func (a App) handleHelmRollback(msg msgs.HelmRollbackRequestedMsg) (tea.Model, tea.Cmd) {
	hc := a.helmClientForFocused()
	if hc == nil {
		a.notify.Add(notify.LevelError, "helm: no client", a.contextFor(nil), "helm")
		return a, nil
	}
	return a, func() tea.Msg {
		if err := hc.Rollback(msg.ReleaseName, msg.Namespace, msg.Revision); err != nil {
			return msgs.ActionResultMsg{Err: err}
		}
		return msgs.ActionResultMsg{ActionID: "helm-rollback:" + msg.ReleaseName}
	}
}

// handleHelmValuesLoaded stamps async-fetched helm values onto the right panel.
// Stale messages are dropped: the (release, namespace, variant) tuple in the
// message must still match the focused panel, otherwise the user has moved on
// (cursor change, mode switch, or a different variant request) and the result
// would clobber unrelated content.
func (a App) handleHelmValuesLoaded(msg msgs.HelmValuesLoadedMsg) (tea.Model, tea.Cmd) {
	if !a.layout.RightPanelVisible() || a.layout.IsLogMode() {
		return a, nil
	}
	panel := a.layout.RightPanel()
	if panel == nil {
		return a, nil
	}
	if panel.Mode() != msg.Mode {
		return a, nil
	}
	focused := a.layout.FocusedSplit()
	if focused == nil || focused.Plugin().Name() != "helmreleases" {
		return a, nil
	}
	sel := focused.Selected()
	if sel == nil {
		return a, nil
	}
	curName := sel.GetName()
	curNs := sel.GetNamespace()
	if curNs == "" {
		curNs = focused.Namespace()
	}
	if curName != msg.ReleaseName || curNs != msg.Namespace {
		return a, nil
	}
	if msg.Err != nil {
		body := "# error: " + msg.Err.Error() + "\n"
		panel.SetContent(render.Content{Raw: body, Display: body}, false)
		return a, nil
	}
	panel.SetContent(msg.Content, false)
	return a, nil
}

func (a App) handleHelmChartRefSet(msg msgs.HelmChartRefSetMsg) (tea.Model, tea.Cmd) {
	if a.config != nil {
		a.config.SetChartRef(msg.Namespace, msg.ReleaseName, msg.ChartRef)
	}
	hc := a.helmClientForFocused()
	if hc == nil {
		a.notify.Add(notify.LevelError, "helm: no client", a.contextFor(nil), "helm")
		return a, nil
	}
	return a, helm.EditValuesCmd(hc, msg.ReleaseName, msg.Namespace)
}

// maybeRefetchValuesAfterEdit dispatches an extra values re-fetch when the
// detail panel was showing a Helm values variant (Values (user) or Values
// (all)) at the moment a `helm-edit-values:` upgrade completed. clearCmd and
// helmCmd are the upstream side-effects of the action result handler that
// must always run; valuesCmd is appended only when all of the early-return
// guards pass. Extracted to flatten the deeply nested chain in Update.
func (a App) maybeRefetchValuesAfterEdit(clearCmd, helmCmd tea.Cmd) (tea.Model, tea.Cmd) {
	if !a.layout.RightPanelVisible() || a.layout.IsLogMode() {
		return a, tea.Batch(clearCmd, helmCmd)
	}
	panel := a.layout.RightPanel()
	if panel == nil {
		return a, tea.Batch(clearCmd, helmCmd)
	}
	mode := panel.Mode()
	if mode != msgs.DetailValues && mode != msgs.DetailValuesAll {
		return a, tea.Batch(clearCmd, helmCmd)
	}
	focused := a.layout.FocusedSplit()
	if focused == nil || focused.Plugin().Name() != "helmreleases" {
		return a, tea.Batch(clearCmd, helmCmd)
	}
	app, valuesCmd := a.reloadDetailPanel()
	return app, tea.Batch(clearCmd, helmCmd, valuesCmd)
}

func (a App) refreshHelmSplits() (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	for i := range a.layout.SplitCount() {
		split := a.layout.SplitAt(i)
		if split != nil && split.Plugin().Name() == "helmreleases" {
			if hc := a.helmClientFor(a.clusterFor(split)); hc != nil {
				opCmd := a.statusBar.StartOperation()
				cmds = append(cmds, opCmd, fetchHelmReleasesCmd(hc, split.Namespace(), a.config.APITimeout()))
			}
		}
	}
	return a, tea.Batch(cmds...)
}

// handleClusterHealth processes a ClusterHealthMsg tick on the Update goroutine.
//
// The status bar exposes a single online indicator, so it reflects the FOCUSED
// pane's cluster only: a tick is displayed when its Context matches the focused
// pane's cluster context. Ticks for other clusters are not displayed but keep
// their own probe loops alive (see below).
//
// Re-arming is 1:1 — a tick re-arms a heartbeat ONLY for the cluster that just
// ticked, preserving the invariant of exactly one heartbeat in flight per
// connected cluster. Re-arming every pane-referenced cluster on every tick (the
// previous behavior) made the in-flight heartbeat count multiply each interval
// once a second context existed: N in-flight ticks each spawned N new ones, so
// the count doubled every interval until CPU/memory blew up. New clusters are
// seeded by their own initialHeartbeatCmd on first connect (startup and
// handleClusterReady), so 1:1 re-arming still covers every cluster without
// fan-out. Heartbeats are tea.Cmds (no goroutine races): all Manager reads
// happen here on the Update goroutine and only the health dial runs off-thread
// inside the returned cmd, against an immutable client handle.
func (a App) handleClusterHealth(msg msgs.ClusterHealthMsg) (tea.Model, tea.Cmd) {
	// Resolve the focused pane's cluster context and only let a matching tick
	// drive the displayed indicator.
	focusedCtx := a.contextFor(a.layout.FocusedSplit())
	tickCtx := msg.Context
	if tickCtx == "" {
		tickCtx = focusedCtx
	}
	if tickCtx == focusedCtx {
		a.statusBar.SetOnline(msg.Online)
	}

	// Re-arm the heartbeat for the ticked cluster only (see heartbeatRearmContexts).
	var cmds []tea.Cmd
	for _, ctxName := range a.heartbeatRearmContexts(tickCtx) {
		cl, _ := a.mgr.Get(ctxName)
		cmds = append(cmds, heartbeatCmd(cl.Context(), cl.Client(), a.config.HeartbeatInterval()))
	}

	// Recompute per-pane offline markers from the (possibly changed) cluster
	// connectivity so a pane recovers its marker automatically once its cluster
	// is connected again, and is flagged when its cluster goes degraded.
	a.syncPaneFooters()
	return a, tea.Batch(cmds...)
}

// heartbeatRearmContexts returns the cluster contexts whose heartbeat should be
// re-armed after a ClusterHealthMsg for tickCtx: exactly the ticked cluster, and
// only while it is still a connected Manager entry. Returning at most one keeps
// the invariant of one heartbeat in flight per cluster — re-arming every
// pane-referenced cluster on every tick multiplied the in-flight heartbeats each
// interval (the CPU/memory blow-up). A torn-down or disconnected cluster returns
// nothing, so its loop ends with no leak; a newly-connected cluster is seeded
// separately by initialHeartbeatCmd, never here.
//
// Note Connected() is client-presence (client != nil && err == nil), not the
// latest health result, so a transiently-offline but still-referenced cluster
// keeps being probed; the loop only ends when SyncRefs tears the cluster down.
func (a App) heartbeatRearmContexts(tickCtx string) []string {
	if cl, ok := a.mgr.Get(tickCtx); ok && cl != nil && cl.Connected() {
		return []string{tickCtx}
	}
	return nil
}

// heartbeatCmd schedules a delayed health check for ctxName's client and reports
// the result tagged with ctxName, so the Update-goroutine handler can decide
// whether it concerns the focused pane's cluster (displayed) or a background one.
func heartbeatCmd(ctxName string, client *k8s.Client, interval time.Duration) tea.Cmd {
	return func() tea.Msg {
		time.Sleep(interval)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		online := k8s.CheckHealth(ctx, client)
		return msgs.ClusterHealthMsg{Context: ctxName, Online: online}
	}
}

// initialHeartbeatCmd fires an immediate health check (no startup delay) tagged
// with ctxName, so the status bar reflects connectivity right after a cluster
// connects (startup global, global switch, or per-pane connect).
func initialHeartbeatCmd(ctxName string, client *k8s.Client) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		online := k8s.CheckHealth(ctx, client)
		return msgs.ClusterHealthMsg{Context: ctxName, Online: online}
	}
}

func readLogLine(ch <-chan string, gen uint64) tea.Cmd {
	return func() tea.Msg {
		line, ok := <-ch
		if !ok {
			return msgs.LogStreamEndedMsg{Gen: gen}
		}
		return msgs.LogLineMsg{Line: line, Gen: gen}
	}
}

// readTermBytes blocks on one chunk from the session's Out channel and wraps
// the result in a message, mirroring readLogLine. A closed channel (the
// session's stream goroutine finished) yields a TermExitMsg carrying the final
// status; otherwise a TermBytesMsg carries the chunk. The session id is read
// once up front so the closure does not retain the *Terminal beyond the read.
func readTermBytes(sess *session.Terminal) tea.Cmd {
	id := sess.ID()
	out := sess.Out()
	return func() tea.Msg {
		chunk, ok := <-out
		if !ok {
			return msgs.TermExitMsg{ID: id, Code: sess.ExitCode(), Err: sess.Err()}
		}
		return msgs.TermBytesMsg{ID: id, Data: chunk}
	}
}

// syncTerminalSizes pushes each live terminal pane's current inner (emulator)
// size to its session so the remote shell reflows to match what is rendered.
// recalcSizes has already set the pane sizes via SetSize; this only forwards
// them to the sessions. Safe to call whenever layout geometry may have changed
// (window resize, add/close split, zoom toggle). Sessions with no matching pane
// (or zero-sized hidden panes) are skipped to avoid sending a degenerate size.
func (a App) syncTerminalSizes() {
	for id, sess := range a.terminals {
		w, h, ok := a.layout.TerminalPaneInnerSize(id)
		if !ok || w <= 0 || h <= 0 {
			continue
		}
		sess.Resize(w, h)
	}
}

// closeTerminalSession tears down a terminal session and removes it from the
// registry. The pane itself is removed from the layout by the caller (the
// shared closeFocusedSplit path).
//
// Lifecycle cleanup, keyed off the session's terminalMeta:
//   - node-debug: the created debug pod is deleted best-effort in a goroutine
//     with a bounded context, so a slow API server never blocks the UI.
//   - ephemeral pod-debug: ephemeral containers cannot be removed from a pod, so
//     there is nothing to delete; the user-facing note is surfaced separately at
//     the call site (close path) since this helper has no App-return.
//
// The value receiver is intentional: a.terminals/a.termCleanup are maps, which
// are reference types, so delete() mutates the shared backing map even though
// App is copied by value. No App-return / struct pointer is needed.
func (a App) closeTerminalSession(id string) {
	if sess, ok := a.terminals[id]; ok {
		// Unregister first so pane close stays instant and any late
		// TermBytesMsg/TermExitMsg for this id is dropped (the handlers key off
		// a.terminals). Then ask the remote shell to exit on its own and hard
		// Close() in a detached goroutine: TerminateGracefully blocks up to the
		// grace period, which must never stall the UI.
		delete(a.terminals, id)
		go func() {
			sess.TerminateGracefully(session.DefaultGraceTimeout)
			sess.Close()
		}()
	}
	if meta, ok := a.termCleanup[id]; ok {
		// Cancel any in-flight pre-flight (GET/patch/create + wait-Running) so
		// closing a placeholder mid-flight does not leave API calls running up to
		// the 60s wait. For node-debug this also triggers PrepareNodeDebug's own
		// ctx-cancel cleanup, deleting a pod it may have already created but not
		// yet reported via DebugReadyMsg.
		if meta.preflightCancel != nil {
			meta.preflightCancel()
		}
		// Delete the created node-debug pod when its name is already known (the
		// pre-flight reported it via DebugReadyMsg). When the name is not yet known
		// the cancel above hands cleanup to PrepareNodeDebug's own teardown.
		if meta.nodeDebug {
			deleteNodeDebugPodAsync(meta.client, meta.podName, meta.namespace)
		}
		delete(a.termCleanup, id)
	}
}

// handleDebugReady binds the result of an async debug pre-flight to the
// placeholder pane that openDebugTerminal/openNodeDebugTerminal already put on
// screen (matched by msg.ID).
//
//   - On error: surface it on the status bar and tear down the placeholder pane
//     and its cleanup metadata. For node-debug, the pre-flight already best-effort
//     deletes any half-created pod, so there is nothing left to delete here.
//   - On success: build the attach executor, start the background session, update
//     the cleanup metadata (node-debug pod name/namespace from the pre-flight),
//     push the current size, and kick off the byte-pump.
//
// If the placeholder pane is gone (user closed it during the pre-flight), the
// result is dropped; for node-debug the pod is deleted so it does not leak.
func (a App) handleDebugReady(msg msgs.DebugReadyMsg) (tea.Model, tea.Cmd) {
	tp, found := a.layout.TerminalPaneByID(msg.ID)

	if msg.Err != nil {
		a.notify.Add(notify.LevelError, "debug: "+msg.Err.Error(), a.contextFor(nil), "debug")
		// Remove the placeholder pane (if still present) and its metadata. Cancel
		// the pre-flight context so its goroutine bookkeeping (parent is
		// Background) is released rather than leaked for the process lifetime.
		if meta, ok := a.termCleanup[msg.ID]; ok && meta.preflightCancel != nil {
			meta.preflightCancel()
		}
		delete(a.termCleanup, msg.ID)
		if found {
			a = a.failTerminalPane(tp, "debug failed: "+msg.Err.Error())
		}
		return a, nil
	}

	// The user may have closed the placeholder pane while the pre-flight ran.
	if !found {
		if meta, ok := a.termCleanup[msg.ID]; ok && meta.preflightCancel != nil {
			meta.preflightCancel()
		}
		if msg.NodeMode {
			// Delete the created pod using the client carried IN the message, NOT
			// from termCleanup: closeTerminalSession may have already run (when the
			// pane was closed after the pod was created but before this msg landed)
			// and removed the termCleanup entry, so depending on that map would
			// leak the pod in the cluster. The message-carried client closes that
			// window. Fall back to the metadata client if the assertion fails.
			client, _ := msg.Client.(*k8s.Client)
			if client == nil {
				if meta, ok := a.termCleanup[msg.ID]; ok {
					client = meta.client
				}
			}
			deleteNodeDebugPodAsync(client, msg.PodName, msg.Namespace)
		}
		delete(a.termCleanup, msg.ID)
		return a, nil
	}

	meta := a.termCleanup[msg.ID]
	exec, err := a.attachExecutorFn(meta.client, msg.PodName, msg.ContainerName, msg.Namespace)
	if err != nil {
		a.notify.Add(notify.LevelError, "debug: "+err.Error(), a.contextFor(nil), "debug")
		// The attach failed but the pre-flight already created the node-debug
		// pod; delete it best-effort so it does not leak. Read the client off
		// the cleanup metadata BEFORE removing it.
		if msg.NodeMode {
			deleteNodeDebugPodAsync(meta.client, msg.PodName, msg.Namespace)
		}
		// The pre-flight has landed; release its context bookkeeping.
		if meta.preflightCancel != nil {
			meta.preflightCancel()
		}
		delete(a.termCleanup, msg.ID)
		a = a.failTerminalPane(tp, "debug failed: "+err.Error())
		return a, nil
	}

	// Fill in the now-known pod identity so close/quit can delete a node pod.
	// The pre-flight has landed, so drop its cancel func (a later close deletes
	// the pod by name instead of cancelling a finished pre-flight).
	meta.podName = msg.PodName
	meta.namespace = msg.Namespace
	// The pre-flight has landed; cancel its context to release the goroutine
	// bookkeeping (parent is Background, otherwise never cancelled), then drop the
	// func so a later close deletes the pod by name instead of cancelling a
	// finished pre-flight. CancelFunc is idempotent, so this is safe.
	if meta.preflightCancel != nil {
		meta.preflightCancel()
		meta.preflightCancel = nil
	}
	a.termCleanup[msg.ID] = meta

	sess := session.Start(exec, msg.ID)
	a.terminals[msg.ID] = sess
	startReplyPump(tp, sess)

	// syncTerminalSizes pushes the session's inner size: the session is now in
	// a.terminals and the placeholder pane is already in the layout, so this covers
	// the just-bound session — no extra direct Resize is needed.
	a.syncTerminalSizes()

	return a, readTermBytes(sess)
}

// startReplyPump forwards the emulator's query replies (responses to DA/DSR/
// DECRQM, etc.) back to the shell's stdin, and tears the drain down when the
// session ends. Without this, a full-screen program (vim, less, top) that queries
// the terminal makes vt.Emulator.Write block on its unbuffered reply pipe — and
// since the emulator is fed on the Bubble Tea update goroutine, that hangs the
// whole UI. The drain touches only the emulator's reply pipe, so it is safe
// alongside Write/Render on the UI goroutine; teardown closes the reply pipe (not
// the emulator) so it never races x/vt's unsynchronized closed flag.
//
// replyPumpExited, when non-nil, is invoked (once per goroutine) as each of the
// two pump goroutines returns. It exists solely so leak tests can observe the
// REAL goroutines exit; production leaves it nil. It is snapshotted here on the
// calling goroutine and captured by the pump goroutines so they never read the
// shared global concurrently (which lingering pumps from earlier tests otherwise
// would, racing a later test's assignment).
func startReplyPump(tp *ui.TerminalPane, sess *session.Terminal) {
	onExit := replyPumpExited
	go func() {
		if onExit != nil {
			defer onExit()
		}
		r := tp.ReplyReader()
		buf := make([]byte, 4096)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				_, _ = sess.Write(buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()
	go func() {
		if onExit != nil {
			defer onExit()
		}
		<-sess.Done()
		tp.StopReplies()
	}()
}

// replyPumpExited is a test-only hook fired as each startReplyPump goroutine
// returns. nil in production. Set/cleared by a test before/after it drives the
// open path; startReplyPump snapshots it synchronously on the caller goroutine.
var replyPumpExited func()

// failTerminalPane handles a placeholder pane whose debug pre-flight/attach
// failed before any session started. It removes the pane via the normal
// close-split path, but tolerates the last-split case: CloseCurrentSplit refuses
// to empty the layout, which would otherwise strand the placeholder ("starting
// debug container…") on screen with no session — it never exits, so the usual
// teardown keys (<prefix> x / ctrl+w) find no shell and the pane is unclosable.
// To keep it closeable, mark the placeholder as exited with a note explaining
// why, so the user can dismiss it through the normal exited-pane path.
func (a App) failTerminalPane(tp *ui.TerminalPane, note string) App {
	if a.layout.SplitCount() <= 1 {
		// Last split: removeTerminalPane would skip CloseCurrentSplit and leave an
		// orphaned, session-less placeholder. Freeze it into the exited state so it
		// behaves like any other dead pane (closeable, shows the failure note).
		tp.MarkExited(1)
		tp.SetExitNote(note)
		a.keyTrie.Reset()
		a.syncTerminalSizes()
		return a
	}
	return a.removeTerminalPane(tp)
}

// removeTerminalPane removes a terminal pane from the layout, tolerating the
// last-split case (CloseCurrentSplit signals quit rather than emptying the
// layout). It does not touch sessions/metadata — callers own that.
func (a App) removeTerminalPane(tp *ui.TerminalPane) App {
	if a.layout.FocusedPane() != tp {
		// Focus the pane so CloseCurrentSplit removes the right one. Scan for it by
		// identity and jump straight to its index with FocusSplitAt, rather than
		// stepping FocusNext until focus lands on it.
		for i := 0; i < a.layout.SplitCount(); i++ {
			if a.layout.PaneAtIdx(i) == tp {
				// FocusSplitAt self-reconciles to resources; CloseCurrentSplit
				// below does the same for the survivor, so no follow-up focus
				// call is needed on this path.
				a.layout.FocusSplitAt(i)
				break
			}
		}
	}
	if a.layout.SplitCount() > 1 {
		a.layout.CloseCurrentSplit()
	}
	a.keyTrie.Reset()
	a.syncTerminalSizes()
	return a
}

// ephemeralCloseNote returns a one-line note when the terminal with id is an
// ephemeral pod-debug session, informing the user that the ephemeral container
// cannot be removed (a k8s limitation). It returns "" for all other terminals.
// Callers post it via the status bar and/or the pane's exit note. Read it BEFORE
// closeTerminalSession, which deletes the metadata entry.
func (a App) ephemeralCloseNote(id string) string {
	if meta, ok := a.termCleanup[id]; ok && meta.ephemeral {
		return "debug: ephemeral container left on " + meta.podName + " (k8s cannot remove it)"
	}
	return ""
}

// shutdownTerminals tears down all embedded terminal sessions and best-effort
// deletes every node-debug pod before the program exits. Bubble Tea's quit is
// asynchronous, so this is invoked synchronously on the quit path(s) (ctrl+c and
// the layered quit command) before returning tea.Quit.
//
// Tradeoff: cleanup must not hang quit if the API server is slow. Each node-pod
// delete runs in its own goroutine under a bounded nodeDebugDeleteTimeout (3s)
// context. A pod whose delete did not complete in time is left to the
// kubelet/GC (the debug pod has RestartPolicy Never and no controller, so it is
// harmless and short-lived). The worst-case main-thread block is the grace
// ceiling (DefaultGraceTimeout, ~400ms) followed by the node-debug wg.Wait()
// ceiling (nodeDebugDeleteTimeout, 3s) — up to ~3.4s when node-debug pods are
// present, and just the grace ceiling otherwise.
//
// Before the hard close, every live shell is asked to exit gracefully
// (TerminateGracefully) concurrently under a SINGLE shared grace budget, so
// quitting N panes costs ~one grace period rather than N × grace. The hard
// Close() loop AFTER the grace phase is itself cheap (Close only cancels the
// stream context and closes the stdin pipe). A short-lived graceful goroutine
// may keep draining its control-byte burst (~150ms) after this function returns;
// that overrun is harmless (see the shared-budget comment below) and each such
// goroutine exits promptly once the hard Close() fires.
//
// The value receiver is intentional: a.terminals/a.termCleanup are maps
// (reference types), so the delete() calls below mutate the shared backing maps
// even though App is copied by value. No App-return / struct pointer is needed.
func (a App) shutdownTerminals() {
	var wg sync.WaitGroup
	for id, meta := range a.termCleanup {
		// Cancel any in-flight pre-flight so a slow create/wait does not run on
		// after quit (and, for node-debug, so PrepareNodeDebug's ctx-cancel
		// cleanup deletes a not-yet-reported pod).
		if meta.preflightCancel != nil {
			meta.preflightCancel()
		}
		if meta.nodeDebug && meta.client != nil && meta.podName != "" {
			client, pod, ns := meta.client, meta.podName, meta.namespace
			wg.Add(1)
			go func() {
				defer wg.Done()
				ctx, cancel := context.WithTimeout(context.Background(), nodeDebugDeleteTimeout)
				defer cancel()
				_ = k8s.DeleteNodeDebugPod(ctx, client, pod, ns)
			}()
		}
		delete(a.termCleanup, id)
	}

	// Best-effort ask every live shell to exit on its own BEFORE the hard close,
	// concurrently and under a SINGLE shared grace budget: fan out
	// TerminateGracefully and wait for them all with one bounded select, so
	// quitting N panes costs ~one grace period, not N × grace. Runs alongside the
	// node-debug pod deletes fired above.
	//
	// The outer time.After(DefaultGraceTimeout) is the HARD ceiling on the main
	// thread: it blocks here for at most one grace period (~400ms), whether or not
	// the goroutines have finished. Each goroutine's TerminateGracefully spends up
	// to ~150ms bursting control bytes before its own time.After(grace), so a
	// goroutine can run up to ~grace+150ms total and may outlive this function by
	// up to ~150ms, draining its remaining burst sleeps. That overrun is harmless:
	// the hard Close() below closes the stdin pipe and cancels the context, which
	// ends the stream goroutine and closes done; the grace goroutine's next select
	// then observes done (or its Write hits the closed pipe), so it exits promptly.
	// It touches only the session it owns.
	if len(a.terminals) > 0 {
		var graceWg sync.WaitGroup
		for _, sess := range a.terminals {
			graceWg.Add(1)
			go func() {
				defer graceWg.Done()
				sess.TerminateGracefully(session.DefaultGraceTimeout)
			}()
		}
		graceDone := make(chan struct{})
		go func() { graceWg.Wait(); close(graceDone) }()
		select {
		case <-graceDone:
		case <-time.After(session.DefaultGraceTimeout):
		}

		for id, sess := range a.terminals {
			sess.Close()
			delete(a.terminals, id)
		}
	}

	// Bounded wait so a slow API server never hangs quit.
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(nodeDebugDeleteTimeout):
	}
}

func (a App) stopLogStream() App {
	if a.logStreamCancel != nil {
		a.logStreamCancel()
		a.logStreamCancel = nil
	}
	a.logCh = nil
	a.logStreamGen++ // reject stale LogLineMsg from cancelled stream
	return a
}

// isLoggablePlugin returns true for plugins that support log streaming.
func isLoggablePlugin(name string) bool {
	return name == "pods" || name == "containers"
}

// syncLogPanel reconciles log panel state with the focused plugin.
// If log mode is active and the focused plugin is not loggable, it stops
// the stream and puts LogView into unavailable state.
// If the plugin is loggable and LogView was unavailable, it restarts the stream.
func (a App) syncLogPanel() (App, tea.Cmd) {
	if !a.layout.IsLogMode() {
		return a, nil
	}
	focused := a.layout.FocusedSplit()
	if focused == nil {
		return a, nil
	}
	lv := a.layout.LogView()
	if lv == nil {
		return a, nil
	}

	if !isLoggablePlugin(focused.Plugin().Name()) {
		a = a.stopLogStream()
		lv.ClearAndRestart()
		lv.SetUnavailable(true)
		return a, nil
	}

	if lv.IsUnavailable() {
		_, _, ok := a.focusedSelection()
		if !ok {
			// No selection yet — keep unavailable until objects arrive
			return a, nil
		}
		lv.SetUnavailable(false)
		lv.ClearAndRestart()
		model, cmd := a.restartLogForCursor()
		return model.(App), cmd
	}
	return a, nil
}

func (a App) logDebounceCmd() tea.Cmd {
	seq := a.logDebounceSeq
	return func() tea.Msg {
		time.Sleep(250 * time.Millisecond)
		return msgs.LogDebounceFiredMsg{Seq: seq}
	}
}

func (a App) startLogStream(podName, containerName, namespace string, opts k8s.LogOptions) (App, tea.Cmd) {
	a = a.stopLogStream()
	a.logStreamGen++
	// Capture the focused cluster's client into a local BEFORE building the
	// closure so the stream keeps the cluster client it captured at open time
	// even if the pane is later switched to another context.
	client := a.clientForFocused()
	if client == nil {
		return a, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.logStreamCancel = cancel
	gen := a.logStreamGen
	opCmd := a.statusBar.StartOperation()
	return a, tea.Batch(opCmd, func() tea.Msg {
		ch, err := k8s.StreamLogs(ctx, client, podName, containerName, namespace, opts)
		if err != nil {
			cancel()
			return msgs.LogStreamReadyMsg{Gen: gen, Err: err}
		}
		return msgs.LogStreamReadyMsg{Ch: ch, Gen: gen}
	})
}

// detailKey returns a composite key for the currently focused split's
// selected resource. The format mirrors resourcelist.go's SetObjects key:
//
//	all-namespaces: Kind/Namespace/Name
//	single namespace: Kind/Name
//
// Returns "" if no split is focused or no resource is selected.
func (a App) detailKey() string {
	focused := a.layout.FocusedSplit()
	if focused == nil {
		return ""
	}
	sel := focused.Selected()
	if sel == nil {
		return ""
	}
	if focused.Namespace() == "" {
		return sel.GetKind() + "/" + sel.GetNamespace() + "/" + sel.GetName()
	}
	return sel.GetKind() + "/" + sel.GetName()
}
