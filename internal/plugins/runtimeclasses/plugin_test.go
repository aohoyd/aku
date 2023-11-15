package runtimeclasses

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestRuntimeClassPluginColumns(t *testing.T) {
	p := New(nil, nil)
	cols := p.Columns()
	if len(cols) != 3 {
		t.Fatalf("expected 3 columns, got %d", len(cols))
	}
}

func TestRuntimeClassPluginRow(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "node.k8s.io/v1",
			"kind":       "RuntimeClass",
			"metadata": map[string]any{
				"name":              "myruntime",
				"creationTimestamp": "2024-01-01T00:00:00Z",
			},
			"handler": "runsc",
		},
	}
	row := p.Row(obj)

	if row[0] != "myruntime" {
		t.Fatalf("expected name 'myruntime', got '%s'", row[0])
	}
	if row[1] != "runsc" {
		t.Fatalf("expected handler 'runsc', got '%s'", row[1])
	}
	// row[2] is age, just check it's non-empty
	if row[2] == "" {
		t.Fatal("expected non-empty age")
	}
}

func TestRuntimeClassPluginDescribe(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "node.k8s.io/v1",
			"kind":       "RuntimeClass",
			"metadata": map[string]any{
				"name":              "gvisor",
				"creationTimestamp": "2026-02-24T10:00:00Z",
				"labels":            map[string]any{"env": "prod"},
				"annotations":       map[string]any{"note": "sandboxed"},
			},
			"handler": "runsc",
			"overhead": map[string]any{
				"podFixed": map[string]any{
					"cpu":    "250m",
					"memory": "120Mi",
				},
			},
			"scheduling": map[string]any{
				"nodeSelector": map[string]any{
					"runtime": "gvisor",
				},
				"tolerations": []any{
					map[string]any{
						"key":      "runtime",
						"operator": "Equal",
						"value":    "gvisor",
						"effect":   "NoSchedule",
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
		"gvisor",
		"runsc",
	}
	for _, want := range checks {
		if !strings.Contains(c.Raw, want) {
			t.Errorf("describe output should contain %q\n\nFull output:\n%s", want, c.Raw)
		}
	}
}
