package networkpolicies

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/render"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var gvr = schema.GroupVersionResource{Group: "networking.k8s.io", Version: "v1", Resource: "networkpolicies"}

// Plugin implements plugin.ResourcePlugin for Kubernetes NetworkPolicies.
type Plugin struct{}

// New creates a new NetworkPolicy plugin.
func New(_ *k8s.Client, _ *k8s.Store) plugin.ResourcePlugin {
	return &Plugin{}
}

func (p *Plugin) Name() string                     { return "networkpolicies" }
func (p *Plugin) ShortName() string                { return "netpol" }
func (p *Plugin) GVR() schema.GroupVersionResource { return gvr }
func (p *Plugin) IsClusterScoped() bool            { return false }

func (p *Plugin) Columns() []plugin.Column {
	return []plugin.Column{
		{Title: "NAME", Flex: true},
		{Title: "POD-SELECTOR", Flex: true},
		{Title: "AGE", Width: 8},
	}
}

func (p *Plugin) Row(obj *unstructured.Unstructured) []string {
	name := obj.GetName()
	podSelector := formatPodSelector(obj)
	age := render.FormatAge(obj)
	return []string{name, podSelector, age}
}

func (p *Plugin) YAML(obj *unstructured.Unstructured) (render.Content, error) {
	return plugin.MarshalYAML(obj)
}

// formatPodSelector extracts spec.podSelector.matchLabels and formats them
// as sorted key=val pairs separated by commas, or "<none>" if empty.
func formatPodSelector(obj *unstructured.Unstructured) string {
	labels, found, _ := unstructured.NestedStringMap(obj.Object, "spec", "podSelector", "matchLabels")
	if !found || len(labels) == 0 {
		return "<none>"
	}

	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	slices.Sort(keys)

	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + "=" + labels[k]
	}
	return strings.Join(parts, ",")
}

func (p *Plugin) Describe(ctx context.Context, obj *unstructured.Unstructured) (render.Content, error) {
	np, err := toNetworkPolicy(obj)
	if err != nil {
		return render.Content{}, fmt.Errorf("networkpolicies: decode: %w", err)
	}

	b := render.NewBuilder()

	// Basic metadata
	b.KV(render.LEVEL_0, "Name", np.Name)
	b.KV(render.LEVEL_0, "Namespace", np.Namespace)

	// Labels
	b.KVMulti(render.LEVEL_0, "Labels", np.Labels)

	// Annotations
	b.KVMulti(render.LEVEL_0, "Annotations", np.Annotations)

	// Pod Selector
	if len(np.Spec.PodSelector.MatchLabels) > 0 {
		b.KVMulti(render.LEVEL_0, "Pod Selector", np.Spec.PodSelector.MatchLabels)
	} else {
		b.KV(render.LEVEL_0, "Pod Selector", "<none>")
	}

	// Policy Types
	if len(np.Spec.PolicyTypes) > 0 {
		types := make([]string, len(np.Spec.PolicyTypes))
		for i, pt := range np.Spec.PolicyTypes {
			types[i] = string(pt)
		}
		b.KV(render.LEVEL_0, "Policy Types", strings.Join(types, ", "))
	} else {
		b.KV(render.LEVEL_0, "Policy Types", "<none>")
	}

	// Ingress Rules
	b.Section(render.LEVEL_0, "Ingress Rules")
	if len(np.Spec.Ingress) > 0 {
		for i, rule := range np.Spec.Ingress {
			b.RawLine(render.LEVEL_1, fmt.Sprintf("Rule %d:", i+1))
			describeIngressRule(b, rule)
		}
	} else {
		b.RawLine(render.LEVEL_1, "<none>")
	}

	// Egress Rules
	b.Section(render.LEVEL_0, "Egress Rules")
	if len(np.Spec.Egress) > 0 {
		for i, rule := range np.Spec.Egress {
			b.RawLine(render.LEVEL_1, fmt.Sprintf("Rule %d:", i+1))
			describeEgressRule(b, rule)
		}
	} else {
		b.RawLine(render.LEVEL_1, "<none>")
	}

	return b.Build(), nil
}

func describeIngressRule(b *render.Builder, rule networkingv1.NetworkPolicyIngressRule) {
	// Ports
	if len(rule.Ports) > 0 {
		b.KV(render.LEVEL_2, "Ports", formatPolicyPorts(rule.Ports))
	}

	// From
	if len(rule.From) > 0 {
		b.Section(render.LEVEL_2, "From")
		for _, peer := range rule.From {
			describePeer(b, render.LEVEL_3, peer)
		}
	}
}

func describeEgressRule(b *render.Builder, rule networkingv1.NetworkPolicyEgressRule) {
	// Ports
	if len(rule.Ports) > 0 {
		b.KV(render.LEVEL_2, "Ports", formatPolicyPorts(rule.Ports))
	}

	// To
	if len(rule.To) > 0 {
		b.Section(render.LEVEL_2, "To")
		for _, peer := range rule.To {
			describePeer(b, render.LEVEL_3, peer)
		}
	}
}

func describePeer(b *render.Builder, level int, peer networkingv1.NetworkPolicyPeer) {
	if peer.PodSelector != nil && len(peer.PodSelector.MatchLabels) > 0 {
		b.KVMulti(level, "Pod Selector", peer.PodSelector.MatchLabels)
	}
	if peer.NamespaceSelector != nil && len(peer.NamespaceSelector.MatchLabels) > 0 {
		b.KVMulti(level, "Namespace Selector", peer.NamespaceSelector.MatchLabels)
	}
	if peer.IPBlock != nil {
		b.KV(level, "IP Block", peer.IPBlock.CIDR)
		if len(peer.IPBlock.Except) > 0 {
			b.KV(level, "Except", strings.Join(peer.IPBlock.Except, ", "))
		}
	}
}

func formatPolicyPorts(ports []networkingv1.NetworkPolicyPort) string {
	parts := make([]string, 0, len(ports))
	for _, port := range ports {
		protocol := "TCP"
		if port.Protocol != nil {
			protocol = string(*port.Protocol)
		}
		if port.Port != nil {
			parts = append(parts, fmt.Sprintf("%s/%s", port.Port.String(), protocol))
		} else {
			parts = append(parts, protocol)
		}
	}
	return strings.Join(parts, ", ")
}

// toNetworkPolicy converts an unstructured object to a typed networkingv1.NetworkPolicy.
func toNetworkPolicy(obj *unstructured.Unstructured) (*networkingv1.NetworkPolicy, error) {
	var np networkingv1.NetworkPolicy
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &np); err != nil {
		return nil, err
	}
	return &np, nil
}
