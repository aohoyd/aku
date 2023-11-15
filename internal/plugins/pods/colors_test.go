package pods

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestRenderStatusRunning(t *testing.T) {
	result := renderStatus("Running")
	if ansi.Strip(result) != "Running" {
		t.Fatalf("expected stripped text 'Running', got %q", ansi.Strip(result))
	}
}

func TestRenderStatusFailed(t *testing.T) {
	result := renderStatus("Failed")
	if ansi.Strip(result) != "Failed" {
		t.Fatalf("expected stripped text 'Failed', got %q", ansi.Strip(result))
	}
}

func TestRenderStatusUnknown(t *testing.T) {
	result := renderStatus("SomethingElse")
	if result != "SomethingElse" {
		t.Fatalf("unknown phases should be returned plain, got %q", result)
	}
}

func TestRenderStatusPreservesBackground(t *testing.T) {
	result := renderStatus("Running")
	// Must NOT contain \x1b[0m (full reset) — only \x1b[39m (fg-only reset)
	if contains(result, "\x1b[0m") {
		t.Fatal("renderStatus must not use full SGR reset (\\e[0m); use fg-only reset (\\e[39m)")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestRenderReadySingleReady(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"spec": map[string]any{
				"containers": []any{
					map[string]any{"name": "main"},
				},
			},
			"status": map[string]any{
				"containerStatuses": []any{
					map[string]any{
						"name":  "main",
						"ready": true,
						"state": map[string]any{
							"running": map[string]any{},
						},
					},
				},
			},
		},
	}
	result := renderReady(obj)
	if result != "1/1" {
		t.Fatalf("expected '1/1', got %q", result)
	}
}

func TestRenderReadyMultiMixed(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"spec": map[string]any{
				"containers": []any{
					map[string]any{"name": "app"},
					map[string]any{"name": "sidecar"},
					map[string]any{"name": "proxy"},
				},
			},
			"status": map[string]any{
				"containerStatuses": []any{
					map[string]any{
						"name": "app", "ready": true,
						"state": map[string]any{"running": map[string]any{}},
					},
					map[string]any{
						"name": "sidecar", "ready": true,
						"state": map[string]any{"running": map[string]any{}},
					},
					map[string]any{
						"name": "proxy", "ready": false,
						"state": map[string]any{
							"waiting": map[string]any{"reason": "CrashLoopBackOff"},
						},
					},
				},
			},
		},
	}
	result := renderReady(obj)
	if result != "2/3" {
		t.Fatalf("expected '2/3', got %q", result)
	}
}

func TestRenderReadyNoStatus(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"spec": map[string]any{
				"containers": []any{
					map[string]any{"name": "main"},
					map[string]any{"name": "sidecar"},
				},
			},
		},
	}
	result := renderReady(obj)
	if result != "0/2" {
		t.Fatalf("expected '0/2', got %q", result)
	}
}

func TestRenderReadyNoContainers(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"spec": map[string]any{},
		},
	}
	result := renderReady(obj)
	if result != "0/0" {
		t.Fatalf("expected '0/0', got %q", result)
	}
}
