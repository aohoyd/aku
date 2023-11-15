package clusterrolebindings

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestClusterRoleBindingPluginColumns(t *testing.T) {
	p := New(nil, nil)
	cols := p.Columns()
	if len(cols) != 3 {
		t.Fatalf("expected 3 columns, got %d", len(cols))
	}
}

func TestClusterRoleBindingPluginRow(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "ClusterRoleBinding",
			"metadata": map[string]any{
				"name":              "admin-binding",
				"creationTimestamp": "2024-01-01T00:00:00Z",
			},
			"roleRef": map[string]any{
				"kind":     "ClusterRole",
				"name":     "admin",
				"apiGroup": "rbac.authorization.k8s.io",
			},
			"subjects": []any{
				map[string]any{
					"kind":      "User",
					"name":      "jane",
					"namespace": "",
				},
			},
		},
	}
	row := p.Row(obj)

	if row[0] != "admin-binding" {
		t.Fatalf("expected name 'admin-binding', got '%s'", row[0])
	}
	if row[1] != "ClusterRole/admin" {
		t.Fatalf("expected role 'ClusterRole/admin', got '%s'", row[1])
	}
	// row[2] is age, just check it's non-empty
	if row[2] == "" {
		t.Fatal("expected non-empty age")
	}
}

func TestClusterRoleBindingPluginDescribe(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "rbac.authorization.k8s.io/v1",
			"kind":       "ClusterRoleBinding",
			"metadata": map[string]any{
				"name":              "test-crb",
				"creationTimestamp": "2026-02-24T10:00:00Z",
				"labels":            map[string]any{"app": "auth"},
				"annotations":       map[string]any{"note": "test"},
			},
			"roleRef": map[string]any{
				"kind":     "ClusterRole",
				"name":     "cluster-admin",
				"apiGroup": "rbac.authorization.k8s.io",
			},
			"subjects": []any{
				map[string]any{
					"kind":      "ServiceAccount",
					"name":      "default",
					"namespace": "kube-system",
				},
				map[string]any{
					"kind": "User",
					"name": "admin-user",
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
		"test-crb",
		"app=auth",
		"note=test",
		"ClusterRole",
		"cluster-admin",
		"rbac.authorization.k8s.io",
		"ServiceAccount",
		"default",
		"kube-system",
		"User",
		"admin-user",
	}
	for _, want := range checks {
		if !strings.Contains(c.Raw, want) {
			t.Errorf("describe output should contain %q\n\nFull output:\n%s", want, c.Raw)
		}
	}
}
