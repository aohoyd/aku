package daemonsets

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
func (m *mockPlugin) IsClusterScoped() bool                                      { return false }
func (m *mockPlugin) Columns() []plugin.Column                                   { return nil }
func (m *mockPlugin) Row(_ *unstructured.Unstructured) []string                  { return nil }
func (m *mockPlugin) YAML(_ *unstructured.Unstructured) (render.Content, error) {
	return render.Content{}, nil
}
func (m *mockPlugin) Describe(_ context.Context, _ *unstructured.Unstructured) (render.Content, error) {
	return render.Content{}, nil
}

func TestPluginName(t *testing.T) {
	p := New(nil, nil)
	if p.Name() != "daemonsets" {
		t.Fatalf("expected 'daemonsets', got '%s'", p.Name())
	}
	if p.ShortName() != "ds" {
		t.Fatalf("expected 'ds', got '%s'", p.ShortName())
	}
	if p.GVR().Resource != "daemonsets" {
		t.Fatalf("expected GVR resource 'daemonsets', got '%s'", p.GVR().Resource)
	}
}

func TestPluginColumns(t *testing.T) {
	p := New(nil, nil)
	cols := p.Columns()
	if len(cols) != 7 {
		t.Fatalf("expected 7 columns, got %d", len(cols))
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
			"kind":       "DaemonSet",
			"metadata": map[string]any{
				"name":              "my-ds",
				"namespace":         "default",
				"creationTimestamp": "2024-01-01T00:00:00Z",
			},
			"status": map[string]any{
				"desiredNumberScheduled": int64(3),
				"currentNumberScheduled": int64(3),
				"numberReady":            int64(3),
				"updatedNumberScheduled": int64(3),
				"numberAvailable":        int64(3),
			},
		},
	}
	row := p.Row(obj)
	if row[0] != "my-ds" {
		t.Fatalf("expected 'my-ds', got '%s'", row[0])
	}
	if row[1] != "3" {
		t.Fatalf("expected desired '3', got '%s'", row[1])
	}
	if row[2] != "3" {
		t.Fatalf("expected current '3', got '%s'", row[2])
	}
	if row[3] != "3" {
		t.Fatalf("expected ready '3', got '%s'", row[3])
	}
	if row[4] != "3" {
		t.Fatalf("expected up-to-date '3', got '%s'", row[4])
	}
	if row[5] != "3" {
		t.Fatalf("expected available '3', got '%s'", row[5])
	}
}

func TestPluginRowPartial(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "apps/v1",
			"kind":       "DaemonSet",
			"metadata": map[string]any{
				"name":              "web",
				"namespace":         "default",
				"creationTimestamp": "2024-01-01T00:00:00Z",
			},
			"status": map[string]any{
				"desiredNumberScheduled": int64(5),
				"currentNumberScheduled": int64(4),
				"numberReady":            int64(3),
				"updatedNumberScheduled": int64(2),
				"numberAvailable":        int64(3),
			},
		},
	}
	row := p.Row(obj)
	if row[0] != "web" {
		t.Fatalf("expected 'web', got '%s'", row[0])
	}
	if row[1] != "5" {
		t.Fatalf("expected desired '5', got '%s'", row[1])
	}
	if row[3] != "3" {
		t.Fatalf("expected ready '3', got '%s'", row[3])
	}
}

func TestPluginDescribeDocument(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"metadata": map[string]any{
				"name":              "my-ds",
				"namespace":         "default",
				"creationTimestamp": "2026-02-24T10:00:00Z",
				"labels":            map[string]any{"app": "fluentd"},
			},
			"spec": map[string]any{
				"selector": map[string]any{
					"matchLabels": map[string]any{"app": "fluentd"},
				},
				"updateStrategy": map[string]any{
					"type": "RollingUpdate",
					"rollingUpdate": map[string]any{
						"maxUnavailable": 1,
						"maxSurge":       0,
					},
				},
				"template": map[string]any{
					"spec": map[string]any{
						"containers": []any{
							map[string]any{
								"name":  "fluentd",
								"image": "fluentd:v1.16",
							},
						},
					},
				},
			},
			"status": map[string]any{
				"desiredNumberScheduled": int64(3),
				"currentNumberScheduled": int64(3),
				"numberReady":            int64(3),
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
		"my-ds", "default",
		"app=fluentd",
		"RollingUpdate",
		"Pod Template:", "fluentd:", "fluentd:v1.16",
	}
	for _, want := range checks {
		if !strings.Contains(c.Raw, want) {
			t.Errorf("describe output should contain %q\n\nFull output:\n%s", want, c.Raw)
		}
	}
}

func TestDaemonSetDrillDown(t *testing.T) {
	podsGVR := schema.GroupVersionResource{Version: "v1", Resource: "pods"}
	store := k8s.NewStore(nil, nil)

	plugin.Reset()
	mockPods := &mockPlugin{name: "pods"}
	plugin.Register(mockPods)

	pod := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{
			"name": "my-ds-abc", "namespace": "default",
			"ownerReferences": []any{map[string]any{"uid": "ds-uid-1"}},
		},
	}}
	store.CacheUpsert(podsGVR, "default", pod)

	p := &Plugin{store: store}
	ds := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{
			"name": "my-ds", "namespace": "default", "uid": "ds-uid-1",
		},
	}}

	childPlugin, children := p.DrillDown(ds)
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

func TestDaemonSetDrillDownNilStore(t *testing.T) {
	p := &Plugin{store: nil}
	ds := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{"name": "my-ds", "namespace": "default", "uid": "ds-uid-1"},
	}}
	childPlugin, children := p.DrillDown(ds)
	if childPlugin != nil || children != nil {
		t.Fatal("expected nil, nil for nil store")
	}
}
