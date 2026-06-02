package app

import (
	"testing"

	"github.com/aohoyd/aku/internal/cluster"
	"github.com/aohoyd/aku/internal/config"
	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/layout"
	"github.com/aohoyd/aku/internal/msgs"
	"github.com/aohoyd/aku/internal/plugin"
)

// newContextTestApp builds an App whose manager knows several context entries,
// so context-picker population can be asserted. The startup cluster is a stub
// (default-namespace client, nil store) registered under the first name; the
// picker lists names from Entries, not from live clusters, so the extra
// entries need not be connected.
func newContextTestApp(t *testing.T, ctxNames ...string) App {
	t.Helper()
	entries := make([]cluster.ContextEntry, len(ctxNames))
	for i, n := range ctxNames {
		entries[i] = cluster.ContextEntry{Name: n, File: "/dev/null"}
	}
	mgr := cluster.NewManager(entries, "", 0)
	startup := ""
	if len(ctxNames) > 0 {
		startup = ctxNames[0]
	}
	mgr.Register(cluster.New(startup, "", &k8s.Client{Namespace: "default"}, nil, k8s.NewDiscovery(), nil))

	km := config.DefaultKeymap()
	cfg := config.DefaultConfig()
	plugin.Reset()
	return New(mgr, km, cfg, nil, nil, nil, nil, layout.OrientationVertical, startup)
}

func TestExecuteCommandContextPicker(t *testing.T) {
	app := newContextTestApp(t, "prod", "staging", "dev")
	model, _ := app.executeCommand("context-picker")
	got := model.(App)
	if got.activeOverlay != overlayContextPicker {
		t.Fatalf("expected overlayContextPicker, got %v", got.activeOverlay)
	}
	if !got.contextPicker.Active() {
		t.Fatal("context picker should be active after context-picker command")
	}
	if n := len(got.contextPicker.Filtered()); n != 3 {
		t.Fatalf("expected picker populated with 3 contexts, got %d", n)
	}
}

// TestContextPickerSelectionRouting asserts that selecting a context routes
// through handleGroupContextSwitch: the overlay closes AND the focused pane's
// context group moves to the selected one.
func TestContextPickerSelectionRouting(t *testing.T) {
	a := appWithManager(t, "ctx-a", "ctx-b")
	a.activeOverlay = overlayContextPicker

	if got := a.contextFor(a.layout.FocusedSplit()); got != "ctx-a" {
		t.Fatalf("precondition: focused context = %q, want ctx-a", got)
	}

	m, _ := a.update(msgs.GlobalContextSelectedMsg{Context: "ctx-b"})
	app := m.(App)

	if app.activeOverlay != overlayNone {
		t.Fatalf("expected overlay closed after global context select, got %v", app.activeOverlay)
	}
	if got := app.contextFor(app.layout.FocusedSplit()); got != "ctx-b" {
		t.Fatalf("focused context = %q, want ctx-b (selection should switch the focused group)", got)
	}
}
