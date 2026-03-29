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
	rl.PushNav(rsPlugin, rsChildren, "nginx", "d-uid", "", "")

	if rl.ParentContext() != "de/nginx" {
		t.Fatalf("expected parentContext 'de/nginx', got %q", rl.ParentContext())
	}
	if snap := rl.ParentSnap(); snap == nil || snap.ParentUID != "d-uid" {
		uid := ""
		if snap != nil {
			uid = snap.ParentUID
		}
		t.Fatalf("expected parentUID 'd-uid', got %q", uid)
	}

	// Level 2: drill from RS into pods filtered by RS "rs-1" (UID: "rs-uid")
	podChildren := []*unstructured.Unstructured{
		{Object: map[string]any{"metadata": map[string]any{"name": "pod-1"}}},
	}
	rl.PushNav(podPlugin, podChildren, "rs-1", "rs-uid", "", "")

	if rl.ParentContext() != "re/rs-1" {
		t.Fatalf("expected parentContext 're/rs-1', got %q", rl.ParentContext())
	}
	if snap := rl.ParentSnap(); snap == nil || snap.ParentUID != "rs-uid" {
		uid := ""
		if snap != nil {
			uid = snap.ParentUID
		}
		t.Fatalf("expected parentUID 'rs-uid', got %q", uid)
	}

	// Pop back to RS — should restore deploy's parent context
	rl.PopNav()
	if rl.ParentContext() != "de/nginx" {
		t.Fatalf("expected parentContext 'de/nginx' after pop, got %q", rl.ParentContext())
	}
	if snap := rl.ParentSnap(); snap == nil || snap.ParentUID != "d-uid" {
		uid := ""
		if snap != nil {
			uid = snap.ParentUID
		}
		t.Fatalf("expected parentUID 'd-uid' after pop, got %q", uid)
	}
	if rl.Plugin().Name() != "replicasets" {
		t.Fatalf("expected plugin 'replicasets' after pop, got %q", rl.Plugin().Name())
	}

	// Pop back to deploys — should clear parent context
	rl.PopNav()
	if rl.ParentContext() != "" {
		t.Fatalf("expected empty parentContext at root, got %q", rl.ParentContext())
	}
	if snap := rl.ParentSnap(); snap != nil {
		t.Fatalf("expected nil ParentSnap at root, got %+v", snap)
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
	rl.PushNav(rsPlugin, rsChildren, "nginx", "d-uid", "", "")

	podChildren := []*unstructured.Unstructured{
		{Object: map[string]any{"metadata": map[string]any{"name": "pod-1"}}},
	}
	rl.PushNav(podPlugin, podChildren, "rs-1", "rs-uid", "", "")

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

func TestNavStackMultiplePushPop(t *testing.T) {
	var s NavStack
	s.Push(NavSnapshot{Plugin: &stubPlugin{name: "pods"}, Cursor: 1})
	s.Push(NavSnapshot{Plugin: &stubPlugin{name: "containers"}, Cursor: 2})
	if s.Depth() != 2 {
		t.Fatalf("expected depth 2, got %d", s.Depth())
	}

	snap, _ := s.Pop()
	if snap.Plugin.Name() != "containers" {
		t.Fatal("should pop in LIFO order")
	}
	snap, _ = s.Pop()
	if snap.Plugin.Name() != "pods" {
		t.Fatal("second pop should return first pushed")
	}
}
