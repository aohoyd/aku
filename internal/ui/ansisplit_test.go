package ui

import (
	"strings"
	"testing"

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
	segs, widths := splitWrappedVisible(line, 5, 0, 10)

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
	segs, widths := splitWrappedVisible(line, 5, 0, 10)

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
	segs, widths := splitWrappedVisible(line, 5, 0, 10)

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
	segs, widths := splitWrappedVisible(line, 5, 0, 10)

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
	segs, _ := splitWrappedVisible(line, 5, 0, 10)

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
	segs, _ := splitWrappedVisible(line, 5, 0, 10)

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
	segs, _ := splitWrappedVisible(line, 5, 0, 10)

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
	segs, _ := splitWrappedVisible(line, 5, 0, 10)

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
	segs, widths := splitWrappedVisible(line, 5, 0, 10)

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
	segs, widths := splitWrappedVisible(line, 3, 0, 10)

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
	segs, widths := splitWrappedVisible(line, 3, 0, 10)

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
	segs, widths := splitWrappedVisible(line, 1, 0, 10)

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
	segs, widths := splitWrappedVisible("", 5, 0, 10)
	if segs != nil || widths != nil {
		t.Errorf("expected nil for empty string, got segs=%v, widths=%v", segs, widths)
	}
}

func TestSplitWrappedVisible_ShorterThanWidth(t *testing.T) {
	line := "abc"
	segs, widths := splitWrappedVisible(line, 10, 0, 10)

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
	segs, widths := splitWrappedVisible(line, 5, 5, 10)

	if len(segs) != 0 {
		t.Errorf("expected 0 segments for startRow beyond total, got %d: %q", len(segs), segs)
	}
	if len(widths) != 0 {
		t.Errorf("expected 0 widths, got %v", widths)
	}
}

func TestSplitWrappedVisible_NumRowsZero(t *testing.T) {
	segs, widths := splitWrappedVisible("abcde", 5, 0, 0)
	if segs != nil || widths != nil {
		t.Errorf("expected nil for numRows=0, got segs=%v, widths=%v", segs, widths)
	}
}

func TestSplitWrappedVisible_WindowMiddle(t *testing.T) {
	// 20 chars, vpWidth=5 => 4 rows: "abcde", "fghij", "klmno", "pqrst"
	// Request rows 1..2 (startRow=1, numRows=2)
	line := "abcdefghijklmnopqrst"
	segs, widths := splitWrappedVisible(line, 5, 1, 2)

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
	segs, widths := splitWrappedVisible(line, 5, 2, 1)

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
			segs, _ := splitWrappedVisible(line, vpWidth, 0, len(ref)+1)

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
			segs, _ := splitWrappedVisible(line, vpWidth, 0, len(ref)+1)

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
	allSegs, _ := splitWrappedVisible(line, vpWidth, 0, 100)

	for startRow := 0; startRow < len(allSegs); startRow++ {
		for numRows := 1; numRows <= len(allSegs)-startRow; numRows++ {
			segs, _ := splitWrappedVisible(line, vpWidth, startRow, numRows)
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

// ---------- No SGR prefix when no styles active ----------

func TestSplitWrappedVisible_NoSGR_NoPrefix(t *testing.T) {
	line := "abcdefghij"
	segs, _ := splitWrappedVisible(line, 5, 0, 10)

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
	segs, _ := splitWrappedVisible(line, 5, 0, 10)

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
		splitWrappedVisible(line, vpWidth, startRow, numRows)
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
		splitWrappedVisible(line, vpWidth, startRow, numRows)
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
