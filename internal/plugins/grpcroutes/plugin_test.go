package grpcroutes

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestGRPCRoutePluginColumns(t *testing.T) {
	p := New(nil, nil)
	cols := p.Columns()
	if len(cols) != 4 {
		t.Fatalf("expected 4 columns, got %d", len(cols))
	}
}

func TestGRPCRoutePluginRow(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "gateway.networking.k8s.io/v1",
			"kind":       "GRPCRoute",
			"metadata": map[string]any{
				"name":              "my-grpcroute",
				"namespace":         "default",
				"creationTimestamp": "2024-01-01T00:00:00Z",
			},
			"spec": map[string]any{
				"hostnames": []any{"grpc.example.com", "api.example.com"},
				"parentRefs": []any{
					map[string]any{
						"name":      "my-gateway",
						"namespace": "infra",
					},
					map[string]any{
						"name": "other-gw",
					},
				},
			},
		},
	}
	row := p.Row(obj)

	if row[0] != "my-grpcroute" {
		t.Fatalf("expected name 'my-grpcroute', got '%s'", row[0])
	}
	if row[1] != "grpc.example.com,api.example.com" {
		t.Fatalf("expected hostnames 'grpc.example.com,api.example.com', got '%s'", row[1])
	}
	if row[2] != "infra/my-gateway,default/other-gw" {
		t.Fatalf("expected parentRefs 'infra/my-gateway,default/other-gw', got '%s'", row[2])
	}
	// row[3] is age, just check it is non-empty
	if row[3] == "" {
		t.Fatal("expected non-empty age")
	}
}

func TestGRPCRoutePluginRowNoHostnames(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "gateway.networking.k8s.io/v1",
			"kind":       "GRPCRoute",
			"metadata": map[string]any{
				"name":              "no-hosts",
				"namespace":         "default",
				"creationTimestamp": "2024-01-01T00:00:00Z",
			},
			"spec": map[string]any{},
		},
	}
	row := p.Row(obj)

	if row[1] != "*" {
		t.Fatalf("expected hostnames '*', got '%s'", row[1])
	}
}

func TestGRPCRoutePluginDescribe(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "gateway.networking.k8s.io/v1",
			"kind":       "GRPCRoute",
			"metadata": map[string]any{
				"name":              "test-grpcroute",
				"namespace":         "default",
				"creationTimestamp": "2026-02-24T10:00:00Z",
				"labels":            map[string]any{"app": "grpc"},
			},
			"spec": map[string]any{
				"hostnames": []any{"grpc.example.com"},
				"parentRefs": []any{
					map[string]any{
						"name":        "my-gateway",
						"namespace":   "infra",
						"sectionName": "grpc",
					},
				},
				"rules": []any{
					map[string]any{
						"matches": []any{
							map[string]any{
								"method": map[string]any{
									"service": "helloworld.Greeter",
									"method":  "SayHello",
								},
							},
						},
						"backendRefs": []any{
							map[string]any{
								"name":      "grpc-svc",
								"namespace": "default",
								"port":      int64(50051),
								"weight":    int64(1),
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
		"test-grpcroute", "default",
		"grpc.example.com",
		"my-gateway", "infra", "grpc",
		"helloworld.Greeter", "SayHello",
		"grpc-svc", "50051",
	}
	for _, want := range checks {
		if !strings.Contains(c.Raw, want) {
			t.Errorf("describe output should contain %q\n\nFull output:\n%s", want, c.Raw)
		}
	}
}
