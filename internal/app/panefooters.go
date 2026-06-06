package app

import "github.com/aohoyd/aku/internal/ui"

// distinctPaneContexts returns the multiset of resolved pane contexts as a
// map of context name → number of panes currently on that context. It is the
// single source for refcount reconciliation (Manager.SyncRefs), the pane-footer
// display rule, the context-picker overlay's in-use markers, and the contexts
// plugin PANES column.
//
// Each pane's context is resolved via contextFor. Panes are seeded with a
// concrete context at creation, so in normal operation every pane contributes a
// refcount under its own context; an empty context (only possible in a degraded
// startup where there is no startup cluster to tear down) is filtered out.
func (a App) distinctPaneContexts() map[string]int {
	counts := make(map[string]int)
	for i := 0; i < a.layout.SplitCount(); i++ {
		// Iterate via PaneAtIdx (not SplitAt) so terminal panes are included:
		// SplitAt returns nil for a *ui.TerminalPane, which would drop a context
		// held only by a live terminal session from the refcount and let
		// Manager.SyncRefs tear that cluster down underneath it.
		if ctx := a.paneContext(a.layout.PaneAtIdx(i)); ctx != "" {
			counts[ctx]++
		}
	}
	return counts
}

// paneContext resolves the kube-context of any pane kind. Resource panes resolve
// through contextFor (which falls back to the startup context for an empty
// context); terminal panes expose their context directly via Context().
func (a App) paneContext(p ui.Pane) string {
	switch pane := p.(type) {
	case *ui.ResourceList:
		return a.contextFor(pane)
	case *ui.TerminalPane:
		return pane.Context()
	default:
		return ""
	}
}

// syncPaneFooters updates every split's top-border context badge. When more than
// one distinct context is present across panes, each pane shows its own context
// name; when all panes share a single context (or have none), no badge is shown.
//
// The badge reuses the top border line (see ui.injectBorderTitle), so it does not
// consume a content row and the table height is unchanged.
//
// App is a value type, but a.layout.PaneAtIdx(i) returns a pane pointer that
// points into the layout's backing slice, so the SetContextLabel /
// SetContextBadgeVisible mutations are visible to subsequent renders without
// reassigning a.layout.
//
// Iterating via PaneAtIdx (not SplitAt) so terminal panes participate too:
// SplitAt returns nil for a *ui.TerminalPane, which would have left a terminal
// pane's badge always visible (it renders whenever ctx != "") even in
// single-context mode. Terminal panes mirror the resource-pane rule via
// SetContextBadgeVisible.
func (a App) syncPaneFooters() {
	multi := len(a.distinctPaneContexts()) > 1
	for i := 0; i < a.layout.SplitCount(); i++ {
		switch pane := a.layout.PaneAtIdx(i).(type) {
		case *ui.ResourceList:
			if multi {
				pane.SetContextLabel(a.contextFor(pane))
			} else {
				pane.SetContextLabel("")
			}
			a.syncPaneOffline(pane)
		case *ui.TerminalPane:
			// Show the badge only when panes span more than one context, the
			// same rule resource panes follow.
			pane.SetContextBadgeVisible(multi)
		}
	}
}

// syncPaneOffline reconciles a pane's offline state (which colors its context
// badge red) against its context's cluster connectivity, using the SAME
// per-context state the contexts plugin STATUS reads. A pane whose cluster is
// present-but-degraded (errored / failing heartbeat) is marked offline; a pane
// whose cluster is connected is cleared.
//
// When the context has no cluster yet (never dialed, or a failed dial left no
// entry) the state is left UNCHANGED: this preserves an explicit offline flag
// set by handleClusterReady's failure path (which registers no cluster) while
// not flagging a pane that is merely mid-connect. Recovery is automatic: once
// the cluster reconnects (RegisterConnected installs a live client), the next
// sync sees Connected()==true and clears the flag.
func (a App) syncPaneOffline(split *ui.ResourceList) {
	// Resolve via contextFor so an empty-context pane is checked under its
	// resolved (startup) context — reading split.Context() raw would clear the
	// offline marker for empty-context panes and mask an outage of the startup
	// cluster.
	ctx := a.contextFor(split)
	if ctx == "" {
		split.SetOffline(false)
		return
	}
	cl, ok := a.mgr.Get(ctx)
	if !ok || cl == nil {
		// No cluster entry: leave the marker as-is (see doc comment).
		return
	}
	split.SetOffline(!cl.Connected())
}
