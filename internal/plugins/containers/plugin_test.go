package containers

import (
	"strings"
	"testing"

	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/charmbracelet/x/ansi"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestPluginMetadata(t *testing.T) {
	p := New(nil, nil)
	if p.Name() != "containers" {
		t.Fatalf("expected 'containers', got %q", p.Name())
	}
	if p.ShortName() != "co" {
		t.Fatalf("expected 'co', got %q", p.ShortName())
	}
	if p.GVR().Resource != "containers" {
		t.Fatalf("expected GVR resource 'containers', got %q", p.GVR().Resource)
	}
}

func TestPluginColumns(t *testing.T) {
	p := New(nil, nil)
	cols := p.Columns()
	if len(cols) != 6 {
		t.Fatalf("expected 6 columns, got %d", len(cols))
	}
	names := []string{"NAME", "TYPE", "IMAGE", "STATUS", "READY", "RESTARTS"}
	for i, want := range names {
		if cols[i].Title != want {
			t.Fatalf("column %d: expected %q, got %q", i, want, cols[i].Title)
		}
	}
}

func TestPluginRow(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"metadata": map[string]any{"name": "nginx", "namespace": "default"},
			"_type":    "regular",
			"_spec":    map[string]any{"name": "nginx", "image": "nginx:1.25"},
			"_status": map[string]any{
				"ready":        true,
				"restartCount": int64(2),
				"state":        map[string]any{"running": map[string]any{"startedAt": "2026-02-24T10:01:00Z"}},
			},
			"_pod": map[string]any{
				"metadata": map[string]any{"name": "nginx-abc", "namespace": "default"},
			},
		},
	}
	row := p.Row(obj)
	if len(row) != 6 {
		t.Fatalf("expected 6 cells, got %d", len(row))
	}
	if row[0] != "nginx" {
		t.Fatalf("expected name 'nginx', got %q", row[0])
	}
	if row[1] != "regular" {
		t.Fatalf("expected type 'regular', got %q", row[1])
	}
	if row[2] != "nginx:1.25" {
		t.Fatalf("expected image 'nginx:1.25', got %q", row[2])
	}
	if ansi.Strip(row[3]) != "Running" {
		t.Fatalf("expected status 'Running', got %q", ansi.Strip(row[3]))
	}
	if row[4] != "true" {
		t.Fatalf("expected ready 'true', got %q", row[4])
	}
	if row[5] != "2" {
		t.Fatalf("expected restarts '2', got %q", row[5])
	}
}

func TestPluginRowMissingStatus(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"metadata": map[string]any{"name": "app"},
			"_type":    "regular",
			"_spec":    map[string]any{"name": "app", "image": "nginx"},
			"_pod": map[string]any{
				"metadata": map[string]any{"name": "pod-1", "namespace": "default"},
			},
		},
	}
	row := p.Row(obj)
	if ansi.Strip(row[3]) != "Pending" {
		t.Fatalf("expected status 'Pending', got %q", ansi.Strip(row[3]))
	}
	if row[4] != "false" {
		t.Fatalf("expected ready 'false', got %q", row[4])
	}
	if row[5] != "0" {
		t.Fatalf("expected restarts '0', got %q", row[5])
	}
}

func TestPluginYAML(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"metadata": map[string]any{"name": "nginx"},
			"_spec": map[string]any{
				"name":  "nginx",
				"image": "nginx:1.25",
				"ports": []any{map[string]any{"containerPort": int64(80)}},
			},
		},
	}
	c, err := p.YAML(obj)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(c.Raw, "nginx:1.25") {
		t.Fatal("YAML should contain the image")
	}
	if !strings.Contains(c.Raw, "containerPort") {
		t.Fatal("YAML should contain port info")
	}
}

func TestPluginDescribe(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"metadata": map[string]any{"name": "nginx", "namespace": "default"},
			"_type":    "regular",
			"_pod": map[string]any{
				"metadata": map[string]any{"name": "nginx-abc", "namespace": "default"},
				"status":   map[string]any{"podIP": "10.0.0.5"},
			},
			"_spec": map[string]any{
				"name":  "nginx",
				"image": "nginx:1.25",
				"ports": []any{
					map[string]any{"containerPort": int64(80), "protocol": "TCP"},
				},
				"env": []any{
					map[string]any{"name": "LOG_LEVEL", "value": "DEBUG"},
					map[string]any{
						"name": "POD_IP",
						"valueFrom": map[string]any{
							"fieldRef": map[string]any{"fieldPath": "status.podIP"},
						},
					},
				},
				"resources": map[string]any{
					"limits": map[string]any{"cpu": "500m", "memory": "128Mi"},
				},
			},
			"_status": map[string]any{
				"ready":        true,
				"restartCount": int64(1),
				"state":        map[string]any{"running": map[string]any{"startedAt": "2026-02-24T10:01:00Z"}},
			},
		},
	}
	c, err := p.Describe(t.Context(), obj)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Raw == "" || c.Display == "" {
		t.Fatal("describe output should not be empty")
	}

	checks := []string{
		"nginx", "nginx-abc", "default", "regular",
		"nginx:1.25", "80/TCP",
		"Running", "Ready", "true",
		"Restart Count", "1",
		"LOG_LEVEL", "DEBUG",
		"cpu: 500m",
		"10.0.0.5", // FieldRef status.podIP should resolve from embedded pod
	}
	for _, want := range checks {
		if !strings.Contains(c.Raw, want) {
			t.Errorf("describe output should contain %q\n\nFull output:\n%s", want, c.Raw)
		}
	}
	// FieldRef should be resolved, not shown as a reference
	if strings.Contains(c.Raw, "FieldRef") {
		t.Errorf("FieldRef should be resolved, not shown as reference\n\nFull output:\n%s", c.Raw)
	}
}

func TestPluginSortValue(t *testing.T) {
	p := New(nil, nil).(*Plugin)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"_type": "init",
			"_status": map[string]any{
				"state": map[string]any{"running": map[string]any{}},
			},
		},
	}
	if v := p.SortValue(obj, "TYPE"); v != "init" {
		t.Fatalf("expected 'init', got %q", v)
	}
	if v := p.SortValue(obj, "STATUS"); v != "Running" {
		t.Fatalf("expected 'Running', got %q", v)
	}
	// NAME falls back to built-in
	if v := p.SortValue(obj, "NAME"); v != "" {
		t.Fatalf("expected empty for NAME, got %q", v)
	}
}

func TestPluginImplementsInterfaces(t *testing.T) {
	p := New(nil, nil)
	if _, ok := p.(plugin.Sortable); !ok {
		t.Fatal("Plugin should implement Sortable")
	}
	if _, ok := p.(plugin.Uncoverable); !ok {
		t.Fatal("Plugin should implement Uncoverable")
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

	// Build a synthetic container object with _pod embedded
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"metadata": map[string]any{"name": "app", "namespace": "default"},
			"_type":    "regular",
			"_pod": map[string]any{
				"metadata": map[string]any{
					"name":      "test-pod",
					"namespace": "default",
				},
				"spec":   map[string]any{},
				"status": map[string]any{"podIP": "10.0.0.1"},
			},
			"_spec": map[string]any{
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
					map[string]any{
						"name": "POD_IP",
						"valueFrom": map[string]any{
							"fieldRef": map[string]any{"fieldPath": "status.podIP"},
						},
					},
				},
			},
			"_status": map[string]any{
				"ready":        true,
				"restartCount": int64(0),
				"state":        map[string]any{"running": map[string]any{}},
			},
		},
	}

	unc := p.(plugin.Uncoverable)
	c, err := unc.DescribeUncovered(t.Context(), obj)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Display == "" {
		t.Fatal("display output should not be empty")
	}

	// ConfigMap envFrom should be resolved
	if !strings.Contains(c.Raw, "postgres.svc") {
		t.Errorf("expected resolved configmap value 'postgres.svc' in output\n%s", c.Raw)
	}

	// Secret valueFrom should be resolved
	if !strings.Contains(c.Raw, "secret") {
		t.Errorf("expected resolved secret value 'secret' in output\n%s", c.Raw)
	}
	if strings.Contains(c.Raw, "<key api-key in Secret my-secret>") {
		t.Errorf("should not contain unresolved reference\n%s", c.Raw)
	}

	// FieldRef should be resolved via embedded pod
	if !strings.Contains(c.Raw, "10.0.0.1") {
		t.Errorf("expected resolved FieldRef value '10.0.0.1' in output\n%s", c.Raw)
	}
}
