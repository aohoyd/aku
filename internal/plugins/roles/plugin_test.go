package roles

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestRolePluginColumns(t *testing.T) {
	p := New(nil, nil)
	cols := p.Columns()
	if len(cols) != 2 {
		t.Fatalf("expected 2 columns, got %d", len(cols))
	}
}

func TestRolePluginRow(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "Role",
			"metadata": map[string]any{
				"name":              "test-role",
				"namespace":         "default",
				"creationTimestamp": "2024-01-01T00:00:00Z",
			},
		},
	}
	row := p.Row(obj)
	if row[0] != "test-role" {
		t.Fatalf("expected name 'test-role', got '%s'", row[0])
	}
	// row[1] is the age string
	if row[1] == "" {
		t.Fatal("expected non-empty age")
	}
}

func TestRolePluginDescribe(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "Role",
			"metadata": map[string]any{
				"name":              "my-role",
				"namespace":         "kube-system",
				"creationTimestamp": "2026-02-24T10:00:00Z",
				"labels":            map[string]any{"app": "test"},
				"annotations":       map[string]any{"note": "hello"},
			},
			"rules": []any{
				map[string]any{
					"apiGroups": []any{"", "apps"},
					"resources": []any{"pods", "deployments"},
					"verbs":     []any{"get", "list", "watch"},
				},
				map[string]any{
					"apiGroups":       []any{""},
					"resources":       []any{"secrets"},
					"verbs":           []any{"get"},
					"resourceNames":   []any{"my-secret"},
					"nonResourceURLs": []any{"/healthz"},
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
		"my-role",
		"kube-system",
		"app=test",
		"note=hello",
		",apps",
		"pods,deployments",
		"get,list,watch",
		"secrets",
		"my-secret",
		"/healthz",
	}
	for _, want := range checks {
		if !strings.Contains(c.Raw, want) {
			t.Errorf("describe output should contain %q\n\nFull output:\n%s", want, c.Raw)
		}
	}
}
