package app

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/aohoyd/aku/internal/k8s/session"
	"github.com/aohoyd/aku/internal/layout"
	"github.com/aohoyd/aku/internal/msgs"
	"github.com/aohoyd/aku/internal/ui"
	"github.com/charmbracelet/x/ansi"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/remotecommand"
	utilexec "k8s.io/client-go/util/exec"
)

// fakeExecutor simulates a live SPDY shell without a cluster. It mirrors the
// fake used in internal/k8s/session/terminal_test.go so the app-level wiring can
// be driven deterministically: initialWrite is shipped to stdout when the
// stream starts; readStdin records one chunk read off stdin; returnImmediately
// ends the stream without waiting for cancellation (simulating shell exit).
type fakeExecutor struct {
	initialWrite []byte
	readStdin    bool
	// drainStdin, when true, drains stdin continuously (multi-reply / graceful
	// burst tests). Mirrors the session-package fake's drainStdin/exitOnEOT
	// drain-and-exit-on-EOT behavior so app- and session-level wiring use the same
	// fake semantics under the same field names.
	drainStdin        bool
	exitOnEOT         bool // when true (with drainStdin), end the stream on the first 0x04
	readSize          bool // when true, read one size off the size queue and record it
	exitErr           error
	returnImmediately bool

	mu            sync.Mutex
	gotStdin      []byte
	gotSize       *remotecommand.TerminalSize
	allSizes      []remotecommand.TerminalSize // every size drained, in order
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

	if f.readSize && opts.TerminalSizeQueue != nil {
		// Drain sizes continuously in the background and record the latest, so a
		// resize that lands after the stream starts is observed (Next blocks until
		// a new size arrives, returns nil when the session is torn down).
		go func() {
			for {
				size := opts.TerminalSizeQueue.Next()
				if size == nil {
					return
				}
				f.mu.Lock()
				f.gotSize = size
				f.allSizes = append(f.allSizes, *size)
				f.mu.Unlock()
			}
		}()
	}

	if f.drainStdin && opts.Stdin != nil {
		// Drain stdin continuously in the background so multiple replies (e.g.
		// back-to-back terminal queries) are all recorded. Read returns an error
		// when the stdin pipe is closed at teardown, ending the loop. When
		// exitOnEOT is set, the first 0x04 (Ctrl-D) byte ends the stream cleanly,
		// mimicking a shell that exits on EOT (so TerminateGracefully returns true).
		sawEOT := make(chan struct{})
		go func() {
			buf := make([]byte, 1024)
			for {
				n, err := opts.Stdin.Read(buf)
				if n > 0 {
					chunk := buf[:n]
					f.mu.Lock()
					f.gotStdin = append(f.gotStdin, chunk...)
					f.mu.Unlock()
					if f.exitOnEOT && bytes.IndexByte(chunk, 0x04) >= 0 {
						close(sawEOT)
						return
					}
				}
				if err != nil {
					return
				}
			}
		}()
		if f.exitOnEOT {
			select {
			case <-ctx.Done():
				return f.exitErr
			case <-sawEOT:
				// Shell "exited" on Ctrl-D: return cleanly so the stream ends.
				return f.exitErr
			}
		}
	} else if f.readStdin && opts.Stdin != nil {
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

// lastSize returns the most recently drained size, or nil if none yet.
func (f *fakeExecutor) lastSize() *remotecommand.TerminalSize {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.allSizes) == 0 {
		return nil
	}
	last := f.allSizes[len(f.allSizes)-1]
	return &last
}

// openTestTerminal injects a started terminal session + pane into the app under
// the given id, mirroring what openExecTerminal does (minus the k8s executor
// construction). It returns the live session so tests can drive/inspect it.
func openTestTerminal(t *testing.T, a *App, id string, fe *fakeExecutor) *session.Terminal {
	t.Helper()
	// newTestApp's plugin.Reset() leaves the registry empty, so the App starts
	// with zero splits. Seed one resource split so the terminal pane has a
	// sibling — focus-move and close-split assertions need SplitCount() >= 2.
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
	a.syncTerminalSizes()
	return sess
}

// drainModel type-asserts an Update result back to App for chaining.
func drainModel(t *testing.T, m tea.Model) App {
	t.Helper()
	a, ok := m.(App)
	if !ok {
		t.Fatalf("model is not App: %T", m)
	}
	return a
}

// runCmd executes a tea.Cmd and returns its message, failing on timeout. The
// byte-pump command blocks on a channel read, so it is run in a goroutine.
func runCmd(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	if cmd == nil {
		t.Fatal("nil cmd")
	}
	ch := make(chan tea.Msg, 1)
	go func() { ch <- cmd() }()
	select {
	case msg := <-ch:
		return msg
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for cmd result")
		return nil
	}
}

// TestTerminalBytesReachPaneAndReschedule drives the readTermBytes →
// TermBytesMsg → pane.Write → reschedule loop and asserts the shell output ends
// up rendered in the pane's View.
func TestTerminalBytesReachPaneAndReschedule(t *testing.T) {
	a := newTestApp()
	model, _ := a.update(tea.WindowSizeMsg{Width: 100, Height: 40})
	a = drainModel(t, model)

	fe := newFakeExecutor()
	fe.initialWrite = []byte("hello-term")
	sess := openTestTerminal(t, &a, "exec:1", fe)
	defer sess.Close()

	// First read picks up the initial write.
	msg := runCmd(t, readTermBytes(sess))
	bmsg, ok := msg.(msgs.TermBytesMsg)
	if !ok {
		t.Fatalf("expected TermBytesMsg, got %T", msg)
	}
	if bmsg.ID != "exec:1" || !bytes.Equal(bmsg.Data, []byte("hello-term")) {
		t.Fatalf("unexpected TermBytesMsg: %+v", bmsg)
	}

	// Feed it through update(): the pane is written and the loop reschedules.
	model, cmd := a.update(bmsg)
	a = drainModel(t, model)
	if cmd == nil {
		t.Fatal("TermBytesMsg should reschedule readTermBytes (got nil cmd)")
	}

	tp, found := a.layout.TerminalPaneByID("exec:1")
	if !found {
		t.Fatal("terminal pane not found after TermBytesMsg")
	}
	if !strings.Contains(tp.View(), "hello-term") {
		t.Fatalf("pane View did not contain shell output:\n%s", tp.View())
	}
}

// TestTermBytesUnknownIDIsNoOp asserts a TermBytesMsg for an id not in the
// registry (session closed between dispatch and arrival) is a no-op: it returns
// no reschedule cmd and does not panic on the nil-map lookup.
func TestTermBytesUnknownIDIsNoOp(t *testing.T) {
	a := newTestApp()
	model, _ := a.update(tea.WindowSizeMsg{Width: 100, Height: 40})
	a = drainModel(t, model)

	model, cmd := a.update(msgs.TermBytesMsg{ID: "exec:gone", Data: []byte("late bytes")})
	_ = drainModel(t, model)
	if cmd != nil {
		t.Fatalf("unknown-id TermBytesMsg must not reschedule, got cmd %T", cmd)
	}
}

// TestLiveTerminalCtrlCForwardsAndDoesNotQuit asserts that ctrl+c, while a live
// terminal pane is focused, is forwarded to the shell (byte 0x03) and does NOT
// produce tea.Quit — the terminal interception is ordered before the global
// ctrl+c→quit guard.
func TestLiveTerminalCtrlCForwardsAndDoesNotQuit(t *testing.T) {
	a := newTestApp()
	model, _ := a.update(tea.WindowSizeMsg{Width: 100, Height: 40})
	a = drainModel(t, model)

	fe := newFakeExecutor()
	fe.readStdin = true
	sess := openTestTerminal(t, &a, "exec:ctrlc", fe)
	defer sess.Close()
	<-fe.streamStarted

	model, cmd := a.update(ctrlKey('c'))
	a = drainModel(t, model)

	// No quit: the global ctrl+c guard must not have fired.
	if cmd != nil {
		t.Fatalf("ctrl+c on a live terminal produced a command (quit leaked?): %T", cmd)
	}
	// Still focused on the terminal (key was consumed by the pane).
	if _, ok := a.layout.FocusedPane().(*ui.TerminalPane); !ok {
		t.Fatal("focus left the terminal pane on ctrl+c")
	}
	// 0x03 reached the shell.
	deadline := time.After(2 * time.Second)
	for {
		if bytes.Equal(fe.recordedStdin(), []byte{0x03}) {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("ctrl+c byte 0x03 not forwarded to shell; got %q", fe.recordedStdin())
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// TestLiveTerminalPrefixCloseRemovesPaneAndSession asserts prefix then 'x' on a
// live terminal pane, via the real key-routing path, removes the pane
// (SplitCount decreases) and tears down the session (removed from a.terminals).
func TestLiveTerminalPrefixCloseRemovesPaneAndSession(t *testing.T) {
	a := newTestApp()
	model, _ := a.update(tea.WindowSizeMsg{Width: 100, Height: 40})
	a = drainModel(t, model)

	fe := newFakeExecutor()
	sess := openTestTerminal(t, &a, "exec:close", fe)
	defer sess.Close()

	if _, ok := a.layout.FocusedPane().(*ui.TerminalPane); !ok {
		t.Fatal("precondition: terminal pane should be focused after open")
	}
	before := a.layout.SplitCount()

	// Prefix (ctrl+a) flips to nav mode.
	model, _ = a.update(ctrlKey('a'))
	a = drainModel(t, model)
	// 'x' → PaneCmdClose → closeFocusedTerminal removes pane + session.
	model, _ = a.update(printableKey('x'))
	a = drainModel(t, model)

	if a.layout.SplitCount() != before-1 {
		t.Fatalf("prefix+x should remove the pane: split count %d → %d", before, a.layout.SplitCount())
	}
	if _, ok := a.terminals["exec:close"]; ok {
		t.Fatal("prefix+x should tear down the session (still in registry)")
	}

	// Session was cancelled.
	select {
	case <-sess.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("session.Done() did not close after prefix+x close")
	}
}

// TestTerminalWindowResizePushesInnerSize asserts that resizing the window
// resizes the live terminal session to the pane's INNER dimensions (outer minus
// border chrome), verified by capturing the size the fake executor reads off the
// size queue.
func TestTerminalWindowResizePushesInnerSize(t *testing.T) {
	a := newTestApp()
	model, _ := a.update(tea.WindowSizeMsg{Width: 100, Height: 40})
	a = drainModel(t, model)

	fe := newFakeExecutor()
	fe.readSize = true
	sess := openTestTerminal(t, &a, "exec:resize", fe)
	defer sess.Close()
	// Confirm the fake's stream goroutine (and thus its size-drain goroutine) is
	// running BEFORE the resize is sent, so the resize cannot be lost to a
	// not-yet-started drain. The size-1 resize channel uses replace-latest
	// semantics, so the FINAL pushed size is always observed.
	<-fe.streamStarted

	// Resize the window; syncTerminalSizes pushes the new inner size.
	model, _ = a.update(tea.WindowSizeMsg{Width: 120, Height: 50})
	a = drainModel(t, model)

	// Derive the expected inner size the layout assigned the pane after the resize.
	iw, ih, ok := a.layout.TerminalPaneInnerSize("exec:resize")
	if !ok || iw <= 0 || ih <= 0 {
		t.Fatalf("could not resolve terminal inner size: %d×%d ok=%v", iw, ih, ok)
	}

	// Poll until the LAST drained size equals the pane's post-resize inner
	// dimensions. Asserting on the last (not any) recorded size eliminates a
	// spurious pass from an earlier (open-time) size that happened to match, and
	// the replace-latest resize channel guarantees the final size is delivered.
	deadline := time.After(2 * time.Second)
	var last *remotecommand.TerminalSize
	for {
		last = fe.lastSize()
		if last != nil && int(last.Width) == iw && int(last.Height) == ih {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("resize never delivered inner %d×%d as the last size (last %+v)", iw, ih, last)
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// TestTerminalExitMarksPaneAndStops asserts a TermExitMsg freezes the pane and
// that the byte-pump naturally stops (the closed Out channel yields TermExitMsg,
// whose handler returns no reschedule cmd).
func TestTerminalExitMarksPaneAndStops(t *testing.T) {
	a := newTestApp()
	model, _ := a.update(tea.WindowSizeMsg{Width: 100, Height: 40})
	a = drainModel(t, model)

	fe := newFakeExecutor()
	fe.returnImmediately = true
	fe.exitErr = utilexec.CodeExitError{Err: context.Canceled, Code: 7}
	sess := openTestTerminal(t, &a, "exec:2", fe)
	defer sess.Close()

	// The stream ends on its own; Out closes → readTermBytes yields TermExitMsg.
	msg := runCmd(t, readTermBytes(sess))
	emsg, ok := msg.(msgs.TermExitMsg)
	if !ok {
		t.Fatalf("expected TermExitMsg, got %T", msg)
	}
	if emsg.ID != "exec:2" || emsg.Code != 7 {
		t.Fatalf("unexpected TermExitMsg: %+v", emsg)
	}

	model, cmd := a.update(emsg)
	a = drainModel(t, model)
	if cmd != nil {
		t.Fatal("TermExitMsg must not reschedule (got non-nil cmd)")
	}

	tp, found := a.layout.TerminalPaneByID("exec:2")
	if !found {
		t.Fatal("terminal pane not found after TermExitMsg")
	}
	if !tp.Exited() {
		t.Fatal("pane was not marked exited")
	}
}

// TestTerminalKeyRoutingTypingSendsBytes asserts that a printable key, while a
// live terminal pane is focused, is forwarded to the session's stdin (observed
// via the fake executor's recorded stdin).
func TestTerminalKeyRoutingTypingSendsBytes(t *testing.T) {
	a := newTestApp()
	model, _ := a.update(tea.WindowSizeMsg{Width: 100, Height: 40})
	a = drainModel(t, model)

	fe := newFakeExecutor()
	fe.readStdin = true
	sess := openTestTerminal(t, &a, "exec:3", fe)
	defer sess.Close()

	<-fe.streamStarted

	// 'a' is a printable key → encoded and written to the shell.
	key := tea.KeyPressMsg{Code: 'a', Text: "a"}
	model, _ = a.update(key)
	a = drainModel(t, model)

	deadline := time.After(2 * time.Second)
	for {
		if bytes.Equal(fe.recordedStdin(), []byte("a")) {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("stdin not recorded; got %q", fe.recordedStdin())
		case <-time.After(5 * time.Millisecond):
		}
	}
}

// TestOverlayBypassesTerminalInterception asserts that when an overlay is open
// (activeOverlay != overlayNone), a printable key while a live terminal pane is
// focused is NOT intercepted/forwarded to the shell — terminal interception is
// gated on overlayNone, so the key is consumed by the overlay instead.
func TestOverlayBypassesTerminalInterception(t *testing.T) {
	a := newTestApp()
	model, _ := a.update(tea.WindowSizeMsg{Width: 100, Height: 40})
	a = drainModel(t, model)

	fe := newFakeExecutor()
	fe.readStdin = true
	sess := openTestTerminal(t, &a, "exec:overlay", fe)
	defer sess.Close()
	<-fe.streamStarted

	if _, ok := a.layout.FocusedPane().(*ui.TerminalPane); !ok {
		t.Fatal("precondition: terminal pane should be focused")
	}

	// Open the search bar overlay; while it is up, keys belong to the overlay.
	a.activeOverlay = overlaySearchBar
	a.searchBar.Open(msgs.SearchModeSearch)

	// A printable key must NOT reach the shell — it is consumed by the overlay.
	model, _ = a.update(printableKey('a'))
	a = drainModel(t, model)

	// The terminal interception (routeTerminalKey) is synchronous within update():
	// by the time update() returns, the key has already been routed (to the overlay
	// or the shell). No sleep needed — assert immediately.
	if got := fe.recordedStdin(); len(got) != 0 {
		t.Fatalf("overlay-open: key leaked to shell: %q", got)
	}
	// Positively assert the OVERLAY processed the key (so the test cannot pass if
	// the key were silently dropped instead of intercepted by the overlay).
	if got := a.searchBar.Value(); got != "a" {
		t.Fatalf("overlay did not consume the key: searchBar value = %q, want %q", got, "a")
	}
}

// TestTerminalKeyRoutingPrefixFocusesSibling asserts that the prefix key
// followed by 'l' issues a focus-right (PaneCmdFocusRight → FocusNext) rather
// than sending bytes to the shell. The terminal pane is inserted after an
// initial resource split, so FocusNext wraps focus back to the resource pane.
func TestTerminalKeyRoutingPrefixFocusesSibling(t *testing.T) {
	a := newTestApp()
	model, _ := a.update(tea.WindowSizeMsg{Width: 100, Height: 40})
	a = drainModel(t, model)

	fe := newFakeExecutor()
	fe.readStdin = true
	sess := openTestTerminal(t, &a, "exec:4", fe)
	defer sess.Close()
	<-fe.streamStarted

	if _, ok := a.layout.FocusedPane().(*ui.TerminalPane); !ok {
		t.Fatal("terminal pane should be focused after open")
	}

	// Prefix key flips the pane into nav mode (consumed, no bytes).
	prefix := tea.KeyPressMsg{Code: 'a', Mod: tea.ModCtrl}
	model, _ = a.update(prefix)
	a = drainModel(t, model)

	// 'l' is interpreted as focus-right → FocusNext moves off the terminal pane.
	lkey := tea.KeyPressMsg{Code: 'l', Text: "l"}
	model, _ = a.update(lkey)
	a = drainModel(t, model)

	if _, ok := a.layout.FocusedPane().(*ui.TerminalPane); ok {
		t.Fatal("focus should have moved off the terminal pane after prefix+l")
	}

	// No shell bytes should have been produced by the prefix sequence.
	if got := fe.recordedStdin(); len(got) != 0 {
		t.Fatalf("prefix+l leaked bytes to shell: %q", got)
	}
}

// TestLiveTerminalCapturedAltZFallsThroughToZoom asserts that a captured key
// (alt+z, bound to toggle-zoom) is NOT consumed by a focused LIVE terminal pane:
// routeTerminalKey returns handled=false so the App's trie acts on it, and a
// full update() flips the split-zoom state. This is the capture mechanism that
// lets selected keys reach aku even over a live shell.
func TestLiveTerminalCapturedAltZFallsThroughToZoom(t *testing.T) {
	a := newTestApp()
	model, _ := a.update(tea.WindowSizeMsg{Width: 100, Height: 40})
	a = drainModel(t, model)

	fe := newFakeExecutor()
	fe.readStdin = true
	sess := openTestTerminal(t, &a, "exec:altz", fe)
	defer sess.Close()
	<-fe.streamStarted

	tp, ok := a.layout.FocusedPane().(*ui.TerminalPane)
	if !ok {
		t.Fatal("precondition: live terminal pane should be focused")
	}
	if a.layout.SplitZoomed() {
		t.Fatal("precondition: split should not start zoomed")
	}

	altZ := tea.KeyPressMsg{Code: 'z', Mod: tea.ModAlt}

	// routeTerminalKey must NOT consume alt+z (it is captured for the trie).
	handled, _, _ := a.routeTerminalKey(tp, altZ)
	if handled {
		t.Fatal("captured alt+z should not be consumed by the live terminal pane")
	}

	// Driven through update(): alt+z falls through to the trie's toggle-zoom.
	model, _ = a.update(altZ)
	a = drainModel(t, model)
	if !a.layout.SplitZoomed() {
		t.Fatal("captured alt+z should toggle split zoom via the trie")
	}
	// alt+z must not have leaked to the shell.
	if got := fe.recordedStdin(); len(got) != 0 {
		t.Fatalf("captured alt+z leaked bytes to shell: %q", got)
	}
}

// TestLiveTerminalCapturedShiftArrowMovesFocus asserts that a captured
// shift-arrow (shift+left → focus-left) over a focused LIVE terminal pane is NOT
// consumed by the pane: routeTerminalKey returns handled=false so the App's trie
// acts on it and actually moves split focus to the adjacent pane — without
// leaking bytes to the shell. This mirrors the captured-alt+z fall-through but
// for the focus-move path.
func TestLiveTerminalCapturedShiftArrowMovesFocus(t *testing.T) {
	a := newTestApp()
	model, _ := a.update(tea.WindowSizeMsg{Width: 100, Height: 40})
	a = drainModel(t, model)

	fe := newFakeExecutor()
	fe.readStdin = true
	sess := openTestTerminal(t, &a, "exec:shift", fe)
	defer sess.Close()
	<-fe.streamStarted

	// Precondition: at least two splits with the live terminal focused.
	if a.layout.SplitCount() < 2 {
		t.Fatalf("precondition: want >=2 splits, got %d", a.layout.SplitCount())
	}
	tp, ok := a.layout.FocusedPane().(*ui.TerminalPane)
	if !ok {
		t.Fatal("precondition: live terminal pane should be focused")
	}
	focusedBefore := a.layout.FocusIndex()

	// The default layout orientation is vertical, where sibling-split focus moves
	// are up/down (left/right address the detail panel). shift+up → focus-up →
	// FocusPrev, moving focus to the adjacent split.
	shiftUp := tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModShift}

	// routeTerminalKey must NOT consume shift+up (it is captured for the trie).
	handled, _, _ := a.routeTerminalKey(tp, shiftUp)
	if handled {
		t.Fatal("captured shift+up should not be consumed by the live terminal pane")
	}

	// Driven through update(): shift+up falls through to the trie's focus-up,
	// moving focus to the adjacent split.
	model, _ = a.update(shiftUp)
	a = drainModel(t, model)

	if a.layout.FocusIndex() == focusedBefore {
		t.Fatalf("captured shift+up should move split focus off %d", focusedBefore)
	}
	if _, ok := a.layout.FocusedPane().(*ui.TerminalPane); ok {
		t.Fatal("focus should have moved off the terminal pane after shift+up")
	}
	// shift+up must not have leaked to the shell.
	if got := fe.recordedStdin(); len(got) != 0 {
		t.Fatalf("captured shift+up leaked bytes to shell: %q", got)
	}
}

// TestExitedTerminalKeyFallsThrough asserts that once a terminal pane has
// exited, keys are NOT intercepted (they reach the normal trie), so close/quit
// keybindings keep working.
func TestExitedTerminalKeyFallsThrough(t *testing.T) {
	a := newTestApp()
	model, _ := a.update(tea.WindowSizeMsg{Width: 100, Height: 40})
	a = drainModel(t, model)

	fe := newFakeExecutor()
	fe.returnImmediately = true
	sess := openTestTerminal(t, &a, "exec:5", fe)
	defer sess.Close()

	tp, _ := a.layout.TerminalPaneByID("exec:5")
	tp.MarkExited(0)

	before := a.layout.SplitCount()

	// ctrl+w on an exited terminal pane falls through to the close-split path.
	model, _ = a.update(tea.KeyPressMsg{Code: 'w', Mod: tea.ModCtrl})
	a = drainModel(t, model)

	if a.layout.SplitCount() != before-1 {
		t.Fatalf("exited terminal should close via trie: split count %d → %d", before, a.layout.SplitCount())
	}
	if _, ok := a.terminals["exec:5"]; ok {
		t.Fatal("closing the exited terminal should remove its session from the registry")
	}
}

// TestCurrentContextTerminalWhenFocused asserts that a focused (live) terminal
// pane reports the "terminal" key-context, driving the statusbar/help hints and
// the (exited-pane) keybinding trie.
func TestCurrentContextTerminalWhenFocused(t *testing.T) {
	a := newTestApp()
	model, _ := a.update(tea.WindowSizeMsg{Width: 100, Height: 40})
	a = drainModel(t, model)

	fe := newFakeExecutor()
	sess := openTestTerminal(t, &a, "exec:ctx", fe)
	defer sess.Close()

	if _, ok := a.layout.FocusedPane().(*ui.TerminalPane); !ok {
		t.Fatal("precondition: terminal pane should be focused after open")
	}
	ct, rn := a.currentContext()
	if ct != "terminal" {
		t.Fatalf("focused terminal currentContext = %q, want terminal", ct)
	}
	if rn != "" {
		t.Fatalf("focused terminal resourceName = %q, want empty", rn)
	}

	// Focusing the sibling resource pane flips context back to resources.
	a.layout.FocusSplitAt(0)
	if ct, _ := a.currentContext(); ct != "resources" {
		t.Fatalf("resource pane currentContext = %q, want resources", ct)
	}
}

// TestLiveTerminalKeyNotDoubleDispatched asserts a key consumed by the live
// terminal's prefix machine (routeTerminalKey) does NOT also flow through the
// trie. A printable key is forwarded to the shell and must not, e.g., move the
// resource cursor or resolve a trie command. We verify by confirming the key
// did not change the trie state (still at root) and the pane stayed focused.
func TestLiveTerminalKeyNotDoubleDispatched(t *testing.T) {
	a := newTestApp()
	model, _ := a.update(tea.WindowSizeMsg{Width: 100, Height: 40})
	a = drainModel(t, model)

	fe := newFakeExecutor()
	fe.readStdin = true
	sess := openTestTerminal(t, &a, "exec:nodbl", fe)
	defer sess.Close()
	<-fe.streamStarted

	// 'q' would, via the trie, resolve to "quit". Routed to a live terminal it
	// must instead be encoded to the shell and consumed before the trie.
	model, cmd := a.update(printableKey('q'))
	a = drainModel(t, model)

	// No quit command was produced.
	if cmd != nil {
		t.Fatalf("live-terminal 'q' produced a command (possible double-dispatch): %T", cmd)
	}
	// Still on the terminal pane (trie did not act).
	if _, ok := a.layout.FocusedPane().(*ui.TerminalPane); !ok {
		t.Fatal("focus left the terminal pane (key reached the trie)")
	}
	// The trie remains at root (no key was consumed by it).
	if !a.keyTrie.AtRoot() {
		t.Fatal("trie advanced — live-terminal key leaked into the trie")
	}
}

// TestMouseWheelOverTerminalScrollsScrollback asserts a wheel event over a
// terminal pane adjusts its scrollback offset (and does not move focus).
func TestMouseWheelOverTerminalScrollsScrollback(t *testing.T) {
	a := newTestApp()
	a.config.Mouse.Enabled = true
	model, _ := a.update(tea.WindowSizeMsg{Width: 100, Height: 40})
	a = drainModel(t, model)

	fe := newFakeExecutor()
	sess := openTestTerminal(t, &a, "exec:wheel", fe)
	defer sess.Close()

	tp, found := a.layout.TerminalPaneByID("exec:wheel")
	if !found {
		t.Fatal("terminal pane not found")
	}
	tp.SetScrollback(1000)
	// Push lines so scrollback exists.
	for i := 0; i < 50; i++ {
		_, _ = tp.Write([]byte("row\r\n"))
	}
	if tp.ScrollOffset() != 0 {
		t.Fatalf("precondition: offset should be 0, got %d", tp.ScrollOffset())
	}

	// Find the terminal pane's rect and aim the wheel at its interior.
	idx := terminalSplitIndex(t, &a, "exec:wheel")
	rect := paneRectForSplit(t, &a, idx)
	focusBefore := a.layout.FocusIndex()

	model, _ = a.update(tea.MouseWheelMsg{X: rect.X + 2, Y: rect.Y + 1, Button: tea.MouseWheelUp})
	a = drainModel(t, model)

	tp, _ = a.layout.TerminalPaneByID("exec:wheel")
	if tp.ScrollOffset() != terminalWheelLines {
		t.Fatalf("wheel-up over terminal: offset = %d, want %d", tp.ScrollOffset(), terminalWheelLines)
	}
	if a.layout.FocusIndex() != focusBefore {
		t.Fatalf("wheel must not change focus: %d → %d", focusBefore, a.layout.FocusIndex())
	}

	// Wheel down brings it back toward the bottom.
	model, _ = a.update(tea.MouseWheelMsg{X: rect.X + 2, Y: rect.Y + 1, Button: tea.MouseWheelDown})
	a = drainModel(t, model)
	tp, _ = a.layout.TerminalPaneByID("exec:wheel")
	if tp.ScrollOffset() != 0 {
		t.Fatalf("wheel-down should return to bottom: offset = %d", tp.ScrollOffset())
	}
}

// TestSyncPaneFootersTerminalBadgeVisibility asserts a terminal pane's context
// badge follows the same single-/multi-context rule as resource panes: hidden
// when all panes share one context, shown when contexts differ. This guards the
// PaneAtIdx iteration in syncPaneFooters (SplitAt returns nil for terminal panes
// and previously left the terminal badge always visible).
func TestSyncPaneFootersTerminalBadgeVisibility(t *testing.T) {
	a := newTestApp()
	model, _ := a.update(tea.WindowSizeMsg{Width: 120, Height: 40})
	a = drainModel(t, model)

	// Seed a resource sibling on ctx-1 and a terminal pane on ctx-1 (all shared).
	fe := newFakeExecutor()
	sess := openTestTerminal(t, &a, "exec:badge", fe) // seeds a resource sibling on ctx-1
	defer sess.Close()

	a.syncPaneFooters()

	tp, found := a.layout.TerminalPaneByID("exec:badge")
	if !found {
		t.Fatal("terminal pane not found")
	}
	// Single shared context → badge hidden.
	if tp.ContextBadgeVisible() {
		t.Fatalf("single-context: terminal badge should be hidden, got visible (View:\n%s)", tp.View())
	}
	if strings.Contains(tp.View(), "ctx-1") {
		t.Fatalf("single-context: terminal View should not render the context badge:\n%s", tp.View())
	}

	// Introduce a second context: retarget the resource sibling to ctx-2 so panes
	// now span two contexts. The terminal badge must become visible.
	for i := 0; i < a.layout.SplitCount(); i++ {
		if rl, ok := a.layout.PaneAtIdx(i).(*ui.ResourceList); ok {
			rl.SetContext("ctx-2")
			break
		}
	}
	a.syncPaneFooters()

	tp, _ = a.layout.TerminalPaneByID("exec:badge")
	if !tp.ContextBadgeVisible() {
		t.Fatalf("multi-context: terminal badge should be visible, got hidden")
	}
	if !strings.Contains(tp.View(), "ctx-1") {
		t.Fatalf("multi-context: terminal View should render its context badge:\n%s", tp.View())
	}
}

// terminalSplitIndex returns the split index of the terminal pane with the
// given id.
func terminalSplitIndex(t *testing.T, a *App, id string) int {
	t.Helper()
	for i := 0; i < a.layout.SplitCount(); i++ {
		if tp, ok := a.layout.PaneAtIdx(i).(*ui.TerminalPane); ok && tp.ID() == id {
			return i
		}
	}
	t.Fatalf("terminal pane %q not found among splits", id)
	return -1
}

// paneRectForSplit scans the pane rects from a rendered layout to find the rect
// covering the given split index. View() must have been called (it has, via the
// preceding window-size update) so PaneAt has populated rects.
func paneRectForSplit(t *testing.T, a *App, idx int) layout.PaneRect {
	t.Helper()
	// Probe the layout's rect cache by hitting the top-left of each pane. Easiest
	// is to render and then sweep PaneAt across the screen; instead we render the
	// view to populate rects and read the rect through a probe sweep.
	_ = a.View()
	for y := 0; y < 40; y++ {
		for x := 0; x < 100; x++ {
			if r, ok := a.layout.PaneAt(x, y); ok && r.Kind == layout.PaneSplit && r.SplitIdx == idx {
				return r
			}
		}
	}
	t.Fatalf("no pane rect found for split idx %d", idx)
	return layout.PaneRect{}
}

// printableKey mirrors the ui-package printable helper for app tests.
func printableKey(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}

// ctrlKey builds a Ctrl+<rune> key press (e.g. ctrlKey('a') == ctrl+a).
func ctrlKey(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Mod: tea.ModCtrl}
}

// TestLiveTerminalPrefixScrollAdjustsOffset asserts prefix+pgup scrolls the live
// terminal pane's scrollback via routeTerminalKey (PaneCmdScrollUp), and
// prefix+pgdown scrolls back down.
func TestLiveTerminalPrefixScrollAdjustsOffset(t *testing.T) {
	a := newTestApp()
	model, _ := a.update(tea.WindowSizeMsg{Width: 100, Height: 40})
	a = drainModel(t, model)

	fe := newFakeExecutor()
	sess := openTestTerminal(t, &a, "exec:scroll", fe)
	defer sess.Close()

	tp, _ := a.layout.TerminalPaneByID("exec:scroll")
	tp.SetScrollback(1000)
	for i := 0; i < 80; i++ {
		_, _ = tp.Write([]byte("row\r\n"))
	}

	// Prefix flips to nav mode.
	model, _ = a.update(tea.KeyPressMsg{Code: 'a', Mod: tea.ModCtrl})
	a = drainModel(t, model)
	// pgup → PaneCmdScrollUp → ScrollUp(one page).
	model, _ = a.update(tea.KeyPressMsg{Code: tea.KeyPgUp})
	a = drainModel(t, model)

	tp, _ = a.layout.TerminalPaneByID("exec:scroll")
	if tp.ScrollOffset() == 0 {
		t.Fatalf("prefix+pgup should have scrolled up, offset still 0")
	}
	upOffset := tp.ScrollOffset()

	// Prefix + pgdown scrolls back down.
	model, _ = a.update(tea.KeyPressMsg{Code: 'a', Mod: tea.ModCtrl})
	a = drainModel(t, model)
	model, _ = a.update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	a = drainModel(t, model)

	tp, _ = a.layout.TerminalPaneByID("exec:scroll")
	if tp.ScrollOffset() >= upOffset {
		t.Fatalf("prefix+pgdown should reduce offset: %d → %d", upOffset, tp.ScrollOffset())
	}
}

// TestExitedTerminalPageUpScrolls asserts that page-up (resolved through the
// trie in the "terminal" context) scrolls an EXITED terminal pane's scrollback.
func TestExitedTerminalPageUpScrolls(t *testing.T) {
	a := newTestApp()
	model, _ := a.update(tea.WindowSizeMsg{Width: 100, Height: 40})
	a = drainModel(t, model)

	fe := newFakeExecutor()
	fe.returnImmediately = true
	sess := openTestTerminal(t, &a, "exec:exitedscroll", fe)
	defer sess.Close()

	tp, _ := a.layout.TerminalPaneByID("exec:exitedscroll")
	tp.SetScrollback(1000)
	for i := 0; i < 80; i++ {
		_, _ = tp.Write([]byte("row\r\n"))
	}
	tp.MarkExited(0)

	// ctrl+b is the global page-up binding; on an exited terminal it falls
	// through to the trie and scrolls the pane's scrollback.
	model, _ = a.update(tea.KeyPressMsg{Code: 'b', Mod: tea.ModCtrl})
	a = drainModel(t, model)

	tp, _ = a.layout.TerminalPaneByID("exec:exitedscroll")
	if tp.ScrollOffset() == 0 {
		t.Fatalf("page-up on exited terminal should scroll scrollback, offset still 0")
	}
}

// TestTerminalCursorInView asserts App.View() positions the real terminal cursor
// at the focused live terminal pane's emulator cursor (pane rect top-left + 1
// border cell + inner cursor coords), and leaves it nil when an overlay is open,
// when a non-terminal pane is focused, or when the pane has exited.
func TestTerminalCursorInView(t *testing.T) {
	a := newTestApp()
	model, _ := a.update(tea.WindowSizeMsg{Width: 100, Height: 40})
	a = drainModel(t, model)

	fe := newFakeExecutor()
	sess := openTestTerminal(t, &a, "exec:cursor", fe) // terminal becomes focused
	defer sess.Close()

	tp, found := a.layout.TerminalPaneByID("exec:cursor")
	if !found {
		t.Fatal("terminal pane not found")
	}
	if _, err := tp.Write([]byte("abc")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// Focused live terminal → cursor at rect.X+1+3, rect.Y+1+0.
	idx := terminalSplitIndex(t, &a, "exec:cursor")
	rect := paneRectForSplit(t, &a, idx)
	cur := a.View().Cursor
	if cur == nil {
		t.Fatal("expected a non-nil cursor for a focused live terminal pane")
	}
	wantX, wantY := rect.X+1+3, rect.Y+1+0
	if cur.Position.X != wantX || cur.Position.Y != wantY {
		t.Fatalf("cursor at (%d,%d), want (%d,%d)", cur.Position.X, cur.Position.Y, wantX, wantY)
	}

	// Overlay open → no cursor.
	a.activeOverlay = overlaySearchBar
	if a.View().Cursor != nil {
		t.Fatal("cursor should be nil while an overlay is open")
	}
	a.activeOverlay = overlayNone

	// Non-terminal pane focused → no cursor.
	a.layout.FocusPrev() // move focus to the seeded resource sibling
	if _, ok := a.layout.FocusedPane().(*ui.TerminalPane); ok {
		t.Fatal("expected focus to move off the terminal pane")
	}
	if a.View().Cursor != nil {
		t.Fatal("cursor should be nil when a non-terminal pane is focused")
	}

	// Refocus the terminal, then exit it → no cursor.
	idx = terminalSplitIndex(t, &a, "exec:cursor")
	a.layout.FocusSplitAt(idx)
	tp, _ = a.layout.TerminalPaneByID("exec:cursor")
	tp.MarkExited(0)
	if a.View().Cursor != nil {
		t.Fatal("cursor should be nil for an exited terminal pane")
	}
}

// TestTerminalCursorBorderlessZoom asserts that when the focused terminal pane is
// zoomed (borderless, fullscreen), the cursor is positioned with the borderless
// content offset: dx=0 (no left border) and dy=1 (one header line). The rect is
// the full fullscreen split rect.
func TestTerminalCursorBorderlessZoom(t *testing.T) {
	a := newTestApp()
	model, _ := a.update(tea.WindowSizeMsg{Width: 100, Height: 40})
	a = drainModel(t, model)

	fe := newFakeExecutor()
	sess := openTestTerminal(t, &a, "exec:zoomcursor", fe) // terminal becomes focused
	defer sess.Close()

	tp, found := a.layout.TerminalPaneByID("exec:zoomcursor")
	if !found {
		t.Fatal("terminal pane not found")
	}
	if _, err := tp.Write([]byte("abc")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// Zoom the focused split fullscreen-borderless and push the new sizes.
	a = a.toggleZoomSplitAndSync()
	if !a.layout.SplitZoomed() {
		t.Fatal("precondition: split should be zoomed")
	}

	idx := terminalSplitIndex(t, &a, "exec:zoomcursor")
	rect := paneRectForSplit(t, &a, idx)
	cur := a.View().Cursor
	if cur == nil {
		t.Fatal("expected a non-nil cursor for a focused zoomed terminal pane")
	}
	// Borderless offset: dx=0, dy=1; cursor sits at inner (3,0).
	wantX, wantY := rect.X+0+3, rect.Y+1+0
	if cur.Position.X != wantX || cur.Position.Y != wantY {
		t.Fatalf("zoomed cursor at (%d,%d), want (%d,%d)", cur.Position.X, cur.Position.Y, wantX, wantY)
	}

	// Bottom-row check: under zoom the pane is sized one row taller than the
	// hit-test rect (the status bar is hidden), so the cursor must be able to
	// reach the LAST emulator row. Move the emulator cursor there by writing
	// enough newlines to scroll/advance to the final inner line, then assert the
	// cursor lands on the absolute bottom row (rect.Y + dy + (ih-1)).
	iw, ih := tp.InnerSize()
	// Carriage-return to column 0, then advance to the last inner row.
	nl := make([]byte, 0, ih+1)
	nl = append(nl, '\r')
	for range ih {
		nl = append(nl, '\n')
	}
	if _, err := tp.Write(nl); err != nil {
		t.Fatalf("Write newlines: %v", err)
	}
	cx, cy, visible := tp.CursorPos()
	if !visible {
		t.Fatal("cursor should be visible after writing newlines")
	}
	if cy != ih-1 {
		t.Fatalf("emulator cursor row = %d, want bottom row %d (inner height %d)", cy, ih-1, ih)
	}
	cur = a.View().Cursor
	if cur == nil {
		t.Fatal("expected a non-nil cursor at the bottom row")
	}
	wantBottomY := rect.Y + 1 + (ih - 1)
	if cur.Position.Y != wantBottomY {
		t.Fatalf("zoomed bottom-row cursor Y = %d, want %d (must not be clamped one row high)", cur.Position.Y, wantBottomY)
	}
	// Sanity: the bottom row is the last on-screen row (no status bar under zoom).
	if wantBottomY != a.height-1 {
		t.Fatalf("bottom emulator row Y=%d should equal screen bottom %d under zoom", wantBottomY, a.height-1)
	}
	_ = iw
	_ = cx
}

// TestViewDropsStatusBarUnderZoom asserts that View() appends the status bar when
// nothing is zoomed, and drops it for ANY zoom (ZoomSplit included, which is now
// fullscreen-borderless).
//
// The status bar is detected by rendering the live ui.StatusBar component
// directly and taking its stripped content as the marker. This keeps the test
// in lockstep with the real component (no hand-copied magic token to drift),
// and is robust because the marker is exactly what App composites into the view.
func TestViewDropsStatusBarUnderZoom(t *testing.T) {
	a := newTestApp()
	model, _ := a.update(tea.WindowSizeMsg{Width: 100, Height: 40})
	a = drainModel(t, model)

	fe := newFakeExecutor()
	sess := openTestTerminal(t, &a, "exec:zoomview", fe)
	defer sess.Close()

	// Derive the marker from the actual status-bar render rather than a literal.
	statusBarMarker := strings.TrimSpace(ansi.Strip(a.statusBar.View()))
	if statusBarMarker == "" {
		t.Fatal("precondition: status bar should render non-empty content")
	}

	if a.layout.SplitZoomed() {
		t.Fatal("precondition: split should not start zoomed")
	}
	unzoomed := ansi.Strip(a.View().Content)
	if !strings.Contains(unzoomed, statusBarMarker) {
		t.Fatalf("non-zoomed view should render the status bar (marker %q missing)", statusBarMarker)
	}

	a = a.toggleZoomSplitAndSync()
	if a.layout.EffectiveZoom() == layout.ZoomNone {
		t.Fatal("precondition: split should be zoomed")
	}
	zoomed := ansi.Strip(a.View().Content)
	if strings.Contains(zoomed, statusBarMarker) {
		t.Fatalf("zoomed view should drop the status bar (marker %q still present)", statusBarMarker)
	}
}
