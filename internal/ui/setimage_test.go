package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/aohoyd/aku/internal/msgs"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var testContainers = []msgs.ContainerImageChange{
	{Name: "nginx", Image: "nginx:1.25", Init: false},
	{Name: "sidecar", Image: "envoy:v1.28", Init: false},
}

var testGVR = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}

func TestSetImageOverlayOpen(t *testing.T) {
	si := NewSetImageOverlay(80, 30)
	si.Open("my-deploy", "default", testGVR, "deployments", testContainers)
	if !si.Active() {
		t.Fatal("expected overlay to be active after Open")
	}
	if si.overlay.InputCount() != 2 {
		t.Fatalf("expected 2 inputs, got %d", si.overlay.InputCount())
	}
	if si.overlay.InputValue(0) != "nginx:1.25" {
		t.Fatalf("expected 'nginx:1.25', got %q", si.overlay.InputValue(0))
	}
	if si.overlay.InputValue(1) != "envoy:v1.28" {
		t.Fatalf("expected 'envoy:v1.28', got %q", si.overlay.InputValue(1))
	}
}

func TestSetImageOverlayEscape(t *testing.T) {
	si := NewSetImageOverlay(80, 30)
	si.Open("my-deploy", "default", testGVR, "deployments", testContainers)
	si, _ = si.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if si.Active() {
		t.Fatal("expected overlay to be inactive after Escape")
	}
}

func TestSetImageOverlaySubmitChanged(t *testing.T) {
	si := NewSetImageOverlay(80, 30)
	si.Open("my-deploy", "default", testGVR, "deployments", testContainers)

	si.overlay.Input(0).SetValue("nginx:1.26")

	si, cmd := si.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if si.Active() {
		t.Fatal("expected overlay to be inactive after submit")
	}
	if cmd == nil {
		t.Fatal("expected a command to be returned")
	}
	msg := cmd()
	siMsg, ok := msg.(msgs.SetImageRequestedMsg)
	if !ok {
		t.Fatalf("expected SetImageRequestedMsg, got %T", msg)
	}
	if len(siMsg.Images) != 1 {
		t.Fatalf("expected 1 changed image, got %d", len(siMsg.Images))
	}
	if siMsg.Images[0].Name != "nginx" || siMsg.Images[0].Image != "nginx:1.26" {
		t.Errorf("unexpected image change: %+v", siMsg.Images[0])
	}
	if siMsg.ResourceName != "my-deploy" {
		t.Errorf("expected resource name 'my-deploy', got %q", siMsg.ResourceName)
	}
}

func TestSetImageOverlaySubmitUnchanged(t *testing.T) {
	si := NewSetImageOverlay(80, 30)
	si.Open("my-deploy", "default", testGVR, "deployments", testContainers)

	si, cmd := si.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if si.Active() {
		t.Fatal("expected overlay to close on unchanged submit")
	}
	if cmd != nil {
		t.Fatal("expected no command when nothing changed")
	}
}

func TestSetImageOverlayTabCycle(t *testing.T) {
	si := NewSetImageOverlay(80, 30)
	si.Open("my-deploy", "default", testGVR, "deployments", testContainers)

	if si.overlay.FocusedInput() != 0 {
		t.Fatalf("expected input 0 focused, got %d", si.overlay.FocusedInput())
	}

	si, _ = si.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if si.overlay.FocusedInput() != 1 {
		t.Fatalf("expected input 1 focused, got %d", si.overlay.FocusedInput())
	}

	si, _ = si.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if si.overlay.FocusedInput() != 0 {
		t.Fatalf("expected input 0 focused after wrap, got %d", si.overlay.FocusedInput())
	}
}

func TestSetImageOverlaySingleContainer(t *testing.T) {
	si := NewSetImageOverlay(80, 30)
	si.Open("my-pod", "default", testGVR, "pods", testContainers[:1])
	if si.overlay.InputCount() != 1 {
		t.Fatalf("expected 1 input, got %d", si.overlay.InputCount())
	}
}

func TestSetImageOverlayInitContainer(t *testing.T) {
	containers := []msgs.ContainerImageChange{
		{Name: "app", Image: "myapp:v1", Init: false},
		{Name: "init-db", Image: "busybox:1.0", Init: true},
	}
	si := NewSetImageOverlay(80, 30)
	si.Open("my-deploy", "default", testGVR, "deployments", containers)

	si.overlay.Input(1).SetValue("busybox:2.0")

	si, cmd := si.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected a command")
	}
	msg := cmd().(msgs.SetImageRequestedMsg)
	if len(msg.Images) != 1 {
		t.Fatalf("expected 1 changed image, got %d", len(msg.Images))
	}
	if msg.Images[0].Init != true {
		t.Fatal("expected init flag to be preserved")
	}
}

func TestSetImageOverlayView(t *testing.T) {
	si := NewSetImageOverlay(80, 30)
	si.Open("my-deploy", "default", testGVR, "deployments", testContainers)
	view := si.View()
	if view == "" {
		t.Fatal("expected non-empty view")
	}
}
