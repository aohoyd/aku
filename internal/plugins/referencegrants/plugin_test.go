package referencegrants

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestReferenceGrantPluginColumns(t *testing.T) {
	p := New(nil, nil)
	cols := p.Columns()
	if len(cols) != 2 {
		t.Fatalf("expected 2 columns, got %d", len(cols))
	}
}

func TestReferenceGrantPluginRow(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "gateway.networking.k8s.io/v1",
			"kind":       "ReferenceGrant",
			"metadata": map[string]any{
				"name":              "my-refgrant",
				"namespace":         "default",
				"creationTimestamp": "2024-01-01T00:00:00Z",
			},
			"spec": map[string]any{
				"from": []any{
					map[string]any{
						"group":     "gateway.networking.k8s.io",
						"kind":      "HTTPRoute",
						"namespace": "web",
					},
				},
				"to": []any{
					map[string]any{
						"group": "",
						"kind":  "Service",
						"name":  "backend-svc",
					},
				},
			},
		},
	}
	row := p.Row(obj)

	if row[0] != "my-refgrant" {
		t.Fatalf("expected name 'my-refgrant', got '%s'", row[0])
	}
	// row[1] is age, just check it's non-empty
	if row[1] == "" {
		t.Fatal("expected non-empty age")
	}
}

func TestReferenceGrantPluginDescribe(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "gateway.networking.k8s.io/v1",
			"kind":       "ReferenceGrant",
			"metadata": map[string]any{
				"name":              "test-refgrant",
				"namespace":         "infra",
				"creationTimestamp": "2026-02-24T10:00:00Z",
				"labels":            map[string]any{"app": "gateway"},
				"annotations":       map[string]any{"note": "test"},
			},
			"spec": map[string]any{
				"from": []any{
					map[string]any{
						"group":     "gateway.networking.k8s.io",
						"kind":      "HTTPRoute",
						"namespace": "web",
					},
					map[string]any{
						"group":     "gateway.networking.k8s.io",
						"kind":      "GRPCRoute",
						"namespace": "grpc",
					},
				},
				"to": []any{
					map[string]any{
						"group": "",
						"kind":  "Service",
						"name":  "backend-svc",
					},
					map[string]any{
						"group": "",
						"kind":  "Secret",
						"name":  "tls-cert",
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

	// Check from entries
	fromChecks := []string{
		"gateway.networking.k8s.io", "HTTPRoute", "web",
		"GRPCRoute", "grpc",
	}
	for _, want := range fromChecks {
		if !strings.Contains(c.Raw, want) {
			t.Errorf("describe output should contain from entry %q\n\nFull output:\n%s", want, c.Raw)
		}
	}

	// Check to entries
	toChecks := []string{
		"Service", "backend-svc",
		"Secret", "tls-cert",
	}
	for _, want := range toChecks {
		if !strings.Contains(c.Raw, want) {
			t.Errorf("describe output should contain to entry %q\n\nFull output:\n%s", want, c.Raw)
		}
	}

	// Check metadata
	metaChecks := []string{
		"test-refgrant", "infra",
		"app=gateway",
		"note=test",
	}
	for _, want := range metaChecks {
		if !strings.Contains(c.Raw, want) {
			t.Errorf("describe output should contain %q\n\nFull output:\n%s", want, c.Raw)
		}
	}
}
