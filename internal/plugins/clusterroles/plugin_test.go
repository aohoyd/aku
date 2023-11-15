package clusterroles

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestClusterRolePluginColumns(t *testing.T) {
	p := New(nil, nil)
	cols := p.Columns()
	if len(cols) != 2 {
		t.Fatalf("expected 2 columns, got %d", len(cols))
	}
}

func TestClusterRolePluginRow(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "ClusterRole",
			"metadata": map[string]any{
				"name":              "test-clusterrole",
				"creationTimestamp": "2024-01-01T00:00:00Z",
			},
		},
	}
	row := p.Row(obj)
	if row[0] != "test-clusterrole" {
		t.Fatalf("expected name 'test-clusterrole', got '%s'", row[0])
	}
	// row[1] is the age column
	if row[1] == "" {
		t.Fatal("expected non-empty age")
	}
}

func TestClusterRolePluginDescribe(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "ClusterRole",
			"metadata": map[string]any{
				"name":              "test-clusterrole",
				"creationTimestamp": "2026-02-24T10:00:00Z",
				"labels":            map[string]any{"app": "rbac"},
				"annotations":       map[string]any{"note": "test"},
			},
			"rules": []any{
				map[string]any{
					"apiGroups": []any{""},
					"resources": []any{"pods", "services"},
					"verbs":     []any{"get", "list", "watch"},
				},
				map[string]any{
					"apiGroups": []any{"apps"},
					"resources": []any{"deployments"},
					"verbs":     []any{"*"},
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
		"test-clusterrole",
		"Rules:",
	}
	for _, want := range checks {
		if !strings.Contains(c.Raw, want) {
			t.Errorf("describe output should contain %q\n\nFull output:\n%s", want, c.Raw)
		}
	}
}
