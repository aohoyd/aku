package highlight

import (
	"testing"
)

const uuidANSI = "\x1b[38;5;6m"

func newTestUUIDHighlighter() *UUIDHighlighter {
	p := Painter{prefix: uuidANSI}
	return NewUUIDHighlighter(p)
}

// uid wraps s with the UUID painter ANSI codes.
func uid(s string) string {
	return uuidANSI + s + reset
}

func TestUUIDHighlighter_ImplementsHighlighter(t *testing.T) {
	var h Highlighter = NewUUIDHighlighter(Painter{})
	_ = h
}

func TestUUIDHighlighter_ValidUUID(t *testing.T) {
	h := newTestUUIDHighlighter()
	input := "550e8400-e29b-41d4-a716-446655440000"
	got := h.Highlight(input)
	want := uid("550e8400-e29b-41d4-a716-446655440000")
	if got != want {
		t.Errorf("ValidUUID:\ngot  %q\nwant %q", got, want)
	}
}

func TestUUIDHighlighter_UUIDInContext(t *testing.T) {
	h := newTestUUIDHighlighter()
	input := "request-id: 550e8400-e29b-41d4-a716-446655440000 processed"
	got := h.Highlight(input)
	want := "request-id: " + uid("550e8400-e29b-41d4-a716-446655440000") + " processed"
	if got != want {
		t.Errorf("UUIDInContext:\ngot  %q\nwant %q", got, want)
	}
}

func TestUUIDHighlighter_MixedCase(t *testing.T) {
	h := newTestUUIDHighlighter()
	input := "id=550E8400-E29B-41D4-A716-446655440000"
	got := h.Highlight(input)
	want := "id=" + uid("550E8400-E29B-41D4-A716-446655440000")
	if got != want {
		t.Errorf("MixedCase:\ngot  %q\nwant %q", got, want)
	}
}

func TestUUIDHighlighter_NotEnoughDashes(t *testing.T) {
	h := newTestUUIDHighlighter()
	input := "abc-def-ghi"
	got := h.Highlight(input)
	if got != input {
		t.Errorf("NotEnoughDashes: expected same string, got %q", got)
	}
}

func TestUUIDHighlighter_NoMatch(t *testing.T) {
	h := newTestUUIDHighlighter()
	input := "plain text without any UUIDs"
	got := h.Highlight(input)
	if got != input {
		t.Errorf("NoMatch: expected same string, got %q", got)
	}
}

func TestUUIDHighlighter_SamePointerOnNoMatch(t *testing.T) {
	h := newTestUUIDHighlighter()

	tests := []string{
		"plain text",
		"",
		"one-two-three",
		"a-b-c only three dashes",
	}

	for _, input := range tests {
		got := h.Highlight(input)
		if got != input {
			t.Errorf("SamePointerOnNoMatch(%q): expected same string, got %q", input, got)
		}
	}
}

func TestUUIDHighlighter_MultipleUUIDs(t *testing.T) {
	h := newTestUUIDHighlighter()
	input := "from 550e8400-e29b-41d4-a716-446655440000 to 6ba7b810-9dad-11d1-80b4-00c04fd430c8 done"
	got := h.Highlight(input)
	want := "from " + uid("550e8400-e29b-41d4-a716-446655440000") + " to " + uid("6ba7b810-9dad-11d1-80b4-00c04fd430c8") + " done"
	if got != want {
		t.Errorf("MultipleUUIDs:\ngot  %q\nwant %q", got, want)
	}
}

func TestUUIDHighlighter_EnoughDashesButNoUUID(t *testing.T) {
	h := newTestUUIDHighlighter()
	input := "a-b-c-d-e not a uuid"
	got := h.Highlight(input)
	if got != input {
		t.Errorf("EnoughDashesButNoUUID: expected same string, got %q", got)
	}
}
