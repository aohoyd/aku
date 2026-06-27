package workload

import (
	"testing"

	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/plugin"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// sliceEndpoint builds a flat EndpointSlice endpoints[] entry carrying the given
// targetRef (nil targetRef → an endpoint with no targetRef field) and optional
// conditions.ready. ready==nil omits conditions entirely.
func sliceEndpoint(ref map[string]any, ready *bool) map[string]any {
	ep := map[string]any{}
	if ref != nil {
		ep["targetRef"] = ref
	}
	if ready != nil {
		ep["conditions"] = map[string]any{"ready": *ready}
	}
	return ep
}

// endpointSliceObj builds a discovery.k8s.io/v1 EndpointSlice object with the
// given kubernetes.io/service-name label (empty → no label) and flat endpoints.
func endpointSliceObj(name, namespace, serviceName string, endpoints ...map[string]any) *unstructured.Unstructured {
	meta := map[string]any{"name": name, "namespace": namespace}
	if serviceName != "" {
		meta["labels"] = map[string]any{ServiceNameLabel: serviceName}
	}
	obj := map[string]any{
		"apiVersion": "discovery.k8s.io/v1",
		"kind":       "EndpointSlice",
		"metadata":   meta,
	}
	if len(endpoints) > 0 {
		anyEps := make([]any, len(endpoints))
		for i, e := range endpoints {
			anyEps[i] = e
		}
		obj["endpoints"] = anyEps
	}
	return &unstructured.Unstructured{Object: obj}
}

func ptrBool(b bool) *bool { return &b }

func TestFindEndpointSlicesForService(t *testing.T) {
	t.Run("label match returns plugin and single slice", func(t *testing.T) {
		plugin.Reset()
		t.Cleanup(plugin.Reset)
		esPlugin := &stubPlugin{name: "endpointslices", gvr: EndpointSlicesGVR}
		plugin.Register(esPlugin)

		slice := endpointSliceObj("web-abc", "default", "web")
		store := k8s.NewStore(nil, "", nil)
		store.CacheUpsert(EndpointSlicesGVR, "default", slice)
		cl := &testCluster{store: store, discovery: k8s.NewDiscovery()}

		svc := namedObj("web", "default", "svc-uid-1", "v1", "Service")
		gotPlugin, gotObjs := FindEndpointSlicesForService(cl, svc)
		if gotPlugin != esPlugin {
			t.Fatalf("expected endpointslices plugin, got %v", gotPlugin)
		}
		if len(gotObjs) != 1 || gotObjs[0].GetName() != "web-abc" {
			t.Fatalf("expected [web-abc], got %v", gotObjs)
		}
	})

	t.Run("multi-slice service returns all matching slices (N results)", func(t *testing.T) {
		plugin.Reset()
		t.Cleanup(plugin.Reset)
		esPlugin := &stubPlugin{name: "endpointslices", gvr: EndpointSlicesGVR}
		plugin.Register(esPlugin)

		store := k8s.NewStore(nil, "", nil)
		store.CacheUpsert(EndpointSlicesGVR, "default", endpointSliceObj("web-aaa", "default", "web"))
		store.CacheUpsert(EndpointSlicesGVR, "default", endpointSliceObj("web-bbb", "default", "web"))
		// A slice for a DIFFERENT service must not be returned.
		store.CacheUpsert(EndpointSlicesGVR, "default", endpointSliceObj("api-ccc", "default", "api"))
		cl := &testCluster{store: store, discovery: k8s.NewDiscovery()}

		svc := namedObj("web", "default", "svc-uid-1", "v1", "Service")
		gotPlugin, gotObjs := FindEndpointSlicesForService(cl, svc)
		if gotPlugin != esPlugin {
			t.Fatalf("expected endpointslices plugin, got %v", gotPlugin)
		}
		if len(gotObjs) != 2 {
			t.Fatalf("expected 2 slices (sharded), got %v", gotObjs)
		}
		gotNames := map[string]bool{}
		for _, o := range gotObjs {
			gotNames[o.GetName()] = true
		}
		if !gotNames["web-aaa"] || !gotNames["web-bbb"] || len(gotNames) != 2 {
			t.Fatalf("expected exactly {web-aaa, web-bbb}, got %v", gotNames)
		}
	})

	t.Run("no matching label returns plugin with nil slice", func(t *testing.T) {
		plugin.Reset()
		t.Cleanup(plugin.Reset)
		esPlugin := &stubPlugin{name: "endpointslices", gvr: EndpointSlicesGVR}
		plugin.Register(esPlugin)

		// A slice exists but its service-name label points elsewhere.
		store := k8s.NewStore(nil, "", nil)
		store.CacheUpsert(EndpointSlicesGVR, "default", endpointSliceObj("api-ccc", "default", "api"))
		cl := &testCluster{store: store, discovery: k8s.NewDiscovery()}

		svc := namedObj("web", "default", "svc-uid-1", "v1", "Service")
		gotPlugin, gotObjs := FindEndpointSlicesForService(cl, svc)
		if gotPlugin != esPlugin {
			t.Fatalf("expected endpointslices plugin (lazy), got %v", gotPlugin)
		}
		if gotObjs != nil {
			t.Fatalf("expected nil slice for no label match, got %v", gotObjs)
		}
	})

	t.Run("nil svc returns nil", func(t *testing.T) {
		plugin.Reset()
		t.Cleanup(plugin.Reset)
		// Register NO endpointslices plugin: a (nil, nil) result can then only
		// come from the early nil-svc guard, never the plugin-lookup path.

		cl := &testCluster{store: k8s.NewStore(nil, "", nil), discovery: k8s.NewDiscovery()}
		if p, o := FindEndpointSlicesForService(cl, nil); p != nil || o != nil {
			t.Fatalf("expected (nil, nil) for nil svc, got (%v, %v)", p, o)
		}
	})

	t.Run("nil store returns plugin with nil slice (lazy view)", func(t *testing.T) {
		plugin.Reset()
		t.Cleanup(plugin.Reset)
		esPlugin := &stubPlugin{name: "endpointslices", gvr: EndpointSlicesGVR}
		plugin.Register(esPlugin)

		cl := &testCluster{store: nil, discovery: k8s.NewDiscovery()}

		svc := namedObj("web", "default", "svc-uid-1", "v1", "Service")
		gotPlugin, gotObjs := FindEndpointSlicesForService(cl, svc)
		if gotPlugin != esPlugin {
			t.Fatalf("expected endpointslices plugin (lazy), got %v", gotPlugin)
		}
		if gotObjs != nil {
			t.Fatalf("expected nil slice for nil store, got %v", gotObjs)
		}
	})

	t.Run("no endpointslices plugin registered returns nil", func(t *testing.T) {
		plugin.Reset()
		t.Cleanup(plugin.Reset)

		store := k8s.NewStore(nil, "", nil)
		store.CacheUpsert(EndpointSlicesGVR, "default", endpointSliceObj("web-abc", "default", "web"))
		cl := &testCluster{store: store, discovery: k8s.NewDiscovery()}

		svc := namedObj("web", "default", "svc-uid-1", "v1", "Service")
		if p, o := FindEndpointSlicesForService(cl, svc); p != nil || o != nil {
			t.Fatalf("expected (nil, nil) with no endpointslices plugin, got (%v, %v)", p, o)
		}
	})

	t.Run("namespace isolation: matching label in different namespace not matched", func(t *testing.T) {
		plugin.Reset()
		t.Cleanup(plugin.Reset)
		esPlugin := &stubPlugin{name: "endpointslices", gvr: EndpointSlicesGVR}
		plugin.Register(esPlugin)

		store := k8s.NewStore(nil, "", nil)
		store.CacheUpsert(EndpointSlicesGVR, "other", endpointSliceObj("web-abc", "other", "web"))
		cl := &testCluster{store: store, discovery: k8s.NewDiscovery()}

		svc := namedObj("web", "default", "svc-uid-1", "v1", "Service")
		gotPlugin, gotObjs := FindEndpointSlicesForService(cl, svc)
		if gotPlugin != esPlugin {
			t.Fatalf("expected endpointslices plugin, got %v", gotPlugin)
		}
		if gotObjs != nil {
			t.Fatalf("expected nil slice (namespace isolation), got %v", gotObjs)
		}
	})
}

func TestFindServiceForEndpointSlice(t *testing.T) {
	t.Run("label match returns plugin and service", func(t *testing.T) {
		plugin.Reset()
		t.Cleanup(plugin.Reset)
		svcPlugin := &stubPlugin{name: "services", gvr: ServicesGVR}
		plugin.Register(svcPlugin)

		svc := namedObj("web", "default", "svc-uid-1", "v1", "Service")
		store := k8s.NewStore(nil, "", nil)
		store.CacheUpsert(ServicesGVR, "default", svc)
		cl := &testCluster{store: store, discovery: k8s.NewDiscovery()}

		slice := endpointSliceObj("web-abc", "default", "web")
		gotPlugin, gotObj := FindServiceForEndpointSlice(cl, slice)
		if gotPlugin != svcPlugin {
			t.Fatalf("expected services plugin, got %v", gotPlugin)
		}
		if gotObj == nil || gotObj.GetName() != "web" {
			t.Fatalf("expected web, got %v", gotObj)
		}
	})

	t.Run("missing service-name label returns plugin with nil object", func(t *testing.T) {
		plugin.Reset()
		t.Cleanup(plugin.Reset)
		svcPlugin := &stubPlugin{name: "services", gvr: ServicesGVR}
		plugin.Register(svcPlugin)

		// Even if the matching service exists, an unlabeled slice resolves to nil.
		store := k8s.NewStore(nil, "", nil)
		store.CacheUpsert(ServicesGVR, "default", namedObj("web", "default", "svc-uid-1", "v1", "Service"))
		cl := &testCluster{store: store, discovery: k8s.NewDiscovery()}

		slice := endpointSliceObj("web-abc", "default", "") // no service-name label
		gotPlugin, gotObj := FindServiceForEndpointSlice(cl, slice)
		if gotPlugin != svcPlugin {
			t.Fatalf("expected services plugin (lazy), got %v", gotPlugin)
		}
		if gotObj != nil {
			t.Fatalf("expected nil object for unlabeled slice, got %q", gotObj.GetName())
		}
	})

	t.Run("orphan slice (label present but no matching svc) returns plugin with nil object", func(t *testing.T) {
		plugin.Reset()
		t.Cleanup(plugin.Reset)
		svcPlugin := &stubPlugin{name: "services", gvr: ServicesGVR}
		plugin.Register(svcPlugin)

		// Label points to "web" but no such Service is cached.
		store := k8s.NewStore(nil, "", nil)
		cl := &testCluster{store: store, discovery: k8s.NewDiscovery()}

		slice := endpointSliceObj("web-abc", "default", "web")
		gotPlugin, gotObj := FindServiceForEndpointSlice(cl, slice)
		if gotPlugin != svcPlugin {
			t.Fatalf("expected services plugin (lazy), got %v", gotPlugin)
		}
		if gotObj != nil {
			t.Fatalf("expected nil object for orphan slice, got %q", gotObj.GetName())
		}
	})

	t.Run("namespace isolation: matching service in different namespace not matched", func(t *testing.T) {
		plugin.Reset()
		t.Cleanup(plugin.Reset)
		svcPlugin := &stubPlugin{name: "services", gvr: ServicesGVR}
		plugin.Register(svcPlugin)

		store := k8s.NewStore(nil, "", nil)
		store.CacheUpsert(ServicesGVR, "other", namedObj("web", "other", "svc-uid-1", "v1", "Service"))
		cl := &testCluster{store: store, discovery: k8s.NewDiscovery()}

		slice := endpointSliceObj("web-abc", "default", "web")
		gotPlugin, gotObj := FindServiceForEndpointSlice(cl, slice)
		if gotPlugin != svcPlugin {
			t.Fatalf("expected services plugin, got %v", gotPlugin)
		}
		if gotObj != nil {
			t.Fatalf("expected nil object (namespace isolation), got %q", gotObj.GetName())
		}
	})

	t.Run("nil slice returns nil", func(t *testing.T) {
		plugin.Reset()
		t.Cleanup(plugin.Reset)
		plugin.Register(&stubPlugin{name: "services", gvr: ServicesGVR})

		cl := &testCluster{store: k8s.NewStore(nil, "", nil), discovery: k8s.NewDiscovery()}
		if p, o := FindServiceForEndpointSlice(cl, nil); p != nil || o != nil {
			t.Fatalf("expected (nil, nil) for nil slice, got (%v, %v)", p, o)
		}
	})

	t.Run("nil store returns plugin with nil object (lazy view)", func(t *testing.T) {
		plugin.Reset()
		t.Cleanup(plugin.Reset)
		svcPlugin := &stubPlugin{name: "services", gvr: ServicesGVR}
		plugin.Register(svcPlugin)

		cl := &testCluster{store: nil, discovery: k8s.NewDiscovery()}

		slice := endpointSliceObj("web-abc", "default", "web")
		gotPlugin, gotObj := FindServiceForEndpointSlice(cl, slice)
		if gotPlugin != svcPlugin {
			t.Fatalf("expected services plugin (lazy), got %v", gotPlugin)
		}
		if gotObj != nil {
			t.Fatalf("expected nil object for nil store, got %q", gotObj.GetName())
		}
	})

	t.Run("no services plugin registered returns nil", func(t *testing.T) {
		plugin.Reset()
		t.Cleanup(plugin.Reset)

		store := k8s.NewStore(nil, "", nil)
		store.CacheUpsert(ServicesGVR, "default", namedObj("web", "default", "svc-uid-1", "v1", "Service"))
		cl := &testCluster{store: store, discovery: k8s.NewDiscovery()}

		slice := endpointSliceObj("web-abc", "default", "web")
		if p, o := FindServiceForEndpointSlice(cl, slice); p != nil || o != nil {
			t.Fatalf("expected (nil, nil) with no services plugin, got (%v, %v)", p, o)
		}
	})
}

func TestFindPodsByEndpointSlice(t *testing.T) {
	t.Run("happy path returns backing pods", func(t *testing.T) {
		plugin.Reset()
		t.Cleanup(plugin.Reset)
		podsPlugin := &stubPlugin{name: "pods", gvr: PodsGVR}
		plugin.Register(podsPlugin)

		store := k8s.NewStore(nil, "", nil)
		store.CacheUpsert(PodsGVR, "default", namedObj("p1", "default", "pod-uid-1", "v1", "Pod"))
		store.CacheUpsert(PodsGVR, "default", namedObj("p2", "default", "pod-uid-2", "v1", "Pod"))
		cl := &testCluster{store: store, discovery: k8s.NewDiscovery()}

		slice := endpointSliceObj("web-abc", "default", "",
			sliceEndpoint(targetRef("Pod", "p1", "", "pod-uid-1"), ptrBool(true)),
			sliceEndpoint(targetRef("Pod", "p2", "", "pod-uid-2"), ptrBool(true)))
		gotPlugin, gotObjs := FindPodsByEndpointSlice(cl, slice)
		if gotPlugin != podsPlugin {
			t.Fatalf("expected pods plugin, got %v", gotPlugin)
		}
		if len(gotObjs) != 2 {
			t.Fatalf("expected 2 pods, got %v", gotObjs)
		}
		gotNames := map[string]bool{}
		for _, o := range gotObjs {
			gotNames[o.GetName()] = true
		}
		if !gotNames["p1"] || !gotNames["p2"] || len(gotNames) != 2 {
			t.Fatalf("expected exactly {p1, p2}, got %v", gotNames)
		}
	})

	t.Run("all conditions included: ready=false endpoint still resolves", func(t *testing.T) {
		plugin.Reset()
		t.Cleanup(plugin.Reset)
		podsPlugin := &stubPlugin{name: "pods", gvr: PodsGVR}
		plugin.Register(podsPlugin)

		store := k8s.NewStore(nil, "", nil)
		store.CacheUpsert(PodsGVR, "default", namedObj("p1", "default", "pod-uid-1", "v1", "Pod"))
		store.CacheUpsert(PodsGVR, "default", namedObj("p2", "default", "pod-uid-2", "v1", "Pod"))
		cl := &testCluster{store: store, discovery: k8s.NewDiscovery()}

		slice := endpointSliceObj("web-abc", "default", "",
			sliceEndpoint(targetRef("Pod", "p1", "", "pod-uid-1"), ptrBool(true)),
			sliceEndpoint(targetRef("Pod", "p2", "", "pod-uid-2"), ptrBool(false))) // not ready
		gotPlugin, gotObjs := FindPodsByEndpointSlice(cl, slice)
		if gotPlugin != podsPlugin {
			t.Fatalf("expected pods plugin, got %v", gotPlugin)
		}
		if len(gotObjs) != 2 {
			t.Fatalf("expected 2 pods (ready=false included), got %v", gotObjs)
		}
		gotNames := map[string]bool{}
		for _, o := range gotObjs {
			gotNames[o.GetName()] = true
		}
		if !gotNames["p1"] || !gotNames["p2"] {
			t.Fatalf("expected {p1, p2} incl. not-ready, got %v", gotNames)
		}
	})

	t.Run("dedup across endpoints: same pod once", func(t *testing.T) {
		plugin.Reset()
		t.Cleanup(plugin.Reset)
		podsPlugin := &stubPlugin{name: "pods", gvr: PodsGVR}
		plugin.Register(podsPlugin)

		store := k8s.NewStore(nil, "", nil)
		store.CacheUpsert(PodsGVR, "default", namedObj("p1", "default", "pod-uid-1", "v1", "Pod"))
		cl := &testCluster{store: store, discovery: k8s.NewDiscovery()}

		// Same pod appears in two endpoints[] entries.
		slice := endpointSliceObj("web-abc", "default", "",
			sliceEndpoint(targetRef("Pod", "p1", "", "pod-uid-1"), ptrBool(true)),
			sliceEndpoint(targetRef("Pod", "p1", "", "pod-uid-1"), ptrBool(true)))
		gotPlugin, gotObjs := FindPodsByEndpointSlice(cl, slice)
		if gotPlugin != podsPlugin {
			t.Fatalf("expected pods plugin, got %v", gotPlugin)
		}
		if len(gotObjs) != 1 || gotObjs[0].GetName() != "p1" {
			t.Fatalf("expected dedup to [p1], got %v", gotObjs)
		}
	})

	t.Run("blank uid everywhere not collapsed (ns/name fallback)", func(t *testing.T) {
		plugin.Reset()
		t.Cleanup(plugin.Reset)
		podsPlugin := &stubPlugin{name: "pods", gvr: PodsGVR}
		plugin.Register(podsPlugin)

		// Two DISTINCT pods, both with blank metadata.uid.
		store := k8s.NewStore(nil, "", nil)
		store.CacheUpsert(PodsGVR, "default", namedObj("p1", "default", "", "v1", "Pod"))
		store.CacheUpsert(PodsGVR, "default", namedObj("p2", "default", "", "v1", "Pod"))
		cl := &testCluster{store: store, discovery: k8s.NewDiscovery()}

		// targetRefs ALSO carry no uid — the dedup key must fall back to ns/name,
		// not collapse onto a single "" key.
		slice := endpointSliceObj("web-abc", "default", "",
			sliceEndpoint(targetRef("Pod", "p1", "", ""), nil),
			sliceEndpoint(targetRef("Pod", "p2", "", ""), nil))
		gotPlugin, gotObjs := FindPodsByEndpointSlice(cl, slice)
		if gotPlugin != podsPlugin {
			t.Fatalf("expected pods plugin, got %v", gotPlugin)
		}
		if len(gotObjs) != 2 {
			t.Fatalf("expected 2 pods (ns/name fallback, not collapsed), got %v", gotObjs)
		}
	})

	t.Run("present-but-empty targetRef uid falls through to ns/name dedup", func(t *testing.T) {
		plugin.Reset()
		t.Cleanup(plugin.Reset)
		podsPlugin := &stubPlugin{name: "pods", gvr: PodsGVR}
		plugin.Register(podsPlugin)

		store := k8s.NewStore(nil, "", nil)
		store.CacheUpsert(PodsGVR, "default", namedObj("p1", "default", "", "v1", "Pod"))
		store.CacheUpsert(PodsGVR, "default", namedObj("p2", "default", "", "v1", "Pod"))
		cl := &testCluster{store: store, discovery: k8s.NewDiscovery()}

		// Unlike targetRef(), which omits an empty uid key entirely, these maps
		// carry an explicit "uid": "" so we exercise the present-but-empty
		// branch: it must still fall through to the ns/name composite, so two
		// distinct pods resolve to two, and the same pod twice dedups to one.
		ref := func(name string) map[string]any {
			return map[string]any{"kind": "Pod", "name": name, "uid": ""}
		}
		slice := endpointSliceObj("web-abc", "default", "",
			sliceEndpoint(ref("p1"), nil),
			sliceEndpoint(ref("p2"), nil),
			sliceEndpoint(ref("p1"), nil)) // duplicate of p1
		gotPlugin, gotObjs := FindPodsByEndpointSlice(cl, slice)
		if gotPlugin != podsPlugin {
			t.Fatalf("expected pods plugin, got %v", gotPlugin)
		}
		gotNames := map[string]int{}
		for _, o := range gotObjs {
			gotNames[o.GetName()]++
		}
		if len(gotObjs) != 2 || gotNames["p1"] != 1 || gotNames["p2"] != 1 {
			t.Fatalf("expected {p1:1, p2:1} (empty-string uid falls through), got %v", gotNames)
		}
	})

	t.Run("blank metadata.uid pods deduped by distinct targetRef.uid", func(t *testing.T) {
		plugin.Reset()
		t.Cleanup(plugin.Reset)
		podsPlugin := &stubPlugin{name: "pods", gvr: PodsGVR}
		plugin.Register(podsPlugin)

		store := k8s.NewStore(nil, "", nil)
		store.CacheUpsert(PodsGVR, "default", namedObj("p1", "default", "", "v1", "Pod"))
		store.CacheUpsert(PodsGVR, "default", namedObj("p2", "default", "", "v1", "Pod"))
		cl := &testCluster{store: store, discovery: k8s.NewDiscovery()}

		slice := endpointSliceObj("web-abc", "default", "",
			sliceEndpoint(targetRef("Pod", "p1", "", "ref-uid-1"), nil),
			sliceEndpoint(targetRef("Pod", "p2", "", "ref-uid-2"), nil))
		gotPlugin, gotObjs := FindPodsByEndpointSlice(cl, slice)
		if gotPlugin != podsPlugin {
			t.Fatalf("expected pods plugin, got %v", gotPlugin)
		}
		if len(gotObjs) != 2 {
			t.Fatalf("expected 2 pods (dedup by targetRef.uid), got %v", gotObjs)
		}
	})

	t.Run("mixed uid and ns/name-fallback in one slice: both pods returned once", func(t *testing.T) {
		plugin.Reset()
		t.Cleanup(plugin.Reset)
		podsPlugin := &stubPlugin{name: "pods", gvr: PodsGVR}
		plugin.Register(podsPlugin)

		// p1 resolves via the targetRef.uid path; p2 (blank uid) via ns/name fallback.
		store := k8s.NewStore(nil, "", nil)
		store.CacheUpsert(PodsGVR, "default", namedObj("p1", "default", "pod-uid-1", "v1", "Pod"))
		store.CacheUpsert(PodsGVR, "default", namedObj("p2", "default", "", "v1", "Pod"))
		cl := &testCluster{store: store, discovery: k8s.NewDiscovery()}

		slice := endpointSliceObj("web-abc", "default", "",
			sliceEndpoint(targetRef("Pod", "p1", "", "ref-uid-1"), nil), // uid-keyed
			sliceEndpoint(targetRef("Pod", "p2", "", ""), nil))          // ns/name fallback
		gotPlugin, gotObjs := FindPodsByEndpointSlice(cl, slice)
		if gotPlugin != podsPlugin {
			t.Fatalf("expected pods plugin, got %v", gotPlugin)
		}
		gotNames := map[string]int{}
		for _, o := range gotObjs {
			gotNames[o.GetName()]++
		}
		if len(gotObjs) != 2 || gotNames["p1"] != 1 || gotNames["p2"] != 1 {
			t.Fatalf("expected exactly {p1:1, p2:1} (uid + ns/name paths coexist), got %v", gotNames)
		}
	})

	t.Run("empty-name targetRef is skipped", func(t *testing.T) {
		plugin.Reset()
		t.Cleanup(plugin.Reset)
		podsPlugin := &stubPlugin{name: "pods", gvr: PodsGVR}
		plugin.Register(podsPlugin)

		store := k8s.NewStore(nil, "", nil)
		store.CacheUpsert(PodsGVR, "default", namedObj("p1", "default", "pod-uid-1", "v1", "Pod"))
		cl := &testCluster{store: store, discovery: k8s.NewDiscovery()}

		slice := endpointSliceObj("web-abc", "default", "",
			sliceEndpoint(targetRef("Pod", "", "", "pod-uid-x"), nil), // empty name
			sliceEndpoint(targetRef("Pod", "p1", "", "pod-uid-1"), nil))
		gotPlugin, gotObjs := FindPodsByEndpointSlice(cl, slice)
		if gotPlugin != podsPlugin {
			t.Fatalf("expected pods plugin, got %v", gotPlugin)
		}
		if len(gotObjs) != 1 || gotObjs[0].GetName() != "p1" {
			t.Fatalf("expected only [p1] (empty-name skipped), got %v", gotObjs)
		}
	})

	t.Run("endpoint without targetRef is skipped", func(t *testing.T) {
		plugin.Reset()
		t.Cleanup(plugin.Reset)
		podsPlugin := &stubPlugin{name: "pods", gvr: PodsGVR}
		plugin.Register(podsPlugin)

		store := k8s.NewStore(nil, "", nil)
		store.CacheUpsert(PodsGVR, "default", namedObj("p1", "default", "pod-uid-1", "v1", "Pod"))
		cl := &testCluster{store: store, discovery: k8s.NewDiscovery()}

		slice := endpointSliceObj("web-abc", "default", "",
			sliceEndpoint(nil, ptrBool(true)), // no targetRef
			sliceEndpoint(targetRef("Pod", "p1", "", "pod-uid-1"), nil))
		gotPlugin, gotObjs := FindPodsByEndpointSlice(cl, slice)
		if gotPlugin != podsPlugin {
			t.Fatalf("expected pods plugin, got %v", gotPlugin)
		}
		if len(gotObjs) != 1 || gotObjs[0].GetName() != "p1" {
			t.Fatalf("expected only [p1] (no-targetRef skipped), got %v", gotObjs)
		}
	})

	t.Run("targetRef kind not Pod is skipped", func(t *testing.T) {
		plugin.Reset()
		t.Cleanup(plugin.Reset)
		podsPlugin := &stubPlugin{name: "pods", gvr: PodsGVR}
		plugin.Register(podsPlugin)

		store := k8s.NewStore(nil, "", nil)
		store.CacheUpsert(PodsGVR, "default", namedObj("p1", "default", "pod-uid-1", "v1", "Pod"))
		cl := &testCluster{store: store, discovery: k8s.NewDiscovery()}

		slice := endpointSliceObj("web-abc", "default", "",
			sliceEndpoint(targetRef("Node", "p1", "", "node-uid-1"), nil)) // not a Pod
		gotPlugin, gotObjs := FindPodsByEndpointSlice(cl, slice)
		if gotPlugin != podsPlugin {
			t.Fatalf("expected pods plugin, got %v", gotPlugin)
		}
		if len(gotObjs) != 0 {
			t.Fatalf("expected empty (non-Pod kind skipped), got %v", gotObjs)
		}
	})

	t.Run("namespace fallback: targetRef without namespace resolves in slice ns", func(t *testing.T) {
		plugin.Reset()
		t.Cleanup(plugin.Reset)
		podsPlugin := &stubPlugin{name: "pods", gvr: PodsGVR}
		plugin.Register(podsPlugin)

		store := k8s.NewStore(nil, "", nil)
		store.CacheUpsert(PodsGVR, "default", namedObj("p1", "default", "pod-uid-1", "v1", "Pod"))
		cl := &testCluster{store: store, discovery: k8s.NewDiscovery()}

		// targetRef omits namespace; must resolve in the slice's namespace.
		slice := endpointSliceObj("web-abc", "default", "",
			sliceEndpoint(targetRef("Pod", "p1", "", "pod-uid-1"), nil))
		gotPlugin, gotObjs := FindPodsByEndpointSlice(cl, slice)
		if gotPlugin != podsPlugin {
			t.Fatalf("expected pods plugin, got %v", gotPlugin)
		}
		if len(gotObjs) != 1 || gotObjs[0].GetName() != "p1" {
			t.Fatalf("expected [p1] via ns fallback, got %v", gotObjs)
		}
	})

	t.Run("cross-namespace targetRef resolves from its own namespace", func(t *testing.T) {
		plugin.Reset()
		t.Cleanup(plugin.Reset)
		podsPlugin := &stubPlugin{name: "pods", gvr: PodsGVR}
		plugin.Register(podsPlugin)

		// Pod lives in "other", slice lives in "default".
		store := k8s.NewStore(nil, "", nil)
		store.CacheUpsert(PodsGVR, "other", namedObj("p1", "other", "pod-uid-1", "v1", "Pod"))
		cl := &testCluster{store: store, discovery: k8s.NewDiscovery()}

		slice := endpointSliceObj("web-abc", "default", "",
			sliceEndpoint(targetRef("Pod", "p1", "other", "pod-uid-1"), nil))
		gotPlugin, gotObjs := FindPodsByEndpointSlice(cl, slice)
		if gotPlugin != podsPlugin {
			t.Fatalf("expected pods plugin, got %v", gotPlugin)
		}
		if len(gotObjs) != 1 || gotObjs[0].GetName() != "p1" || gotObjs[0].GetNamespace() != "other" {
			t.Fatalf("expected [p1 in other], got %v", gotObjs)
		}
	})

	t.Run("pod absent from store is skipped (lazy)", func(t *testing.T) {
		plugin.Reset()
		t.Cleanup(plugin.Reset)
		podsPlugin := &stubPlugin{name: "pods", gvr: PodsGVR}
		plugin.Register(podsPlugin)

		store := k8s.NewStore(nil, "", nil)
		cl := &testCluster{store: store, discovery: k8s.NewDiscovery()}

		slice := endpointSliceObj("web-abc", "default", "",
			sliceEndpoint(targetRef("Pod", "p1", "", "pod-uid-1"), nil))
		gotPlugin, gotObjs := FindPodsByEndpointSlice(cl, slice)
		if gotPlugin != podsPlugin {
			t.Fatalf("expected pods plugin (lazy), got %v", gotPlugin)
		}
		if len(gotObjs) != 0 {
			t.Fatalf("expected empty (uncached pod skipped), got %v", gotObjs)
		}
	})

	t.Run("absent endpoints returns empty", func(t *testing.T) {
		plugin.Reset()
		t.Cleanup(plugin.Reset)
		podsPlugin := &stubPlugin{name: "pods", gvr: PodsGVR}
		plugin.Register(podsPlugin)

		cl := &testCluster{store: k8s.NewStore(nil, "", nil), discovery: k8s.NewDiscovery()}

		slice := endpointSliceObj("web-abc", "default", "") // no endpoints
		gotPlugin, gotObjs := FindPodsByEndpointSlice(cl, slice)
		if gotPlugin != podsPlugin {
			t.Fatalf("expected pods plugin, got %v", gotPlugin)
		}
		if gotObjs != nil {
			t.Fatalf("expected nil slice for absent endpoints, got %v", gotObjs)
		}
	})

	t.Run("nil slice returns nil", func(t *testing.T) {
		plugin.Reset()
		t.Cleanup(plugin.Reset)
		plugin.Register(&stubPlugin{name: "pods", gvr: PodsGVR})

		cl := &testCluster{store: k8s.NewStore(nil, "", nil), discovery: k8s.NewDiscovery()}
		if p, o := FindPodsByEndpointSlice(cl, nil); p != nil || o != nil {
			t.Fatalf("expected (nil, nil) for nil slice, got (%v, %v)", p, o)
		}
	})

	t.Run("nil store returns plugin with nil slice", func(t *testing.T) {
		plugin.Reset()
		t.Cleanup(plugin.Reset)
		podsPlugin := &stubPlugin{name: "pods", gvr: PodsGVR}
		plugin.Register(podsPlugin)

		cl := &testCluster{store: nil, discovery: k8s.NewDiscovery()}

		slice := endpointSliceObj("web-abc", "default", "",
			sliceEndpoint(targetRef("Pod", "p1", "", "pod-uid-1"), nil))
		gotPlugin, gotObjs := FindPodsByEndpointSlice(cl, slice)
		if gotPlugin != podsPlugin {
			t.Fatalf("expected pods plugin (lazy), got %v", gotPlugin)
		}
		if gotObjs != nil {
			t.Fatalf("expected nil slice for nil store, got %v", gotObjs)
		}
	})

	t.Run("no pods plugin registered returns nil", func(t *testing.T) {
		plugin.Reset()
		t.Cleanup(plugin.Reset)

		store := k8s.NewStore(nil, "", nil)
		store.CacheUpsert(PodsGVR, "default", namedObj("p1", "default", "pod-uid-1", "v1", "Pod"))
		cl := &testCluster{store: store, discovery: k8s.NewDiscovery()}

		slice := endpointSliceObj("web-abc", "default", "",
			sliceEndpoint(targetRef("Pod", "p1", "", "pod-uid-1"), nil))
		if p, o := FindPodsByEndpointSlice(cl, slice); p != nil || o != nil {
			t.Fatalf("expected (nil, nil) with no pods plugin, got (%v, %v)", p, o)
		}
	})
}
