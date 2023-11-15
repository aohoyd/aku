package validatingadmissionpolicies

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestValidatingAdmissionPolicyPluginColumns(t *testing.T) {
	p := New(nil, nil)
	cols := p.Columns()
	if len(cols) != 4 {
		t.Fatalf("expected 4 columns, got %d", len(cols))
	}
}

func TestValidatingAdmissionPolicyPluginRow(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "admissionregistration.k8s.io/v1",
			"kind":       "ValidatingAdmissionPolicy",
			"metadata": map[string]any{
				"name":              "no-host-network",
				"creationTimestamp": "2024-01-01T00:00:00Z",
			},
			"spec": map[string]any{
				"failurePolicy": "Fail",
				"validations": []any{
					map[string]any{
						"expression": "!object.spec.template.spec.hostNetwork",
						"message":    "Host network not allowed",
					},
					map[string]any{
						"expression": "object.spec.replicas <= 100",
					},
				},
			},
		},
	}
	row := p.Row(obj)
	if row[0] != "no-host-network" {
		t.Fatalf("expected name 'no-host-network', got '%s'", row[0])
	}
	if row[1] != "2" {
		t.Fatalf("expected validations '2', got '%s'", row[1])
	}
	if row[2] != "Fail" {
		t.Fatalf("expected failure policy 'Fail', got '%s'", row[2])
	}
	if row[3] == "" {
		t.Fatal("expected non-empty age")
	}
}

func TestValidatingAdmissionPolicyPluginDescribe(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "admissionregistration.k8s.io/v1",
			"kind":       "ValidatingAdmissionPolicy",
			"metadata": map[string]any{
				"name":              "test-vap",
				"creationTimestamp": "2026-02-24T10:00:00Z",
				"labels":            map[string]any{"app": "policy"},
				"annotations":       map[string]any{"note": "test"},
			},
			"spec": map[string]any{
				"failurePolicy": "Fail",
				"paramKind": map[string]any{
					"apiVersion": "v1",
					"kind":       "ConfigMap",
				},
				"matchConstraints": map[string]any{
					"matchPolicy": "Equivalent",
					"resourceRules": []any{
						map[string]any{
							"apiGroups":   []any{"apps"},
							"apiVersions": []any{"v1"},
							"operations":  []any{"CREATE", "UPDATE"},
							"resources":   []any{"deployments"},
						},
					},
					"namespaceSelector": map[string]any{
						"matchLabels": map[string]any{"env": "production"},
					},
				},
				"validations": []any{
					map[string]any{
						"expression": "!object.spec.template.spec.hostNetwork",
						"message":    "Host network not allowed",
						"reason":     "Forbidden",
					},
				},
				"matchConditions": []any{
					map[string]any{
						"name":       "production-only",
						"expression": "object.metadata.namespace == 'production'",
					},
				},
				"auditAnnotations": []any{
					map[string]any{
						"key":             "policy-check",
						"valueExpression": "'checked by no-host-network'",
					},
				},
				"variables": []any{
					map[string]any{
						"name":       "replicas",
						"expression": "object.spec.replicas",
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
		"test-vap",
		"Fail",
		"ConfigMap",
		"Equivalent",
		"CREATE, UPDATE",
		"deployments",
		"production",
		"!object.spec.template.spec.hostNetwork",
		"Host network not allowed",
		"Forbidden",
		"production-only",
		"policy-check",
		"replicas",
	}
	for _, want := range checks {
		if !strings.Contains(c.Raw, want) {
			t.Errorf("describe output should contain %q\n\nFull output:\n%s", want, c.Raw)
		}
	}
}
