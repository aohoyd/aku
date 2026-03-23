package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/aohoyd/aku/internal/msgs"
)

var testPorts = []PortItem{
	{ContainerName: "nginx", Port: 80, Protocol: "TCP"},
	{ContainerName: "nginx", Port: 443, Protocol: "TCP"},
	{ContainerName: "redis", Port: 6379, Protocol: "TCP"},
}

func TestPortForwardOverlayOpen(t *testing.T) {
	pf := NewPortForwardOverlay(80, 30)
	pf.Open("my-pod", "default", testPorts[:2])
	if !pf.Active() {
		t.Fatal("expected overlay to be active after Open")
	}
	view := pf.View()
	if view == "" {
		t.Fatal("expected non-empty view")
	}
}

func TestPortForwardOverlayEscape(t *testing.T) {
	pf := NewPortForwardOverlay(80, 30)
	pf.Open("my-pod", "default", testPorts[:1])
	pf, _ = pf.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if pf.Active() {
		t.Fatal("expected overlay to be inactive after Escape")
	}
}

func TestPortForwardOverlaySubmit(t *testing.T) {
	pf := NewPortForwardOverlay(80, 30)
	pf.Open("my-pod", "default", testPorts[:1])
	// Single Enter submits immediately with auto-filled local port
	pf, cmd := pf.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if pf.Active() {
		t.Fatal("expected overlay to be inactive after submit")
	}
	if cmd == nil {
		t.Fatal("expected a command to be returned")
	}
	msg := cmd()
	pfMsg, ok := msg.(msgs.PortForwardRequestedMsg)
	if !ok {
		t.Fatalf("expected PortForwardRequestedMsg, got %T", msg)
	}
	if pfMsg.LocalPort != 80 || pfMsg.RemotePort != 80 {
		t.Errorf("expected ports 80:80, got %d:%d", pfMsg.LocalPort, pfMsg.RemotePort)
	}
	if pfMsg.PodName != "my-pod" || pfMsg.PodNamespace != "default" {
		t.Errorf("expected pod my-pod/default, got %s/%s", pfMsg.PodName, pfMsg.PodNamespace)
	}
}

func TestPortForwardOverlayNoItems(t *testing.T) {
	pf := NewPortForwardOverlay(80, 30)
	pf.Open("my-pod", "default", nil)
	if !pf.Active() {
		t.Fatal("expected overlay to be active even with no ports")
	}
	// Enter should do nothing when no items
	pf, cmd := pf.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("expected no command when no items selected")
	}
}

func TestPortForwardTabCycle(t *testing.T) {
	pf := NewPortForwardOverlay(80, 30)
	pf.Open("my-pod", "default", testPorts[:2])

	// Initially filter is focused (input 0)
	if pf.overlay.FocusedInput() != 0 {
		t.Fatalf("expected filter focused (0), got %d", pf.overlay.FocusedInput())
	}
	if !pf.InputFocused() {
		t.Fatal("expected input focused initially")
	}

	// Tab -> list
	pf, _ = pf.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if pf.overlay.FocusedInput() != -1 {
		t.Fatalf("expected list focused (-1), got %d", pf.overlay.FocusedInput())
	}
	if !pf.InputFocused() {
		t.Fatal("expected input focused (list is still input area)")
	}

	// Tab -> local port (input 1)
	pf, _ = pf.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if pf.overlay.FocusedInput() != 1 {
		t.Fatalf("expected local port focused (1), got %d", pf.overlay.FocusedInput())
	}
	if !pf.InputFocused() {
		t.Fatal("expected input focused on local port")
	}

	// Tab -> buttons
	pf, _ = pf.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if pf.InputFocused() {
		t.Fatal("expected buttons focused after Tab from local port")
	}

	// Tab -> back to filter (input 0)
	pf, _ = pf.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if pf.overlay.FocusedInput() != 0 {
		t.Fatalf("expected filter focused (0), got %d", pf.overlay.FocusedInput())
	}
	if !pf.InputFocused() {
		t.Fatal("expected input focused after cycling back to filter")
	}
}

func TestPortForwardCursorSyncsLocalPort(t *testing.T) {
	pf := NewPortForwardOverlay(80, 30)
	pf.Open("my-pod", "default", testPorts[:2])

	// Initial local port should be first port
	val := pf.overlay.InputValue(1)
	if val != "localhost:80" {
		t.Fatalf("expected 'localhost:80', got %q", val)
	}

	// Move cursor down — local port should sync to second port
	pf, _ = pf.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	val = pf.overlay.InputValue(1)
	if val != "localhost:443" {
		t.Fatalf("expected 'localhost:443', got %q", val)
	}
}

func TestPortForwardCustomLocalPort(t *testing.T) {
	pf := NewPortForwardOverlay(80, 30)
	pf.Open("my-pod", "default", testPorts[:1])

	// Tab to list, then to local port input
	pf, _ = pf.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	pf, _ = pf.Update(tea.KeyPressMsg{Code: tea.KeyTab})

	// Set custom port value
	pf.overlay.Input(1).SetValue("localhost:9090")

	// Submit
	pf, cmd := pf.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a command")
	}
	msg := cmd()
	pfMsg := msg.(msgs.PortForwardRequestedMsg)
	if pfMsg.LocalPort != 9090 {
		t.Fatalf("expected local port 9090, got %d", pfMsg.LocalPort)
	}
	if pfMsg.RemotePort != 80 {
		t.Fatalf("expected remote port 80, got %d", pfMsg.RemotePort)
	}
}

func TestPortForwardFilterSyncsLocalPort(t *testing.T) {
	pf := NewPortForwardOverlay(80, 30)
	pf.Open("my-pod", "default", testPorts)

	// Initial: first port (80)
	if pf.overlay.InputValue(1) != "localhost:80" {
		t.Fatalf("expected 'localhost:80', got %q", pf.overlay.InputValue(1))
	}

	// Type "redis" to filter — should narrow to redis:6379
	for _, ch := range "redis" {
		pf, _ = pf.Update(tea.KeyPressMsg{Code: -1, Text: string(ch)})
	}

	if len(pf.filtered) != 1 {
		t.Fatalf("expected 1 filtered item, got %d", len(pf.filtered))
	}
	if pf.overlay.InputValue(1) != "localhost:6379" {
		t.Fatalf("expected 'localhost:6379', got %q", pf.overlay.InputValue(1))
	}
}

func TestPortForwardInvalidPort(t *testing.T) {
	pf := NewPortForwardOverlay(80, 30)
	pf.Open("my-pod", "default", testPorts[:1])

	// Tab to local port and set invalid value
	pf, _ = pf.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	pf, _ = pf.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	pf.overlay.Input(1).SetValue("localhost:abc")

	// Enter should not close, should show error
	pf, cmd := pf.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("expected no command for invalid port")
	}
	if !pf.Active() {
		t.Fatal("overlay should stay active on invalid port")
	}
}

// ── Focus and button tests ──

func TestPortForwardTabTogglesFocus(t *testing.T) {
	pf := NewPortForwardOverlay(80, 30)
	pf.Open("my-pod", "default", testPorts[:1])

	if !pf.InputFocused() {
		t.Fatal("expected input focused initially")
	}

	// Tab through: filter -> list -> local port -> buttons
	pf, _ = pf.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // list
	pf, _ = pf.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // local port
	pf, _ = pf.Update(tea.KeyPressMsg{Code: tea.KeyTab}) // buttons
	if pf.InputFocused() {
		t.Fatal("expected buttons focused after Tab from local port")
	}

	// Tab back to filter (input).
	pf, _ = pf.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if !pf.InputFocused() {
		t.Fatal("expected input focused after Tab from buttons")
	}
}

func TestPortForwardButtonHotkeys(t *testing.T) {
	pf := NewPortForwardOverlay(80, 30)
	pf.Open("my-pod", "default", testPorts[:1])

	// Tab to buttons: filter -> list -> local port -> buttons
	pf, _ = pf.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	pf, _ = pf.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	pf, _ = pf.Update(tea.KeyPressMsg{Code: tea.KeyTab})

	// Press y — should submit.
	pf, cmd := pf.Update(tea.KeyPressMsg{Code: -1, Text: "y"})
	if pf.Active() {
		t.Fatal("expected overlay to close after y hotkey")
	}
	if cmd == nil {
		t.Fatal("expected a command from y hotkey")
	}
	msg := cmd()
	pfMsg, ok := msg.(msgs.PortForwardRequestedMsg)
	if !ok {
		t.Fatalf("expected PortForwardRequestedMsg, got %T", msg)
	}
	if pfMsg.LocalPort != 80 || pfMsg.RemotePort != 80 {
		t.Errorf("expected ports 80:80, got %d:%d", pfMsg.LocalPort, pfMsg.RemotePort)
	}
}

func TestPortForwardHotkeysIgnoredWhenInputFocused(t *testing.T) {
	pf := NewPortForwardOverlay(80, 30)
	pf.Open("my-pod", "default", testPorts[:1])

	// Input is focused — y/n should go to the input, not act as hotkeys.
	pf, _ = pf.Update(tea.KeyPressMsg{Code: -1, Text: "y"})
	if !pf.Active() {
		t.Fatal("overlay should remain active when y typed into input")
	}

	pf, _ = pf.Update(tea.KeyPressMsg{Code: -1, Text: "n"})
	if !pf.Active() {
		t.Fatal("overlay should remain active when n typed into input")
	}
}

func TestPortForwardButtonNavigation(t *testing.T) {
	pf := NewPortForwardOverlay(80, 30)
	pf.Open("my-pod", "default", testPorts[:1])

	// Tab to buttons: filter -> list -> local port -> buttons
	pf, _ = pf.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	pf, _ = pf.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	pf, _ = pf.Update(tea.KeyPressMsg{Code: tea.KeyTab})

	// Yes is focused by default.
	if pf.FocusedButton() != pfBtnYes {
		t.Fatal("expected Yes button focused")
	}

	// Right arrow → No.
	pf, _ = pf.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	if pf.FocusedButton() != pfBtnNo {
		t.Fatal("expected No button focused after Right")
	}

	// Left arrow → Yes.
	pf, _ = pf.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	if pf.FocusedButton() != pfBtnYes {
		t.Fatal("expected Yes button focused after Left")
	}
}

func TestPortForwardEnterOnNoButton(t *testing.T) {
	pf := NewPortForwardOverlay(80, 30)
	pf.Open("my-pod", "default", testPorts[:1])

	// Tab to buttons, navigate to No, press Enter.
	pf, _ = pf.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	pf, _ = pf.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	pf, _ = pf.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	pf, _ = pf.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	pf, cmd := pf.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if pf.Active() {
		t.Fatal("expected overlay to close after Enter on No")
	}
	if cmd != nil {
		t.Fatal("expected no command from No button")
	}
}

func TestPortForwardUpFromButtonsFocusesInput(t *testing.T) {
	pf := NewPortForwardOverlay(80, 30)
	pf.Open("my-pod", "default", testPorts[:1])

	// Tab to buttons: filter -> list -> local port -> buttons
	pf, _ = pf.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	pf, _ = pf.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	pf, _ = pf.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if pf.InputFocused() {
		t.Fatal("expected buttons focused")
	}

	// Up arrow returns to local port input (1).
	pf, _ = pf.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if !pf.InputFocused() {
		t.Fatal("expected input focused after Up from buttons")
	}
	if pf.overlay.FocusedInput() != 1 {
		t.Fatalf("expected local port focused (1), got %d", pf.overlay.FocusedInput())
	}
}

func TestPortForwardSubmitViaButtonYes(t *testing.T) {
	pf := NewPortForwardOverlay(80, 30)
	pf.Open("my-pod", "default", testPorts[:1])

	// Tab to buttons: filter -> list -> local port -> buttons
	pf, _ = pf.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	pf, _ = pf.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	pf, _ = pf.Update(tea.KeyPressMsg{Code: tea.KeyTab})

	// Yes is focused by default. Press Enter.
	pf, cmd := pf.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if pf.Active() {
		t.Fatal("expected overlay to close after Enter on Yes button")
	}
	if cmd == nil {
		t.Fatal("expected a command from Yes button")
	}
	msg := cmd()
	pfMsg, ok := msg.(msgs.PortForwardRequestedMsg)
	if !ok {
		t.Fatalf("expected PortForwardRequestedMsg, got %T", msg)
	}
	if pfMsg.LocalPort != 80 || pfMsg.RemotePort != 80 {
		t.Errorf("expected ports 80:80, got %d:%d", pfMsg.LocalPort, pfMsg.RemotePort)
	}
	if pfMsg.PodName != "my-pod" || pfMsg.PodNamespace != "default" {
		t.Errorf("expected pod my-pod/default, got %s/%s", pfMsg.PodName, pfMsg.PodNamespace)
	}
}
