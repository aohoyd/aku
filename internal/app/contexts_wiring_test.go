package app

import (
	"testing"

	"github.com/aohoyd/aku/internal/cluster"
	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/plugins/contexts"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// registerContextsPlugin registers the real contexts plugin over mgr alongside
// the pods plugin that newContextSwitchApp already registered. It returns the
// instance so tests can inspect its PANES counts.
func registerContextsPlugin(t *testing.T, mgr *cluster.Manager) *contexts.Plugin {
	t.Helper()
	p := contexts.New(mgr)
	plugin.Register(p)
	// Reset the process-global plugin registry at test end so registrations do
	// not leak into other tests that share the registry.
	t.Cleanup(func() { plugin.Reset() })
	return p
}

// TestGotoContexts_PushesContextsViewOntoFocusedPane proves gX (goto-contexts)
// opens the contexts plugin IN the focused pane by pushing the current resource
// onto the nav stack, so the pane's plugin becomes "contexts" and the pane is in
// a drill-down (so Esc/back returns to the prior resource).
func TestGotoContexts_PushesContextsViewOntoFocusedPane(t *testing.T) {
	mgr := newTestManagerWithContexts(t, "global", map[string][]*unstructured.Unstructured{
		"global":  {testPod("global-pod", "default")},
		"staging": {testPod("staging-pod", "default")},
	})
	app := newContextSwitchApp(t, mgr)
	registerContextsPlugin(t, mgr)
	app.layout.FocusSplitAt(0)

	pane := app.layout.FocusedSplit()
	if pane.Plugin().Name() != "pods" {
		t.Fatalf("precondition: focused pane should be on pods, got %q", pane.Plugin().Name())
	}

	model, _ := app.executeCommand("goto-contexts")
	app = model.(App)

	pane = app.layout.FocusedSplit()
	if pane.Plugin().Name() != "contexts" {
		t.Fatalf("expected focused pane plugin 'contexts' after gX, got %q", pane.Plugin().Name())
	}
	if !pane.InDrillDown() {
		t.Fatalf("expected pane to be in a drill-down so back returns to prior resource")
	}
	// The pane keeps its current context while browsing.
	if pane.Context() != "global" {
		t.Fatalf("expected pane to keep context 'global' while browsing, got %q", pane.Context())
	}
}

// TestPaneSwitchContext_FromGX_ReturnsToPriorResource proves selecting a context
// from the gX in-pane contexts view switches the focused pane to the chosen
// context AND returns it to the prior resource (pods) — not the contexts list.
func TestPaneSwitchContext_FromGX_ReturnsToPriorResource(t *testing.T) {
	mgr := newTestManagerWithContexts(t, "global", map[string][]*unstructured.Unstructured{
		"global":  {testPod("global-pod", "default")},
		"staging": {testPod("staging-a", "default"), testPod("staging-b", "default")},
	})
	app := newContextSwitchApp(t, mgr)
	registerContextsPlugin(t, mgr)
	app.layout.FocusSplitAt(0)

	// Open contexts in the focused pane (gX).
	model, _ := app.executeCommand("goto-contexts")
	app = model.(App)
	if app.layout.FocusedSplit().Plugin().Name() != "contexts" {
		t.Fatalf("precondition: pane should show contexts view")
	}

	// Select "staging" — dispatched as the Commander command.
	model, connectCmd := app.executeCommand("pane-switch-context staging")
	app = model.(App)
	if connectCmd == nil {
		t.Fatalf("expected an async connect command from pane-switch-context")
	}

	pane := app.layout.FocusedSplit()
	// Pane returned to the prior resource (pods), NOT the contexts list.
	if pane.Plugin().Name() != "pods" {
		t.Fatalf("expected pane to return to prior resource 'pods', got %q", pane.Plugin().Name())
	}
	// And it is optimistically on the new context.
	if pane.Context() != "staging" {
		t.Fatalf("expected pane on 'staging', got %q", pane.Context())
	}
	if pane.InDrillDown() {
		t.Fatalf("expected pane to no longer be in a drill-down after switch")
	}

	// Complete the async connect; the pane lands on pods on staging.
	ready := extractClusterReady(t, connectCmd)
	if ready.Context != "staging" {
		t.Fatalf("expected connect for 'staging', got %q", ready.Context)
	}
	model, _ = app.handleClusterReady(ready)
	app = model.(App)

	pane = app.layout.FocusedSplit()
	if pane.Plugin().Name() != "pods" {
		t.Fatalf("expected pane on 'pods' after ready, got %q", pane.Plugin().Name())
	}
	if pane.Context() != "staging" {
		t.Fatalf("expected pane on 'staging' after ready, got %q", pane.Context())
	}

	// The staging informer populates the pods pane with staging's two pods.
	app = deliverResourceUpdate(t, app, "staging", 2)
	if app.layout.FocusedSplit().Len() != 2 {
		t.Fatalf("expected staging pods pane to show 2 objects, got %d", app.layout.FocusedSplit().Len())
	}
}

// TestSplitContexts_AddsContextsSplitAndLandsOnPods proves oX (split-contexts)
// adds a NEW split showing the contexts plugin, and selecting a context pins
// that split to the chosen context and lands it on pods (no prior resource).
func TestSplitContexts_AddsContextsSplitAndLandsOnPods(t *testing.T) {
	mgr := newTestManagerWithContexts(t, "global", map[string][]*unstructured.Unstructured{
		"global":  {testPod("global-pod", "default")},
		"staging": {testPod("staging-pod", "default")},
	})
	app := newContextSwitchApp(t, mgr)
	registerContextsPlugin(t, mgr)
	app.layout.FocusSplitAt(0)

	before := app.layout.SplitCount()

	// oX → split-contexts.
	model, _ := app.executeCommand("split-contexts")
	app = model.(App)

	if app.layout.SplitCount() != before+1 {
		t.Fatalf("expected %d splits after oX, got %d", before+1, app.layout.SplitCount())
	}
	newSplit := app.layout.FocusedSplit()
	if newSplit.Plugin().Name() != "contexts" {
		t.Fatalf("expected new split to show 'contexts', got %q", newSplit.Plugin().Name())
	}
	// A fresh oX split is rooted on contexts: NOT in a drill-down.
	if newSplit.InDrillDown() {
		t.Fatalf("expected fresh oX contexts split to be at root (no nav stack)")
	}

	// Select staging in the new split.
	model, connectCmd := app.executeCommand("pane-switch-context staging")
	app = model.(App)
	if connectCmd == nil {
		t.Fatalf("expected an async connect command")
	}

	newSplit = app.layout.FocusedSplit()
	// Fresh split with no prior resource lands on pods.
	if newSplit.Plugin().Name() != "pods" {
		t.Fatalf("expected fresh contexts split to land on 'pods', got %q", newSplit.Plugin().Name())
	}
	if newSplit.Context() != "staging" {
		t.Fatalf("expected new split pinned to 'staging', got %q", newSplit.Context())
	}

	// Other split is untouched: still on global.
	if p0 := app.layout.SplitAt(0); p0.Context() != "global" {
		t.Fatalf("expected original split to stay on 'global', got %q", p0.Context())
	}

	ready := extractClusterReady(t, connectCmd)
	model, _ = app.handleClusterReady(ready)
	app = model.(App)
	if app.layout.FocusedSplit().Context() != "staging" {
		t.Fatalf("expected new split on 'staging' after ready, got %q", app.layout.FocusedSplit().Context())
	}
}

// TestPaneSwitchContext_IsAsyncNoInlineDial proves the pane-switch-context
// command uses the async path: the (potentially blocking) connect MUST NOT run
// on the Update goroutine. A dial counter injected via the manager's connect
// seam (mgr.SetConnect) stays at zero while the handler runs, and increments
// only when the returned tea.Cmd is later executed off-thread — mirroring
// TestGlobalContextSwitchDoesNotDialInline.
func TestPaneSwitchContext_IsAsyncNoInlineDial(t *testing.T) {
	mgr := newTestManagerWithContexts(t, "global", map[string][]*unstructured.Unstructured{
		"global":  {testPod("global-pod", "default")},
		"staging": {testPod("staging-pod", "default")},
	})

	var dials int
	mgr.SetConnect(func(file, ctx string) (*k8s.Client, error) {
		dials++
		return paneFakeClient(ctx, []*unstructured.Unstructured{testPod("staging-pod", "default")}), nil
	})

	app := newContextSwitchApp(t, mgr)
	registerContextsPlugin(t, mgr)
	app.layout.FocusSplitAt(0)

	model, _ := app.executeCommand("goto-contexts")
	app = model.(App)

	model, connectCmd := app.executeCommand("pane-switch-context staging")
	app = model.(App)
	if connectCmd == nil {
		t.Fatalf("expected a non-nil async connect command")
	}
	// The blocking connect must NOT have run inline on the Update goroutine.
	if dials != 0 {
		t.Fatalf("pane-switch-context dialed inline (%d connects); it must dial off-thread", dials)
	}
	// No inline install: staging is not in the manager yet.
	if _, ok := mgr.Get("staging"); ok {
		t.Fatalf("expected no manager entry for 'staging' before handleClusterReady (no inline dial)")
	}

	// Running the returned cmd (what the runtime does off the Update goroutine)
	// performs exactly one dial and yields a ClusterReadyMsg for the target.
	ready := extractClusterReady(t, connectCmd)
	if dials != 1 {
		t.Fatalf("expected exactly 1 dial after the cmd ran, got %d", dials)
	}
	if ready.Context != "staging" {
		t.Fatalf("expected ClusterReadyMsg for 'staging', got %q", ready.Context)
	}
}

// TestSyncContextPaneCounts_FeedsPaneCounts proves the App pushes per-context
// pane counts into the contexts plugin (these drive the STATUS glyph's
// in-use/idle distinction). The startup app already has one global pane and
// newContextSwitchApp adds a second pane seeded with the startup context
// (global), so two panes count under global before staging is added.
func TestSyncContextPaneCounts_FeedsPaneCounts(t *testing.T) {
	mgr := newTestManagerWithContexts(t, "global", map[string][]*unstructured.Unstructured{
		"global":  {testPod("global-pod", "default")},
		"staging": {testPod("staging-pod", "default")},
	})
	app := newContextSwitchApp(t, mgr)
	cp := registerContextsPlugin(t, mgr)
	app.layout.FocusSplitAt(0)

	// Both existing panes are on global (both seeded explicitly). staging has no
	// pane yet.
	app.syncContextPaneCounts()
	if cp.PaneCount("global") != 2 {
		t.Fatalf("expected 2 panes on 'global', got %d", cp.PaneCount("global"))
	}
	if cp.PaneCount("staging") != 0 {
		t.Fatalf("expected 0 panes on 'staging', got %d", cp.PaneCount("staging"))
	}
	// staging has no pane and is not connected → idle ○.
	if got := contextStatus(cp, "staging"); got != statusIdleGlyph {
		t.Fatalf("staging STATUS = %q, want idle ○", got)
	}

	// Add a split on staging.
	pods := app.layout.FocusedSplit().Plugin()
	app.layout.AddSplit(pods, "default", "staging")

	// Re-focus the global pane; the two global panes still count under global.
	app.layout.FocusSplitAt(0)
	app.syncContextPaneCounts()
	if cp.PaneCount("global") != 2 {
		t.Fatalf("expected 2 panes on 'global' after adding staging, got %d", cp.PaneCount("global"))
	}
	if cp.PaneCount("staging") != 1 {
		t.Fatalf("expected 1 pane on 'staging', got %d", cp.PaneCount("staging"))
	}
	// staging now has a pane but is not connected → in-use ● (not idle).
	if got := contextStatus(cp, "staging"); got == statusIdleGlyph {
		t.Fatalf("staging STATUS should be in-use ● now it has a pane, got idle ○")
	}
}

// TestSplitContexts_RefreshesPaneCountsEndToEnd proves that running the
// split-contexts (oX) command end-to-end refreshes the contexts plugin's pane
// counts — not just by calling syncContextPaneCounts manually. handleSplit adds
// a new pane (inheriting the focused pane's context) and refreshes the counts,
// so after oX the inherited context's pane count reflects the new split.
func TestSplitContexts_RefreshesPaneCountsEndToEnd(t *testing.T) {
	mgr := newTestManagerWithContexts(t, "global", map[string][]*unstructured.Unstructured{
		"global":  {testPod("global-pod", "default")},
		"staging": {testPod("staging-pod", "default")},
	})
	app := newContextSwitchApp(t, mgr)
	cp := registerContextsPlugin(t, mgr)
	app.layout.FocusSplitAt(0)

	// Baseline: prime the counts once. The two existing panes are both on global.
	app.syncContextPaneCounts()
	if cp.PaneCount("global") != 2 {
		t.Fatalf("precondition: expected 2 panes on 'global', got %d", cp.PaneCount("global"))
	}

	// oX → split-contexts adds a new pane that inherits the focused (global)
	// context. handleSplit refreshes the counts end-to-end (we do NOT call
	// syncContextPaneCounts ourselves after this point).
	model, _ := app.executeCommand("split-contexts")
	app = model.(App)

	if app.layout.FocusedSplit().Plugin().Name() != "contexts" {
		t.Fatalf("expected new split to show 'contexts'")
	}

	// Assert the count refreshed end-to-end (NOT by calling syncContextPaneCounts
	// ourselves).
	if cp.PaneCount("global") != 3 {
		t.Fatalf("expected 3 panes on 'global' after oX split, got %d", cp.PaneCount("global"))
	}
}

// selectRowByName moves the focused pane's cursor onto the row whose object
// name equals want, scanning from the top. It fails the test if no such row is
// present, so callers can rely on Selected() returning the wanted row afterward.
func selectRowByName(t *testing.T, app App, want string) {
	t.Helper()
	pane := app.layout.FocusedSplit()
	for i := 0; i < pane.Len(); i++ {
		pane.SetCursor(i)
		if sel := pane.Selected(); sel != nil && sel.GetName() == want {
			return
		}
	}
	t.Fatalf("no contexts row named %q to select (pane has %d rows)", want, pane.Len())
}

// TestEnterDetail_OnContextsRow_SwitchesPaneViaCommander exercises the REAL gX
// selection chain end-to-end: a real contexts-plugin row → enter-detail →
// plugin.Commander.Command → "pane-switch-context <name>" → executeCommand →
// handlePaneSwitchContext. It deliberately drives executeCommand("enter-detail")
// (the same path the runtime uses for Enter) rather than calling
// executeCommand("pane-switch-context …") directly, so the Commander wiring is
// what routes the switch. The async ClusterReadyMsg is driven to completion and
// the pane is asserted to land on the chosen context showing the prior resource.
func TestEnterDetail_OnContextsRow_SwitchesPaneViaCommander(t *testing.T) {
	mgr := newTestManagerWithContexts(t, "global", map[string][]*unstructured.Unstructured{
		"global":  {testPod("global-pod", "default")},
		"staging": {testPod("staging-a", "default"), testPod("staging-b", "default")},
	})
	app := newContextSwitchApp(t, mgr)
	registerContextsPlugin(t, mgr)
	app.layout.FocusSplitAt(0)

	// Open the contexts view in the focused pane (gX). The pane is now showing the
	// real contexts plugin rows, with the prior resource (pods) on its nav stack.
	model, _ := app.executeCommand("goto-contexts")
	app = model.(App)
	if app.layout.FocusedSplit().Plugin().Name() != "contexts" {
		t.Fatalf("precondition: focused pane should show the contexts view")
	}

	// Put the cursor on the "staging" row so Selected() returns it, exactly as a
	// user pressing j to it would.
	selectRowByName(t, app, "staging")

	// Press Enter via the SAME command the runtime dispatches. This must route
	// through plugin.Commander (Command → "pane-switch-context staging") →
	// executeCommand → handlePaneSwitchContext, NOT through goto/drill-down.
	model, connectCmd := app.executeCommand("enter-detail")
	app = model.(App)
	if connectCmd == nil {
		t.Fatalf("expected an async connect command from the enter-detail→Commander chain")
	}

	pane := app.layout.FocusedSplit()
	// The pane is optimistically on staging and returned to the prior resource.
	if pane.Context() != "staging" {
		t.Fatalf("expected pane optimistically on 'staging' after enter-detail, got %q", pane.Context())
	}
	if pane.Plugin().Name() != "pods" {
		t.Fatalf("expected pane back on prior resource 'pods', got %q", pane.Plugin().Name())
	}

	// Drive the async connect to completion; the pane lands on pods on staging.
	ready := extractClusterReady(t, connectCmd)
	if ready.Context != "staging" {
		t.Fatalf("expected connect for 'staging', got %q", ready.Context)
	}
	model, _ = app.handleClusterReady(ready)
	app = model.(App)

	pane = app.layout.FocusedSplit()
	if pane.Plugin().Name() != "pods" {
		t.Fatalf("expected pane on 'pods' after ready, got %q", pane.Plugin().Name())
	}
	if pane.Context() != "staging" {
		t.Fatalf("expected pane on 'staging' after ready, got %q", pane.Context())
	}

	// The staging informer populates the pods pane with staging's two pods.
	app = deliverResourceUpdate(t, app, "staging", 2)
	if app.layout.FocusedSplit().Len() != 2 {
		t.Fatalf("expected staging pods pane to show 2 objects, got %d", app.layout.FocusedSplit().Len())
	}
}

// TestFocusChange_UpdatesStatusBarContext proves the status-line context name
// and its online/offline color follow the focused pane: with two panes on two
// contexts, focusing each makes the badge show that pane's context (and the
// online flag matches its cluster's connection state — global is connected,
// staging is not).
func TestFocusChange_UpdatesStatusBarContext(t *testing.T) {
	mgr := newTestManagerWithContexts(t, "global", map[string][]*unstructured.Unstructured{
		"global":  {testPod("global-pod", "default")},
		"staging": {testPod("staging-pod", "default")},
	})
	app := newContextSwitchApp(t, mgr)

	// Put the two panes on distinct contexts.
	app.layout.SplitAt(0).SetContext("global")
	app.layout.SplitAt(1).SetContext("staging")

	// Focus the global pane: badge shows global, online (global is connected).
	app.layout.FocusSplitAt(0)
	app = app.syncStatusBarContext()
	if got := app.statusBar.ContextName(); got != "global" {
		t.Fatalf("focused global: ContextName = %q, want global", got)
	}
	if !app.statusBar.Online() {
		t.Fatalf("focused global: expected online (global cluster is connected)")
	}

	// Focus the next pane (staging) via the real command: badge follows to
	// staging, offline (staging is not connected).
	model, _ := app.executeCommand("focus-next-split")
	app = model.(App)
	if got := app.layout.FocusedSplit().Context(); got != "staging" {
		t.Fatalf("precondition: focus-next-split should focus staging pane, got %q", got)
	}
	if got := app.statusBar.ContextName(); got != "staging" {
		t.Fatalf("focused staging: ContextName = %q, want staging", got)
	}
	if app.statusBar.Online() {
		t.Fatalf("focused staging: expected offline (staging cluster not connected)")
	}
}

// statusIdleGlyph is the uncolored STATUS rendering for a context no pane uses.
// In-use contexts render a color-wrapped ● (ANSI codes), so a plain "○" match is
// an unambiguous "idle" check.
const statusIdleGlyph = "○"

// contextStatus reads the STATUS cell the contexts plugin produced for the named
// context.
func contextStatus(cp *contexts.Plugin, name string) string {
	for _, o := range cp.Objects() {
		if o.GetName() == name {
			s, _, _ := unstructured.NestedString(o.Object, "status")
			return s
		}
	}
	return ""
}
