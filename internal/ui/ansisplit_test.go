package ui

import (
	"regexp"
	"strings"
	"testing"

	lipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// ---------- helpers ----------

// refCut returns segments using the existing ansi.Cut approach (O(N^2) reference).
func refCut(line string, vpWidth int) []string {
	w := ansi.StringWidth(line)
	var segs []string
	for offset := 0; offset < w; offset += vpWidth {
		end := offset + vpWidth
		if end > w {
			end = w
		}
		segs = append(segs, ansi.Cut(line, offset, end))
	}
	return segs
}

// ---------- plain ASCII tests ----------

func TestSplitWrappedVisible_PlainASCII(t *testing.T) {
	line := "abcdefghij" // 10 chars
	segs, widths := splitWrappedVisible(line, 5, 5, 0, 10)

	if len(segs) != 2 {
		t.Fatalf("expected 2 segments, got %d: %q", len(segs), segs)
	}
	if segs[0] != "abcde" {
		t.Errorf("seg[0] = %q, want %q", segs[0], "abcde")
	}
	if segs[1] != "fghij" {
		t.Errorf("seg[1] = %q, want %q", segs[1], "fghij")
	}
	if widths[0] != 5 || widths[1] != 5 {
		t.Errorf("widths = %v, want [5, 5]", widths)
	}
}

func TestSplitWrappedVisible_ExactBoundary(t *testing.T) {
	line := "abcde" // exactly vpWidth
	segs, widths := splitWrappedVisible(line, 5, 5, 0, 10)

	if len(segs) != 1 {
		t.Fatalf("expected 1 segment, got %d: %q", len(segs), segs)
	}
	if segs[0] != "abcde" {
		t.Errorf("seg[0] = %q, want %q", segs[0], "abcde")
	}
	if widths[0] != 5 {
		t.Errorf("widths[0] = %d, want 5", widths[0])
	}
}

func TestSplitWrappedVisible_PartialLastRow(t *testing.T) {
	line := "abcdefgh" // 8 chars, vpWidth=5 => rows: "abcde", "fgh"
	segs, widths := splitWrappedVisible(line, 5, 5, 0, 10)

	if len(segs) != 2 {
		t.Fatalf("expected 2 segments, got %d: %q", len(segs), segs)
	}
	if segs[0] != "abcde" {
		t.Errorf("seg[0] = %q, want %q", segs[0], "abcde")
	}
	if segs[1] != "fgh" {
		t.Errorf("seg[1] = %q, want %q", segs[1], "fgh")
	}
	if widths[1] != 3 {
		t.Errorf("widths[1] = %d, want 3", widths[1])
	}
}

// ---------- ANSI color spanning chunk boundaries ----------

func TestSplitWrappedVisible_ANSIColorAcrossChunks(t *testing.T) {
	// Red text that spans two chunks
	red := "\x1b[31m"
	reset := "\x1b[m"
	line := red + "abcdefghij" + reset // 10 visible chars
	segs, widths := splitWrappedVisible(line, 5, 5, 0, 10)

	if len(segs) != 2 {
		t.Fatalf("expected 2 segments, got %d: %q", len(segs), segs)
	}

	// First segment: starts with red, ends with reset
	if !strings.HasPrefix(segs[0], red) {
		t.Errorf("seg[0] should start with red SGR: %q", segs[0])
	}
	if !strings.HasSuffix(segs[0], reset) {
		t.Errorf("seg[0] should end with reset: %q", segs[0])
	}

	// Second segment: should also start with red (SGR carried)
	if !strings.HasPrefix(segs[1], red) {
		t.Errorf("seg[1] should start with red SGR (carry-over): %q", segs[1])
	}
	if !strings.HasSuffix(segs[1], reset) {
		t.Errorf("seg[1] should end with reset: %q", segs[1])
	}

	// Visible text must match
	if ansi.Strip(segs[0]) != "abcde" {
		t.Errorf("visible seg[0] = %q, want %q", ansi.Strip(segs[0]), "abcde")
	}
	if ansi.Strip(segs[1]) != "fghij" {
		t.Errorf("visible seg[1] = %q, want %q", ansi.Strip(segs[1]), "fghij")
	}

	if widths[0] != 5 || widths[1] != 5 {
		t.Errorf("widths = %v, want [5, 5]", widths)
	}
}

func TestSplitWrappedVisible_TrueColor(t *testing.T) {
	// True color (24-bit) SGR
	fg := "\x1b[38;2;255;128;0m"
	reset := "\x1b[m"
	line := fg + "abcdefghij" + reset
	segs, _ := splitWrappedVisible(line, 5, 5, 0, 10)

	if len(segs) != 2 {
		t.Fatalf("expected 2 segments, got %d", len(segs))
	}

	// Second segment must carry the true color
	if !strings.HasPrefix(segs[1], fg) {
		t.Errorf("seg[1] should carry true color: %q", segs[1])
	}
	if ansi.Strip(segs[1]) != "fghij" {
		t.Errorf("visible seg[1] = %q, want %q", ansi.Strip(segs[1]), "fghij")
	}
}

func TestSplitWrappedVisible_256Color(t *testing.T) {
	fg := "\x1b[38;5;196m"
	reset := "\x1b[m"
	line := fg + "abcdefghij" + reset
	segs, _ := splitWrappedVisible(line, 5, 5, 0, 10)

	if len(segs) != 2 {
		t.Fatalf("expected 2 segments, got %d", len(segs))
	}
	if !strings.HasPrefix(segs[1], fg) {
		t.Errorf("seg[1] should carry 256-color: %q", segs[1])
	}
}

func TestSplitWrappedVisible_BoldAndColor(t *testing.T) {
	bold := "\x1b[1m"
	red := "\x1b[31m"
	reset := "\x1b[m"
	line := bold + red + "abcdefghij" + reset
	segs, _ := splitWrappedVisible(line, 5, 5, 0, 10)

	if len(segs) != 2 {
		t.Fatalf("expected 2 segments, got %d", len(segs))
	}

	// Second segment must carry both bold and red
	if !strings.Contains(segs[1], bold) || !strings.Contains(segs[1], red) {
		t.Errorf("seg[1] should carry bold+red: %q", segs[1])
	}
}

// ---------- SGR reset in the middle ----------

func TestSplitWrappedVisible_SGRResetMiddle(t *testing.T) {
	red := "\x1b[31m"
	reset := "\x1b[0m"
	blue := "\x1b[34m"
	// "abc" in red, then reset, then "defghij" in blue
	// vpWidth=5: row0 = "abc" + reset + "de" (blue starts), row1 = "fghij"
	line := red + "abc" + reset + blue + "defghij"
	segs, _ := splitWrappedVisible(line, 5, 5, 0, 10)

	if len(segs) != 2 {
		t.Fatalf("expected 2 segments, got %d: %q", len(segs), segs)
	}

	// First segment starts with red
	if !strings.HasPrefix(segs[0], red) {
		t.Errorf("seg[0] should start with red: %q", segs[0])
	}

	// Second segment should carry only blue (red was reset)
	if !strings.HasPrefix(segs[1], blue) {
		t.Errorf("seg[1] should start with blue (not red): %q", segs[1])
	}
	if strings.Contains(segs[1], red) {
		t.Errorf("seg[1] should NOT contain red (was reset): %q", segs[1])
	}

	if ansi.Strip(segs[0]) != "abcde" {
		t.Errorf("visible seg[0] = %q, want %q", ansi.Strip(segs[0]), "abcde")
	}
	if ansi.Strip(segs[1]) != "fghij" {
		t.Errorf("visible seg[1] = %q, want %q", ansi.Strip(segs[1]), "fghij")
	}
}

// ---------- Multi-byte graphemes (emoji, CJK) ----------

func TestSplitWrappedVisible_CJKAtBoundary(t *testing.T) {
	// CJK characters are width 2. vpWidth=5: "aa" (2) + CJK (2) = 4, next CJK won't fit (4+2>5).
	// So row 0 = "aa\u4e16" (width 4), row 1 = "\u4e16\u4e16" (width 4), row 2 = "\u4e16" (width 2)
	line := "aa\u4e16\u4e16\u4e16\u4e16" // "aa世世世世" width = 2 + 2*4 = 10
	segs, widths := splitWrappedVisible(line, 5, 5, 0, 10)

	// Row0: "aa世" (w=4), row1: "世世" (w=4), row2: "世" (w=2)
	if len(segs) != 3 {
		t.Fatalf("expected 3 segments, got %d: %q", len(segs), segs)
	}
	if segs[0] != "aa\u4e16" {
		t.Errorf("seg[0] = %q, want %q", segs[0], "aa\u4e16")
	}
	if widths[0] != 4 {
		t.Errorf("widths[0] = %d, want 4", widths[0])
	}
	if segs[1] != "\u4e16\u4e16" {
		t.Errorf("seg[1] = %q, want %q", segs[1], "\u4e16\u4e16")
	}
	if widths[1] != 4 {
		t.Errorf("widths[1] = %d, want 4", widths[1])
	}
}

func TestSplitWrappedVisible_EmojiAtBoundary(t *testing.T) {
	// Each emoji typically has width 2.
	// vpWidth=3: "a" (1) + emoji (2) = 3, fills exactly.
	emoji := "😀"
	line := "a" + emoji + "b" + emoji // visible width = 1+2+1+2 = 6
	segs, widths := splitWrappedVisible(line, 3, 3, 0, 10)

	// Row0: "a😀" (w=3), row1: "b😀" (w=3)
	if len(segs) != 2 {
		t.Fatalf("expected 2 segments, got %d: %q", len(segs), segs)
	}
	if segs[0] != "a"+emoji {
		t.Errorf("seg[0] = %q, want %q", segs[0], "a"+emoji)
	}
	if widths[0] != 3 {
		t.Errorf("widths[0] = %d, want 3", widths[0])
	}
	if segs[1] != "b"+emoji {
		t.Errorf("seg[1] = %q, want %q", segs[1], "b"+emoji)
	}
}

func TestSplitWrappedVisible_WideCharForcesBreak(t *testing.T) {
	// vpWidth=3: "ab" (w=2), then CJK (w=2) -> 2+2=4 > 3, so break before CJK.
	// Row0: "ab" (w=2), row1: "世c" (w=3)
	line := "ab\u4e16c" // width = 2 + 2 + 1 = 5
	segs, widths := splitWrappedVisible(line, 3, 3, 0, 10)

	if len(segs) != 2 {
		t.Fatalf("expected 2 segments, got %d: %q", len(segs), segs)
	}
	if segs[0] != "ab" {
		t.Errorf("seg[0] = %q, want %q", segs[0], "ab")
	}
	if widths[0] != 2 {
		t.Errorf("widths[0] = %d, want 2", widths[0])
	}
	if segs[1] != "\u4e16c" {
		t.Errorf("seg[1] = %q, want %q", segs[1], "\u4e16c")
	}
	if widths[1] != 3 {
		t.Errorf("widths[1] = %d, want 3", widths[1])
	}
}

func TestSplitWrappedVisible_WideCharNarrowVP(t *testing.T) {
	// vpWidth=1: wide chars (width=2) exceed vpWidth even at col=0.
	// They should still be placed on the current row, not emit empty segments.
	line := "a\u4e16b" // width = 1 + 2 + 1 = 4
	segs, widths := splitWrappedVisible(line, 1, 1, 0, 10)

	// Expect: "a" (w=1), "世" (w=2, overflow OK), "b" (w=1)
	if len(segs) != 3 {
		t.Fatalf("expected 3 segments, got %d: %q", len(segs), segs)
	}
	for i, w := range widths {
		if w == 0 {
			t.Errorf("widths[%d] = 0, spurious empty segment: %q", i, segs[i])
		}
	}
	if ansi.Strip(segs[0]) != "a" {
		t.Errorf("seg[0] stripped = %q, want %q", ansi.Strip(segs[0]), "a")
	}
	if ansi.Strip(segs[1]) != "\u4e16" {
		t.Errorf("seg[1] stripped = %q, want %q", ansi.Strip(segs[1]), "\u4e16")
	}
	if ansi.Strip(segs[2]) != "b" {
		t.Errorf("seg[2] stripped = %q, want %q", ansi.Strip(segs[2]), "b")
	}
}

// ---------- Edge cases ----------

func TestSplitWrappedVisible_EmptyString(t *testing.T) {
	segs, widths := splitWrappedVisible("", 5, 5, 0, 10)
	if segs != nil || widths != nil {
		t.Errorf("expected nil for empty string, got segs=%v, widths=%v", segs, widths)
	}
}

func TestSplitWrappedVisible_ShorterThanWidth(t *testing.T) {
	line := "abc"
	segs, widths := splitWrappedVisible(line, 10, 10, 0, 10)

	if len(segs) != 1 {
		t.Fatalf("expected 1 segment, got %d", len(segs))
	}
	if segs[0] != "abc" {
		t.Errorf("seg[0] = %q, want %q", segs[0], "abc")
	}
	if widths[0] != 3 {
		t.Errorf("widths[0] = %d, want 3", widths[0])
	}
}

func TestSplitWrappedVisible_StartRowBeyondTotal(t *testing.T) {
	line := "abcde" // 1 row at vpWidth=5
	segs, widths := splitWrappedVisible(line, 5, 5, 5, 10)

	if len(segs) != 0 {
		t.Errorf("expected 0 segments for startRow beyond total, got %d: %q", len(segs), segs)
	}
	if len(widths) != 0 {
		t.Errorf("expected 0 widths, got %v", widths)
	}
}

func TestSplitWrappedVisible_NumRowsZero(t *testing.T) {
	segs, widths := splitWrappedVisible("abcde", 5, 5, 0, 0)
	if segs != nil || widths != nil {
		t.Errorf("expected nil for numRows=0, got segs=%v, widths=%v", segs, widths)
	}
}

func TestSplitWrappedVisible_WindowMiddle(t *testing.T) {
	// 20 chars, vpWidth=5 => 4 rows: "abcde", "fghij", "klmno", "pqrst"
	// Request rows 1..2 (startRow=1, numRows=2)
	line := "abcdefghijklmnopqrst"
	segs, widths := splitWrappedVisible(line, 5, 5, 1, 2)

	if len(segs) != 2 {
		t.Fatalf("expected 2 segments, got %d: %q", len(segs), segs)
	}
	if segs[0] != "fghij" {
		t.Errorf("seg[0] = %q, want %q", segs[0], "fghij")
	}
	if segs[1] != "klmno" {
		t.Errorf("seg[1] = %q, want %q", segs[1], "klmno")
	}
	if widths[0] != 5 || widths[1] != 5 {
		t.Errorf("widths = %v, want [5, 5]", widths)
	}
}

func TestSplitWrappedVisible_WindowLastRow(t *testing.T) {
	// 13 chars, vpWidth=5 => 3 rows: "abcde", "fghij", "klm"
	// Request last row only (startRow=2, numRows=1)
	line := "abcdefghijklm"
	segs, widths := splitWrappedVisible(line, 5, 5, 2, 1)

	if len(segs) != 1 {
		t.Fatalf("expected 1 segment, got %d: %q", len(segs), segs)
	}
	if segs[0] != "klm" {
		t.Errorf("seg[0] = %q, want %q", segs[0], "klm")
	}
	if widths[0] != 3 {
		t.Errorf("widths[0] = %d, want 3", widths[0])
	}
}

// ---------- Correctness comparison: splitWrappedVisible vs ansi.Cut ----------

func TestSplitWrappedVisible_MatchesCut_PlainASCII(t *testing.T) {
	lines := []string{
		"abcdefghijklmnopqrstuvwxyz",
		"short",
		"exactly10!",
		"a",
		strings.Repeat("x", 100),
	}

	for _, line := range lines {
		for _, vpWidth := range []int{3, 5, 10, 20} {
			ref := refCut(line, vpWidth)
			segs, _ := splitWrappedVisible(line, vpWidth, vpWidth, 0, len(ref)+1)

			if len(segs) != len(ref) {
				t.Errorf("line=%q vpWidth=%d: got %d segments, want %d",
					line, vpWidth, len(segs), len(ref))
				continue
			}

			for i := range ref {
				got := ansi.Strip(segs[i])
				want := ansi.Strip(ref[i])
				if got != want {
					t.Errorf("line=%q vpWidth=%d seg[%d]: got %q, want %q",
						line, vpWidth, i, got, want)
				}
			}
		}
	}
}

func TestSplitWrappedVisible_MatchesCut_ANSI(t *testing.T) {
	red := "\x1b[31m"
	bold := "\x1b[1m"
	trueColor := "\x1b[38;2;100;200;50m"
	color256 := "\x1b[38;5;196m"
	reset := "\x1b[m"

	lines := []string{
		red + "abcdefghijklmno" + reset,
		bold + red + "helloworldfoo" + reset,
		trueColor + strings.Repeat("z", 30) + reset,
		color256 + "short" + reset + red + "tail" + reset,
		"plain" + red + "mid" + reset + "end",
	}

	for _, line := range lines {
		for _, vpWidth := range []int{3, 5, 8, 10} {
			ref := refCut(line, vpWidth)
			segs, _ := splitWrappedVisible(line, vpWidth, vpWidth, 0, len(ref)+1)

			if len(segs) != len(ref) {
				t.Errorf("vpWidth=%d: got %d segments, want %d for line %q",
					vpWidth, len(segs), len(ref), line)
				continue
			}

			for i := range ref {
				got := ansi.Strip(segs[i])
				want := ansi.Strip(ref[i])
				if got != want {
					t.Errorf("vpWidth=%d seg[%d]: got %q, want %q",
						vpWidth, i, got, want)
				}
			}
		}
	}
}

func TestSplitWrappedVisible_MatchesCut_Windowed(t *testing.T) {
	// Verify that windowed access produces the same segments as full access
	// but only the requested slice.
	red := "\x1b[31m"
	reset := "\x1b[m"
	line := red + strings.Repeat("x", 25) + reset // 25 chars, vpWidth=5 => 5 rows

	vpWidth := 5
	allSegs, _ := splitWrappedVisible(line, vpWidth, vpWidth, 0, 100)

	for startRow := 0; startRow < len(allSegs); startRow++ {
		for numRows := 1; numRows <= len(allSegs)-startRow; numRows++ {
			segs, _ := splitWrappedVisible(line, vpWidth, vpWidth, startRow, numRows)
			if len(segs) != numRows {
				t.Errorf("startRow=%d numRows=%d: got %d segs, want %d",
					startRow, numRows, len(segs), numRows)
				continue
			}
			for i := range segs {
				got := ansi.Strip(segs[i])
				want := ansi.Strip(allSegs[startRow+i])
				if got != want {
					t.Errorf("startRow=%d numRows=%d seg[%d]: got %q, want %q",
						startRow, numRows, i, got, want)
				}
			}
		}
	}
}

// ---------- Asymmetric vpWidth/contWidth (first row wider than continuations) ----------

func TestSplitWrappedVisible_AsymmetricWidths_PlainASCII(t *testing.T) {
	// vpWidth=10, contWidth=8 (simulates wrapIndicatorWidth=2).
	// "abcdefghijklmnopqr" = 18 chars.
	// Row 0: 10 chars "abcdefghij", Row 1: 8 chars "klmnopqr".
	line := "abcdefghijklmnopqr"
	segs, widths := splitWrappedVisible(line, 10, 8, 0, 10)

	if len(segs) != 2 {
		t.Fatalf("expected 2 segments, got %d: %q", len(segs), segs)
	}
	if segs[0] != "abcdefghij" {
		t.Errorf("seg[0] = %q, want %q", segs[0], "abcdefghij")
	}
	if widths[0] != 10 {
		t.Errorf("widths[0] = %d, want 10", widths[0])
	}
	if segs[1] != "klmnopqr" {
		t.Errorf("seg[1] = %q, want %q", segs[1], "klmnopqr")
	}
	if widths[1] != 8 {
		t.Errorf("widths[1] = %d, want 8", widths[1])
	}
}

func TestSplitWrappedVisible_AsymmetricWidths_ThreeRows(t *testing.T) {
	// vpWidth=10, contWidth=8.
	// "abcdefghijklmnopqrstuvwxyz" = 26 chars.
	// Row 0: 10 "abcdefghij", Row 1: 8 "klmnopqr", Row 2: 8 "stuvwxyz".
	line := "abcdefghijklmnopqrstuvwxyz"
	segs, widths := splitWrappedVisible(line, 10, 8, 0, 10)

	if len(segs) != 3 {
		t.Fatalf("expected 3 segments, got %d: %q", len(segs), segs)
	}
	if segs[0] != "abcdefghij" || widths[0] != 10 {
		t.Errorf("seg[0] = %q (w=%d), want %q (w=10)", segs[0], widths[0], "abcdefghij")
	}
	if segs[1] != "klmnopqr" || widths[1] != 8 {
		t.Errorf("seg[1] = %q (w=%d), want %q (w=8)", segs[1], widths[1], "klmnopqr")
	}
	if segs[2] != "stuvwxyz" || widths[2] != 8 {
		t.Errorf("seg[2] = %q (w=%d), want %q (w=8)", segs[2], widths[2], "stuvwxyz")
	}
}

func TestSplitWrappedVisible_AsymmetricWidths_PartialLastRow(t *testing.T) {
	// vpWidth=10, contWidth=8.
	// "abcdefghijklm" = 13 chars.
	// Row 0: 10 "abcdefghij", Row 1: 3 "klm".
	line := "abcdefghijklm"
	segs, widths := splitWrappedVisible(line, 10, 8, 0, 10)

	if len(segs) != 2 {
		t.Fatalf("expected 2 segments, got %d: %q", len(segs), segs)
	}
	if segs[0] != "abcdefghij" || widths[0] != 10 {
		t.Errorf("seg[0] = %q (w=%d), want %q (w=10)", segs[0], widths[0], "abcdefghij")
	}
	if segs[1] != "klm" || widths[1] != 3 {
		t.Errorf("seg[1] = %q (w=%d), want %q (w=3)", segs[1], widths[1], "klm")
	}
}

func TestSplitWrappedVisible_AsymmetricWidths_ANSI(t *testing.T) {
	// vpWidth=10, contWidth=8 with ANSI-colored text.
	red := "\x1b[31m"
	reset := "\x1b[m"
	line := red + "abcdefghijklmnopqr" + reset // 18 visible chars
	segs, widths := splitWrappedVisible(line, 10, 8, 0, 10)

	if len(segs) != 2 {
		t.Fatalf("expected 2 segments, got %d: %q", len(segs), segs)
	}
	if ansi.Strip(segs[0]) != "abcdefghij" || widths[0] != 10 {
		t.Errorf("seg[0] visible = %q (w=%d), want %q (w=10)", ansi.Strip(segs[0]), widths[0], "abcdefghij")
	}
	if ansi.Strip(segs[1]) != "klmnopqr" || widths[1] != 8 {
		t.Errorf("seg[1] visible = %q (w=%d), want %q (w=8)", ansi.Strip(segs[1]), widths[1], "klmnopqr")
	}
	// Second segment should carry the red SGR.
	if !strings.HasPrefix(segs[1], red) {
		t.Errorf("seg[1] should start with red SGR: %q", segs[1])
	}
}

func TestSplitWrappedVisible_AsymmetricWidths_Windowed(t *testing.T) {
	// vpWidth=10, contWidth=8.
	// 26 chars: row 0 (10), row 1 (8), row 2 (8).
	// Request only row 1.
	line := "abcdefghijklmnopqrstuvwxyz"
	segs, widths := splitWrappedVisible(line, 10, 8, 1, 1)

	if len(segs) != 1 {
		t.Fatalf("expected 1 segment, got %d: %q", len(segs), segs)
	}
	if segs[0] != "klmnopqr" || widths[0] != 8 {
		t.Errorf("seg[0] = %q (w=%d), want %q (w=8)", segs[0], widths[0], "klmnopqr")
	}
}

// ---------- No SGR prefix when no styles active ----------

func TestSplitWrappedVisible_NoSGR_NoPrefix(t *testing.T) {
	line := "abcdefghij"
	segs, _ := splitWrappedVisible(line, 5, 5, 0, 10)

	for i, seg := range segs {
		if strings.Contains(seg, "\x1b") {
			t.Errorf("seg[%d] = %q should not contain escape sequences for plain text", i, seg)
		}
	}
}

func TestSplitWrappedVisible_ResetClearsSGR(t *testing.T) {
	// Red text, then reset, then plain text spanning to next row.
	// The second row should NOT have any SGR prefix.
	red := "\x1b[31m"
	reset := "\x1b[m"
	line := red + "ab" + reset + "cdefgh" // visible: "abcdefgh", vpWidth=5
	segs, _ := splitWrappedVisible(line, 5, 5, 0, 10)

	if len(segs) != 2 {
		t.Fatalf("expected 2 segments, got %d: %q", len(segs), segs)
	}

	// Second segment: after reset, no SGR should be prepended
	if strings.HasPrefix(segs[1], "\x1b[") {
		t.Errorf("seg[1] should not start with SGR after reset: %q", segs[1])
	}
	if ansi.Strip(segs[1]) != "fgh" {
		t.Errorf("visible seg[1] = %q, want %q", ansi.Strip(segs[1]), "fgh")
	}
}

// ---------- Benchmarks ----------

// make20kANSILine builds a ~20k visible-character line with mixed ANSI colors.
func make20kANSILine() string {
	var b strings.Builder
	colors := []string{"\x1b[31m", "\x1b[32m", "\x1b[38;2;100;150;200m", "\x1b[1;33m"}
	for i := 0; b.Len() < 20000; i++ {
		b.WriteString(colors[i%len(colors)])
		for j := 0; j < 50 && b.Len() < 20000; j++ {
			b.WriteByte('a' + byte((i+j)%26))
		}
		b.WriteString("\x1b[m")
	}
	return b.String()
}

func BenchmarkSplitWrappedVisible_20kLine(b *testing.B) {
	line := make20kANSILine()
	vpWidth := 100
	startRow := 0
	numRows := 50

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		splitWrappedVisible(line, vpWidth, vpWidth, startRow, numRows)
	}
}

func BenchmarkSplitWrappedVisible_20kLine_MiddleScroll(b *testing.B) {
	line := make20kANSILine()
	vpWidth := 100
	startRow := 100
	numRows := 50

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		splitWrappedVisible(line, vpWidth, vpWidth, startRow, numRows)
	}
}

// ========== injectHighlights tests ==========

// ---------- styleToSGR ----------

func TestStyleToSGR_Basic(t *testing.T) {
	s := lipgloss.NewStyle().Foreground(lipgloss.Color("1")) // red
	sgr := styleToSGR(s)
	if sgr == "" {
		t.Fatal("styleToSGR returned empty for a styled style")
	}
	// The SGR should start with \x1b[ and end with m
	if !strings.HasPrefix(sgr, "\x1b[") {
		t.Errorf("SGR should start with ESC[: %q", sgr)
	}
	// Should not contain the dummy "X"
	if strings.Contains(sgr, "X") {
		t.Errorf("SGR should not contain dummy char: %q", sgr)
	}
}

func TestStyleToSGR_NoStyle(t *testing.T) {
	s := lipgloss.NewStyle()
	sgr := styleToSGR(s)
	// An unstyled style may or may not produce SGR; just ensure no panic.
	_ = sgr
}

// ---------- injectHighlights: plain ASCII, single match ----------

func TestInjectHighlights_PlainSingleMatch(t *testing.T) {
	line := "hello world"
	hiSGR := "\x1b[43m"  // yellow bg
	selSGR := "\x1b[42m" // green bg
	matches := []highlightRange{{start: 6, end: 11}} // "world"

	result := injectHighlights(line, matches, -1, hiSGR, selSGR)

	// Visible text must be preserved.
	if ansi.Strip(result) != "hello world" {
		t.Errorf("stripped = %q, want %q", ansi.Strip(result), "hello world")
	}

	// Must contain the highlight SGR.
	if !strings.Contains(result, hiSGR) {
		t.Errorf("result should contain hiSGR %q: %q", hiSGR, result)
	}

	// Must NOT contain selSGR (selectedIdx=-1).
	if strings.Contains(result, selSGR) {
		t.Errorf("result should not contain selSGR %q: %q", selSGR, result)
	}
}

// ---------- injectHighlights: ANSI-colored text, match spanning color changes ----------

func TestInjectHighlights_ANSIMatchSpanningColorChange(t *testing.T) {
	red := "\x1b[31m"
	blue := "\x1b[34m"
	reset := "\x1b[m"
	// "abc" in red, "def" in blue => visible "abcdef"
	line := red + "abc" + reset + blue + "def" + reset
	hiSGR := "\x1b[43m"
	selSGR := "\x1b[42m"
	// Match spans columns 1..5 ("bcde"), crossing the red->blue boundary.
	matches := []highlightRange{{start: 1, end: 5}}

	result := injectHighlights(line, matches, 0, hiSGR, selSGR)

	// Visible text must be preserved.
	if ansi.Strip(result) != "abcdef" {
		t.Errorf("stripped = %q, want %q", ansi.Strip(result), "abcdef")
	}

	// The selected match should use selSGR (selectedIdx=0).
	if !strings.Contains(result, selSGR) {
		t.Errorf("result should contain selSGR %q: %q", selSGR, result)
	}

	// After the match ends at col 5, the original blue styling should be restored.
	// Check that blue appears after the match region.
	afterMatch := result[strings.LastIndex(result, "e")+1:]
	if !strings.Contains(afterMatch, blue) {
		t.Errorf("blue should be restored after match: after=%q, full=%q", afterMatch, result)
	}
}

// ---------- injectHighlights: multiple matches on one line ----------

func TestInjectHighlights_MultipleMatches(t *testing.T) {
	line := "foo bar baz qux"
	hiSGR := "\x1b[43m"
	selSGR := "\x1b[42m"
	// Match "bar" (4..7) and "qux" (12..15)
	matches := []highlightRange{
		{start: 4, end: 7},
		{start: 12, end: 15},
	}

	result := injectHighlights(line, matches, 1, hiSGR, selSGR)

	if ansi.Strip(result) != "foo bar baz qux" {
		t.Errorf("stripped = %q, want %q", ansi.Strip(result), "foo bar baz qux")
	}

	// First match should use hiSGR (not selected).
	// Second match should use selSGR (selectedIdx=1).
	// Count occurrences.
	hiCount := strings.Count(result, hiSGR)
	selCount := strings.Count(result, selSGR)
	if hiCount < 1 {
		t.Errorf("expected at least 1 hiSGR occurrence, got %d: %q", hiCount, result)
	}
	if selCount < 1 {
		t.Errorf("expected at least 1 selSGR occurrence, got %d: %q", selCount, result)
	}
}

// ---------- injectHighlights: selected match vs non-selected ----------

func TestInjectHighlights_SelectedVsNonSelected(t *testing.T) {
	line := "aaa bbb ccc"
	hiSGR := "\x1b[43m"
	selSGR := "\x1b[42m"
	matches := []highlightRange{
		{start: 0, end: 3},
		{start: 4, end: 7},
		{start: 8, end: 11},
	}

	// Select middle match (index 1).
	result := injectHighlights(line, matches, 1, hiSGR, selSGR)

	if ansi.Strip(result) != "aaa bbb ccc" {
		t.Errorf("stripped = %q, want %q", ansi.Strip(result), "aaa bbb ccc")
	}

	// selSGR should appear exactly once (for match index 1).
	if strings.Count(result, selSGR) != 1 {
		t.Errorf("expected exactly 1 selSGR, got %d: %q", strings.Count(result, selSGR), result)
	}
	// hiSGR should appear twice (match 0 and match 2).
	if strings.Count(result, hiSGR) != 2 {
		t.Errorf("expected exactly 2 hiSGR, got %d: %q", strings.Count(result, hiSGR), result)
	}
}

// ---------- injectHighlights: match at column 0 ----------

func TestInjectHighlights_MatchAtColumn0(t *testing.T) {
	line := "hello"
	hiSGR := "\x1b[43m"
	selSGR := "\x1b[42m"
	matches := []highlightRange{{start: 0, end: 3}} // "hel"

	result := injectHighlights(line, matches, 0, hiSGR, selSGR)

	if ansi.Strip(result) != "hello" {
		t.Errorf("stripped = %q, want %q", ansi.Strip(result), "hello")
	}

	// Should start with reset + selSGR (since selectedIdx=0).
	if !strings.HasPrefix(result, "\x1b[m"+selSGR) {
		t.Errorf("result should start with reset+selSGR: %q", result)
	}
}

// ---------- injectHighlights: match at end of line ----------

func TestInjectHighlights_MatchAtEnd(t *testing.T) {
	line := "hello"
	hiSGR := "\x1b[43m"
	selSGR := "\x1b[42m"
	matches := []highlightRange{{start: 3, end: 5}} // "lo"

	result := injectHighlights(line, matches, -1, hiSGR, selSGR)

	if ansi.Strip(result) != "hello" {
		t.Errorf("stripped = %q, want %q", ansi.Strip(result), "hello")
	}

	// Should end with a reset after the match.
	if !strings.HasSuffix(result, "\x1b[m") {
		t.Errorf("result should end with reset: %q", result)
	}
}

// ---------- injectHighlights: SGR reset within a match span ----------

func TestInjectHighlights_SGRResetWithinMatch(t *testing.T) {
	red := "\x1b[31m"
	reset := "\x1b[m"
	blue := "\x1b[34m"
	// "abc" in red, reset, "def" in blue => visible "abcdef"
	line := red + "abc" + reset + blue + "def" + reset
	hiSGR := "\x1b[43m"
	selSGR := "\x1b[42m"

	// Match starts at col 2, ends at col 5 — spans across the red->blue boundary.
	// The red/reset/blue SGR sequences within the match should be suppressed.
	matches := []highlightRange{{start: 2, end: 5}}

	result := injectHighlights(line, matches, -1, hiSGR, selSGR)

	if ansi.Strip(result) != "abcdef" {
		t.Errorf("stripped = %q, want %q", ansi.Strip(result), "abcdef")
	}

	// hiSGR must be present for the match.
	if !strings.Contains(result, hiSGR) {
		t.Errorf("result should contain hiSGR: %q", result)
	}

	// Within the matched region ("cde"), the original red and blue SGRs should
	// be suppressed so the highlight remains continuous.
	// Find the match region: it starts after \x1b[m + hiSGR before "c" and ends
	// after "e" with \x1b[m. Check that between hiSGR and the closing reset,
	// there are no mid-match SGR overrides.
	hiIdx := strings.Index(result, hiSGR)
	if hiIdx < 0 {
		t.Fatalf("hiSGR not found in result: %q", result)
	}
	matchRegion := result[hiIdx+len(hiSGR):]
	// matchRegion starts with the highlighted characters.
	// Find the reset that closes the match (should come after "cde").
	closeReset := strings.Index(matchRegion, "\x1b[m")
	if closeReset < 0 {
		t.Fatalf("no closing reset found after hiSGR: %q", result)
	}
	withinMatch := matchRegion[:closeReset]
	// Within the match, original color SGRs (red, blue) should NOT appear.
	if strings.Contains(withinMatch, red) {
		t.Errorf("red SGR should be suppressed within match: within=%q, full=%q", withinMatch, result)
	}
	if strings.Contains(withinMatch, blue) {
		t.Errorf("blue SGR should be suppressed within match: within=%q, full=%q", withinMatch, result)
	}

	// After the match, the blue color should be restored (since sgrBuf tracked
	// the reset+blue that occurred within the match).
	afterMatch := result[hiIdx+len(hiSGR)+closeReset:]
	if !strings.Contains(afterMatch, blue) {
		t.Errorf("blue should be restored after match: after=%q, full=%q", afterMatch, result)
	}

	// Result should end with a reset (from the original trailing reset).
	if !strings.HasSuffix(result, "\x1b[m") {
		t.Errorf("result should end with reset: %q", result)
	}
}

// ---------- injectHighlights: empty line and no matches (fast path) ----------

func TestInjectHighlights_EmptyLine(t *testing.T) {
	result := injectHighlights("", nil, -1, "\x1b[43m", "\x1b[42m")
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestInjectHighlights_NoMatches(t *testing.T) {
	line := "\x1b[31mhello\x1b[m"
	result := injectHighlights(line, nil, -1, "\x1b[43m", "\x1b[42m")
	if result != line {
		t.Errorf("no matches should return line unchanged: got %q, want %q", result, line)
	}

	result2 := injectHighlights(line, []highlightRange{}, -1, "\x1b[43m", "\x1b[42m")
	if result2 != line {
		t.Errorf("empty matches should return line unchanged: got %q, want %q", result2, line)
	}
}

// ---------- injectHighlights: correctness comparison with lipgloss.StyleRanges ----------

func TestInjectHighlights_MatchesStyleRanges_PlainASCII(t *testing.T) {
	hiStyle := lipgloss.NewStyle().Background(lipgloss.Color("3"))   // yellow bg
	selStyle := lipgloss.NewStyle().Background(lipgloss.Color("2"))  // green bg
	hiSGR := styleToSGR(hiStyle)
	selSGR := styleToSGR(selStyle)

	tests := []struct {
		name    string
		line    string
		matches []highlightRange
		selIdx  int
	}{
		{
			name:    "single match middle",
			line:    "hello world foo",
			matches: []highlightRange{{start: 6, end: 11}},
			selIdx:  -1,
		},
		{
			name:    "match at start",
			line:    "abcdefghij",
			matches: []highlightRange{{start: 0, end: 5}},
			selIdx:  0,
		},
		{
			name:    "match at end",
			line:    "abcdefghij",
			matches: []highlightRange{{start: 7, end: 10}},
			selIdx:  -1,
		},
		{
			name:    "two matches",
			line:    "foo bar baz qux",
			matches: []highlightRange{{start: 4, end: 7}, {start: 12, end: 15}},
			selIdx:  1,
		},
		{
			name:    "full line match",
			line:    "hello",
			matches: []highlightRange{{start: 0, end: 5}},
			selIdx:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := injectHighlights(tt.line, tt.matches, tt.selIdx, hiSGR, selSGR)

			// Build equivalent lipgloss.StyleRanges call.
			var ranges []lipgloss.Range
			for i, m := range tt.matches {
				style := hiStyle
				if i == tt.selIdx {
					style = selStyle
				}
				ranges = append(ranges, lipgloss.NewRange(m.start, m.end, style))
			}
			want := lipgloss.StyleRanges(tt.line, ranges...)

			// Compare stripped text (must be identical).
			gotStripped := ansi.Strip(got)
			wantStripped := ansi.Strip(want)
			if gotStripped != wantStripped {
				t.Errorf("stripped mismatch:\n  got:  %q\n  want: %q", gotStripped, wantStripped)
			}
		})
	}
}

func TestInjectHighlights_MatchesStyleRanges_ANSIColored(t *testing.T) {
	hiStyle := lipgloss.NewStyle().Background(lipgloss.Color("3"))
	selStyle := lipgloss.NewStyle().Background(lipgloss.Color("2"))
	hiSGR := styleToSGR(hiStyle)
	selSGR := styleToSGR(selStyle)

	red := "\x1b[31m"
	blue := "\x1b[34m"
	reset := "\x1b[m"

	tests := []struct {
		name    string
		line    string
		matches []highlightRange
		selIdx  int
	}{
		{
			name:    "colored text single match",
			line:    red + "hello world" + reset,
			matches: []highlightRange{{start: 6, end: 11}},
			selIdx:  -1,
		},
		{
			name:    "color change within match",
			line:    red + "abc" + reset + blue + "def" + reset,
			matches: []highlightRange{{start: 1, end: 5}},
			selIdx:  0,
		},
		{
			name:    "match at start of colored",
			line:    blue + "foobar" + reset,
			matches: []highlightRange{{start: 0, end: 3}},
			selIdx:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use the stripped version as input to StyleRanges since it works on
			// display columns of the stripped text.
			stripped := ansi.Strip(tt.line)

			var ranges []lipgloss.Range
			for i, m := range tt.matches {
				style := hiStyle
				if i == tt.selIdx {
					style = selStyle
				}
				ranges = append(ranges, lipgloss.NewRange(m.start, m.end, style))
			}
			ref := lipgloss.StyleRanges(stripped, ranges...)

			got := injectHighlights(tt.line, tt.matches, tt.selIdx, hiSGR, selSGR)

			// Stripped text should match.
			gotStripped := ansi.Strip(got)
			refStripped := ansi.Strip(ref)
			if gotStripped != refStripped {
				t.Errorf("stripped mismatch:\n  got:  %q\n  want: %q", gotStripped, refStripped)
			}
		})
	}
}

// ---------- injectHighlights: match covering entire colored line ----------

func TestInjectHighlights_FullLineColored(t *testing.T) {
	red := "\x1b[31m"
	reset := "\x1b[m"
	line := red + "hello" + reset
	hiSGR := "\x1b[43m"
	selSGR := "\x1b[42m"
	matches := []highlightRange{{start: 0, end: 5}}

	result := injectHighlights(line, matches, -1, hiSGR, selSGR)

	if ansi.Strip(result) != "hello" {
		t.Errorf("stripped = %q, want %q", ansi.Strip(result), "hello")
	}

	// Red should be suppressed within the match.
	// The hiSGR should be present.
	if !strings.Contains(result, hiSGR) {
		t.Errorf("result should contain hiSGR: %q", result)
	}
}

// ---------- injectHighlights: adjacent matches ----------

func TestInjectHighlights_AdjacentMatches(t *testing.T) {
	line := "abcdef"
	hiSGR := "\x1b[43m"
	selSGR := "\x1b[42m"
	// Two adjacent matches: "abc" (0..3) and "def" (3..6)
	matches := []highlightRange{
		{start: 0, end: 3},
		{start: 3, end: 6},
	}

	result := injectHighlights(line, matches, 1, hiSGR, selSGR)

	if ansi.Strip(result) != "abcdef" {
		t.Errorf("stripped = %q, want %q", ansi.Strip(result), "abcdef")
	}

	// Should contain both hiSGR and selSGR.
	if !strings.Contains(result, hiSGR) {
		t.Errorf("result should contain hiSGR: %q", result)
	}
	if !strings.Contains(result, selSGR) {
		t.Errorf("result should contain selSGR: %q", result)
	}
}

func BenchmarkAnsiCutLoop_20kLine(b *testing.B) {
	line := make20kANSILine()
	vpWidth := 100
	totalWidth := ansi.StringWidth(line)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		for offset := 0; offset < totalWidth; offset += vpWidth {
			end := offset + vpWidth
			if end > totalWidth {
				end = totalWidth
			}
			ansi.Cut(line, offset, end)
		}
	}
}

// makeHighlightRanges creates n evenly-spaced 5-char highlight ranges across
// visibleWidth display columns.
func makeHighlightRanges(n, visibleWidth int) []highlightRange {
	if n == 0 {
		return nil
	}
	matches := make([]highlightRange, n)
	spacing := visibleWidth / n
	for i := range n {
		start := i * spacing
		end := start + 5
		if end > visibleWidth {
			end = visibleWidth
		}
		matches[i] = highlightRange{start: start, end: end}
	}
	return matches
}

// ---------- BenchmarkInjectHighlights_20kLine ----------

func BenchmarkInjectHighlights_20kLine(b *testing.B) {
	line := make20kANSILine()
	visibleWidth := ansi.StringWidth(line)
	matches := makeHighlightRanges(10, visibleWidth)

	hiSGR := "\x1b[43m"  // yellow bg
	selSGR := "\x1b[42m" // green bg

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		injectHighlights(line, matches, 0, hiSGR, selSGR)
	}
}

func BenchmarkStyleRanges_20kLine(b *testing.B) {
	line := make20kANSILine()
	visibleWidth := ansi.StringWidth(line)
	matchSpecs := makeHighlightRanges(10, visibleWidth)

	hiStyle := lipgloss.NewStyle().Background(lipgloss.Color("3"))

	ranges := make([]lipgloss.Range, len(matchSpecs))
	for i, m := range matchSpecs {
		ranges[i] = lipgloss.NewRange(m.start, m.end, hiStyle)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		lipgloss.StyleRanges(line, ranges...)
	}
}

// ---------- BenchmarkInjectHighlights_20kLine_ManyMatches ----------

func BenchmarkInjectHighlights_20kLine_ManyMatches(b *testing.B) {
	line := make20kANSILine()
	visibleWidth := ansi.StringWidth(line)
	matches := makeHighlightRanges(100, visibleWidth)

	hiSGR := "\x1b[43m"
	selSGR := "\x1b[42m"

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		injectHighlights(line, matches, 50, hiSGR, selSGR)
	}
}

func BenchmarkStyleRanges_20kLine_ManyMatches(b *testing.B) {
	line := make20kANSILine()
	visibleWidth := ansi.StringWidth(line)
	matchSpecs := makeHighlightRanges(100, visibleWidth)

	hiStyle := lipgloss.NewStyle().Background(lipgloss.Color("3"))

	ranges := make([]lipgloss.Range, len(matchSpecs))
	for i, m := range matchSpecs {
		ranges[i] = lipgloss.NewRange(m.start, m.end, hiStyle)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		lipgloss.StyleRanges(line, ranges...)
	}
}

// ---------- BenchmarkRebuildMatchPositions_PerLine ----------

func BenchmarkRebuildMatchPositions_PerLine(b *testing.B) {
	// Build 1000 lines of ~100 chars each.
	lines := make([]string, 1000)
	for i := range lines {
		var sb strings.Builder
		for j := 0; j < 100; j++ {
			sb.WriteByte('a' + byte((i+j)%26))
		}
		lines[i] = sb.String()
	}
	re := regexp.MustCompile(`abc`)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		var positions []matchPosition
		for lineIdx, line := range lines {
			positions = append(positions, computeLineMatchPositions(line, re, lineIdx)...)
		}
		_ = positions
	}
}

func BenchmarkRebuildMatchPositions_JoinAll(b *testing.B) {
	// Same 1000 lines, old join-all approach for comparison.
	lines := make([]string, 1000)
	for i := range lines {
		var sb strings.Builder
		for j := 0; j < 100; j++ {
			sb.WriteByte('a' + byte((i+j)%26))
		}
		lines[i] = sb.String()
	}
	re := regexp.MustCompile(`abc`)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		joined := strings.Join(lines, "\n")
		rawMatches := re.FindAllStringIndex(joined, -1)
		positions := computeMatchPositions(joined, rawMatches)
		_ = positions
	}
}

// ========== prefixContinuationRows tests ==========

// ---------- plain ASCII segments ----------

func TestPrefixContinuationRows_PlainASCII(t *testing.T) {
	// Continuation segments are already split at contWidth (3 chars).
	// The function just prepends the prefix and adds prefixWidth to widths.
	segs := []string{"abcde", "fgh", "klm"}
	widths := []int{5, 3, 3}
	prefix := "↪ "
	prefixWidth := 2

	prefixContinuationRows(segs, widths, prefix, prefixWidth, false)

	// First segment should be untouched (not a continuation).
	if segs[0] != "abcde" {
		t.Errorf("segs[0] = %q, want %q", segs[0], "abcde")
	}
	if widths[0] != 5 {
		t.Errorf("widths[0] = %d, want 5", widths[0])
	}

	// Second and third should have prefix prepended (no content removed).
	if ansi.Strip(segs[1]) != "↪ fgh" {
		t.Errorf("visible segs[1] = %q, want %q", ansi.Strip(segs[1]), "↪ fgh")
	}
	if widths[1] != 5 {
		t.Errorf("widths[1] = %d, want 5", widths[1])
	}
	if ansi.Strip(segs[2]) != "↪ klm" {
		t.Errorf("visible segs[2] = %q, want %q", ansi.Strip(segs[2]), "↪ klm")
	}
	if widths[2] != 5 {
		t.Errorf("widths[2] = %d, want 5", widths[2])
	}
}

// ---------- ANSI-decorated segments: SGR state preserved ----------

func TestPrefixContinuationRows_ANSIPreservedSGR(t *testing.T) {
	red := "\x1b[31m"
	reset := "\x1b[m"
	// Continuation segments are already split at contWidth (3 chars).
	// The second segment carries the red SGR.
	segs := []string{
		red + "abcde" + reset,
		red + "fgh" + reset,
	}
	widths := []int{5, 3}
	prefix := "↪ "
	prefixWidth := 2

	prefixContinuationRows(segs, widths, prefix, prefixWidth, false)

	// First segment untouched.
	if segs[0] != red+"abcde"+reset {
		t.Errorf("segs[0] should be untouched: %q", segs[0])
	}

	// Second segment: prefix prepended, content preserved.
	stripped := ansi.Strip(segs[1])
	if stripped != "↪ fgh" {
		t.Errorf("visible segs[1] = %q, want %q", stripped, "↪ fgh")
	}
	if widths[1] != 5 {
		t.Errorf("widths[1] = %d, want 5", widths[1])
	}
	// The original segment should still carry the red SGR.
	if !strings.Contains(segs[1], red) {
		t.Errorf("segs[1] should contain red SGR: %q", segs[1])
	}
}

// ---------- firstIsCont=true prefixes all segments ----------

func TestPrefixContinuationRows_FirstIsCont(t *testing.T) {
	// All segments already at contWidth (3 chars). Prefix prepended to all.
	segs := []string{"abc", "fgh", "klm"}
	widths := []int{3, 3, 3}
	prefix := "↪ "
	prefixWidth := 2

	prefixContinuationRows(segs, widths, prefix, prefixWidth, true)

	// All segments should be prefixed.
	for i, seg := range segs {
		stripped := ansi.Strip(seg)
		if !strings.HasPrefix(stripped, "↪ ") {
			t.Errorf("segs[%d] should start with prefix: %q", i, stripped)
		}
	}

	// Verify visible content: prefix prepended, nothing removed.
	if ansi.Strip(segs[0]) != "↪ abc" {
		t.Errorf("visible segs[0] = %q, want %q", ansi.Strip(segs[0]), "↪ abc")
	}
	if ansi.Strip(segs[1]) != "↪ fgh" {
		t.Errorf("visible segs[1] = %q, want %q", ansi.Strip(segs[1]), "↪ fgh")
	}
	if ansi.Strip(segs[2]) != "↪ klm" {
		t.Errorf("visible segs[2] = %q, want %q", ansi.Strip(segs[2]), "↪ klm")
	}

	// Widths should all be 3 + 2 = 5.
	for i, w := range widths {
		if w != 5 {
			t.Errorf("widths[%d] = %d, want 5", i, w)
		}
	}
}

// ---------- narrow segment (width <= replaceWidth): prepend without cutting ----------

func TestPrefixContinuationRows_NarrowSegment(t *testing.T) {
	segs := []string{"abcde", "x"}
	widths := []int{5, 1}
	prefix := "↪ "
	prefixWidth := 2

	prefixContinuationRows(segs, widths, prefix, prefixWidth, false)

	// First segment untouched.
	if segs[0] != "abcde" {
		t.Errorf("segs[0] = %q, want %q", segs[0], "abcde")
	}

	// Second segment: prefix prepended, width increased.
	stripped := ansi.Strip(segs[1])
	if stripped != "↪ x" {
		t.Errorf("visible segs[1] = %q, want %q", stripped, "↪ x")
	}
	if widths[1] != 3 {
		t.Errorf("widths[1] = %d, want 3", widths[1])
	}
}

// ---------- empty/nil segments ----------

func TestPrefixContinuationRows_EmptySegs(t *testing.T) {
	// nil slices: should not panic.
	prefixContinuationRows(nil, nil, "↪ ", 2, false)
	prefixContinuationRows(nil, nil, "↪ ", 2, true)
}

func TestPrefixContinuationRows_SingleSegment(t *testing.T) {
	segs := []string{"abcde"}
	widths := []int{5}
	prefix := "↪ "
	prefixWidth := 2

	// With firstIsCont=false, single segment is never prefixed.
	prefixContinuationRows(segs, widths, prefix, prefixWidth, false)
	if segs[0] != "abcde" {
		t.Errorf("single segment should be untouched: %q", segs[0])
	}
	if widths[0] != 5 {
		t.Errorf("widths[0] = %d, want 5", widths[0])
	}
}

func TestPrefixContinuationRows_SingleSegmentFirstIsCont(t *testing.T) {
	// Segment already at contWidth (3 chars). Prefix prepended.
	segs := []string{"cde"}
	widths := []int{3}
	prefix := "↪ "
	prefixWidth := 2

	// With firstIsCont=true, even single segment is prefixed.
	prefixContinuationRows(segs, widths, prefix, prefixWidth, true)
	stripped := ansi.Strip(segs[0])
	if stripped != "↪ cde" {
		t.Errorf("visible segs[0] = %q, want %q", stripped, "↪ cde")
	}
	if widths[0] != 5 {
		t.Errorf("widths[0] = %d, want 5", widths[0])
	}
}

// ---------- cachedWrapIndicatorPrefix smoke test ----------

func TestCachedWrapIndicatorPrefix(t *testing.T) {
	p := cachedWrapIndicatorPrefix
	// Should contain the arrow character and a trailing space.
	stripped := ansi.Strip(p)
	if stripped != "↪ " {
		t.Errorf("stripped prefix = %q, want %q", stripped, "↪ ")
	}
	// Display width should be 2.
	w := ansi.StringWidth(p)
	if w != wrapIndicatorWidth {
		t.Errorf("prefix width = %d, want %d", w, wrapIndicatorWidth)
	}
}

// ========== wrapLines tests ==========

// ---------- short line (no wrap) ----------

func TestWrapLines_ShortLineNoWrap(t *testing.T) {
	lines := []string{"hello"}
	wrapped, mapping := wrapLines(lines, 10)

	if len(wrapped) != 1 {
		t.Fatalf("expected 1 wrapped line, got %d: %q", len(wrapped), wrapped)
	}
	if wrapped[0] != "hello" {
		t.Errorf("wrapped[0] = %q, want %q", wrapped[0], "hello")
	}
	if len(mapping) != 1 {
		t.Fatalf("expected 1 mapping entry, got %d", len(mapping))
	}
	if mapping[0].logicalLine != 0 || mapping[0].colOffset != 0 {
		t.Errorf("mapping[0] = %+v, want {logicalLine:0, colOffset:0}", mapping[0])
	}
}

// ---------- long line wrapping at exact boundary ----------

func TestWrapLines_LongLineExactBoundary(t *testing.T) {
	// "abcdefghij" is 10 chars, width=5 => row0=5, contWidth=3
	// Row 0: "abcde" (5), Row 1: "↪ fgh" (3+2=5), Row 2: "↪ ij" (2+2=4)
	lines := []string{"abcdefghij"}
	wrapped, mapping := wrapLines(lines, 5)

	if len(wrapped) != 3 {
		t.Fatalf("expected 3 wrapped lines, got %d: %q", len(wrapped), wrapped)
	}

	// First row: original content, no prefix.
	if ansi.Strip(wrapped[0]) != "abcde" {
		t.Errorf("visible wrapped[0] = %q, want %q", ansi.Strip(wrapped[0]), "abcde")
	}

	// Second row: continuation with wrap indicator prefix prepended.
	stripped1 := ansi.Strip(wrapped[1])
	if !strings.HasPrefix(stripped1, "↪ ") {
		t.Errorf("wrapped[1] should start with wrap indicator: %q", stripped1)
	}
	if stripped1 != "↪ fgh" {
		t.Errorf("visible wrapped[1] = %q, want %q", stripped1, "↪ fgh")
	}

	// Third row: last continuation segment.
	stripped2 := ansi.Strip(wrapped[2])
	if !strings.HasPrefix(stripped2, "↪ ") {
		t.Errorf("wrapped[2] should start with wrap indicator: %q", stripped2)
	}
	if stripped2 != "↪ ij" {
		t.Errorf("visible wrapped[2] = %q, want %q", stripped2, "↪ ij")
	}

	// Mapping correctness.
	if len(mapping) != 3 {
		t.Fatalf("expected 3 mapping entries, got %d", len(mapping))
	}
	if mapping[0].logicalLine != 0 || mapping[0].colOffset != 0 {
		t.Errorf("mapping[0] = %+v, want {0, 0}", mapping[0])
	}
	if mapping[1].logicalLine != 0 || mapping[1].colOffset != 5 {
		t.Errorf("mapping[1] = %+v, want {0, 5}", mapping[1])
	}
	if mapping[2].logicalLine != 0 || mapping[2].colOffset != 8 {
		t.Errorf("mapping[2] = %+v, want {0, 8}", mapping[2])
	}
}

// ---------- ANSI content ----------

func TestWrapLines_ANSIContent(t *testing.T) {
	red := "\x1b[31m"
	reset := "\x1b[m"
	// 10 visible chars in red, width=5, contWidth=3 => 3 rows
	lines := []string{red + "abcdefghij" + reset}
	wrapped, mapping := wrapLines(lines, 5)

	if len(wrapped) != 3 {
		t.Fatalf("expected 3 wrapped lines, got %d: %q", len(wrapped), wrapped)
	}

	// First row: visible "abcde", should contain red SGR.
	if ansi.Strip(wrapped[0]) != "abcde" {
		t.Errorf("visible wrapped[0] = %q, want %q", ansi.Strip(wrapped[0]), "abcde")
	}
	if !strings.Contains(wrapped[0], red) {
		t.Errorf("wrapped[0] should contain red SGR: %q", wrapped[0])
	}

	// Second row: continuation with wrap prefix, tail should carry red SGR.
	stripped1 := ansi.Strip(wrapped[1])
	if !strings.HasPrefix(stripped1, "↪ ") {
		t.Errorf("wrapped[1] should start with wrap indicator: %q", stripped1)
	}

	// Third row: last continuation.
	stripped2 := ansi.Strip(wrapped[2])
	if !strings.HasPrefix(stripped2, "↪ ") {
		t.Errorf("wrapped[2] should start with wrap indicator: %q", stripped2)
	}

	// Mapping: all map to logical line 0.
	for i, m := range mapping {
		if m.logicalLine != 0 {
			t.Errorf("mapping[%d].logicalLine = %d, want 0", i, m.logicalLine)
		}
	}
	if mapping[0].colOffset != 0 || mapping[1].colOffset != 5 || mapping[2].colOffset != 8 {
		t.Errorf("colOffsets = {%d, %d, %d}, want {0, 5, 8}",
			mapping[0].colOffset, mapping[1].colOffset, mapping[2].colOffset)
	}
}

// ---------- mapping correctness with multiple lines ----------

func TestWrapLines_MappingMultipleLines(t *testing.T) {
	// width=10, contWidth=8 (10-2)
	lines := []string{
		"short",            // fits in width=10, 1 visual row
		"abcdefghijklmno", // 15 chars: row0=10, row1=5 (fits in contWidth=8) => 2 visual rows
		"x",               // fits, 1 visual row
	}
	wrapped, mapping := wrapLines(lines, 10)

	// Expected: 4 visual rows total
	if len(wrapped) != 4 {
		t.Fatalf("expected 4 wrapped lines, got %d: %q", len(wrapped), wrapped)
	}
	if len(mapping) != 4 {
		t.Fatalf("expected 4 mapping entries, got %d", len(mapping))
	}

	// Row 0: logical line 0, col 0
	if mapping[0] != (lineMap{logicalLine: 0, colOffset: 0}) {
		t.Errorf("mapping[0] = %+v, want {0, 0}", mapping[0])
	}
	// Row 1: logical line 1, col 0 (first segment of wrapped line)
	if mapping[1] != (lineMap{logicalLine: 1, colOffset: 0}) {
		t.Errorf("mapping[1] = %+v, want {1, 0}", mapping[1])
	}
	// Row 2: logical line 1, col 10 (continuation)
	if mapping[2] != (lineMap{logicalLine: 1, colOffset: 10}) {
		t.Errorf("mapping[2] = %+v, want {1, 10}", mapping[2])
	}
	// Row 3: logical line 2, col 0
	if mapping[3] != (lineMap{logicalLine: 2, colOffset: 0}) {
		t.Errorf("mapping[3] = %+v, want {2, 0}", mapping[3])
	}
}

// ---------- edge cases ----------

func TestWrapLines_EmptyInput(t *testing.T) {
	wrapped, mapping := wrapLines(nil, 10)
	if len(wrapped) != 0 {
		t.Errorf("expected 0 wrapped lines for nil input, got %d", len(wrapped))
	}
	if len(mapping) != 0 {
		t.Errorf("expected 0 mapping entries for nil input, got %d", len(mapping))
	}

	wrapped2, mapping2 := wrapLines([]string{}, 10)
	if len(wrapped2) != 0 {
		t.Errorf("expected 0 wrapped lines for empty input, got %d", len(wrapped2))
	}
	if len(mapping2) != 0 {
		t.Errorf("expected 0 mapping entries for empty input, got %d", len(mapping2))
	}
}

func TestWrapLines_SingleCharWidth(t *testing.T) {
	lines := []string{"abc"}
	wrapped, mapping := wrapLines(lines, 1)

	// "abc" at width=1 => 3 visual rows: "a", "b" (continuation), "c" (continuation)
	if len(wrapped) != 3 {
		t.Fatalf("expected 3 wrapped lines, got %d: %q", len(wrapped), wrapped)
	}

	// First row: "a" (no prefix).
	if ansi.Strip(wrapped[0]) != "a" {
		t.Errorf("visible wrapped[0] = %q, want %q", ansi.Strip(wrapped[0]), "a")
	}

	// Continuation rows get the wrap prefix prepended (though the segment
	// is narrower than replaceWidth, the prefix is simply prepended).
	stripped1 := ansi.Strip(wrapped[1])
	if !strings.HasPrefix(stripped1, "↪ ") {
		t.Errorf("wrapped[1] should start with wrap indicator: %q", stripped1)
	}
	stripped2 := ansi.Strip(wrapped[2])
	if !strings.HasPrefix(stripped2, "↪ ") {
		t.Errorf("wrapped[2] should start with wrap indicator: %q", stripped2)
	}

	// Mapping: all point to logical line 0.
	for j, m := range mapping {
		if m.logicalLine != 0 {
			t.Errorf("mapping[%d].logicalLine = %d, want 0", j, m.logicalLine)
		}
		if m.colOffset != j {
			t.Errorf("mapping[%d].colOffset = %d, want %d", j, m.colOffset, j)
		}
	}
}

func TestWrapLines_LineExactlyEqualToWidth(t *testing.T) {
	lines := []string{"abcde"}
	wrapped, mapping := wrapLines(lines, 5)

	if len(wrapped) != 1 {
		t.Fatalf("expected 1 wrapped line, got %d: %q", len(wrapped), wrapped)
	}
	if wrapped[0] != "abcde" {
		t.Errorf("wrapped[0] = %q, want %q", wrapped[0], "abcde")
	}
	if len(mapping) != 1 {
		t.Fatalf("expected 1 mapping entry, got %d", len(mapping))
	}
	if mapping[0] != (lineMap{logicalLine: 0, colOffset: 0}) {
		t.Errorf("mapping[0] = %+v, want {0, 0}", mapping[0])
	}
}

func TestWrapLines_WidthZero(t *testing.T) {
	lines := []string{"hello", "world"}
	wrapped, mapping := wrapLines(lines, 0)

	// Width <= 0: return as-is.
	if len(wrapped) != 2 {
		t.Fatalf("expected 2 lines for width=0, got %d", len(wrapped))
	}
	if wrapped[0] != "hello" || wrapped[1] != "world" {
		t.Errorf("wrapped = %q, want [hello, world]", wrapped)
	}
	if len(mapping) != 2 {
		t.Fatalf("expected 2 mapping entries for width=0, got %d", len(mapping))
	}
	if mapping[0] != (lineMap{logicalLine: 0, colOffset: 0}) {
		t.Errorf("mapping[0] = %+v, want {0, 0}", mapping[0])
	}
	if mapping[1] != (lineMap{logicalLine: 1, colOffset: 0}) {
		t.Errorf("mapping[1] = %+v, want {1, 0}", mapping[1])
	}
}

func TestWrapLines_NegativeWidth(t *testing.T) {
	lines := []string{"hello"}
	wrapped, mapping := wrapLines(lines, -5)

	if len(wrapped) != 1 {
		t.Fatalf("expected 1 line for negative width, got %d", len(wrapped))
	}
	if wrapped[0] != "hello" {
		t.Errorf("wrapped[0] = %q, want %q", wrapped[0], "hello")
	}
	if mapping[0] != (lineMap{logicalLine: 0, colOffset: 0}) {
		t.Errorf("mapping[0] = %+v, want {0, 0}", mapping[0])
	}
}

func TestWrapLines_EmptyLineInInput(t *testing.T) {
	lines := []string{"abc", "", "def"}
	wrapped, mapping := wrapLines(lines, 10)

	if len(wrapped) != 3 {
		t.Fatalf("expected 3 wrapped lines, got %d: %q", len(wrapped), wrapped)
	}
	if wrapped[1] != "" {
		t.Errorf("wrapped[1] = %q, want empty string", wrapped[1])
	}
	if mapping[1] != (lineMap{logicalLine: 1, colOffset: 0}) {
		t.Errorf("mapping[1] = %+v, want {1, 0}", mapping[1])
	}
}
