package portforwards

import (
	"context"
	"fmt"

	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/portforward"
	"github.com/aohoyd/aku/internal/render"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var _ plugin.ResourcePlugin = (*Plugin)(nil)

var syntheticGVR = schema.GroupVersionResource{
	Group: "_ktui", Version: "v1", Resource: "portforwards",
}

// Plugin displays active port-forwards in a table view.
type Plugin struct {
	registry *portforward.Registry
}

// New creates a new portforwards plugin.
func New(registry *portforward.Registry) *Plugin {
	return &Plugin{registry: registry}
}

func (p *Plugin) Name() string                            { return "portforwards" }
func (p *Plugin) ShortName() string                       { return "pf" }
func (p *Plugin) GVR() schema.GroupVersionResource        { return syntheticGVR }
func (p *Plugin) IsClusterScoped() bool                   { return true }

func (p *Plugin) YAML(obj *unstructured.Unstructured) (render.Content, error) {
	return plugin.MarshalYAML(obj)
}

func (p *Plugin) Columns() []plugin.Column {
	return []plugin.Column{
		{Title: "POD", Flex: true},
		{Title: "CONTAINER", Width: 16},
		{Title: "PORTS", Width: 16},
		{Title: "STATUS", Width: 10},
	}
}

func (p *Plugin) Row(obj *unstructured.Unstructured) []string {
	pod, _, _ := unstructured.NestedString(obj.Object, "pod")
	container, _, _ := unstructured.NestedString(obj.Object, "container")
	ports, _, _ := unstructured.NestedString(obj.Object, "ports")
	status, _, _ := unstructured.NestedString(obj.Object, "status")
	return []string{pod, container, ports, status}
}

// Objects implements plugin.SelfPopulating.
func (p *Plugin) Objects() []*unstructured.Unstructured {
	entries := p.registry.List()
	objs := make([]*unstructured.Unstructured, len(entries))
	for i, e := range entries {
		objs[i] = &unstructured.Unstructured{
			Object: map[string]any{
				"metadata": map[string]any{
					"name":              e.ID,
					"namespace":         e.PodNamespace,
					"creationTimestamp": nil,
				},
				"pod":       e.PodName,
				"container": e.ContainerName,
				"ports":     fmt.Sprintf("%d:%d/%s", e.LocalPort, e.RemotePort, e.Protocol),
				"status":    e.Status,
			},
		}
	}
	return objs
}

func (p *Plugin) Describe(_ context.Context, obj *unstructured.Unstructured) (render.Content, error) {
	b := render.NewBuilder()
	b.KV(render.LEVEL_0, "ID", obj.GetName())
	pod, _, _ := unstructured.NestedString(obj.Object, "pod")
	b.KV(render.LEVEL_0, "Pod", pod)
	ns := obj.GetNamespace()
	b.KV(render.LEVEL_0, "Namespace", ns)
	container, _, _ := unstructured.NestedString(obj.Object, "container")
	b.KV(render.LEVEL_0, "Container", container)
	ports, _, _ := unstructured.NestedString(obj.Object, "ports")
	b.KV(render.LEVEL_0, "Ports", ports)
	status, _, _ := unstructured.NestedString(obj.Object, "status")
	b.KV(render.LEVEL_0, "Status", status)
	return b.Build(), nil
}
