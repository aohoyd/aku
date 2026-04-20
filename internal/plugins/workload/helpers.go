package workload

import (
	"fmt"
	"slices"
	"strings"

	"github.com/aohoyd/aku/internal/plugins/containers"
	"github.com/aohoyd/aku/internal/render"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func GetInt64(obj *unstructured.Unstructured, fields ...string) int64 {
	val, found, err := unstructured.NestedInt64(obj.Object, fields...)
	if err != nil || !found {
		return 0
	}
	return val
}

func FormatSelector(m map[string]string) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", k, m[k]))
	}
	return strings.Join(parts, ",")
}

func DescribeMetadata(b *render.Builder, obj *unstructured.Unstructured, name, namespace string, labels, annotations map[string]string) {
	b.KV(render.LEVEL_0, "Name", name)
	b.KV(render.LEVEL_0, "Namespace", namespace)
	b.KV(render.LEVEL_0, "CreationTimestamp", render.FormatAge(obj))
	b.KVMulti(render.LEVEL_0, "Labels", labels)
	b.KVMulti(render.LEVEL_0, "Annotations", annotations)
}

func DescribeSelector(b *render.Builder, matchLabels map[string]string) {
	if len(matchLabels) > 0 {
		b.KV(render.LEVEL_0, "Selector", FormatSelector(matchLabels))
	}
}

func DescribePodTemplate(b *render.Builder, template corev1.PodTemplateSpec, configMaps, secrets []*unstructured.Unstructured) {
	b.Section(render.LEVEL_0, "Pod Template")
	if len(template.Labels) > 0 {
		b.KVMulti(render.LEVEL_1, "Labels", template.Labels)
	}
	if len(template.Annotations) > 0 {
		b.KVMulti(render.LEVEL_1, "Annotations", template.Annotations)
	}
	if template.Spec.ServiceAccountName != "" {
		b.KV(render.LEVEL_1, "Service Account", template.Spec.ServiceAccountName)
	}
	DescribeTemplateContainers(b, "Containers", template.Spec.Containers, configMaps, secrets)
	DescribeTemplateContainers(b, "Init Containers", template.Spec.InitContainers, configMaps, secrets)
}

func DescribeTemplateContainers(b *render.Builder, label string, ctrs []corev1.Container, configMaps, secrets []*unstructured.Unstructured) {
	if len(ctrs) == 0 {
		return
	}
	b.Section(render.LEVEL_1, label)
	for _, c := range ctrs {
		b.Section(render.LEVEL_2, c.Name)
		containers.DescribeContainer(b, render.LEVEL_3, c, nil, nil, configMaps, secrets)
	}
}

// condition is a uniform representation of k8s resource conditions.
type condition struct {
	Type   string
	Status corev1.ConditionStatus
}

// conditionsFor extracts conditions from any supported k8s resource type.
func conditionsFor(r any) []condition {
	switch obj := r.(type) {
	case *appsv1.Deployment:
		conds := make([]condition, len(obj.Status.Conditions))
		for i, c := range obj.Status.Conditions {
			conds[i] = condition{Type: string(c.Type), Status: corev1.ConditionStatus(c.Status)}
		}
		return conds
	case *appsv1.DaemonSet:
		conds := make([]condition, len(obj.Status.Conditions))
		for i, c := range obj.Status.Conditions {
			conds[i] = condition{Type: string(c.Type), Status: corev1.ConditionStatus(c.Status)}
		}
		return conds
	case *appsv1.ReplicaSet:
		conds := make([]condition, len(obj.Status.Conditions))
		for i, c := range obj.Status.Conditions {
			conds[i] = condition{Type: string(c.Type), Status: corev1.ConditionStatus(c.Status)}
		}
		return conds
	case *appsv1.StatefulSet:
		conds := make([]condition, len(obj.Status.Conditions))
		for i, c := range obj.Status.Conditions {
			conds[i] = condition{Type: string(c.Type), Status: corev1.ConditionStatus(c.Status)}
		}
		return conds
	case *batchv1.Job:
		conds := make([]condition, len(obj.Status.Conditions))
		for i, c := range obj.Status.Conditions {
			conds[i] = condition{Type: string(c.Type), Status: corev1.ConditionStatus(c.Status)}
		}
		return conds
	case *corev1.Pod:
		conds := make([]condition, len(obj.Status.Conditions))
		for i, c := range obj.Status.Conditions {
			conds[i] = condition{Type: string(c.Type), Status: corev1.ConditionStatus(c.Status)}
		}
		return conds
	case *autoscalingv2.HorizontalPodAutoscaler:
		conds := make([]condition, len(obj.Status.Conditions))
		for i, c := range obj.Status.Conditions {
			conds[i] = condition{Type: string(c.Type), Status: corev1.ConditionStatus(c.Status)}
		}
		return conds
	default:
		return nil
	}
}

// DescribeConditions renders a "Conditions" section for any supported resource type.
func DescribeConditions(b *render.Builder, r any) {
	conds := conditionsFor(r)
	if len(conds) == 0 {
		return
	}
	b.Section(render.LEVEL_0, "Conditions")
	for _, c := range conds {
		b.KVStyled(render.LEVEL_1, render.ConditionKind(string(c.Status)), c.Type, string(c.Status))
	}
}
