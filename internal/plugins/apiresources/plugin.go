package apiresources

import (
	"context"
	"strings"

	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/render"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var _ plugin.ResourcePlugin = (*Plugin)(nil)

var syntheticGVR = schema.GroupVersionResource{
	Group: "_ktui", Version: "v1", Resource: "api-resources",
}

// Plugin displays all discovered API resources in a table view.
type Plugin struct {
	objects []*unstructured.Unstructured
}

// New creates a new api-resources plugin.
func New() *Plugin {
	return &Plugin{}
}

func (p *Plugin) Name() string                            { return "api-resources" }
func (p *Plugin) ShortName() string                       { return "api" }
func (p *Plugin) GVR() schema.GroupVersionResource        { return syntheticGVR }
func (p *Plugin) IsClusterScoped() bool                   { return true }


func (p *Plugin) YAML(obj *unstructured.Unstructured) (render.Content, error) {
	return plugin.MarshalYAML(obj)
}

// SetResources stores discovery data and builds synthetic objects.
func (p *Plugin) SetResources(resources []k8s.APIResource) {
	p.objects = make([]*unstructured.Unstructured, len(resources))
	for i, r := range resources {
		shortNames := strings.Join(r.ShortNames, ",")
		p.objects[i] = &unstructured.Unstructured{
			Object: map[string]any{
				"metadata": map[string]any{
					"name":              r.Name,
					"creationTimestamp": nil,
				},
				"shortNames": shortNames,
				"apiVersion": r.APIVersion,
				"namespaced": r.Namespaced,
				"kind":       r.Kind,
			},
		}
	}
}

// Objects implements plugin.SelfPopulating.
func (p *Plugin) Objects() []*unstructured.Unstructured {
	return p.objects
}

func (p *Plugin) Columns() []plugin.Column {
	return []plugin.Column{
		{Title: "NAME", Flex: true},
		{Title: "SHORTNAMES", Width: 14},
		{Title: "APIVERSION", Flex: true},
		{Title: "NAMESPACED", Width: 12},
		{Title: "KIND", Flex: true},
	}
}

func (p *Plugin) Row(obj *unstructured.Unstructured) []string {
	name := obj.GetName()
	shortNames, _, _ := unstructured.NestedString(obj.Object, "shortNames")
	apiVersion, _, _ := unstructured.NestedString(obj.Object, "apiVersion")
	namespaced, _, _ := unstructured.NestedBool(obj.Object, "namespaced")
	kind, _, _ := unstructured.NestedString(obj.Object, "kind")

	nsStr := "false"
	if namespaced {
		nsStr = "true"
	}
	return []string{name, shortNames, apiVersion, nsStr, kind}
}

func (p *Plugin) Describe(_ context.Context, obj *unstructured.Unstructured) (render.Content, error) {
	b := render.NewBuilder()
	b.KV(render.LEVEL_0, "Name", obj.GetName())
	kind, _, _ := unstructured.NestedString(obj.Object, "kind")
	b.KV(render.LEVEL_0, "Kind", kind)
	apiVersion, _, _ := unstructured.NestedString(obj.Object, "apiVersion")
	b.KV(render.LEVEL_0, "API Version", apiVersion)
	namespaced, _, _ := unstructured.NestedBool(obj.Object, "namespaced")
	if namespaced {
		b.KV(render.LEVEL_0, "Namespaced", "true")
	} else {
		b.KV(render.LEVEL_0, "Namespaced", "false")
	}
	shortNames, _, _ := unstructured.NestedString(obj.Object, "shortNames")
	b.KV(render.LEVEL_0, "Short Names", shortNames)
	return b.Build(), nil
}

// GoTo implements plugin.GoToer — Enter navigates to that resource.
func (p *Plugin) GoTo(obj *unstructured.Unstructured) (string, string, bool) {
	name := obj.GetName()
	if name == "" {
		return "", "", false
	}
	if _, ok := plugin.ByName(name); !ok {
		return "", "", false
	}
	return name, "", true
}
