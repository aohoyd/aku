package replicasets

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

// drillUpStub is a minimal plugin.ResourcePlugin for DrillUp registry tests.
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

func ownerRef(apiVersion, kind, name, uid string, controller bool) map[string]any {
	return map[string]any{
		"apiVersion": apiVersion,
		"kind":       kind,
		"name":       name,
		"uid":        uid,
		"controller": controller,
	}
}

func rsWithOwners(name, namespace string, refs ...map[string]any) *unstructured.Unstructured {
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

func parentObj(name, namespace, uid, apiVersion, kind string) *unstructured.Unstructured {
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

func TestReplicaSetDrillUp(t *testing.T) {
	deployGVR := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}

	plugin.Reset()
	deployPlugin := &drillUpStub{name: "deployments", gvr: deployGVR}
	plugin.Register(deployPlugin)
	t.Cleanup(plugin.Reset)

	disc := k8s.NewDiscovery()
	disc.Populate([]k8s.APIResource{
		{Name: "deployments", APIVersion: "apps/v1", Group: "apps", Version: "v1", Kind: "Deployment", Namespaced: true, GVR: deployGVR},
	})

	deploy := parentObj("web", "default", "deploy-uid-1", "apps/v1", "Deployment")

	t.Run("replicaset owned by deployment", func(t *testing.T) {
		store := k8s.NewStore(nil, "", nil)
		store.CacheUpsert(deployGVR, "default", deploy)
		cl := plugintest.NewFakeClusterWithDiscovery(store, disc)

		p := &Plugin{}
		rs := rsWithOwners("web-abc", "default", ownerRef("apps/v1", "Deployment", "web", "deploy-uid-1", true))

		gotPlugin, gotObj := p.DrillUp(cl, rs)
		if gotPlugin != deployPlugin {
			t.Fatalf("expected deployment plugin, got %v", gotPlugin)
		}
		if gotObj == nil || gotObj.GetName() != "web" {
			t.Fatalf("expected deployment 'web', got %v", gotObj)
		}
	})

	t.Run("ownerless replicaset returns nil", func(t *testing.T) {
		store := k8s.NewStore(nil, "", nil)
		cl := plugintest.NewFakeClusterWithDiscovery(store, disc)

		p := &Plugin{}
		rs := rsWithOwners("lonely", "default")

		gotPlugin, gotObj := p.DrillUp(cl, rs)
		if gotPlugin != nil || gotObj != nil {
			t.Fatalf("expected (nil, nil), got (%v, %v)", gotPlugin, gotObj)
		}
	})
}
