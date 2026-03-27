package highlight

import (
	"testing"
)

const quoteANSI = "\x1b[38;5;3m"

func newTestQuoteHighlighter() *QuoteHighlighter {
	p := Painter{prefix: quoteANSI}
	return NewQuoteHighlighter('"', p)
}

// q wraps s with the quote painter ANSI codes.
func q(s string) string {
	return quoteANSI + s + reset
}

func TestQuoteHighlighter_ImplementsHighlighter(t *testing.T) {
	var h Highlighter = NewQuoteHighlighter('"', Painter{})
	_ = h
}

func TestQuoteHighlighter_SimpleQuotedString(t *testing.T) {
	h := newTestQuoteHighlighter()
	input := "say \"hello world\""
	got := h.Highlight(input)
	want := "say " + q("\"hello world\"")
	if got != want {
		t.Errorf("SimpleQuotedString:\ngot  %q\nwant %q", got, want)
	}
}

func TestQuoteHighlighter_EmptyQuotes(t *testing.T) {
	h := newTestQuoteHighlighter()
	input := "value: \"\""
	got := h.Highlight(input)
	want := "value: " + q("\"\"")
	if got != want {
		t.Errorf("EmptyQuotes:\ngot  %q\nwant %q", got, want)
	}
}

func TestQuoteHighlighter_UnbalancedQuotes(t *testing.T) {
	h := newTestQuoteHighlighter()
	input := "say \"hello"
	got := h.Highlight(input)
	if got != input {
		t.Errorf("UnbalancedQuotes: expected same string, got %q", got)
	}
}

func TestQuoteHighlighter_NestedHighlightInsideQuotes(t *testing.T) {
	h := newTestQuoteHighlighter()
	// Simulates a number highlighter having already colored "42" with cyan.
	input := "\"count \x1b[36m42\x1b[0m items\""
	got := h.Highlight(input)
	// The quote color wraps around, and after the inner \x1b[0m the quote color is re-injected.
	want := quoteANSI + "\"count \x1b[36m42\x1b[0m" + quoteANSI + " items\"" + reset
	if got != want {
		t.Errorf("NestedHighlightInsideQuotes:\ngot  %q\nwant %q", got, want)
	}
}

func TestQuoteHighlighter_MultipleQuotedStrings(t *testing.T) {
	h := newTestQuoteHighlighter()
	input := "\"first\" and \"second\""
	got := h.Highlight(input)
	want := q("\"first\"") + " and " + q("\"second\"")
	if got != want {
		t.Errorf("MultipleQuotedStrings:\ngot  %q\nwant %q", got, want)
	}
}

func TestQuoteHighlighter_NoQuotes(t *testing.T) {
	h := newTestQuoteHighlighter()
	input := "plain text without quotes"
	got := h.Highlight(input)
	if got != input {
		t.Errorf("NoQuotes: expected same string, got %q", got)
	}
}

func TestQuoteHighlighter_SamePointerOnNoMatch(t *testing.T) {
	h := newTestQuoteHighlighter()

	tests := []string{
		"plain text",
		"",
		"no quotes here",
	}

	for _, input := range tests {
		got := h.Highlight(input)
		if got != input {
			t.Errorf("SamePointerOnNoMatch(%q): expected same string, got %q", input, got)
		}
	}
}

func TestQuoteHighlighter_TextBetweenQuotesNotHighlighted(t *testing.T) {
	h := newTestQuoteHighlighter()
	input := "between \"a\" here \"b\""
	got := h.Highlight(input)
	want := "between " + q("\"a\"") + " here " + q("\"b\"")
	if got != want {
		t.Errorf("TextBetweenQuotesNotHighlighted:\ngot  %q\nwant %q", got, want)
	}
}

func TestQuoteHighlighter_ThreeQuotes_Unbalanced(t *testing.T) {
	h := newTestQuoteHighlighter()
	input := "\"a\" \"b"
	got := h.Highlight(input)
	if got != input {
		t.Errorf("ThreeQuotes_Unbalanced: expected same string, got %q", got)
	}
}

func TestQuoteHighlighter_MultipleNestedResets(t *testing.T) {
	h := newTestQuoteHighlighter()
	// Two highlighted tokens inside the same quoted string.
	input := "\"a \x1b[36m1\x1b[0m b \x1b[36m2\x1b[0m c\""
	got := h.Highlight(input)
	want := quoteANSI + "\"a \x1b[36m1\x1b[0m" + quoteANSI + " b \x1b[36m2\x1b[0m" + quoteANSI + " c\"" + reset
	if got != want {
		t.Errorf("MultipleNestedResets:\ngot  %q\nwant %q", got, want)
	}
}
