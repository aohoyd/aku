package deployments

import (
	"context"
	"strings"
	"testing"

	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/manifest"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/plugin/plugintest"
	"github.com/aohoyd/aku/internal/render"
	"github.com/charmbracelet/x/ansi"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// mockPlugin implements plugin.ResourcePlugin for testing.
type mockPlugin struct {
	name string
}

func (m *mockPlugin) Name() string      { return m.name }
func (m *mockPlugin) ShortName() string { return m.name[:2] }
func (m *mockPlugin) GVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{}
}
func (m *mockPlugin) IsClusterScoped() bool                     { return false }
func (m *mockPlugin) Columns() []plugin.Column                  { return nil }
func (m *mockPlugin) Row(_ *unstructured.Unstructured) []string { return nil }
func (m *mockPlugin) YAML(_ *unstructured.Unstructured) (render.Content, error) {
	return render.Content{}, nil
}
func (m *mockPlugin) Describe(_ context.Context, _ *unstructured.Unstructured) (render.Content, error) {
	return render.Content{}, nil
}

func TestPluginColumns(t *testing.T) {
	p := New()
	cols := p.Columns()
	if len(cols) != 5 {
		t.Fatalf("expected 5 columns, got %d", len(cols))
	}
}

func TestPluginRow(t *testing.T) {
	p := New()
	obj := makeDeployment("nginx", 3, 3, 3)
	row := p.Row(obj)
	if row[0] != "nginx" {
		t.Fatalf("expected 'nginx', got '%s'", row[0])
	}
}

func TestPluginRowPartial(t *testing.T) {
	p := New()
	obj := makeDeployment("web", 3, 2, 1)
	row := p.Row(obj)
	if row[0] != "web" {
		t.Fatalf("expected 'web', got '%s'", row[0])
	}
	if row[1] != "1/3" {
		t.Fatalf("expected ready '1/3', got '%s'", row[1])
	}
}

func TestPluginName(t *testing.T) {
	p := New()
	if p.Name() != "deployments" {
		t.Fatalf("expected 'deployments', got '%s'", p.Name())
	}
	if p.ShortName() != "deploy" {
		t.Fatalf("expected 'deploy', got '%s'", p.ShortName())
	}
}

func TestPluginDescribeDocument(t *testing.T) {
	p := New()
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"metadata": map[string]any{
				"name":              "nginx-deploy",
				"namespace":         "default",
				"creationTimestamp": "2026-02-24T10:00:00Z",
				"labels":            map[string]any{"app": "nginx"},
			},
			"spec": map[string]any{
				"replicas": int64(3),
				"selector": map[string]any{
					"matchLabels": map[string]any{"app": "nginx"},
				},
				"strategy": map[string]any{
					"type": "RollingUpdate",
					"rollingUpdate": map[string]any{
						"maxUnavailable": "25%",
						"maxSurge":       "25%",
					},
				},
				"template": map[string]any{
					"spec": map[string]any{
						"initContainers": []any{
							map[string]any{
								"name":  "init-schema",
								"image": "migrate:latest",
							},
						},
						"containers": []any{
							map[string]any{
								"name":  "nginx",
								"image": "nginx:1.25",
								"ports": []any{
									map[string]any{"containerPort": int64(80), "protocol": "TCP"},
								},
							},
						},
					},
				},
			},
			"status": map[string]any{
				"replicas":          int64(3),
				"readyReplicas":     int64(3),
				"updatedReplicas":   int64(3),
				"availableReplicas": int64(3),
				"conditions": []any{
					map[string]any{"type": "Available", "status": "True"},
					map[string]any{"type": "Progressing", "status": "True"},
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
		"nginx-deploy", "default",
		"app=nginx",
		"Replicas:", "3",
		"RollingUpdate", "25%",
		"Pod Template:", "nginx:", "nginx:1.25",
		// Init containers
		"Init Containers:", "init-schema:", "migrate:latest",
		"Conditions:", "Available", "True",
	}
	for _, want := range checks {
		if !strings.Contains(c.Raw, want) {
			t.Errorf("describe output should contain %q\n\nFull output:\n%s", want, c.Raw)
		}
	}
}

// TestYAMLShowsManifestProvenance proves the zero-cost provenance surface: an
// object carrying the manifest-source annotation renders that annotation
// (key and value) in the YAML view, so a user viewing a manifest object's YAML
// can see where it came from. MarshalYAML strips managedFields but must keep
// arbitrary annotations.
func TestYAMLShowsManifestProvenance(t *testing.T) {
	p := New()
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]any{
				"name":      "nginx",
				"namespace": "default",
				"annotations": map[string]any{
					manifest.SourceAnnotation: "chart/templates/x.yaml",
				},
				// managedFields must be stripped; a real annotation must survive.
				"managedFields": []any{
					map[string]any{"manager": "helm"},
				},
			},
		},
	}

	c, err := p.YAML(obj)
	if err != nil {
		t.Fatalf("YAML err = %v", err)
	}
	if !strings.Contains(c.Raw, manifest.SourceAnnotation) {
		t.Errorf("YAML output missing annotation key %q; got:\n%s", manifest.SourceAnnotation, c.Raw)
	}
	if !strings.Contains(c.Raw, "chart/templates/x.yaml") {
		t.Errorf("YAML output missing annotation value; got:\n%s", c.Raw)
	}
	if strings.Contains(c.Raw, "managedFields") {
		t.Errorf("YAML output should strip managedFields; got:\n%s", c.Raw)
	}
}

func TestPluginDescribeUncovered(t *testing.T) {
	store := k8s.NewStore(nil, "", nil)
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

	p := New()

	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"metadata": map[string]any{
				"name":              "nginx-deploy",
				"namespace":         "default",
				"creationTimestamp": "2026-02-24T10:00:00Z",
			},
			"spec": map[string]any{
				"replicas": int64(1),
				"selector": map[string]any{"matchLabels": map[string]any{"app": "nginx"}},
				"template": map[string]any{
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
				},
			},
		},
	}

	unc, ok := p.(plugin.Uncoverable)
	if !ok {
		t.Fatal("Plugin should implement Uncoverable")
	}

	c, err := unc.DescribeUncovered(t.Context(), plugintest.NewFakeCluster(store), obj)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Display == "" {
		t.Fatal("display output should not be empty")
	}
	if stripped := ansi.Strip(c.Display); stripped != c.Raw {
		t.Errorf("strip invariant violated: ansi.Strip(c.Display) != raw\nstripped: %q\nraw:      %q",
			stripped, c.Raw)
	}
	// envFrom resolved values should appear with the source configmap.
	if !strings.Contains(c.Raw, "app-config") {
		t.Errorf("expected 'app-config' source in uncovered output\n%s", c.Raw)
	}
	if !strings.Contains(c.Raw, "postgres.svc") {
		t.Errorf("expected resolved configmap value 'postgres.svc' in output\n%s", c.Raw)
	}
	// Direct secret valueFrom should be resolved.
	if !strings.Contains(c.Raw, "secret") {
		t.Errorf("expected resolved secret value 'secret' in output\n%s", c.Raw)
	}
	if strings.Contains(c.Raw, "<key api-key in Secret my-secret>") {
		t.Errorf("should not contain unresolved reference\n%s", c.Raw)
	}
}

func TestPluginDescribeUncoveredNilStore(t *testing.T) {
	// With a nil store, DescribeUncovered must still succeed and return the
	// same output as Describe (falling back to the unresolved env refs).
	p := &Plugin{}
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"metadata": map[string]any{"name": "x", "namespace": "default"},
			"spec":     map[string]any{"template": map[string]any{"spec": map[string]any{}}},
		},
	}
	unc := plugin.ResourcePlugin(p).(plugin.Uncoverable)
	if _, err := unc.DescribeUncovered(t.Context(), plugintest.NewFakeCluster(nil), obj); err != nil {
		t.Fatalf("unexpected error with nil store: %v", err)
	}
}

func TestDeploymentDrillDown(t *testing.T) {
	rsGVR := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "replicasets"}
	store := k8s.NewStore(nil, "", nil)

	plugin.Reset()
	mockRS := &mockPlugin{name: "replicasets"}
	plugin.Register(mockRS)

	rs1 := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{
			"name": "nginx-rs-v1", "namespace": "default",
			"ownerReferences": []any{map[string]any{"uid": "deploy-uid-1"}},
		},
	}}
	rs2 := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{
			"name": "nginx-rs-v2", "namespace": "default",
			"ownerReferences": []any{map[string]any{"uid": "deploy-uid-1"}},
		},
	}}
	store.CacheUpsert(rsGVR, "default", rs1)
	store.CacheUpsert(rsGVR, "default", rs2)

	p := &Plugin{}
	deploy := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{
			"name": "nginx", "namespace": "default", "uid": "deploy-uid-1",
		},
	}}

	childPlugin, children := p.DrillDown(plugintest.NewFakeCluster(store), deploy)
	if childPlugin == nil {
		t.Fatal("expected child plugin, got nil")
	}
	if childPlugin.Name() != "replicasets" {
		t.Fatalf("expected child plugin 'replicasets', got %q", childPlugin.Name())
	}
	if len(children) != 2 {
		t.Fatalf("expected 2 child replicasets, got %d", len(children))
	}
}

func TestDeploymentDrillDownNilStore(t *testing.T) {
	p := &Plugin{}
	deploy := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{"name": "nginx", "namespace": "default", "uid": "deploy-uid-1"},
	}}
	childPlugin, children := p.DrillDown(plugintest.NewFakeCluster(nil), deploy)
	if childPlugin != nil || children != nil {
		t.Fatal("expected nil, nil for nil store")
	}
}

// TestDeploymentDrillDownUsesInjectedCluster proves the multi-cluster guarantee:
// one plugin instance, called with two different plugin.Cluster values, reads the
// children from whichever cluster's store was injected at call time — never a
// store baked in at construction.
func TestDeploymentDrillDownUsesInjectedCluster(t *testing.T) {
	rsGVR := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "replicasets"}

	plugin.Reset()
	plugin.Register(&mockPlugin{name: "replicasets"})

	// Cluster A: one replicaset owned by the deployment.
	storeA := k8s.NewStore(nil, "ctx-a", nil)
	storeA.CacheUpsert(rsGVR, "default", &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{
			"name": "rs-a", "namespace": "default",
			"ownerReferences": []any{map[string]any{"uid": "deploy-uid-1"}},
		},
	}})

	// Cluster B: two replicasets owned by the same-UID deployment.
	storeB := k8s.NewStore(nil, "ctx-b", nil)
	storeB.CacheUpsert(rsGVR, "default", &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{
			"name": "rs-b1", "namespace": "default",
			"ownerReferences": []any{map[string]any{"uid": "deploy-uid-1"}},
		},
	}})
	storeB.CacheUpsert(rsGVR, "default", &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{
			"name": "rs-b2", "namespace": "default",
			"ownerReferences": []any{map[string]any{"uid": "deploy-uid-1"}},
		},
	}})

	// SAME plugin instance for both calls.
	p := &Plugin{}
	deploy := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{"name": "nginx", "namespace": "default", "uid": "deploy-uid-1"},
	}}

	if _, children := p.DrillDown(plugintest.NewFakeCluster(storeA), deploy); len(children) != 1 {
		t.Fatalf("cluster A: expected 1 child from injected store, got %d", len(children))
	}
	if _, children := p.DrillDown(plugintest.NewFakeCluster(storeB), deploy); len(children) != 2 {
		t.Fatalf("cluster B: expected 2 children from injected store, got %d", len(children))
	}
}

func TestDeploymentRowHealth(t *testing.T) {
	availableFalse := []any{
		map[string]any{"type": "Available", "status": "False"},
	}
	progressDeadline := []any{
		map[string]any{"type": "Progressing", "status": "False", "reason": "ProgressDeadlineExceeded"},
	}

	tests := []struct {
		name       string
		desired    int64
		ready      int64
		conditions []any
		noStatus   bool
		want       plugin.Health
	}{
		{name: "all ready", desired: 3, ready: 3, want: plugin.Healthy},
		{name: "partial ready", desired: 3, ready: 1, want: plugin.Warning},
		{name: "available false overrides ready", desired: 3, ready: 3, conditions: availableFalse, want: plugin.Error},
		{name: "available false while unready", desired: 3, ready: 1, conditions: availableFalse, want: plugin.Error},
		{name: "progress deadline exceeded", desired: 3, ready: 1, conditions: progressDeadline, want: plugin.Error},
		{name: "scaled to zero", desired: 0, ready: 0, want: plugin.Healthy},
		{name: "missing status with desired", desired: 2, noStatus: true, want: plugin.Warning},
		{name: "missing status scaled to zero", desired: 0, noStatus: true, want: plugin.Healthy},
	}

	p := New().(plugin.HealthReporter)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := makeHealthDeployment(tt.desired, tt.ready, tt.conditions, tt.noStatus)
			if got := p.RowHealth(obj); got != tt.want {
				t.Fatalf("RowHealth = %v, want %v", got, tt.want)
			}
		})
	}
}

func makeHealthDeployment(desired, ready int64, conditions []any, noStatus bool) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"metadata": map[string]any{"name": "d", "namespace": "default"},
			"spec":     map[string]any{"replicas": desired},
		},
	}
	if !noStatus {
		status := map[string]any{"readyReplicas": ready}
		if conditions != nil {
			status["conditions"] = conditions
		}
		obj.Object["status"] = status
	}
	return obj
}

func makeDeployment(name string, replicas, updated, available int64) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]any{
				"name":              name,
				"namespace":         "default",
				"creationTimestamp": "2024-01-01T00:00:00Z",
			},
			"spec": map[string]any{
				"replicas": replicas,
			},
			"status": map[string]any{
				"replicas":          replicas,
				"updatedReplicas":   updated,
				"readyReplicas":     available,
				"availableReplicas": available,
			},
		},
	}
	return obj
}
