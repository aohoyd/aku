package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/aohoyd/aku/internal/msgs"
)

func TestChartInputOverlayOpen(t *testing.T) {
	ci := NewChartInputOverlay(80, 30)
	ci.Open("my-release", "production", "oci://ghcr.io/org/chart")
	if !ci.Active() {
		t.Fatal("expected overlay to be active after Open")
	}
	if ci.overlay.InputCount() != 1 {
		t.Fatalf("expected 1 input, got %d", ci.overlay.InputCount())
	}
	if ci.overlay.InputValue(0) != "oci://ghcr.io/org/chart" {
		t.Fatalf("expected prefilled value, got %q", ci.overlay.InputValue(0))
	}
}

func TestChartInputOverlayOpenEmpty(t *testing.T) {
	ci := NewChartInputOverlay(80, 30)
	ci.Open("my-release", "production", "")
	if !ci.Active() {
		t.Fatal("expected overlay to be active")
	}
	if ci.overlay.InputValue(0) != "" {
		t.Fatalf("expected empty value, got %q", ci.overlay.InputValue(0))
	}
}

func TestChartInputOverlayEscape(t *testing.T) {
	ci := NewChartInputOverlay(80, 30)
	ci.Open("my-release", "production", "")
	ci, _ = ci.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if ci.Active() {
		t.Fatal("expected overlay to be inactive after Escape")
	}
}

func TestChartInputOverlaySubmit(t *testing.T) {
	ci := NewChartInputOverlay(80, 30)
	ci.Open("my-release", "production", "")
	ci.overlay.Input(0).SetValue("oci://ghcr.io/org/chart")

	ci, cmd := ci.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if ci.Active() {
		t.Fatal("expected overlay to be inactive after submit")
	}
	if cmd == nil {
		t.Fatal("expected a command")
	}
	msg := cmd()
	csMsg, ok := msg.(msgs.HelmChartRefSetMsg)
	if !ok {
		t.Fatalf("expected HelmChartRefSetMsg, got %T", msg)
	}
	if csMsg.ReleaseName != "my-release" {
		t.Errorf("expected release 'my-release', got %q", csMsg.ReleaseName)
	}
	if csMsg.Namespace != "production" {
		t.Errorf("expected namespace 'production', got %q", csMsg.Namespace)
	}
	if csMsg.ChartRef != "oci://ghcr.io/org/chart" {
		t.Errorf("expected chart ref, got %q", csMsg.ChartRef)
	}
}

func TestChartInputOverlaySubmitEmpty(t *testing.T) {
	ci := NewChartInputOverlay(80, 30)
	ci.Open("my-release", "production", "")

	ci, cmd := ci.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if ci.Active() {
		t.Fatal("expected overlay to close")
	}
	if cmd != nil {
		t.Fatal("expected no command for empty submit")
	}
}

func TestChartInputOverlayView(t *testing.T) {
	ci := NewChartInputOverlay(80, 30)
	ci.Open("my-release", "production", "")
	view := ci.View()
	if view == "" {
		t.Fatal("expected non-empty view")
	}
}
