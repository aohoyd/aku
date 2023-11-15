package workload

import (
	"github.com/aohoyd/aku/internal/k8s"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var PodsGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}

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
