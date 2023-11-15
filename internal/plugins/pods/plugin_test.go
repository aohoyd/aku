package pods

import (
	"strings"
	"testing"

	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/plugins/containers"
	"github.com/charmbracelet/x/ansi"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestPluginMetadata(t *testing.T) {
	p := New(nil, nil)
	if p.Name() != "pods" {
		t.Fatalf("expected 'pods', got '%s'", p.Name())
	}
	if p.ShortName() != "po" {
		t.Fatalf("expected 'po', got '%s'", p.ShortName())
	}
	if p.GVR().Resource != "pods" {
		t.Fatalf("expected GVR resource 'pods', got '%s'", p.GVR().Resource)
	}
}

func TestPluginColumns(t *testing.T) {
	p := New(nil, nil)
	cols := p.Columns()
	if len(cols) != 5 {
		t.Fatalf("expected 5 columns, got %d", len(cols))
	}
	if cols[0].Title != "NAME" {
		t.Fatalf("expected first column 'NAME', got '%s'", cols[0].Title)
	}
}

func TestPluginRow(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"metadata": map[string]any{
				"name":              "test-pod",
				"namespace":         "default",
				"creationTimestamp": "2026-02-24T10:00:00Z",
			},
			"status": map[string]any{
				"phase": "Running",
				"containerStatuses": []any{
					map[string]any{
						"ready":        true,
						"restartCount": int64(0),
					},
				},
			},
			"spec": map[string]any{
				"containers": []any{
					map[string]any{"name": "main"},
				},
			},
		},
	}
	row := p.Row(obj)
	if row[0] != "test-pod" {
		t.Fatalf("expected name 'test-pod', got '%s'", row[0])
	}
	if row[1] != "1/1" {
		t.Fatalf("expected ready '1/1', got '%s'", row[1])
	}
	if ansi.Strip(row[2]) != "Running" {
		t.Fatalf("expected status 'Running', got '%s'", ansi.Strip(row[2]))
	}
	if row[3] != "0" {
		t.Fatalf("expected restarts '0', got '%s'", row[3])
	}
}

func TestPluginYAML(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Pod",
			"metadata":   map[string]any{"name": "test"},
		},
	}
	c, err := p.YAML(obj)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Raw == "" {
		t.Fatal("YAML should not be empty")
	}
}

func TestPluginCrashLoopBackOff(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"status": map[string]any{
				"phase": "Running",
				"containerStatuses": []any{
					map[string]any{
						"state": map[string]any{
							"waiting": map[string]any{
								"reason": "CrashLoopBackOff",
							},
						},
						"ready":        false,
						"restartCount": int64(5),
					},
				},
			},
			"spec": map[string]any{
				"containers": []any{
					map[string]any{"name": "main"},
				},
			},
		},
	}
	row := p.Row(obj)
	if ansi.Strip(row[2]) != "CrashLoopBackOff" {
		t.Fatalf("expected 'CrashLoopBackOff', got '%s'", ansi.Strip(row[2]))
	}
	if row[3] != "5" {
		t.Fatalf("expected restarts '5', got '%s'", row[3])
	}
}

func TestPluginSortValueStatus(t *testing.T) {
	p := New(nil, nil).(*Plugin)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"status": map[string]any{
				"phase": "Running",
			},
		},
	}
	v := p.SortValue(obj, "STATUS")
	if v != "Running" {
		t.Fatalf("expected 'Running', got %q", v)
	}
}

func TestPluginSortValueFallback(t *testing.T) {
	p := New(nil, nil).(*Plugin)
	obj := &unstructured.Unstructured{
		Object: map[string]any{},
	}
	// NAME and AGE return "" to fall back to built-in handling
	if v := p.SortValue(obj, "NAME"); v != "" {
		t.Fatalf("expected empty for NAME (built-in fallback), got %q", v)
	}
	if v := p.SortValue(obj, "AGE"); v != "" {
		t.Fatalf("expected empty for AGE (built-in fallback), got %q", v)
	}
	if v := p.SortValue(obj, "UNKNOWN"); v != "" {
		t.Fatalf("expected empty for unknown column, got %q", v)
	}
}

func TestPluginDescribeDocument(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"metadata": map[string]any{
				"name":              "test-pod",
				"namespace":         "default",
				"creationTimestamp": "2026-02-24T10:00:00Z",
				"labels":            map[string]any{"app": "nginx"},
				"annotations":       map[string]any{"note": "test"},
				"ownerReferences": []any{
					map[string]any{"kind": "ReplicaSet", "name": "nginx-abc123"},
				},
			},
			"spec": map[string]any{
				"nodeName":           "node-1",
				"serviceAccountName": "default",
				"containers": []any{
					map[string]any{
						"name":  "nginx",
						"image": "nginx:1.25",
						"ports": []any{
							map[string]any{"containerPort": int64(80), "protocol": "TCP", "name": "http"},
							map[string]any{"containerPort": int64(9090), "protocol": "TCP", "name": "metrics"},
						},
						"resources": map[string]any{
							"limits":   map[string]any{"cpu": "500m", "memory": "128Mi"},
							"requests": map[string]any{"cpu": "250m", "memory": "64Mi"},
						},
						"env": []any{
							map[string]any{"name": "LOG_LEVEL", "value": "DEBUG"},
							map[string]any{"name": "DB_HOST", "value": "localhost"},
							map[string]any{
								"name": "SECRET_KEY",
								"valueFrom": map[string]any{
									"secretKeyRef": map[string]any{"name": "my-secret", "key": "api-key"},
								},
							},
						},
						"envFrom": []any{
							map[string]any{
								"configMapRef": map[string]any{"name": "app-config"},
							},
						},
						"livenessProbe": map[string]any{
							"httpGet": map[string]any{
								"path": "/healthz", "port": int64(8080), "scheme": "HTTP",
							},
							"initialDelaySeconds": int64(10),
							"timeoutSeconds":      int64(5),
							"periodSeconds":       int64(30),
							"successThreshold":    int64(1),
							"failureThreshold":    int64(3),
						},
						"readinessProbe": map[string]any{
							"tcpSocket":     map[string]any{"port": int64(80)},
							"periodSeconds": int64(10),
						},
					},
				},
				"initContainers": []any{
					map[string]any{
						"name":  "init-db",
						"image": "busybox:1.36",
						"env": []any{
							map[string]any{"name": "DB_URL", "value": "postgres://db:5432"},
						},
					},
				},
				"ephemeralContainers": []any{
					map[string]any{
						"name":  "debugger",
						"image": "busybox:latest",
					},
				},
				"volumes": []any{
					map[string]any{
						"name":      "config",
						"configMap": map[string]any{"name": "my-config"},
					},
				},
				"tolerations": []any{
					map[string]any{
						"key":               "node.kubernetes.io/not-ready",
						"operator":          "Exists",
						"effect":            "NoExecute",
						"tolerationSeconds": int64(300),
					},
				},
			},
			"status": map[string]any{
				"phase":    "Running",
				"podIP":    "10.0.0.5",
				"hostIP":   "192.168.1.1",
				"qosClass": "Burstable",
				"containerStatuses": []any{
					map[string]any{
						"name":         "nginx",
						"ready":        true,
						"restartCount": int64(2),
						"state": map[string]any{
							"running": map[string]any{
								"startedAt": "2026-02-24T10:01:00Z",
							},
						},
						"lastState": map[string]any{
							"terminated": map[string]any{
								"reason":     "OOMKilled",
								"exitCode":   int64(137),
								"startedAt":  "2026-02-24T09:50:00Z",
								"finishedAt": "2026-02-24T10:00:00Z",
							},
						},
					},
				},
				"initContainerStatuses": []any{
					map[string]any{
						"name":         "init-db",
						"ready":        false,
						"restartCount": int64(0),
						"state": map[string]any{
							"terminated": map[string]any{
								"reason":   "Completed",
								"exitCode": int64(0),
							},
						},
					},
				},
				"ephemeralContainerStatuses": []any{
					map[string]any{
						"name":         "debugger",
						"ready":        false,
						"restartCount": int64(0),
						"state": map[string]any{
							"running": map[string]any{
								"startedAt": "2026-02-24T11:00:00Z",
							},
						},
					},
				},
				"conditions": []any{
					map[string]any{"type": "Ready", "status": "True"},
					map[string]any{"type": "PodScheduled", "status": "True"},
				},
			},
		},
	}

	c, err := p.Describe(t.Context(), obj)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Raw == "" {
		t.Fatal("raw output should not be empty")
	}
	if c.Display == "" {
		t.Fatal("display output should not be empty")
	}
	if stripped := ansi.Strip(c.Display); stripped != c.Raw {
		t.Errorf("strip invariant violated: ansi.Strip(c.Display) != raw\nstripped: %q\nraw:      %q", stripped, c.Raw)
	}

	checks := []string{
		"test-pod", "default", "node-1", "Running", "10.0.0.5",
		"default",
		"Burstable",
		"ReplicaSet/nginx-abc123",
		"app=nginx",
		"Containers:", "nginx:", "nginx:1.25",
		"Ports:", "80/TCP (http)", "9090/TCP (metrics)",
		// Probes
		"Liveness:", "http-get http://:8080/healthz", "delay=10s", "timeout=5s",
		"Readiness:", "tcp-socket :80", "period=10s",
		// Environment Variables from
		"Environment Variables from:",
		"app-config", "ConfigMap  Optional: false",
		// Environment as kv pairs
		"Environment:",
		"LOG_LEVEL:", "DEBUG",
		"DB_HOST:", "localhost",
		"SECRET_KEY:", "<key api-key in Secret my-secret>",
		// Last state
		"Last State:", "Terminated",
		"OOMKilled", "137",
		// Init containers
		"Init Containers:", "init-db:", "busybox:1.36",
		"DB_URL:", "postgres://db:5432",
		"Completed",
		// Ephemeral containers
		"Ephemeral Containers:", "debugger:", "busybox:latest",
		"Conditions:", "Ready",
		"Volumes:", "config:", "ConfigMap",
		"Tolerations:",
	}
	for _, want := range checks {
		if !strings.Contains(c.Raw, want) {
			t.Errorf("describe output should contain %q\n\nFull output:\n%s", want, c.Raw)
		}
	}
}

func TestPluginDescribeUncovered(t *testing.T) {
	store := k8s.NewStore(nil, nil)
	cmGVR := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}
	secGVR := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "secrets"}

	store.CacheUpsert(cmGVR, "default", &unstructured.Unstructured{
		Object: map[string]any{
			"metadata": map[string]any{"name": "app-config", "namespace": "default"},
			"data":     map[string]any{"DB_HOST": "postgres.svc"},
		},
	})
	store.CacheUpsert(secGVR, "default", &unstructured.Unstructured{
		Object: map[string]any{
			"metadata": map[string]any{"name": "my-secret", "namespace": "default"},
			"data":     map[string]any{"api-key": "c2VjcmV0"},
		},
	})

	p := New(nil, store)

	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"metadata": map[string]any{
				"name":              "test-pod",
				"namespace":         "default",
				"creationTimestamp": "2026-02-24T10:00:00Z",
			},
			"spec": map[string]any{
				"containers": []any{
					map[string]any{
						"name":  "app",
						"image": "nginx",
						"envFrom": []any{
							map[string]any{
								"configMapRef": map[string]any{"name": "app-config"},
							},
						},
						"env": []any{
							map[string]any{
								"name": "API_KEY",
								"valueFrom": map[string]any{
									"secretKeyRef": map[string]any{"name": "my-secret", "key": "api-key"},
								},
							},
						},
					},
				},
			},
			"status": map[string]any{
				"phase": "Running",
			},
		},
	}

	unc, ok := p.(plugin.Uncoverable)
	if !ok {
		t.Fatal("Plugin should implement Uncoverable")
	}

	c, err := unc.DescribeUncovered(t.Context(), obj)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Display == "" {
		t.Fatal("display output should not be empty")
	}
	if stripped := ansi.Strip(c.Display); stripped != c.Raw {
		t.Errorf("strip invariant violated: ansi.Strip(c.Display) != raw\nstripped: %q\nraw:      %q", stripped, c.Raw)
	}

	// envFrom resolved values should appear nested under source
	if !strings.Contains(c.Raw, "Environment Variables from:") {
		t.Errorf("expected 'Environment Variables from:' section in uncovered output\n%s", c.Raw)
	}
	if !strings.Contains(c.Raw, "app-config") {
		t.Errorf("expected 'app-config' source in uncovered output\n%s", c.Raw)
	}
	if !strings.Contains(c.Raw, "postgres.svc") {
		t.Errorf("expected resolved configmap value 'postgres.svc' in output\n%s", c.Raw)
	}

	// Direct valueFrom should still be resolved
	if !strings.Contains(c.Raw, "secret") {
		t.Errorf("expected resolved secret value 'secret' in output\n%s", c.Raw)
	}
	if strings.Contains(c.Raw, "<key api-key in Secret my-secret>") {
		t.Errorf("should not contain unresolved reference\n%s", c.Raw)
	}
}

func TestPluginImplementsDrillDowner(t *testing.T) {
	p := New(nil, nil)
	_, ok := p.(plugin.DrillDowner)
	if !ok {
		t.Fatal("Plugin should implement DrillDowner")
	}
}

func TestPluginDrillDown(t *testing.T) {
	plugin.Reset()
	plugin.Register(containers.New(nil, nil))
	p := New(nil, nil)
	drillable := p.(plugin.DrillDowner)
	pod := &unstructured.Unstructured{
		Object: map[string]any{
			"metadata": map[string]any{"name": "test-pod", "namespace": "default"},
			"spec": map[string]any{
				"containers": []any{
					map[string]any{"name": "nginx", "image": "nginx:1.25"},
					map[string]any{"name": "sidecar", "image": "envoy"},
				},
			},
			"status": map[string]any{
				"containerStatuses": []any{
					map[string]any{"name": "nginx", "ready": true, "restartCount": int64(0), "state": map[string]any{"running": map[string]any{}}},
					map[string]any{"name": "sidecar", "ready": true, "restartCount": int64(0), "state": map[string]any{"running": map[string]any{}}},
				},
			},
		},
	}
	childPlugin, children := drillable.DrillDown(pod)
	if childPlugin == nil {
		t.Fatal("DrillDown should return a child plugin")
	}
	if childPlugin.Name() != "containers" {
		t.Fatalf("expected child plugin 'containers', got %q", childPlugin.Name())
	}
	if len(children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(children))
	}
}

func TestDescribeShowsContainerConfigError(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"metadata": map[string]any{
				"name":              "bad-pod",
				"namespace":         "default",
				"creationTimestamp": "2026-02-24T10:00:00Z",
			},
			"spec": map[string]any{
				"containers": []any{
					map[string]any{"name": "app", "image": "nginx"},
				},
			},
			"status": map[string]any{
				"phase": "Running",
				"containerStatuses": []any{
					map[string]any{
						"name":  "app",
						"ready": false,
						"state": map[string]any{
							"waiting": map[string]any{"reason": "CreateContainerConfigError"},
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
	if !strings.Contains(c.Raw, "CreateContainerConfigError") {
		t.Errorf("describe should show CreateContainerConfigError status\n%s", c.Raw)
	}
}

func TestComputePodStatus(t *testing.T) {
	tests := []struct {
		name      string
		status    corev1.PodStatus
		initTotal int
		deleted   bool
		want      string
	}{
		{
			name: "normal running",
			status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{Ready: true, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
				},
			},
			want: "Running",
		},
		{
			name: "CreateContainerConfigError",
			status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CreateContainerConfigError"}}},
				},
			},
			want: "CreateContainerConfigError",
		},
		{
			name: "CrashLoopBackOff",
			status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}}},
				},
			},
			want: "CrashLoopBackOff",
		},
		{
			name: "ImagePullBackOff",
			status: corev1.PodStatus{
				Phase: corev1.PodPending,
				ContainerStatuses: []corev1.ContainerStatus{
					{State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff"}}},
				},
			},
			want: "ImagePullBackOff",
		},
		{
			name: "init container waiting",
			status: corev1.PodStatus{
				Phase: corev1.PodPending,
				InitContainerStatuses: []corev1.ContainerStatus{
					{State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}}},
				},
			},
			initTotal: 1,
			want:      "Init:CrashLoopBackOff",
		},
		{
			name: "init container terminated error",
			status: corev1.PodStatus{
				Phase: corev1.PodPending,
				InitContainerStatuses: []corev1.ContainerStatus{
					{State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 1}}},
				},
			},
			initTotal: 1,
			want:      "Init:ExitCode:1",
		},
		{
			name: "init container in progress",
			status: corev1.PodStatus{
				Phase: corev1.PodPending,
				InitContainerStatuses: []corev1.ContainerStatus{
					{State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 0}}},
					{State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "PodInitializing"}}},
				},
			},
			initTotal: 3,
			want:      "Init:1/3",
		},
		{
			name: "terminated with reason",
			status: corev1.PodStatus{
				Phase: corev1.PodFailed,
				ContainerStatuses: []corev1.ContainerStatus{
					{State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "OOMKilled", ExitCode: 137}}},
				},
			},
			want: "OOMKilled",
		},
		{
			name: "terminated with signal",
			status: corev1.PodStatus{
				Phase: corev1.PodFailed,
				ContainerStatuses: []corev1.ContainerStatus{
					{State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Signal: 9}}},
				},
			},
			want: "Signal:9",
		},
		{
			name: "terminated with exit code",
			status: corev1.PodStatus{
				Phase: corev1.PodFailed,
				ContainerStatuses: []corev1.ContainerStatus{
					{State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{ExitCode: 1}}},
				},
			},
			want: "ExitCode:1",
		},
		{
			name: "evicted",
			status: corev1.PodStatus{
				Phase:  corev1.PodFailed,
				Reason: "Evicted",
			},
			want: "Evicted",
		},
		{
			name: "terminating",
			status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{Ready: true, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
				},
			},
			deleted: true,
			want:    "Terminating",
		},
		{
			name: "completed with running ready",
			status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "Completed", ExitCode: 0}}},
					{Ready: true, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
				},
				Conditions: []corev1.PodCondition{
					{Type: corev1.PodReady, Status: corev1.ConditionTrue},
				},
			},
			want: "Running",
		},
		{
			name: "completed with running not ready",
			status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{State: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "Completed", ExitCode: 0}}},
					{Ready: false, State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
				},
			},
			want: "NotReady",
		},
		{
			name:   "unknown phase when empty",
			status: corev1.PodStatus{},
			want:   "Unknown",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computePodStatus(tt.status, tt.initTotal, tt.deleted)
			if got != tt.want {
				t.Fatalf("computePodStatus() = %q, want %q", got, tt.want)
			}
		})
	}
}
