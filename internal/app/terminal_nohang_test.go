package app

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/aohoyd/aku/internal/k8s/session"
	"github.com/aohoyd/aku/internal/msgs"
	"github.com/aohoyd/aku/internal/ui"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// openTestTerminalWithPump mirrors openTestTerminal but also starts the reply
// pump (startReplyPump), matching the production open path. It returns the live
// session so tests can drive/inspect it. Used by no-hang/leak tests that need
// the emulator's query replies forwarded back to the shell.
func openTestTerminalWithPump(t *testing.T, a *App, id string, fe *fakeExecutor) (*session.Terminal, *ui.TerminalPane) {
	t.Helper()
	if a.layout.SplitCount() == 0 {
		gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
		a.layout.AddSplit(&mockPlugin{name: "pods", gvr: gvr}, "default", "ctx-1")
	}
	sess := session.Start(fe, id)
	a.terminals[id] = sess
	w, h := a.layout.SplitSeedSize()
	tp := ui.NewTerminalPane("exec: test", "ctx-1", w, h)
	tp.SetID(id)
	a.layout.AddTerminalSplit(tp)
	startReplyPump(tp, sess)
	a.syncTerminalSizes()
	return sess, tp
}

// TestTerminalTwoQueriesNoHang delivers two back-to-back terminal queries (DSR
// "\x1b[6n" then DA "\x1b[c") through a.update and asserts update returns
// promptly each time (the now-non-blocking Write must not stall the UI
// goroutine) and that both replies are forwarded to the shell's stdin. This is
// the regression guard for the vim/full-screen hang.
func TestTerminalTwoQueriesNoHang(t *testing.T) {
	a := newTestApp()
	model, _ := a.update(tea.WindowSizeMsg{Width: 120, Height: 40})
	a = drainModel(t, model)

	fe := newFakeExecutor()
	fe.drainStdin = true // drain all replies, not just one
	id := "exec:twoqueries"
	sess, _ := openTestTerminalWithPump(t, &a, id, fe)
	defer sess.Close()
	<-fe.streamStarted

	queries := [][]byte{[]byte("\x1b[6n"), []byte("\x1b[c")}
	for i, q := range queries {
		// The worker goroutine reads and reassigns `a` (incl. a.terminals); the outer
		// goroutine touches `a` only AFTER receiving on `done`. The close(done)/<-done
		// pair establishes happens-before, so there is no concurrent access to the App
		// or its maps — confirmed race-clean across repeated -race runs.
		done := make(chan struct{})
		go func() {
			m, _ := a.update(msgs.TermBytesMsg{ID: id, Data: q})
			a = drainModelNoT(m)
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("update() hung delivering query %d (reply not drained)", i)
		}
	}

	// Both replies must reach the shell stdin: the DSR reply ends in 'R' (CPR)
	// and the DA reply is a CSI '?' ... 'c' device-attributes report.
	deadline := time.After(3 * time.Second)
	for {
		got := string(fe.recordedStdin())
		if strings.Contains(got, "R") && strings.Contains(got, "c") {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("both query replies not forwarded to shell stdin; got %q", fe.recordedStdin())
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// TestReplyPumpGoroutineExitsAfterTeardown asserts the startReplyPump goroutines
// exit after the session is torn down (Done→StopReplies→reply pipe EOF). The
// prefix-close path (closeFocusedTerminal) tears the session down; the reply
// pump's Read must then return and the goroutine must finish. A leak here would
// hang the WaitGroup wait and fail under the timeout.
func TestReplyPumpGoroutineExitsAfterTeardown(t *testing.T) {
	a := newTestApp()
	model, _ := a.update(tea.WindowSizeMsg{Width: 120, Height: 40})
	a = drainModel(t, model)

	// Instrument the REAL pump goroutines via the test-only hook: it fires once as
	// each of startReplyPump's two goroutines (the reply-forwarder and the
	// <-Done()→StopReplies watcher) returns. Installing it BEFORE the open path
	// guarantees both goroutines carry the deferred signal. startReplyPump always
	// starts exactly two goroutines, so we expect exactly two exits.
	var exits atomic.Int32
	replyPumpExited = func() { exits.Add(1) }
	t.Cleanup(func() { replyPumpExited = nil })

	fe := newFakeExecutor()
	id := "exec:pumpleak"
	sess, _ := openTestTerminalWithPump(t, &a, id, fe)
	<-fe.streamStarted

	// Close the focused terminal pane via the real close path (prefix x routes
	// here): it tears down the session, whose Done() fires the watcher goroutine
	// (StopReplies), closing the reply pipe so the forwarder's Read returns EOF.
	if _, ok := a.layout.FocusedPane().(*ui.TerminalPane); !ok {
		t.Fatal("precondition: terminal pane should be focused")
	}
	a, _ = a.closeFocusedTerminal()

	// Session torn down.
	select {
	case <-sess.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("session.Done() did not close after terminal close")
	}

	// BOTH real pump goroutines (forwarder + Done-watcher) must exit. A leak in
	// either leaves the count below 2 and fails under the timeout.
	deadline := time.After(2 * time.Second)
	for {
		if exits.Load() == 2 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("reply-pump goroutines did not all exit after teardown (got %d/2, goroutine leak)", exits.Load())
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// TestCloseNodeDebugMidPreflightCancelsAndNoLeak asserts that closing a
// node-debug placeholder pane while its pre-flight is still running cancels the
// pre-flight context (so the in-flight API calls do not leak up to the 60s wait)
// and, once the pod name is known, issues the delete. Here we exercise the
// branch where the pod name is NOT yet known (placeholder closed before
// DebugReadyMsg): the cancel func must fire, which is what hands cleanup of any
// created pod to PrepareNodeDebug's own ctx-cancel teardown.
func TestCloseNodeDebugMidPreflightCancelsAndNoLeak(t *testing.T) {
	a := newTestApp()
	model, _ := a.update(tea.WindowSizeMsg{Width: 120, Height: 40})
	a = drainModel(t, model)

	// Seed a resource sibling so the placeholder is removable.
	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
	a.layout.AddSplit(&mockPlugin{name: "pods", gvr: gvr}, "default", "ctx-1")

	client, deleted := fakeDeleteClient()
	id := "debug-node:midflight"

	// Place a node-debug placeholder pane with a live pre-flight cancel func, as
	// openNodeDebugTerminal would (pod name not yet known).
	var cancelled atomic.Bool
	_, cancel := context.WithCancel(context.Background())
	wrapped := func() {
		cancelled.Store(true)
		cancel()
	}
	w, h := a.layout.SplitSeedSize()
	tp := ui.NewTerminalPane("node: x", "ctx-1", w, h)
	tp.SetID(id)
	a.termCleanup[id] = terminalMeta{nodeDebug: true, client: client, preflightCancel: wrapped}
	a.layout.AddTerminalSplit(tp)
	a.syncTerminalSizes()

	if _, ok := a.layout.FocusedPane().(*ui.TerminalPane); !ok {
		t.Fatal("precondition: terminal placeholder should be focused")
	}

	// Close the placeholder mid-flight.
	a, _ = a.closeFocusedTerminal()

	// The pre-flight cancel must have fired (aborting the in-flight API calls).
	if !cancelled.Load() {
		t.Fatal("closing the placeholder did not cancel the in-flight pre-flight")
	}
	// Metadata cleared.
	if _, ok := a.termCleanup[id]; ok {
		t.Fatal("cleanup metadata not cleared on close")
	}
	// No pod name was known, so no by-name delete is issued from closeTerminalSession
	// (PrepareNodeDebug's own teardown owns that case). Ensure we did not spuriously
	// delete an empty-named pod.
	select {
	case name := <-deleted:
		t.Fatalf("unexpected delete issued for an unnamed pod: %q", name)
	case <-time.After(200 * time.Millisecond):
		// expected: no delete with an empty name
	}
}

// TestCloseNodeDebugAfterReadyDeletesPodByName asserts the companion case: once
// the pre-flight has landed (DebugReadyMsg filled in the pod name and cleared
// preflightCancel), closing the pane deletes the created pod by name.
func TestCloseNodeDebugAfterReadyDeletesPodByName(t *testing.T) {
	a := newTestApp()
	model, _ := a.update(tea.WindowSizeMsg{Width: 120, Height: 40})
	a = drainModel(t, model)

	client, deleted := fakeDeleteClient("ktui-debug-node1-bbbbb")
	fe := newFakeExecutor()
	sess := openTestTerminalWithMeta(t, &a, "debug-node:ready", fe, terminalMeta{
		nodeDebug: true,
		client:    client,
		podName:   "ktui-debug-node1-bbbbb",
		namespace: "default",
		// preflightCancel intentionally nil: the pre-flight already landed.
	})
	_ = sess

	a, _ = a.closeFocusedTerminal()

	select {
	case name := <-deleted:
		if name != "ktui-debug-node1-bbbbb" {
			t.Fatalf("deleted unexpected pod: %s", name)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("node-debug close after ready did not delete the pod by name")
	}
}

// TestDebugReadyAfterCloseDeletesPodViaMsgClient guards the node-debug pod-leak
// window: a node-debug pane is closed AFTER the pod was created but BEFORE the
// DebugReadyMsg lands. closeTerminalSession runs first (pod name not yet known,
// so it issues no by-name delete) and removes the termCleanup entry. When the
// DebugReadyMsg then arrives carrying the real pod name, the !found branch must
// still delete the pod — using the client carried IN the message, because the
// termCleanup entry is gone. Without the message-carried client the pod leaks.
func TestDebugReadyAfterCloseDeletesPodViaMsgClient(t *testing.T) {
	a := newTestApp()
	model, _ := a.update(tea.WindowSizeMsg{Width: 120, Height: 40})
	a = drainModel(t, model)

	// Seed a resource sibling so the placeholder is removable.
	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
	a.layout.AddSplit(&mockPlugin{name: "pods", gvr: gvr}, "default", "ctx-1")

	client, deleted := fakeDeleteClient("created-pod")
	id := "debug-node:leakwindow"

	// Place a node-debug placeholder WITHOUT a known pod name, as
	// openNodeDebugTerminal does before the pre-flight reports back.
	_, cancel := context.WithCancel(context.Background())
	w, h := a.layout.SplitSeedSize()
	tp := ui.NewTerminalPane("node: x", "ctx-1", w, h)
	tp.SetID(id)
	a.termCleanup[id] = terminalMeta{nodeDebug: true, client: client, preflightCancel: cancel}
	a.layout.AddTerminalSplit(tp)
	a.syncTerminalSizes()

	if _, ok := a.layout.FocusedPane().(*ui.TerminalPane); !ok {
		t.Fatal("precondition: terminal placeholder should be focused")
	}

	// Close the placeholder first via the real close path: removes the pane from
	// the layout and the termCleanup entry, with no by-name delete (pod name not
	// yet known). This is the window in which a pod was created but not reported.
	a, _ = a.closeFocusedTerminal()
	if _, ok := a.termCleanup[id]; ok {
		t.Fatal("precondition: termCleanup entry should be gone after close")
	}
	if _, found := a.layout.TerminalPaneByID(id); found {
		t.Fatal("precondition: terminal pane should be gone after close")
	}

	// The DebugReadyMsg now lands with the real pod name and the message client.
	model, _ = a.update(msgs.DebugReadyMsg{
		ID:        id,
		NodeMode:  true,
		PodName:   "created-pod",
		Namespace: "default",
		Client:    client,
	})
	_ = drainModel(t, model)

	select {
	case name := <-deleted:
		if name != "created-pod" {
			t.Fatalf("deleted unexpected pod: %s", name)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("node-debug pod leaked: DebugReadyMsg after close did not delete it via the message client")
	}
}

// TestTermExitMsgStreamErrorSurfaced asserts a mid-session stream error (Err set,
// Code 0) is surfaced in the pane's exit banner rather than masquerading as a
// clean "[exited — status 0]".
func TestTermExitMsgStreamErrorSurfaced(t *testing.T) {
	a := newTestApp()
	model, _ := a.update(tea.WindowSizeMsg{Width: 120, Height: 40})
	a = drainModel(t, model)

	fe := newFakeExecutor()
	id := "exec:streamerr"
	sess := openTestTerminal(t, &a, id, fe)
	defer sess.Close()

	model, cmd := a.update(msgs.TermExitMsg{ID: id, Code: 0, Err: errTest("connection reset")})
	a = drainModel(t, model)
	if cmd != nil {
		t.Fatalf("TermExitMsg must not reschedule, got %T", cmd)
	}

	tp, found := a.layout.TerminalPaneByID(id)
	if !found {
		t.Fatal("terminal pane not found after TermExitMsg")
	}
	if !tp.Exited() {
		t.Fatal("pane not marked exited")
	}
	view := tp.View()
	if !strings.Contains(view, "connection reset") {
		t.Fatalf("stream error not surfaced in exit banner:\n%s", view)
	}
	// Must NOT read as a clean status-0 exit (the synthetic non-zero code).
	if strings.Contains(view, "status 0") {
		t.Fatalf("stream error rendered as clean status 0:\n%s", view)
	}
}
