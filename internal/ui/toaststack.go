package ui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/aohoyd/aku/internal/notify"
	"github.com/aohoyd/aku/internal/theme"
	"github.com/charmbracelet/x/ansi"
)

// Glyphs prefixed to each toast based on its level.
const (
	toastGlyphInfo    = "ℹ"
	toastGlyphWarning = "⚠"
	toastGlyphError   = "✖"
)

const defaultToastMaxVisible = 5

// Per-render cap constants. Toast boxes are sized proportionally to the
// terminal, bounded by absolute floors and ceilings. Width is a fraction of the
// terminal width; height a fraction of the terminal height (in content lines).
const (
	// Width cap: maxW = clamp(termW * num/den, floor, ceil).
	toastWidthNum   = 2
	toastWidthDen   = 5
	toastWidthFloor = 8
	toastWidthCeil  = 60

	// Height cap (content lines): maxH = clamp(termH * num/den, floor, ceil).
	toastHeightNum   = 3
	toastHeightDen   = 10
	toastHeightFloor = 1
	toastHeightCeil  = 6
)

// Per-level toast box styles. Each reuses OverlayStyle's rounded border shape
// (so tests detecting ╭╮╰╯ still pass) but intentionally overrides padding to a
// tighter Padding(0,1): toasts are compact corner notifications, not modal
// panels. Theme colors are fixed at startup (theme.Load runs in theme's package
// init, before these vars initialize and before any rendering), so capturing
// theme.Error/Warning/Accent here cannot go stale — the same convention used by
// the styles in styles.go. Building these once avoids rebuilding the level/border
// styles on every renderToast call. (The inner Width style in renderToast is
// necessarily per-toast, since boxW varies with content, and is not cached here.)
var (
	toastStyleError = OverlayStyle.BorderForeground(theme.Error).Padding(0, 1)
	toastStyleWarn  = OverlayStyle.BorderForeground(theme.Warning).Padding(0, 1)
	toastStyleInfo  = OverlayStyle.BorderForeground(theme.Accent).Padding(0, 1)

	// toastMoreStyle renders the "+N more…" overflow line.
	toastMoreStyle = lipgloss.NewStyle().Foreground(theme.Muted).Faint(true)

	// toastChrome is the horizontal cells consumed by box chrome around the
	// inner content: border (left+right) + padding (left+right). It is derived
	// once at package init from toastStyleInfo's border+padding accessors (all
	// three level styles share the same border/padding), so it reflects the
	// toast style's chrome. Unlike overlay_panel.go's overlayChrome(), which is
	// a function recomputed live, this is a single value captured at init — it
	// tracks a style change only via re-initialization, not per call.
	toastChrome = toastStyleInfo.GetBorderLeftSize() + toastStyleInfo.GetBorderRightSize() +
		toastStyleInfo.GetPaddingLeft() + toastStyleInfo.GetPaddingRight()
)

// ToastStack is a pure renderer for a vertical, right-aligned stack of toast
// notifications (noice.nvim-style). It holds no message state: View takes the
// live messages and terminal dimensions on each call; those dimensions drive the
// per-render width/height caps each box is sized within. Construct with
// NewToastStack.
type ToastStack struct {
	maxVisible int
}

// NewToastStack returns a ToastStack rendering up to maxVisible toasts. A
// non-positive maxVisible falls back to the default (5 visible).
func NewToastStack(maxVisible int) ToastStack {
	if maxVisible <= 0 {
		maxVisible = defaultToastMaxVisible
	}
	return ToastStack{maxVisible: maxVisible}
}

// View renders the stack as bordered boxes joined vertically, right-aligned
// (the stack sits in the top-right corner of the screen). It is a pure,
// side-effect-free renderer.
//
// live is expected newest-first — notify.Store.Live already returns messages in
// that order, so View renders them in the order given and does NOT re-sort.
// Empty or nil live yields the empty string.
//
// Each box's width and height caps are derived proportionally from termW/termH
// (a fraction of each), bounded by absolute floors and ceilings — see the cap
// constants (toastWidth*/toastHeight*) for the exact fractions and bounds.
func (t ToastStack) View(live []notify.Message, termW, termH int) string {
	if len(live) == 0 {
		return ""
	}

	// Too narrow/short to hold even a minimal toast: render nothing rather than
	// overflow the terminal. The minimal outer box is toastWidthFloor inner
	// cells + toastChrome; below that (or a non-positive height) any box we draw
	// would spill past the screen edge, so suppress the whole stack. Above this
	// threshold the floors below guarantee a box that fits.
	if termW < toastWidthFloor+toastChrome || termH < 1 {
		return ""
	}

	// Width cap, clamped between floor and ceiling. Because termW is guaranteed
	// >= toastWidthFloor+toastChrome here, the resulting outer box (maxW+chrome)
	// always fits the terminal: for termW in [floor+chrome, ...] the
	// proportional value termW*num/den never exceeds termW-chrome, so no
	// separate box-fit clamp is needed.
	maxW := min(max(termW*toastWidthNum/toastWidthDen, toastWidthFloor), toastWidthCeil)
	maxH := min(max(termH*toastHeightNum/toastHeightDen, toastHeightFloor), toastHeightCeil)

	visible := live
	overflow := 0
	if len(live) > t.maxVisible {
		visible = live[:t.maxVisible]
		overflow = len(live) - t.maxVisible
	}

	boxes := make([]string, 0, len(visible)+1)
	for _, m := range visible {
		boxes = append(boxes, renderToast(m, maxW, maxH))
	}

	if overflow > 0 {
		more := fmt.Sprintf("+%d more…", overflow)
		boxes = append(boxes, toastMoreStyle.Render(more))
	}

	return lipgloss.JoinVertical(lipgloss.Right, boxes...)
}

// renderToast renders a single message as a bordered box, colored by level. The
// box shrinks to fit its content horizontally (capped at maxW), word-wraps onto
// up to maxH content lines (capped), and appends a trailing ellipsis on the last
// visible line when wrapped content still overflows the height cap.
func renderToast(m notify.Message, maxW, maxH int) string {
	glyph, style := toastDecoration(m.Level)

	content := glyph + " " + m.Text
	// Size the box to the widest single line. m.Text may contain embedded
	// newlines; summing the whole string's width (across "\n") would pad the box
	// wider than its widest line and break shrink-to-fit. Measure per line.
	widest := 0
	for _, line := range strings.Split(content, "\n") {
		if w := ansi.StringWidth(line); w > widest {
			widest = w
		}
	}
	boxW := min(widest, maxW)               // shrink to fit, capped
	wrapped := ansi.Wrap(content, boxW, "") // word wrap; long words hard-break
	lines := strings.Split(wrapped, "\n")
	if len(lines) > maxH { // height overflow → ellipsis
		lines = lines[:maxH]
		last := lines[maxH-1]
		lines[maxH-1] = ansi.Truncate(last+"…", boxW, "…")
	}
	inner := lipgloss.NewStyle().Width(boxW).Render(strings.Join(lines, "\n"))

	return style.Render(inner)
}

// toastDecoration returns the glyph and the cached box style for a level. The
// style is one of the package-level toastStyle* vars (built once from
// OverlayStyle) so renderToast does not rebuild a lipgloss.Style per call.
func toastDecoration(level notify.Level) (glyph string, style lipgloss.Style) {
	switch level {
	case notify.LevelError:
		return toastGlyphError, toastStyleError
	case notify.LevelWarning:
		return toastGlyphWarning, toastStyleWarn
	default:
		return toastGlyphInfo, toastStyleInfo
	}
}
