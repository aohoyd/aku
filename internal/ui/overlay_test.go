package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func makeBg(w, h int) string {
	line := strings.Repeat("x", w)
	lines := make([]string, h)
	for i := range lines {
		lines[i] = line
	}
	return strings.Join(lines, "\n")
}

func TestPlaceOverlayContainsOverlayContent(t *testing.T) {
	bg := makeBg(20, 5)
	overlay := "POPUP"
	result := PlaceOverlay(20, 5, bg, overlay)
	stripped := ansi.Strip(result)
	if !strings.Contains(stripped, "POPUP") {
		t.Fatalf("expected overlay content in result, got: %q", stripped)
	}
}

func TestPlaceOverlayPreservesBackground(t *testing.T) {
	bg := makeBg(20, 5)
	overlay := "P"
	result := PlaceOverlay(20, 5, bg, overlay, WithDim(false))
	stripped := ansi.Strip(result)
	if !strings.Contains(stripped, "x") {
		t.Fatalf("expected background content outside overlay, got: %q", stripped)
	}
}

func TestPlaceOverlayDimsBg(t *testing.T) {
	bg := makeBg(20, 5)
	overlay := "hello"
	result := PlaceOverlay(20, 5, bg, overlay)
	if !strings.Contains(result, "\x1b[2m") {
		t.Error("expected dim SGR code in result")
	}
}

func TestPlaceOverlayNoDim(t *testing.T) {
	bg := makeBg(20, 5)
	overlay := "hello"
	result := PlaceOverlay(20, 5, bg, overlay, WithDim(false))
	if strings.Contains(result, "\x1b[2m") {
		t.Error("did not expect dim SGR code when WithDim(false)")
	}
}

func TestPlaceOverlayEmptyOverlay(t *testing.T) {
	bg := makeBg(20, 5)
	result := PlaceOverlay(20, 5, bg, "")
	if result != bg {
		t.Error("empty overlay should return bg unchanged")
	}
}

func TestPlaceOverlayLineCount(t *testing.T) {
	bg := makeBg(30, 10)
	overlay := "box"
	result := PlaceOverlay(30, 10, bg, overlay)
	bgLines := strings.Count(bg, "\n")
	resultLines := strings.Count(result, "\n")
	if resultLines != bgLines {
		t.Errorf("line count changed: bg=%d result=%d", bgLines, resultLines)
	}
}

func TestPlaceOverlayCustomPosition(t *testing.T) {
	bg := makeBg(30, 10)
	overlay := "BTM"
	result := PlaceOverlay(30, 10, bg, overlay, WithOverlayPosition(0.5, 1.0), WithDim(false))
	lines := strings.Split(result, "\n")
	lastLine := ansi.Strip(lines[len(lines)-1])
	if !strings.Contains(lastLine, "BTM") {
		t.Errorf("expected overlay on last line, got: %q", lastLine)
	}
}

func TestPlaceOverlayClampsPosition(t *testing.T) {
	bg := makeBg(10, 3)
	overlay := strings.Repeat("A", 15)
	result := PlaceOverlay(10, 3, bg, overlay)
	if result == "" {
		t.Error("expected non-empty result even with oversized overlay")
	}
}

func TestPlaceOverlayWithRectEmpty(t *testing.T) {
	bg := makeBg(20, 5)
	out, r, ok := PlaceOverlayWithRect(20, 5, bg, "")
	if ok {
		t.Fatal("expected ok=false for empty overlay")
	}
	if out != bg {
		t.Fatal("empty overlay should return bg unchanged")
	}
	if r != (OverlayRect{}) {
		t.Fatalf("expected zero rect, got %+v", r)
	}
}

func TestPlaceOverlayWithRectCentered(t *testing.T) {
	// 10-wide, 3-line overlay centered in 30x9 bg: (30-10)/2 = 10 col, (9-3)/2 = 3 row.
	bg := makeBg(30, 9)
	overlay := strings.Join([]string{"aaaaaaaaaa", "bbbbbbbbbb", "cccccccccc"}, "\n")
	_, r, ok := PlaceOverlayWithRect(30, 9, bg, overlay)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if r.X != 10 || r.Y != 3 || r.W != 10 || r.H != 3 {
		t.Fatalf("expected rect {10,3,10,3}, got %+v", r)
	}
}

func TestPlaceOverlayWithRectCustomPosition(t *testing.T) {
	bg := makeBg(20, 5)
	overlay := "XX"
	_, r, ok := PlaceOverlayWithRect(20, 5, bg, overlay, WithOverlayPosition(0, 0))
	if !ok {
		t.Fatal("expected ok=true")
	}
	if r.X != 0 || r.Y != 0 || r.W != 2 || r.H != 1 {
		t.Fatalf("expected rect {0,0,2,1}, got %+v", r)
	}
}

func TestPlaceOverlayWithRectClamped(t *testing.T) {
	bg := makeBg(10, 3)
	overlay := strings.Repeat("A", 15) // wider than bg
	_, r, ok := PlaceOverlayWithRect(10, 3, bg, overlay)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if r.X < 0 || r.Y < 0 {
		t.Fatalf("expected non-negative X,Y, got %+v", r)
	}
	// Visible dimensions must not exceed the background.
	if r.X+r.W > 10 {
		t.Fatalf("X+W=%d exceeds bgWidth=10: %+v", r.X+r.W, r)
	}
	if r.Y+r.H > 3 {
		t.Fatalf("Y+H=%d exceeds bgHeight=3: %+v", r.Y+r.H, r)
	}
}

// TestPlaceOverlayWithRectClampedMultiline verifies both axes clamp when the
// overlay is larger than the background in both dimensions.
func TestPlaceOverlayWithRectClampedMultiline(t *testing.T) {
	overlay := strings.Join([]string{
		strings.Repeat("A", 15),
		strings.Repeat("B", 15),
		strings.Repeat("C", 15),
		strings.Repeat("D", 15),
		strings.Repeat("E", 15),
		strings.Repeat("F", 15),
	}, "\n")
	bg := makeBg(10, 3)
	_, r, ok := PlaceOverlayWithRect(10, 3, bg, overlay)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if r.X+r.W > 10 || r.Y+r.H > 3 {
		t.Fatalf("rect overflows bg 10x3: %+v", r)
	}
	if r.W <= 0 || r.H <= 0 {
		t.Fatalf("expected positive visible W,H, got %+v", r)
	}
}

// TestPlaceOverlayWithRectMatchesPlaceOverlay verifies the rendered string
// from PlaceOverlayWithRect equals the one from PlaceOverlay for the same
// inputs, so callers that pick the Rect-returning variant do not get a
// different composite.
func TestPlaceOverlayWithRectMatchesPlaceOverlay(t *testing.T) {
	bg := makeBg(40, 12)
	overlay := strings.Join([]string{"----", "-XX-", "----"}, "\n")
	expected := PlaceOverlay(40, 12, bg, overlay, WithDim(false))
	gotStr, gotRect, ok := PlaceOverlayWithRect(40, 12, bg, overlay, WithDim(false))
	if !ok {
		t.Fatal("expected ok=true")
	}
	if gotStr != expected {
		t.Fatal("PlaceOverlayWithRect rendered string differs from PlaceOverlay")
	}
	if gotRect.W == 0 || gotRect.H == 0 {
		t.Fatalf("expected non-zero rect, got %+v", gotRect)
	}
	// Verify rect matches the overlay size.
	if gotRect.W != 4 || gotRect.H != 3 {
		t.Fatalf("expected rect 4x3, got %+v", gotRect)
	}
}

// TestOverlayRectContainsZeroDim verifies Contains returns false for any
// coordinate when the rect has zero width or zero height — the sentinel for
// "no active overlay".
func TestOverlayRectContainsZeroDim(t *testing.T) {
	cases := []struct {
		name string
		rect OverlayRect
	}{
		{"zero everything", OverlayRect{}},
		{"zero W", OverlayRect{X: 5, Y: 5, W: 0, H: 3}},
		{"zero H", OverlayRect{X: 5, Y: 5, W: 3, H: 0}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.rect.Contains(0, 0) {
				t.Error("Contains(0,0) must be false for zero rect")
			}
			if tc.rect.Contains(5, 5) {
				t.Error("Contains(5,5) must be false for zero rect")
			}
			if tc.rect.Contains(10, 10) {
				t.Error("Contains(10,10) must be false for zero rect")
			}
		})
	}
}

// TestOverlayRectContainsInclusiveTopLeft verifies the top-left corner
// (x == X, y == Y) is inside the rect.
func TestOverlayRectContainsInclusiveTopLeft(t *testing.T) {
	r := OverlayRect{X: 3, Y: 4, W: 5, H: 6}
	if !r.Contains(r.X, r.Y) {
		t.Errorf("top-left (%d,%d) must be inside rect %+v", r.X, r.Y, r)
	}
}

// TestOverlayRectContainsExclusiveBottomRight verifies the coordinate one past
// the right/bottom edge (x == X+W, y == Y+H) is NOT inside the rect — the
// standard half-open convention.
func TestOverlayRectContainsExclusiveBottomRight(t *testing.T) {
	r := OverlayRect{X: 3, Y: 4, W: 5, H: 6}
	if r.Contains(r.X+r.W, r.Y+r.H/2) {
		t.Errorf("x == X+W must be outside rect %+v", r)
	}
	if r.Contains(r.X+r.W/2, r.Y+r.H) {
		t.Errorf("y == Y+H must be outside rect %+v", r)
	}
	if r.Contains(r.X+r.W, r.Y+r.H) {
		t.Errorf("bottom-right corner (X+W, Y+H) must be outside rect %+v", r)
	}
	// Last inclusive cell (X+W-1, Y+H-1) must still be inside.
	if !r.Contains(r.X+r.W-1, r.Y+r.H-1) {
		t.Errorf("last inclusive cell (X+W-1, Y+H-1) must be inside rect %+v", r)
	}
}

// TestOverlayRectContainsOutOfBounds covers negative and far-outside coords.
func TestOverlayRectContainsOutOfBounds(t *testing.T) {
	r := OverlayRect{X: 3, Y: 4, W: 5, H: 6}
	for _, pt := range []struct{ x, y int }{
		{-1, 5},       // negative x
		{5, -1},       // negative y
		{-1, -1},      // both negative
		{100, 5},      // far right
		{5, 100},      // far below
		{2, 5},        // x < X (just outside left)
		{5, 3},        // y < Y (just outside top)
	} {
		if r.Contains(pt.x, pt.y) {
			t.Errorf("Contains(%d,%d) must be false for rect %+v", pt.x, pt.y, r)
		}
	}
}

func TestPlaceOverlayWithRectPositionMatchesRender(t *testing.T) {
	// The returned rect must land on the same cells as the rendered overlay,
	// otherwise mouse hit-testing would drift from the visible overlay.
	bg := makeBg(40, 12)
	overlay := strings.Join([]string{"----", "-XX-", "----"}, "\n")
	rendered, r, ok := PlaceOverlayWithRect(40, 12, bg, overlay, WithDim(false))
	if !ok {
		t.Fatal("expected ok=true")
	}
	// The first '-' of the overlay must appear at cell column r.X on line r.Y.
	lines := strings.Split(rendered, "\n")
	if r.Y >= len(lines) {
		t.Fatalf("rect Y %d out of range (%d lines)", r.Y, len(lines))
	}
	// Use rune slicing for cell-width correctness even though the overlay
	// here is ASCII: r.X and r.W are cell coordinates, not byte indices.
	rowRunes := []rune(ansi.Strip(lines[r.Y]))
	if r.X+r.W > len(rowRunes) {
		t.Fatalf("rect overshoots rendered row width: rect=%+v rowLen=%d", r, len(rowRunes))
	}
	got := string(rowRunes[r.X : r.X+r.W])
	if got != "----" {
		t.Fatalf("expected overlay first row '----' at X=%d of rendered line, got %q (full line %q)",
			r.X, got, string(rowRunes))
	}
}
