package app

import (
	"errors"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/aohoyd/aku/internal/cluster"
	"github.com/aohoyd/aku/internal/config"
	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/layout"
	"github.com/aohoyd/aku/internal/msgs"
	"github.com/aohoyd/aku/internal/plugin"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
)

var paneCtxPodsGVR = schema.GroupVersionResource{Version: "v1", Resource: "pods"}

// testPod builds an unstructured pod for a fake dynamic client.
func testPod(name, ns string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]any{
			"name":      name,
			"namespace": ns,
		},
	}}
}

// paneFakeClient builds a fake k8s.Client for ctxName seeded with the given
// pods. The dynamic fake registers the pods list kind so the informer's LIST
// does not panic (mirrors the global-switch tests' fakeClientFor).
func paneFakeClient(ctxName string, pods []*unstructured.Unstructured) *k8s.Client {
	objs := make([]runtime.Object, 0, len(pods))
	for _, p := range pods {
		objs = append(objs, p)
	}
	dyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			{Version: "v1", Resource: "pods"}: "PodList",
		},
		objs...,
	)
	return &k8s.Client{
		Dynamic:   dyn,
		Typed:     fake.NewSimpleClientset(),
		Namespace: "default",
		Context:   ctxName,
	}
}

// newTestManagerWithContexts builds a Manager that knows the given contexts and
// connects each to its own fake cluster seeded with that context's pods. The
// global context's cluster is eagerly registered (connected) with a real store
// seeded from its fake dynamic client. Other contexts connect lazily through
// SetConnect when GetOrCreate/Dial is called, exactly like the per-pane switch
// does in production.
func newTestManagerWithContexts(t *testing.T, globalCtx string, pods map[string][]*unstructured.Unstructured) *cluster.Manager {
	t.Helper()

	entries := make([]cluster.ContextEntry, 0, len(pods))
	for name := range pods {
		entries = append(entries, cluster.ContextEntry{Name: name, File: ""})
	}
	mgr := cluster.NewManager(entries, "", 0)

	// Lazy connect builds a fake client per context, seeded with that context's
	// pods. GetOrCreate attaches a store/discovery built from client.Dynamic.
	mgr.SetConnect(func(file, ctx string) (*k8s.Client, error) {
		return paneFakeClient(ctx, pods[ctx]), nil
	})

	// Eagerly register the connected startup cluster with a real store seeded from
	// its fake dynamic client.
	gclient := paneFakeClient(globalCtx, pods[globalCtx])
	gstore := k8s.NewStore(gclient.Dynamic, globalCtx, nil)
	mgr.Register(cluster.New(globalCtx, "", gclient, gstore, k8s.NewDiscovery(), nil))

	return mgr
}

// newContextSwitchApp builds an App backed by mgr whose split[0] is a pods pane
// explicitly stamped with the global context, mirroring how the initial split is
// created in production. Tests operate on split[0] (or add further splits and
// assert against the resulting indices); they must not assume split[0] is the
// only pane. The explicit SetContext on split[0] keeps its context well-defined
// regardless of how app.New seeds the initial layout.
func newContextSwitchApp(t *testing.T, mgr *cluster.Manager) App {
	t.Helper()
	km := config.DefaultKeymap()
	cfg := config.DefaultConfig()
	plugin.Reset()
	pods := &mockPlugin{name: "pods", gvr: paneCtxPodsGVR}
	plugin.Register(pods)

	// The startup context is the single eagerly-registered cluster's context
	// (newTestManagerWithContexts registers exactly one).
	startupCtx := ""
	mgr.ForEach(func(c *cluster.Cluster) { startupCtx = c.Context() })

	a := New(mgr, km, cfg, nil, nil, nil, nil, layout.OrientationVertical, startupCtx)
	// The second pane is born carrying the startup context, mirroring the real
	// inherit-on-split behavior (panes are never created context-less).
	a.layout.AddSplit(pods, "default", startupCtx)
	return a
}

// deliverResourceUpdate simulates the informer firing a ResourceUpdatedMsg for
// the given context once the cluster's store cache has synced, driving the
// pane's objects through the same Update path the real app uses. It polls the
// store until the cache reports the expected count, then runs Update.
func deliverResourceUpdate(t *testing.T, a App, ctxName string, want int) App {
	t.Helper()
	store := a.storeForContext(ctxName)
	if store == nil {
		t.Fatalf("no store for context %q", ctxName)
	}
	for i := 0; i < 200; i++ {
		if len(store.List(paneCtxPodsGVR, "default")) >= want {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	model, _ := a.Update(k8s.ResourceUpdatedMsg{GVR: paneCtxPodsGVR, Namespace: "default", Context: ctxName})
	return model.(App)
}

// extractClusterReady pulls a msgs.ClusterReadyMsg out of a command result,
// which may be the message directly or a tea.BatchMsg of sub-commands
// (handlePaneContextSwitch batches a status update with the connect cmd).
func extractClusterReady(t *testing.T, c tea.Cmd) msgs.ClusterReadyMsg {
	t.Helper()
	if cr, ok := extractClusterReadyFromCmd(c); ok {
		return cr
	}
	t.Fatalf("no ClusterReadyMsg found in command result")
	return msgs.ClusterReadyMsg{}
}

func extractClusterReadyFromCmd(c tea.Cmd) (msgs.ClusterReadyMsg, bool) {
	if c == nil {
		return msgs.ClusterReadyMsg{}, false
	}
	switch m := c().(type) {
	case msgs.ClusterReadyMsg:
		return m, true
	case tea.BatchMsg:
		for _, sub := range m {
			if cr, ok := extractClusterReadyFromCmd(sub); ok {
				return cr, true
			}
		}
	}
	return msgs.ClusterReadyMsg{}, false
}

func TestHandlePaneContextSwitch_OptimisticPin(t *testing.T) {
	mgr := newTestManagerWithContexts(t, "global", map[string][]*unstructured.Unstructured{
		"global":  {testPod("global-pod", "default")},
		"staging": {testPod("staging-pod", "default")},
	})
	app := newContextSwitchApp(t, mgr)

	model, cmd := app.handlePaneContextSwitch("staging")
	got := model.(App)

	pane := got.layout.FocusedSplit()
	if pane.Context() != "staging" {
		t.Fatalf("expected pane context 'staging', got %q", pane.Context())
	}
	if pane.Len() != 0 {
		t.Fatalf("expected pane cleared optimistically, got %d objects", pane.Len())
	}
	if cmd == nil {
		t.Fatalf("expected a non-nil connect command")
	}
}

func TestHandlePaneContextSwitch_NoOpWhenAlreadyOnSameContext(t *testing.T) {
	mgr := newTestManagerWithContexts(t, "global", map[string][]*unstructured.Unstructured{
		"global":  {testPod("global-pod", "default")},
		"staging": {testPod("staging-pod", "default")},
	})
	app := newContextSwitchApp(t, mgr)
	pane := app.layout.FocusedSplit()
	pane.SetContext("staging")
	// Already on a CONNECTED staging cluster: re-selecting it is a no-op.
	app.mgr.RegisterConnected("staging", paneFakeClient("staging", nil))

	_, cmd := app.handlePaneContextSwitch("staging")
	if cmd != nil {
		t.Fatalf("expected no-op (nil cmd) when re-selecting the same connected context")
	}
}

// TestHandlePaneContextSwitch_RetriesAfterFailedConnect proves a pane left on a
// broken context by a failed connect can RETRY: re-selecting the SAME context
// when its cluster is NOT connected must re-attempt the dial (non-nil cmd), not
// silently dead-end on the already-on-context guard.
func TestHandlePaneContextSwitch_RetriesAfterFailedConnect(t *testing.T) {
	mgr := newTestManagerWithContexts(t, "global", map[string][]*unstructured.Unstructured{
		"global":  {testPod("global-pod", "default")},
		"staging": {testPod("staging-pod", "default")},
	})
	app := newContextSwitchApp(t, mgr)
	pane := app.layout.FocusedSplit()
	// On staging but the connect FAILED: no cluster is registered for it.
	pane.SetContext("staging")

	_, cmd := app.handlePaneContextSwitch("staging")
	if cmd == nil {
		t.Fatal("expected retry connect cmd for broken context, got nil")
	}
	ready := extractClusterReady(t, cmd)
	if ready.Context != "staging" {
		t.Fatalf("expected retry to connect staging, got %q", ready.Context)
	}
}

func TestHandlePaneContextSwitch_ClusterReadyPopulatesAndLandsOnDefaultNs(t *testing.T) {
	mgr := newTestManagerWithContexts(t, "global", map[string][]*unstructured.Unstructured{
		"global":  {testPod("global-pod", "default")},
		"staging": {testPod("staging-a", "default"), testPod("staging-b", "default")},
	})
	app := newContextSwitchApp(t, mgr)

	// Pin the focused pane to staging.
	model, connectCmd := app.handlePaneContextSwitch("staging")
	app = model.(App)
	if connectCmd == nil {
		t.Fatalf("expected connect command")
	}

	// Run the async connect off-thread and deliver the resulting ClusterReadyMsg.
	ready := extractClusterReady(t, connectCmd)
	if ready.Err != nil {
		t.Fatalf("unexpected connect error: %v", ready.Err)
	}

	model, _ = app.handleClusterReady(ready)
	app = model.(App)

	pane := app.layout.FocusedSplit()
	if pane.Context() != "staging" {
		t.Fatalf("expected pane on staging, got %q", pane.Context())
	}
	// staging cluster's fake client default namespace is "default".
	if pane.Namespace() != "default" {
		t.Fatalf("expected pane to land on staging default ns 'default', got %q", pane.Namespace())
	}

	// The pane subscribed on the staging store. Drive the informer's
	// ResourceUpdatedMsg through Update; the pane populates from the staging
	// store with staging's two pods, distinct from the single global pod.
	app = deliverResourceUpdate(t, app, "staging", 2)
	if app.layout.FocusedSplit().Len() != 2 {
		t.Fatalf("expected staging pane to show 2 objects, got %d", app.layout.FocusedSplit().Len())
	}
}

func TestHandlePaneContextSwitch_SecondPaneStaysOnGlobal(t *testing.T) {
	mgr := newTestManagerWithContexts(t, "global", map[string][]*unstructured.Unstructured{
		"global":  {testPod("global-pod", "default")},
		"staging": {testPod("staging-pod", "default")},
	})
	app := newContextSwitchApp(t, mgr)

	// Add a second pane (inherits global).
	pods := app.layout.FocusedSplit().Plugin()
	app.layout.AddSplit(pods, "default", "")
	app.layout.SplitAt(1).SetContext("global")

	// Focus the second pane and switch it to staging.
	app.layout.FocusSplitAt(1)
	model, connectCmd := app.handlePaneContextSwitch("staging")
	app = model.(App)
	ready := extractClusterReady(t, connectCmd)
	model, _ = app.handleClusterReady(ready)
	app = model.(App)

	// Pane 0 is untouched: still on global.
	p0 := app.layout.SplitAt(0)
	if p0.Context() != "global" {
		t.Fatalf("expected pane 0 to stay on global, got %q", p0.Context())
	}
	// Pane 1 is now on staging.
	p1 := app.layout.SplitAt(1)
	if p1.Context() != "staging" {
		t.Fatalf("expected pane 1 on staging, got ctx=%q", p1.Context())
	}
}

func TestHandlePaneContextSwitch_ReswitchNoPanic(t *testing.T) {
	mgr := newTestManagerWithContexts(t, "global", map[string][]*unstructured.Unstructured{
		"global":  {testPod("global-pod", "default")},
		"staging": {testPod("staging-pod", "default")},
		"prod":    {testPod("prod-pod", "default")},
	})
	app := newContextSwitchApp(t, mgr)

	// global -> staging (switch to staging, drop global)
	model, c1 := app.handlePaneContextSwitch("staging")
	app = model.(App)
	model, _ = app.handleClusterReady(extractClusterReady(t, c1))
	app = model.(App)

	// staging -> prod (switch to prod, drop staging). staging hits refcount 0 and
	// is torn down; must not panic.
	model, c2 := app.handlePaneContextSwitch("prod")
	app = model.(App)
	model, _ = app.handleClusterReady(extractClusterReady(t, c2))
	app = model.(App)

	pane := app.layout.FocusedSplit()
	if pane.Context() != "prod" {
		t.Fatalf("expected pane on prod after reswitch, got %q", pane.Context())
	}

	// The staging cluster should have been released and torn down (no longer in
	// the manager's map), while prod is now referenced.
	if _, ok := mgr.Get("staging"); ok {
		t.Fatalf("expected staging cluster to be torn down after release to refcount 0")
	}
	if prod, ok := mgr.Get("prod"); !ok {
		t.Fatalf("expected prod cluster to remain referenced")
	} else if prod.RefCount() != 1 {
		t.Fatalf("expected prod refCount 1 (one pane), got %d", prod.RefCount())
	}
}

// TestHandleClusterReady_FailedConnectLeaksNoRef proves a failed async connect
// (dial error) leaves the Manager with no entry for the failed context and no
// dangling reference — closing the race-fix gap where the old flow Acquired
// (refCount++) even on error and never Released on the failure path.
func TestHandleClusterReady_FailedConnectLeaksNoRef(t *testing.T) {
	entries := []cluster.ContextEntry{{Name: "global"}, {Name: "broken"}}
	mgr := cluster.NewManager(entries, "", 0)
	mgr.SetConnect(func(file, ctx string) (*k8s.Client, error) {
		if ctx == "broken" {
			return nil, errors.New("dial failed")
		}
		return paneFakeClient(ctx, nil), nil
	})
	gclient := paneFakeClient("global", []*unstructured.Unstructured{testPod("global-pod", "default")})
	gstore := k8s.NewStore(gclient.Dynamic, "global", nil)
	mgr.Register(cluster.New("global", "", gclient, gstore, k8s.NewDiscovery(), nil))

	app := newContextSwitchApp(t, mgr)

	// Pin the focused pane to the broken context.
	model, connectCmd := app.handlePaneContextSwitch("broken")
	app = model.(App)
	if connectCmd == nil {
		t.Fatalf("expected connect command")
	}

	// The dial must NOT have registered or ref'd anything (it only dials).
	if _, ok := mgr.Get("broken"); ok {
		t.Fatalf("expected no manager entry for broken context before/after dial")
	}

	// Deliver the failed ClusterReadyMsg.
	ready := extractClusterReady(t, connectCmd)
	if ready.Err == nil {
		t.Fatalf("expected a dial error in ClusterReadyMsg")
	}
	// The dial returned (nil, err): the message carries a typed-nil *k8s.Client.
	// A typed-nil pointer boxed in an interface is NOT interface-nil, so assert on
	// the asserted pointer (which is what handleClusterReady checks), not on the
	// any-typed field.
	if c, _ := ready.Client.(*k8s.Client); c != nil {
		t.Fatalf("expected nil client on failed dial, got %v", c)
	}
	model, _ = app.handleClusterReady(ready)
	app = model.(App)

	// No entry, no leaked ref for the broken context.
	if _, ok := mgr.Get("broken"); ok {
		t.Fatalf("failed connect leaked a manager entry for broken context")
	}

	// The pane is left on the broken context but empty so the user can switch again.
	pane := app.layout.FocusedSplit()
	if pane.Context() != "broken" {
		t.Fatalf("expected pane to stay on broken, got ctx=%q", pane.Context())
	}
	if pane.Len() != 0 {
		t.Fatalf("expected pane empty after failed connect, got %d objects", pane.Len())
	}
}

// TestHandleClusterReady_FailedConnectMarksPaneOffline proves a failed async
// connect flags the affected pane with an "⚠ offline" marker, returns without an
// inline connect, leaves OTHER panes untouched (online), and leaks no manager
// entry for the failed context.
func TestHandleClusterReady_FailedConnectMarksPaneOffline(t *testing.T) {
	entries := []cluster.ContextEntry{{Name: "global"}, {Name: "broken"}}
	mgr := cluster.NewManager(entries, "", 0)
	mgr.SetConnect(func(file, ctx string) (*k8s.Client, error) {
		if ctx == "broken" {
			return nil, errors.New("dial failed")
		}
		return paneFakeClient(ctx, nil), nil
	})
	gclient := paneFakeClient("global", []*unstructured.Unstructured{testPod("global-pod", "default")})
	gstore := k8s.NewStore(gclient.Dynamic, "global", nil)
	mgr.Register(cluster.New("global", "", gclient, gstore, k8s.NewDiscovery(), nil))

	app := newContextSwitchApp(t, mgr)

	// A second pane stays on the (healthy) global context.
	pods := app.layout.FocusedSplit().Plugin()
	app.layout.AddSplit(pods, "default", "")
	app.layout.SplitAt(1).SetContext("global")

	// Focus pane 1 and switch it to the broken context.
	app.layout.FocusSplitAt(1)
	model, connectCmd := app.handlePaneContextSwitch("broken")
	app = model.(App)
	if connectCmd == nil {
		t.Fatalf("expected connect command")
	}

	ready := extractClusterReady(t, connectCmd)
	if ready.Err == nil {
		t.Fatalf("expected a dial error in ClusterReadyMsg")
	}
	model, _ = app.handleClusterReady(ready)
	app = model.(App)

	// The broken pane is flagged offline.
	broken := app.layout.SplitAt(1)
	if broken.Context() != "broken" {
		t.Fatalf("expected pane 1 to stay on broken, got %q", broken.Context())
	}
	if !broken.Offline() {
		t.Fatal("expected the failed pane to be marked offline")
	}

	// The OTHER pane (on global) is intact and NOT offline.
	p0 := app.layout.SplitAt(0)
	if p0.Context() != "global" {
		t.Fatalf("expected pane 0 to stay on global, got %q", p0.Context())
	}
	if p0.Offline() {
		t.Fatal("expected the global pane to remain online (not flagged offline)")
	}

	// No manager entry was leaked for the failed context.
	if _, ok := mgr.Get("broken"); ok {
		t.Fatalf("failed connect leaked a manager entry for broken context")
	}
	// The global cluster is untouched.
	if g, ok := mgr.Get("global"); !ok || !g.Connected() {
		t.Fatal("global cluster must remain connected after a failed connect on another pane")
	}
}

// TestHandleClusterReady_OfflineMarkerRecovers proves the offline marker clears
// automatically once the cluster becomes connected and the sync path runs: a
// pane left offline by a failed dial recovers when a subsequent successful
// connect registers a live cluster and the heartbeat/sync path recomputes the
// markers.
func TestHandleClusterReady_OfflineMarkerRecovers(t *testing.T) {
	entries := []cluster.ContextEntry{{Name: "global"}, {Name: "flaky"}}
	mgr := cluster.NewManager(entries, "", 0)
	fail := true
	mgr.SetConnect(func(file, ctx string) (*k8s.Client, error) {
		if ctx == "flaky" && fail {
			return nil, errors.New("dial failed")
		}
		return paneFakeClient(ctx, nil), nil
	})
	gclient := paneFakeClient("global", []*unstructured.Unstructured{testPod("global-pod", "default")})
	gstore := k8s.NewStore(gclient.Dynamic, "global", nil)
	mgr.Register(cluster.New("global", "", gclient, gstore, k8s.NewDiscovery(), nil))

	app := newContextSwitchApp(t, mgr)

	// First switch fails: the pane is flagged offline.
	model, c1 := app.handlePaneContextSwitch("flaky")
	app = model.(App)
	model, _ = app.handleClusterReady(extractClusterReady(t, c1))
	app = model.(App)
	if !app.layout.FocusedSplit().Offline() {
		t.Fatal("precondition: pane should be offline after the failed dial")
	}

	// The cluster recovers: simulate it becoming connected by registering a live
	// client (as a successful connect would), then run the sync path.
	fail = false
	app.mgr.RegisterConnected("flaky", paneFakeClient("flaky", nil))
	app.syncPaneFooters()

	if app.layout.FocusedSplit().Offline() {
		t.Fatal("expected the offline marker to clear once the cluster is connected")
	}
}

// TestHandleClusterHealth_ClearsOfflineMarkerOnRecovery proves the production
// recovery trigger: when a heartbeat tick is processed after the cluster has
// reconnected, handleClusterHealth's sync path clears the pane's offline marker.
func TestHandleClusterHealth_ClearsOfflineMarkerOnRecovery(t *testing.T) {
	entries := []cluster.ContextEntry{{Name: "global"}, {Name: "flaky"}}
	mgr := cluster.NewManager(entries, "", 0)
	fail := true
	mgr.SetConnect(func(file, ctx string) (*k8s.Client, error) {
		if ctx == "flaky" && fail {
			return nil, errors.New("dial failed")
		}
		return paneFakeClient(ctx, nil), nil
	})
	gclient := paneFakeClient("global", []*unstructured.Unstructured{testPod("global-pod", "default")})
	gstore := k8s.NewStore(gclient.Dynamic, "global", nil)
	mgr.Register(cluster.New("global", "", gclient, gstore, k8s.NewDiscovery(), nil))

	app := newContextSwitchApp(t, mgr)

	model, c1 := app.handlePaneContextSwitch("flaky")
	app = model.(App)
	model, _ = app.handleClusterReady(extractClusterReady(t, c1))
	app = model.(App)
	if !app.layout.FocusedSplit().Offline() {
		t.Fatal("precondition: pane should be offline after the failed dial")
	}

	// Cluster comes back; a heartbeat tick lands and drives the recovery sync.
	fail = false
	app.mgr.RegisterConnected("flaky", paneFakeClient("flaky", nil))
	model, _ = app.handleClusterHealth(msgs.ClusterHealthMsg{Context: "flaky", Online: true})
	app = model.(App)

	if app.layout.FocusedSplit().Offline() {
		t.Fatal("expected heartbeat recovery to clear the offline marker")
	}
}

// TestHandleClusterReady_AppliesToNonFocusedMatchingPane proves that when focus
// moves between dispatching the connect and receiving ClusterReadyMsg, the
// handler still populates the correct (now non-focused) matching pane — instead
// of dropping the message because it no longer matches the focused pane.
func TestHandleClusterReady_AppliesToNonFocusedMatchingPane(t *testing.T) {
	mgr := newTestManagerWithContexts(t, "global", map[string][]*unstructured.Unstructured{
		"global":  {testPod("global-pod", "default")},
		"staging": {testPod("staging-a", "default"), testPod("staging-b", "default")},
	})
	app := newContextSwitchApp(t, mgr)

	// Add a second pane following global, focus it. Pane 0 stays on global.
	pods := app.layout.FocusedSplit().Plugin()
	app.layout.AddSplit(pods, "default", "")
	app.layout.SplitAt(1).SetContext("global")

	// Focus pane 0 and switch it to staging (dispatch connect).
	app.layout.FocusSplitAt(0)
	model, connectCmd := app.handlePaneContextSwitch("staging")
	app = model.(App)
	ready := extractClusterReady(t, connectCmd)

	// Move focus to pane 1 BEFORE the ready msg is handled.
	app.layout.FocusSplitAt(1)

	// Deliver ClusterReadyMsg: it must populate pane 0 (the matching, awaiting,
	// non-focused pane), not silently drop because focus moved.
	model, _ = app.handleClusterReady(ready)
	app = model.(App)

	p0 := app.layout.SplitAt(0)
	if p0.Context() != "staging" {
		t.Fatalf("expected pane 0 on staging, got ctx=%q", p0.Context())
	}
	if p0.Namespace() != "default" {
		t.Fatalf("expected pane 0 to land on staging default ns, got %q", p0.Namespace())
	}

	// The staging cluster must be registered and ref'd by exactly the one
	// matching pane (pane 0) — not leaked, not dropped.
	staging, ok := mgr.Get("staging")
	if !ok {
		t.Fatalf("expected staging cluster registered after ready")
	}
	if staging.RefCount() != 1 {
		t.Fatalf("expected staging refCount 1 (pane 0), got %d", staging.RefCount())
	}

	// Drive the staging informer; pane 0 populates with staging's two pods.
	app = deliverResourceUpdate(t, app, "staging", 2)
	if app.layout.SplitAt(0).Len() != 2 {
		t.Fatalf("expected non-focused matching pane 0 to show 2 staging objects, got %d", app.layout.SplitAt(0).Len())
	}
}

// TestHandlePaneContextSwitch_RapidReswitchReconcilesRefs proves the
// double-switch race is fixed: switching A->B before A's ClusterReadyMsg arrives
// leaves exactly B ref'd and A not ref'd (A torn down, no leak), regardless of
// message ordering, because refcounts are reconciled against the pane's CURRENT
// context.
func TestHandlePaneContextSwitch_RapidReswitchReconcilesRefs(t *testing.T) {
	mgr := newTestManagerWithContexts(t, "global", map[string][]*unstructured.Unstructured{
		"global": {testPod("global-pod", "default")},
		"alpha":  {testPod("alpha-pod", "default")},
		"beta":   {testPod("beta-pod", "default")},
	})
	app := newContextSwitchApp(t, mgr)

	// Switch to alpha (dispatch connect A) but DO NOT deliver its ready msg yet.
	model, cA := app.handlePaneContextSwitch("alpha")
	app = model.(App)
	readyA := extractClusterReady(t, cA)

	// Before A arrives, re-switch to beta (dispatch connect B).
	model, cB := app.handlePaneContextSwitch("beta")
	app = model.(App)
	readyB := extractClusterReady(t, cB)

	// Now deliver the LATE ready msg for A first, then B — the worst ordering.
	model, _ = app.handleClusterReady(readyA)
	app = model.(App)
	model, _ = app.handleClusterReady(readyB)
	app = model.(App)

	pane := app.layout.FocusedSplit()
	if pane.Context() != "beta" {
		t.Fatalf("expected pane on beta after rapid re-switch, got ctx=%q", pane.Context())
	}

	// alpha must NOT be referenced: no pane is on it, so even though A's
	// late ready msg registered it momentarily, SyncRefs tore it back down.
	if a, ok := mgr.Get("alpha"); ok {
		t.Fatalf("expected alpha torn down (no pane references it), still present with refCount %d", a.RefCount())
	}
	// beta is ref'd by exactly the one matching pane.
	beta, ok := mgr.Get("beta")
	if !ok {
		t.Fatalf("expected beta cluster registered and referenced")
	}
	if beta.RefCount() != 1 {
		t.Fatalf("expected beta refCount 1, got %d", beta.RefCount())
	}

	// And beta populates correctly.
	app = deliverResourceUpdate(t, app, "beta", 1)
	if app.layout.FocusedSplit().Len() != 1 {
		t.Fatalf("expected beta pane to show 1 object, got %d", app.layout.FocusedSplit().Len())
	}
}

// TestCloseSplit_ClosesSplitOnStartupContext proves that closing one of two
// panes on the startup context neither panics nor mis-counts: SyncRefs reconciles
// against the remaining panes, and because one pane still references the startup
// context its cluster survives and is still usable.
func TestCloseSplit_ClosesSplitOnStartupContext(t *testing.T) {
	mgr := newTestManagerWithContexts(t, "global", map[string][]*unstructured.Unstructured{
		"global": {testPod("global-pod", "default")},
	})
	app := newContextSwitchApp(t, mgr)

	// Add a second pane on the startup context name.
	pods := app.layout.FocusedSplit().Plugin()
	app.layout.AddSplit(pods, "default", "")
	app.layout.SplitAt(app.layout.SplitCount() - 1).SetContext("global")
	app.layout.FocusSplitAt(app.layout.SplitCount() - 1)

	if app.layout.FocusedSplit().Context() != "global" {
		t.Fatalf("precondition: focused pane should be on the startup context")
	}

	// Close one pane. Must not panic; the startup cluster must survive because the
	// other pane still references it.
	app.layout.FocusSplitAt(app.layout.SplitCount() - 1)
	before := app.layout.SplitCount()
	model, _ := app.executeCommand("close-current-panel")
	app = model.(App)

	if app.layout.SplitCount() != before-1 {
		t.Fatalf("expected %d splits after close, got %d", before-1, app.layout.SplitCount())
	}
	g, ok := mgr.Get("global")
	if !ok {
		t.Fatal("startup cluster must not be torn down while a pane still references it")
	}
	if g == nil || !g.Connected() {
		t.Fatal("startup cluster should remain connected after closing one referencing pane")
	}
}

// TestHandleClusterReady_FailedConnect_PaneEmptyAndWarned extends the failed-
// connect coverage: beyond leaking no ref, a dial failure must leave the pane
// empty AND surface a status error so the user knows the switch failed.
func TestHandleClusterReady_FailedConnect_PaneEmptyAndWarned(t *testing.T) {
	entries := []cluster.ContextEntry{{Name: "global"}, {Name: "broken"}}
	mgr := cluster.NewManager(entries, "", 0)
	mgr.SetConnect(func(file, ctx string) (*k8s.Client, error) {
		if ctx == "broken" {
			return nil, errors.New("dial failed")
		}
		return paneFakeClient(ctx, nil), nil
	})
	gclient := paneFakeClient("global", []*unstructured.Unstructured{testPod("global-pod", "default")})
	gstore := k8s.NewStore(gclient.Dynamic, "global", nil)
	mgr.Register(cluster.New("global", "", gclient, gstore, k8s.NewDiscovery(), nil))

	app := newContextSwitchApp(t, mgr)

	model, connectCmd := app.handlePaneContextSwitch("broken")
	app = model.(App)
	ready := extractClusterReady(t, connectCmd)
	model, _ = app.handleClusterReady(ready)
	app = model.(App)

	pane := app.layout.FocusedSplit()
	if pane.Len() != 0 {
		t.Fatalf("expected pane empty after failed connect, got %d objects", pane.Len())
	}
	if et := app.statusBar.ErrText(); et == "" {
		t.Fatal("expected a status error after a failed connect")
	}
}

// TestHandleClusterReady_SuccessClearsOfflineMarker proves the SUCCESS path of
// handleClusterReady clears a pane's "⚠ offline" marker once its context
// reconnects. A pane marked offline (e.g. by a prior failed connect) must come
// back online when a connected cluster arrives — this is the success-path
// counterpart to handleClusterHealth's recovery. It guards against removing the
// success path's syncPaneFooters call (the only thing that clears the marker
// here).
func TestHandleClusterReady_SuccessClearsOfflineMarker(t *testing.T) {
	mgr := newTestManagerWithContexts(t, "global", map[string][]*unstructured.Unstructured{
		"global":  {testPod("global-pod", "default")},
		"staging": {testPod("staging-pod", "default")},
	})
	app := newContextSwitchApp(t, mgr)

	// Switch the focused pane to staging (dispatch the async connect) and mark the
	// pane offline, simulating the state left by a prior failed connect.
	model, connectCmd := app.handlePaneContextSwitch("staging")
	app = model.(App)
	pane := app.layout.FocusedSplit()
	pane.SetOffline(true)
	if !pane.Offline() {
		t.Fatalf("precondition: pane should be marked offline")
	}

	// Deliver a SUCCESSFUL ClusterReadyMsg for staging. The success path installs
	// the connected client and refreshes footers, which must clear the marker.
	ready := extractClusterReady(t, connectCmd)
	if ready.Err != nil {
		t.Fatalf("precondition: expected a successful connect, got err %v", ready.Err)
	}
	model, _ = app.handleClusterReady(ready)
	app = model.(App)

	if app.layout.FocusedSplit().Offline() {
		t.Fatal("expected pane offline marker cleared after a successful reconnect")
	}
}

// TestHandleClusterReady_MissingResourceLeavesPaneEmpty proves the per-pane
// missing-resource check: when the target cluster's Discovery is populated and
// does NOT know the pane's GVR, the pane is left empty with a warning and no
// subscribe happens (the store has no informer for that GVR).
func TestHandleClusterReady_MissingResourceLeavesPaneEmpty(t *testing.T) {
	mgr := newTestManagerWithContexts(t, "global", map[string][]*unstructured.Unstructured{
		"global":  {testPod("global-pod", "default")},
		"staging": {testPod("staging-pod", "default")},
	})
	app := newContextSwitchApp(t, mgr)

	model, connectCmd := app.handlePaneContextSwitch("staging")
	app = model.(App)
	ready := extractClusterReady(t, connectCmd)

	// Pre-populate staging's Discovery with a resource set that EXCLUDES the
	// pane's pods GVR, so the missing-resource check fires authoritatively.
	stagingCl, _ := app.mgr.RegisterConnected("staging", ready.Client.(*k8s.Client))
	stagingCl.Discovery().Populate([]k8s.APIResource{
		{
			Name:    "services",
			Group:   "",
			Version: "v1",
			Kind:    "Service",
			GVR:     schema.GroupVersionResource{Group: "", Version: "v1", Resource: "services"},
		},
	})
	if stagingCl.Discovery().IsEmpty() {
		t.Fatal("precondition: staging discovery should be populated")
	}

	model, _ = app.handleClusterReady(ready)
	app = model.(App)

	pane := app.layout.FocusedSplit()
	if pane.Len() != 0 {
		t.Fatalf("expected pane empty (pods not available on staging), got %d", pane.Len())
	}
	if wt := app.statusBar.WarningText(); wt == "" {
		t.Fatal("expected a missing-resource warning")
	}
	// No subscribe should have happened: the staging store must have no pods
	// informer registered for the pane's GVR/namespace.
	//
	// Asserting on Store.List() here would be tautological: List reads the
	// asynchronously-populated informer cache, which is empty immediately after a
	// Subscribe call regardless of whether Subscribe ever ran — so it would pass
	// even on broken code that wrongly subscribed. IsSubscribed inspects the
	// informers map directly, deterministically proving Subscribe was NOT called.
	// The positive control TestHandleClusterReady_PresentResourceSubscribes shows
	// IsSubscribed reports true when the resource IS present, so the accessor
	// distinguishes the two cases (this assertion is not vacuously true).
	if stagingCl.Store().IsSubscribed(paneCtxPodsGVR, "default") {
		t.Fatalf("expected no pods subscription on staging store, but one was registered (keys: %v)",
			stagingCl.Store().SubscriptionKeys())
	}
}

// TestHandleClusterReady_PresentResourceSubscribes is the positive control for
// the missing-resource case: when the target cluster's Discovery IS populated
// and DOES know the pane's GVR, handleClusterReady subscribes on that cluster's
// store. It asserts via the same Store.IsSubscribed accessor used by
// TestHandleClusterReady_MissingResourceLeavesPaneEmpty, so the two tests
// together prove IsSubscribed distinguishes "subscribed" from "not subscribed"
// (i.e. the missing-resource assertion is meaningful, not vacuously true).
func TestHandleClusterReady_PresentResourceSubscribes(t *testing.T) {
	mgr := newTestManagerWithContexts(t, "global", map[string][]*unstructured.Unstructured{
		"global":  {testPod("global-pod", "default")},
		"staging": {testPod("staging-pod", "default")},
	})
	app := newContextSwitchApp(t, mgr)

	model, connectCmd := app.handlePaneContextSwitch("staging")
	app = model.(App)
	ready := extractClusterReady(t, connectCmd)

	// Populate staging's Discovery WITH the pane's pods GVR so the missing-resource
	// check does NOT fire and handleClusterReady proceeds to Subscribe.
	stagingCl, _ := app.mgr.RegisterConnected("staging", ready.Client.(*k8s.Client))
	stagingCl.Discovery().Populate([]k8s.APIResource{
		{
			Name:    "pods",
			Group:   "",
			Version: "v1",
			Kind:    "Pod",
			GVR:     paneCtxPodsGVR,
		},
	})
	if stagingCl.Discovery().IsEmpty() {
		t.Fatal("precondition: staging discovery should be populated")
	}

	model, _ = app.handleClusterReady(ready)
	app = model.(App)

	pane := app.layout.FocusedSplit()
	if pane.Context() != "staging" {
		t.Fatalf("expected pane on staging, got %q", pane.Context())
	}
	// The pane's GVR/namespace must now be subscribed on the staging store. This
	// is the inverse of the missing-resource assertion and proves IsSubscribed is
	// not vacuously false in the negative test above.
	if !stagingCl.Store().IsSubscribed(paneCtxPodsGVR, pane.EffectiveNamespace()) {
		t.Fatalf("expected a pods subscription on staging store, but none was registered (keys: %v)",
			stagingCl.Store().SubscriptionKeys())
	}
}

// TestResourceUpdatedMsg_ContextRouting proves the strict per-context routing in
// app.go: a pane on "staging" is updated ONLY by a ResourceUpdatedMsg
// tagged Context:"staging", never by one tagged with another live context.
func TestResourceUpdatedMsg_ContextRouting(t *testing.T) {
	mgr := newTestManagerWithContexts(t, "global", map[string][]*unstructured.Unstructured{
		"global":  {testPod("global-pod", "default")},
		"staging": {testPod("staging-a", "default"), testPod("staging-b", "default")},
	})
	app := newContextSwitchApp(t, mgr)

	// Pin the focused pane to staging and complete the connect.
	model, connectCmd := app.handlePaneContextSwitch("staging")
	app = model.(App)
	model, _ = app.handleClusterReady(extractClusterReady(t, connectCmd))
	app = model.(App)

	pane := app.layout.FocusedSplit()
	if pane.Context() != "staging" {
		t.Fatalf("precondition: pane should be on staging, got %q", pane.Context())
	}

	// Wait for the staging store cache to sync, then deliver a ResourceUpdatedMsg
	// tagged with the WRONG context ("global"): the staging pane must NOT update.
	stagingStore := app.storeForContext("staging")
	for i := 0; i < 200 && len(stagingStore.List(paneCtxPodsGVR, "default")) < 2; i++ {
		time.Sleep(5 * time.Millisecond)
	}
	model, _ = app.Update(k8s.ResourceUpdatedMsg{GVR: paneCtxPodsGVR, Namespace: "default", Context: "global"})
	app = model.(App)
	if app.layout.FocusedSplit().Len() != 0 {
		t.Fatalf("staging pane must NOT update from a Context:global message, got %d objects", app.layout.FocusedSplit().Len())
	}

	// Now deliver the correctly-tagged message: the pane updates with staging's 2 pods.
	model, _ = app.Update(k8s.ResourceUpdatedMsg{GVR: paneCtxPodsGVR, Namespace: "default", Context: "staging"})
	app = model.(App)
	if app.layout.FocusedSplit().Len() != 2 {
		t.Fatalf("staging pane should update from a Context:staging message, got %d objects", app.layout.FocusedSplit().Len())
	}
}
