package k8s

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestFilterSubResources(t *testing.T) {
	resources := filterAPIResources([]metav1.APIResourceList{
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
	resources := filterAPIResources(nil)
	if len(resources) != 0 {
		t.Fatalf("expected 0 resources for nil input, got %d", len(resources))
	}
}

func TestFilterInvalidGroupVersion(t *testing.T) {
	resources := filterAPIResources([]metav1.APIResourceList{
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
	resources := filterAPIResources([]metav1.APIResourceList{
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
	resources := filterAPIResources([]metav1.APIResourceList{
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
	lists := []metav1.APIResourceList{
		{
			GroupVersion: "v1",
			APIResources: []metav1.APIResource{
				{Name: "services", Kind: "Service", Namespaced: true, Verbs: metav1.Verbs{"list", "get"}},
				{Name: "configmaps", Kind: "ConfigMap", Namespaced: true, Verbs: metav1.Verbs{"list", "get"}},
			},
		},
	}
	filterAPIResources(lists)

	gvr, ok := ResolveGVR("v1", "Service")
	if !ok {
		t.Fatal("expected to resolve Service")
	}
	expected := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "services"}
	if gvr != expected {
		t.Fatalf("expected %v, got %v", expected, gvr)
	}
}

func TestResolveGVR_AppsGroup(t *testing.T) {
	lists := []metav1.APIResourceList{
		{
			GroupVersion: "apps/v1",
			APIResources: []metav1.APIResource{
				{Name: "deployments", Kind: "Deployment", Namespaced: true, Verbs: metav1.Verbs{"list", "get"}},
			},
		},
	}
	filterAPIResources(lists)

	gvr, ok := ResolveGVR("apps/v1", "Deployment")
	if !ok {
		t.Fatal("expected to resolve Deployment")
	}
	expected := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	if gvr != expected {
		t.Fatalf("expected %v, got %v", expected, gvr)
	}
}

func TestResolveGVR_NotFound(t *testing.T) {
	gvrIndex.Clear()

	_, ok := ResolveGVR("v1", "NonExistent")
	if ok {
		t.Fatal("expected not found for NonExistent")
	}
}

func TestKindForGVR_AfterFilter(t *testing.T) {
	lists := []metav1.APIResourceList{
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
	}
	filterAPIResources(lists)
	t.Cleanup(func() {
		kindIndex.Clear()
	})

	kind, ok := KindForGVR(schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"})
	if !ok || kind != "Pod" {
		t.Fatalf("expected (Pod, true), got (%s, %v)", kind, ok)
	}

	kind, ok = KindForGVR(schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"})
	if !ok || kind != "Deployment" {
		t.Fatalf("expected (Deployment, true), got (%s, %v)", kind, ok)
	}

	_, ok = KindForGVR(schema.GroupVersionResource{Group: "fake", Version: "v1", Resource: "widgets"})
	if ok {
		t.Fatal("expected not found for unknown GVR")
	}
}

func TestFilterAPIResources_ClearsStaleEntries(t *testing.T) {
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
	filterAPIResources(clusterA)

	// Verify Widget is resolvable after first discovery
	if _, ok := ResolveGVR("example.com/v1", "Widget"); !ok {
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
	filterAPIResources(clusterB)

	// Widget must no longer be resolvable — this is the bug scenario
	if _, ok := ResolveGVR("example.com/v1", "Widget"); ok {
		t.Fatal("Widget should NOT be resolvable after switching to cluster B (stale cache)")
	}
	widgetGVR := schema.GroupVersionResource{Group: "example.com", Version: "v1", Resource: "widgets"}
	if _, ok := KindForGVR(widgetGVR); ok {
		t.Fatal("kind for widgets GVR should NOT be found after switching to cluster B")
	}

	// Pod should still be resolvable on cluster B
	if _, ok := ResolveGVR("v1", "Pod"); !ok {
		t.Fatal("expected Pod to be resolvable after cluster B discovery")
	}
}
