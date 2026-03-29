package k8s

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestBuildEphemeralContainer(t *testing.T) {
	ec := buildEphemeralContainer("debugger-abc12", "busybox:latest", []string{"sh"}, "mycontainer", false)
	if ec.Name != "debugger-abc12" {
		t.Fatal("wrong name")
	}
	if ec.Image != "busybox:latest" {
		t.Fatal("wrong image")
	}
	if ec.TargetContainerName != "mycontainer" {
		t.Fatal("wrong target")
	}
	if !ec.Stdin || !ec.TTY {
		t.Fatal("stdin/tty not set")
	}
	if ec.SecurityContext != nil {
		t.Fatal("non-privileged should have nil security context")
	}
}

func TestBuildEphemeralContainerPrivileged(t *testing.T) {
	ec := buildEphemeralContainer("debugger-abc12", "busybox:latest", []string{"sh"}, "", true)
	if ec.SecurityContext == nil || ec.SecurityContext.Privileged == nil || !*ec.SecurityContext.Privileged {
		t.Fatal("privileged not set")
	}
	if ec.TargetContainerName != "" {
		t.Fatal("target should be empty")
	}
}

func TestBuildDebugNodePod(t *testing.T) {
	pod := buildDebugNodePod("debugger-node1-abc12", "mynode", "busybox:latest", []string{"sh"}, true)
	if pod.Spec.NodeName != "mynode" {
		t.Fatal("wrong node")
	}
	if !pod.Spec.HostPID || !pod.Spec.HostNetwork || !pod.Spec.HostIPC {
		t.Fatal("host namespaces not set")
	}
	if pod.Spec.RestartPolicy != corev1.RestartPolicyNever {
		t.Fatal("wrong restart policy")
	}
	if len(pod.Spec.Volumes) != 1 {
		t.Fatal("expected 1 volume")
	}
	if len(pod.Spec.Tolerations) != 1 || pod.Spec.Tolerations[0].Operator != corev1.TolerationOpExists {
		t.Fatal("wrong tolerations")
	}
	container := pod.Spec.Containers[0]
	if container.VolumeMounts[0].MountPath != "/host" {
		t.Fatal("wrong mount path")
	}
	if container.SecurityContext == nil || !*container.SecurityContext.Privileged {
		t.Fatal("not privileged")
	}
}

func TestBuildEphemeralContainerCustomCommand(t *testing.T) {
	ec := buildEphemeralContainer("debugger-abc12", "busybox:latest", []string{"bash", "-l"}, "", false)
	if len(ec.Command) != 2 || ec.Command[0] != "bash" || ec.Command[1] != "-l" {
		t.Fatalf("wrong command: %v", ec.Command)
	}
}

func TestBuildDebugNodePodCustomCommand(t *testing.T) {
	pod := buildDebugNodePod("debug-node1", "mynode", "busybox:latest", []string{"bash", "-l"}, true)
	cmd := pod.Spec.Containers[0].Command
	if len(cmd) != 2 || cmd[0] != "bash" || cmd[1] != "-l" {
		t.Fatalf("wrong command: %v", cmd)
	}
}

func TestGenerateDebugName(t *testing.T) {
	name := generateDebugName("debugger")
	if len(name) < len("debugger-")+5 {
		t.Fatalf("name too short: %s", name)
	}
	// Verify uniqueness
	name2 := generateDebugName("debugger")
	if name == name2 {
		t.Fatal("names should be unique")
	}
}

func TestRunPodDebugSpinnerOnGetPodError(t *testing.T) {
	// Use an empty fake clientset — no pods exist, so Get will fail.
	fakeClient := fake.NewClientset()
	var buf bytes.Buffer

	d := &debugCommand{
		stdout:    &buf,
		stderr:    &buf,
		client:    &Client{Typed: fakeClient},
		podName:   "nonexistent-pod",
		namespace: "default",
		image:     "busybox:latest",
		command:   []string{"sh"},
	}

	err := d.runPodDebug(context.Background())
	if err == nil {
		t.Fatal("expected error for nonexistent pod")
	}
	if !strings.Contains(err.Error(), "get pod") {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	// Spinner should have rendered something (at least the clear sequence).
	// The ANSI clear sequence \r\033[2K is written on Stop.
	if !strings.Contains(output, "\033[2K") {
		t.Fatal("expected spinner clear sequence in output")
	}
}

func TestRunPodDebugSpinnerStatusProgression(t *testing.T) {
	// Create a fake clientset with a pod that exists but whose container
	// never becomes Running — we cancel the context to exit waitForContainerRunning.
	existingPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: "app", Image: "nginx"},
			},
		},
	}
	fakeClient := fake.NewClientset(existingPod)
	var buf bytes.Buffer

	d := &debugCommand{
		stdout:    &buf,
		stderr:    &buf,
		client:    &Client{Typed: fakeClient},
		podName:   "test-pod",
		namespace: "default",
		image:     "busybox:latest",
		command:   []string{"sh"},
	}

	// Cancel context after enough time for spinner to render both statuses.
	ctx, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
	defer cancel()

	err := d.runPodDebug(ctx)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}

	output := buf.String()
	// Verify spinner rendered the initial status.
	if !strings.Contains(output, "Creating debug container...") {
		t.Fatal("expected 'Creating debug container...' in output")
	}
	// Verify spinner updated to the waiting status.
	if !strings.Contains(output, "Waiting for container...") {
		t.Fatal("expected 'Waiting for container...' in output")
	}
}

func TestRunNodeDebugSpinnerOnCreateError(t *testing.T) {
	fakeClient := fake.NewClientset()
	fakeClient.PrependReactor("create", "pods", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("simulated create failure")
	})
	var buf bytes.Buffer

	d := &debugCommand{
		stdout:   &buf,
		stderr:   &buf,
		client:   &Client{Typed: fakeClient, Namespace: "default"},
		nodeMode: true,
		nodeName: "test-node",
		image:    "busybox:latest",
		command:  []string{"sh"},
	}

	err := d.runNodeDebug(context.Background())
	if err == nil {
		t.Fatal("expected error from create failure")
	}
	if !strings.Contains(err.Error(), "create debug pod") {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	// Spinner should have been started and stopped (clear sequence present).
	if !strings.Contains(output, "\033[2K") {
		t.Fatal("expected spinner clear sequence in output")
	}
}

func TestRunNodeDebugSpinnerStatusProgression(t *testing.T) {
	fakeClient := fake.NewClientset()
	var buf bytes.Buffer

	d := &debugCommand{
		stdout:     &buf,
		stderr:     &buf,
		client:     &Client{Typed: fakeClient, Namespace: "default"},
		nodeMode:   true,
		nodeName:   "mynode",
		image:      "busybox:latest",
		command:    []string{"sh"},
		privileged: true,
	}

	// Cancel context after enough time for spinner to render both statuses.
	ctx, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
	defer cancel()

	err := d.runNodeDebug(ctx)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}

	output := buf.String()
	// Verify spinner rendered the node-specific initial status.
	if !strings.Contains(output, "Creating debug pod on mynode...") {
		t.Fatal("expected 'Creating debug pod on mynode...' in output")
	}
	// Verify spinner updated to the waiting status.
	if !strings.Contains(output, "Waiting for pod...") {
		t.Fatal("expected 'Waiting for pod...' in output")
	}
}

func TestWaitForContainerRunningCancelledContext(t *testing.T) {
	fakeClient := fake.NewClientset()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := waitForContainerRunning(ctx, fakeClient, "pod", "container", "default")
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	if !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("unexpected error: %v", err)
	}
}
