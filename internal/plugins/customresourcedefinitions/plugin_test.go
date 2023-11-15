package customresourcedefinitions

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestCRDPluginColumns(t *testing.T) {
	p := New(nil, nil)
	cols := p.Columns()
	if len(cols) != 2 {
		t.Fatalf("expected 2 columns, got %d", len(cols))
	}
}

func TestCRDPluginRow(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "apiextensions.k8s.io/v1",
			"kind":       "CustomResourceDefinition",
			"metadata": map[string]any{
				"name":              "widgets.example.com",
				"creationTimestamp": "2024-01-01T00:00:00Z",
			},
		},
	}
	row := p.Row(obj)
	if row[0] != "widgets.example.com" {
		t.Fatalf("expected name 'widgets.example.com', got '%s'", row[0])
	}
	// row[1] is age, just verify it's non-empty
	if row[1] == "" {
		t.Fatal("expected non-empty age")
	}
}

func TestCRDPluginDescribe(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "apiextensions.k8s.io/v1",
			"kind":       "CustomResourceDefinition",
			"metadata": map[string]any{
				"name":              "widgets.example.com",
				"creationTimestamp": "2024-01-01T00:00:00Z",
			},
			"spec": map[string]any{
				"group": "example.com",
				"scope": "Namespaced",
				"versions": []any{
					map[string]any{
						"name":    "v1",
						"served":  true,
						"storage": true,
					},
					map[string]any{
						"name":    "v1beta1",
						"served":  true,
						"storage": false,
					},
				},
				"names": map[string]any{
					"plural":     "widgets",
					"singular":   "widget",
					"kind":       "Widget",
					"shortNames": []any{"wg"},
				},
			},
			"status": map[string]any{
				"conditions": []any{
					map[string]any{
						"type":    "Established",
						"status":  "True",
						"reason":  "InitialNamesAccepted",
						"message": "the initial names have been accepted",
					},
					map[string]any{
						"type":    "NamesAccepted",
						"status":  "True",
						"reason":  "NoConflicts",
						"message": "no conflicts found",
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
		"widgets.example.com",
		"example.com",
		"Namespaced",
		"v1",
		"v1beta1",
		"Widget",
		"widgets",
		"widget",
		"wg",
		"Established",
		"NamesAccepted",
		"True",
		"InitialNamesAccepted",
		"NoConflicts",
	}
	for _, want := range checks {
		if !strings.Contains(c.Raw, want) {
			t.Errorf("describe output should contain %q\n\nFull output:\n%s", want, c.Raw)
		}
	}
}
