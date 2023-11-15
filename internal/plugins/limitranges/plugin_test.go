package limitranges

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestLimitRangePluginColumns(t *testing.T) {
	p := New(nil, nil)
	cols := p.Columns()
	if len(cols) != 2 {
		t.Fatalf("expected 2 columns, got %d", len(cols))
	}
	if cols[0].Title != "NAME" {
		t.Fatalf("expected first column 'NAME', got '%s'", cols[0].Title)
	}
	if !cols[0].Flex {
		t.Fatal("expected NAME column to be flex")
	}
	if cols[1].Title != "AGE" {
		t.Fatalf("expected second column 'AGE', got '%s'", cols[1].Title)
	}
	if cols[1].Width != 8 {
		t.Fatalf("expected AGE width 8, got %d", cols[1].Width)
	}
}

func TestLimitRangePluginRow(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "LimitRange",
			"metadata": map[string]any{
				"name":              "my-limits",
				"namespace":         "default",
				"creationTimestamp": "2024-01-01T00:00:00Z",
			},
		},
	}
	row := p.Row(obj)
	if row[0] != "my-limits" {
		t.Fatalf("expected name 'my-limits', got '%s'", row[0])
	}
	if len(row) != 2 {
		t.Fatalf("expected 2 row cells, got %d", len(row))
	}
}

func TestLimitRangePluginDescribe(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "LimitRange",
			"metadata": map[string]any{
				"name":              "mem-cpu-limits",
				"namespace":         "default",
				"creationTimestamp": "2026-02-24T10:00:00Z",
				"labels":            map[string]any{"env": "prod"},
				"annotations":       map[string]any{"note": "test"},
			},
			"spec": map[string]any{
				"limits": []any{
					map[string]any{
						"type": "Container",
						"default": map[string]any{
							"cpu":    "500m",
							"memory": "128Mi",
						},
						"defaultRequest": map[string]any{
							"cpu":    "100m",
							"memory": "64Mi",
						},
						"min": map[string]any{
							"cpu":    "50m",
							"memory": "32Mi",
						},
						"max": map[string]any{
							"cpu":    "1",
							"memory": "256Mi",
						},
					},
					map[string]any{
						"type": "Pod",
						"max": map[string]any{
							"cpu":    "2",
							"memory": "1Gi",
						},
					},
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
		"mem-cpu-limits",
		"default",
		"env=prod",
		"note=test",
		"Type: Container",
		"Default:",
		"cpu",
		"500m",
		"memory",
		"128Mi",
		"Default Request:",
		"100m",
		"64Mi",
		"Min:",
		"50m",
		"32Mi",
		"Max:",
		"256Mi",
		"Type: Pod",
		"1Gi",
	}
	for _, want := range checks {
		if !strings.Contains(c.Raw, want) {
			t.Errorf("describe output should contain %q\n\nFull output:\n%s", want, c.Raw)
		}
	}
}
