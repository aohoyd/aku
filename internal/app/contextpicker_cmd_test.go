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
// so context-picker population can be asserted. The global cluster is a stub
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
	global := ""
	if len(ctxNames) > 0 {
		global = ctxNames[0]
	}
	mgr.Register(cluster.New(global, "", &k8s.Client{Namespace: "default"}, nil, k8s.NewDiscovery(), nil), true)

	km := config.DefaultKeymap()
	cfg := config.DefaultConfig()
	plugin.Reset()
	return New(mgr, km, cfg, nil, nil, nil, nil, layout.OrientationVertical)
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

func TestExecuteCommandPaneContextPicker(t *testing.T) {
	app := newContextTestApp(t, "prod", "staging")
	model, _ := app.executeCommand("pane-context-picker")
	got := model.(App)
	if got.activeOverlay != overlayContextPicker {
		t.Fatalf("expected overlayContextPicker, got %v", got.activeOverlay)
	}
	if !got.contextPicker.Active() {
		t.Fatal("context picker should be active after pane-context-picker command")
	}
	if n := len(got.contextPicker.Filtered()); n != 2 {
		t.Fatalf("expected picker populated with 2 contexts, got %d", n)
	}
}

// TestContextPickerScopeRouting_Global asserts that selecting a context in
// global scope routes through handleGlobalContextSwitch: the overlay closes AND
// the manager's global context actually changes to the selected one.
func TestContextPickerScopeRouting_Global(t *testing.T) {
	a := appWithManager(t, "ctx-a", "ctx-b")
	a.activeOverlay = overlayContextPicker

	if a.mgr.GlobalContext() != "ctx-a" {
		t.Fatalf("precondition: global = %q, want ctx-a", a.mgr.GlobalContext())
	}

	m, _ := a.update(msgs.GlobalContextSelectedMsg{Context: "ctx-b"})
	app := m.(App)

	if app.activeOverlay != overlayNone {
		t.Fatalf("expected overlay closed after global context select, got %v", app.activeOverlay)
	}
	if app.mgr.GlobalContext() != "ctx-b" {
		t.Fatalf("global context = %q, want ctx-b (global scope should switch the baseline)", app.mgr.GlobalContext())
	}
	// An unpinned pane follows global, so the global switch must not pin it.
	if f := app.layout.FocusedSplit(); f != nil && f.Pinned() {
		t.Fatalf("global switch should not pin the focused pane")
	}
}

// TestContextPickerScopeRouting_Pane asserts that selecting a context in pane
// scope routes through handlePaneContextSwitch: the overlay closes, the global
// context is unchanged, and the focused pane is pinned to the selected context.
func TestContextPickerScopeRouting_Pane(t *testing.T) {
	a := appWithManager(t, "ctx-a", "ctx-b")
	a.activeOverlay = overlayContextPicker

	m, _ := a.update(msgs.PaneContextSelectedMsg{Context: "ctx-b"})
	app := m.(App)

	if app.activeOverlay != overlayNone {
		t.Fatalf("expected overlay closed after pane context select, got %v", app.activeOverlay)
	}
	if app.mgr.GlobalContext() != "ctx-a" {
		t.Fatalf("global context = %q, want ctx-a (pane scope must NOT change the global)", app.mgr.GlobalContext())
	}
	focused := app.layout.FocusedSplit()
	if focused == nil {
		t.Fatal("no focused split after pane context select")
	}
	if !focused.Pinned() {
		t.Fatal("focused pane should be pinned after pane-scope context select")
	}
	if focused.Context() != "ctx-b" {
		t.Fatalf("focused pane context = %q, want ctx-b", focused.Context())
	}
}
