package workload

import (
	"github.com/aohoyd/aku/internal/plugin"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// EndpointSlicesGVR is the single-source discovery.k8s.io/v1 endpointslices GVR
// referenced by the endpointslices plugin.
var EndpointSlicesGVR = schema.GroupVersionResource{Group: "discovery.k8s.io", Version: "v1", Resource: "endpointslices"}

// ServiceNameLabel links an EndpointSlice to its backing Service. The K8s
// EndpointSlice controller stamps it on every slice it manages.
const ServiceNameLabel = "kubernetes.io/service-name"

// FindEndpointSlicesForService resolves the EndpointSlice objects backing a
// Service. A slice is linked to its Service via the kubernetes.io/service-name
// label (NOT a name match), and a Service may have many sharded slices, so this
// returns a LIST of every slice in the Service's namespace whose
// kubernetes.io/service-name label equals the Service's name. It is the shared
// backend for the services plugin's DrillDown (svc → endpointslice).
//
// Returns (nil, nil) when the object is nil or no plugin is registered under the
// name "endpointslices". When the endpointslices plugin is registered but no
// store is available (lazy view not yet backed by a store) or no matching slice
// is present (e.g. ExternalName/selectorless services, or the informer not yet
// synced), the non-nil plugin is returned with a nil slice so the caller can
// push an empty view that fills asynchronously.
func FindEndpointSlicesForService(cl plugin.Cluster, svc *unstructured.Unstructured) (plugin.ResourcePlugin, []*unstructured.Unstructured) {
	if svc == nil {
		return nil, nil
	}
	esPlugin, ok := plugin.ByName("endpointslices")
	if !ok || esPlugin == nil {
		return nil, nil
	}
	store := plugin.StoreOf(cl)
	if store == nil {
		return esPlugin, nil
	}
	store.Subscribe(EndpointSlicesGVR, svc.GetNamespace())
	var result []*unstructured.Unstructured
	for _, slice := range store.List(EndpointSlicesGVR, svc.GetNamespace()) {
		if slice.GetLabels()[ServiceNameLabel] == svc.GetName() {
			result = append(result, slice)
		}
	}
	return esPlugin, result
}

// FindServiceForEndpointSlice resolves the Service owning an EndpointSlice. The
// slice carries its Service's name in the kubernetes.io/service-name label (NOT
// an ownerReference), so the lookup reads the kubernetes.io/service-name label
// value and finds the Service with that name in the slice's namespace bucket. It
// is the shared backend for the
// endpointslices plugin's DrillUp (endpointslice → svc); it is the inverse of
// FindEndpointSlicesForService.
//
// Returns (nil, nil) when the object is nil or no plugin is registered under the
// name "services". When the services plugin is registered but no store is
// available (lazy view not yet backed by a store), the slice carries no
// kubernetes.io/service-name label (an orphaned slice), or no matching Service
// object is present (the informer not yet synced), the non-nil plugin is
// returned with a nil object so the caller can push an empty view that fills
// asynchronously.
func FindServiceForEndpointSlice(cl plugin.Cluster, slice *unstructured.Unstructured) (plugin.ResourcePlugin, *unstructured.Unstructured) {
	if slice == nil {
		return nil, nil
	}
	svcPlugin, ok := plugin.ByName("services")
	if !ok || svcPlugin == nil {
		return nil, nil
	}
	svcName := slice.GetLabels()[ServiceNameLabel]
	if svcName == "" {
		// Orphaned slice: no service-name label, nothing to resolve.
		return svcPlugin, nil
	}
	store := plugin.StoreOf(cl)
	if store == nil {
		return svcPlugin, nil
	}
	store.Subscribe(ServicesGVR, slice.GetNamespace())
	for _, svc := range store.List(ServicesGVR, slice.GetNamespace()) {
		if svc.GetName() == svcName {
			return svcPlugin, svc
		}
	}
	return svcPlugin, nil
}

// FindPodsByEndpointSlice resolves the backing Pods of an EndpointSlice by
// walking its flat endpoints[] (an EndpointSlice has NO subsets), reading each
// endpoint's targetRef of kind Pod, and resolving that pod by name within the
// targetRef's namespace (falling back to the slice's namespace). It is the
// shared backend for the endpointslices plugin's DrillDown (endpointslice →
// pods). ALL endpoints are included regardless of conditions.ready/serving/
// terminating.
//
// Returns (nil, nil) when the object is nil or no plugin is registered under the
// name "pods". When the pods plugin is registered but no store is available, the
// non-nil plugin is returned with a nil slice so the caller can push an empty
// view that fills asynchronously. A targetRef whose pod is not yet cached is
// skipped — it will appear on the next refresh.
//
// Pods are deduplicated by targetRef.uid when present, else by the resolved
// pod's namespace/name composite (the K8s uniqueness invariant for pods), so
// distinct pods with blank metadata.uid are not collapsed onto each other.
// Per-namespace pod indexes are built once to avoid repeated store.List calls
// inside the endpoints loop.
func FindPodsByEndpointSlice(cl plugin.Cluster, slice *unstructured.Unstructured) (plugin.ResourcePlugin, []*unstructured.Unstructured) {
	if slice == nil {
		return nil, nil
	}
	podsPlugin, ok := plugin.ByName("pods")
	if !ok || podsPlugin == nil {
		return nil, nil
	}
	store := plugin.StoreOf(cl)
	if store == nil {
		return podsPlugin, nil
	}

	endpoints, found, _ := unstructured.NestedSlice(slice.Object, "endpoints")
	if !found || len(endpoints) == 0 {
		return podsPlugin, nil
	}

	// Pod index per distinct namespace, built lazily once.
	byNamespace := map[string]map[string]*unstructured.Unstructured{}
	podsInNamespace := func(ns string) map[string]*unstructured.Unstructured {
		if idx, ok := byNamespace[ns]; ok {
			return idx
		}
		store.Subscribe(PodsGVR, ns)
		idx := map[string]*unstructured.Unstructured{}
		for _, pod := range store.List(PodsGVR, ns) {
			idx[pod.GetName()] = pod
		}
		byNamespace[ns] = idx
		return idx
	}

	var result []*unstructured.Unstructured
	seen := map[string]bool{}
	for _, ep := range endpoints {
		epMap, ok := ep.(map[string]any)
		if !ok {
			continue
		}
		targetRef, found, _ := unstructured.NestedMap(epMap, "targetRef")
		if !found || targetRef == nil {
			continue
		}
		if kind, _ := targetRef["kind"].(string); kind != "Pod" {
			continue
		}
		name, _ := targetRef["name"].(string)
		if name == "" {
			// Malformed targetRef with no name: nothing to resolve, and
			// resolving its namespace would issue a spurious subscription.
			continue
		}
		ns, _ := targetRef["namespace"].(string)
		if ns == "" {
			ns = slice.GetNamespace()
		}
		pod, ok := podsInNamespace(ns)[name]
		if !ok || pod == nil {
			continue
		}
		key, _ := targetRef["uid"].(string)
		if key == "" {
			// No targetRef.uid: fall back to the resolved pod's
			// namespace/name composite, the K8s uniqueness invariant.
			// (Pod metadata.uid alone would collapse distinct
			// blank-uid pods onto a single "" key.)
			key = pod.GetNamespace() + "/" + pod.GetName()
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, pod)
	}
	return podsPlugin, result
}
