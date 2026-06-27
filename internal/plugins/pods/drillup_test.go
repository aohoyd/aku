package pods

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
	name          string
	gvr           schema.GroupVersionResource
	clusterScoped bool
}

func (p *drillUpStub) Name() string                     { return p.name }
func (p *drillUpStub) ShortName() string                { return p.name }
func (p *drillUpStub) GVR() schema.GroupVersionResource { return p.gvr }
func (p *drillUpStub) IsClusterScoped() bool            { return p.clusterScoped }
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

func podWithOwners(name, namespace string, refs ...map[string]any) *unstructured.Unstructured {
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

func TestPodDrillUp(t *testing.T) {
	rsGVR := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "replicasets"}
	jobsGVR := schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "jobs"}

	plugin.Reset()
	rsPlugin := &drillUpStub{name: "replicasets", gvr: rsGVR}
	jobPlugin := &drillUpStub{name: "jobs", gvr: jobsGVR}
	plugin.Register(rsPlugin)
	plugin.Register(jobPlugin)
	t.Cleanup(plugin.Reset)

	disc := k8s.NewDiscovery()
	disc.Populate([]k8s.APIResource{
		{Name: "replicasets", APIVersion: "apps/v1", Group: "apps", Version: "v1", Kind: "ReplicaSet", Namespaced: true, GVR: rsGVR},
		{Name: "jobs", APIVersion: "batch/v1", Group: "batch", Version: "v1", Kind: "Job", Namespaced: true, GVR: jobsGVR},
	})

	rs := parentObj("web-abc", "default", "rs-uid-1", "apps/v1", "ReplicaSet")
	job := parentObj("nightly", "default", "job-uid-1", "batch/v1", "Job")

	tests := []struct {
		name        string
		pod         *unstructured.Unstructured
		storeGVR    schema.GroupVersionResource
		storeObj    *unstructured.Unstructured
		wantPlugin  plugin.ResourcePlugin
		wantObjName string
	}{
		{
			name:        "pod owned by replicaset",
			pod:         podWithOwners("web-abc-xyz", "default", ownerRef("apps/v1", "ReplicaSet", "web-abc", "rs-uid-1", true)),
			storeGVR:    rsGVR,
			storeObj:    rs,
			wantPlugin:  rsPlugin,
			wantObjName: "web-abc",
		},
		{
			name:        "pod owned by job",
			pod:         podWithOwners("nightly-pod", "default", ownerRef("batch/v1", "Job", "nightly", "job-uid-1", true)),
			storeGVR:    jobsGVR,
			storeObj:    job,
			wantPlugin:  jobPlugin,
			wantObjName: "nightly",
		},
		{
			name:       "ownerless pod returns nil",
			pod:        podWithOwners("bare-pod", "default"),
			wantPlugin: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := k8s.NewStore(nil, "", nil)
			if tt.storeObj != nil {
				store.CacheUpsert(tt.storeGVR, "default", tt.storeObj)
			}
			cl := plugintest.NewFakeClusterWithDiscovery(store, disc)

			p := &Plugin{}
			gotPlugin, gotObj := p.DrillUp(cl, tt.pod)

			if tt.wantPlugin == nil {
				if gotPlugin != nil {
					t.Fatalf("expected nil plugin, got %q", gotPlugin.Name())
				}
				if gotObj != nil {
					t.Fatalf("expected nil object, got %q", gotObj.GetName())
				}
				return
			}
			if gotPlugin != tt.wantPlugin {
				t.Fatalf("plugin mismatch: got %v, want %v", gotPlugin, tt.wantPlugin)
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
