package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/aohoyd/aku/internal/config"
	"github.com/aohoyd/aku/internal/helm"
	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/layout"
	"github.com/aohoyd/aku/internal/msgs"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/plugins/apiresources"
	"github.com/aohoyd/aku/internal/plugins/generic"
	"github.com/aohoyd/aku/internal/portforward"
	"github.com/aohoyd/aku/internal/render"
	"github.com/aohoyd/aku/internal/ui"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var eventsGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "events"}

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
)

// App is the root Bubbletea model.
type App struct {
	k8sClient  *k8s.Client
	store      *k8s.Store
	bindingSet *config.BindingSet
	keyTrie          *config.KeyTrie
	trieContextType  string
	trieResourceName string
	layout           layout.Layout
	statusBar  ui.StatusBar
	width      int
	height     int

	// envResolved toggles resolved environment variable display in describe view
	envResolved bool

	// Overlay components
	activeOverlay       overlay
	resourcePicker      ui.ResourcePicker
	nsPicker            ui.NsPicker
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
	helmClient helm.Client

	// Port-forward
	pfRegistry *portforward.Registry
	config     *config.Config
	pendingRun        *config.RunConfig             // external command waiting for confirm
	pendingBulkDelete []*unstructured.Unstructured // bulk delete targets waiting for confirm
	pendingDelete     *unstructured.Unstructured   // single delete target waiting for confirm

	// Log stream
	logStreamCancel context.CancelFunc
	logCh           <-chan string
	logDebounceSeq  uint64
	logStreamGen    uint64
}

// New creates a new App with all dependencies.
func New(client *k8s.Client, store *k8s.Store, keymap *config.Keymap, cfg *config.Config, pfRegistry *portforward.Registry, helmClient helm.Client) App {
	bs := keymap.BindingSet()
	defaultTimeRange := cfg.LogDefaultTimeRange()
	defaultSinceSeconds, ok := ui.LookupTimePreset(defaultTimeRange)
	if !ok {
		defaultTimeRange = "15m"
		defaultSinceSeconds = 900
	}
	a := App{
		k8sClient:          client,
		store:              store,
		bindingSet:         bs,
		keyTrie:            bs.TrieFor("resources", ""),
		layout:             layout.New(80, 24, cfg.LogBufferSize(), defaultTimeRange, defaultSinceSeconds),
		statusBar:          ui.NewStatusBar(80),
		resourcePicker:     ui.NewResourcePicker(40, 20),
		nsPicker:           ui.NewNsPicker(40, 20),
		searchBar:          ui.NewSearchBar(80),
		helpOverlay:        ui.NewHelpOverlay(80, 24),
		helmClient:         helmClient,
		pfRegistry:         pfRegistry,
		portForwardOverlay: ui.NewPortForwardOverlay(40, 20),
		setImageOverlay:    ui.NewSetImageOverlay(40, 20),
		helmRollbackOverlay: ui.NewHelmRollbackOverlay(40, 20),
		config:              cfg,
		chartInputOverlay:   ui.NewChartInputOverlay(40, 20),
		containerPicker:     ui.NewContainerPicker(40, 20),
		timeRangePicker:     ui.NewTimeRangePicker(40, 20),
		scaleOverlay:        ui.NewScaleOverlay(40, 20),
	}

	// Populate fuzzy picker with all registered plugins
	allPlugins := plugin.All()
	entries := make([]ui.PluginEntry, len(allPlugins))
	for i, p := range allPlugins {
		entries[i] = ui.PluginEntry{Name: p.Name(), ShortName: p.ShortName()}
	}
	a.resourcePicker.SetPlugins(entries)

	// Add initial pods split
	initialNs := "default"
	if client != nil {
		initialNs = client.Namespace
	}

	if p, ok := plugin.ByName("pods"); ok {
		a.layout.AddSplit(p, initialNs)
		if client != nil && store != nil {
			store.Subscribe(p.GVR(), initialNs)
		}
		a.keyTrie = bs.TrieFor("resources", p.Name())
	}

	// Set initial status bar hints
	a.statusBar.SetHints(a.currentHints())

	return a
}

func (a App) Init() tea.Cmd {
	if a.k8sClient == nil {
		return nil
	}
	return func() tea.Msg {
		resources, err := k8s.DiscoverAPIResources(a.k8sClient.Typed)
		return k8s.APIResourcesDiscoveredMsg{Resources: resources, Err: err}
	}
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
		return a, nil

	case tea.KeyPressMsg:
		// Ctrl-c always quits immediately
		if msg.String() == "ctrl+c" {
			return a, tea.Quit
		}

		// ctrl+w closes current panel / overlay
		if msg.String() == "ctrl+w" {
			if a.activeOverlay != overlayNone {
				a.activeOverlay = overlayNone
				a.pendingRun = nil
				a.pendingBulkDelete = nil
				a.pendingDelete = nil
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

	case msgs.ResourcePickedMsg:
		// Handle resource picker submissions
		return a.handleResourcePickerCommand(msg.Command)

	case msgs.NamespaceSelectedMsg:
		return a.handleNamespaceSwitch(msg.Namespace)

	case msgs.NamespacesLoadedMsg:
		if msg.Err != nil {
			a.statusBar.SetError("namespaces: " + msg.Err.Error())
		} else if a.activeOverlay == overlayNsPicker {
			a.nsPicker.SetNamespaces(msg.Namespaces)
		}
		return a, nil

	case msgs.ConfirmResultMsg:
		a.activeOverlay = overlayNone
		if a.pendingBulkDelete != nil {
			objs := a.pendingBulkDelete
			a.pendingBulkDelete = nil
			if msg.Action == msgs.ConfirmYes || msg.Action == msgs.ConfirmForce {
				if focused := a.layout.FocusedSplit(); focused != nil {
					focused.ClearSelection()
				}
				return a.executeBulkDelete(objs, msg.Action == msgs.ConfirmForce)
			}
			return a, nil
		}
		if a.pendingDelete != nil {
			obj := a.pendingDelete
			a.pendingDelete = nil
			if msg.Action == msgs.ConfirmYes || msg.Action == msgs.ConfirmForce {
				return a.executeSingleDelete(obj, msg.Action == msgs.ConfirmForce)
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
		return a, nil

	case msgs.ActionResultMsg:
		if msg.Err != nil {
			a.statusBar.SetError(msg.Err.Error())
		} else {
			a.statusBar.SetError("") // clear any previous error
		}
		if strings.HasPrefix(msg.ActionID, "helm-") {
			return a.refreshHelmSplits()
		}
		return a, nil

	case k8s.ResourceUpdatedMsg:
		// Find plugin by GVR and update matching splits
		if p, ok := plugin.ByGVR(msg.GVR); ok {
			objs := a.store.List(msg.GVR, msg.Namespace)
			a.layout.UpdateSplitObjects(p, msg.Namespace, objs)
		}
		// Refresh drill-down child views when relevant resources update
		a.refreshDrillDownSplits(msg.GVR, msg.Namespace)
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
			a = a.reloadDetailPanel()
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

	case msgs.HelmChartRefSetMsg:
		return a.handleHelmChartRefSet(msg)

	case msgs.HelmReleasesRefreshMsg:
		return a.refreshHelmSplits()

	case msgs.PortForwardStartedMsg:
		if msg.Err != nil {
			a.statusBar.SetError("port-forward failed: " + msg.Err.Error())
		} else {
			a.statusBar.SetError(fmt.Sprintf("port-forward active: localhost:%d", msg.LocalPort))
		}
		a = a.syncIndicators()
		// Refresh portforwards view if visible
		for i := range a.layout.SplitCount() {
			split := a.layout.SplitAt(i)
			if split != nil && split.Plugin().Name() == "portforwards" {
				if sp, ok := split.Plugin().(plugin.SelfPopulating); ok {
					split.SetObjects(sp.Objects())
				}
			}
		}
		return a, nil

	case msgs.PortForwardStoppedMsg:
		a = a.syncIndicators()
		return a, nil

	case msgs.WarningMsg:
		a.statusBar.SetWarning(msg.Text)
		return a, nil

	case msgs.ErrMsg:
		a.statusBar.SetError(msg.Error())
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
			a.statusBar.SetError("log stream: " + msg.Err.Error())
		}
		a.logCh = nil
		return a, nil

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
	}

	return a, nil
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

func (a App) refreshDetailPanelOpts(preserve bool) App {
	if a.layout.IsLogMode() {
		// In log mode, refreshes are handled by the log stream, not the detail panel
		return a
	}
	if !a.layout.RightPanelVisible() {
		return a
	}
	focused := a.layout.FocusedSplit()
	if focused == nil {
		return a
	}
	sel := focused.Selected()
	if sel == nil {
		return a
	}
	panel := a.layout.RightPanel()
	refresh := !preserve
	switch panel.Mode() {
	case msgs.DetailYAML:
		content, err := focused.Plugin().YAML(sel)
		if err == nil {
			panel.SetContent(content, refresh)
		}
	case msgs.DetailDescribe:
		var content render.Content
		var err error
		if a.envResolved {
			if unc, ok := focused.Plugin().(plugin.Uncoverable); ok {
				content, err = unc.DescribeUncovered(context.Background(), sel)
				if err != nil {
					content, err = focused.Plugin().Describe(context.Background(), sel)
				}
			}
		}
		if content.Raw == "" && err == nil {
			content, err = focused.Plugin().Describe(context.Background(), sel)
		}
		if err == nil && content.Raw != "" {
			if a.store != nil && focused.Plugin().GVR() != eventsGVR {
				kind, ok := k8s.KindForGVR(focused.Plugin().GVR())
				if ok {
					ns := sel.GetNamespace()
					if ns == "" {
						ns = focused.Namespace()
					}
					a.store.Subscribe(eventsGVR, ns)
					allEvents := a.store.List(eventsGVR, ns)
					content = content.Append(render.RenderEvents(allEvents, kind, sel.GetName(), ns))
				}
			}
			panel.SetContent(content, refresh)
		}
	case msgs.DetailLogs:
		// Log mode: handled by LogView + streaming, not by detail panel
		if a.layout.IsLogMode() {
			return a
		}
		msg := "Log streaming not yet implemented"
		panel.SetContent(render.Content{Raw: msg, Display: msg}, refresh)
	}
	return a
}

// refreshDetailPanel resets scroll to top — used after cursor navigation, tab switch, focus change.
func (a App) refreshDetailPanel() App {
	return a.refreshDetailPanelOpts(false)
}

// reloadDetailPanel preserves scroll position — used for background auto-reload and manual ctrl+r.
func (a App) reloadDetailPanel() App {
	return a.refreshDetailPanelOpts(true)
}

// refreshDetailPanelOrLog wraps refreshDetailPanel for cursor navigation.
// In log mode it debounces a stream restart; otherwise it delegates to
// refreshDetailPanel and returns a nil cmd.
func (a App) refreshDetailPanelOrLog() (App, tea.Cmd) {
	if !a.layout.IsLogMode() {
		return a.refreshDetailPanel(), nil
	}
	a = a.stopLogStream()
	if lv := a.layout.LogView(); lv != nil {
		lv.ClearAndRestart()
	}
	a.logDebounceSeq++
	return a, a.logDebounceCmd()
}

// restartLogForCursor resolves the pod/container from the current cursor
// and starts a log stream. Always defaults to the first container.
// Used by both the debounce handler and startLogViewForSelected.
func (a App) restartLogForCursor() (tea.Model, tea.Cmd) {
	focused, selected, ok := a.focusedSelection()
	if !ok {
		return a, nil
	}
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
			a.statusBar.SetError("no containers found")
			return a, nil
		}
		containerName = containers[0]
	}

	lv := a.layout.LogView()
	lv.SetContainers(containers)
	lv.SetActiveContainer(containerName)

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
		indicator += ui.ZoomIndicatorStyle.Render("⤢ ")
	}
	if a.pfRegistry != nil && a.pfRegistry.Count() > 0 {
		indicator += ui.PortForwardIndicatorStyle.Render(fmt.Sprintf("PF:%d ", a.pfRegistry.Count()))
	}
	a.statusBar.SetIndicator(indicator)
	return a
}

func (a App) handleAPIResourcesDiscovered(msg k8s.APIResourcesDiscoveredMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil && len(msg.Resources) == 0 {
		a.statusBar.SetError("api discovery: " + msg.Err.Error())
		return a, nil
	}
	if msg.Err != nil {
		a.statusBar.SetError("api discovery partial: " + msg.Err.Error())
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
	allPlugins := plugin.All()
	entries := make([]ui.PluginEntry, len(allPlugins))
	for i, p := range allPlugins {
		entries[i] = ui.PluginEntry{Name: p.Name(), ShortName: p.ShortName()}
	}
	a.resourcePicker.SetPlugins(entries)

	// If any split is currently showing api-resources, update its objects
	for i := range a.layout.SplitCount() {
		split := a.layout.SplitAt(i)
		if split != nil && split.Plugin().Name() == "api-resources" {
			if sp, ok := split.Plugin().(plugin.SelfPopulating); ok {
				split.SetObjects(sp.Objects())
			}
		}
	}

	return a, nil
}

// toggleZoomDetailAndSync toggles detail zoom and updates the indicator.
func (a App) toggleZoomDetailAndSync() App {
	a.layout.ToggleZoomDetail()
	return a.syncIndicators()
}

// toggleZoomSplitAndSync toggles split zoom and updates the indicator.
func (a App) toggleZoomSplitAndSync() App {
	a.layout.ToggleZoomSplit()
	return a.syncIndicators()
}

func (a App) View() tea.View {
	body := a.layout.View()

	var main string
	if a.layout.EffectiveZoom() == layout.ZoomDetail {
		main = body
	} else {
		statusBar := a.statusBar.View()
		main = lipgloss.JoinVertical(lipgloss.Left, body, statusBar)
	}

	switch a.activeOverlay {
	case overlayResourcePicker:
		if v := a.resourcePicker.View(); v != "" {
			main = ui.PlaceOverlay(a.width, a.height, main, v)
		}
	case overlayNsPicker:
		if v := a.nsPicker.View(); v != "" {
			main = ui.PlaceOverlay(a.width, a.height, main, v)
		}
	case overlayPortForward:
		if v := a.portForwardOverlay.View(); v != "" {
			main = ui.PlaceOverlay(a.width, a.height, main, v)
		}
	case overlaySetImage:
		if v := a.setImageOverlay.View(); v != "" {
			main = ui.PlaceOverlay(a.width, a.height, main, v)
		}
	case overlayHelmRollback:
		if v := a.helmRollbackOverlay.View(); v != "" {
			main = ui.PlaceOverlay(a.width, a.height, main, v)
		}
	case overlayChartInput:
		if v := a.chartInputOverlay.View(); v != "" {
			main = ui.PlaceOverlay(a.width, a.height, main, v)
		}
	case overlayConfirm:
		if v := a.confirmDialog.View(); v != "" {
			main = ui.PlaceOverlay(a.width, a.height, main, v)
		}
	case overlayHelp:
		if v := a.helpOverlay.View(); v != "" {
			main = ui.PlaceOverlay(a.width, a.height, main, v)
		}
	case overlayContainerPicker:
		if v := a.containerPicker.View(); v != "" {
			main = ui.PlaceOverlay(a.width, a.height, main, v)
		}
	case overlayTimeRange:
		if v := a.timeRangePicker.View(); v != "" {
			main = ui.PlaceOverlay(a.width, a.height, main, v)
		}
	case overlayScale:
		if v := a.scaleOverlay.View(); v != "" {
			main = ui.PlaceOverlay(a.width, a.height, main, v)
		}
	}

	return tea.View{
		Content:   main,
		AltScreen: true,
	}
}

// refreshDrillDownSplit re-runs the parent's DrillDown to get fresh filtered
// children for a single split that is currently in a drill-down view.
func (a App) refreshDrillDownSplit(split *ui.ResourceList) {
	snap := split.ParentSnap()
	if snap == nil {
		return
	}
	drillable, ok := snap.Plugin.(plugin.DrillDowner)
	if !ok {
		return
	}

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
		parentNs := split.Namespace()
		if snap.Plugin.IsClusterScoped() {
			parentNs = ""
		}
		for _, obj := range a.store.List(snap.Plugin.GVR(), parentNs) {
			if string(obj.GetUID()) == snap.ParentUID {
				parentObj = obj
				break
			}
		}
	}

	if parentObj == nil {
		return
	}
	_, children := drillable.DrillDown(parentObj)
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
		// Only refresh if the updated GVR matches this split's current child view
		if split.Plugin().GVR() != updatedGVR {
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
	if a.helmClient == nil {
		a.statusBar.SetError("helm: no client")
		return a, nil
	}
	return a, func() tea.Msg {
		if err := a.helmClient.Rollback(msg.ReleaseName, msg.Namespace, msg.Revision); err != nil {
			return msgs.ActionResultMsg{Err: err}
		}
		return msgs.ActionResultMsg{ActionID: "helm-rollback:" + msg.ReleaseName}
	}
}

func (a App) handleHelmChartRefSet(msg msgs.HelmChartRefSetMsg) (tea.Model, tea.Cmd) {
	if a.config != nil {
		a.config.SetChartRef(msg.Namespace, msg.ReleaseName, msg.ChartRef)
	}
	if a.helmClient == nil {
		a.statusBar.SetError("helm: no client")
		return a, nil
	}
	return a, helm.EditValuesCmd(a.helmClient, msg.ReleaseName, msg.Namespace)
}

func (a App) refreshHelmSplits() (tea.Model, tea.Cmd) {
	for i := range a.layout.SplitCount() {
		split := a.layout.SplitAt(i)
		if split != nil && split.Plugin().Name() == "helmreleases" {
			if r, ok := split.Plugin().(plugin.Refreshable); ok {
				r.Refresh(split.Namespace())
			}
			if sp, ok := split.Plugin().(plugin.SelfPopulating); ok {
				split.SetObjects(sp.Objects())
			}
		}
	}
	return a, nil
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

func (a App) stopLogStream() App {
	if a.logStreamCancel != nil {
		a.logStreamCancel()
		a.logStreamCancel = nil
	}
	a.logCh = nil
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
	if a.k8sClient == nil {
		return a, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.logStreamCancel = cancel
	ch, err := k8s.StreamLogs(ctx, a.k8sClient, podName, containerName, namespace, opts)
	if err != nil {
		cancel()
		a.statusBar.SetError("logs: " + err.Error())
		return a, nil
	}
	a.logCh = ch
	return a, readLogLine(ch, a.logStreamGen)
}
