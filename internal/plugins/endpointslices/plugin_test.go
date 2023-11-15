package endpointslices

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestEndpointSlicePluginColumns(t *testing.T) {
	p := New(nil, nil)
	cols := p.Columns()
	if len(cols) != 5 {
		t.Fatalf("expected 5 columns, got %d", len(cols))
	}
}

func TestEndpointSlicePluginRow(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "discovery.k8s.io/v1",
			"kind":       "EndpointSlice",
			"metadata": map[string]any{
				"name":              "my-svc-abc",
				"namespace":         "default",
				"creationTimestamp": "2024-01-01T00:00:00Z",
			},
			"addressType": "IPv4",
			"ports": []any{
				map[string]any{
					"port":     int64(8080),
					"protocol": "TCP",
				},
				map[string]any{
					"port":     int64(443),
					"protocol": "TCP",
				},
			},
			"endpoints": []any{
				map[string]any{
					"addresses": []any{"10.0.0.1"},
				},
				map[string]any{
					"addresses": []any{"10.0.0.2"},
				},
				map[string]any{
					"addresses": []any{"10.0.0.3"},
				},
			},
		},
	}

	row := p.Row(obj)

	if row[0] != "my-svc-abc" {
		t.Fatalf("expected name 'my-svc-abc', got '%s'", row[0])
	}
	if row[1] != "IPv4" {
		t.Fatalf("expected addressType 'IPv4', got '%s'", row[1])
	}
	if row[2] != "8080/TCP,443/TCP" {
		t.Fatalf("expected ports '8080/TCP,443/TCP', got '%s'", row[2])
	}
	if row[3] != "3" {
		t.Fatalf("expected endpoints '3', got '%s'", row[3])
	}
}

func TestEndpointSlicePluginDescribe(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "discovery.k8s.io/v1",
			"kind":       "EndpointSlice",
			"metadata": map[string]any{
				"name":              "my-svc-abc",
				"namespace":         "default",
				"creationTimestamp": "2026-02-24T10:00:00Z",
				"labels":            map[string]any{"app": "web"},
			},
			"addressType": "IPv4",
			"ports": []any{
				map[string]any{
					"name":     "https",
					"port":     int64(443),
					"protocol": "TCP",
				},
			},
			"endpoints": []any{
				map[string]any{
					"addresses": []any{"10.0.0.1", "10.0.0.2"},
					"conditions": map[string]any{
						"ready":       true,
						"serving":     true,
						"terminating": false,
					},
					"targetRef": map[string]any{
						"kind":      "Pod",
						"name":      "my-pod-1",
						"namespace": "default",
					},
					"zone": "us-east-1a",
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
		"my-svc-abc",
		"default",
		"IPv4",
		"app=web",
		"https",
		"443",
		"TCP",
		"10.0.0.1",
		"10.0.0.2",
		"true",
		"my-pod-1",
		"us-east-1a",
	}
	for _, want := range checks {
		if !strings.Contains(c.Raw, want) {
			t.Errorf("describe output should contain %q\n\nFull output:\n%s", want, c.Raw)
		}
	}
}
