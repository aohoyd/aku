package highlight

import (
	"testing"
)

const (
	protocolANSI = "\x1b[38;5;1m"
	hostANSI     = "\x1b[38;5;2m"
	pathANSI     = "\x1b[38;5;3m"
	queryANSI    = "\x1b[38;5;4m"
	symbolANSI   = "\x1b[38;5;5m"
)

func newTestURLHighlighter() *URLHighlighter {
	return NewURLHighlighter(
		Painter{prefix: protocolANSI},
		Painter{prefix: hostANSI},
		Painter{prefix: pathANSI},
		Painter{prefix: queryANSI},
		Painter{prefix: symbolANSI},
	)
}

// proto wraps s with protocol ANSI codes.
func proto(s string) string {
	return protocolANSI + s + reset
}

// host wraps s with host ANSI codes.
func host(s string) string {
	return hostANSI + s + reset
}

// pth wraps s with path ANSI codes.
func pth(s string) string {
	return pathANSI + s + reset
}

// qry wraps s with query ANSI codes.
func qry(s string) string {
	return queryANSI + s + reset
}

// sym wraps s with symbol ANSI codes.
func sym(s string) string {
	return symbolANSI + s + reset
}

func TestURLHighlighter_ImplementsHighlighter(t *testing.T) {
	var h Highlighter = NewURLHighlighter(Painter{}, Painter{}, Painter{}, Painter{}, Painter{})
	_ = h
}

func TestURLHighlighter_SimpleURL(t *testing.T) {
	h := newTestURLHighlighter()
	input := "https://example.com"
	got := h.Highlight(input)

	want := proto("https") + sym("://") + host("example.com")
	if got != want {
		t.Errorf("SimpleURL:\ngot  %q\nwant %q", got, want)
	}
}

func TestURLHighlighter_URLWithPath(t *testing.T) {
	h := newTestURLHighlighter()
	input := "http://example.com/api/v1"
	got := h.Highlight(input)

	want := proto("http") + sym("://") + host("example.com") + pth("/api/v1")
	if got != want {
		t.Errorf("URLWithPath:\ngot  %q\nwant %q", got, want)
	}
}

func TestURLHighlighter_URLWithQuery(t *testing.T) {
	h := newTestURLHighlighter()
	input := "https://example.com/search?q=test&limit=10"
	got := h.Highlight(input)

	want := proto("https") + sym("://") + host("example.com") + pth("/search") + qry("?q=test&limit=10")
	if got != want {
		t.Errorf("URLWithQuery:\ngot  %q\nwant %q", got, want)
	}
}

func TestURLHighlighter_URLInContext(t *testing.T) {
	h := newTestURLHighlighter()
	input := "request to https://api.example.com/v1 failed"
	got := h.Highlight(input)

	want := "request to " + proto("https") + sym("://") + host("api.example.com") + pth("/v1") + " failed"
	if got != want {
		t.Errorf("URLInContext:\ngot  %q\nwant %q", got, want)
	}
}

func TestURLHighlighter_NoURL(t *testing.T) {
	h := newTestURLHighlighter()
	input := "no url here at all"
	got := h.Highlight(input)

	if got != input {
		t.Errorf("NoURL: expected same string, got %q", got)
	}
}

func TestURLHighlighter_SamePointerOnNoMatch(t *testing.T) {
	h := newTestURLHighlighter()

	tests := []string{
		"plain text",
		"",
		"just some words",
		"has colons: but no slash-slash",
	}

	for _, input := range tests {
		got := h.Highlight(input)
		if got != input {
			t.Errorf("SamePointerOnNoMatch(%q): expected same string, got %q", input, got)
		}
	}
}

func TestURLHighlighter_MultipleURLs(t *testing.T) {
	h := newTestURLHighlighter()
	input := "see https://example.com and http://other.org/path for info"
	got := h.Highlight(input)

	want := "see " +
		proto("https") + sym("://") + host("example.com") +
		" and " +
		proto("http") + sym("://") + host("other.org") + pth("/path") +
		" for info"
	if got != want {
		t.Errorf("MultipleURLs:\ngot  %q\nwant %q", got, want)
	}
}

func TestURLHighlighter_HTTPOnly(t *testing.T) {
	h := newTestURLHighlighter()
	input := "http://localhost:8080/health"
	got := h.Highlight(input)

	want := proto("http") + sym("://") + host("localhost:8080") + pth("/health")
	if got != want {
		t.Errorf("HTTPOnly:\ngot  %q\nwant %q", got, want)
	}
}

func TestURLHighlighter_QueryWithoutPath(t *testing.T) {
	h := newTestURLHighlighter()
	input := "https://example.com?key=value"
	got := h.Highlight(input)

	want := proto("https") + sym("://") + host("example.com") + qry("?key=value")
	if got != want {
		t.Errorf("QueryWithoutPath:\ngot  %q\nwant %q", got, want)
	}
}
