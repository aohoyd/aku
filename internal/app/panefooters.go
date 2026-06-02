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
		split := a.layout.SplitAt(i)
		if split == nil {
			continue
		}
		if ctx := a.contextFor(split); ctx != "" {
			counts[ctx]++
		}
	}
	return counts
}

// syncPaneFooters updates every split's top-border context badge. When more than
// one distinct context is present across panes, each pane shows its own context
// name; when all panes share a single context (or have none), no badge is shown.
//
// The badge reuses the top border line (see ui.injectBorderTitle), so it does not
// consume a content row and the table height is unchanged.
//
// App is a value type, but a.layout.SplitAt(i) returns a *ui.ResourceList that
// points into the layout's backing slice, so the SetContextLabel mutations are
// visible to subsequent renders without reassigning a.layout.
func (a App) syncPaneFooters() {
	multi := len(a.distinctPaneContexts()) > 1
	for i := 0; i < a.layout.SplitCount(); i++ {
		split := a.layout.SplitAt(i)
		if split == nil {
			continue
		}
		if multi {
			split.SetContextLabel(a.contextFor(split))
		} else {
			split.SetContextLabel("")
		}
		a.syncPaneOffline(split)
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
