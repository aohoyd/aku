package resourcequotas

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestResourceQuotaPluginColumns(t *testing.T) {
	p := New(nil, nil)
	cols := p.Columns()
	if len(cols) != 2 {
		t.Fatalf("expected 2 columns, got %d", len(cols))
	}
}

func TestResourceQuotaPluginRow(t *testing.T) {
	p := New(nil, nil)
	obj := makeResourceQuota("my-quota")
	row := p.Row(obj)
	if row[0] != "my-quota" {
		t.Fatalf("expected 'my-quota', got '%s'", row[0])
	}
}

func TestResourceQuotaPluginDescribe(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "ResourceQuota",
			"metadata": map[string]any{
				"name":              "my-quota",
				"namespace":         "default",
				"creationTimestamp": "2026-02-24T10:00:00Z",
				"labels":            map[string]any{"app": "nginx"},
			},
			"status": map[string]any{
				"hard": map[string]any{
					"cpu":    "1",
					"memory": "1Gi",
				},
				"used": map[string]any{
					"cpu":    "500m",
					"memory": "256Mi",
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
		"my-quota",
		"default",
		"app=nginx",
		"Resource:",
		"cpu:",
		"500m / 1",
		"memory:",
		"256Mi / 1Gi",
	}
	for _, want := range checks {
		if !strings.Contains(c.Raw, want) {
			t.Errorf("describe output should contain %q\n\nFull output:\n%s", want, c.Raw)
		}
	}
}

func makeResourceQuota(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "ResourceQuota",
			"metadata": map[string]any{
				"name":              name,
				"namespace":         "default",
				"creationTimestamp": "2024-01-01T00:00:00Z",
			},
		},
	}
}
