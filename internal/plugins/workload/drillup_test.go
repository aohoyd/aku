package workload

import (
	"cmp"
	"context"
	"testing"

	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/render"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// stubPlugin is a minimal plugin.ResourcePlugin for registry-driven tests.
type stubPlugin struct {
	name          string
	gvr           schema.GroupVersionResource
	clusterScoped bool
}

func (p *stubPlugin) Name() string                     { return p.name }
func (p *stubPlugin) ShortName() string                { return p.name }
func (p *stubPlugin) GVR() schema.GroupVersionResource { return p.gvr }
func (p *stubPlugin) IsClusterScoped() bool            { return p.clusterScoped }
func (p *stubPlugin) Columns() []plugin.Column         { return nil }
func (p *stubPlugin) Row(*unstructured.Unstructured) []string {
	return nil
}
func (p *stubPlugin) YAML(obj *unstructured.Unstructured) (render.Content, error) {
	return render.Content{}, nil
}
func (p *stubPlugin) Describe(context.Context, *unstructured.Unstructured) (render.Content, error) {
	return render.Content{}, nil
}

// testCluster satisfies plugin.Cluster with an in-memory store + discovery.
type testCluster struct {
	store     *k8s.Store
	discovery *k8s.Discovery
}

func (c *testCluster) Store() *k8s.Store         { return c.store }
func (c *testCluster) Discovery() *k8s.Discovery { return c.discovery }

// discoveryFor builds a Discovery index covering the given API resources.
func discoveryFor(resources ...k8s.APIResource) *k8s.Discovery {
	d := k8s.NewDiscovery()
	d.Populate(resources)
	return d
}

var (
	deployGVR = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	rsGVR     = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "replicasets"}
)

func deploymentResource() k8s.APIResource {
	return k8s.APIResource{
		Name: "deployments", APIVersion: "apps/v1", Group: "apps", Version: "v1",
		Kind: "Deployment", Namespaced: true, GVR: deployGVR,
	}
}

func replicaSetResource() k8s.APIResource {
	return k8s.APIResource{
		Name: "replicasets", APIVersion: "apps/v1", Group: "apps", Version: "v1",
		Kind: "ReplicaSet", Namespaced: true, GVR: rsGVR,
	}
}

// objWithOwnerRefs builds a namespaced object carrying the given ownerReferences.
func objWithOwnerRefs(name, namespace string, refs ...map[string]any) *unstructured.Unstructured {
	anyRefs := make([]any, len(refs))
	for i, r := range refs {
		anyRefs[i] = r
	}
	return &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{
			"name":            name,
			"namespace":       namespace,
			"ownerReferences": anyRefs,
		},
	}}
}

// namedObj builds an object with a metadata.uid for store matching.
func namedObj(name, namespace, uid, apiVersion, kind string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": apiVersion,
		"kind":       kind,
		"metadata": map[string]any{
			"name":      name,
			"namespace": namespace,
			"uid":       uid,
		},
	}}
}

func ownerRef(apiVersion, kind, name, uid string, controller bool) map[string]any {
	return map[string]any{
		"apiVersion": apiVersion,
		"kind":       kind,
		"name":       name,
		"uid":        uid,
		"controller": controller,
	}
}

func TestFindParentByOwnerRef(t *testing.T) {
	plugin.Reset()
	rsPlugin := &stubPlugin{name: "replicasets", gvr: rsGVR}
	deployPlugin := &stubPlugin{name: "deployments", gvr: deployGVR}
	plugin.Register(rsPlugin)
	plugin.Register(deployPlugin)
	t.Cleanup(plugin.Reset)

	disc := discoveryFor(deploymentResource(), replicaSetResource())

	// discWithSvc additionally resolves Service so the "owner kind has no
	// registered plugin" case can resolve a GVR that has no registered plugin.
	discWithSvc := discoveryFor(
		deploymentResource(), replicaSetResource(),
		k8s.APIResource{
			Name: "services", APIVersion: "v1", Group: "", Version: "v1",
			Kind: "Service", Namespaced: true,
			GVR: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "services"},
		},
	)

	deploy := namedObj("web", "default", "deploy-uid-1", "apps/v1", "Deployment")
	rs := namedObj("web-abc", "default", "rs-uid-1", "apps/v1", "ReplicaSet")

	tests := []struct {
		name        string
		obj         *unstructured.Unstructured
		disc        *k8s.Discovery // per-case discovery (defaults to disc when nil)
		storeGVR    schema.GroupVersionResource
		storeNS     string
		storeObjs   []*unstructured.Unstructured
		wantPlugin  plugin.ResourcePlugin
		wantObjName string // "" means nil object expected
	}{
		{
			name: "controller owner picked over non-controller",
			obj: objWithOwnerRefs("web-abc", "default",
				ownerRef("apps/v1", "ReplicaSet", "ignored-rs", "rs-uid-other", false),
				ownerRef("apps/v1", "Deployment", "web", "deploy-uid-1", true),
			),
			storeGVR:    deployGVR,
			storeNS:     "default",
			storeObjs:   []*unstructured.Unstructured{deploy},
			wantPlugin:  deployPlugin,
			wantObjName: "web",
		},
		{
			name: "first owner used when none marked controller",
			obj: objWithOwnerRefs("web-abc", "default",
				ownerRef("apps/v1", "ReplicaSet", "web-abc-rs", "rs-uid-1", false),
				ownerRef("apps/v1", "Deployment", "web", "deploy-uid-1", false),
			),
			storeGVR:    rsGVR,
			storeNS:     "default",
			storeObjs:   []*unstructured.Unstructured{rs},
			wantPlugin:  rsPlugin,
			wantObjName: "web-abc",
		},
		{
			name:       "no ownerReferences returns nil",
			obj:        namedObj("lonely", "default", "x", "apps/v1", "ReplicaSet"),
			wantPlugin: nil,
		},
		{
			name: "unresolvable kind returns nil",
			obj: objWithOwnerRefs("thing", "default",
				ownerRef("example.com/v1", "Widget", "w", "widget-uid", true),
			),
			wantPlugin: nil,
		},
		{
			name: "UID match returns correct object",
			obj: objWithOwnerRefs("web-abc", "default",
				ownerRef("apps/v1", "Deployment", "web", "deploy-uid-1", true),
			),
			storeGVR:    deployGVR,
			storeNS:     "default",
			storeObjs:   []*unstructured.Unstructured{deploy},
			wantPlugin:  deployPlugin,
			wantObjName: "web",
		},
		{
			name: "owner kind has no registered plugin returns nil",
			obj: objWithOwnerRefs("ep", "default",
				// Service resolves via discovery but no plugin is registered for it.
				ownerRef("v1", "Service", "svc", "svc-uid", true),
			),
			disc:       discWithSvc,
			wantPlugin: nil,
		},
		{
			name: "empty store bucket still returns plugin with nil object",
			obj: objWithOwnerRefs("web-abc", "default",
				ownerRef("apps/v1", "Deployment", "web", "deploy-uid-1", true),
			),
			storeGVR:    deployGVR,
			storeNS:     "default",
			storeObjs:   nil, // nothing in the store yet
			wantPlugin:  deployPlugin,
			wantObjName: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := cmp.Or(tt.disc, disc)
			store := k8s.NewStore(nil, "", nil)
			for _, o := range tt.storeObjs {
				store.CacheUpsert(tt.storeGVR, tt.storeNS, o)
			}
			cl := &testCluster{store: store, discovery: d}

			gotPlugin, gotObj := FindParentByOwnerRef(cl, tt.obj)

			if tt.wantPlugin == nil {
				if gotPlugin != nil {
					t.Fatalf("expected nil plugin, got %T (%s)", gotPlugin, gotPlugin.Name())
				}
				if gotObj != nil {
					t.Fatalf("expected nil object when plugin is nil, got %v", gotObj.GetName())
				}
				return
			}

			if gotPlugin != tt.wantPlugin {
				t.Fatalf("plugin mismatch: got %v, want %v", gotPlugin, tt.wantPlugin)
			}
			if tt.wantObjName == "" {
				if gotObj != nil {
					t.Fatalf("expected nil object, got %q", gotObj.GetName())
				}
				return
			}
			if gotObj == nil {
				t.Fatalf("expected object %q, got nil", tt.wantObjName)
			}
			if gotObj.GetName() != tt.wantObjName {
				t.Fatalf("object name mismatch: got %q, want %q", gotObj.GetName(), tt.wantObjName)
			}
		})
	}
}

// TestFindParentByOwnerRefClusterScopedNamespace verifies that a cluster-scoped
// parent plugin is looked up in the "" (all-namespaces) bucket regardless of the
// child's namespace.
func TestFindParentByOwnerRefClusterScoped(t *testing.T) {
	plugin.Reset()
	t.Cleanup(plugin.Reset)

	nodeGVR := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "nodes"}
	nodePlugin := &stubPlugin{name: "nodes", gvr: nodeGVR, clusterScoped: true}
	plugin.Register(nodePlugin)

	disc := discoveryFor(k8s.APIResource{
		Name: "nodes", APIVersion: "v1", Group: "", Version: "v1",
		Kind: "Node", Namespaced: false, GVR: nodeGVR,
	})

	node := namedObj("node-1", "", "node-uid-1", "v1", "Node")
	store := k8s.NewStore(nil, "", nil)
	store.CacheUpsert(nodeGVR, "", node)
	cl := &testCluster{store: store, discovery: disc}

	child := objWithOwnerRefs("pod-x", "kube-system",
		ownerRef("v1", "Node", "node-1", "node-uid-1", true),
	)

	gotPlugin, gotObj := FindParentByOwnerRef(cl, child)
	if gotPlugin != nodePlugin {
		t.Fatalf("expected node plugin, got %v", gotPlugin)
	}
	if gotObj == nil || gotObj.GetName() != "node-1" {
		t.Fatalf("expected node-1 from the cluster-scoped bucket, got %v", gotObj)
	}
}

// TestFindParentByOwnerRefClusterScopedLazy guards the cluster-scoped lazy path:
// the owner resolves to a cluster-scoped plugin but the object is not yet in the
// store (informer not synced). FindParentByOwnerRef must return (plugin, nil) —
// non-nil plugin so the caller can push an empty view, nil object — and the
// store lookup must use the forced-"" namespace, not the child's namespace.
func TestFindParentByOwnerRefClusterScopedLazy(t *testing.T) {
	plugin.Reset()
	t.Cleanup(plugin.Reset)

	nodeGVR := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "nodes"}
	nodePlugin := &stubPlugin{name: "nodes", gvr: nodeGVR, clusterScoped: true}
	plugin.Register(nodePlugin)

	disc := discoveryFor(k8s.APIResource{
		Name: "nodes", APIVersion: "v1", Group: "", Version: "v1",
		Kind: "Node", Namespaced: false, GVR: nodeGVR,
	})

	// Node owner exists in discovery + registry, but NOTHING is in the store.
	store := k8s.NewStore(nil, "", nil)
	cl := &testCluster{store: store, discovery: disc}

	child := objWithOwnerRefs("pod-x", "kube-system",
		ownerRef("v1", "Node", "node-1", "node-uid-1", true),
	)

	gotPlugin, gotObj := FindParentByOwnerRef(cl, child)
	if gotPlugin != nodePlugin {
		t.Fatalf("expected node plugin (lazy), got %v", gotPlugin)
	}
	if gotObj != nil {
		t.Fatalf("expected nil object for unsynced cluster-scoped owner, got %q", gotObj.GetName())
	}
}

// TestFindParentByOwnerRefNilDiscovery guards the `if disc == nil { return }`
// line: a cluster whose Discovery() is nil must yield (nil, nil) without panic,
// even when the child carries valid ownerReferences.
func TestFindParentByOwnerRefNilDiscovery(t *testing.T) {
	cl := &testCluster{store: k8s.NewStore(nil, "", nil), discovery: nil}
	child := objWithOwnerRefs("pod-x", "default",
		ownerRef("apps/v1", "Deployment", "web", "deploy-uid-1", true),
	)
	if p, o := FindParentByOwnerRef(cl, child); p != nil || o != nil {
		t.Fatalf("expected (nil, nil) for nil discovery, got (%v, %v)", p, o)
	}
}

// TestFindParentByOwnerRefNilObj guards the nil-input path.
func TestFindParentByOwnerRefNilObj(t *testing.T) {
	cl := &testCluster{store: k8s.NewStore(nil, "", nil), discovery: k8s.NewDiscovery()}
	if p, o := FindParentByOwnerRef(cl, nil); p != nil || o != nil {
		t.Fatalf("expected (nil, nil) for nil object, got (%v, %v)", p, o)
	}
}
