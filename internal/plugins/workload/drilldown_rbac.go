package workload

import (
	"github.com/aohoyd/aku/internal/k8s"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var RoleBindingsGVR = schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "rolebindings"}
var ClusterRoleBindingsGVR = schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterrolebindings"}
var PersistentVolumesGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "persistentvolumes"}
var GatewaysGVR = schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "gateways"}
var HTTPRoutesGVR = schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "httproutes"}
var GRPCRoutesGVR = schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "grpcroutes"}

func FindRoleBindingsByRoleRef(store *k8s.Store, namespace, roleName, roleKind string) []*unstructured.Unstructured {
	if store == nil || roleName == "" {
		return nil
	}
	all := store.List(RoleBindingsGVR, namespace)
	var matched []*unstructured.Unstructured
	for _, obj := range all {
		refName, _, _ := unstructured.NestedString(obj.Object, "roleRef", "name")
		refKind, _, _ := unstructured.NestedString(obj.Object, "roleRef", "kind")
		if refName == roleName && refKind == roleKind {
			matched = append(matched, obj)
		}
	}
	return matched
}

func FindClusterRoleBindingsByRoleRef(store *k8s.Store, roleName, roleKind string) []*unstructured.Unstructured {
	if store == nil || roleName == "" {
		return nil
	}
	all := store.List(ClusterRoleBindingsGVR, "")
	var matched []*unstructured.Unstructured
	for _, obj := range all {
		refName, _, _ := unstructured.NestedString(obj.Object, "roleRef", "name")
		refKind, _, _ := unstructured.NestedString(obj.Object, "roleRef", "kind")
		if refName == roleName && refKind == roleKind {
			matched = append(matched, obj)
		}
	}
	return matched
}

func FindPVsByStorageClass(store *k8s.Store, storageClassName string) []*unstructured.Unstructured {
	if store == nil || storageClassName == "" {
		return nil
	}
	all := store.List(PersistentVolumesGVR, "")
	var matched []*unstructured.Unstructured
	for _, obj := range all {
		sc, _, _ := unstructured.NestedString(obj.Object, "spec", "storageClassName")
		if sc == storageClassName {
			matched = append(matched, obj)
		}
	}
	return matched
}

func FindGatewaysByClassName(store *k8s.Store, namespace, className string) []*unstructured.Unstructured {
	if store == nil || className == "" {
		return nil
	}
	all := store.List(GatewaysGVR, namespace)
	var matched []*unstructured.Unstructured
	for _, obj := range all {
		cn, _, _ := unstructured.NestedString(obj.Object, "spec", "gatewayClassName")
		if cn == className {
			matched = append(matched, obj)
		}
	}
	return matched
}

func FindHTTPRoutesByGateway(store *k8s.Store, gatewayNamespace, gatewayName string) []*unstructured.Unstructured {
	if store == nil || gatewayName == "" {
		return nil
	}
	all := store.List(HTTPRoutesGVR, "")
	var matched []*unstructured.Unstructured
	for _, obj := range all {
		if routeReferencesGateway(obj, gatewayNamespace, gatewayName) {
			matched = append(matched, obj)
		}
	}
	return matched
}

func FindRoutesByGateway(store *k8s.Store, gatewayNamespace, gatewayName string) []*unstructured.Unstructured {
	if store == nil || gatewayName == "" {
		return nil
	}
	var matched []*unstructured.Unstructured
	for _, gvr := range []schema.GroupVersionResource{HTTPRoutesGVR, GRPCRoutesGVR} {
		all := store.List(gvr, "")
		for _, obj := range all {
			if routeReferencesGateway(obj, gatewayNamespace, gatewayName) {
				matched = append(matched, obj)
			}
		}
	}
	return matched
}

func routeReferencesGateway(obj *unstructured.Unstructured, gwNamespace, gwName string) bool {
	parentRefs, found, _ := unstructured.NestedSlice(obj.Object, "spec", "parentRefs")
	if !found {
		return false
	}
	routeNamespace := obj.GetNamespace()
	for _, ref := range parentRefs {
		refMap, ok := ref.(map[string]any)
		if !ok {
			continue
		}
		name, _ := refMap["name"].(string)
		ns, _ := refMap["namespace"].(string)
		if ns == "" {
			ns = routeNamespace
		}
		if name == gwName && ns == gwNamespace {
			return true
		}
	}
	return false
}
