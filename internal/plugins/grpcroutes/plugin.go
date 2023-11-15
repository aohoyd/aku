package grpcroutes

import (
	"context"
	"fmt"
	"strings"

	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/render"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var gvr = schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "grpcroutes"}

// Plugin implements plugin.ResourcePlugin for Kubernetes GRPCRoutes.
type Plugin struct{}

// New creates a new GRPCRoute plugin.
func New(_ *k8s.Client, _ *k8s.Store) plugin.ResourcePlugin {
	return &Plugin{}
}

func (p *Plugin) Name() string                     { return "grpcroutes" }
func (p *Plugin) ShortName() string                { return "grpcroute" }
func (p *Plugin) GVR() schema.GroupVersionResource { return gvr }
func (p *Plugin) IsClusterScoped() bool            { return false }

func (p *Plugin) Columns() []plugin.Column {
	return []plugin.Column{
		{Title: "NAME", Flex: true},
		{Title: "HOSTNAMES", Flex: true},
		{Title: "PARENT-REFS", Flex: true},
		{Title: "AGE", Width: 8},
	}
}

func (p *Plugin) Row(obj *unstructured.Unstructured) []string {
	name := obj.GetName()
	hostnames := extractHostnames(obj)
	parentRefs := extractParentRefs(obj)
	age := render.FormatAge(obj)

	return []string{name, hostnames, parentRefs, age}
}

func (p *Plugin) YAML(obj *unstructured.Unstructured) (render.Content, error) {
	return plugin.MarshalYAML(obj)
}

func (p *Plugin) Describe(ctx context.Context, obj *unstructured.Unstructured) (render.Content, error) {
	b := render.NewBuilder()

	// Basic metadata
	b.KV(render.LEVEL_0, "Name", obj.GetName())
	b.KV(render.LEVEL_0, "Namespace", obj.GetNamespace())

	// Labels
	b.KVMulti(render.LEVEL_0, "Labels", obj.GetLabels())

	// Annotations
	b.KVMulti(render.LEVEL_0, "Annotations", obj.GetAnnotations())

	// Hostnames
	hostnames := extractHostnames(obj)
	b.KV(render.LEVEL_0, "Hostnames", hostnames)

	// ParentRefs
	b.Section(render.LEVEL_0, "ParentRefs")
	parentRefs, found, _ := unstructured.NestedSlice(obj.Object, "spec", "parentRefs")
	if found && len(parentRefs) > 0 {
		for i, ref := range parentRefs {
			refMap, ok := ref.(map[string]any)
			if !ok {
				continue
			}
			if i > 0 {
				b.RawLine(render.LEVEL_1, "")
			}
			name, _, _ := unstructured.NestedString(refMap, "name")
			b.KV(render.LEVEL_1, "Name", name)
			ns, _, _ := unstructured.NestedString(refMap, "namespace")
			if ns == "" {
				ns = obj.GetNamespace()
			}
			b.KV(render.LEVEL_1, "Namespace", ns)
			sectionName, _, _ := unstructured.NestedString(refMap, "sectionName")
			if sectionName != "" {
				b.KV(render.LEVEL_1, "SectionName", sectionName)
			}
		}
	} else {
		b.RawLine(render.LEVEL_1, "<none>")
	}

	// Rules
	b.Section(render.LEVEL_0, "Rules")
	rules, found, _ := unstructured.NestedSlice(obj.Object, "spec", "rules")
	if found && len(rules) > 0 {
		for i, rule := range rules {
			ruleMap, ok := rule.(map[string]any)
			if !ok {
				continue
			}
			if i > 0 {
				b.RawLine(render.LEVEL_1, "")
			}
			b.RawLine(render.LEVEL_1, fmt.Sprintf("Rule %d:", i+1))

			// Matches
			matches, _, _ := unstructured.NestedSlice(ruleMap, "matches")
			if len(matches) > 0 {
				b.Section(render.LEVEL_2, "Matches")
				for _, m := range matches {
					mMap, ok := m.(map[string]any)
					if !ok {
						continue
					}
					methodMap, ok := mMap["method"].(map[string]any)
					if ok {
						svc, _, _ := unstructured.NestedString(methodMap, "service")
						method, _, _ := unstructured.NestedString(methodMap, "method")
						b.KV(render.LEVEL_3, "Method", fmt.Sprintf("%s/%s", svc, method))
					}
				}
			}

			// BackendRefs
			backendRefs, _, _ := unstructured.NestedSlice(ruleMap, "backendRefs")
			if len(backendRefs) > 0 {
				b.Section(render.LEVEL_2, "BackendRefs")
				for _, br := range backendRefs {
					brMap, ok := br.(map[string]any)
					if !ok {
						continue
					}
					brName, _, _ := unstructured.NestedString(brMap, "name")
					brNs, _, _ := unstructured.NestedString(brMap, "namespace")
					if brNs == "" {
						brNs = obj.GetNamespace()
					}
					b.KV(render.LEVEL_3, "Name", brName)
					b.KV(render.LEVEL_3, "Namespace", brNs)
					port, portFound, _ := unstructured.NestedInt64(brMap, "port")
					if portFound {
						b.KV(render.LEVEL_3, "Port", fmt.Sprintf("%d", port))
					}
					weight, weightFound, _ := unstructured.NestedInt64(brMap, "weight")
					if weightFound {
						b.KV(render.LEVEL_3, "Weight", fmt.Sprintf("%d", weight))
					}
				}
			}
		}
	} else {
		b.RawLine(render.LEVEL_1, "<none>")
	}

	return b.Build(), nil
}

// extractHostnames returns a comma-separated list of hostnames from
// spec.hostnames[], or "*" if the field is absent or empty.
func extractHostnames(obj *unstructured.Unstructured) string {
	hostnames, found, _ := unstructured.NestedSlice(obj.Object, "spec", "hostnames")
	if !found || len(hostnames) == 0 {
		return "*"
	}

	var parts []string
	for _, h := range hostnames {
		if s, ok := h.(string); ok && s != "" {
			parts = append(parts, s)
		}
	}
	if len(parts) == 0 {
		return "*"
	}
	return strings.Join(parts, ",")
}

// extractParentRefs returns a comma-separated list of "ns/name" entries from
// spec.parentRefs[]. If a parentRef has no namespace, the route's own
// namespace is used as the default.
func extractParentRefs(obj *unstructured.Unstructured) string {
	refs, found, _ := unstructured.NestedSlice(obj.Object, "spec", "parentRefs")
	if !found || len(refs) == 0 {
		return "<none>"
	}

	routeNs := obj.GetNamespace()
	var parts []string
	for _, r := range refs {
		rMap, ok := r.(map[string]any)
		if !ok {
			continue
		}
		name, _, _ := unstructured.NestedString(rMap, "name")
		ns, _, _ := unstructured.NestedString(rMap, "namespace")
		if ns == "" {
			ns = routeNs
		}
		parts = append(parts, fmt.Sprintf("%s/%s", ns, name))
	}
	if len(parts) == 0 {
		return "<none>"
	}
	return strings.Join(parts, ",")
}
