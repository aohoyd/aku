package replicasets

import (
	"context"
	"testing"

	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/render"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// mockPlugin implements plugin.ResourcePlugin for testing.
type mockPlugin struct {
	name string
}

func (m *mockPlugin) Name() string      { return m.name }
func (m *mockPlugin) ShortName() string { return m.name[:2] }
func (m *mockPlugin) GVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{}
}
func (m *mockPlugin) IsClusterScoped() bool                     { return false }
func (m *mockPlugin) Columns() []plugin.Column                  { return nil }
func (m *mockPlugin) Row(_ *unstructured.Unstructured) []string { return nil }
func (m *mockPlugin) YAML(_ *unstructured.Unstructured) (render.Content, error) {
	return render.Content{}, nil
}
func (m *mockPlugin) Describe(_ context.Context, _ *unstructured.Unstructured) (render.Content, error) {
	return render.Content{}, nil
}

func TestReplicaSetDrillDown(t *testing.T) {
	podsGVR := schema.GroupVersionResource{Version: "v1", Resource: "pods"}
	store := k8s.NewStore(nil, nil)

	plugin.Reset()
	mockPods := &mockPlugin{name: "pods"}
	plugin.Register(mockPods)

	pod := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{
			"name": "pod-1", "namespace": "default",
			"ownerReferences": []any{map[string]any{"uid": "rs-uid-1"}},
		},
	}}
	store.CacheUpsert(podsGVR, "default", pod)

	p := &Plugin{store: store}
	rs := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{
			"name": "rs-1", "namespace": "default", "uid": "rs-uid-1",
		},
	}}

	childPlugin, children := p.DrillDown(rs)
	if childPlugin == nil {
		t.Fatal("expected child plugin, got nil")
	}
	if childPlugin.Name() != "pods" {
		t.Fatalf("expected child plugin 'pods', got %q", childPlugin.Name())
	}
	if len(children) != 1 {
		t.Fatalf("expected 1 child pod, got %d", len(children))
	}
}

func TestReplicaSetDrillDownNilStore(t *testing.T) {
	p := &Plugin{store: nil}
	rs := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{"name": "rs-1", "namespace": "default", "uid": "rs-uid-1"},
	}}
	childPlugin, children := p.DrillDown(rs)
	if childPlugin != nil || children != nil {
		t.Fatal("expected nil, nil for nil store")
	}
}
