package workload

import (
	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/plugin"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var PodsGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
var NodesGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "nodes"}

func FindOwned(store *k8s.Store, gvr schema.GroupVersionResource, namespace string, ownerUID string) []*unstructured.Unstructured {
	if store == nil || ownerUID == "" {
		return nil
	}
	all := store.List(gvr, namespace)
	var matched []*unstructured.Unstructured
	for _, obj := range all {
		refs, found, _ := unstructured.NestedSlice(obj.Object, "metadata", "ownerReferences")
		if !found {
			continue
		}
		for _, ref := range refs {
			refMap, ok := ref.(map[string]any)
			if !ok {
				continue
			}
			uid, _ := refMap["uid"].(string)
			if uid == ownerUID {
				matched = append(matched, obj)
				break
			}
		}
	}
	return matched
}

func FindOwnedPods(store *k8s.Store, namespace string, ownerUID string) []*unstructured.Unstructured {
	return FindOwned(store, PodsGVR, namespace, ownerUID)
}

// FindParentByOwnerRef resolves an object's owning Kubernetes parent from its
// metadata.ownerReferences. It is the shared backend for per-plugin DrillUp
// implementations (ownerReferences are universal). The controller owner is
// preferred; if none is marked controller the first ownerReference is used.
//
// Returns (nil, nil) when: there are no ownerReferences, the owner's
// apiVersion/kind does not resolve to a GVR on this cluster, or no plugin is
// registered for that GVR. When the owner resolves to a plugin but the matching
// object is not yet in the store (informer not synced), the non-nil plugin is
// still returned with a nil object so the caller can push an empty view that
// fills asynchronously.
func FindParentByOwnerRef(cl plugin.Cluster, obj *unstructured.Unstructured) (plugin.ResourcePlugin, *unstructured.Unstructured) {
	if obj == nil {
		return nil, nil
	}
	refs, found, _ := unstructured.NestedSlice(obj.Object, "metadata", "ownerReferences")
	if !found || len(refs) == 0 {
		return nil, nil
	}

	owner := pickOwnerRef(refs)
	if owner == nil {
		return nil, nil
	}
	apiVersion, _ := owner["apiVersion"].(string)
	kind, _ := owner["kind"].(string)
	ownerUID, _ := owner["uid"].(string)

	disc := plugin.DiscoveryOf(cl)
	if disc == nil {
		return nil, nil
	}
	gvr, ok := disc.ResolveGVR(apiVersion, kind)
	if !ok {
		return nil, nil
	}

	parentPlugin, ok := plugin.ByGVR(gvr)
	if !ok || parentPlugin == nil {
		return nil, nil
	}

	namespace := obj.GetNamespace()
	if parentPlugin.IsClusterScoped() {
		namespace = ""
	}

	store := plugin.StoreOf(cl)
	if store == nil {
		return parentPlugin, nil
	}
	store.Subscribe(gvr, namespace)
	for _, candidate := range store.List(gvr, namespace) {
		if string(candidate.GetUID()) == ownerUID {
			return parentPlugin, candidate
		}
	}
	return parentPlugin, nil
}

// pickOwnerRef chooses the ownerReference to follow: the one marked
// controller: true, else the first entry. Returns nil if no entry is a map.
func pickOwnerRef(refs []any) map[string]any {
	var first map[string]any
	for _, ref := range refs {
		refMap, ok := ref.(map[string]any)
		if !ok {
			continue
		}
		if first == nil {
			first = refMap
		}
		if controller, _ := refMap["controller"].(bool); controller {
			return refMap
		}
	}
	return first
}

var PVCsGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "persistentvolumeclaims"}
var JobsGVR = schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "jobs"}

func FindPodsByNodeName(store *k8s.Store, nodeName string) []*unstructured.Unstructured {
	if store == nil || nodeName == "" {
		return nil
	}
	all := store.List(PodsGVR, "")
	var matched []*unstructured.Unstructured
	for _, obj := range all {
		specNodeName, _, _ := unstructured.NestedString(obj.Object, "spec", "nodeName")
		if specNodeName == nodeName {
			matched = append(matched, obj)
		}
	}
	return matched
}

// FindNodeForPod resolves the Node a pod is scheduled onto from its
// spec.nodeName. It is the shared backend for per-plugin GoToNode
// implementations (the inverse of FindPodsByNodeName). Nodes are cluster-scoped,
// so the lookup uses the "" (all-namespaces) bucket.
//
// Returns (nil, nil) when: the object is nil, spec.nodeName is empty, or no
// plugin is registered under the name "nodes". When the nodes plugin is
// registered but the matching Node is not yet in the store (informer not
// synced), the non-nil plugin is still returned with a nil object so the caller
// can push an empty view that fills asynchronously.
func FindNodeForPod(cl plugin.Cluster, obj *unstructured.Unstructured) (plugin.ResourcePlugin, *unstructured.Unstructured) {
	if obj == nil {
		return nil, nil
	}
	nodeName, _, _ := unstructured.NestedString(obj.Object, "spec", "nodeName")
	if nodeName == "" {
		return nil, nil
	}
	nodesPlugin, ok := plugin.ByName("nodes")
	if !ok || nodesPlugin == nil {
		return nil, nil
	}
	store := plugin.StoreOf(cl)
	if store == nil {
		return nodesPlugin, nil
	}
	store.Subscribe(NodesGVR, "")
	for _, n := range store.List(NodesGVR, "") {
		if n.GetName() == nodeName {
			return nodesPlugin, n
		}
	}
	return nodesPlugin, nil
}

func FindPodsByVolumeClaim(store *k8s.Store, namespace, claimName string) []*unstructured.Unstructured {
	if store == nil || claimName == "" {
		return nil
	}
	all := store.List(PodsGVR, namespace)
	var matched []*unstructured.Unstructured
	for _, obj := range all {
		volumes, found, _ := unstructured.NestedSlice(obj.Object, "spec", "volumes")
		if !found {
			continue
		}
		for _, vol := range volumes {
			volMap, ok := vol.(map[string]any)
			if !ok {
				continue
			}
			cn, _, _ := unstructured.NestedString(volMap, "persistentVolumeClaim", "claimName")
			if cn == claimName {
				matched = append(matched, obj)
				break
			}
		}
	}
	return matched
}

func FindPVCByClaimRef(store *k8s.Store, namespace, claimName string) []*unstructured.Unstructured {
	if store == nil || claimName == "" {
		return nil
	}
	all := store.List(PVCsGVR, namespace)
	for _, pvc := range all {
		if pvc.GetName() == claimName {
			return []*unstructured.Unstructured{pvc}
		}
	}
	return nil
}
