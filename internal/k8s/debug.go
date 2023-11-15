package k8s

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/aohoyd/aku/internal/msgs"
	"github.com/muesli/cancelreader"
	"golang.org/x/term"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
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
	}
	if targetContainer != "" {
		ec.TargetContainerName = targetContainer
	}
	if privileged {
		p := true
		ec.SecurityContext = &corev1.SecurityContext{Privileged: &p}
	}
	return ec
}

// buildDebugNodePod creates a Pod spec for node-level debugging.
func buildDebugNodePod(name, nodeName, image string, command []string) *corev1.Pod {
	privileged := true
	hostPathType := corev1.HostPathDirectory
	return &corev1.Pod{
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
					SecurityContext: &corev1.SecurityContext{
						Privileged: &privileged,
					},
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
}

// debugCommand implements tea.ExecCommand for attaching to debug containers.
type debugCommand struct {
	stdin      io.Reader
	stdout     io.Writer
	stderr     io.Writer
	client     *Client
	podName    string
	container  string
	namespace  string
	image      string
	command    []string
	privileged bool
	nodeMode   bool
	nodeName   string
}

func (d *debugCommand) SetStdin(r io.Reader)  { d.stdin = r }
func (d *debugCommand) SetStdout(w io.Writer) { d.stdout = w }
func (d *debugCommand) SetStderr(w io.Writer) { d.stderr = w }

// Run executes the debug workflow: create ephemeral container or debug pod, then attach.
func (d *debugCommand) Run() error {
	ctx := context.Background()

	if d.nodeMode {
		return d.runNodeDebug(ctx)
	}
	return d.runPodDebug(ctx)
}

func (d *debugCommand) runPodDebug(ctx context.Context) error {
	debugName := generateDebugName("debugger")
	ec := buildEphemeralContainer(debugName, d.image, d.command, d.container, d.privileged)

	// Get current pod
	pod, err := d.client.Typed.CoreV1().Pods(d.namespace).Get(ctx, d.podName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get pod: %w", err)
	}

	// Append ephemeral container
	pod.Spec.EphemeralContainers = append(pod.Spec.EphemeralContainers, ec)

	// Update ephemeral containers
	_, err = d.client.Typed.CoreV1().Pods(d.namespace).UpdateEphemeralContainers(
		ctx, d.podName, pod, metav1.UpdateOptions{},
	)
	if err != nil {
		return fmt.Errorf("update ephemeral containers: %w", err)
	}

	// Wait for the container to be running
	if err := waitForContainerRunning(ctx, d.client.Typed, d.podName, debugName, d.namespace); err != nil {
		return err
	}

	// Attach
	return attachContainer(d.stdin, d.stdout, d.stderr, d.client.Config, d.client.Typed, d.podName, debugName, d.namespace)
}

func (d *debugCommand) runNodeDebug(ctx context.Context) error {
	prefix := "ktui-debug-" + d.nodeName
	if len(prefix) > 57 { // leave room for "-xxxxx"
		prefix = prefix[:57]
	}
	debugName := generateDebugName(prefix)
	pod := buildDebugNodePod(debugName, d.nodeName, d.image, d.command)

	ns := d.client.Namespace

	created, err := d.client.Typed.CoreV1().Pods(ns).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("create debug pod: %w", err)
	}

	// Clean up the debug pod in the background to avoid blocking TUI resume.
	defer func() {
		go func() {
			grace := int64(0)
			_ = d.client.Typed.CoreV1().Pods(ns).Delete(
				context.Background(), created.Name,
				metav1.DeleteOptions{GracePeriodSeconds: &grace},
			)
		}()
	}()

	containerName := created.Spec.Containers[0].Name

	if err := waitForContainerRunning(ctx, d.client.Typed, created.Name, containerName, ns); err != nil {
		return err
	}

	return attachContainer(d.stdin, d.stdout, d.stderr, d.client.Config, d.client.Typed, created.Name, containerName, ns)
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

// attachContainer sets the terminal to raw mode and attaches to the container via SPDY.
func attachContainer(stdin io.Reader, stdout, stderr io.Writer, restConfig *rest.Config, typed kubernetes.Interface, podName, containerName, namespace string) error {
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return fmt.Errorf("set raw terminal: %w", err)
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	attachURL := typed.CoreV1().RESTClient().Post().
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

	executor, err := remotecommand.NewSPDYExecutor(restConfig, "POST", attachURL)
	if err != nil {
		return fmt.Errorf("create executor: %w", err)
	}

	tsq := newTermSizeQueue()
	defer tsq.stop()

	inR, inW := io.Pipe()

	cr, crErr := cancelreader.NewReader(stdin)
	if crErr != nil {
		return fmt.Errorf("create cancel reader: %w", crErr)
	}

	// Use a WaitGroup so cleanup can wait for the goroutine to fully exit,
	// ensuring no competing Read on stdin when Bubble Tea resumes.
	var stdinWg sync.WaitGroup
	stdinWg.Go(func() {
		defer inW.Close()
		buf := make([]byte, 32*1024)
		for {
			nr, readErr := cr.Read(buf)
			if nr > 0 {
				if _, writeErr := inW.Write(buf[:nr]); writeErr != nil {
					return
				}
			}
			if readErr != nil {
				return
			}
		}
	})

	err = executor.StreamWithContext(context.Background(), remotecommand.StreamOptions{
		Stdin:             inR,
		Stdout:            stdout,
		Tty:               true,
		TerminalSizeQueue: tsq,
	})

	// Stop the stdin forwarding goroutine. Two possible blocked states:
	// 1. cr.Read() — unblocked by cr.Cancel() (uses select(2) + pipe on macOS)
	// 2. inW.Write() — unblocked by inR.Close() (io.Pipe is unbuffered,
	//    so Write blocks when SPDY stops reading from inR)
	cr.Cancel()
	inR.Close()
	stdinWg.Wait()
	cr.Close()

	if err != nil && errors.Is(err, io.EOF) {
		return nil
	}
	return err
}

// termSizeQueue implements remotecommand.TerminalSizeQueue with persistent
// SIGWINCH handling and clean cancellation.
type termSizeQueue struct {
	ch   chan os.Signal
	done chan struct{}
	once sync.Once
}

func newTermSizeQueue() *termSizeQueue {
	q := &termSizeQueue{
		ch:   make(chan os.Signal, 1),
		done: make(chan struct{}),
	}
	signal.Notify(q.ch, syscall.SIGWINCH)
	// Send an initial signal to return the current size immediately.
	q.ch <- syscall.SIGWINCH
	return q
}

func (q *termSizeQueue) Next() *remotecommand.TerminalSize {
	select {
	case <-q.ch:
	case <-q.done:
		return nil
	}
	w, h, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		return nil
	}
	return &remotecommand.TerminalSize{Width: uint16(w), Height: uint16(h)}
}

func (q *termSizeQueue) stop() {
	q.once.Do(func() {
		signal.Stop(q.ch)
		close(q.done)
	})
}

// DebugCmd returns a tea.Cmd that suspends the TUI and attaches to an ephemeral debug container.
func DebugCmd(client *Client, podName, containerName, ns, image string, command []string, privileged bool) tea.Cmd {
	dc := &debugCommand{
		client:     client,
		podName:    podName,
		container:  containerName,
		namespace:  ns,
		image:      image,
		command:    command,
		privileged: privileged,
	}
	return tea.Exec(dc, func(err error) tea.Msg {
		if err != nil {
			return msgs.ActionResultMsg{Err: err}
		}
		return msgs.ActionResultMsg{ActionID: "debug:" + podName}
	})
}

// DebugNodeCmd returns a tea.Cmd that suspends the TUI and attaches to a debug pod on a node.
func DebugNodeCmd(client *Client, nodeName, image string, command []string) tea.Cmd {
	dc := &debugCommand{
		client:   client,
		nodeMode: true,
		nodeName: nodeName,
		image:    image,
		command:  command,
	}
	return tea.Exec(dc, func(err error) tea.Msg {
		if err != nil {
			return msgs.ActionResultMsg{Err: err}
		}
		return msgs.ActionResultMsg{ActionID: "debug-node:" + nodeName}
	})
}
