package highlight

import (
	"testing"
)

const (
	httpMethodANSI = "\x1b[48;5;4;38;5;15m" // bg blue + fg white
	boolTrueANSI   = "\x1b[38;5;2m"         // green
	boolFalseANSI  = "\x1b[38;5;1m"         // red
	nullANSI       = "\x1b[38;5;8m"         // grey
)

func newTestKeywordHighlighter() *KeywordHighlighter {
	return NewKeywordHighlighter([]KeywordGroup{
		{
			Words:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS", "CONNECT"},
			Painter: Painter{prefix: httpMethodANSI},
		},
		{
			Words:   []string{"true", "TRUE"},
			Painter: Painter{prefix: boolTrueANSI},
		},
		{
			Words:   []string{"false", "FALSE"},
			Painter: Painter{prefix: boolFalseANSI},
		},
		{
			Words:   []string{"null", "NULL", "nil", "NIL", "NaN", "undefined"},
			Painter: Painter{prefix: nullANSI},
		},
	})
}

func kpaint(ansi, s string) string {
	return ansi + s + AnsiReset
}

func TestKeywordHighlighter_ImplementsHighlighter(t *testing.T) {
	var h Highlighter = NewKeywordHighlighter(nil)
	_ = h
}

func TestKeywordHighlighter_HTTPMethod(t *testing.T) {
	h := newTestKeywordHighlighter()
	input := "GET /api/users"
	want := kpaint(httpMethodANSI, "GET") + " /api/users"
	got := h.Highlight(input)
	if got != want {
		t.Errorf("HTTPMethod:\ngot  %q\nwant %q", got, want)
	}
}

func TestKeywordHighlighter_AllHTTPMethods(t *testing.T) {
	h := newTestKeywordHighlighter()
	methods := []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS", "CONNECT"}
	for _, m := range methods {
		input := m + " /path"
		want := kpaint(httpMethodANSI, m) + " /path"
		got := h.Highlight(input)
		if got != want {
			t.Errorf("AllHTTPMethods(%q):\ngot  %q\nwant %q", m, got, want)
		}
	}
}

func TestKeywordHighlighter_BooleanTrue(t *testing.T) {
	h := newTestKeywordHighlighter()
	tests := []struct {
		input string
		want  string
	}{
		{"enabled: true", "enabled: " + kpaint(boolTrueANSI, "true")},
		{"enabled: TRUE", "enabled: " + kpaint(boolTrueANSI, "TRUE")},
	}
	for _, tt := range tests {
		got := h.Highlight(tt.input)
		if got != tt.want {
			t.Errorf("BooleanTrue(%q):\ngot  %q\nwant %q", tt.input, got, tt.want)
		}
	}
}

func TestKeywordHighlighter_BooleanFalse(t *testing.T) {
	h := newTestKeywordHighlighter()
	tests := []struct {
		input string
		want  string
	}{
		{"enabled: false", "enabled: " + kpaint(boolFalseANSI, "false")},
		{"enabled: FALSE", "enabled: " + kpaint(boolFalseANSI, "FALSE")},
	}
	for _, tt := range tests {
		got := h.Highlight(tt.input)
		if got != tt.want {
			t.Errorf("BooleanFalse(%q):\ngot  %q\nwant %q", tt.input, got, tt.want)
		}
	}
}

func TestKeywordHighlighter_NullVariants(t *testing.T) {
	h := newTestKeywordHighlighter()
	tests := []struct {
		input string
		want  string
	}{
		{"value: null", "value: " + kpaint(nullANSI, "null")},
		{"value: NULL", "value: " + kpaint(nullANSI, "NULL")},
		{"value: nil", "value: " + kpaint(nullANSI, "nil")},
		{"value: NIL", "value: " + kpaint(nullANSI, "NIL")},
		{"value: NaN", "value: " + kpaint(nullANSI, "NaN")},
		{"value: undefined", "value: " + kpaint(nullANSI, "undefined")},
	}
	for _, tt := range tests {
		got := h.Highlight(tt.input)
		if got != tt.want {
			t.Errorf("NullVariants(%q):\ngot  %q\nwant %q", tt.input, got, tt.want)
		}
	}
}

func TestKeywordHighlighter_WordBoundary(t *testing.T) {
	h := newTestKeywordHighlighter()
	tests := []string{
		"GETTING things done",
		"POSTED the letter",
		"PUTTING it away",
		"DELETER removed it",
		"nullable field",
		"undefined_var should not match partial",
	}
	for _, input := range tests {
		got := h.Highlight(input)
		if got != input {
			t.Errorf("WordBoundary(%q): expected no match, got %q", input, got)
		}
	}
}

func TestKeywordHighlighter_CaseSensitive(t *testing.T) {
	h := newTestKeywordHighlighter()
	tests := []string{
		"get /api/users",
		"post /api/users",
		"Get /api/users",
		"True value",
		"False value",
		"Null value",
		"Nil value",
	}
	for _, input := range tests {
		got := h.Highlight(input)
		if got != input {
			t.Errorf("CaseSensitive(%q): expected no match (case mismatch), got %q", input, got)
		}
	}
}

func TestKeywordHighlighter_MultipleKeywords(t *testing.T) {
	h := newTestKeywordHighlighter()
	input := "POST /api true"
	want := kpaint(httpMethodANSI, "POST") + " /api " + kpaint(boolTrueANSI, "true")
	got := h.Highlight(input)
	if got != want {
		t.Errorf("MultipleKeywords:\ngot  %q\nwant %q", got, want)
	}
}

func TestKeywordHighlighter_NoMatchReturnsSamePointer(t *testing.T) {
	h := newTestKeywordHighlighter()
	tests := []string{
		"plain text with no keywords",
		"",
		"just some numbers 12345",
		"GETTING things done",
	}
	for _, input := range tests {
		got := h.Highlight(input)
		if got != input {
			t.Errorf("NoMatchReturnsSamePointer(%q): expected same string, got %q", input, got)
		}
	}
}

func TestKeywordHighlighter_NilGroups(t *testing.T) {
	h := NewKeywordHighlighter(nil)
	input := "GET /api/users"
	got := h.Highlight(input)
	if got != input {
		t.Errorf("NilGroups: expected same string, got %q", got)
	}
}

func TestKeywordHighlighter_EmptyGroups(t *testing.T) {
	h := NewKeywordHighlighter([]KeywordGroup{})
	input := "GET /api/users"
	got := h.Highlight(input)
	if got != input {
		t.Errorf("EmptyGroups: expected same string, got %q", got)
	}
}
