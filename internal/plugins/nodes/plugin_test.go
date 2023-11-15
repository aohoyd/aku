package nodes

import (
	"context"
	"strings"
	"testing"

	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/render"
	"github.com/aohoyd/aku/internal/plugins/workload"
	"github.com/charmbracelet/x/ansi"
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
func (m *mockPlugin) IsClusterScoped() bool                                      { return false }
func (m *mockPlugin) Columns() []plugin.Column                                   { return nil }
func (m *mockPlugin) Row(_ *unstructured.Unstructured) []string                  { return nil }
func (m *mockPlugin) YAML(_ *unstructured.Unstructured) (render.Content, error) {
	return render.Content{}, nil
}
func (m *mockPlugin) Describe(_ context.Context, _ *unstructured.Unstructured) (render.Content, error) {
	return render.Content{}, nil
}

func TestNodePluginColumns(t *testing.T) {
	p := New(nil, nil)
	cols := p.Columns()
	if len(cols) != 5 {
		t.Fatalf("expected 5 columns, got %d", len(cols))
	}
}

func TestNodePluginRow(t *testing.T) {
	p := New(nil, nil)
	obj := makeNode("node-1", "True", "v1.28.3")
	row := p.Row(obj)
	if row[0] != "node-1" {
		t.Fatalf("expected 'node-1', got '%s'", row[0])
	}
	if row[1] != "Ready" {
		t.Fatalf("expected 'Ready', got '%s'", row[1])
	}
	if row[3] != "v1.28.3" {
		t.Fatalf("expected 'v1.28.3', got '%s'", row[3])
	}
}

func TestNodePluginRowRoles(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Node",
			"metadata": map[string]any{
				"name":              "master-1",
				"creationTimestamp": "2024-01-01T00:00:00Z",
				"labels": map[string]any{
					"node-role.kubernetes.io/control-plane": "",
					"node-role.kubernetes.io/master":        "",
				},
			},
			"status": map[string]any{
				"conditions": []any{
					map[string]any{"type": "Ready", "status": "True"},
				},
				"nodeInfo": map[string]any{
					"kubeletVersion": "v1.28.3",
				},
			},
		},
	}
	row := p.Row(obj)
	roles := row[2]
	if !strings.Contains(roles, "control-plane") {
		t.Fatalf("expected roles to contain 'control-plane', got '%s'", roles)
	}
	if !strings.Contains(roles, "master") {
		t.Fatalf("expected roles to contain 'master', got '%s'", roles)
	}
}

func TestNodePluginDescribe(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Node",
			"metadata": map[string]any{
				"name":              "node-1",
				"creationTimestamp": "2024-01-01T00:00:00Z",
				"labels":           map[string]any{"beta.kubernetes.io/os": "linux"},
			},
			"status": map[string]any{
				"conditions": []any{
					map[string]any{"type": "Ready", "status": "True", "reason": "KubeletReady", "message": "kubelet is posting ready status"},
				},
				"addresses": []any{
					map[string]any{"type": "InternalIP", "address": "10.0.0.1"},
				},
				"capacity": map[string]any{
					"cpu":    "4",
					"memory": "8Gi",
				},
				"allocatable": map[string]any{
					"cpu":    "3800m",
					"memory": "7Gi",
				},
				"nodeInfo": map[string]any{
					"operatingSystem":         "linux",
					"architecture":            "amd64",
					"kernelVersion":           "5.15.0",
					"containerRuntimeVersion": "containerd://1.6.0",
					"kubeletVersion":          "v1.28.3",
				},
			},
			"spec": map[string]any{
				"taints": []any{
					map[string]any{"key": "node-role.kubernetes.io/master", "effect": "NoSchedule"},
				},
			},
		},
	}

	c, err := p.Describe(t.Context(), obj)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Display == "" {
		t.Fatal("display output should not be empty")
	}
	if stripped := ansi.Strip(c.Display); stripped != c.Raw {
		t.Errorf("strip invariant violated: ansi.Strip(c.Display) != raw\nstripped: %q\nraw:      %q", stripped, c.Raw)
	}

	checks := []string{
		"node-1",
		"Ready", "True", "KubeletReady",
		"10.0.0.1",
		"cpu", "memory",
		"linux", "amd64", "5.15.0",
		"v1.28.3",
		"NoSchedule",
	}
	for _, want := range checks {
		if !strings.Contains(c.Raw, want) {
			t.Errorf("describe output should contain %q\n\nFull output:\n%s", want, c.Raw)
		}
	}
}

func TestNodeDrillDown(t *testing.T) {
	store := k8s.NewStore(nil, nil)

	plugin.Reset()
	mockPods := &mockPlugin{name: "pods"}
	plugin.Register(mockPods)

	pod1 := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{
			"name": "nginx-pod-1", "namespace": "default",
		},
		"spec": map[string]any{
			"nodeName": "node-1",
		},
	}}
	store.CacheUpsert(workload.PodsGVR, "", pod1)

	p := &Plugin{
		store: store,
	}
	node := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{
			"name": "node-1",
		},
	}}

	childPlugin, children := p.DrillDown(node)
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

func TestNodeDrillDownNilStore(t *testing.T) {
	p := &Plugin{
		store: nil,
	}
	node := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{"name": "node-1"},
	}}
	childPlugin, children := p.DrillDown(node)
	if childPlugin != nil || children != nil {
		t.Fatal("expected nil, nil for nil store")
	}
}

func makeNode(name, readyStatus, version string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Node",
			"metadata": map[string]any{
				"name":              name,
				"creationTimestamp": "2024-01-01T00:00:00Z",
			},
			"status": map[string]any{
				"conditions": []any{
					map[string]any{"type": "Ready", "status": readyStatus},
				},
				"nodeInfo": map[string]any{
					"kubeletVersion": version,
				},
			},
		},
	}
}
