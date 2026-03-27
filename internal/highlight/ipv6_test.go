package highlight

import (
	"testing"
)

const ipv6ANSI = "\x1b[38;5;3m"

func newTestIPv6Highlighter() *IPv6Highlighter {
	p := Painter{prefix: ipv6ANSI}
	return NewIPv6Highlighter(p)
}

// ipv6p wraps s with the IPv6 painter ANSI codes.
func ipv6p(s string) string {
	return ipv6ANSI + s + reset
}

func TestIPv6Highlighter_ImplementsHighlighter(t *testing.T) {
	var h Highlighter = NewIPv6Highlighter(Painter{})
	_ = h
}

func TestIPv6Highlighter_FullIPv6(t *testing.T) {
	h := newTestIPv6Highlighter()
	input := "connected to 2001:0db8:85a3:0000:0000:8a2e:0370:7334 ok"
	got := h.Highlight(input)

	want := "connected to " + ipv6p("2001:0db8:85a3:0000:0000:8a2e:0370:7334") + " ok"
	if got != want {
		t.Errorf("FullIPv6:\ngot  %q\nwant %q", got, want)
	}
}

func TestIPv6Highlighter_Compressed(t *testing.T) {
	h := newTestIPv6Highlighter()
	input := "listening on ::1 port 8080"
	got := h.Highlight(input)

	want := "listening on " + ipv6p("::1") + " port 8080"
	if got != want {
		t.Errorf("Compressed:\ngot  %q\nwant %q", got, want)
	}
}

func TestIPv6Highlighter_Mixed(t *testing.T) {
	h := newTestIPv6Highlighter()
	// fe80::1%eth0 — regex should match fe80::1, the %eth0 is a zone ID not part of the IP
	input := "link-local fe80::1%eth0"
	got := h.Highlight(input)

	want := "link-local " + ipv6p("fe80::1") + "%eth0"
	if got != want {
		t.Errorf("Mixed:\ngot  %q\nwant %q", got, want)
	}
}

func TestIPv6Highlighter_WithPrefix(t *testing.T) {
	h := newTestIPv6Highlighter()
	input := "route 2001:db8::/32 added"
	got := h.Highlight(input)

	want := "route " + ipv6p("2001:db8::/32") + " added"
	if got != want {
		t.Errorf("WithPrefix:\ngot  %q\nwant %q", got, want)
	}
}

func TestIPv6Highlighter_TooShort(t *testing.T) {
	h := newTestIPv6Highlighter()
	// 12:34 looks like hex:hex but is not a valid IPv6
	input := "time is 12:34 now"
	got := h.Highlight(input)

	if got != input {
		t.Errorf("TooShort: expected same string, got %q", got)
	}
}

func TestIPv6Highlighter_EarlyExit(t *testing.T) {
	h := newTestIPv6Highlighter()
	input := "no colons here"
	got := h.Highlight(input)

	if got != input {
		t.Errorf("EarlyExit: expected same string, got %q", got)
	}
}

func TestIPv6Highlighter_NoMatchReturnsSamePointer(t *testing.T) {
	h := newTestIPv6Highlighter()

	tests := []string{
		"plain text",
		"",
		"just numbers 12345",
		"no colons here",
	}

	for _, input := range tests {
		got := h.Highlight(input)
		if got != input {
			t.Errorf("NoMatchReturnsSamePointer(%q): expected same string, got %q", input, got)
		}
	}
}

func TestIPv6Highlighter_MultipleAddresses(t *testing.T) {
	h := newTestIPv6Highlighter()
	input := "from ::1 to 2001:db8::1"
	got := h.Highlight(input)

	want := "from " + ipv6p("::1") + " to " + ipv6p("2001:db8::1")
	if got != want {
		t.Errorf("MultipleAddresses:\ngot  %q\nwant %q", got, want)
	}
}

func TestIPv6Highlighter_StandaloneAddress(t *testing.T) {
	h := newTestIPv6Highlighter()
	input := "2001:0db8:85a3:0000:0000:8a2e:0370:7334"
	got := h.Highlight(input)

	want := ipv6p("2001:0db8:85a3:0000:0000:8a2e:0370:7334")
	if got != want {
		t.Errorf("StandaloneAddress:\ngot  %q\nwant %q", got, want)
	}
}
