package workload

import (
	"strings"
	"testing"

	"github.com/aohoyd/aku/internal/render"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestDescribeConditions(t *testing.T) {
	deploy := &appsv1.Deployment{
		Status: appsv1.DeploymentStatus{
			Conditions: []appsv1.DeploymentCondition{
				{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue},
				{Type: appsv1.DeploymentProgressing, Status: corev1.ConditionTrue},
			},
		},
	}
	b := render.NewBuilder()
	DescribeConditions(b, deploy)
	content := b.Build()
	if !strings.Contains(content.Raw, "Conditions") {
		t.Fatal("expected Conditions section")
	}
	if !strings.Contains(content.Raw, "Available") {
		t.Fatal("expected Available condition")
	}
}

func TestDescribeConditionsEmpty(t *testing.T) {
	deploy := &appsv1.Deployment{}
	b := render.NewBuilder()
	DescribeConditions(b, deploy)
	content := b.Build()
	if strings.Contains(content.Raw, "Conditions") {
		t.Fatal("expected no Conditions section for empty conditions")
	}
}

// makeConfigMap is a test helper that builds an unstructured ConfigMap with
// the given name and key=value data entries.
func makeConfigMap(name string, data map[string]string) *unstructured.Unstructured {
	d := map[string]any{}
	for k, v := range data {
		d[k] = v
	}
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata":   map[string]any{"name": name, "namespace": "default"},
			"data":       d,
		},
	}
}

// makeSecret is a test helper that builds an unstructured Secret with the
// given name and base64-encoded data entries (values are taken verbatim).
func makeSecret(name string, data map[string]string) *unstructured.Unstructured {
	d := map[string]any{}
	for k, v := range data {
		d[k] = v
	}
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata":   map[string]any{"name": name, "namespace": "default"},
			"data":       d,
		},
	}
}

// TestDescribeTemplateContainersResolvesEnv verifies that passing a non-nil
// configMaps/secrets slice causes envFrom and valueFrom references to be
// resolved in the rendered output.
func TestDescribeTemplateContainersResolvesEnv(t *testing.T) {
	cms := []*unstructured.Unstructured{
		makeConfigMap("app-config", map[string]string{"DB_HOST": "postgres.svc"}),
	}
	secs := []*unstructured.Unstructured{
		// "c2VjcmV0" is base64("secret").
		makeSecret("my-secret", map[string]string{"api-key": "c2VjcmV0"}),
	}
	ctrs := []corev1.Container{
		{
			Name:  "app",
			Image: "nginx",
			EnvFrom: []corev1.EnvFromSource{
				{ConfigMapRef: &corev1.ConfigMapEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: "app-config"},
				}},
			},
			Env: []corev1.EnvVar{
				{
					Name: "API_KEY",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: "my-secret"},
							Key:                  "api-key",
						},
					},
				},
			},
		},
	}

	b := render.NewBuilder()
	DescribeTemplateContainers(b, "Containers", ctrs, cms, secs)
	raw := b.Build().Raw

	if !strings.Contains(raw, "Containers") {
		t.Fatalf("expected 'Containers' section, got:\n%s", raw)
	}
	if !strings.Contains(raw, "app-config") {
		t.Errorf("expected resolved configmap source 'app-config' in output:\n%s", raw)
	}
	if !strings.Contains(raw, "postgres.svc") {
		t.Errorf("expected resolved value 'postgres.svc' in output:\n%s", raw)
	}
	if !strings.Contains(raw, "secret") {
		t.Errorf("expected resolved secret value 'secret' in output:\n%s", raw)
	}
}

// TestDescribeTemplateContainersNilSources verifies the function is safe to
// call with nil configMaps/secrets slices (the pre-refactor behavior).
func TestDescribeTemplateContainersNilSources(t *testing.T) {
	ctrs := []corev1.Container{{Name: "app", Image: "nginx"}}
	b := render.NewBuilder()
	DescribeTemplateContainers(b, "Containers", ctrs, nil, nil)
	raw := b.Build().Raw
	if !strings.Contains(raw, "Containers") {
		t.Fatalf("expected 'Containers' section with nil sources, got:\n%s", raw)
	}
	if !strings.Contains(raw, "app") || !strings.Contains(raw, "nginx") {
		t.Errorf("expected container name/image in output:\n%s", raw)
	}
}

// TestDescribeTemplateContainersEmpty verifies that an empty container slice
// emits nothing (no section header).
func TestDescribeTemplateContainersEmpty(t *testing.T) {
	b := render.NewBuilder()
	DescribeTemplateContainers(b, "Init Containers", nil, nil, nil)
	raw := b.Build().Raw
	if strings.Contains(raw, "Init Containers") {
		t.Errorf("empty slice should emit no section header, got:\n%s", raw)
	}
}

// TestDescribePodTemplateResolvesEnv verifies that DescribePodTemplate forwards
// configMaps/secrets to its nested container rendering.
func TestDescribePodTemplateResolvesEnv(t *testing.T) {
	cms := []*unstructured.Unstructured{
		makeConfigMap("app-config", map[string]string{"DB_HOST": "postgres.svc"}),
	}
	secs := []*unstructured.Unstructured{
		makeSecret("my-secret", map[string]string{"api-key": "c2VjcmV0"}),
	}
	tmpl := corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "test-pod",
			Labels: map[string]string{"app": "nginx"},
		},
		Spec: corev1.PodSpec{
			ServiceAccountName: "sa",
			Containers: []corev1.Container{
				{
					Name:  "app",
					Image: "nginx",
					EnvFrom: []corev1.EnvFromSource{
						{ConfigMapRef: &corev1.ConfigMapEnvSource{
							LocalObjectReference: corev1.LocalObjectReference{Name: "app-config"},
						}},
					},
					Env: []corev1.EnvVar{
						{
							Name: "API_KEY",
							ValueFrom: &corev1.EnvVarSource{
								SecretKeyRef: &corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{Name: "my-secret"},
									Key:                  "api-key",
								},
							},
						},
					},
				},
			},
		},
	}

	b := render.NewBuilder()
	DescribePodTemplate(b, tmpl, cms, secs)
	raw := b.Build().Raw

	for _, want := range []string{
		"Pod Template", "Service Account", "sa",
		"Containers", "app", "nginx",
		"app-config", "postgres.svc", "secret",
	} {
		if !strings.Contains(raw, want) {
			t.Errorf("expected %q in output:\n%s", want, raw)
		}
	}
}
