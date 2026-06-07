package ui

import "testing"

// TestResourceListSatisfiesPane asserts a *ResourceList is usable as a ui.Pane
// and that interface dispatch reaches the concrete method. The compile-time
// guarantee lives in resourcelist.go (`var _ Pane = (*ResourceList)(nil)`); this
// exercises a real method through the interface so it is not tautological.
func TestResourceListSatisfiesPane(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 24)
	var p Pane = &rl
	if got := p.Title(); got != "pods" {
		t.Fatalf("Pane.Title() through interface = %q, want %q", got, "pods")
	}
}

// TestResourceListTitleFromPlugin verifies Title() returns the plugin name.
func TestResourceListTitleFromPlugin(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 24)
	if got := rl.Title(); got != "pods" {
		t.Fatalf("expected title 'pods', got %q", got)
	}
}
