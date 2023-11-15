package endpoints

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestEndpointsPluginColumns(t *testing.T) {
	p := New(nil, nil)
	cols := p.Columns()
	if len(cols) != 3 {
		t.Fatalf("expected 3 columns, got %d", len(cols))
	}
}

func TestEndpointsPluginRow(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Endpoints",
			"metadata": map[string]any{
				"name":              "my-svc",
				"namespace":         "default",
				"creationTimestamp": "2024-01-01T00:00:00Z",
			},
			"subsets": []any{
				map[string]any{
					"addresses": []any{
						map[string]any{"ip": "10.0.0.1"},
						map[string]any{"ip": "10.0.0.2"},
					},
					"ports": []any{
						map[string]any{"port": int64(80)},
						map[string]any{"port": int64(443)},
					},
				},
			},
		},
	}
	row := p.Row(obj)
	if row[0] != "my-svc" {
		t.Fatalf("expected name 'my-svc', got '%s'", row[0])
	}
	// Each address paired with each port: 10.0.0.1:80,10.0.0.1:443,10.0.0.2:80,10.0.0.2:443
	expected := "10.0.0.1:80,10.0.0.1:443,10.0.0.2:80,10.0.0.2:443"
	if row[1] != expected {
		t.Fatalf("expected endpoints '%s', got '%s'", expected, row[1])
	}
}

func TestEndpointsPluginDescribe(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Endpoints",
			"metadata": map[string]any{
				"name":              "my-svc",
				"namespace":         "default",
				"creationTimestamp": "2026-02-24T10:00:00Z",
				"labels":            map[string]any{"app": "web"},
				"annotations":       map[string]any{"note": "test"},
			},
			"subsets": []any{
				map[string]any{
					"addresses": []any{
						map[string]any{"ip": "10.0.0.1"},
					},
					"ports": []any{
						map[string]any{"port": int64(80), "protocol": "TCP"},
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
		"my-svc", "default",
		"app=web",
		"note=test",
		"Subsets:",
		"10.0.0.1",
		"80",
	}
	for _, want := range checks {
		if !strings.Contains(c.Raw, want) {
			t.Errorf("describe output should contain %q\n\nFull output:\n%s", want, c.Raw)
		}
	}
}
