package clusterrolebindings

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

var gvr = schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterrolebindings"}

// Plugin implements plugin.ResourcePlugin for Kubernetes ClusterRoleBindings.
type Plugin struct{}

// New creates a new ClusterRoleBindings plugin.
func New(_ *k8s.Client, _ *k8s.Store) plugin.ResourcePlugin {
	return &Plugin{}
}

func (p *Plugin) Name() string                     { return "clusterrolebindings" }
func (p *Plugin) ShortName() string                { return "clusterrolebinding" }
func (p *Plugin) GVR() schema.GroupVersionResource { return gvr }
func (p *Plugin) IsClusterScoped() bool            { return true }

func (p *Plugin) Columns() []plugin.Column {
	return []plugin.Column{
		{Title: "NAME", Flex: true},
		{Title: "ROLE", Width: 28},
		{Title: "AGE", Width: 8},
	}
}

func (p *Plugin) Row(obj *unstructured.Unstructured) []string {
	name := obj.GetName()

	roleKind, _, _ := unstructured.NestedString(obj.Object, "roleRef", "kind")
	roleName, _, _ := unstructured.NestedString(obj.Object, "roleRef", "name")
	role := roleKind + "/" + roleName
	if roleKind == "" && roleName == "" {
		role = "<none>"
	}

	age := render.FormatAge(obj)

	return []string{name, role, age}
}

func (p *Plugin) YAML(obj *unstructured.Unstructured) (render.Content, error) {
	return plugin.MarshalYAML(obj)
}

func (p *Plugin) Describe(ctx context.Context, obj *unstructured.Unstructured) (render.Content, error) {
	crb, err := toClusterRoleBinding(obj)
	if err != nil {
		return render.Content{}, fmt.Errorf("converting to ClusterRoleBinding: %w", err)
	}

	b := render.NewBuilder()

	// Metadata
	b.KV(render.LEVEL_0, "Name", crb.Name)
	b.KVMulti(render.LEVEL_0, "Labels", crb.Labels)
	b.KVMulti(render.LEVEL_0, "Annotations", crb.Annotations)

	// RoleRef
	b.Section(render.LEVEL_0, "RoleRef")
	b.KV(render.LEVEL_1, "Kind", crb.RoleRef.Kind)
	b.KV(render.LEVEL_1, "Name", crb.RoleRef.Name)
	b.KV(render.LEVEL_1, "API Group", crb.RoleRef.APIGroup)

	// Subjects
	b.Section(render.LEVEL_0, "Subjects")
	if len(crb.Subjects) > 0 {
		for _, subj := range crb.Subjects {
			b.KV(render.LEVEL_1, "Kind", subj.Kind)
			b.KV(render.LEVEL_1, "Name", subj.Name)
			if subj.Namespace != "" {
				b.KV(render.LEVEL_1, "Namespace", subj.Namespace)
			}
		}
	} else {
		b.RawLine(render.LEVEL_1, "<none>")
	}

	return b.Build(), nil
}

// toClusterRoleBinding converts an unstructured object to a typed rbacv1.ClusterRoleBinding.
func toClusterRoleBinding(obj *unstructured.Unstructured) (*rbacv1.ClusterRoleBinding, error) {
	var crb rbacv1.ClusterRoleBinding
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &crb); err != nil {
		return nil, err
	}
	return &crb, nil
}
