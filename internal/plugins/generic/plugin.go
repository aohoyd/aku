package generic

import (
	"context"

	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/render"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Plugin implements plugin.ResourcePlugin for any Kubernetes resource
// that does not have a dedicated plugin.
type Plugin struct {
	gvr           schema.GroupVersionResource
	name          string
	shortName     string
	clusterScoped bool
}

// New creates a Plugin for the given GVR.
func New(gvr schema.GroupVersionResource) plugin.ResourcePlugin {
	return &Plugin{gvr: gvr}
}

// NewDiscovered creates a Plugin from API discovery data.
func NewDiscovered(gvr schema.GroupVersionResource, name, shortName string, clusterScoped bool) plugin.ResourcePlugin {
	return &Plugin{
		gvr:           gvr,
		name:          name,
		shortName:     shortName,
		clusterScoped: clusterScoped,
	}
}

func (p *Plugin) Name() string {
	if p.name != "" {
		return p.name
	}
	return p.gvr.Resource
}

func (p *Plugin) ShortName() string {
	if p.shortName != "" {
		return p.shortName
	}
	return p.Name()
}

func (p *Plugin) GVR() schema.GroupVersionResource { return p.gvr }
func (p *Plugin) IsClusterScoped() bool            { return p.clusterScoped }

func (p *Plugin) Columns() []plugin.Column {
	return []plugin.Column{
		{Title: "NAME", Flex: true},
		{Title: "AGE", Width: 8},
	}
}

func (p *Plugin) Row(obj *unstructured.Unstructured) []string {
	return []string{obj.GetName(), render.FormatAge(obj)}
}

func (p *Plugin) YAML(obj *unstructured.Unstructured) (render.Content, error) {
	return plugin.MarshalYAML(obj)
}

func (p *Plugin) Describe(_ context.Context, obj *unstructured.Unstructured) (render.Content, error) {
	b := render.NewBuilder()
	describeGeneric(b, obj)
	return b.Build(), nil
}

func describeGeneric(b *render.Builder, obj *unstructured.Unstructured) {
	b.KV(render.LEVEL_0, "Name", obj.GetName())
	b.KV(render.LEVEL_0, "Namespace", obj.GetNamespace())
	b.KVMulti(render.LEVEL_0, "Labels", obj.GetLabels())
	b.KVMulti(render.LEVEL_0, "Annotations", obj.GetAnnotations())
}
