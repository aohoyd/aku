package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/aohoyd/aku/internal/config"
	"github.com/aohoyd/aku/internal/helm"
	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/msgs"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/portforward"
	"github.com/aohoyd/aku/internal/ui"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// focusedSelection returns the focused split and its selected object.
// Returns false if either is nil.
func (a App) focusedSelection() (*ui.ResourceList, *unstructured.Unstructured, bool) {
	focused := a.layout.FocusedSplit()
	if focused == nil {
		return nil, nil, false
	}
	selected := focused.Selected()
	if selected == nil {
		return focused, nil, false
	}
	return focused, selected, true
}

func (a App) executeCommand(command string) (tea.Model, tea.Cmd) {
	switch {
	// Navigation: goto-<resource>
	case strings.HasPrefix(command, "goto-"):
		resourceName := strings.TrimPrefix(command, "goto-")
		return a.handleGoto(resourceName, "")

	// Split: split-<resource>
	case strings.HasPrefix(command, "split-"):
		resourceName := strings.TrimPrefix(command, "split-")
		return a.handleSplit(resourceName)

	// Sort: sort-<COLUMN>
	case strings.HasPrefix(command, "sort-"):
		column := strings.TrimPrefix(command, "sort-")
		return a.handleSort(column)

	// Views
	case command == "view-yaml":
		return a.handleView(msgs.DetailYAML)
	case command == "view-describe":
		return a.handleView(msgs.DetailDescribe)
	case command == "view-logs":
		return a.handleView(msgs.DetailLogs)
	case command == "view-yaml-focused":
		return a.handleViewFocused(msgs.DetailYAML)
	case command == "view-describe-focused":
		return a.handleViewFocused(msgs.DetailDescribe)
	case command == "view-logs-focused":
		return a.handleViewFocused(msgs.DetailLogs)
	// Cursor navigation — mode-aware
	case command == "cursor-up":
		if a.layout.FocusedDetails() && a.layout.RightPanelVisible() {
			a.layout.ActiveDetailPanel().ScrollUp()
		} else if focused := a.layout.FocusedSplit(); focused != nil && focused.Cursor() != 0 {
			focused.CursorUp()
			a, cmd := a.refreshDetailPanelOrLog()
			return a, cmd
		}
		return a, nil

	case command == "cursor-down":
		if a.layout.FocusedDetails() && a.layout.RightPanelVisible() {
			a.layout.ActiveDetailPanel().ScrollDown()
		} else if focused := a.layout.FocusedSplit(); focused != nil && focused.Cursor() != focused.Len()-1 {
			focused.CursorDown()
			a, cmd := a.refreshDetailPanelOrLog()
			return a, cmd
		}
		return a, nil

	// Enter/exit detail-scroll mode
	case command == "enter-detail":
		focused := a.layout.FocusedSplit()
		if focused == nil {
			return a, nil
		}
		sel := focused.Selected()
		if sel == nil {
			if a.layout.FocusedResources() && a.layout.RightPanelVisible() {
				a.layout.FocusDetails()
				a.statusBar.SetHints(a.currentHints())
			}
			return a, nil
		}

		// Check goto first
		if goToer, ok := focused.Plugin().(plugin.GoToer); ok {
			if resource, ns, isGoto := goToer.GoTo(sel); isGoto {
				return a.handleGoto(resource, ns)
			}
		}

		// Then check drill-down
		drillDowner, ok := focused.Plugin().(plugin.DrillDowner)
		if !ok {
			if a.layout.FocusedResources() && a.layout.RightPanelVisible() {
				a.layout.FocusDetails()
				a.statusBar.SetHints(a.currentHints())
			}
			return a, nil
		}

		childPlugin, children := drillDowner.DrillDown(sel)
		if childPlugin == nil {
			return a, nil
		}
		focused.PushNav(childPlugin, children, sel.GetName(), string(sel.GetUID()), sel.GetAPIVersion(), sel.GetKind())
		if a.layout.RightPanelVisible() {
			a.layout.FocusResources()
		}
		var cmd tea.Cmd
		a, cmd = a.refreshDetailPanelOrLog()
		a.keyTrie.Reset()
		a.envResolved = false
		a.statusBar.SetHints(a.currentHints())

		return a, cmd

	case command == "focus-panel":
		if a.layout.FocusedSplit() != nil && a.layout.FocusedResources() && a.layout.RightPanelVisible() {
			a.layout.FocusDetails()
			a.statusBar.SetHints(a.currentHints())
		}
		return a, nil

	case command == "exit-detail":
		if a.layout.FocusedDetails() {
			if a.layout.DetailZoomed() {
				a.layout.ToggleZoomDetail()
				a = a.syncIndicators()
				return a, nil
			}
			a.envResolved = false
			a.layout.FocusResources()
			a.keyTrie.Reset()
			a.statusBar.SetHints(a.currentHints())
		}
		return a, nil

	// Search navigation
	case command == "search-next":
		if target := a.searchTarget(); target != nil && target.SearchActive() {
			target.SearchNext()
			if a.layout.FocusedResources() {
				var descCmd tea.Cmd
				a, descCmd = a.refreshDetailPanel()
				return a, descCmd
			}
		}
		return a, nil

	case command == "search-prev":
		if target := a.searchTarget(); target != nil && target.SearchActive() {
			target.SearchPrev()
			if a.layout.FocusedResources() {
				var descCmd tea.Cmd
				a, descCmd = a.refreshDetailPanel()
				return a, descCmd
			}
		}
		return a, nil

	// Layered clear: selection → search → filter → exit-detail → drill-down pop → split unzoom
	case command == "clear-overlay":
		// Clear selection first (highest priority)
		if focused := a.layout.FocusedSplit(); focused != nil && focused.HasSelection() {
			focused.ClearSelection()
			return a, nil
		}
		if target := a.searchTarget(); target != nil {
			if target.SearchActive() {
				target.ClearSearch()
				if a.layout.FocusedResources() {
					var descCmd tea.Cmd
					a, descCmd = a.refreshDetailPanel()
					return a, descCmd
				}
				return a, nil
			}
			if target.FilterActive() {
				target.ClearFilter()
				if a.layout.FocusedResources() {
					var descCmd tea.Cmd
					a, descCmd = a.refreshDetailPanel()
					return a, descCmd
				}
				return a, nil
			}
		}
		if a.layout.FocusedDetails() {
			return a.executeCommand("exit-detail")
		}
		// Pop drill-down before closing panel
		if focused := a.layout.FocusedSplit(); focused != nil && focused.InDrillDown() {
			focused.PopNav()
			// Refresh with latest data from store
			if a.store != nil {
				if focused.InDrillDown() {
					if sp, ok := focused.Plugin().(plugin.SelfPopulating); ok {
						if r, ok := focused.Plugin().(plugin.Refreshable); ok {
							r.Refresh(focused.EffectiveNamespace())
						}
						focused.SetObjects(sp.Objects())
					} else {
						a.refreshDrillDownSplit(focused)
					}
				} else if focused.Plugin().GVR().Group != "_ktui" {
					// Back at root — load all objects from store
					objs := a.store.List(focused.Plugin().GVR(), focused.EffectiveNamespace())
					focused.SetObjects(objs)
				}
			}
			if sp, ok := focused.Plugin().(plugin.SelfPopulating); ok && !focused.InDrillDown() {
				if r, ok := focused.Plugin().(plugin.Refreshable); ok {
					r.Refresh(focused.EffectiveNamespace())
				}
				focused.SetObjects(sp.Objects())
			}
			if a.layout.RightPanelVisible() {
				a.layout.FocusResources()
			}
			var cmd tea.Cmd
			a, cmd = a.refreshDetailPanelOrLog()
			a.keyTrie.Reset()
			a.envResolved = false
			a.statusBar.SetHints(a.currentHints())
			return a, cmd
		}

		if a.layout.SplitZoomed() {
			a = a.toggleZoomSplitAndSync()
			return a, nil
		}
		return a, nil

	case command == "toggle-zoom":
		if a.layout.FocusedDetails() {
			a = a.toggleZoomDetailAndSync()
		} else {
			a = a.toggleZoomSplitAndSync()
		}
		return a, nil

	// Help overlay
	case command == "help":
		a.activeOverlay = overlayHelp
		ct, rn := a.currentContext()
		a.helpOverlay.Open(a.bindingSet.HelpGroups(ct, rn))
		return a, nil

	case command == "close-panel":
		a = a.closeRightPanel()
		return a, nil

	case command == "close-current-panel":
		if a.layout.FocusedDetails() && a.layout.RightPanelVisible() {
			a = a.closeRightPanel()
			return a, nil
		}
		if a.layout.SplitCount() > 1 {
			closing := a.layout.FocusedSplit()
			closingGVR := closing.Plugin().GVR()
			closingNs := closing.EffectiveNamespace()
			a.keyTrie.Reset()
			a.layout.CloseCurrentSplit()
			a.unsubscribeIfUnused(closingGVR, closingNs)
			a.statusBar.SetHints(a.currentHints())
			return a, nil
		}
		return a, nil

	// Focus
	case command == "focus-next":
		a.keyTrie.Reset()
		a.layout.FocusNext()
		var descCmd tea.Cmd
		a, descCmd = a.refreshDetailPanel()
		var cmd tea.Cmd
		a, cmd = a.syncLogPanel()
		a.statusBar.SetHints(a.currentHints())
		return a, tea.Batch(descCmd, cmd)
	case command == "focus-prev":
		a.keyTrie.Reset()
		a.layout.FocusPrev()
		var descCmd tea.Cmd
		a, descCmd = a.refreshDetailPanel()
		var cmd tea.Cmd
		a, cmd = a.syncLogPanel()
		a.statusBar.SetHints(a.currentHints())
		return a, tea.Batch(descCmd, cmd)

	// Quit
	case command == "quit":
		if a.layout.DetailZoomed() {
			a = a.toggleZoomDetailAndSync()
			return a, nil
		}
		if a.layout.SplitZoomed() {
			a = a.toggleZoomSplitAndSync()
			return a, nil
		}
		if a.layout.RightPanelVisible() {
			a = a.closeRightPanel()
			return a, nil
		}
		if a.layout.SplitCount() > 1 {
			closing := a.layout.FocusedSplit()
			closingGVR := closing.Plugin().GVR()
			closingNs := closing.EffectiveNamespace()
			a.keyTrie.Reset()
			a.layout.CloseCurrentSplit()
			a.unsubscribeIfUnused(closingGVR, closingNs)
			a.statusBar.SetHints(a.currentHints())
			return a, nil
		}
		a = a.stopLogStream()
		return a, tea.Quit

	// Search/filter bar
	case command == "search-open":
		a.activeOverlay = overlaySearchBar
		a.searchBar.Open(msgs.SearchModeSearch)
		return a, nil
	case command == "filter-open":
		a.activeOverlay = overlaySearchBar
		a.searchBar.Open(msgs.SearchModeFilter)
		return a, nil

	// Command bar
	case command == "resource-picker":
		a.activeOverlay = overlayResourcePicker
		a.resourcePicker.Open()
		return a, nil

	// Namespace picker
	case command == "namespace-picker":
		a.activeOverlay = overlayNsPicker
		a.nsPicker.Open()
		if a.k8sClient != nil {
			client := a.k8sClient
			timeout := a.config.APITimeout()
			opCmd := a.statusBar.StartOperation()
			return a, tea.Batch(opCmd, func() tea.Msg {
				ctx, cancel := context.WithTimeout(context.Background(), timeout)
				defer cancel()
				nsList, err := client.ListNamespaces(ctx)
				return msgs.NamespacesLoadedMsg{Namespaces: nsList, Err: err}
			})
		}
		return a, nil

	case command == "toggle-select":
		if focused := a.layout.FocusedSplit(); focused != nil {
			focused.ToggleSelect()
			focused.CursorDown()
		}
		return a, nil

	case command == "select-all":
		if focused := a.layout.FocusedSplit(); focused != nil {
			focused.SelectAll()
		}
		return a, nil

	// Delete with confirmation dialog
	case command == "delete":
		focused := a.layout.FocusedSplit()
		if focused == nil {
			return a, nil
		}

		// Bulk delete when multi-select is active
		if focused.HasSelection() {
			objs := focused.SelectedObjects()
			if len(objs) == 0 {
				return a, nil
			}
			p := focused.Plugin()
			resource := p.GVR().Resource
			if p.Name() == "helmreleases" {
				resource = "helm release"
			}
			var names []string
			for _, obj := range objs {
				if p.IsClusterScoped() {
					names = append(names, obj.GetName())
				} else {
					names = append(names, obj.GetNamespace()+"/"+obj.GetName())
				}
			}
			nameList := strings.Join(names, "\n  ")
			if len(names) > 20 {
				nameList = strings.Join(names[:20], "\n  ") + fmt.Sprintf("\n  ... and %d more", len(names)-20)
			}
			msg := fmt.Sprintf("Delete %d %s?\n\n  %s", len(objs), resource, nameList)
			a.pendingBulkDelete = objs
			a.confirmDialog = ui.NewConfirmDialog(msg, a.width)
			a.activeOverlay = overlayConfirm
			return a, nil
		}

		// Single delete (existing path)
		selected := focused.Selected()
		if selected == nil {
			return a, nil
		}
		p := focused.Plugin()
		resource := p.GVR().Resource
		if p.Name() == "helmreleases" {
			resource = "helm release"
		}
		var msg string
		if p.IsClusterScoped() {
			msg = fmt.Sprintf("Delete %s %s?", resource, selected.GetName())
		} else {
			msg = fmt.Sprintf("Delete %s %s/%s?", resource, selected.GetNamespace(), selected.GetName())
		}
		a.pendingDelete = selected
		a.confirmDialog = ui.NewConfirmDialog(msg, a.width)
		a.activeOverlay = overlayConfirm
		return a, nil

	// Exec into pod
	case command == "exec":
		focused, selected, ok := a.focusedSelection()
		if !ok {
			return a, nil
		}
		if a.k8sClient == nil {
			cmd := a.statusBar.SetError("exec: no k8s client")
			return a, cmd
		}
		ns := selected.GetNamespace()
		if ns == "" {
			ns = focused.Namespace()
		}
		podName := resolvePodName(focused, selected)
		containerName := resolveContainerName(focused, selected)
		return a, k8s.ExecCmd(a.k8sClient, podName, containerName, ns, a.config.ExecCommand())

	case command == "debug" || command == "debug-privileged":
		return a.handleDebug(command == "debug-privileged")

	// Toggle env resolve
	case command == "toggle-env-resolve":
		a.envResolved = !a.envResolved
		if a.layout.RightPanelVisible() && a.layout.RightPanel().Mode() == msgs.DetailDescribe {
			var descCmd tea.Cmd
			a, descCmd = a.reloadDetailPanel()
			return a, descCmd
		}
		return a, nil

	// Port-forward
	case command == "port-forward":
		focused, selected, ok := a.focusedSelection()
		if !ok {
			return a, nil
		}
		ports := a.extractPorts(focused, selected)
		if len(ports) == 0 {
			cmd := a.statusBar.SetError("no ports found on this resource")
			return a, cmd
		}
		ns := selected.GetNamespace()
		if ns == "" {
			ns = focused.Namespace()
		}
		podName := resolvePodName(focused, selected)
		a.portForwardOverlay.Open(podName, ns, ports)
		a.activeOverlay = overlayPortForward
		return a, nil
	case command == "rollout-restart":
		cmd := a.statusBar.SetError("rollout-restart: not yet implemented")
		return a, cmd
	case command == "edit":
		focused, selected, ok := a.focusedSelection()
		if !ok {
			return a, nil
		}
		if a.k8sClient == nil {
			cmd := a.statusBar.SetError("edit: no k8s client")
			return a, cmd
		}
		if focused.Plugin().Name() == "helmreleases" {
			if a.helmClient == nil {
				cmd := a.statusBar.SetError("helm: no client")
				return a, cmd
			}
			name := selected.GetName()
			ns := selected.GetNamespace()
			chartRef := ""
			if a.config != nil {
				chartRef = a.config.ChartRef(ns, name)
			}
			if chartRef == "" {
				a.chartInputOverlay.Open(name, ns, "")
				a.activeOverlay = overlayChartInput
				return a, nil
			}
			return a, helm.EditValuesCmd(a.helmClient, name, ns)
		}
		if focused.Plugin().GVR().Group == "_ktui" {
			return a, nil
		}
		p := focused.Plugin()
		return a, k8s.EditCmd(a.k8sClient.Dynamic, p.GVR(), p.IsClusterScoped(), selected)

	case command == "set-image":
		focused, selected, ok := a.focusedSelection()
		if !ok {
			return a, nil
		}
		if a.k8sClient == nil {
			cmd := a.statusBar.SetError("set-image: no k8s client")
			return a, cmd
		}
		pluginName := focused.Plugin().Name()
		containers := extractContainerImages(pluginName, selected)
		if len(containers) == 0 {
			cmd := a.statusBar.SetError("no containers found on this resource")
			return a, cmd
		}

		// Resolve target resource for patching
		gvr := focused.Plugin().GVR()
		resourceName := selected.GetName()
		ns := selected.GetNamespace()
		if ns == "" {
			ns = focused.Namespace()
		}

		// Containers view: patch the parent pod
		if pluginName == "containers" {
			if podObj, ok := selected.Object["_pod"].(map[string]any); ok {
				resourceName, _, _ = unstructured.NestedString(podObj, "metadata", "name")
			}
			gvr = schema.GroupVersionResource{Version: "v1", Resource: "pods"}
			pluginName = "pods"
		}

		a.setImageOverlay.Open(resourceName, ns, gvr, pluginName, containers)
		a.activeOverlay = overlaySetImage
		return a, nil

	case command == "scale":
		focused, selected, ok := a.focusedSelection()
		if !ok {
			return a, nil
		}
		if a.k8sClient == nil {
			cmd := a.statusBar.SetError("scale: no k8s client")
			return a, cmd
		}
		gvr := focused.Plugin().GVR()
		name := selected.GetName()
		ns := selected.GetNamespace()
		if ns == "" {
			ns = focused.Namespace()
		}
		replicas, found, _ := unstructured.NestedInt64(selected.Object, "spec", "replicas")
		if !found {
			replicas = 1
		}
		a.scaleOverlay.Open(name, ns, gvr, int32(replicas))
		a.activeOverlay = overlayScale
		return a, nil

	case command == "helm-set-chart":
		focused := a.layout.FocusedSplit()
		if focused == nil || focused.Plugin().Name() != "helmreleases" {
			return a, nil
		}
		selected := focused.Selected()
		if selected == nil {
			return a, nil
		}
		name := selected.GetName()
		ns := selected.GetNamespace()
		currentRef := ""
		if a.config != nil {
			currentRef = a.config.ChartRef(ns, name)
		}
		a.chartInputOverlay.Open(name, ns, currentRef)
		a.activeOverlay = overlayChartInput
		return a, nil

	case command == "helm-rollback":
		focused := a.layout.FocusedSplit()
		if focused == nil || focused.Plugin().Name() != "helmreleases" {
			return a, nil
		}
		selected := focused.Selected()
		if selected == nil {
			return a, nil
		}
		if a.helmClient == nil {
			cmd := a.statusBar.SetError("helm: no client")
			return a, cmd
		}
		name := selected.GetName()
		ns := selected.GetNamespace()
		a.helmRollbackOverlay.OpenLoading(name, ns)
		a.activeOverlay = overlayHelmRollback
		opCmd := a.statusBar.StartOperation()
		return a, tea.Batch(opCmd, fetchHelmHistoryCmd(a.helmClient, name, ns, a.config.APITimeout()))

	case command == "toggle-autoscroll":
		if a.layout.IsLogMode() {
			a.layout.LogView().ToggleAutoscroll()
		}
		return a, nil

	case command == "toggle-log-syntax":
		if a.layout.IsLogMode() {
			a.layout.LogView().ToggleSyntax()
		}
		return a, nil

	case command == "log-insert-marker":
		if a.layout.IsLogMode() {
			a.layout.LogView().InsertMarker()
		}
		return a, nil

	case command == "select-container":
		if !a.layout.IsLogMode() {
			return a, nil
		}
		lv := a.layout.LogView()
		containers := lv.Containers()
		if len(containers) <= 1 {
			return a, nil
		}
		a.containerPicker.SetContainers(containers)
		a.containerPicker.Open()
		a.activeOverlay = overlayContainerPicker
		return a, nil

	case command == "select-time-range":
		if !a.layout.IsLogMode() {
			return a, nil
		}
		a.timeRangePicker.OpenPresets()
		a.activeOverlay = overlayTimeRange
		return a, nil

	// List horizontal scroll
	case command == "list-scroll-left":
		if focused := a.layout.FocusedSplit(); focused != nil {
			focused.ScrollLeft()
		}
		return a, nil
	case command == "list-scroll-right":
		if focused := a.layout.FocusedSplit(); focused != nil {
			focused.ScrollRight()
		}
		return a, nil

	// Scroll and refresh
	case command == "scroll-left":
		if a.layout.FocusedDetails() && a.layout.RightPanelVisible() {
			a.layout.ActiveDetailPanel().ScrollLeft()
		}
		return a, nil
	case command == "scroll-right":
		if a.layout.FocusedDetails() && a.layout.RightPanelVisible() {
			a.layout.ActiveDetailPanel().ScrollRight()
		}
		return a, nil
	case command == "toggle-wrap":
		if a.layout.RightPanelVisible() {
			a.layout.ActiveDetailPanel().ToggleWrap()
		}
		return a, nil
	case command == "toggle-header":
		if a.layout.RightPanelVisible() {
			a.layout.ActiveDetailPanel().ToggleHeader()
		}
		return a, nil
	case command == "page-down":
		if a.layout.FocusedDetails() && a.layout.RightPanelVisible() {
			a.layout.ActiveDetailPanel().PageDown()
		} else if focused := a.layout.FocusedSplit(); focused != nil {
			focused.PageDown()
			a, cmd := a.refreshDetailPanelOrLog()
			return a, cmd
		}
		return a, nil
	case command == "page-up":
		if a.layout.FocusedDetails() && a.layout.RightPanelVisible() {
			a.layout.ActiveDetailPanel().PageUp()
		} else if focused := a.layout.FocusedSplit(); focused != nil {
			focused.PageUp()
			a, cmd := a.refreshDetailPanelOrLog()
			return a, cmd
		}
		return a, nil
	case command == "cursor-top":
		if a.layout.FocusedDetails() && a.layout.RightPanelVisible() {
			a.layout.ActiveDetailPanel().GotoTop()
		} else if focused := a.layout.FocusedSplit(); focused != nil {
			focused.GotoTop()
			a, cmd := a.refreshDetailPanelOrLog()
			return a, cmd
		}
		return a, nil
	case command == "cursor-bottom":
		if a.layout.FocusedDetails() && a.layout.RightPanelVisible() {
			a.layout.ActiveDetailPanel().GotoBottom()
		} else if focused := a.layout.FocusedSplit(); focused != nil {
			focused.GotoBottom()
			a, cmd := a.refreshDetailPanelOrLog()
			return a, cmd
		}
		return a, nil
	case command == "reload-all":
		return a.reloadAll()
	case command == "refresh-detail":
		if a.layout.RightPanelVisible() {
			if a.layout.IsLogMode() {
				// For log mode, restart the stream
				return a.startLogViewForSelected()
			}
			var descCmd tea.Cmd
			a, descCmd = a.reloadDetailPanel()
			return a, descCmd
		}
		return a, nil
	}

	cmd := a.statusBar.SetError(fmt.Sprintf("unknown command: %s", command))
	return a, cmd
}

// closeRightPanel performs full right-panel teardown: stops log stream,
// resets mode/state, hides panel, and refreshes indicators/hints.
func (a App) closeRightPanel() App {
	a = a.stopLogStream()
	a.layout.SetLogMode(false)
	a.envResolved = false
	a.layout.FocusResources()
	a.keyTrie.Reset()
	a.layout.HideRightPanel()
	a = a.syncIndicators()
	a.statusBar.SetHints(a.currentHints())
	return a
}

func (a App) handleGoto(resourceName string, targetNs string) (tea.Model, tea.Cmd) {
	var p plugin.ResourcePlugin
	var ok bool
	if strings.Contains(resourceName, "/") {
		p, ok = plugin.ByQualifiedName(resourceName)
	}
	if !ok {
		p, ok = plugin.ByName(resourceName)
	}
	if !ok {
		cmd := a.statusBar.SetError(fmt.Sprintf("unknown resource: %s", resourceName))
		return a, cmd
	}
	return a.handleGotoPlugin(p, targetNs)
}

func (a App) handleGotoPlugin(p plugin.ResourcePlugin, targetNs string) (tea.Model, tea.Cmd) {
	if a.layout.AnyZoomed() {
		a.layout.UnzoomAll()
		a = a.syncIndicators()
	}

	focused := a.layout.FocusedSplit()
	if focused == nil {
		return a, nil
	}

	oldPlugin := focused.Plugin()
	oldNs := focused.EffectiveNamespace()

	a.keyTrie.Reset()
	a.envResolved = false
	focused.ResetNav()
	focused.SetPlugin(p)
	if targetNs != "" {
		focused.SetNamespace(targetNs)
	}
	ns := focused.EffectiveNamespace()
	populateCmd := a.subscribeAndPopulate(focused, p, ns)

	// Clean up old GVR subscription if unused
	if a.store != nil && oldPlugin.GVR() != p.GVR() {
		a.unsubscribeIfUnused(oldPlugin.GVR(), oldNs)
	}

	var descCmd tea.Cmd
	a, descCmd = a.refreshDetailPanel()
	var cmd tea.Cmd
	a, cmd = a.syncLogPanel()
	a.statusBar.SetHints(a.currentHints())
	return a, tea.Batch(populateCmd, descCmd, cmd)
}

func (a App) handleGotoGVR(gvrStr string) (tea.Model, tea.Cmd) {
	parts := strings.Split(gvrStr, "/")
	var gvr schema.GroupVersionResource
	switch len(parts) {
	case 3:
		// group/version/resource
		gvr = schema.GroupVersionResource{Group: parts[0], Version: parts[1], Resource: parts[2]}
	case 2:
		// version/resource (core group, group="")
		gvr = schema.GroupVersionResource{Version: parts[0], Resource: parts[1]}
	default:
		cmd := a.statusBar.SetError(fmt.Sprintf("invalid GVR format: %s (expected group/version/resource or version/resource)", gvrStr))
		return a, cmd
	}
	p, ok := plugin.ByGVR(gvr)
	if !ok {
		cmd := a.statusBar.SetError(fmt.Sprintf("unknown GVR: %s", gvrStr))
		return a, cmd
	}
	return a.handleGotoPlugin(p, "")
}

func (a App) handleSplit(resourceName string) (tea.Model, tea.Cmd) {
	p, ok := plugin.ByName(resourceName)
	if !ok {
		cmd := a.statusBar.SetError(fmt.Sprintf("unknown resource: %s", resourceName))
		return a, cmd
	}

	if a.layout.AnyZoomed() {
		a.layout.UnzoomAll()
		a = a.syncIndicators()
	}

	ns := "default"
	if a.k8sClient != nil {
		ns = a.k8sClient.Namespace
	}
	if prev := a.layout.FocusedSplit(); prev != nil {
		if prev.Namespace() != "" {
			ns = prev.Namespace()
		}
	}

	a.keyTrie.Reset()
	a.layout.AddSplit(p, ns)
	newSplit := a.layout.FocusedSplit()
	populateCmd := a.subscribeAndPopulate(newSplit, p, newSplit.EffectiveNamespace())

	var descCmd tea.Cmd
	a, descCmd = a.refreshDetailPanel()
	var cmd tea.Cmd
	a, cmd = a.syncLogPanel()
	a.statusBar.SetHints(a.currentHints())
	return a, tea.Batch(populateCmd, descCmd, cmd)
}

func (a App) handleView(mode msgs.DetailMode) (tea.Model, tea.Cmd) {
	_, _, ok := a.focusedSelection()
	if !ok {
		return a, nil
	}

	// Guard: only pods/containers support logs
	if mode == msgs.DetailLogs {
		focused := a.layout.FocusedSplit()
		if focused == nil || !isLoggablePlugin(focused.Plugin().Name()) {
			return a, nil
		}
	}

	if mode != msgs.DetailDescribe {
		a.envResolved = false
	}

	// Switching away from log mode
	if mode != msgs.DetailLogs {
		a = a.stopLogStream()
		a.layout.SetLogMode(false)
	}

	if mode == msgs.DetailLogs {
		a.layout.SetLogMode(true)
		a.layout.ShowRightPanel()
		return a.startLogViewForSelected()
	}

	a.layout.ShowRightPanel()
	panel := a.layout.RightPanel()
	panel.SetMode(mode)
	a, cmd := a.refreshDetailPanel()
	return a, cmd
}

func (a App) handleViewFocused(mode msgs.DetailMode) (tea.Model, tea.Cmd) {
	_, _, ok := a.focusedSelection()
	if !ok {
		return a, nil
	}

	// Guard: only pods/containers support logs
	if mode == msgs.DetailLogs {
		focused := a.layout.FocusedSplit()
		if focused == nil || !isLoggablePlugin(focused.Plugin().Name()) {
			return a, nil
		}
	}

	if mode != msgs.DetailDescribe {
		a.envResolved = false
	}

	// Switching away from log mode
	if mode != msgs.DetailLogs {
		a = a.stopLogStream()
		a.layout.SetLogMode(false)
	}

	if mode == msgs.DetailLogs {
		a.layout.SetLogMode(true)
		a.layout.ShowRightPanel()
		model, cmd := a.startLogViewForSelected()
		app := model.(App)
		app.layout.FocusDetails()
		app.statusBar.SetHints(app.currentHints())
		return app, cmd
	}

	a.layout.ShowRightPanel()
	a.layout.RightPanel().SetMode(mode)
	var descCmd tea.Cmd
	a, descCmd = a.refreshDetailPanel()
	a.layout.FocusDetails()
	a.statusBar.SetHints(a.currentHints())
	return a, descCmd
}

func (a App) handleResourcePickerCommand(input string) (tea.Model, tea.Cmd) {
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return a, nil
	}
	switch parts[0] {
	case "goto":
		if len(parts) >= 2 {
			return a.handleGoto(parts[1], "")
		}
		cmd := a.statusBar.SetError("usage: goto <resource>")
		return a, cmd
	case "goto-gvr":
		if len(parts) >= 2 {
			return a.handleGotoGVR(parts[1])
		}
		cmd := a.statusBar.SetError("usage: goto-gvr <group/version/resource>")
		return a, cmd
	default:
		return a.handleGoto(parts[0], "")
	}
}

// subscribeAndPopulate subscribes to the store for real resources,
// or falls back to SelfPopulating for synthetic plugins.
// For helm releases it returns a tea.Cmd to fetch data asynchronously.
func (a App) subscribeAndPopulate(split *ui.ResourceList, p plugin.ResourcePlugin, ns string) tea.Cmd {
	if a.store != nil && p.GVR().Group != "_ktui" {
		objs := a.store.Subscribe(p.GVR(), ns)
		split.SetObjects(objs)
		return nil
	}
	if sp, ok := p.(plugin.SelfPopulating); ok {
		if _, ok := p.(plugin.Refreshable); ok {
			if a.helmClient != nil {
				split.SetObjects(sp.Objects()) // show cached data immediately
				opCmd := a.statusBar.StartOperation()
				return tea.Batch(opCmd, fetchHelmReleasesCmd(a.helmClient, ns, a.config.APITimeout()))
			}
		}
		split.SetObjects(sp.Objects())
	}
	return nil
}

func (a App) reloadAll() (tea.Model, tea.Cmd) {
	wasLogMode := a.layout.IsLogMode()
	wasRightVisible := a.layout.RightPanelVisible()

	// Tear down all informers and clear store cache
	if a.store != nil {
		a.store.UnsubscribeAll()
	}
	a = a.stopLogStream()
	a.layout.SetLogMode(false)

	// Dismiss any open overlays
	a.activeOverlay = overlayNone
	a.pendingRun = nil
	a.pendingBulkDelete = nil
	a.pendingDelete = nil
	a.pendingDebug = nil

	// Reset UI state
	if a.layout.AnyZoomed() {
		a.layout.UnzoomAll()
	}
	a.layout.FocusResources()
	a.envResolved = false
	a.keyTrie.Reset()

	// Reset each split to root and re-subscribe
	var populateCmds []tea.Cmd
	for i := range a.layout.SplitCount() {
		split := a.layout.SplitAt(i)
		if split == nil {
			continue
		}
		split.ResetForReload()
		if cmd := a.subscribeAndPopulate(split, split.Plugin(), split.EffectiveNamespace()); cmd != nil {
			populateCmds = append(populateCmds, cmd)
		}
	}

	// Restore detail panel state
	var descCmd tea.Cmd
	if wasRightVisible && wasLogMode {
		a.layout.SetLogMode(true)
		if lv := a.layout.LogView(); lv != nil {
			lv.ClearAndRestart()
			lv.SetUnavailable(true)
		}
	} else {
		a, descCmd = a.reloadDetailPanel()
	}

	// Update status bar
	a.statusBar.SetError("") // clear immediately, no cmd needed
	a.statusBar.SetHints(a.currentHints())
	a = a.syncIndicators()

	allCmds := append(populateCmds, descCmd, tea.ClearScreen)
	return a, tea.Batch(allCmds...)
}

func (a App) unsubscribeIfUnused(gvr schema.GroupVersionResource, namespace string) {
	// Skip synthetic GVRs that have no real informer
	if gvr.Group == "_ktui" {
		return
	}
	for i := range a.layout.SplitCount() {
		s := a.layout.SplitAt(i)
		if s == nil {
			continue
		}
		if s.Plugin().GVR() == gvr && s.EffectiveNamespace() == namespace {
			return // still in use by visible plugin
		}
		// Also check if a drill-down parent in the nav stack needs this GVR
		if s.NavStackHasGVR(gvr, namespace) {
			return // still needed by a parent view
		}
	}
	if a.store != nil {
		a.store.Unsubscribe(gvr, namespace)
	}
}

func (a App) handleNamespaceSwitch(ns string) (tea.Model, tea.Cmd) {
	a.envResolved = false
	focused := a.layout.FocusedSplit()
	if focused == nil {
		return a, nil
	}

	if focused.Plugin().IsClusterScoped() {
		return a, nil
	}

	oldNs := focused.Namespace()
	if oldNs == ns {
		return a, nil
	}

	// Tear down log stream only after confirming we'll actually switch
	wasLogMode := a.layout.IsLogMode()
	a = a.stopLogStream()
	a.layout.SetLogMode(false)

	oldGVR := focused.Plugin().GVR()

	// Update pane namespace and clear stale data
	focused.ResetNav()
	focused.SetNamespace(ns)
	focused.SetObjects(nil)

	// Subscribe to new (GVR, namespace) pair
	populateCmd := a.subscribeAndPopulate(focused, focused.Plugin(), ns)

	// Unsubscribe old if no other pane uses it
	a.unsubscribeIfUnused(oldGVR, oldNs)

	// Restore log mode: mark unavailable until objects arrive
	if wasLogMode && isLoggablePlugin(focused.Plugin().Name()) {
		a.layout.SetLogMode(true)
		if lv := a.layout.LogView(); lv != nil {
			lv.ClearAndRestart()
			lv.SetUnavailable(true)
		}
	}

	a, cmd := a.refreshDetailPanel()
	return a, tea.Batch(populateCmd, cmd)
}

func (a App) executeSingleDelete(obj *unstructured.Unstructured, force bool) (tea.Model, tea.Cmd) {
	focused := a.layout.FocusedSplit()
	if focused == nil {
		return a, nil
	}
	// Stop port-forward
	if focused.Plugin().Name() == "portforwards" && a.pfRegistry != nil {
		id := obj.GetName()
		a.pfRegistry.Remove(id)
		delete(a.pfHandles, id)
		a = a.syncIndicators()
		if sp, ok := focused.Plugin().(plugin.SelfPopulating); ok {
			focused.SetObjects(sp.Objects())
		}
		return a, func() tea.Msg {
			return msgs.PortForwardStoppedMsg{ID: id}
		}
	}
	// Uninstall Helm release
	if focused.Plugin().Name() == "helmreleases" && a.helmClient != nil {
		name := obj.GetName()
		ns := obj.GetNamespace()
		return a, func() tea.Msg {
			if err := a.helmClient.Uninstall(name, ns); err != nil {
				return msgs.ActionResultMsg{Err: err}
			}
			return msgs.ActionResultMsg{ActionID: "helm-uninstall:" + name}
		}
	}
	// Delete using dynamic client
	if a.k8sClient != nil {
		p := focused.Plugin()
		return a, func() tea.Msg {
			opts := metav1.DeleteOptions{}
			if force {
				opts.GracePeriodSeconds = new(int64)
			}
			var err error
			if p.IsClusterScoped() {
				err = a.k8sClient.Dynamic.Resource(p.GVR()).Delete(
					context.Background(), obj.GetName(), opts)
			} else {
				err = a.k8sClient.Dynamic.Resource(p.GVR()).Namespace(obj.GetNamespace()).Delete(
					context.Background(), obj.GetName(), opts)
			}
			if err != nil {
				return msgs.ActionResultMsg{Err: err}
			}
			return msgs.ActionResultMsg{ActionID: "delete:" + obj.GetName()}
		}
	}
	return a, nil
}

func (a App) executeBulkDelete(targets []*unstructured.Unstructured, force bool) (tea.Model, tea.Cmd) {
	focused := a.layout.FocusedSplit()
	if focused == nil {
		return a, nil
	}
	p := focused.Plugin()

	// Helm releases
	if p.Name() == "helmreleases" && a.helmClient != nil {
		helmClient := a.helmClient
		return a, func() tea.Msg {
			var errs []string
			for _, obj := range targets {
				if err := helmClient.Uninstall(obj.GetName(), obj.GetNamespace()); err != nil {
					errs = append(errs, obj.GetName()+": "+err.Error())
				}
			}
			if len(errs) > 0 {
				return msgs.ActionResultMsg{Err: fmt.Errorf("bulk delete: %d/%d failed: %s", len(errs), len(targets), strings.Join(errs, "; "))}
			}
			return msgs.ActionResultMsg{ActionID: fmt.Sprintf("helm-uninstall:%d", len(targets))}
		}
	}

	// Port-forwards
	if p.Name() == "portforwards" && a.pfRegistry != nil {
		for _, obj := range targets {
			a.pfRegistry.Remove(obj.GetName())
			delete(a.pfHandles, obj.GetName())
		}
		a = a.syncIndicators()
		if sp, ok := p.(plugin.SelfPopulating); ok {
			focused.SetObjects(sp.Objects())
		}
		return a, nil
	}

	// K8s resources via dynamic client
	if a.k8sClient == nil {
		return a, nil
	}
	client := a.k8sClient
	gvr := p.GVR()
	clusterScoped := p.IsClusterScoped()

	return a, func() tea.Msg {
		opts := metav1.DeleteOptions{}
		if force {
			opts.GracePeriodSeconds = new(int64)
		}
		var errs []string
		for _, obj := range targets {
			var err error
			if clusterScoped {
				err = client.Dynamic.Resource(gvr).Delete(context.Background(), obj.GetName(), opts)
			} else {
				err = client.Dynamic.Resource(gvr).Namespace(obj.GetNamespace()).Delete(context.Background(), obj.GetName(), opts)
			}
			if err != nil {
				errs = append(errs, obj.GetName()+": "+err.Error())
			}
		}
		if len(errs) > 0 {
			return msgs.ActionResultMsg{Err: fmt.Errorf("bulk delete: %d/%d failed: %s", len(errs), len(targets), strings.Join(errs, "; "))}
		}
		return msgs.ActionResultMsg{ActionID: fmt.Sprintf("delete:%d-resources", len(targets))}
	}
}

func (a App) handleSort(column string) (tea.Model, tea.Cmd) {
	focused := a.layout.FocusedSplit()
	if focused == nil {
		return a, nil
	}
	focused.SetSort(column)
	return a, nil
}

func (a App) handleDebug(privileged bool) (tea.Model, tea.Cmd) {
	focused, selected, ok := a.focusedSelection()
	if !ok {
		return a, nil
	}
	if a.k8sClient == nil {
		a.statusBar.SetError("debug: no k8s client")
		return a, nil
	}

	image := "busybox:latest"
	if a.config != nil {
		image = a.config.DebugImage()
	}

	command := []string{"sh"}
	if a.config != nil {
		command = a.config.DebugCommand()
	}

	pluginName := focused.Plugin().Name()

	// Node debugging — always privileged, requires confirmation
	if pluginName == "nodes" {
		msg := fmt.Sprintf("Debug node %s?\n\nThis creates a privileged pod with full host access\n(HostPID, HostNetwork, HostIPC, host root at /host).", selected.GetName())
		a.pendingDebug = &pendingDebugAction{
			nodeMode: true,
			nodeName: selected.GetName(),
			image:    image,
			command:  command,
		}
		a.confirmDialog = ui.NewConfirmDialog(msg, a.width)
		a.activeOverlay = overlayConfirm
		return a, nil
	}

	// Pod / container debugging
	ns := selected.GetNamespace()
	if ns == "" {
		ns = focused.Namespace()
	}
	podName := resolvePodName(focused, selected)
	containerName := resolveContainerName(focused, selected)

	// Privileged debug requires confirmation
	if privileged {
		msg := fmt.Sprintf("Debug pod %s/%s with privileged container?\n\nThis creates an ephemeral container with elevated privileges.", ns, podName)
		a.pendingDebug = &pendingDebugAction{
			privileged:    true,
			podName:       podName,
			containerName: containerName,
			namespace:     ns,
			image:         image,
			command:       command,
		}
		a.confirmDialog = ui.NewConfirmDialog(msg, a.width)
		a.activeOverlay = overlayConfirm
		return a, nil
	}

	return a, k8s.DebugCmd(a.k8sClient, podName, containerName, ns, image, command, false)
}

// executePendingDebug runs a debug action that was confirmed by the user.
func (a App) executePendingDebug(dbg *pendingDebugAction) (tea.Model, tea.Cmd) {
	if a.k8sClient == nil {
		a.statusBar.SetError("debug: no k8s client")
		return a, nil
	}
	if dbg.nodeMode {
		return a, k8s.DebugNodeCmd(a.k8sClient, dbg.nodeName, dbg.image, dbg.command)
	}
	return a, k8s.DebugCmd(a.k8sClient, dbg.podName, dbg.containerName, dbg.namespace, dbg.image, dbg.command, dbg.privileged)
}

func (a App) handleSearchSubmitted(msg msgs.SearchSubmittedMsg) (tea.Model, tea.Cmd) {
	target := a.searchTarget()
	if target == nil {
		a.statusBar.SetError("no active panel to search")
		return a, nil
	}
	if err := target.ApplySearch(msg.Pattern, msg.Mode); err != nil {
		a.statusBar.SetError("invalid regex: " + err.Error())
	} else {
		a.statusBar.SetError("")
		if a.layout.FocusedResources() {
			var descCmd tea.Cmd
			a, descCmd = a.refreshDetailPanel()
			return a, descCmd
		}
	}
	return a, nil
}

func (a App) handleSearchChanged(msg msgs.SearchChangedMsg) (tea.Model, tea.Cmd) {
	target := a.searchTarget()
	if target == nil {
		return a, nil
	}
	if err := target.ApplySearch(msg.Pattern, msg.Mode); err != nil {
		a.searchBar.SetError(err.Error())
	} else {
		a.searchBar.SetError("")
		if a.layout.FocusedResources() {
			var descCmd tea.Cmd
			a, descCmd = a.refreshDetailPanel()
			return a, descCmd
		}
	}
	return a, nil
}

func (a App) handleSearchCleared(msg msgs.SearchClearedMsg) (tea.Model, tea.Cmd) {
	target := a.searchTarget()
	if target == nil {
		return a, nil
	}
	if msg.Mode == msgs.SearchModeFilter {
		target.ClearFilter()
	} else {
		target.ClearSearch()
	}
	if a.layout.FocusedResources() {
		var descCmd tea.Cmd
		a, descCmd = a.refreshDetailPanel()
		return a, descCmd
	}
	return a, nil
}

func (a App) extractPorts(split *ui.ResourceList, obj *unstructured.Unstructured) []ui.PortItem {
	var ports []ui.PortItem

	if split.Plugin().Name() == "containers" {
		// Container view: read ports from _spec
		specMap, ok := obj.Object["_spec"].(map[string]any)
		if !ok {
			return nil
		}
		ports = appendContainerPorts(ports, obj.GetName(), specMap)
		return ports
	}

	// Pod view: iterate all containers
	specs, _, _ := unstructured.NestedSlice(obj.Object, "spec", "containers")
	for _, spec := range specs {
		specMap, ok := spec.(map[string]any)
		if !ok {
			continue
		}
		containerName, _ := specMap["name"].(string)
		ports = appendContainerPorts(ports, containerName, specMap)
	}
	return ports
}

func appendContainerPorts(ports []ui.PortItem, containerName string, specMap map[string]any) []ui.PortItem {
	portSlice, _ := specMap["ports"].([]any)
	for _, p := range portSlice {
		pm, ok := p.(map[string]any)
		if !ok {
			continue
		}
		port := toInt32(pm["containerPort"])
		protocol := "TCP"
		if proto, ok := pm["protocol"].(string); ok && proto != "" {
			protocol = proto
		}
		if port > 0 {
			ports = append(ports, ui.PortItem{ContainerName: containerName, Port: port, Protocol: protocol})
		}
	}
	return ports
}

func toInt32(v any) int32 {
	switch n := v.(type) {
	case int64:
		return int32(n)
	case float64:
		return int32(n)
	case int32:
		return n
	case int:
		return int32(n)
	}
	return 0
}

func (a App) handleSetImageRequested(msg msgs.SetImageRequestedMsg) (tea.Model, tea.Cmd) {
	if a.k8sClient == nil {
		return a, nil
	}
	return a, k8s.SetImageCmd(a.k8sClient.Dynamic, msg.GVR, msg.ResourceName, msg.Namespace, msg.PluginName, msg.Images)
}

func (a App) handleScaleRequested(msg msgs.ScaleRequestedMsg) (tea.Model, tea.Cmd) {
	if a.k8sClient == nil {
		return a, nil
	}
	return a, k8s.ScaleCmd(a.k8sClient.Dynamic, msg.GVR, msg.ResourceName, msg.Namespace, msg.Replicas)
}

func extractContainerImages(pluginName string, obj *unstructured.Unstructured) []msgs.ContainerImageChange {
	if pluginName == "containers" {
		typ, _ := obj.Object["_type"].(string)
		if typ == "ephemeral" {
			return nil
		}
		specMap, ok := obj.Object["_spec"].(map[string]any)
		if !ok {
			return nil
		}
		image, _ := specMap["image"].(string)
		return []msgs.ContainerImageChange{
			{Name: obj.GetName(), Image: image, Init: typ == "init"},
		}
	}

	var containerPath, initContainerPath []string
	if pluginName == "pods" {
		containerPath = []string{"spec", "containers"}
		initContainerPath = []string{"spec", "initContainers"}
	} else {
		containerPath = []string{"spec", "template", "spec", "containers"}
		initContainerPath = []string{"spec", "template", "spec", "initContainers"}
	}

	var result []msgs.ContainerImageChange
	result = appendImageChanges(result, obj, containerPath, false)
	result = appendImageChanges(result, obj, initContainerPath, true)
	return result
}

func appendImageChanges(result []msgs.ContainerImageChange, obj *unstructured.Unstructured, path []string, init bool) []msgs.ContainerImageChange {
	specs, _, _ := unstructured.NestedSlice(obj.Object, path...)
	for _, spec := range specs {
		specMap, ok := spec.(map[string]any)
		if !ok {
			continue
		}
		name, _ := specMap["name"].(string)
		image, _ := specMap["image"].(string)
		if name != "" {
			result = append(result, msgs.ContainerImageChange{Name: name, Image: image, Init: init})
		}
	}
	return result
}

func (a App) handlePortForwardRequested(msg msgs.PortForwardRequestedMsg) (tea.Model, tea.Cmd) {
	if a.pfRegistry == nil || a.k8sClient == nil {
		return a, nil
	}
	if a.pfRegistry.HasLocalPort(msg.LocalPort) {
		return a, func() tea.Msg {
			return msgs.PortForwardStartedMsg{Err: fmt.Errorf("local port %d already in use by another port-forward", msg.LocalPort)}
		}
	}
	return a, func() tea.Msg {
		apf, err := k8s.PortForward(a.k8sClient, msg.PodName, msg.PodNamespace, msg.LocalPort, msg.RemotePort)
		if err != nil {
			return msgs.PortForwardStartedMsg{Err: err}
		}
		id, err := a.pfRegistry.AddIfNotPresent(portforward.Entry{
			PodName:       msg.PodName,
			PodNamespace:  msg.PodNamespace,
			ContainerName: msg.ContainerName,
			LocalPort:     msg.LocalPort,
			RemotePort:    msg.RemotePort,
			Protocol:      msg.Protocol,
			Status:        portforward.StatusStarting,
			Cancel:        apf.Stop,
		})
		if err != nil {
			apf.Stop()
			return msgs.PortForwardStartedMsg{Err: err}
		}
		return msgs.PortForwardStartedMsg{ID: id, LocalPort: msg.LocalPort, Handle: apf}
	}
}

func watchPortForwardReady(id string, h msgs.PortForwardHandle) tea.Cmd {
	return func() tea.Msg {
		select {
		case <-h.Ready():
			return msgs.PortForwardStatusMsg{ID: id, Status: portforward.StatusReady}
		case <-h.Done():
			select {
			case err := <-h.Err():
				if err != nil {
					return msgs.PortForwardStatusMsg{ID: id, Status: portforward.StatusError, Err: err}
				}
			default:
			}
			return msgs.PortForwardStatusMsg{ID: id, Status: portforward.StatusStopped}
		}
	}
}

func watchPortForwardDone(id string, h msgs.PortForwardHandle) tea.Cmd {
	return func() tea.Msg {
		<-h.Done()
		select {
		case err := <-h.Err():
			if err != nil {
				return msgs.PortForwardStatusMsg{ID: id, Status: portforward.StatusError, Err: err}
			}
		default:
		}
		return msgs.PortForwardStatusMsg{ID: id, Status: portforward.StatusStopped}
	}
}

func (a App) handleRunCommand(run *config.RunConfig) (tea.Model, tea.Cmd) {
	focused := a.layout.FocusedSplit()
	if focused == nil || focused.Selected() == nil {
		a.statusBar.SetError("run: no resource selected")
		return a, nil
	}
	expanded, err := a.substituteVars(run.Command)
	if err != nil {
		a.statusBar.SetError(err.Error())
		return a, nil
	}
	if run.Confirm {
		a.pendingRun = &config.RunConfig{
			Command:    expanded,
			Background: run.Background,
		}
		a.confirmDialog = ui.NewConfirmDialog(fmt.Sprintf("Run: %s?", expanded), a.width)
		a.activeOverlay = overlayConfirm
		return a, nil
	}
	return a.executeRunExpanded(expanded, run.Background)
}

func (a App) executeRun(run *config.RunConfig) (tea.Model, tea.Cmd) {
	// pendingRun stores already-expanded command; otherwise expand now
	return a.executeRunExpanded(run.Command, run.Background)
}

func (a App) executeRunExpanded(expanded string, background bool) (tea.Model, tea.Cmd) {
	if background {
		return a, func() tea.Msg {
			c := exec.Command("sh", "-c", expanded)
			output, err := c.CombinedOutput()
			if err != nil {
				return msgs.ActionResultMsg{Err: fmt.Errorf("%s: %w (%s)", expanded, err, strings.TrimSpace(string(output)))}
			}
			return msgs.ActionResultMsg{ActionID: "run:" + expanded}
		}
	}
	c := exec.Command("sh", "-c", expanded)
	return a, tea.ExecProcess(c, func(err error) tea.Msg {
		if err != nil {
			return msgs.ActionResultMsg{Err: err}
		}
		return msgs.ActionResultMsg{ActionID: "run:" + expanded}
	})
}

// safeValueRe matches values that are safe to interpolate into a shell command.
var safeValueRe = regexp.MustCompile(`^[a-zA-Z0-9._:/-]*$`)

func (a App) substituteVars(cmd string) (string, error) {
	focused := a.layout.FocusedSplit()
	if focused == nil {
		return cmd, nil
	}
	selected := focused.Selected()
	if selected == nil {
		return cmd, nil
	}
	gvr := focused.Plugin().GVR()

	var validationErr error
	result := os.Expand(cmd, func(key string) string {
		var val string
		switch key {
		case "NAME":
			val = selected.GetName()
		case "NAMESPACE":
			val = selected.GetNamespace()
		case "KIND":
			val = gvr.Resource
		case "APIVERSION":
			if gvr.Group == "" {
				val = gvr.Version
			} else {
				val = gvr.Group + "/" + gvr.Version
			}
		case "PARENT":
			if focused.Plugin().Name() == "containers" {
				val = resolvePodName(focused, selected)
			}
		default:
			return "$" + key
		}
		if !safeValueRe.MatchString(val) {
			validationErr = fmt.Errorf("run: unsafe value for $%s: %q", key, val)
			return ""
		}
		return val
	})
	if validationErr != nil {
		return "", validationErr
	}
	return result, nil
}

func (a App) startLogViewForSelected() (tea.Model, tea.Cmd) {
	if lv := a.layout.LogView(); lv != nil {
		lv.ClearAndRestart()
	}
	return a.restartLogForCursor()
}

// resolvePodName returns the pod name for the selected object. When the plugin
// is "containers", the real pod name is stored in the synthetic _pod field.
func resolvePodName(split *ui.ResourceList, obj *unstructured.Unstructured) string {
	if split.Plugin().Name() == "containers" {
		if podObj, ok := obj.Object["_pod"].(map[string]any); ok {
			if name, _, _ := unstructured.NestedString(podObj, "metadata", "name"); name != "" {
				return name
			}
		}
	}
	return obj.GetName()
}

// resolveContainerName returns an explicit container name for exec/debug.
// In the containers view the selected row is the container itself. In the pods
// view we pick the first regular container from spec.containers so the
// Kubernetes API never receives an empty name (which fails for multi-container pods).
func resolveContainerName(split *ui.ResourceList, obj *unstructured.Unstructured) string {
	if split.Plugin().Name() == "containers" {
		return obj.GetName()
	}
	specs, _, _ := unstructured.NestedSlice(obj.Object, "spec", "containers")
	for _, spec := range specs {
		if m, ok := spec.(map[string]any); ok {
			if name, ok := m["name"].(string); ok {
				return name
			}
		}
	}
	return ""
}

func extractContainerNames(obj *unstructured.Unstructured) []string {
	var names []string
	specs, _, _ := unstructured.NestedSlice(obj.Object, "spec", "containers")
	for _, spec := range specs {
		if m, ok := spec.(map[string]any); ok {
			if name, ok := m["name"].(string); ok {
				names = append(names, name)
			}
		}
	}
	initSpecs, _, _ := unstructured.NestedSlice(obj.Object, "spec", "initContainers")
	for _, spec := range initSpecs {
		if m, ok := spec.(map[string]any); ok {
			if name, ok := m["name"].(string); ok {
				names = append(names, name)
			}
		}
	}
	return names
}

func fetchHelmReleasesCmd(hc helm.Client, ns string, timeout time.Duration) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		type result struct {
			releases []helm.ReleaseInfo
			err      error
		}
		ch := make(chan result, 1)
		go func() {
			r, err := hc.ListReleases(ns)
			ch <- result{r, err}
		}()
		var releases []helm.ReleaseInfo
		var err error
		select {
		case res := <-ch:
			releases, err = res.releases, res.err
		case <-ctx.Done():
			err = ctx.Err()
		}
		if err != nil {
			return msgs.HelmReleasesLoadedMsg{Namespace: ns, Err: err}
		}
		objs := make([]*unstructured.Unstructured, len(releases))
		for i, r := range releases {
			objs[i] = helm.ReleaseToUnstructured(r)
		}
		return msgs.HelmReleasesLoadedMsg{Namespace: ns, Objects: objs}
	}
}

func fetchHelmHistoryCmd(hc helm.Client, name, ns string, timeout time.Duration) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		type result struct {
			revisions []helm.RevisionInfo
			err       error
		}
		ch := make(chan result, 1)
		go func() {
			r, err := hc.History(name, ns)
			ch <- result{r, err}
		}()
		select {
		case res := <-ch:
			if res.err != nil {
				return msgs.HelmHistoryLoadedMsg{ReleaseName: name, Namespace: ns, Err: res.err}
			}
			entries := make([]msgs.HelmHistoryEntry, len(res.revisions))
			for i, r := range res.revisions {
				entries[i] = msgs.HelmHistoryEntry{
					Revision: r.Revision,
					Display:  fmt.Sprintf("Rev %d | %s | %s | %s", r.Revision, r.Status, r.Chart, r.Updated.Format("2006-01-02 15:04")),
				}
			}
			return msgs.HelmHistoryLoadedMsg{ReleaseName: name, Namespace: ns, Entries: entries}
		case <-ctx.Done():
			return msgs.HelmHistoryLoadedMsg{ReleaseName: name, Namespace: ns, Err: ctx.Err()}
		}
	}
}
