package clipboard

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func names(ns ...string) []*unstructured.Unstructured {
	objs := make([]*unstructured.Unstructured, 0, len(ns))
	for _, n := range ns {
		obj := &unstructured.Unstructured{}
		obj.SetName(n)
		objs = append(objs, obj)
	}
	return objs
}

func TestJoinNames(t *testing.T) {
	tests := []struct {
		name string
		objs []*unstructured.Unstructured
		want string
	}{
		{"zero", nil, ""},
		{"one", names("pod-a"), "pod-a"},
		{"many", names("pod-a", "pod-b", "pod-c"), "pod-a\npod-b\npod-c"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := JoinNames(tt.objs); got != tt.want {
				t.Errorf("JoinNames() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestJoinYAML(t *testing.T) {
	tests := []struct {
		name string
		docs []string
		want string
	}{
		{"zero", nil, ""},
		{"one", []string{"a: 1"}, "a: 1"},
		{"many", []string{"a: 1", "b: 2"}, "a: 1\n---\nb: 2"},
		{
			"trailing newlines trimmed",
			[]string{"a: 1\n\n", "b: 2\n"},
			"a: 1\n---\nb: 2",
		},
		{
			"trailing whitespace trimmed",
			[]string{"a: 1\t \n", "b: 2  "},
			"a: 1\n---\nb: 2",
		},
		{
			"leading separator trimmed (no doubled ---)",
			[]string{"---\na: 1\n", "---\nb: 2\n"},
			"a: 1\n---\nb: 2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := JoinYAML(tt.docs); got != tt.want {
				t.Errorf("JoinYAML() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCopy(t *testing.T) {
	const input = "copy-me"
	cmd := Copy(input)
	if cmd == nil {
		t.Fatal("Copy() returned nil cmd")
	}
	if !hasClipboardPayload(cmd, input) {
		t.Errorf("Copy() did not carry payload %q", input)
	}
}

// hasClipboardPayload runs cmd and reports whether any resulting message's
// string form equals want. Copy batches a native-write cmd with
// tea.SetClipboard, so running the top-level cmd yields a tea.BatchMsg (a slice
// of sub-cmds); we run each sub-cmd and match the OSC52 payload. We compare via
// fmt rather than a type assertion because tea.SetClipboard's message type
// (the OSC52 setClipboardMsg, a string type) is unexported by bubbletea.
func hasClipboardPayload(cmd tea.Cmd, want string) bool {
	if cmd == nil {
		return false
	}
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		for _, sub := range msg {
			if hasClipboardPayload(sub, want) {
				return true
			}
		}
		return false
	case nil:
		return false
	default:
		return fmt.Sprintf("%v", msg) == want
	}
}
