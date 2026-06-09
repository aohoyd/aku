package ui

import (
	"strings"
	"testing"
)

func TestCycleFieldNextPrevWrap(t *testing.T) {
	f := NewPullPolicyField("")
	// options: (default), Always, IfNotPresent, Never
	if got := f.Value(); got != "" {
		t.Fatalf("initial Value = %q, want \"\"", got)
	}

	f.Next() // Always
	if got := f.Value(); got != "Always" {
		t.Fatalf("after Next Value = %q, want Always", got)
	}
	f.Next() // IfNotPresent
	f.Next() // Never
	if got := f.Value(); got != "Never" {
		t.Fatalf("after 3x Next Value = %q, want Never", got)
	}
	f.Next() // wrap to (default)
	if got := f.Value(); got != "" {
		t.Fatalf("after wrap Next Value = %q, want \"\"", got)
	}

	// Prev from (default) wraps to Never.
	f.Prev()
	if got := f.Value(); got != "Never" {
		t.Fatalf("after wrap Prev Value = %q, want Never", got)
	}
}

func TestCycleFieldSetValue(t *testing.T) {
	tests := []struct {
		name string
		set  string
		want string
	}{
		{name: "empty selects default", set: "", want: ""},
		{name: "Always", set: "Always", want: "Always"},
		{name: "IfNotPresent", set: "IfNotPresent", want: "IfNotPresent"},
		{name: "Never", set: "Never", want: "Never"},
		{name: "bogus falls back to default", set: "bogus", want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Start off default so SetValue("") is a real change.
			f := NewPullPolicyField("Always")
			f.SetValue(tc.set)
			if got := f.Value(); got != tc.want {
				t.Fatalf("SetValue(%q) Value = %q, want %q", tc.set, got, tc.want)
			}
		})
	}
}

func TestCycleFieldZeroValueNoPanic(t *testing.T) {
	var f CycleField // zero value: nil options, idx 0
	if got := f.Value(); got != "" {
		t.Fatalf("zero-value Value = %q, want \"\"", got)
	}
	if got := f.View(); got != "" {
		t.Fatalf("zero-value View = %q, want \"\"", got)
	}
	// Next/Prev on a nil-options field must not panic (exercises the len==0
	// guards) and must leave Value() at "".
	f.Next()
	if got := f.Value(); got != "" {
		t.Fatalf("after Next on zero value, Value = %q, want \"\"", got)
	}
	f.Prev()
	if got := f.Value(); got != "" {
		t.Fatalf("after Prev on zero value, Value = %q, want \"\"", got)
	}
}

func TestCycleFieldViewFocusedDiffersFromBlurred(t *testing.T) {
	f := NewPullPolicyField("IfNotPresent")

	f.Blur()
	blurred := f.View()
	f.Focus()
	focused := f.View()

	if blurred == "" {
		t.Fatal("blurred View is empty")
	}
	if focused == "" {
		t.Fatal("focused View is empty")
	}
	if blurred == focused {
		t.Fatalf("focused and blurred View are identical: %q", focused)
	}
	// Both renderings must carry the selected value and the prompt label.
	for _, v := range []string{blurred, focused} {
		if !strings.Contains(v, "IfNotPresent") {
			t.Fatalf("View missing selected value %q: %q", "IfNotPresent", v)
		}
		if !strings.Contains(v, "pull:") {
			t.Fatalf("View missing prompt label %q: %q", "pull:", v)
		}
	}
}
