package gatewayclasses

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestGatewayClassPluginColumns(t *testing.T) {
	p := New(nil, nil)
	cols := p.Columns()
	if len(cols) != 4 {
		t.Fatalf("expected 4 columns, got %d", len(cols))
	}
}

func TestGatewayClassPluginRow(t *testing.T) {
	p := New(nil, nil)
	obj := makeGatewayClass("my-gateway-class", "example.com/gateway-controller", "True", "Accepted", "All good")
	row := p.Row(obj)

	if row[0] != "my-gateway-class" {
		t.Fatalf("expected name 'my-gateway-class', got '%s'", row[0])
	}
	if row[1] != "example.com/gateway-controller" {
		t.Fatalf("expected controller 'example.com/gateway-controller', got '%s'", row[1])
	}
	if row[2] != "True" {
		t.Fatalf("expected accepted 'True', got '%s'", row[2])
	}
}

func TestGatewayClassPluginRowNoConditions(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "gateway.networking.k8s.io/v1",
			"kind":       "GatewayClass",
			"metadata": map[string]any{
				"name":              "no-conditions",
				"creationTimestamp": "2024-01-01T00:00:00Z",
			},
			"spec": map[string]any{
				"controllerName": "example.com/controller",
			},
		},
	}
	row := p.Row(obj)
	if row[2] != "Unknown" {
		t.Fatalf("expected accepted 'Unknown', got '%s'", row[2])
	}
}

func TestGatewayClassPluginDescribe(t *testing.T) {
	p := New(nil, nil)
	obj := makeGatewayClass("test-gc", "example.com/gateway-controller", "True", "Accepted", "Controller accepted")

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
		"test-gc",
		"example.com/gateway-controller",
		"Accepted",
		"True",
		"Controller accepted",
	}
	for _, want := range checks {
		if !strings.Contains(c.Raw, want) {
			t.Errorf("describe output should contain %q\n\nFull output:\n%s", want, c.Raw)
		}
	}
}

func makeGatewayClass(name, controller, acceptedStatus, condReason, condMessage string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "gateway.networking.k8s.io/v1",
			"kind":       "GatewayClass",
			"metadata": map[string]any{
				"name":              name,
				"creationTimestamp": "2024-01-01T00:00:00Z",
				"labels":            map[string]any{"app": "gateway"},
			},
			"spec": map[string]any{
				"controllerName": controller,
			},
			"status": map[string]any{
				"conditions": []any{
					map[string]any{
						"type":    "Accepted",
						"status":  acceptedStatus,
						"reason":  condReason,
						"message": condMessage,
					},
				},
			},
		},
	}
}
