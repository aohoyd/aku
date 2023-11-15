package roles

import (
	"context"
	"fmt"
	"strings"

	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/plugins/workload"
	"github.com/aohoyd/aku/internal/render"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var gvr = schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "roles"}

// Plugin implements plugin.ResourcePlugin and plugin.DrillDowner for Kubernetes Roles.
type Plugin struct {
	store *k8s.Store
}

// New creates a new Roles plugin.
func New(_ *k8s.Client, store *k8s.Store) plugin.ResourcePlugin {
	return &Plugin{
		store: store,
	}
}

func (p *Plugin) Name() string                     { return "roles" }
func (p *Plugin) ShortName() string                { return "role" }
func (p *Plugin) GVR() schema.GroupVersionResource { return gvr }
func (p *Plugin) IsClusterScoped() bool            { return false }

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

func (p *Plugin) YAML(obj *unstructured.Unstructured) (render.Content, error) {
	return plugin.MarshalYAML(obj)
}

func (p *Plugin) Describe(ctx context.Context, obj *unstructured.Unstructured) (render.Content, error) {
	role, err := toRole(obj)
	if err != nil {
		return render.Content{}, fmt.Errorf("converting to Role: %w", err)
	}

	b := render.NewBuilder()

	// Metadata
	b.KV(render.LEVEL_0, "Name", role.Name)
	b.KV(render.LEVEL_0, "Namespace", role.Namespace)
	b.KVMulti(render.LEVEL_0, "Labels", role.Labels)
	b.KVMulti(render.LEVEL_0, "Annotations", role.Annotations)

	// Rules
	if len(role.Rules) > 0 {
		b.Section(render.LEVEL_0, "Rules")
		for _, rule := range role.Rules {
			b.KV(render.LEVEL_1, "APIGroups", strings.Join(rule.APIGroups, ","))
			b.KV(render.LEVEL_1, "Resources", strings.Join(rule.Resources, ","))
			b.KV(render.LEVEL_1, "Verbs", strings.Join(rule.Verbs, ","))
			if len(rule.ResourceNames) > 0 {
				b.KV(render.LEVEL_1, "ResourceNames", strings.Join(rule.ResourceNames, ","))
			}
			if len(rule.NonResourceURLs) > 0 {
				b.KV(render.LEVEL_1, "NonResourceURLs", strings.Join(rule.NonResourceURLs, ","))
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
	pp, ok := plugin.ByName("rolebindings")
	if !ok {
		return nil, nil
	}
	namespace := obj.GetNamespace()
	name := obj.GetName()
	p.store.Subscribe(workload.RoleBindingsGVR, namespace)
	bindings := workload.FindRoleBindingsByRoleRef(p.store, namespace, name, "Role")
	return pp, bindings
}

// toRole converts an unstructured object to a typed rbacv1.Role.
func toRole(obj *unstructured.Unstructured) (*rbacv1.Role, error) {
	var role rbacv1.Role
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &role); err != nil {
		return nil, err
	}
	return &role, nil
}
