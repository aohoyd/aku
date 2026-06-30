package app

import "testing"

// TestHandleSplitInheritsContextImmediately is the regression guard for the
// wrong-context badge flicker: when more than one context is in use, a newly
// split pane must carry the inherited (focused) context — and show it on its
// badge — immediately after handleSplit returns, never the startup context.
func TestHandleSplitInheritsContextImmediately(t *testing.T) {
	a := appWithManager(t, "ctx-a", "ctx-b") // split[0] on ctx-a (startup)
	connectWithNamespaces(a, map[string]string{"ctx-b": "team-b"})

	// Put a second pane on ctx-b and focus it, so two distinct contexts are in
	// play (badges are shown) and the next split inherits ctx-b.
	a = addSplit(t, a, "deployments")
	a.layout.SplitAt(1).SetContext("ctx-b")
	a.layout.FocusSplitAt(1)

	model, _ := a.handleSplit("deployments")
	a = model.(App)

	newPane := a.layout.FocusedSplit()
	if got := newPane.Context(); got != "ctx-b" {
		t.Fatalf("new pane context = %q, want inherited 'ctx-b' (not startup 'ctx-a')", got)
	}
	if got := newPane.ContextLabel(); got != "ctx-b" {
		t.Fatalf("new pane badge label = %q, want 'ctx-b' immediately (flicker regression)", got)
	}
}

// TestDistinctPaneContexts_CountsByContext verifies the helper returns the
// per-context pane counts. Panes carry a concrete context (seeded at creation),
// so each counts under its own context; a pane with an empty context (a
// degraded-startup edge that does not arise in normal operation) is filtered
// out rather than folded into another context.
func TestDistinctPaneContexts_CountsByContext(t *testing.T) {
	a := appWithManager(t, "ctx-a", "ctx-b")
	connectWithNamespaces(a, map[string]string{"ctx-b": "team-b"})

	// split[0] is on ctx-a (startup). Add two more on ctx-b and one with no ctx.
	a = addSplit(t, a, "deployments")
	a.layout.SplitAt(1).SetContext("ctx-b")
	a = addSplit(t, a, "deployments")
	a.layout.SplitAt(2).SetContext("ctx-b")
	a = addSplit(t, a, "deployments")
	a.layout.SplitAt(3).SetContext("")

	got := a.distinctPaneContexts()
	// split[0] is on ctx-a; split[3] has an empty context and is filtered out.
	if got["ctx-a"] != 1 {
		t.Errorf("ctx-a count = %d, want 1", got["ctx-a"])
	}
	if got["ctx-b"] != 2 {
		t.Errorf("ctx-b count = %d, want 2", got["ctx-b"])
	}
	if _, ok := got[""]; ok {
		t.Errorf("empty context must not be counted as a distinct key, got %v", got)
	}
	if len(got) != 2 {
		t.Errorf("distinct context count = %d, want 2", len(got))
	}
}

// TestSyncRefsTearsDownContextWithZeroPanes verifies the app-level lifecycle:
// after a pane stops referencing a context (and no other pane references it),
// SyncRefs(distinctPaneContexts()) tears that cluster down, while a still-referenced
// context survives.
func TestSyncRefsTearsDownContextWithZeroPanes(t *testing.T) {
	a := appWithManager(t, "ctx-a", "ctx-b")
	connectWithNamespaces(a, map[string]string{"ctx-b": "team-b"})

	// Add a second pane and put it on ctx-b; connect ctx-b so a cluster exists.
	a = addSplit(t, a, "deployments")
	a.layout.SplitAt(1).SetContext("ctx-b")
	if _, err := a.mgr.GetOrCreate("ctx-b"); err != nil {
		t.Fatalf("GetOrCreate(ctx-b) err = %v", err)
	}

	a.mgr.SyncRefs(a.distinctPaneContexts())
	if _, ok := a.mgr.Get("ctx-b"); !ok {
		t.Fatalf("ctx-b torn down while a pane still references it")
	}

	// Move the second pane back to ctx-a: ctx-b now has zero panes and is removed.
	a.layout.SplitAt(1).SetContext("ctx-a")
	a.mgr.SyncRefs(a.distinctPaneContexts())
	if _, ok := a.mgr.Get("ctx-b"); ok {
		t.Errorf("ctx-b still present after zero panes reference it")
	}
	if _, ok := a.mgr.Get("ctx-a"); !ok {
		t.Errorf("ctx-a torn down despite being referenced by both panes")
	}
}
