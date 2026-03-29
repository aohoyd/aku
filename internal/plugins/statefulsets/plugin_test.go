package statefulsets

import (
	"context"
	"strings"
	"testing"

	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/render"
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
func (m *mockPlugin) IsClusterScoped() bool                     { return false }
func (m *mockPlugin) Columns() []plugin.Column                  { return nil }
func (m *mockPlugin) Row(_ *unstructured.Unstructured) []string { return nil }
func (m *mockPlugin) YAML(_ *unstructured.Unstructured) (render.Content, error) {
	return render.Content{}, nil
}
func (m *mockPlugin) Describe(_ context.Context, _ *unstructured.Unstructured) (render.Content, error) {
	return render.Content{}, nil
}

func TestPluginName(t *testing.T) {
	p := New(nil, nil)
	if p.Name() != "statefulsets" {
		t.Fatalf("expected 'statefulsets', got '%s'", p.Name())
	}
	if p.ShortName() != "sts" {
		t.Fatalf("expected 'sts', got '%s'", p.ShortName())
	}
	if p.GVR().Resource != "statefulsets" {
		t.Fatalf("expected GVR resource 'statefulsets', got '%s'", p.GVR().Resource)
	}
}

func TestPluginColumns(t *testing.T) {
	p := New(nil, nil)
	cols := p.Columns()
	if len(cols) != 3 {
		t.Fatalf("expected 3 columns, got %d", len(cols))
	}
	if cols[0].Title != "NAME" {
		t.Fatalf("expected first column 'NAME', got '%s'", cols[0].Title)
	}
}

func TestPluginRow(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "apps/v1",
			"kind":       "StatefulSet",
			"metadata": map[string]any{
				"name":              "my-sts",
				"namespace":         "default",
				"creationTimestamp": "2024-01-01T00:00:00Z",
			},
			"spec": map[string]any{
				"replicas": int64(3),
			},
			"status": map[string]any{
				"readyReplicas": int64(3),
			},
		},
	}
	row := p.Row(obj)
	if row[0] != "my-sts" {
		t.Fatalf("expected 'my-sts', got '%s'", row[0])
	}
	if row[1] != "3/3" {
		t.Fatalf("expected ready '3/3', got '%s'", row[1])
	}
}

func TestPluginRowPartial(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "apps/v1",
			"kind":       "StatefulSet",
			"metadata": map[string]any{
				"name":              "web",
				"namespace":         "default",
				"creationTimestamp": "2024-01-01T00:00:00Z",
			},
			"spec": map[string]any{
				"replicas": int64(3),
			},
			"status": map[string]any{
				"readyReplicas": int64(1),
			},
		},
	}
	row := p.Row(obj)
	if row[0] != "web" {
		t.Fatalf("expected 'web', got '%s'", row[0])
	}
	if row[1] != "1/3" {
		t.Fatalf("expected ready '1/3', got '%s'", row[1])
	}
}

func TestPluginDescribeDocument(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"metadata": map[string]any{
				"name":              "my-sts",
				"namespace":         "default",
				"creationTimestamp": "2026-02-24T10:00:00Z",
				"labels":            map[string]any{"app": "redis"},
			},
			"spec": map[string]any{
				"replicas": int64(3),
				"selector": map[string]any{
					"matchLabels": map[string]any{"app": "redis"},
				},
				"updateStrategy": map[string]any{
					"type": "RollingUpdate",
					"rollingUpdate": map[string]any{
						"partition": int64(0),
					},
				},
				"template": map[string]any{
					"spec": map[string]any{
						"containers": []any{
							map[string]any{
								"name":  "redis",
								"image": "redis:7.0",
							},
						},
					},
				},
				"volumeClaimTemplates": []any{
					map[string]any{
						"metadata": map[string]any{"name": "data"},
						"spec": map[string]any{
							"accessModes": []any{"ReadWriteOnce"},
							"resources": map[string]any{
								"requests": map[string]any{
									"storage": "10Gi",
								},
							},
						},
					},
				},
			},
			"status": map[string]any{
				"replicas":      int64(3),
				"readyReplicas": int64(3),
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
		"my-sts", "default",
		"app=redis",
		"Replicas:", "3",
		"RollingUpdate",
		"Pod Template:", "redis:", "redis:7.0",
		"Volume Claim Templates:", "data",
		"ReadWriteOnce", "10Gi",
	}
	for _, want := range checks {
		if !strings.Contains(c.Raw, want) {
			t.Errorf("describe output should contain %q\n\nFull output:\n%s", want, c.Raw)
		}
	}
}

func TestStatefulSetDrillDown(t *testing.T) {
	podsGVR := schema.GroupVersionResource{Version: "v1", Resource: "pods"}
	store := k8s.NewStore(nil, nil)

	plugin.Reset()
	mockPods := &mockPlugin{name: "pods"}
	plugin.Register(mockPods)

	pod := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{
			"name": "my-sts-0", "namespace": "default",
			"ownerReferences": []any{map[string]any{"uid": "sts-uid-1"}},
		},
	}}
	store.CacheUpsert(podsGVR, "default", pod)

	p := &Plugin{store: store}
	sts := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{
			"name": "my-sts", "namespace": "default", "uid": "sts-uid-1",
		},
	}}

	childPlugin, children := p.DrillDown(sts)
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

func TestStatefulSetDrillDownNilStore(t *testing.T) {
	p := &Plugin{store: nil}
	sts := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{"name": "my-sts", "namespace": "default", "uid": "sts-uid-1"},
	}}
	childPlugin, children := p.DrillDown(sts)
	if childPlugin != nil || children != nil {
		t.Fatal("expected nil, nil for nil store")
	}
}
