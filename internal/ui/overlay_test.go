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
