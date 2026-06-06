package k8s

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
)

// generateDebugName produces a name like "prefix-a1b2c" with 5 random hex chars.
func generateDebugName(prefix string) string {
	b := make([]byte, 3)
	_, _ = rand.Read(b)
	suffix := hex.EncodeToString(b)[:5]
	name := prefix + "-" + suffix
	// DNS label limit is 63 chars
	if len(name) > 63 {
		name = name[:63]
	}
	return name
}

// buildEphemeralContainer creates an EphemeralContainer spec for debugging.
func buildEphemeralContainer(name, image string, command []string, targetContainer string, privileged bool) corev1.EphemeralContainer {
	ec := corev1.EphemeralContainer{
		EphemeralContainerCommon: corev1.EphemeralContainerCommon{
			Name:    name,
			Image:   image,
			Command: command,
			Stdin:   true,
			TTY:     true,
		},
		TargetContainerName: targetContainer,
	}
	if privileged {
		ec.SecurityContext = &corev1.SecurityContext{Privileged: new(true)}
	}
	return ec
}

// buildDebugNodePod creates a Pod spec for node-level debugging.
func buildDebugNodePod(name, nodeName, image string, command []string, privileged bool) *corev1.Pod {
	hostPathType := corev1.HostPathDirectory
	p := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: corev1.PodSpec{
			NodeName:      nodeName,
			HostPID:       true,
			HostNetwork:   true,
			HostIPC:       true,
			RestartPolicy: corev1.RestartPolicyNever,
			Tolerations: []corev1.Toleration{
				{Operator: corev1.TolerationOpExists},
			},
			Volumes: []corev1.Volume{
				{
					Name: "host-root",
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: "/",
							Type: &hostPathType,
						},
					},
				},
			},
			Containers: []corev1.Container{
				{
					Name:    "debugger",
					Image:   image,
					Command: command,
					Stdin:   true,
					TTY:     true,
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "host-root",
							MountPath: "/host",
						},
					},
				},
			},
		},
	}

	if privileged {
		p.Spec.Containers[0].SecurityContext = &corev1.SecurityContext{Privileged: new(true)}
	}

	return &p
}

// PrepareEphemeralDebug performs the pod-debug pre-flight: it GETs the pod,
// appends an ephemeral debug container, calls UpdateEphemeralContainers, then
// waits for that container to reach Running. It returns the generated debug
// container name so the caller can attach to it via NewAttachExecutor.
//
// onStatus, when non-nil, is invoked with human-readable progress strings
// ("Waiting for container...") so callers driving a spinner can surface them; it
// is a no-op for the embedded-terminal path (which shows its own placeholder).
func PrepareEphemeralDebug(ctx context.Context, client *Client, podName, namespace, targetContainer, image string, command []string, privileged bool, onStatus func(string)) (debugContainerName string, err error) {
	debugName := generateDebugName("debugger")
	ec := buildEphemeralContainer(debugName, image, command, targetContainer, privileged)

	pod, err := client.Typed.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("get pod: %w", err)
	}

	pod.Spec.EphemeralContainers = append(pod.Spec.EphemeralContainers, ec)

	_, err = client.Typed.CoreV1().Pods(namespace).UpdateEphemeralContainers(
		ctx, podName, pod, metav1.UpdateOptions{},
	)
	if err != nil {
		return "", fmt.Errorf("update ephemeral containers: %w", err)
	}

	if onStatus != nil {
		onStatus("Waiting for container...")
	}

	if err := waitForContainerRunning(ctx, client.Typed, podName, debugName, namespace); err != nil {
		return "", err
	}

	return debugName, nil
}

// PrepareNodeDebug performs the node-debug pre-flight: it creates a privileged
// debug pod pinned to the node, then waits for its container to reach Running.
// It returns the created pod name, the namespace it was created in, and the
// debug container name so the caller can attach. The caller owns deleting the
// pod (see DeleteNodeDebugPod) on pane-close / quit.
func PrepareNodeDebug(ctx context.Context, client *Client, nodeName, image string, command []string, privileged bool, onStatus func(string)) (podName, namespace, containerName string, err error) {
	prefix := "ktui-debug-" + nodeName
	if len(prefix) > 57 { // leave room for "-xxxxx"
		prefix = prefix[:57]
	}
	debugName := generateDebugName(prefix)
	pod := buildDebugNodePod(debugName, nodeName, image, command, privileged)

	ns := client.Namespace

	created, err := client.Typed.CoreV1().Pods(ns).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		return "", "", "", fmt.Errorf("create debug pod: %w", err)
	}

	if onStatus != nil {
		onStatus("Waiting for pod...")
	}

	cName := created.Spec.Containers[0].Name

	if err := waitForContainerRunning(ctx, client.Typed, created.Name, cName, ns); err != nil {
		// Best-effort cleanup: the pod was created but never came up, so it would
		// otherwise leak. Fire-and-forget so we do not block the caller.
		go func() { _ = DeleteNodeDebugPod(context.Background(), client, created.Name, ns) }()
		return "", "", "", err
	}

	return created.Name, ns, cName, nil
}

// DeleteNodeDebugPod deletes a node-debug pod (created by PrepareNodeDebug) with
// a zero grace period. It is best-effort: callers typically fire it in a
// goroutine on pane-close / app-quit and ignore the error.
func DeleteNodeDebugPod(ctx context.Context, client *Client, podName, namespace string) error {
	return client.Typed.CoreV1().Pods(namespace).Delete(
		ctx, podName,
		metav1.DeleteOptions{GracePeriodSeconds: new(int64)},
	)
}

// waitForContainerRunning polls until the named container is in Running state.
func waitForContainerRunning(ctx context.Context, typed kubernetes.Interface, podName, containerName, namespace string) error {
	timeout := time.After(60 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timeout waiting for container %s to start", containerName)
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			pod, err := typed.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
			if err != nil {
				continue
			}
			// Check regular container statuses
			for _, cs := range pod.Status.ContainerStatuses {
				if cs.Name == containerName {
					if cs.State.Running != nil {
						return nil
					}
					if cs.State.Terminated != nil {
						return fmt.Errorf("container %s terminated: %s (exit %d)", containerName, cs.State.Terminated.Reason, cs.State.Terminated.ExitCode)
					}
				}
			}
			// Check ephemeral container statuses
			for _, cs := range pod.Status.EphemeralContainerStatuses {
				if cs.Name == containerName {
					if cs.State.Running != nil {
						return nil
					}
					if cs.State.Terminated != nil {
						return fmt.Errorf("container %s terminated: %s (exit %d)", containerName, cs.State.Terminated.Reason, cs.State.Terminated.ExitCode)
					}
				}
			}
		}
	}
}

// attachURL builds the /attach subresource request URL for a pod/container.
func attachURL(typed kubernetes.Interface, podName, containerName, namespace string) *url.URL {
	return typed.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).
		SubResource("attach").
		VersionedParams(&corev1.PodAttachOptions{
			Stdin:     true,
			Stdout:    true,
			TTY:       true,
			Container: containerName,
		}, scheme.ParameterCodec).
		URL()
}

// NewAttachExecutor builds a SPDY executor for attaching to a (debug/ephemeral)
// container in a pod. The returned executor satisfies the minimal interface
// consumed by the session package and can be driven over in-memory pipes.
// Callers are responsible for any pre-flight work (creating the ephemeral
// container/pod and waiting for it to be Running) before attaching.
func NewAttachExecutor(client *Client, podName, containerName, namespace string) (remotecommand.Executor, error) {
	reqURL := attachURL(client.Typed, podName, containerName, namespace)
	return remotecommand.NewSPDYExecutor(client.Config, "POST", reqURL)
}
