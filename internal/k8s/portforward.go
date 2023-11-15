package k8s

import (
	"context"
	"fmt"
	"os/exec"
)

// PortForward starts kubectl port-forward in the background.
// Returns a cancel function to stop the port-forward.
func PortForward(ctx context.Context, podName, namespace string, localPort, remotePort int) (context.CancelFunc, error) {
	ctx, cancel := context.WithCancel(ctx)
	portSpec := fmt.Sprintf("%d:%d", localPort, remotePort)
	cmd := exec.CommandContext(ctx, "kubectl", "port-forward", "-n", namespace, podName, portSpec)

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, err
	}

	// Reap the process in the background.
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = cmd.Wait()
	}()

	// Return a cancel function that waits for the process to exit,
	// ensuring the port is released before the caller proceeds.
	cancelAndWait := func() {
		cancel()
		<-done
	}
	return cancelAndWait, nil
}
