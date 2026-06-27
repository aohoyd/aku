package pods

import (
	"testing"

	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/plugin/plugintest"
	"github.com/aohoyd/aku/internal/plugins/workload"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Compile-time assertion that *Plugin satisfies plugin.NodeLinker.
var _ plugin.NodeLinker = (*Plugin)(nil)

// podOnNode builds a pod with the given spec.nodeName.
func podOnNode(name, namespace, nodeName string) *unstructured.Unstructured {
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

// TestPodGoToNode verifies GoToNode is a thin delegation to
// workload.FindNodeForPod: for a representative input it returns exactly what
// the shared helper returns. The found-vs-not-found matrix lives in the workload
// FindNodeForPod test.
func TestPodGoToNode(t *testing.T) {
	plugin.Reset()
	nodesPlugin := &drillUpStub{name: "nodes", gvr: workload.NodesGVR, clusterScoped: true}
	plugin.Register(nodesPlugin)
	t.Cleanup(plugin.Reset)

	node := parentObj("node-1", "", "node-uid-1", "v1", "Node")
	store := k8s.NewStore(nil, "", nil)
	store.CacheUpsert(workload.NodesGVR, "", node)
	cl := plugintest.NewFakeCluster(store)

	pod := podOnNode("web-abc-xyz", "default", "node-1")

	gotPlugin, gotObj := (&Plugin{}).GoToNode(cl, pod)
	wantPlugin, wantObj := workload.FindNodeForPod(cl, pod)

	if gotPlugin != wantPlugin {
		t.Fatalf("plugin mismatch: got %v, want %v", gotPlugin, wantPlugin)
	}
	if gotObj != wantObj {
		t.Fatalf("object mismatch: got %v, want %v", gotObj, wantObj)
	}
	if gotPlugin != nodesPlugin {
		t.Fatalf("expected nodes plugin, got %v", gotPlugin)
	}
	if gotObj == nil || gotObj.GetName() != "node-1" {
		t.Fatalf("expected node-1 object, got %v", gotObj)
	}
}
