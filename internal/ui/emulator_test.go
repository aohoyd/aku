package ui

import (
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

// strippedLines renders the emulator, strips ANSI styling, and returns the
// screen rows with trailing whitespace trimmed per line. Cell-grid renders pad
// rows to the grid width, so per-line trimming keeps assertions tolerant.
func strippedLines(t *testing.T, e Emulator) []string {
	t.Helper()
	plain := ansi.Strip(e.Render())
	rows := strings.Split(plain, "\n")
	for i := range rows {
		rows[i] = strings.TrimRight(rows[i], " ")
	}
	return rows
}

// TestEmulatorReplyDrainAndClose verifies the query-reply back-channel: a DSR
// cursor-position query produces a reply on the emulator's reply stream, and
// CloseReplies makes a blocked Read return io.EOF. The reply pipe is unbuffered,
// so Write blocks until the reply is drained — hence Write runs concurrently.
func TestEmulatorReplyDrainAndClose(t *testing.T) {
	e := NewEmulator(20, 5)

	writeDone := make(chan struct{})
	go func() {
		_, _ = e.Write([]byte("\x1b[6n")) // DSR: report cursor position (CPR)
		close(writeDone)
	}()

	type readRes struct {
		s   string
		err error
	}
	got := make(chan readRes, 1)
	go func() {
		buf := make([]byte, 64)
		n, err := e.Read(buf)
		got <- readRes{string(buf[:n]), err}
	}()

	select {
	case r := <-got:
		if r.err != nil {
			t.Fatalf("Read: %v", r.err)
		}
		if !strings.Contains(r.s, "R") { // CPR ends with 'R', e.g. "\x1b[1;1R"
			t.Fatalf("expected a cursor-position reply containing 'R', got %q", r.s)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out draining the emulator reply (Write likely blocked)")
	}
	<-writeDone // Write unblocked once the reply was drained.

	if err := e.CloseReplies(); err != nil {
		t.Fatalf("CloseReplies: %v", err)
	}
	eof := make(chan error, 1)
	go func() {
		buf := make([]byte, 8)
		_, err := e.Read(buf)
		eof <- err
	}()
	select {
	case err := <-eof:
		if err != io.EOF {
			t.Fatalf("Read after CloseReplies = %v, want io.EOF", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Read did not return after CloseReplies")
	}
}

func TestEmulatorPlainText(t *testing.T) {
	e := NewEmulator(20, 5)
	if _, err := e.Write([]byte("hello world")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	rows := strippedLines(t, e)
	if len(rows) == 0 || rows[0] != "hello world" {
		t.Fatalf("expected first row %q, got rows %#v", "hello world", rows)
	}
}

func TestEmulatorSGRColorPreserved(t *testing.T) {
	e := NewEmulator(20, 3)
	// Red "RED" then reset.
	if _, err := e.Write([]byte("\x1b[31mRED\x1b[0m")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	render := e.Render()
	plain := ansi.Strip(render)

	if !strings.Contains(plain, "RED") {
		t.Fatalf("stripped render should contain RED, got %q", plain)
	}
	// Styling must be present: the styled render differs from the stripped one
	// and contains an ANSI escape (ESC).
	if render == plain {
		t.Fatalf("styled render should differ from stripped render; both %q", render)
	}
	if !strings.Contains(render, "\x1b") {
		t.Fatalf("styled render should contain an ANSI escape, got %q", render)
	}
}

func TestEmulatorCursorMoveOverwrite(t *testing.T) {
	e := NewEmulator(20, 3)
	// Write "ABCDE", move cursor to column 1 (CSI 1 G is column 1), overwrite
	// "X". Final first row should read "XBCDE".
	if _, err := e.Write([]byte("ABCDE\x1b[1GX")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	rows := strippedLines(t, e)
	if len(rows) == 0 || rows[0] != "XBCDE" {
		t.Fatalf("expected first row %q, got rows %#v", "XBCDE", rows)
	}
}

func TestEmulatorWrapAtWidth(t *testing.T) {
	const w = 5
	e := NewEmulator(w, 4)
	// 8 chars into a 5-wide grid should wrap to a second row.
	if _, err := e.Write([]byte("ABCDEFGH")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	rows := strippedLines(t, e)
	if len(rows) < 2 {
		t.Fatalf("expected at least 2 rows after wrap, got %#v", rows)
	}
	if rows[0] != "ABCDE" {
		t.Fatalf("expected first row %q, got %q (all %#v)", "ABCDE", rows[0], rows)
	}
	if rows[1] != "FGH" {
		t.Fatalf("expected second row %q, got %q (all %#v)", "FGH", rows[1], rows)
	}
}

func TestEmulatorResizeReflows(t *testing.T) {
	e := NewEmulator(20, 5)
	if _, err := e.Write([]byte("ABCDEFGH")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	// At width 20 this all fits on one row.
	rows := strippedLines(t, e)
	if rows[0] != "ABCDEFGH" {
		t.Fatalf("pre-resize expected %q, got %q", "ABCDEFGH", rows[0])
	}

	// Shrink width; the new dimension must take effect, so no row may exceed the
	// new width.
	e.Resize(4, 5)
	rows = strippedLines(t, e)
	for i, r := range rows {
		if len(r) > 4 {
			t.Fatalf("row %d %q exceeds resized width 4 (all %#v)", i, r, rows)
		}
	}

	// Writing fresh content after resize must wrap at the new width.
	e2 := NewEmulator(20, 5)
	e2.Resize(4, 5)
	if _, err := e2.Write([]byte("ABCDEF")); err != nil {
		t.Fatalf("Write after resize: %v", err)
	}
	rows = strippedLines(t, e2)
	if len(rows) < 2 || rows[0] != "ABCD" || rows[1] != "EF" {
		t.Fatalf("post-resize wrap expected rows [ABCD EF], got %#v", rows)
	}
}

// stripViewport renders a viewport at the given offset, strips ANSI, and trims
// trailing whitespace per row.
func stripViewport(t *testing.T, e Emulator, offset int) []string {
	t.Helper()
	plain := ansi.Strip(e.RenderViewport(offset))
	rows := strings.Split(plain, "\n")
	for i := range rows {
		rows[i] = strings.TrimRight(rows[i], " ")
	}
	return rows
}

func TestEmulatorScrollbackAccumulates(t *testing.T) {
	e := NewEmulator(10, 3)
	e.SetScrollbackSize(100)
	// Push 8 lines through a 3-row screen: 6 scroll off into scrollback.
	for i := 0; i < 8; i++ {
		if _, err := e.Write([]byte("line" + string(rune('0'+i)) + "\r\n")); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}
	if got := e.ScrollbackLen(); got != 6 {
		t.Fatalf("expected 6 scrollback lines, got %d", got)
	}
}

func TestEmulatorRenderViewportOffsetZeroIsLiveScreen(t *testing.T) {
	e := NewEmulator(10, 3)
	e.SetScrollbackSize(100)
	for i := 0; i < 8; i++ {
		if _, err := e.Write([]byte("line" + string(rune('0'+i)) + "\r\n")); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}
	live := strippedLines(t, e)
	vp := stripViewport(t, e, 0)
	if strings.Join(live, "\n") != strings.Join(vp, "\n") {
		t.Fatalf("offset 0 viewport %#v != live screen %#v", vp, live)
	}
}

// TestEmulatorRenderViewportNoScrollbackClampsToLive asserts that requesting a
// positive offset on an emulator with no accumulated scrollback clamps to the
// live screen (identical to Render()).
func TestEmulatorRenderViewportNoScrollbackClampsToLive(t *testing.T) {
	e := NewEmulator(10, 3)
	e.SetScrollbackSize(100)
	// A single short line: nothing scrolls off, so the scrollback is empty.
	if _, err := e.Write([]byte("hello")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if sb := e.ScrollbackLen(); sb != 0 {
		t.Fatalf("precondition: expected empty scrollback, got %d", sb)
	}
	if got, want := e.RenderViewport(5), e.Render(); got != want {
		t.Fatalf("RenderViewport(5) with no scrollback = %q, want Render() %q", got, want)
	}
}

func TestEmulatorRenderViewportScrollsBack(t *testing.T) {
	e := NewEmulator(10, 3)
	e.SetScrollbackSize(100)
	for i := 0; i < 8; i++ {
		if _, err := e.Write([]byte("line" + string(rune('0'+i)) + "\r\n")); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}
	// Bottom (offset 0) shows the live screen: after writing line0..line7 with
	// trailing CRLFs, the cursor sits on a fresh blank row, so the screen rows
	// are line6, line7, "".
	bottom := stripViewport(t, e, 0)
	if len(bottom) < 2 || bottom[0] != "line6" {
		t.Fatalf("offset 0 expected top row line6, got %#v", bottom)
	}
	// Scroll up by 3 lines: the visible window shifts up by 3, revealing older
	// lines. Top row should now be line3.
	up := stripViewport(t, e, 3)
	if len(up) == 0 || up[0] != "line3" {
		t.Fatalf("offset 3 expected top row line3, got %#v", up)
	}
}

func TestEmulatorRenderViewportClampsToOldest(t *testing.T) {
	e := NewEmulator(10, 3)
	e.SetScrollbackSize(100)
	for i := 0; i < 8; i++ {
		if _, err := e.Write([]byte("line" + string(rune('0'+i)) + "\r\n")); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}
	// An over-large offset is clamped: the top row never goes past the oldest
	// retained line (line0).
	vp := stripViewport(t, e, 9999)
	if len(vp) == 0 || vp[0] != "line0" {
		t.Fatalf("clamped viewport expected top row line0, got %#v", vp)
	}
}

// TestSplitScreenLinesHandlesCRLF asserts the viewport line-splitter is robust to
// a renderer that separates rows with "\r\n": each row must come back without a
// trailing "\r" so the composed scrollback window is not corrupted. (x/vt's
// current Render() uses bare "\n", but this guards a future/alternate backend.)
func TestSplitScreenLinesHandlesCRLF(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"a\nb\nc", []string{"a", "b", "c"}},
		{"a\r\nb\r\nc", []string{"a", "b", "c"}},
		{"a\r\nb\nc", []string{"a", "b", "c"}}, // mixed separators
		{"", []string{""}},
	}
	for _, tc := range cases {
		got := splitScreenLines(tc.in)
		if len(got) != len(tc.want) {
			t.Errorf("splitScreenLines(%q) = %#v, want %#v", tc.in, got, tc.want)
			continue
		}
		for i := range tc.want {
			if got[i] != tc.want[i] {
				t.Errorf("splitScreenLines(%q)[%d] = %q, want %q", tc.in, i, got[i], tc.want[i])
			}
			if strings.HasSuffix(got[i], "\r") {
				t.Errorf("splitScreenLines(%q)[%d] still has trailing CR: %q", tc.in, i, got[i])
			}
		}
	}
}

// TestEmulatorRenderViewportNoCarriageReturns asserts a scrolled-back viewport
// never contains a stray "\r" that would corrupt the composed frame.
func TestEmulatorRenderViewportNoCarriageReturns(t *testing.T) {
	e := NewEmulator(10, 3)
	e.SetScrollbackSize(100)
	for i := 0; i < 8; i++ {
		if _, err := e.Write([]byte("line" + string(rune('0'+i)) + "\r\n")); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}
	if vp := e.RenderViewport(3); strings.Contains(vp, "\r") {
		t.Fatalf("scrolled viewport contains a carriage return: %q", vp)
	}
}

// TestEmulatorConcurrentWriteRenderRead exercises the documented concurrency
// contract: Write/Render/RenderViewport run on the UI goroutine while Read
// drains the reply pipe on a background goroutine. The DSR cursor-position query
// ("\x1b[6n") makes the emulator emit a reply on its pipe, so the Read goroutine
// has data to drain concurrently. This must be clean under -race.
func TestEmulatorConcurrentWriteRenderRead(t *testing.T) {
	e := NewEmulator(40, 10)

	var wg sync.WaitGroup
	wg.Add(2)

	// Reader goroutine: drain the reply pipe until it is closed.
	go func() {
		defer wg.Done()
		buf := make([]byte, 256)
		for {
			if _, err := e.Read(buf); err != nil {
				return
			}
		}
	}()

	// Writer/renderer goroutine: interleave Write (with a query that produces a
	// reply), Render and RenderViewport.
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			// "\x1b[6n" is a DSR cursor-position request → the emulator writes a
			// reply onto its pipe, which the reader drains concurrently.
			_, _ = e.Write([]byte("hello\x1b[6n\r\n"))
			_ = e.Render()
			_ = e.RenderViewport(1)
		}
		// Closing the replies unblocks the reader so the test can finish.
		_ = e.CloseReplies()
	}()

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("concurrent Write/Render/Read did not finish (possible deadlock)")
	}
}

func TestEmulatorCursorPosition(t *testing.T) {
	e := NewEmulator(20, 5)
	if _, err := e.Write([]byte("abc")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	x, y := e.CursorPosition()
	if x != 3 || y != 0 {
		t.Fatalf("expected cursor at (3,0) after writing 3 chars, got (%d,%d)", x, y)
	}
}
