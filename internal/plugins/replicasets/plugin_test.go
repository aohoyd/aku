package replicasets

import (
	"context"
	"testing"

	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/plugin/plugintest"
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

func TestPluginRowHealth(t *testing.T) {
	p := New()
	tests := []struct {
		name   string
		spec   map[string]any
		status map[string]any
		want   plugin.Health
	}{
		{
			name:   "all ready",
			spec:   map[string]any{"replicas": int64(3)},
			status: map[string]any{"readyReplicas": int64(3)},
			want:   plugin.Healthy,
		},
		{
			name:   "short of replicas",
			spec:   map[string]any{"replicas": int64(3)},
			status: map[string]any{"readyReplicas": int64(1)},
			want:   plugin.Warning,
		},
		{
			name:   "scaled to zero",
			spec:   map[string]any{"replicas": int64(0)},
			status: map[string]any{"readyReplicas": int64(0)},
			want:   plugin.Healthy,
		},
		{
			name: "missing status with desired",
			spec: map[string]any{"replicas": int64(2)},
			want: plugin.Warning,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := &unstructured.Unstructured{Object: map[string]any{
				"metadata": map[string]any{"name": "rs", "namespace": "default"},
			}}
			if tt.spec != nil {
				obj.Object["spec"] = tt.spec
			}
			if tt.status != nil {
				obj.Object["status"] = tt.status
			}
			if got := p.(plugin.HealthReporter).RowHealth(obj); got != tt.want {
				t.Fatalf("RowHealth() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReplicaSetDrillDown(t *testing.T) {
	podsGVR := schema.GroupVersionResource{Version: "v1", Resource: "pods"}
	store := k8s.NewStore(nil, "", nil)

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

	p := &Plugin{}
	rs := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{
			"name": "rs-1", "namespace": "default", "uid": "rs-uid-1",
		},
	}}

	childPlugin, children := p.DrillDown(plugintest.NewFakeCluster(store), rs)
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
	p := &Plugin{}
	rs := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{"name": "rs-1", "namespace": "default", "uid": "rs-uid-1"},
	}}
	childPlugin, children := p.DrillDown(plugintest.NewFakeCluster(nil), rs)
	if childPlugin != nil || children != nil {
		t.Fatal("expected nil, nil for nil store")
	}
}
