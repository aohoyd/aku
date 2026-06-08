// Package session provides background, embeddable terminal sessions that run a
// client-go SPDY stream over in-memory pipes. Unlike the fullscreen tea.Exec
// path, a Terminal does not touch the real TTY: keystrokes are fed in via
// Write, output is read off the Out channel, and pane resizes are delivered via
// Resize. This lets the Bubble Tea program keep the real terminal while many
// sessions run concurrently as panes.
package session

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"

	"k8s.io/client-go/tools/remotecommand"
	utilexec "k8s.io/client-go/util/exec"
)

// Executor is the minimal surface of remotecommand.Executor the session needs.
// *remotecommand.SPDYExecutor (the value returned by NewSPDYExecutor) satisfies
// it, and tests can supply a fake without a real cluster.
type Executor interface {
	StreamWithContext(ctx context.Context, opts remotecommand.StreamOptions) error
}

// Terminal is a single background SPDY shell session. Write and Close are safe
// to call from any goroutine while a consumer ranges over Out(); Resize must be
// called from a single goroutine (see Resize). Err(), ExitCode(), and Out() are
// safe to call from any goroutine (Err/ExitCode are mutex-guarded; Out returns a
// channel) — though Err/ExitCode are only meaningful after Done() is closed.
//
// Write is non-blocking: it hands the keystrokes off to a buffered input channel
// drained by a dedicated goroutine, so it is safe to call from the Bubble Tea UI
// goroutine even when the underlying SPDY stdin copy is stalled by network
// backpressure (slow API server, a pod not reading stdin). The only bound on
// Write is a single channel hand-off; once the buffer is full Write drops the
// chunk rather than block the UI, and after teardown it returns immediately.
type Terminal struct {
	id string

	stdinW *io.PipeWriter                  // keystrokes → shell
	input  chan []byte                     // Write callers → input drain goroutine
	out    chan []byte                     // shell stdout bytes → UI
	resize chan remotecommand.TerminalSize // pane resize → size queue

	cancel context.CancelFunc
	done   chan struct{} // closed when the stream goroutine returns

	mu       sync.Mutex
	exitErr  error
	exitCode int
	closed   bool // guards idempotent Close
}

// Graceful-exit tuning for TerminateGracefully. These bound how hard and how
// long we try to coax the remote shell into exiting on its own before the
// caller falls back to a hard Close().
const (
	// gracefulExitGrace is the post-burst time to wait for the shell to die. It
	// is the final time.After bound in TerminateGracefully, applied AFTER the
	// control-byte burst phase; the worst-case total block is therefore this
	// value plus the burst phase (~(1+gracefulExitEOTBurst) × gracefulExitInterval).
	gracefulExitGrace = 400 * time.Millisecond
	// gracefulExitEOTBurst is the number of Ctrl-D (EOT) presses sent to peel
	// the nested shells of the default exec command (sh → subshell → bash).
	gracefulExitEOTBurst = 4
	// gracefulExitInterval is the pause between control-byte presses.
	gracefulExitInterval = 30 * time.Millisecond
)

// DefaultGraceTimeout is the recommended grace period to pass to
// TerminateGracefully. "Default" reflects that it is the package's standard
// post-burst wait (gracefulExitGrace) exported for callers (e.g. pane-close/quit
// wiring) so they do not invent their own timeout; callers may pass a different
// value, but there is no other named alternative in this package. Note it bounds
// only the wait AFTER the control-byte burst, not the total method duration.
const DefaultGraceTimeout = gracefulExitGrace

// Start opens the SPDY stream against exec in a background goroutine and returns
// a live Terminal. The stream runs until it ends on its own (shell exit) or
// Close is called. id is an opaque identifier echoed back by ID().
func Start(exec Executor, id string) *Terminal {
	ctx, cancel := context.WithCancel(context.Background())

	inR, inW := io.Pipe()

	t := &Terminal{
		id:     id,
		stdinW: inW,
		input:  make(chan []byte, 256),
		out:    make(chan []byte, 64),
		resize: make(chan remotecommand.TerminalSize, 1),
		cancel: cancel,
		done:   make(chan struct{}),
	}

	tsq := &sizeQueue{resize: t.resize, ctx: ctx}
	stdout := &chanWriter{ctx: ctx, out: t.out}

	// Input drain goroutine: decouples Write callers (the UI goroutine and the
	// reply pump) from SPDY stdin latency. It writes each buffered chunk to the
	// stdin pipe; on teardown inR.Close()/stdinW.Close() unblocks a stalled
	// stdinW.Write with ErrClosedPipe, and the t.done select abandons the loop so
	// the goroutine can never leak. t.input is never closed (Close only cancels
	// the context and closes stdinW), so the goroutine exits via <-t.done once the
	// stream is torn down, or when stdinW.Write returns ErrClosedPipe after Close.
	go func() {
		for {
			select {
			case buf := <-t.input:
				if _, err := t.stdinW.Write(buf); err != nil {
					// stdin pipe is gone (teardown); nothing more can be written.
					return
				}
			case <-t.done:
				return
			}
		}
	}()

	go func() {
		defer close(t.done)
		// Closing out must happen exactly once, and only after the stream
		// goroutine (the sole producer) has returned.
		defer close(t.out)
		// Ensure the stdin pipe is unblocked/torn down on exit.
		defer inR.Close()

		err := exec.StreamWithContext(ctx, remotecommand.StreamOptions{
			Stdin:             inR,
			Stdout:            stdout,
			Tty:               true,
			TerminalSizeQueue: tsq,
		})

		// A bare io.EOF is the normal end-of-stream for a TTY session. Use an
		// identity comparison so only the naked sentinel is treated as a clean
		// exit — an error that merely *wraps* io.EOF carries real failure context
		// and must be preserved.
		if err == io.EOF {
			err = nil
		}

		code := 0
		if err != nil {
			var codeErr utilexec.CodeExitError
			if errors.As(err, &codeErr) {
				code = codeErr.Code
			}
		}

		t.mu.Lock()
		t.exitErr = err
		t.exitCode = code
		t.mu.Unlock()
	}()

	return t
}

// Write forwards keystrokes to the shell's stdin without blocking the caller on
// SPDY/network latency. The bytes are copied (callers — e.g. the reply pump —
// reuse their buffer) and handed to the buffered input channel drained by the
// input goroutine. Write blocks only for a single channel hand-off; if the
// buffer is full it drops the chunk rather than freeze the UI goroutine, and
// after teardown it returns io.ErrClosedPipe immediately. It is safe to call
// from the Bubble Tea update goroutine.
func (t *Terminal) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	buf := make([]byte, len(p))
	copy(buf, p)

	select {
	case t.input <- buf:
		return len(p), nil
	case <-t.done:
		return 0, io.ErrClosedPipe
	default:
		// Buffer saturated: drop rather than block the UI goroutine. Report the
		// bytes as accepted so callers do not treat backpressure as a hard error.
		return len(p), nil
	}
}

// Resize delivers a new pane size to the terminal size queue. It never blocks:
// if a pending size has not yet been consumed it is replaced with the latest.
//
// Resize must be called from a single goroutine (in practice the UI goroutine).
// The drain/refill loop below is safe against the stream consumer draining the
// queue concurrently, but two concurrent Resize callers could in theory ping-pong
// on the one-slot channel; single-caller use sidesteps that entirely.
func (t *Terminal) Resize(w, h int) {
	size := remotecommand.TerminalSize{Width: uint16(w), Height: uint16(h)}
	for {
		select {
		case t.resize <- size:
			return
		default:
			// Channel full: drop the stale pending size and retry so the
			// queue always reflects the most recent dimensions.
			select {
			case <-t.resize:
			default:
			}
		}
	}
}

// Out is the stream of stdout chunks. It is closed once the session ends.
func (t *Terminal) Out() <-chan []byte { return t.out }

// Done is closed when the underlying stream goroutine has returned.
func (t *Terminal) Done() <-chan struct{} { return t.done }

// Err returns the stream's terminal error, or nil on clean exit. It is only
// meaningful after Done() is closed.
func (t *Terminal) Err() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.exitErr
}

// ExitCode returns the shell exit code (0 on clean exit). It is only meaningful
// after Done() is closed.
func (t *Terminal) ExitCode() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.exitCode
}

// ID returns the session's opaque identifier.
func (t *Terminal) ID() string { return t.id }

// Close tears down the session: it cancels the stream context and closes the
// stdin pipe so the stream goroutine unblocks and exits. It is idempotent and
// does not block on the stream finishing; callers can wait on Done() if needed.
func (t *Terminal) Close() {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return
	}
	t.closed = true
	t.mu.Unlock()

	t.cancel()
	// Closing the write end signals EOF to the SPDY stream's stdin reader.
	_ = t.stdinW.Close()
}

// TerminateGracefully best-effort asks the remote shell to exit on its own by
// writing control bytes to its stdin: Ctrl-C (0x03) to clear any partial line,
// then a burst of Ctrl-D (0x04, EOT) to peel the nested shells of the default
// exec command (sh → subshell → bash). It waits up to grace for the stream to
// end and reports whether it did.
//
// It is advisory and never errors. It does NOT tear down the stream: the caller
// must still call Close() afterward to guarantee the session is released even if
// the shell ignored the control bytes. If the session has already ended (or been
// Close()d) at entry it returns true immediately and enqueues no bytes.
//
// This is best-effort across a concurrent Close(): the closed check below is a
// fast-path optimization, not a lock held across the writes. If Close() races in
// after the check, t.Write may still enqueue control bytes onto the internal
// input channel, but the drain goroutine's stdinW.Write hits the now-closed pipe
// (ErrClosedPipe) and drops them, so no byte ever reaches the shell. There is no
// data race (the channel/pipe ops are synchronized) and no observable effect.
//
// Limitation: a foreground full-screen TUI (vim, less, k9s, …) captures these
// control bytes itself rather than passing them to the shell, so the shell
// beneath it may still orphan. This is accepted; the hard Close() still runs.
func (t *Terminal) TerminateGracefully(grace time.Duration) bool {
	// Fast path: nothing to do if the stream is already gone or Close()d. Check
	// done (stream returned) and the closed guard (Close() called) and send no
	// bytes in either case.
	select {
	case <-t.done:
		return true
	default:
	}
	t.mu.Lock()
	closed := t.closed
	t.mu.Unlock()
	if closed {
		return true
	}

	// Ctrl-C to discard any partially typed line before asking the shell to exit.
	_, _ = t.Write([]byte{0x03})
	time.Sleep(gracefulExitInterval)

	// Burst of EOTs to peel each nested shell, bailing as soon as the stream ends.
	for i := 0; i < gracefulExitEOTBurst; i++ {
		select {
		case <-t.done:
			return true
		default:
		}
		_, _ = t.Write([]byte{0x04})
		time.Sleep(gracefulExitInterval)
	}

	select {
	case <-t.done:
		return true
	case <-time.After(grace):
		return false
	}
}

// chanWriter is an io.Writer that ships copies of written bytes onto out. It
// stops (and reports the write as successful) once ctx is cancelled so the
// SPDY stream can unwind without blocking on a full/abandoned channel.
type chanWriter struct {
	ctx context.Context
	out chan []byte
}

func (w *chanWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	// Copy: client-go reuses its read buffer, so the slice must not be aliased
	// past this call.
	buf := make([]byte, len(p))
	copy(buf, p)

	select {
	case w.out <- buf:
		return len(p), nil
	case <-w.ctx.Done():
		// Pretend the write succeeded; the session is shutting down and the
		// consumer is no longer reading.
		return len(p), nil
	}
}

// sizeQueue implements remotecommand.TerminalSizeQueue backed by a channel of
// pane sizes. Next blocks until a new size arrives or the session is cancelled,
// at which point it returns nil to signal the queue is exhausted.
type sizeQueue struct {
	resize chan remotecommand.TerminalSize
	ctx    context.Context
}

func (q *sizeQueue) Next() *remotecommand.TerminalSize {
	select {
	case s := <-q.resize:
		return &s
	case <-q.ctx.Done():
		return nil
	}
}
