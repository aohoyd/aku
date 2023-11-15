package render

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestContentAppend(t *testing.T) {
	a := Content{Raw: "Name:  pod\n", Display: "Name:  pod\n"}
	b := Content{Raw: "Events:\n  <none>\n", Display: "Events:\n  <none>\n"}
	result := a.Append(b)
	if result.Raw != "Name:  pod\nEvents:\n  <none>\n" {
		t.Errorf("unexpected raw: %q", result.Raw)
	}
	if result.Display != "Name:  pod\nEvents:\n  <none>\n" {
		t.Errorf("unexpected display: %q", result.Display)
	}
}

func TestContentAppendEmptyOther(t *testing.T) {
	a := Content{Raw: "Name:  pod\n", Display: "Name:  pod\n"}
	result := a.Append(Content{})
	if result != a {
		t.Errorf("appending empty should return original, got raw=%q", result.Raw)
	}
}

func TestContentAppendEmptySelf(t *testing.T) {
	b := Content{Raw: "Events:\n", Display: "Events:\n"}
	result := Content{}.Append(b)
	if result != b {
		t.Errorf("appending to empty should return other, got raw=%q", result.Raw)
	}
}

func TestContentAppendStripInvariant(t *testing.T) {
	b1 := NewBuilder()
	b1.KV(LEVEL_0, "Name", "test-pod")
	b1.KVStyled(LEVEL_0, ValueStatusOK, "Status", "Running")
	c1 := b1.Build()

	b2 := NewBuilder()
	b2.Section(LEVEL_0, "Events")
	b2.RawLine(LEVEL_1, "some event line")
	c2 := b2.Build()

	result := c1.Append(c2)
	stripped := ansi.Strip(result.Display)
	if stripped != result.Raw {
		t.Errorf("strip invariant broken:\nstripped: %q\nraw:      %q", stripped, result.Raw)
	}
}
