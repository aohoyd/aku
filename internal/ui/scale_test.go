package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/aohoyd/aku/internal/msgs"
)

func TestScaleOverlayOpen(t *testing.T) {
	s := NewScaleOverlay(80, 30)
	s.Open("my-deploy", "default", testGVR, 3)
	if !s.Active() {
		t.Fatal("expected overlay to be active after Open")
	}
	if s.overlay.InputCount() != 1 {
		t.Fatalf("expected 1 input, got %d", s.overlay.InputCount())
	}
	if s.overlay.InputValue(0) != "3" {
		t.Fatalf("expected input value '3', got %q", s.overlay.InputValue(0))
	}
	if !s.InputFocused() {
		t.Fatal("expected input to be focused after Open")
	}
	if s.FocusedButton() != scaleBtnYes {
		t.Fatal("expected Yes button focused by default")
	}
}

func TestScaleOverlayEscape(t *testing.T) {
	s := NewScaleOverlay(80, 30)
	s.Open("my-deploy", "default", testGVR, 3)
	s, _ = s.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if s.Active() {
		t.Fatal("expected overlay to be inactive after Escape")
	}
}

func TestScaleOverlaySubmitChanged(t *testing.T) {
	s := NewScaleOverlay(80, 30)
	s.Open("my-deploy", "default", testGVR, 3)

	s.overlay.Input(0).SetValue("5")

	s, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if s.Active() {
		t.Fatal("expected overlay to be inactive after submit")
	}
	if cmd == nil {
		t.Fatal("expected a command to be returned")
	}
	msg := cmd()
	scaleMsg, ok := msg.(msgs.ScaleRequestedMsg)
	if !ok {
		t.Fatalf("expected ScaleRequestedMsg, got %T", msg)
	}
	if scaleMsg.Replicas != 5 {
		t.Fatalf("expected 5 replicas, got %d", scaleMsg.Replicas)
	}
	if scaleMsg.ResourceName != "my-deploy" {
		t.Errorf("expected resource name 'my-deploy', got %q", scaleMsg.ResourceName)
	}
	if scaleMsg.Namespace != "default" {
		t.Errorf("expected namespace 'default', got %q", scaleMsg.Namespace)
	}
}

func TestScaleOverlaySubmitUnchanged(t *testing.T) {
	s := NewScaleOverlay(80, 30)
	s.Open("my-deploy", "default", testGVR, 3)

	s, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if s.Active() {
		t.Fatal("expected overlay to close on unchanged submit")
	}
	if cmd != nil {
		t.Fatal("expected no command when nothing changed")
	}
}

func TestScaleOverlaySubmitInvalidNonNumeric(t *testing.T) {
	s := NewScaleOverlay(80, 30)
	s.Open("my-deploy", "default", testGVR, 3)

	s.overlay.Input(0).SetValue("abc")

	s, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !s.Active() {
		t.Fatal("expected overlay to remain active on invalid input")
	}
	if cmd != nil {
		t.Fatal("expected no command on invalid input")
	}
}

func TestScaleOverlaySubmitNegative(t *testing.T) {
	s := NewScaleOverlay(80, 30)
	s.Open("my-deploy", "default", testGVR, 3)

	s.overlay.Input(0).SetValue("-1")

	s, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !s.Active() {
		t.Fatal("expected overlay to remain active on negative input")
	}
	if cmd != nil {
		t.Fatal("expected no command on negative input")
	}
}

func TestScaleOverlayView(t *testing.T) {
	s := NewScaleOverlay(80, 30)
	s.Open("my-deploy", "default", testGVR, 3)
	view := s.View()
	if view == "" {
		t.Fatal("expected non-empty view")
	}
}

// ── Arrow increment/decrement tests ──

func TestScaleOverlayArrowUp(t *testing.T) {
	s := NewScaleOverlay(80, 30)
	s.Open("my-deploy", "default", testGVR, 3)

	s, _ = s.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if s.overlay.InputValue(0) != "4" {
		t.Fatalf("expected value '4' after up arrow, got %q", s.overlay.InputValue(0))
	}
}

func TestScaleOverlayArrowDown(t *testing.T) {
	s := NewScaleOverlay(80, 30)
	s.Open("my-deploy", "default", testGVR, 3)

	s, _ = s.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if s.overlay.InputValue(0) != "2" {
		t.Fatalf("expected value '2' after down arrow, got %q", s.overlay.InputValue(0))
	}
}

func TestScaleOverlayArrowDownClampsAtZero(t *testing.T) {
	s := NewScaleOverlay(80, 30)
	s.Open("my-deploy", "default", testGVR, 0)

	s, _ = s.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if s.overlay.InputValue(0) != "0" {
		t.Fatalf("expected value '0' (clamped), got %q", s.overlay.InputValue(0))
	}
}

func TestScaleOverlayArrowOnInvalidInput(t *testing.T) {
	s := NewScaleOverlay(80, 30)
	s.Open("my-deploy", "default", testGVR, 3)
	s.overlay.Input(0).SetValue("abc")

	s, _ = s.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	// Should be no-op on non-numeric input.
	if s.overlay.InputValue(0) != "abc" {
		t.Fatalf("expected value 'abc' unchanged, got %q", s.overlay.InputValue(0))
	}
}

// ── Focus and button tests ──

func TestScaleOverlayTabTogglesFocus(t *testing.T) {
	s := NewScaleOverlay(80, 30)
	s.Open("my-deploy", "default", testGVR, 3)

	if !s.InputFocused() {
		t.Fatal("expected input focused initially")
	}

	// Tab to buttons.
	s, _ = s.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if s.InputFocused() {
		t.Fatal("expected buttons focused after Tab")
	}

	// Tab back to input.
	s, _ = s.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if !s.InputFocused() {
		t.Fatal("expected input focused after second Tab")
	}
}

func TestScaleOverlayButtonHotkeys(t *testing.T) {
	s := NewScaleOverlay(80, 30)
	s.Open("my-deploy", "default", testGVR, 3)
	s.overlay.Input(0).SetValue("5")

	// Tab to buttons, then press y.
	s, _ = s.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	s, cmd := s.Update(tea.KeyPressMsg{Code: -1, Text: "y"})
	if s.Active() {
		t.Fatal("expected overlay to close after y hotkey")
	}
	if cmd == nil {
		t.Fatal("expected a command from y hotkey")
	}
	msg := cmd()
	scaleMsg, ok := msg.(msgs.ScaleRequestedMsg)
	if !ok {
		t.Fatalf("expected ScaleRequestedMsg, got %T", msg)
	}
	if scaleMsg.Replicas != 5 {
		t.Fatalf("expected 5 replicas, got %d", scaleMsg.Replicas)
	}
}

func TestScaleOverlayButtonNoHotkey(t *testing.T) {
	s := NewScaleOverlay(80, 30)
	s.Open("my-deploy", "default", testGVR, 3)

	// Tab to buttons, then press n.
	s, _ = s.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	s, cmd := s.Update(tea.KeyPressMsg{Code: -1, Text: "n"})
	if s.Active() {
		t.Fatal("expected overlay to close after n hotkey")
	}
	if cmd != nil {
		t.Fatal("expected no command from n hotkey (cancel)")
	}
}

func TestScaleOverlayYKeySubmitsWhenInputFocused(t *testing.T) {
	s := NewScaleOverlay(80, 30)
	s.Open("my-deploy", "default", testGVR, 3)
	s.overlay.Input(0).SetValue("5")

	// With input focused, "y" should still trigger submit.
	s, cmd := s.Update(tea.KeyPressMsg{Code: -1, Text: "y"})
	if s.Active() {
		t.Fatal("expected overlay to close after y hotkey while input focused")
	}
	if cmd == nil {
		t.Fatal("expected a command from y hotkey")
	}
	msg := cmd()
	scaleMsg, ok := msg.(msgs.ScaleRequestedMsg)
	if !ok {
		t.Fatalf("expected ScaleRequestedMsg, got %T", msg)
	}
	if scaleMsg.Replicas != 5 {
		t.Fatalf("expected 5 replicas, got %d", scaleMsg.Replicas)
	}
}

func TestScaleOverlayNKeyClosesWhenInputFocused(t *testing.T) {
	s := NewScaleOverlay(80, 30)
	s.Open("my-deploy", "default", testGVR, 3)

	// With input focused, "n" should still close the overlay.
	s, cmd := s.Update(tea.KeyPressMsg{Code: -1, Text: "n"})
	if s.Active() {
		t.Fatal("expected overlay to close after n hotkey while input focused")
	}
	if cmd != nil {
		t.Fatal("expected no command from n hotkey (cancel)")
	}
}

func TestScaleOverlayYKeySubmitsWhenInputNotFocused(t *testing.T) {
	s := NewScaleOverlay(80, 30)
	s.Open("my-deploy", "default", testGVR, 3)
	s.overlay.Input(0).SetValue("5")

	// Tab to buttons so input is NOT focused.
	s, _ = s.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if s.InputFocused() {
		t.Fatal("expected input to not be focused after Tab")
	}

	// Now "y" should trigger submit.
	s, cmd := s.Update(tea.KeyPressMsg{Code: -1, Text: "y"})
	if s.Active() {
		t.Fatal("expected overlay to close after y hotkey with buttons focused")
	}
	if cmd == nil {
		t.Fatal("expected a command from y hotkey")
	}
	msg := cmd()
	scaleMsg, ok := msg.(msgs.ScaleRequestedMsg)
	if !ok {
		t.Fatalf("expected ScaleRequestedMsg, got %T", msg)
	}
	if scaleMsg.Replicas != 5 {
		t.Fatalf("expected 5 replicas, got %d", scaleMsg.Replicas)
	}
}

func TestScaleOverlayNKeyClosesWhenInputNotFocused(t *testing.T) {
	s := NewScaleOverlay(80, 30)
	s.Open("my-deploy", "default", testGVR, 3)

	// Tab to buttons so input is NOT focused.
	s, _ = s.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if s.InputFocused() {
		t.Fatal("expected input to not be focused after Tab")
	}

	// Now "n" should close the overlay.
	s, cmd := s.Update(tea.KeyPressMsg{Code: -1, Text: "n"})
	if s.Active() {
		t.Fatal("expected overlay to close after n hotkey with buttons focused")
	}
	if cmd != nil {
		t.Fatal("expected no command from n hotkey (cancel)")
	}
}

func TestScaleOverlayButtonNavigation(t *testing.T) {
	s := NewScaleOverlay(80, 30)
	s.Open("my-deploy", "default", testGVR, 3)

	// Tab to buttons — Yes is focused by default.
	s, _ = s.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if s.FocusedButton() != scaleBtnYes {
		t.Fatal("expected Yes button focused")
	}

	// Right arrow → No.
	s, _ = s.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	if s.FocusedButton() != scaleBtnNo {
		t.Fatal("expected No button focused after Right")
	}

	// Left arrow → Yes.
	s, _ = s.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	if s.FocusedButton() != scaleBtnYes {
		t.Fatal("expected Yes button focused after Left")
	}
}

func TestScaleOverlayEnterOnNoButton(t *testing.T) {
	s := NewScaleOverlay(80, 30)
	s.Open("my-deploy", "default", testGVR, 3)

	// Tab to buttons, navigate to No, press Enter.
	s, _ = s.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	s, _ = s.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	s, cmd := s.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if s.Active() {
		t.Fatal("expected overlay to close after Enter on No")
	}
	if cmd != nil {
		t.Fatal("expected no command from No button")
	}
}

// ── Digit-only validation tests ──

func TestScaleOverlayRejectsLetterKeystrokes(t *testing.T) {
	s := NewScaleOverlay(80, 30)
	s.Open("my-deploy", "default", testGVR, 3)

	// Send a letter keystroke while the input is focused.
	s, _ = s.Update(tea.KeyPressMsg{Code: -1, Text: "a"})

	if s.overlay.InputValue(0) != "3" {
		t.Fatalf("expected input value '3' unchanged after letter key, got %q", s.overlay.InputValue(0))
	}
}

func TestScaleOverlayAcceptsDigitKeystrokes(t *testing.T) {
	s := NewScaleOverlay(80, 30)
	s.Open("my-deploy", "default", testGVR, 3)

	// Send a digit keystroke while the input is focused.
	s, _ = s.Update(tea.KeyPressMsg{Code: -1, Text: "5"})

	if s.overlay.InputValue(0) != "35" {
		t.Fatalf("expected input value '35' after digit key, got %q", s.overlay.InputValue(0))
	}
}

func TestScaleOverlayUpFromButtonsFocusesInput(t *testing.T) {
	s := NewScaleOverlay(80, 30)
	s.Open("my-deploy", "default", testGVR, 3)

	// Tab to buttons.
	s, _ = s.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if s.InputFocused() {
		t.Fatal("expected buttons focused")
	}

	// Up arrow returns to input.
	s, _ = s.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if !s.InputFocused() {
		t.Fatal("expected input focused after Up from buttons")
	}
}
