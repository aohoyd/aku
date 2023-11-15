package ingresses

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestIngressPluginColumns(t *testing.T) {
	p := New(nil, nil)
	cols := p.Columns()
	if len(cols) != 6 {
		t.Fatalf("expected 6 columns, got %d", len(cols))
	}
}

func TestIngressPluginRow(t *testing.T) {
	p := New(nil, nil)
	obj := makeIngress("my-ingress", "nginx", []string{"foo.example.com", "bar.example.com"}, true)
	row := p.Row(obj)

	if row[0] != "my-ingress" {
		t.Fatalf("expected name 'my-ingress', got '%s'", row[0])
	}
	if row[1] != "nginx" {
		t.Fatalf("expected class 'nginx', got '%s'", row[1])
	}
	if row[2] != "foo.example.com,bar.example.com" {
		t.Fatalf("expected hosts 'foo.example.com,bar.example.com', got '%s'", row[2])
	}
	if row[3] != "10.0.0.1" {
		t.Fatalf("expected address '10.0.0.1', got '%s'", row[3])
	}
	if row[4] != "80,443" {
		t.Fatalf("expected ports '80,443', got '%s'", row[4])
	}
}

func TestIngressPluginRowNoTLS(t *testing.T) {
	p := New(nil, nil)
	obj := makeIngress("simple-ingress", "nginx", []string{"example.com"}, false)
	row := p.Row(obj)

	if row[4] != "80" {
		t.Fatalf("expected ports '80', got '%s'", row[4])
	}
}

func TestIngressPluginRowNoClass(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "networking.k8s.io/v1",
			"kind":       "Ingress",
			"metadata": map[string]any{
				"name":              "no-class",
				"namespace":         "default",
				"creationTimestamp": "2024-01-01T00:00:00Z",
			},
			"spec": map[string]any{
				"rules": []any{
					map[string]any{"host": "example.com"},
				},
			},
		},
	}
	row := p.Row(obj)
	if row[1] != "<none>" {
		t.Fatalf("expected class '<none>', got '%s'", row[1])
	}
}

func TestIngressPluginDescribe(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "networking.k8s.io/v1",
			"kind":       "Ingress",
			"metadata": map[string]any{
				"name":              "test-ingress",
				"namespace":         "default",
				"creationTimestamp": "2026-02-24T10:00:00Z",
				"labels":            map[string]any{"app": "web"},
			},
			"spec": map[string]any{
				"ingressClassName": "nginx",
				"rules": []any{
					map[string]any{
						"host": "foo.example.com",
						"http": map[string]any{
							"paths": []any{
								map[string]any{
									"path":     "/api",
									"pathType": "Prefix",
									"backend": map[string]any{
										"service": map[string]any{
											"name": "api-svc",
											"port": map[string]any{
												"number": int64(8080),
											},
										},
									},
								},
							},
						},
					},
				},
				"tls": []any{
					map[string]any{
						"secretName": "tls-secret",
						"hosts":      []any{"foo.example.com"},
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
		"test-ingress", "default",
		"app=web",
		"nginx",
		"foo.example.com",
		"/api", "Prefix", "api-svc:8080",
		"TLS:", "tls-secret",
	}
	for _, want := range checks {
		if !strings.Contains(c.Raw, want) {
			t.Errorf("describe output should contain %q\n\nFull output:\n%s", want, c.Raw)
		}
	}
}

func makeIngress(name, class string, hosts []string, withTLS bool) *unstructured.Unstructured {
	rules := make([]any, len(hosts))
	for i, host := range hosts {
		rules[i] = map[string]any{"host": host}
	}

	spec := map[string]any{
		"ingressClassName": class,
		"rules":            rules,
	}

	if withTLS {
		spec["tls"] = []any{
			map[string]any{
				"secretName": "tls-secret",
				"hosts":      hosts,
			},
		}
	}

	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "networking.k8s.io/v1",
			"kind":       "Ingress",
			"metadata": map[string]any{
				"name":              name,
				"namespace":         "default",
				"creationTimestamp": "2024-01-01T00:00:00Z",
			},
			"spec": spec,
			"status": map[string]any{
				"loadBalancer": map[string]any{
					"ingress": []any{
						map[string]any{"ip": "10.0.0.1"},
					},
				},
			},
		},
	}
}
