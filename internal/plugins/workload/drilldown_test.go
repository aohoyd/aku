package workload

import (
	"testing"

	"github.com/aohoyd/aku/internal/k8s"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func newTestStore(gvr schema.GroupVersionResource, namespace string, objs []*unstructured.Unstructured) *k8s.Store {
	store := k8s.NewStore(nil, nil)
	for _, obj := range objs {
		store.CacheUpsert(gvr, namespace, obj)
	}
	return store
}

func makeOwnedObj(name, namespace, ownerUID string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{
			"name":      name,
			"namespace": namespace,
			"ownerReferences": []any{
				map[string]any{"uid": ownerUID},
			},
		},
	}}
}

func TestFindOwned(t *testing.T) {
	rsGVR := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "replicasets"}
	owned1 := makeOwnedObj("rs-1", "default", "deploy-uid-1")
	owned2 := makeOwnedObj("rs-2", "default", "deploy-uid-1")
	unowned := makeOwnedObj("rs-other", "default", "deploy-uid-2")
	store := newTestStore(rsGVR, "default", []*unstructured.Unstructured{owned1, owned2, unowned})

	result := FindOwned(store, rsGVR, "default", "deploy-uid-1")
	if len(result) != 2 {
		t.Fatalf("expected 2 owned objects, got %d", len(result))
	}
}

func TestFindOwnedNilStore(t *testing.T) {
	rsGVR := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "replicasets"}
	result := FindOwned(nil, rsGVR, "default", "uid-1")
	if result != nil {
		t.Fatal("expected nil for nil store")
	}
}

func TestFindOwnedEmptyUID(t *testing.T) {
	rsGVR := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "replicasets"}
	store := k8s.NewStore(nil, nil)
	result := FindOwned(store, rsGVR, "default", "")
	if result != nil {
		t.Fatal("expected nil for empty ownerUID")
	}
}

func TestFindOwnedPodsWrapper(t *testing.T) {
	pod := makeOwnedObj("pod-1", "default", "rs-uid-1")
	store := newTestStore(PodsGVR, "default", []*unstructured.Unstructured{pod})

	result := FindOwnedPods(store, "default", "rs-uid-1")
	if len(result) != 1 {
		t.Fatalf("expected 1 owned pod, got %d", len(result))
	}
}

func makePodOnNode(name, namespace, nodeName string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{
			"name":      name,
			"namespace": namespace,
		},
		"spec": map[string]any{
			"nodeName": nodeName,
		},
	}}
}

func TestFindPodsByNodeName(t *testing.T) {
	pod1 := makePodOnNode("pod-1", "default", "node-1")
	pod2 := makePodOnNode("pod-2", "kube-system", "node-1")
	pod3 := makePodOnNode("pod-3", "default", "node-2")

	store := k8s.NewStore(nil, nil)
	store.CacheUpsert(PodsGVR, "", pod1)
	store.CacheUpsert(PodsGVR, "", pod2)
	store.CacheUpsert(PodsGVR, "", pod3)

	result := FindPodsByNodeName(store, "node-1")
	if len(result) != 2 {
		t.Fatalf("expected 2 pods on node-1, got %d", len(result))
	}
}

func TestFindPodsByNodeNameNilStore(t *testing.T) {
	result := FindPodsByNodeName(nil, "node-1")
	if result != nil {
		t.Fatal("expected nil for nil store")
	}
}

func makePodWithPVC(name, namespace, pvcName string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{
			"name":      name,
			"namespace": namespace,
		},
		"spec": map[string]any{
			"volumes": []any{
				map[string]any{
					"name": "data",
					"persistentVolumeClaim": map[string]any{
						"claimName": pvcName,
					},
				},
			},
		},
	}}
}

func TestFindPodsByVolumeClaim(t *testing.T) {
	pod1 := makePodWithPVC("pod-1", "default", "my-pvc")
	pod2 := makePodWithPVC("pod-2", "default", "other-pvc")

	store := k8s.NewStore(nil, nil)
	store.CacheUpsert(PodsGVR, "default", pod1)
	store.CacheUpsert(PodsGVR, "default", pod2)

	result := FindPodsByVolumeClaim(store, "default", "my-pvc")
	if len(result) != 1 {
		t.Fatalf("expected 1 pod using my-pvc, got %d", len(result))
	}
}

func TestFindPodsByVolumeClaimNilStore(t *testing.T) {
	result := FindPodsByVolumeClaim(nil, "default", "my-pvc")
	if result != nil {
		t.Fatal("expected nil for nil store")
	}
}

func makePVC(name, namespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{
			"name":      name,
			"namespace": namespace,
		},
	}}
}

func TestFindPVCByClaimRef(t *testing.T) {
	pvc := makePVC("my-pvc", "default")

	store := k8s.NewStore(nil, nil)
	store.CacheUpsert(PVCsGVR, "default", pvc)

	result := FindPVCByClaimRef(store, "default", "my-pvc")
	if len(result) != 1 {
		t.Fatalf("expected 1 PVC, got %d", len(result))
	}
}

func TestFindPVCByClaimRefNotFound(t *testing.T) {
	store := k8s.NewStore(nil, nil)
	result := FindPVCByClaimRef(store, "default", "nonexistent")
	if len(result) != 0 {
		t.Fatalf("expected 0 PVCs, got %d", len(result))
	}
}
