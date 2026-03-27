package highlight

import (
	"strings"
	"testing"
)

const (
	keyANSI    = "\x1b[38;5;1m"
	markerANSI = "\x1b[38;5;2m"
	reset      = "\x1b[0m"
)

func newTestJSONHighlighter() *JSONHighlighter {
	kp := Painter{prefix: keyANSI}
	mp := Painter{prefix: markerANSI}
	return NewJSONHighlighter(kp, mp)
}

// m wraps s with marker ANSI codes.
func m(s string) string {
	return markerANSI + s + reset
}

// k wraps s with key ANSI codes.
func k(s string) string {
	return keyANSI + s + reset
}

func TestJSONHighlighter_ValidObject(t *testing.T) {
	h := newTestJSONHighlighter()
	input := `{"name":"John","age":43}`
	got := h.Highlight(input)

	// Expected: { "name": "John", "age": 43 }
	// Keys colored, markers colored, values plain
	want := m("{") + " " +
		m(`"`) + k("name") + m(`"`) + m(":") + " " + `"John"` +
		m(",") + " " +
		m(`"`) + k("age") + m(`"`) + m(":") + " " + "43" +
		" " + m("}")

	if got != want {
		t.Errorf("ValidObject:\ngot  %q\nwant %q", got, want)
	}
}

func TestJSONHighlighter_ValidArray(t *testing.T) {
	h := newTestJSONHighlighter()
	input := `[1, 2, 3]`
	got := h.Highlight(input)

	// Expected: [1, 2, 3]
	// Brackets and commas colored, numbers plain
	want := m("[") +
		"1" + m(",") + " " +
		"2" + m(",") + " " +
		"3" +
		m("]")

	if got != want {
		t.Errorf("ValidArray:\ngot  %q\nwant %q", got, want)
	}
}

func TestJSONHighlighter_NestedJSON(t *testing.T) {
	h := newTestJSONHighlighter()
	input := `{"data":{"inner":"value"}}`
	got := h.Highlight(input)

	// Expected: { "data": { "inner": "value" } }
	want := m("{") + " " +
		m(`"`) + k("data") + m(`"`) + m(":") + " " +
		m("{") + " " +
		m(`"`) + k("inner") + m(`"`) + m(":") + " " + `"value"` +
		" " + m("}") +
		" " + m("}")

	if got != want {
		t.Errorf("NestedJSON:\ngot  %q\nwant %q", got, want)
	}
}

func TestJSONHighlighter_MixedLine(t *testing.T) {
	h := newTestJSONHighlighter()
	input := `INFO {"key":"value"}`
	got := h.Highlight(input)

	// "INFO " should be plain, JSON fragment colorized
	want := "INFO " +
		m("{") + " " +
		m(`"`) + k("key") + m(`"`) + m(":") + " " + `"value"` +
		" " + m("}")

	if got != want {
		t.Errorf("MixedLine:\ngot  %q\nwant %q", got, want)
	}
}

func TestJSONHighlighter_InvalidJSON(t *testing.T) {
	h := newTestJSONHighlighter()
	input := "not json at all"
	got := h.Highlight(input)

	if got != input {
		t.Errorf("InvalidJSON: expected same string, got %q", got)
	}
}

func TestJSONHighlighter_BraceButInvalid(t *testing.T) {
	h := newTestJSONHighlighter()
	input := "{broken"
	got := h.Highlight(input)

	if got != input {
		t.Errorf("BraceButInvalid: expected same string, got %q", got)
	}
}

func TestJSONHighlighter_EmptyInput(t *testing.T) {
	h := newTestJSONHighlighter()
	input := ""
	got := h.Highlight(input)

	if got != input {
		t.Errorf("EmptyInput: expected empty string, got %q", got)
	}
}

func TestJSONHighlighter_BooleansAndNull(t *testing.T) {
	h := newTestJSONHighlighter()
	input := `{"flag":true,"val":null}`
	got := h.Highlight(input)

	// true and null should be plain text (not colored by JSON highlighter)
	want := m("{") + " " +
		m(`"`) + k("flag") + m(`"`) + m(":") + " " + "true" +
		m(",") + " " +
		m(`"`) + k("val") + m(`"`) + m(":") + " " + "null" +
		" " + m("}")

	if got != want {
		t.Errorf("BooleansAndNull:\ngot  %q\nwant %q", got, want)
	}
}

func TestJSONHighlighter_FalseValue(t *testing.T) {
	h := newTestJSONHighlighter()
	input := `{"enabled":false}`
	got := h.Highlight(input)

	want := m("{") + " " +
		m(`"`) + k("enabled") + m(`"`) + m(":") + " " + "false" +
		" " + m("}")

	if got != want {
		t.Errorf("FalseValue:\ngot  %q\nwant %q", got, want)
	}
}

func TestJSONHighlighter_ArrayOfStrings(t *testing.T) {
	h := newTestJSONHighlighter()
	input := `["hello","world"]`
	got := h.Highlight(input)

	want := m("[") +
		`"hello"` + m(",") + " " +
		`"world"` +
		m("]")

	if got != want {
		t.Errorf("ArrayOfStrings:\ngot  %q\nwant %q", got, want)
	}
}

func TestJSONHighlighter_NestedArray(t *testing.T) {
	h := newTestJSONHighlighter()
	input := `{"items":[1,2]}`
	got := h.Highlight(input)

	want := m("{") + " " +
		m(`"`) + k("items") + m(`"`) + m(":") + " " +
		m("[") + "1" + m(",") + " " + "2" + m("]") +
		" " + m("}")

	if got != want {
		t.Errorf("NestedArray:\ngot  %q\nwant %q", got, want)
	}
}

func TestJSONHighlighter_MultipleFragments(t *testing.T) {
	h := newTestJSONHighlighter()
	input := `first {"a":1} middle {"b":2} last`
	got := h.Highlight(input)

	frag1 := m("{") + " " +
		m(`"`) + k("a") + m(`"`) + m(":") + " " + "1" +
		" " + m("}")
	frag2 := m("{") + " " +
		m(`"`) + k("b") + m(`"`) + m(":") + " " + "2" +
		" " + m("}")
	want := "first " + frag1 + " middle " + frag2 + " last"

	if got != want {
		t.Errorf("MultipleFragments:\ngot  %q\nwant %q", got, want)
	}
}

func TestJSONHighlighter_EscapedQuotesInString(t *testing.T) {
	h := newTestJSONHighlighter()
	input := `{"msg":"say \"hello\""}`
	got := h.Highlight(input)

	// The string value should be rendered with escaped quotes preserved
	want := m("{") + " " +
		m(`"`) + k("msg") + m(`"`) + m(":") + " " + `"say \"hello\""` +
		" " + m("}")

	if got != want {
		t.Errorf("EscapedQuotesInString:\ngot  %q\nwant %q", got, want)
	}
}

func TestJSONHighlighter_EmptyObject(t *testing.T) {
	h := newTestJSONHighlighter()
	input := `{}`
	got := h.Highlight(input)

	want := m("{") + " " + " " + m("}")

	if got != want {
		t.Errorf("EmptyObject:\ngot  %q\nwant %q", got, want)
	}
}

func TestJSONHighlighter_EmptyArray(t *testing.T) {
	h := newTestJSONHighlighter()
	input := `[]`
	got := h.Highlight(input)

	want := m("[") + m("]")

	if got != want {
		t.Errorf("EmptyArray:\ngot  %q\nwant %q", got, want)
	}
}

func TestJSONHighlighter_NumberTypes(t *testing.T) {
	h := newTestJSONHighlighter()
	input := `{"int":42,"float":3.14,"neg":-1}`
	got := h.Highlight(input)

	want := m("{") + " " +
		m(`"`) + k("int") + m(`"`) + m(":") + " " + "42" +
		m(",") + " " +
		m(`"`) + k("float") + m(`"`) + m(":") + " " + "3.14" +
		m(",") + " " +
		m(`"`) + k("neg") + m(`"`) + m(":") + " " + "-1" +
		" " + m("}")

	if got != want {
		t.Errorf("NumberTypes:\ngot  %q\nwant %q", got, want)
	}
}

func TestJSONHighlighter_ImplementsHighlighter(t *testing.T) {
	var h Highlighter = NewJSONHighlighter(Painter{}, Painter{})
	_ = h
}

func TestJSONHighlighter_SamePointerOnNoMatch(t *testing.T) {
	h := newTestJSONHighlighter()

	tests := []string{
		"plain text",
		"no braces here",
		"",
		"just some words 123",
	}

	for _, input := range tests {
		got := h.Highlight(input)
		if got != input {
			t.Errorf("SamePointerOnNoMatch(%q): expected same string", input)
		}
	}
}

func TestJSONHighlighter_ValuesArePlain(t *testing.T) {
	h := newTestJSONHighlighter()
	input := `{"s":"text","n":99,"b":true,"x":null}`
	got := h.Highlight(input)

	// Verify that string values, numbers, booleans, and null do NOT contain
	// key or marker ANSI codes wrapping them — they should be plain text.
	if !strings.Contains(got, `"text"`) {
		t.Error("String value should be plain text")
	}
	if !strings.Contains(got, "99") {
		t.Error("Number value should be plain text")
	}
	if !strings.Contains(got, "true") {
		t.Error("Boolean value should be plain text")
	}
	if !strings.Contains(got, "null") {
		t.Error("Null value should be plain text")
	}
}
