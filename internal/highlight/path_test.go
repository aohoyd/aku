package highlight

import (
	"strings"
	"testing"
)

const (
	pathSegANSI = "\x1b[38;5;4m"
	pathSepANSI = "\x1b[38;5;8m"
)

func newTestPathHighlighter() *PathHighlighter {
	seg := Painter{prefix: pathSegANSI}
	sep := Painter{prefix: pathSepANSI}
	return NewPathHighlighter(seg, sep)
}

// pseg wraps s with the segment painter ANSI codes.
func pseg(s string) string {
	return pathSegANSI + s + reset
}

// psep wraps s with the separator painter ANSI codes.
func psep(s string) string {
	return pathSepANSI + s + reset
}

// ppathLeadingSlash builds a path that starts with "/" followed by segments.
func ppathLeadingSlash(parts ...string) string {
	var sb strings.Builder
	sb.WriteString(psep("/"))
	for i, part := range parts {
		if i > 0 {
			sb.WriteString(psep("/"))
		}
		sb.WriteString(pseg(part))
	}
	return sb.String()
}

func TestPathHighlighter_ImplementsHighlighter(t *testing.T) {
	var h Highlighter = NewPathHighlighter(Painter{}, Painter{})
	_ = h
}

func TestPathHighlighter_Absolute(t *testing.T) {
	h := newTestPathHighlighter()
	input := "/usr/local/bin"
	got := h.Highlight(input)
	want := ppathLeadingSlash("usr", "local", "bin")
	if got != want {
		t.Errorf("Absolute:\ngot  %q\nwant %q", got, want)
	}
}

func TestPathHighlighter_Relative(t *testing.T) {
	h := newTestPathHighlighter()
	input := "./config/app.yaml"
	got := h.Highlight(input)
	want := pseg(".") + psep("/") + pseg("config") + psep("/") + pseg("app.yaml")
	if got != want {
		t.Errorf("Relative:\ngot  %q\nwant %q", got, want)
	}
}

func TestPathHighlighter_Home(t *testing.T) {
	h := newTestPathHighlighter()
	input := "~/Documents/file.txt"
	got := h.Highlight(input)
	want := pseg("~") + psep("/") + pseg("Documents") + psep("/") + pseg("file.txt")
	if got != want {
		t.Errorf("Home:\ngot  %q\nwant %q", got, want)
	}
}

func TestPathHighlighter_SingleComponentNoMatch(t *testing.T) {
	h := newTestPathHighlighter()
	input := "/usr"
	got := h.Highlight(input)
	if got != input {
		t.Errorf("SingleComponentNoMatch: expected same string, got %q", got)
	}
}

func TestPathHighlighter_PathInContext(t *testing.T) {
	h := newTestPathHighlighter()
	input := "reading /etc/config/app.conf failed"
	got := h.Highlight(input)
	want := "reading " + ppathLeadingSlash("etc", "config", "app.conf") + " failed"
	if got != want {
		t.Errorf("PathInContext:\ngot  %q\nwant %q", got, want)
	}
}

func TestPathHighlighter_NoSlash(t *testing.T) {
	h := newTestPathHighlighter()
	input := "no slash here at all"
	got := h.Highlight(input)
	if got != input {
		t.Errorf("NoSlash: expected same string, got %q", got)
	}
}

func TestPathHighlighter_PathWithDots(t *testing.T) {
	h := newTestPathHighlighter()
	input := "/path/to/file.go"
	got := h.Highlight(input)
	want := ppathLeadingSlash("path", "to", "file.go")
	if got != want {
		t.Errorf("PathWithDots:\ngot  %q\nwant %q", got, want)
	}
}

func TestPathHighlighter_SamePointerOnNoMatch(t *testing.T) {
	h := newTestPathHighlighter()

	tests := []string{
		"plain text",
		"",
		"no paths here",
		"just a single /usr component",
	}

	for _, input := range tests {
		got := h.Highlight(input)
		if got != input {
			t.Errorf("SamePointerOnNoMatch(%q): expected same string, got %q", input, got)
		}
	}
}

func TestPathHighlighter_MultiplePaths(t *testing.T) {
	h := newTestPathHighlighter()
	input := "copy /usr/local/bin to /tmp/backup"
	got := h.Highlight(input)
	want := "copy " + ppathLeadingSlash("usr", "local", "bin") + " to " + ppathLeadingSlash("tmp", "backup")
	if got != want {
		t.Errorf("MultiplePaths:\ngot  %q\nwant %q", got, want)
	}
}

func TestPathHighlighter_NetworkPath(t *testing.T) {
	h := newTestPathHighlighter()
	input := "mount //server/share"
	got := h.Highlight(input)
	want := "mount " + psep("/") + psep("/") + pseg("server") + psep("/") + pseg("share")
	if got != want {
		t.Errorf("NetworkPath:\ngot  %q\nwant %q", got, want)
	}
}
