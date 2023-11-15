package render

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestYAMLLineCountInvariant(t *testing.T) {
	m := map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata":   map[string]any{"name": "nginx", "namespace": "default"},
		"spec": map[string]any{"containers": []any{
			map[string]any{"name": "nginx", "image": "nginx:latest"},
		}},
	}
	c, err := YAML(m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rawLines := strings.Count(c.Raw, "\n")
	displayLines := strings.Count(c.Display, "\n")
	if rawLines != displayLines {
		t.Fatalf("line count mismatch: raw=%d display=%d\nraw:\n%s\ndisplay:\n%s", rawLines, displayLines, c.Raw, c.Display)
	}
}

func TestYAMLStripInvariant(t *testing.T) {
	m := map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata":   map[string]any{"name": "test"},
	}
	c, err := YAML(m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	stripped := ansi.Strip(c.Display)
	if stripped != c.Raw {
		t.Fatalf("stripped display should equal raw\nraw:     %q\nstripped:%q", c.Raw, stripped)
	}
}

func TestYAMLScalarTypes(t *testing.T) {
	m := map[string]any{
		"count":   int64(3),
		"enabled": true,
		"name":    "test",
		"nothing": nil,
		"ratio":   float64(1.5),
	}
	c, err := YAML(m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(c.Raw, "3") {
		t.Fatal("raw should contain number")
	}
	if !strings.Contains(c.Raw, "true") {
		t.Fatal("raw should contain bool")
	}
	if !strings.Contains(c.Raw, "null") {
		t.Fatal("raw should contain null")
	}
	if len(c.Display) <= len(c.Raw) {
		t.Fatal("display should be longer than raw due to ANSI codes")
	}
}

func TestYAMLNestedMap(t *testing.T) {
	m := map[string]any{
		"metadata": map[string]any{
			"name": "test",
		},
	}
	c, err := YAML(m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(c.Raw, "metadata:") {
		t.Fatal("raw should contain parent key")
	}
	if !strings.Contains(c.Raw, "  name:") {
		t.Fatal("raw should contain indented child key")
	}
}

func TestYAMLSequence(t *testing.T) {
	m := map[string]any{
		"items": []any{"a", "b"},
	}
	c, err := YAML(m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Count(c.Raw, "- ") < 2 {
		t.Fatal("raw should contain list markers")
	}
	stripped := ansi.Strip(c.Display)
	if stripped != c.Raw {
		t.Fatalf("strip invariant broken for sequence\nraw:     %q\nstripped:%q", c.Raw, stripped)
	}
}

func TestYAMLMapInSequence(t *testing.T) {
	m := map[string]any{
		"containers": []any{
			map[string]any{"name": "nginx", "image": "nginx:latest"},
		},
	}
	c, err := YAML(m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(c.Raw, "- ") {
		t.Fatal("raw should contain list marker for map-in-sequence")
	}
	if !strings.Contains(c.Raw, "name:") {
		t.Fatal("raw should contain key inside sequence item")
	}
	stripped := ansi.Strip(c.Display)
	if stripped != c.Raw {
		t.Fatalf("strip invariant broken for map-in-sequence\nraw:     %q\nstripped:%q", c.Raw, stripped)
	}
}

func TestYAMLMapOffsets(t *testing.T) {
	raw := "name: test\n"
	display := "\x1b[38;5;75mname:\x1b[m \x1b[38;5;114mtest\x1b[m\n"
	rawOffsets := [][]int{{6, 10}}
	displayOffsets := MapOffsets(raw, display, rawOffsets)
	if len(displayOffsets) != 1 {
		t.Fatalf("expected 1 offset pair, got %d", len(displayOffsets))
	}
	matched := display[displayOffsets[0][0]:displayOffsets[0][1]]
	if ansi.Strip(matched) != "test" {
		t.Fatalf("mapped display offset should extract 'test', got %q (stripped: %q)", matched, ansi.Strip(matched))
	}
}

func TestYAMLEmptyObject(t *testing.T) {
	c, err := YAML(map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Raw != "" || c.Display != "" {
		t.Fatal("empty object should produce empty output")
	}
}

func TestYAMLDeeplyNested(t *testing.T) {
	inner := map[string]any{"leaf": "value"}
	for i := range 10 {
		inner = map[string]any{fmt.Sprintf("level%d", i): inner}
	}
	c, err := YAML(inner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(c.Raw, "leaf:") {
		t.Fatal("deeply nested leaf should be present")
	}
	stripped := ansi.Strip(c.Display)
	if stripped != c.Raw {
		t.Fatal("strip invariant broken for deeply nested")
	}
}

func TestYAMLTopLevelKeyOrder(t *testing.T) {
	m := map[string]any{
		"status":     map[string]any{"phase": "Running"},
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata":   map[string]any{"name": "test"},
		"spec":       map[string]any{"nodeName": "node1"},
	}
	c, err := YAML(m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	raw := c.Raw
	lines := strings.Split(raw, "\n")

	var order []string
	for _, line := range lines {
		for _, key := range []string{"apiVersion:", "kind:", "metadata:", "spec:", "status:"} {
			if strings.HasPrefix(line, key) {
				order = append(order, key)
			}
		}
	}
	expected := []string{"apiVersion:", "kind:", "metadata:", "spec:", "status:"}
	if len(order) != len(expected) {
		t.Fatalf("expected %d top-level keys, got %d: %v", len(expected), len(order), order)
	}
	for i, k := range expected {
		if order[i] != k {
			t.Fatalf("position %d: expected %s, got %s (order: %v)", i, k, order[i], order)
		}
	}
}

func TestYAMLTopLevelKeyOrderWithExtra(t *testing.T) {
	m := map[string]any{
		"zebra":      "last",
		"apiVersion": "v1",
		"alpha":      "first-extra",
		"kind":       "Service",
	}
	c, err := YAML(m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	raw := c.Raw
	lines := strings.Split(raw, "\n")

	var order []string
	for _, line := range lines {
		if strings.Contains(line, ":") && !strings.HasPrefix(line, " ") {
			order = append(order, strings.TrimSpace(strings.SplitN(line, ":", 2)[0]))
		}
	}
	// apiVersion and kind first (kubectl order), then alpha, zebra (alphabetical)
	expected := []string{"apiVersion", "kind", "alpha", "zebra"}
	if len(order) != len(expected) {
		t.Fatalf("expected %d keys, got %d: %v", len(expected), len(order), order)
	}
	for i, k := range expected {
		if order[i] != k {
			t.Fatalf("position %d: expected %s, got %s (order: %v)", i, k, order[i], order)
		}
	}
}

func TestYAMLNestedKeysStillAlphabetical(t *testing.T) {
	m := map[string]any{
		"metadata": map[string]any{
			"spec":      "not-a-real-field",
			"name":      "test",
			"namespace": "default",
			"kind":      "not-a-real-field",
		},
	}
	c, err := YAML(m)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	raw := c.Raw
	lines := strings.Split(raw, "\n")

	var nestedKeys []string
	for _, line := range lines {
		if strings.HasPrefix(line, "  ") && strings.Contains(line, ":") {
			nestedKeys = append(nestedKeys, strings.TrimSpace(strings.SplitN(line, ":", 2)[0]))
		}
	}
	// Nested keys should be alphabetical, NOT kubectl-ordered
	expected := []string{"kind", "name", "namespace", "spec"}
	if len(nestedKeys) != len(expected) {
		t.Fatalf("expected %d nested keys, got %d: %v", len(expected), len(nestedKeys), nestedKeys)
	}
	for i, k := range expected {
		if nestedKeys[i] != k {
			t.Fatalf("nested position %d: expected %s, got %s", i, k, nestedKeys[i])
		}
	}
}
