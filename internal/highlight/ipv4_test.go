package highlight

import (
	"testing"
)

const ipv4ANSI = "\x1b[38;5;3m"

func newTestIPv4Highlighter() *IPv4Highlighter {
	p := Painter{prefix: ipv4ANSI}
	return NewIPv4Highlighter(p)
}

// ip wraps s with the IPv4 painter ANSI codes.
func ip(s string) string {
	return ipv4ANSI + s + reset
}

func TestIPv4Highlighter_ImplementsHighlighter(t *testing.T) {
	var h Highlighter = NewIPv4Highlighter(Painter{})
	_ = h
}

func TestIPv4Highlighter_ValidIP(t *testing.T) {
	h := newTestIPv4Highlighter()
	input := "connecting to 192.168.1.1 now"
	got := h.Highlight(input)
	want := "connecting to " + ip("192.168.1.1") + " now"
	if got != want {
		t.Errorf("ValidIP:\ngot  %q\nwant %q", got, want)
	}
}

func TestIPv4Highlighter_IPWithPort(t *testing.T) {
	h := newTestIPv4Highlighter()
	input := "server at 10.0.0.1:8080 ready"
	got := h.Highlight(input)
	want := "server at " + ip("10.0.0.1:8080") + " ready"
	if got != want {
		t.Errorf("IPWithPort:\ngot  %q\nwant %q", got, want)
	}
}

func TestIPv4Highlighter_InvalidOctets(t *testing.T) {
	h := newTestIPv4Highlighter()
	input := "addr 999.1.2.3 bad"
	got := h.Highlight(input)
	if got != input {
		t.Errorf("InvalidOctets: expected same string, got %q", got)
	}
}

func TestIPv4Highlighter_MultipleIPs(t *testing.T) {
	h := newTestIPv4Highlighter()
	input := "from 10.0.0.1 to 192.168.0.2"
	got := h.Highlight(input)
	want := "from " + ip("10.0.0.1") + " to " + ip("192.168.0.2")
	if got != want {
		t.Errorf("MultipleIPs:\ngot  %q\nwant %q", got, want)
	}
}

func TestIPv4Highlighter_WordBoundary(t *testing.T) {
	h := newTestIPv4Highlighter()
	// Leading digit should prevent match due to word boundary.
	input := "1192.168.1.1 is not valid"
	got := h.Highlight(input)
	if got != input {
		t.Errorf("WordBoundary: expected same string, got %q", got)
	}
}

func TestIPv4Highlighter_NoDots(t *testing.T) {
	h := newTestIPv4Highlighter()
	input := "no dots here at all"
	got := h.Highlight(input)
	if got != input {
		t.Errorf("NoDots: expected same string, got %q", got)
	}
}

func TestIPv4Highlighter_SamePointerOnNoMatch(t *testing.T) {
	h := newTestIPv4Highlighter()

	tests := []string{
		"plain text",
		"",
		"one.two.three",
		"only two dots 1.2",
	}

	for _, input := range tests {
		got := h.Highlight(input)
		if got != input {
			t.Errorf("SamePointerOnNoMatch(%q): expected same string, got %q", input, got)
		}
	}
}

func TestIPv4Highlighter_BoundaryIPs(t *testing.T) {
	h := newTestIPv4Highlighter()
	input := "range 0.0.0.0 to 255.255.255.255"
	got := h.Highlight(input)
	want := "range " + ip("0.0.0.0") + " to " + ip("255.255.255.255")
	if got != want {
		t.Errorf("BoundaryIPs:\ngot  %q\nwant %q", got, want)
	}
}

func TestIPv4Highlighter_OctetJustOver255(t *testing.T) {
	h := newTestIPv4Highlighter()
	input := "addr 256.1.2.3 bad"
	got := h.Highlight(input)
	if got != input {
		t.Errorf("OctetJustOver255: expected same string, got %q", got)
	}
}

func TestIPv4Highlighter_IPAtStartAndEnd(t *testing.T) {
	h := newTestIPv4Highlighter()
	input := "10.0.0.1"
	got := h.Highlight(input)
	want := ip("10.0.0.1")
	if got != want {
		t.Errorf("IPAtStartAndEnd:\ngot  %q\nwant %q", got, want)
	}
}

func TestIPv4Highlighter_MixedValidAndInvalid(t *testing.T) {
	h := newTestIPv4Highlighter()
	input := "valid 10.0.0.1 invalid 300.1.2.3 valid 1.2.3.4"
	got := h.Highlight(input)
	want := "valid " + ip("10.0.0.1") + " invalid 300.1.2.3 valid " + ip("1.2.3.4")
	if got != want {
		t.Errorf("MixedValidAndInvalid:\ngot  %q\nwant %q", got, want)
	}
}
