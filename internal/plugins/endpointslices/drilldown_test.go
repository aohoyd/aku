package endpointslices

import (
	"fmt"
	"testing"

	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/plugin/plugintest"
	"github.com/aohoyd/aku/internal/plugins/pods"
	"github.com/aohoyd/aku/internal/plugins/services"
	"github.com/aohoyd/aku/internal/plugins/workload"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// compile-time assertions that the endpointslices plugin satisfies both
// DrillDowner (endpointslice → pods) and DrillUp (endpointslice → svc).
var _ plugin.DrillDowner = (*Plugin)(nil)
var _ plugin.DrillUp = (*Plugin)(nil)

// sliceWithPodTargets builds an EndpointSlice with a flat endpoints[] (no
// subsets), each endpoint carrying a Pod targetRef. svcName, when non-empty, is
// stamped as the kubernetes.io/service-name label.
func sliceWithPodTargets(name, namespace, svcName string, podNames ...string) *unstructured.Unstructured {
	var eps []any
	for i, pn := range podNames {
		eps = append(eps, map[string]any{
			"addresses": []any{fmt.Sprintf("10.0.0.%d", i+1)},
			"targetRef": map[string]any{
				"kind":      "Pod",
				"name":      pn,
				"namespace": namespace,
			},
		})
	}
	meta := map[string]any{"name": name, "namespace": namespace}
	if svcName != "" {
		meta["labels"] = map[string]any{"kubernetes.io/service-name": svcName}
	}
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "discovery.k8s.io/v1",
		"kind":       "EndpointSlice",
		"metadata":   meta,
		"endpoints":  eps,
	}}
}

// sliceWithUIDTargets builds an EndpointSlice whose endpoints each carry a
// targetRef WITH a "uid" field (in addition to kind/name/namespace), so the
// targetRef.uid dedup branch in FindPodsByEndpointSlice is exercised. Each
// endpoint is (podName, uid); passing the same (podName, uid) twice exercises
// the uid-keyed dedup collapse.
func sliceWithUIDTargets(name, namespace string, targets ...[2]string) *unstructured.Unstructured {
	var eps []any
	for i, t := range targets {
		eps = append(eps, map[string]any{
			"addresses": []any{fmt.Sprintf("10.0.0.%d", i+1)},
			"targetRef": map[string]any{
				"kind":      "Pod",
				"name":      t[0],
				"namespace": namespace,
				"uid":       t[1],
			},
		})
	}
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "discovery.k8s.io/v1",
		"kind":       "EndpointSlice",
		"metadata":   map[string]any{"name": name, "namespace": namespace},
		"endpoints":  eps,
	}}
}

func podObj(name, namespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata":   map[string]any{"name": name, "namespace": namespace},
	}}
}

func svcObj(name, namespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Service",
		"metadata":   map[string]any{"name": name, "namespace": namespace},
	}}
}

func TestEndpointSlicesGVRMatchesWorkload(t *testing.T) {
	p := &Plugin{}
	if p.GVR() != workload.EndpointSlicesGVR {
		t.Fatalf("expected GVR %v, got %v", workload.EndpointSlicesGVR, p.GVR())
	}
	// Guard the byte-for-byte invariant against the old literal.
	if got := p.GVR(); got.Group != "discovery.k8s.io" || got.Version != "v1" || got.Resource != "endpointslices" {
		t.Fatalf("GVR drifted from discovery.k8s.io/v1 endpointslices: %+v", got)
	}
}

func TestEndpointSlicesDrillDown(t *testing.T) {
	t.Run("happy path returns pods plugin and backing pods", func(t *testing.T) {
		plugin.Reset()
		t.Cleanup(plugin.Reset)
		podsPlugin := pods.New()
		plugin.Register(podsPlugin)

		store := k8s.NewStore(nil, "", nil)
		store.CacheUpsert(workload.PodsGVR, "default", podObj("web-abc", "default"))
		cl := plugintest.NewFakeCluster(store)

		p := &Plugin{}
		gotPlugin, gotObjs := p.DrillDown(cl, sliceWithPodTargets("web-xyz", "default", "web", "web-abc"))
		if gotPlugin != podsPlugin {
			t.Fatalf("expected pods plugin, got %v", gotPlugin)
		}
		if len(gotObjs) != 1 || gotObjs[0].GetName() != "web-abc" {
			t.Fatalf("expected [web-abc], got %v", gotObjs)
		}
	})

	t.Run("targetRef.uid dedup returns backing pod exactly once", func(t *testing.T) {
		plugin.Reset()
		t.Cleanup(plugin.Reset)
		podsPlugin := pods.New()
		plugin.Register(podsPlugin)

		store := k8s.NewStore(nil, "", nil)
		store.CacheUpsert(workload.PodsGVR, "default", podObj("web-abc", "default"))
		cl := plugintest.NewFakeCluster(store)

		// Two endpoints reference the SAME pod via the SAME targetRef.uid; the
		// uid-keyed dedup must collapse them to a single backing pod.
		slice := sliceWithUIDTargets("web-xyz", "default",
			[2]string{"web-abc", "uid-1"},
			[2]string{"web-abc", "uid-1"},
		)

		p := &Plugin{}
		gotPlugin, gotObjs := p.DrillDown(cl, slice)
		if gotPlugin != podsPlugin {
			t.Fatalf("expected pods plugin, got %v", gotPlugin)
		}
		if len(gotObjs) != 1 || gotObjs[0].GetName() != "web-abc" {
			t.Fatalf("expected [web-abc] deduped via targetRef.uid, got %v", gotObjs)
		}
	})

	t.Run("nil store returns pods plugin with nil slice (lazy view)", func(t *testing.T) {
		plugin.Reset()
		t.Cleanup(plugin.Reset)
		podsPlugin := pods.New()
		plugin.Register(podsPlugin)

		cl := plugintest.NewFakeCluster(nil)

		p := &Plugin{}
		gotPlugin, gotObjs := p.DrillDown(cl, sliceWithPodTargets("web-xyz", "default", "web", "web-abc"))
		if gotPlugin != podsPlugin {
			t.Fatalf("expected pods plugin (lazy), got %v", gotPlugin)
		}
		if gotObjs != nil {
			t.Fatalf("expected nil slice for nil store, got %v", gotObjs)
		}
	})

	t.Run("no pods plugin registered returns (nil, nil)", func(t *testing.T) {
		plugin.Reset()
		t.Cleanup(plugin.Reset)
		// No pods plugin registered.

		store := k8s.NewStore(nil, "", nil)
		store.CacheUpsert(workload.PodsGVR, "default", podObj("web-abc", "default"))
		cl := plugintest.NewFakeCluster(store)

		p := &Plugin{}
		gotPlugin, gotObjs := p.DrillDown(cl, sliceWithPodTargets("web-xyz", "default", "web", "web-abc"))
		if gotPlugin != nil || gotObjs != nil {
			t.Fatalf("expected (nil, nil) with no pods plugin, got (%v, %v)", gotPlugin, gotObjs)
		}
	})

	t.Run("nil obj does not panic and returns (nil, nil)", func(t *testing.T) {
		plugin.Reset()
		t.Cleanup(plugin.Reset)
		plugin.Register(pods.New())

		store := k8s.NewStore(nil, "", nil)
		cl := plugintest.NewFakeCluster(store)

		p := &Plugin{}
		gotPlugin, gotObjs := p.DrillDown(cl, nil)
		if gotPlugin != nil || gotObjs != nil {
			t.Fatalf("expected (nil, nil) for nil obj, got (%v, %v)", gotPlugin, gotObjs)
		}
	})
}

func TestEndpointSlicesDrillUp(t *testing.T) {
	t.Run("happy path returns services plugin and owning service", func(t *testing.T) {
		plugin.Reset()
		t.Cleanup(plugin.Reset)
		svcPlugin := services.New()
		plugin.Register(svcPlugin)

		store := k8s.NewStore(nil, "", nil)
		store.CacheUpsert(workload.ServicesGVR, "default", svcObj("web", "default"))
		cl := plugintest.NewFakeCluster(store)

		p := &Plugin{}
		gotPlugin, gotObj := p.DrillUp(cl, sliceWithPodTargets("web-xyz", "default", "web"))
		if gotPlugin != svcPlugin {
			t.Fatalf("expected services plugin, got %v", gotPlugin)
		}
		if gotObj == nil || gotObj.GetName() != "web" {
			t.Fatalf("expected service web, got %v", gotObj)
		}
	})

	t.Run("nil store returns services plugin with nil object (lazy view)", func(t *testing.T) {
		plugin.Reset()
		t.Cleanup(plugin.Reset)
		svcPlugin := services.New()
		plugin.Register(svcPlugin)

		cl := plugintest.NewFakeCluster(nil)

		p := &Plugin{}
		gotPlugin, gotObj := p.DrillUp(cl, sliceWithPodTargets("web-xyz", "default", "web"))
		if gotPlugin != svcPlugin {
			t.Fatalf("expected services plugin (lazy), got %v", gotPlugin)
		}
		if gotObj != nil {
			t.Fatalf("expected nil object for nil store, got %v", gotObj)
		}
	})

	t.Run("orphaned slice (no matching service) returns services plugin with nil object", func(t *testing.T) {
		plugin.Reset()
		t.Cleanup(plugin.Reset)
		svcPlugin := services.New()
		plugin.Register(svcPlugin)

		// Store has the services plugin registered but NO matching Service object.
		// The slice still carries a kubernetes.io/service-name label.
		store := k8s.NewStore(nil, "", nil)
		cl := plugintest.NewFakeCluster(store)

		p := &Plugin{}
		gotPlugin, gotObj := p.DrillUp(cl, sliceWithPodTargets("orphan-xyz", "default", "orphan"))
		if gotPlugin != svcPlugin {
			t.Fatalf("expected services plugin (orphaned), got %v", gotPlugin)
		}
		if gotObj != nil {
			t.Fatalf("expected nil object for orphaned slice, got %v", gotObj)
		}
	})

	t.Run("no services plugin registered returns (nil, nil)", func(t *testing.T) {
		plugin.Reset()
		t.Cleanup(plugin.Reset)
		// No services plugin registered.

		store := k8s.NewStore(nil, "", nil)
		store.CacheUpsert(workload.ServicesGVR, "default", svcObj("web", "default"))
		cl := plugintest.NewFakeCluster(store)

		p := &Plugin{}
		gotPlugin, gotObj := p.DrillUp(cl, sliceWithPodTargets("web-xyz", "default", "web"))
		if gotPlugin != nil || gotObj != nil {
			t.Fatalf("expected (nil, nil) with no services plugin, got (%v, %v)", gotPlugin, gotObj)
		}
	})

	t.Run("nil obj does not panic and returns (nil, nil)", func(t *testing.T) {
		plugin.Reset()
		t.Cleanup(plugin.Reset)
		plugin.Register(services.New())

		store := k8s.NewStore(nil, "", nil)
		cl := plugintest.NewFakeCluster(store)

		p := &Plugin{}
		gotPlugin, gotObj := p.DrillUp(cl, nil)
		if gotPlugin != nil || gotObj != nil {
			t.Fatalf("expected (nil, nil) for nil obj, got (%v, %v)", gotPlugin, gotObj)
		}
	})
}
