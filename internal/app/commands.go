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
	"github.com/aohoyd/aku/internal/cluster"
	"github.com/aohoyd/aku/internal/config"
	"github.com/aohoyd/aku/internal/helm"
	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/layout"
	"github.com/aohoyd/aku/internal/logs"
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
	case command == "view-describe-uncovered":
		a.envResolved = true
		return a.handleViewFocused(msgs.DetailDescribe)
	case command == "view-logs-focused":
		return a.handleViewFocused(msgs.DetailLogs)
	case command == "view-helm-values-user":
		return a.handleViewHelmValues(msgs.DetailValues)
	case command == "view-helm-values-all":
		return a.handleViewHelmValues(msgs.DetailValuesAll)
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

		childPlugin, children := drillDowner.DrillDown(a.clusterFor(focused), sel)
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

	case command == "focus-left":
		if a.layout.Orientation() == layout.OrientationVertical {
			// Vertical: left = FocusResources (exit-detail behavior)
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
		} else {
			// Horizontal: left = FocusPrev
			a.keyTrie.Reset()
			a.layout.FocusPrev()
			var descCmd tea.Cmd
			a, descCmd = a.refreshDetailPanel()
			var cmd tea.Cmd
			a, cmd = a.syncLogPanel()
			a.statusBar.SetHints(a.currentHints())
			return a, tea.Batch(descCmd, cmd)
		}
		return a, nil

	case command == "focus-right":
		if a.layout.Orientation() == layout.OrientationVertical {
			// Vertical: right = FocusDetails (focus-panel behavior)
			if a.layout.FocusedSplit() != nil && a.layout.FocusedResources() && a.layout.RightPanelVisible() {
				a.layout.FocusDetails()
				a.statusBar.SetHints(a.currentHints())
			}
		} else {
			// Horizontal: right = FocusNext
			a.keyTrie.Reset()
			a.layout.FocusNext()
			var descCmd tea.Cmd
			a, descCmd = a.refreshDetailPanel()
			var cmd tea.Cmd
			a, cmd = a.syncLogPanel()
			a.statusBar.SetHints(a.currentHints())
			return a, tea.Batch(descCmd, cmd)
		}
		return a, nil

	case command == "focus-up":
		if a.layout.Orientation() == layout.OrientationVertical {
			// Vertical: up = FocusPrev
			a.keyTrie.Reset()
			a.layout.FocusPrev()
			var descCmd tea.Cmd
			a, descCmd = a.refreshDetailPanel()
			var cmd tea.Cmd
			a, cmd = a.syncLogPanel()
			a.statusBar.SetHints(a.currentHints())
			return a, tea.Batch(descCmd, cmd)
		} else {
			// Horizontal: up = FocusResources (exit-detail behavior)
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
		}
		return a, nil

	case command == "focus-down":
		if a.layout.Orientation() == layout.OrientationVertical {
			// Vertical: down = FocusNext
			a.keyTrie.Reset()
			a.layout.FocusNext()
			var descCmd tea.Cmd
			a, descCmd = a.refreshDetailPanel()
			var cmd tea.Cmd
			a, cmd = a.syncLogPanel()
			a.statusBar.SetHints(a.currentHints())
			return a, tea.Batch(descCmd, cmd)
		} else {
			// Horizontal: down = FocusDetails (focus-panel behavior)
			if a.layout.FocusedSplit() != nil && a.layout.FocusedResources() && a.layout.RightPanelVisible() {
				a.layout.FocusDetails()
				a.statusBar.SetHints(a.currentHints())
			}
		}
		return a, nil

	case command == "toggle-orientation":
		a.layout.ToggleOrientation()
		return a, nil

	case command == "toggle-panel-focus":
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
		} else if a.layout.FocusedSplit() != nil && a.layout.RightPanelVisible() {
			a.layout.FocusDetails()
			a.statusBar.SetHints(a.currentHints())
		}
		return a, nil

	case command == "focus-next-split":
		a.keyTrie.Reset()
		a.layout.FocusNext()
		var descCmd tea.Cmd
		a, descCmd = a.refreshDetailPanel()
		var cmd tea.Cmd
		a, cmd = a.syncLogPanel()
		a.statusBar.SetHints(a.currentHints())
		return a, tea.Batch(descCmd, cmd)

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
			if a.layout.DetailZoomed() {
				a.layout.ToggleZoomDetail()
				a = a.syncIndicators()
				return a, nil
			}
			a.envResolved = false
			a.layout.FocusResources()
			a.keyTrie.Reset()
			a.statusBar.SetHints(a.currentHints())
			return a, nil
		}
		// Pop drill-down before closing panel
		if focused := a.layout.FocusedSplit(); focused != nil && focused.InDrillDown() {
			focused.PopNav()
			// Refresh with latest data from the pane's cluster store
			if store := a.storeFor(focused); store != nil {
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
					objs := store.List(focused.Plugin().GVR(), focused.EffectiveNamespace())
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
			a = a.closeFocusedSplit()
			a.statusBar.SetHints(a.currentHints())
			return a, nil
		}
		return a, nil

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
			a = a.closeFocusedSplit()
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
		if client := a.clientForFocused(); client != nil {
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

	// Context picker (global scope: gx)
	case command == "context-picker":
		a.activeOverlay = overlayContextPicker
		a.contextPicker.SetScope(true)
		a.contextPicker.SetContexts(a.contextNames())
		a.contextPicker.Open()
		return a, nil

	// Context picker (pane scope: gX)
	case command == "pane-context-picker":
		a.activeOverlay = overlayContextPicker
		a.contextPicker.SetScope(false)
		a.contextPicker.SetContexts(a.contextNames())
		a.contextPicker.Open()
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
			a.pendingDelete = objs
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
		a.pendingDelete = []*unstructured.Unstructured{selected}
		a.confirmDialog = ui.NewConfirmDialog(msg, a.width)
		a.activeOverlay = overlayConfirm
		return a, nil

	// Exec into pod
	case command == "exec":
		focused, selected, ok := a.focusedSelection()
		if !ok {
			return a, nil
		}
		execClient := a.clientForFocused()
		if execClient == nil {
			cmd := a.statusBar.SetError("exec: no k8s client")
			return a, cmd
		}
		ns := selected.GetNamespace()
		if ns == "" {
			ns = focused.Namespace()
		}
		podName := resolvePodName(focused, selected)
		containerName := resolveContainerName(focused, selected)
		return a, k8s.ExecCmd(execClient, podName, containerName, ns, a.config.ExecCommand())

	case command == "debug" || command == "debug-privileged":
		return a.handleDebug(command == "debug-privileged")

	// Toggle env resolve
	case command == "toggle-env-resolve":
		a.envResolved = !a.envResolved
		if a.layout.RightPanelVisible() {
			a.layout.RightPanel().SetEnvResolved(a.envResolved)
			if a.layout.RightPanel().Mode() == msgs.DetailDescribe {
				var descCmd tea.Cmd
				a, descCmd = a.reloadDetailPanel()
				return a, descCmd
			}
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
		focused := a.layout.FocusedSplit()
		if focused == nil {
			return a, nil
		}
		if cl := a.clusterFor(focused); cl == nil || !cl.Connected() {
			cmd := a.statusBar.SetError("rollout-restart: no k8s client")
			return a, cmd
		}
		var objs []*unstructured.Unstructured
		if focused.HasSelection() {
			objs = focused.SelectedObjects()
		} else if _, selected, ok := a.focusedSelection(); ok {
			objs = []*unstructured.Unstructured{selected}
		}
		if len(objs) == 0 {
			return a, nil
		}
		p := focused.Plugin()
		targets := make([]k8s.RestartTarget, 0, len(objs))
		names := make([]string, 0, len(objs))
		for _, obj := range objs {
			targets = append(targets, k8s.RestartTarget{
				Name:      obj.GetName(),
				Namespace: obj.GetNamespace(),
			})
			names = append(names, obj.GetName())
		}
		a.pendingRestart = targets
		a.pendingRestartGVR = p.GVR()
		msg := buildRestartConfirmMessage(p.Name(), names)
		a.confirmDialog = ui.NewConfirmDialog(msg, a.width)
		a.activeOverlay = overlayConfirm
		return a, nil
	case command == "edit":
		focused, selected, ok := a.focusedSelection()
		if !ok {
			return a, nil
		}
		editCl := a.clusterFor(focused)
		if editCl == nil || !editCl.Connected() {
			cmd := a.statusBar.SetError("edit: no k8s client")
			return a, cmd
		}
		if focused.Plugin().Name() == "helmreleases" {
			hc := a.helmClientFor(editCl)
			if hc == nil {
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
			return a, helm.EditValuesCmd(hc, name, ns)
		}
		if focused.Plugin().GVR().Group == "_ktui" {
			return a, nil
		}
		p := focused.Plugin()
		return a, k8s.EditCmd(editCl.Client().Dynamic, p.GVR(), p.IsClusterScoped(), selected)

	case command == "set-image":
		focused, selected, ok := a.focusedSelection()
		if !ok {
			return a, nil
		}
		if cl := a.clusterFor(focused); cl == nil || !cl.Connected() {
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
		if cl := a.clusterFor(focused); cl == nil || !cl.Connected() {
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
		hc := a.helmClientFor(a.clusterFor(focused))
		if hc == nil {
			cmd := a.statusBar.SetError("helm: no client")
			return a, cmd
		}
		name := selected.GetName()
		ns := selected.GetNamespace()
		a.helmRollbackOverlay.OpenLoading(name, ns)
		a.activeOverlay = overlayHelmRollback
		opCmd := a.statusBar.StartOperation()
		return a, tea.Batch(opCmd, fetchHelmHistoryCmd(hc, name, ns, a.config.APITimeout()))

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

	case command == "save-logs":
		if !a.layout.IsLogMode() {
			return a, nil
		}
		cluster, ns, pod, container, lines, ok := a.currentLogContext()
		if !ok {
			cmd := a.statusBar.SetError("No log lines to save")
			return a, cmd
		}
		path, err := logs.BuildPath(cluster, ns, pod, container, time.Now())
		if err != nil {
			cmd := a.statusBar.SetError(fmt.Sprintf("Save failed: %s", err))
			return a, cmd
		}
		if err := logs.Write(path, lines); err != nil {
			cmd := a.statusBar.SetError(fmt.Sprintf("Save failed: %s", err))
			return a, cmd
		}
		// SetWarning is reused for success confirmation: no SetInfo channel exists
		// and the 5s timeout fits a transient acknowledgement.
		cmd := a.statusBar.SetWarning(fmt.Sprintf("Saved %d lines → %s", len(lines), path))
		return a, cmd

	case command == "save-and-open-logs":
		if !a.layout.IsLogMode() {
			return a, nil
		}
		cluster, ns, pod, container, lines, ok := a.currentLogContext()
		if !ok {
			cmd := a.statusBar.SetError("No log lines to save")
			return a, cmd
		}
		path, err := logs.BuildPath(cluster, ns, pod, container, time.Now())
		if err != nil {
			cmd := a.statusBar.SetError(fmt.Sprintf("Save failed: %s", err))
			return a, cmd
		}
		if err := logs.Write(path, lines); err != nil {
			cmd := a.statusBar.SetError(fmt.Sprintf("Save failed: %s", err))
			return a, cmd
		}
		return a, logs.OpenCmd(path)

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

	// Horizontal scroll
	case command == "scroll-left":
		if a.layout.FocusedDetails() && a.layout.RightPanelVisible() {
			a.layout.ActiveDetailPanel().ScrollLeft()
		} else if focused := a.layout.FocusedSplit(); focused != nil {
			focused.ScrollLeft()
		}
		return a, nil
	case command == "scroll-right":
		if a.layout.FocusedDetails() && a.layout.RightPanelVisible() {
			a.layout.ActiveDetailPanel().ScrollRight()
		} else if focused := a.layout.FocusedSplit(); focused != nil {
			focused.ScrollRight()
		}
		return a, nil
	case command == "scroll-home":
		if a.layout.FocusedDetails() && a.layout.RightPanelVisible() {
			a.layout.ActiveDetailPanel().ScrollHome()
		} else if focused := a.layout.FocusedSplit(); focused != nil {
			focused.ScrollHome()
		}
		return a, nil
	case command == "scroll-end":
		if a.layout.FocusedDetails() && a.layout.RightPanelVisible() {
			a.layout.ActiveDetailPanel().ScrollEnd()
		} else if focused := a.layout.FocusedSplit(); focused != nil {
			focused.ScrollEnd()
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

// currentLogContext returns the metadata needed to save the current log view's
// contents to disk. It returns ok=false when there are no buffered lines or any
// of ns/pod/container are missing. When no k8s client is attached, cluster
// falls back to "unknown-cluster" so the on-disk layout still has a directory
// level for the cluster.
func (a *App) currentLogContext() (cluster, ns, pod, container string, lines []string, ok bool) {
	lv := a.layout.LogView()
	lines = lv.RawLines()
	if len(lines) == 0 {
		return "", "", "", "", nil, false
	}
	ns = lv.Namespace()
	pod = lv.PodName()
	container = lv.ActiveContainer()
	if ns == "" || pod == "" || container == "" {
		return "", "", "", "", nil, false
	}
	cluster = "unknown-cluster"
	if cl := a.clusterForFocused(); cl != nil && cl.Context() != "" {
		cluster = cl.Context()
	}
	return cluster, ns, pod, container, lines, true
}

// closeRightPanel performs full right-panel teardown: stops log stream,
// resets mode/state, hides panel, and refreshes indicators/hints.
func (a App) closeRightPanel() App {
	a = a.stopLogStream()
	a.layout.SetLogMode(false)
	a.envResolved = false
	a.lastDetailKey = ""
	a.layout.FocusResources()
	a.keyTrie.Reset()
	a.layout.HideRightPanel()
	a = a.syncIndicators()
	a.statusBar.SetHints(a.currentHints())
	return a
}

// closeFocusedSplit removes the currently-focused split and releases the
// resources it held. Callers must guarantee SplitCount() > 1 (the last split is
// never closed).
//
// Refcount invariant enforced here: a split holds exactly one manager reference
// iff it is PINNED to a NON-GLOBAL context (Task 9 takes that ref via Acquire on
// pin; a pane following global holds no ref). So on close we Release(closingCtx)
// only in that case, keeping Acquire/Release balanced with handlePaneContextSwitch.
//
// Order matters: the closing split's (gvr, ns) is unsubscribed on the CLOSING
// split's OWN cluster store FIRST (resolved before removal — Release may tear the
// cluster down and drop the store), then the split is removed from the layout (so
// unsubscribeOnStoreIfUnused's "still in use?" scan no longer sees the closing
// pane), then Release decrements the ref. If several panes share the same pinned
// context, Release just decrements and the Manager tears down only at the last
// one.
func (a App) closeFocusedSplit() App {
	closing := a.layout.FocusedSplit()
	if closing == nil {
		return a
	}

	closingCtx := closing.Context()
	closingGVR := closing.Plugin().GVR()
	closingNs := closing.EffectiveNamespace()

	// Resolve the closing split's cluster store BEFORE removal/reconcile:
	// SyncRefs can tear down a non-global cluster and drop its store, so we must
	// grab it while the cluster is still live.
	closingStore := a.storeFor(closing)

	a.keyTrie.Reset()
	a.layout.CloseCurrentSplit()

	// Stop the closing split's informer for (gvr, ns) on its OWN cluster store
	// when no remaining pane on that cluster still needs it. Done after removal so
	// the "still in use?" scan does not count the just-closed pane.
	a.unsubscribeOnStoreIfUnused(closingStore, closingCtx, closingGVR, closingNs)

	// Reconcile manager refcounts against the panes that remain pinned; a
	// non-global cluster no remaining pane references is torn down here. The
	// global is never torn down.
	a.mgr.SyncRefs(a.pinnedContexts())

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
	if a.storeFor(focused) != nil && oldPlugin.GVR() != p.GVR() {
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

	// New panes inherit the currently-focused pane's context (the cluster the
	// user is looking at). Resolve its default namespace from that cluster.
	inheritCtx := a.mgr.GlobalContext()
	if f := a.layout.FocusedSplit(); f != nil {
		inheritCtx = f.Context()
	}
	ns := "default"
	if cl := a.clusterFor(a.layout.FocusedSplit()); cl != nil && cl.Client() != nil {
		ns = cl.Client().Namespace
	}
	if prev := a.layout.FocusedSplit(); prev != nil {
		if prev.Namespace() != "" {
			ns = prev.Namespace()
		}
	}

	a.keyTrie.Reset()
	a.layout.AddSplit(p, ns)
	// New splits inherit the global context (no footer), but keep footers in
	// sync across all panes for consistency.
	a.syncPaneFooters()
	a.layout.FocusResources()
	newSplit := a.layout.FocusedSplit()
	newSplit.SetContext(inheritCtx)
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

	a.lastDetailKey = ""

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

	a.lastDetailKey = ""

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

// handleViewHelmValues switches the detail panel into a Helm values mode
// (DetailValues for user-supplied, DetailValuesAll for the full coalesced set).
// Only valid for helmrelease rows; for any other plugin it is a no-op.
func (a App) handleViewHelmValues(mode msgs.DetailMode) (tea.Model, tea.Cmd) {
	focused, _, ok := a.focusedSelection()
	if !ok {
		return a, nil
	}
	if focused.Plugin().Name() != "helmreleases" {
		return a, nil
	}
	// Match the pattern used by other helm actions (see `edit`,
	// `helm-rollback`): surface an explicit status when no helm client is
	// configured rather than silently falling through to the manifest path.
	if a.helmClientForFocused() == nil {
		cmd := a.statusBar.SetError("helm: no client")
		return a, cmd
	}

	a.lastDetailKey = ""

	// Switching away from log mode (defensive — values views are YAML).
	a = a.stopLogStream()
	a.layout.SetLogMode(false)

	a.layout.ShowRightPanel()
	panel := a.layout.RightPanel()
	panel.SetMode(mode)
	a, descCmd := a.refreshDetailPanel()
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
	cl := a.clusterFor(split)
	store := plugin.StoreOf(cl)
	if store != nil && p.GVR().Group != "_ktui" {
		objs := store.Subscribe(p.GVR(), ns)
		split.SetObjects(objs)
		return nil
	}
	if sp, ok := p.(plugin.SelfPopulating); ok {
		if _, ok := p.(plugin.Refreshable); ok {
			if hc := a.helmClientFor(cl); hc != nil {
				split.SetObjects(sp.Objects()) // show cached data immediately
				opCmd := a.statusBar.StartOperation()
				return tea.Batch(opCmd, fetchHelmReleasesCmd(hc, ns, a.config.APITimeout()))
			}
		}
		split.SetObjects(sp.Objects())
	}
	return nil
}

func (a App) reloadAll() (tea.Model, tea.Cmd) {
	wasLogMode := a.layout.IsLogMode()
	wasRightVisible := a.layout.RightPanelVisible()

	// Tear down all informers and clear store cache across every live cluster.
	a.mgr.ForEach(func(c *cluster.Cluster) {
		if s := c.Store(); s != nil {
			s.UnsubscribeAll()
		}
	})
	a = a.stopLogStream()
	a.layout.SetLogMode(false)

	// Dismiss any open overlays
	a.activeOverlay = overlayNone
	a.pendingRun = nil
	a.pendingDelete = nil
	a.pendingDebug = nil
	a.pendingRestart = nil
	a.pendingRestartGVR = schema.GroupVersionResource{}

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
	// Unsubscribe on the focused split's cluster store (the GVR/ns being torn
	// down belongs to the operation that triggered this, which runs on the
	// focused pane's cluster). Through Task 6 there is a single cluster.
	if store := a.storeForFocused(); store != nil {
		store.Unsubscribe(gvr, namespace)
	}
}

// contextNames returns the kube-context names known to the manager, mapped from
// its ContextEntry list. ScanKubeconfigs already sorts and dedupes entries, so
// the slice is returned in that order without further processing.
func (a App) contextNames() []string {
	entries := a.mgr.Entries()
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name)
	}
	return names
}

// handleGlobalContextSwitch changes the app's baseline (global) cluster and
// retargets every pane that is still following global (not pinned) onto the new
// cluster. Pinned panes are left untouched.
//
// Connect strategy: synchronous, with a GetOrCreate-first guard. Manager.
// SetGlobal sets the global pointer unconditionally (even for a degraded
// cluster), so a naive SetGlobal-then-check would leave global pointing at a
// broken cluster when the switch fails. Instead we GetOrCreate the target first
// and only call SetGlobal once we know it connected — so a failed switch never
// moves the global off the working cluster. (Per the plan, async connect via
// ClusterReadyMsg is introduced for the per-pane path in Task 9; the global
// switch stays synchronous here, matching how startup connects.)
func (a App) handleGlobalContextSwitch(ctx string) (tea.Model, tea.Cmd) {
	a.activeOverlay = overlayNone

	// No-op if we're already on this context.
	if ctx == a.mgr.GlobalContext() {
		return a, nil
	}

	// Connect (or look up) the target cluster WITHOUT yet promoting it to
	// global. If it fails to connect, report the error and keep the current
	// working global — do not retarget any panes.
	cl, err := a.mgr.GetOrCreate(ctx)
	if err != nil || cl == nil || !cl.Connected() {
		msg := fmt.Sprintf("context %s: failed to connect", ctx)
		if err != nil {
			msg = fmt.Sprintf("context %s: %s", ctx, err)
		}
		cmd := a.statusBar.SetError(msg)
		return a, cmd
	}

	// Remember the old global so we can clean up its now-unused informers.
	oldGlobal := a.mgr.GlobalContext()
	oldStore := a.storeForContext(oldGlobal)

	// Promote the connected cluster to global.
	if _, err := a.mgr.SetGlobal(ctx); err != nil {
		cmd := a.statusBar.SetError(fmt.Sprintf("context %s: %s", ctx, err))
		return a, cmd
	}

	// Update the status bar to reflect the new baseline.
	a.statusBar.SetContextName(ctx)
	newNs := ""
	if cl.Client() != nil {
		newNs = cl.Client().Namespace
	}

	// Stop any log stream — log streams are bound to their cluster's client and
	// the focused pane may be following global.
	wasLogMode := a.layout.IsLogMode()
	a = a.stopLogStream()
	a.layout.SetLogMode(false)
	a.envResolved = false

	// Retarget every following (unpinned) pane onto the new global cluster.
	type prevSub struct {
		gvr schema.GroupVersionResource
		ns  string
	}
	var oldSubs []prevSub
	var populateCmds []tea.Cmd
	for i := range a.layout.SplitCount() {
		split := a.layout.SplitAt(i)
		if split == nil || split.Pinned() {
			continue
		}

		// Capture the pane's previous (gvr, ns) for old-store cleanup.
		oldSubs = append(oldSubs, prevSub{gvr: split.Plugin().GVR(), ns: split.EffectiveNamespace()})

		// Point the pane at the new global, landing on its default namespace,
		// and clear stale data from the old cluster.
		split.SetContext(ctx)
		if newNs != "" {
			split.SetNamespace(newNs)
		}
		split.ResetNav()
		split.SetObjects(nil)

		// Best-effort missing-resource check: if the new cluster's discovery is
		// populated and does not know this GVR, leave the pane empty with an
		// informational message rather than subscribing. Synthetic (_ktui) GVRs
		// have no informer and are always "available". When discovery has not
		// refreshed yet (IsEmpty) we cannot be authoritative, so we subscribe
		// and rely on an empty informer yielding an empty list; the discovery
		// refresh dispatched below will populate the index for next time.
		gvr := split.Plugin().GVR()
		if gvr.Group != "_ktui" && cl.Discovery() != nil && !cl.Discovery().IsEmpty() {
			if _, ok := cl.Discovery().KindForGVR(gvr); !ok {
				continue
			}
		}

		if cmd := a.subscribeAndPopulate(split, split.Plugin(), split.EffectiveNamespace()); cmd != nil {
			populateCmds = append(populateCmds, cmd)
		}
	}

	// Clean up informers on the OLD global store that no retargeted pane needs
	// anymore. The old cluster itself is NOT torn down — it remains a valid
	// cluster the user may switch back to, and pinned panes may still use it.
	if oldStore != nil {
		for _, sub := range oldSubs {
			a.unsubscribeOnStoreIfUnused(oldStore, oldGlobal, sub.gvr, sub.ns)
		}
	}

	// Refresh discovery for the new global and re-arm the heartbeat on the new
	// global's client so connectivity status tracks the new cluster. Both mirror
	// the cmds Init() dispatches at startup.
	cmds := populateCmds
	if disc := cl.Discovery(); disc != nil && cl.Client() != nil {
		typed := cl.Client().Typed
		newCtx := ctx
		cmds = append(cmds, func() tea.Msg {
			resources, derr := disc.Refresh(typed)
			return k8s.APIResourcesDiscoveredMsg{Context: newCtx, Resources: resources, Err: derr}
		})
	}
	if cl.Client() != nil {
		cmds = append(cmds, initialHeartbeatCmd(ctx, cl.Client()))
	}

	// Restore detail/log panel state.
	if wasLogMode {
		if focused := a.layout.FocusedSplit(); focused != nil && isLoggablePlugin(focused.Plugin().Name()) {
			a.layout.SetLogMode(true)
			if lv := a.layout.LogView(); lv != nil {
				lv.ClearAndRestart()
				lv.SetUnavailable(true)
			}
		}
	}
	var descCmd tea.Cmd
	a, descCmd = a.refreshDetailPanel()
	if descCmd != nil {
		cmds = append(cmds, descCmd)
	}

	a.statusBar.SetHints(a.currentHints())
	a = a.syncIndicators()

	return a, tea.Batch(cmds...)
}

// storeForContext returns the informer store for a given context name without
// creating the cluster. Returns nil when the cluster is absent or degraded.
func (a App) storeForContext(ctx string) *k8s.Store {
	if c, ok := a.mgr.Get(ctx); ok {
		return c.Store()
	}
	return nil
}

// unsubscribeOnStoreIfUnused unsubscribes (gvr, ns) on the given store when no
// remaining pane bound to ctxName still needs that pair (either as its visible
// resource or as a parent in its drill-down nav stack). Synthetic (_ktui) GVRs
// have no informer and are skipped. This mirrors unsubscribeIfUnused but targets
// an explicit store/context, which the global switch needs because the panes
// have already been retargeted off the old store.
func (a App) unsubscribeOnStoreIfUnused(store *k8s.Store, ctxName string, gvr schema.GroupVersionResource, namespace string) {
	if store == nil || gvr.Group == "_ktui" {
		return
	}
	for i := range a.layout.SplitCount() {
		s := a.layout.SplitAt(i)
		if s == nil || s.Context() != ctxName {
			continue
		}
		if s.Plugin().GVR() == gvr && s.EffectiveNamespace() == namespace {
			return // still in use by a pane on this cluster
		}
		if s.NavStackHasGVR(gvr, namespace) {
			return // still needed by a drill-down parent on this cluster
		}
	}
	store.Unsubscribe(gvr, namespace)
}

// asyncConnectCmd performs ONLY the blocking dial for ctxName off the Bubble Tea
// Update goroutine and reports completion via msgs.ClusterReadyMsg. It calls
// Manager.Dial, which runs k8s.NewClient (blocking REST/raw-config reads that
// must never run on the Update goroutine — Risk 3) WITHOUT touching any Manager
// state. The dialed *k8s.Client (an immutable handle) travels back in the
// message; all Manager map/refcount mutation happens later in handleClusterReady
// on the Update goroutine, keeping the lock-free Manager invariant intact.
func asyncConnectCmd(mgr *cluster.Manager, ctxName string) tea.Cmd {
	return func() tea.Msg {
		client, err := mgr.Dial(ctxName)
		return msgs.ClusterReadyMsg{Context: ctxName, Client: client, Err: err}
	}
}

// handlePaneContextSwitch pins the focused pane to ctxName and runs it live on
// that cluster, simultaneously with other panes on other clusters (true
// side-by-side multi-cluster).
//
// Approach (optimistic pin, no pending-map): the focused pane is immediately
// pinned and re-pointed at ctxName, its stale data cleared, and a "connecting"
// status shown. The actual connect happens off-thread (asyncConnectCmd); when
// msgs.ClusterReadyMsg arrives, handleClusterReady completes the switch
// (subscribe/populate) or surfaces an error, leaving the pane pinned-but-empty
// on failure so the user can switch again. The awaiting pane is identified on
// completion by (Pinned && Context()==ctxName) — see handleClusterReady.
//
// Picking any context always PINS the pane (even when ctxName equals the global
// context — the footer added in Task 10 simply will not show in that case).
// There is no "unpin / follow global" affordance here; that is out of scope.
//
// Refcount bookkeeping is reconciliation-based (idempotent, order-independent):
// after every pin/repin/close we call Manager.SyncRefs(pinnedContexts()), which
// makes each cluster's refCount equal the number of panes currently pinned to it
// and tears down any non-global cluster with zero pinned panes. The global is
// never torn down. This is robust under rapid re-pins and focus changes: a stale
// ClusterReadyMsg for a context no pane is pinned to anymore reconciles to zero
// and is cleaned up. The actual connect (k8s.NewClient) happens off-thread in
// asyncConnectCmd via Manager.Dial; install + reconcile happen on the Update
// goroutine in handleClusterReady.
func (a App) handlePaneContextSwitch(ctxName string) (tea.Model, tea.Cmd) {
	a.activeOverlay = overlayNone

	focused := a.layout.FocusedSplit()
	if focused == nil {
		return a, nil
	}

	old := focused.Context()
	wasPinned := focused.Pinned()

	// No-op only when already pinned to this exact context AND its cluster is
	// connected. A pane left pinned-but-broken by a failed connect must be able to
	// retry: re-selecting the same context re-attempts the dial rather than
	// dead-ending here.
	if wasPinned && old == ctxName {
		if cl, ok := a.mgr.Get(ctxName); ok && cl.Connected() {
			return a, nil
		}
	}

	// Stop any log stream on the focused pane: log streams are bound to the old
	// cluster's client.
	a = a.stopLogStream()
	a.layout.SetLogMode(false)
	a.envResolved = false

	// Optimistically pin and re-point the pane, clearing stale data from the old
	// cluster. The new cluster's data arrives via ClusterReadyMsg.
	focused.SetPinned(true)
	focused.SetContext(ctxName)
	focused.ResetNav()
	focused.SetObjects(nil)

	// Reconcile refcounts against the new pinned set. The target cluster may not
	// be in the Manager yet (still dialing); SyncRefs counts what it can now and
	// is called again in handleClusterReady once the dial returns. Any cluster the
	// pane just stopped pinning (its old context) that no other pane holds is torn
	// down here.
	a.mgr.SyncRefs(a.pinnedContexts())

	cmd := a.statusBar.SetWarning(fmt.Sprintf("connecting to %s…", ctxName))
	return a, tea.Batch(cmd, asyncConnectCmd(a.mgr, ctxName))
}

// pinnedContexts returns the set of contexts currently pinned by some pane — the
// authoritative input to Manager.SyncRefs. A pane contributes its context iff it
// is pinned and its context is non-empty.
func (a App) pinnedContexts() []string {
	var ctxs []string
	for i := 0; i < a.layout.SplitCount(); i++ {
		split := a.layout.SplitAt(i)
		if split == nil || !split.Pinned() {
			continue
		}
		if c := split.Context(); c != "" {
			ctxs = append(ctxs, c)
		}
	}
	return ctxs
}

// handleClusterReady completes a per-pane connect once the async connect cmd
// reports back. It applies the connected cluster to the focused pane when that
// pane is pinned to msg.Context and awaiting data; on failure it surfaces an
// error and leaves the pane pinned-but-empty.
func (a App) handleClusterReady(msg msgs.ClusterReadyMsg) (tea.Model, tea.Cmd) {
	client, _ := msg.Client.(*k8s.Client)
	if msg.Err != nil || client == nil {
		// Failed dial: nothing was registered and no ref was taken. Reconcile so a
		// context the user re-pinned away from is torn down, then report the error
		// and leave the awaiting pane(s) pinned-but-empty so the user can retry.
		a.mgr.SyncRefs(a.pinnedContexts())
		errMsg := fmt.Sprintf("context %s: failed to connect", msg.Context)
		if msg.Err != nil {
			errMsg = fmt.Sprintf("context %s: %s", msg.Context, msg.Err)
		}
		cmd := a.statusBar.SetError(errMsg)
		return a, cmd
	}

	// Install the dialed client on the Update goroutine. RegisterConnected returns
	// the already-cached cluster if another pane connected this context first
	// (discarding the redundant client); newlyConnected gates arming a fresh
	// heartbeat/discovery so two panes on the same context do not start duplicate
	// heartbeat loops.
	cl, newlyConnected := a.mgr.RegisterConnected(msg.Context, client)

	// Reconcile refcounts against the panes currently pinned. If no pane is pinned
	// to msg.Context anymore (the requester re-pinned elsewhere), this tears the
	// just-registered cluster back down — no leak.
	a.mgr.SyncRefs(a.pinnedContexts())

	if cl == nil || !cl.Connected() {
		cmd := a.statusBar.SetError(fmt.Sprintf("context %s: failed to connect", msg.Context))
		return a, cmd
	}

	var cmds []tea.Cmd

	// Arm discovery + heartbeat only on the cluster's first connect.
	if newlyConnected && cl.Client() != nil {
		if disc := cl.Discovery(); disc != nil {
			typed := cl.Client().Typed
			newCtx := msg.Context
			cmds = append(cmds, func() tea.Msg {
				resources, derr := disc.Refresh(typed)
				return k8s.APIResourcesDiscoveredMsg{Context: newCtx, Resources: resources, Err: derr}
			})
		}
		cmds = append(cmds, initialHeartbeatCmd(msg.Context, cl.Client()))
	}

	// Apply to EVERY pinned split awaiting this context (identified by pane
	// context across ALL splits, NOT by focus — if the user moved focus between
	// dispatch and arrival, the correct non-focused pinned pane is still
	// populated). Re-applying to an already-populated pane is harmless.
	defaultNs := ""
	if cl.Client() != nil {
		defaultNs = cl.Client().Namespace
	}
	for i := range a.layout.SplitCount() {
		split := a.layout.SplitAt(i)
		if split == nil || !split.Pinned() || split.Context() != msg.Context {
			continue
		}
		if defaultNs != "" {
			split.SetNamespace(defaultNs)
		}
		gvr := split.Plugin().GVR()
		// Missing-resource check: if discovery is populated and does not know this
		// GVR, leave the pane empty with a message instead of subscribing.
		// Synthetic (_ktui) GVRs are always available; the check is only
		// authoritative once discovery has loaded (IsEmpty).
		if gvr.Group != "_ktui" && cl.Discovery() != nil && !cl.Discovery().IsEmpty() {
			if _, known := cl.Discovery().KindForGVR(gvr); !known {
				split.SetObjects(nil)
				cmds = append(cmds, a.statusBar.SetWarning(fmt.Sprintf("%s not available on %s", gvr.Resource, msg.Context)))
				continue
			}
		}
		if pc := a.subscribeAndPopulate(split, split.Plugin(), split.EffectiveNamespace()); pc != nil {
			cmds = append(cmds, pc)
		}
	}

	// A pane's context is now live: refresh per-pane footers so a pane that
	// differs from the global context shows its footer.
	a.syncPaneFooters()

	// Clear the transient "connecting…" status now that we are live.
	a.statusBar.SetError("")

	var dc tea.Cmd
	a, dc = a.refreshDetailPanel()
	if dc != nil {
		cmds = append(cmds, dc)
	}
	a.statusBar.SetHints(a.currentHints())
	return a, tea.Batch(cmds...)
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

// executeDelete handles deletion of one or more resources. The same helper
// covers both single-select (N=1) and multi-select (N>1) paths. Behavior
// branches by the focused plugin's name: helm releases use the helm client,
// port-forwards use the in-process registry (no async cmd), everything else
// goes through the k8s dynamic client. ActionID strings preserve the existing
// format: N=1 success → `<verb>:<name>`; N>1 success → `<verb>:%d-resources`
// (or `helm-uninstall:%d` for helm). Failures aggregate via the same
// `bulk delete: %d/%d failed: ...` format used previously by the bulk path.
func (a App) executeDelete(targets []*unstructured.Unstructured, force bool) (tea.Model, tea.Cmd) {
	focused := a.layout.FocusedSplit()
	if focused == nil || len(targets) == 0 {
		return a, nil
	}
	p := focused.Plugin()
	n := len(targets)

	// Helm releases
	if p.Name() == "helmreleases" {
		helmClient := a.helmClientFor(a.clusterFor(focused))
		if helmClient == nil {
			return a, nil
		}
		return a, func() tea.Msg {
			var errs []string
			var firstErr error
			for _, obj := range targets {
				if err := helmClient.Uninstall(obj.GetName(), obj.GetNamespace()); err != nil {
					if firstErr == nil {
						firstErr = err
					}
					errs = append(errs, obj.GetName()+": "+err.Error())
				}
			}
			if len(errs) > 0 {
				if n == 1 {
					return msgs.ActionResultMsg{Err: fmt.Errorf("delete %s: %w", targets[0].GetName(), firstErr)}
				}
				return msgs.ActionResultMsg{Err: fmt.Errorf("bulk delete: %d/%d failed: %s", len(errs), n, strings.Join(errs, "; "))}
			}
			if n == 1 {
				return msgs.ActionResultMsg{ActionID: "helm-uninstall:" + targets[0].GetName()}
			}
			return msgs.ActionResultMsg{ActionID: fmt.Sprintf("helm-uninstall:%d", n)}
		}
	}

	// Port-forwards: synchronous, no async cmd.
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
	client := a.clientForFocused()
	if client == nil {
		return a, nil
	}
	gvr := p.GVR()
	clusterScoped := p.IsClusterScoped()

	return a, func() tea.Msg {
		opts := metav1.DeleteOptions{}
		if force {
			opts.GracePeriodSeconds = new(int64)
		}
		var errs []string
		var firstErr error
		for _, obj := range targets {
			var err error
			if clusterScoped {
				err = client.Dynamic.Resource(gvr).Delete(context.Background(), obj.GetName(), opts)
			} else {
				err = client.Dynamic.Resource(gvr).Namespace(obj.GetNamespace()).Delete(context.Background(), obj.GetName(), opts)
			}
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				errs = append(errs, obj.GetName()+": "+err.Error())
			}
		}
		if len(errs) > 0 {
			if n == 1 {
				return msgs.ActionResultMsg{Err: fmt.Errorf("delete %s: %w", targets[0].GetName(), firstErr)}
			}
			return msgs.ActionResultMsg{Err: fmt.Errorf("bulk delete: %d/%d failed: %s", len(errs), n, strings.Join(errs, "; "))}
		}
		if n == 1 {
			return msgs.ActionResultMsg{ActionID: "delete:" + targets[0].GetName()}
		}
		return msgs.ActionResultMsg{ActionID: fmt.Sprintf("delete:%d-resources", n)}
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
	debugClient := a.clientForFocused()
	if debugClient == nil {
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

	return a, k8s.DebugCmd(debugClient, podName, containerName, ns, image, command, false)
}

// executePendingDebug runs a debug action that was confirmed by the user.
func (a App) executePendingDebug(dbg *pendingDebugAction) (tea.Model, tea.Cmd) {
	client := a.clientForFocused()
	if client == nil {
		a.statusBar.SetError("debug: no k8s client")
		return a, nil
	}
	if dbg.nodeMode {
		return a, k8s.DebugNodeCmd(client, dbg.nodeName, dbg.image, dbg.command)
	}
	return a, k8s.DebugCmd(client, dbg.podName, dbg.containerName, dbg.namespace, dbg.image, dbg.command, dbg.privileged)
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
	// Empty pattern: clear immediately without debounce.
	// Bump seq to invalidate any pending debounce from a previous non-empty pattern.
	if msg.Pattern == "" {
		a.searchDebounceSeq++
		if msg.Mode == msgs.SearchModeFilter {
			target.ClearFilter()
		} else {
			target.ClearSearch()
		}
		a.searchBar.SetError("")
		if a.layout.FocusedResources() {
			var descCmd tea.Cmd
			a, descCmd = a.refreshDetailPanel()
			return a, descCmd
		}
		return a, nil
	}
	// Non-empty pattern: debounce
	a.searchDebounceSeq++
	return a, a.searchDebounceCmd(msg.Pattern, msg.Mode)
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
	client := a.clientForFocused()
	if client == nil {
		return a, nil
	}
	return a, k8s.SetImageCmd(client.Dynamic, msg.GVR, msg.ResourceName, msg.Namespace, msg.PluginName, msg.Images)
}

func (a App) handleScaleRequested(msg msgs.ScaleRequestedMsg) (tea.Model, tea.Cmd) {
	client := a.clientForFocused()
	if client == nil {
		return a, nil
	}
	return a, k8s.ScaleCmd(client.Dynamic, msg.GVR, msg.ResourceName, msg.Namespace, msg.Replicas)
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
	// Capture the focused cluster's client into a local before the closure so
	// the forward stays pinned to its cluster across later context switches.
	client := a.clientForFocused()
	if a.pfRegistry == nil || client == nil {
		return a, nil
	}
	if a.pfRegistry.HasLocalPort(msg.LocalPort) {
		return a, func() tea.Msg {
			return msgs.PortForwardStartedMsg{Err: fmt.Errorf("local port %d already in use by another port-forward", msg.LocalPort)}
		}
	}
	return a, func() tea.Msg {
		apf, err := k8s.PortForward(client, msg.PodName, msg.PodNamespace, msg.LocalPort, msg.RemotePort)
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

// buildRestartConfirmMessage returns the confirmation prompt for a rollout
// restart. pluginName is the (plural) plugin identifier such as "deployments";
// for N=1 it is singularized by stripping a trailing "s". The list of names is
// truncated to 20 entries with a "... and X more" trailer.
func buildRestartConfirmMessage(pluginName string, names []string) string {
	resource := pluginName
	if len(names) == 1 {
		resource = strings.TrimSuffix(pluginName, "s")
	}
	displayed := names
	var trailer string
	if len(names) > 20 {
		displayed = names[:20]
		trailer = fmt.Sprintf("\n  ... and %d more", len(names)-20)
	}
	nameList := strings.Join(displayed, "\n  - ")
	return fmt.Sprintf("Rollout restart %d %s?\n\n  - %s%s", len(names), resource, nameList, trailer)
}
