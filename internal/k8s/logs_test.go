package k8s

import (
	"context"
	"testing"
	"time"
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
