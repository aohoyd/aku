package httproutes

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestHTTPRoutePluginColumns(t *testing.T) {
	p := New(nil, nil)
	cols := p.Columns()
	if len(cols) != 4 {
		t.Fatalf("expected 4 columns, got %d", len(cols))
	}
}

func TestHTTPRoutePluginRow(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "gateway.networking.k8s.io/v1",
			"kind":       "HTTPRoute",
			"metadata": map[string]any{
				"name":              "my-route",
				"namespace":         "default",
				"creationTimestamp": "2024-01-01T00:00:00Z",
			},
			"spec": map[string]any{
				"hostnames": []any{"foo.com", "bar.com"},
				"parentRefs": []any{
					map[string]any{
						"name":      "my-gw",
						"namespace": "default",
					},
				},
			},
		},
	}
	row := p.Row(obj)

	if row[0] != "my-route" {
		t.Fatalf("expected name 'my-route', got '%s'", row[0])
	}
	if row[1] != "foo.com,bar.com" {
		t.Fatalf("expected hostnames 'foo.com,bar.com', got '%s'", row[1])
	}
	if row[2] != "default/my-gw" {
		t.Fatalf("expected parentRefs 'default/my-gw', got '%s'", row[2])
	}
}

func TestHTTPRoutePluginRowNoHostnames(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "gateway.networking.k8s.io/v1",
			"kind":       "HTTPRoute",
			"metadata": map[string]any{
				"name":              "no-hosts",
				"namespace":         "default",
				"creationTimestamp": "2024-01-01T00:00:00Z",
			},
			"spec": map[string]any{
				"parentRefs": []any{
					map[string]any{
						"name":      "my-gw",
						"namespace": "default",
					},
				},
			},
		},
	}
	row := p.Row(obj)
	if row[1] != "*" {
		t.Fatalf("expected hostnames '*', got '%s'", row[1])
	}
}

func TestHTTPRoutePluginDescribe(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "gateway.networking.k8s.io/v1",
			"kind":       "HTTPRoute",
			"metadata": map[string]any{
				"name":              "test-route",
				"namespace":         "default",
				"creationTimestamp": "2026-02-24T10:00:00Z",
				"labels":            map[string]any{"app": "web"},
			},
			"spec": map[string]any{
				"hostnames": []any{"example.com"},
				"parentRefs": []any{
					map[string]any{
						"name":        "my-gateway",
						"namespace":   "infra",
						"sectionName": "https",
					},
				},
				"rules": []any{
					map[string]any{
						"matches": []any{
							map[string]any{
								"path": map[string]any{
									"type":  "PathPrefix",
									"value": "/api",
								},
								"headers": []any{
									map[string]any{
										"name":  "X-Env",
										"value": "prod",
									},
								},
								"method": "GET",
							},
						},
						"backendRefs": []any{
							map[string]any{
								"name":      "api-svc",
								"namespace": "default",
								"port":      int64(8080),
								"weight":    int64(100),
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
		"test-route", "default",
		"example.com",
		"my-gateway", "infra", "https",
		"/api", "PathPrefix",
		"X-Env", "prod",
		"GET",
		"api-svc", "8080", "100",
	}
	for _, want := range checks {
		if !strings.Contains(c.Raw, want) {
			t.Errorf("describe output should contain %q\n\nFull output:\n%s", want, c.Raw)
		}
	}
}
