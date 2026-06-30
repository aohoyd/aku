package app

import (
	"bytes"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/k8s/session"
	"github.com/aohoyd/aku/internal/msgs"
	"github.com/aohoyd/aku/internal/notify"
	"github.com/aohoyd/aku/internal/ui"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

// waitAllDone waits for every session's Done() to close under ONE shared ceiling,
// rather than giving each its own timeout (which would multiply the worst-case
// wait by the session count). It genuinely fans out — one goroutine per session,
// each blocking on its OWN Done() — so a hung non-first session cannot mask an
// already-closed earlier one; a single collector closes the channel once every
// goroutine has signalled, selected against the shared deadline.
func waitAllDone(t *testing.T, sessions []*session.Terminal, ceiling time.Duration, ctx string) {
	t.Helper()
	var wg sync.WaitGroup
	for _, s := range sessions {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-s.Done()
		}()
	}
	all := make(chan struct{})
	go func() { wg.Wait(); close(all) }()
	select {
	case <-all:
	case <-time.After(ceiling):
		t.Fatalf("%s: not all sessions closed within %v", ctx, ceiling)
	}
}

// isGracefulSequence reports whether got is a leading Ctrl-C (0x03) followed by
// at least one Ctrl-D (0x04). Checking got[1:] for the EOT (instead of
// IndexByte(got, 0x04) > 0) makes the intent explicit: a real EOT lands AFTER the
// leading Ctrl-C, not merely "somewhere past index 0". Polling loops use this
// predicate directly; assertGracefulSequence wraps it for one-shot assertions.
func isGracefulSequence(got []byte) bool {
	return len(got) >= 2 && got[0] == 0x03 && bytes.IndexByte(got[1:], 0x04) >= 0
}

// assertGracefulSequence asserts the recorded stdin satisfies isGracefulSequence.
func assertGracefulSequence(t *testing.T, got []byte, ctx string) {
	t.Helper()
	if !isGracefulSequence(got) {
		t.Fatalf("%s: stdin = %q, want a leading Ctrl-C (0x03) then at least one Ctrl-D (0x04)", ctx, got)
	}
}

// waitGracefulSequence polls fe's recorded stdin until it satisfies
// isGracefulSequence or the deadline elapses, routing the byte-condition through
// the shared predicate so every graceful test asserts the same thing.
func waitGracefulSequence(t *testing.T, fe *fakeExecutor, ctx string) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		if isGracefulSequence(fe.recordedStdin()) {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("%s: graceful sequence (0x03 then 0x04) not delivered; got %q", ctx, fe.recordedStdin())
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// fakeDeleteClient builds a k8s.Client whose typed fake records pod deletes on
// the returned channel, so lifecycle tests can assert a node-debug pod was
// deleted on close / quit.
func fakeDeleteClient(seedPods ...string) (*k8s.Client, <-chan string) {
	deleted := make(chan string, 4)
	objs := make([]runtime.Object, 0, len(seedPods))
	for _, name := range seedPods {
		objs = append(objs, &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"}})
	}
	fc := fake.NewClientset(objs...)
	fc.PrependReactor("delete", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		da := action.(k8stesting.DeleteAction)
		select {
		case deleted <- da.GetName():
		default:
		}
		return false, nil, nil // fall through to the tracker
	})
	return &k8s.Client{Typed: fc, Namespace: "default", Context: "ctx-1"}, deleted
}

// openTestTerminalWithMeta injects a started session + pane + cleanup metadata,
// mirroring openNodeDebugTerminal/openDebugTerminal post-pre-flight state.
func openTestTerminalWithMeta(t *testing.T, a *App, id string, fe *fakeExecutor, meta terminalMeta) *session.Terminal {
	t.Helper()
	if a.layout.SplitCount() == 0 {
		gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
		a.layout.AddSplit(&mockPlugin{name: "pods", gvr: gvr}, "default", "ctx-1")
	}
	sess := session.Start(fe, id)
	a.terminals[id] = sess
	a.termCleanup[id] = meta
	w, h := a.layout.SplitSeedSize()
	tp := ui.NewTerminalPane("term", "ctx-1", w, h)
	tp.SetID(id)
	a.layout.AddTerminalSplit(tp)
	a.syncTerminalSizes()
	return sess
}

// TestCloseFocusedTerminalLastSplitGuard asserts that closing the focused
// terminal pane when it is the ONLY split tears down its session but leaves the
// pane in place (the layout is never emptied; quit is owned by the ctrl+w/q
// path). SplitCount stays unchanged.
func TestCloseFocusedTerminalLastSplitGuard(t *testing.T) {
	a := newTestApp()
	model, _ := a.update(tea.WindowSizeMsg{Width: 100, Height: 40})
	a = drainModel(t, model)

	// Open a terminal as the only split (no resource sibling seeded). Drain stdin
	// and exit on the EOT burst so the detached graceful-terminate goroutine
	// completes promptly rather than blocking the full grace budget.
	fe := newFakeExecutor()
	fe.drainStdin = true
	fe.exitOnEOT = true
	id := "exec:only"
	sess := session.Start(fe, id)
	a.terminals[id] = sess
	w, h := a.layout.SplitSeedSize()
	tp := ui.NewTerminalPane("exec: only", "ctx-1", w, h)
	tp.SetID(id)
	a.layout.AddTerminalSplit(tp)
	a.syncTerminalSizes()
	defer sess.Close()
	<-fe.streamStarted

	if a.layout.SplitCount() != 1 {
		t.Fatalf("precondition: expected exactly one split, got %d", a.layout.SplitCount())
	}
	before := a.layout.SplitCount()

	a, _ = a.closeFocusedTerminal()

	// Session torn down...
	if _, ok := a.terminals[id]; ok {
		t.Fatal("last-split close should tear down the session (still in registry)")
	}
	// ...but the pane is still present and the count is unchanged.
	if a.layout.SplitCount() != before {
		t.Fatalf("last-split close should not remove the pane: %d → %d", before, a.layout.SplitCount())
	}
	if _, found := a.layout.TerminalPaneByID(id); !found {
		t.Fatal("last-split close removed the pane; it should remain in place")
	}

	// The FULL graceful sequence (Ctrl-C then the EOT burst) reaches the shell
	// before teardown — asserted BEFORE waiting on Done() so a regression that
	// drops the EOT burst (Ctrl-C alone) cannot slip through on a Done() that
	// closes via the hard-Close() timeout.
	waitGracefulSequence(t, fe, "last-split guard close")

	select {
	case <-sess.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("session.Done() did not close after last-split terminal close")
	}
}

// TestCloseTerminalPane_ReleasesLastReferencedCluster asserts that closing a
// terminal pane whose context is referenced by no other pane drives the
// cluster's refcount to zero so SyncRefs tears it down. Terminal panes count
// toward refcounts (distinctPaneContexts/paneContexts include them), so the
// terminal close path must reconcile refcounts just like the resource path —
// otherwise the cluster leaks.
func TestCloseTerminalPane_ReleasesLastReferencedCluster(t *testing.T) {
	mgr := newTestManagerWithContexts(t, "global", map[string][]*unstructured.Unstructured{
		"global":  {testPod("global-pod", "default")},
		"staging": {testPod("staging-pod", "default")},
	})
	a := newContextSwitchApp(t, mgr)

	// Ensure the staging cluster exists (a terminal pane references it).
	if _, err := mgr.GetOrCreate("staging"); err != nil {
		t.Fatalf("GetOrCreate(staging) err = %v", err)
	}

	// Open a terminal pane bound to the staging context as a new split. It is the
	// only pane referencing staging; the baseline panes are on the startup context.
	fe := newFakeExecutor()
	id := "exec:staging"
	sess := session.Start(fe, id)
	a.terminals[id] = sess
	defer sess.Close()
	w, h := a.layout.SplitSeedSize()
	tp := ui.NewTerminalPane("exec: staging", "staging", w, h)
	tp.SetID(id)
	a.layout.AddTerminalSplit(tp)
	a.syncTerminalSizes()

	// Reconcile so staging's refcount reflects the new terminal pane.
	a.mgr.SyncRefs(a.distinctPaneContexts())
	if _, ok := mgr.Get("staging"); !ok {
		t.Fatalf("precondition: expected staging cluster present with a terminal pane referencing it")
	}

	// Focus and close the terminal pane via the shared close path.
	if _, ok := a.layout.FocusedPane().(*ui.TerminalPane); !ok {
		t.Fatalf("precondition: terminal pane should be focused after AddTerminalSplit")
	}
	a, _ = a.closeFocusedSplit()

	// SyncRefs must have driven staging to refcount 0 and torn it down — no leak.
	if _, ok := mgr.Get("staging"); ok {
		t.Fatalf("expected staging cluster torn down after closing the only terminal pane referencing it (refcount leak)")
	}
	// The startup cluster survives (baseline panes still reference it).
	if _, ok := mgr.Get("global"); !ok {
		t.Fatalf("expected global cluster to remain (baseline panes reference it)")
	}
}

// TestCloseFocusedTerminalLastSplitEphemeral asserts the last-split close path's
// EPHEMERAL sub-branch: an ephemeral debug terminal opened as the ONLY split,
// when closed, tears down the session and returns a non-nil status-bar note cmd
// (the "ephemeral container left" note) while leaving the pane in place.
func TestCloseFocusedTerminalLastSplitEphemeral(t *testing.T) {
	a := newTestApp()
	model, _ := a.update(tea.WindowSizeMsg{Width: 120, Height: 40})
	a = drainModel(t, model)

	// Open an ephemeral debug terminal as the only split (no resource sibling).
	// Drain stdin and exit on the EOT burst so the detached graceful-terminate
	// goroutine completes promptly rather than blocking the full grace budget.
	fe := newFakeExecutor()
	fe.drainStdin = true
	fe.exitOnEOT = true
	id := "debug:only"
	sess := session.Start(fe, id)
	a.terminals[id] = sess
	a.termCleanup[id] = terminalMeta{ephemeral: true, podName: "mypod", namespace: "default"}
	w, h := a.layout.SplitSeedSize()
	tp := ui.NewTerminalPane("debug: only", "ctx-1", w, h)
	tp.SetID(id)
	a.layout.AddTerminalSplit(tp)
	a.syncTerminalSizes()
	defer sess.Close()
	<-fe.streamStarted

	if a.layout.SplitCount() != 1 {
		t.Fatalf("precondition: expected exactly one split, got %d", a.layout.SplitCount())
	}
	before := a.layout.SplitCount()

	a, _ = a.closeFocusedTerminal()

	// The ephemeral note is recorded in the notify store (toast), not returned as
	// a status-bar cmd.
	if !hasNotifyLevel(a, notify.LevelWarning) {
		t.Fatal("last-split ephemeral close should record an ephemeral note in the notify store")
	}
	// Session torn down...
	if _, ok := a.terminals[id]; ok {
		t.Fatal("last-split close should tear down the session (still in registry)")
	}
	if _, ok := a.termCleanup[id]; ok {
		t.Fatal("ephemeral cleanup metadata not removed on close")
	}
	// ...but the pane is still present and the count is unchanged.
	if a.layout.SplitCount() != before {
		t.Fatalf("last-split close should not remove the pane: %d → %d", before, a.layout.SplitCount())
	}
	if _, found := a.layout.TerminalPaneByID(id); !found {
		t.Fatal("last-split close removed the pane; it should remain in place")
	}

	select {
	case <-sess.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("session.Done() did not close after last-split ephemeral terminal close")
	}
}

// TestNodeDebugCloseDeletesPod asserts that closing a node-debug terminal pane
// fires a best-effort delete of the created debug pod and cancels the session.
func TestNodeDebugCloseDeletesPod(t *testing.T) {
	a := newTestApp()
	model, _ := a.update(tea.WindowSizeMsg{Width: 100, Height: 40})
	a = drainModel(t, model)

	client, deleted := fakeDeleteClient("ktui-debug-node1-aaaaa")
	// Drain stdin and exit on the EOT burst so the detached graceful-terminate
	// goroutine completes promptly rather than blocking the full grace budget.
	fe := newFakeExecutor()
	fe.drainStdin = true
	fe.exitOnEOT = true
	sess := openTestTerminalWithMeta(t, &a, "debug-node:1", fe, terminalMeta{
		nodeDebug: true,
		client:    client,
		podName:   "ktui-debug-node1-aaaaa",
		namespace: "default",
	})
	<-fe.streamStarted

	// Close the focused (terminal) pane via the shared close path.
	a, _ = a.closeFocusedTerminal()

	select {
	case name := <-deleted:
		if name != "ktui-debug-node1-aaaaa" {
			t.Fatalf("deleted unexpected pod: %s", name)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("node-debug close did not delete the debug pod")
	}

	if _, ok := a.terminals["debug-node:1"]; ok {
		t.Fatal("session not removed from registry on close")
	}
	if _, ok := a.termCleanup["debug-node:1"]; ok {
		t.Fatal("cleanup metadata not removed on close")
	}

	// Session was cancelled: Done() closes.
	select {
	case <-sess.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("session.Done() did not close after pane close (leak)")
	}
}

// TestCloseTerminalSessionUnregistersImmediately asserts pane close stays
// instant: closeTerminalSession removes the session from a.terminals (and the
// node-debug cleanup metadata) synchronously, BEFORE the detached
// graceful-terminate goroutine has a chance to run. The graceful sequence +
// Close() happen in the background; the maps must already be drained on return.
func TestCloseTerminalSessionUnregistersImmediately(t *testing.T) {
	a := newTestApp()
	model, _ := a.update(tea.WindowSizeMsg{Width: 100, Height: 40})
	a = drainModel(t, model)

	// A fake that blocks on ctx (does not exit) so the background goroutine is
	// still mid-grace while we assert the synchronous removal.
	fe := newFakeExecutor()
	id := "exec:instant"
	sess := openTestTerminalWithMeta(t, &a, id, fe, terminalMeta{ephemeral: true, podName: "p", namespace: "default"})
	defer sess.Close()
	<-fe.streamStarted

	a.closeTerminalSession(id)

	// Synchronous removal: no waiting on the background goroutine.
	if _, ok := a.terminals[id]; ok {
		t.Fatal("session not removed from a.terminals synchronously on close")
	}
	if _, ok := a.termCleanup[id]; ok {
		t.Fatal("cleanup metadata not removed synchronously on close")
	}
}

// TestCloseTerminalSessionGracefulThenClose asserts the background path: after
// closeTerminalSession returns, the detached goroutine sends the graceful
// control sequence (Ctrl-C 0x03 then a burst of Ctrl-D 0x04) to the shell's
// stdin and then hard-Close()s the session (Done() closes). Assertions poll with
// a timeout rather than sleeping a fixed duration to stay non-flaky.
func TestCloseTerminalSessionGracefulThenClose(t *testing.T) {
	a := newTestApp()
	model, _ := a.update(tea.WindowSizeMsg{Width: 100, Height: 40})
	a = drainModel(t, model)

	// drainStdin records every chunk written to stdin so the graceful sequence
	// is observable; the fake blocks on ctx until Close() cancels it.
	fe := newFakeExecutor()
	fe.drainStdin = true
	id := "exec:graceful"
	sess := openTestTerminalWithMeta(t, &a, id, fe, terminalMeta{})
	defer sess.Close()
	<-fe.streamStarted

	a.closeTerminalSession(id)

	// The background goroutine eventually delivers the graceful sequence: a
	// leading Ctrl-C (0x03) followed by at least one Ctrl-D (0x04).
	waitGracefulSequence(t, fe, "closeTerminalSession background path")

	// And the session is hard-Close()d afterward: Done() closes (ctx cancelled).
	select {
	case <-sess.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("background path did not Close() the session (Done() never closed)")
	}
}

// TestCloseFocusedTerminalLastSplitGraceful asserts the last-split close path
// (closeFocusedTerminal with SplitCount==1) routes the frozen pane's session
// through closeTerminalSession, so it still receives graceful termination + a
// hard Close() even though the pane stays on screen.
func TestCloseFocusedTerminalLastSplitGraceful(t *testing.T) {
	a := newTestApp()
	model, _ := a.update(tea.WindowSizeMsg{Width: 100, Height: 40})
	a = drainModel(t, model)

	// Open a terminal as the only split (no resource sibling seeded).
	fe := newFakeExecutor()
	fe.drainStdin = true
	id := "exec:lastsplit"
	sess := session.Start(fe, id)
	a.terminals[id] = sess
	w, h := a.layout.SplitSeedSize()
	tp := ui.NewTerminalPane("exec: only", "ctx-1", w, h)
	tp.SetID(id)
	a.layout.AddTerminalSplit(tp)
	a.syncTerminalSizes()
	defer sess.Close()
	<-fe.streamStarted

	if a.layout.SplitCount() != 1 {
		t.Fatalf("precondition: expected exactly one split, got %d", a.layout.SplitCount())
	}

	a, _ = a.closeFocusedTerminal()

	// Session unregistered immediately, pane left in place.
	if _, ok := a.terminals[id]; ok {
		t.Fatal("last-split close should unregister the session")
	}
	if _, found := a.layout.TerminalPaneByID(id); !found {
		t.Fatal("last-split close removed the pane; it should remain in place")
	}

	// Full graceful sequence reaches the shell before the hard teardown: a
	// leading Ctrl-C (0x03) followed by at least one Ctrl-D (0x04).
	waitGracefulSequence(t, fe, "last-split close")

	select {
	case <-sess.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("session.Done() did not close after last-split terminal close")
	}
}

// TestOpenExecTerminalEndToEnd drives openExecTerminal through its execExecutorFn
// seam with a fake executor: it registers the session in a.terminals, opens a
// terminal pane carrying the title/context, applies the configured scrollback,
// and returns the byte-pump cmd. This exercises the full exec open path without a
// real cluster Config.
func TestOpenExecTerminalEndToEnd(t *testing.T) {
	a := newTestApp()
	model, _ := a.update(tea.WindowSizeMsg{Width: 120, Height: 40})
	a = drainModel(t, model)

	// Seed a resource sibling so the new terminal pane has a sibling (SplitCount
	// >= 2 after open) and focus assertions hold.
	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
	a.layout.AddSplit(&mockPlugin{name: "pods", gvr: gvr}, "default", "ctx-1")

	// Substitute the exec-executor seam with a fake so no real Config is needed.
	fe := newFakeExecutor()
	fe.initialWrite = []byte("exec-ready")
	var gotPod, gotCtr, gotNs string
	a.execExecutorFn = func(_ *k8s.Client, podName, containerName, namespace string, _ []string) (session.Executor, error) {
		gotPod, gotCtr, gotNs = podName, containerName, namespace
		return fe, nil
	}

	client := &k8s.Client{Context: "ctx-1", Namespace: "default"}
	before := a.layout.SplitCount()

	model, cmd := a.openExecTerminal(client, "mypod", "myctr", "default", "ctx-1", "exec: mypod/myctr")
	a = drainModel(t, model)

	// The byte-pump cmd kicks off the read loop.
	if cmd == nil {
		t.Fatal("openExecTerminal should return the byte-pump cmd (got nil)")
	}
	// The seam was called with the right identity.
	if gotPod != "mypod" || gotCtr != "myctr" || gotNs != "default" {
		t.Fatalf("execExecutorFn called with %q/%q/%q, want mypod/myctr/default", gotPod, gotCtr, gotNs)
	}

	// A new pane was added.
	if a.layout.SplitCount() != before+1 {
		t.Fatalf("expected a new terminal split: %d → %d", before, a.layout.SplitCount())
	}

	// The session is registered under a generated exec id, and the pane exists.
	var id string
	for k := range a.terminals {
		if strings.HasPrefix(k, "exec:") {
			id = k
			break
		}
	}
	if id == "" {
		t.Fatal("no exec session registered in a.terminals")
	}
	sess := a.terminals[id]
	defer sess.Close()

	tp, found := a.layout.TerminalPaneByID(id)
	if !found {
		t.Fatalf("terminal pane %q not found in layout", id)
	}
	if tp.Title() != "exec: mypod/myctr" {
		t.Fatalf("pane title = %q, want %q", tp.Title(), "exec: mypod/myctr")
	}
	if tp.Context() != "ctx-1" {
		t.Fatalf("pane context = %q, want ctx-1", tp.Context())
	}

	// The byte pump delivers the seeded output through the pane (scrollback applied
	// and the stream wired). Run the pump cmd and feed the result back.
	msg := runCmd(t, cmd)
	bmsg, ok := msg.(msgs.TermBytesMsg)
	if !ok {
		t.Fatalf("expected TermBytesMsg from the byte pump, got %T", msg)
	}
	if bmsg.ID != id || !strings.Contains(string(bmsg.Data), "exec-ready") {
		t.Fatalf("unexpected pump output: %+v", bmsg)
	}
	model, _ = a.update(bmsg)
	a = drainModel(t, model)
	tp, _ = a.layout.TerminalPaneByID(id)
	if !strings.Contains(tp.View(), "exec-ready") {
		t.Fatalf("pane View did not render the streamed output:\n%s", tp.View())
	}
}

// TestTerminalQueryReplyForwardedNoHang is the regression test for the vim hang:
// a full-screen program's terminal query (here DSR cursor-position, "\x1b[6n")
// must not block the UI goroutine, and its reply must be forwarded back to the
// shell's stdin. openExecTerminal starts the reply-drain pump; feeding the query
// through update() must return promptly and the fake executor's recorded stdin
// must receive the cursor-position reply (CPR, ends in 'R').
func TestTerminalQueryReplyForwardedNoHang(t *testing.T) {
	a := newTestApp()
	model, _ := a.update(tea.WindowSizeMsg{Width: 120, Height: 40})
	a = drainModel(t, model)

	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
	a.layout.AddSplit(&mockPlugin{name: "pods", gvr: gvr}, "default", "ctx-1")

	fe := newFakeExecutor()
	fe.readStdin = true // record one chunk forwarded back to the shell
	a.execExecutorFn = func(_ *k8s.Client, _, _, _ string, _ []string) (session.Executor, error) {
		return fe, nil
	}

	client := &k8s.Client{Context: "ctx-1", Namespace: "default"}
	model, _ = a.openExecTerminal(client, "mypod", "myctr", "default", "ctx-1", "exec: mypod/myctr")
	a = drainModel(t, model)

	var id string
	for k := range a.terminals {
		if strings.HasPrefix(k, "exec:") {
			id = k
			break
		}
	}
	if id == "" {
		t.Fatal("no exec session registered")
	}
	sess := a.terminals[id]
	defer sess.Close()
	<-fe.streamStarted

	// Deliver a terminal query as if the program emitted it on stdout. Feeding it
	// to the emulator on the update goroutine must NOT hang: the pump drains the
	// reply concurrently. Run update() with a timeout guard to catch a regression.
	// The worker reads `a` and a.terminals; the outer goroutine touches `a` again
	// only after receiving on `done`. close(done)/<-done establishes happens-before,
	// so there is no concurrent App/map access — confirmed race-clean under -race.
	done := make(chan struct{})
	go func() {
		m, _ := a.update(msgs.TermBytesMsg{ID: id, Data: []byte("\x1b[6n")})
		_ = drainModelNoT(m)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("update() hung feeding a terminal query to the emulator (reply not drained)")
	}

	// The CPR reply must be forwarded to the shell stdin (fake records one chunk).
	deadline := time.After(2 * time.Second)
	for {
		if strings.Contains(string(fe.recordedStdin()), "R") {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("cursor-position reply not forwarded to shell stdin; got %q", fe.recordedStdin())
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// drainModelNoT type-asserts a tea.Model back to App without a *testing.T (for use
// inside goroutines, where t.Fatal is unsafe).
func drainModelNoT(m tea.Model) App {
	a, _ := m.(App)
	return a
}

// TestShutdownTerminalsSweepsNodePods asserts shutdownTerminals deletes every
// node-debug pod and closes all sessions.
func TestShutdownTerminalsSweepsNodePods(t *testing.T) {
	a := newTestApp()

	client, deleted := fakeDeleteClient("pod-a", "pod-b")

	feA := newFakeExecutor()
	feB := newFakeExecutor()
	feC := newFakeExecutor()
	sessA := session.Start(feA, "n:a")
	sessB := session.Start(feB, "n:b")
	sessC := session.Start(feC, "x:c") // exec session — must be closed, not deleted
	a.terminals["n:a"] = sessA
	a.terminals["n:b"] = sessB
	a.terminals["x:c"] = sessC
	a.termCleanup["n:a"] = terminalMeta{nodeDebug: true, client: client, podName: "pod-a", namespace: "default"}
	a.termCleanup["n:b"] = terminalMeta{nodeDebug: true, client: client, podName: "pod-b", namespace: "default"}
	a.termCleanup["x:c"] = terminalMeta{ephemeral: true, client: client, podName: "epod", namespace: "default"}

	a.shutdownTerminals()

	got := map[string]bool{}
	for i := 0; i < 2; i++ {
		select {
		case name := <-deleted:
			got[name] = true
		case <-time.After(2 * time.Second):
			t.Fatalf("expected 2 node-pod deletes, got %v", got)
		}
	}
	if !got["pod-a"] || !got["pod-b"] {
		t.Fatalf("not all node pods deleted: %v", got)
	}

	// All sessions closed and registries drained.
	if len(a.terminals) != 0 || len(a.termCleanup) != 0 {
		t.Fatalf("registries not drained: terminals=%d cleanup=%d", len(a.terminals), len(a.termCleanup))
	}
	waitAllDone(t, []*session.Terminal{sessA, sessB, sessC}, 2*time.Second, "shutdownTerminals sweep")
}

// TestShutdownTerminalsGracefulMultiSession asserts the quit path asks EVERY
// live shell to exit gracefully BEFORE the hard close, and that the bytes land
// while the session is still live. Each fake exits cleanly on the first EOT
// (exitOnEOT), so the stream ends as a direct consequence of the graceful burst
// — not the later hard Close(). To prove ordering, run shutdownTerminals in the
// background and poll for the full graceful sequence FIRST (while the sessions
// are still draining the burst), then assert Done() closes. The bytes therefore
// provably arrive at a live shell before teardown completes.
func TestShutdownTerminalsGracefulMultiSession(t *testing.T) {
	a := newTestApp()

	type sf struct {
		id   string
		fe   *fakeExecutor
		sess *session.Terminal
	}
	specs := []*sf{
		{id: "exec:g1", fe: newFakeExecutor()},
		{id: "exec:g2", fe: newFakeExecutor()},
		{id: "exec:g3", fe: newFakeExecutor()},
	}
	for _, s := range specs {
		s.fe.drainStdin = true // record the graceful control bytes
		s.fe.exitOnEOT = true  // exit cleanly on the first Ctrl-D, while still live
		s.sess = session.Start(s.fe, s.id)
		a.terminals[s.id] = s.sess
		<-s.fe.streamStarted
	}

	done := make(chan struct{})
	go func() {
		a.shutdownTerminals()
		close(done)
	}()

	// Poll for the graceful sequence FIRST — this happens while the sessions are
	// still live (the shell exits on the EOT we are watching for). Each shell
	// must receive a leading Ctrl-C (0x03) followed by at least one Ctrl-D (0x04).
	for _, s := range specs {
		waitGracefulSequence(t, s.fe, "session "+s.id+" before teardown")
	}

	// Only after the bytes are observed do we let teardown finish: shutdownTerminals
	// returns and every session ends up hard-Close()d (Done() closes).
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("shutdownTerminals did not return after sessions exited on EOT")
	}
	if len(a.terminals) != 0 {
		t.Fatalf("terminals not drained after shutdown: %d", len(a.terminals))
	}
	sessions := make([]*session.Terminal, 0, len(specs))
	for _, s := range specs {
		sessions = append(sessions, s.sess)
	}
	waitAllDone(t, sessions, 2*time.Second, "graceful multi-session shutdown")
}

// TestShutdownTerminalsBoundedWhenShellsIgnoreEOT asserts quit completes within
// a bounded time even when sessions never exit on their own: the fakes block on
// ctx (ignoring the EOT burst) until Close() cancels them, so the shared grace
// phase must hit its time.After ceiling rather than waiting indefinitely. Many
// sessions share ONE grace budget, so the whole call must finish well under the
// node-debug 3s ceiling.
func TestShutdownTerminalsBoundedWhenShellsIgnoreEOT(t *testing.T) {
	a := newTestApp()

	const n = 6
	for i := 0; i < n; i++ {
		fe := newFakeExecutor() // blocks on ctx; never exits on EOT
		id := "exec:bounded:" + string(rune('a'+i))
		sess := session.Start(fe, id)
		a.terminals[id] = sess
		<-fe.streamStarted
	}

	done := make(chan struct{})
	start := time.Now()
	go func() {
		a.shutdownTerminals()
		close(done)
	}()

	// Generous ceiling: one shared ~400ms grace budget (plus the ~150ms burst and
	// slack), far under the node-debug 3s ceiling. If the grace budget were
	// per-session (N × grace ≈ 2.4s here) or unbounded, this would blow past it.
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("shutdownTerminals did not complete within bounded time (%d sessions ignoring EOT)", n)
	}
	// Tighter bound the 2s select can't catch: a shared budget completes well under
	// 1.5s, whereas a per-session budget (6 × ~400ms) would exceed it. This proves
	// the sessions share ONE grace window rather than serializing.
	if elapsed := time.Since(start); elapsed > 1500*time.Millisecond {
		t.Fatalf("shutdownTerminals took %v, expected ~one shared grace budget, not N × grace", elapsed)
	}
}

// TestShutdownTerminalsFastExitOnEOT asserts the fast-exit path: when every
// shell exits cleanly on the EOT burst, TerminateGracefully returns true and the
// shared grace select unblocks via graceDone WELL BEFORE the outer
// time.After(DefaultGraceTimeout) ceiling fires. The whole shutdown therefore
// completes in a small fraction of the full grace budget. Measured elapsed with a
// generous-but-meaningful upper bound to stay non-flaky.
func TestShutdownTerminalsFastExitOnEOT(t *testing.T) {
	a := newTestApp()

	const n = 4
	type sf struct {
		fe   *fakeExecutor
		sess *session.Terminal
	}
	specs := make([]*sf, 0, n)
	for i := 0; i < n; i++ {
		fe := newFakeExecutor()
		fe.drainStdin = true
		fe.exitOnEOT = true // exit cleanly on the first Ctrl-D
		id := "exec:fast:" + string(rune('a'+i))
		sess := session.Start(fe, id)
		a.terminals[id] = sess
		specs = append(specs, &sf{fe: fe, sess: sess})
		<-fe.streamStarted
	}

	start := time.Now()
	a.shutdownTerminals()
	elapsed := time.Since(start)

	// The grace burst sends Ctrl-C then up to 4× Ctrl-D at ~30ms intervals; the
	// shells exit on the first EOT, so the grace phase unblocks shortly after the
	// first Ctrl-D rather than running the full ~400ms ceiling. 350ms is a
	// comfortable upper bound (well clear of intermittent -race/loaded-CI jitter
	// over the ~60ms minimum) that still sits below the ~550ms ignore-EOT timeout
	// path, so it still proves the fast-exit path is taken.
	if elapsed > 350*time.Millisecond {
		t.Fatalf("shutdownTerminals took %v with shells exiting on EOT; expected well under the full grace budget", elapsed)
	}

	if len(a.terminals) != 0 {
		t.Fatalf("terminals not drained after shutdown: %d", len(a.terminals))
	}
	sessions := make([]*session.Terminal, 0, len(specs))
	for _, s := range specs {
		sessions = append(sessions, s.sess)
	}
	waitAllDone(t, sessions, 2*time.Second, "fast-exit shutdown")

	for i, s := range specs {
		// The graceful sequence still reached each shell before it exited.
		assertGracefulSequence(t, s.fe.recordedStdin(), "session "+string(rune('a'+i)))
	}
}

// TestShutdownTerminalsExitsReplyPumps asserts the quit/shutdown path tears down
// reply-pump goroutines too: sessions opened WITH startReplyPump (the production
// open path) must, after shutdownTerminals(), have both Done() close and both
// pump goroutines exit. Building sessions via raw session.Start (as the sweep
// test above does) would not catch a reply-pump leak on the quit path.
func TestShutdownTerminalsExitsReplyPumps(t *testing.T) {
	a := newTestApp()
	model, _ := a.update(tea.WindowSizeMsg{Width: 120, Height: 40})
	a = drainModel(t, model)

	// Instrument the REAL pump goroutines BEFORE opening so both carry the signal.
	var exits atomic.Int32
	replyPumpExited = func() { exits.Add(1) }
	t.Cleanup(func() { replyPumpExited = nil })

	feA := newFakeExecutor()
	feB := newFakeExecutor()
	sessA, _ := openTestTerminalWithPump(t, &a, "exec:pumpA", feA)
	sessB, _ := openTestTerminalWithPump(t, &a, "exec:pumpB", feB)
	<-feA.streamStarted
	<-feB.streamStarted

	a.shutdownTerminals()

	waitAllDone(t, []*session.Terminal{sessA, sessB}, 2*time.Second, "reply-pump shutdown")

	// Two sessions × two pump goroutines each = four exits. A leak in any leaves
	// the count short and fails under the timeout.
	deadline := time.After(2 * time.Second)
	for {
		if exits.Load() == 4 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("reply-pump goroutines did not all exit after shutdown (got %d/4, goroutine leak)", exits.Load())
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// TestEphemeralCloseSurfacesNote asserts closing an ephemeral pod-debug pane
// surfaces the one-line "cannot be removed" note on the status bar.
func TestEphemeralCloseSurfacesNote(t *testing.T) {
	a := newTestApp()
	model, _ := a.update(tea.WindowSizeMsg{Width: 120, Height: 40})
	a = drainModel(t, model)

	fe := newFakeExecutor()
	_ = openTestTerminalWithMeta(t, &a, "debug:1", fe, terminalMeta{
		ephemeral: true,
		podName:   "mypod",
		namespace: "default",
	})

	note := a.ephemeralCloseNote("debug:1")
	if !strings.Contains(note, "ephemeral container") || !strings.Contains(note, "mypod") {
		t.Fatalf("unexpected ephemeral note: %q", note)
	}

	a, _ = a.closeFocusedTerminal()

	if _, ok := a.termCleanup["debug:1"]; ok {
		t.Fatal("ephemeral metadata not removed on close")
	}
	// The notify store reflects the ephemeral note (it backs the toast overlay).
	if !notifyContains(a, "ephemeral container") {
		t.Fatalf("notify store did not surface ephemeral note:\n%v", a.notify.List())
	}
}

// TestTermExitMsgEphemeralSetsExitNote asserts the TermExitMsg handler embeds
// the ephemeral "container can't be removed" note into the frozen pane's exit
// banner (SetExitNote wiring): an EPHEMERAL session receiving TermExitMsg ends
// with the pane's View() containing the note text.
func TestTermExitMsgEphemeralSetsExitNote(t *testing.T) {
	a := newTestApp()
	model, _ := a.update(tea.WindowSizeMsg{Width: 120, Height: 40})
	a = drainModel(t, model)

	fe := newFakeExecutor()
	id := "debug:exit:1"
	_ = openTestTerminalWithMeta(t, &a, id, fe, terminalMeta{
		ephemeral: true,
		podName:   "mypod",
		namespace: "default",
	})

	// Deliver the exit message for the ephemeral session.
	model, cmd := a.update(msgs.TermExitMsg{ID: id, Code: 0})
	a = drainModel(t, model)
	if cmd != nil {
		t.Fatalf("TermExitMsg must not reschedule, got %T", cmd)
	}

	tp, found := a.layout.TerminalPaneByID(id)
	if !found {
		t.Fatal("terminal pane not found after TermExitMsg")
	}
	if !tp.Exited() {
		t.Fatal("pane was not marked exited")
	}
	// The ephemeral note is embedded in the frozen pane's exit banner.
	if !strings.Contains(tp.View(), "ephemeral container") {
		t.Fatalf("ephemeral exit note not embedded in pane View:\n%s", tp.View())
	}
}

// TestDebugReadyErrorSurfacesAndCleansUp asserts the DebugReadyMsg error path
// surfaces the error, removes the placeholder pane, and clears metadata without
// leaving a half-open session.
func TestDebugReadyErrorSurfacesAndCleansUp(t *testing.T) {
	a := newTestApp()
	model, _ := a.update(tea.WindowSizeMsg{Width: 120, Height: 40})
	a = drainModel(t, model)

	// Seed a resource sibling so the placeholder pane is removable (count >= 2).
	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
	a.layout.AddSplit(&mockPlugin{name: "pods", gvr: gvr}, "default", "ctx-1")

	// Place a debug placeholder pane (no session yet — pre-flight pending).
	id := "debug:err:1"
	w, h := a.layout.SplitSeedSize()
	tp := ui.NewTerminalPane("debug: x", "ctx-1", w, h)
	tp.SetID(id)
	a.termCleanup[id] = terminalMeta{ephemeral: true, podName: "x", namespace: "default"}
	a.layout.AddTerminalSplit(tp)
	a.syncTerminalSizes()

	before := a.layout.SplitCount()

	model, _ = a.update(msgs.DebugReadyMsg{ID: id, Err: errTestPreflight})
	a = drainModel(t, model)

	if a.layout.SplitCount() != before-1 {
		t.Fatalf("placeholder pane not removed on error: %d -> %d", before, a.layout.SplitCount())
	}
	if _, ok := a.termCleanup[id]; ok {
		t.Fatal("cleanup metadata not cleared on error")
	}
	if _, ok := a.terminals[id]; ok {
		t.Fatal("no session should exist for a failed pre-flight")
	}
	if !notifyContains(a, "boom") {
		t.Fatalf("error not surfaced in notify store:\n%v", a.notify.List())
	}
}

// TestDebugReadyErrorLastSplitMarksPaneExited asserts the last-split error path:
// when the debug placeholder is the ONLY split, the error path must not strand a
// session-less, unclosable pane (CloseCurrentSplit refuses to empty the layout).
// Instead the placeholder is frozen into the exited state — closeable via the
// normal exited-pane path — with the failure reason in its exit note.
func TestDebugReadyErrorLastSplitMarksPaneExited(t *testing.T) {
	a := newTestApp()
	model, _ := a.update(tea.WindowSizeMsg{Width: 120, Height: 40})
	a = drainModel(t, model)

	// Place the debug placeholder as the SOLE split (no sibling).
	id := "debug:err:last"
	w, h := a.layout.SplitSeedSize()
	tp := ui.NewTerminalPane("debug: x", "ctx-1", w, h)
	tp.SetID(id)
	a.termCleanup[id] = terminalMeta{ephemeral: true, podName: "x", namespace: "default"}
	a.layout.AddTerminalSplit(tp)
	a.syncTerminalSizes()

	if a.layout.SplitCount() != 1 {
		t.Fatalf("test setup expected a single split, got %d", a.layout.SplitCount())
	}

	model, _ = a.update(msgs.DebugReadyMsg{ID: id, Err: errTestPreflight})
	a = drainModel(t, model)

	// The pane stays on screen (cannot empty the layout) but must be marked exited
	// so it is closeable via the normal exited-pane path rather than orphaned.
	gone, found := a.layout.TerminalPaneByID(id)
	if !found {
		t.Fatal("last-split error path unexpectedly removed the sole pane (empty layout)")
	}
	if !gone.Exited() {
		t.Fatal("last-split placeholder not marked exited — pane is orphaned/unclosable")
	}
	if !strings.Contains(gone.View(), "debug failed") {
		t.Fatalf("exit note with failure reason not shown in pane View:\n%s", gone.View())
	}
	if _, ok := a.termCleanup[id]; ok {
		t.Fatal("cleanup metadata not cleared on error")
	}
	if _, ok := a.terminals[id]; ok {
		t.Fatal("no session should exist for a failed pre-flight")
	}
}

var errTestPreflight = errTest("boom")

type errTest string

func (e errTest) Error() string { return string(e) }

// placeDebugPlaceholder inserts a debug placeholder terminal pane (no live
// session yet — pre-flight pending) plus a resource sibling so the pane is
// removable, mirroring the on-screen state between openDebugTerminal and the
// arriving DebugReadyMsg. Returns the pane id.
func placeDebugPlaceholder(t *testing.T, a *App, id string, meta terminalMeta) {
	t.Helper()
	if a.layout.SplitCount() == 0 {
		gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
		a.layout.AddSplit(&mockPlugin{name: "pods", gvr: gvr}, "default", "ctx-1")
	}
	w, h := a.layout.SplitSeedSize()
	tp := ui.NewTerminalPane("debug: x", "ctx-1", w, h)
	tp.SetID(id)
	a.termCleanup[id] = meta
	a.layout.AddTerminalSplit(tp)
	a.syncTerminalSizes()
}

// TestDebugReadySuccessStartsSession asserts the DebugReadyMsg success path:
// with a fake attach-executor seam it builds a live session, records the pod
// identity in cleanup metadata, and returns the byte-pump cmd.
func TestDebugReadySuccessStartsSession(t *testing.T) {
	a := newTestApp()
	model, _ := a.update(tea.WindowSizeMsg{Width: 120, Height: 40})
	a = drainModel(t, model)

	// Fake the attach executor so no real cluster Config is needed.
	fe := newFakeExecutor()
	a.attachExecutorFn = func(_ *k8s.Client, _, _, _ string) (session.Executor, error) {
		return fe, nil
	}

	client, _ := fakeDeleteClient()
	id := "debug-node:ok:1"
	placeDebugPlaceholder(t, &a, id, terminalMeta{nodeDebug: true, client: client})

	model, cmd := a.update(msgs.DebugReadyMsg{
		ID:            id,
		PodName:       "ktui-debug-node1-aaaaa",
		Namespace:     "default",
		ContainerName: "debugger",
		NodeMode:      true,
	})
	a = drainModel(t, model)

	if cmd == nil {
		t.Fatal("success path should return the byte-pump cmd (got nil)")
	}
	sess, ok := a.terminals[id]
	if !ok || sess == nil {
		t.Fatal("success path should register a live session")
	}
	defer sess.Close()

	// The pane must STILL be present in the layout after success — a regression
	// that erroneously removes the placeholder on success is caught here.
	if _, found := a.layout.TerminalPaneByID(id); !found {
		t.Fatal("success path must keep the pane in the layout (it was removed)")
	}

	meta, ok := a.termCleanup[id]
	if !ok {
		t.Fatal("cleanup metadata should remain after success")
	}
	if meta.podName != "ktui-debug-node1-aaaaa" || meta.namespace != "default" {
		t.Fatalf("cleanup metadata not filled with pod identity: %+v", meta)
	}
}

// TestDebugReadyPlaceholderGoneDeletesNodePod asserts the placeholder-gone
// branch (found==false) with NodeMode: the orphaned node pod is deleted and the
// cleanup metadata cleared (no leak when the user closed the pane mid-pre-flight).
func TestDebugReadyPlaceholderGoneDeletesNodePod(t *testing.T) {
	a := newTestApp()
	model, _ := a.update(tea.WindowSizeMsg{Width: 120, Height: 40})
	a = drainModel(t, model)

	client, deleted := fakeDeleteClient("orphan-pod")
	id := "debug-node:gone:1"
	// No pane is placed — only the cleanup metadata exists, simulating a pane the
	// user closed while the pre-flight ran.
	a.termCleanup[id] = terminalMeta{nodeDebug: true, client: client}

	model, cmd := a.update(msgs.DebugReadyMsg{
		ID:        id,
		PodName:   "orphan-pod",
		Namespace: "default",
		NodeMode:  true,
	})
	a = drainModel(t, model)

	if cmd != nil {
		t.Fatalf("placeholder-gone path should not start a pump (got %T)", cmd)
	}
	select {
	case name := <-deleted:
		if name != "orphan-pod" {
			t.Fatalf("deleted unexpected pod: %s", name)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("orphaned node pod was not deleted")
	}
	if _, ok := a.termCleanup[id]; ok {
		t.Fatal("cleanup metadata not cleared for the gone placeholder")
	}
	if _, ok := a.terminals[id]; ok {
		t.Fatal("no session should be registered for a gone placeholder")
	}
}

// TestDebugReadyAttachExecErrorDeletesNodePod asserts the NewAttachExecutor
// error path: the error is surfaced on the status bar, the placeholder pane is
// removed, and (node mode) the created pod is deleted.
func TestDebugReadyAttachExecErrorDeletesNodePod(t *testing.T) {
	a := newTestApp()
	model, _ := a.update(tea.WindowSizeMsg{Width: 120, Height: 40})
	a = drainModel(t, model)

	a.attachExecutorFn = func(_ *k8s.Client, _, _, _ string) (session.Executor, error) {
		return nil, errTest("attach failed")
	}

	client, deleted := fakeDeleteClient("err-pod")
	id := "debug-node:err:1"
	placeDebugPlaceholder(t, &a, id, terminalMeta{nodeDebug: true, client: client})
	before := a.layout.SplitCount()

	model, _ = a.update(msgs.DebugReadyMsg{
		ID:        id,
		PodName:   "err-pod",
		Namespace: "default",
		NodeMode:  true,
	})
	a = drainModel(t, model)

	if a.layout.SplitCount() != before-1 {
		t.Fatalf("placeholder not removed on attach error: %d -> %d", before, a.layout.SplitCount())
	}
	if _, ok := a.termCleanup[id]; ok {
		t.Fatal("cleanup metadata not cleared on attach error")
	}
	if _, ok := a.terminals[id]; ok {
		t.Fatal("no session should exist after attach error")
	}
	if !notifyContains(a, "attach failed") {
		t.Fatalf("attach error not surfaced in notify store:\n%v", a.notify.List())
	}
	select {
	case name := <-deleted:
		if name != "err-pod" {
			t.Fatalf("deleted unexpected pod: %s", name)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("node pod not deleted after attach-exec failure")
	}
}
