package workload

import (
	"testing"

	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/plugin"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// podWithNodeName builds a namespaced pod object carrying spec.nodeName.
func podWithNodeName(name, namespace, nodeName string) *unstructured.Unstructured {
	spec := map[string]any{}
	if nodeName != "" {
		spec["nodeName"] = nodeName
	}
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata":   map[string]any{"name": name, "namespace": namespace},
		"spec":       spec,
	}}
}

func TestFindNodeForPod(t *testing.T) {
	t.Run("match found returns plugin and node", func(t *testing.T) {
		plugin.Reset()
		t.Cleanup(plugin.Reset)
		nodesPlugin := &stubPlugin{name: "nodes", gvr: NodesGVR, clusterScoped: true}
		plugin.Register(nodesPlugin)

		node := namedObj("node-1", "", "node-uid-1", "v1", "Node")
		store := k8s.NewStore(nil, "", nil)
		store.CacheUpsert(NodesGVR, "", node)
		cl := &testCluster{store: store, discovery: k8s.NewDiscovery()}

		pod := podWithNodeName("web", "default", "node-1")
		gotPlugin, gotObj := FindNodeForPod(cl, pod)
		if gotPlugin != nodesPlugin {
			t.Fatalf("expected nodes plugin, got %v", gotPlugin)
		}
		if gotObj == nil || gotObj.GetName() != "node-1" {
			t.Fatalf("expected node-1, got %v", gotObj)
		}
	})

	t.Run("empty spec.nodeName returns nil", func(t *testing.T) {
		plugin.Reset()
		t.Cleanup(plugin.Reset)
		plugin.Register(&stubPlugin{name: "nodes", gvr: NodesGVR, clusterScoped: true})

		store := k8s.NewStore(nil, "", nil)
		cl := &testCluster{store: store, discovery: k8s.NewDiscovery()}

		pod := podWithNodeName("web", "default", "")
		if p, o := FindNodeForPod(cl, pod); p != nil || o != nil {
			t.Fatalf("expected (nil, nil) for empty nodeName, got (%v, %v)", p, o)
		}
	})

	t.Run("nil obj returns nil", func(t *testing.T) {
		plugin.Reset()
		t.Cleanup(plugin.Reset)
		plugin.Register(&stubPlugin{name: "nodes", gvr: NodesGVR, clusterScoped: true})

		cl := &testCluster{store: k8s.NewStore(nil, "", nil), discovery: k8s.NewDiscovery()}
		if p, o := FindNodeForPod(cl, nil); p != nil || o != nil {
			t.Fatalf("expected (nil, nil) for nil obj, got (%v, %v)", p, o)
		}
	})

	t.Run("node not cached returns plugin with nil object", func(t *testing.T) {
		plugin.Reset()
		t.Cleanup(plugin.Reset)
		nodesPlugin := &stubPlugin{name: "nodes", gvr: NodesGVR, clusterScoped: true}
		plugin.Register(nodesPlugin)

		// Nothing in the store yet.
		store := k8s.NewStore(nil, "", nil)
		cl := &testCluster{store: store, discovery: k8s.NewDiscovery()}

		pod := podWithNodeName("web", "default", "node-1")
		gotPlugin, gotObj := FindNodeForPod(cl, pod)
		if gotPlugin != nodesPlugin {
			t.Fatalf("expected nodes plugin (lazy), got %v", gotPlugin)
		}
		if gotObj != nil {
			t.Fatalf("expected nil object for unsynced node, got %q", gotObj.GetName())
		}
	})

	t.Run("nil store returns plugin with nil object (lazy view)", func(t *testing.T) {
		plugin.Reset()
		t.Cleanup(plugin.Reset)
		nodesPlugin := &stubPlugin{name: "nodes", gvr: NodesGVR, clusterScoped: true}
		plugin.Register(nodesPlugin)

		// Cluster whose Store() returns nil (lazy view not yet backed by a store).
		cl := &testCluster{store: nil, discovery: k8s.NewDiscovery()}

		pod := podWithNodeName("web", "default", "node-1")
		gotPlugin, gotObj := FindNodeForPod(cl, pod)
		if gotPlugin != nodesPlugin {
			t.Fatalf("expected nodes plugin (lazy), got %v", gotPlugin)
		}
		if gotObj != nil {
			t.Fatalf("expected nil object for nil store, got %q", gotObj.GetName())
		}
	})

	t.Run("no nodes plugin registered returns nil", func(t *testing.T) {
		plugin.Reset()
		t.Cleanup(plugin.Reset)

		node := namedObj("node-1", "", "node-uid-1", "v1", "Node")
		store := k8s.NewStore(nil, "", nil)
		store.CacheUpsert(NodesGVR, "", node)
		cl := &testCluster{store: store, discovery: k8s.NewDiscovery()}

		pod := podWithNodeName("web", "default", "node-1")
		if p, o := FindNodeForPod(cl, pod); p != nil || o != nil {
			t.Fatalf("expected (nil, nil) with no nodes plugin, got (%v, %v)", p, o)
		}
	})
}
