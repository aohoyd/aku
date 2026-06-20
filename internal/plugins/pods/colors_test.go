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

func TestPluginRowHealth(t *testing.T) {
	pod := func(phase string, ready, total int) *unstructured.Unstructured {
		containers := make([]any, total)
		for i := range containers {
			containers[i] = map[string]any{"name": "c"}
		}
		statuses := make([]any, total)
		for i := range statuses {
			statuses[i] = map[string]any{"name": "c", "ready": i < ready}
		}
		return &unstructured.Unstructured{
			Object: map[string]any{
				"spec": map[string]any{"containers": containers},
				"status": map[string]any{
					"phase":             phase,
					"containerStatuses": statuses,
				},
			},
		}
	}

	tests := []struct {
		name string
		obj  *unstructured.Unstructured
		want plugin.Health
	}{
		{"running all ready", pod("Running", 2, 2), plugin.Healthy},
		{"running partial ready", pod("Running", 1, 2), plugin.Error},
		// status.phase delivered directly as a failure string (no container
		// status overrides it) → Error.
		{"phase failed direct", pod("Failed", 0, 1), plugin.Error},
		{
			// Container terminated with reason OOMKilled makes computePodStatus
			// surface "OOMKilled" as the phase → Error.
			name: "oom killed",
			obj: &unstructured.Unstructured{
				Object: map[string]any{
					"spec": map[string]any{"containers": []any{map[string]any{"name": "c"}}},
					"status": map[string]any{
						"phase": "Running",
						"containerStatuses": []any{
							map[string]any{
								"name":  "c",
								"ready": false,
								"state": map[string]any{"terminated": map[string]any{"reason": "OOMKilled", "exitCode": int64(137)}},
							},
						},
					},
				},
			},
			want: plugin.Error,
		},
		{
			// A pod with a deletionTimestamp computes phase "Terminating" → Error.
			name: "terminating",
			obj: &unstructured.Unstructured{
				Object: map[string]any{
					"metadata": map[string]any{"deletionTimestamp": "2026-06-20T10:00:00Z"},
					"spec":     map[string]any{"containers": []any{map[string]any{"name": "c"}}},
					"status": map[string]any{
						"phase":             "Running",
						"containerStatuses": []any{map[string]any{"name": "c", "ready": true}},
					},
				},
			},
			want: plugin.Error,
		},
		// Running pod with zero containers: total == 0 so the ready<total branch
		// cannot fire; the current code returns Healthy. Lock that branch.
		{"running zero containers", pod("Running", 0, 0), plugin.Healthy},
		{
			name: "crash loop back off",
			obj: &unstructured.Unstructured{
				Object: map[string]any{
					"spec": map[string]any{"containers": []any{map[string]any{"name": "c"}}},
					"status": map[string]any{
						"phase": "Running",
						"containerStatuses": []any{
							map[string]any{
								"name":  "c",
								"ready": false,
								"state": map[string]any{"waiting": map[string]any{"reason": "CrashLoopBackOff"}},
							},
						},
					},
				},
			},
			want: plugin.Error,
		},
		{
			name: "image pull back off",
			obj: &unstructured.Unstructured{
				Object: map[string]any{
					"spec": map[string]any{"containers": []any{map[string]any{"name": "c"}}},
					"status": map[string]any{
						"phase": "Pending",
						"containerStatuses": []any{
							map[string]any{
								"name":  "c",
								"ready": false,
								"state": map[string]any{"waiting": map[string]any{"reason": "ImagePullBackOff"}},
							},
						},
					},
				},
			},
			want: plugin.Error,
		},
		{"pending", pod("Pending", 0, 1), plugin.Warning},
		{
			name: "container creating",
			obj: &unstructured.Unstructured{
				Object: map[string]any{
					"spec": map[string]any{"containers": []any{map[string]any{"name": "c"}}},
					"status": map[string]any{
						"phase": "Pending",
						"containerStatuses": []any{
							map[string]any{
								"name":  "c",
								"ready": false,
								"state": map[string]any{"waiting": map[string]any{"reason": "ContainerCreating"}},
							},
						},
					},
				},
			},
			want: plugin.Warning,
		},
		{"succeeded", pod("Succeeded", 0, 1), plugin.Healthy},
		{
			name: "completed",
			obj: &unstructured.Unstructured{
				Object: map[string]any{
					"spec": map[string]any{"containers": []any{map[string]any{"name": "c"}}},
					"status": map[string]any{
						"phase": "Failed",
						"containerStatuses": []any{
							map[string]any{
								"name":  "c",
								"ready": false,
								"state": map[string]any{"terminated": map[string]any{"reason": "Completed", "exitCode": int64(0)}},
							},
						},
					},
				},
			},
			want: plugin.Healthy,
		},
		{
			name: "missing status does not panic",
			obj: &unstructured.Unstructured{
				Object: map[string]any{
					"spec": map[string]any{"containers": []any{map[string]any{"name": "c"}}},
				},
			},
			want: plugin.Healthy, // extractPodPhase returns "Unknown" -> default Healthy
		},
	}

	p := New().(*Plugin)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := p.RowHealth(tt.obj)
			if got != tt.want {
				t.Fatalf("RowHealth() = %v, want %v", got, tt.want)
			}
		})
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
