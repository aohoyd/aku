package gateways

import (
	"context"
	"fmt"

	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/plugins/workload"
	"github.com/aohoyd/aku/internal/render"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var gvr = schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "gateways"}

// Plugin implements plugin.ResourcePlugin and plugin.DrillDowner for Gateway API Gateways.
type Plugin struct {
	store *k8s.Store
}

// New creates a new Gateways plugin.
func New(_ *k8s.Client, store *k8s.Store) plugin.ResourcePlugin {
	return &Plugin{store: store}
}

func (p *Plugin) Name() string                     { return "gateways" }
func (p *Plugin) ShortName() string                { return "gtw" }
func (p *Plugin) GVR() schema.GroupVersionResource { return gvr }
func (p *Plugin) IsClusterScoped() bool            { return false }

func (p *Plugin) Columns() []plugin.Column {
	return []plugin.Column{
		{Title: "NAME", Flex: true},
		{Title: "CLASS", Width: 16},
		{Title: "ADDRESS", Width: 20},
		{Title: "PROGRAMMED", Width: 12},
		{Title: "AGE", Width: 8},
	}
}

func (p *Plugin) Row(obj *unstructured.Unstructured) []string {
	name := obj.GetName()

	className, _, _ := unstructured.NestedString(obj.Object, "spec", "gatewayClassName")

	address := extractFirstAddress(obj)

	programmed := extractProgrammedStatus(obj)

	age := render.FormatAge(obj)

	return []string{name, className, address, programmed, age}
}

func (p *Plugin) YAML(obj *unstructured.Unstructured) (render.Content, error) {
	return plugin.MarshalYAML(obj)
}

func (p *Plugin) Describe(ctx context.Context, obj *unstructured.Unstructured) (render.Content, error) {
	b := render.NewBuilder()

	// Metadata
	b.KV(render.LEVEL_0, "Name", obj.GetName())
	b.KV(render.LEVEL_0, "Namespace", obj.GetNamespace())
	b.KVMulti(render.LEVEL_0, "Labels", obj.GetLabels())
	b.KVMulti(render.LEVEL_0, "Annotations", obj.GetAnnotations())

	// GatewayClassName
	className, _, _ := unstructured.NestedString(obj.Object, "spec", "gatewayClassName")
	b.KV(render.LEVEL_0, "GatewayClassName", className)

	// Listeners
	listeners, found, _ := unstructured.NestedSlice(obj.Object, "spec", "listeners")
	if found && len(listeners) > 0 {
		b.Section(render.LEVEL_0, "Listeners")
		for _, l := range listeners {
			lMap, ok := l.(map[string]any)
			if !ok {
				continue
			}
			lName, _ := lMap["name"].(string)
			hostname, _ := lMap["hostname"].(string)
			port := formatPort(lMap["port"])
			protocol, _ := lMap["protocol"].(string)

			b.KV(render.LEVEL_1, "Name", lName)
			b.KV(render.LEVEL_1, "Hostname", hostname)
			b.KV(render.LEVEL_1, "Port", port)
			b.KV(render.LEVEL_1, "Protocol", protocol)
		}
	}

	// Addresses
	addresses, found, _ := unstructured.NestedSlice(obj.Object, "status", "addresses")
	if found && len(addresses) > 0 {
		b.Section(render.LEVEL_0, "Addresses")
		for _, a := range addresses {
			aMap, ok := a.(map[string]any)
			if !ok {
				continue
			}
			aType, _ := aMap["type"].(string)
			aValue, _ := aMap["value"].(string)
			b.KV(render.LEVEL_1, "Type", aType)
			b.KV(render.LEVEL_1, "Value", aValue)
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
			cType, _ := cMap["type"].(string)
			cStatus, _ := cMap["status"].(string)
			cReason, _ := cMap["reason"].(string)
			cMessage, _ := cMap["message"].(string)

			b.KVStyled(render.LEVEL_1, render.ConditionKind(cStatus), cType, cStatus)
			if cReason != "" {
				b.KV(render.LEVEL_2, "Reason", cReason)
			}
			if cMessage != "" {
				b.KV(render.LEVEL_2, "Message", cMessage)
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
	pp, ok := plugin.ByName("httproutes")
	if !ok {
		return nil, nil
	}
	p.store.Subscribe(workload.HTTPRoutesGVR, "")
	routes := workload.FindHTTPRoutesByGateway(p.store, obj.GetNamespace(), obj.GetName())
	return pp, routes
}

// extractFirstAddress returns the first address value from status.addresses,
// or "<none>" if no addresses are present.
func extractFirstAddress(obj *unstructured.Unstructured) string {
	addresses, found, _ := unstructured.NestedSlice(obj.Object, "status", "addresses")
	if !found || len(addresses) == 0 {
		return "<none>"
	}
	for _, a := range addresses {
		aMap, ok := a.(map[string]any)
		if !ok {
			continue
		}
		if val, ok := aMap["value"].(string); ok && val != "" {
			return val
		}
	}
	return "<none>"
}

// extractProgrammedStatus returns the status of the Programmed condition,
// or "Unknown" if not found.
func extractProgrammedStatus(obj *unstructured.Unstructured) string {
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
		if cType == "Programmed" {
			status, _ := cMap["status"].(string)
			if status == "" {
				return "Unknown"
			}
			return status
		}
	}
	return "Unknown"
}

// formatPort converts a port value (which may be int64 or float64 from JSON) to a string.
func formatPort(v any) string {
	switch p := v.(type) {
	case int64:
		return fmt.Sprintf("%d", p)
	case float64:
		return fmt.Sprintf("%d", int64(p))
	default:
		return ""
	}
}
