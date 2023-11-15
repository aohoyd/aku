package networkpolicies

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestNetworkPolicyPluginColumns(t *testing.T) {
	p := New(nil, nil)
	cols := p.Columns()
	if len(cols) != 3 {
		t.Fatalf("expected 3 columns, got %d", len(cols))
	}
}

func TestNetworkPolicyPluginRow(t *testing.T) {
	p := New(nil, nil)

	t.Run("with labels", func(t *testing.T) {
		obj := &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "networking.k8s.io/v1",
				"kind":       "NetworkPolicy",
				"metadata": map[string]any{
					"name":              "allow-web",
					"namespace":         "default",
					"creationTimestamp": "2024-01-01T00:00:00Z",
				},
				"spec": map[string]any{
					"podSelector": map[string]any{
						"matchLabels": map[string]any{
							"role": "web",
							"app":  "nginx",
						},
					},
				},
			},
		}
		row := p.Row(obj)
		if row[0] != "allow-web" {
			t.Fatalf("expected name 'allow-web', got '%s'", row[0])
		}
		// Labels should be sorted: app=nginx,role=web
		if row[1] != "app=nginx,role=web" {
			t.Fatalf("expected pod-selector 'app=nginx,role=web', got '%s'", row[1])
		}
	})

	t.Run("without labels", func(t *testing.T) {
		obj := &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "networking.k8s.io/v1",
				"kind":       "NetworkPolicy",
				"metadata": map[string]any{
					"name":              "deny-all",
					"namespace":         "default",
					"creationTimestamp": "2024-01-01T00:00:00Z",
				},
				"spec": map[string]any{
					"podSelector": map[string]any{},
				},
			},
		}
		row := p.Row(obj)
		if row[1] != "<none>" {
			t.Fatalf("expected pod-selector '<none>', got '%s'", row[1])
		}
	})
}

func TestNetworkPolicyPluginDescribe(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "networking.k8s.io/v1",
			"kind":       "NetworkPolicy",
			"metadata": map[string]any{
				"name":              "test-policy",
				"namespace":         "default",
				"creationTimestamp": "2026-02-24T10:00:00Z",
				"labels":            map[string]any{"app": "web"},
			},
			"spec": map[string]any{
				"podSelector": map[string]any{
					"matchLabels": map[string]any{
						"role": "db",
					},
				},
				"policyTypes": []any{"Ingress", "Egress"},
				"ingress": []any{
					map[string]any{
						"from": []any{
							map[string]any{
								"podSelector": map[string]any{
									"matchLabels": map[string]any{
										"role": "frontend",
									},
								},
							},
						},
						"ports": []any{
							map[string]any{
								"protocol": "TCP",
								"port":     int64(5432),
							},
						},
					},
				},
				"egress": []any{
					map[string]any{
						"to": []any{
							map[string]any{
								"ipBlock": map[string]any{
									"cidr":   "10.0.0.0/24",
									"except": []any{"10.0.0.1/32"},
								},
							},
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
		"test-policy", "default",
		"app=web",
		"role=db",
		"Ingress, Egress",
		"Ingress Rules:",
		"5432/TCP",
		"role=frontend",
		"Egress Rules:",
		"10.0.0.0/24",
		"10.0.0.1/32",
	}
	for _, want := range checks {
		if !strings.Contains(c.Raw, want) {
			t.Errorf("describe output should contain %q\n\nFull output:\n%s", want, c.Raw)
		}
	}
}
