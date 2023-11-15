package validatingwebhookconfigurations

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

var gvr = schema.GroupVersionResource{Group: "admissionregistration.k8s.io", Version: "v1", Resource: "validatingwebhookconfigurations"}

// Plugin implements plugin.ResourcePlugin for Kubernetes ValidatingWebhookConfigurations.
type Plugin struct{}

// New creates a new ValidatingWebhookConfigurations plugin.
func New(_ *k8s.Client, _ *k8s.Store) plugin.ResourcePlugin {
	return &Plugin{}
}

func (p *Plugin) Name() string                     { return "validatingwebhookconfigurations" }
func (p *Plugin) ShortName() string                { return "vwc" }
func (p *Plugin) GVR() schema.GroupVersionResource { return gvr }
func (p *Plugin) IsClusterScoped() bool            { return true }

func (p *Plugin) Columns() []plugin.Column {
	return []plugin.Column{
		{Title: "NAME", Flex: true},
		{Title: "WEBHOOKS", Width: 10},
		{Title: "FAILURE-POLICY", Width: 16},
		{Title: "AGE", Width: 8},
	}
}

func (p *Plugin) Row(obj *unstructured.Unstructured) []string {
	name := obj.GetName()
	webhooks, _, _ := unstructured.NestedSlice(obj.Object, "webhooks")
	count := strconv.Itoa(len(webhooks))
	failurePolicy := "Fail"
	if len(webhooks) > 0 {
		if wh, ok := webhooks[0].(map[string]any); ok {
			if fp, ok := wh["failurePolicy"].(string); ok && fp != "" {
				failurePolicy = fp
			}
		}
	}
	age := render.FormatAge(obj)
	return []string{name, count, failurePolicy, age}
}

func (p *Plugin) YAML(obj *unstructured.Unstructured) (render.Content, error) {
	return plugin.MarshalYAML(obj)
}

func (p *Plugin) Describe(ctx context.Context, obj *unstructured.Unstructured) (render.Content, error) {
	vwc, err := toValidatingWebhookConfiguration(obj)
	if err != nil {
		return render.Content{}, fmt.Errorf("converting to ValidatingWebhookConfiguration: %w", err)
	}

	b := render.NewBuilder()

	// Metadata
	b.KV(render.LEVEL_0, "Name", vwc.Name)
	b.KVMulti(render.LEVEL_0, "Labels", vwc.Labels)
	b.KVMulti(render.LEVEL_0, "Annotations", vwc.Annotations)

	// Webhooks
	if len(vwc.Webhooks) > 0 {
		b.Section(render.LEVEL_0, "Webhooks")
		for _, wh := range vwc.Webhooks {
			b.Section(render.LEVEL_1, wh.Name)

			fp := "Fail"
			if wh.FailurePolicy != nil {
				fp = string(*wh.FailurePolicy)
			}
			b.KV(render.LEVEL_2, "Failure Policy", fp)

			if wh.MatchPolicy != nil {
				b.KV(render.LEVEL_2, "Match Policy", string(*wh.MatchPolicy))
			}

			if wh.SideEffects != nil {
				b.KV(render.LEVEL_2, "Side Effects", string(*wh.SideEffects))
			}

			if wh.TimeoutSeconds != nil {
				b.KV(render.LEVEL_2, "Timeout", fmt.Sprintf("%ds", *wh.TimeoutSeconds))
			}

			if len(wh.AdmissionReviewVersions) > 0 {
				b.KV(render.LEVEL_2, "Admission Review Versions", strings.Join(wh.AdmissionReviewVersions, ", "))
			}

			// Client Config
			describeClientConfig(b, render.LEVEL_2, wh.ClientConfig)

			// Namespace Selector
			describeLabelSelector(b, render.LEVEL_2, "Namespace Selector", wh.NamespaceSelector)

			// Object Selector
			describeLabelSelector(b, render.LEVEL_2, "Object Selector", wh.ObjectSelector)

			// Rules
			if len(wh.Rules) > 0 {
				b.Section(render.LEVEL_2, "Rules")
				for _, rule := range wh.Rules {
					describeRuleWithOperations(b, render.LEVEL_3, rule)
				}
			}

			// Match Conditions
			if len(wh.MatchConditions) > 0 {
				b.Section(render.LEVEL_2, "Match Conditions")
				for _, mc := range wh.MatchConditions {
					b.KV(render.LEVEL_3, "Name", mc.Name)
					b.KV(render.LEVEL_3, "Expression", mc.Expression)
					b.RawLine(render.LEVEL_3, "---")
				}
			}
		}
	}

	return b.Build(), nil
}

// describeClientConfig renders a WebhookClientConfig into the builder.
func describeClientConfig(b *render.Builder, level int, cc admissionregistrationv1.WebhookClientConfig) {
	b.Section(level, "Client Config")
	if cc.URL != nil {
		b.KV(level+1, "URL", *cc.URL)
	}
	if cc.Service != nil {
		b.KV(level+1, "Service", cc.Service.Namespace+"/"+cc.Service.Name)
		if cc.Service.Path != nil {
			b.KV(level+1, "Path", *cc.Service.Path)
		}
		port := int32(443)
		if cc.Service.Port != nil {
			port = *cc.Service.Port
		}
		b.KV(level+1, "Port", fmt.Sprintf("%d", port))
	}
	if len(cc.CABundle) > 0 {
		b.KV(level+1, "CA Bundle", fmt.Sprintf("%d bytes", len(cc.CABundle)))
	}
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

// toValidatingWebhookConfiguration converts an unstructured object to a typed admissionregistrationv1.ValidatingWebhookConfiguration.
func toValidatingWebhookConfiguration(obj *unstructured.Unstructured) (*admissionregistrationv1.ValidatingWebhookConfiguration, error) {
	var vwc admissionregistrationv1.ValidatingWebhookConfiguration
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &vwc); err != nil {
		return nil, err
	}
	return &vwc, nil
}
