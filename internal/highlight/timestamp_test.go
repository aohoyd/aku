package highlight

import (
	"testing"
)

const (
	dateANSI = "\x1b[38;5;3m"
	timeANSI = "\x1b[38;5;4m"
	tzANSI   = "\x1b[38;5;5m"
)

func newTestTimestampHighlighter() *TimestampHighlighter {
	dp := Painter{prefix: dateANSI}
	tp := Painter{prefix: timeANSI}
	zp := Painter{prefix: tzANSI}
	return NewTimestampHighlighter(dp, tp, zp)
}

// d wraps s with date ANSI codes.
func d(s string) string {
	return dateANSI + s + reset
}

// tm wraps s with time ANSI codes.
func tm(s string) string {
	return timeANSI + s + reset
}

// tz wraps s with timezone ANSI codes.
func tz(s string) string {
	return tzANSI + s + reset
}

func TestTimestampHighlighter_ISO8601WithT(t *testing.T) {
	h := newTestTimestampHighlighter()
	input := "2024-03-22T14:30:00Z"
	got := h.Highlight(input)

	want := d("2024-03-22") + "T" + tm("14:30:00") + tz("Z")

	if got != want {
		t.Errorf("ISO8601WithT:\ngot  %q\nwant %q", got, want)
	}
}

func TestTimestampHighlighter_SpaceSeparator(t *testing.T) {
	h := newTestTimestampHighlighter()
	input := "2024-03-22 14:30:00"
	got := h.Highlight(input)

	want := d("2024-03-22") + " " + tm("14:30:00")

	if got != want {
		t.Errorf("SpaceSeparator:\ngot  %q\nwant %q", got, want)
	}
}

func TestTimestampHighlighter_FractionalSeconds(t *testing.T) {
	h := newTestTimestampHighlighter()
	input := "2024-03-22T14:30:00.123456Z"
	got := h.Highlight(input)

	want := d("2024-03-22") + "T" + tm("14:30:00.123456") + tz("Z")

	if got != want {
		t.Errorf("FractionalSeconds:\ngot  %q\nwant %q", got, want)
	}
}

func TestTimestampHighlighter_TimezoneOffset(t *testing.T) {
	h := newTestTimestampHighlighter()
	input := "2024-03-22T14:30:00+05:30"
	got := h.Highlight(input)

	want := d("2024-03-22") + "T" + tm("14:30:00") + tz("+05:30")

	if got != want {
		t.Errorf("TimezoneOffset:\ngot  %q\nwant %q", got, want)
	}
}

func TestTimestampHighlighter_NoTimezone(t *testing.T) {
	h := newTestTimestampHighlighter()
	input := "2024-03-22T14:30:00"
	got := h.Highlight(input)

	want := d("2024-03-22") + "T" + tm("14:30:00")

	if got != want {
		t.Errorf("NoTimezone:\ngot  %q\nwant %q", got, want)
	}
}

func TestTimestampHighlighter_EarlyExitNoDash(t *testing.T) {
	h := newTestTimestampHighlighter()
	input := "no dashes here: just colons"
	got := h.Highlight(input)

	if got != input {
		t.Errorf("EarlyExitNoDash: expected same string, got %q", got)
	}
}

func TestTimestampHighlighter_EarlyExitNoColon(t *testing.T) {
	h := newTestTimestampHighlighter()
	input := "no colons here - just dashes"
	got := h.Highlight(input)

	if got != input {
		t.Errorf("EarlyExitNoColon: expected same string, got %q", got)
	}
}

func TestTimestampHighlighter_NoMatch(t *testing.T) {
	h := newTestTimestampHighlighter()
	input := "some-text:with-dashes:and-colons"
	got := h.Highlight(input)

	if got != input {
		t.Errorf("NoMatch: expected same string, got %q", got)
	}
}

func TestTimestampHighlighter_MultipleTimestamps(t *testing.T) {
	h := newTestTimestampHighlighter()
	input := "start 2024-03-22T14:30:00Z middle 2024-12-01T09:00:00+02:00 end"
	got := h.Highlight(input)

	want := "start " +
		d("2024-03-22") + "T" + tm("14:30:00") + tz("Z") +
		" middle " +
		d("2024-12-01") + "T" + tm("09:00:00") + tz("+02:00") +
		" end"

	if got != want {
		t.Errorf("MultipleTimestamps:\ngot  %q\nwant %q", got, want)
	}
}

func TestTimestampHighlighter_ImplementsHighlighter(t *testing.T) {
	var h Highlighter = NewTimestampHighlighter(Painter{}, Painter{}, Painter{})
	_ = h
}

func TestTimestampHighlighter_SamePointerOnNoMatch(t *testing.T) {
	h := newTestTimestampHighlighter()

	tests := []string{
		"plain text",
		"",
		"just some words 123",
		"no-match:here",
	}

	for _, input := range tests {
		got := h.Highlight(input)
		if got != input {
			t.Errorf("SamePointerOnNoMatch(%q): expected same string, got %q", input, got)
		}
	}
}

func TestTimestampHighlighter_TimestampInLogLine(t *testing.T) {
	h := newTestTimestampHighlighter()
	input := "INFO 2024-03-22T14:30:00.123Z Starting server on port 8080"
	got := h.Highlight(input)

	want := "INFO " +
		d("2024-03-22") + "T" + tm("14:30:00.123") + tz("Z") +
		" Starting server on port 8080"

	if got != want {
		t.Errorf("TimestampInLogLine:\ngot  %q\nwant %q", got, want)
	}
}

func TestTimestampHighlighter_TimezoneOffsetNoColon(t *testing.T) {
	h := newTestTimestampHighlighter()
	input := "2024-03-22T14:30:00+0530"
	got := h.Highlight(input)

	want := d("2024-03-22") + "T" + tm("14:30:00") + tz("+0530")

	if got != want {
		t.Errorf("TimezoneOffsetNoColon:\ngot  %q\nwant %q", got, want)
	}
}
