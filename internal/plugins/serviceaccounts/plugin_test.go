package serviceaccounts

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestServiceAccountPluginColumns(t *testing.T) {
	p := New(nil, nil)
	cols := p.Columns()
	if len(cols) != 3 {
		t.Fatalf("expected 3 columns, got %d", len(cols))
	}
}

func TestServiceAccountPluginRow(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "ServiceAccount",
			"metadata": map[string]any{
				"name":              "default",
				"namespace":         "kube-system",
				"creationTimestamp": "2024-01-01T00:00:00Z",
			},
			"secrets": []any{
				map[string]any{"name": "default-token-abc"},
				map[string]any{"name": "default-token-xyz"},
			},
		},
	}
	row := p.Row(obj)
	if row[0] != "default" {
		t.Fatalf("expected name 'default', got '%s'", row[0])
	}
	if row[1] != "2" {
		t.Fatalf("expected secrets '2', got '%s'", row[1])
	}
}

func TestServiceAccountPluginDescribe(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "ServiceAccount",
			"metadata": map[string]any{
				"name":              "my-sa",
				"namespace":         "default",
				"creationTimestamp": "2026-02-24T10:00:00Z",
				"labels":            map[string]any{"app": "web"},
			},
			"secrets": []any{
				map[string]any{"name": "my-sa-token-abc"},
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

	checks := []string{"my-sa", "default", "app=web", "my-sa-token-abc"}
	for _, want := range checks {
		if !strings.Contains(c.Raw, want) {
			t.Errorf("describe output should contain %q\n\nFull output:\n%s", want, c.Raw)
		}
	}
}
