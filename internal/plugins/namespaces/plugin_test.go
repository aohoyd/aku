package namespaces

import (
	"strings"
	"testing"

	"github.com/aohoyd/aku/internal/plugin"
	"github.com/charmbracelet/x/ansi"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestPluginName(t *testing.T) {
	p := New(nil, nil)
	if p.Name() != "namespaces" {
		t.Fatalf("expected 'namespaces', got '%s'", p.Name())
	}
	if p.ShortName() != "ns" {
		t.Fatalf("expected 'ns', got '%s'", p.ShortName())
	}
	if p.GVR().Resource != "namespaces" {
		t.Fatalf("expected GVR resource 'namespaces', got '%s'", p.GVR().Resource)
	}
	if !p.IsClusterScoped() {
		t.Fatal("expected namespaces to be cluster-scoped")
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
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]any{
				"name":              "my-ns",
				"creationTimestamp": "2024-01-01T00:00:00Z",
			},
			"status": map[string]any{
				"phase": "Active",
			},
		},
	}
	row := p.Row(obj)
	if row[0] != "my-ns" {
		t.Fatalf("expected 'my-ns', got '%s'", row[0])
	}
	if row[1] != "Active" {
		t.Fatalf("expected status 'Active', got '%s'", row[1])
	}
}

func TestPluginRowDefaultPhase(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]any{
				"name":              "my-ns",
				"creationTimestamp": "2024-01-01T00:00:00Z",
			},
			"status": map[string]any{},
		},
	}
	row := p.Row(obj)
	if row[1] != "Active" {
		t.Fatalf("expected default status 'Active', got '%s'", row[1])
	}
}

func TestPluginDescribeDocument(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"metadata": map[string]any{
				"name":              "my-ns",
				"creationTimestamp": "2026-02-24T10:00:00Z",
				"labels":            map[string]any{"env": "prod"},
			},
			"status": map[string]any{
				"phase": "Active",
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
		"my-ns",
		"env=prod",
		"Active",
	}
	for _, want := range checks {
		if !strings.Contains(c.Raw, want) {
			t.Errorf("describe output should contain %q\n\nFull output:\n%s", want, c.Raw)
		}
	}
}

func TestPluginGoTo(t *testing.T) {
	plugin.Reset()
	// Register a mock pods plugin so GoTo can find it
	plugin.Register(New(nil, nil)) // just need something registered as "pods" won't be found without it

	p := New(nil, nil)
	goToer, ok := p.(plugin.GoToer)
	if !ok {
		t.Fatal("Plugin should implement GoToer")
	}

	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"metadata": map[string]any{
				"name": "kube-system",
			},
		},
	}

	// Without pods plugin registered, GoTo returns false
	plugin.Reset()
	_, _, ok = goToer.GoTo(obj)
	if ok {
		t.Fatal("expected GoTo to return false when pods plugin is not registered")
	}
}
