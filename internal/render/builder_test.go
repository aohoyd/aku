package render

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestBuilderStripInvariant(t *testing.T) {
	b := NewBuilder()
	b.KV(LEVEL_0, "Name", "test-pod")
	b.KVStyled(LEVEL_0, ValueStatusOK, "Status", "Running")
	b.Section(LEVEL_0, "Containers")
	b.KV(LEVEL_2, "Image", "nginx:1.25")
	b.RawLine(LEVEL_1, "----")
	c := b.Build()
	stripped := ansi.Strip(c.Display)
	if stripped != c.Raw {
		t.Errorf("ansi.Strip(display) != raw\nstripped: %q\nraw:      %q", stripped, c.Raw)
	}
}

func TestBuilderLineCountInvariant(t *testing.T) {
	b := NewBuilder()
	b.KV(LEVEL_0, "Name", "test-pod")
	b.KVStyled(LEVEL_0, ValueStatusOK, "Status", "Running")
	b.Section(LEVEL_0, "Containers")
	b.KV(LEVEL_2, "Image", "nginx:1.25")
	b.RawLine(LEVEL_1, "plain line")
	c := b.Build()
	rawLines := strings.Count(c.Raw, "\n")
	displayLines := strings.Count(c.Display, "\n")
	if rawLines != displayLines {
		t.Errorf("line count mismatch: raw=%d, display=%d", rawLines, displayLines)
	}
}

func TestBuilderPerSectionAlignment(t *testing.T) {
	b := NewBuilder()
	b.KV(LEVEL_0, "Name", "test-pod")
	b.KV(LEVEL_0, "Service Account", "default")
	b.Section(LEVEL_0, "Containers")
	b.KV(LEVEL_2, "Image", "nginx:1.25")
	b.KV(LEVEL_2, "Port", "80/TCP")
	raw := b.Build().Raw
	lines := strings.Split(strings.TrimRight(raw, "\n"), "\n")

	nameValCol := strings.Index(lines[0], "test-pod")
	saValCol := strings.Index(lines[1], "default")
	if nameValCol != saValCol {
		t.Errorf("preamble values not aligned: Name at col %d, Service Account at col %d", nameValCol, saValCol)
	}

	imageValCol := strings.Index(lines[3], "nginx:1.25")
	portValCol := strings.Index(lines[4], "80/TCP")
	if imageValCol != portValCol {
		t.Errorf("container values not aligned: Image at col %d, Port at col %d", imageValCol, portValCol)
	}

	if nameValCol == imageValCol {
		t.Error("preamble and container sections should have different alignment")
	}
}

func TestBuilderMultiValueKV(t *testing.T) {
	b := NewBuilder()
	b.KVMulti(LEVEL_0, "Labels", map[string]string{"app": "nginx", "env": "prod", "tier": "frontend"})
	raw := b.Build().Raw
	lines := strings.Split(strings.TrimRight(raw, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d: %q", len(lines), raw)
	}
	if !strings.Contains(lines[0], "Labels:") || !strings.Contains(lines[0], "app=nginx") {
		t.Errorf("first line should have key and first value, got: %q", lines[0])
	}
	firstValCol := strings.Index(lines[0], "app=nginx")
	secondValCol := strings.Index(lines[1], "env=prod")
	thirdValCol := strings.Index(lines[2], "tier=frontend")
	if firstValCol != secondValCol || secondValCol != thirdValCol {
		t.Errorf("continuation values not aligned: cols %d, %d, %d", firstValCol, secondValCol, thirdValCol)
	}
}

func TestBuilderUnalignedExcluded(t *testing.T) {
	b := NewBuilder()
	b.KV(LEVEL_0, "A", "val")
	b.KV(LEVEL_0, "A Very Long Key That Is Huge", "x", Unaligned())
	raw := b.Build().Raw
	lines := strings.Split(strings.TrimRight(raw, "\n"), "\n")
	valCol := strings.Index(lines[0], "val")
	if valCol > 10 {
		t.Errorf("unaligned KV affected alignment: 'val' at col %d (should be small)", valCol)
	}
	if !strings.Contains(lines[1], "A Very Long Key That Is Huge x") {
		t.Errorf("unaligned KV should be key+space+value, got: %q", lines[1])
	}
}

func TestBuilderEmptyBuild(t *testing.T) {
	b := NewBuilder()
	c := b.Build()
	if c.Raw != "" || c.Display != "" {
		t.Errorf("empty builder should produce empty output, got raw=%q display=%q", c.Raw, c.Display)
	}
}

func TestBuilderSingleKV(t *testing.T) {
	b := NewBuilder()
	b.KV(LEVEL_0, "Name", "test")
	c := b.Build()
	if !strings.Contains(c.Raw, "Name:") || !strings.Contains(c.Raw, "test") {
		t.Errorf("expected Name: test in raw, got: %q", c.Raw)
	}
	stripped := ansi.Strip(c.Display)
	if stripped != c.Raw {
		t.Errorf("strip invariant failed for single KV")
	}
}

func TestBuilderSectionRendersColon(t *testing.T) {
	b := NewBuilder()
	b.Section(LEVEL_0, "Containers")
	raw := b.Build().Raw
	if !strings.Contains(raw, "Containers:\n") {
		t.Errorf("section should render as 'Containers:\\n', got: %q", raw)
	}
}

func TestBuilderRawLine(t *testing.T) {
	b := NewBuilder()
	b.RawLine(LEVEL_1, "----")
	c := b.Build()
	if !strings.Contains(c.Raw, "  ----\n") {
		t.Errorf("expected indented raw line, got: %q", c.Raw)
	}
	if c.Raw != c.Display {
		t.Errorf("RawLine should have identical raw and display, got raw=%q display=%q", c.Raw, c.Display)
	}
}

func TestBuilderWithKindOption(t *testing.T) {
	b := NewBuilder()
	b.KV(LEVEL_0, "Status", "Running", WithKind(ValueStatusOK))
	c := b.Build()
	if c.Raw == c.Display {
		t.Error("styled KV should have ANSI codes in display")
	}
	if !strings.Contains(c.Display, "\x1b[") {
		t.Error("display should contain ANSI escape sequences")
	}
}

func TestBuilderLevels(t *testing.T) {
	tests := []struct {
		level      int
		wantIndent string
	}{
		{LEVEL_0, ""},
		{LEVEL_1, "  "},
		{LEVEL_2, "    "},
		{LEVEL_3, "      "},
		{LEVEL_4, "        "},
	}
	for _, tt := range tests {
		b := NewBuilder()
		b.KV(tt.level, "Key", "Value")
		raw := b.Build().Raw
		if !strings.HasPrefix(raw, tt.wantIndent+"Key:") {
			t.Errorf("level %d: expected prefix %q, got line: %q", tt.level, tt.wantIndent+"Key:", raw)
		}
	}
}

func TestBuilderKVNoTab(t *testing.T) {
	b := NewBuilder()
	b.KV(LEVEL_0, "Name", "test-pod")
	raw := b.Build().Raw
	if strings.Contains(raw, "\t") {
		t.Fatal("raw output should not contain literal tab")
	}
}

func TestBuilderKVAutoColon(t *testing.T) {
	b := NewBuilder()
	b.KV(LEVEL_0, "Name", "test-pod")
	raw := b.Build().Raw
	if !strings.Contains(raw, "Name:") {
		t.Errorf("aligned KV should auto-append colon, got: %q", raw)
	}
}

func TestBuilderUnalignedNoAutoColon(t *testing.T) {
	b := NewBuilder()
	b.KV(LEVEL_0, "/var/data", "from vol (rw)", Unaligned())
	raw := b.Build().Raw
	if strings.Contains(raw, "/var/data:") {
		t.Errorf("unaligned KV should not auto-append colon, got: %q", raw)
	}
	if !strings.Contains(raw, "/var/data from vol (rw)") {
		t.Errorf("expected '/var/data from vol (rw)', got: %q", raw)
	}
}

func TestKVMultiSorted(t *testing.T) {
	b := NewBuilder()
	b.KVMulti(LEVEL_0, "Labels", map[string]string{"app": "nginx", "env": "prod"})
	raw := b.Build().Raw
	if !strings.Contains(raw, "app=nginx") {
		t.Fatalf("expected label in output, got:\n%s", raw)
	}
	if !strings.Contains(raw, "env=prod") {
		t.Fatalf("expected label in output, got:\n%s", raw)
	}
	if !strings.Contains(raw, "Labels:") {
		t.Fatalf("expected 'Labels:' title, got:\n%s", raw)
	}
	appIdx := strings.Index(raw, "app=nginx")
	envIdx := strings.Index(raw, "env=prod")
	if appIdx > envIdx {
		t.Fatalf("labels should be sorted, got app at %d and env at %d", appIdx, envIdx)
	}
}

func TestKVMultiEmpty(t *testing.T) {
	b := NewBuilder()
	b.KVMulti(LEVEL_0, "Labels", nil)
	raw := b.Build().Raw
	if !strings.Contains(raw, "Labels:") {
		t.Fatalf("expected 'Labels:' even for nil map, got:\n%s", raw)
	}
	if !strings.Contains(raw, "<none>") {
		t.Fatalf("expected '<none>' for nil map, got:\n%s", raw)
	}
}

func TestKVMultiAtLevel(t *testing.T) {
	b := NewBuilder()
	b.KVMulti(LEVEL_0, "Annotations", map[string]string{
		"kubectl.kubernetes.io/last-applied-configuration": "...",
		"app.kubernetes.io/name":                           "test",
	})
	raw := b.Build().Raw
	if !strings.Contains(raw, "Annotations:") {
		t.Fatalf("expected Annotations title, got:\n%s", raw)
	}
	appIdx := strings.Index(raw, "app.kubernetes.io/name=test")
	kubectlIdx := strings.Index(raw, "kubectl.kubernetes.io/last-applied-configuration=...")
	if appIdx > kubectlIdx {
		t.Fatalf("annotations should be sorted, got app at %d, kubectl at %d", appIdx, kubectlIdx)
	}
}

func TestKVMultiEmptyMap(t *testing.T) {
	b := NewBuilder()
	b.KVMulti(LEVEL_0, "Annotations", map[string]string{})
	raw := b.Build().Raw
	if !strings.Contains(raw, "Annotations:") {
		t.Fatalf("expected Annotations title, got:\n%s", raw)
	}
	if !strings.Contains(raw, "<none>") {
		t.Fatalf("expected '<none>' for empty map, got:\n%s", raw)
	}
}
