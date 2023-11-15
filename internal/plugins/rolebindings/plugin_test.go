package rolebindings

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestRoleBindingPluginColumns(t *testing.T) {
	p := New(nil, nil)
	cols := p.Columns()
	if len(cols) != 3 {
		t.Fatalf("expected 3 columns, got %d", len(cols))
	}
}

func TestRoleBindingPluginRow(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "RoleBinding",
			"metadata": map[string]any{
				"name":              "my-binding",
				"namespace":         "default",
				"creationTimestamp": "2024-01-01T00:00:00Z",
			},
			"roleRef": map[string]any{
				"kind":     "Role",
				"name":     "my-role",
				"apiGroup": "rbac.authorization.k8s.io",
			},
		},
	}
	row := p.Row(obj)

	if row[0] != "my-binding" {
		t.Fatalf("expected name 'my-binding', got '%s'", row[0])
	}
	if row[1] != "Role/my-role" {
		t.Fatalf("expected role 'Role/my-role', got '%s'", row[1])
	}
	// row[2] is age, just check it's non-empty
	if row[2] == "" {
		t.Fatal("expected non-empty age")
	}
}

func TestRoleBindingPluginDescribe(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "RoleBinding",
			"metadata": map[string]any{
				"name":              "test-binding",
				"namespace":         "kube-system",
				"creationTimestamp": "2026-02-24T10:00:00Z",
				"labels":            map[string]any{"app": "auth"},
				"annotations":       map[string]any{"note": "test"},
			},
			"roleRef": map[string]any{
				"kind":     "ClusterRole",
				"name":     "admin",
				"apiGroup": "rbac.authorization.k8s.io",
			},
			"subjects": []any{
				map[string]any{
					"kind":      "ServiceAccount",
					"name":      "my-sa",
					"namespace": "kube-system",
				},
				map[string]any{
					"kind": "User",
					"name": "jane",
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
		"test-binding", "kube-system",
		"app=auth",
		"note=test",
		"ClusterRole",
		"admin",
		"rbac.authorization.k8s.io",
		"ServiceAccount",
		"my-sa",
		"User",
		"jane",
	}
	for _, want := range checks {
		if !strings.Contains(c.Raw, want) {
			t.Errorf("describe output should contain %q\n\nFull output:\n%s", want, c.Raw)
		}
	}
}
