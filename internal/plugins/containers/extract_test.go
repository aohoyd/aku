package containers

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func testPod() *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"metadata": map[string]any{
				"name":              "nginx-abc",
				"namespace":         "default",
				"creationTimestamp": "2026-02-24T10:00:00Z",
			},
			"spec": map[string]any{
				"containers": []any{
					map[string]any{
						"name":  "nginx",
						"image": "nginx:1.25",
						"ports": []any{
							map[string]any{"containerPort": int64(80)},
						},
					},
					map[string]any{
						"name":  "sidecar",
						"image": "envoy:latest",
					},
				},
				"initContainers": []any{
					map[string]any{
						"name":  "init-db",
						"image": "busybox:1.36",
					},
				},
				"ephemeralContainers": []any{
					map[string]any{
						"name":  "debugger",
						"image": "busybox:latest",
					},
				},
			},
			"status": map[string]any{
				"containerStatuses": []any{
					map[string]any{
						"name":         "nginx",
						"ready":        true,
						"restartCount": int64(2),
						"state":        map[string]any{"running": map[string]any{"startedAt": "2026-02-24T10:01:00Z"}},
					},
					map[string]any{
						"name":         "sidecar",
						"ready":        true,
						"restartCount": int64(0),
						"state":        map[string]any{"running": map[string]any{"startedAt": "2026-02-24T10:01:00Z"}},
					},
				},
				"initContainerStatuses": []any{
					map[string]any{
						"name":         "init-db",
						"ready":        false,
						"restartCount": int64(0),
						"state":        map[string]any{"terminated": map[string]any{"reason": "Completed", "exitCode": int64(0)}},
					},
				},
				"ephemeralContainerStatuses": []any{
					map[string]any{
						"name":         "debugger",
						"ready":        false,
						"restartCount": int64(0),
						"state":        map[string]any{"running": map[string]any{"startedAt": "2026-02-24T11:00:00Z"}},
					},
				},
			},
		},
	}
}

func TestExtractContainersCount(t *testing.T) {
	pod := testPod()
	containers := ExtractContainers(pod)
	// 2 regular + 1 init + 1 ephemeral = 4
	if len(containers) != 4 {
		t.Fatalf("expected 4 containers, got %d", len(containers))
	}
}

func TestExtractContainersTypes(t *testing.T) {
	pod := testPod()
	containers := ExtractContainers(pod)

	types := make(map[string]int)
	for _, c := range containers {
		ct, _, _ := unstructured.NestedString(c.Object, "_type")
		types[ct]++
	}
	if types["regular"] != 2 {
		t.Fatalf("expected 2 regular, got %d", types["regular"])
	}
	if types["init"] != 1 {
		t.Fatalf("expected 1 init, got %d", types["init"])
	}
	if types["ephemeral"] != 1 {
		t.Fatalf("expected 1 ephemeral, got %d", types["ephemeral"])
	}
}

func TestExtractContainersMetadata(t *testing.T) {
	pod := testPod()
	containers := ExtractContainers(pod)

	// First container should be "nginx"
	first := containers[0]
	if first.GetName() != "nginx" {
		t.Fatalf("expected name 'nginx', got %q", first.GetName())
	}
	if first.GetNamespace() != "default" {
		t.Fatalf("expected namespace 'default', got %q", first.GetNamespace())
	}
}

func TestExtractContainersSpec(t *testing.T) {
	pod := testPod()
	containers := ExtractContainers(pod)
	first := containers[0]

	spec, ok := first.Object["_spec"].(map[string]any)
	if !ok {
		t.Fatal("_spec should be a map")
	}
	image, _ := spec["image"].(string)
	if image != "nginx:1.25" {
		t.Fatalf("expected image 'nginx:1.25', got %q", image)
	}
}

func TestExtractContainersStatus(t *testing.T) {
	pod := testPod()
	containers := ExtractContainers(pod)
	first := containers[0]

	status, ok := first.Object["_status"].(map[string]any)
	if !ok {
		t.Fatal("_status should be a map")
	}
	ready, _, _ := unstructured.NestedBool(status, "ready")
	if !ready {
		t.Fatal("nginx container should be ready")
	}
	restarts, _, _ := unstructured.NestedInt64(status, "restartCount")
	if restarts != 2 {
		t.Fatalf("expected 2 restarts, got %d", restarts)
	}
}

func TestExtractContainersPodRef(t *testing.T) {
	pod := testPod()
	containers := ExtractContainers(pod)
	first := containers[0]

	podObj, ok := first.Object["_pod"].(map[string]any)
	if !ok {
		t.Fatal("_pod should be a map")
	}
	name, _, _ := unstructured.NestedString(podObj, "metadata", "name")
	if name != "nginx-abc" {
		t.Fatalf("expected pod name 'nginx-abc', got %q", name)
	}
	if _, exists := first.Object["_podName"]; exists {
		t.Fatal("_podName should no longer exist, use _pod instead")
	}
}

func TestExtractContainersEmptyPod(t *testing.T) {
	pod := &unstructured.Unstructured{
		Object: map[string]any{
			"metadata": map[string]any{"name": "empty-pod", "namespace": "default"},
			"spec":     map[string]any{},
		},
	}
	containers := ExtractContainers(pod)
	if len(containers) != 0 {
		t.Fatalf("expected 0 containers for empty pod, got %d", len(containers))
	}
}

func TestExtractContainersMissingStatus(t *testing.T) {
	pod := &unstructured.Unstructured{
		Object: map[string]any{
			"metadata": map[string]any{"name": "pending-pod", "namespace": "default"},
			"spec": map[string]any{
				"containers": []any{
					map[string]any{"name": "app", "image": "nginx"},
				},
			},
		},
	}
	containers := ExtractContainers(pod)
	if len(containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(containers))
	}
	// _status should be nil when no status exists
	if containers[0].Object["_status"] != nil {
		t.Fatal("_status should be nil for container with no status")
	}
}
