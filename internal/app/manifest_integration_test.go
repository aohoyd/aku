package app

import (
	"strings"
	"testing"

	"github.com/aohoyd/aku/internal/cluster"
	"github.com/aohoyd/aku/internal/config"
	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/layout"
	"github.com/aohoyd/aku/internal/manifest"
	"github.com/aohoyd/aku/internal/notify"
	"github.com/aohoyd/aku/internal/plugin"
	akudeployments "github.com/aohoyd/aku/internal/plugins/deployments"
	akupods "github.com/aohoyd/aku/internal/plugins/pods"
	akureplicasets "github.com/aohoyd/aku/internal/plugins/replicasets"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// deploymentManifest is a single Deployment (replicas=2) in namespace "foo". The
// manifest synthesizer fabricates the Deployment→ReplicaSet→Pods chain with
// consistent owner-reference UIDs, which the real owner-ref drill-down resolves.
const deploymentManifest = `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
  namespace: foo
spec:
  replicas: 2
  selector:
    matchLabels:
      app: web
  template:
    metadata:
      labels:
        app: web
    spec:
      containers:
        - name: c
          image: nginx:1
`

var deploymentsGVRForTest = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}

// newManifestDeploymentApp loads the given manifest YAML into a pinned static
// cluster, registers the real deployments/replicasets/pods plugins, and builds an
// App focused on the deployments list for the "manifests" context at ns "" (All
// Namespaces, the pinned default). The focused deployments pane is populated from
// the store so the drill-down/scale paths see real rows. It returns the App and
// the deployments plugin.
func newManifestDeploymentApp(t *testing.T, manifestYAML string) (App, plugin.ResourcePlugin) {
	t.Helper()
	km := config.DefaultKeymap()
	cfg := config.DefaultConfig()
	plugin.Reset()
	t.Cleanup(plugin.Reset)

	depPlugin := akudeployments.New()
	plugin.Register(depPlugin)
	plugin.Register(akureplicasets.New())
	plugin.Register(akupods.New())

	cl, _, err := manifest.Load(strings.NewReader(manifestYAML), "default")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	mgr := cluster.NewManager(nil, "", 0)
	mgr.RegisterPinned(cl)

	specs := []ResourceSpec{{Plugin: depPlugin, Namespace: ""}}
	app := New(mgr, km, cfg, nil, nil, nil, specs, nil, layout.OrientationVertical, manifestCtx)

	// Populate the focused deployments pane from the static store (no informers in
	// tests). The pinned cluster dual-keys namespaced objects into the "" bucket.
	store := app.clusterForFocused().Store()
	app.layout.FocusedSplit().SetObjects(store.List(deploymentsGVRForTest, ""))
	return app, depPlugin
}

// TestManifestDrillDown_DeploymentToReplicaSetToPods proves the real owner-ref
// drill-down chains through the static manifest store: the deployments pane shows
// the Deployment; the deployments plugin's DrillDown yields a ReplicaSet; and the
// replicasets plugin's DrillDown on that ReplicaSet yields 2 Pods. This exercises
// the actual deployments/replicasets DrillDown methods (the app Enter path) and
// workload.FindOwned against the store, not a hand-rolled lookup.
func TestManifestDrillDown_DeploymentToReplicaSetToPods(t *testing.T) {
	app, depPlugin := newManifestDeploymentApp(t, deploymentManifest)

	focused := app.layout.FocusedSplit()
	if focused == nil {
		t.Fatal("expected a focused deployments split")
	}
	if focused.Plugin().Name() != "deployments" {
		t.Fatalf("expected focused pane on deployments, got %q", focused.Plugin().Name())
	}

	// The deployments list shows the Deployment.
	dep := focused.Selected()
	if dep == nil || dep.GetName() != "web" {
		got := "<nil>"
		if dep != nil {
			got = dep.GetName()
		}
		t.Fatalf("expected deployments pane to show 'web', got %q", got)
	}

	cl := app.clusterForFocused()

	// Deployment → ReplicaSet via the real DrillDown method.
	drillDep, ok := depPlugin.(plugin.DrillDowner)
	if !ok {
		t.Fatal("deployments plugin must implement DrillDowner")
	}
	rsPlugin, replicaSets := drillDep.DrillDown(cl, dep)
	if len(replicaSets) == 0 {
		t.Fatal("expected Deployment drill-down to yield a ReplicaSet (owner-ref UID chaining), got none")
	}
	if rsPlugin == nil || rsPlugin.Name() != "replicasets" {
		got := "<nil>"
		if rsPlugin != nil {
			got = rsPlugin.Name()
		}
		t.Fatalf("expected drill-down child plugin 'replicasets', got %q", got)
	}

	// ReplicaSet → Pods via the real DrillDown method.
	drillRS, ok := rsPlugin.(plugin.DrillDowner)
	if !ok {
		t.Fatal("replicasets plugin must implement DrillDowner")
	}
	podPlugin, pods := drillRS.DrillDown(cl, replicaSets[0])
	if len(pods) != 2 {
		t.Fatalf("expected ReplicaSet drill-down to yield 2 Pods (replicas=2), got %d", len(pods))
	}
	if podPlugin == nil || podPlugin.Name() != "pods" {
		got := "<nil>"
		if podPlugin != nil {
			got = podPlugin.Name()
		}
		t.Fatalf("expected drill-down child plugin 'pods', got %q", got)
	}
}

// TestManifestEnterDetail_DrillsThroughStore drives the REAL app drill path —
// executeCommand("enter-detail") — rather than calling plugin.DrillDown directly.
// It exercises the DrillDowner type assertion, clusterFor(focused) for the pinned
// pane, and PushNav: entering the focused Deployment navigates the pane to its
// ReplicaSet, and entering again navigates to its 2 Pods. This guards the
// populate-from-store path that finding #1's pinned-switch relies on.
func TestManifestEnterDetail_DrillsThroughStore(t *testing.T) {
	app, _ := newManifestDeploymentApp(t, deploymentManifest)

	focused := app.layout.FocusedSplit()
	if focused.Selected() == nil || focused.Selected().GetName() != "web" {
		t.Fatalf("precondition: expected deployments pane showing 'web'")
	}

	// Deployment → ReplicaSet via the real Enter dispatch.
	model, _ := app.executeCommand("enter-detail")
	app = model.(App)
	focused = app.layout.FocusedSplit()
	if got := focused.Plugin().Name(); got != "replicasets" {
		t.Fatalf("expected pane to navigate to replicasets, got %q", got)
	}
	if focused.Len() == 0 {
		t.Fatalf("expected non-empty replicasets after drill-down, got %d", focused.Len())
	}
	if focused.Selected() == nil {
		t.Fatal("expected a selected ReplicaSet after drill-down")
	}

	// ReplicaSet → Pods via another Enter dispatch.
	model, _ = app.executeCommand("enter-detail")
	app = model.(App)
	focused = app.layout.FocusedSplit()
	if got := focused.Plugin().Name(); got != "pods" {
		t.Fatalf("expected pane to navigate to pods, got %q", got)
	}
	if pods := focused.Len(); pods != 2 {
		t.Fatalf("expected 2 pods after drill-down (replicas=2), got %d", pods)
	}
}

// TestManifestSwitchPaneToPinned_PopulatesFromStore proves that switching a pane
// TO the pinned "manifests" context (after it was on some other context) does NOT
// dial/mark the pane offline, but instead populates it directly from the static
// store on All Namespaces. This is the core of finding #1: a pinned target must
// skip the async Dial path entirely.
func TestManifestSwitchPaneToPinned_PopulatesFromStore(t *testing.T) {
	app, _ := newManifestDeploymentApp(t, deploymentManifest)

	// Move the focused pane onto a different (live-looking) context first so the
	// switch back to "manifests" is a genuine cross-context switch, not a no-op.
	focused := app.layout.FocusedSplit()
	focused.SetContext("other")
	focused.SetObjects(nil)
	focused.SetOffline(true)

	model, cmd := app.handlePaneContextSwitch(manifestCtx)
	got := model.(App)

	// Pinned switch must not dispatch an async connect command.
	if cmd != nil {
		t.Fatalf("expected nil cmd from pinned context switch (no dial), got %v", cmd)
	}

	f := got.layout.FocusedSplit()
	if f.Context() != manifestCtx {
		t.Fatalf("expected pane on %q, got %q", manifestCtx, f.Context())
	}
	if f.Offline() {
		t.Fatal("expected pane NOT marked offline after pinned switch")
	}
	if f.Namespace() != "" {
		t.Fatalf("expected pane on All Namespaces (\"\") after pinned switch, got %q", f.Namespace())
	}
	if sel := f.Selected(); sel == nil || sel.GetName() != "web" {
		got := "<nil>"
		if sel != nil {
			got = sel.GetName()
		}
		t.Fatalf("expected pane populated with 'web' from the store, got %q", got)
	}
}

// TestManifestScale_BlockedOffline proves that invoking scale on the
// not-Connected() manifest cluster emits the offline "not available in manifest
// mode" toast and does NOT open the scale overlay (so no real patch path is reached).
func TestManifestScale_BlockedOffline(t *testing.T) {
	app, _ := newManifestDeploymentApp(t, deploymentManifest)
	prevOverlay := app.activeOverlay

	if app.clusterForFocused().Connected() {
		t.Fatal("precondition: manifest cluster must report Connected()==false")
	}

	model, cmd := app.executeCommand("scale")
	got := model.(App)

	if cmd != nil {
		t.Fatalf("expected nil cmd from guarded scale, got %v", cmd)
	}
	if !hasNotifyLevel(got, notify.LevelError) {
		t.Fatalf("expected an error notification when scaling in manifest mode, got %+v", got.notify.List())
	}
	if !notifyContains(got, "not available in manifest mode") {
		t.Fatalf("expected toast mentioning manifest mode, got %+v", got.notify.List())
	}
	if got.activeOverlay != prevOverlay {
		t.Fatalf("expected activeOverlay unchanged (%v), got %v", prevOverlay, got.activeOverlay)
	}
}

// TestManifestRolloutRestart_BlockedOffline proves that invoking rollout-restart
// on the not-Connected() manifest cluster emits the offline "not available in
// manifest mode" toast and does NOT open an overlay.
func TestManifestRolloutRestart_BlockedOffline(t *testing.T) {
	app, _ := newManifestDeploymentApp(t, deploymentManifest)
	prevOverlay := app.activeOverlay

	model, cmd := app.executeCommand("rollout-restart")
	got := model.(App)

	if cmd != nil {
		t.Fatalf("expected nil cmd from guarded rollout-restart, got %v", cmd)
	}
	if !hasNotifyLevel(got, notify.LevelError) {
		t.Fatalf("expected an error notification, got %+v", got.notify.List())
	}
	if !notifyContains(got, "not available in manifest mode") {
		t.Fatalf("expected toast mentioning manifest mode, got %+v", got.notify.List())
	}
	if got.activeOverlay != prevOverlay {
		t.Fatalf("expected activeOverlay unchanged (%v), got %v", prevOverlay, got.activeOverlay)
	}
}

// TestManifestEdit_BlockedOffline proves that invoking edit on the
// not-Connected() manifest cluster emits the offline "not available in manifest
// mode" toast and does NOT open an overlay.
func TestManifestEdit_BlockedOffline(t *testing.T) {
	app, _ := newManifestDeploymentApp(t, deploymentManifest)
	prevOverlay := app.activeOverlay

	model, cmd := app.executeCommand("edit")
	got := model.(App)

	if cmd != nil {
		t.Fatalf("expected nil cmd from guarded edit, got %v", cmd)
	}
	if !hasNotifyLevel(got, notify.LevelError) {
		t.Fatalf("expected an error notification, got %+v", got.notify.List())
	}
	if !notifyContains(got, "not available in manifest mode") {
		t.Fatalf("expected toast mentioning manifest mode, got %+v", got.notify.List())
	}
	if got.activeOverlay != prevOverlay {
		t.Fatalf("expected activeOverlay unchanged (%v), got %v", prevOverlay, got.activeOverlay)
	}
}

// TestManifestDelete_BlockedOffline proves that executeDelete on the
// not-Connected() manifest cluster emits a "not available in manifest mode"
// toast instead of silently no-oping.
func TestManifestDelete_BlockedOffline(t *testing.T) {
	app, _ := newManifestDeploymentApp(t, deploymentManifest)

	focused := app.layout.FocusedSplit()
	if focused == nil {
		t.Fatal("precondition: expected a focused split")
	}
	target := focused.Selected()
	if target == nil {
		t.Fatal("precondition: expected a selected object to delete")
	}

	model, cmd := app.executeDelete([]*unstructured.Unstructured{target}, false)
	got := model.(App)

	if cmd != nil {
		t.Fatalf("expected nil cmd from guarded delete, got %v", cmd)
	}
	if !hasNotifyLevel(got, notify.LevelError) {
		t.Fatalf("expected an error toast when deleting in manifest mode, got %+v", got.notify.List())
	}
	if !notifyContains(got, "not available in manifest mode") {
		t.Fatalf("expected toast mentioning manifest mode, got %+v", got.notify.List())
	}
}

// TestManifestLogs_BlockedOffline proves that startLogStream on the
// not-Connected() manifest cluster emits a "not available in manifest mode"
// toast instead of silently no-oping.
func TestManifestLogs_BlockedOffline(t *testing.T) {
	app, _ := newManifestDeploymentApp(t, deploymentManifest)

	got, cmd := app.startLogStream("web-pod", "c", "foo", k8s.LogOptions{})

	if cmd != nil {
		t.Fatalf("expected nil cmd from guarded log stream, got %v", cmd)
	}
	if !hasNotifyLevel(got, notify.LevelError) {
		t.Fatalf("expected an error toast when streaming logs in manifest mode, got %+v", got.notify.List())
	}
	if !notifyContains(got, "not available in manifest mode") {
		t.Fatalf("expected toast mentioning manifest mode, got %+v", got.notify.List())
	}
}

// TestManifestAllNamespaces_NonDefaultVisible proves the loader's dual-keying
// plus the pinned default ns="" surfaces a Deployment that lives in a
// non-default namespace ("foo") when listing at "All Namespaces" through the
// store the focused pane reads from.
func TestManifestAllNamespaces_NonDefaultVisible(t *testing.T) {
	app, _ := newManifestDeploymentApp(t, deploymentManifest)

	store := app.clusterForFocused().Store()
	all := store.List(deploymentsGVRForTest, "")
	if len(all) != 1 {
		t.Fatalf("expected 1 deployment in All Namespaces bucket, got %d", len(all))
	}
	if got := all[0].GetNamespace(); got != "foo" {
		t.Fatalf("expected the deployment to live in namespace 'foo', got %q", got)
	}
	if got := all[0].GetName(); got != "web" {
		t.Fatalf("expected deployment 'web', got %q", got)
	}

	// It must NOT be visible to a query scoped to "default" (proving the dual-key
	// is the only reason All Namespaces sees it, not a misplaced namespace).
	if def := store.List(deploymentsGVRForTest, "default"); len(def) != 0 {
		t.Fatalf("expected no deployments in 'default', got %d", len(def))
	}

	// And the focused pane (opened at ns="") shows it.
	if sel := app.layout.FocusedSplit().Selected(); sel == nil || sel.GetName() != "web" {
		got := "<nil>"
		if sel != nil {
			got = sel.GetName()
		}
		t.Fatalf("expected focused All-Namespaces pane to show 'web', got %q", got)
	}
}

// TestManifestStartup_PaneAutoPopulated proves that New() populates the initial
// pinned pane synchronously from the static store, WITHOUT the manual SetObjects
// the other tests' harness performs. The pinned store has no informers, so the
// async ResourceUpdatedMsg that fills live panes never fires; New() must seed the
// pane from Subscribe's return instead. (Regression: piped startup opened an
// empty All-Namespaces view.)
func TestManifestStartup_PaneAutoPopulated(t *testing.T) {
	km := config.DefaultKeymap()
	cfg := config.DefaultConfig()
	plugin.Reset()
	t.Cleanup(plugin.Reset)

	depPlugin := akudeployments.New()
	plugin.Register(depPlugin)
	plugin.Register(akureplicasets.New())
	plugin.Register(akupods.New())

	cl, _, err := manifest.Load(strings.NewReader(deploymentManifest), "default")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	mgr := cluster.NewManager(nil, "", 0)
	mgr.RegisterPinned(cl)

	specs := []ResourceSpec{{Plugin: depPlugin, Namespace: ""}}
	app := New(mgr, km, cfg, nil, nil, nil, specs, nil, layout.OrientationVertical, manifestCtx)

	// No manual SetObjects here — New() must have populated the pane on its own.
	if sel := app.layout.FocusedSplit().Selected(); sel == nil || sel.GetName() != "web" {
		got := "<nil>"
		if sel != nil {
			got = sel.GetName()
		}
		t.Fatalf("startup pinned pane should auto-populate 'web', got %q", got)
	}
}

// TestManifestNamespaceRoundTrip_PreservesAllNamespaces proves the All-Namespaces
// bucket survives a namespace switch away and back. Switching "" → "foo" → ""
// previously tore down the "" bucket via unsubscribeIfUnused → Store.Unsubscribe,
// and the client-less static store could never rebuild it, so the pane stayed
// empty forever. The static-store teardown guard prevents that.
func TestManifestNamespaceRoundTrip_PreservesAllNamespaces(t *testing.T) {
	app, _ := newManifestDeploymentApp(t, deploymentManifest)

	// Precondition: All Namespaces shows the deployment.
	if sel := app.layout.FocusedSplit().Selected(); sel == nil || sel.GetName() != "web" {
		t.Fatalf("precondition: All Namespaces should show 'web'")
	}

	// Switch to the deployment's own namespace.
	m, _ := app.handleNamespaceSwitch("foo")
	app = m.(App)
	if sel := app.layout.FocusedSplit().Selected(); sel == nil || sel.GetName() != "web" {
		t.Fatalf("ns 'foo' should show 'web'")
	}

	// Switch back to All Namespaces — the "" bucket must still be intact.
	m, _ = app.handleNamespaceSwitch("")
	app = m.(App)
	if sel := app.layout.FocusedSplit().Selected(); sel == nil || sel.GetName() != "web" {
		got := "<nil>"
		if sel != nil {
			got = sel.GetName()
		}
		t.Fatalf("switching back to All Namespaces must still show 'web', got %q", got)
	}
}
