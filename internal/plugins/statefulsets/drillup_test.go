package statefulsets

import (
	"context"
	"testing"

	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/plugin/plugintest"
	"github.com/aohoyd/aku/internal/render"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// drillUpStub is a minimal parent plugin registered for DrillUp resolution.
type drillUpStub struct {
	name string
	gvr  schema.GroupVersionResource
}

func (p *drillUpStub) Name() string                     { return p.name }
func (p *drillUpStub) ShortName() string                { return p.name }
func (p *drillUpStub) GVR() schema.GroupVersionResource { return p.gvr }
func (p *drillUpStub) IsClusterScoped() bool            { return false }
func (p *drillUpStub) Columns() []plugin.Column         { return nil }
func (p *drillUpStub) Row(*unstructured.Unstructured) []string {
	return nil
}
func (p *drillUpStub) YAML(*unstructured.Unstructured) (render.Content, error) {
	return render.Content{}, nil
}
func (p *drillUpStub) Describe(context.Context, *unstructured.Unstructured) (render.Content, error) {
	return render.Content{}, nil
}

func drillUpOwnerRef(apiVersion, kind, name, uid string, controller bool) map[string]any {
	return map[string]any{
		"apiVersion": apiVersion,
		"kind":       kind,
		"name":       name,
		"uid":        uid,
		"controller": controller,
	}
}

func stsWithOwners(name, namespace string, refs ...map[string]any) *unstructured.Unstructured {
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

func TestStatefulSetDrillUp(t *testing.T) {
	crGVR := schema.GroupVersionResource{Group: "example.com", Version: "v1", Resource: "widgets"}

	plugin.Reset()
	crPlugin := &drillUpStub{name: "widgets", gvr: crGVR}
	plugin.Register(crPlugin)
	t.Cleanup(plugin.Reset)

	disc := k8s.NewDiscovery()
	disc.Populate([]k8s.APIResource{
		{Name: "widgets", APIVersion: "example.com/v1", Group: "example.com", Version: "v1", Kind: "Widget", Namespaced: true, GVR: crGVR},
	})

	t.Run("statefulset owned by operator CR resolves parent", func(t *testing.T) {
		cr := &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "example.com/v1",
			"kind":       "Widget",
			"metadata":   map[string]any{"name": "w", "namespace": "default", "uid": "widget-uid"},
		}}
		store := k8s.NewStore(nil, "", nil)
		store.CacheUpsert(crGVR, "default", cr)
		cl := plugintest.NewFakeClusterWithDiscovery(store, disc)

		p := &Plugin{}
		sts := stsWithOwners("db", "default", drillUpOwnerRef("example.com/v1", "Widget", "w", "widget-uid", true))

		gotPlugin, gotObj := p.DrillUp(cl, sts)
		if gotPlugin != crPlugin {
			t.Fatalf("expected widget plugin, got %v", gotPlugin)
		}
		if gotObj == nil || gotObj.GetName() != "w" {
			t.Fatalf("expected widget 'w', got %v", gotObj)
		}
	})

	t.Run("ownerless statefulset is a no-op", func(t *testing.T) {
		store := k8s.NewStore(nil, "", nil)
		cl := plugintest.NewFakeClusterWithDiscovery(store, disc)

		p := &Plugin{}
		sts := stsWithOwners("db", "default")

		gotPlugin, gotObj := p.DrillUp(cl, sts)
		if gotPlugin != nil || gotObj != nil {
			t.Fatalf("expected (nil, nil), got (%v, %v)", gotPlugin, gotObj)
		}
	})
}
