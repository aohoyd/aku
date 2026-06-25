package k8s

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestStoreCacheUpsertAndList(t *testing.T) {
	s := NewStore(nil, "", nil)
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
	s := NewStore(nil, "", nil)
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
	s := NewStore(nil, "", nil)
	gvr := schema.GroupVersionResource{Version: "v1", Resource: "pods"}
	items := s.List(gvr, "default")
	if items != nil {
		t.Fatalf("expected nil for empty cache, got %v", items)
	}
}

func TestStoreMultipleGVRs(t *testing.T) {
	s := NewStore(nil, "", nil)
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

func TestStoreCountInNamespace(t *testing.T) {
	s := NewStore(nil, "", nil)
	podsGVR := schema.GroupVersionResource{Version: "v1", Resource: "pods"}
	svcsGVR := schema.GroupVersionResource{Version: "v1", Resource: "services"}

	// Mirror the static-manifest store's dual-keying: each namespaced object is
	// upserted under its namespace AND once into the all-namespaces ("") bucket.
	upsertDual := func(gvr schema.GroupVersionResource, ns, name string) {
		o := &unstructured.Unstructured{}
		o.SetName(name)
		o.SetNamespace(ns)
		s.CacheUpsert(gvr, ns, o)
		s.CacheUpsert(gvr, "", o)
	}

	// 2 pods + 1 svc in "default"; 1 pod in "kube-system".
	upsertDual(podsGVR, "default", "pod-a")
	upsertDual(podsGVR, "default", "pod-b")
	upsertDual(svcsGVR, "default", "svc-a")
	upsertDual(podsGVR, "kube-system", "pod-c")

	// The "" bucket holds every distinct object exactly once: 4 total.
	if got := s.CountInNamespace(""); got != 4 {
		t.Fatalf("expected CountInNamespace(\"\")=4 distinct objects, got %d", got)
	}

	// A namespaced bucket counts only that namespace's objects across all GVRs.
	if got := s.CountInNamespace("default"); got != 3 {
		t.Fatalf("expected CountInNamespace(\"default\")=3, got %d", got)
	}
	if got := s.CountInNamespace("kube-system"); got != 1 {
		t.Fatalf("expected CountInNamespace(\"kube-system\")=1, got %d", got)
	}

	// An unknown namespace counts zero.
	if got := s.CountInNamespace("nope"); got != 0 {
		t.Fatalf("expected CountInNamespace(\"nope\")=0, got %d", got)
	}
}

func TestStoreUnsubscribeClearsCache(t *testing.T) {
	s := NewStore(nil, "", nil)
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

// TestStoreStaticUnsubscribePreservesCache verifies that a store marked static
// (the client-less manifest pseudo-cluster) never tears down its cache on
// Unsubscribe. Its cache is the sole copy of its data — no informer will ever
// repopulate it — so teardown would orphan it permanently (the bug behind
// "switching back to All Namespaces hides all pods forever").
func TestStoreStaticUnsubscribePreservesCache(t *testing.T) {
	s := NewStore(nil, "", nil)
	s.MarkStatic()
	gvr := schema.GroupVersionResource{Version: "v1", Resource: "pods"}

	obj := &unstructured.Unstructured{}
	obj.SetName("test-pod")
	s.CacheUpsert(gvr, "", obj)

	s.Unsubscribe(gvr, "")
	if items := s.List(gvr, ""); len(items) != 1 {
		t.Fatalf("static store cache should survive Unsubscribe, got %d items", len(items))
	}
}

// TestStoreStaticUnsubscribeAllPreservesCache mirrors the above for the bulk
// teardown path (reloadAll's UnsubscribeAll).
func TestStoreStaticUnsubscribeAllPreservesCache(t *testing.T) {
	s := NewStore(nil, "", nil)
	s.MarkStatic()
	gvr := schema.GroupVersionResource{Version: "v1", Resource: "pods"}

	obj := &unstructured.Unstructured{}
	obj.SetName("test-pod")
	s.CacheUpsert(gvr, "", obj)

	s.UnsubscribeAll()
	if items := s.List(gvr, ""); len(items) != 1 {
		t.Fatalf("static store cache should survive UnsubscribeAll, got %d items", len(items))
	}
}

func TestStoreUnsubscribeAll(t *testing.T) {
	s := NewStore(nil, "", nil)
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
	var got tea.Msg
	s := NewStore(nil, "prod-ctx", func(msg tea.Msg) {
		received = true
		got = msg
	})
	gvr := schema.GroupVersionResource{Version: "v1", Resource: "pods"}

	obj := &unstructured.Unstructured{}
	obj.SetName("test-pod")
	s.CacheUpsert(gvr, "default", obj)
	s.doNotify(watchKey{GVR: gvr, Namespace: "default"})

	if !received {
		t.Fatal("send function should have been called")
	}

	upd, ok := got.(ResourceUpdatedMsg)
	if !ok {
		t.Fatalf("expected ResourceUpdatedMsg, got %T", got)
	}
	if upd.Context != "prod-ctx" {
		t.Fatalf("expected msg.Context %q, got %q", "prod-ctx", upd.Context)
	}
	if upd.GVR != gvr {
		t.Fatalf("expected msg.GVR %v, got %v", gvr, upd.GVR)
	}
	if upd.Namespace != "default" {
		t.Fatalf("expected msg.Namespace %q, got %q", "default", upd.Namespace)
	}
}

// TestStoreContextAccessor verifies the store reports the context name it was
// constructed with.
func TestStoreContextAccessor(t *testing.T) {
	s := NewStore(nil, "staging-ctx", nil)
	if s.Context() != "staging-ctx" {
		t.Fatalf("expected Context() %q, got %q", "staging-ctx", s.Context())
	}
}

func TestStoreCacheAllNamespacesNoCollision(t *testing.T) {
	s := NewStore(nil, "", nil)
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
	s := NewStore(nil, "", nil)
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
	s := NewStore(nil, "", nil)
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

func TestStoreConcurrentCacheOperations(t *testing.T) {
	s := NewStore(nil, "", func(msg tea.Msg) {})
	gvr := schema.GroupVersionResource{Version: "v1", Resource: "pods"}

	const goroutines = 20
	const opsPerGoroutine = 100

	var wg sync.WaitGroup

	// Spawn goroutines that perform concurrent CacheUpsert
	for i := range goroutines {
		wg.Go(func() {
			for j := range opsPerGoroutine {
				obj := &unstructured.Unstructured{}
				obj.SetName(fmt.Sprintf("pod-%d-%d", i, j))
				obj.SetNamespace("default")
				s.CacheUpsert(gvr, "default", obj)
			}
		})
	}

	// Spawn goroutines that perform concurrent List
	for range goroutines {
		wg.Go(func() {
			for range opsPerGoroutine {
				_ = s.List(gvr, "default")
			}
		})
	}

	// Spawn goroutines that perform concurrent CacheDelete
	for i := range goroutines {
		wg.Go(func() {
			for j := range opsPerGoroutine {
				obj := &unstructured.Unstructured{}
				obj.SetName(fmt.Sprintf("pod-%d-%d", i, j))
				obj.SetNamespace("default")
				s.CacheDelete(gvr, "default", obj)
			}
		})
	}

	wg.Wait()

	// Verify the store is still consistent: List should not panic
	items := s.List(gvr, "default")
	t.Logf("items remaining after concurrent ops: %d", len(items))
}

func TestStoreSubscribeUnsubscribeNoLeak(t *testing.T) {
	// NewStore with nil client: Subscribe will launch runInformer which will
	// panic or fail on nil client. We manually insert a cancel entry to
	// simulate the subscribe path without needing a real client.
	// Instead, we directly test Subscribe+Unsubscribe on cache-only paths.
	s := NewStore(nil, "", func(msg tea.Msg) {})
	gvr := schema.GroupVersionResource{Version: "v1", Resource: "pods"}

	// Force GC and stabilize goroutine count
	runtime.GC()
	time.Sleep(50 * time.Millisecond)
	before := runtime.NumGoroutine()

	const iterations = 50
	for range iterations {
		// Manually set up a cache bucket and informer cancel entry
		// to mimic Subscribe without starting a real informer (no k8s client).
		key := watchKey{GVR: gvr, Namespace: "default"}
		s.mu.Lock()
		if s.cache[key] == nil {
			s.cache[key] = make(map[string]*unstructured.Unstructured)
		}
		_, cancel := context.WithCancel(context.Background())
		s.informers[key] = cancel
		s.mu.Unlock()

		s.Unsubscribe(gvr, "default")
	}

	// Allow goroutines to settle
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	after := runtime.NumGoroutine()

	// Allow a small margin for runtime goroutines
	if after > before+5 {
		t.Fatalf("possible goroutine leak: before=%d, after=%d", before, after)
	}
}

// TestStoreSubscribeNilClientNoInformer verifies that Subscribe on a store with
// a nil dynamic client does not launch an informer goroutine (which would
// dereference the nil client on a background goroutine and panic
// asynchronously). It must instead behave like a cache-only store: return the
// currently-cached items and start no informer. Degraded clusters and many unit
// tests build a Store this way.
func TestStoreSubscribeNilClientNoInformer(t *testing.T) {
	s := NewStore(nil, "", nil)
	gvr := schema.GroupVersionResource{Version: "v1", Resource: "pods"}

	// Seed one cached object so we can confirm Subscribe returns the cache.
	obj := &unstructured.Unstructured{}
	obj.SetName("cached-pod")
	obj.SetNamespace("default")
	s.CacheUpsert(gvr, "default", obj)

	runtime.GC()
	time.Sleep(50 * time.Millisecond)
	before := runtime.NumGoroutine()

	// This must not panic and must not start an informer goroutine.
	items := s.Subscribe(gvr, "default")
	if len(items) != 1 || items[0].GetName() != "cached-pod" {
		t.Fatalf("expected the cached pod back from Subscribe, got %v", items)
	}

	// No informer should have been registered for the nil-client store.
	s.mu.RLock()
	_, hasInformer := s.informers[watchKey{GVR: gvr, Namespace: "default"}]
	s.mu.RUnlock()
	if hasInformer {
		t.Fatal("Subscribe on a nil-client store must not register an informer")
	}

	// Give any erroneously-launched informer goroutine a chance to appear.
	runtime.GC()
	time.Sleep(100 * time.Millisecond)
	after := runtime.NumGoroutine()
	if after > before+2 {
		t.Fatalf("Subscribe on a nil-client store started goroutines: before=%d, after=%d", before, after)
	}
}
