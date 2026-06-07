package ui

import (
	"fmt"
	"io"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/aohoyd/aku/internal/theme"
	"github.com/charmbracelet/x/ansi"
)

// inputMode is the TerminalPane's two-state input machine. In modeTyping every
// key (except the configured prefix) is encoded and forwarded to the shell. The
// prefix key flips the pane into modeNav, where the NEXT key is interpreted as a
// tmux-style pane command and the pane returns to modeTyping.
type inputMode int

const (
	modeTyping inputMode = iota // keys go to the shell
	modeNav                     // next key is a nav command (after prefix)
)

// PaneCommand is a nav action the App executes on behalf of a focused terminal
// pane. The pane itself never moves focus, zooms, or closes — it only reports
// the user's intent so the App (which owns the layout) can act. PaneCmdNone
// means "no layout action" (e.g. a key was consumed but does nothing).
type PaneCommand int

const (
	PaneCmdNone       PaneCommand = iota // no action
	PaneCmdFocusLeft                     // move focus to the pane on the left
	PaneCmdFocusRight                    // move focus to the pane on the right
	PaneCmdFocusUp                       // move focus to the pane above
	PaneCmdFocusDown                     // move focus to the pane below
	PaneCmdZoom                          // toggle zoom (maximize) of this pane
	PaneCmdClose                         // close this pane
	PaneCmdScrollUp                      // scroll the pane's scrollback up
	PaneCmdScrollDown                    // scroll the pane's scrollback down
)

// TerminalPane is a ui.Pane that renders a terminal emulator inside the shared
// bordered/titled box and implements a tmux-style prefix input machine. It is
// self-contained: it holds an Emulator, forwards encoded keystrokes via
// HandleKey's toShell return, and reports nav intents via PaneCommand. It does
// not own the shell stream — the App feeds output bytes in through Write and
// drains toShell into the session.
type TerminalPane struct {
	emu      Emulator
	id       string // session id, matches session.Terminal.ID() and the App registry key
	title    string // session label, e.g. "exec: pod/ctr", "debug: ...", "node: ..."
	ctx      string // kube-context badge (Context())
	focused  bool
	mode     inputMode
	exited   bool
	exitCode int
	exitNote string // optional extra note (e.g. ephemeral cleanup notice)

	// ctxBadgeHidden suppresses the context badge even when ctx is non-empty.
	// The app owns the "panes use more than one context" decision (see
	// syncPaneFooters) and hides the badge when all panes share one context,
	// mirroring ResourceList.SetContextLabel(""). New panes default to visible.
	ctxBadgeHidden bool

	// scrollOffset is how many lines the viewport is scrolled up into the
	// scrollback buffer. 0 means the live screen (bottom); a positive value
	// shows older output. New shell output and any key/typing resets it to 0 so
	// the user is snapped back to the prompt.
	scrollOffset int

	width, height int // outer dimensions including the border box
}

// Compile-time assertion that *TerminalPane satisfies the Pane interface.
var _ Pane = (*TerminalPane)(nil)

// borderChrome is the per-axis size the rounded border box adds around content
// (1 cell left + 1 cell right, 1 line top + 1 line bottom). The emulator is
// sized to the inner area = outer - borderChrome, matching ResourceList/LogView
// which both build their content at width-2 / height-2.
const borderChrome = 2

// NewTerminalPane creates a terminal pane with the given session title and
// kube-context badge at outer size w×h. The emulator is created at the inner
// content size (outer minus the border chrome). The pane starts blurred and in
// typing mode.
func NewTerminalPane(title, ctx string, w, h int) *TerminalPane {
	iw, ih := innerSize(w, h)
	return &TerminalPane{
		emu:    NewEmulator(iw, ih),
		title:  title,
		ctx:    ctx,
		width:  w,
		height: h,
		mode:   modeTyping,
	}
}

// innerSize returns the emulator content size for an outer box of w×h, clamped
// to a minimum of 1 so a degenerate size cannot panic the backend.
func innerSize(w, h int) (int, int) {
	iw := w - borderChrome
	ih := h - borderChrome
	if iw < 1 {
		iw = 1
	}
	if ih < 1 {
		ih = 1
	}
	return iw, ih
}

// Write forwards shell output bytes (ANSI included) to the emulator. The App's
// byte-pump loop calls this for each chunk read off the session's Out channel.
// New output snaps the viewport back to the live screen (offset 0): a scrolled-
// back user sees fresh output rather than staring at a frozen historical
// window, matching how most terminals behave.
func (t *TerminalPane) Write(p []byte) (int, error) {
	t.scrollOffset = 0
	return t.emu.Write(p)
}

// SetScrollback sets the number of off-screen lines the emulator retains for
// scrollback. Called once at creation from the configured value.
func (t *TerminalPane) SetScrollback(maxLines int) {
	t.emu.SetScrollbackSize(maxLines)
}

// ScrollUp scrolls the viewport up (toward older output) by n lines, clamped to
// the available scrollback. n<=0 is a no-op.
func (t *TerminalPane) ScrollUp(n int) {
	if n <= 0 {
		return
	}
	t.scrollOffset += n
	if max := t.emu.ScrollbackLen(); t.scrollOffset > max {
		t.scrollOffset = max
	}
}

// ScrollDown scrolls the viewport down (toward the live screen) by n lines,
// clamped at 0 (the bottom). n<=0 is a no-op.
func (t *TerminalPane) ScrollDown(n int) {
	if n <= 0 {
		return
	}
	t.scrollOffset -= n
	if t.scrollOffset < 0 {
		t.scrollOffset = 0
	}
}

// ScrollOffset reports the current viewport offset into the scrollback buffer
// (0 = live screen). Exposed for tests and footer hints.
func (t *TerminalPane) ScrollOffset() int { return t.scrollOffset }

// MarkExited freezes the pane: it records the exit status and switches the pane
// into the exited state where keys no longer reach the shell.
func (t *TerminalPane) MarkExited(code int) {
	t.exited = true
	t.exitCode = code
}

// SetExitNote attaches an extra note shown alongside the exit banner (e.g. an
// ephemeral-container cleanup notice).
func (t *TerminalPane) SetExitNote(note string) {
	t.exitNote = note
}

// Exited reports whether the session has ended.
func (t *TerminalPane) Exited() bool { return t.exited }

// SetID stamps the session id on the pane so the App can match a TermBytesMsg /
// TermExitMsg (keyed by id) back to this pane, and look the pane up in the
// layout by id. Set once at creation.
func (t *TerminalPane) SetID(id string) { t.id = id }

// ID returns the session id stamped via SetID.
func (t *TerminalPane) ID() string { return t.id }

// InnerSize returns the emulator (content) dimensions for the pane's current
// outer size. The App pushes these to the session's Resize so the remote shell
// reflows to match what the emulator renders.
func (t *TerminalPane) InnerSize() (w, h int) {
	return innerSize(t.width, t.height)
}

// IsHidden reports whether the pane is currently hidden, i.e. it was assigned a
// degenerate (non-positive) outer size by the layout — which happens to every
// non-focused split under ZoomSplit/ZoomDetail. It is derived from the RAW
// stored outer dimensions, NOT from InnerSize (which clamps to a 1×1 minimum so
// the emulator can never be sized to zero). Callers that forward resizes to the
// remote shell must skip hidden panes: pushing the clamped 1×1 inner size would
// reflow a full-screen program (vim/less) running in a background pane.
func (t *TerminalPane) IsHidden() bool {
	return t.width <= 0 || t.height <= 0
}

// --- ui.Pane interface ---

// SetSize stores the outer dimensions and resizes the emulator to the inner
// content area, mirroring how ResourceList/LogView derive content size from the
// border box.
func (t *TerminalPane) SetSize(w, h int) {
	t.width = w
	t.height = h
	iw, ih := innerSize(w, h)
	t.emu.Resize(iw, ih)
}

// Focus marks the pane focused. Focusing always starts in typing mode (Blur
// resets it), so the user lands directly on the shell.
func (t *TerminalPane) Focus() { t.focused = true }

// Blur marks the pane unfocused and resets the input machine to typing mode so
// a stale nav state cannot leak into the next focus.
func (t *TerminalPane) Blur() {
	t.focused = false
	t.mode = modeTyping
}

// Title returns the session label used in the bordered title.
func (t *TerminalPane) Title() string { return t.title }

// Context returns the kube-context this pane's session belongs to.
func (t *TerminalPane) Context() string { return t.ctx }

// SetContextBadgeVisible controls whether the top-border context badge is shown.
// It mirrors ResourceList.SetContextLabel's hide mechanism: the app shows the
// badge only when panes span more than one context and hides it when all panes
// share one. View() stays dumb and renders the badge iff ctx is non-empty AND
// the badge is not hidden.
func (t *TerminalPane) SetContextBadgeVisible(visible bool) {
	t.ctxBadgeHidden = !visible
}

// ContextBadgeVisible reports whether the context badge would render (ctx is
// non-empty and the badge is not hidden). Exposed for tests.
func (t *TerminalPane) ContextBadgeVisible() bool {
	return t.ctx != "" && !t.ctxBadgeHidden
}

// CursorPos reports the live cursor position (inner-grid coords) and whether a
// real terminal cursor should be shown for this pane: only when focused, not
// exited, viewing the live screen (not scrolled into history), and when the
// running program hasn't hidden the cursor via DECTCEM. The app translates these
// inner coords into an absolute frame position.
func (t *TerminalPane) CursorPos() (x, y int, visible bool) {
	if !t.focused || t.exited || t.scrollOffset > 0 || !t.emu.CursorVisible() {
		return 0, 0, false
	}
	cx, cy := t.emu.CursorPosition()
	return cx, cy, true
}

// CursorShape returns the shape the running program requested via DECSCUSR
// (block by default), for the app to apply to the real terminal cursor.
func (t *TerminalPane) CursorShape() CursorShape { return t.emu.CursorShape() }

// ReplyReader exposes the emulator's reply stream so the app can forward query
// replies (DA/DSR/DECRQM responses) back to the shell stdin. Reading happens on a
// background goroutine; the emulator's reply path touches only its internal pipe,
// so this is safe alongside Write/Render on the UI goroutine.
func (t *TerminalPane) ReplyReader() io.Reader { return t.emu }

// StopReplies closes the emulator's reply stream so a blocked ReplyReader.Read
// returns io.EOF, letting the reply-forwarding goroutine exit at teardown.
func (t *TerminalPane) StopReplies() { _ = t.emu.CloseReplies() }

// View renders the emulator screen inside the shared bordered/titled box using
// the same focused/blurred border styling as every other pane (so a terminal
// pane is visually indistinguishable from a resource or log pane). When the
// session has exited, a dim "[exited — status N]" banner (plus any exit note)
// is overlaid on the first content line above the frozen final screen.
func (t *TerminalPane) View() string {
	content := t.emu.RenderViewport(t.scrollOffset)

	if t.exited {
		iw, _ := innerSize(t.width, t.height)
		content = overlayExitBanner(content, t.exitBannerText(), iw)
	}

	borderStyle := UnfocusedBorderStyle
	if t.focused {
		borderStyle = FocusedBorderStyle
	}
	styled := borderStyle.Width(t.width).Height(t.height).Render(content)

	title := t.title
	// Subtle NAV indicator only while focused and waiting for a nav command.
	if t.focused && t.mode == modeNav {
		title += " -- NAV --"
	}
	// Scrollback indicator when the viewport is not at the live bottom.
	if t.scrollOffset > 0 {
		title += fmt.Sprintf(" [scroll -%d]", t.scrollOffset)
	}
	titleRendered := BuildPanelTitle(title, "", "", t.width, "")

	var rightRendered string
	if t.ctx != "" && !t.ctxBadgeHidden {
		rightRendered = PaneContextOnlineStyle.Render(" " + truncateContext(t.ctx, maxBadgeContext) + " ")
	}

	return injectBorderTitle(styled, titleRendered, rightRendered, t.focused)
}

// exitBannerText builds the dim banner line shown when the session has exited.
func (t *TerminalPane) exitBannerText() string {
	msg := fmt.Sprintf("[exited — status %d]", t.exitCode)
	if t.exitNote != "" {
		msg += " " + t.exitNote
	}
	return msg
}

// exitBannerStyle dims the exit banner so it reads as chrome over the frozen
// final screen rather than as live shell output.
var exitBannerStyle = lipgloss.NewStyle().Foreground(theme.Muted).Faint(true)

// overlayExitBanner replaces the first rendered line of content with the styled
// banner, preserving the remaining (frozen) screen lines beneath it. The banner
// is clipped (ansi-aware) to innerWidth first so a long pod-name + note cannot
// overflow the border box and corrupt the frame.
func overlayExitBanner(content, banner string, innerWidth int) string {
	if innerWidth > 0 {
		banner = ansi.Truncate(banner, innerWidth, "…")
	}
	styled := exitBannerStyle.Render(banner)
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return styled
	}
	lines[0] = styled
	return strings.Join(lines, "\n")
}

// --- input state machine ---

// HandleKey advances the prefix input machine for a single key press.
//
// Returns:
//   - handled: whether the pane consumed the key. When false, the App should
//     fall through to its normal global key handling (trie).
//   - toShell: bytes to forward to the shell session (nil when none).
//   - cmd: a PaneCommand the App must execute (PaneCmdNone when none).
//
// prefix is the configured prefix keystroke (e.g. "ctrl+a"), compared against
// msg.String().
//
// Behavior:
//
//	exited        → handled=false (pane is now a normal closeable split; the App
//	                 owns close/focus). Keys never reach a dead shell.
//	modeTyping    → prefix key flips to modeNav (handled, no bytes);
//	                 any other key is encoded to terminal bytes (handled).
//	modeNav       → the key is interpreted as a command, then mode→typing:
//	                 prefix prefix → send one literal prefix byte to the shell
//	                 h / left      → PaneCmdFocusLeft
//	                 l / right     → PaneCmdFocusRight
//	                 k / up        → PaneCmdFocusUp
//	                 j / down      → PaneCmdFocusDown
//	                 pgup          → PaneCmdScrollUp
//	                 pgdown        → PaneCmdScrollDown
//	                 z             → PaneCmdZoom
//	                 x             → PaneCmdClose
//	                 (any other)   → no-op (consumed, returns to typing)
func (t *TerminalPane) HandleKey(msg tea.KeyPressMsg, prefix string) (handled bool, toShell []byte, cmd PaneCommand) {
	// A dead shell is just a normal split — let the App handle every key.
	if t.exited {
		return false, nil, PaneCmdNone
	}

	key := msg.String()

	if t.mode == modeTyping {
		if key == prefix {
			t.mode = modeNav
			return true, nil, PaneCmdNone
		}
		// Any keystroke sent to the shell snaps the viewport back to the live
		// bottom so the user types at the prompt, not in stale history.
		t.scrollOffset = 0
		return true, encodeKey(msg), PaneCmdNone
	}

	// modeNav: interpret one command key, then return to typing.
	t.mode = modeTyping

	// Prefix-prefix sends a single literal prefix byte to the shell.
	if key == prefix {
		return true, encodeKey(msg), PaneCmdNone
	}

	switch key {
	case "h", "left":
		return true, nil, PaneCmdFocusLeft
	case "l", "right":
		return true, nil, PaneCmdFocusRight
	case "k", "up":
		return true, nil, PaneCmdFocusUp
	case "j", "down":
		return true, nil, PaneCmdFocusDown
	case "pgup":
		return true, nil, PaneCmdScrollUp
	case "pgdown":
		return true, nil, PaneCmdScrollDown
	case "z":
		return true, nil, PaneCmdZoom
	case "x":
		return true, nil, PaneCmdClose
	default:
		// Unmapped nav key: consume it (do not leak to the shell), no-op.
		return true, nil, PaneCmdNone
	}
}

// encodeKey converts a bubbletea v2 key press into the byte sequence a shell
// expects on its PTY input. It covers the keys an interactive shell needs:
// printable runes, Ctrl+<letter> control codes, Enter/Tab/Backspace/Esc, the
// arrow keys (as xterm CSI sequences), and Home/End/Delete/PageUp/PageDown.
//
// Detection uses the real tea.Key fields:
//   - msg.Mod.Contains(tea.ModCtrl) for control combinations
//   - msg.Code for special keys (tea.KeyUp, tea.KeyEnter, …) — note the C0
//     specials (Enter/Tab/Backspace/Esc) carry their control byte directly as
//     Code, so they fall through to the rune path
//   - msg.Text for the printable character(s) actually produced
//
// There is no off-the-shelf bubbletea/ansi "key → bytes" helper, so this is a
// focused hand-written table.
func encodeKey(msg tea.KeyPressMsg) []byte {
	k := msg.Key()

	// Alt/Meta+<key> → ESC-prefixed sequence (the standard xterm "meta sends
	// escape" convention readline relies on for word-motion shortcuts like
	// Alt+b / Alt+f). Emit ESC followed by the key encoded without its Alt
	// modifier. Handled here, before the ctrl/special tables, so combinations
	// like Alt+Backspace (ESC + 0x7f) and Alt+<ctrl-letter> compose correctly.
	if k.Mod.Contains(tea.ModAlt) {
		inner := k
		inner.Mod &^= tea.ModAlt
		// Alt+<printable> often carries no Text (the modifier suppresses the
		// produced character), so synthesize it from the rune Code when needed so
		// e.g. Alt+b encodes to ESC 'b' rather than dropping.
		if inner.Text == "" && inner.Mod == 0 && inner.Code >= 0x20 && inner.Code < 0x7f {
			inner.Text = string(rune(inner.Code))
		}
		if rest := encodeKey(tea.KeyPressMsg(inner)); len(rest) > 0 {
			return append([]byte{0x1b}, rest...)
		}
		return nil
	}

	// Ctrl+<letter> → C0 control code (ctrl+a → 0x01 … ctrl+z → 0x1a).
	// ctrl+space / ctrl+@ → NUL. The C0 special keys (enter/tab/esc/backspace)
	// also report ModCtrl-free with their byte as Code and are handled below.
	if k.Mod.Contains(tea.ModCtrl) {
		switch {
		case k.Code >= 'a' && k.Code <= 'z':
			return []byte{byte(k.Code - 'a' + 1)}
		case k.Code >= 'A' && k.Code <= 'Z':
			return []byte{byte(k.Code - 'A' + 1)}
		case k.Code == ' ' || k.Code == '@':
			return []byte{0x00}
		// Ctrl+<punctuation> → the remaining C0 control codes. Ctrl+[ is already
		// covered by KeyEscape (0x1b) below, so it is intentionally absent here.
		case k.Code == '\\':
			return []byte{0x1c}
		case k.Code == ']':
			return []byte{0x1d}
		case k.Code == '^':
			return []byte{0x1e}
		case k.Code == '_':
			return []byte{0x1f}
		}
	}

	switch k.Code {
	case tea.KeyUp:
		return []byte("\x1b[A")
	case tea.KeyDown:
		return []byte("\x1b[B")
	case tea.KeyRight:
		return []byte("\x1b[C")
	case tea.KeyLeft:
		return []byte("\x1b[D")
	case tea.KeyHome:
		return []byte("\x1b[H")
	case tea.KeyEnd:
		return []byte("\x1b[F")
	case tea.KeyDelete:
		return []byte("\x1b[3~")
	case tea.KeyPgUp:
		return []byte("\x1b[5~")
	case tea.KeyPgDown:
		return []byte("\x1b[6~")
	case tea.KeyEnter:
		// KeyEnter == CR; emit \r so the shell sees a carriage return.
		return []byte{'\r'}
	case tea.KeyTab:
		return []byte{'\t'}
	case tea.KeyBackspace:
		// KeyBackspace == DEL (0x7f) — the conventional erase byte.
		return []byte{0x7f}
	case tea.KeyEscape:
		return []byte{0x1b}
	case tea.KeySpace:
		return []byte{' '}
	// Function keys. F1–F4 use the SS3 (\x1bO) form; F5–F12 use CSI ~ sequences
	// (xterm convention). The gaps in the CSI numbers (no 16, 22) are part of the
	// xterm encoding, not omissions.
	case tea.KeyF1:
		return []byte("\x1bOP")
	case tea.KeyF2:
		return []byte("\x1bOQ")
	case tea.KeyF3:
		return []byte("\x1bOR")
	case tea.KeyF4:
		return []byte("\x1bOS")
	case tea.KeyF5:
		return []byte("\x1b[15~")
	case tea.KeyF6:
		return []byte("\x1b[17~")
	case tea.KeyF7:
		return []byte("\x1b[18~")
	case tea.KeyF8:
		return []byte("\x1b[19~")
	case tea.KeyF9:
		return []byte("\x1b[20~")
	case tea.KeyF10:
		return []byte("\x1b[21~")
	case tea.KeyF11:
		return []byte("\x1b[23~")
	case tea.KeyF12:
		return []byte("\x1b[24~")
	}

	// Printable characters: forward the produced text verbatim.
	if k.Text != "" {
		return []byte(k.Text)
	}

	// Unknown / unprintable key with no text — nothing to send.
	return nil
}
