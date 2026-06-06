package ui

import (
	"io"
	"strings"

	"github.com/charmbracelet/x/vt"
)

// Emulator is the minimal terminal-emulator surface a terminal pane needs. It
// turns raw shell output (including ANSI control sequences) into a renderable,
// styled screen grid. It is kept deliberately small (YAGNI): only what a pane
// requires to feed bytes, resize, render, and place a focus cursor.
//
// Keeping it behind an interface lets the terminal pane be unit-tested against
// a fake and makes the concrete VT backend swappable.
//
// The production implementation (vtEmulator) is a thin adapter over
// github.com/charmbracelet/x/vt. Most methods forward directly; RenderViewport
// is the exception — x/vt has no offset-aware render, so the adapter composes
// the scrollback viewport by hand (see vtEmulator.RenderViewport).
type Emulator interface {
	// Write feeds shell output bytes (ANSI included) into the emulator.
	Write(p []byte) (int, error)
	// Resize changes the grid size to w columns by h rows.
	Resize(w, h int)
	// Render produces a renderable snapshot of the current screen with styles
	// encoded as ANSI escape codes, so the existing lipgloss/ansi rendering can
	// display colors and attributes.
	Render() string
	// CursorPosition returns the cursor's column (x) and row (y), 0-based. It is
	// useful for drawing a focus indicator at the cursor.
	CursorPosition() (x, y int)
	// CursorVisible reports whether the cursor should be drawn, tracking the
	// program's DECTCEM (ESC[?25h/l) state. A full-screen TUI that hides its
	// cursor (vim, pagers) reports false so callers don't draw a phantom cursor.
	CursorVisible() bool
	// CursorShape returns the cursor shape requested via DECSCUSR (ESC[ q),
	// defaulting to CursorShapeBlock.
	CursorShape() CursorShape
	// Read drains the terminal's reply stream — responses the emulator generates
	// to program queries (Device Attributes, Device Status / cursor-position
	// reports, DECRQM mode reports, etc.). The host MUST forward these back to the
	// program's stdin, or the emulator's internal reply pipe fills and the next
	// Write blocks. Read blocks until reply bytes are available or the reply
	// stream is closed (then it returns io.EOF).
	Read(p []byte) (int, error)
	// CloseReplies closes the reply stream so a blocked Read returns io.EOF. It is
	// used to tear down the reply-drain goroutine without touching x/vt's
	// unsynchronized closed flag (which would race with the draining Read).
	CloseReplies() error
	// SetScrollbackSize sets the maximum number of off-screen lines retained for
	// scrollback.
	SetScrollbackSize(maxLines int)
	// ScrollbackLen reports the number of lines currently held in the scrollback
	// buffer (i.e. lines that have scrolled off the top of the live screen).
	ScrollbackLen() int
	// RenderViewport renders a height-tall snapshot of the screen scrolled back
	// by offset lines. offset==0 renders the live screen (identical to Render);
	// a positive offset walks up into the scrollback buffer, clamped so the
	// viewport never scrolls past the oldest retained line. Styles are encoded as
	// ANSI escape codes, like Render.
	RenderViewport(offset int) string
}

// CursorShape is the renderable cursor shape, decoupled from both the x/vt and
// bubbletea cursor-shape enums so the emulator interface stays self-contained.
type CursorShape int

const (
	CursorShapeBlock CursorShape = iota
	CursorShapeUnderline
	CursorShapeBar
)

// vtEmulator adapts github.com/charmbracelet/x/vt to the Emulator interface.
//
// The underlying *vt.Emulator already exposes Write([]byte) (int, error),
// Resize(w, h int), Render() string (styles encoded as ANSI), and
// CursorPosition() returning a uv.Position (an image.Point). This adapter is
// therefore a thin shim that mostly narrows the surface and flattens the
// position to a plain (x, y) pair.
//
// x/vt exposes cursor visibility and shape only through its Callbacks hook (the
// *Emulator has no Cursor()/Screen() accessor), so we register callbacks at
// construction to mirror DECTCEM (visibility) and DECSCUSR (shape) into local
// fields. The callbacks fire synchronously inside term.Write, on the same
// goroutine that feeds bytes and renders, so no locking is needed.
type vtEmulator struct {
	term         *vt.Emulator
	cursorHidden bool
	cursorShape  CursorShape
}

// NewEmulator constructs an Emulator backed by x/vt with an initial grid of w
// columns and h rows. Width and height are clamped to a minimum of 1 so a
// degenerate (zero) initial size cannot panic the backend.
func NewEmulator(w, h int) Emulator {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	e := &vtEmulator{term: vt.NewEmulator(w, h)}
	e.term.SetCallbacks(vt.Callbacks{
		CursorVisibility: func(visible bool) { e.cursorHidden = !visible },
		CursorStyle: func(style vt.CursorStyle, _ bool) {
			e.cursorShape = mapVTCursorStyle(style)
		},
	})
	return e
}

// mapVTCursorStyle translates x/vt's cursor style enum to the local shape enum.
func mapVTCursorStyle(s vt.CursorStyle) CursorShape {
	switch s {
	case vt.CursorUnderline:
		return CursorShapeUnderline
	case vt.CursorBar:
		return CursorShapeBar
	default:
		return CursorShapeBlock
	}
}

func (e *vtEmulator) Write(p []byte) (int, error) {
	return e.term.Write(p)
}

func (e *vtEmulator) Resize(w, h int) {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	e.term.Resize(w, h)
}

// Render returns the screen with styles encoded as ANSI escape codes. x/vt's
// Render() already preserves SGR colors/attributes, so no manual cell-grid
// walking is required to keep color.
func (e *vtEmulator) Render() string {
	return e.term.Render()
}

func (e *vtEmulator) CursorPosition() (x, y int) {
	pos := e.term.CursorPosition()
	return pos.X, pos.Y
}

// CursorVisible reports the tracked DECTCEM state. The cursor starts visible
// (vt's default), and the CursorVisibility callback flips cursorHidden on
// ESC[?25h/l.
func (e *vtEmulator) CursorVisible() bool { return !e.cursorHidden }

// CursorShape returns the tracked DECSCUSR shape (block by default).
func (e *vtEmulator) CursorShape() CursorShape { return e.cursorShape }

// Read forwards to x/vt's reply reader (the read end of its internal reply
// pipe). It touches only the pipe, so it is safe to call from a drain goroutine
// concurrently with Write/Render on the UI goroutine.
func (e *vtEmulator) Read(p []byte) (int, error) { return e.term.Read(p) }

// CloseReplies closes the reply pipe's write end (obtained via InputPipe, whose
// dynamic type is *io.PipeWriter) so a blocked Read returns io.EOF. This avoids
// vt.Emulator.Close(), which mutates an unsynchronized closed bool and would race
// with the draining Read.
func (e *vtEmulator) CloseReplies() error {
	if c, ok := e.term.InputPipe().(io.Closer); ok {
		return c.Close()
	}
	return nil
}

func (e *vtEmulator) SetScrollbackSize(maxLines int) {
	if maxLines < 0 {
		maxLines = 0
	}
	e.term.SetScrollbackSize(maxLines)
}

func (e *vtEmulator) ScrollbackLen() int {
	return e.term.ScrollbackLen()
}

// RenderViewport composes a viewport of the logical buffer
// [scrollback... , live-screen...] scrolled up by offset lines.
//
// x/vt's Render() only paints the live screen; there is no built-in
// offset-aware render. The scrollback buffer exposes its off-screen lines via
// Scrollback().Line(i) (oldest→newest), each of which renders to an ANSI
// string. We therefore build the visible window by hand: take the live screen
// rows (Render()), prepend the requested number of scrollback lines, and slice
// out a height-tall window ending offset lines above the bottom.
//
// offset is clamped to [0, scrollbackLen] so the viewport can never scroll past
// the oldest retained line. offset==0 returns the live screen verbatim.
func (e *vtEmulator) RenderViewport(offset int) string {
	if offset <= 0 {
		return e.term.Render()
	}
	sbLen := e.term.ScrollbackLen()
	if offset > sbLen {
		offset = sbLen
	}
	if offset == 0 {
		return e.term.Render()
	}

	height := e.term.Height()
	if height < 1 {
		height = 1
	}

	// x/vt's current Render() separates rows with bare "\n", but be defensive:
	// some VT renderers emit "\r\n", which would leave a trailing "\r" on every
	// row and corrupt the composed viewport (the carriage return resets the
	// cursor column mid-line when the window is reassembled). Strip a trailing
	// "\r" per row so the composition is robust regardless of the separator.
	// The offset<=0 fast path above returns Render() verbatim and is unaffected.
	screen := splitScreenLines(e.term.Render())

	// Logical buffer = scrollback (oldest→newest) followed by the live screen.
	// The bottom of the window sits offset lines above the very bottom, so the
	// window covers logical indices [start, start+height).
	total := sbLen + len(screen)
	end := total - offset // exclusive upper bound of the visible window
	start := end - height
	if start < 0 {
		start = 0
	}

	sb := e.term.Scrollback()
	lines := make([]string, 0, height)
	for i := start; i < end && i < total; i++ {
		if i < sbLen {
			lines = append(lines, strings.TrimRight(sb.Line(i).Render(), "\r"))
		} else {
			lines = append(lines, screen[i-sbLen])
		}
	}
	return strings.Join(lines, "\n")
}

// splitScreenLines splits a rendered screen into per-row strings, tolerating
// either "\n" or "\r\n" separators by trimming a trailing "\r" from each row.
// A bare "\r" mid-row is rare from a VT renderer and intentionally left intact.
func splitScreenLines(s string) []string {
	rows := strings.Split(s, "\n")
	for i := range rows {
		rows[i] = strings.TrimRight(rows[i], "\r")
	}
	return rows
}
