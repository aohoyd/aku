package highlight

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/aohoyd/aku/internal/theme"
)

// Painter holds a pre-computed ANSI prefix string and paints text by wrapping
// it with prefix + text + AnsiReset. A zero-value Painter (empty prefix) is
// valid and returns input unchanged.
type Painter struct {
	prefix string // e.g., "\x1b[38;2;126;156;216m"
}

// FgPainter creates a Painter for foreground color from a theme hex color.
func FgPainter(c theme.Color) Painter {
	return Painter{prefix: "\x1b[" + colorToSGR(string(c), false) + "m"}
}

// BoldFgPainter creates a Painter for bold + foreground color.
func BoldFgPainter(c theme.Color) Painter {
	return Painter{prefix: "\x1b[1;" + colorToSGR(string(c), false) + "m"}
}

// FaintFgPainter creates a Painter for faint/dim + foreground color.
func FaintFgPainter(c theme.Color) Painter {
	return Painter{prefix: "\x1b[2;" + colorToSGR(string(c), false) + "m"}
}

// BgFgPainter creates a Painter for background + foreground color.
func BgFgPainter(bg, fg theme.Color) Painter {
	return Painter{prefix: "\x1b[" + colorToSGR(string(bg), true) + ";" + colorToSGR(string(fg), false) + "m"}
}

// Paint wraps s with the ANSI prefix and reset. Returns s unchanged if the
// Painter has no prefix (zero value).
func (p Painter) Paint(s string) string {
	if p.prefix == "" {
		return s
	}
	return p.prefix + s + AnsiReset
}

// WriteTo writes the painted string to a strings.Builder. If the Painter has
// no prefix, it writes s as-is.
func (p Painter) WriteTo(sb *strings.Builder, s string) {
	if p.prefix == "" {
		sb.WriteString(s)
		return
	}
	sb.WriteString(p.prefix)
	sb.WriteString(s)
	sb.WriteString(AnsiReset)
}

// Prefix returns the pre-computed ANSI prefix string.
func (p Painter) Prefix() string {
	return p.prefix
}

// colorToSGR converts a theme color string to an SGR parameter.
// Accepts ANSI-256 index ("43") or hex ("#FF5555").
// bg selects background (48) vs foreground (38).
func colorToSGR(c string, bg bool) string {
	base := 38
	if bg {
		base = 48
	}
	if strings.HasPrefix(c, "#") && len(c) == 7 {
		r, err1 := strconv.ParseUint(c[1:3], 16, 8)
		g, err2 := strconv.ParseUint(c[3:5], 16, 8)
		b, err3 := strconv.ParseUint(c[5:7], 16, 8)
		if err1 != nil || err2 != nil || err3 != nil {
			return fmt.Sprintf("%d;5;1", base) // fallback to red
		}
		return fmt.Sprintf("%d;2;%d;%d;%d", base, r, g, b)
	}
	return fmt.Sprintf("%d;5;%s", base, c)
}
