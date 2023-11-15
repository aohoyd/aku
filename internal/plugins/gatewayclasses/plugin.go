package gatewayclasses

import (
	"context"

	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/plugins/workload"
	"github.com/aohoyd/aku/internal/render"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var gvr = schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "gatewayclasses"}

// Plugin implements plugin.ResourcePlugin and plugin.DrillDowner for Kubernetes GatewayClasses.
type Plugin struct {
	store *k8s.Store
}

// New creates a new GatewayClasses plugin.
func New(_ *k8s.Client, store *k8s.Store) plugin.ResourcePlugin {
	return &Plugin{store: store}
}

func (p *Plugin) Name() string                     { return "gatewayclasses" }
func (p *Plugin) ShortName() string                { return "gc" }
func (p *Plugin) GVR() schema.GroupVersionResource { return gvr }
func (p *Plugin) IsClusterScoped() bool            { return true }

func (p *Plugin) Columns() []plugin.Column {
	return []plugin.Column{
		{Title: "NAME", Flex: true},
		{Title: "CONTROLLER", Flex: true},
		{Title: "ACCEPTED", Width: 10},
		{Title: "AGE", Width: 8},
	}
}

func (p *Plugin) Row(obj *unstructured.Unstructured) []string {
	name := obj.GetName()

	controllerName, _, _ := unstructured.NestedString(obj.Object, "spec", "controllerName")

	accepted := acceptedStatus(obj)

	age := render.FormatAge(obj)

	return []string{name, controllerName, accepted, age}
}

func (p *Plugin) YAML(obj *unstructured.Unstructured) (render.Content, error) {
	return plugin.MarshalYAML(obj)
}

func (p *Plugin) Describe(_ context.Context, obj *unstructured.Unstructured) (render.Content, error) {
	b := render.NewBuilder()

	// Metadata
	b.KV(render.LEVEL_0, "Name", obj.GetName())
	b.KVMulti(render.LEVEL_0, "Labels", obj.GetLabels())
	b.KVMulti(render.LEVEL_0, "Annotations", obj.GetAnnotations())

	// Controller
	controllerName, _, _ := unstructured.NestedString(obj.Object, "spec", "controllerName")
	b.KV(render.LEVEL_0, "ControllerName", controllerName)

	// ParametersRef
	paramRef, found, _ := unstructured.NestedMap(obj.Object, "spec", "parametersRef")
	if found && len(paramRef) > 0 {
		b.Section(render.LEVEL_0, "ParametersRef")
		if group, ok := paramRef["group"].(string); ok {
			b.KV(render.LEVEL_1, "Group", group)
		}
		if kind, ok := paramRef["kind"].(string); ok {
			b.KV(render.LEVEL_1, "Kind", kind)
		}
		if name, ok := paramRef["name"].(string); ok {
			b.KV(render.LEVEL_1, "Name", name)
		}
		if ns, ok := paramRef["namespace"].(string); ok {
			b.KV(render.LEVEL_1, "Namespace", ns)
		}
	}

	// Conditions
	conditions, found, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if found && len(conditions) > 0 {
		b.Section(render.LEVEL_0, "Conditions")
		for _, c := range conditions {
			cMap, ok := c.(map[string]any)
			if !ok {
				continue
			}
			condType, _ := cMap["type"].(string)
			status, _ := cMap["status"].(string)
			reason, _ := cMap["reason"].(string)
			message, _ := cMap["message"].(string)

			b.KVStyled(render.LEVEL_1, render.ConditionKind(status), condType, status)
			if reason != "" {
				b.KV(render.LEVEL_2, "Reason", reason)
			}
			if message != "" {
				b.KV(render.LEVEL_2, "Message", message)
			}
		}
	}

	return b.Build(), nil
}

// DrillDown implements plugin.DrillDowner.
func (p *Plugin) DrillDown(obj *unstructured.Unstructured) (plugin.ResourcePlugin, []*unstructured.Unstructured) {
	if p.store == nil {
		return nil, nil
	}
	gw, ok := plugin.ByName("gateways")
	if !ok {
		return nil, nil
	}
	p.store.Subscribe(workload.GatewaysGVR, "")
	gateways := workload.FindGatewaysByClassName(p.store, "", obj.GetName())
	return gw, gateways
}

// acceptedStatus extracts the Accepted condition status from a GatewayClass object.
func acceptedStatus(obj *unstructured.Unstructured) string {
	conditions, found, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if !found {
		return "Unknown"
	}
	for _, c := range conditions {
		cMap, ok := c.(map[string]any)
		if !ok {
			continue
		}
		cType, _ := cMap["type"].(string)
		if cType == "Accepted" {
			status, _ := cMap["status"].(string)
			if status == "" {
				return "Unknown"
			}
			return status
		}
	}
	return "Unknown"
}
