package validatingadmissionpolicybindings

import (
	"context"
	"fmt"
	"strings"

	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/render"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var gvr = schema.GroupVersionResource{Group: "admissionregistration.k8s.io", Version: "v1", Resource: "validatingadmissionpolicybindings"}

// Plugin implements plugin.ResourcePlugin for Kubernetes ValidatingAdmissionPolicyBindings.
type Plugin struct{}

// New creates a new ValidatingAdmissionPolicyBindings plugin.
func New(_ *k8s.Client, _ *k8s.Store) plugin.ResourcePlugin {
	return &Plugin{}
}

func (p *Plugin) Name() string                     { return "validatingadmissionpolicybindings" }
func (p *Plugin) ShortName() string                { return "vapb" }
func (p *Plugin) GVR() schema.GroupVersionResource { return gvr }
func (p *Plugin) IsClusterScoped() bool            { return true }

func (p *Plugin) Columns() []plugin.Column {
	return []plugin.Column{
		{Title: "NAME", Flex: true},
		{Title: "POLICY", Flex: true},
		{Title: "AGE", Width: 8},
	}
}

func (p *Plugin) Row(obj *unstructured.Unstructured) []string {
	name := obj.GetName()

	policyName, _, _ := unstructured.NestedString(obj.Object, "spec", "policyName")
	if policyName == "" {
		policyName = "<none>"
	}

	age := render.FormatAge(obj)

	return []string{name, policyName, age}
}

func (p *Plugin) YAML(obj *unstructured.Unstructured) (render.Content, error) {
	return plugin.MarshalYAML(obj)
}

func (p *Plugin) Describe(ctx context.Context, obj *unstructured.Unstructured) (render.Content, error) {
	vapb, err := toValidatingAdmissionPolicyBinding(obj)
	if err != nil {
		return render.Content{}, fmt.Errorf("converting to ValidatingAdmissionPolicyBinding: %w", err)
	}

	b := render.NewBuilder()

	// Metadata
	b.KV(render.LEVEL_0, "Name", vapb.Name)
	b.KVMulti(render.LEVEL_0, "Labels", vapb.Labels)
	b.KVMulti(render.LEVEL_0, "Annotations", vapb.Annotations)

	// Policy Name
	b.KV(render.LEVEL_0, "Policy Name", vapb.Spec.PolicyName)

	// Validation Actions
	actions := make([]string, len(vapb.Spec.ValidationActions))
	for i, a := range vapb.Spec.ValidationActions {
		actions[i] = string(a)
	}
	b.KV(render.LEVEL_0, "Validation Actions", strings.Join(actions, ", "))

	// Param Ref
	if vapb.Spec.ParamRef != nil {
		ref := vapb.Spec.ParamRef
		b.Section(render.LEVEL_0, "Param Ref")
		b.KV(render.LEVEL_1, "Name", ref.Name)
		b.KV(render.LEVEL_1, "Namespace", ref.Namespace)
		if ref.Selector != nil {
			describeLabelSelector(b, render.LEVEL_1, "Selector", ref.Selector)
		}
		if ref.ParameterNotFoundAction != nil {
			b.KV(render.LEVEL_1, "ParameterNotFoundAction", string(*ref.ParameterNotFoundAction))
		}
	}

	// Match Resources
	if vapb.Spec.MatchResources != nil {
		mr := vapb.Spec.MatchResources
		b.Section(render.LEVEL_0, "Match Resources")
		if mr.MatchPolicy != nil {
			b.KV(render.LEVEL_1, "Match Policy", string(*mr.MatchPolicy))
		}
		if len(mr.ResourceRules) > 0 {
			b.Section(render.LEVEL_1, "Resource Rules")
			for _, rule := range mr.ResourceRules {
				describeRuleWithOperations(b, render.LEVEL_2, rule.RuleWithOperations)
			}
		}
		if mr.NamespaceSelector != nil {
			describeLabelSelector(b, render.LEVEL_1, "Namespace Selector", mr.NamespaceSelector)
		}
		if mr.ObjectSelector != nil {
			describeLabelSelector(b, render.LEVEL_1, "Object Selector", mr.ObjectSelector)
		}
	}

	return b.Build(), nil
}

// describeLabelSelector renders a LabelSelector into the builder.
func describeLabelSelector(b *render.Builder, level int, title string, sel *metav1.LabelSelector) {
	if sel == nil {
		return
	}
	b.Section(level, title)
	if len(sel.MatchLabels) > 0 {
		b.KVMulti(level+1, "Match Labels", sel.MatchLabels)
	}
	for _, expr := range sel.MatchExpressions {
		b.KV(level+1, "Match Expression", fmt.Sprintf("%s %s [%s]", expr.Key, expr.Operator, strings.Join(expr.Values, ", ")))
	}
}

// describeRuleWithOperations renders a RuleWithOperations into the builder.
func describeRuleWithOperations(b *render.Builder, level int, rule admissionregistrationv1.RuleWithOperations) {
	ops := make([]string, len(rule.Operations))
	for i, op := range rule.Operations {
		ops[i] = string(op)
	}
	b.KV(level, "Operations", strings.Join(ops, ", "))
	b.KV(level, "API Groups", strings.Join(rule.APIGroups, ", "))
	b.KV(level, "API Versions", strings.Join(rule.APIVersions, ", "))
	b.KV(level, "Resources", strings.Join(rule.Resources, ", "))
}

// toValidatingAdmissionPolicyBinding converts an unstructured object to a typed admissionregistrationv1.ValidatingAdmissionPolicyBinding.
func toValidatingAdmissionPolicyBinding(obj *unstructured.Unstructured) (*admissionregistrationv1.ValidatingAdmissionPolicyBinding, error) {
	var vapb admissionregistrationv1.ValidatingAdmissionPolicyBinding
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &vapb); err != nil {
		return nil, err
	}
	return &vapb, nil
}
