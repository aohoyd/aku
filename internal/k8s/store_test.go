package k8s

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestStoreCacheUpsertAndList(t *testing.T) {
	s := NewStore(nil, nil)
	gvr := schema.GroupVersionResource{Version: "v1", Resource: "pods"}

	obj := &unstructured.Unstructured{}
	obj.SetName("test-pod")
	obj.SetNamespace("default")

	s.CacheUpsert(gvr, "default", obj)
	items := s.List(gvr, "default")
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].GetName() != "test-pod" {
		t.Fatalf("expected 'test-pod', got '%s'", items[0].GetName())
	}
}

func TestStoreCacheDelete(t *testing.T) {
	s := NewStore(nil, nil)
	gvr := schema.GroupVersionResource{Version: "v1", Resource: "pods"}

	obj := &unstructured.Unstructured{}
	obj.SetName("test-pod")
	s.CacheUpsert(gvr, "default", obj)

	s.CacheDelete(gvr, "default", obj)
	items := s.List(gvr, "default")
	if len(items) != 0 {
		t.Fatalf("expected 0 items after delete, got %d", len(items))
	}
}

func TestStoreListEmpty(t *testing.T) {
	s := NewStore(nil, nil)
	gvr := schema.GroupVersionResource{Version: "v1", Resource: "pods"}
	items := s.List(gvr, "default")
	if items != nil {
		t.Fatalf("expected nil for empty cache, got %v", items)
	}
}

func TestStoreMultipleGVRs(t *testing.T) {
	s := NewStore(nil, nil)
	podsGVR := schema.GroupVersionResource{Version: "v1", Resource: "pods"}
	svcsGVR := schema.GroupVersionResource{Version: "v1", Resource: "services"}

	pod := &unstructured.Unstructured{}
	pod.SetName("my-pod")
	s.CacheUpsert(podsGVR, "default", pod)

	svc := &unstructured.Unstructured{}
	svc.SetName("my-svc")
	s.CacheUpsert(svcsGVR, "default", svc)

	pods := s.List(podsGVR, "default")
	svcs := s.List(svcsGVR, "default")
	if len(pods) != 1 || len(svcs) != 1 {
		t.Fatalf("expected 1 pod and 1 svc, got %d pods, %d svcs", len(pods), len(svcs))
	}
}

func TestStoreUnsubscribeClearsCache(t *testing.T) {
	s := NewStore(nil, nil)
	gvr := schema.GroupVersionResource{Version: "v1", Resource: "pods"}

	obj := &unstructured.Unstructured{}
	obj.SetName("test-pod")
	s.CacheUpsert(gvr, "default", obj)

	s.Unsubscribe(gvr, "default")
	items := s.List(gvr, "default")
	if items != nil {
		t.Fatal("cache should be cleared after Unsubscribe")
	}
}

func TestStoreUnsubscribeAll(t *testing.T) {
	s := NewStore(nil, nil)
	gvr1 := schema.GroupVersionResource{Version: "v1", Resource: "pods"}
	gvr2 := schema.GroupVersionResource{Version: "v1", Resource: "services"}

	obj1 := &unstructured.Unstructured{}
	obj1.SetName("pod")
	s.CacheUpsert(gvr1, "default", obj1)

	obj2 := &unstructured.Unstructured{}
	obj2.SetName("svc")
	s.CacheUpsert(gvr2, "default", obj2)

	s.UnsubscribeAll()
	if s.List(gvr1, "default") != nil || s.List(gvr2, "default") != nil {
		t.Fatal("all caches should be cleared after UnsubscribeAll")
	}
}

func TestStoreNotify(t *testing.T) {
	var received bool
	s := NewStore(nil, func(msg tea.Msg) {
		received = true
	})
	gvr := schema.GroupVersionResource{Version: "v1", Resource: "pods"}

	obj := &unstructured.Unstructured{}
	obj.SetName("test-pod")
	s.CacheUpsert(gvr, "default", obj)
	s.doNotify(watchKey{GVR: gvr, Namespace: "default"})

	if !received {
		t.Fatal("send function should have been called")
	}
}

func TestStoreCacheAllNamespacesNoCollision(t *testing.T) {
	s := NewStore(nil, nil)
	gvr := schema.GroupVersionResource{Version: "v1", Resource: "pods"}

	// Two pods with same name in different namespaces, stored under all-ns watch key ""
	pod1 := &unstructured.Unstructured{}
	pod1.SetName("nginx")
	pod1.SetNamespace("staging")
	s.CacheUpsert(gvr, "", pod1)

	pod2 := &unstructured.Unstructured{}
	pod2.SetName("nginx")
	pod2.SetNamespace("production")
	s.CacheUpsert(gvr, "", pod2)

	items := s.List(gvr, "")
	if len(items) != 2 {
		t.Fatalf("expected 2 items (same name, different namespaces), got %d", len(items))
	}

	// Verify both namespaces are present
	nss := map[string]bool{}
	for _, item := range items {
		nss[item.GetNamespace()] = true
	}
	if !nss["staging"] || !nss["production"] {
		t.Fatalf("expected both staging and production, got %v", nss)
	}
}

func TestStoreCacheDeleteAllNamespaces(t *testing.T) {
	s := NewStore(nil, nil)
	gvr := schema.GroupVersionResource{Version: "v1", Resource: "pods"}

	pod1 := &unstructured.Unstructured{}
	pod1.SetName("nginx")
	pod1.SetNamespace("staging")
	s.CacheUpsert(gvr, "", pod1)

	pod2 := &unstructured.Unstructured{}
	pod2.SetName("nginx")
	pod2.SetNamespace("production")
	s.CacheUpsert(gvr, "", pod2)

	// Delete only the staging one
	s.CacheDelete(gvr, "", pod1)
	items := s.List(gvr, "")
	if len(items) != 1 {
		t.Fatalf("expected 1 item after delete, got %d", len(items))
	}
	if items[0].GetNamespace() != "production" {
		t.Fatalf("expected production pod to remain, got %q", items[0].GetNamespace())
	}
}

func TestStoreDeepCopy(t *testing.T) {
	s := NewStore(nil, nil)
	gvr := schema.GroupVersionResource{Version: "v1", Resource: "pods"}

	obj := &unstructured.Unstructured{}
	obj.SetName("test-pod")
	obj.SetLabels(map[string]string{"app": "test"})
	s.CacheUpsert(gvr, "default", obj)

	// Modify original
	obj.SetLabels(map[string]string{"app": "modified"})

	// Cached version should be unaffected
	items := s.List(gvr, "default")
	labels := items[0].GetLabels()
	if labels["app"] != "test" {
		t.Fatalf("expected cached label 'test', got '%s'", labels["app"])
	}
}
