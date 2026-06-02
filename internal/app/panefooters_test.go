package app

import (
	"errors"
	"testing"

	"github.com/aohoyd/aku/internal/cluster"
	"github.com/aohoyd/aku/internal/k8s"
)

// TestSyncPaneFooters_SinglePaneNoFooter verifies that a single pane never shows
// a context footer label, regardless of which context it carries.
func TestSyncPaneFooters_SinglePaneNoFooter(t *testing.T) {
	a := appWithManager(t, "ctx-a", "ctx-b")

	// split[0] is the lone startup pane on ctx-a.
	if a.layout.SplitCount() != 1 {
		t.Fatalf("precondition: expected exactly one split, got %d", a.layout.SplitCount())
	}

	a.syncPaneFooters()

	if got := a.layout.SplitAt(0).ContextLabel(); got != "" {
		t.Errorf("single pane footer = %q, want empty", got)
	}
}

// TestSyncPaneFooters_TwoPanesSameContextNoLabels verifies that when every pane
// shares the same context, no footer labels are shown.
func TestSyncPaneFooters_TwoPanesSameContextNoLabels(t *testing.T) {
	a := appWithManager(t, "ctx-a", "ctx-b")

	// Add a second pane on the SAME context as split[0] (ctx-a).
	a = addSplit(t, a, "deployments")
	a.layout.SplitAt(1).SetContext("ctx-a")

	a.syncPaneFooters()

	for i := 0; i < a.layout.SplitCount(); i++ {
		if got := a.layout.SplitAt(i).ContextLabel(); got != "" {
			t.Errorf("pane %d footer = %q, want empty (all panes share one context)", i, got)
		}
	}
}

// TestSyncPaneFooters_TwoPanesDistinctContextsShowLabels verifies that when more
// than one distinct context exists across panes, each pane shows its own context
// name as the footer label.
func TestSyncPaneFooters_TwoPanesDistinctContextsShowLabels(t *testing.T) {
	a := appWithManager(t, "ctx-a", "ctx-b")

	// split[0] is on ctx-a; add a second pane on ctx-b.
	a = addSplit(t, a, "deployments")
	a.layout.SplitAt(1).SetContext("ctx-b")

	a.syncPaneFooters()

	if got := a.layout.SplitAt(0).ContextLabel(); got != "ctx-a" {
		t.Errorf("pane 0 footer = %q, want %q", got, "ctx-a")
	}
	if got := a.layout.SplitAt(1).ContextLabel(); got != "ctx-b" {
		t.Errorf("pane 1 footer = %q, want %q", got, "ctx-b")
	}
}

// TestSyncPaneFooters_PresentButDegradedMarksOffline verifies the present-but-
// degraded branch of syncPaneOffline at the footer-sync level: a pane whose
// context has a registered cluster that reports Connected()==false (a non-nil
// error / failing heartbeat) is marked offline by syncPaneFooters, and recovery
// (re-registering the context as connected) clears the marker on the next sync.
func TestSyncPaneFooters_PresentButDegradedMarksOffline(t *testing.T) {
	a := appWithManager(t, "ctx-a", "ctx-b")

	// Register ctx-b as a present-but-degraded cluster: a non-nil error and no
	// live client, so Connected() is false.
	a.mgr.Register(cluster.New("ctx-b", "", nil, nil, nil, errors.New("heartbeat failed")))

	// Point a second pane at ctx-b.
	a = addSplit(t, a, "deployments")
	a.layout.SplitAt(1).SetContext("ctx-b")

	a.syncPaneFooters()

	if !a.layout.SplitAt(1).Offline() {
		t.Fatal("expected pane on degraded ctx-b to be marked offline")
	}

	// Recovery: re-register ctx-b as a connected cluster, then re-sync.
	gclient := fakeClientFor("ctx-b")
	gstore := k8s.NewStore(gclient.Dynamic, "ctx-b", nil)
	a.mgr.Register(cluster.New("ctx-b", "", gclient, gstore, k8s.NewDiscovery(), nil))

	a.syncPaneFooters()

	if a.layout.SplitAt(1).Offline() {
		t.Fatal("expected offline marker cleared after ctx-b reconnected")
	}
}

// TestSyncPaneFooters_ConvergingBackToOneClearsLabels verifies that once panes
// converge back to a single shared context, a re-run of syncPaneFooters clears
// the previously-set labels on every pane.
func TestSyncPaneFooters_ConvergingBackToOneClearsLabels(t *testing.T) {
	a := appWithManager(t, "ctx-a", "ctx-b")

	// Two panes on distinct contexts: labels appear.
	a = addSplit(t, a, "deployments")
	a.layout.SplitAt(1).SetContext("ctx-b")
	a.syncPaneFooters()
	if a.layout.SplitAt(0).ContextLabel() == "" || a.layout.SplitAt(1).ContextLabel() == "" {
		t.Fatalf("precondition: expected both panes labeled while distinct, got %q / %q",
			a.layout.SplitAt(0).ContextLabel(), a.layout.SplitAt(1).ContextLabel())
	}

	// Converge: put pane 1 back on ctx-a so all panes share one context.
	a.layout.SplitAt(1).SetContext("ctx-a")
	a.syncPaneFooters()

	for i := 0; i < a.layout.SplitCount(); i++ {
		if got := a.layout.SplitAt(i).ContextLabel(); got != "" {
			t.Errorf("pane %d footer = %q after converging to one context, want empty", i, got)
		}
	}
}
