package session

import (
	"bytes"
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"k8s.io/client-go/tools/remotecommand"
	utilexec "k8s.io/client-go/util/exec"
)

// fakeExecutor simulates a live SPDY shell without a cluster.
type fakeExecutor struct {
	// initialWrite, if non-nil, is written to opts.Stdout when the stream starts.
	initialWrite []byte
	// readStdin, if true, reads one chunk from opts.Stdin and records it.
	readStdin bool
	// readSize, if true, calls opts.TerminalSizeQueue.Next() once and records it.
	readSize bool
	// exitErr is returned when the stream ends (after ctx is cancelled, unless
	// returnImmediately is set).
	exitErr error
	// floodStdout, if > 0, writes that many small chunks to opts.Stdout (each in a
	// fresh slice) so a consumer that never reads Out() makes the bounded channel
	// saturate and chanWriter block on the send until ctx is cancelled.
	floodStdout int
	// returnImmediately makes StreamWithContext return exitErr without waiting
	// for ctx cancellation (simulates a shell that exits on its own).
	returnImmediately bool

	mu            sync.Mutex
	gotStdin      []byte
	gotSize       *remotecommand.TerminalSize
	streamStarted chan struct{}
}

func newFakeExecutor() *fakeExecutor {
	return &fakeExecutor{streamStarted: make(chan struct{})}
}

func (f *fakeExecutor) StreamWithContext(ctx context.Context, opts remotecommand.StreamOptions) error {
	close(f.streamStarted)

	if len(f.initialWrite) > 0 && opts.Stdout != nil {
		if _, err := opts.Stdout.Write(f.initialWrite); err != nil {
			return err
		}
	}

	if f.floodStdout > 0 && opts.Stdout != nil {
		// Each Write copies into the out channel; with no reader the channel fills
		// and a later Write blocks until ctx is cancelled (chanWriter returns then).
		for i := 0; i < f.floodStdout; i++ {
			if _, err := opts.Stdout.Write([]byte("x")); err != nil {
				return err
			}
		}
	}

	if f.readSize && opts.TerminalSizeQueue != nil {
		size := opts.TerminalSizeQueue.Next()
		f.mu.Lock()
		f.gotSize = size
		f.mu.Unlock()
	}

	if f.readStdin && opts.Stdin != nil {
		buf := make([]byte, 1024)
		n, _ := opts.Stdin.Read(buf)
		f.mu.Lock()
		f.gotStdin = append(f.gotStdin, buf[:n]...)
		f.mu.Unlock()
	}

	if f.returnImmediately {
		return f.exitErr
	}

	<-ctx.Done()
	return f.exitErr
}

func (f *fakeExecutor) recordedStdin() []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]byte, len(f.gotStdin))
	copy(out, f.gotStdin)
	return out
}

func (f *fakeExecutor) recordedSize() *remotecommand.TerminalSize {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.gotSize
}

func TestTerminalStdoutReachesOut(t *testing.T) {
	fe := newFakeExecutor()
	fe.initialWrite = []byte("hello world")

	term := Start(fe, "exec:pod-1")
	defer term.Close()

	if term.ID() != "exec:pod-1" {
		t.Fatalf("ID() = %q, want %q", term.ID(), "exec:pod-1")
	}

	select {
	case chunk := <-term.Out():
		if !bytes.Equal(chunk, []byte("hello world")) {
			t.Fatalf("Out() = %q, want %q", chunk, "hello world")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for stdout chunk on Out()")
	}
}

func TestTerminalWriteReachesStdin(t *testing.T) {
	fe := newFakeExecutor()
	fe.readStdin = true

	term := Start(fe, "t")
	defer term.Close()

	<-fe.streamStarted

	if _, err := term.Write([]byte("ls -la\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// Wait for the fake to finish reading and record stdin.
	deadline := time.After(2 * time.Second)
	for {
		if bytes.Equal(fe.recordedStdin(), []byte("ls -la\n")) {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("stdin not recorded; got %q", fe.recordedStdin())
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func TestTerminalResizeReachesSizeQueue(t *testing.T) {
	fe := newFakeExecutor()
	fe.readSize = true

	term := Start(fe, "t")
	defer term.Close()

	<-fe.streamStarted
	term.Resize(120, 40)

	deadline := time.After(2 * time.Second)
	for {
		if s := fe.recordedSize(); s != nil {
			if s.Width != 120 || s.Height != 40 {
				t.Fatalf("size = %dx%d, want 120x40", s.Width, s.Height)
			}
			break
		}
		select {
		case <-deadline:
			t.Fatal("size not delivered to size queue")
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func TestTerminalCleanExit(t *testing.T) {
	fe := newFakeExecutor()
	fe.returnImmediately = true
	fe.exitErr = nil

	term := Start(fe, "t")

	select {
	case <-term.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("Done() did not close on clean exit")
	}

	if err := term.Err(); err != nil {
		t.Fatalf("Err() = %v, want nil", err)
	}
	if code := term.ExitCode(); code != 0 {
		t.Fatalf("ExitCode() = %d, want 0", code)
	}

	// Out() must be closed once the stream goroutine finished.
	select {
	case _, ok := <-term.Out():
		if ok {
			// Drain any buffered chunk, then it should close.
			for range term.Out() {
			}
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Out() not closed after exit")
	}
}

// TestTerminalEOFIsCleanExit asserts a bare io.EOF returned by the stream (the
// normal end-of-stream for a TTY session) is normalized to a clean exit:
// Err()==nil and ExitCode()==0. The Start path uses an identity comparison, so
// only the naked sentinel is treated this way.
func TestTerminalEOFIsCleanExit(t *testing.T) {
	fe := newFakeExecutor()
	fe.returnImmediately = true
	fe.exitErr = io.EOF

	term := Start(fe, "t")

	select {
	case <-term.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("Done() did not close on EOF exit")
	}

	if err := term.Err(); err != nil {
		t.Fatalf("Err() = %v, want nil (io.EOF should normalize to clean exit)", err)
	}
	if code := term.ExitCode(); code != 0 {
		t.Fatalf("ExitCode() = %d, want 0", code)
	}
}

// TestTerminalCloseUnblocksFullChannel asserts the stream goroutine never leaks
// when nobody reads Out(): the fake floods stdout until the bounded channel is
// full and chanWriter.Write blocks on the send, then Close() cancels the context
// and the blocked write returns so the goroutine can unwind (Done() closes).
func TestTerminalCloseUnblocksFullChannel(t *testing.T) {
	fe := newFakeExecutor()
	// Write far more chunks than the 64-slot out channel can hold; with no reader
	// on Out() the chanWriter blocks on a full channel until ctx is cancelled.
	fe.floodStdout = 1000

	term := Start(fe, "t")
	<-fe.streamStarted

	// Never read term.Out(). Close should unblock the writer via ctx cancel.
	term.Close()

	select {
	case <-term.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("Done() did not close after Close() while stdout was blocked on a full channel (goroutine leak)")
	}
}

func TestTerminalNonzeroExitCode(t *testing.T) {
	fe := newFakeExecutor()
	fe.returnImmediately = true
	fe.exitErr = utilexec.CodeExitError{Err: context.Canceled, Code: 137}

	term := Start(fe, "t")

	select {
	case <-term.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("Done() did not close")
	}

	if code := term.ExitCode(); code != 137 {
		t.Fatalf("ExitCode() = %d, want 137", code)
	}
	if term.Err() == nil {
		t.Fatal("Err() = nil, want non-nil for nonzero exit")
	}
}

func TestTerminalCloseTearsDown(t *testing.T) {
	fe := newFakeExecutor()
	// Blocks until ctx cancelled (the default behavior).

	term := Start(fe, "t")
	<-fe.streamStarted

	term.Close()

	select {
	case <-term.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("Done() did not close after Close()")
	}

	// Out() must end without a send-on-closed-channel panic.
	select {
	case _, ok := <-term.Out():
		if ok {
			for range term.Out() {
			}
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Out() not closed after Close()")
	}
}

func TestTerminalCloseIdempotent(t *testing.T) {
	fe := newFakeExecutor()
	term := Start(fe, "t")
	<-fe.streamStarted

	term.Close()
	term.Close()
	term.Close()

	select {
	case <-term.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("Done() did not close")
	}
}

// TestTerminalRepeatedStartCloseNoLeak opens and closes many sessions in a tight
// loop and asserts every stream goroutine returns (Done() closes). A leaked
// goroutine (stuck stream or unclosed done) would time out here, and the -race
// build would also flag any send-on-closed/double-close along the way.
func TestTerminalRepeatedStartCloseNoLeak(t *testing.T) {
	const n = 50
	for i := 0; i < n; i++ {
		fe := newFakeExecutor() // blocks until ctx cancelled (default behavior)
		term := Start(fe, "t")
		<-fe.streamStarted
		term.Close()

		select {
		case <-term.Done():
		case <-time.After(2 * time.Second):
			t.Fatalf("iteration %d: Done() did not close after Close()", i)
		}
	}
}

// TestChanWriterPreCancelledContextReturns asserts chanWriter.Write returns
// cleanly (no panic, no send) when its context is ALREADY cancelled at entry.
// This exercises the `case <-ctx.Done()` select arm directly, distinct from the
// fill-then-cancel path: with an unbuffered out channel and no reader, the send
// arm can never proceed, so the cancelled-ctx arm must be taken and the write
// must report success.
func TestChanWriterPreCancelledContextReturns(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel before any Write

	// Unbuffered out with no reader: the send can never succeed, so the only way
	// Write returns is via the ctx.Done() arm.
	out := make(chan []byte)
	w := &chanWriter{ctx: ctx, out: out}

	done := make(chan struct{})
	var n int
	var err error
	go func() {
		n, err = w.Write([]byte("payload"))
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("chanWriter.Write blocked with a pre-cancelled context (ctx.Done arm not taken)")
	}

	if err != nil {
		t.Fatalf("Write err = %v, want nil (cancelled session reports success)", err)
	}
	if n != len("payload") {
		t.Fatalf("Write n = %d, want %d", n, len("payload"))
	}

	// Nothing was sent on out: a non-blocking receive finds it empty.
	select {
	case got := <-out:
		t.Fatalf("chanWriter sent on out despite cancelled ctx: %q", got)
	default:
	}
}

// TestTerminalWriteNeverBlocksOnStalledStdin asserts Write does not block the
// caller (the Bubble Tea UI goroutine) when the SPDY stdin reader is stalled.
// The fake never reads opts.Stdin and blocks until ctx is cancelled, so the
// underlying stdin pipe accepts nothing; Write must still return promptly for
// far more chunks than the input buffer can hold (it drops once saturated rather
// than freeze the UI).
func TestTerminalWriteNeverBlocksOnStalledStdin(t *testing.T) {
	fe := newFakeExecutor()
	// Default behavior: never reads Stdin, blocks on ctx.Done(). The stdin pipe
	// is therefore never drained, so a direct stdinW.Write would block.
	term := Start(fe, "t")
	<-fe.streamStarted
	defer term.Close()

	done := make(chan struct{})
	var badErr error
	var badN int
	go func() {
		defer close(done)
		// Far exceed the 256-slot input buffer so a blocking Write would hang here.
		// EVERY Write — including those that hit the drop (default) path once the
		// buffer saturates — must report the full byte count and a nil error: a
		// dropped chunk is still "accepted" so callers do not treat backpressure as
		// a hard error or a partial write.
		p := []byte("keystroke")
		for i := 0; i < 10000; i++ {
			n, err := term.Write(p)
			if err != nil {
				badErr = err
				return
			}
			if n != len(p) {
				badN = n
				return
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Write blocked when the SPDY stdin reader was stalled (UI goroutine would freeze)")
	}

	if badErr != nil {
		t.Fatalf("Write returned an error under saturation (drops must report success): %v", badErr)
	}
	if badN != 0 {
		t.Fatalf("Write returned partial count under saturation: n = %d, want %d", badN, len("keystroke"))
	}
}

// TestTerminalInputDrainExitsOnClose asserts the input drain goroutine does not
// leak: with a stalled stdin reader (stdinW.Write blocked) and a full input
// buffer, Close must tear the session down so the drain goroutine returns and
// Done() closes.
func TestTerminalInputDrainExitsOnClose(t *testing.T) {
	fe := newFakeExecutor()
	// Stalled reader: stdin is never drained, so the in-flight stdinW.Write in the
	// drain goroutine blocks until teardown unblocks it with ErrClosedPipe.
	term := Start(fe, "t")
	<-fe.streamStarted

	// Deterministically wedge the drain goroutine: write exactly one chunk. The
	// drain dequeues it and calls stdinW.Write, which blocks forever because the
	// fake never reads opts.Stdin (the pipe has no reader buffer). io.Pipe writes
	// are fully synchronous — Write only returns once a reader consumes the bytes —
	// so once this single chunk is in flight the drain is parked in stdinW.Write,
	// not in the channel-receive select. (Avoiding the buffer-saturation/drop path
	// keeps this from being probabilistic.)
	term.Write([]byte("x"))

	// Give the drain a moment to dequeue the chunk and reach the blocking Write.
	// Even if it has not yet, Close still unblocks it and the assertion below holds;
	// this just makes the "wedged in Write" precondition the common case.
	time.Sleep(20 * time.Millisecond)

	// Close must unblock the wedged stdinW.Write (ErrClosedPipe) AND cancel the
	// stream, so both the drain goroutine and the stream goroutine return.
	term.Close()

	select {
	case <-term.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("Done() did not close after Close() with a wedged input drain (goroutine leak)")
	}

	// A Write after teardown must not block or panic; the t.done select arm reports
	// the closed pipe (the buffer is empty so this returns immediately either way).
	if _, err := term.Write([]byte("late")); err == nil {
		// done may not be observed by the select arm if the buffer still has room,
		// in which case the chunk is accepted; either outcome is non-blocking and
		// safe. The test would have hung if Write blocked.
	}
}

func TestTerminalResizeNeverBlocks(t *testing.T) {
	fe := newFakeExecutor()
	// Fake never reads the size queue, so the resize channel can saturate.
	term := Start(fe, "t")
	<-fe.streamStarted
	defer term.Close()

	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			term.Resize(80+i, 24)
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Resize blocked when size queue was not drained")
	}
}
