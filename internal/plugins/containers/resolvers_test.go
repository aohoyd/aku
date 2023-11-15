package containers

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestIndexByName(t *testing.T) {
	objs := []*unstructured.Unstructured{
		{Object: map[string]any{"metadata": map[string]any{"name": "a"}}},
		{Object: map[string]any{"metadata": map[string]any{"name": "b"}}},
	}
	m := indexByName(objs)
	if len(m) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(m))
	}
	if m["a"].GetName() != "a" || m["b"].GetName() != "b" {
		t.Fatal("expected entries keyed by name")
	}
}

func TestIndexByNameNil(t *testing.T) {
	m := indexByName(nil)
	if m == nil || len(m) != 0 {
		t.Fatal("expected non-nil empty map")
	}
}

func TestResolveEnvFieldRef(t *testing.T) {
	pod := &corev1.Pod{}
	pod.Name = "test-pod"
	pod.Namespace = "default"
	pod.Status.PodIP = "10.0.0.1"
	pod.Spec.NodeName = "node-1"
	pod.Spec.ServiceAccountName = "my-sa"
	pod.Labels = map[string]string{"app": "web"}
	pod.Annotations = map[string]string{"note": "hello"}

	c := &corev1.Container{Name: "main"}

	cases := []struct {
		fieldPath string
		want      string
	}{
		{"metadata.name", "test-pod"},
		{"metadata.namespace", "default"},
		{"status.podIP", "10.0.0.1"},
		{"spec.nodeName", "node-1"},
		{"spec.serviceAccountName", "my-sa"},
		{"metadata.labels['app']", "web"},
		{"metadata.annotations['note']", "hello"},
	}
	for _, tc := range cases {
		e := corev1.EnvVar{
			Name:      "TEST",
			ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: tc.fieldPath}},
		}
		got := resolveEnv(e, pod, c, nil, nil)
		if got != tc.want {
			t.Errorf("FieldRef %s: got %q, want %q", tc.fieldPath, got, tc.want)
		}
	}
}

func TestResolveEnvResourceFieldRef(t *testing.T) {
	c := &corev1.Container{
		Name: "main",
		Resources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("500m"),
				corev1.ResourceMemory: resource.MustParse("128Mi"),
			},
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("250m"),
				corev1.ResourceMemory: resource.MustParse("64Mi"),
			},
		},
	}
	pod := &corev1.Pod{}

	cases := []struct {
		resource string
		want     string
	}{
		{"limits.cpu", "500m"},
		{"limits.memory", "128Mi"},
		{"requests.cpu", "250m"},
		{"requests.memory", "64Mi"},
	}
	for _, tc := range cases {
		e := corev1.EnvVar{
			Name: "TEST",
			ValueFrom: &corev1.EnvVarSource{
				ResourceFieldRef: &corev1.ResourceFieldSelector{Resource: tc.resource},
			},
		}
		got := resolveEnv(e, pod, c, nil, nil)
		if got != tc.want {
			t.Errorf("ResourceFieldRef %s: got %q, want %q", tc.resource, got, tc.want)
		}
	}
}

func TestResolveEnvConfigMapKeyRef(t *testing.T) {
	cms := map[string]*unstructured.Unstructured{
		"app-config": {Object: map[string]any{
			"data": map[string]any{"DB_HOST": "postgres.svc"},
		}},
	}
	pod := &corev1.Pod{}
	c := &corev1.Container{Name: "main"}

	e := corev1.EnvVar{
		Name: "DB_HOST",
		ValueFrom: &corev1.EnvVarSource{
			ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "app-config"},
				Key:                  "DB_HOST",
			},
		},
	}
	if got := resolveEnv(e, pod, c, cms, nil); got != "postgres.svc" {
		t.Fatalf("expected postgres.svc, got %q", got)
	}
}

func TestResolveEnvSecretKeyRef(t *testing.T) {
	secrets := map[string]*unstructured.Unstructured{
		"my-secret": {Object: map[string]any{
			"data": map[string]any{"api-key": "c2VjcmV0dmFsdWU="},
		}},
	}
	pod := &corev1.Pod{}
	c := &corev1.Container{Name: "main"}

	e := corev1.EnvVar{
		Name: "API_KEY",
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "my-secret"},
				Key:                  "api-key",
			},
		},
	}
	if got := resolveEnv(e, pod, c, nil, secrets); got != "secretvalue" {
		t.Fatalf("expected secretvalue, got %q", got)
	}
}

func TestResolveEnvNilMapsReturnEmpty(t *testing.T) {
	pod := &corev1.Pod{}
	c := &corev1.Container{Name: "main"}

	e := corev1.EnvVar{
		Name: "DB_HOST",
		ValueFrom: &corev1.EnvVarSource{
			ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "app-config"},
				Key:                  "DB_HOST",
			},
		},
	}
	if got := resolveEnv(e, pod, c, nil, nil); got != "" {
		t.Fatalf("expected empty string without lookup, got %q", got)
	}
}

func TestResolveEnvFromConfigMap(t *testing.T) {
	cms := map[string]*unstructured.Unstructured{
		"app-config": {Object: map[string]any{
			"data": map[string]any{"DB_HOST": "postgres.svc", "DB_PORT": "5432"},
		}},
	}
	ef := corev1.EnvFromSource{
		ConfigMapRef: &corev1.ConfigMapEnvSource{
			LocalObjectReference: corev1.LocalObjectReference{Name: "app-config"},
		},
	}
	vals := resolveEnvFrom(ef, cms, nil)
	if vals["DB_HOST"] != "postgres.svc" || vals["DB_PORT"] != "5432" {
		t.Fatalf("unexpected resolved values: %v", vals)
	}
}

func TestResolveEnvFromWithPrefix(t *testing.T) {
	cms := map[string]*unstructured.Unstructured{
		"app-config": {Object: map[string]any{
			"data": map[string]any{"HOST": "postgres.svc"},
		}},
	}
	ef := corev1.EnvFromSource{
		Prefix: "APP_",
		ConfigMapRef: &corev1.ConfigMapEnvSource{
			LocalObjectReference: corev1.LocalObjectReference{Name: "app-config"},
		},
	}
	vals := resolveEnvFrom(ef, cms, nil)
	if vals["APP_HOST"] != "postgres.svc" {
		t.Fatalf("expected prefixed key APP_HOST, got %v", vals)
	}
}

func TestResolveEnvFromNilMaps(t *testing.T) {
	ef := corev1.EnvFromSource{
		ConfigMapRef: &corev1.ConfigMapEnvSource{
			LocalObjectReference: corev1.LocalObjectReference{Name: "anything"},
		},
	}
	vals := resolveEnvFrom(ef, nil, nil)
	if vals != nil {
		t.Fatal("expected nil for nil maps")
	}
}

func TestResolveEnvFromMissingSource(t *testing.T) {
	cms := map[string]*unstructured.Unstructured{}
	secrets := map[string]*unstructured.Unstructured{}
	ef := corev1.EnvFromSource{
		ConfigMapRef: &corev1.ConfigMapEnvSource{
			LocalObjectReference: corev1.LocalObjectReference{Name: "nonexistent"},
		},
	}
	if vals := resolveEnvFrom(ef, cms, secrets); vals != nil {
		t.Fatalf("expected nil for missing source, got %v", vals)
	}
}

func TestResolveEnvFromSecretBase64Decode(t *testing.T) {
	secrets := map[string]*unstructured.Unstructured{
		"my-secret": {Object: map[string]any{
			"data": map[string]any{"api-key": "c2VjcmV0dmFsdWU="},
		}},
	}
	ef := corev1.EnvFromSource{
		SecretRef: &corev1.SecretEnvSource{
			LocalObjectReference: corev1.LocalObjectReference{Name: "my-secret"},
		},
	}
	vals := resolveEnvFrom(ef, nil, secrets)
	if vals["api-key"] != "secretvalue" {
		t.Fatalf("expected decoded secret, got %q", vals["api-key"])
	}
}
