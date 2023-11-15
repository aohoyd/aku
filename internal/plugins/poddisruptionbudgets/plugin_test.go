package poddisruptionbudgets

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestPDBPluginColumns(t *testing.T) {
	p := New(nil, nil)
	cols := p.Columns()
	if len(cols) != 5 {
		t.Fatalf("expected 5 columns, got %d", len(cols))
	}
}

func TestPDBPluginRow(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "policy/v1",
			"kind":       "PodDisruptionBudget",
			"metadata": map[string]any{
				"name":              "my-pdb",
				"namespace":         "default",
				"creationTimestamp": "2024-01-01T00:00:00Z",
			},
			"spec": map[string]any{
				"minAvailable": int64(2),
			},
			"status": map[string]any{
				"disruptionsAllowed": int64(1),
			},
		},
	}
	row := p.Row(obj)
	if row[0] != "my-pdb" {
		t.Fatalf("expected name 'my-pdb', got '%s'", row[0])
	}
	if row[1] != "2" {
		t.Fatalf("expected minAvailable '2', got '%s'", row[1])
	}
	if row[3] != "1" {
		t.Fatalf("expected disruptionsAllowed '1', got '%s'", row[3])
	}
}

func TestPDBPluginDescribe(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "policy/v1",
			"kind":       "PodDisruptionBudget",
			"metadata": map[string]any{
				"name":              "my-pdb",
				"namespace":         "default",
				"creationTimestamp": "2026-02-24T10:00:00Z",
				"labels":            map[string]any{"app": "web"},
			},
			"spec": map[string]any{
				"minAvailable": int64(3),
				"selector": map[string]any{
					"matchLabels": map[string]any{
						"app": "web",
					},
				},
			},
			"status": map[string]any{
				"currentHealthy":     int64(3),
				"desiredHealthy":     int64(3),
				"disruptionsAllowed": int64(0),
				"expectedPods":       int64(3),
				"conditions": []any{
					map[string]any{
						"type":   "DisruptionAllowed",
						"status": "True",
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
		"my-pdb",
		"default",
		"app=web",
		"Min Available",
		"Max Unavailable",
		"Selector",
		"Status",
		"Current Healthy",
		"Desired Healthy",
		"Disruptions Allowed",
		"Expected Pods",
		"DisruptionAllowed",
	}
	for _, want := range checks {
		if !strings.Contains(c.Raw, want) {
			t.Errorf("describe output should contain %q\n\nFull output:\n%s", want, c.Raw)
		}
	}
}
