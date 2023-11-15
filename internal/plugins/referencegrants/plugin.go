package referencegrants

import (
	"context"

	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/render"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var gvr = schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "referencegrants"}

// Plugin implements plugin.ResourcePlugin for Gateway API ReferenceGrants.
type Plugin struct{}

// New creates a new ReferenceGrant plugin.
func New(_ *k8s.Client, _ *k8s.Store) plugin.ResourcePlugin {
	return &Plugin{}
}

func (p *Plugin) Name() string                     { return "referencegrants" }
func (p *Plugin) ShortName() string                { return "refgrant" }
func (p *Plugin) GVR() schema.GroupVersionResource { return gvr }
func (p *Plugin) IsClusterScoped() bool            { return false }

func (p *Plugin) Columns() []plugin.Column {
	return []plugin.Column{
		{Title: "NAME", Flex: true},
		{Title: "AGE", Width: 8},
	}
}

func (p *Plugin) Row(obj *unstructured.Unstructured) []string {
	name := obj.GetName()
	age := render.FormatAge(obj)
	return []string{name, age}
}

func (p *Plugin) YAML(obj *unstructured.Unstructured) (render.Content, error) {
	return plugin.MarshalYAML(obj)
}

func (p *Plugin) Describe(ctx context.Context, obj *unstructured.Unstructured) (render.Content, error) {
	b := render.NewBuilder()

	// Basic metadata
	name, _, _ := unstructured.NestedString(obj.Object, "metadata", "name")
	namespace, _, _ := unstructured.NestedString(obj.Object, "metadata", "namespace")

	b.KV(render.LEVEL_0, "Name", name)
	b.KV(render.LEVEL_0, "Namespace", namespace)

	// Labels
	labels, _, _ := unstructured.NestedStringMap(obj.Object, "metadata", "labels")
	b.KVMulti(render.LEVEL_0, "Labels", labels)

	// Annotations
	annotations, _, _ := unstructured.NestedStringMap(obj.Object, "metadata", "annotations")
	b.KVMulti(render.LEVEL_0, "Annotations", annotations)

	// From section
	fromSlice, _, _ := unstructured.NestedSlice(obj.Object, "spec", "from")
	b.Section(render.LEVEL_0, "From")
	if len(fromSlice) > 0 {
		for _, entry := range fromSlice {
			entryMap, ok := entry.(map[string]any)
			if !ok {
				continue
			}
			group, _ := entryMap["group"].(string)
			kind, _ := entryMap["kind"].(string)
			ns, _ := entryMap["namespace"].(string)

			b.KV(render.LEVEL_1, "Group", group)
			b.KV(render.LEVEL_1, "Kind", kind)
			b.KV(render.LEVEL_1, "Namespace", ns)
		}
	} else {
		b.RawLine(render.LEVEL_1, "<none>")
	}

	// To section
	toSlice, _, _ := unstructured.NestedSlice(obj.Object, "spec", "to")
	b.Section(render.LEVEL_0, "To")
	if len(toSlice) > 0 {
		for _, entry := range toSlice {
			entryMap, ok := entry.(map[string]any)
			if !ok {
				continue
			}
			group, _ := entryMap["group"].(string)
			kind, _ := entryMap["kind"].(string)
			toName, _ := entryMap["name"].(string)

			b.KV(render.LEVEL_1, "Group", group)
			b.KV(render.LEVEL_1, "Kind", kind)
			b.KV(render.LEVEL_1, "Name", toName)
		}
	} else {
		b.RawLine(render.LEVEL_1, "<none>")
	}

	return b.Build(), nil
}
