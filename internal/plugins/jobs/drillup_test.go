package jobs

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

func jobWithOwners(name, namespace string, refs ...map[string]any) *unstructured.Unstructured {
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

func TestJobDrillUp(t *testing.T) {
	cronGVR := schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "cronjobs"}

	plugin.Reset()
	cronPlugin := &drillUpStub{name: "cronjobs", gvr: cronGVR}
	plugin.Register(cronPlugin)
	t.Cleanup(plugin.Reset)

	disc := k8s.NewDiscovery()
	disc.Populate([]k8s.APIResource{
		{Name: "cronjobs", APIVersion: "batch/v1", Group: "batch", Version: "v1", Kind: "CronJob", Namespaced: true, GVR: cronGVR},
	})

	cron := parentObj("nightly", "default", "cron-uid-1", "batch/v1", "CronJob")

	t.Run("job owned by cronjob", func(t *testing.T) {
		store := k8s.NewStore(nil, "", nil)
		store.CacheUpsert(cronGVR, "default", cron)
		cl := plugintest.NewFakeClusterWithDiscovery(store, disc)

		p := &Plugin{}
		job := jobWithOwners("nightly-12345", "default", ownerRef("batch/v1", "CronJob", "nightly", "cron-uid-1", true))

		gotPlugin, gotObj := p.DrillUp(cl, job)
		if gotPlugin != cronPlugin {
			t.Fatalf("expected cronjob plugin, got %v", gotPlugin)
		}
		if gotObj == nil || gotObj.GetName() != "nightly" {
			t.Fatalf("expected cronjob 'nightly', got %v", gotObj)
		}
	})

	t.Run("ownerless job returns nil", func(t *testing.T) {
		store := k8s.NewStore(nil, "", nil)
		cl := plugintest.NewFakeClusterWithDiscovery(store, disc)

		p := &Plugin{}
		job := jobWithOwners("standalone", "default")

		gotPlugin, gotObj := p.DrillUp(cl, job)
		if gotPlugin != nil || gotObj != nil {
			t.Fatalf("expected (nil, nil), got (%v, %v)", gotPlugin, gotObj)
		}
	})
}
