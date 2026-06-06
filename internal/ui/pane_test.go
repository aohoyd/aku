package ui

import "testing"

// TestResourceListSatisfiesPane asserts (at runtime) that a *ResourceList is
// usable as a ui.Pane. The compile-time guarantee lives in resourcelist.go
// (`var _ Pane = (*ResourceList)(nil)`); this guards the assignment too.
func TestResourceListSatisfiesPane(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 24)
	var p Pane = &rl
	if p == nil {
		t.Fatal("*ResourceList should satisfy Pane")
	}
}

// TestResourceListKindIsResources verifies a resource pane reports PaneResources.
func TestResourceListKindIsResources(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 24)
	var p Pane = &rl
	if got := p.Kind(); got != PaneResources {
		t.Fatalf("expected PaneResources, got %d", got)
	}
}

// TestResourceListTitleFromPlugin verifies Title() returns the plugin name.
func TestResourceListTitleFromPlugin(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 24)
	if got := rl.Title(); got != "pods" {
		t.Fatalf("expected title 'pods', got %q", got)
	}
}
