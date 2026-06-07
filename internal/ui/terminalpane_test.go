package ui

import (
	"bytes"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// printable builds a KeyPressMsg for a printable rune, mirroring how bubbletea
// v2 populates a plain character key (Code == the rune, Text == the rune).
func printable(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}

// ctrl builds a KeyPressMsg for ctrl+<letter>. bubbletea reports these with the
// base rune in Code, ModCtrl set, and no Text.
func ctrl(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Mod: tea.ModCtrl}
}

func TestTerminalPane_RendersWrittenText(t *testing.T) {
	p := NewTerminalPane("exec: pod/ctr", "minikube", 40, 10)
	if _, err := p.Write([]byte("hello world")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	out := ansi.Strip(p.View())
	if !strings.Contains(out, "hello world") {
		t.Fatalf("View did not contain written text; got:\n%s", out)
	}
	if !strings.Contains(out, "exec: pod/ctr") {
		t.Fatalf("View did not contain title; got:\n%s", out)
	}
}

func TestTerminalPane_CursorPos(t *testing.T) {
	p := NewTerminalPane("t", "", 40, 10)

	// Unfocused: no cursor regardless of content.
	if _, _, visible := p.CursorPos(); visible {
		t.Fatal("unfocused pane should not report a visible cursor")
	}

	p.Focus()
	if _, err := p.Write([]byte("abc")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	x, y, visible := p.CursorPos()
	if !visible {
		t.Fatal("focused live pane should report a visible cursor")
	}
	if x != 3 || y != 0 {
		t.Fatalf("cursor = (%d,%d), want (3,0) after writing 3 chars", x, y)
	}

	// Scrolled into history: cursor hidden (live cursor is off the viewport).
	if _, err := p.Write(bytes.Repeat([]byte("line\r\n"), 20)); err != nil {
		t.Fatalf("Write: %v", err)
	}
	p.ScrollUp(5)
	if _, _, visible := p.CursorPos(); visible {
		t.Fatal("scrolled-back pane should not report a visible cursor")
	}
	p.ScrollDown(100) // snap back to live bottom
	if _, _, visible := p.CursorPos(); !visible {
		t.Fatal("pane snapped to live bottom should report a visible cursor")
	}

	// DECTCEM: the program hides the cursor (ESC[?25l) → not visible; showing it
	// again (ESC[?25h) → visible.
	if _, err := p.Write([]byte("\x1b[?25l")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, _, visible := p.CursorPos(); visible {
		t.Fatal("cursor hidden via DECTCEM should report not visible")
	}
	if _, err := p.Write([]byte("\x1b[?25h")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, _, visible := p.CursorPos(); !visible {
		t.Fatal("cursor shown via DECTCEM should report visible")
	}

	// Exited: cursor hidden.
	p.MarkExited(0)
	if _, _, visible := p.CursorPos(); visible {
		t.Fatal("exited pane should not report a visible cursor")
	}
}

func TestTerminalPane_CursorShape(t *testing.T) {
	p := NewTerminalPane("t", "", 40, 10)
	if p.CursorShape() != CursorShapeBlock {
		t.Fatalf("default shape = %v, want block", p.CursorShape())
	}
	// DECSCUSR: steady bar (ESC[6 q) → bar; steady underline (ESC[4 q) → underline.
	if _, err := p.Write([]byte("\x1b[6 q")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if p.CursorShape() != CursorShapeBar {
		t.Fatalf("after ESC[6 q shape = %v, want bar", p.CursorShape())
	}
	if _, err := p.Write([]byte("\x1b[4 q")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if p.CursorShape() != CursorShapeUnderline {
		t.Fatalf("after ESC[4 q shape = %v, want underline", p.CursorShape())
	}
}

func TestTerminalPane_FocusBlurBorderDiffers(t *testing.T) {
	p := NewTerminalPane("t", "", 40, 10)
	p.Blur()
	blurred := p.View()
	p.Focus()
	focused := p.View()
	if blurred == focused {
		t.Fatal("focused and blurred View are identical; border should differ")
	}
}

func TestTerminalPane_KindContextTitle(t *testing.T) {
	p := NewTerminalPane("debug: x", "ctx-1", 40, 10)
	if p.Context() != "ctx-1" {
		t.Fatalf("Context = %q, want ctx-1", p.Context())
	}
	if p.Title() != "debug: x" {
		t.Fatalf("Title = %q, want debug: x", p.Title())
	}
}

// TestTerminalPane_IsHidden asserts IsHidden reflects the RAW outer dimensions
// (not the clamped InnerSize): a pane sized to 0×0 (a non-focused split under
// zoom) reports hidden even though InnerSize clamps to 1×1, so the app skips
// forwarding a degenerate resize to its session.
func TestTerminalPane_IsHidden(t *testing.T) {
	p := NewTerminalPane("t", "", 40, 10)
	if p.IsHidden() {
		t.Fatal("a normally-sized pane should not be hidden")
	}
	p.SetSize(0, 0)
	if !p.IsHidden() {
		t.Fatal("a 0x0 pane should be hidden")
	}
	// InnerSize still clamps to a 1x1 minimum — IsHidden must NOT be derived from
	// it, or the clamp would mask the hidden state.
	if iw, ih := p.InnerSize(); iw != 1 || ih != 1 {
		t.Fatalf("InnerSize of hidden pane = (%d,%d), want clamped (1,1)", iw, ih)
	}
	p.SetSize(20, 6)
	if p.IsHidden() {
		t.Fatal("a re-shown pane should not be hidden")
	}
}

func TestTerminalPane_ExitedBannerAndKeysFallThrough(t *testing.T) {
	p := NewTerminalPane("t", "", 40, 10)
	p.Focus()
	p.MarkExited(137)
	p.SetExitNote("ephemeral container removed")

	out := ansi.Strip(p.View())
	if !strings.Contains(out, "[exited") {
		t.Fatalf("View missing exit banner; got:\n%s", out)
	}
	if !strings.Contains(out, "137") {
		t.Fatalf("View missing exit status; got:\n%s", out)
	}
	// The note may soft-wrap inside the box; assert on a single word so the
	// line-wrap boundary does not make the substring check brittle.
	if !strings.Contains(out, "ephemeral") {
		t.Fatalf("View missing exit note; got:\n%s", out)
	}

	handled, toShell, cmd := p.HandleKey(printable('a'), "ctrl+a")
	if handled {
		t.Fatal("exited pane should return handled=false")
	}
	if toShell != nil || cmd != PaneCmdNone {
		t.Fatalf("exited pane should produce no bytes/cmd; got %v %v", toShell, cmd)
	}
}

func TestTerminalPane_TypingEncodesPrintable(t *testing.T) {
	p := NewTerminalPane("t", "", 40, 10)
	p.Focus()
	handled, toShell, cmd := p.HandleKey(printable('l'), "ctrl+a")
	if !handled {
		t.Fatal("printable key should be handled")
	}
	if !bytes.Equal(toShell, []byte("l")) {
		t.Fatalf("toShell = %q, want %q", toShell, "l")
	}
	if cmd != PaneCmdNone {
		t.Fatalf("cmd = %v, want PaneCmdNone", cmd)
	}
	if p.mode != modeTyping {
		t.Fatal("still typing after a printable key")
	}
}

func TestTerminalPane_PrefixEntersNavThenFocusRight(t *testing.T) {
	p := NewTerminalPane("t", "", 40, 10)
	p.Focus()

	// Prefix flips to nav, no bytes.
	handled, toShell, cmd := p.HandleKey(ctrl('a'), "ctrl+a")
	if !handled || toShell != nil || cmd != PaneCmdNone {
		t.Fatalf("prefix: handled=%v toShell=%q cmd=%v", handled, toShell, cmd)
	}
	if p.mode != modeNav {
		t.Fatal("mode should be nav after prefix")
	}

	// 'l' → focus right, back to typing.
	handled, toShell, cmd = p.HandleKey(printable('l'), "ctrl+a")
	if !handled || toShell != nil {
		t.Fatalf("nav l: handled=%v toShell=%q", handled, toShell)
	}
	if cmd != PaneCmdFocusRight {
		t.Fatalf("cmd = %v, want PaneCmdFocusRight", cmd)
	}
	if p.mode != modeTyping {
		t.Fatal("mode should be typing after nav command")
	}
}

// TestTerminalPane_NavIndicatorShownInTitle asserts the "-- NAV --" indicator
// appears in the title while the focused pane is waiting for a nav command
// (entered via the prefix key).
func TestTerminalPane_NavIndicatorShownInTitle(t *testing.T) {
	p := NewTerminalPane("t", "", 40, 10)
	p.Focus()
	// Before the prefix, no NAV indicator.
	if strings.Contains(ansi.Strip(p.View()), "NAV") {
		t.Fatalf("NAV indicator shown before entering nav mode:\n%s", ansi.Strip(p.View()))
	}
	// Prefix flips to nav mode.
	p.HandleKey(ctrl('a'), "ctrl+a")
	if p.mode != modeNav {
		t.Fatal("precondition: should be in nav mode after prefix")
	}
	if !strings.Contains(ansi.Strip(p.View()), "NAV") {
		t.Fatalf("NAV indicator missing in nav mode:\n%s", ansi.Strip(p.View()))
	}
}

// TestTerminalPane_DegenerateSize asserts a 1x1 pane and a 0x0 SetSize keep a
// valid (>=1x1) inner size and that View() does not panic.
func TestTerminalPane_DegenerateSize(t *testing.T) {
	p := NewTerminalPane("t", "", 1, 1)
	iw, ih := p.InnerSize()
	if iw < 1 || ih < 1 {
		t.Fatalf("1x1 pane inner size = %dx%d, want >= 1x1", iw, ih)
	}
	_ = p.View() // must not panic

	p.SetSize(0, 0)
	iw, ih = p.InnerSize()
	if iw < 1 || ih < 1 {
		t.Fatalf("0x0 pane inner size = %dx%d, want >= 1x1", iw, ih)
	}
	_ = p.View() // must not panic
}

// TestTerminalPane_EncodeUnmappedKeyReturnsNil asserts that an unmapped key (no
// text, no modifiers, an unknown Code) is handled in typing mode but produces no
// shell bytes (toShell==nil).
func TestTerminalPane_EncodeUnmappedKeyReturnsNil(t *testing.T) {
	p := NewTerminalPane("t", "", 40, 10)
	p.Focus()
	key := tea.KeyPressMsg{Code: rune(999)}
	handled, toShell, cmd := p.HandleKey(key, "ctrl+a")
	if !handled {
		t.Fatal("unmapped key in typing mode should be handled")
	}
	if toShell != nil {
		t.Fatalf("unmapped key should produce no bytes; got %v", toShell)
	}
	if cmd != PaneCmdNone {
		t.Fatalf("unmapped key cmd = %v, want PaneCmdNone", cmd)
	}
}

func TestTerminalPane_PrefixPrefixSendsLiteral(t *testing.T) {
	p := NewTerminalPane("t", "", 40, 10)
	p.Focus()

	p.HandleKey(ctrl('a'), "ctrl+a") // enter nav
	handled, toShell, cmd := p.HandleKey(ctrl('a'), "ctrl+a")
	if !handled || cmd != PaneCmdNone {
		t.Fatalf("prefix-prefix: handled=%v cmd=%v", handled, cmd)
	}
	if !bytes.Equal(toShell, []byte{0x01}) {
		t.Fatalf("toShell = %v, want [0x01]", toShell)
	}
	if p.mode != modeTyping {
		t.Fatal("mode should be typing after prefix-prefix")
	}
}

func TestTerminalPane_NavCommands(t *testing.T) {
	cases := []struct {
		key  tea.KeyPressMsg
		want PaneCommand
	}{
		{printable('h'), PaneCmdFocusLeft},
		{printable('l'), PaneCmdFocusRight},
		{printable('k'), PaneCmdFocusUp},
		{printable('j'), PaneCmdFocusDown},
		{tea.KeyPressMsg{Code: tea.KeyLeft}, PaneCmdFocusLeft},
		{tea.KeyPressMsg{Code: tea.KeyRight}, PaneCmdFocusRight},
		{tea.KeyPressMsg{Code: tea.KeyUp}, PaneCmdFocusUp},
		{tea.KeyPressMsg{Code: tea.KeyDown}, PaneCmdFocusDown},
		{tea.KeyPressMsg{Code: tea.KeyPgUp}, PaneCmdScrollUp},
		{tea.KeyPressMsg{Code: tea.KeyPgDown}, PaneCmdScrollDown},
		{printable('z'), PaneCmdZoom},
		{printable('x'), PaneCmdClose},
	}
	for _, tc := range cases {
		p := NewTerminalPane("t", "", 40, 10)
		p.Focus()
		p.HandleKey(ctrl('a'), "ctrl+a") // enter nav
		handled, toShell, cmd := p.HandleKey(tc.key, "ctrl+a")
		if !handled || toShell != nil {
			t.Fatalf("%v: handled=%v toShell=%q", tc.key, handled, toShell)
		}
		if cmd != tc.want {
			t.Fatalf("%v: cmd = %v, want %v", tc.key, cmd, tc.want)
		}
		if p.mode != modeTyping {
			t.Fatalf("%v: mode should return to typing", tc.key)
		}
	}
}

func TestTerminalPane_NavUnmappedIsNoOp(t *testing.T) {
	p := NewTerminalPane("t", "", 40, 10)
	p.Focus()
	p.HandleKey(ctrl('a'), "ctrl+a") // enter nav
	handled, toShell, cmd := p.HandleKey(printable('q'), "ctrl+a")
	if !handled {
		t.Fatal("unmapped nav key should be consumed (handled)")
	}
	if toShell != nil || cmd != PaneCmdNone {
		t.Fatalf("unmapped nav key should be a no-op; got %q %v", toShell, cmd)
	}
	if p.mode != modeTyping {
		t.Fatal("mode should return to typing after unmapped nav key")
	}
}

func TestTerminalPane_EncodesControlAndSpecialKeys(t *testing.T) {
	cases := []struct {
		name string
		key  tea.KeyPressMsg
		want []byte
	}{
		{"ctrl+c", ctrl('c'), []byte{0x03}},
		{"ctrl+a", ctrl('a'), []byte{0x01}},
		{"ctrl+z", ctrl('z'), []byte{0x1a}},
		{"enter", tea.KeyPressMsg{Code: tea.KeyEnter}, []byte{'\r'}},
		{"tab", tea.KeyPressMsg{Code: tea.KeyTab}, []byte{'\t'}},
		{"backspace", tea.KeyPressMsg{Code: tea.KeyBackspace}, []byte{0x7f}},
		{"esc", tea.KeyPressMsg{Code: tea.KeyEscape}, []byte{0x1b}},
		{"up", tea.KeyPressMsg{Code: tea.KeyUp}, []byte("\x1b[A")},
		{"down", tea.KeyPressMsg{Code: tea.KeyDown}, []byte("\x1b[B")},
		{"right", tea.KeyPressMsg{Code: tea.KeyRight}, []byte("\x1b[C")},
		{"left", tea.KeyPressMsg{Code: tea.KeyLeft}, []byte("\x1b[D")},
		{"home", tea.KeyPressMsg{Code: tea.KeyHome}, []byte("\x1b[H")},
		{"end", tea.KeyPressMsg{Code: tea.KeyEnd}, []byte("\x1b[F")},
		{"delete", tea.KeyPressMsg{Code: tea.KeyDelete}, []byte("\x1b[3~")},
	}
	// Use a prefix that none of the encoded keys collide with, so each key is
	// encoded in typing mode rather than being swallowed as the prefix.
	for _, tc := range cases {
		p := NewTerminalPane("t", "", 40, 10)
		p.Focus()
		handled, toShell, cmd := p.HandleKey(tc.key, "ctrl+b")
		if !handled || cmd != PaneCmdNone {
			t.Fatalf("%s: handled=%v cmd=%v", tc.name, handled, cmd)
		}
		if !bytes.Equal(toShell, tc.want) {
			t.Fatalf("%s: toShell = %v, want %v", tc.name, toShell, tc.want)
		}
	}
}

// TestTerminalPane_EncodesFunctionKeys asserts F1–F12 are encoded with the
// xterm SS3 (F1–F4) and CSI~ (F5–F12) sequences a shell/full-screen program
// expects, rather than being silently dropped.
func TestTerminalPane_EncodesFunctionKeys(t *testing.T) {
	cases := []struct {
		name string
		code rune
		want []byte
	}{
		{"f1", tea.KeyF1, []byte("\x1bOP")},
		{"f2", tea.KeyF2, []byte("\x1bOQ")},
		{"f3", tea.KeyF3, []byte("\x1bOR")},
		{"f4", tea.KeyF4, []byte("\x1bOS")},
		{"f5", tea.KeyF5, []byte("\x1b[15~")},
		{"f6", tea.KeyF6, []byte("\x1b[17~")},
		{"f7", tea.KeyF7, []byte("\x1b[18~")},
		{"f8", tea.KeyF8, []byte("\x1b[19~")},
		{"f9", tea.KeyF9, []byte("\x1b[20~")},
		{"f10", tea.KeyF10, []byte("\x1b[21~")},
		{"f11", tea.KeyF11, []byte("\x1b[23~")},
		{"f12", tea.KeyF12, []byte("\x1b[24~")},
	}
	for _, tc := range cases {
		if got := encodeKey(tea.KeyPressMsg{Code: tc.code}); !bytes.Equal(got, tc.want) {
			t.Fatalf("%s: encodeKey = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestTerminalPane_EncodesCtrlPunctuation asserts Ctrl+\ ] ^ _ encode to their
// C0 control codes (0x1c–0x1f), and that Ctrl+[ stays the ESC byte (handled via
// KeyEscape, not the ctrl-punctuation table).
func TestTerminalPane_EncodesCtrlPunctuation(t *testing.T) {
	cases := []struct {
		name string
		key  tea.KeyPressMsg
		want []byte
	}{
		{"ctrl+backslash", tea.KeyPressMsg{Code: '\\', Mod: tea.ModCtrl}, []byte{0x1c}},
		{"ctrl+rbracket", tea.KeyPressMsg{Code: ']', Mod: tea.ModCtrl}, []byte{0x1d}},
		{"ctrl+caret", tea.KeyPressMsg{Code: '^', Mod: tea.ModCtrl}, []byte{0x1e}},
		{"ctrl+underscore", tea.KeyPressMsg{Code: '_', Mod: tea.ModCtrl}, []byte{0x1f}},
		{"esc still esc", tea.KeyPressMsg{Code: tea.KeyEscape}, []byte{0x1b}},
	}
	for _, tc := range cases {
		if got := encodeKey(tc.key); !bytes.Equal(got, tc.want) {
			t.Fatalf("%s: encodeKey = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestTerminalPane_EncodesAltKeys asserts Alt/Meta combinations are encoded with
// the standard ESC meta-prefix so readline word shortcuts (Alt+b/Alt+f) and
// Alt+Backspace reach the shell rather than being dropped.
func TestTerminalPane_EncodesAltKeys(t *testing.T) {
	cases := []struct {
		name string
		key  tea.KeyPressMsg
		want []byte
	}{
		// Alt+b with Text present (some terminals deliver it).
		{"alt+b text", tea.KeyPressMsg{Code: 'b', Mod: tea.ModAlt, Text: "b"}, []byte("\x1bb")},
		// Alt+b with no Text — must synthesize from the rune Code.
		{"alt+b no-text", tea.KeyPressMsg{Code: 'b', Mod: tea.ModAlt}, []byte("\x1bb")},
		{"alt+f", tea.KeyPressMsg{Code: 'f', Mod: tea.ModAlt}, []byte("\x1bf")},
		// Alt+Backspace → ESC + DEL (readline backward-kill-word).
		{"alt+backspace", tea.KeyPressMsg{Code: tea.KeyBackspace, Mod: tea.ModAlt}, []byte{0x1b, 0x7f}},
	}
	for _, tc := range cases {
		if got := encodeKey(tc.key); !bytes.Equal(got, tc.want) {
			t.Fatalf("%s: encodeKey = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestTerminalPane_BlurResetsModeToTyping(t *testing.T) {
	p := NewTerminalPane("t", "", 40, 10)
	p.Focus()
	p.HandleKey(ctrl('a'), "ctrl+a") // enter nav
	if p.mode != modeNav {
		t.Fatal("precondition: should be in nav")
	}
	p.Blur()
	p.Focus()
	// Next key must encode to the shell (typing), not be treated as a nav cmd.
	handled, toShell, cmd := p.HandleKey(printable('l'), "ctrl+a")
	if !handled || cmd != PaneCmdNone {
		t.Fatalf("after blur/refocus: handled=%v cmd=%v", handled, cmd)
	}
	if !bytes.Equal(toShell, []byte("l")) {
		t.Fatalf("after blur/refocus: toShell = %q, want %q", toShell, "l")
	}
}

// fillScrollback writes n single-char lines through a small pane so most scroll
// off the live screen into the scrollback buffer.
func fillScrollback(t *testing.T, p *TerminalPane, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		if _, err := p.Write([]byte("L" + string(rune('a'+i%26)) + "\r\n")); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}
}

func TestTerminalPane_ScrollUpDownAdjustsOffset(t *testing.T) {
	p := NewTerminalPane("t", "", 20, 5) // inner height 3
	p.SetScrollback(1000)
	fillScrollback(t, p, 20)

	if p.ScrollOffset() != 0 {
		t.Fatalf("offset should start at 0, got %d", p.ScrollOffset())
	}
	p.ScrollUp(5)
	if p.ScrollOffset() != 5 {
		t.Fatalf("after ScrollUp(5), offset = %d, want 5", p.ScrollOffset())
	}
	p.ScrollDown(2)
	if p.ScrollOffset() != 3 {
		t.Fatalf("after ScrollDown(2), offset = %d, want 3", p.ScrollOffset())
	}
	// Scroll down past the bottom clamps to 0.
	p.ScrollDown(100)
	if p.ScrollOffset() != 0 {
		t.Fatalf("ScrollDown past bottom should clamp to 0, got %d", p.ScrollOffset())
	}
}

func TestTerminalPane_ScrollUpClampsToScrollbackLen(t *testing.T) {
	p := NewTerminalPane("t", "", 20, 5)
	p.SetScrollback(1000)
	fillScrollback(t, p, 20)

	p.ScrollUp(100000)
	if p.ScrollOffset() == 0 {
		t.Fatal("ScrollUp should have moved the offset above 0")
	}
	if p.ScrollOffset() > p.emu.ScrollbackLen() {
		t.Fatalf("offset %d exceeds scrollback length %d", p.ScrollOffset(), p.emu.ScrollbackLen())
	}
}

func TestTerminalPane_ScrollOffsetShownInTitle(t *testing.T) {
	p := NewTerminalPane("t", "", 20, 5)
	p.SetScrollback(1000)
	p.Focus()
	fillScrollback(t, p, 20)
	p.ScrollUp(4)
	if !strings.Contains(ansi.Strip(p.View()), "scroll -4") {
		t.Fatalf("expected scroll indicator in title, got:\n%s", ansi.Strip(p.View()))
	}
}

func TestTerminalPane_WriteSnapsToBottom(t *testing.T) {
	p := NewTerminalPane("t", "", 20, 5)
	p.SetScrollback(1000)
	fillScrollback(t, p, 20)
	p.ScrollUp(5)
	if p.ScrollOffset() == 0 {
		t.Fatal("precondition: should be scrolled up")
	}
	// New shell output snaps the viewport back to the live bottom.
	if _, err := p.Write([]byte("new output\r\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if p.ScrollOffset() != 0 {
		t.Fatalf("Write should snap offset to 0, got %d", p.ScrollOffset())
	}
}

func TestTerminalPane_TypingSnapsToBottom(t *testing.T) {
	p := NewTerminalPane("t", "", 20, 5)
	p.SetScrollback(1000)
	p.Focus()
	fillScrollback(t, p, 20)
	p.ScrollUp(5)
	if p.ScrollOffset() == 0 {
		t.Fatal("precondition: should be scrolled up")
	}
	// A printable key sent to the shell snaps back to the bottom.
	p.HandleKey(printable('a'), "ctrl+a")
	if p.ScrollOffset() != 0 {
		t.Fatalf("typing should snap offset to 0, got %d", p.ScrollOffset())
	}
}
