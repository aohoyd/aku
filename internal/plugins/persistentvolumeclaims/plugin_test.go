package persistentvolumeclaims

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

func TestPVCPluginColumns(t *testing.T) {
	p := New(nil, nil)
	cols := p.Columns()
	if len(cols) != 7 {
		t.Fatalf("expected 7 columns, got %d", len(cols))
	}
}

func TestPVCPluginRow(t *testing.T) {
	p := New(nil, nil)
	obj := makePVC("data-pvc", "Bound", "pv-001", "10Gi", []string{"ReadWriteOnce"}, "standard")
	row := p.Row(obj)
	if row[0] != "data-pvc" {
		t.Fatalf("expected name 'data-pvc', got '%s'", row[0])
	}
	if row[1] != "Bound" {
		t.Fatalf("expected status 'Bound', got '%s'", row[1])
	}
	if row[4] != "RWO" {
		t.Fatalf("expected access modes 'RWO', got '%s'", row[4])
	}
}

func TestPVCPluginDescribe(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"metadata": map[string]any{
				"name":              "data-pvc",
				"namespace":         "default",
				"creationTimestamp": "2026-02-24T10:00:00Z",
				"labels":            map[string]any{"app": "db"},
			},
			"spec": map[string]any{
				"accessModes":      []any{"ReadWriteOnce"},
				"storageClassName": "standard",
				"volumeName":       "pv-001",
				"volumeMode":       "Filesystem",
			},
			"status": map[string]any{
				"phase": "Bound",
				"capacity": map[string]any{
					"storage": "10Gi",
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
		"data-pvc", "default", "Bound", "pv-001", "10Gi",
		"ReadWriteOnce", "standard", "Filesystem",
	}
	for _, want := range checks {
		if !strings.Contains(c.Raw, want) {
			t.Errorf("describe output should contain %q\n\nFull output:\n%s", want, c.Raw)
		}
	}
}

func TestPVCDrillDown(t *testing.T) {
	podsGVR := schema.GroupVersionResource{Version: "v1", Resource: "pods"}
	store := k8s.NewStore(nil, nil)

	plugin.Reset()
	mockPods := &mockPlugin{name: "pods"}
	plugin.Register(mockPods)

	pod := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{
			"name": "web-pod-1", "namespace": "default",
		},
		"spec": map[string]any{
			"volumes": []any{
				map[string]any{
					"name": "data",
					"persistentVolumeClaim": map[string]any{
						"claimName": "data-pvc",
					},
				},
			},
		},
	}}
	store.CacheUpsert(podsGVR, "default", pod)

	p := &Plugin{
		store: store,
	}
	pvc := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{
			"name": "data-pvc", "namespace": "default",
		},
	}}

	childPlugin, children := p.DrillDown(pvc)
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

func TestPVCDrillDownNilStore(t *testing.T) {
	p := &Plugin{
		store: nil,
	}
	pvc := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{"name": "data-pvc", "namespace": "default"},
	}}
	childPlugin, children := p.DrillDown(pvc)
	if childPlugin != nil || children != nil {
		t.Fatal("expected nil, nil for nil store")
	}
}

func makePVC(name, phase, volumeName, capacity string, accessModes []string, storageClass string) *unstructured.Unstructured {
	modes := make([]any, len(accessModes))
	for i, m := range accessModes {
		modes[i] = m
	}
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "PersistentVolumeClaim",
			"metadata": map[string]any{
				"name":              name,
				"namespace":         "default",
				"creationTimestamp": "2024-01-01T00:00:00Z",
			},
			"spec": map[string]any{
				"accessModes":      modes,
				"storageClassName": storageClass,
				"volumeName":       volumeName,
			},
			"status": map[string]any{
				"phase": phase,
				"capacity": map[string]any{
					"storage": capacity,
				},
			},
		},
	}
}
