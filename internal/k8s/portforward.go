package k8s

import (
	"fmt"
	"io"
	"net/http"

	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

// ActivePortForward represents a running port-forward session.
// It implements msgs.PortForwardHandle.
type ActivePortForward struct {
	ready <-chan struct{}
	done  <-chan struct{}
	errCh <-chan error
	stop  chan struct{}
}

// Stop signals the port-forward to stop and waits for it to finish.
// It is safe to call multiple times.
func (a *ActivePortForward) Stop() {
	select {
	case <-a.stop:
	default:
		close(a.stop)
	}
	<-a.done
}

// Ready returns a channel that is closed when the port-forward is ready.
func (a *ActivePortForward) Ready() <-chan struct{} { return a.ready }

// Done returns a channel that is closed when the port-forward has terminated.
func (a *ActivePortForward) Done() <-chan struct{} { return a.done }

// Err returns a channel that receives an error if the port-forward fails.
func (a *ActivePortForward) Err() <-chan error { return a.errCh }

// PortForward starts a native port-forward to the given pod using SPDY.
func PortForward(client *Client, podName, namespace string, localPort, remotePort int) (*ActivePortForward, error) {
	reqURL := client.Typed.CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace(namespace).
		Name(podName).
		SubResource("portforward").
		URL()

	transport, upgrader, err := spdy.RoundTripperFor(client.Config)
	if err != nil {
		return nil, fmt.Errorf("creating SPDY round-tripper: %w", err)
	}

	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, reqURL)

	stopCh := make(chan struct{})
	readyCh := make(chan struct{})
	doneCh := make(chan struct{})
	errCh := make(chan error, 1)

	ports := []string{fmt.Sprintf("%d:%d", localPort, remotePort)}

	fw, err := portforward.New(dialer, ports, stopCh, readyCh, io.Discard, io.Discard)
	if err != nil {
		return nil, fmt.Errorf("creating port-forwarder: %w", err)
	}

	go func() {
		defer close(doneCh)
		if err := fw.ForwardPorts(); err != nil {
			errCh <- err
		}
		close(errCh)
	}()

	return &ActivePortForward{
		ready: readyCh,
		done:  doneCh,
		errCh: errCh,
		stop:  stopCh,
	}, nil
}
