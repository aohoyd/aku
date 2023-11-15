package services

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestServicePluginColumns(t *testing.T) {
	p := New(nil, nil)
	cols := p.Columns()
	if len(cols) != 6 {
		t.Fatalf("expected 6 columns, got %d", len(cols))
	}
}

func TestServicePluginRow(t *testing.T) {
	p := New(nil, nil)
	obj := makeService("my-svc", "ClusterIP", "10.0.0.1", "80/TCP")
	row := p.Row(obj)
	if row[0] != "my-svc" {
		t.Fatalf("expected 'my-svc', got '%s'", row[0])
	}
	if row[1] != "ClusterIP" {
		t.Fatalf("expected 'ClusterIP', got '%s'", row[1])
	}
}

func TestServicePluginName(t *testing.T) {
	p := New(nil, nil)
	if p.Name() != "services" {
		t.Fatalf("expected 'services', got '%s'", p.Name())
	}
}

func TestServicePluginDescribeDocument(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"metadata": map[string]any{
				"name":              "nginx-svc",
				"namespace":         "default",
				"creationTimestamp": "2026-02-24T10:00:00Z",
				"labels":            map[string]any{"app": "nginx"},
			},
			"spec": map[string]any{
				"type":            "ClusterIP",
				"clusterIP":       "10.96.0.1",
				"selector":        map[string]any{"app": "nginx"},
				"sessionAffinity": "None",
				"ports": []any{
					map[string]any{
						"port":       int64(80),
						"targetPort": int64(8080),
						"protocol":   "TCP",
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
		"nginx-svc", "default", "ClusterIP", "10.96.0.1",
		"80/TCP", "8080",
		"app=nginx",
		"Session Affinity:", "None",
	}
	for _, want := range checks {
		if !strings.Contains(c.Raw, want) {
			t.Errorf("describe output should contain %q\n\nFull output:\n%s", want, c.Raw)
		}
	}
}

func makeService(name, svcType, clusterIP, ports string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Service",
			"metadata": map[string]any{
				"name":              name,
				"namespace":         "default",
				"creationTimestamp": "2024-01-01T00:00:00Z",
			},
			"spec": map[string]any{
				"type":      svcType,
				"clusterIP": clusterIP,
				"ports": []any{
					map[string]any{
						"port":     int64(80),
						"protocol": "TCP",
					},
				},
			},
		},
	}
}
