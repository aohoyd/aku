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

func TestHelmRollbackOpenLoading(t *testing.T) {
	o := NewHelmRollbackOverlay(60, 20)
	o.OpenLoading("my-release", "default")
	if !o.Active() {
		t.Fatal("expected overlay to be active after OpenLoading")
	}
	if !o.loading {
		t.Fatal("expected loading to be true after OpenLoading")
	}
}

func TestHelmRollbackSetRevisions(t *testing.T) {
	o := NewHelmRollbackOverlay(60, 20)
	o.OpenLoading("my-release", "default")
	entries := []HelmRevisionEntry{
		{Revision: 3, Display: "Rev 3 | deployed"},
		{Revision: 2, Display: "Rev 2 | superseded"},
	}
	o.SetRevisions(entries)
	if o.loading {
		t.Fatal("expected loading to be false after SetRevisions")
	}
	if o.loadErr != "" {
		t.Fatalf("expected no error, got %q", o.loadErr)
	}
	if len(o.revisions) != 2 {
		t.Fatalf("expected 2 revisions, got %d", len(o.revisions))
	}
}

func TestHelmRollbackSetError(t *testing.T) {
	o := NewHelmRollbackOverlay(60, 20)
	o.OpenLoading("my-release", "default")
	o.SetError("timeout")
	if o.loading {
		t.Fatal("expected loading to be false after SetError")
	}
	if o.loadErr != "timeout" {
		t.Fatalf("expected loadErr %q, got %q", "timeout", o.loadErr)
	}
}

func TestHelmRollbackEnterDuringLoading(t *testing.T) {
	o := NewHelmRollbackOverlay(60, 20)
	o.OpenLoading("my-release", "default")
	o, cmd := o.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("Enter during loading should return nil cmd")
	}
	if !o.Active() {
		t.Fatal("overlay should still be active during loading")
	}
}

func TestHelmRollbackEnterDuringError(t *testing.T) {
	o := NewHelmRollbackOverlay(60, 20)
	o.OpenLoading("my-release", "default")
	o.SetError("something went wrong")
	o, cmd := o.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("Enter during error should return nil cmd")
	}
	if !o.Active() {
		t.Fatal("overlay should still be active during error")
	}
}

func TestHelmRollbackEscDuringLoading(t *testing.T) {
	o := NewHelmRollbackOverlay(60, 20)
	o.OpenLoading("my-release", "default")
	o, _ = o.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if o.Active() {
		t.Fatal("Esc should close the overlay even during loading")
	}
}
