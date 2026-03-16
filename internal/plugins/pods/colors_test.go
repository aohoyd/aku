package pods

import (
	"strings"
	"testing"

	"github.com/aohoyd/aku/internal/plugin"
	"github.com/charmbracelet/x/ansi"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestRenderStatusRunning(t *testing.T) {
	result := renderStatus("Running", false)
	if ansi.Strip(result) != "Running" {
		t.Fatalf("expected stripped text 'Running', got %q", ansi.Strip(result))
	}
}

func TestRenderStatusFailed(t *testing.T) {
	result := renderStatus("Failed", false)
	if ansi.Strip(result) != "Failed" {
		t.Fatalf("expected stripped text 'Failed', got %q", ansi.Strip(result))
	}
}

func TestRenderStatusUnknown(t *testing.T) {
	result := renderStatus("SomethingElse", false)
	if result != "SomethingElse" {
		t.Fatalf("unknown phases should be returned plain, got %q", result)
	}
}

func TestRenderStatusRunningNotFullyReady(t *testing.T) {
	result := renderStatus("Running", true)
	stripped := ansi.Strip(result)
	if stripped != "Running" {
		t.Fatalf("expected stripped text 'Running', got %q", stripped)
	}
	// Should use FgFailed (red), not FgRunning (green).
	expected := plugin.StyledFg("Running", plugin.FgFailed)
	if result != expected {
		t.Fatalf("expected red-styled Running, got %q", result)
	}
}

func TestRenderStatusRunningFullyReady(t *testing.T) {
	result := renderStatus("Running", false)
	stripped := ansi.Strip(result)
	if stripped != "Running" {
		t.Fatalf("expected stripped text 'Running', got %q", stripped)
	}
	// Should use FgRunning (green) — same as before.
	expected := plugin.StyledFg("Running", plugin.FgRunning)
	if result != expected {
		t.Fatalf("expected green-styled Running, got %q", result)
	}
}

func TestRenderStatusPreservesBackground(t *testing.T) {
	result := renderStatus("Running", false)
	// Must NOT contain \x1b[0m (full reset) — only \x1b[39m (fg-only reset)
	if strings.Contains(result, "\x1b[0m") {
		t.Fatal("renderStatus must not use full SGR reset (\\e[0m); use fg-only reset (\\e[39m)")
	}
}

func TestReadyCountAllReady(t *testing.T) {
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
						"state": map[string]any{"running": map[string]any{}},
					},
				},
			},
		},
	}
	ready, total := readyCount(obj)
	if ready != 1 || total != 1 {
		t.Fatalf("expected (1, 1), got (%d, %d)", ready, total)
	}
}

func TestReadyCountPartialReady(t *testing.T) {
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
					map[string]any{"name": "app", "ready": true, "state": map[string]any{"running": map[string]any{}}},
					map[string]any{"name": "sidecar", "ready": true, "state": map[string]any{"running": map[string]any{}}},
					map[string]any{"name": "proxy", "ready": false, "state": map[string]any{"waiting": map[string]any{"reason": "CrashLoopBackOff"}}},
				},
			},
		},
	}
	ready, total := readyCount(obj)
	if ready != 2 || total != 3 {
		t.Fatalf("expected (2, 3), got (%d, %d)", ready, total)
	}
}

func TestReadyCountNoStatus(t *testing.T) {
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
	ready, total := readyCount(obj)
	if ready != 0 || total != 2 {
		t.Fatalf("expected (0, 2), got (%d, %d)", ready, total)
	}
}

