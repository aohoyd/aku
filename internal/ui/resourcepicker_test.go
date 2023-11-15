package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/aohoyd/aku/internal/msgs"
)

func TestResourcePickerOpen(t *testing.T) {
	cb := NewResourcePicker(80, 24)
	cb.Open()
	if !cb.Active() {
		t.Fatal("resource picker should be active after Open")
	}
	view := cb.View()
	if view == "" {
		t.Fatal("active resource picker should render something")
	}
}

func TestResourcePickerEscCancels(t *testing.T) {
	cb := NewResourcePicker(80, 24)
	cb.Open()
	updated, _ := cb.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if updated.Active() {
		t.Fatal("resource picker should close after Esc")
	}
}

func testPlugins() []PluginEntry {
	return []PluginEntry{
		{Name: "pods", ShortName: "po"},
		{Name: "deployments", ShortName: "deploy"},
		{Name: "services", ShortName: "svc"},
		{Name: "daemonsets", ShortName: "ds"},
		{Name: "horizontalpodautoscalers", ShortName: "hpa"},
	}
}

func TestResourcePickerDropdownShownOnOpen(t *testing.T) {
	cb := NewResourcePicker(80, 24)
	cb.SetPlugins(testPlugins())
	cb.Open()
	if len(cb.Filtered()) != 5 {
		t.Fatalf("expected 5 matches on open, got %d", len(cb.Filtered()))
	}
}

func TestResourcePickerDropdownFilters(t *testing.T) {
	cb := NewResourcePicker(80, 24)
	cb.SetPlugins(testPlugins())
	cb.Open()
	cb, _ = cb.Update(tea.KeyPressMsg{Code: -1, Text: "p"})
	cb, _ = cb.Update(tea.KeyPressMsg{Code: -1, Text: "o"})
	filtered := cb.Filtered()
	if len(filtered) == 0 {
		t.Fatal("expected matches for 'po'")
	}
	if filtered[0].Name != "pods" {
		t.Fatalf("expected 'pods' first, got %q", filtered[0].Name)
	}
}

func TestResourcePickerArrowNavigation(t *testing.T) {
	cb := NewResourcePicker(80, 24)
	cb.SetPlugins(testPlugins())
	cb.Open()

	if cb.Cursor() != 0 {
		t.Fatalf("initial cursor should be 0, got %d", cb.Cursor())
	}
	cb, _ = cb.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if cb.Cursor() != 1 {
		t.Fatalf("cursor should be 1 after down, got %d", cb.Cursor())
	}
	cb, _ = cb.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if cb.Cursor() != 0 {
		t.Fatalf("cursor should be 0 after up, got %d", cb.Cursor())
	}
	cb, _ = cb.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if cb.Cursor() != 0 {
		t.Fatalf("cursor should clamp at 0, got %d", cb.Cursor())
	}
}

func TestResourcePickerEnterSelectsMatch(t *testing.T) {
	cb := NewResourcePicker(80, 24)
	cb.SetPlugins(testPlugins())
	cb.Open()
	cb, _ = cb.Update(tea.KeyPressMsg{Code: -1, Text: "d"})
	cb, _ = cb.Update(tea.KeyPressMsg{Code: -1, Text: "e"})
	cb, _ = cb.Update(tea.KeyPressMsg{Code: -1, Text: "p"})

	cb, cmd := cb.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cb.Active() {
		t.Fatal("should close after Enter")
	}
	if cmd == nil {
		t.Fatal("Enter should produce a command")
	}
	msg := cmd()
	cmdMsg, ok := msg.(msgs.ResourcePickedMsg)
	if !ok {
		t.Fatalf("expected ResourcePickedMsg, got %T", msg)
	}
	if cmdMsg.Command != "goto deployments" {
		t.Fatalf("expected 'goto deployments', got %q", cmdMsg.Command)
	}
}

func manyPlugins() []PluginEntry {
	return []PluginEntry{
		{Name: "pods", ShortName: "po"},
		{Name: "deployments", ShortName: "deploy"},
		{Name: "statefulsets", ShortName: "sts"},
		{Name: "daemonsets", ShortName: "ds"},
		{Name: "replicasets", ShortName: "rs"},
		{Name: "services", ShortName: "svc"},
		{Name: "configmaps", ShortName: "cm"},
		{Name: "namespaces", ShortName: "ns"},
		{Name: "nodes", ShortName: "no"},
		{Name: "secrets", ShortName: "sec"},
		{Name: "jobs", ShortName: "job"},
		{Name: "cronjobs", ShortName: ""},
		{Name: "persistentvolumeclaims", ShortName: "pvc"},
		{Name: "persistentvolumes", ShortName: "pv"},
	}
}

func TestResourcePickerDropdownScrollsWithCursor(t *testing.T) {
	cb := NewResourcePicker(80, 24)
	cb.SetPlugins(manyPlugins())
	cb.Open()

	if len(cb.Filtered()) != 14 {
		t.Fatalf("expected 14 matches, got %d", len(cb.Filtered()))
	}

	for range 12 {
		cb, _ = cb.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	if cb.Cursor() != 12 {
		t.Fatalf("expected cursor at 12, got %d", cb.Cursor())
	}

	view := cb.View()
	selected := cb.Filtered()[cb.Cursor()]
	display := selected.Name
	if selected.ShortName != "" && selected.ShortName != selected.Name {
		display = selected.Name + " (" + selected.ShortName + ")"
	}
	if !strings.Contains(view, display) {
		t.Fatalf("view should contain selected item %q", display)
	}
}

func TestResourcePickerDropdownScrollsBackUp(t *testing.T) {
	cb := NewResourcePicker(80, 24)
	cb.SetPlugins(manyPlugins())
	cb.Open()

	for range 13 {
		cb, _ = cb.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	if cb.Cursor() != 13 {
		t.Fatalf("expected cursor at 13, got %d", cb.Cursor())
	}

	for range 13 {
		cb, _ = cb.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	}
	if cb.Cursor() != 0 {
		t.Fatalf("expected cursor at 0, got %d", cb.Cursor())
	}
}
