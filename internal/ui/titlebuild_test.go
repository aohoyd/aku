package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestTruncatePatternShort(t *testing.T) {
	if got := truncatePattern("nginx", 20); got != "nginx" {
		t.Fatalf("short pattern should be unchanged, got %q", got)
	}
}

func TestTruncatePatternLong(t *testing.T) {
	long := strings.Repeat("a", 25)
	result := truncatePattern(long, 20)
	runes := []rune(result)
	if len(runes) != 20 {
		t.Fatalf("expected 20 runes, got %d", len(runes))
	}
	if !strings.HasSuffix(result, "…") {
		t.Fatalf("expected trailing ellipsis, got %q", result)
	}
}

func TestTruncatePatternZero(t *testing.T) {
	if got := truncatePattern("hello", 0); got != "" {
		t.Fatalf("expected empty for zero max, got %q", got)
	}
}

func TestBuildPatternSuffixBothInactive(t *testing.T) {
	if got := buildPatternSuffix("", ""); got != "" {
		t.Fatalf("expected empty suffix, got %q", got)
	}
}

func TestBuildPatternSuffixFilterOnly(t *testing.T) {
	got := buildPatternSuffix("nginx", "")
	if !strings.Contains(got, "|nginx|") {
		t.Fatalf("expected |nginx|, got %q", got)
	}
	if strings.Contains(got, "/") {
		t.Fatalf("should not contain search delimiter, got %q", got)
	}
}

func TestBuildPatternSuffixSearchOnly(t *testing.T) {
	got := buildPatternSuffix("", "err")
	if !strings.Contains(got, "/err/") {
		t.Fatalf("expected /err/, got %q", got)
	}
	if strings.Contains(got, "|") {
		t.Fatalf("should not contain filter delimiter, got %q", got)
	}
}

func TestBuildPatternSuffixBothActive(t *testing.T) {
	got := buildPatternSuffix("nginx", "err")
	if !strings.Contains(got, "|nginx|") || !strings.Contains(got, "/err/") {
		t.Fatalf("expected both indicators, got %q", got)
	}
	filterIdx := strings.Index(got, "|")
	searchIdx := strings.Index(got, "/")
	if filterIdx > searchIdx {
		t.Fatalf("filter should precede search, got %q", got)
	}
}

func TestBuildPanelTitleNoPatterns(t *testing.T) {
	result := BuildPanelTitle(" pods (5)", "", "", 60, "")
	stripped := ansi.Strip(result)
	if !strings.Contains(stripped, "pods (5)") {
		t.Fatalf("base title should be present, got %q", stripped)
	}
	if strings.Contains(stripped, "|") || strings.Contains(stripped, "/") {
		t.Fatalf("no delimiters expected, got %q", stripped)
	}
}

func TestBuildPanelTitleFilterActive(t *testing.T) {
	result := BuildPanelTitle(" pods (3/12)", "nginx", "", 80, "")
	stripped := ansi.Strip(result)
	if !strings.Contains(stripped, "pods (3/12)") {
		t.Fatalf("base title should contain count, got %q", stripped)
	}
	if !strings.Contains(stripped, "|nginx|") {
		t.Fatalf("filter pattern should appear, got %q", stripped)
	}
}

func TestBuildPanelTitleSearchActive(t *testing.T) {
	result := BuildPanelTitle("YAML", "", "err", 80, "")
	stripped := ansi.Strip(result)
	if !strings.Contains(stripped, "YAML") {
		t.Fatalf("base title should be present, got %q", stripped)
	}
	if !strings.Contains(stripped, "/err/") {
		t.Fatalf("search pattern should appear, got %q", stripped)
	}
}

func TestBuildPanelTitleBothActive(t *testing.T) {
	result := BuildPanelTitle(" pods (3/12)", "nginx", "err", 80, "")
	stripped := ansi.Strip(result)
	if !strings.Contains(stripped, "|nginx|") {
		t.Fatalf("filter should appear, got %q", stripped)
	}
	if !strings.Contains(stripped, "/err/") {
		t.Fatalf("search should appear, got %q", stripped)
	}
}

func TestBuildPanelTitleNarrowPanelSuppressesIndicators(t *testing.T) {
	result := BuildPanelTitle(" pods (1)", "nginx", "err", 15, "")
	stripped := ansi.Strip(result)
	if strings.Contains(stripped, "|nginx|") {
		t.Fatal("indicators should be suppressed on narrow panel")
	}
}

func TestBuildPanelTitleLongPatternTruncated(t *testing.T) {
	longPattern := strings.Repeat("x", 50)
	result := BuildPanelTitle(" pods (1)", longPattern, "", 80, "")
	stripped := ansi.Strip(result)
	if strings.Contains(stripped, longPattern) {
		t.Fatal("full long pattern should have been truncated")
	}
	if !strings.Contains(stripped, "…") {
		t.Fatalf("expected ellipsis for truncated pattern, got %q", stripped)
	}
}

func TestBuildPanelTitleVeryNarrow(t *testing.T) {
	result := BuildPanelTitle("X", "nginx", "", 3, "")
	if result == "" {
		t.Fatal("very narrow panel should still return something")
	}
}

func TestBuildPanelTitleInlineInput(t *testing.T) {
	result := BuildPanelTitle(" pods (5)", "nginx", "err", 80, "/typing█")
	stripped := ansi.Strip(result)
	if !strings.Contains(stripped, "pods (5)") {
		t.Fatalf("base title should be present, got %q", stripped)
	}
	if !strings.Contains(stripped, "/typing█") {
		t.Fatalf("inline input should appear, got %q", stripped)
	}
	if strings.Contains(stripped, "|nginx|") {
		t.Fatal("static filter suffix should not appear when inline input is active")
	}
	if strings.Contains(stripped, "/err/") {
		t.Fatal("static search suffix should not appear when inline input is active")
	}
}

func TestBuildPanelTitleEmptyInlineInputUsesStaticSuffix(t *testing.T) {
	result := BuildPanelTitle(" pods (5)", "nginx", "", 80, "")
	stripped := ansi.Strip(result)
	if !strings.Contains(stripped, "|nginx|") {
		t.Fatalf("static filter suffix should appear when inline input is empty, got %q", stripped)
	}
}

func TestBuildPanelTitleWithPrefixInlineInput(t *testing.T) {
	result := BuildPanelTitleWithPrefix(" default >", " pods (5)", "", "", 80, "/search█")
	stripped := ansi.Strip(result)
	if !strings.Contains(stripped, "default >") {
		t.Fatalf("prefix should be present, got %q", stripped)
	}
	if !strings.Contains(stripped, "/search█") {
		t.Fatalf("inline input should appear, got %q", stripped)
	}
}
