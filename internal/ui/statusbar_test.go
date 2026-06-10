package ui

import (
	"strings"
	"testing"

	"github.com/aohoyd/aku/internal/config"
	"github.com/aohoyd/aku/internal/msgs"
)

func TestStatusBarRenderHints(t *testing.T) {
	sb := NewStatusBar(80)
	hints := []config.KeyHint{
		{Key: "q", Help: "quit"},
		{Key: "g", Help: "go to"},
	}
	sb.SetHints(hints)
	view := sb.View()
	if !strings.Contains(view, "q") || !strings.Contains(view, "quit") {
		t.Fatal("status bar should contain key hints")
	}
	if !strings.Contains(view, "g") || !strings.Contains(view, "go to") {
		t.Fatal("status bar should contain all hints")
	}
}

func TestStatusBarClearHints(t *testing.T) {
	sb := NewStatusBar(80)
	sb.SetHints([]config.KeyHint{{Key: "q", Help: "quit"}})
	sb.ClearHints()
	_ = sb.View() // should not panic
}

func TestStatusBarHintLimit(t *testing.T) {
	// Narrow width forces truncation — each hint "a action" is ~10 chars + gap
	sb := NewStatusBar(40)
	hints := make([]config.KeyHint, 8)
	for i := range hints {
		hints[i] = config.KeyHint{Key: string(rune('a' + i)), Help: "action"}
	}
	sb.SetHints(hints)
	view := sb.View()
	// "? more" indicator should appear since not all hints fit
	if !strings.Contains(view, "more") {
		t.Fatal("should show '? more' indicator when hints exceed available width")
	}
}

func TestStatusBarHintsFillWidth(t *testing.T) {
	// Wide enough to show all 8 hints without truncation
	sb := NewStatusBar(200)
	hints := make([]config.KeyHint, 8)
	for i := range hints {
		hints[i] = config.KeyHint{Key: string(rune('a' + i)), Help: "action"}
	}
	sb.SetHints(hints)
	view := sb.View()
	// All hints should be shown
	for i := range 8 {
		key := string(rune('a' + i))
		if !strings.Contains(view, key) {
			t.Fatalf("hint %q should be visible on wide terminal", key)
		}
	}
	// No "? more" since all fit
	if strings.Contains(view, "more") {
		t.Fatal("should not show '? more' when all hints fit")
	}
}

func TestStatusBarIndicator(t *testing.T) {
	sb := NewStatusBar(80)
	sb.SetIndicator("[Z]")
	sb.SetHints([]config.KeyHint{{Key: "q", Help: "quit"}})

	view := sb.View()
	if !strings.Contains(view, "[Z]") {
		t.Fatal("status bar should display indicator")
	}
	if !strings.Contains(view, "q") {
		t.Fatal("status bar should still display hints alongside indicator")
	}
}

func TestStatusBarIndicatorClear(t *testing.T) {
	sb := NewStatusBar(80)
	sb.SetIndicator("[Z]")
	sb.SetIndicator("")

	view := sb.View()
	if strings.Contains(view, "[Z]") {
		t.Fatal("indicator should be cleared")
	}
}

func TestStatusBarSetContextName(t *testing.T) {
	sb := NewStatusBar(80)
	if sb.contextName != "default" {
		t.Fatalf("expected default context name %q, got %q", "default", sb.contextName)
	}
	sb.SetContextName("minikube")
	if sb.contextName != "minikube" {
		t.Fatalf("expected context name %q, got %q", "minikube", sb.contextName)
	}
	sb.SetContextName("")
	if sb.contextName != "default" {
		t.Fatalf("empty name should fall back to %q, got %q", "default", sb.contextName)
	}
}

func TestTruncateContext(t *testing.T) {
	short := "minikube"
	if got := truncateContext(short, 24); got != short {
		t.Fatalf("short name should be unchanged, got %q", got)
	}
	long := "my-very-long-cluster-name-exceeds"
	got := truncateContext(long, 24)
	runes := []rune(got)
	if len(runes) != 24 {
		t.Fatalf("truncated name should be 24 runes, got %d", len(runes))
	}
	if runes[len(runes)-1] != '…' {
		t.Fatalf("truncated name should end with ellipsis, got %q", got)
	}
}

func TestStatusBarHealthDotOnline(t *testing.T) {
	sb := NewStatusBar(80)
	sb.SetContextName("minikube")
	sb.SetOnline(true)
	view := sb.View()
	if !strings.Contains(view, "minikube") {
		t.Fatal("online status bar should contain context name")
	}
}

func TestStatusBarHealthDotOffline(t *testing.T) {
	sb := NewStatusBar(80)
	sb.SetContextName("minikube")
	view := sb.View()
	if !strings.Contains(view, "minikube") {
		t.Fatal("offline status bar should contain context name")
	}
}

func TestStatusBarStartEndOperation(t *testing.T) {
	sb := NewStatusBar(80)
	if sb.Busy() {
		t.Fatal("should not be busy initially")
	}
	sb.StartOperation()
	if !sb.Busy() {
		t.Fatal("should be busy after StartOperation")
	}
	sb.StartOperation()
	if !sb.Busy() {
		t.Fatal("should still be busy after second StartOperation")
	}
	sb.EndOperation()
	if !sb.Busy() {
		t.Fatal("should still be busy with one inflight")
	}
	sb.EndOperation()
	if sb.Busy() {
		t.Fatal("should not be busy after all EndOperation calls")
	}
}

func TestStatusBarStartOperationReturnsTick(t *testing.T) {
	sb := NewStatusBar(80)
	cmd := sb.StartOperation()
	if cmd == nil {
		t.Fatal("first StartOperation should return a non-nil cmd")
	}
	cmd2 := sb.StartOperation()
	if cmd2 != nil {
		t.Fatal("second StartOperation should return nil")
	}
}

func TestStatusBarViewShowsSpinnerWhenActive(t *testing.T) {
	sb := NewStatusBar(80)
	sb.SetContextName("minikube")
	sb.SetHints([]config.KeyHint{{Key: "q", Help: "quit"}})

	// An operation is in flight and the delayed show-spinner message has arrived,
	// so the spinner becomes visible alongside the context badge and key hints.
	sb.StartOperation()
	sb.Update(msgs.StatusBarShowSpinnerMsg{})

	view := sb.View()
	if !strings.Contains(view, sb.spinner.View()) {
		t.Fatal("status bar view should render the spinner while an operation is active")
	}
	if !strings.Contains(view, "minikube") {
		t.Fatal("status bar should still render the context badge with the spinner")
	}
	if !strings.Contains(view, "quit") {
		t.Fatal("status bar should still render key hints with the spinner")
	}
}

func TestStatusBarViewHidesSpinnerWhenIdle(t *testing.T) {
	sb := NewStatusBar(80)
	sb.SetContextName("minikube")
	// No StartOperation/show-spinner message: the spinner stays hidden and only
	// the badge + hints render.
	view := sb.View()
	if strings.Contains(view, sb.spinner.View()) {
		t.Fatal("status bar view should not render the spinner when no operation is active")
	}
}

func TestStatusBarSetOnline(t *testing.T) {
	sb := NewStatusBar(80)
	if sb.online {
		t.Fatal("should default to offline")
	}
	sb.SetOnline(true)
	if !sb.online {
		t.Fatal("should be online after SetOnline(true)")
	}
	sb.SetOnline(false)
	if sb.online {
		t.Fatal("should be offline after SetOnline(false)")
	}
}
