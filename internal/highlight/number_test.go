package highlight

import (
	"testing"
)

const numberANSI = "\x1b[38;5;3m"

func newTestNumberHighlighter() *NumberHighlighter {
	p := Painter{prefix: numberANSI}
	return NewNumberHighlighter(p)
}

// num wraps s with the Number painter ANSI codes.
func num(s string) string {
	return numberANSI + s + reset
}

func TestNumberHighlighter_ImplementsHighlighter(t *testing.T) {
	var h Highlighter = NewNumberHighlighter(Painter{})
	_ = h
}

func TestNumberHighlighter_Integer(t *testing.T) {
	h := newTestNumberHighlighter()
	input := "count: 42"
	got := h.Highlight(input)
	want := "count: " + num("42")
	if got != want {
		t.Errorf("Integer:\ngot  %q\nwant %q", got, want)
	}
}

func TestNumberHighlighter_Decimal(t *testing.T) {
	h := newTestNumberHighlighter()
	input := "latency: 0.042"
	got := h.Highlight(input)
	want := "latency: " + num("0.042")
	if got != want {
		t.Errorf("Decimal:\ngot  %q\nwant %q", got, want)
	}
}

func TestNumberHighlighter_MultipleNumbers(t *testing.T) {
	h := newTestNumberHighlighter()
	input := "items 10 of 100"
	got := h.Highlight(input)
	want := "items " + num("10") + " of " + num("100")
	if got != want {
		t.Errorf("MultipleNumbers:\ngot  %q\nwant %q", got, want)
	}
}

func TestNumberHighlighter_WordBoundary(t *testing.T) {
	h := newTestNumberHighlighter()
	// abc123: between 'c' and '1', both are \w, so no \b boundary — no match.
	input := "abc123"
	got := h.Highlight(input)
	if got != input {
		t.Errorf("WordBoundary: expected same string, got %q", got)
	}
}

func TestNumberHighlighter_NumberAfterSpace(t *testing.T) {
	h := newTestNumberHighlighter()
	input := "abc 123"
	got := h.Highlight(input)
	want := "abc " + num("123")
	if got != want {
		t.Errorf("NumberAfterSpace:\ngot  %q\nwant %q", got, want)
	}
}

func TestNumberHighlighter_NoNumbers(t *testing.T) {
	h := newTestNumberHighlighter()
	input := "no numbers here"
	got := h.Highlight(input)
	if got != input {
		t.Errorf("NoNumbers: expected same string, got %q", got)
	}
}

func TestNumberHighlighter_SamePointerOnNoMatch(t *testing.T) {
	h := newTestNumberHighlighter()

	tests := []string{
		"plain text",
		"",
		"abc123def",
	}

	for _, input := range tests {
		got := h.Highlight(input)
		if got != input {
			t.Errorf("SamePointerOnNoMatch(%q): expected same string, got %q", input, got)
		}
	}
}

func TestNumberHighlighter_NegativeNumber(t *testing.T) {
	h := newTestNumberHighlighter()
	// Negative sign is not part of the regex, only 42 matches.
	input := "value: -42"
	got := h.Highlight(input)
	want := "value: -" + num("42")
	if got != want {
		t.Errorf("NegativeNumber:\ngot  %q\nwant %q", got, want)
	}
}
