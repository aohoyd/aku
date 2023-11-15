package persistentvolumes

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

func TestPVPluginColumns(t *testing.T) {
	p := New(nil, nil)
	cols := p.Columns()
	if len(cols) != 8 {
		t.Fatalf("expected 8 columns, got %d", len(cols))
	}
}

func TestPVPluginRow(t *testing.T) {
	p := New(nil, nil)
	obj := makePV("my-pv", "10Gi", []string{"ReadWriteOnce"}, "Retain", "Bound", "default", "my-pvc", "standard")
	row := p.Row(obj)
	if row[0] != "my-pv" {
		t.Fatalf("expected 'my-pv', got '%s'", row[0])
	}
	if row[1] != "10Gi" {
		t.Fatalf("expected '10Gi', got '%s'", row[1])
	}
	if row[2] != "RWO" {
		t.Fatalf("expected 'RWO', got '%s'", row[2])
	}
	if row[3] != "Retain" {
		t.Fatalf("expected 'Retain', got '%s'", row[3])
	}
	if row[4] != "Bound" {
		t.Fatalf("expected 'Bound', got '%s'", row[4])
	}
	if row[5] != "default/my-pvc" {
		t.Fatalf("expected 'default/my-pvc', got '%s'", row[5])
	}
	if row[6] != "standard" {
		t.Fatalf("expected 'standard', got '%s'", row[6])
	}
}

func TestPVPluginDescribe(t *testing.T) {
	p := New(nil, nil)
	obj := makePV("my-pv", "10Gi", []string{"ReadWriteOnce"}, "Retain", "Bound", "default", "my-pvc", "standard")
	// Add mount options and hostPath for volume source type
	obj.Object["spec"].(map[string]any)["mountOptions"] = []any{"/mnt/data"}
	obj.Object["spec"].(map[string]any)["hostPath"] = map[string]any{
		"path": "/mnt/data",
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
		"my-pv",
		"10Gi",
		"ReadWriteOnce",
		"Retain",
		"Bound",
		"default/my-pvc",
		"standard",
	}
	for _, want := range checks {
		if !strings.Contains(c.Raw, want) {
			t.Errorf("describe output should contain %q\n\nFull output:\n%s", want, c.Raw)
		}
	}
}

func TestPVDrillDown(t *testing.T) {
	store := k8s.NewStore(nil, nil)

	plugin.Reset()
	mockPVC := &mockPlugin{name: "persistentvolumeclaims"}
	plugin.Register(mockPVC)

	pvc1 := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{
			"name": "my-pvc", "namespace": "default",
		},
	}}
	store.CacheUpsert(workload.PVCsGVR, "default", pvc1)

	p := &Plugin{
		store: store,
	}
	pv := makePV("my-pv", "10Gi", []string{"ReadWriteOnce"}, "Retain", "Bound", "default", "my-pvc", "standard")

	childPlugin, children := p.DrillDown(pv)
	if childPlugin == nil {
		t.Fatal("expected child plugin, got nil")
	}
	if childPlugin.Name() != "persistentvolumeclaims" {
		t.Fatalf("expected child plugin 'persistentvolumeclaims', got %q", childPlugin.Name())
	}
	if len(children) != 1 {
		t.Fatalf("expected 1 child PVC, got %d", len(children))
	}
}

func TestPVDrillDownNilStore(t *testing.T) {
	p := &Plugin{
		store: nil,
	}
	pv := makePV("my-pv", "10Gi", []string{"ReadWriteOnce"}, "Retain", "Bound", "default", "my-pvc", "standard")
	childPlugin, children := p.DrillDown(pv)
	if childPlugin != nil || children != nil {
		t.Fatal("expected nil, nil for nil store")
	}
}

func TestPVDrillDownNoClaimRef(t *testing.T) {
	store := k8s.NewStore(nil, nil)

	plugin.Reset()
	mockPVC := &mockPlugin{name: "persistentvolumeclaims"}
	plugin.Register(mockPVC)

	p := &Plugin{
		store: store,
	}
	// PV without claimRef
	pv := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "PersistentVolume",
			"metadata": map[string]any{
				"name":              "my-pv",
				"creationTimestamp": "2024-01-01T00:00:00Z",
			},
			"spec": map[string]any{
				"capacity": map[string]any{
					"storage": "10Gi",
				},
				"accessModes":                   []any{"ReadWriteOnce"},
				"persistentVolumeReclaimPolicy": "Retain",
				"storageClassName":              "standard",
			},
			"status": map[string]any{
				"phase": "Available",
			},
		},
	}
	childPlugin, children := p.DrillDown(pv)
	if childPlugin != nil || children != nil {
		t.Fatal("expected nil, nil for PV without claimRef")
	}
}

func makePV(name, capacity string, accessModes []string, reclaimPolicy, phase, claimNs, claimName, storageClass string) *unstructured.Unstructured {
	modes := make([]any, len(accessModes))
	for i, m := range accessModes {
		modes[i] = m
	}
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "PersistentVolume",
			"metadata": map[string]any{
				"name":              name,
				"creationTimestamp": "2024-01-01T00:00:00Z",
			},
			"spec": map[string]any{
				"capacity": map[string]any{
					"storage": capacity,
				},
				"accessModes":                   modes,
				"persistentVolumeReclaimPolicy": reclaimPolicy,
				"storageClassName":              storageClass,
				"claimRef": map[string]any{
					"namespace": claimNs,
					"name":      claimName,
				},
			},
			"status": map[string]any{
				"phase": phase,
			},
		},
	}
	return obj
}
