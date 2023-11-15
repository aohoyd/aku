package validatingadmissionpolicybindings

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestValidatingAdmissionPolicyBindingPluginColumns(t *testing.T) {
	p := New(nil, nil)
	cols := p.Columns()
	if len(cols) != 3 {
		t.Fatalf("expected 3 columns, got %d", len(cols))
	}
}

func TestValidatingAdmissionPolicyBindingPluginRow(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "admissionregistration.k8s.io/v1",
			"kind":       "ValidatingAdmissionPolicyBinding",
			"metadata": map[string]any{
				"name":              "no-host-network-binding",
				"creationTimestamp": "2024-01-01T00:00:00Z",
			},
			"spec": map[string]any{
				"policyName": "no-host-network",
			},
		},
	}
	row := p.Row(obj)
	if row[0] != "no-host-network-binding" {
		t.Fatalf("expected name 'no-host-network-binding', got '%s'", row[0])
	}
	if row[1] != "no-host-network" {
		t.Fatalf("expected policy 'no-host-network', got '%s'", row[1])
	}
	if row[2] == "" {
		t.Fatal("expected non-empty age")
	}
}

func TestValidatingAdmissionPolicyBindingPluginDescribe(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "admissionregistration.k8s.io/v1",
			"kind":       "ValidatingAdmissionPolicyBinding",
			"metadata": map[string]any{
				"name":              "test-vapb",
				"creationTimestamp": "2026-02-24T10:00:00Z",
				"labels":            map[string]any{"app": "policy"},
				"annotations":       map[string]any{"note": "test"},
			},
			"spec": map[string]any{
				"policyName":        "restrict-host-network",
				"validationActions": []any{"Deny", "Audit"},
				"paramRef": map[string]any{
					"name":                    "my-config",
					"namespace":               "default",
					"parameterNotFoundAction": "Deny",
				},
				"matchResources": map[string]any{
					"matchPolicy": "Equivalent",
					"namespaceSelector": map[string]any{
						"matchLabels": map[string]any{"env": "production"},
					},
					"resourceRules": []any{
						map[string]any{
							"apiGroups":   []any{"apps"},
							"apiVersions": []any{"v1"},
							"operations":  []any{"CREATE", "UPDATE"},
							"resources":   []any{"deployments"},
						},
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
		"test-vapb",
		"restrict-host-network",
		"Deny, Audit",
		"my-config",
		"default",
		"Equivalent",
		"production",
		"CREATE, UPDATE",
		"deployments",
	}
	for _, want := range checks {
		if !strings.Contains(c.Raw, want) {
			t.Errorf("describe output should contain %q\n\nFull output:\n%s", want, c.Raw)
		}
	}
}
