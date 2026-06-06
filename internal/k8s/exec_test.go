package k8s

import (
	"testing"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func TestExecURLConstruction(t *testing.T) {
	// Verify that the exec URL built by execURL has the correct path and query
	// parameters. We create a real typed clientset backed by a dummy host so
	// that RESTClient() is non-nil.
	cfg := &rest.Config{Host: "http://localhost:0"}
	typed, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		t.Fatalf("failed to create typed client: %v", err)
	}

	podName := "test-pod"
	containerName := "test-container"
	namespace := "test-ns"
	command := []string{"/bin/sh", "-c", "echo hello"}

	// Build the URL using the live execURL helper.
	url := execURL(typed, podName, containerName, namespace, command)

	// Verify the URL path contains the expected segments.
	expectedPath := "/api/v1/namespaces/" + namespace + "/pods/" + podName + "/exec"
	if url.Path != expectedPath {
		t.Fatalf("unexpected URL path:\n  got:  %s\n  want: %s", url.Path, expectedPath)
	}

	// Verify query parameters.
	q := url.Query()

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
