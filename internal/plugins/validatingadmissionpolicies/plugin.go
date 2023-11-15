package validatingadmissionpolicies

import (
	"context"
	"fmt"
	"strconv"
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

var gvr = schema.GroupVersionResource{Group: "admissionregistration.k8s.io", Version: "v1", Resource: "validatingadmissionpolicies"}

var vapbGVR = schema.GroupVersionResource{Group: "admissionregistration.k8s.io", Version: "v1", Resource: "validatingadmissionpolicybindings"}

// Plugin implements plugin.ResourcePlugin and plugin.DrillDowner for Kubernetes ValidatingAdmissionPolicies.
type Plugin struct {
	store *k8s.Store
}

// New creates a new ValidatingAdmissionPolicies plugin.
func New(_ *k8s.Client, store *k8s.Store) plugin.ResourcePlugin {
	return &Plugin{store: store}
}

func (p *Plugin) Name() string                     { return "validatingadmissionpolicies" }
func (p *Plugin) ShortName() string                { return "vap" }
func (p *Plugin) GVR() schema.GroupVersionResource { return gvr }
func (p *Plugin) IsClusterScoped() bool            { return true }

func (p *Plugin) Columns() []plugin.Column {
	return []plugin.Column{
		{Title: "NAME", Flex: true},
		{Title: "VALIDATIONS", Width: 13},
		{Title: "FAILURE-POLICY", Width: 16},
		{Title: "AGE", Width: 8},
	}
}

func (p *Plugin) Row(obj *unstructured.Unstructured) []string {
	name := obj.GetName()

	validations, _, _ := unstructured.NestedSlice(obj.Object, "spec", "validations")
	validationCount := strconv.Itoa(len(validations))

	failurePolicy, _, _ := unstructured.NestedString(obj.Object, "spec", "failurePolicy")
	if failurePolicy == "" {
		failurePolicy = "Fail"
	}

	age := render.FormatAge(obj)

	return []string{name, validationCount, failurePolicy, age}
}

func (p *Plugin) YAML(obj *unstructured.Unstructured) (render.Content, error) {
	return plugin.MarshalYAML(obj)
}

func (p *Plugin) Describe(ctx context.Context, obj *unstructured.Unstructured) (render.Content, error) {
	vap, err := toValidatingAdmissionPolicy(obj)
	if err != nil {
		return render.Content{}, fmt.Errorf("converting to ValidatingAdmissionPolicy: %w", err)
	}

	b := render.NewBuilder()

	// Metadata
	b.KV(render.LEVEL_0, "Name", vap.Name)
	b.KVMulti(render.LEVEL_0, "Labels", vap.Labels)
	b.KVMulti(render.LEVEL_0, "Annotations", vap.Annotations)

	// Failure Policy
	failurePolicy := "Fail"
	if vap.Spec.FailurePolicy != nil {
		failurePolicy = string(*vap.Spec.FailurePolicy)
	}
	b.KV(render.LEVEL_0, "Failure Policy", failurePolicy)

	// Param Kind
	if vap.Spec.ParamKind != nil {
		b.Section(render.LEVEL_0, "Param Kind")
		b.KV(render.LEVEL_1, "API Version", vap.Spec.ParamKind.APIVersion)
		b.KV(render.LEVEL_1, "Kind", vap.Spec.ParamKind.Kind)
	}

	// Match Constraints
	if vap.Spec.MatchConstraints != nil {
		mc := vap.Spec.MatchConstraints
		b.Section(render.LEVEL_0, "Match Constraints")
		if mc.MatchPolicy != nil {
			b.KV(render.LEVEL_1, "Match Policy", string(*mc.MatchPolicy))
		}
		if len(mc.ResourceRules) > 0 {
			b.Section(render.LEVEL_1, "Resource Rules")
			for _, rule := range mc.ResourceRules {
				describeRuleWithOperations(b, render.LEVEL_2, rule.RuleWithOperations)
			}
		}
		if mc.NamespaceSelector != nil {
			describeLabelSelector(b, render.LEVEL_1, "Namespace Selector", mc.NamespaceSelector)
		}
		if mc.ObjectSelector != nil {
			describeLabelSelector(b, render.LEVEL_1, "Object Selector", mc.ObjectSelector)
		}
	}

	// Validations
	if len(vap.Spec.Validations) > 0 {
		b.Section(render.LEVEL_0, "Validations")
		for _, v := range vap.Spec.Validations {
			b.KV(render.LEVEL_1, "Expression", v.Expression)
			if v.Message != "" {
				b.KV(render.LEVEL_1, "Message", v.Message)
			}
			if v.MessageExpression != "" {
				b.KV(render.LEVEL_1, "Message Expression", v.MessageExpression)
			}
			if v.Reason != nil {
				b.KV(render.LEVEL_1, "Reason", string(*v.Reason))
			}
			b.RawLine(render.LEVEL_1, "---")
		}
	}

	// Match Conditions
	if len(vap.Spec.MatchConditions) > 0 {
		b.Section(render.LEVEL_0, "Match Conditions")
		for _, mc := range vap.Spec.MatchConditions {
			b.KV(render.LEVEL_1, "Name", mc.Name)
			b.KV(render.LEVEL_1, "Expression", mc.Expression)
			b.RawLine(render.LEVEL_1, "---")
		}
	}

	// Audit Annotations
	if len(vap.Spec.AuditAnnotations) > 0 {
		b.Section(render.LEVEL_0, "Audit Annotations")
		for _, aa := range vap.Spec.AuditAnnotations {
			b.KV(render.LEVEL_1, "Key", aa.Key)
			b.KV(render.LEVEL_1, "Value Expression", aa.ValueExpression)
			b.RawLine(render.LEVEL_1, "---")
		}
	}

	// Variables
	if len(vap.Spec.Variables) > 0 {
		b.Section(render.LEVEL_0, "Variables")
		for _, v := range vap.Spec.Variables {
			b.KV(render.LEVEL_1, "Name", v.Name)
			b.KV(render.LEVEL_1, "Expression", v.Expression)
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
	pp, ok := plugin.ByName("validatingadmissionpolicybindings")
	if !ok {
		return nil, nil
	}
	p.store.Subscribe(vapbGVR, "")
	name := obj.GetName()
	all := p.store.List(vapbGVR, "")
	var matched []*unstructured.Unstructured
	for _, o := range all {
		policyName, _, _ := unstructured.NestedString(o.Object, "spec", "policyName")
		if policyName == name {
			matched = append(matched, o)
		}
	}
	return pp, matched
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
	if len(sel.MatchLabels) == 0 && len(sel.MatchExpressions) == 0 {
		b.RawLine(level+1, "<none>")
	}
}

// describeRuleWithOperations renders a RuleWithOperations into the builder.
func describeRuleWithOperations(b *render.Builder, level int, rule admissionregistrationv1.RuleWithOperations) {
	ops := make([]string, len(rule.Operations))
	for i, op := range rule.Operations {
		ops[i] = string(op)
	}
	b.KV(level, "Operations", strings.Join(ops, ", "))
	if len(rule.APIGroups) > 0 {
		b.KV(level, "API Groups", strings.Join(rule.APIGroups, ", "))
	}
	if len(rule.APIVersions) > 0 {
		b.KV(level, "API Versions", strings.Join(rule.APIVersions, ", "))
	}
	if len(rule.Resources) > 0 {
		b.KV(level, "Resources", strings.Join(rule.Resources, ", "))
	}
	if rule.Scope != nil {
		b.KV(level, "Scope", string(*rule.Scope))
	}
}

// toValidatingAdmissionPolicy converts an unstructured object to a typed admissionregistrationv1.ValidatingAdmissionPolicy.
func toValidatingAdmissionPolicy(obj *unstructured.Unstructured) (*admissionregistrationv1.ValidatingAdmissionPolicy, error) {
	var vap admissionregistrationv1.ValidatingAdmissionPolicy
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &vap); err != nil {
		return nil, err
	}
	return &vap, nil
}
