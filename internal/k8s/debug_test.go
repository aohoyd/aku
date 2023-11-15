package k8s

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
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
	pod := buildDebugNodePod("debugger-node1-abc12", "mynode", "busybox:latest", []string{"sh"})
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
	pod := buildDebugNodePod("debug-node1", "mynode", "busybox:latest", []string{"bash", "-l"})
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
