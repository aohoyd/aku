package k8s

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
	"time"

	"k8s.io/client-go/rest"
)

func TestNewClientErrorsOnInvalidKubeconfig(t *testing.T) {
	_, err := NewClient("/nonexistent/kubeconfig", "", "")
	if err == nil {
		t.Fatal("expected error for nonexistent kubeconfig")
	}
}

func TestClientWithNamespace(t *testing.T) {
	wh := &warningHandler{}
	c := &Client{Namespace: "default", Context: "test", WarningHandler: wh}
	updated := c.WithNamespace("kube-system")
	if updated.Namespace != "kube-system" {
		t.Fatalf("expected 'kube-system', got '%s'", updated.Namespace)
	}
	if c.Namespace != "default" {
		t.Fatal("original client namespace should be unchanged")
	}
	if updated.Context != "test" {
		t.Fatal("context should be preserved")
	}
	if updated.WarningHandler != wh {
		t.Fatal("WarningHandler should be preserved")
	}
}

func TestCheckHealthSuccess(t *testing.T) {
	// Spin up a minimal fake API server that responds to /version.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"major":"1","minor":"30","gitVersion":"v1.30.0"}`)
	}))
	defer srv.Close()

	c := &Client{
		Config: &rest.Config{Host: srv.URL},
	}
	if !CheckHealth(context.Background(), c) {
		t.Fatal("expected CheckHealth to return true with fake server")
	}
}

func TestCheckHealthNilClient(t *testing.T) {
	if CheckHealth(context.Background(), nil) {
		t.Fatal("expected CheckHealth to return false for nil client")
	}
}

func TestCheckHealthNilConfig(t *testing.T) {
	c := &Client{Config: nil}
	if CheckHealth(context.Background(), c) {
		t.Fatal("expected CheckHealth to return false when Config is nil")
	}
}

func TestCheckHealthCancelledContext(t *testing.T) {
	c := &Client{
		Config: &rest.Config{Host: "https://192.0.2.1:1"}, // unreachable
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	if CheckHealth(ctx, c) {
		t.Fatal("expected CheckHealth to return false for cancelled context")
	}
}

func TestCheckHealthUnreachableNoLeak(t *testing.T) {
	// Bind a port and immediately close the listener so connects are refused.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()

	c := &Client{
		Config: &rest.Config{Host: "http://" + addr},
	}

	goroutinesBefore := runtime.NumGoroutine()

	// Call CheckHealth several times against the unreachable server.
	for i := 0; i < 5; i++ {
		result := CheckHealth(context.Background(), c)
		if result {
			t.Fatal("expected CheckHealth to return false for unreachable server")
		}
	}

	// Give a moment for any leaked goroutines to show up.
	time.Sleep(100 * time.Millisecond)
	runtime.GC()

	goroutinesAfter := runtime.NumGoroutine()
	// Allow a small margin (2) for background runtime goroutines.
	if goroutinesAfter > goroutinesBefore+2 {
		t.Fatalf("goroutine leak detected: before=%d, after=%d", goroutinesBefore, goroutinesAfter)
	}
}
