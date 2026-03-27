package highlight

import (
	"strings"
	"testing"
)

// testHighlighter wraps every occurrence of a target substring with brackets.
type testHighlighter struct {
	target  string
	replace string
}

func (h testHighlighter) Highlight(line string) string {
	// Must return same pointer when no match (contract).
	idx := indexOf(line, h.target)
	if idx == -1 {
		return line
	}
	var sb strings.Builder
	for {
		idx = indexOf(line, h.target)
		if idx == -1 {
			sb.WriteString(line)
			break
		}
		sb.WriteString(line[:idx])
		sb.WriteString(h.replace)
		line = line[idx+len(h.target):]
	}
	return sb.String()
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func TestApplyToUnhighlighted_NoANSI(t *testing.T) {
	h := testHighlighter{target: "foo", replace: "[FOO]"}
	line := "hello foo world foo"
	got := ApplyToUnhighlighted(line, h)
	want := "hello [FOO] world [FOO]"
	if got != want {
		t.Errorf("NoANSI:\ngot  %q\nwant %q", got, want)
	}
}

func TestApplyToUnhighlighted_SingleStyledRegion(t *testing.T) {
	h := testHighlighter{target: "foo", replace: "[FOO]"}
	// "foo \x1b[31mstyled\x1b[0m foo"
	// The "foo" before and after styled should be highlighted; "styled" should not.
	line := "foo \x1b[31mstyled\x1b[0m foo"
	got := ApplyToUnhighlighted(line, h)
	want := "[FOO] \x1b[31mstyled\x1b[0m [FOO]"
	if got != want {
		t.Errorf("SingleStyledRegion:\ngot  %q\nwant %q", got, want)
	}
}

func TestApplyToUnhighlighted_MultipleStyledRegions(t *testing.T) {
	h := testHighlighter{target: "x", replace: "[X]"}
	// "x\x1b[31mR\x1b[0mx\x1b[32mG\x1b[0mx"
	line := "x\x1b[31mR\x1b[0mx\x1b[32mG\x1b[0mx"
	got := ApplyToUnhighlighted(line, h)
	want := "[X]\x1b[31mR\x1b[0m[X]\x1b[32mG\x1b[0m[X]"
	if got != want {
		t.Errorf("MultipleStyledRegions:\ngot  %q\nwant %q", got, want)
	}
}

func TestApplyToUnhighlighted_NoMatch(t *testing.T) {
	h := testHighlighter{target: "zzz", replace: "[ZZZ]"}
	line := "hello world"
	got := ApplyToUnhighlighted(line, h)
	// Should return same pointer (no alloc)
	if got != line {
		t.Errorf("NoMatch: expected same string pointer, got %q", got)
	}
}

func TestApplyToUnhighlighted_NoMatchWithANSI(t *testing.T) {
	h := testHighlighter{target: "zzz", replace: "[ZZZ]"}
	line := "hello \x1b[31mred\x1b[0m world"
	got := ApplyToUnhighlighted(line, h)
	// Should return same pointer since no plain segments matched
	if got != line {
		t.Errorf("NoMatchWithANSI: expected same string pointer, got %q", got)
	}
}

func TestApplyToUnhighlighted_MalformedSequence(t *testing.T) {
	h := testHighlighter{target: "foo", replace: "[FOO]"}
	// Malformed: \x1b without [ — should be treated as plain text
	line := "foo\x1bXbar"
	got := ApplyToUnhighlighted(line, h)
	want := "[FOO]\x1bXbar"
	if got != want {
		t.Errorf("MalformedSequence:\ngot  %q\nwant %q", got, want)
	}
}

func TestApplyToUnhighlighted_UnclosedStyledRegion(t *testing.T) {
	h := testHighlighter{target: "foo", replace: "[FOO]"}
	// Styled region never closed — everything after SGR is styled
	line := "foo \x1b[31mstyled text foo"
	got := ApplyToUnhighlighted(line, h)
	want := "[FOO] \x1b[31mstyled text foo"
	if got != want {
		t.Errorf("UnclosedStyledRegion:\ngot  %q\nwant %q", got, want)
	}
}

func TestApplyToUnhighlighted_EmptyLine(t *testing.T) {
	h := testHighlighter{target: "foo", replace: "[FOO]"}
	line := ""
	got := ApplyToUnhighlighted(line, h)
	if got != "" {
		t.Errorf("EmptyLine: expected empty, got %q", got)
	}
}

func TestApplyToUnhighlighted_OnlyANSI(t *testing.T) {
	h := testHighlighter{target: "foo", replace: "[FOO]"}
	line := "\x1b[31mred\x1b[0m"
	got := ApplyToUnhighlighted(line, h)
	// No plain segments to highlight, should return original
	if got != line {
		t.Errorf("OnlyANSI: expected same string pointer, got %q", got)
	}
}

func TestApplyToUnhighlighted_ResetWithoutContent(t *testing.T) {
	h := testHighlighter{target: "ab", replace: "[AB]"}
	// \x1b[0m at start (reset outside styled region) then plain text
	line := "\x1b[0mab"
	got := ApplyToUnhighlighted(line, h)
	want := "\x1b[0m[AB]"
	if got != want {
		t.Errorf("ResetWithoutContent:\ngot  %q\nwant %q", got, want)
	}
}
