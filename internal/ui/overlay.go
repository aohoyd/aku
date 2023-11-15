package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// overlayOpts holds configuration for PlaceOverlay.
type overlayOpts struct {
	hPos float64
	vPos float64
	dim  bool
}

// OverlayOption configures PlaceOverlay behavior.
type OverlayOption func(*overlayOpts)

// WithOverlayPosition sets the normalized horizontal and vertical anchor.
// 0.5, 0.5 is center (the default). 0.5, 1.0 pins to bottom-center.
func WithOverlayPosition(h, v float64) OverlayOption {
	return func(o *overlayOpts) {
		o.hPos = h
		o.vPos = v
	}
}

// WithDim controls whether the background is dimmed. Enabled by default.
func WithDim(d bool) OverlayOption {
	return func(o *overlayOpts) { o.dim = d }
}

const (
	dimOn  = "\x1b[2m"
	dimOff = "\x1b[22m"
)

// PlaceOverlay composites the overlay string centered (by default) over bg,
// dimming the background content that is not covered by the overlay.
// bgWidth and bgHeight are the terminal dimensions.
func PlaceOverlay(bgWidth, bgHeight int, bg, overlay string, opts ...OverlayOption) string {
	if overlay == "" {
		return bg
	}

	cfg := overlayOpts{hPos: 0.5, vPos: 0.5, dim: true}
	for _, o := range opts {
		o(&cfg)
	}

	bgLines := strings.Split(bg, "\n")
	for len(bgLines) < bgHeight {
		bgLines = append(bgLines, strings.Repeat(" ", bgWidth))
	}
	if len(bgLines) > bgHeight {
		bgLines = bgLines[:bgHeight]
	}

	overlayLines := strings.Split(overlay, "\n")
	oh := len(overlayLines)

	ow := 0
	for _, l := range overlayLines {
		if w := ansi.StringWidth(l); w > ow {
			ow = w
		}
	}

	col := int(float64(bgWidth-ow) * cfg.hPos)
	row := int(float64(bgHeight-oh) * cfg.vPos)
	if col < 0 {
		col = 0
	}
	if row < 0 {
		row = 0
	}
	if col+ow > bgWidth {
		col = max(0, bgWidth-ow)
	}
	if row+oh > bgHeight {
		row = max(0, bgHeight-oh)
	}

	var sb strings.Builder
	sb.Grow(len(bg) + len(overlay) + bgHeight*10)

	for i, bgLine := range bgLines {
		lw := ansi.StringWidth(bgLine)
		if lw < bgWidth {
			bgLine += strings.Repeat(" ", bgWidth-lw)
		}

		overlayRow := i - row

		if overlayRow < 0 || overlayRow >= oh {
			if cfg.dim {
				sb.WriteString(dimOn)
				sb.WriteString(bgLine)
				sb.WriteString(dimOff)
			} else {
				sb.WriteString(bgLine)
			}
		} else {
			oLine := overlayLines[overlayRow]
			oLineW := ansi.StringWidth(oLine)

			left := ansi.Truncate(bgLine, col, "")
			right := ansi.Cut(bgLine, col+ow, bgWidth)

			pad := max(ow-oLineW, 0)

			if cfg.dim {
				sb.WriteString(dimOn)
				sb.WriteString(left)
				sb.WriteString(dimOff)
			} else {
				sb.WriteString(left)
			}

			sb.WriteString(oLine)
			if pad > 0 {
				sb.WriteString(strings.Repeat(" ", pad))
			}

			if cfg.dim {
				sb.WriteString(dimOn)
				sb.WriteString(right)
				sb.WriteString(dimOff)
			} else {
				sb.WriteString(right)
			}
		}

		if i < len(bgLines)-1 {
			sb.WriteRune('\n')
		}
	}

	return sb.String()
}
