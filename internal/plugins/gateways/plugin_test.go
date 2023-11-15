package gateways

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestGatewayPluginColumns(t *testing.T) {
	p := New(nil, nil)
	cols := p.Columns()
	if len(cols) != 5 {
		t.Fatalf("expected 5 columns, got %d", len(cols))
	}
}

func TestGatewayPluginRow(t *testing.T) {
	p := New(nil, nil)
	obj := makeGateway("my-gateway", "istio", "10.0.0.1", "True")
	row := p.Row(obj)

	if row[0] != "my-gateway" {
		t.Fatalf("expected name 'my-gateway', got '%s'", row[0])
	}
	if row[1] != "istio" {
		t.Fatalf("expected class 'istio', got '%s'", row[1])
	}
	if row[2] != "10.0.0.1" {
		t.Fatalf("expected address '10.0.0.1', got '%s'", row[2])
	}
	if row[3] != "True" {
		t.Fatalf("expected programmed 'True', got '%s'", row[3])
	}
}

func TestGatewayPluginRowNoAddress(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "gateway.networking.k8s.io/v1",
			"kind":       "Gateway",
			"metadata": map[string]any{
				"name":              "no-addr-gw",
				"namespace":         "default",
				"creationTimestamp": "2024-01-01T00:00:00Z",
			},
			"spec": map[string]any{
				"gatewayClassName": "istio",
			},
		},
	}
	row := p.Row(obj)
	if row[2] != "<none>" {
		t.Fatalf("expected address '<none>', got '%s'", row[2])
	}
}

func TestGatewayPluginDescribe(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "gateway.networking.k8s.io/v1",
			"kind":       "Gateway",
			"metadata": map[string]any{
				"name":              "test-gateway",
				"namespace":         "default",
				"creationTimestamp": "2026-02-24T10:00:00Z",
				"labels":            map[string]any{"app": "web"},
			},
			"spec": map[string]any{
				"gatewayClassName": "istio",
				"listeners": []any{
					map[string]any{
						"name":     "http",
						"hostname": "foo.example.com",
						"port":     int64(80),
						"protocol": "HTTP",
					},
					map[string]any{
						"name":     "https",
						"hostname": "foo.example.com",
						"port":     int64(443),
						"protocol": "HTTPS",
					},
				},
			},
			"status": map[string]any{
				"addresses": []any{
					map[string]any{
						"type":  "IPAddress",
						"value": "10.0.0.1",
					},
				},
				"conditions": []any{
					map[string]any{
						"type":    "Accepted",
						"status":  "True",
						"reason":  "Accepted",
						"message": "Gateway accepted",
					},
					map[string]any{
						"type":    "Programmed",
						"status":  "True",
						"reason":  "Programmed",
						"message": "Gateway programmed",
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
		"test-gateway", "default",
		"app=web",
		"istio",
		"http", "foo.example.com", "80", "HTTP",
		"https", "443", "HTTPS",
		"10.0.0.1", "IPAddress",
		"Accepted", "True", "Accepted", "Gateway accepted",
		"Programmed", "True", "Programmed", "Gateway programmed",
	}
	for _, want := range checks {
		if !strings.Contains(c.Raw, want) {
			t.Errorf("describe output should contain %q\n\nFull output:\n%s", want, c.Raw)
		}
	}
}

func makeGateway(name, className, address, programmed string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "gateway.networking.k8s.io/v1",
			"kind":       "Gateway",
			"metadata": map[string]any{
				"name":              name,
				"namespace":         "default",
				"creationTimestamp": "2024-01-01T00:00:00Z",
			},
			"spec": map[string]any{
				"gatewayClassName": className,
			},
			"status": map[string]any{
				"addresses": []any{
					map[string]any{
						"type":  "IPAddress",
						"value": address,
					},
				},
				"conditions": []any{
					map[string]any{
						"type":   "Programmed",
						"status": programmed,
					},
				},
			},
		},
	}
	return obj
}
