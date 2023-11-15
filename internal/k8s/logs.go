package k8s

import (
	"bufio"
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
)

// LogOptions configures log streaming behaviour.
type LogOptions struct {
	TailLines    *int64
	SinceSeconds *int64
	Follow       bool
}

// DefaultLogOptions returns sensible defaults: tail 200 lines, follow enabled.
func DefaultLogOptions() LogOptions {
	tailLines := int64(200)
	return LogOptions{
		TailLines: &tailLines,
		Follow:    true,
	}
}

// StreamLogs opens a follow-stream for a pod's container and returns a channel of log lines.
// The caller should cancel the context to stop streaming.
func StreamLogs(ctx context.Context, client *Client, podName, containerName, namespace string, opts LogOptions) (<-chan string, error) {
	if client == nil || client.Typed == nil {
		ch := make(chan string)
		close(ch)
		return ch, nil
	}

	podLogOpts := &corev1.PodLogOptions{
		Container: containerName,
		Follow:    opts.Follow,
		TailLines: opts.TailLines,
	}

	if opts.SinceSeconds != nil {
		podLogOpts.SinceSeconds = opts.SinceSeconds
	}

	req := client.Typed.CoreV1().Pods(namespace).GetLogs(podName, podLogOpts)
	stream, err := req.Stream(ctx)
	if err != nil {
		return nil, err
	}

	ch := make(chan string, 100)
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			stream.Close()
		case <-done:
		}
	}()
	go func() {
		defer close(done)
		defer close(ch)
		defer stream.Close()
		scanner := bufio.NewScanner(stream)
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			case ch <- scanner.Text():
			}
		}
		if err := scanner.Err(); err != nil {
			select {
			case ch <- fmt.Sprintf("[stream error: %v]", err):
			default:
			}
		}
	}()

	return ch, nil
}
