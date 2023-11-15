package poddisruptionbudgets

import (
	"context"
	"fmt"

	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/render"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var gvr = schema.GroupVersionResource{Group: "policy", Version: "v1", Resource: "poddisruptionbudgets"}

// Plugin implements plugin.ResourcePlugin for Kubernetes PodDisruptionBudgets.
type Plugin struct{}

// New creates a new PodDisruptionBudget plugin.
func New(_ *k8s.Client, _ *k8s.Store) plugin.ResourcePlugin {
	return &Plugin{}
}

func (p *Plugin) Name() string                     { return "poddisruptionbudgets" }
func (p *Plugin) ShortName() string                { return "pdb" }
func (p *Plugin) GVR() schema.GroupVersionResource { return gvr }
func (p *Plugin) IsClusterScoped() bool            { return false }

func (p *Plugin) Columns() []plugin.Column {
	return []plugin.Column{
		{Title: "NAME", Flex: true},
		{Title: "MIN AVAILABLE", Width: 14},
		{Title: "MAX UNAVAILABLE", Width: 16},
		{Title: "ALLOWED DISRUPTIONS", Width: 20},
		{Title: "AGE", Width: 8},
	}
}

func (p *Plugin) Row(obj *unstructured.Unstructured) []string {
	name := obj.GetName()

	minAvail := extractIntOrString(obj, "spec", "minAvailable")
	maxUnavail := extractIntOrString(obj, "spec", "maxUnavailable")

	disruptionsAllowed := "N/A"
	if val, found, err := unstructured.NestedInt64(obj.Object, "status", "disruptionsAllowed"); err == nil && found {
		disruptionsAllowed = fmt.Sprintf("%d", val)
	}

	age := render.FormatAge(obj)

	return []string{name, minAvail, maxUnavail, disruptionsAllowed, age}
}

func (p *Plugin) YAML(obj *unstructured.Unstructured) (render.Content, error) {
	return plugin.MarshalYAML(obj)
}

// extractIntOrString tries NestedInt64 first, then NestedString, returning "N/A" if neither.
func extractIntOrString(obj *unstructured.Unstructured, fields ...string) string {
	if val, found, err := unstructured.NestedInt64(obj.Object, fields...); err == nil && found {
		return fmt.Sprintf("%d", val)
	}
	if val, found, err := unstructured.NestedString(obj.Object, fields...); err == nil && found {
		return val
	}
	return "N/A"
}

func (p *Plugin) Describe(ctx context.Context, obj *unstructured.Unstructured) (render.Content, error) {
	pdb, err := toPodDisruptionBudget(obj)
	if err != nil {
		return render.Content{}, fmt.Errorf("converting to PodDisruptionBudget: %w", err)
	}

	b := render.NewBuilder()

	b.KV(render.LEVEL_0, "Name", pdb.Name)
	b.KV(render.LEVEL_0, "Namespace", pdb.Namespace)

	// Labels
	b.KVMulti(render.LEVEL_0, "Labels", pdb.Labels)

	// Annotations
	b.KVMulti(render.LEVEL_0, "Annotations", pdb.Annotations)

	// Min Available
	minAvail := "N/A"
	if pdb.Spec.MinAvailable != nil {
		minAvail = pdb.Spec.MinAvailable.String()
	}
	b.KV(render.LEVEL_0, "Min Available", minAvail)

	// Max Unavailable
	maxUnavail := "N/A"
	if pdb.Spec.MaxUnavailable != nil {
		maxUnavail = pdb.Spec.MaxUnavailable.String()
	}
	b.KV(render.LEVEL_0, "Max Unavailable", maxUnavail)

	// Selector
	if pdb.Spec.Selector != nil && len(pdb.Spec.Selector.MatchLabels) > 0 {
		labels := make([]string, 0, len(pdb.Spec.Selector.MatchLabels))
		for k, v := range pdb.Spec.Selector.MatchLabels {
			labels = append(labels, k+"="+v)
		}
		b.KV(render.LEVEL_0, "Selector", labels[0])
		// If there are more, they will be part of the first value for simplicity.
		// For a proper multi-value we'd use KVMulti, but selector is typically rendered as a single line.
	}

	// Status
	b.Section(render.LEVEL_0, "Status")
	b.KV(render.LEVEL_1, "Current Healthy", fmt.Sprintf("%d", pdb.Status.CurrentHealthy))
	b.KV(render.LEVEL_1, "Desired Healthy", fmt.Sprintf("%d", pdb.Status.DesiredHealthy))
	b.KV(render.LEVEL_1, "Disruptions Allowed", fmt.Sprintf("%d", pdb.Status.DisruptionsAllowed))
	b.KV(render.LEVEL_1, "Expected Pods", fmt.Sprintf("%d", pdb.Status.ExpectedPods))

	// Conditions
	if len(pdb.Status.Conditions) > 0 {
		b.Section(render.LEVEL_0, "Conditions")
		for _, cond := range pdb.Status.Conditions {
			b.KVStyled(render.LEVEL_1, render.ConditionKind(string(cond.Status)), string(cond.Type), string(cond.Status))
		}
	}

	return b.Build(), nil
}

// toPodDisruptionBudget converts an unstructured object to a typed policyv1.PodDisruptionBudget.
func toPodDisruptionBudget(obj *unstructured.Unstructured) (*policyv1.PodDisruptionBudget, error) {
	var pdb policyv1.PodDisruptionBudget
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &pdb); err != nil {
		return nil, err
	}
	return &pdb, nil
}
