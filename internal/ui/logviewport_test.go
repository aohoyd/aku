package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// ---------- helpers ----------

// lineCount returns the number of lines in s (split by "\n").
func lineCount(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

// displayWidths returns the display width of each line.
func displayWidths(s string) []int {
	lines := strings.Split(s, "\n")
	widths := make([]int, len(lines))
	for i, l := range lines {
		widths[i] = ansi.StringWidth(l)
	}
	return widths
}

// ---------- View() dimension tests ----------

func TestLogViewport_View_CorrectDimensions(t *testing.T) {
	vp := &logViewport{}
	vp.SetSize(40, 10)

	lines := []string{"hello", "world", "foo"}
	widths := []int{5, 5, 3}
	vp.SetLines(lines, widths)

	out := vp.View()
	if lc := lineCount(out); lc != 10 {
		t.Fatalf("expected 10 lines, got %d", lc)
	}
	for i, l := range strings.Split(out, "\n") {
		w := ansi.StringWidth(l)
		if w != 40 {
			t.Errorf("line %d: display width = %d, want 40", i, w)
		}
	}
}

func TestLogViewport_View_ExactHeight(t *testing.T) {
	vp := &logViewport{}
	vp.SetSize(20, 3)

	lines := []string{"aaa", "bbb", "ccc"}
	widths := []int{3, 3, 3}
	vp.SetLines(lines, widths)

	out := vp.View()
	if lc := lineCount(out); lc != 3 {
		t.Fatalf("expected 3 lines, got %d", lc)
	}
}

func TestLogViewport_View_MoreLinesThanHeight(t *testing.T) {
	vp := &logViewport{}
	vp.SetSize(20, 2)

	lines := []string{"aaa", "bbb", "ccc", "ddd"}
	widths := []int{3, 3, 3, 3}
	vp.SetLines(lines, widths)

	out := vp.View()
	if lc := lineCount(out); lc != 2 {
		t.Fatalf("expected 2 lines (height), got %d", lc)
	}
	// Only first 2 lines should be visible
	outLines := strings.Split(out, "\n")
	if !strings.HasPrefix(outLines[0], "aaa") {
		t.Errorf("first line should start with 'aaa', got %q", outLines[0])
	}
	if !strings.HasPrefix(outLines[1], "bbb") {
		t.Errorf("second line should start with 'bbb', got %q", outLines[1])
	}
}

// ---------- View() with H-scroll ----------

func TestLogViewport_View_HScroll(t *testing.T) {
	vp := &logViewport{}
	vp.SetSize(5, 2)
	vp.xOffset = 3

	lines := []string{"abcdefghij", "1234567890"}
	widths := []int{10, 10}
	vp.SetLines(lines, widths)

	out := vp.View()
	outLines := strings.Split(out, "\n")
	if len(outLines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(outLines))
	}
	// ansi.Cut("abcdefghij", 3, 8) should give "defgh"
	if got := ansi.StringWidth(outLines[0]); got != 5 {
		t.Errorf("line 0 width = %d, want 5", got)
	}
	stripped0 := ansi.Strip(outLines[0])
	if stripped0 != "defgh" {
		t.Errorf("line 0 = %q, want %q", stripped0, "defgh")
	}
	stripped1 := ansi.Strip(outLines[1])
	if stripped1 != "45678" {
		t.Errorf("line 1 = %q, want %q", stripped1, "45678")
	}
}

func TestLogViewport_View_HScrollBeyondContent(t *testing.T) {
	vp := &logViewport{}
	vp.SetSize(10, 1)
	vp.xOffset = 20

	lines := []string{"short"}
	widths := []int{5}
	vp.SetLines(lines, widths)

	out := vp.View()
	// visibleWidth = max(0, 5 - 20) = 0, so entire line is padding
	w := ansi.StringWidth(out)
	if w != 10 {
		t.Errorf("display width = %d, want 10", w)
	}
}

func TestLogViewport_View_HScrollPartialLine(t *testing.T) {
	vp := &logViewport{}
	vp.SetSize(10, 1)
	vp.xOffset = 3

	lines := []string{"abcde"} // width 5, after offset 3 only 2 chars visible
	widths := []int{5}
	vp.SetLines(lines, widths)

	out := vp.View()
	outLines := strings.Split(out, "\n")
	w := ansi.StringWidth(outLines[0])
	if w != 10 {
		t.Errorf("display width = %d, want 10", w)
	}
	// "de" + 8 spaces
	stripped := ansi.Strip(outLines[0])
	if !strings.HasPrefix(stripped, "de") {
		t.Errorf("expected line to start with 'de', got %q", stripped)
	}
}

// ---------- View() with ANSI content ----------

func TestLogViewport_View_ANSIContent(t *testing.T) {
	// Red "hello" via SGR
	redHello := "\x1b[31mhello\x1b[0m"
	rawWidth := 5

	vp := &logViewport{}
	vp.SetSize(10, 1)
	vp.SetLines([]string{redHello}, []int{rawWidth})

	out := vp.View()
	outLines := strings.Split(out, "\n")

	// Should still contain the ANSI escape
	if !strings.Contains(outLines[0], "\x1b[31m") {
		t.Error("ANSI escape was stripped from output")
	}
	// Display width should be 10 (5 chars + 5 padding)
	w := ansi.StringWidth(outLines[0])
	if w != 10 {
		t.Errorf("display width = %d, want 10", w)
	}
}

func TestLogViewport_View_ANSIWithHScroll(t *testing.T) {
	// "\x1b[31mhello\x1b[0m world" — display width 11
	styledLine := "\x1b[31mhello\x1b[0m world"
	rawWidth := 11

	vp := &logViewport{}
	vp.SetSize(8, 1)
	vp.xOffset = 3
	vp.SetLines([]string{styledLine}, []int{rawWidth})

	out := vp.View()
	outLines := strings.Split(out, "\n")
	w := ansi.StringWidth(outLines[0])
	if w != 8 {
		t.Errorf("display width = %d, want 8", w)
	}
	// After Cut(3, 11), visible width = min(11-3, 8) = 8
	// The visible text should be "lo world" (characters 3..10 of "hello world")
	stripped := ansi.Strip(outLines[0])
	if stripped != "lo world" {
		t.Errorf("got %q, want %q", stripped, "lo world")
	}
}

func TestLogViewport_View_ANSINoPadding(t *testing.T) {
	// Line that fills exactly the width — no padding needed
	styledLine := "\x1b[32mabcdefghij\x1b[0m"
	rawWidth := 10

	vp := &logViewport{}
	vp.SetSize(10, 1)
	vp.SetLines([]string{styledLine}, []int{rawWidth})

	out := vp.View()
	w := ansi.StringWidth(out)
	if w != 10 {
		t.Errorf("display width = %d, want 10", w)
	}
	// Should still contain ANSI
	if !strings.Contains(out, "\x1b[32m") {
		t.Error("ANSI escape was stripped")
	}
}

// ---------- SetLines with nil/empty input ----------

func TestLogViewport_SetLines_Nil(t *testing.T) {
	vp := &logViewport{}
	vp.SetSize(20, 5)
	vp.SetLines(nil, nil)

	out := vp.View()
	if lc := lineCount(out); lc != 5 {
		t.Fatalf("expected 5 lines, got %d", lc)
	}
	for i, l := range strings.Split(out, "\n") {
		w := ansi.StringWidth(l)
		if w != 20 {
			t.Errorf("line %d: display width = %d, want 20", i, w)
		}
	}
}

func TestLogViewport_SetLines_Empty(t *testing.T) {
	vp := &logViewport{}
	vp.SetSize(20, 5)
	vp.SetLines([]string{}, []int{})

	out := vp.View()
	if lc := lineCount(out); lc != 5 {
		t.Fatalf("expected 5 lines, got %d", lc)
	}
	for i, l := range strings.Split(out, "\n") {
		w := ansi.StringWidth(l)
		if w != 20 {
			t.Errorf("line %d: display width = %d, want 20", i, w)
		}
	}
}

// ---------- Edge cases: zero width/height ----------

func TestLogViewport_View_ZeroWidth(t *testing.T) {
	vp := &logViewport{}
	vp.SetSize(0, 5)
	vp.SetLines([]string{"hello"}, []int{5})

	out := vp.View()
	if out != "" {
		t.Errorf("expected empty string for zero width, got %q", out)
	}
}

func TestLogViewport_View_ZeroHeight(t *testing.T) {
	vp := &logViewport{}
	vp.SetSize(20, 0)
	vp.SetLines([]string{"hello"}, []int{5})

	out := vp.View()
	if out != "" {
		t.Errorf("expected empty string for zero height, got %q", out)
	}
}

func TestLogViewport_View_NoXOffsetSkipsAnsiCut(t *testing.T) {
	// Verify that when xOffset == 0, the original line is preserved exactly
	// (plus padding). We check by including a complex ANSI string.
	styledLine := "\x1b[1;31mERROR\x1b[0m some text"
	rawWidth := 15

	vp := &logViewport{}
	vp.SetSize(20, 1)
	vp.SetLines([]string{styledLine}, []int{rawWidth})

	out := vp.View()
	// The output should start with the exact styled line (no ansi.Cut applied)
	if !strings.HasPrefix(out, styledLine) {
		t.Errorf("output should start with original styled line\ngot:  %q\nwant prefix: %q", out, styledLine)
	}
}

// ---------- Benchmarks ----------

// makeANSILine returns a JSON-highlighted log line with 20+ ANSI escape
// sequences, simulating realistic syntax-highlighted output.
func makeANSILine() string {
	return "\x1b[38;2;127;132;156m{\x1b[0m \x1b[38;2;122;162;247m\"level\"\x1b[0m\x1b[38;2;127;132;156m:\x1b[0m \x1b[38;2;152;195;121m\"info\"\x1b[0m\x1b[38;2;127;132;156m,\x1b[0m \x1b[38;2;122;162;247m\"msg\"\x1b[0m\x1b[38;2;127;132;156m:\x1b[0m \x1b[38;2;152;195;121m\"request processed\"\x1b[0m\x1b[38;2;127;132;156m,\x1b[0m \x1b[38;2;122;162;247m\"duration\"\x1b[0m\x1b[38;2;127;132;156m:\x1b[0m \x1b[38;2;210;126;153m0.042\x1b[0m\x1b[38;2;127;132;156m,\x1b[0m \x1b[38;2;122;162;247m\"status\"\x1b[0m\x1b[38;2;127;132;156m:\x1b[0m \x1b[38;2;210;126;153m200\x1b[0m\x1b[38;2;127;132;156m,\x1b[0m \x1b[38;2;122;162;247m\"ip\"\x1b[0m\x1b[38;2;127;132;156m:\x1b[0m \x1b[38;2;152;195;121m\"10.0.0.1\"\x1b[0m \x1b[38;2;127;132;156m}\x1b[0m"
}

func BenchmarkLogViewportView_ANSIContent(b *testing.B) {
	const nLines = 40
	const width = 120
	const height = 40

	base := makeANSILine()
	baseWidth := ansi.StringWidth(base)

	lines := make([]string, nLines)
	widths := make([]int, nLines)
	for i := range lines {
		lines[i] = base
		widths[i] = baseWidth
	}

	vp := &logViewport{}
	vp.SetSize(width, height)
	vp.SetLines(lines, widths)

	b.ResetTimer()
	for range b.N {
		_ = vp.View()
	}
}

func BenchmarkLogViewportView_PlainText(b *testing.B) {
	const nLines = 40
	const width = 120
	const height = 40

	lines := make([]string, nLines)
	widths := make([]int, nLines)
	for i := range lines {
		lines[i] = fmt.Sprintf("2025-03-27T10:00:%02d.000Z INFO controller/reconciler: processing resource/%d completed successfully", i%60, i)
		widths[i] = len(lines[i]) // plain ASCII, byte len == display width
	}

	vp := &logViewport{}
	vp.SetSize(width, height)
	vp.SetLines(lines, widths)

	b.ResetTimer()
	for range b.N {
		_ = vp.View()
	}
}

func BenchmarkLogViewportView_HScroll(b *testing.B) {
	const nLines = 40
	const width = 120
	const height = 40

	base := makeANSILine()
	baseWidth := ansi.StringWidth(base)

	lines := make([]string, nLines)
	widths := make([]int, nLines)
	for i := range lines {
		lines[i] = base
		widths[i] = baseWidth
	}

	vp := &logViewport{}
	vp.SetSize(width, height)
	vp.xOffset = 20
	vp.SetLines(lines, widths)

	b.ResetTimer()
	for range b.N {
		_ = vp.View()
	}
}
