package containers

import (
	"strings"
	"testing"

	"github.com/aohoyd/aku/internal/render"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestDescribeContainerNoStatus(t *testing.T) {
	c := corev1.Container{Image: "nginx:1.25"}
	b := render.NewBuilder()
	DescribeContainer(b, render.LEVEL_0, c, nil, nil, nil, nil)
	out := b.Build()
	if !strings.Contains(out.Raw, "nginx:1.25") {
		t.Errorf("expected image in output, got:\n%s", out.Raw)
	}
	if strings.Contains(out.Raw, "State:") {
		t.Errorf("should not have State when status is nil, got:\n%s", out.Raw)
	}
}

func TestDescribeContainerWithStatus(t *testing.T) {
	c := corev1.Container{Image: "nginx:1.25"}
	now := metav1.Now()
	status := &corev1.ContainerStatus{
		State:        corev1.ContainerState{Running: &corev1.ContainerStateRunning{StartedAt: now}},
		Ready:        true,
		RestartCount: 3,
		LastTerminationState: corev1.ContainerState{
			Terminated: &corev1.ContainerStateTerminated{Reason: "OOMKilled", ExitCode: 137},
		},
	}
	b := render.NewBuilder()
	DescribeContainer(b, render.LEVEL_2, c, status, nil, nil, nil)
	out := b.Build()
	for _, want := range []string{"Running", "Started", "Last State", "OOMKilled", "137", "Ready", "true", "Restart Count", "3"} {
		if !strings.Contains(out.Raw, want) {
			t.Errorf("expected %q in output, got:\n%s", want, out.Raw)
		}
	}
}

func TestDescribeSpec(t *testing.T) {
	b := render.NewBuilder()
	c := corev1.Container{
		Image: "nginx:1.25",
		Ports: []corev1.ContainerPort{
			{ContainerPort: 80, Protocol: corev1.ProtocolTCP, Name: "http"},
		},
	}
	describeSpec(b, render.LEVEL_2, c)
	out := b.Build()
	if !strings.Contains(out.Raw, "nginx:1.25") {
		t.Fatalf("expected image in output, got:\n%s", out.Raw)
	}
	if !strings.Contains(out.Raw, "80/TCP (http)") {
		t.Fatalf("expected port in output, got:\n%s", out.Raw)
	}
	if !strings.Contains(out.Raw, "Port:") {
		t.Fatalf("expected Port: label in output, got:\n%s", out.Raw)
	}
}

func TestDescribeSpecMultiplePorts(t *testing.T) {
	b := render.NewBuilder()
	c := corev1.Container{
		Image: "app:latest",
		Ports: []corev1.ContainerPort{
			{ContainerPort: 80, Protocol: corev1.ProtocolTCP},
			{ContainerPort: 443, Protocol: corev1.ProtocolTCP},
		},
	}
	describeSpec(b, render.LEVEL_2, c)
	out := b.Build()
	if !strings.Contains(out.Raw, "Ports:") {
		t.Fatalf("expected 'Ports:' (plural) for multiple ports, got:\n%s", out.Raw)
	}
	if !strings.Contains(out.Raw, "80/TCP") {
		t.Fatalf("expected first port in output, got:\n%s", out.Raw)
	}
}

func TestDescribeSpecHostPort(t *testing.T) {
	b := render.NewBuilder()
	c := corev1.Container{
		Image: "app:latest",
		Ports: []corev1.ContainerPort{
			{ContainerPort: 80, HostPort: 8080, Protocol: corev1.ProtocolTCP},
		},
	}
	describeSpec(b, render.LEVEL_2, c)
	out := b.Build()
	if !strings.Contains(out.Raw, "Host Port:") {
		t.Fatalf("expected Host Port in output, got:\n%s", out.Raw)
	}
	if !strings.Contains(out.Raw, "8080/TCP") {
		t.Fatalf("expected host port value in output, got:\n%s", out.Raw)
	}
}

func TestDescribeEnv(t *testing.T) {
	b := render.NewBuilder()
	c := corev1.Container{
		Env: []corev1.EnvVar{
			{Name: "LOG_LEVEL", Value: "DEBUG"},
		},
	}
	describeEnvFrom(b, render.LEVEL_2, c, nil, nil)
	describeEnv(b, render.LEVEL_2, c, nil, nil, nil)
	out := b.Build()
	if !strings.Contains(out.Raw, "LOG_LEVEL") || !strings.Contains(out.Raw, "DEBUG") {
		t.Fatalf("expected env var in output, got:\n%s", out.Raw)
	}
	if !strings.Contains(out.Raw, "Environment:") {
		t.Fatalf("expected 'Environment:' section header, got:\n%s", out.Raw)
	}
}

func TestDescribeEnvNone(t *testing.T) {
	b := render.NewBuilder()
	c := corev1.Container{}
	describeEnvFrom(b, render.LEVEL_2, c, nil, nil)
	describeEnv(b, render.LEVEL_2, c, nil, nil, nil)
	out := b.Build()
	if !strings.Contains(out.Raw, "Environment:") {
		t.Fatalf("expected 'Environment:' even with no env, got:\n%s", out.Raw)
	}
	if !strings.Contains(out.Raw, "<none>") {
		t.Fatalf("expected '<none>' for empty env, got:\n%s", out.Raw)
	}
}

func TestDescribeEnvResolved(t *testing.T) {
	b := render.NewBuilder()

	optional := true
	c := corev1.Container{
		EnvFrom: []corev1.EnvFromSource{
			{
				ConfigMapRef: &corev1.ConfigMapEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: "config"},
				},
			},
			{
				SecretRef: &corev1.SecretEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: "pass"},
					Optional:             &optional,
				},
			},
		},
		Env: []corev1.EnvVar{
			{Name: "EXPLICIT", Value: "direct"},
		},
	}

	cms := map[string]*unstructured.Unstructured{
		"config": {Object: map[string]any{
			"data": map[string]any{"LOG_LEVEL": "DEBUG", "TIMEOUT": "30s"},
		}},
	}
	secrets := map[string]*unstructured.Unstructured{
		"pass": {Object: map[string]any{
			"data": map[string]any{"PASSWORD": "YWJjZA=="},
		}},
	}

	describeEnvFrom(b, render.LEVEL_2, c, cms, secrets)
	describeEnv(b, render.LEVEL_2, c, nil, cms, secrets)
	out := b.Build()

	if !strings.Contains(out.Raw, "Environment Variables from:") {
		t.Fatalf("expected 'Environment Variables from:' header, got:\n%s", out.Raw)
	}
	if !strings.Contains(out.Raw, "config") {
		t.Fatalf("expected configmap name 'config', got:\n%s", out.Raw)
	}
	if !strings.Contains(out.Raw, "ConfigMap") {
		t.Fatalf("expected 'ConfigMap' type, got:\n%s", out.Raw)
	}
	if !strings.Contains(out.Raw, "LOG_LEVEL") || !strings.Contains(out.Raw, "DEBUG") {
		t.Fatalf("expected resolved LOG_LEVEL, got:\n%s", out.Raw)
	}
	if !strings.Contains(out.Raw, "TIMEOUT") || !strings.Contains(out.Raw, "30s") {
		t.Fatalf("expected resolved TIMEOUT, got:\n%s", out.Raw)
	}
	if !strings.Contains(out.Raw, "pass") {
		t.Fatalf("expected secret name 'pass', got:\n%s", out.Raw)
	}
	if !strings.Contains(out.Raw, "Secret") {
		t.Fatalf("expected 'Secret' type, got:\n%s", out.Raw)
	}
	if !strings.Contains(out.Raw, "Optional: true") {
		t.Fatalf("expected 'Optional: true' for secret, got:\n%s", out.Raw)
	}
	if !strings.Contains(out.Raw, "PASSWORD") || !strings.Contains(out.Raw, "abcd") {
		t.Fatalf("expected resolved PASSWORD, got:\n%s", out.Raw)
	}
	if !strings.Contains(out.Raw, "EXPLICIT") || !strings.Contains(out.Raw, "direct") {
		t.Fatalf("expected explicit env var, got:\n%s", out.Raw)
	}
}

func TestDescribeEnvUnresolved(t *testing.T) {
	b := render.NewBuilder()
	c := corev1.Container{
		EnvFrom: []corev1.EnvFromSource{
			{
				ConfigMapRef: &corev1.ConfigMapEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: "missing"},
				},
			},
		},
	}
	describeEnvFrom(b, render.LEVEL_2, c, nil, nil)
	describeEnv(b, render.LEVEL_2, c, nil, nil, nil)
	out := b.Build()

	if !strings.Contains(out.Raw, "Environment Variables from:") {
		t.Fatalf("expected envFrom header, got:\n%s", out.Raw)
	}
	if !strings.Contains(out.Raw, "missing") {
		t.Fatalf("expected source name 'missing', got:\n%s", out.Raw)
	}
	lines := strings.Split(out.Raw, "\n")
	envCount := 0
	for _, line := range lines {
		if strings.Contains(line, "Environment:") {
			envCount++
		}
	}
	if envCount > 0 {
		t.Fatalf("should not have 'Environment:' section when only envFrom exists, got:\n%s", out.Raw)
	}
}

func TestDescribeEnvValueFrom(t *testing.T) {
	b := render.NewBuilder()
	c := corev1.Container{
		Env: []corev1.EnvVar{
			{
				Name: "DB_HOST",
				ValueFrom: &corev1.EnvVarSource{
					ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "app-config"},
						Key:                  "host",
					},
				},
			},
			{
				Name: "POD_NAME",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						APIVersion: "v1",
						FieldPath:  "metadata.name",
					},
				},
			},
		},
	}
	describeEnvFrom(b, render.LEVEL_2, c, nil, nil)
	describeEnv(b, render.LEVEL_2, c, nil, nil, nil)
	out := b.Build()
	if !strings.Contains(out.Raw, "<key host in ConfigMap app-config>") {
		t.Fatalf("expected configmap key ref, got:\n%s", out.Raw)
	}
	if !strings.Contains(out.Raw, "metadata.name") {
		t.Fatalf("expected field ref, got:\n%s", out.Raw)
	}
}

func TestDescribeEnvWithResolverFn(t *testing.T) {
	b := render.NewBuilder()
	c := corev1.Container{
		Env: []corev1.EnvVar{
			{
				Name: "DB_HOST",
				ValueFrom: &corev1.EnvVarSource{
					ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "app-config"},
						Key:                  "host",
					},
				},
			},
			{
				Name: "UNRESOLVABLE",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "missing-secret"},
						Key:                  "key",
					},
				},
			},
		},
	}
	cms := map[string]*unstructured.Unstructured{
		"app-config": {Object: map[string]any{
			"data": map[string]any{"host": "postgres.svc"},
		}},
	}
	describeEnvFrom(b, render.LEVEL_2, c, nil, nil)
	describeEnv(b, render.LEVEL_2, c, nil, cms, nil)
	out := b.Build()

	if !strings.Contains(out.Raw, "DB_HOST") || !strings.Contains(out.Raw, "postgres.svc") {
		t.Fatalf("expected resolved DB_HOST=postgres.svc, got:\n%s", out.Raw)
	}
	if !strings.Contains(out.Raw, "<key key in Secret missing-secret>") {
		t.Fatalf("expected fallback reference string for UNRESOLVABLE, got:\n%s", out.Raw)
	}
}

func TestDescribeResources(t *testing.T) {
	b := render.NewBuilder()
	c := corev1.Container{
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
	describeResources(b, render.LEVEL_2, c)
	out := b.Build()
	if !strings.Contains(out.Raw, "Limits:") {
		t.Fatalf("expected Limits, got:\n%s", out.Raw)
	}
	if !strings.Contains(out.Raw, "cpu: 500m") {
		t.Fatalf("expected cpu limit, got:\n%s", out.Raw)
	}
	if !strings.Contains(out.Raw, "Requests:") {
		t.Fatalf("expected Requests, got:\n%s", out.Raw)
	}
	if !strings.Contains(out.Raw, "memory: 64Mi") {
		t.Fatalf("expected memory request, got:\n%s", out.Raw)
	}
}

func TestDescribeMounts(t *testing.T) {
	b := render.NewBuilder()
	c := corev1.Container{
		VolumeMounts: []corev1.VolumeMount{
			{Name: "data", MountPath: "/var/data", ReadOnly: true},
			{Name: "config", MountPath: "/etc/config"},
		},
	}
	describeMounts(b, render.LEVEL_2, c)
	out := b.Build()
	if !strings.Contains(out.Raw, "Mounts:") {
		t.Fatalf("expected Mounts header, got:\n%s", out.Raw)
	}
	if !strings.Contains(out.Raw, "/var/data") {
		t.Fatalf("expected mount path, got:\n%s", out.Raw)
	}
	if !strings.Contains(out.Raw, "from data (ro)") {
		t.Fatalf("expected read-only mount, got:\n%s", out.Raw)
	}
	if !strings.Contains(out.Raw, "from config (rw)") {
		t.Fatalf("expected read-write mount, got:\n%s", out.Raw)
	}
}

func TestDescribeMountsEmpty(t *testing.T) {
	b := render.NewBuilder()
	c := corev1.Container{}
	describeMounts(b, render.LEVEL_2, c)
	out := b.Build()
	if !strings.Contains(out.Raw, "Mounts:") {
		t.Fatalf("expected Mounts header, got:\n%s", out.Raw)
	}
	if !strings.Contains(out.Raw, "<none>") {
		t.Fatalf("expected '<none>' for no mounts, got:\n%s", out.Raw)
	}
}

func TestDescribeProbes(t *testing.T) {
	b := render.NewBuilder()
	c := corev1.Container{
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path:   "/healthz",
					Port:   intstr.FromInt32(8080),
					Scheme: corev1.URISchemeHTTP,
				},
			},
			PeriodSeconds:  10,
			TimeoutSeconds: 1,
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				TCPSocket: &corev1.TCPSocketAction{
					Port: intstr.FromInt32(3306),
				},
			},
		},
	}
	describeProbes(b, render.LEVEL_2, c)
	out := b.Build()
	if !strings.Contains(out.Raw, "Liveness:") {
		t.Fatalf("expected Liveness probe, got:\n%s", out.Raw)
	}
	if !strings.Contains(out.Raw, "http-get http://:8080/healthz") {
		t.Fatalf("expected http-get probe details, got:\n%s", out.Raw)
	}
	if !strings.Contains(out.Raw, "Readiness:") {
		t.Fatalf("expected Readiness probe, got:\n%s", out.Raw)
	}
	if !strings.Contains(out.Raw, "tcp-socket :3306") {
		t.Fatalf("expected tcp-socket probe, got:\n%s", out.Raw)
	}
}

func TestFormatResources(t *testing.T) {
	res := map[string]string{"cpu": "500m", "memory": "128Mi"}
	got := formatResources(res)
	if got != "cpu: 500m, memory: 128Mi" {
		t.Fatalf("expected 'cpu: 500m, memory: 128Mi', got: %s", got)
	}
}
