package httproutes

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

var gvr = schema.GroupVersionResource{Group: "gateway.networking.k8s.io", Version: "v1", Resource: "httproutes"}

// Plugin implements plugin.ResourcePlugin for Gateway API HTTPRoutes.
type Plugin struct{}

// New creates a new HTTPRoute plugin.
func New(_ *k8s.Client, _ *k8s.Store) plugin.ResourcePlugin {
	return &Plugin{}
}

func (p *Plugin) Name() string                     { return "httproutes" }
func (p *Plugin) ShortName() string                { return "httproute" }
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

	// Hostnames
	hostnames := extractHostnames(obj)
	b.KV(render.LEVEL_0, "Hostnames", hostnames)

	// ParentRefs
	parentRefSlice, _, _ := unstructured.NestedSlice(obj.Object, "spec", "parentRefs")
	b.Section(render.LEVEL_0, "ParentRefs")
	if len(parentRefSlice) > 0 {
		for _, ref := range parentRefSlice {
			refMap, ok := ref.(map[string]any)
			if !ok {
				continue
			}
			refName, _ := refMap["name"].(string)
			refNS, _ := refMap["namespace"].(string)
			if refNS == "" {
				refNS = namespace
			}
			sectionName, _ := refMap["sectionName"].(string)

			b.KV(render.LEVEL_1, "Name", refName)
			b.KV(render.LEVEL_1, "Namespace", refNS)
			if sectionName != "" {
				b.KV(render.LEVEL_1, "SectionName", sectionName)
			}
		}
	} else {
		b.RawLine(render.LEVEL_1, "<none>")
	}

	// Rules
	rules, _, _ := unstructured.NestedSlice(obj.Object, "spec", "rules")
	b.Section(render.LEVEL_0, "Rules")
	if len(rules) > 0 {
		for i, rule := range rules {
			ruleMap, ok := rule.(map[string]any)
			if !ok {
				continue
			}
			b.RawLine(render.LEVEL_1, fmt.Sprintf("Rule %d:", i+1))
			describeRule(b, ruleMap)
		}
	} else {
		b.RawLine(render.LEVEL_1, "<none>")
	}

	return b.Build(), nil
}

// describeRule renders a single rule's matches and backendRefs.
func describeRule(b *render.Builder, ruleMap map[string]any) {
	// Matches
	matches, _ := ruleMap["matches"].([]any)
	if len(matches) > 0 {
		b.Section(render.LEVEL_2, "Matches")
		for _, m := range matches {
			matchMap, ok := m.(map[string]any)
			if !ok {
				continue
			}
			describeMatch(b, matchMap)
		}
	}

	// BackendRefs
	backendRefs, _ := ruleMap["backendRefs"].([]any)
	if len(backendRefs) > 0 {
		b.Section(render.LEVEL_2, "BackendRefs")
		for _, br := range backendRefs {
			brMap, ok := br.(map[string]any)
			if !ok {
				continue
			}
			describeBackendRef(b, brMap)
		}
	}
}

// describeMatch renders a single match entry (path, headers, method).
func describeMatch(b *render.Builder, matchMap map[string]any) {
	// Path
	if pathMap, ok := matchMap["path"].(map[string]any); ok {
		pathType, _ := pathMap["type"].(string)
		pathValue, _ := pathMap["value"].(string)
		b.KV(render.LEVEL_3, "Path", fmt.Sprintf("%s %s", pathType, pathValue))
	}

	// Headers
	if headers, ok := matchMap["headers"].([]any); ok && len(headers) > 0 {
		b.Section(render.LEVEL_3, "Headers")
		for _, h := range headers {
			hMap, ok := h.(map[string]any)
			if !ok {
				continue
			}
			hName, _ := hMap["name"].(string)
			hValue, _ := hMap["value"].(string)
			b.KV(render.LEVEL_4, hName, hValue)
		}
	}

	// Method
	if method, ok := matchMap["method"].(string); ok && method != "" {
		b.KV(render.LEVEL_3, "Method", method)
	}
}

// describeBackendRef renders a single backendRef entry.
func describeBackendRef(b *render.Builder, brMap map[string]any) {
	brName, _ := brMap["name"].(string)
	brNS, _ := brMap["namespace"].(string)
	b.KV(render.LEVEL_3, "Name", brName)
	if brNS != "" {
		b.KV(render.LEVEL_3, "Namespace", brNS)
	}
	if port, ok := brMap["port"]; ok {
		b.KV(render.LEVEL_3, "Port", fmt.Sprintf("%v", port))
	}
	if weight, ok := brMap["weight"]; ok {
		b.KV(render.LEVEL_3, "Weight", fmt.Sprintf("%v", weight))
	}
}

// extractHostnames returns the joined hostnames from spec.hostnames or "*" if absent/empty.
func extractHostnames(obj *unstructured.Unstructured) string {
	hostnamesRaw, found, _ := unstructured.NestedSlice(obj.Object, "spec", "hostnames")
	if !found || len(hostnamesRaw) == 0 {
		return "*"
	}

	var hostnames []string
	for _, h := range hostnamesRaw {
		if s, ok := h.(string); ok && s != "" {
			hostnames = append(hostnames, s)
		}
	}

	if len(hostnames) == 0 {
		return "*"
	}
	return strings.Join(hostnames, ",")
}

// extractParentRefs returns formatted parentRefs as "ns/name" joined by comma.
// If a parentRef has no namespace, the route's own namespace is used.
func extractParentRefs(obj *unstructured.Unstructured) string {
	refs, found, _ := unstructured.NestedSlice(obj.Object, "spec", "parentRefs")
	if !found || len(refs) == 0 {
		return "<none>"
	}

	routeNS := obj.GetNamespace()

	var parts []string
	for _, ref := range refs {
		refMap, ok := ref.(map[string]any)
		if !ok {
			continue
		}
		refName, _ := refMap["name"].(string)
		refNS, _ := refMap["namespace"].(string)
		if refNS == "" {
			refNS = routeNS
		}
		parts = append(parts, refNS+"/"+refName)
	}

	if len(parts) == 0 {
		return "<none>"
	}
	return strings.Join(parts, ",")
}
