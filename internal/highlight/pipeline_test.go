package highlight

import (
	"strings"
	"testing"
)

func TestPipeline_Empty(t *testing.T) {
	p := NewPipelineBuilder().Build()
	line := "hello world"
	got := p.Highlight(line)
	if got != line {
		t.Errorf("Empty pipeline: expected same string, got %q", got)
	}
}

func TestPipeline_Nil(t *testing.T) {
	var p *Pipeline
	line := "hello world"
	got := p.Highlight(line)
	if got != line {
		t.Errorf("Nil pipeline: expected same string, got %q", got)
	}
}

func TestPipeline_SingleGuarded(t *testing.T) {
	h := testHighlighter{target: "foo", replace: "\x1b[31mfoo\x1b[0m"}
	p := NewPipelineBuilder().Add(h).Build()
	line := "hello foo world"
	got := p.Highlight(line)
	want := "hello \x1b[31mfoo\x1b[0m world"
	if got != want {
		t.Errorf("SingleGuarded:\ngot  %q\nwant %q", got, want)
	}
}

func TestPipeline_SingleRaw(t *testing.T) {
	h := testHighlighter{target: "foo", replace: "\x1b[31mfoo\x1b[0m"}
	p := NewPipelineBuilder().AddRaw(h).Build()
	line := "hello foo world"
	got := p.Highlight(line)
	want := "hello \x1b[31mfoo\x1b[0m world"
	if got != want {
		t.Errorf("SingleRaw:\ngot  %q\nwant %q", got, want)
	}
}

func TestPipeline_GuardedSkipsPreviousANSI(t *testing.T) {
	// First highlighter wraps "foo" in red ANSI.
	h1 := testHighlighter{target: "foo", replace: "\x1b[31mfoo\x1b[0m"}
	// Second highlighter targets "foo" — but as a guarded step, it should NOT
	// see the "foo" inside the ANSI-wrapped region from h1.
	h2 := testHighlighter{target: "foo", replace: "\x1b[32mfoo\x1b[0m"}

	p := NewPipelineBuilder().Add(h1).Add(h2).Build()
	line := "foo bar"
	got := p.Highlight(line)
	// h1 wraps "foo" in red; h2 is guarded and skips the styled region.
	want := "\x1b[31mfoo\x1b[0m bar"
	if got != want {
		t.Errorf("GuardedSkipsPreviousANSI:\ngot  %q\nwant %q", got, want)
	}
}

func TestPipeline_RawSeesFullLine(t *testing.T) {
	// First highlighter wraps "foo" in red ANSI.
	h1 := testHighlighter{target: "foo", replace: "\x1b[31mfoo\x1b[0m"}
	// rawCounter counts occurrences of \x1b in the line it receives.
	// A raw step should see the ANSI codes from h1.
	p := NewPipelineBuilder().Add(h1).AddRaw(rawEscCounter{}).Build()
	line := "foo bar"
	got := p.Highlight(line)
	// h1 produces "\x1b[31mfoo\x1b[0m bar"
	// rawEscCounter is a raw step — it sees the full line and prepends "ESC:2 "
	// (two \x1b characters from h1's output).
	want := "ESC:2 \x1b[31mfoo\x1b[0m bar"
	if got != want {
		t.Errorf("RawSeesFullLine:\ngot  %q\nwant %q", got, want)
	}
}

func TestPipeline_NoChange(t *testing.T) {
	h := testHighlighter{target: "zzz", replace: "[ZZZ]"}
	p := NewPipelineBuilder().Add(h).Build()
	line := "hello world"
	got := p.Highlight(line)
	// No highlighter matched, should return original string.
	if got != line {
		t.Errorf("NoChange: expected same string pointer, got %q", got)
	}
}

// rawEscCounter is a test Highlighter that counts \x1b bytes in the input
// and prepends "ESC:N " to the line. It always modifies the line.
type rawEscCounter struct{}

func (rawEscCounter) Highlight(line string) string {
	count := strings.Count(line, "\x1b")
	var sb strings.Builder
	sb.WriteString("ESC:")
	sb.WriteByte(byte('0' + count))
	sb.WriteByte(' ')
	sb.WriteString(line)
	return sb.String()
}
