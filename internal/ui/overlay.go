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

// OverlayRect describes the screen area occupied by a rendered overlay.
// X,Y are the top-left cell coordinates; W,H are the overlay content width
// and height in cells.
type OverlayRect struct {
	X, Y, W, H int
}

// Contains reports whether the given cell coordinate lies inside the rect.
// Returns false for zero-sized rects (W == 0 || H == 0), which is the
// sentinel for "no active overlay".
func (r OverlayRect) Contains(x, y int) bool {
	if r.W == 0 || r.H == 0 {
		return false
	}
	return x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H
}

// overlayContentWidth returns the maximum cell-width across overlay lines.
func overlayContentWidth(overlayLines []string) int {
	ow := 0
	for _, l := range overlayLines {
		if w := ansi.StringWidth(l); w > ow {
			ow = w
		}
	}
	return ow
}

// overlayAnchor returns the top-left (col, row) where an ow x oh overlay
// should be placed on a bgWidth x bgHeight background at the given
// normalized position. Coordinates are clamped so the overlay fits inside
// the background whenever it is small enough.
func overlayAnchor(bgWidth, bgHeight, ow, oh int, hPos, vPos float64) (int, int) {
	col := int(float64(bgWidth-ow) * hPos)
	row := int(float64(bgHeight-oh) * vPos)
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
	return col, row
}

// PlaceOverlay composites the overlay string centered (by default) over bg,
// dimming the background content that is not covered by the overlay.
// bgWidth and bgHeight are the terminal dimensions.
func PlaceOverlay(bgWidth, bgHeight int, bg, overlay string, opts ...OverlayOption) string {
	out, _, _ := placeOverlayInternal(bgWidth, bgHeight, bg, overlay, opts...)
	return out
}

// PlaceOverlayWithRect is like PlaceOverlay but also returns the rect that the
// overlay occupies on screen. Splits and width measurement happen once. When
// overlay is empty, returns bg and a zero rect with ok=false.
func PlaceOverlayWithRect(bgWidth, bgHeight int, bg, overlay string, opts ...OverlayOption) (string, OverlayRect, bool) {
	return placeOverlayInternal(bgWidth, bgHeight, bg, overlay, opts...)
}

func placeOverlayInternal(bgWidth, bgHeight int, bg, overlay string, opts ...OverlayOption) (string, OverlayRect, bool) {
	if overlay == "" {
		return bg, OverlayRect{}, false
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

	ow := overlayContentWidth(overlayLines)
	col, row := overlayAnchor(bgWidth, bgHeight, ow, oh, cfg.hPos, cfg.vPos)

	// Visible dimensions of the rect, clamped to the background so callers
	// hit-testing with Contains never reach past the terminal edge.
	visW := ow
	if col+visW > bgWidth {
		visW = max(0, bgWidth-col)
	}
	visH := oh
	if row+visH > bgHeight {
		visH = max(0, bgHeight-row)
	}
	rect := OverlayRect{X: col, Y: row, W: visW, H: visH}

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

	return sb.String(), rect, true
}
