package ui

import (
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestHighlightMatchesPlainText(t *testing.T) {
	re := regexp.MustCompile("run")
	result := HighlightMatches("running pod", re)
	stripped := ansi.Strip(result)
	if stripped != "running pod" {
		t.Fatalf("stripped result should be 'running pod', got %q", stripped)
	}
	if !strings.Contains(result, highlightOn) {
		t.Fatal("result should contain highlight ANSI code")
	}
}

func TestHighlightMatchesANSICell(t *testing.T) {
	// Simulate a colored status cell from pods/colors.go
	cell := "\x1b[38;2;80;250;123mRunning\x1b[39m"
	re := regexp.MustCompile("Run")
	result := HighlightMatches(cell, re)
	stripped := ansi.Strip(result)
	if stripped != "Running" {
		t.Fatalf("stripped result should be 'Running', got %q", stripped)
	}
	if !strings.Contains(result, highlightOn) {
		t.Fatal("result should contain highlight ANSI code")
	}
}

func TestHighlightMatchesNoMatch(t *testing.T) {
	re := regexp.MustCompile("xyz")
	result := HighlightMatches("Running", re)
	if result != "Running" {
		t.Fatalf("no match should return original unchanged, got %q", result)
	}
}

func TestHighlightMatchesNilRegex(t *testing.T) {
	result := HighlightMatches("anything", nil)
	if result != "anything" {
		t.Fatal("nil regex should return original unchanged")
	}
}

func TestHighlightMatchesMultiple(t *testing.T) {
	re := regexp.MustCompile("o")
	result := HighlightMatches("foo-pod", re)
	stripped := ansi.Strip(result)
	if stripped != "foo-pod" {
		t.Fatalf("stripped result should be 'foo-pod', got %q", stripped)
	}
	// Should have 3 highlights (two o's in "foo" and one in "pod")
	count := strings.Count(result, highlightOn)
	if count != 3 {
		t.Fatalf("expected 3 highlights, got %d", count)
	}
}

func TestHighlightMatchesEmptyString(t *testing.T) {
	re := regexp.MustCompile("test")
	result := HighlightMatches("", re)
	if result != "" {
		t.Fatalf("empty string should stay empty, got %q", result)
	}
}

func TestHighlightMatchesColorPlainText(t *testing.T) {
	re := regexp.MustCompile("run")
	result := HighlightMatchesColor("running pod", re)
	stripped := ansi.Strip(result)
	if stripped != "running pod" {
		t.Fatalf("stripped result should be 'running pod', got %q", stripped)
	}
	if !strings.Contains(result, highlightMatchOn) {
		t.Fatal("result should contain color highlight ANSI code")
	}
	if !strings.Contains(result, highlightMatchOff) {
		t.Fatal("result should contain color highlight off ANSI code")
	}
	// Must NOT contain reverse video codes
	if strings.Contains(result, highlightOn) {
		t.Fatal("color highlight must not use reverse video")
	}
}

func TestHighlightMatchesColorMultiple(t *testing.T) {
	re := regexp.MustCompile("o")
	result := HighlightMatchesColor("foo-pod", re)
	stripped := ansi.Strip(result)
	if stripped != "foo-pod" {
		t.Fatalf("stripped result should be 'foo-pod', got %q", stripped)
	}
	count := strings.Count(result, highlightMatchOn)
	if count != 3 {
		t.Fatalf("expected 3 color highlights, got %d", count)
	}
}

func TestHighlightMatchesColorNoMatch(t *testing.T) {
	re := regexp.MustCompile("xyz")
	result := HighlightMatchesColor("Running", re)
	if result != "Running" {
		t.Fatalf("no match should return original unchanged, got %q", result)
	}
}

func TestHighlightMatchesColorANSICell(t *testing.T) {
	cell := "\x1b[38;2;80;250;123mRunning\x1b[39m"
	re := regexp.MustCompile("Run")
	result := HighlightMatchesColor(cell, re)
	stripped := ansi.Strip(result)
	if stripped != "Running" {
		t.Fatalf("stripped result should be 'Running', got %q", stripped)
	}
	if !strings.Contains(result, highlightMatchOn) {
		t.Fatal("result should contain color highlight ANSI code")
	}
	// After highlight-off, the original green color must be re-emitted
	// so "ning" keeps its color.
	offIdx := strings.Index(result, highlightMatchOff)
	if offIdx < 0 {
		t.Fatal("result should contain highlight off code")
	}
	after := result[offIdx+len(highlightMatchOff):]
	if !strings.HasPrefix(after, "\x1b[38;2;80;250;123m") {
		t.Fatalf("color should be restored after highlight-off, got %q", after)
	}
}

func TestColorToSGR(t *testing.T) {
	tests := []struct {
		color string
		bg    bool
		want  string
	}{
		{"43", true, "48;5;43"},
		{"0", false, "38;5;0"},
		{"#FF5555", true, "48;2;255;85;85"},
		{"#50FA7B", false, "38;2;80;250;123"},
		{"#GGHHII", true, "48;5;1"},  // invalid hex falls back to red
		{"#ZZZZZZ", false, "38;5;1"}, // invalid hex falls back to red
	}
	for _, tt := range tests {
		got := colorToSGR(tt.color, tt.bg)
		if got != tt.want {
			t.Errorf("colorToSGR(%q, %v) = %q, want %q", tt.color, tt.bg, got, tt.want)
		}
	}
}
