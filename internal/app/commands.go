package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/aohoyd/aku/internal/clipboard"
	"github.com/aohoyd/aku/internal/cluster"
	"github.com/aohoyd/aku/internal/config"
	"github.com/aohoyd/aku/internal/helm"
	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/k8s/session"
	"github.com/aohoyd/aku/internal/layout"
	"github.com/aohoyd/aku/internal/logs"
	"github.com/aohoyd/aku/internal/manifest"
	"github.com/aohoyd/aku/internal/msgs"
	"github.com/aohoyd/aku/internal/notify"
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
	// Per-pane context switch dispatched by the contexts plugin's Commander:
	// "pane-switch-context <name>". Matched before the generic goto-/split-
	// prefixes (it carries an argument).
	case strings.HasPrefix(command, "pane-switch-context "):
		ctxName := strings.TrimSpace(strings.TrimPrefix(command, "pane-switch-context "))
		return a.handlePaneSwitchContext(ctxName)

	// Open the contexts plugin IN the focused pane (gX): push the current
	// resource onto the nav stack so Esc returns to it.
	case command == "goto-contexts":
		return a.handleGotoContexts()

	// Navigation: goto-<resource>
	case strings.HasPrefix(command, "goto-"):
		resourceName := strings.TrimPrefix(command, "goto-")
		return a.handleGoto(resourceName, "")

	// Split a child/parent drill into a NEW floor-pinned split. These exact-match
	// cases must precede the generic split-<resource> prefix below: otherwise
	// handleSplit would be handed "children"/"parent" as a resource name and emit
	// an "unknown resource" error rather than performing the drill.
	case command == "split-children":
		focused := a.layout.FocusedSplit()
		if focused == nil {
			return a, nil
		}
		sel := focused.Selected()
		if sel == nil {
			return a, nil
		}
		dd, ok := focused.Plugin().(plugin.DrillDowner)
		if !ok {
			return a, nil
		}
		childPlugin, children := dd.DrillDown(a.clusterFor(focused), sel)
		if childPlugin == nil {
			return a, nil
		}
		// DrillDown already subscribed the child GVR internally; openDrilledSplit
		// subscribes/populates the ROOT plugin then PushNav-installs this child subset.
		return a.openDrilledSplit(focused, childPlugin, children, sel, ui.NavChild)

	case command == "split-parent":
		focused := a.layout.FocusedSplit()
		if focused == nil {
			return a, nil
		}
		sel := focused.Selected()
		if sel == nil {
			return a, nil
		}
		du, ok := focused.Plugin().(plugin.DrillUp)
		if !ok {
			return a, nil
		}
		parentPlugin, parentObj := du.DrillUp(a.clusterFor(focused), sel)
		if parentPlugin == nil {
			return a, nil
		}
		var objs []*unstructured.Unstructured
		if parentObj != nil {
			objs = []*unstructured.Unstructured{parentObj}
		}
		// DrillUp already subscribed the parent GVR internally; openDrilledSplit
		// subscribes/populates the ROOT plugin then PushNav-installs this parent subset.
		return a.openDrilledSplit(focused, parentPlugin, objs, sel, ui.NavParent)

	// Split a node-ward drill (oN) into a NEW floor-pinned split. Mirrors
	// split-parent but uses NodeLinker/GoToNode. MUST precede the generic
	// split-<resource> prefix below: otherwise it is trimmed to "nav-node" and
	// handed to handleSplit as a resource name.
	case command == "split-nav-node":
		focused := a.layout.FocusedSplit()
		if focused == nil {
			return a, nil
		}
		sel := focused.Selected()
		if sel == nil {
			return a, nil
		}
		nl, ok := focused.Plugin().(plugin.NodeLinker)
		if !ok {
			return a, nil
		}
		nodePlugin, node := nl.GoToNode(a.clusterFor(focused), sel)
		if nodePlugin == nil {
			return a, nil
		}
		var objs []*unstructured.Unstructured
		if node != nil {
			objs = []*unstructured.Unstructured{node}
		}
		// GoToNode already subscribed the nodes GVR internally; openDrilledSplit
		// subscribes/populates the ROOT plugin then PushNav-installs this node subset.
		return a.openDrilledSplit(focused, nodePlugin, objs, sel, ui.NavNode)

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

		// Check Commander first so a context-switch plugin (which maps Enter to an
		// app command string) takes precedence over goto/drill-down behavior.
		if commander, ok := focused.Plugin().(plugin.Commander); ok {
			if cmd, isCmd := commander.Command(sel); isCmd {
				return a.executeCommand(cmd)
			}
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
		focused.PushNav(childPlugin, children, sel.GetName(), string(sel.GetUID()), sel.GetAPIVersion(), sel.GetKind(), ui.NavChild)
		if a.layout.RightPanelVisible() {
			a.layout.FocusResources()
		}
		var cmd tea.Cmd
		a, cmd = a.refreshDetailPanelOrLog()
		a.keyTrie.Reset()
		a.envResolved = false
		a.statusBar.SetHints(a.currentHints())

		return a, cmd

	// Go to parent (Backspace): inverse of enter-detail's drill-down. Resolves
	// the selected resource's real K8s parent (via ownerReferences) and pushes it
	// onto the nav stack as a parent-ward frame. A nil selection, a plugin without
	// DrillUp, or an unresolvable/absent owner are all plain no-ops (unlike
	// enter-detail's nil-sel branch, a nil selection here does NOT focus the
	// detail panel).
	case command == "nav-parent":
		focused := a.layout.FocusedSplit()
		if focused == nil {
			return a, nil
		}
		sel := focused.Selected()
		if sel == nil {
			return a, nil
		}
		du, ok := focused.Plugin().(plugin.DrillUp)
		if !ok {
			return a, nil
		}
		parentPlugin, parentObj := du.DrillUp(a.clusterFor(focused), sel)
		if parentPlugin == nil {
			return a, nil
		}
		var objs []*unstructured.Unstructured
		if parentObj != nil {
			objs = []*unstructured.Unstructured{parentObj}
		}
		focused.PushNav(parentPlugin, objs, sel.GetName(), string(sel.GetUID()), sel.GetAPIVersion(), sel.GetKind(), ui.NavParent)
		if a.layout.RightPanelVisible() {
			a.layout.FocusResources()
		}
		var cmd tea.Cmd
		a, cmd = a.refreshDetailPanelOrLog()
		a.keyTrie.Reset()
		a.envResolved = false
		a.statusBar.SetHints(a.currentHints())

		return a, cmd

	// Go to hosting Node (gN): mirrors nav-parent but follows spec.nodeName via
	// the NodeLinker interface rather than ownerReferences. Pushes the hosting
	// Node onto the nav stack as a node-ward frame. A nil selection, a plugin
	// without NodeLinker (e.g. non-pods), or an empty spec.nodeName (GoToNode
	// returns a nil plugin) are all plain no-ops. In-pane drill — navFloor stays 0.
	case command == "nav-node":
		focused := a.layout.FocusedSplit()
		if focused == nil {
			return a, nil
		}
		sel := focused.Selected()
		if sel == nil {
			return a, nil
		}
		nl, ok := focused.Plugin().(plugin.NodeLinker)
		if !ok {
			return a, nil
		}
		nodePlugin, node := nl.GoToNode(a.clusterFor(focused), sel)
		if nodePlugin == nil {
			return a, nil
		}
		var objs []*unstructured.Unstructured
		if node != nil {
			objs = []*unstructured.Unstructured{node}
		}
		focused.PushNav(nodePlugin, objs, sel.GetName(), string(sel.GetUID()), sel.GetAPIVersion(), sel.GetKind(), ui.NavNode)
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
			a = a.syncStatusBarContext()
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
			a = a.syncStatusBarContext()
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
			a = a.syncStatusBarContext()
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
			a = a.syncStatusBarContext()
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

	// Move-pane: reorder the focused split along the orientation axis. The
	// perpendicular axis is a no-op (e.g. left/right do nothing in a vertical
	// stack), mirroring how focus-* branches on Orientation().
	//
	// movePane reorders only when the requested axis matches the current
	// orientation. No post-move resync (syncStatusBarContext/refreshDetailPanel/
	// syncLogPanel/SetHints) is needed: unlike focus-next-split, the move keeps
	// the SAME pane focused (focusIdx follows it), so context, detail and log
	// content — and the hints derived from the focused pane — are unchanged. Only
	// the visual pane order changes, which recalcSizes handles inside the layout.
	case command == "move-pane-up", command == "move-pane-down",
		command == "move-pane-left", command == "move-pane-right":
		movePane := func(axis layout.Orientation, delta int) {
			if a.layout.Orientation() == axis {
				a.layout.MoveFocusedSplit(delta)
			}
		}
		switch command {
		case "move-pane-up":
			movePane(layout.OrientationVertical, -1)
		case "move-pane-down":
			movePane(layout.OrientationVertical, +1)
		case "move-pane-left":
			movePane(layout.OrientationHorizontal, -1)
		case "move-pane-right":
			movePane(layout.OrientationHorizontal, +1)
		}
		a.keyTrie.Reset()
		a.syncTerminalSizes()
		return a, nil

	case command == "toggle-orientation":
		a.layout.ToggleOrientation()
		a.syncTerminalSizes()
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
		a = a.syncStatusBarContext()
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
		// Pop drill-down before closing panel. Guard on Depth>NavFloor (not
		// InDrillDown) so a split-opened drill cannot unwind past its home frame:
		// the nested InDrillDown re-checks below still distinguish "still drilled
		// after pop" from "back at root".
		if focused := a.layout.FocusedSplit(); focused != nil && focused.Depth() > focused.NavFloor() {
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

	case command == "clear-notifications":
		// Dismiss all live toasts. The work (marking IDs dismissed) lives in the
		// ClearNotificationsMsg handler so it stays the single source of truth.
		return a, func() tea.Msg { return msgs.ClearNotificationsMsg{} }

	case command == "close-panel":
		a = a.closeRightPanel()
		return a, nil

	case command == "close-current-panel":
		if a.layout.FocusedDetails() && a.layout.RightPanelVisible() {
			a = a.closeRightPanel()
			return a, nil
		}
		if a.layout.SplitCount() > 1 {
			var cmd tea.Cmd
			a, cmd = a.closeFocusedSplit()
			a.statusBar.SetHints(a.currentHints())
			return a, cmd
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
			var cmd tea.Cmd
			a, cmd = a.closeFocusedSplit()
			a.statusBar.SetHints(a.currentHints())
			return a, cmd
		}
		a = a.stopLogStream()
		// Sweep terminal sessions / node-debug pods before quitting (best-effort,
		// bounded) so quit does not leak remote state.
		a.shutdownTerminals()
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
		// Pinned cluster (e.g. manifest mode): the client is nil, so list the
		// fabricated Namespace objects from the store and feed them through the
		// same NamespacesLoadedMsg path so the UI is identical. This is restricted
		// to pinned clusters — a live cluster whose dial merely failed must not
		// fall back to the (empty) store; it falls through to the client branch.
		if cl := a.clusterForFocused(); cl != nil && a.mgr.IsPinned(cl.Context()) {
			names := storeNamespaceNames(cl.Store())
			return a, func() tea.Msg {
				return msgs.NamespacesLoadedMsg{Namespaces: names}
			}
		}
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
		a.contextPicker.SetContexts(a.contextNames())
		a.contextPicker.SetAnnotations(a.distinctPaneContexts(), a.contextFor(nil))
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
		execCl := a.clusterFor(focused)
		if execCl == nil || !execCl.Connected() {
			a.notify.Add(notify.LevelError, "exec: not available in manifest mode", a.contextFor(focused), "exec")
			return a, nil
		}
		execClient := execCl.Client()
		ns := selected.GetNamespace()
		if ns == "" {
			ns = focused.Namespace()
		}
		podName := resolvePodName(focused, selected)
		containerName := resolveContainerName(focused, selected)
		ctxName := focused.Context()
		title := "exec: " + podName
		if containerName != "" {
			title += "/" + containerName
		}
		return a.openExecTerminal(execClient, podName, containerName, ns, ctxName, title)

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
		if cl := a.clusterFor(focused); cl == nil || !cl.Connected() {
			a.notify.Add(notify.LevelError, "port-forward: not available in manifest mode", a.contextFor(focused), "port-forward")
			return a, nil
		}
		ports := a.extractPorts(focused, selected)
		if len(ports) == 0 {
			a.notify.Add(notify.LevelError, "no ports found on this resource", a.contextFor(focused), "port-forward")
			return a, nil
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
			a.notify.Add(notify.LevelError, "rollout-restart: not available in manifest mode", a.contextFor(focused), "rollout-restart")
			return a, nil
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
			a.notify.Add(notify.LevelError, "edit: not available in manifest mode", a.contextFor(focused), "edit")
			return a, nil
		}
		if focused.Plugin().Name() == "helmreleases" {
			hc := a.helmClientFor(editCl)
			if hc == nil {
				a.notify.Add(notify.LevelError, "helm: no client", a.contextFor(focused), "helm")
				return a, nil
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
			a.notify.Add(notify.LevelError, "set-image: not available in manifest mode", a.contextFor(focused), "set-image")
			return a, nil
		}
		pluginName := focused.Plugin().Name()
		containers := extractContainerImages(pluginName, selected)
		if len(containers) == 0 {
			a.notify.Add(notify.LevelError, "no containers found on this resource", a.contextFor(focused), "set-image")
			return a, nil
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
			a.notify.Add(notify.LevelError, "scale: not available in manifest mode", a.contextFor(focused), "scale")
			return a, nil
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
			a.notify.Add(notify.LevelError, "helm: no client", a.contextFor(focused), "helm")
			return a, nil
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
		path, lineCount, ok := a.saveLogBuffer("save-logs")
		if !ok {
			return a, nil
		}
		a.notify.Add(notify.LevelInfo, fmt.Sprintf("Saved %d lines → %s", lineCount, path), a.contextFor(nil), "save-logs")
		return a, nil

	case command == "save-and-open-logs":
		if !a.layout.IsLogMode() {
			return a, nil
		}
		path, _, ok := a.saveLogBuffer("save-and-open-logs")
		if !ok {
			return a, nil
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
		} else if tp, ok := a.layout.FocusedPane().(*ui.TerminalPane); ok {
			tp.ScrollDown(terminalScrollPageLines(tp))
		} else if focused := a.layout.FocusedSplit(); focused != nil {
			focused.PageDown()
			a, cmd := a.refreshDetailPanelOrLog()
			return a, cmd
		}
		return a, nil
	case command == "page-up":
		if a.layout.FocusedDetails() && a.layout.RightPanelVisible() {
			a.layout.ActiveDetailPanel().PageUp()
		} else if tp, ok := a.layout.FocusedPane().(*ui.TerminalPane); ok {
			tp.ScrollUp(terminalScrollPageLines(tp))
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
	case command == "copy-current":
		return a.handleCopyCurrent()
	case command == "copy-yaml":
		return a.handleCopyYAML()
	case command == "open-current":
		return a.handleOpenCurrent()
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

	a.notify.Add(notify.LevelError, fmt.Sprintf("unknown command: %s", command), a.contextFor(nil), "command")
	return a, nil
}

// handleCopyCurrent implements the "cc" command: copy the focused pane's
// current content to the clipboard. The source depends on the focused context:
//   - resources: the marked rows' names (falling back to the cursor row), joined
//     by newlines.
//   - yaml/describe (detail panel): the panel's raw buffer.
//   - logs: the buffered log lines, joined by newlines.
//
// Nothing-to-copy paths emit an error toast and return a nil cmd. Success emits
// an info toast and returns clipboard.Copy's tea.Cmd (OSC52 + native fallback).
func (a App) handleCopyCurrent() (tea.Model, tea.Cmd) {
	componentType, _ := a.currentContext()
	switch componentType {
	case "yaml", "describe", "details":
		panel := a.layout.RightPanel()
		if panel == nil {
			a.notify.Add(notify.LevelError, "Nothing to copy", a.contextFor(nil), "copy-current")
			return a, nil
		}
		text := panel.RawContent()
		if text == "" {
			a.notify.Add(notify.LevelError, "Nothing to copy", a.contextFor(nil), "copy-current")
			return a, nil
		}
		a.notify.Add(notify.LevelInfo, "Copied buffer", a.contextFor(nil), "copy-current")
		return a, clipboard.Copy(text)

	case "logs":
		_, _, _, _, lines, ok := a.currentLogContext()
		if !ok || len(lines) == 0 {
			a.notify.Add(notify.LevelError, "Nothing to copy", a.contextFor(nil), "copy-current")
			return a, nil
		}
		a.notify.Add(notify.LevelInfo, fmt.Sprintf("Copied %d log line(s)", len(lines)), a.contextFor(nil), "copy-current")
		return a, clipboard.Copy(strings.Join(lines, "\n"))

	default:
		focused, selected, _ := a.focusedSelection()
		if focused == nil {
			a.notify.Add(notify.LevelError, "Nothing to copy", a.contextFor(nil), "copy-current")
			return a, nil
		}
		objs := focused.SelectedObjects()
		if len(objs) == 0 {
			if selected == nil {
				a.notify.Add(notify.LevelError, "Nothing to copy", a.contextFor(focused), "copy-current")
				return a, nil
			}
			objs = []*unstructured.Unstructured{selected}
		}
		a.notify.Add(notify.LevelInfo, fmt.Sprintf("Copied %d name(s)", len(objs)), a.contextFor(focused), "copy-current")
		return a, clipboard.Copy(clipboard.JoinNames(objs))
	}
}

// selectedResourceYAML gathers the focused pane's resource selection (marked
// rows, falling back to the cursor row) and renders each object's YAML via the
// plugin, returning the raw docs. It is the shared core of copy-yaml and
// open-current's resources branch. On any no-op (no focused pane / nothing
// selected) or render error it emits an error toast — tagged with tag, worded
// with the given nothing/renderFail messages — and returns ok=false. Callers
// join the docs (clipboard.JoinYAML) for the clipboard or a temp file.
func (a App) selectedResourceYAML(tag, nothingMsg, renderFail string) (docs []string, ok bool) {
	focused, selected, _ := a.focusedSelection()
	if focused == nil {
		a.notify.Add(notify.LevelError, nothingMsg, a.contextFor(nil), tag)
		return nil, false
	}
	objs := focused.SelectedObjects()
	if len(objs) == 0 {
		if selected == nil {
			a.notify.Add(notify.LevelError, nothingMsg, a.contextFor(focused), tag)
			return nil, false
		}
		objs = []*unstructured.Unstructured{selected}
	}
	docs = make([]string, 0, len(objs))
	for _, obj := range objs {
		content, err := focused.Plugin().YAML(obj)
		if err != nil {
			a.notify.Add(notify.LevelError, fmt.Sprintf("%s: %s", renderFail, err), a.contextFor(focused), tag)
			return nil, false
		}
		docs = append(docs, content.Raw)
	}
	return docs, true
}

// handleCopyYAML implements the "cy" command: copy the selected resource(s)'
// rendered YAML to the clipboard. Unlike copy-current it is "resource only" —
// it always operates on the focused pane's resource selection regardless of
// which pane is focused. The marked rows are used when present, falling back to
// the cursor row. Each resource's YAML is rendered via the plugin and the docs
// are joined with "\n---\n".
//
// Nothing-to-copy paths emit an error toast and return a nil cmd. A render error
// emits an error toast and returns a nil cmd (no partial copy). Success emits an
// info toast and returns clipboard.Copy's tea.Cmd (OSC52 + native fallback).
func (a App) handleCopyYAML() (tea.Model, tea.Cmd) {
	docs, ok := a.selectedResourceYAML("copy-yaml", "Nothing to copy", "Copy YAML failed")
	if !ok {
		return a, nil
	}
	text := clipboard.JoinYAML(docs)
	a.notify.Add(notify.LevelInfo, fmt.Sprintf("Copied YAML (%d resource(s))", len(docs)), a.contextFor(a.layout.FocusedSplit()), "copy-yaml")
	return a, clipboard.Copy(text)
}

// openInEditorTemp writes content to a new OS temp file (in os.TempDir) with the
// given filename suffix (e.g. ".yaml" or ".txt"), closes it, and returns a
// tea.Cmd that opens the file in the resolved editor via logs.OpenCmd. The temp file is left
// on disk after the editor exits (matching logs.OpenCmd's "file kept" semantics).
func openInEditorTemp(content, suffix string) (tea.Cmd, error) {
	f, err := os.CreateTemp("", "aku-*"+suffix)
	if err != nil {
		return nil, err
	}
	if _, err := f.WriteString(content); err != nil {
		f.Close()
		os.Remove(f.Name())
		return nil, err
	}
	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		return nil, err
	}
	return logs.OpenCmd(f.Name()), nil
}

// handleOpenCurrent implements the "co" command: open the focused pane's current
// content in $EDITOR. The source depends on the focused context:
//   - resources: the selected resource(s)' rendered YAML (marked rows, falling
//     back to the cursor row) written to an OS temp .yaml file.
//   - yaml/describe (detail panel): the panel's raw buffer written to an OS temp
//     file (.yaml in yaml/details mode, .txt for describe).
//   - logs: the log buffer saved to its real destination via saveLogBuffer; the
//     file is opened AND its path is copied to the clipboard.
//
// Nothing-to-open paths emit an error/no-op toast and return a nil cmd. Temp
// write and YAML render errors emit an error toast and return a nil cmd.
func (a App) handleOpenCurrent() (tea.Model, tea.Cmd) {
	componentType, _ := a.currentContext()
	switch componentType {
	case "yaml", "describe", "details":
		panel := a.layout.RightPanel()
		if panel == nil {
			a.notify.Add(notify.LevelError, "Nothing to open", a.contextFor(nil), "open-current")
			return a, nil
		}
		text := panel.RawContent()
		if text == "" {
			a.notify.Add(notify.LevelError, "Nothing to open", a.contextFor(nil), "open-current")
			return a, nil
		}
		// .yaml for yaml-mode AND details-mode (Helm values are YAML); .txt only
		// for describe (plain text) — see currentContext() for the mode→type map.
		suffix := ".txt"
		if componentType == "yaml" || componentType == "details" {
			suffix = ".yaml"
		}
		cmd, err := openInEditorTemp(text, suffix)
		if err != nil {
			a.notify.Add(notify.LevelError, fmt.Sprintf("Open failed: %s", err), a.contextFor(nil), "open-current")
			return a, nil
		}
		a.notify.Add(notify.LevelInfo, "Opening buffer in editor", a.contextFor(nil), "open-current")
		return a, cmd

	case "logs":
		path, _, ok := a.saveLogBuffer("open-current")
		if !ok {
			return a, nil
		}
		a.notify.Add(notify.LevelInfo, "Saved & opened; path copied", a.contextFor(nil), "open-current")
		return a, tea.Batch(logs.OpenCmd(path), clipboard.Copy(path))

	default:
		docs, ok := a.selectedResourceYAML("open-current", "Nothing to open", "Open failed")
		if !ok {
			return a, nil
		}
		focused := a.layout.FocusedSplit()
		cmd, err := openInEditorTemp(clipboard.JoinYAML(docs), ".yaml")
		if err != nil {
			a.notify.Add(notify.LevelError, fmt.Sprintf("Open failed: %s", err), a.contextFor(focused), "open-current")
			return a, nil
		}
		a.notify.Add(notify.LevelInfo, fmt.Sprintf("Opening %d resource(s) in editor", len(docs)), a.contextFor(focused), "open-current")
		return a, cmd
	}
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

// saveLogBuffer writes the current log buffer to disk and emits the failure
// notifications (no log lines, BuildPath error, Write error) shared by the
// "save-logs", "save-and-open-logs", and "open-current" (logs branch) commands.
// Failure toasts are tagged with tag so they attribute to the calling command
// ("save-logs" or "open-current"). On success it returns the written path and
// the number of lines saved with ok=true and emits NO notification, leaving the
// success action (an info toast vs. opening the file) to the caller.
//
// Callers must reach this only in log mode. The save-logs/save-and-open-logs
// callers guard explicitly on IsLogMode(); handleOpenCurrent satisfies it
// implicitly — it reaches here only in currentContext()'s "logs" case.
func (a App) saveLogBuffer(tag string) (path string, lineCount int, ok bool) {
	cluster, ns, pod, container, lines, ctxOk := a.currentLogContext()
	if !ctxOk {
		a.notify.Add(notify.LevelError, "No log lines to save", a.contextFor(nil), tag)
		return "", 0, false
	}
	path, err := logs.BuildPath(cluster, ns, pod, container, time.Now())
	if err != nil {
		a.notify.Add(notify.LevelError, fmt.Sprintf("Save failed: %s", err), a.contextFor(nil), tag)
		return "", 0, false
	}
	if err := logs.Write(path, lines); err != nil {
		a.notify.Add(notify.LevelError, fmt.Sprintf("Save failed: %s", err), a.contextFor(nil), tag)
		return "", 0, false
	}
	return path, len(lines), true
}

// closeRightPanel performs full right-panel teardown: stops log stream,
// resets mode/state, hides panel, and refreshes indicators/hints.
func (a App) closeRightPanel() App {
	a = a.stopLogStream()
	a.layout.SetLogMode(false)
	a.envResolved = false
	a.lastDetailKey = ""
	a.keyTrie.Reset()
	// HideRightPanel sets focusTarget=Resources and self-reconciles, so an
	// explicit FocusResources here would be a redundant reconcile (with
	// rightVisible still true) immediately superseded.
	a.layout.HideRightPanel()
	a = a.syncIndicators()
	a.statusBar.SetHints(a.currentHints())
	return a
}

// closeFocusedSplit removes the currently-focused split and releases the
// resources it held. Callers must guarantee SplitCount() > 1 (the last split is
// never closed).
//
// Every pane references exactly one context. A cluster's refcount is the number
// of remaining panes referencing it; SyncRefs recomputes this from the live
// pane set and tears a cluster down when its count reaches zero. There is no
// global exemption — even the startup cluster is torn down once no pane uses it.
//
// Order matters: the closing split's (gvr, ns) is unsubscribed on the CLOSING
// split's OWN cluster store FIRST (resolved before removal — SyncRefs may tear
// the cluster down and drop the store), then the split is removed from the
// layout (so unsubscribeOnStoreIfUnused's "still in use?" scan no longer sees
// the closing pane), then SyncRefs reconciles refcounts against the remaining
// panes. If several panes share the closing context, SyncRefs keeps the cluster
// alive and tears it down only when the last referencing pane is gone.
func (a App) closeFocusedSplit() (App, tea.Cmd) {
	// A focused terminal pane closes via its own path: tear down the SPDY
	// session and remove the pane. There is no informer subscription or cluster
	// refcount tied to a terminal pane, so the resource-pane reconcile below
	// does not apply. Node-debug pod deletion is fired inside closeTerminalSession;
	// ephemeral pod-debug surfaces a one-line note (the container can't be removed).
	if tp, ok := a.layout.FocusedPane().(*ui.TerminalPane); ok {
		note := a.ephemeralCloseNote(tp.ID())
		a.closeTerminalSession(tp.ID())
		if note != "" {
			a.notify.Add(notify.LevelWarning, note, a.contextFor(nil), "debug")
		}
		a.keyTrie.Reset()
		a.layout.CloseCurrentSplit()
		// A terminal pane references a context just like a resource pane
		// (distinctPaneContexts/paneContexts count it), so its removal must
		// reconcile manager refcounts: closing the last pane on a context tears
		// that cluster down. There is no informer/store tied to a terminal pane,
		// so only the cluster-refcount reconcile applies (no unsubscribe). Done
		// AFTER CloseCurrentSplit so the just-closed pane is no longer counted.
		a.mgr.SyncRefs(a.distinctPaneContexts())
		a.syncTerminalSizes()
		// Re-derive context badges against the remaining panes: closing a pane can
		// drop the pane set back to a single context, in which case the badge must
		// clear. Only open/switch paths called this before, so close left it stale.
		a.syncPaneFooters()
		return a, nil
	}

	closing := a.layout.FocusedSplit()
	if closing == nil {
		return a, nil
	}

	closingCtx := closing.Context()
	closingGVR := closing.Plugin().GVR()
	closingNs := closing.EffectiveNamespace()

	// Resolve the closing split's cluster store BEFORE removal/reconcile:
	// SyncRefs can tear down any cluster no pane references and drop its store, so
	// we must grab it while the cluster is still live.
	closingStore := a.storeFor(closing)

	a.keyTrie.Reset()
	a.layout.CloseCurrentSplit()

	// Stop the closing split's informer for (gvr, ns) on its OWN cluster store
	// when no remaining pane on that cluster still needs it. Done after removal so
	// the "still in use?" scan does not count the just-closed pane.
	a.unsubscribeOnStoreIfUnused(closingStore, closingCtx, closingGVR, closingNs)

	// Reconcile manager refcounts against the remaining panes; a cluster no
	// remaining pane references is torn down here (no global exemption).
	a.mgr.SyncRefs(a.distinctPaneContexts())

	// Re-derive context badges against the remaining panes: closing a pane can
	// drop the pane set back to a single context, in which case the badge must
	// clear. Only open/switch paths called this before, so close left it stale.
	a.syncPaneFooters()

	return a, nil
}

// openExecTerminal builds an exec SPDY executor, starts a background session,
// opens a TerminalPane for it as a new split, and returns the command that
// kicks off the byte-pump loop plus an initial resize. The pane and session
// share a unique id (the registry key and TerminalPaneByID lookup key).
//
// This replaces the fullscreen tea.Exec exec path with an embedded pane.
func (a App) openExecTerminal(client *k8s.Client, podName, containerName, ns, ctxName, title string) (tea.Model, tea.Cmd) {
	exec, err := a.execExecutorFn(client, podName, containerName, ns, a.config.ExecCommand())
	if err != nil {
		a.notify.Add(notify.LevelError, "exec: "+err.Error(), ctxName, "exec")
		return a, nil
	}

	a.termSeq++
	id := fmt.Sprintf("exec:%s/%s/%s:%d", ns, podName, containerName, a.termSeq)

	// Seed the pane at the current screen size; recalcSizes (via AddTerminalSplit)
	// corrects it immediately, and syncTerminalSizes pushes the final size to the
	// session.
	w, h := a.layout.SplitSeedSize()
	tp := ui.NewTerminalPane(title, ctxName, w, h)
	tp.SetID(id)
	tp.SetScrollback(a.config.TerminalScrollback())

	sess := session.Start(exec, id)
	a.terminals[id] = sess
	startReplyPump(tp, sess)

	a.layout.AddTerminalSplit(tp)
	a.keyTrie.Reset()
	// Reconcile badge visibility immediately: a new pane may push the layout from
	// single- to multi-context (or vice versa), and this pane's own badge must be
	// shown/hidden per the same rule resource panes follow.
	a.syncPaneFooters()
	// syncTerminalSizes pushes the session's inner size: the session is already in
	// a.terminals and the pane is already in the layout (AddTerminalSplit ran
	// recalcSizes), so this covers the just-opened session — no extra direct Resize
	// is needed.
	a.syncTerminalSizes()

	return a, readTermBytes(sess)
}

// openDebugTerminal opens an embedded terminal pane for an ephemeral pod-debug
// container. The pane is shown IMMEDIATELY with a static "starting debug
// container…" placeholder (written into the emulator) and the SPDY-attach is
// deferred: the ephemeral-container pre-flight (GET pod → patch → wait Running)
// runs async in a tea.Cmd and reports back via DebugReadyMsg keyed by the pane's
// id, at which point the handler binds a live session to this pane.
//
// Starting-state choice: a static placeholder line rather than an animated
// spinner. It is robust (no extra tick wiring), and real shell bytes overwrite
// it once they flow.
func (a App) openDebugTerminal(client *k8s.Client, podName, containerName, ns, ctxName, image string, command []string, privileged bool) (tea.Model, tea.Cmd) {
	a.termSeq++
	id := fmt.Sprintf("debug:%s/%s:%d", ns, podName, a.termSeq)

	title := "debug: " + podName
	if containerName != "" {
		title += "/" + containerName
	}

	w, h := a.layout.SplitSeedSize()
	tp := ui.NewTerminalPane(title, ctxName, w, h)
	tp.SetID(id)
	tp.SetScrollback(a.config.TerminalScrollback())
	_, _ = tp.Write([]byte("starting debug container…\r\n"))

	// Record cleanup metadata up front: ephemeral containers cannot be removed,
	// so close/exit only surfaces a note (no delete). The cancel func ties the
	// pre-flight to the placeholder so closing it mid-flight aborts the API calls.
	preflightCtx, preflightCancel := context.WithCancel(context.Background())
	a.termCleanup[id] = terminalMeta{ephemeral: true, client: client, podName: podName, namespace: ns, preflightCancel: preflightCancel}

	a.layout.AddTerminalSplit(tp)
	a.keyTrie.Reset()
	a.syncPaneFooters()
	a.syncTerminalSizes()

	return a, debugPreflightCmd(preflightCtx, id, client, podName, containerName, ns, image, command, privileged)
}

// openNodeDebugTerminal opens an embedded terminal pane for a node-debug pod.
// Like openDebugTerminal it shows a placeholder immediately and defers the
// pod-create + wait-Running pre-flight to an async tea.Cmd reporting back via
// DebugReadyMsg. The created pod is recorded for deletion on close/quit.
func (a App) openNodeDebugTerminal(client *k8s.Client, nodeName, image string, command []string) (tea.Model, tea.Cmd) {
	a.termSeq++
	id := fmt.Sprintf("debug-node:%s:%d", nodeName, a.termSeq)

	title := "node: " + nodeName

	w, h := a.layout.SplitSeedSize()
	tp := ui.NewTerminalPane(title, client.Context, w, h)
	tp.SetID(id)
	tp.SetScrollback(a.config.TerminalScrollback())
	_, _ = tp.Write([]byte("starting debug pod on " + nodeName + "…\r\n"))

	// The pod name/namespace are not known until the pre-flight creates the pod;
	// DebugReadyMsg fills them in. Record the client now so a partially-created
	// session can still be cleaned up, and nodeDebug so close fires a delete. The
	// cancel func ties the pre-flight (create pod + wait-Running) to the
	// placeholder: closing it mid-flight cancels the API calls, and
	// PrepareNodeDebug's own ctx-cancel cleanup deletes a pod it already created.
	preflightCtx, preflightCancel := context.WithCancel(context.Background())
	a.termCleanup[id] = terminalMeta{nodeDebug: true, client: client, preflightCancel: preflightCancel}

	a.layout.AddTerminalSplit(tp)
	a.keyTrie.Reset()
	a.syncPaneFooters()
	a.syncTerminalSizes()

	return a, nodeDebugPreflightCmd(preflightCtx, id, client, nodeName, image, command)
}

// debugPreflightCmd runs the ephemeral pod-debug pre-flight off the UI goroutine
// and reports its result as a DebugReadyMsg keyed by the pane id.
func debugPreflightCmd(ctx context.Context, id string, client *k8s.Client, podName, containerName, ns, image string, command []string, privileged bool) tea.Cmd {
	return func() tea.Msg {
		dbgCtr, err := k8s.PrepareEphemeralDebug(ctx, client, podName, ns, containerName, image, command, privileged, nil)
		return msgs.DebugReadyMsg{
			ID:            id,
			PodName:       podName,
			Namespace:     ns,
			ContainerName: dbgCtr,
			Client:        client,
			Err:           err,
		}
	}
}

// nodeDebugPreflightCmd runs the node-debug pre-flight (create pod + wait) off
// the UI goroutine and reports a DebugReadyMsg with the created pod's identity.
func nodeDebugPreflightCmd(ctx context.Context, id string, client *k8s.Client, nodeName, image string, command []string) tea.Cmd {
	return func() tea.Msg {
		// node-debug is always privileged: the pod needs HostPID/HostNetwork/HostIPC
		// and a /host mount to be useful for node troubleshooting.
		podName, ns, containerName, err := k8s.PrepareNodeDebug(ctx, client, nodeName, image, command, true, nil)
		return msgs.DebugReadyMsg{
			ID:            id,
			PodName:       podName,
			Namespace:     ns,
			ContainerName: containerName,
			NodeMode:      true,
			Client:        client,
			Err:           err,
		}
	}
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
		a.notify.Add(notify.LevelError, fmt.Sprintf("unknown resource: %s", resourceName), a.contextFor(nil), "goto")
		return a, nil
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
		a.notify.Add(notify.LevelError, fmt.Sprintf("invalid GVR format: %s (expected group/version/resource or version/resource)", gvrStr), a.contextFor(nil), "goto")
		return a, nil
	}
	p, ok := plugin.ByGVR(gvr)
	if !ok {
		a.notify.Add(notify.LevelError, fmt.Sprintf("unknown GVR: %s", gvrStr), a.contextFor(nil), "goto")
		return a, nil
	}
	return a.handleGotoPlugin(p, "")
}

func (a App) handleSplit(resourceName string) (tea.Model, tea.Cmd) {
	p, ok := plugin.ByName(resourceName)
	if !ok {
		a.notify.Add(notify.LevelError, fmt.Sprintf("unknown resource: %s", resourceName), a.contextFor(nil), "split")
		return a, nil
	}

	if a.layout.AnyZoomed() {
		a.layout.UnzoomAll()
		a = a.syncIndicators()
	}

	// New panes inherit the currently-focused pane's context (the cluster the
	// user is looking at). Resolve the default namespace from that cluster.
	inheritCtx := a.contextFor(a.layout.FocusedSplit())
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
	// The new pane is born carrying inheritCtx, so the footer sync below sees
	// the correct context immediately (no wrong-context flicker).
	// AddSplit self-reconciles to resources (sets focusTarget=Resources and runs
	// reconcileFocus on the new pane), so no follow-up FocusResources is needed.
	a.layout.AddSplit(p, ns, inheritCtx)
	a.syncPaneFooters()
	newSplit := a.layout.FocusedSplit()
	// A new split changes the pane count for inheritCtx; refresh the contexts
	// plugin's PANES column before it (re-)populates.
	a.syncContextPaneCounts()
	populateCmd := a.subscribeAndPopulate(newSplit, p, newSplit.EffectiveNamespace())

	var descCmd tea.Cmd
	a, descCmd = a.refreshDetailPanel()
	var cmd tea.Cmd
	a, cmd = a.syncLogPanel()
	a.statusBar.SetHints(a.currentHints())
	return a, tea.Batch(populateCmd, descCmd, cmd)
}

// openDrilledSplit creates a NEW split rooted at the focused pane's CURRENT
// plugin, then drills it exactly one level via PushNav with a nav floor so
// Escape cannot unwind past the home drill. It is the shared tail of
// split-children (child-ward), split-parent (parent-ward), and split-nav-node
// (node-ward).
//
// The new split inherits the focused pane's context + namespace the same way
// handleSplit does, and AddSplit self-reconciles focus to it (no hand-rolled
// Blur/Focus). It subscribes + populates the ROOT plugin (so the home frame is
// backed and the root GVR's informer is armed), then PushNav installs the drilled
// subset — the drilled child/parent/node GVR was already subscribed by
// DrillDown/DrillUp/GoToNode. Footers/context counts are synced before,
// detail/log/hints after.
func (a App) openDrilledSplit(
	focused *ui.ResourceList,
	drillPlugin plugin.ResourcePlugin,
	objs []*unstructured.Unstructured,
	sel *unstructured.Unstructured,
	dir ui.NavDirection,
) (tea.Model, tea.Cmd) {
	if a.layout.AnyZoomed() {
		a.layout.UnzoomAll()
		a = a.syncIndicators()
	}

	// The new split is rooted at the CURRENT plugin, then drilled.
	currentPlugin := focused.Plugin()

	// Inherit the focused pane's context + namespace (mirrors handleSplit).
	inheritCtx := a.contextFor(focused)
	ns := "default"
	if cl := a.clusterFor(focused); cl != nil && cl.Client() != nil {
		ns = cl.Client().Namespace
	}
	if focused.Namespace() != "" {
		ns = focused.Namespace()
	}

	a.keyTrie.Reset()
	a.layout.AddSplit(currentPlugin, ns, inheritCtx)
	a.syncPaneFooters()
	newSplit := a.layout.FocusedSplit()
	a.syncContextPaneCounts()

	// Subscribe + populate the ROOT plugin BEFORE drilling, exactly like
	// handleSplit does: this arms the root GVR's informer and fills the new
	// split's live objects from the store. PushNav (below) then saves those root
	// objects into the home frame and installs the drilled subset as the live
	// view — so the root frame is backed even though the nav floor keeps the user
	// from unwinding to it. (subscribeAndPopulate's SetObjects is harmless here
	// because PushNav immediately replaces the live objects.)
	populateCmd := a.subscribeAndPopulate(newSplit, currentPlugin, newSplit.EffectiveNamespace())

	newSplit.PushNav(drillPlugin, objs, sel.GetName(), string(sel.GetUID()), sel.GetAPIVersion(), sel.GetKind(), dir)
	newSplit.SetNavFloor(1)

	a.envResolved = false
	var descCmd tea.Cmd
	a, descCmd = a.refreshDetailPanel()
	var cmd tea.Cmd
	a, cmd = a.syncLogPanel()
	a.statusBar.SetHints(a.currentHints())
	return a, tea.Batch(populateCmd, descCmd, cmd)
}

// contextsPlugin returns the registered synthetic contexts plugin, if present.
func (a App) contextsPlugin() (plugin.ResourcePlugin, bool) {
	return plugin.ByName("contexts")
}

// syncContextPaneCounts pushes the current per-context pane counts into the
// contexts plugin (if registered and capable) so its PANES column stays
// accurate. Called whenever pane contexts change or the contexts view is shown.
func (a App) syncContextPaneCounts() {
	p, ok := a.contextsPlugin()
	if !ok {
		return
	}
	if setter, ok := p.(plugin.PaneCountSetter); ok {
		setter.SetPaneCounts(a.distinctPaneContexts())
	}
}

// handleGotoContexts opens the synthetic contexts plugin IN the focused pane,
// pushing the current resource onto the pane's nav stack so Esc/back returns to
// the prior resource (reusing the drill-down nav machinery). The pane keeps its
// current context; selecting a row dispatches "pane-switch-context <name>".
func (a App) handleGotoContexts() (tea.Model, tea.Cmd) {
	p, ok := a.contextsPlugin()
	if !ok {
		a.notify.Add(notify.LevelError, "contexts view unavailable", a.contextFor(nil), "contexts")
		return a, nil
	}

	focused := a.layout.FocusedSplit()
	if focused == nil {
		return a, nil
	}

	if a.layout.AnyZoomed() {
		a.layout.UnzoomAll()
		a = a.syncIndicators()
	}

	// Refresh the PANES column before building rows.
	a.syncContextPaneCounts()

	var children []*unstructured.Unstructured
	if sp, ok := p.(plugin.SelfPopulating); ok {
		children = sp.Objects()
	}

	// Push the current resource onto the nav stack so back/Esc returns to it.
	// parentName carries the focused pane's context purely as a label.
	a.keyTrie.Reset()
	a.envResolved = false
	focused.PushNav(p, children, focused.Context(), "", "", "", ui.NavChild)
	if a.layout.RightPanelVisible() {
		a.layout.FocusResources()
	}

	var descCmd tea.Cmd
	a, descCmd = a.refreshDetailPanel()
	var cmd tea.Cmd
	a, cmd = a.syncLogPanel()
	a.statusBar.SetHints(a.currentHints())
	return a, tea.Batch(descCmd, cmd)
}

// handlePaneSwitchContext switches the focused pane to ctxName via the async
// per-pane path, then returns the pane to the resource it was showing before
// the contexts list was opened. If the contexts view was reached by drilling in
// (gX), the prior resource is restored by popping the nav stack; the
// subsequent context switch then re-points that restored resource at ctxName.
// A fresh oX split that has the contexts plugin as its root (no prior resource)
// lands on the default pods plugin instead.
func (a App) handlePaneSwitchContext(ctxName string) (tea.Model, tea.Cmd) {
	a.activeOverlay = overlayNone

	focused := a.layout.FocusedSplit()
	if focused == nil {
		return a, nil
	}

	// Return to the prior resource before switching context so the pane ends up
	// showing pods/the-prior-resource on the new cluster, NOT the contexts list.
	if focused.InDrillDown() {
		// gX path: the contexts view sits on top of the real resource. Pop back to
		// it. PopNav restores the snapshot's (old) context; the switch below
		// overwrites it with ctxName.
		focused.PopNav()
	} else if focused.Plugin().Name() == "contexts" {
		// Fresh oX split rooted on the contexts plugin: no prior resource to return
		// to — land on the default pods plugin.
		if pods, ok := plugin.ByName("pods"); ok {
			focused.SetPlugin(pods)
			focused.ResetNav()
		} else {
			// "pods" not registered: leave nav reset (so the pane is not stuck on the
			// contexts list) and surface the failure rather than silently keeping the
			// contexts view after the switch.
			focused.ResetNav()
			a.notify.Add(notify.LevelError, "cannot switch context: 'pods' plugin not registered", a.contextFor(focused), "context")
			return a, nil
		}
	}

	// Switch the (now-restored) pane to the chosen context via the async path.
	return a.handlePaneContextSwitch(ctxName)
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
		a.notify.Add(notify.LevelError, "helm: no client", a.contextFor(focused), "helm")
		return a, nil
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
		a.notify.Add(notify.LevelError, "usage: goto <resource>", a.contextFor(nil), "goto")
		return a, nil
	case "goto-gvr":
		if len(parts) >= 2 {
			return a.handleGotoGVR(parts[1])
		}
		a.notify.Add(notify.LevelError, "usage: goto-gvr <group/version/resource>", a.contextFor(nil), "goto")
		return a, nil
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
	// The pinned "manifests" pseudo-context is a static, client-less store with no
	// informers, so the normal reload path (UnsubscribeAll + re-subscribe) is a
	// no-op for it. When the focused context is the pinned manifest cluster, rebuild
	// it from the recorded source instead.
	if ctx := a.contextFor(a.layout.FocusedSplit()); a.mgr.IsPinned(ctx) && a.manifestSource != nil {
		return a.reloadManifest(ctx)
	}

	wasLogMode := a.layout.IsLogMode()
	wasRightVisible := a.layout.RightPanelVisible()

	// Tear down all informers and clear store cache across every live cluster.
	// Pinned clusters (the synthetic "manifests" pseudo-context) carry a static,
	// client-less store with no informers: UnsubscribeAll would clear their cache
	// and the subsequent re-subscribe would hand back an empty bucket, silently
	// wiping a concurrently-open manifest pane. Skip them.
	a.mgr.ForEach(func(c *cluster.Cluster) {
		if a.mgr.IsPinned(c.Context()) {
			return
		}
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

	// Reset each split to root and re-subscribe. SplitAt returns nil for terminal
	// panes, so they are intentionally skipped here: a terminal has no informer
	// subscription to rebuild and its live session must not be disturbed by a
	// reload. Their geometry is reconciled by syncTerminalSizes below.
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
	a.statusBar.SetHints(a.currentHints())
	a = a.syncIndicators()

	// The reload above can change pane geometry (unzoom, log-mode toggle). Push
	// the settled inner sizes to any live terminal sessions so the remote shells
	// reflow to match what their emulators now render.
	a.syncTerminalSizes()

	allCmds := append(populateCmds, descCmd, tea.ClearScreen)
	return a, tea.Batch(allCmds...)
}

// reloadManifest rebuilds the pinned "manifests" pseudo-context on Ctrl+r. The
// static manifest store carries no informers, so the normal reload path cannot
// refresh it; instead this re-runs manifest.LoadFiles from the recorded file
// paths and re-registers the rebuilt cluster under the SAME context name (so open
// panes on that context stay valid). Panes on the manifests context are reset to
// root and repopulated from the rebuilt store. A stdin source cannot be re-read,
// so that path silently rewrites the screen (no data refresh, no toast).
func (a App) reloadManifest(ctx string) (tea.Model, tea.Cmd) {
	src := a.manifestSource

	// stdin mode: the stream was consumed at startup and cannot be re-read, so
	// there is no new data to load. Reload just rewrites the screen silently
	// (matching the file path's redraw) rather than surfacing an error toast.
	if src.fromStdin || len(src.paths) == 0 {
		return a, tea.ClearScreen
	}

	// Rebuild the static cluster from the original file paths.
	rebuilt, warns, err := manifest.LoadFiles(src.paths, src.defaultNS)
	if err != nil {
		a.notify.Add(notify.LevelError, fmt.Sprintf("reload failed: %s", err), ctx, "reload")
		return a, nil
	}

	// Re-register the rebuilt cluster as pinned under the same context name. The
	// map entry is replaced; panes referencing "manifests" now resolve to the new
	// store via clusterFor.
	a.mgr.RegisterPinned(rebuilt)

	// Surface any new warnings (guessed plurals, unreadable files, malformed docs).
	for _, w := range warns {
		a.notify.Add(notify.LevelWarning, w.Reason, ctx, "manifest")
	}

	// Repopulate every pane on the manifests context from the rebuilt store. Reset
	// each to root first (drill-down state references objects from the old store),
	// then reuse subscribeAndPopulate — for a client-less store Subscribe returns
	// the cached objects, exactly mirroring a normal pane populate.
	objCount := 0
	if rebuilt.Store() != nil {
		objCount = rebuilt.Store().CountInNamespace("")
	}
	var populateCmds []tea.Cmd
	for i := range a.layout.SplitCount() {
		split := a.layout.SplitAt(i)
		if split == nil || split.Context() != ctx {
			continue
		}
		split.ResetForReload()
		if cmd := a.subscribeAndPopulate(split, split.Plugin(), split.EffectiveNamespace()); cmd != nil {
			populateCmds = append(populateCmds, cmd)
		}
	}

	a.notify.Add(notify.LevelInfo, fmt.Sprintf("reloaded %d objects from %s", objCount, ctx), ctx, "reload")

	a.statusBar.SetHints(a.currentHints())
	a = a.syncIndicators()
	return a, tea.Batch(append(populateCmds, tea.ClearScreen)...)
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
	// focused pane's cluster).
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

// handleGroupContextSwitch retargets the focused pane's whole context group
// (every pane currently on the focused pane's context) onto the chosen cluster.
// It is dispatched by the context-picker overlay (the `gx` binding); despite the
// historical "global" name in the message type (GlobalContextSelectedMsg), there
// is no global baseline — this retargets a group, not a process-wide default.
//
// Connect strategy: ASYNC. Like handlePaneContextSwitch, this NEVER dials inline
// on the Bubble Tea Update goroutine — a hung exec-credential plugin must not be
// able to freeze the UI. Instead it optimistically retargets the focused-context
// group (clear objects, show a "connecting…" status) and dispatches
// asyncConnectCmd(mgr, chosen) to dial off-thread. When the resulting
// msgs.ClusterReadyMsg lands, handleClusterReady installs the cluster and
// populates EVERY pane whose Context()==chosen — which, after this optimistic
// retarget, is exactly the moved group.
//
// gx and gX converge on the same async machinery; gx differs only in that it
// retargets the whole focused-context group rather than a single pane.
func (a App) handleGroupContextSwitch(ctx string) (tea.Model, tea.Cmd) {
	a.activeOverlay = overlayNone

	// The focused pane's current context is the group reference: it identifies
	// which panes move and which old store to clean up. Falls back to the startup
	// context when there is no focused pane. Capture it BEFORE mutating anything.
	target := a.contextFor(a.layout.FocusedSplit())

	// Pinned target (the static "manifests" pseudo-context): never dial. Retarget
	// the whole focused-context group onto the pinned store and populate each pane
	// directly, mirroring switchPaneToPinned/startup. Re-selecting a group already
	// on the pinned context is a clean no-op.
	if a.mgr.IsPinned(ctx) {
		if ctx == target {
			return a, nil
		}
		return a.switchGroupToPinned(target, ctx)
	}

	// No-op only when the whole group is already on this context AND its cluster
	// is connected. A group left on a broken context by a failed connect must be
	// able to retry: re-selecting the same context re-attempts the dial rather
	// than dead-ending here (mirrors handlePaneContextSwitch's retry guard).
	if ctx == target {
		if cl, ok := a.mgr.Get(ctx); ok && cl.Connected() {
			return a, nil
		}
	}

	// Remember the old context group's store so we can clean up its now-unused
	// informers after the optimistic retarget.
	oldStore := a.storeForContext(target)

	// Update the status bar to reflect the focused pane's new context (name plus
	// online/offline color). On an optimistic switch the cluster is usually not
	// connected yet, so the badge shows offline until ClusterReadyMsg arrives
	// (unless ctx already has a live cluster, e.g. another pane uses it).
	a = a.setStatusBarContext(ctx)

	// Stop any log stream — log streams are bound to their cluster's client.
	a = a.stopLogStream()
	a.layout.SetLogMode(false)
	a.envResolved = false

	// Optimistically retarget every pane in the focused pane's context group onto
	// the chosen cluster, clearing stale data from the old cluster. Panes on other
	// contexts are left untouched. The new cluster's data arrives later via
	// ClusterReadyMsg → handleClusterReady, which populates all panes matching the
	// chosen context.
	type prevSub struct {
		gvr schema.GroupVersionResource
		ns  string
	}
	var oldSubs []prevSub
	for i := range a.layout.SplitCount() {
		split := a.layout.SplitAt(i)
		if split == nil || split.Context() != target {
			continue
		}

		// Capture the pane's previous (gvr, ns) for old-store cleanup.
		oldSubs = append(oldSubs, prevSub{gvr: split.Plugin().GVR(), ns: split.EffectiveNamespace()})

		// Only clear the pane's nav/objects when its context is actually changing.
		// On a same-context retry (ctx==target after a failed connect) the user's
		// drill-down state is preserved; the retry just re-dials and re-populates.
		// On a genuine context change the optimistic clear stays in place: nav is
		// reset and stale data dropped, with fresh data arriving via
		// handleClusterReady once the dial returns.
		if ctx != split.Context() {
			split.SetContext(ctx)
			split.ResetNav()
			split.SetObjects(nil)
		}
	}

	// Reconcile manager refcounts against the panes' new contexts. The target
	// cluster may not be in the Manager yet (still dialing); SyncRefs counts what
	// it can now and is called again in handleClusterReady once the dial returns.
	// If the old context group is now empty, its cluster is torn down here.
	a.mgr.SyncRefs(a.distinctPaneContexts())

	// Clean up informers on the OLD store that no retargeted pane needs anymore.
	// If the old cluster still has panes it remains live; SyncRefs above tore it
	// down only when zero panes reference it.
	if oldStore != nil {
		for _, sub := range oldSubs {
			a.unsubscribeOnStoreIfUnused(oldStore, target, sub.gvr, sub.ns)
		}
	}

	a.statusBar.SetHints(a.currentHints())
	a = a.syncIndicators()

	// Refresh footer labels and offline markers. On a same-context reconnect retry
	// (ctx==target, cluster not connected) the panes keep their nav and objects
	// (M5 nav-preservation), but syncPaneFooters re-runs syncPaneOffline so any
	// pane on a degraded cluster shows the "⚠ offline" cue rather than presenting
	// stale data as live. It touches only the footer/offline marker, never nav.
	a.syncPaneFooters()

	// Dial off-thread; handleClusterReady completes the switch and populates the
	// retargeted group when msgs.ClusterReadyMsg arrives.
	a.notify.Add(notify.LevelInfo, fmt.Sprintf("connecting to %s…", ctx), ctx, "context")
	return a, asyncConnectCmd(a.mgr, ctx)
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
// an explicit store/context, which the group switch needs because the panes
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

// handlePaneContextSwitch re-points the focused pane to ctxName and runs it live
// on that cluster, simultaneously with other panes on other clusters (true
// side-by-side multi-cluster).
//
// Approach (optimistic, no pending-map): the focused pane is immediately
// re-pointed at ctxName, its stale data cleared, and a "connecting" status
// shown. The actual connect happens off-thread (asyncConnectCmd); when
// msgs.ClusterReadyMsg arrives, handleClusterReady completes the switch
// (subscribe/populate) or surfaces an error, leaving the pane on its context but
// empty on failure so the user can switch again. The awaiting pane(s) are
// identified on completion by Context()==msg.Context — see handleClusterReady.
//
// There is no "global context" concept: every pane simply carries a context, and
// the focused pane is the source of truth for new-split defaults and the status
// bar.
//
// Refcount bookkeeping is reconciliation-based (idempotent, order-independent):
// after every switch/close we call Manager.SyncRefs(distinctPaneContexts()), which makes
// each cluster's refCount equal the number of panes currently on it and tears
// down any cluster with zero referencing panes. This is robust under rapid
// re-switches and focus changes: a stale ClusterReadyMsg for a context no pane
// is on anymore reconciles to zero and is cleaned up. The actual connect
// (k8s.NewClient) happens off-thread in asyncConnectCmd via Manager.Dial;
// install + reconcile happen on the Update goroutine in handleClusterReady.
func (a App) handlePaneContextSwitch(ctxName string) (tea.Model, tea.Cmd) {
	a.activeOverlay = overlayNone

	focused := a.layout.FocusedSplit()
	if focused == nil {
		return a, nil
	}

	old := focused.Context()

	// Pinned target (the static "manifests" pseudo-context): never dial. Its
	// cluster is already registered with a client-less store and reports
	// Connected()==false, so the normal "connected?" no-op guard and the async
	// Dial path below are both wrong for it (Dial would call k8s.NewClient for a
	// non-existent kubeconfig context and fail, leaving the pane offline/empty).
	// Instead retarget the pane and populate directly from the static store,
	// mirroring startup. Re-selecting an already-pinned pane is a clean no-op.
	if a.mgr.IsPinned(ctxName) {
		if old == ctxName {
			return a, nil
		}
		return a.switchPaneToPinned(focused, ctxName)
	}

	// No-op only when already on this exact context AND its cluster is
	// connected. A pane left on a broken context by a failed connect must be able
	// to retry: re-selecting the same context re-attempts the dial rather than
	// dead-ending here.
	if old == ctxName {
		if cl, ok := a.mgr.Get(ctxName); ok && cl.Connected() {
			return a, nil
		}
	}

	// Stop any log stream on the focused pane: log streams are bound to the old
	// cluster's client.
	a = a.stopLogStream()
	a.layout.SetLogMode(false)
	a.envResolved = false

	// Optimistically re-point the pane at the new context, clearing stale data
	// from the old cluster. The new cluster's data arrives via ClusterReadyMsg.
	focused.SetContext(ctxName)
	focused.ResetNav()
	focused.SetObjects(nil)

	// Reconcile refcounts against the panes' new contexts. The target cluster may
	// not be in the Manager yet (still dialing); SyncRefs counts what it can now
	// and is called again in handleClusterReady once the dial returns. The cluster
	// the pane just left (its old context) that no other pane references is torn
	// down here.
	a.mgr.SyncRefs(a.distinctPaneContexts())

	// The focused pane now carries ctxName, so reflect it in the status bar
	// (offline until the dial resolves in handleClusterReady).
	a = a.setStatusBarContext(ctxName)

	a.notify.Add(notify.LevelInfo, fmt.Sprintf("connecting to %s…", ctxName), ctxName, "context")
	return a, asyncConnectCmd(a.mgr, ctxName)
}

// switchPaneToPinned retargets a single pane onto a pinned (client-less) context
// such as the static "manifests" cluster, populating it directly from that
// cluster's store instead of dialing. The pinned cluster dual-keys every object
// into the all-namespaces ("") bucket, so the pane opens on All Namespaces —
// matching startup (app.go) and reloadManifest. No async connect is dispatched;
// the data is already present in the store.
func (a App) switchPaneToPinned(focused *ui.ResourceList, ctxName string) (tea.Model, tea.Cmd) {
	// Stop any log stream / log mode bound to the old cluster's client.
	a = a.stopLogStream()
	a.layout.SetLogMode(false)
	a.envResolved = false

	focused.SetContext(ctxName)
	focused.ResetNav()
	focused.SetNamespace("")
	focused.SetOffline(false)

	// Reconcile refcounts against the panes' new contexts; tears down the cluster
	// the pane just left if no other pane references it.
	a.mgr.SyncRefs(a.distinctPaneContexts())

	// Populate from the static store (returns nil cmd for a client-less store).
	cmd := a.subscribeAndPopulate(focused, focused.Plugin(), focused.EffectiveNamespace())

	a = a.setStatusBarContext(ctxName)
	a.syncPaneFooters()
	a.statusBar.SetHints(a.currentHints())
	a = a.syncIndicators()
	return a, cmd
}

// switchGroupToPinned retargets every pane in the focused pane's context group
// (those on context==target) onto a pinned (client-less) context such as the
// static "manifests" cluster, populating each directly from the pinned store
// instead of dialing. Mirrors switchPaneToPinned but for the whole group.
func (a App) switchGroupToPinned(target, ctxName string) (tea.Model, tea.Cmd) {
	oldStore := a.storeForContext(target)

	a = a.stopLogStream()
	a.layout.SetLogMode(false)
	a.envResolved = false

	type prevSub struct {
		gvr schema.GroupVersionResource
		ns  string
	}
	var oldSubs []prevSub
	var cmds []tea.Cmd
	for i := range a.layout.SplitCount() {
		split := a.layout.SplitAt(i)
		if split == nil || split.Context() != target {
			continue
		}
		oldSubs = append(oldSubs, prevSub{gvr: split.Plugin().GVR(), ns: split.EffectiveNamespace()})
		split.SetContext(ctxName)
		split.ResetNav()
		split.SetNamespace("")
		split.SetOffline(false)
		if cmd := a.subscribeAndPopulate(split, split.Plugin(), split.EffectiveNamespace()); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	a.mgr.SyncRefs(a.distinctPaneContexts())

	// Clean up informers on the OLD store that no retargeted pane needs anymore.
	if oldStore != nil {
		for _, sub := range oldSubs {
			a.unsubscribeOnStoreIfUnused(oldStore, target, sub.gvr, sub.ns)
		}
	}

	a = a.setStatusBarContext(ctxName)
	a.syncPaneFooters()
	a.statusBar.SetHints(a.currentHints())
	a = a.syncIndicators()
	return a, tea.Batch(cmds...)
}

// handleClusterReady completes a per-pane connect once the async connect cmd
// reports back. It applies the connected cluster to every pane whose
// Context() == msg.Context (across all splits, not just the focused one); on
// failure it surfaces an error and leaves those panes on their context but empty,
// marking them offline (an "⚠ offline" marker).
func (a App) handleClusterReady(msg msgs.ClusterReadyMsg) (tea.Model, tea.Cmd) {
	client, _ := msg.Client.(*k8s.Client)
	if msg.Err != nil || client == nil {
		// Failed dial: nothing was registered and no ref was taken. Reconcile so a
		// context no pane references anymore is torn down, then report the error
		// and leave the awaiting pane(s) on their context but empty so the user can
		// retry. Mark every pane on the failed context offline so it carries an
		// "⚠ offline" marker; OTHER panes/clusters are untouched. Because the failed
		// dial registers no cluster, syncPaneFooters leaves this marker in place
		// (only a connected cluster clears it), and it recovers automatically when
		// a later successful connect/heartbeat marks the cluster connected.
		a.mgr.SyncRefs(a.distinctPaneContexts())
		for i := range a.layout.SplitCount() {
			split := a.layout.SplitAt(i)
			if split != nil && split.Context() == msg.Context {
				split.SetOffline(true)
			}
		}
		a.syncPaneFooters()
		errMsg := fmt.Sprintf("context %s: failed to connect", msg.Context)
		if msg.Err != nil {
			errMsg = fmt.Sprintf("context %s: %s", msg.Context, msg.Err)
		}
		a.notify.Add(notify.LevelError, errMsg, msg.Context, "context")
		return a, nil
	}

	// Install the dialed client on the Update goroutine. RegisterConnected returns
	// the already-cached cluster if another pane connected this context first
	// (discarding the redundant client); newlyConnected gates arming a fresh
	// heartbeat/discovery so two panes on the same context do not start duplicate
	// heartbeat loops.
	cl, newlyConnected := a.mgr.RegisterConnected(msg.Context, client)

	// Reconcile refcounts: SyncRefs makes each cluster's refcount equal the
	// number of panes whose context references it, and tears down any cluster
	// that no remaining pane references (no global exemption). If no pane
	// references msg.Context anymore (the requester retargeted elsewhere), this
	// tears the just-registered cluster back down — no leak.
	a.mgr.SyncRefs(a.distinctPaneContexts())

	if cl == nil || !cl.Connected() {
		// The just-registered cluster is gone or not connected (e.g. SyncRefs tore
		// it down because no pane references it, or RegisterConnected rejected the
		// client). Mirror the failed-dial path: mark every pane on this context
		// offline and refresh footers so the "⚠ offline" marker is shown.
		for i := range a.layout.SplitCount() {
			split := a.layout.SplitAt(i)
			if split != nil && split.Context() == msg.Context {
				split.SetOffline(true)
			}
		}
		a.syncPaneFooters()
		a.notify.Add(notify.LevelError, fmt.Sprintf("context %s: failed to connect", msg.Context), msg.Context, "context")
		return a, nil
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

	// Apply to every matching split awaiting this context (identified by
	// Context()==msg.Context across ALL splits, NOT by focus — if the user moved
	// focus between dispatch and arrival, the correct non-focused matching pane is
	// still populated). Re-applying to an already-populated pane is harmless.
	defaultNs := ""
	if cl.Client() != nil {
		defaultNs = cl.Client().Namespace
	}
	for i := range a.layout.SplitCount() {
		split := a.layout.SplitAt(i)
		if split == nil || split.Context() != msg.Context {
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
				a.notify.Add(notify.LevelWarning, fmt.Sprintf("%s not available on %s", gvr.Resource, msg.Context), msg.Context, "discovery")
				continue
			}
		}
		if pc := a.subscribeAndPopulate(split, split.Plugin(), split.EffectiveNamespace()); pc != nil {
			cmds = append(cmds, pc)
		}
	}

	// A pane's context is now live: refresh per-pane footers (a footer is shown
	// when more than one distinct context exists across panes), and refresh the
	// contexts plugin's pane counts so a visible contexts view's STATUS glyph
	// reflects the new state.
	a.syncPaneFooters()
	a.syncContextPaneCounts()

	// The dial resolved, so refresh the status-bar badge: if the focused pane is
	// on the now-connected context its online color flips to green.
	a = a.syncStatusBarContext()

	// The dial is live: surface a success note (it also supersedes the transient
	// "connecting…" toast for this context).
	a.notify.Add(notify.LevelInfo, fmt.Sprintf("connected to %s", msg.Context), msg.Context, "context")

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
		a.notify.Add(notify.LevelError, "delete: not available in manifest mode", a.contextFor(focused), "delete")
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
	debugCl := a.clusterFor(focused)
	if debugCl == nil || !debugCl.Connected() {
		a.notify.Add(notify.LevelError, "debug: not available in manifest mode", a.contextFor(focused), "debug")
		return a, nil
	}
	debugClient := debugCl.Client()

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
	ctxName := focused.Context()

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

	return a.openDebugTerminal(debugClient, podName, containerName, ns, ctxName, image, command, false)
}

// executePendingDebug runs a debug action that was confirmed by the user. Both
// privileged pod-debug and node-debug open embedded terminal panes (the same
// async pre-flight + DebugReadyMsg flow as the direct, non-privileged path).
func (a App) executePendingDebug(dbg *pendingDebugAction) (tea.Model, tea.Cmd) {
	focused := a.layout.FocusedSplit()
	cl := a.clusterFor(focused)
	if cl == nil || !cl.Connected() {
		// Mirror handleDebug's "debug: not available in manifest mode" error: tag it with the
		// focused split's context. contextFor(focused) and contextFor(nil) both
		// resolve to the focused split here, but read it explicitly so the two
		// duplicate sites match.
		a.notify.Add(notify.LevelError, "debug: not available in manifest mode", a.contextFor(focused), "debug")
		return a, nil
	}
	client := cl.Client()
	if dbg.nodeMode {
		return a.openNodeDebugTerminal(client, dbg.nodeName, dbg.image, dbg.command)
	}
	return a.openDebugTerminal(client, dbg.podName, dbg.containerName, dbg.namespace, client.Context, dbg.image, dbg.command, dbg.privileged)
}

func (a App) handleSearchSubmitted(msg msgs.SearchSubmittedMsg) (tea.Model, tea.Cmd) {
	target := a.searchTarget()
	if target == nil {
		a.notify.Add(notify.LevelError, "no active panel to search", a.contextFor(nil), "search")
		return a, nil
	}
	return a.applySearchToTarget(target, msg.Pattern, msg.Mode)
}

// handleSearchApply applies a coalesced search request. The seq guard discards
// stale applies from earlier keystrokes; only the latest in-flight seq is honored.
func (a App) handleSearchApply(msg msgs.SearchApplyMsg) (tea.Model, tea.Cmd) {
	if msg.Seq != a.searchApplySeq {
		return a, nil
	}
	target := a.searchTarget()
	if target == nil {
		return a, nil
	}
	return a.applySearchToTarget(target, msg.Pattern, msg.Mode)
}

// applySearchToTarget applies a search/filter to target and handles the shared
// error-reporting and detail-panel refresh. On compile error it records a notify
// message; on success it, when the resource list is focused, refreshes the
// detail panel.
func (a App) applySearchToTarget(target ui.Searchable, pattern string, mode msgs.SearchMode) (tea.Model, tea.Cmd) {
	if err := target.ApplySearch(pattern, mode); err != nil {
		a.notify.Add(notify.LevelError, "invalid regex: "+err.Error(), a.contextFor(nil), "search")
	} else {
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
	// Empty pattern: clear immediately.
	// Bump seq to invalidate any in-flight apply from a previous non-empty pattern.
	if msg.Pattern == "" {
		a.searchApplySeq++
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
	// Non-empty pattern: apply immediately, no timer. A burst of keystrokes is
	// coalesced because each keystroke bumps searchApplySeq; the apply handler's
	// seq guard then discards any earlier in-flight apply whose Seq no longer
	// matches the counter, so only the latest keystroke's apply takes effect.
	a.searchApplySeq++
	seq := a.searchApplySeq
	pattern := msg.Pattern
	mode := msg.Mode
	return a, func() tea.Msg {
		return msgs.SearchApplyMsg{Seq: seq, Pattern: pattern, Mode: mode}
	}
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
		pullPolicy, _ := specMap["imagePullPolicy"].(string)
		return []msgs.ContainerImageChange{
			{Name: obj.GetName(), Image: image, Init: typ == "init", PullPolicy: pullPolicy},
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
		pullPolicy, _ := specMap["imagePullPolicy"].(string)
		if name != "" {
			result = append(result, msgs.ContainerImageChange{Name: name, Image: image, Init: init, PullPolicy: pullPolicy})
		}
	}
	return result
}

// namespacesGVR is the core/v1 namespaces GVR. Namespaces are cluster-scoped, so
// the store keys them under namespace "".
var namespacesGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}

// storeNamespaceNames returns the sorted names of every Namespace object held in
// the store. Used by the namespace picker for offline (e.g. manifest-mode)
// clusters whose nil client cannot serve ListNamespaces.
func storeNamespaceNames(store *k8s.Store) []string {
	if store == nil {
		return nil
	}
	objs := store.List(namespacesGVR, "")
	names := make([]string, 0, len(objs))
	for _, obj := range objs {
		if name := obj.GetName(); name != "" {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names
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
		a.notify.Add(notify.LevelError, "run: no resource selected", a.contextFor(nil), "run")
		return a, nil
	}
	expanded, err := a.substituteVars(run.Command)
	if err != nil {
		a.notify.Add(notify.LevelError, err.Error(), a.contextFor(focused), "run")
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
