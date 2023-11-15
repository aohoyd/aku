package jobs

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

func TestJobPluginColumns(t *testing.T) {
	p := New(nil, nil)
	cols := p.Columns()
	if len(cols) != 4 {
		t.Fatalf("expected 4 columns, got %d", len(cols))
	}
}

func TestJobPluginRow(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "batch/v1",
			"kind":       "Job",
			"metadata": map[string]any{
				"name":              "my-job",
				"namespace":         "default",
				"creationTimestamp": "2024-01-01T00:00:00Z",
			},
			"spec": map[string]any{
				"completions": int64(3),
			},
			"status": map[string]any{
				"succeeded": int64(1),
				"startTime": "2024-01-01T00:00:00Z",
			},
		},
	}
	row := p.Row(obj)
	if row[0] != "my-job" {
		t.Fatalf("expected 'my-job', got '%s'", row[0])
	}
	if row[1] != "1/3" {
		t.Fatalf("expected completions '1/3', got '%s'", row[1])
	}
}

func TestJobPluginDescribe(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "batch/v1",
			"kind":       "Job",
			"metadata": map[string]any{
				"name":              "my-job",
				"namespace":         "default",
				"creationTimestamp": "2026-02-24T10:00:00Z",
				"labels":           map[string]any{"app": "batch"},
			},
			"spec": map[string]any{
				"completions":  int64(3),
				"parallelism":  int64(2),
				"backoffLimit": int64(6),
				"selector": map[string]any{
					"matchLabels": map[string]any{"job-name": "my-job"},
				},
				"template": map[string]any{
					"spec": map[string]any{
						"containers": []any{
							map[string]any{
								"name":  "worker",
								"image": "busybox:latest",
							},
						},
					},
				},
			},
			"status": map[string]any{
				"succeeded": int64(1),
				"active":    int64(2),
				"failed":    int64(0),
				"conditions": []any{
					map[string]any{"type": "Complete", "status": "False"},
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
		"my-job", "default",
		"Parallelism:", "2",
		"Completions:", "3",
		"BackoffLimit:", "6",
		"Active:", "2",
		"Succeeded:", "1",
		"Failed:", "0",
		"Pod Template:", "worker:", "busybox:latest",
		"Conditions:", "Complete", "False",
	}
	for _, want := range checks {
		if !strings.Contains(c.Raw, want) {
			t.Errorf("describe output should contain %q\n\nFull output:\n%s", want, c.Raw)
		}
	}
}

func TestJobDrillDown(t *testing.T) {
	podsGVR := schema.GroupVersionResource{Version: "v1", Resource: "pods"}
	store := k8s.NewStore(nil, nil)

	plugin.Reset()
	mockPods := &mockPlugin{name: "pods"}
	plugin.Register(mockPods)

	pod := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{
			"name": "my-job-pod-1", "namespace": "default",
			"ownerReferences": []any{map[string]any{"uid": "job-uid-1"}},
		},
	}}
	store.CacheUpsert(podsGVR, "default", pod)

	p := &Plugin{store: store}
	job := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{
			"name": "my-job", "namespace": "default", "uid": "job-uid-1",
		},
	}}

	childPlugin, children := p.DrillDown(job)
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

func TestJobDrillDownNilStore(t *testing.T) {
	p := &Plugin{store: nil}
	job := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{"name": "my-job", "namespace": "default", "uid": "job-uid-1"},
	}}
	childPlugin, children := p.DrillDown(job)
	if childPlugin != nil || children != nil {
		t.Fatal("expected nil, nil for nil store")
	}
}
