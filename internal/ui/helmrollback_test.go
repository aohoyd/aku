package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/aohoyd/aku/internal/msgs"
)

func TestHelmRollbackRender(t *testing.T) {
	o := NewHelmRollbackOverlay(60, 20)
	o.Open("my-release", "default", []HelmRevisionEntry{
		{Revision: 3, Display: "Rev 3 | deployed | nginx-1.0 | 2026-03-06"},
		{Revision: 2, Display: "Rev 2 | superseded | nginx-0.9 | 2026-03-05"},
	})
	view := o.View()
	if view == "" {
		t.Fatal("expected non-empty view")
	}
}

func TestHelmRollbackEsc(t *testing.T) {
	o := NewHelmRollbackOverlay(60, 20)
	o.Open("my-release", "default", []HelmRevisionEntry{
		{Revision: 3, Display: "Rev 3 | deployed"},
	})
	o, _ = o.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if o.Active() {
		t.Fatal("Esc should close the overlay")
	}
}

func TestHelmRollbackSubmit(t *testing.T) {
	o := NewHelmRollbackOverlay(60, 20)
	o.Open("my-release", "default", []HelmRevisionEntry{
		{Revision: 3, Display: "Rev 3 | deployed"},
		{Revision: 2, Display: "Rev 2 | superseded"},
	})
	o, cmd := o.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter should return a command")
	}
	result := cmd().(msgs.HelmRollbackRequestedMsg)
	if result.Revision != 3 {
		t.Fatalf("expected revision 3, got %d", result.Revision)
	}
	if result.ReleaseName != "my-release" {
		t.Fatalf("expected my-release, got %s", result.ReleaseName)
	}
}

func TestHelmRollbackNavigation(t *testing.T) {
	o := NewHelmRollbackOverlay(60, 20)
	o.Open("my-release", "default", []HelmRevisionEntry{
		{Revision: 3, Display: "Rev 3"},
		{Revision: 2, Display: "Rev 2"},
	})
	// Move down to second item
	o, _ = o.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	o, cmd := o.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	result := cmd().(msgs.HelmRollbackRequestedMsg)
	if result.Revision != 2 {
		t.Fatalf("expected revision 2, got %d", result.Revision)
	}
}
