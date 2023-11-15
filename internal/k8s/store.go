package k8s

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/tools/cache"
)

// watchKey uniquely identifies an active watch.
type watchKey struct {
	GVR       schema.GroupVersionResource
	Namespace string
}

// Store manages dynamic informers per (GVR, namespace) pair.
// It maintains a thread-safe cache and pushes updates via send func.
type Store struct {
	client    dynamic.Interface
	mu        sync.RWMutex
	cache     map[watchKey]map[string]*unstructured.Unstructured // outer=watchKey, inner=name
	informers map[watchKey]context.CancelFunc
	send      func(tea.Msg)
	debouncer *Debouncer
}

// NewStore creates a new Store. send can be nil initially and set later via SetSend.
func NewStore(client dynamic.Interface, send func(tea.Msg)) *Store {
	s := &Store{
		client:    client,
		cache:     make(map[watchKey]map[string]*unstructured.Unstructured),
		informers: make(map[watchKey]context.CancelFunc),
		send:      send,
	}
	s.debouncer = NewDebouncer(50*time.Millisecond, s.doNotify)
	return s
}

// SetSend sets the send function (typically p.Send from tea.Program).
func (s *Store) SetSend(send func(tea.Msg)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.send = send
}

// Subscribe starts an informer for the given GVR+namespace if not already running.
// Returns the current cached items immediately.
func (s *Store) Subscribe(gvr schema.GroupVersionResource, namespace string) []*unstructured.Unstructured {
	key := watchKey{GVR: gvr, Namespace: namespace}

	s.mu.Lock()
	if _, exists := s.informers[key]; exists {
		s.mu.Unlock()
		return s.List(gvr, namespace)
	}

	// Initialize cache bucket
	if s.cache[key] == nil {
		s.cache[key] = make(map[string]*unstructured.Unstructured)
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.informers[key] = cancel
	s.mu.Unlock()

	go s.runInformer(ctx, key)

	return s.List(gvr, namespace)
}

// Unsubscribe stops the informer for a GVR+namespace and clears its cache.
func (s *Store) Unsubscribe(gvr schema.GroupVersionResource, namespace string) {
	key := watchKey{GVR: gvr, Namespace: namespace}
	s.mu.Lock()
	defer s.mu.Unlock()

	if cancel, ok := s.informers[key]; ok {
		cancel()
		delete(s.informers, key)
	}
	delete(s.cache, key)
	s.debouncer.Cancel(key)
}

// UnsubscribeAll stops all informers and clears all caches.
func (s *Store) UnsubscribeAll() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.debouncer.Stop()
	for _, cancel := range s.informers {
		cancel()
	}
	s.informers = make(map[watchKey]context.CancelFunc)
	s.cache = make(map[watchKey]map[string]*unstructured.Unstructured)
}

// List returns cached items for a GVR+namespace (no API call).
// The returned pointers are shared with the cache; callers must not mutate them.
func (s *Store) List(gvr schema.GroupVersionResource, namespace string) []*unstructured.Unstructured {
	key := watchKey{GVR: gvr, Namespace: namespace}
	s.mu.RLock()
	defer s.mu.RUnlock()

	bucket := s.cache[key]
	if bucket == nil {
		return nil
	}
	items := make([]*unstructured.Unstructured, 0, len(bucket))
	for _, obj := range bucket {
		items = append(items, obj)
	}
	return items
}

// cacheObjKey returns a unique key for an object within a watch bucket.
// For namespaced objects, includes namespace to avoid collisions in all-namespaces mode.
func cacheObjKey(obj *unstructured.Unstructured) string {
	if ns := obj.GetNamespace(); ns != "" {
		return ns + "/" + obj.GetName()
	}
	return obj.GetName()
}

// CacheUpsert adds or updates an object in the cache. Used internally and for testing.
func (s *Store) CacheUpsert(gvr schema.GroupVersionResource, namespace string, obj *unstructured.Unstructured) {
	key := watchKey{GVR: gvr, Namespace: namespace}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cache[key] == nil {
		s.cache[key] = make(map[string]*unstructured.Unstructured)
	}
	s.cache[key][cacheObjKey(obj)] = obj.DeepCopy()
}

// CacheDelete removes an object from the cache. Used internally and for testing.
func (s *Store) CacheDelete(gvr schema.GroupVersionResource, namespace string, obj *unstructured.Unstructured) {
	key := watchKey{GVR: gvr, Namespace: namespace}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cache[key] != nil {
		delete(s.cache[key], cacheObjKey(obj))
	}
}

func (s *Store) runInformer(ctx context.Context, key watchKey) {
	factory := dynamicinformer.NewFilteredDynamicSharedInformerFactory(
		s.client, 30*time.Second, key.Namespace, nil,
	)
	informer := factory.ForResource(key.GVR).Informer()

	// Suppress per-object notifications during initial list sync.
	// After sync completes, we send one notification with the full state.
	var synced atomic.Bool

	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			if u, ok := obj.(*unstructured.Unstructured); ok {
				s.CacheUpsert(key.GVR, key.Namespace, u)
				if synced.Load() {
					s.debouncer.Trigger(key)
				}
			}
		},
		UpdateFunc: func(_, obj any) {
			if u, ok := obj.(*unstructured.Unstructured); ok {
				s.CacheUpsert(key.GVR, key.Namespace, u)
				if synced.Load() {
					s.debouncer.Trigger(key)
				}
			}
		},
		DeleteFunc: func(obj any) {
			u, ok := obj.(*unstructured.Unstructured)
			if !ok {
				// Handle tombstone from missed watch events
				tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
				if !ok {
					return
				}
				u, ok = tombstone.Obj.(*unstructured.Unstructured)
				if !ok {
					return
				}
			}
			s.CacheDelete(key.GVR, key.Namespace, u)
			if synced.Load() {
				s.debouncer.Trigger(key)
			}
		},
	})

	factory.Start(ctx.Done())
	cache.WaitForCacheSync(ctx.Done(), informer.HasSynced)
	synced.Store(true)
	if ctx.Err() == nil {
		s.doNotify(key)
	}
	<-ctx.Done()
	factory.Shutdown()
}

func (s *Store) doNotify(key watchKey) {
	s.mu.RLock()
	send := s.send
	s.mu.RUnlock()
	if send != nil {
		send(ResourceUpdatedMsg{GVR: key.GVR, Namespace: key.Namespace})
	}
}

// ResourceUpdatedMsg is sent when informer data changes. Defined here to avoid import cycle.
// The app layer should match on this type.
type ResourceUpdatedMsg struct {
	GVR       schema.GroupVersionResource
	Namespace string
}
