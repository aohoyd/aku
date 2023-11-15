package workload

import (
	"testing"

	"github.com/aohoyd/aku/internal/k8s"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// --- Helper constructors ---

func makeRoleBinding(name, namespace, refName, refKind string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{
			"name":      name,
			"namespace": namespace,
		},
		"roleRef": map[string]any{
			"name": refName,
			"kind": refKind,
		},
	}}
}

func makeClusterRoleBinding(name, refName, refKind string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{
			"name": name,
		},
		"roleRef": map[string]any{
			"name": refName,
			"kind": refKind,
		},
	}}
}

func makePVWithStorageClass(name, sc string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{
			"name": name,
		},
		"spec": map[string]any{
			"storageClassName": sc,
		},
	}}
}

func makeGateway(name, namespace, className string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{
			"name":      name,
			"namespace": namespace,
		},
		"spec": map[string]any{
			"gatewayClassName": className,
		},
	}}
}

func makeRoute(name, namespace, gwName, gwNamespace string) *unstructured.Unstructured {
	parentRef := map[string]any{
		"name": gwName,
	}
	if gwNamespace != "" {
		parentRef["namespace"] = gwNamespace
	}
	return &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{
			"name":      name,
			"namespace": namespace,
		},
		"spec": map[string]any{
			"parentRefs": []any{parentRef},
		},
	}}
}

// --- FindRoleBindingsByRoleRef tests ---

func TestFindRoleBindingsByRoleRef(t *testing.T) {
	rb1 := makeRoleBinding("rb-1", "default", "my-role", "Role")
	rb2 := makeRoleBinding("rb-2", "default", "my-role", "Role")
	rb3 := makeRoleBinding("rb-other", "default", "other-role", "Role")

	store := k8s.NewStore(nil, nil)
	store.CacheUpsert(RoleBindingsGVR, "default", rb1)
	store.CacheUpsert(RoleBindingsGVR, "default", rb2)
	store.CacheUpsert(RoleBindingsGVR, "default", rb3)

	result := FindRoleBindingsByRoleRef(store, "default", "my-role", "Role")
	if len(result) != 2 {
		t.Fatalf("expected 2 role bindings, got %d", len(result))
	}
}

func TestFindRoleBindingsByRoleRefNilStore(t *testing.T) {
	result := FindRoleBindingsByRoleRef(nil, "default", "my-role", "Role")
	if result != nil {
		t.Fatal("expected nil for nil store")
	}
}

func TestFindRoleBindingsByRoleRefKindMismatch(t *testing.T) {
	rb1 := makeRoleBinding("rb-1", "default", "my-role", "ClusterRole")

	store := k8s.NewStore(nil, nil)
	store.CacheUpsert(RoleBindingsGVR, "default", rb1)

	result := FindRoleBindingsByRoleRef(store, "default", "my-role", "Role")
	if len(result) != 0 {
		t.Fatalf("expected 0 role bindings (kind mismatch), got %d", len(result))
	}
}

// --- FindClusterRoleBindingsByRoleRef tests ---

func TestFindClusterRoleBindingsByRoleRef(t *testing.T) {
	crb1 := makeClusterRoleBinding("crb-1", "my-cluster-role", "ClusterRole")
	crb2 := makeClusterRoleBinding("crb-2", "my-cluster-role", "ClusterRole")
	crb3 := makeClusterRoleBinding("crb-other", "other-role", "ClusterRole")

	store := k8s.NewStore(nil, nil)
	store.CacheUpsert(ClusterRoleBindingsGVR, "", crb1)
	store.CacheUpsert(ClusterRoleBindingsGVR, "", crb2)
	store.CacheUpsert(ClusterRoleBindingsGVR, "", crb3)

	result := FindClusterRoleBindingsByRoleRef(store, "my-cluster-role", "ClusterRole")
	if len(result) != 2 {
		t.Fatalf("expected 2 cluster role bindings, got %d", len(result))
	}
}

func TestFindClusterRoleBindingsByRoleRefNilStore(t *testing.T) {
	result := FindClusterRoleBindingsByRoleRef(nil, "my-cluster-role", "ClusterRole")
	if result != nil {
		t.Fatal("expected nil for nil store")
	}
}

// --- FindPVsByStorageClass tests ---

func TestFindPVsByStorageClass(t *testing.T) {
	pv1 := makePVWithStorageClass("pv-1", "standard")
	pv2 := makePVWithStorageClass("pv-2", "standard")
	pv3 := makePVWithStorageClass("pv-other", "premium")

	store := k8s.NewStore(nil, nil)
	store.CacheUpsert(PersistentVolumesGVR, "", pv1)
	store.CacheUpsert(PersistentVolumesGVR, "", pv2)
	store.CacheUpsert(PersistentVolumesGVR, "", pv3)

	result := FindPVsByStorageClass(store, "standard")
	if len(result) != 2 {
		t.Fatalf("expected 2 PVs, got %d", len(result))
	}
}

func TestFindPVsByStorageClassNilStore(t *testing.T) {
	result := FindPVsByStorageClass(nil, "standard")
	if result != nil {
		t.Fatal("expected nil for nil store")
	}
}

func TestFindPVsByStorageClassEmpty(t *testing.T) {
	store := k8s.NewStore(nil, nil)
	result := FindPVsByStorageClass(store, "")
	if result != nil {
		t.Fatal("expected nil for empty storageClassName")
	}
}

// --- FindGatewaysByClassName tests ---

func TestFindGatewaysByClassName(t *testing.T) {
	gw1 := makeGateway("gw-1", "default", "my-class")
	gw2 := makeGateway("gw-2", "default", "my-class")
	gw3 := makeGateway("gw-other", "default", "other-class")

	store := k8s.NewStore(nil, nil)
	store.CacheUpsert(GatewaysGVR, "default", gw1)
	store.CacheUpsert(GatewaysGVR, "default", gw2)
	store.CacheUpsert(GatewaysGVR, "default", gw3)

	result := FindGatewaysByClassName(store, "default", "my-class")
	if len(result) != 2 {
		t.Fatalf("expected 2 gateways, got %d", len(result))
	}
}

func TestFindGatewaysByClassNameNilStore(t *testing.T) {
	result := FindGatewaysByClassName(nil, "default", "my-class")
	if result != nil {
		t.Fatal("expected nil for nil store")
	}
}

// --- FindRoutesByGateway tests ---

func TestFindRoutesByGateway(t *testing.T) {
	httpRoute := makeRoute("http-route-1", "default", "my-gw", "default")
	grpcRoute := makeRoute("grpc-route-1", "default", "my-gw", "default")
	noMatch := makeRoute("http-route-other", "default", "other-gw", "default")

	store := k8s.NewStore(nil, nil)
	store.CacheUpsert(HTTPRoutesGVR, "", httpRoute)
	store.CacheUpsert(GRPCRoutesGVR, "", grpcRoute)
	store.CacheUpsert(HTTPRoutesGVR, "", noMatch)

	result := FindRoutesByGateway(store, "default", "my-gw")
	if len(result) != 2 {
		t.Fatalf("expected 2 routes, got %d", len(result))
	}
}

func TestFindRoutesByGatewayImplicitNamespace(t *testing.T) {
	// parentRef without namespace should default to route's own namespace
	route := makeRoute("http-route-1", "default", "my-gw", "")

	store := k8s.NewStore(nil, nil)
	store.CacheUpsert(HTTPRoutesGVR, "", route)

	result := FindRoutesByGateway(store, "default", "my-gw")
	if len(result) != 1 {
		t.Fatalf("expected 1 route (implicit namespace), got %d", len(result))
	}
}

func TestFindRoutesByGatewayNilStore(t *testing.T) {
	result := FindRoutesByGateway(nil, "default", "my-gw")
	if result != nil {
		t.Fatal("expected nil for nil store")
	}
}

func TestFindRoutesByGatewayNoMatch(t *testing.T) {
	route := makeRoute("http-route-1", "default", "my-gw", "other-namespace")

	store := k8s.NewStore(nil, nil)
	store.CacheUpsert(HTTPRoutesGVR, "", route)

	result := FindRoutesByGateway(store, "default", "my-gw")
	if len(result) != 0 {
		t.Fatalf("expected 0 routes (different namespace), got %d", len(result))
	}
}
