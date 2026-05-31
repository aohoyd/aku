package k8s

import (
	"sync"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestFilterSubResources(t *testing.T) {
	d := NewDiscovery()
	resources := d.filterAPIResources([]metav1.APIResourceList{
		{
			GroupVersion: "v1",
			APIResources: []metav1.APIResource{
				{Name: "pods", Kind: "Pod", Namespaced: true, Verbs: metav1.Verbs{"list", "get"}},
				{Name: "pods/log", Kind: "Pod", Namespaced: true, Verbs: metav1.Verbs{"get"}},
				{Name: "pods/status", Kind: "Pod", Namespaced: true, Verbs: metav1.Verbs{"get", "patch"}},
				{Name: "namespaces", Kind: "Namespace", Namespaced: false, Verbs: metav1.Verbs{"list", "get"}},
				{Name: "bindings", Kind: "Binding", Namespaced: true, Verbs: metav1.Verbs{"create"}},
			},
		},
		{
			GroupVersion: "apps/v1",
			APIResources: []metav1.APIResource{
				{Name: "deployments", ShortNames: []string{"deploy"}, Kind: "Deployment", Namespaced: true, Verbs: metav1.Verbs{"list", "get", "create"}},
			},
		},
	})

	// Should filter: pods/log, pods/status (sub-resources), bindings (no list verb)
	if len(resources) != 3 {
		t.Fatalf("expected 3 resources, got %d: %+v", len(resources), resources)
	}
	// Results are sorted alphabetically
	if resources[0].Name != "deployments" {
		t.Fatalf("expected first resource 'deployments', got '%s'", resources[0].Name)
	}
	if resources[0].ShortNames[0] != "deploy" {
		t.Fatalf("expected short name 'deploy', got '%s'", resources[0].ShortNames[0])
	}
	if resources[0].APIVersion != "apps/v1" {
		t.Fatalf("expected apiVersion 'apps/v1', got '%s'", resources[0].APIVersion)
	}
	if resources[1].Name != "namespaces" {
		t.Fatalf("expected second resource 'namespaces', got '%s'", resources[1].Name)
	}
	if resources[1].Namespaced {
		t.Fatal("expected 'namespaces' to be cluster-scoped")
	}
	if resources[2].Name != "pods" {
		t.Fatalf("expected third resource 'pods', got '%s'", resources[2].Name)
	}
}

func TestFilterEmptyInput(t *testing.T) {
	d := NewDiscovery()
	resources := d.filterAPIResources(nil)
	if len(resources) != 0 {
		t.Fatalf("expected 0 resources for nil input, got %d", len(resources))
	}
}

func TestFilterInvalidGroupVersion(t *testing.T) {
	d := NewDiscovery()
	resources := d.filterAPIResources([]metav1.APIResourceList{
		{
			GroupVersion: "invalid/group/version",
			APIResources: []metav1.APIResource{
				{Name: "things", Kind: "Thing", Namespaced: true, Verbs: metav1.Verbs{"list"}},
			},
		},
	})
	if len(resources) != 0 {
		t.Fatalf("expected 0 resources for invalid group version, got %d", len(resources))
	}
}

func TestFilterCoreGroupAPIVersion(t *testing.T) {
	d := NewDiscovery()
	resources := d.filterAPIResources([]metav1.APIResourceList{
		{
			GroupVersion: "v1",
			APIResources: []metav1.APIResource{
				{Name: "pods", Kind: "Pod", Namespaced: true, Verbs: metav1.Verbs{"list"}},
			},
		},
	})
	if len(resources) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(resources))
	}
	// Core group should have apiVersion "v1" not "/v1"
	if resources[0].APIVersion != "v1" {
		t.Fatalf("expected apiVersion 'v1', got '%s'", resources[0].APIVersion)
	}
	if resources[0].Group != "" {
		t.Fatalf("expected empty group for core resources, got '%s'", resources[0].Group)
	}
}

func TestFilterGVRPopulated(t *testing.T) {
	d := NewDiscovery()
	resources := d.filterAPIResources([]metav1.APIResourceList{
		{
			GroupVersion: "apps/v1",
			APIResources: []metav1.APIResource{
				{Name: "deployments", Kind: "Deployment", Namespaced: true, Verbs: metav1.Verbs{"list"}},
			},
		},
	})
	if len(resources) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(resources))
	}
	gvr := resources[0].GVR
	if gvr.Group != "apps" || gvr.Version != "v1" || gvr.Resource != "deployments" {
		t.Fatalf("expected GVR apps/v1/deployments, got %v", gvr)
	}
}

func TestResolveGVR_CoreGroup(t *testing.T) {
	d := NewDiscovery()
	d.filterAPIResources([]metav1.APIResourceList{
		{
			GroupVersion: "v1",
			APIResources: []metav1.APIResource{
				{Name: "services", Kind: "Service", Namespaced: true, Verbs: metav1.Verbs{"list", "get"}},
				{Name: "configmaps", Kind: "ConfigMap", Namespaced: true, Verbs: metav1.Verbs{"list", "get"}},
			},
		},
	})

	gvr, ok := d.ResolveGVR("v1", "Service")
	if !ok {
		t.Fatal("expected to resolve Service")
	}
	expected := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "services"}
	if gvr != expected {
		t.Fatalf("expected %v, got %v", expected, gvr)
	}
}

func TestResolveGVR_AppsGroup(t *testing.T) {
	d := NewDiscovery()
	d.filterAPIResources([]metav1.APIResourceList{
		{
			GroupVersion: "apps/v1",
			APIResources: []metav1.APIResource{
				{Name: "deployments", Kind: "Deployment", Namespaced: true, Verbs: metav1.Verbs{"list", "get"}},
			},
		},
	})

	gvr, ok := d.ResolveGVR("apps/v1", "Deployment")
	if !ok {
		t.Fatal("expected to resolve Deployment")
	}
	expected := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	if gvr != expected {
		t.Fatalf("expected %v, got %v", expected, gvr)
	}
}

func TestResolveGVR_NotFound(t *testing.T) {
	d := NewDiscovery()

	_, ok := d.ResolveGVR("v1", "NonExistent")
	if ok {
		t.Fatal("expected not found for NonExistent")
	}
}

func TestKindForGVR_AfterFilter(t *testing.T) {
	d := NewDiscovery()
	d.filterAPIResources([]metav1.APIResourceList{
		{
			GroupVersion: "v1",
			APIResources: []metav1.APIResource{
				{Name: "pods", Kind: "Pod", Namespaced: true, Verbs: metav1.Verbs{"list"}},
			},
		},
		{
			GroupVersion: "apps/v1",
			APIResources: []metav1.APIResource{
				{Name: "deployments", Kind: "Deployment", Namespaced: true, Verbs: metav1.Verbs{"list"}},
			},
		},
	})

	kind, ok := d.KindForGVR(schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"})
	if !ok || kind != "Pod" {
		t.Fatalf("expected (Pod, true), got (%s, %v)", kind, ok)
	}

	kind, ok = d.KindForGVR(schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"})
	if !ok || kind != "Deployment" {
		t.Fatalf("expected (Deployment, true), got (%s, %v)", kind, ok)
	}

	_, ok = d.KindForGVR(schema.GroupVersionResource{Group: "fake", Version: "v1", Resource: "widgets"})
	if ok {
		t.Fatal("expected not found for unknown GVR")
	}
}

func TestFilterAPIResources_ClearsStaleEntries(t *testing.T) {
	// A single Discovery refreshed against two different clusters must drop
	// entries from the first that are absent in the second.
	d := NewDiscovery()

	// Simulate first cluster with a CRD "widgets.example.com"
	clusterA := []metav1.APIResourceList{
		{
			GroupVersion: "example.com/v1",
			APIResources: []metav1.APIResource{
				{Name: "widgets", Kind: "Widget", Namespaced: true, Verbs: metav1.Verbs{"list", "get"}},
			},
		},
		{
			GroupVersion: "v1",
			APIResources: []metav1.APIResource{
				{Name: "pods", Kind: "Pod", Namespaced: true, Verbs: metav1.Verbs{"list", "get"}},
			},
		},
	}
	d.filterAPIResources(clusterA)

	// Verify Widget is resolvable after first discovery
	if _, ok := d.ResolveGVR("example.com/v1", "Widget"); !ok {
		t.Fatal("expected Widget to be resolvable after cluster A discovery")
	}

	// Simulate context switch to cluster B which does NOT have the Widget CRD
	clusterB := []metav1.APIResourceList{
		{
			GroupVersion: "v1",
			APIResources: []metav1.APIResource{
				{Name: "pods", Kind: "Pod", Namespaced: true, Verbs: metav1.Verbs{"list", "get"}},
			},
		},
	}
	d.filterAPIResources(clusterB)

	// Widget must no longer be resolvable — this is the bug scenario
	if _, ok := d.ResolveGVR("example.com/v1", "Widget"); ok {
		t.Fatal("Widget should NOT be resolvable after switching to cluster B (stale cache)")
	}
	widgetGVR := schema.GroupVersionResource{Group: "example.com", Version: "v1", Resource: "widgets"}
	if _, ok := d.KindForGVR(widgetGVR); ok {
		t.Fatal("kind for widgets GVR should NOT be found after switching to cluster B")
	}

	// Pod should still be resolvable on cluster B
	if _, ok := d.ResolveGVR("v1", "Pod"); !ok {
		t.Fatal("expected Pod to be resolvable after cluster B discovery")
	}
}

// TestDiscoveryRoundTrip verifies ResolveGVR and KindForGVR are inverse
// lookups on the same Discovery instance.
func TestDiscoveryRoundTrip(t *testing.T) {
	d := NewDiscovery()
	gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	d.SetGVRForTest("apps/v1", "Deployment", gvr)

	got, ok := d.ResolveGVR("apps/v1", "Deployment")
	if !ok {
		t.Fatal("expected to resolve Deployment GVR")
	}
	if got != gvr {
		t.Errorf("ResolveGVR = %v, want %v", got, gvr)
	}

	kind, ok := d.KindForGVR(gvr)
	if !ok {
		t.Fatal("expected to resolve Kind for GVR")
	}
	if kind != "Deployment" {
		t.Errorf("KindForGVR = %q, want %q", kind, "Deployment")
	}
}

// TestDiscoveryInstancesIsolated verifies two Discovery instances do not share
// index entries.
func TestDiscoveryInstancesIsolated(t *testing.T) {
	a := NewDiscovery()
	b := NewDiscovery()

	gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	a.SetGVRForTest("apps/v1", "Deployment", gvr)

	if _, ok := b.ResolveGVR("apps/v1", "Deployment"); ok {
		t.Error("Discovery b should not see entries stored on Discovery a (ResolveGVR)")
	}
	if _, ok := b.KindForGVR(gvr); ok {
		t.Error("Discovery b should not see entries stored on Discovery a (KindForGVR)")
	}

	// Sanity: a still has the entry.
	if _, ok := a.ResolveGVR("apps/v1", "Deployment"); !ok {
		t.Error("Discovery a should retain its own entry")
	}
}

// TestDiscoveryConcurrentPopulateAndRead exercises the atomic index swap:
// Populate (which runs off the Update goroutine in production) must never expose
// a half-cleared index to concurrent ResolveGVR/KindForGVR/IsEmpty readers. Run
// with -race to validate there is no data race and that a reader always sees a
// complete index — an observed Kind/GVR hit must be internally consistent, never
// a partial one. It does not assert hit-vs-miss timing (a reader may legitimately
// observe the pre-populate empty index); it asserts only that any observed hit is
// consistent and that the run is race-free.
func TestDiscoveryConcurrentPopulateAndRead(t *testing.T) {
	d := NewDiscovery()
	res := []APIResource{
		{Name: "pods", Group: "", Version: "v1", Kind: "Pod",
			GVR: schema.GroupVersionResource{Version: "v1", Resource: "pods"}},
		{Name: "deployments", Group: "apps", Version: "v1", Kind: "Deployment",
			GVR: schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}},
	}

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// Writers: repeatedly swap the index between the full set and empty.
	for w := 0; w < 2; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					d.Populate(res)
					d.Populate(nil)
				}
			}
		}()
	}

	// Readers: hammer the read paths. A half-cleared index would either race or
	// yield an inconsistent GVR for a Kind hit.
	for r := 0; r < 4; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 5000; i++ {
				if gvr, ok := d.ResolveGVR("apps/v1", "Deployment"); ok {
					if gvr.Resource != "deployments" {
						t.Errorf("inconsistent GVR for Deployment: %+v", gvr)
						return
					}
				}
				_, _ = d.KindForGVR(schema.GroupVersionResource{Version: "v1", Resource: "pods"})
				_ = d.IsEmpty()
			}
		}()
	}

	time.Sleep(50 * time.Millisecond)
	close(stop)
	wg.Wait()
}

// TestDiscoveryIsEmptyTransitions covers the IsEmpty lifecycle directly: a fresh
// Discovery is empty; Populate with a non-empty set makes it non-empty; Populate
// with nil clears it back to empty. Callers rely on IsEmpty to know whether a
// KindForGVR miss is authoritative ("not on this cluster") or premature
// ("discovery not refreshed yet").
func TestDiscoveryIsEmptyTransitions(t *testing.T) {
	d := NewDiscovery()
	if !d.IsEmpty() {
		t.Fatal("fresh NewDiscovery() should report IsEmpty() == true")
	}

	d.Populate([]APIResource{
		{
			Name:    "pods",
			Group:   "",
			Version: "v1",
			Kind:    "Pod",
			GVR:     schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
		},
	})
	if d.IsEmpty() {
		t.Fatal("after Populate(non-empty), IsEmpty() should be false")
	}

	d.Populate(nil)
	if !d.IsEmpty() {
		t.Fatal("after Populate(nil), IsEmpty() should be true again")
	}
}
