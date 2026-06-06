package k8s

import (
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

func TestPrepareEphemeralDebugError(t *testing.T) {
	// No pod exists, so the GET fails and no ephemeral container is patched.
	fakeClient := fake.NewClientset()
	_, err := PrepareEphemeralDebug(context.Background(), &Client{Typed: fakeClient}, "nope", "default", "app", "busybox", []string{"sh"}, false, nil)
	if err == nil {
		t.Fatal("expected error for nonexistent pod")
	}
	if !strings.Contains(err.Error(), "get pod") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPrepareNodeDebugCreateError(t *testing.T) {
	fakeClient := fake.NewClientset()
	fakeClient.PrependReactor("create", "pods", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, fmt.Errorf("simulated create failure")
	})
	_, _, _, err := PrepareNodeDebug(context.Background(), &Client{Typed: fakeClient, Namespace: "default"}, "node1", "busybox", []string{"sh"}, true, nil)
	if err == nil || !strings.Contains(err.Error(), "create debug pod") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPrepareNodeDebugDeletesPodOnWaitFailure(t *testing.T) {
	// The pod is created but its container never becomes Running. A cancelled
	// context aborts the wait; PrepareNodeDebug must best-effort delete the pod
	// it created so it does not leak.
	fakeClient := fake.NewClientset()

	deleted := make(chan string, 1)
	fakeClient.PrependReactor("delete", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		da := action.(k8stesting.DeleteAction)
		select {
		case deleted <- da.GetName():
		default:
		}
		return false, nil, nil // fall through to default tracker
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, _, _, err := PrepareNodeDebug(ctx, &Client{Typed: fakeClient, Namespace: "default"}, "mynode", "busybox", []string{"sh"}, true, nil)
	if err == nil {
		t.Fatal("expected wait error from cancelled context")
	}

	select {
	case name := <-deleted:
		if !strings.Contains(name, "ktui-debug-mynode") {
			t.Fatalf("deleted unexpected pod: %s", name)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("PrepareNodeDebug did not delete the created pod after wait failure")
	}
}

func TestDeleteNodeDebugPod(t *testing.T) {
	existing := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "debug-pod", Namespace: "default"},
	}
	fakeClient := fake.NewClientset(existing)

	if err := DeleteNodeDebugPod(context.Background(), &Client{Typed: fakeClient}, "debug-pod", "default"); err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	if _, err := fakeClient.CoreV1().Pods("default").Get(context.Background(), "debug-pod", metav1.GetOptions{}); err == nil {
		t.Fatal("pod still present after DeleteNodeDebugPod")
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
