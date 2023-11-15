package customresourcedefinitions

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

var gvr = schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"}

// Plugin implements plugin.ResourcePlugin for CustomResourceDefinitions.
type Plugin struct{}

// New creates a new CustomResourceDefinitions plugin.
func New(_ *k8s.Client, _ *k8s.Store) plugin.ResourcePlugin {
	return &Plugin{}
}

func (p *Plugin) Name() string                            { return "customresourcedefinitions" }
func (p *Plugin) ShortName() string                       { return "crd" }
func (p *Plugin) GVR() schema.GroupVersionResource        { return gvr }
func (p *Plugin) IsClusterScoped() bool                   { return true }

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

func (p *Plugin) YAML(obj *unstructured.Unstructured) (render.Content, error) { return plugin.MarshalYAML(obj) }

func (p *Plugin) Describe(_ context.Context, obj *unstructured.Unstructured) (render.Content, error) {
	b := render.NewBuilder()

	// Name
	name, _, _ := unstructured.NestedString(obj.Object, "metadata", "name")
	b.KV(render.LEVEL_0, "Name", name)

	// Group
	group, _, _ := unstructured.NestedString(obj.Object, "spec", "group")
	b.KV(render.LEVEL_0, "Group", group)

	// Scope
	scope, _, _ := unstructured.NestedString(obj.Object, "spec", "scope")
	b.KV(render.LEVEL_0, "Scope", scope)

	// Versions
	b.Section(render.LEVEL_0, "Versions")
	versions, _, _ := unstructured.NestedSlice(obj.Object, "spec", "versions")
	if len(versions) > 0 {
		for _, v := range versions {
			vm, ok := v.(map[string]any)
			if !ok {
				continue
			}
			vName, _, _ := unstructured.NestedString(vm, "name")
			served, _, _ := unstructured.NestedBool(vm, "served")
			storage, _, _ := unstructured.NestedBool(vm, "storage")
			b.RawLine(render.LEVEL_1, fmt.Sprintf("%s  Served: %t  Storage: %t", vName, served, storage))
		}
	} else {
		b.RawLine(render.LEVEL_1, "<none>")
	}

	// Names
	b.Section(render.LEVEL_0, "Names")
	plural, _, _ := unstructured.NestedString(obj.Object, "spec", "names", "plural")
	singular, _, _ := unstructured.NestedString(obj.Object, "spec", "names", "singular")
	kind, _, _ := unstructured.NestedString(obj.Object, "spec", "names", "kind")
	shortNames, _, _ := unstructured.NestedStringSlice(obj.Object, "spec", "names", "shortNames")

	b.KV(render.LEVEL_1, "Plural", plural)
	b.KV(render.LEVEL_1, "Singular", singular)
	b.KV(render.LEVEL_1, "Kind", kind)
	b.KV(render.LEVEL_1, "Short Names", strings.Join(shortNames, ","))

	// Conditions
	b.Section(render.LEVEL_0, "Conditions")
	conditions, _, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if len(conditions) > 0 {
		for i, c := range conditions {
			cm, ok := c.(map[string]any)
			if !ok {
				continue
			}
			if i > 0 {
				b.RawLine(render.LEVEL_1, "")
			}
			cType, _, _ := unstructured.NestedString(cm, "type")
			cStatus, _, _ := unstructured.NestedString(cm, "status")
			cReason, _, _ := unstructured.NestedString(cm, "reason")
			cMessage, _, _ := unstructured.NestedString(cm, "message")

			b.KV(render.LEVEL_1, "Type", cType)
			b.KVStyled(render.LEVEL_1, render.ConditionKind(cStatus), "Status", cStatus)
			b.KV(render.LEVEL_1, "Reason", cReason)
			b.KV(render.LEVEL_1, "Message", cMessage)
		}
	} else {
		b.RawLine(render.LEVEL_1, "<none>")
	}

	return b.Build(), nil
}

