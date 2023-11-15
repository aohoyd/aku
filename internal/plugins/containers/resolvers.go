package containers

import (
	"encoding/base64"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// indexByName builds a name-keyed lookup from a slice of unstructured objects.
func indexByName(objs []*unstructured.Unstructured) map[string]*unstructured.Unstructured {
	m := make(map[string]*unstructured.Unstructured, len(objs))
	for _, o := range objs {
		m[o.GetName()] = o
	}
	return m
}

// getData extracts the string data map from an unstructured ConfigMap or Secret.
func getData(obj *unstructured.Unstructured) map[string]string {
	data, _, _ := unstructured.NestedStringMap(obj.Object, "data")
	return data
}

// resolveEnv resolves a single EnvVar's valueFrom to its actual value.
// Returns "" when the value cannot be resolved.
// pod may be nil (e.g. deployment templates) — FieldRef won't resolve.
// cms/secrets may be nil — ConfigMapKeyRef/SecretKeyRef won't resolve.
func resolveEnv(
	e corev1.EnvVar,
	pod *corev1.Pod,
	c *corev1.Container,
	cms, secrets map[string]*unstructured.Unstructured,
) string {
	if e.ValueFrom == nil {
		return ""
	}
	switch {
	case e.ValueFrom.FieldRef != nil:
		if pod == nil {
			return ""
		}
		return resolveFieldRef(pod, e.ValueFrom.FieldRef.FieldPath)
	case e.ValueFrom.ResourceFieldRef != nil:
		if c == nil {
			return ""
		}
		return resolveResourceFieldRef(c, e.ValueFrom.ResourceFieldRef)
	case e.ValueFrom.ConfigMapKeyRef != nil:
		ref := e.ValueFrom.ConfigMapKeyRef
		if obj, ok := cms[ref.Name]; ok {
			if data := getData(obj); data != nil {
				return data[ref.Key]
			}
		}
	case e.ValueFrom.SecretKeyRef != nil:
		ref := e.ValueFrom.SecretKeyRef
		if obj, ok := secrets[ref.Name]; ok {
			if data := getData(obj); data != nil {
				v := data[ref.Key]
				if decoded, err := base64.StdEncoding.DecodeString(v); err == nil {
					return string(decoded)
				}
				return v
			}
		}
	}
	return ""
}

// resolveEnvFrom resolves an EnvFromSource to its expanded key-value pairs.
// Returns nil when the source is not found.
func resolveEnvFrom(
	ef corev1.EnvFromSource,
	cms, secrets map[string]*unstructured.Unstructured,
) map[string]string {
	var data map[string]string
	var isSecret bool
	switch {
	case ef.ConfigMapRef != nil:
		if obj, ok := cms[ef.ConfigMapRef.Name]; ok {
			data = getData(obj)
		}
	case ef.SecretRef != nil:
		isSecret = true
		if obj, ok := secrets[ef.SecretRef.Name]; ok {
			data = getData(obj)
		}
	}
	if data == nil {
		return nil
	}
	result := make(map[string]string, len(data))
	for k, v := range data {
		if isSecret {
			if decoded, err := base64.StdEncoding.DecodeString(v); err == nil {
				v = string(decoded)
			}
		}
		result[ef.Prefix+k] = v
	}
	return result
}

func resolveFieldRef(pod *corev1.Pod, fieldPath string) string {
	switch fieldPath {
	case "metadata.name":
		return pod.Name
	case "metadata.namespace":
		return pod.Namespace
	case "metadata.uid":
		return string(pod.UID)
	case "spec.nodeName":
		return pod.Spec.NodeName
	case "spec.serviceAccountName":
		return pod.Spec.ServiceAccountName
	case "status.podIP":
		return pod.Status.PodIP
	case "status.hostIP":
		return pod.Status.HostIP
	}
	if after, ok := strings.CutPrefix(fieldPath, "metadata.labels['"); ok {
		key := strings.TrimSuffix(after, "']")
		return pod.Labels[key]
	}
	if after, ok := strings.CutPrefix(fieldPath, "metadata.annotations['"); ok {
		key := strings.TrimSuffix(after, "']")
		return pod.Annotations[key]
	}
	return ""
}

func resolveResourceFieldRef(c *corev1.Container, ref *corev1.ResourceFieldSelector) string {
	switch ref.Resource {
	case "limits.cpu":
		if q, ok := c.Resources.Limits[corev1.ResourceCPU]; ok {
			return formatQuantity(q, ref.Divisor)
		}
	case "limits.memory":
		if q, ok := c.Resources.Limits[corev1.ResourceMemory]; ok {
			return formatQuantity(q, ref.Divisor)
		}
	case "requests.cpu":
		if q, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
			return formatQuantity(q, ref.Divisor)
		}
	case "requests.memory":
		if q, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
			return formatQuantity(q, ref.Divisor)
		}
	}
	return ""
}

func formatQuantity(q resource.Quantity, divisor resource.Quantity) string {
	if divisor.IsZero() || divisor.Cmp(resource.MustParse("1")) == 0 {
		return q.String()
	}
	d := divisor.MilliValue()
	if d == 0 {
		return q.String()
	}
	return fmt.Sprintf("%d", q.MilliValue()/d)
}
