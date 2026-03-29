package helmreleases

import (
	"context"
	"strings"
	"testing"

	"github.com/aohoyd/aku/internal/helm"
	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/render"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func newTestHelmmanifest() *helmmanifest {
	return &helmmanifest{}
}

func TestHelmmanifestColumns(t *testing.T) {
	p := newTestHelmmanifest()
	cols := p.Columns()
	if len(cols) != 3 {
		t.Fatalf("expected 3 columns, got %d", len(cols))
	}
	expected := []string{"KIND", "NAME", "NAMESPACE"}
	for i, col := range cols {
		if col.Title != expected[i] {
			t.Fatalf("column %d: expected %s, got %s", i, expected[i], col.Title)
		}
	}
}

func TestHelmmanifestRow(t *testing.T) {
	p := newTestHelmmanifest()
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Service",
		"metadata":   map[string]any{"name": "nginx-svc", "namespace": "default"},
	}}
	row := p.Row(obj)
	if row[0] != "Service" || row[1] != "nginx-svc" || row[2] != "default" {
		t.Fatalf("unexpected row: %v", row)
	}
}

func TestHelmmanifestYAML(t *testing.T) {
	p := newTestHelmmanifest()
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Service",
		"metadata":   map[string]any{"name": "nginx-svc"},
		"_raw":       "apiVersion: v1\nkind: Service\nmetadata:\n  name: nginx-svc\n",
	}}
	c, err := p.YAML(obj)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Raw == "" {
		t.Fatal("expected non-empty raw YAML")
	}
}

func TestHelmmanifestSortValueKnownKinds(t *testing.T) {
	p := newTestHelmmanifest()

	tests := []struct {
		kindA, kindB string
	}{
		{"Namespace", "ConfigMap"},
		{"ConfigMap", "Service"},
		{"Service", "Deployment"},
		{"Deployment", "Ingress"},
	}
	for _, tt := range tests {
		objA := &unstructured.Unstructured{Object: map[string]any{
			"kind": tt.kindA, "metadata": map[string]any{"name": "a"},
		}}
		objB := &unstructured.Unstructured{Object: map[string]any{
			"kind": tt.kindB, "metadata": map[string]any{"name": "b"},
		}}
		valA := p.SortValue(objA, "KIND")
		valB := p.SortValue(objB, "KIND")
		if valA >= valB {
			t.Errorf("%s (%s) should sort before %s (%s)", tt.kindA, valA, tt.kindB, valB)
		}
	}
}

func TestHelmmanifestSortValueUnknownKindsAfterKnown(t *testing.T) {
	p := newTestHelmmanifest()
	known := &unstructured.Unstructured{Object: map[string]any{
		"kind": "Ingress", "metadata": map[string]any{"name": "a"},
	}}
	unknown := &unstructured.Unstructured{Object: map[string]any{
		"kind": "MyCRD", "metadata": map[string]any{"name": "b"},
	}}
	valKnown := p.SortValue(known, "KIND")
	valUnknown := p.SortValue(unknown, "KIND")
	if valKnown >= valUnknown {
		t.Errorf("known kind (%s) should sort before unknown kind (%s)", valKnown, valUnknown)
	}
}

func TestHelmmanifestSortValueUnknownKindsAlphabetical(t *testing.T) {
	p := newTestHelmmanifest()
	objA := &unstructured.Unstructured{Object: map[string]any{
		"kind": "AlphaCRD", "metadata": map[string]any{"name": "a"},
	}}
	objB := &unstructured.Unstructured{Object: map[string]any{
		"kind": "BetaCRD", "metadata": map[string]any{"name": "b"},
	}}
	valA := p.SortValue(objA, "KIND")
	valB := p.SortValue(objB, "KIND")
	if valA >= valB {
		t.Errorf("AlphaCRD (%s) should sort before BetaCRD (%s)", valA, valB)
	}
}

func TestHelmmanifestSortValueNonKindColumn(t *testing.T) {
	p := newTestHelmmanifest()
	obj := &unstructured.Unstructured{Object: map[string]any{
		"kind": "Service", "metadata": map[string]any{"name": "a"},
	}}
	if v := p.SortValue(obj, "NAME"); v != "" {
		t.Errorf("expected empty string for NAME column, got %s", v)
	}
}

func TestHelmmanifestDefaultSort(t *testing.T) {
	p := newTestHelmmanifest()
	pref := p.DefaultSort()
	if pref.Column != "KIND" {
		t.Errorf("expected default sort column KIND, got %s", pref.Column)
	}
	if !pref.Ascending {
		t.Error("expected ascending sort")
	}
}

func TestHelmmanifestDescribe(t *testing.T) {
	p := newTestHelmmanifest()
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Service",
		"metadata":   map[string]any{"name": "nginx-svc", "namespace": "default"},
	}}
	c, err := p.Describe(context.Background(), obj)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Raw == "" {
		t.Fatal("expected non-empty describe output")
	}
}

type mockDescribePlugin struct {
	pluginName    string
	pluginGVR     schema.GroupVersionResource
	clusterScoped bool
}

func (m *mockDescribePlugin) Name() string                              { return m.pluginName }
func (m *mockDescribePlugin) ShortName() string                         { return "mk" }
func (m *mockDescribePlugin) GVR() schema.GroupVersionResource          { return m.pluginGVR }
func (m *mockDescribePlugin) IsClusterScoped() bool                     { return m.clusterScoped }
func (m *mockDescribePlugin) Columns() []plugin.Column                  { return nil }
func (m *mockDescribePlugin) Row(_ *unstructured.Unstructured) []string { return nil }
func (m *mockDescribePlugin) YAML(_ *unstructured.Unstructured) (render.Content, error) {
	return render.Content{}, nil
}
func (m *mockDescribePlugin) Describe(_ context.Context, obj *unstructured.Unstructured) (render.Content, error) {
	s := "delegated: " + obj.GetKind()
	return render.Content{Raw: s, Display: s}, nil
}

func TestHelmmanifestDescribeDelegates(t *testing.T) {
	plugin.Reset()
	k8s.TestSeedGVRIndex(t)

	mock := &mockDescribePlugin{
		pluginName: "services",
		pluginGVR:  schema.GroupVersionResource{Version: "v1", Resource: "services"},
	}
	plugin.Register(mock)

	p := newTestHelmmanifest()
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Service",
		"metadata":   map[string]any{"name": "nginx-svc", "namespace": "default"},
	}}
	c, err := p.Describe(context.Background(), obj)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(c.Raw, "delegated") {
		t.Fatalf("expected delegated describe output, got: %s", c.Raw)
	}
}

type mockUncoverablePlugin struct {
	mockDescribePlugin
}

func (m *mockUncoverablePlugin) DescribeUncovered(_ context.Context, obj *unstructured.Unstructured) (render.Content, error) {
	s := "uncovered: " + obj.GetKind()
	return render.Content{Raw: s, Display: s}, nil
}

func TestHelmmanifestImplementsUncoverable(t *testing.T) {
	p := newTestHelmmanifest()
	if _, ok := any(p).(plugin.Uncoverable); !ok {
		t.Fatal("helmmanifest should implement plugin.Uncoverable")
	}
}

func TestHelmmanifestDescribeUncoveredDelegates(t *testing.T) {
	plugin.Reset()
	k8s.TestSeedGVRIndex(t)

	mock := &mockUncoverablePlugin{
		mockDescribePlugin: mockDescribePlugin{
			pluginName: "secrets",
			pluginGVR:  schema.GroupVersionResource{Version: "v1", Resource: "secrets"},
		},
	}
	plugin.Register(mock)

	p := newTestHelmmanifest()
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata":   map[string]any{"name": "my-secret", "namespace": "default"},
	}}
	c, err := p.DescribeUncovered(context.Background(), obj)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(c.Raw, "uncovered") {
		t.Fatalf("expected uncovered output, got: %s", c.Raw)
	}
}

func TestHelmmanifestDescribeUncoveredFallback(t *testing.T) {
	plugin.Reset()
	k8s.TestSeedGVRIndex(t)

	mock := &mockDescribePlugin{
		pluginName: "services",
		pluginGVR:  schema.GroupVersionResource{Version: "v1", Resource: "services"},
	}
	plugin.Register(mock)

	p := newTestHelmmanifest()
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Service",
		"metadata":   map[string]any{"name": "my-svc", "namespace": "default"},
	}}
	c, err := p.DescribeUncovered(context.Background(), obj)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(c.Raw, "delegated") {
		t.Fatalf("expected delegated describe fallback, got: %s", c.Raw)
	}
}

func TestHelmmanifestDescribeFallback(t *testing.T) {
	plugin.Reset()

	p := newTestHelmmanifest()
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "custom.io/v1",
		"kind":       "Widget",
		"metadata":   map[string]any{"name": "my-widget", "namespace": "default"},
	}}
	c, err := p.Describe(context.Background(), obj)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(c.Raw, "Widget") {
		t.Fatalf("expected fallback with kind, got: %s", c.Raw)
	}
}

func TestHelmmanifestDrillDownNilStore(t *testing.T) {
	p := newTestHelmmanifest()
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata":   map[string]any{"name": "nginx", "namespace": "default"},
	}}
	childPlugin, children := p.DrillDown(obj)
	if childPlugin != nil || children != nil {
		t.Fatal("expected nil,nil for nil store")
	}
}

func TestHelmmanifestDrillDownFiltersLiveObjects(t *testing.T) {
	plugin.Reset()
	k8s.TestSeedGVRIndex(t)

	store := k8s.NewStore(nil, nil)
	deploymentsGVR := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}

	// Seed live objects in store
	liveNginx := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1", "kind": "Deployment",
		"metadata": map[string]any{"name": "nginx", "namespace": "default"},
	}}
	liveWorker := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1", "kind": "Deployment",
		"metadata": map[string]any{"name": "worker", "namespace": "default"},
	}}
	liveUnrelated := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1", "kind": "Deployment",
		"metadata": map[string]any{"name": "unrelated", "namespace": "default"},
	}}
	store.CacheUpsert(deploymentsGVR, "default", liveNginx)
	store.CacheUpsert(deploymentsGVR, "default", liveWorker)
	store.CacheUpsert(deploymentsGVR, "default", liveUnrelated)

	// Manifest children: 2 deployments + 1 service
	children := []*unstructured.Unstructured{
		{Object: map[string]any{
			"apiVersion": "apps/v1", "kind": "Deployment",
			"metadata": map[string]any{"name": "nginx", "namespace": "default"},
		}},
		{Object: map[string]any{
			"apiVersion": "apps/v1", "kind": "Deployment",
			"metadata": map[string]any{"name": "worker", "namespace": "default"},
		}},
		{Object: map[string]any{
			"apiVersion": "v1", "kind": "Service",
			"metadata": map[string]any{"name": "nginx-svc", "namespace": "default"},
		}},
	}

	p := &helmmanifest{
		store:    store,
		children: children,
	}

	// Drill on a Deployment — should return only nginx and worker, not unrelated
	selected := children[0] // nginx deployment
	childPlugin, matched := p.DrillDown(selected)
	if childPlugin == nil {
		t.Fatal("expected non-nil child plugin")
	}
	if len(matched) != 2 {
		t.Fatalf("expected 2 matched live objects, got %d", len(matched))
	}
	names := map[string]bool{}
	for _, obj := range matched {
		names[obj.GetName()] = true
	}
	if !names["nginx"] || !names["worker"] {
		t.Fatalf("expected nginx and worker, got %v", names)
	}
	if names["unrelated"] {
		t.Fatal("unrelated deployment should not be included")
	}
}

func TestHelmmanifestDrillDownUnresolvableGVR(t *testing.T) {
	plugin.Reset()
	// Don't seed GVR index — resolution will fail

	store := k8s.NewStore(nil, nil)
	p := &helmmanifest{
		store: store,
		children: []*unstructured.Unstructured{
			{Object: map[string]any{
				"apiVersion": "custom.io/v1", "kind": "Widget",
				"metadata": map[string]any{"name": "w1", "namespace": "default"},
			}},
		},
	}

	selected := p.children[0]
	childPlugin, matched := p.DrillDown(selected)
	if childPlugin != nil || matched != nil {
		t.Fatal("expected nil,nil for unresolvable GVR")
	}
}

func TestHelmmanifestRowNamespaceFallback(t *testing.T) {
	plugin.Reset()
	k8s.TestSeedGVRIndex(t)

	mock := &mockDescribePlugin{
		pluginName: "services",
		pluginGVR:  schema.GroupVersionResource{Version: "v1", Resource: "services"},
	}
	plugin.Register(mock)

	p := &helmmanifest{
		releaseNamespace: "production",
	}

	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Service",
		"metadata":   map[string]any{"name": "nginx-svc"},
	}}
	row := p.Row(obj)
	if row[2] != "production" {
		t.Fatalf("expected namespace 'production', got '%s'", row[2])
	}
}

func TestHelmmanifestRowClusterScopedNoFallback(t *testing.T) {
	plugin.Reset()
	k8s.TestSeedGVRIndex(t)

	mock := &mockDescribePlugin{
		pluginName:    "namespaces",
		pluginGVR:     schema.GroupVersionResource{Version: "v1", Resource: "namespaces"},
		clusterScoped: true,
	}
	plugin.Register(mock)

	p := &helmmanifest{
		releaseNamespace: "production",
	}

	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Namespace",
		"metadata":   map[string]any{"name": "my-ns"},
	}}
	row := p.Row(obj)
	if row[2] != "" {
		t.Fatalf("expected empty namespace for cluster-scoped resource, got '%s'", row[2])
	}
}

func TestHelmmanifestDrillDownNamespaceFallback(t *testing.T) {
	plugin.Reset()
	k8s.TestSeedGVRIndex(t)

	// Register namespaced mock for Deployment
	mock := &mockDescribePlugin{
		pluginName: "deployments",
		pluginGVR:  schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"},
	}
	plugin.Register(mock)

	store := k8s.NewStore(nil, nil)
	deploymentsGVR := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}

	// Live object in "production" namespace
	liveNginx := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1", "kind": "Deployment",
		"metadata": map[string]any{"name": "nginx", "namespace": "production"},
	}}
	store.CacheUpsert(deploymentsGVR, "production", liveNginx)

	// Manifest child WITHOUT explicit namespace
	children := []*unstructured.Unstructured{
		{Object: map[string]any{
			"apiVersion": "apps/v1", "kind": "Deployment",
			"metadata": map[string]any{"name": "nginx"},
		}},
	}

	p := &helmmanifest{
		store:            store,
		releaseNamespace: "production",
		children:         children,
	}

	childPlugin, matched := p.DrillDown(children[0])
	if childPlugin == nil {
		t.Fatal("expected non-nil child plugin")
	}
	if len(matched) != 1 {
		t.Fatalf("expected 1 matched live object, got %d", len(matched))
	}
	if matched[0].GetName() != "nginx" {
		t.Fatalf("expected nginx, got %s", matched[0].GetName())
	}
}

func TestHelmmanifestRowExplicitNamespacePreserved(t *testing.T) {
	p := &helmmanifest{
		releaseNamespace: "production",
	}

	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Service",
		"metadata":   map[string]any{"name": "nginx-svc", "namespace": "kube-system"},
	}}
	row := p.Row(obj)
	if row[2] != "kube-system" {
		t.Fatalf("expected namespace 'kube-system', got '%s'", row[2])
	}
}

func TestHelmmanifestIsNamespaced(t *testing.T) {
	p := newTestHelmmanifest()
	if p.IsClusterScoped() {
		t.Fatal("helmmanifest should not be cluster-scoped")
	}
}

func TestHelmmanifestImplementsSelfPopulating(t *testing.T) {
	p := newTestHelmmanifest()
	if _, ok := any(p).(plugin.SelfPopulating); !ok {
		t.Fatal("helmmanifest should implement SelfPopulating")
	}
}

func TestHelmmanifestImplementsRefreshable(t *testing.T) {
	p := newTestHelmmanifest()
	if _, ok := any(p).(plugin.Refreshable); !ok {
		t.Fatal("helmmanifest should implement Refreshable")
	}
}

func TestHelmmanifestRefresh(t *testing.T) {
	mc := &mockClient{releases: []helm.ReleaseInfo{testRelease()}}
	p := &helmmanifest{
		helmClient:       mc,
		releaseName:      "nginx",
		releaseNamespace: "default",
	}
	if len(p.Objects()) != 0 {
		t.Fatal("expected no objects before refresh")
	}
	p.Refresh("")
	objs := p.Objects()
	if len(objs) != 1 {
		t.Fatalf("expected 1 object after refresh, got %d", len(objs))
	}
	if objs[0].GetKind() != "Service" {
		t.Fatalf("expected Service, got %s", objs[0].GetKind())
	}
}

func TestHelmmanifestRefreshNilClient(t *testing.T) {
	p := &helmmanifest{
		releaseName:      "nginx",
		releaseNamespace: "default",
	}
	p.Refresh("") // should not panic
	if len(p.Objects()) != 0 {
		t.Fatal("expected no objects with nil client")
	}
}

func TestHelmmanifestRefreshUpdatesChildren(t *testing.T) {
	r := testRelease()
	mc := &mockClient{releases: []helm.ReleaseInfo{r}}
	p := &helmmanifest{
		helmClient:       mc,
		releaseName:      "nginx",
		releaseNamespace: "default",
		children: []*unstructured.Unstructured{
			{Object: map[string]any{"kind": "OldThing", "metadata": map[string]any{"name": "old"}}},
		},
	}
	if len(p.Objects()) != 1 || p.Objects()[0].GetKind() != "OldThing" {
		t.Fatal("expected initial OldThing object")
	}
	p.Refresh("")
	objs := p.Objects()
	if len(objs) != 1 {
		t.Fatalf("expected 1 object after refresh, got %d", len(objs))
	}
	if objs[0].GetKind() != "Service" {
		t.Fatalf("expected refreshed Service, got %s", objs[0].GetKind())
	}
}
