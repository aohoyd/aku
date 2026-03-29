package k8s

import (
	"context"
	"io"
	"os"
	"os/signal"

	tea "charm.land/bubbletea/v2"
	"github.com/aohoyd/aku/internal/msgs"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
)

// execCommand implements tea.ExecCommand for exec-ing into containers via SPDY.
type execCommand struct {
	stdin     io.Reader
	stdout    io.Writer
	stderr    io.Writer
	client    *Client
	podName   string
	container string
	namespace string
	command   []string
}

func (e *execCommand) SetStdin(r io.Reader)  { e.stdin = r }
func (e *execCommand) SetStdout(w io.Writer) { e.stdout = w }
func (e *execCommand) SetStderr(w io.Writer) { e.stderr = w }

// Run executes the exec workflow: set raw terminal and attach via SPDY exec subresource.
func (e *execCommand) Run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	return execContainer(ctx, e.stdin, e.stdout, e.stderr, e.client.Config, e.client.Typed, e.podName, e.container, e.namespace, e.command)
}

// execContainer builds the exec subresource URL and streams via SPDY.
func execContainer(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, restConfig *rest.Config, typed kubernetes.Interface, podName, containerName, namespace string, command []string) error {
	execURL := typed.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Stdin:     true,
			Stdout:    true,
			TTY:       true,
			Container: containerName,
			Command:   command,
		}, scheme.ParameterCodec).
		URL()

	return spdyStream(ctx, stdin, stdout, restConfig, execURL)
}

// ExecCmd returns a tea.Cmd that suspends the TUI and execs into a container via SPDY.
func ExecCmd(client *Client, podName, containerName, namespace string, command []string) tea.Cmd {
	ec := &execCommand{
		client:    client,
		podName:   podName,
		container: containerName,
		namespace: namespace,
		command:   command,
	}
	return tea.Exec(ec, func(err error) tea.Msg {
		if err != nil {
			return msgs.ActionResultMsg{Err: err}
		}
		return msgs.ActionResultMsg{ActionID: "exec:" + podName}
	})
}
