package secrets

import (
	"strings"
	"testing"

	"github.com/aohoyd/aku/internal/plugin"
	"github.com/charmbracelet/x/ansi"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestSecretsPluginColumns(t *testing.T) {
	p := New(nil, nil)
	cols := p.Columns()
	if len(cols) != 4 {
		t.Fatalf("expected 4 columns, got %d", len(cols))
	}
}

func TestSecretsPluginRow(t *testing.T) {
	p := New(nil, nil)
	obj := makeSecret("my-secret", "Opaque", map[string]any{
		"username": "dXNlcg==",
		"password": "cGFzc3dvcmQxMjM=",
	})
	row := p.Row(obj)
	if row[0] != "my-secret" {
		t.Fatalf("expected 'my-secret', got '%s'", row[0])
	}
	if row[1] != "Opaque" {
		t.Fatalf("expected type 'Opaque', got '%s'", row[1])
	}
	if row[2] != "2" {
		t.Fatalf("expected '2' data entries, got '%s'", row[2])
	}
}

func TestSecretsPluginUncoverable(t *testing.T) {
	p := New(nil, nil)
	_, ok := p.(plugin.Uncoverable)
	if !ok {
		t.Fatal("expected Plugin to implement plugin.Uncoverable")
	}
}

func TestSecretsPluginDescribeUncovered(t *testing.T) {
	p := New(nil, nil)
	unc, ok := p.(plugin.Uncoverable)
	if !ok {
		t.Fatal("expected Plugin to implement plugin.Uncoverable")
	}

	obj := makeSecret("my-secret", "Opaque", map[string]any{
		"password": "cGFzc3dvcmQxMjM=",
	})
	obj.Object["metadata"].(map[string]any)["namespace"] = "default"
	obj.Object["metadata"].(map[string]any)["labels"] = map[string]any{"app": "web"}

	c, err := unc.DescribeUncovered(t.Context(), obj)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Display == "" {
		t.Fatal("display output should not be empty")
	}
	if stripped := ansi.Strip(c.Display); stripped != c.Raw {
		t.Errorf("strip invariant violated: ansi.Strip(c.Display) != raw\nstripped: %q\nraw:      %q", stripped, c.Raw)
	}
	if !strings.Contains(c.Raw, "password123") {
		t.Errorf("uncovered describe should contain decoded value 'password123'\n\nFull output:\n%s", c.Raw)
	}
}

func TestSecretsPluginDescribeMasked(t *testing.T) {
	p := New(nil, nil)

	obj := makeSecret("my-secret", "Opaque", map[string]any{
		"password": "cGFzc3dvcmQxMjM=",
	})
	obj.Object["metadata"].(map[string]any)["namespace"] = "default"

	c, err := p.Describe(t.Context(), obj)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(c.Raw, "password123") {
		t.Errorf("masked describe should NOT contain decoded value 'password123'\n\nFull output:\n%s", c.Raw)
	}
	// Should contain the byte count instead
	if !strings.Contains(c.Raw, "11 bytes") {
		t.Errorf("masked describe should contain '11 bytes' for password key\n\nFull output:\n%s", c.Raw)
	}
}

func TestSecretsPluginDescribe(t *testing.T) {
	p := New(nil, nil)

	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]any{
				"name":              "my-secret",
				"namespace":         "default",
				"creationTimestamp": "2026-02-24T10:00:00Z",
				"labels":            map[string]any{"app": "web"},
			},
			"type": "Opaque",
			"data": map[string]any{
				"username": "dXNlcg==",
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

	checks := []string{"my-secret", "default", "app=web", "Opaque", "Data:", "username"}
	for _, want := range checks {
		if !strings.Contains(c.Raw, want) {
			t.Errorf("describe output should contain %q\n\nFull output:\n%s", want, c.Raw)
		}
	}
}

func makeSecret(name, secretType string, data map[string]any) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]any{
				"name":              name,
				"namespace":         "default",
				"creationTimestamp": "2024-01-01T00:00:00Z",
			},
			"type": secretType,
			"data": data,
		},
	}
}
