package configmaps

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestConfigMapPluginColumns(t *testing.T) {
	p := New(nil, nil)
	cols := p.Columns()
	if len(cols) != 3 {
		t.Fatalf("expected 3 columns, got %d", len(cols))
	}
}

func TestConfigMapPluginRow(t *testing.T) {
	p := New(nil, nil)
	obj := makeConfigMap("my-cm", map[string]any{"key1": "val1", "key2": "val2"})
	row := p.Row(obj)
	if row[0] != "my-cm" {
		t.Fatalf("expected 'my-cm', got '%s'", row[0])
	}
	if row[1] != "2" {
		t.Fatalf("expected '2' data entries, got '%s'", row[1])
	}
}

func TestConfigMapPluginName(t *testing.T) {
	p := New(nil, nil)
	if p.Name() != "configmaps" {
		t.Fatalf("expected 'configmaps', got '%s'", p.Name())
	}
}

func TestConfigMapPluginDescribeDocument(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"metadata": map[string]any{
				"name":              "my-config",
				"namespace":         "default",
				"creationTimestamp": "2026-02-24T10:00:00Z",
				"labels":            map[string]any{"app": "nginx"},
			},
			"data": map[string]any{
				"config.yaml": "server:\n  port: 8080\n  host: 0.0.0.0",
				"key":         "value",
			},
			"binaryData": map[string]any{
				"cert.pem": "c29tZSBiaW5hcnkgZGF0YQ==",
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
		"my-config", "default",
		"app=nginx",
		"Data:",
		"config.yaml:", "----", "server:", "port: 8080", "host: 0.0.0.0",
		"key:", "----", "value",
		"BinaryData:", "cert.pem",
	}
	for _, want := range checks {
		if !strings.Contains(c.Raw, want) {
			t.Errorf("describe output should contain %q\n\nFull output:\n%s", want, c.Raw)
		}
	}
}

func makeConfigMap(name string, data map[string]any) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]any{
				"name":              name,
				"namespace":         "default",
				"creationTimestamp": "2024-01-01T00:00:00Z",
			},
			"data": data,
		},
	}
}
