package services

import (
	"testing"

	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/plugin/plugintest"
	"github.com/aohoyd/aku/internal/plugins/endpointslices"
	"github.com/aohoyd/aku/internal/plugins/workload"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// compile-time assertion that the services plugin satisfies DrillDowner.
var _ plugin.DrillDowner = (*Plugin)(nil)

func svcObj(name, namespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Service",
		"metadata":   map[string]any{"name": name, "namespace": namespace},
	}}
}

// sliceObj builds an EndpointSlice linked to svcName via the
// kubernetes.io/service-name label (NOT a name match).
func sliceObj(name, namespace, svcName string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "discovery.k8s.io/v1",
		"kind":       "EndpointSlice",
		"metadata": map[string]any{
			"name":      name,
			"namespace": namespace,
			"labels":    map[string]any{"kubernetes.io/service-name": svcName},
		},
	}}
}

func TestServiceGVRMatchesWorkload(t *testing.T) {
	p := &Plugin{}
	if p.GVR() != workload.ServicesGVR {
		t.Fatalf("expected GVR %v, got %v", workload.ServicesGVR, p.GVR())
	}
	// Guard the byte-for-byte invariant against the old literal.
	if got := p.GVR(); got.Group != "" || got.Version != "v1" || got.Resource != "services" {
		t.Fatalf("GVR drifted from core/v1 services: %+v", got)
	}
}

func TestServiceDrillDown(t *testing.T) {
	t.Run("happy path returns endpointslices plugin and matching slice", func(t *testing.T) {
		plugin.Reset()
		t.Cleanup(plugin.Reset)
		esPlugin := endpointslices.New()
		plugin.Register(esPlugin)

		store := k8s.NewStore(nil, "", nil)
		store.CacheUpsert(workload.EndpointSlicesGVR, "default", sliceObj("web-abc", "default", "web"))
		cl := plugintest.NewFakeCluster(store)

		p := &Plugin{}
		gotPlugin, gotObjs := p.DrillDown(cl, svcObj("web", "default"))
		if gotPlugin != esPlugin {
			t.Fatalf("expected endpointslices plugin, got %v", gotPlugin)
		}
		if len(gotObjs) != 1 || gotObjs[0].GetName() != "web-abc" {
			t.Fatalf("expected [web-abc], got %v", gotObjs)
		}
	})

	t.Run("multi-slice service returns all matching slices", func(t *testing.T) {
		plugin.Reset()
		t.Cleanup(plugin.Reset)
		esPlugin := endpointslices.New()
		plugin.Register(esPlugin)

		// Two sharded slices both carry the service-name label "web".
		store := k8s.NewStore(nil, "", nil)
		store.CacheUpsert(workload.EndpointSlicesGVR, "default", sliceObj("web-aaa", "default", "web"))
		store.CacheUpsert(workload.EndpointSlicesGVR, "default", sliceObj("web-bbb", "default", "web"))
		cl := plugintest.NewFakeCluster(store)

		p := &Plugin{}
		gotPlugin, gotObjs := p.DrillDown(cl, svcObj("web", "default"))
		if gotPlugin != esPlugin {
			t.Fatalf("expected endpointslices plugin, got %v", gotPlugin)
		}
		if len(gotObjs) != 2 {
			t.Fatalf("expected 2 sharded slices (no cap-at-1), got %v", gotObjs)
		}
		gotNames := map[string]bool{}
		for _, o := range gotObjs {
			gotNames[o.GetName()] = true
		}
		if !gotNames["web-aaa"] || !gotNames["web-bbb"] {
			t.Fatalf("expected {web-aaa, web-bbb}, got %v", gotNames)
		}
	})

	t.Run("no endpointslices plugin registered returns (nil, nil)", func(t *testing.T) {
		plugin.Reset()
		t.Cleanup(plugin.Reset)
		// No endpointslices plugin registered.

		store := k8s.NewStore(nil, "", nil)
		store.CacheUpsert(workload.EndpointSlicesGVR, "default", sliceObj("web-abc", "default", "web"))
		cl := plugintest.NewFakeCluster(store)

		p := &Plugin{}
		gotPlugin, gotObjs := p.DrillDown(cl, svcObj("web", "default"))
		if gotPlugin != nil || gotObjs != nil {
			t.Fatalf("expected (nil, nil) with no endpointslices plugin, got (%v, %v)", gotPlugin, gotObjs)
		}
	})

	t.Run("namespace isolation: matching slice in different namespace not returned", func(t *testing.T) {
		plugin.Reset()
		t.Cleanup(plugin.Reset)
		esPlugin := endpointslices.New()
		plugin.Register(esPlugin)

		// A slice for service "web" lives in a DIFFERENT namespace.
		store := k8s.NewStore(nil, "", nil)
		store.CacheUpsert(workload.EndpointSlicesGVR, "other", sliceObj("web-abc", "other", "web"))
		cl := plugintest.NewFakeCluster(store)

		p := &Plugin{}
		gotPlugin, gotObjs := p.DrillDown(cl, svcObj("web", "default"))
		if gotPlugin != esPlugin {
			t.Fatalf("expected endpointslices plugin, got %v", gotPlugin)
		}
		if gotObjs != nil {
			t.Fatalf("expected nil slice (namespace isolation), got %v", gotObjs)
		}
	})

	t.Run("no matching slice (label mismatch) returns plugin with nil slice", func(t *testing.T) {
		plugin.Reset()
		t.Cleanup(plugin.Reset)
		esPlugin := endpointslices.New()
		plugin.Register(esPlugin)

		// A slice exists but its service-name label points elsewhere.
		store := k8s.NewStore(nil, "", nil)
		store.CacheUpsert(workload.EndpointSlicesGVR, "default", sliceObj("api-ccc", "default", "api"))
		cl := plugintest.NewFakeCluster(store)

		p := &Plugin{}
		gotPlugin, gotObjs := p.DrillDown(cl, svcObj("web", "default"))
		if gotPlugin != esPlugin {
			t.Fatalf("expected endpointslices plugin, got %v", gotPlugin)
		}
		if gotObjs != nil {
			t.Fatalf("expected nil slice for label mismatch, got %v", gotObjs)
		}
	})

	t.Run("nil store returns endpointslices plugin with nil slice (lazy view)", func(t *testing.T) {
		plugin.Reset()
		t.Cleanup(plugin.Reset)
		esPlugin := endpointslices.New()
		plugin.Register(esPlugin)

		cl := plugintest.NewFakeCluster(nil)

		p := &Plugin{}
		gotPlugin, gotObjs := p.DrillDown(cl, svcObj("web", "default"))
		if gotPlugin != esPlugin {
			t.Fatalf("expected endpointslices plugin (lazy), got %v", gotPlugin)
		}
		if gotObjs != nil {
			t.Fatalf("expected nil slice for nil store, got %v", gotObjs)
		}
	})

	t.Run("nil obj does not panic and returns (nil, nil)", func(t *testing.T) {
		plugin.Reset()
		t.Cleanup(plugin.Reset)
		plugin.Register(endpointslices.New())

		store := k8s.NewStore(nil, "", nil)
		cl := plugintest.NewFakeCluster(store)

		p := &Plugin{}
		gotPlugin, gotObjs := p.DrillDown(cl, nil)
		if gotPlugin != nil || gotObjs != nil {
			t.Fatalf("expected (nil, nil) for nil obj, got (%v, %v)", gotPlugin, gotObjs)
		}
	})
}
