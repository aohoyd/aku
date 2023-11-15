package clusterroles

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

var gvr = schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterroles"}

// Plugin implements plugin.ResourcePlugin and plugin.DrillDowner for Kubernetes ClusterRoles.
type Plugin struct {
	store *k8s.Store
}

// New creates a new ClusterRoles plugin.
func New(_ *k8s.Client, store *k8s.Store) plugin.ResourcePlugin {
	return &Plugin{
		store: store,
	}
}

func (p *Plugin) Name() string                     { return "clusterroles" }
func (p *Plugin) ShortName() string                { return "clusterrole" }
func (p *Plugin) GVR() schema.GroupVersionResource { return gvr }
func (p *Plugin) IsClusterScoped() bool            { return true }

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
	cr, err := toClusterRole(obj)
	if err != nil {
		return render.Content{}, fmt.Errorf("converting to ClusterRole: %w", err)
	}

	b := render.NewBuilder()

	// Metadata
	b.KV(render.LEVEL_0, "Name", cr.Name)
	b.KVMulti(render.LEVEL_0, "Labels", cr.Labels)
	b.KVMulti(render.LEVEL_0, "Annotations", cr.Annotations)

	// AggregationRule
	if cr.AggregationRule != nil && len(cr.AggregationRule.ClusterRoleSelectors) > 0 {
		b.Section(render.LEVEL_0, "ClusterRoleSelectors")
		for _, sel := range cr.AggregationRule.ClusterRoleSelectors {
			var parts []string
			for k, v := range sel.MatchLabels {
				parts = append(parts, k+"="+v)
			}
			if len(parts) > 0 {
				b.KV(render.LEVEL_1, "MatchLabels", strings.Join(parts, ", "))
			}
			for _, expr := range sel.MatchExpressions {
				b.KV(render.LEVEL_1, "MatchExpression", fmt.Sprintf("%s %s [%s]", expr.Key, expr.Operator, strings.Join(expr.Values, ", ")))
			}
		}
	}

	// Rules
	if len(cr.Rules) > 0 {
		b.Section(render.LEVEL_0, "Rules")
		for _, rule := range cr.Rules {
			b.KV(render.LEVEL_1, "APIGroups", strings.Join(rule.APIGroups, ", "))
			b.KV(render.LEVEL_1, "Resources", strings.Join(rule.Resources, ", "))
			b.KV(render.LEVEL_1, "Verbs", strings.Join(rule.Verbs, ", "))
			b.RawLine(render.LEVEL_1, "---")
		}
	}

	return b.Build(), nil
}

// DrillDown implements plugin.DrillDowner.
func (p *Plugin) DrillDown(obj *unstructured.Unstructured) (plugin.ResourcePlugin, []*unstructured.Unstructured) {
	if p.store == nil {
		return nil, nil
	}
	pp, ok := plugin.ByName("clusterrolebindings")
	if !ok {
		return nil, nil
	}
	p.store.Subscribe(workload.ClusterRoleBindingsGVR, "")
	name := obj.GetName()
	bindings := workload.FindClusterRoleBindingsByRoleRef(p.store, name, "ClusterRole")
	return pp, bindings
}

// toClusterRole converts an unstructured object to a typed rbacv1.ClusterRole.
func toClusterRole(obj *unstructured.Unstructured) (*rbacv1.ClusterRole, error) {
	var cr rbacv1.ClusterRole
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &cr); err != nil {
		return nil, err
	}
	return &cr, nil
}
