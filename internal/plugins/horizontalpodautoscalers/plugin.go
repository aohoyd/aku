package horizontalpodautoscalers

import (
	"context"
	"fmt"

	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/plugins/workload"
	"github.com/aohoyd/aku/internal/render"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var gvr = schema.GroupVersionResource{Group: "autoscaling", Version: "v2", Resource: "horizontalpodautoscalers"}

// Plugin implements plugin.ResourcePlugin for Kubernetes HorizontalPodAutoscalers.
type Plugin struct{}

// New creates a new HorizontalPodAutoscaler plugin.
func New(_ *k8s.Client, _ *k8s.Store) plugin.ResourcePlugin {
	return &Plugin{}
}

func (p *Plugin) Name() string                     { return "horizontalpodautoscalers" }
func (p *Plugin) ShortName() string                { return "hpa" }
func (p *Plugin) GVR() schema.GroupVersionResource { return gvr }
func (p *Plugin) IsClusterScoped() bool            { return false }

func (p *Plugin) Columns() []plugin.Column {
	return []plugin.Column{
		{Title: "NAME", Flex: true},
		{Title: "REFERENCE", Width: 24},
		{Title: "TARGETS", Flex: true},
		{Title: "MINPODS", Width: 8},
		{Title: "MAXPODS", Width: 8},
		{Title: "REPLICAS", Width: 8},
		{Title: "AGE", Width: 8},
	}
}

func (p *Plugin) Row(obj *unstructured.Unstructured) []string {
	name := obj.GetName()

	// Reference: spec.scaleTargetRef.kind/name
	refKind, _, _ := unstructured.NestedString(obj.Object, "spec", "scaleTargetRef", "kind")
	refName, _, _ := unstructured.NestedString(obj.Object, "spec", "scaleTargetRef", "name")
	reference := fmt.Sprintf("%s/%s", refKind, refName)

	// Targets: format current/target from metrics
	targets := formatTargets(obj)

	// MinPods: spec.minReplicas (default 1)
	minPods := int64(1)
	if v, found, err := unstructured.NestedInt64(obj.Object, "spec", "minReplicas"); err == nil && found {
		minPods = v
	}

	// MaxPods: spec.maxReplicas
	maxPods, _, _ := unstructured.NestedInt64(obj.Object, "spec", "maxReplicas")

	// Replicas: status.currentReplicas
	currentReplicas, _, _ := unstructured.NestedInt64(obj.Object, "status", "currentReplicas")

	age := render.FormatAge(obj)

	return []string{
		name,
		reference,
		targets,
		fmt.Sprintf("%d", minPods),
		fmt.Sprintf("%d", maxPods),
		fmt.Sprintf("%d", currentReplicas),
		age,
	}
}

func (p *Plugin) YAML(obj *unstructured.Unstructured) (render.Content, error) {
	return plugin.MarshalYAML(obj)
}

// formatTargets formats the metrics targets column.
// It tries to show current/target for each metric, falling back to "<unknown>" for the current value.
func formatTargets(obj *unstructured.Unstructured) string {
	metrics, found, _ := unstructured.NestedSlice(obj.Object, "spec", "metrics")
	if !found || len(metrics) == 0 {
		return "<none>"
	}

	currentMetrics, _, _ := unstructured.NestedSlice(obj.Object, "status", "currentMetrics")

	parts := make([]string, 0, len(metrics))
	for i, m := range metrics {
		metric, ok := m.(map[string]any)
		if !ok {
			continue
		}
		target := extractTarget(metric)
		current := "<unknown>"
		if i < len(currentMetrics) {
			if cm, ok := currentMetrics[i].(map[string]any); ok {
				if v := extractCurrent(cm); v != "" {
					current = v
				}
			}
		}
		parts = append(parts, fmt.Sprintf("%s/%s", current, target))
	}

	if len(parts) == 0 {
		return "<none>"
	}
	return parts[0]
}

// extractTarget extracts the target value string from a metric spec.
func extractTarget(metric map[string]any) string {
	metricType, _ := metric["type"].(string)
	var targetMap map[string]any

	switch metricType {
	case "Resource":
		if res, ok := metric["resource"].(map[string]any); ok {
			targetMap, _ = res["target"].(map[string]any)
		}
	case "Pods":
		if pods, ok := metric["pods"].(map[string]any); ok {
			targetMap, _ = pods["target"].(map[string]any)
		}
	case "Object":
		if obj, ok := metric["object"].(map[string]any); ok {
			targetMap, _ = obj["target"].(map[string]any)
		}
	case "External":
		if ext, ok := metric["external"].(map[string]any); ok {
			targetMap, _ = ext["target"].(map[string]any)
		}
	}

	if targetMap == nil {
		return "<unknown>"
	}

	targetType, _ := targetMap["type"].(string)
	switch targetType {
	case "Utilization":
		if v, ok := targetMap["averageUtilization"]; ok {
			return fmt.Sprintf("%v%%", v)
		}
	case "AverageValue":
		if v, ok := targetMap["averageValue"]; ok {
			return fmt.Sprintf("%v", v)
		}
	case "Value":
		if v, ok := targetMap["value"]; ok {
			return fmt.Sprintf("%v", v)
		}
	}

	return "<unknown>"
}

// extractCurrent extracts the current value string from a metric status.
func extractCurrent(metric map[string]any) string {
	metricType, _ := metric["type"].(string)
	var currentMap map[string]any

	switch metricType {
	case "Resource":
		if res, ok := metric["resource"].(map[string]any); ok {
			currentMap, _ = res["current"].(map[string]any)
		}
	case "Pods":
		if pods, ok := metric["pods"].(map[string]any); ok {
			currentMap, _ = pods["current"].(map[string]any)
		}
	case "Object":
		if obj, ok := metric["object"].(map[string]any); ok {
			currentMap, _ = obj["current"].(map[string]any)
		}
	case "External":
		if ext, ok := metric["external"].(map[string]any); ok {
			currentMap, _ = ext["current"].(map[string]any)
		}
	}

	if currentMap == nil {
		return ""
	}

	if v, ok := currentMap["averageUtilization"]; ok {
		return fmt.Sprintf("%v%%", v)
	}
	if v, ok := currentMap["averageValue"]; ok {
		return fmt.Sprintf("%v", v)
	}
	if v, ok := currentMap["value"]; ok {
		return fmt.Sprintf("%v", v)
	}

	return ""
}

func (p *Plugin) Describe(ctx context.Context, obj *unstructured.Unstructured) (render.Content, error) {
	hpa, err := toHPA(obj)
	if err != nil {
		return render.Content{}, fmt.Errorf("converting to HorizontalPodAutoscaler: %w", err)
	}

	b := render.NewBuilder()

	b.KV(render.LEVEL_0, "Name", hpa.Name)
	b.KV(render.LEVEL_0, "Namespace", hpa.Namespace)
	b.KV(render.LEVEL_0, "CreationTimestamp", render.FormatAge(obj))

	// Labels
	b.KVMulti(render.LEVEL_0, "Labels", hpa.Labels)

	// Annotations
	b.KVMulti(render.LEVEL_0, "Annotations", hpa.Annotations)

	// Reference
	ref := fmt.Sprintf("%s/%s", hpa.Spec.ScaleTargetRef.Kind, hpa.Spec.ScaleTargetRef.Name)
	b.KV(render.LEVEL_0, "Reference", ref)

	// Min/Max Replicas
	minReplicas := int32(1)
	if hpa.Spec.MinReplicas != nil {
		minReplicas = *hpa.Spec.MinReplicas
	}
	b.KV(render.LEVEL_0, "Min Replicas", fmt.Sprintf("%d", minReplicas))
	b.KV(render.LEVEL_0, "Max Replicas", fmt.Sprintf("%d", hpa.Spec.MaxReplicas))

	// Current Replicas
	b.KV(render.LEVEL_0, "Current Replicas", fmt.Sprintf("%d", hpa.Status.CurrentReplicas))
	b.KV(render.LEVEL_0, "Desired Replicas", fmt.Sprintf("%d", hpa.Status.DesiredReplicas))

	// Metrics
	describeMetrics(b, hpa)

	// Conditions
	workload.DescribeConditions(b, hpa)

	return b.Build(), nil
}

func describeMetrics(b *render.Builder, hpa *autoscalingv2.HorizontalPodAutoscaler) {
	if len(hpa.Spec.Metrics) == 0 {
		return
	}
	b.Section(render.LEVEL_0, "Metrics")
	for _, m := range hpa.Spec.Metrics {
		switch m.Type {
		case autoscalingv2.ResourceMetricSourceType:
			if m.Resource != nil {
				target := formatMetricTarget(m.Resource.Target)
				b.KV(render.LEVEL_1, fmt.Sprintf("resource %s", m.Resource.Name), target)
			}
		case autoscalingv2.PodsMetricSourceType:
			if m.Pods != nil {
				target := formatMetricTarget(m.Pods.Target)
				b.KV(render.LEVEL_1, fmt.Sprintf("pods %s", m.Pods.Metric.Name), target)
			}
		case autoscalingv2.ObjectMetricSourceType:
			if m.Object != nil {
				target := formatMetricTarget(m.Object.Target)
				b.KV(render.LEVEL_1, fmt.Sprintf("object %s", m.Object.Metric.Name), target)
			}
		case autoscalingv2.ExternalMetricSourceType:
			if m.External != nil {
				target := formatMetricTarget(m.External.Target)
				b.KV(render.LEVEL_1, fmt.Sprintf("external %s", m.External.Metric.Name), target)
			}
		}
	}
}

func formatMetricTarget(target autoscalingv2.MetricTarget) string {
	switch target.Type {
	case autoscalingv2.UtilizationMetricType:
		if target.AverageUtilization != nil {
			return fmt.Sprintf("%d%% average utilization", *target.AverageUtilization)
		}
	case autoscalingv2.AverageValueMetricType:
		if target.AverageValue != nil {
			return fmt.Sprintf("%s average", target.AverageValue.String())
		}
	case autoscalingv2.ValueMetricType:
		if target.Value != nil {
			return target.Value.String()
		}
	}
	return "<unknown>"
}

// toHPA converts an unstructured object to a typed autoscalingv2.HorizontalPodAutoscaler.
func toHPA(obj *unstructured.Unstructured) (*autoscalingv2.HorizontalPodAutoscaler, error) {
	var hpa autoscalingv2.HorizontalPodAutoscaler
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &hpa); err != nil {
		return nil, err
	}
	return &hpa, nil
}
