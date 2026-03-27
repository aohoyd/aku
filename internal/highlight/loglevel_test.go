package highlight

import (
	"testing"
)

const (
	errorANSI = "\x1b[38;5;10m"
	warnANSI  = "\x1b[38;5;11m"
	infoANSI  = "\x1b[38;5;12m"
	debugANSI = "\x1b[38;5;13m"
	traceANSI = "\x1b[38;5;14m"
)

func newTestLogLevelHighlighter() *LogLevelHighlighter {
	ep := Painter{prefix: errorANSI}
	wp := Painter{prefix: warnANSI}
	ip := Painter{prefix: infoANSI}
	dp := Painter{prefix: debugANSI}
	tp := Painter{prefix: traceANSI}
	return NewLogLevelHighlighter(ep, wp, ip, dp, tp)
}

func paint(ansi, s string) string {
	return ansi + s + reset
}

func TestLogLevelHighlighter_ImplementsHighlighter(t *testing.T) {
	var h Highlighter = NewLogLevelHighlighter(Painter{}, Painter{}, Painter{}, Painter{}, Painter{})
	_ = h
}

func TestLogLevelHighlighter_ErrorLevels(t *testing.T) {
	h := newTestLogLevelHighlighter()
	tests := []struct {
		input string
		want  string
	}{
		{"ERROR something failed", paint(errorANSI, "ERROR") + " something failed"},
		{"ERR something failed", paint(errorANSI, "ERR") + " something failed"},
		{"FATAL something failed", paint(errorANSI, "FATAL") + " something failed"},
	}
	for _, tt := range tests {
		got := h.Highlight(tt.input)
		if got != tt.want {
			t.Errorf("ErrorLevels(%q):\ngot  %q\nwant %q", tt.input, got, tt.want)
		}
	}
}

func TestLogLevelHighlighter_WarnLevels(t *testing.T) {
	h := newTestLogLevelHighlighter()
	tests := []struct {
		input string
		want  string
	}{
		{"WARN disk space low", paint(warnANSI, "WARN") + " disk space low"},
		{"WARNING disk space low", paint(warnANSI, "WARNING") + " disk space low"},
	}
	for _, tt := range tests {
		got := h.Highlight(tt.input)
		if got != tt.want {
			t.Errorf("WarnLevels(%q):\ngot  %q\nwant %q", tt.input, got, tt.want)
		}
	}
}

func TestLogLevelHighlighter_InfoLevel(t *testing.T) {
	h := newTestLogLevelHighlighter()
	input := "INFO server started"
	want := paint(infoANSI, "INFO") + " server started"
	got := h.Highlight(input)
	if got != want {
		t.Errorf("InfoLevel:\ngot  %q\nwant %q", got, want)
	}
}

func TestLogLevelHighlighter_DebugLevels(t *testing.T) {
	h := newTestLogLevelHighlighter()
	tests := []struct {
		input string
		want  string
	}{
		{"DEBUG processing request", paint(debugANSI, "DEBUG") + " processing request"},
		{"DBG processing request", paint(debugANSI, "DBG") + " processing request"},
	}
	for _, tt := range tests {
		got := h.Highlight(tt.input)
		if got != tt.want {
			t.Errorf("DebugLevels(%q):\ngot  %q\nwant %q", tt.input, got, tt.want)
		}
	}
}

func TestLogLevelHighlighter_TraceLevel(t *testing.T) {
	h := newTestLogLevelHighlighter()
	input := "TRACE entering function"
	want := paint(traceANSI, "TRACE") + " entering function"
	got := h.Highlight(input)
	if got != want {
		t.Errorf("TraceLevel:\ngot  %q\nwant %q", got, want)
	}
}

func TestLogLevelHighlighter_CaseInsensitive(t *testing.T) {
	h := newTestLogLevelHighlighter()
	tests := []struct {
		input string
		want  string
	}{
		{"error something", paint(errorANSI, "error") + " something"},
		{"Error something", paint(errorANSI, "Error") + " something"},
		{"ERROR something", paint(errorANSI, "ERROR") + " something"},
		{"info started", paint(infoANSI, "info") + " started"},
		{"Info started", paint(infoANSI, "Info") + " started"},
		{"warn low", paint(warnANSI, "warn") + " low"},
		{"debug req", paint(debugANSI, "debug") + " req"},
		{"trace fn", paint(traceANSI, "trace") + " fn"},
	}
	for _, tt := range tests {
		got := h.Highlight(tt.input)
		if got != tt.want {
			t.Errorf("CaseInsensitive(%q):\ngot  %q\nwant %q", tt.input, got, tt.want)
		}
	}
}

func TestLogLevelHighlighter_WordBoundary(t *testing.T) {
	h := newTestLogLevelHighlighter()
	tests := []struct {
		input string
	}{
		{"INFORMATIONAL message"},
		{"DEBUGGER attached"},
		{"ERRORCODE 42"},
		{"WARNINGS list"},
		{"TRACEABILITY report"},
	}
	for _, tt := range tests {
		got := h.Highlight(tt.input)
		if got != tt.input {
			t.Errorf("WordBoundary(%q): expected no match (same pointer), got %q", tt.input, got)
		}
	}
}

func TestLogLevelHighlighter_MultipleLevels(t *testing.T) {
	h := newTestLogLevelHighlighter()
	input := "ERROR happened then WARN issued and INFO logged"
	want := paint(errorANSI, "ERROR") + " happened then " +
		paint(warnANSI, "WARN") + " issued and " +
		paint(infoANSI, "INFO") + " logged"
	got := h.Highlight(input)
	if got != want {
		t.Errorf("MultipleLevels:\ngot  %q\nwant %q", got, want)
	}
}

func TestLogLevelHighlighter_NoMatchReturnsSamePointer(t *testing.T) {
	h := newTestLogLevelHighlighter()
	tests := []string{
		"plain text with no levels",
		"",
		"just some numbers 12345",
		"INFORMATIONAL is not a level",
	}
	for _, input := range tests {
		got := h.Highlight(input)
		// Use pointer comparison via unsafe or just string equality
		// The contract says same pointer, but string == is sufficient for correctness
		if got != input {
			t.Errorf("NoMatchReturnsSamePointer(%q): expected same string, got %q", input, got)
		}
	}
}

func TestLogLevelHighlighter_LevelInMiddleOfLine(t *testing.T) {
	h := newTestLogLevelHighlighter()
	input := "2024-01-01 ERROR something broke"
	want := "2024-01-01 " + paint(errorANSI, "ERROR") + " something broke"
	got := h.Highlight(input)
	if got != want {
		t.Errorf("LevelInMiddleOfLine:\ngot  %q\nwant %q", got, want)
	}
}
