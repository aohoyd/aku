package k8s

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func TestStreamLogsNilClient(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	ch, err := StreamLogs(ctx, nil, "pod", "container", "ns", DefaultLogOptions(60))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, ok := <-ch
	if ok {
		t.Fatal("expected channel to be closed for nil client")
	}
}

func TestStreamLogsContextCancelClosesChannel(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	ch, err := StreamLogs(ctx, nil, "pod", "container", "ns", DefaultLogOptions(60))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cancel()
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected channel to be closed")
		}
	case <-time.After(time.Second):
		t.Fatal("channel should have been closed promptly after cancel")
	}
}

// newLogServer creates an httptest.Server that serves pod log lines.
// The handler receives the http.ResponseWriter and *http.Request for
// full control over what gets streamed.
func newLogServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *Client) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	typedClient, err := kubernetes.NewForConfig(&rest.Config{Host: srv.URL})
	if err != nil {
		t.Fatalf("failed to create typed client: %v", err)
	}
	return srv, &Client{
		Typed:  typedClient,
		Config: &rest.Config{Host: srv.URL},
	}
}

func TestStreamLogsReceivesLines(t *testing.T) {
	expectedLines := []string{
		"2024-01-01T00:00:00Z INFO starting up",
		"2024-01-01T00:00:01Z INFO ready",
		"2024-01-01T00:00:02Z WARN something happened",
		"2024-01-01T00:00:03Z INFO shutting down",
	}

	_, client := newLogServer(t, func(w http.ResponseWriter, r *http.Request) {
		// Verify the request path looks like a pod log request.
		if !strings.Contains(r.URL.Path, "/log") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		for _, line := range expectedLines {
			fmt.Fprintln(w, line)
		}
	})

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	opts := LogOptions{Follow: false}
	ch, err := StreamLogs(ctx, client, "test-pod", "app", "default", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var received []string
	for line := range ch {
		received = append(received, line)
	}

	if len(received) != len(expectedLines) {
		t.Fatalf("expected %d lines, got %d: %v", len(expectedLines), len(received), received)
	}
	for i, want := range expectedLines {
		if received[i] != want {
			t.Errorf("line %d: got %q, want %q", i, received[i], want)
		}
	}
}

func TestStreamLogsEmptyResponse(t *testing.T) {
	_, client := newLogServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		// Write nothing — empty log stream.
	})

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	opts := LogOptions{Follow: false}
	ch, err := StreamLogs(ctx, client, "test-pod", "app", "default", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var received []string
	for line := range ch {
		received = append(received, line)
	}

	if len(received) != 0 {
		t.Fatalf("expected 0 lines for empty response, got %d: %v", len(received), received)
	}
}

func TestStreamLogsCancelDuringActiveStream(t *testing.T) {
	// This server writes one line immediately, then blocks until the request
	// context is cancelled (simulating a long-lived follow stream).
	wrote := make(chan struct{})
	_, client := newLogServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "first line")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		close(wrote)
		// Block until the client disconnects.
		<-r.Context().Done()
	})

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	opts := LogOptions{Follow: true}
	ch, err := StreamLogs(ctx, client, "test-pod", "app", "default", opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Wait for the first line to be served.
	select {
	case <-wrote:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for server to write first line")
	}

	// Read the first line.
	select {
	case line, ok := <-ch:
		if !ok {
			t.Fatal("channel closed before receiving first line")
		}
		if line != "first line" {
			t.Fatalf("expected 'first line', got %q", line)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for first line on channel")
	}

	// Cancel the context to stop streaming.
	cancel()

	// The channel should close promptly.
	select {
	case _, ok := <-ch:
		if ok {
			// It's acceptable to receive a stream error line, but eventually
			// the channel must close.
			select {
			case _, ok2 := <-ch:
				if ok2 {
					t.Fatal("expected channel to close after cancel")
				}
			case <-time.After(3 * time.Second):
				t.Fatal("channel did not close after cancel")
			}
		}
		// Channel is closed — good.
	case <-time.After(3 * time.Second):
		t.Fatal("channel did not close after cancel")
	}
}
