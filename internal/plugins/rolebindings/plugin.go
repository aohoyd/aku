package rolebindings

import (
	"context"
	"fmt"

	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/render"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var gvr = schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "rolebindings"}

// Plugin implements plugin.ResourcePlugin for Kubernetes RoleBindings.
type Plugin struct{}

// New creates a new RoleBindings plugin.
func New(_ *k8s.Client, _ *k8s.Store) plugin.ResourcePlugin {
	return &Plugin{}
}

func (p *Plugin) Name() string                     { return "rolebindings" }
func (p *Plugin) ShortName() string                { return "rolebinding" }
func (p *Plugin) GVR() schema.GroupVersionResource { return gvr }
func (p *Plugin) IsClusterScoped() bool            { return false }

func (p *Plugin) Columns() []plugin.Column {
	return []plugin.Column{
		{Title: "NAME", Flex: true},
		{Title: "ROLE", Width: 24},
		{Title: "AGE", Width: 8},
	}
}

func (p *Plugin) Row(obj *unstructured.Unstructured) []string {
	name := obj.GetName()
	role := extractRole(obj)
	age := render.FormatAge(obj)

	return []string{name, role, age}
}

func (p *Plugin) YAML(obj *unstructured.Unstructured) (render.Content, error) {
	return plugin.MarshalYAML(obj)
}

func (p *Plugin) Describe(ctx context.Context, obj *unstructured.Unstructured) (render.Content, error) {
	rb, err := toRoleBinding(obj)
	if err != nil {
		return render.Content{}, fmt.Errorf("converting to RoleBinding: %w", err)
	}

	b := render.NewBuilder()

	b.KV(render.LEVEL_0, "Name", rb.Name)
	b.KV(render.LEVEL_0, "Namespace", rb.Namespace)

	// Labels
	b.KVMulti(render.LEVEL_0, "Labels", rb.Labels)

	// Annotations
	b.KVMulti(render.LEVEL_0, "Annotations", rb.Annotations)

	// RoleRef
	b.Section(render.LEVEL_0, "RoleRef")
	b.KV(render.LEVEL_1, "Kind", rb.RoleRef.Kind)
	b.KV(render.LEVEL_1, "Name", rb.RoleRef.Name)
	b.KV(render.LEVEL_1, "API Group", rb.RoleRef.APIGroup)

	// Subjects
	b.Section(render.LEVEL_0, "Subjects")
	if len(rb.Subjects) > 0 {
		for i, subject := range rb.Subjects {
			if i > 0 {
				b.RawLine(render.LEVEL_1, "")
			}
			b.KV(render.LEVEL_1, "Kind", subject.Kind)
			b.KV(render.LEVEL_1, "Name", subject.Name)
			if subject.Namespace != "" {
				b.KV(render.LEVEL_1, "Namespace", subject.Namespace)
			}
		}
	} else {
		b.RawLine(render.LEVEL_1, "<none>")
	}

	return b.Build(), nil
}

// toRoleBinding converts an unstructured object to a typed rbacv1.RoleBinding.
func toRoleBinding(obj *unstructured.Unstructured) (*rbacv1.RoleBinding, error) {
	var rb rbacv1.RoleBinding
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &rb); err != nil {
		return nil, err
	}
	return &rb, nil
}

// extractRole builds the "kind/name" string from unstructured roleRef.
func extractRole(obj *unstructured.Unstructured) string {
	kind, _, _ := unstructured.NestedString(obj.Object, "roleRef", "kind")
	name, _, _ := unstructured.NestedString(obj.Object, "roleRef", "name")
	if kind == "" && name == "" {
		return "<none>"
	}
	return kind + "/" + name
}
