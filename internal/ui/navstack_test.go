package ui

import (
	"context"
	"testing"

	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/render"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// stubPlugin is a minimal ResourcePlugin for nav stack tests.
type stubPlugin struct{ name string }

func (s *stubPlugin) Name() string      { return s.name }
func (s *stubPlugin) ShortName() string { return s.name[:2] }
func (s *stubPlugin) GVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{Resource: s.name}
}
func (s *stubPlugin) IsClusterScoped() bool                     { return false }
func (s *stubPlugin) Columns() []plugin.Column                  { return nil }
func (s *stubPlugin) Row(_ *unstructured.Unstructured) []string { return nil }
func (s *stubPlugin) YAML(_ *unstructured.Unstructured) (render.Content, error) {
	return render.Content{}, nil
}
func (s *stubPlugin) Describe(_ context.Context, _ *unstructured.Unstructured) (render.Content, error) {
	return render.Content{}, nil
}

func TestNavStackPushPop(t *testing.T) {
	var s NavStack
	if s.Depth() != 0 {
		t.Fatal("empty stack should have depth 0")
	}
	_, ok := s.Pop()
	if ok {
		t.Fatal("pop on empty stack should return false")
	}

	p1 := &stubPlugin{name: "pods"}
	objs := []*unstructured.Unstructured{
		{Object: map[string]any{"metadata": map[string]any{"name": "pod-1"}}},
	}
	s.Push(NavSnapshot{
		Plugin:    p1,
		Namespace: "default",
		Objects:   objs,
		Cursor:    3,
		SortState: DefaultSortState(),
	})
	if s.Depth() != 1 {
		t.Fatalf("expected depth 1, got %d", s.Depth())
	}

	snap, ok := s.Pop()
	if !ok {
		t.Fatal("pop should return true")
	}
	if snap.Plugin.Name() != "pods" {
		t.Fatalf("expected plugin 'pods', got %q", snap.Plugin.Name())
	}
	if snap.Cursor != 3 {
		t.Fatalf("expected cursor 3, got %d", snap.Cursor)
	}
	if snap.Namespace != "default" {
		t.Fatalf("expected namespace 'default', got %q", snap.Namespace)
	}
	if s.Depth() != 0 {
		t.Fatal("stack should be empty after pop")
	}
}

func TestNavStackParentFields(t *testing.T) {
	var s NavStack
	s.Push(NavSnapshot{
		Plugin:     &stubPlugin{name: "replicasets"},
		Namespace:  "default",
		Cursor:     1,
		ParentUID:  "deploy-uid-123",
		ParentName: "nginx-deploy",
	})
	snap, ok := s.Pop()
	if !ok {
		t.Fatal("pop should return true")
	}
	if snap.ParentUID != "deploy-uid-123" {
		t.Fatalf("expected ParentUID 'deploy-uid-123', got %q", snap.ParentUID)
	}
	if snap.ParentName != "nginx-deploy" {
		t.Fatalf("expected ParentName 'nginx-deploy', got %q", snap.ParentName)
	}
}

func TestResourceListPushPopParentContext(t *testing.T) {
	deployPlugin := &stubPlugin{name: "deployments"}
	rsPlugin := &stubPlugin{name: "replicasets"}
	podPlugin := &stubPlugin{name: "pods"}

	rl := NewResourceList(deployPlugin, 80, 24)

	// Level 1: drill from deploys into RS filtered by deploy "nginx" (UID: "d-uid")
	rsChildren := []*unstructured.Unstructured{
		{Object: map[string]any{"metadata": map[string]any{"name": "rs-1"}}},
	}
	rl.PushNav(rsPlugin, rsChildren, "nginx", "d-uid", "", "", NavChild)

	if rl.ParentContext() != "de/nginx" {
		t.Fatalf("expected parentContext 'de/nginx', got %q", rl.ParentContext())
	}
	if snap, ok := rl.ParentSnap(); !ok || snap.ParentUID != "d-uid" {
		uid := ""
		if ok {
			uid = snap.ParentUID
		}
		t.Fatalf("expected parentUID 'd-uid', got %q", uid)
	}

	// Level 2: drill from RS into pods filtered by RS "rs-1" (UID: "rs-uid")
	podChildren := []*unstructured.Unstructured{
		{Object: map[string]any{"metadata": map[string]any{"name": "pod-1"}}},
	}
	rl.PushNav(podPlugin, podChildren, "rs-1", "rs-uid", "", "", NavChild)

	if rl.ParentContext() != "re/rs-1" {
		t.Fatalf("expected parentContext 're/rs-1', got %q", rl.ParentContext())
	}
	if snap, ok := rl.ParentSnap(); !ok || snap.ParentUID != "rs-uid" {
		uid := ""
		if ok {
			uid = snap.ParentUID
		}
		t.Fatalf("expected parentUID 'rs-uid', got %q", uid)
	}

	// Pop back to RS — should restore deploy's parent context
	if !rl.PopNav() {
		t.Fatal("PopNav back to RS should return true")
	}
	if rl.ParentContext() != "de/nginx" {
		t.Fatalf("expected parentContext 'de/nginx' after pop, got %q", rl.ParentContext())
	}
	if snap, ok := rl.ParentSnap(); !ok || snap.ParentUID != "d-uid" {
		uid := ""
		if ok {
			uid = snap.ParentUID
		}
		t.Fatalf("expected parentUID 'd-uid' after pop, got %q", uid)
	}
	if rl.Plugin().Name() != "replicasets" {
		t.Fatalf("expected plugin 'replicasets' after pop, got %q", rl.Plugin().Name())
	}

	// Pop back to deploys — should clear parent context
	if !rl.PopNav() {
		t.Fatal("PopNav back to deploys should return true")
	}
	if rl.ParentContext() != "" {
		t.Fatalf("expected empty parentContext at root, got %q", rl.ParentContext())
	}
	if snap, ok := rl.ParentSnap(); ok {
		t.Fatalf("expected no ParentSnap at root, got %+v", snap)
	}
	if rl.Plugin().Name() != "deployments" {
		t.Fatalf("expected plugin 'deployments' at root, got %q", rl.Plugin().Name())
	}
}

type stubClusterPlugin struct {
	stubPlugin
}

func (s *stubClusterPlugin) IsClusterScoped() bool { return true }

func TestEffectiveNamespace(t *testing.T) {
	nsPlugin := &stubPlugin{name: "pods"}
	clusterPlugin := &stubClusterPlugin{stubPlugin: stubPlugin{name: "nodes"}}

	rl := NewResourceList(nsPlugin, 80, 24)

	// Namespaced plugin: EffectiveNamespace matches Namespace
	rl.SetNamespace("myns")
	if got := rl.EffectiveNamespace(); got != "myns" {
		t.Fatalf("expected 'myns', got %q", got)
	}

	// Switch to cluster-scoped plugin: EffectiveNamespace returns ""
	rl.SetPlugin(clusterPlugin)
	if got := rl.EffectiveNamespace(); got != "" {
		t.Fatalf("expected '' for cluster-scoped, got %q", got)
	}
	// But raw Namespace is preserved
	if got := rl.Namespace(); got != "myns" {
		t.Fatalf("expected raw namespace 'myns', got %q", got)
	}

	// Switch back to namespaced plugin: namespace is still there
	rl.SetPlugin(nsPlugin)
	if got := rl.EffectiveNamespace(); got != "myns" {
		t.Fatalf("expected 'myns' after switch back, got %q", got)
	}
}

func TestResetForReload(t *testing.T) {
	deployPlugin := &stubPlugin{name: "deployments"}
	rsPlugin := &stubPlugin{name: "replicasets"}
	podPlugin := &stubPlugin{name: "pods"}

	rl := NewResourceList(deployPlugin, 80, 24)
	rl.SetNamespace("production")

	// Push two drill-down levels
	rsChildren := []*unstructured.Unstructured{
		{Object: map[string]any{"metadata": map[string]any{"name": "rs-1"}}},
	}
	rl.PushNav(rsPlugin, rsChildren, "nginx", "d-uid", "", "", NavChild)

	podChildren := []*unstructured.Unstructured{
		{Object: map[string]any{"metadata": map[string]any{"name": "pod-1"}}},
	}
	rl.PushNav(podPlugin, podChildren, "rs-1", "rs-uid", "", "", NavChild)

	// Verify we're in drill-down
	if !rl.InDrillDown() {
		t.Fatal("expected to be in drill-down")
	}
	if rl.Plugin().Name() != "pods" {
		t.Fatalf("expected plugin 'pods', got %q", rl.Plugin().Name())
	}

	// Reset for reload
	rl.ResetForReload()

	// Should be back at root
	if rl.InDrillDown() {
		t.Fatal("expected nav stack to be empty after ResetForReload")
	}
	if rl.Plugin().Name() != "deployments" {
		t.Fatalf("expected plugin 'deployments', got %q", rl.Plugin().Name())
	}
	if rl.Namespace() != "production" {
		t.Fatalf("expected namespace 'production', got %q", rl.Namespace())
	}
	// Objects should be cleared
	if rl.Len() != 0 {
		t.Fatalf("expected 0 display objects, got %d", rl.Len())
	}
}

func TestResetForReloadNoDrillDown(t *testing.T) {
	podPlugin := &stubPlugin{name: "pods"}
	rl := NewResourceList(podPlugin, 80, 24)
	rl.SetNamespace("default")

	objs := []*unstructured.Unstructured{
		{Object: map[string]any{"metadata": map[string]any{"name": "pod-1"}}},
	}
	rl.SetObjects(objs)

	// Reset with no drill-down active
	rl.ResetForReload()

	// Plugin and namespace preserved
	if rl.Plugin().Name() != "pods" {
		t.Fatalf("expected plugin 'pods', got %q", rl.Plugin().Name())
	}
	if rl.Namespace() != "default" {
		t.Fatalf("expected namespace 'default', got %q", rl.Namespace())
	}
	// Objects should be cleared
	if rl.Len() != 0 {
		t.Fatalf("expected 0 display objects, got %d", rl.Len())
	}
}

func TestNavSnapshotStoresParentKindAndAPIVersion(t *testing.T) {
	var s NavStack
	s.Push(NavSnapshot{
		Plugin:           &stubPlugin{name: "helmmanifest"},
		ParentName:       "my-app",
		ParentUID:        "",
		ParentAPIVersion: "v1",
		ParentKind:       "Secret",
	})
	snap, ok := s.Peek()
	if !ok {
		t.Fatal("expected snapshot")
	}
	if snap.ParentAPIVersion != "v1" {
		t.Fatalf("expected ParentAPIVersion 'v1', got %q", snap.ParentAPIVersion)
	}
	if snap.ParentKind != "Secret" {
		t.Fatalf("expected ParentKind 'Secret', got %q", snap.ParentKind)
	}
}

// TestNavDirectionZeroValueIsChild guards that the NavChild enum value is the
// zero value, preserving the prior default-false = child-ward behavior.
func TestNavDirectionZeroValueIsChild(t *testing.T) {
	var d NavDirection
	if d != NavChild {
		t.Fatalf("expected zero-value NavDirection to be NavChild, got %v", d)
	}
	// A snapshot with no explicit Direction must default to NavChild.
	var s NavStack
	s.Push(NavSnapshot{Plugin: &stubPlugin{name: "pods"}})
	snap, ok := s.Pop()
	if !ok {
		t.Fatal("pop should return true")
	}
	if snap.Direction != NavChild {
		t.Fatalf("expected default Direction NavChild, got %v", snap.Direction)
	}
}

func TestNavStackDirectionPreserved(t *testing.T) {
	cases := []struct {
		name string
		dir  NavDirection
	}{
		{name: "child-ward frame", dir: NavChild},
		{name: "parent-ward frame", dir: NavParent},
		{name: "node-ward frame", dir: NavNode},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var s NavStack
			s.Push(NavSnapshot{
				Plugin:    &stubPlugin{name: "pods"},
				Direction: tc.dir,
			})
			snap, ok := s.Pop()
			if !ok {
				t.Fatal("pop should return true")
			}
			if snap.Direction != tc.dir {
				t.Fatalf("expected Direction %v, got %v", tc.dir, snap.Direction)
			}
		})
	}
}

func TestResourceListPushNavDirectionPreserved(t *testing.T) {
	cases := []struct {
		name string
		dir  NavDirection
	}{
		{name: "child-ward push", dir: NavChild},
		{name: "parent-ward push", dir: NavParent},
		{name: "node-ward push", dir: NavNode},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rl := NewResourceList(&stubPlugin{name: "deployments"}, 80, 24)
			rl.PushNav(&stubPlugin{name: "replicasets"}, nil, "nginx", "d-uid", "", "", tc.dir)
			snap, ok := rl.ParentSnap()
			if !ok {
				t.Fatal("expected ParentSnap after PushNav")
			}
			// PushNav's dir param must thread into the stored snapshot's Direction.
			if snap.Direction != tc.dir {
				t.Fatalf("expected Direction %v, got %v", tc.dir, snap.Direction)
			}
		})
	}
}

// TestNavStackMixedDirectionLIFO models the worked example:
// deployments(root) → push rs (child-ward) → push pods (child-ward)
// → push node (node-ward) → push rs2 (parent-ward) → push deploy2 (parent-ward),
// then pops and asserts LIFO order with each frame's Direction intact.
func TestNavStackMixedDirectionLIFO(t *testing.T) {
	var s NavStack
	pushes := []struct {
		plugin string
		dir    NavDirection
	}{
		{plugin: "replicasets", dir: NavChild},  // enter: deploy → rs
		{plugin: "pods", dir: NavChild},         // enter: rs → pods
		{plugin: "nodes", dir: NavNode},         // gN:    pods → node
		{plugin: "replicasets", dir: NavParent}, // bspc: pods → rs2
		{plugin: "deployments", dir: NavParent}, // bspc: rs2 → deploy2
	}
	for _, p := range pushes {
		s.Push(NavSnapshot{Plugin: &stubPlugin{name: p.plugin}, Direction: p.dir})
	}
	if s.Depth() != len(pushes) {
		t.Fatalf("expected depth %d, got %d", len(pushes), s.Depth())
	}

	// Pop in reverse (LIFO) order, asserting plugin + Direction each frame.
	for i := len(pushes) - 1; i >= 0; i-- {
		snap, ok := s.Pop()
		if !ok {
			t.Fatalf("pop %d should return true", i)
		}
		if snap.Plugin.Name() != pushes[i].plugin {
			t.Fatalf("pop %d: expected plugin %q, got %q", i, pushes[i].plugin, snap.Plugin.Name())
		}
		if snap.Direction != pushes[i].dir {
			t.Fatalf("pop %d: expected Direction %v, got %v", i, pushes[i].dir, snap.Direction)
		}
	}
	if s.Depth() != 0 {
		t.Fatalf("expected empty stack after popping all, got depth %d", s.Depth())
	}
}

// TestResourceListParentWardPushNavPopNavRoundTrip exercises a full ResourceList
// round trip across a parent-ward frame: PushNav installs the parent view (and
// InDrillDown becomes true), PopNav restores the original plugin/objects/state
// and InDrillDown returns to false. The Direction lives only on the saved
// frame, so PopNav (which discards the frame) leaves no residue.
func TestResourceListParentWardPushNavPopNavRoundTrip(t *testing.T) {
	rootPlugin := &stubPlugin{name: "pods"}
	rl := NewResourceList(rootPlugin, 80, 24)
	rootObjs := []*unstructured.Unstructured{
		{Object: map[string]any{"metadata": map[string]any{"name": "pod-1"}}},
		{Object: map[string]any{"metadata": map[string]any{"name": "pod-2"}}},
	}
	rl.SetObjects(rootObjs)

	if rl.InDrillDown() {
		t.Fatal("precondition: root view must not report InDrillDown")
	}

	// Parent-ward push: the child (pods) is saved, the parent (replicasets) view
	// becomes current.
	parentPlugin := &stubPlugin{name: "replicasets"}
	parentObjs := []*unstructured.Unstructured{
		{Object: map[string]any{"metadata": map[string]any{"name": "rs-1"}}},
	}
	rl.PushNav(parentPlugin, parentObjs, "pod-1", "pod-uid-1", "v1", "Pod", NavParent)

	if !rl.InDrillDown() {
		t.Fatal("expected InDrillDown after parent-ward PushNav")
	}
	if rl.Plugin().Name() != "replicasets" {
		t.Fatalf("expected parent plugin 'replicasets' current, got %q", rl.Plugin().Name())
	}
	if rl.Len() != 1 {
		t.Fatalf("expected 1 parent object after push, got %d", rl.Len())
	}
	snap, ok := rl.ParentSnap()
	if !ok || snap.Direction != NavParent {
		t.Fatalf("expected a NavParent parent snapshot, ok=%v dir=%v", ok, snap.Direction)
	}

	// PopNav restores the child view.
	if !rl.PopNav() {
		t.Fatal("expected PopNav to return true")
	}
	if rl.InDrillDown() {
		t.Fatal("expected InDrillDown to be false after PopNav back to root")
	}
	if rl.Plugin().Name() != "pods" {
		t.Fatalf("expected restored plugin 'pods', got %q", rl.Plugin().Name())
	}
	if rl.Len() != len(rootObjs) {
		t.Fatalf("expected restored object count %d, got %d", len(rootObjs), rl.Len())
	}
}

func TestNavStackMultiplePushPop(t *testing.T) {
	var s NavStack
	s.Push(NavSnapshot{Plugin: &stubPlugin{name: "pods"}, Cursor: 1})
	s.Push(NavSnapshot{Plugin: &stubPlugin{name: "containers"}, Cursor: 2})
	if s.Depth() != 2 {
		t.Fatalf("expected depth 2, got %d", s.Depth())
	}

	snap, ok := s.Pop()
	if !ok {
		t.Fatal("first pop should return true")
	}
	if snap.Plugin.Name() != "containers" {
		t.Fatal("should pop in LIFO order")
	}
	snap, ok = s.Pop()
	if !ok {
		t.Fatal("second pop should return true")
	}
	if snap.Plugin.Name() != "pods" {
		t.Fatal("second pop should return first pushed")
	}
}
