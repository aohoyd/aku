package k8s

import "testing"

func TestActivePortForwardStopIdempotent(t *testing.T) {
	stop := make(chan struct{})
	done := make(chan struct{})
	errCh := make(chan error, 1)

	// Simulate a running port-forward that stops when stop is closed
	go func() {
		<-stop
		close(done)
	}()

	apf := &ActivePortForward{
		ready: make(chan struct{}),
		done:  done,
		errCh: errCh,
		stop:  stop,
	}

	apf.Stop()  // First call should work
	apf.Stop()  // Second call should not panic
}
