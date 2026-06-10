package ui

import (
	"strings"
	"testing"

	"github.com/aohoyd/aku/internal/notify"
	"github.com/charmbracelet/x/ansi"
)

func TestToastStackEmpty(t *testing.T) {
	ts := NewToastStack(5)

	if got := ts.View(nil, 120, 40); got != "" {
		t.Errorf("View(nil) = %q, want empty", got)
	}
	if got := ts.View([]notify.Message{}, 120, 40); got != "" {
		t.Errorf("View(empty) = %q, want empty", got)
	}
}

func TestToastStackSingleInfo(t *testing.T) {
	ts := NewToastStack(5)

	out := ansi.Strip(ts.View([]notify.Message{
		{Level: notify.LevelInfo, Text: "deployment scaled"},
	}, 120, 40))

	if !strings.Contains(out, toastGlyphInfo) {
		t.Errorf("output missing info glyph %q:\n%s", toastGlyphInfo, out)
	}
	if !strings.Contains(out, "deployment scaled") {
		t.Errorf("output missing message text:\n%s", out)
	}
}

func TestToastStackLevelGlyphs(t *testing.T) {
	cases := []struct {
		name  string
		level notify.Level
		glyph string
	}{
		{"info", notify.LevelInfo, toastGlyphInfo},
		{"warning", notify.LevelWarning, toastGlyphWarning},
		{"error", notify.LevelError, toastGlyphError},
	}

	ts := NewToastStack(5)
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := ansi.Strip(ts.View([]notify.Message{
				{Level: c.level, Text: "something happened"},
			}, 120, 40))
			if !strings.Contains(out, c.glyph) {
				t.Errorf("level %s: output missing glyph %q:\n%s", c.name, c.glyph, out)
			}
		})
	}
}

func TestToastStackTruncatesLongText(t *testing.T) {
	// Box width is now derived from terminal size, not a fixed parameter. With
	// termW=30: maxW = clamp(30*2/5=12, 8, 60) = 12 (the termW-4=26 box-fit cap
	// does not bind). A 200-char single word far exceeds maxW*maxH, so it must
	// be wrapped/hard-broken and the last line capped with an ellipsis.
	const termW, termH = 30, 40
	const maxW = 12 // clamp(termW*2/5, 8, 60)
	ts := NewToastStack(5)

	long := strings.Repeat("abcdefghij", 20) // 200 chars, far wider than maxW
	out := ts.View([]notify.Message{
		{Level: notify.LevelInfo, Text: long},
	}, termW, termH)

	// Every content line must fit within maxW + chrome (border 2 + padding 2).
	stripped := ansi.Strip(out)
	for _, l := range strings.Split(stripped, "\n") {
		if w := ansi.StringWidth(l); w > maxW+toastChrome {
			t.Errorf("line visible width = %d, want <= %d (maxW %d + chrome):\n%s", w, maxW+toastChrome, maxW, l)
		}
	}

	// The capped (overflowing) content must end with an ellipsis somewhere.
	if !strings.Contains(stripped, "…") {
		t.Errorf("truncated output missing ellipsis:\n%s", stripped)
	}
	if strings.Contains(stripped, long) {
		t.Errorf("long text was not truncated:\n%s", stripped)
	}
}

func TestToastStackOverflow(t *testing.T) {
	ts := NewToastStack(2)

	msgs := []notify.Message{
		{Level: notify.LevelInfo, Text: "msg one"},
		{Level: notify.LevelInfo, Text: "msg two"},
		{Level: notify.LevelInfo, Text: "msg three"},
		{Level: notify.LevelInfo, Text: "msg four"},
		{Level: notify.LevelInfo, Text: "msg five"},
	}

	out := ansi.Strip(ts.View(msgs, 120, 40))

	if !strings.Contains(out, "+3 more") {
		t.Errorf("output missing overflow indicator %q:\n%s", "+3 more", out)
	}

	// Exactly two message boxes should be rendered: count glyph occurrences.
	if n := strings.Count(out, toastGlyphInfo); n != 2 {
		t.Errorf("rendered %d toast boxes, want 2:\n%s", n, out)
	}

	// Only the first two (newest-first) messages should appear.
	if !strings.Contains(out, "msg one") || !strings.Contains(out, "msg two") {
		t.Errorf("expected the first two messages to render:\n%s", out)
	}
	if strings.Contains(out, "msg three") {
		t.Errorf("overflowed message should not render:\n%s", out)
	}
}

// TestToastStackExactlyMaxVisible verifies that when the message count equals
// maxVisible, every message renders and no "+N more…" overflow line appears.
func TestToastStackExactlyMaxVisible(t *testing.T) {
	const maxVisible = 3
	ts := NewToastStack(maxVisible)

	msgs := []notify.Message{
		{Level: notify.LevelInfo, Text: "msg one"},
		{Level: notify.LevelInfo, Text: "msg two"},
		{Level: notify.LevelInfo, Text: "msg three"},
	}

	out := ansi.Strip(ts.View(msgs, 120, 40))

	if strings.Contains(out, "more") {
		t.Errorf("exactly maxVisible messages should produce no overflow line:\n%s", out)
	}
	if n := strings.Count(out, toastGlyphInfo); n != maxVisible {
		t.Errorf("rendered %d toast boxes, want %d:\n%s", n, maxVisible, out)
	}
	for _, m := range msgs {
		if !strings.Contains(out, m.Text) {
			t.Errorf("expected all messages to render, missing %q:\n%s", m.Text, out)
		}
	}
}

// TestToastStackUnknownLevelFallsBackToInfo verifies an unrecognized Level value
// renders via View using the info glyph (toastDecoration's default branch).
func TestToastStackUnknownLevelFallsBackToInfo(t *testing.T) {
	ts := NewToastStack(5)

	out := ansi.Strip(ts.View([]notify.Message{
		{Level: notify.Level(99), Text: "mystery"},
	}, 120, 40))

	if !strings.Contains(out, toastGlyphInfo) {
		t.Errorf("unknown level should fall back to info glyph %q:\n%s", toastGlyphInfo, out)
	}
	if strings.Contains(out, toastGlyphWarning) || strings.Contains(out, toastGlyphError) {
		t.Errorf("unknown level should not use warning/error glyphs:\n%s", out)
	}
	if !strings.Contains(out, "mystery") {
		t.Errorf("unknown level toast should still render its text:\n%s", out)
	}
}

// maxLineWidth returns the maximum visible width across all non-empty lines of
// a rendered string after stripping ANSI. For a single-toast render this is the
// box's outer width.
func maxLineWidth(rendered string) int {
	widest := 0
	for _, l := range strings.Split(ansi.Strip(rendered), "\n") {
		if strings.TrimSpace(l) == "" {
			continue
		}
		if w := ansi.StringWidth(l); w > widest {
			widest = w
		}
	}
	return widest
}

// TestToastStackShrinkToFit verifies a short message on a wide terminal renders
// a box sized to its content (≈ "ℹ ok" + 4 chrome ≈ 8 cells) rather than
// filling the width ceiling (maxW=60 at termW=200).
func TestToastStackShrinkToFit(t *testing.T) {
	ts := NewToastStack(5)

	out := ts.View([]notify.Message{
		{Level: notify.LevelInfo, Text: "ok"},
	}, 200, 40)

	// Tight bound: the box must shrink to its content. Content is
	// glyph+" "+text = "ℹ ok"; outer box width = content width + chrome. This
	// must fail if shrink-to-fit is removed and the box uses full maxW (=60 at
	// termW=200), giving an outer width of 64.
	w := maxLineWidth(out)
	want := ansi.StringWidth(toastGlyphInfo+" ok") + toastChrome
	if w != want {
		t.Errorf("short toast box width = %d, want exactly %d (content %q + chrome %d, shrunk not filling maxW=60):\n%s",
			w, want, toastGlyphInfo+" ok", toastChrome, ansi.Strip(out))
	}
}

// TestToastStackWordWrap verifies a message wider than maxW but short enough to
// fit within maxH wraps onto multiple lines without splitting normal words, and
// the full text is recoverable from the wrapped output.
func TestToastStackWordWrap(t *testing.T) {
	// termW=120 → maxW = clamp(120*2/5=48, 8, 60) = 48. maxH=6.
	const maxW = 48
	ts := NewToastStack(5)

	// ~120 chars of short, space-separated real words (no word longer than
	// maxW, so none should be hard-broken). Includes a sentinel "ELEPHANT".
	text := "the quick brown fox jumps over a lazy dog while a wise old ELEPHANT walks past the river bank under warm sun rays"
	out := ts.View([]notify.Message{
		{Level: notify.LevelInfo, Text: text},
	}, 120, 40)

	stripped := ansi.Strip(out)
	// Collect non-empty content rows (single toast: no overflow line). Use the
	// structural chrome-detecting helper so rounded-border (╭╮╰╯) rows are
	// correctly dropped and payload runes that happen to look like chrome are not
	// mistaken for border.
	lines := contentRows(out)
	// The input is 113 visible cells of words; wrapping at maxW=48 without
	// splitting words yields at least 3 content lines (113/48 ≈ 2.35, but word
	// boundaries push it to 3). Assert a concrete lower bound, not just >1.
	if len(lines) < 3 {
		t.Fatalf("expected wrapped output to span >= 3 content lines, got %d:\n%s", len(lines), stripped)
	}

	// Every visible line must fit within maxW + chrome.
	for _, l := range strings.Split(stripped, "\n") {
		if w := ansi.StringWidth(l); w > maxW+toastChrome {
			t.Errorf("line visible width = %d, want <= %d (maxW %d + chrome):\n%s", w, maxW+toastChrome, maxW, l)
		}
	}

	// Sentinel word must survive intact (no mid-word break).
	if !strings.Contains(stripped, "ELEPHANT") {
		t.Errorf("normal word \"ELEPHANT\" was split during wrap:\n%s", stripped)
	}

	// Full text recoverable IN ORDER: strip box chrome from each line, join,
	// collect words. Compare the full ordered sequence so a dropped, duplicated,
	// or reordered word is caught (a set comparison would silently dedup).
	var contentWords []string
	for _, r := range contentRows(out) {
		contentWords = append(contentWords, strings.Fields(r)...)
	}
	// Drop the leading level glyph token (prefixed to the rendered content).
	if len(contentWords) > 0 && contentWords[0] == toastGlyphInfo {
		contentWords = contentWords[1:]
	}
	wantWords := strings.Fields(text)
	if len(contentWords) != len(wantWords) {
		t.Fatalf("recovered %d words, want %d (ordered):\ngot:  %v\nwant: %v",
			len(contentWords), len(wantWords), contentWords, wantWords)
	}
	for i := range wantWords {
		if contentWords[i] != wantWords[i] {
			t.Errorf("word %d mismatch: got %q, want %q\ngot:  %v\nwant: %v",
				i, contentWords[i], wantWords[i], contentWords, wantWords)
		}
	}
}

// TestToastStackHeightCapEllipsis verifies a message far exceeding the height
// cap renders exactly maxH content lines with a trailing ellipsis.
func TestToastStackHeightCapEllipsis(t *testing.T) {
	// Mirror the exact caps View computes for termW=120, termH=100:
	//   maxW = clamp(120*2/5=48, 8, 60) = 48
	//   maxH = clamp(100*3/10=30, 1, 6) = 6
	// Call renderToast directly so the assertions don't depend on counting
	// border rows of the joined stack.
	const maxW, maxH = 48, 6

	// ~600 chars of words: far more than maxW*maxH can hold → overflow.
	text := strings.TrimSpace(strings.Repeat("alpha bravo charlie delta echo foxtrot ", 16))
	out := renderToast(notify.Message{Level: notify.LevelInfo, Text: text}, maxW, maxH)

	// Use the package-level contentRows helper to extract non-chrome rows with
	// side border/padding already trimmed. Their count must equal exactly maxH.
	rows := contentRows(out)
	if len(rows) != maxH {
		t.Errorf("rendered %d content lines, want exactly maxH=%d:\n%s", len(rows), maxH, ansi.Strip(out))
	}

	// The ellipsis must terminate the LAST content line, not merely appear
	// somewhere. The helper already trims the row's side chrome/padding.
	last := rows[len(rows)-1]
	if !strings.HasSuffix(last, "…") {
		t.Errorf("ellipsis must terminate the final visible content line, got last line %q:\n%s", last, ansi.Strip(out))
	}
	// And no earlier content line should carry the truncation ellipsis.
	for _, l := range rows[:len(rows)-1] {
		if strings.Contains(l, "…") {
			t.Errorf("ellipsis appeared on a non-final content line %q:\n%s", l, ansi.Strip(out))
		}
	}
}

// TestToastStackProportionalCaps verifies box width scales with terminal width
// and the wide case honors the ceiling clamp (maxW=60).
func TestToastStackProportionalCaps(t *testing.T) {
	ts := NewToastStack(5)
	long := strings.TrimSpace(strings.Repeat("lorem ipsum dolor sit amet ", 10))
	msgs := []notify.Message{{Level: notify.LevelInfo, Text: long}}

	narrow := maxLineWidth(ts.View(msgs, 30, 40)) // maxW=12
	wide := maxLineWidth(ts.View(msgs, 200, 40))  // maxW=60 (ceiling)

	if narrow >= wide {
		t.Errorf("narrow box width (%d) should be strictly less than wide box width (%d)", narrow, wide)
	}
	// Pin the narrow box exactly: at termW=30, maxW = clamp(30*2/5=12, 8, 60) =
	// 12; the message is far longer than 12, so the box fills maxW and the outer
	// width is exactly maxW + chrome.
	if want := 12 + toastChrome; narrow != want {
		t.Errorf("narrow box outer width = %d, want exactly %d (maxW 12 + chrome %d)", narrow, want, toastChrome)
	}
	// Pin the wide box exactly at the ceiling: at termW=200, maxW =
	// clamp(200*2/5=80, 8, 60) = 60 (ceiling). The message (270 cells) far
	// exceeds 60, so the box fills the ceiling and the outer width is exactly
	// toastWidthCeil + chrome. An exact pin (not just <=) fails if the ceiling
	// is lowered.
	if want := toastWidthCeil + toastChrome; wide != want {
		t.Errorf("wide box outer width = %d, want exactly %d (ceiling %d + chrome %d)", wide, want, toastWidthCeil, toastChrome)
	}
}

// TestToastStackZeroSizeGuard verifies non-positive / tiny terminal dimensions
// do not panic. A terminal too small to hold even a minimal toast (inner floor
// + chrome, i.e. termW < toastWidthFloor+toastChrome, or termH < 1) must render
// nothing rather than overflow the screen.
func TestToastStackZeroSizeGuard(t *testing.T) {
	ts := NewToastStack(5)
	msgs := []notify.Message{{Level: notify.LevelInfo, Text: "hi"}}

	cases := []struct {
		name         string
		termW, termH int
	}{
		{"negative", -1, -1},
		{"zero", 0, 0},
		{"one", 1, 1},
		{"five", 5, 5},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("View(%d,%d) panicked: %v", c.termW, c.termH, r)
				}
			}()
			out := ts.View(msgs, c.termW, c.termH)
			if strings.TrimSpace(ansi.Strip(out)) != "" {
				t.Errorf("View(%d,%d) = %q, want empty (terminal too small to hold a toast)", c.termW, c.termH, ansi.Strip(out))
			}
		})
	}
}

// TestToastStackTinyTerminalGuardThreshold verifies the exact guard boundary:
// just below toastWidthFloor+toastChrome renders nothing, while exactly at the
// threshold renders a (minimal, non-overflowing) box.
func TestToastStackTinyTerminalGuardThreshold(t *testing.T) {
	ts := NewToastStack(5)
	msgs := []notify.Message{{Level: notify.LevelInfo, Text: "x"}}

	below := toastWidthFloor + toastChrome - 1
	if out := ts.View(msgs, below, 40); strings.TrimSpace(ansi.Strip(out)) != "" {
		t.Errorf("View at termW=%d (below threshold) = %q, want empty", below, ansi.Strip(out))
	}

	at := toastWidthFloor + toastChrome
	out := ts.View(msgs, at, 40)
	if strings.TrimSpace(ansi.Strip(out)) == "" {
		t.Fatalf("View at termW=%d (threshold) returned empty, want a box", at)
	}
	if w := maxLineWidth(out); w > at {
		t.Errorf("box outer width = %d at termW=%d, must not overflow the terminal", w, at)
	}
}

// TestToastStackFloorWins verifies that on a small-but-usable terminal where the
// proportional width math would fall below the floor, the floor pins the inner
// width, giving an outer box of toastWidthFloor+toastChrome. termW=12:
// proportional = 12*2/5 = 4 → floored to 8; content "ℹ small msg here" is wider
// than 8 so the box fills the full floored width.
func TestToastStackFloorWins(t *testing.T) {
	ts := NewToastStack(5)
	out := ts.View([]notify.Message{
		{Level: notify.LevelInfo, Text: "small msg here so it exceeds the floor"},
	}, 12, 40)

	w := maxLineWidth(out)
	want := toastWidthFloor + toastChrome
	if w != want {
		t.Errorf("floor-bound box outer width = %d, want %d (floor %d + chrome %d):\n%s",
			w, want, toastWidthFloor, toastChrome, ansi.Strip(out))
	}
}

// contentRows returns the non-chrome content rows of a single rendered toast,
// with the box border/padding trimmed from each. Pure-border rows are dropped.
func contentRows(rendered string) []string {
	var rows []string
	for _, l := range strings.Split(ansi.Strip(rendered), "\n") {
		if strings.TrimSpace(l) == "" {
			continue
		}
		if strings.TrimSpace(strings.Trim(l, "─│╭╮╰╯ ")) == "" {
			continue // pure box-drawing (border) row
		}
		rows = append(rows, strings.Trim(l, "│ "))
	}
	return rows
}

// TestToastStackMultiLineBoxWidth verifies that when m.Text contains embedded
// newlines, the box is sized to the WIDEST line, not the sum of all lines'
// widths. Regression for boxW measuring the whole string across "\n".
func TestToastStackMultiLineBoxWidth(t *testing.T) {
	// Two lines: the second ("longer line two") is the widest. With the glyph
	// prefix the first content line is "ℹ line one". The widest visible line is
	// "longer line two" (15 cells). maxW=48 (termW=120) does not bind.
	const maxW, maxH = 48, 6
	out := renderToast(notify.Message{
		Level: notify.LevelInfo,
		Text:  "line one\nlonger line two",
	}, maxW, maxH)

	// Expected widest line = "longer line two" = 15 cells (wider than the
	// glyph-prefixed first line "ℹ line one" = 10). Outer box = widest + chrome.
	widest := ansi.StringWidth("longer line two")
	want := widest + toastChrome
	if w := maxLineWidth(out); w != want {
		t.Errorf("multi-line box outer width = %d, want %d (widest line %d + chrome %d). "+
			"A wider box means boxW summed across newlines:\n%s", w, want, widest, toastChrome, ansi.Strip(out))
	}
}

// TestToastStackShrinksBelowMaxW verifies content narrower than maxW produces a
// box sized to the content width, not padded out to maxW.
func TestToastStackShrinksBelowMaxW(t *testing.T) {
	// termW=120 → maxW = clamp(120*2/5=48, 8, 60) = 48. Craft text so that
	// glyph+" "+text is exactly maxW-1 = 47 cells, then assert the outer box is
	// (maxW-1)+chrome, NOT maxW+chrome.
	const maxW = 48
	glyphW := ansi.StringWidth(toastGlyphInfo + " ") // glyph + space
	textW := (maxW - 1) - glyphW
	text := strings.Repeat("a", textW)
	if ansi.StringWidth(toastGlyphInfo+" "+text) != maxW-1 {
		t.Fatalf("test setup: content width = %d, want %d", ansi.StringWidth(toastGlyphInfo+" "+text), maxW-1)
	}

	ts := NewToastStack(5)
	out := ts.View([]notify.Message{{Level: notify.LevelInfo, Text: text}}, 120, 40)

	w := maxLineWidth(out)
	want := (maxW - 1) + toastChrome
	if w != want {
		t.Errorf("box outer width = %d, want %d (content %d + chrome %d), not maxW(%d)+chrome=%d:\n%s",
			w, want, maxW-1, toastChrome, maxW, maxW+toastChrome, ansi.Strip(out))
	}
}

// TestToastStackExactlyMaxHNoEllipsis verifies a message that wraps to exactly
// maxH content lines renders all maxH lines with NO truncation ellipsis.
func TestToastStackExactlyMaxHNoEllipsis(t *testing.T) {
	const maxW, maxH = 20, 4
	// Each "word" is 18 cells; with the glyph prefix on the first line and a
	// box width of 20, each line holds one word, producing exactly maxH=4 lines.
	const word = "wwwwwwwwwwwwwwwwww" // 18 chars
	text := strings.TrimSpace(strings.Repeat(word+" ", maxH))
	out := renderToast(notify.Message{Level: notify.LevelInfo, Text: text}, maxW, maxH)

	rows := contentRows(out)
	if len(rows) != maxH {
		t.Fatalf("content line count = %d, want exactly maxH=%d:\n%s", len(rows), maxH, ansi.Strip(out))
	}
	for i, r := range rows {
		if strings.Contains(r, "…") {
			t.Errorf("content line %d has an ellipsis but text fits in maxH exactly: %q:\n%s", i, r, ansi.Strip(out))
		}
	}
}

// TestToastStackHardBreaksLongWord verifies a single word longer than maxW (no
// spaces) is hard-broken so every full line is exactly maxW wide, and no
// characters are lost (concatenated content, minus any ellipsis, is a prefix of
// the original word).
func TestToastStackHardBreaksLongWord(t *testing.T) {
	const maxW, maxH = 12, 6
	// One 100-char word, far wider than maxW. maxW*maxH=72 inner cells minus the
	// 2-cell glyph prefix < 100, so this overflows the height cap and the last
	// visible line carries a truncation ellipsis. Every non-last line is a full
	// maxW-wide hard break.
	word := strings.Repeat("z", 100)
	out := renderToast(notify.Message{Level: notify.LevelInfo, Text: word}, maxW, maxH)

	rows := contentRows(out)
	if len(rows) == 0 {
		t.Fatalf("no content rows rendered:\n%s", ansi.Strip(out))
	}

	// The glyph is space-separated from the long word, so it occupies the first
	// content line alone; the hard-broken word fills the remaining lines.
	if rows[0] != toastGlyphInfo {
		t.Fatalf("first content row = %q, want the glyph %q alone:\n%s", rows[0], toastGlyphInfo, ansi.Strip(out))
	}
	wordRows := rows[1:]
	if len(wordRows) == 0 {
		t.Fatalf("no hard-break rows for the long word:\n%s", ansi.Strip(out))
	}

	// Every word line except the last must be exactly maxW wide (full hard
	// break); none may exceed maxW.
	var reconstructed strings.Builder
	for i, r := range wordRows {
		w := ansi.StringWidth(r)
		if i < len(wordRows)-1 && w != maxW {
			t.Errorf("hard-break line %d width = %d, want exactly maxW=%d: %q", i, w, maxW, r)
		}
		if w > maxW {
			t.Errorf("line %d width = %d exceeds maxW=%d: %q", i, w, maxW, r)
		}
		reconstructed.WriteString(r)
	}

	// Strip any trailing ellipsis (this input overflows maxH), then confirm the
	// remaining characters are a prefix of the original word — nothing lost or
	// invented.
	got := strings.TrimSuffix(reconstructed.String(), "…")
	if !strings.HasPrefix(word, got) {
		t.Errorf("reconstructed content %q is not a prefix of the original word; characters were lost or altered", got)
	}
	if got == "" {
		t.Errorf("reconstructed content is empty; the word was dropped entirely")
	}
}

// TestToastStackBoxAlwaysFits documents and verifies that, for every usable
// terminal width at or above the tiny-terminal guard threshold, the rendered
// outer box never exceeds termW. This is why View needs no separate
// "termW-chrome" box-fit clamp: the proportional width (clamped to the floor)
// already fits once termW >= toastWidthFloor+toastChrome. The previously
// present fit-clamp branch was dead code (it could only fire for termW < ~6.7,
// where the floor immediately overrode it) and was removed.
func TestToastStackBoxAlwaysFits(t *testing.T) {
	ts := NewToastStack(5)
	long := strings.TrimSpace(strings.Repeat("lorem ipsum dolor ", 20))
	msgs := []notify.Message{{Level: notify.LevelInfo, Text: long}}

	for termW := toastWidthFloor + toastChrome; termW <= 200; termW++ {
		out := ts.View(msgs, termW, 40)
		if w := maxLineWidth(out); w > termW {
			t.Fatalf("at termW=%d outer box width = %d, overflows the terminal", termW, w)
		}
	}
}

// TestToastStackViewHeightFloor verifies that on a valid-width terminal with a
// very short height, View computes maxH = clamp(termH*3/10, 1, 6) = 1 and a
// wrapping message renders exactly one content row terminated by an ellipsis.
// This exercises View's height handling end-to-end (not just renderToast).
func TestToastStackViewHeightFloor(t *testing.T) {
	ts := NewToastStack(5)
	// Long enough to wrap onto many lines at maxW=48 (termW=120), forcing the
	// maxH=1 cap to truncate down to a single ellipsised row.
	text := strings.TrimSpace(strings.Repeat("alpha bravo charlie delta echo ", 12))
	msgs := []notify.Message{{Level: notify.LevelInfo, Text: text}}

	for _, termH := range []int{1, 2, 3} {
		// maxH = clamp(termH*3/10, 1, 6): for termH in {1,2,3} → 0 floored to 1.
		rows := contentRows(ts.View(msgs, 120, termH))
		if len(rows) != 1 {
			t.Errorf("termH=%d: rendered %d content rows, want exactly 1 (maxH floor)", termH, len(rows))
			continue
		}
		if !strings.HasSuffix(rows[0], "…") {
			t.Errorf("termH=%d: single content row %q must end with an ellipsis", termH, rows[0])
		}
	}
}

// TestToastStackViewHeightGuard isolates the termH < 1 guard: with a valid
// width (>= toastWidthFloor+toastChrome, so the width guard does NOT fire),
// termH of 0 and -1 must each make View return "". Removing the termH guard
// would fail this test.
func TestToastStackViewHeightGuard(t *testing.T) {
	ts := NewToastStack(5)
	msgs := []notify.Message{{Level: notify.LevelInfo, Text: "hello"}}

	const validW = 120 // well above the width guard threshold
	for _, termH := range []int{0, -1} {
		if out := ts.View(msgs, validW, termH); strings.TrimSpace(ansi.Strip(out)) != "" {
			t.Errorf("View(termW=%d, termH=%d) = %q, want empty (termH < 1 guard)", validW, termH, ansi.Strip(out))
		}
	}
}

// TestToastStackViewHeightCeiling verifies that on a tall terminal View honors
// the height ceiling: maxH = clamp(termH*3/10, 1, 6) = 6 at termH=20, so a very
// long message renders exactly 6 content rows.
func TestToastStackViewHeightCeiling(t *testing.T) {
	ts := NewToastStack(5)
	// Far more than maxW(48)*maxH(6) can hold → must clamp to the 6-row ceiling.
	text := strings.TrimSpace(strings.Repeat("alpha bravo charlie delta echo foxtrot ", 20))
	msgs := []notify.Message{{Level: notify.LevelInfo, Text: text}}

	rows := contentRows(ts.View(msgs, 120, 20))
	if len(rows) != toastHeightCeil {
		t.Errorf("rendered %d content rows, want exactly %d (height ceiling):\n%v", len(rows), toastHeightCeil, rows)
	}
}

func TestToastStackConstructorGuards(t *testing.T) {
	// Non-positive maxVisible must fall back to the default and still render.
	ts := NewToastStack(0)

	out := ansi.Strip(ts.View([]notify.Message{
		{Level: notify.LevelInfo, Text: "hello"},
	}, 120, 40))
	if !strings.Contains(out, "hello") || !strings.Contains(out, toastGlyphInfo) {
		t.Errorf("zero-arg constructor did not render a usable toast:\n%s", out)
	}
}
