package k8s

import (
	"context"
	"io"
	"net/url"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
)

func TestContextPropagationSignatures(t *testing.T) {
	// Compile-time verification that execContainer, attachContainer, and spdyStream
	// all accept context.Context as their first parameter. If someone removes the
	// context parameter, this test will fail to compile.
	var (
		_ func(context.Context, io.Reader, io.Writer, io.Writer, *rest.Config, kubernetes.Interface, string, string, string, []string) error = execContainer
		_ func(context.Context, io.Reader, io.Writer, io.Writer, *rest.Config, kubernetes.Interface, string, string, string) error            = attachContainer
		_ func(context.Context, io.Reader, io.Writer, *rest.Config, *url.URL) error                                                          = spdyStream
	)
}

func TestExecCmdReturnsNonNilCmd(t *testing.T) {
	// ExecCmd should return a non-nil tea.Cmd regardless of client validity.
	// We construct a minimal Client with a typed clientset created from a
	// rest.Config pointing at a dummy host. The Cmd is never executed, so the
	// host need not be reachable.
	cfg := &rest.Config{Host: "http://localhost:0"}
	typed, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		t.Fatalf("failed to create typed client: %v", err)
	}
	client := &Client{
		Config: cfg,
		Typed:  typed,
	}

	cmd := ExecCmd(client, "my-pod", "my-container", "default", []string{"/bin/sh"})
	if cmd == nil {
		t.Fatal("ExecCmd returned nil tea.Cmd, expected non-nil")
	}
}

func TestExecContainerURLConstruction(t *testing.T) {
	// Verify that the exec URL built using the same pattern as execContainer
	// has the correct path and query parameters. We create a real typed
	// clientset backed by a dummy host so that RESTClient() is non-nil.
	cfg := &rest.Config{Host: "http://localhost:0"}
	typed, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		t.Fatalf("failed to create typed client: %v", err)
	}

	podName := "test-pod"
	containerName := "test-container"
	namespace := "test-ns"
	command := []string{"/bin/sh", "-c", "echo hello"}

	// Build the URL using the same pattern as execContainer.
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

	// Verify the URL path contains the expected segments.
	expectedPath := "/api/v1/namespaces/" + namespace + "/pods/" + podName + "/exec"
	if execURL.Path != expectedPath {
		t.Fatalf("unexpected URL path:\n  got:  %s\n  want: %s", execURL.Path, expectedPath)
	}

	// Verify query parameters.
	q := execURL.Query()

	if q.Get("container") != containerName {
		t.Fatalf("container param: got %q, want %q", q.Get("container"), containerName)
	}
	if q.Get("stdin") != "true" {
		t.Fatalf("stdin param: got %q, want %q", q.Get("stdin"), "true")
	}
	if q.Get("stdout") != "true" {
		t.Fatalf("stdout param: got %q, want %q", q.Get("stdout"), "true")
	}
	if q.Get("tty") != "true" {
		t.Fatalf("tty param: got %q, want %q", q.Get("tty"), "true")
	}

	// Verify all command segments are present.
	commands := q["command"]
	if len(commands) != len(command) {
		t.Fatalf("command params: got %d entries, want %d", len(commands), len(command))
	}
	for i, c := range command {
		if commands[i] != c {
			t.Fatalf("command[%d]: got %q, want %q", i, commands[i], c)
		}
	}
}
