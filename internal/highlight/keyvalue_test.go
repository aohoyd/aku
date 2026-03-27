package highlight

import (
	"testing"
)

const (
	kvKeyANSI = "\x1b[38;5;20m"
	kvSepANSI = "\x1b[38;5;21m"
)

func newTestKeyValueHighlighter() *KeyValueHighlighter {
	kp := Painter{prefix: kvKeyANSI}
	sp := Painter{prefix: kvSepANSI}
	return NewKeyValueHighlighter(kp, sp)
}

func TestKeyValueHighlighter_ImplementsHighlighter(t *testing.T) {
	var h Highlighter = NewKeyValueHighlighter(Painter{}, Painter{})
	_ = h
}

func TestKeyValueHighlighter_Basic(t *testing.T) {
	h := newTestKeyValueHighlighter()
	input := `level=info msg="starting"`
	want := paint(kvKeyANSI, "level") + paint(kvSepANSI, "=") + `info ` +
		paint(kvKeyANSI, "msg") + paint(kvSepANSI, "=") + `"starting"`
	got := h.Highlight(input)
	if got != want {
		t.Errorf("Basic:\ngot  %q\nwant %q", got, want)
	}
}

func TestKeyValueHighlighter_KeyAtLineStart(t *testing.T) {
	h := newTestKeyValueHighlighter()
	input := "key=value"
	want := paint(kvKeyANSI, "key") + paint(kvSepANSI, "=") + "value"
	got := h.Highlight(input)
	if got != want {
		t.Errorf("KeyAtLineStart:\ngot  %q\nwant %q", got, want)
	}
}

func TestKeyValueHighlighter_MultiplePairs(t *testing.T) {
	h := newTestKeyValueHighlighter()
	input := "a=1 b=2 c=3"
	want := paint(kvKeyANSI, "a") + paint(kvSepANSI, "=") + "1 " +
		paint(kvKeyANSI, "b") + paint(kvSepANSI, "=") + "2 " +
		paint(kvKeyANSI, "c") + paint(kvSepANSI, "=") + "3"
	got := h.Highlight(input)
	if got != want {
		t.Errorf("MultiplePairs:\ngot  %q\nwant %q", got, want)
	}
}

func TestKeyValueHighlighter_NoMatch(t *testing.T) {
	h := newTestKeyValueHighlighter()
	input := "no equals here"
	got := h.Highlight(input)
	if got != input {
		t.Errorf("NoMatch: expected original string, got %q", got)
	}
}

func TestKeyValueHighlighter_ValueWithQuotes(t *testing.T) {
	h := newTestKeyValueHighlighter()
	input := `msg="hello world"`
	want := paint(kvKeyANSI, "msg") + paint(kvSepANSI, "=") + `"hello world"`
	got := h.Highlight(input)
	if got != want {
		t.Errorf("ValueWithQuotes:\ngot  %q\nwant %q", got, want)
	}
}

func TestKeyValueHighlighter_KeyWithUnderscoreAndNumbers(t *testing.T) {
	h := newTestKeyValueHighlighter()
	input := "request_id=abc123"
	want := paint(kvKeyANSI, "request_id") + paint(kvSepANSI, "=") + "abc123"
	got := h.Highlight(input)
	if got != want {
		t.Errorf("KeyWithUnderscoreAndNumbers:\ngot  %q\nwant %q", got, want)
	}
}

func TestKeyValueHighlighter_EmptyString(t *testing.T) {
	h := newTestKeyValueHighlighter()
	input := ""
	got := h.Highlight(input)
	if got != input {
		t.Errorf("EmptyString: expected original string, got %q", got)
	}
}
