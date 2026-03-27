package highlight

import (
	"strings"
	"testing"

	"github.com/aohoyd/aku/internal/theme"
)

func TestFgPainter(t *testing.T) {
	p := FgPainter(theme.Color("#7E9CD8"))
	got := p.Paint("hello")
	want := "\x1b[38;2;126;156;216mhello\x1b[0m"
	if got != want {
		t.Errorf("FgPainter.Paint(\"hello\")\ngot  %q\nwant %q", got, want)
	}
}

func TestBoldFgPainter(t *testing.T) {
	p := BoldFgPainter(theme.Color("#FF5555"))
	got := p.Paint("bold")
	want := "\x1b[1;38;2;255;85;85mbold\x1b[0m"
	if got != want {
		t.Errorf("BoldFgPainter.Paint(\"bold\")\ngot  %q\nwant %q", got, want)
	}
}

func TestFaintFgPainter(t *testing.T) {
	p := FaintFgPainter(theme.Color("#727169"))
	got := p.Paint("dim")
	want := "\x1b[2;38;2;114;113;105mdim\x1b[0m"
	if got != want {
		t.Errorf("FaintFgPainter.Paint(\"dim\")\ngot  %q\nwant %q", got, want)
	}
}

func TestBgFgPainter(t *testing.T) {
	p := BgFgPainter(theme.Color("#FF9E3B"), theme.Color("#1F1F28"))
	got := p.Paint("hl")
	want := "\x1b[48;2;255;158;59;38;2;31;31;40mhl\x1b[0m"
	if got != want {
		t.Errorf("BgFgPainter.Paint(\"hl\")\ngot  %q\nwant %q", got, want)
	}
}

func TestEmptyPainter(t *testing.T) {
	var p Painter
	got := p.Paint("unchanged")
	if got != "unchanged" {
		t.Errorf("empty Painter.Paint should return input unchanged, got %q", got)
	}
}

func TestPainterWriteTo(t *testing.T) {
	p := FgPainter(theme.Color("#7E9CD8"))
	var sb strings.Builder
	p.WriteTo(&sb, "hello")
	got := sb.String()
	want := "\x1b[38;2;126;156;216mhello\x1b[0m"
	if got != want {
		t.Errorf("FgPainter.WriteTo(\"hello\")\ngot  %q\nwant %q", got, want)
	}
}

func TestEmptyPainterWriteTo(t *testing.T) {
	var p Painter
	var sb strings.Builder
	p.WriteTo(&sb, "raw")
	got := sb.String()
	if got != "raw" {
		t.Errorf("empty Painter.WriteTo should write input unchanged, got %q", got)
	}
}

func TestColorToSGR_ANSI256(t *testing.T) {
	// ANSI-256 index input
	got := colorToSGR("43", false)
	want := "38;5;43"
	if got != want {
		t.Errorf("colorToSGR(\"43\", false) = %q, want %q", got, want)
	}

	got = colorToSGR("43", true)
	want = "48;5;43"
	if got != want {
		t.Errorf("colorToSGR(\"43\", true) = %q, want %q", got, want)
	}
}

func TestColorToSGR_InvalidHex(t *testing.T) {
	// Invalid hex should fallback to red
	got := colorToSGR("#ZZZZZZ", false)
	want := "38;5;1"
	if got != want {
		t.Errorf("colorToSGR(\"#ZZZZZZ\", false) = %q, want %q", got, want)
	}
}

func TestPainterPrefix(t *testing.T) {
	p := FgPainter(theme.Color("#7E9CD8"))
	if p.Prefix() != "\x1b[38;2;126;156;216m" {
		t.Errorf("Prefix() = %q, want %q", p.Prefix(), "\x1b[38;2;126;156;216m")
	}

	var empty Painter
	if empty.Prefix() != "" {
		t.Errorf("empty Painter.Prefix() = %q, want empty", empty.Prefix())
	}
}
