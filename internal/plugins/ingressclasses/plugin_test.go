package ingressclasses

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestIngressClassPluginColumns(t *testing.T) {
	p := New(nil, nil)
	cols := p.Columns()
	if len(cols) != 3 {
		t.Fatalf("expected 3 columns, got %d", len(cols))
	}
}

func TestIngressClassPluginRow(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "networking.k8s.io/v1",
			"kind":       "IngressClass",
			"metadata": map[string]any{
				"name":              "nginx",
				"creationTimestamp": "2024-01-01T00:00:00Z",
			},
			"spec": map[string]any{
				"controller": "k8s.io/ingress-nginx",
			},
		},
	}
	row := p.Row(obj)

	if row[0] != "nginx" {
		t.Fatalf("expected name 'nginx', got '%s'", row[0])
	}
	if row[1] != "k8s.io/ingress-nginx" {
		t.Fatalf("expected controller 'k8s.io/ingress-nginx', got '%s'", row[1])
	}
	// row[2] is age — just verify it's non-empty
	if row[2] == "" {
		t.Fatal("expected non-empty age")
	}
}

func TestIngressClassPluginDescribe(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "networking.k8s.io/v1",
			"kind":       "IngressClass",
			"metadata": map[string]any{
				"name":              "nginx",
				"creationTimestamp": "2026-02-24T10:00:00Z",
				"labels":            map[string]any{"app": "nginx"},
				"annotations":       map[string]any{"ingressclass.kubernetes.io/is-default-class": "true"},
			},
			"spec": map[string]any{
				"controller": "k8s.io/ingress-nginx",
				"parameters": map[string]any{
					"apiGroup":  "example.com",
					"kind":      "IngressParameters",
					"name":      "my-params",
					"namespace": "default",
					"scope":     "Namespace",
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
		"nginx",
		"k8s.io/ingress-nginx",
	}
	for _, want := range checks {
		if !strings.Contains(c.Raw, want) {
			t.Errorf("describe output should contain %q\n\nFull output:\n%s", want, c.Raw)
		}
	}
}
