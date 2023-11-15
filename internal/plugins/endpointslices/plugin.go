package endpointslices

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/render"
	discoveryv1 "k8s.io/api/discovery/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var gvr = schema.GroupVersionResource{Group: "discovery.k8s.io", Version: "v1", Resource: "endpointslices"}

// Plugin implements plugin.ResourcePlugin for Kubernetes EndpointSlices.
type Plugin struct{}

// New creates a new EndpointSlices plugin.
func New(_ *k8s.Client, _ *k8s.Store) plugin.ResourcePlugin {
	return &Plugin{}
}

func (p *Plugin) Name() string                     { return "endpointslices" }
func (p *Plugin) ShortName() string                { return "endpointslice" }
func (p *Plugin) GVR() schema.GroupVersionResource { return gvr }
func (p *Plugin) IsClusterScoped() bool            { return false }

func (p *Plugin) Columns() []plugin.Column {
	return []plugin.Column{
		{Title: "NAME", Flex: true},
		{Title: "ADDRESSTYPE", Width: 14},
		{Title: "PORTS", Width: 16},
		{Title: "ENDPOINTS", Width: 10},
		{Title: "AGE", Width: 8},
	}
}

func (p *Plugin) Row(obj *unstructured.Unstructured) []string {
	name := obj.GetName()
	addressType := extractAddressType(obj)
	ports := extractPorts(obj)
	endpoints := extractEndpointCount(obj)
	age := render.FormatAge(obj)

	return []string{name, addressType, ports, endpoints, age}
}

func (p *Plugin) YAML(obj *unstructured.Unstructured) (render.Content, error) {
	return plugin.MarshalYAML(obj)
}

func (p *Plugin) Describe(ctx context.Context, obj *unstructured.Unstructured) (render.Content, error) {
	es, err := toEndpointSlice(obj)
	if err != nil {
		return render.Content{}, fmt.Errorf("converting to EndpointSlice: %w", err)
	}

	b := render.NewBuilder()

	b.KV(render.LEVEL_0, "Name", es.Name)
	b.KV(render.LEVEL_0, "Namespace", es.Namespace)

	// Labels
	b.KVMulti(render.LEVEL_0, "Labels", es.Labels)

	// Annotations
	b.KVMulti(render.LEVEL_0, "Annotations", es.Annotations)

	b.KV(render.LEVEL_0, "AddressType", string(es.AddressType))

	// Ports
	b.Section(render.LEVEL_0, "Ports")
	if len(es.Ports) > 0 {
		for i, port := range es.Ports {
			if i > 0 {
				b.RawLine(render.LEVEL_1, "")
			}
			name := "<unset>"
			if port.Name != nil && *port.Name != "" {
				name = *port.Name
			}
			b.KV(render.LEVEL_1, "Name", name)
			if port.Port != nil {
				b.KV(render.LEVEL_1, "Port", strconv.Itoa(int(*port.Port)))
			} else {
				b.KV(render.LEVEL_1, "Port", "<unset>")
			}
			protocol := "TCP"
			if port.Protocol != nil {
				protocol = string(*port.Protocol)
			}
			b.KV(render.LEVEL_1, "Protocol", protocol)
		}
	} else {
		b.RawLine(render.LEVEL_1, "<none>")
	}

	// Endpoints
	b.Section(render.LEVEL_0, "Endpoints")
	if len(es.Endpoints) > 0 {
		for i, ep := range es.Endpoints {
			if i > 0 {
				b.RawLine(render.LEVEL_1, "")
			}

			// Addresses
			b.Section(render.LEVEL_1, "Addresses")
			if len(ep.Addresses) > 0 {
				for _, addr := range ep.Addresses {
					b.RawLine(render.LEVEL_2, addr)
				}
			} else {
				b.RawLine(render.LEVEL_2, "<none>")
			}

			// Conditions
			b.Section(render.LEVEL_1, "Conditions")
			if ep.Conditions.Ready != nil {
				b.KV(render.LEVEL_2, "Ready", strconv.FormatBool(*ep.Conditions.Ready))
			}
			if ep.Conditions.Serving != nil {
				b.KV(render.LEVEL_2, "Serving", strconv.FormatBool(*ep.Conditions.Serving))
			}
			if ep.Conditions.Terminating != nil {
				b.KV(render.LEVEL_2, "Terminating", strconv.FormatBool(*ep.Conditions.Terminating))
			}

			// TargetRef
			if ep.TargetRef != nil {
				b.Section(render.LEVEL_1, "TargetRef")
				b.KV(render.LEVEL_2, "Kind", ep.TargetRef.Kind)
				b.KV(render.LEVEL_2, "Name", ep.TargetRef.Name)
				if ep.TargetRef.Namespace != "" {
					b.KV(render.LEVEL_2, "Namespace", ep.TargetRef.Namespace)
				}
			}

			// Zone
			if ep.Zone != nil && *ep.Zone != "" {
				b.KV(render.LEVEL_1, "Zone", *ep.Zone)
			}
		}
	} else {
		b.RawLine(render.LEVEL_1, "<none>")
	}

	return b.Build(), nil
}

// toEndpointSlice converts an unstructured object to a typed discoveryv1.EndpointSlice.
func toEndpointSlice(obj *unstructured.Unstructured) (*discoveryv1.EndpointSlice, error) {
	var es discoveryv1.EndpointSlice
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &es); err != nil {
		return nil, err
	}
	return &es, nil
}

// extractAddressType reads the addressType field from an unstructured EndpointSlice.
func extractAddressType(obj *unstructured.Unstructured) string {
	at, found, _ := unstructured.NestedString(obj.Object, "addressType")
	if !found || at == "" {
		return "<unknown>"
	}
	return at
}

// extractPorts builds a "port/protocol,..." string from the ports field.
func extractPorts(obj *unstructured.Unstructured) string {
	ports, found, _ := unstructured.NestedSlice(obj.Object, "ports")
	if !found || len(ports) == 0 {
		return "<none>"
	}

	var parts []string
	for _, p := range ports {
		portMap, ok := p.(map[string]any)
		if !ok {
			continue
		}
		port, _, _ := unstructured.NestedInt64(portMap, "port")
		protocol, _, _ := unstructured.NestedString(portMap, "protocol")
		if protocol == "" {
			protocol = "TCP"
		}
		parts = append(parts, fmt.Sprintf("%d/%s", port, protocol))
	}

	if len(parts) == 0 {
		return "<none>"
	}
	return strings.Join(parts, ",")
}

// extractEndpointCount returns the count of endpoints as a string.
func extractEndpointCount(obj *unstructured.Unstructured) string {
	endpoints, found, _ := unstructured.NestedSlice(obj.Object, "endpoints")
	if !found {
		return "0"
	}
	return strconv.Itoa(len(endpoints))
}
