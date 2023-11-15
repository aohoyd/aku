package priorityclasses

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestPriorityClassPluginColumns(t *testing.T) {
	p := New(nil, nil)
	cols := p.Columns()
	if len(cols) != 4 {
		t.Fatalf("expected 4 columns, got %d", len(cols))
	}
}

func TestPriorityClassPluginRow(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "scheduling.k8s.io/v1",
			"kind":       "PriorityClass",
			"metadata": map[string]any{
				"name":              "high-priority",
				"creationTimestamp": "2024-01-01T00:00:00Z",
			},
			"value":         int64(1000000),
			"globalDefault": false,
		},
	}
	row := p.Row(obj)

	if row[0] != "high-priority" {
		t.Fatalf("expected name 'high-priority', got '%s'", row[0])
	}
	if row[1] != "1000000" {
		t.Fatalf("expected value '1000000', got '%s'", row[1])
	}
	if row[2] != "false" {
		t.Fatalf("expected globalDefault 'false', got '%s'", row[2])
	}
}

func TestPriorityClassPluginDescribe(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "scheduling.k8s.io/v1",
			"kind":       "PriorityClass",
			"metadata": map[string]any{
				"name":              "system-cluster-critical",
				"creationTimestamp": "2026-02-24T10:00:00Z",
				"labels":            map[string]any{"app": "system"},
			},
			"value":            int64(2000000000),
			"globalDefault":    false,
			"preemptionPolicy": "PreemptLowerPriority",
			"description":      "Used for system critical pods",
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
		"system-cluster-critical",
		"2000000000",
		"false",
		"PreemptLowerPriority",
	}
	for _, want := range checks {
		if !strings.Contains(c.Raw, want) {
			t.Errorf("describe output should contain %q\n\nFull output:\n%s", want, c.Raw)
		}
	}
}
