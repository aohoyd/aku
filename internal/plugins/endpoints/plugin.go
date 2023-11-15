package endpoints

import (
	"context"
	"fmt"
	"strings"

	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/render"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var gvr = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "endpoints"}

// Plugin implements plugin.ResourcePlugin for Kubernetes Endpoints.
type Plugin struct{}

// New creates a new Endpoints plugin.
func New(_ *k8s.Client, _ *k8s.Store) plugin.ResourcePlugin {
	return &Plugin{}
}

func (p *Plugin) Name() string                     { return "endpoints" }
func (p *Plugin) ShortName() string                { return "ep" }
func (p *Plugin) GVR() schema.GroupVersionResource { return gvr }
func (p *Plugin) IsClusterScoped() bool            { return false }

func (p *Plugin) Columns() []plugin.Column {
	return []plugin.Column{
		{Title: "NAME", Flex: true},
		{Title: "ENDPOINTS", Flex: true},
		{Title: "AGE", Width: 8},
	}
}

func (p *Plugin) Row(obj *unstructured.Unstructured) []string {
	name := obj.GetName()
	endpoints := extractEndpoints(obj)
	age := render.FormatAge(obj)

	return []string{name, endpoints, age}
}

func (p *Plugin) YAML(obj *unstructured.Unstructured) (render.Content, error) {
	return plugin.MarshalYAML(obj)
}

func (p *Plugin) Describe(ctx context.Context, obj *unstructured.Unstructured) (render.Content, error) {
	ep, err := toEndpoints(obj)
	if err != nil {
		return render.Content{}, fmt.Errorf("converting to Endpoints: %w", err)
	}

	b := render.NewBuilder()

	b.KV(render.LEVEL_0, "Name", ep.Name)
	b.KV(render.LEVEL_0, "Namespace", ep.Namespace)

	// Labels
	b.KVMulti(render.LEVEL_0, "Labels", ep.Labels)

	// Annotations
	b.KVMulti(render.LEVEL_0, "Annotations", ep.Annotations)

	// Subsets
	b.Section(render.LEVEL_0, "Subsets")
	if len(ep.Subsets) > 0 {
		for i, subset := range ep.Subsets {
			if i > 0 {
				b.RawLine(render.LEVEL_1, "")
			}

			// Addresses
			b.Section(render.LEVEL_1, "Addresses")
			if len(subset.Addresses) > 0 {
				for _, addr := range subset.Addresses {
					b.RawLine(render.LEVEL_2, addr.IP)
				}
			} else {
				b.RawLine(render.LEVEL_2, "<none>")
			}

			// Ports
			b.Section(render.LEVEL_1, "Ports")
			if len(subset.Ports) > 0 {
				for _, port := range subset.Ports {
					protocol := string(port.Protocol)
					if protocol == "" {
						protocol = "TCP"
					}
					portStr := fmt.Sprintf("%d/%s", port.Port, protocol)
					if port.Name != "" {
						portStr = fmt.Sprintf("%s %d/%s", port.Name, port.Port, protocol)
					}
					b.RawLine(render.LEVEL_2, portStr)
				}
			} else {
				b.RawLine(render.LEVEL_2, "<none>")
			}
		}
	} else {
		b.RawLine(render.LEVEL_1, "<none>")
	}

	return b.Build(), nil
}

// toEndpoints converts an unstructured object to a typed corev1.Endpoints.
func toEndpoints(obj *unstructured.Unstructured) (*corev1.Endpoints, error) {
	var ep corev1.Endpoints
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &ep); err != nil {
		return nil, err
	}
	return &ep, nil
}

// extractEndpoints builds the "ip:port,..." string from unstructured subsets.
func extractEndpoints(obj *unstructured.Unstructured) string {
	subsets, found, _ := unstructured.NestedSlice(obj.Object, "subsets")
	if !found || len(subsets) == 0 {
		return "<none>"
	}

	var parts []string
	for _, s := range subsets {
		subset, ok := s.(map[string]any)
		if !ok {
			continue
		}

		addresses, _, _ := unstructured.NestedSlice(subset, "addresses")
		ports, _, _ := unstructured.NestedSlice(subset, "ports")

		for _, a := range addresses {
			addrMap, ok := a.(map[string]any)
			if !ok {
				continue
			}
			ip, _, _ := unstructured.NestedString(addrMap, "ip")
			if ip == "" {
				continue
			}

			if len(ports) == 0 {
				parts = append(parts, ip)
				continue
			}

			for _, pt := range ports {
				portMap, ok := pt.(map[string]any)
				if !ok {
					continue
				}
				port, _, _ := unstructured.NestedInt64(portMap, "port")
				parts = append(parts, fmt.Sprintf("%s:%d", ip, port))
			}
		}
	}

	if len(parts) == 0 {
		return "<none>"
	}
	return strings.Join(parts, ",")
}
