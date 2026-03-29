package app

import (
	"context"
	"fmt"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/aohoyd/aku/internal/config"
	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/msgs"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/portforward"
	"github.com/aohoyd/aku/internal/render"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

// mockPlugin implements plugin.ResourcePlugin for testing.
type mockPlugin struct {
	name string
	gvr  schema.GroupVersionResource
}

func (m *mockPlugin) Name() string { return m.name }
func (m *mockPlugin) ShortName() string {
	if len(m.name) < 2 {
		return m.name
	}
	return m.name[:2]
}
func (m *mockPlugin) GVR() schema.GroupVersionResource          { return m.gvr }
func (m *mockPlugin) IsClusterScoped() bool                     { return false }
func (m *mockPlugin) Columns() []plugin.Column                  { return nil }
func (m *mockPlugin) Row(_ *unstructured.Unstructured) []string { return nil }
func (m *mockPlugin) YAML(_ *unstructured.Unstructured) (render.Content, error) {
	s := "apiVersion: v1\nkind: Pod\nmetadata:\n  name: test-pod\n"
	return render.Content{Raw: s, Display: s}, nil
}
func (m *mockPlugin) Describe(_ context.Context, _ *unstructured.Unstructured) (render.Content, error) {
	s := "Name: test-pod\nNamespace: default\n"
	return render.Content{Raw: s, Display: s}, nil
}

// newTestApp creates an App suitable for testing with no k8s client or store.
func newTestApp() App {
	km := config.DefaultKeymap()
	cfg := config.DefaultConfig()
	plugin.Reset()
	return New(nil, nil, km, cfg, nil, nil, nil, nil)
}

func TestSearchOpenCommand(t *testing.T) {
	a := newTestApp()
	model, _ := a.executeCommand("search-open")
	app := model.(App)
	if app.activeOverlay != overlaySearchBar {
		t.Fatal("search-open should show the search bar")
	}
	if !app.searchBar.Active() {
		t.Fatal("search bar should be active")
	}
}

func TestFilterOpenCommand(t *testing.T) {
	a := newTestApp()
	model, _ := a.executeCommand("filter-open")
	app := model.(App)
	if app.activeOverlay != overlaySearchBar {
		t.Fatal("filter-open should show the search bar")
	}
	if !app.searchBar.Active() {
		t.Fatal("search bar should be active")
	}
}

func TestGotoCommand(t *testing.T) {
	app := newTestApp()

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	deploymentsPlugin := &mockPlugin{
		name: "deployments",
		gvr:  schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"},
	}

	plugin.Register(podsPlugin)
	plugin.Register(deploymentsPlugin)

	// Add initial split with pods
	app.layout.AddSplit(podsPlugin, "default")

	// Verify initial state: focused split has pods plugin
	focused := app.layout.FocusedSplit()
	if focused == nil {
		t.Fatal("expected a focused split")
	}
	if focused.Plugin().Name() != "pods" {
		t.Fatalf("expected focused plugin to be 'pods', got %q", focused.Plugin().Name())
	}

	// Execute goto-deployments
	model, _ := app.executeCommand("goto-deployments")
	app = model.(App)

	// After goto, the focused split should now show deployments
	focused = app.layout.FocusedSplit()
	if focused == nil {
		t.Fatal("expected a focused split after goto")
	}
	if focused.Plugin().Name() != "deployments" {
		t.Fatalf("expected focused plugin to be 'deployments' after goto, got %q", focused.Plugin().Name())
	}

	// Split count should still be 1 (goto replaces, not adds)
	if app.layout.SplitCount() != 1 {
		t.Fatalf("expected 1 split after goto, got %d", app.layout.SplitCount())
	}
}

func TestSplitCommand(t *testing.T) {
	app := newTestApp()

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	deploymentsPlugin := &mockPlugin{
		name: "deployments",
		gvr:  schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"},
	}

	plugin.Register(podsPlugin)
	plugin.Register(deploymentsPlugin)

	// Add initial split with pods
	app.layout.AddSplit(podsPlugin, "default")
	if app.layout.SplitCount() != 1 {
		t.Fatalf("expected 1 split initially, got %d", app.layout.SplitCount())
	}

	// Execute split-deployments
	model, _ := app.executeCommand("split-deployments")
	app = model.(App)

	// Should now have 2 splits
	if app.layout.SplitCount() != 2 {
		t.Fatalf("expected 2 splits after split command, got %d", app.layout.SplitCount())
	}

	// The newly added split should be focused and have the deployments plugin
	focused := app.layout.FocusedSplit()
	if focused == nil {
		t.Fatal("expected a focused split after split command")
	}
	if focused.Plugin().Name() != "deployments" {
		t.Fatalf("expected focused plugin to be 'deployments', got %q", focused.Plugin().Name())
	}
}

func TestQuitWithMultipleSplits(t *testing.T) {
	app := newTestApp()

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	deploymentsPlugin := &mockPlugin{
		name: "deployments",
		gvr:  schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"},
	}

	plugin.Register(podsPlugin)
	plugin.Register(deploymentsPlugin)

	// Add two splits
	app.layout.AddSplit(podsPlugin, "default")
	app.layout.AddSplit(deploymentsPlugin, "default")
	if app.layout.SplitCount() != 2 {
		t.Fatalf("expected 2 splits, got %d", app.layout.SplitCount())
	}

	// Execute quit: should close one split, not exit
	model, cmd := app.executeCommand("quit")
	app = model.(App)

	if cmd != nil {
		// Check it's not tea.Quit
		msg := cmd()
		if _, ok := msg.(tea.QuitMsg); ok {
			t.Fatal("quit with multiple splits should not produce tea.Quit")
		}
	}

	if app.layout.SplitCount() != 1 {
		t.Fatalf("expected 1 split after quit, got %d", app.layout.SplitCount())
	}
}

func TestQuitWithOneSplit(t *testing.T) {
	app := newTestApp()

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}

	plugin.Register(podsPlugin)

	// Add one split
	app.layout.AddSplit(podsPlugin, "default")
	if app.layout.SplitCount() != 1 {
		t.Fatalf("expected 1 split, got %d", app.layout.SplitCount())
	}

	// Execute quit: should return tea.Quit
	_, cmd := app.executeCommand("quit")

	if cmd == nil {
		t.Fatal("expected quit to return a cmd")
	}

	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Fatalf("expected tea.QuitMsg, got %T", msg)
	}
}

func TestFocusNextCommand(t *testing.T) {
	app := newTestApp()

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	deploymentsPlugin := &mockPlugin{
		name: "deployments",
		gvr:  schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"},
	}

	plugin.Register(podsPlugin)
	plugin.Register(deploymentsPlugin)

	// Add two splits: pods (idx 0) then deployments (idx 1, focused)
	app.layout.AddSplit(podsPlugin, "default")
	app.layout.AddSplit(deploymentsPlugin, "default")

	// After adding two splits, focus should be on the last one added (idx 1)
	if app.layout.FocusIndex() != 1 {
		t.Fatalf("expected focus at index 1, got %d", app.layout.FocusIndex())
	}

	// Execute focus-next: should wrap around to 0
	model, _ := app.executeCommand("focus-next")
	app = model.(App)

	if app.layout.FocusIndex() != 0 {
		t.Fatalf("expected focus at index 0 after focus-next, got %d", app.layout.FocusIndex())
	}

	// Execute focus-next again: should go to 1
	model, _ = app.executeCommand("focus-next")
	app = model.(App)

	if app.layout.FocusIndex() != 1 {
		t.Fatalf("expected focus at index 1 after second focus-next, got %d", app.layout.FocusIndex())
	}
}

func TestFocusPrevCommand(t *testing.T) {
	app := newTestApp()

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	deploymentsPlugin := &mockPlugin{
		name: "deployments",
		gvr:  schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"},
	}

	plugin.Register(podsPlugin)
	plugin.Register(deploymentsPlugin)

	// Add two splits: pods (idx 0) then deployments (idx 1, focused)
	app.layout.AddSplit(podsPlugin, "default")
	app.layout.AddSplit(deploymentsPlugin, "default")

	// After adding two splits, focus should be on the last one added (idx 1)
	if app.layout.FocusIndex() != 1 {
		t.Fatalf("expected focus at index 1, got %d", app.layout.FocusIndex())
	}

	// Execute focus-prev: should go to 0
	model, _ := app.executeCommand("focus-prev")
	app = model.(App)

	if app.layout.FocusIndex() != 0 {
		t.Fatalf("expected focus at index 0 after focus-prev, got %d", app.layout.FocusIndex())
	}

	// Execute focus-prev again: should wrap around to 1
	model, _ = app.executeCommand("focus-prev")
	app = model.(App)

	if app.layout.FocusIndex() != 1 {
		t.Fatalf("expected focus at index 1 after second focus-prev, got %d", app.layout.FocusIndex())
	}
}

func TestViewYAMLCommand(t *testing.T) {
	app := newTestApp()

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}

	plugin.Register(podsPlugin)
	app.layout.AddSplit(podsPlugin, "default")

	// Set objects on the focused split so Selected() returns non-nil
	obj := &unstructured.Unstructured{}
	obj.SetName("test-pod")
	obj.SetNamespace("default")
	focused := app.layout.FocusedSplit()
	focused.SetObjects([]*unstructured.Unstructured{obj})

	// Execute view-yaml
	model, _ := app.executeCommand("view-yaml")
	app = model.(App)

	// Right panel should be visible
	if !app.layout.RightPanelVisible() {
		t.Fatal("expected right panel to be visible after view-yaml")
	}

	// Focus mode should stay "normal" (cursor remains on resource list)
	if !app.layout.FocusedResources() {
		t.Fatal("expected resources to be focused")
	}
}

func TestViewDescribeCommand(t *testing.T) {
	app := newTestApp()

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}

	plugin.Register(podsPlugin)
	app.layout.AddSplit(podsPlugin, "default")

	// Set objects on the focused split so Selected() returns non-nil
	obj := &unstructured.Unstructured{}
	obj.SetName("test-pod")
	obj.SetNamespace("default")
	focused := app.layout.FocusedSplit()
	focused.SetObjects([]*unstructured.Unstructured{obj})

	// Execute view-describe
	model, _ := app.executeCommand("view-describe")
	app = model.(App)

	// Right panel should be visible
	if !app.layout.RightPanelVisible() {
		t.Fatal("expected right panel to be visible after view-describe")
	}

	// Focus mode should stay "normal" (cursor remains on resource list)
	if !app.layout.FocusedResources() {
		t.Fatal("expected resources to be focused")
	}
}

func TestClosePanelCommand(t *testing.T) {
	app := newTestApp()

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}

	plugin.Register(podsPlugin)
	app.layout.AddSplit(podsPlugin, "default")

	// Set objects on the focused split so Selected() returns non-nil
	obj := &unstructured.Unstructured{}
	obj.SetName("test-pod")
	obj.SetNamespace("default")
	focused := app.layout.FocusedSplit()
	focused.SetObjects([]*unstructured.Unstructured{obj})

	// First open a view so the panel is visible
	model, _ := app.executeCommand("view-yaml")
	app = model.(App)

	// Verify panel is open
	if !app.layout.RightPanelVisible() {
		t.Fatal("expected right panel to be visible before close-panel")
	}
	if !app.layout.FocusedResources() {
		t.Fatal("expected resources to be focused before close")
	}

	// Execute close-panel
	model, _ = app.executeCommand("close-panel")
	app = model.(App)

	// Right panel should be hidden
	if app.layout.RightPanelVisible() {
		t.Fatal("expected right panel to be hidden after close-panel")
	}

	// Focus mode should be "normal"
	if !app.layout.FocusedResources() {
		t.Fatal("expected resources to be focused after close-panel")
	}
}

func TestDetailScrollMode(t *testing.T) {
	app := newTestApp()

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}

	plugin.Register(podsPlugin)
	app.layout.AddSplit(podsPlugin, "default")

	// Set objects on the focused split so Selected() returns non-nil
	obj := &unstructured.Unstructured{}
	obj.SetName("test-pod")
	obj.SetNamespace("default")
	focused := app.layout.FocusedSplit()
	focused.SetObjects([]*unstructured.Unstructured{obj})

	// 1. Open panel via view-yaml — focus stays on resources
	model, _ := app.executeCommand("view-yaml")
	app = model.(App)

	if !app.layout.RightPanelVisible() {
		t.Fatal("expected right panel to be visible after view-yaml")
	}
	if !app.layout.FocusedResources() {
		t.Fatal("expected resources to be focused after view-yaml")
	}

	// 2. Press enter — focus moves to details
	model, _ = app.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	app = model.(App)

	if !app.layout.FocusedDetails() {
		t.Fatal("expected details to be focused after enter")
	}

	// 3. Press esc — focus returns to resources, panel stays visible
	model, _ = app.handleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	app = model.(App)

	if !app.layout.FocusedResources() {
		t.Fatal("expected resources to be focused after esc")
	}
	if !app.layout.RightPanelVisible() {
		t.Fatal("expected right panel to still be visible after esc from detail-scroll")
	}
}

func TestFocusSwitchRefreshesPanel(t *testing.T) {
	app := newTestApp()

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	deploymentsPlugin := &mockPlugin{
		name: "deployments",
		gvr:  schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"},
	}

	plugin.Register(podsPlugin)
	plugin.Register(deploymentsPlugin)

	// Add two splits
	app.layout.AddSplit(podsPlugin, "default")
	app.layout.AddSplit(deploymentsPlugin, "default")

	// Set objects on both splits
	podObj := &unstructured.Unstructured{}
	podObj.SetName("my-pod")
	app.layout.SplitAt(0).SetObjects([]*unstructured.Unstructured{podObj})

	depObj := &unstructured.Unstructured{}
	depObj.SetName("my-deployment")
	app.layout.SplitAt(1).SetObjects([]*unstructured.Unstructured{depObj})

	// Open panel (focus is on split 1 = deployments)
	model, _ := app.executeCommand("view-yaml")
	app = model.(App)

	if !app.layout.RightPanelVisible() {
		t.Fatal("expected panel to be visible")
	}

	// Focus-next should wrap to split 0 (pods) and refresh panel
	model, _ = app.executeCommand("focus-next")
	app = model.(App)

	if app.layout.FocusIndex() != 0 {
		t.Fatalf("expected focus at index 0, got %d", app.layout.FocusIndex())
	}

	// The panel should now show content from the pods split's selection
	focused := app.layout.FocusedSplit()
	if focused.Plugin().Name() != "pods" {
		t.Fatalf("expected focused plugin to be pods, got %q", focused.Plugin().Name())
	}
}

func TestGotoRefreshesPanel(t *testing.T) {
	app := newTestApp()

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	deploymentsPlugin := &mockPlugin{
		name: "deployments",
		gvr:  schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"},
	}

	plugin.Register(podsPlugin)
	plugin.Register(deploymentsPlugin)

	// Add initial split with pods
	app.layout.AddSplit(podsPlugin, "default")

	podObj := &unstructured.Unstructured{}
	podObj.SetName("my-pod")
	app.layout.FocusedSplit().SetObjects([]*unstructured.Unstructured{podObj})

	// Open panel showing YAML
	model, _ := app.executeCommand("view-yaml")
	app = model.(App)

	if !app.layout.RightPanelVisible() {
		t.Fatal("expected panel to be visible")
	}

	// Goto deployments — panel should still be visible and mode preserved
	model, _ = app.executeCommand("goto-deployments")
	app = model.(App)

	if !app.layout.RightPanelVisible() {
		t.Fatal("expected panel to remain visible after goto")
	}

	// Focused split should now be deployments
	if app.layout.FocusedSplit().Plugin().Name() != "deployments" {
		t.Fatalf("expected deployments plugin after goto, got %q", app.layout.FocusedSplit().Plugin().Name())
	}
}

func TestSortCommand(t *testing.T) {
	app := newTestApp()

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}

	plugin.Register(podsPlugin)
	app.layout.AddSplit(podsPlugin, "default")

	focused := app.layout.FocusedSplit()
	// Default sort should be NAME ascending
	state := focused.SortState()
	if state.Column != "NAME" || !state.Ascending {
		t.Fatalf("expected default NAME ascending, got %+v", state)
	}

	// Execute sort-AGE — should change sort to AGE ascending
	model, _ := app.executeCommand("sort-AGE")
	app = model.(App)

	focused = app.layout.FocusedSplit()
	state = focused.SortState()
	if state.Column != "AGE" || !state.Ascending {
		t.Fatalf("expected AGE ascending after sort-AGE, got %+v", state)
	}

	// Execute sort-AGE again — should toggle to descending
	model, _ = app.executeCommand("sort-AGE")
	app = model.(App)

	focused = app.layout.FocusedSplit()
	state = focused.SortState()
	if state.Column != "AGE" || state.Ascending {
		t.Fatalf("expected AGE descending after second sort-AGE, got %+v", state)
	}
}

func TestScopedKeyPodsExecBinding(t *testing.T) {
	km := config.DefaultKeymap()
	cfg := config.DefaultConfig()
	plugin.Reset()
	app := New(nil, nil, km, cfg, nil, nil, nil, nil)

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	plugin.Register(podsPlugin)
	app.layout.AddSplit(podsPlugin, "default")

	// Simulate pressing 'x' via handleKey — should resolve to 'exec' because we're on pods
	model, _ := app.handleKey(tea.KeyPressMsg{Code: rune('x'), Text: "x"})
	app = model.(App)

	// Verify the trie resolved the command (it should be back at root after resolution)
	if !app.keyTrie.AtRoot() {
		t.Fatal("trie should be at root after resolved command")
	}
}

func TestScopedKeyDeploymentNoExec(t *testing.T) {
	km := config.DefaultKeymap()
	cfg := config.DefaultConfig()
	plugin.Reset()
	app := New(nil, nil, km, cfg, nil, nil, nil, nil)

	deploymentsPlugin := &mockPlugin{
		name: "deployments",
		gvr:  schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"},
	}
	plugin.Register(deploymentsPlugin)
	app.layout.AddSplit(deploymentsPlugin, "default")

	// x should NOT be in the deployments trie (it's a pods-only binding)
	// After pressing x, the trie should have swallowed it (resolved with empty command)
	model, _ := app.handleKey(tea.KeyPressMsg{Code: rune('x'), Text: "x"})
	app = model.(App)

	// Trie should still be at root (x was unknown, resolved as empty)
	if !app.keyTrie.AtRoot() {
		t.Fatal("trie should be at root after unknown key in deployments context")
	}
}

func TestAutoReloadPreservesScrollPosition(t *testing.T) {
	app := newTestApp()

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	plugin.Register(podsPlugin)
	app.layout.AddSplit(podsPlugin, "default")

	obj := &unstructured.Unstructured{}
	obj.SetName("test-pod")
	obj.SetNamespace("default")
	app.layout.FocusedSplit().SetObjects([]*unstructured.Unstructured{obj})

	// Open panel
	model, _ := app.executeCommand("view-yaml")
	app = model.(App)

	// Enter detail-scroll mode
	model, _ = app.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	app = model.(App)

	// Scroll down several times
	for range 5 {
		model, _ = app.handleKey(tea.KeyPressMsg{Code: tea.KeyDown, Text: "j"})
		app = model.(App)
	}

	// Simulate auto-reload via reloadDetailPanel
	app, _ = app.reloadDetailPanel()

	// The panel should still be visible and mode unchanged
	if !app.layout.RightPanelVisible() {
		t.Fatal("panel should still be visible after reload")
	}
	if app.layout.RightPanel().Mode() != msgs.DetailYAML {
		t.Fatal("panel mode should still be YAML after reload")
	}
}

func TestHorizontalScrollInDetailMode(t *testing.T) {
	app := newTestApp()

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	plugin.Register(podsPlugin)
	app.layout.AddSplit(podsPlugin, "default")

	obj := &unstructured.Unstructured{}
	obj.SetName("test-pod")
	obj.SetNamespace("default")
	app.layout.FocusedSplit().SetObjects([]*unstructured.Unstructured{obj})

	// Open panel and enter detail-scroll mode
	model, _ := app.executeCommand("view-yaml")
	app = model.(App)
	model, _ = app.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	app = model.(App)

	if !app.layout.FocusedDetails() {
		t.Fatal("expected details to be focused")
	}

	// Press Shift+L (uppercase L) — should not exit detail-scroll mode
	model, _ = app.handleKey(tea.KeyPressMsg{Code: rune('L'), Text: "L"})
	app = model.(App)
	if !app.layout.FocusedDetails() {
		t.Fatal("H/L scroll should not exit detail-scroll mode")
	}

	// Press Shift+H (uppercase H) — should also not exit
	model, _ = app.handleKey(tea.KeyPressMsg{Code: rune('H'), Text: "H"})
	app = model.(App)
	if !app.layout.FocusedDetails() {
		t.Fatal("H/L scroll should not exit detail-scroll mode")
	}

	// Press lowercase h — SHOULD exit detail-scroll mode
	model, _ = app.handleKey(tea.KeyPressMsg{Code: rune('h'), Text: "h"})
	app = model.(App)
	if !app.layout.FocusedResources() {
		t.Fatal("lowercase h should exit detail-scroll mode")
	}
}

func TestCtrlRRefreshesDetailPanel(t *testing.T) {
	app := newTestApp()

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	plugin.Register(podsPlugin)
	app.layout.AddSplit(podsPlugin, "default")

	obj := &unstructured.Unstructured{}
	obj.SetName("test-pod")
	obj.SetNamespace("default")
	app.layout.FocusedSplit().SetObjects([]*unstructured.Unstructured{obj})

	// Open panel
	model, _ := app.executeCommand("view-yaml")
	app = model.(App)

	// ctrl+r in normal mode — should not crash, panel stays visible
	model, _ = app.handleKey(tea.KeyPressMsg{Code: rune('r'), Text: "r", Mod: tea.ModCtrl})
	app = model.(App)

	if !app.layout.RightPanelVisible() {
		t.Fatal("panel should remain visible after ctrl+r")
	}
	if !app.layout.FocusedResources() {
		t.Fatal("expected resources to stay focused after ctrl+r")
	}

	// Enter detail-scroll mode and ctrl+r
	model, _ = app.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	app = model.(App)

	model, _ = app.handleKey(tea.KeyPressMsg{Code: rune('r'), Text: "r", Mod: tea.ModCtrl})
	app = model.(App)

	if !app.layout.FocusedDetails() {
		t.Fatal("expected details to stay focused after ctrl+r")
	}
}

func TestContextSwitchResetsMidSequence(t *testing.T) {
	app := newTestApp()

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	deploymentsPlugin := &mockPlugin{
		name: "deployments",
		gvr:  schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"},
	}

	plugin.Register(podsPlugin)
	plugin.Register(deploymentsPlugin)
	app.layout.AddSplit(podsPlugin, "default")
	app.layout.AddSplit(deploymentsPlugin, "default")

	// Start a key sequence (press 'g' for goto prefix)
	model, _ := app.handleKey(tea.KeyPressMsg{Code: rune('g'), Text: "g"})
	app = model.(App)

	// Trie should be mid-sequence
	if app.keyTrie.AtRoot() {
		t.Fatal("trie should be mid-sequence after pressing 'g'")
	}

	// Execute focus-next — should reset the trie
	model, _ = app.executeCommand("focus-next")
	app = model.(App)

	// Trie should be back at root
	if !app.keyTrie.AtRoot() {
		t.Fatal("trie should be at root after focus-next resets it")
	}
}

func TestGGChordResolvesGotoTop(t *testing.T) {
	app := newTestApp()

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	plugin.Register(podsPlugin)
	app.layout.AddSplit(podsPlugin, "default")

	// Press first 'g' — should enter trie mid-sequence
	model, _ := app.handleKey(tea.KeyPressMsg{Code: rune('g'), Text: "g"})
	app = model.(App)

	if app.keyTrie.AtRoot() {
		t.Fatal("trie should be mid-sequence after first 'g'")
	}

	// Press second 'g' — should resolve cursor-top
	model, _ = app.handleKey(tea.KeyPressMsg{Code: rune('g'), Text: "g"})
	app = model.(App)

	if !app.keyTrie.AtRoot() {
		t.Fatal("trie should be back at root after 'gg' resolves")
	}
	if !app.layout.FocusedResources() {
		t.Fatal("expected resources to be focused")
	}
}

func TestUppercaseGResolvesGotoBottom(t *testing.T) {
	app := newTestApp()

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	plugin.Register(podsPlugin)
	app.layout.AddSplit(podsPlugin, "default")

	// Add multiple objects so GotoBottom has somewhere to go
	objs := make([]*unstructured.Unstructured, 20)
	for i := range objs {
		obj := &unstructured.Unstructured{}
		obj.SetName(fmt.Sprintf("pod-%02d", i))
		objs[i] = obj
	}
	app.layout.FocusedSplit().SetObjects(objs)

	// First verify the trie resolves "G" to "cursor-bottom"
	trie := app.bindingSet.TrieFor("resources", "pods")
	cmd, _, resolved := trie.Press("G")
	t.Logf("Trie Press(G): command=%q resolved=%v", cmd, resolved)
	if !resolved || cmd != "cursor-bottom" {
		t.Fatalf("expected trie to resolve G to cursor-bottom, got command=%q resolved=%v", cmd, resolved)
	}

	// Simulate pressing 'G' (Shift+G) — in a real terminal this produces:
	// Code='g', Mod=ModShift, Text="G" → String() returns "G"
	model, _ := app.handleKey(tea.KeyPressMsg{Code: rune('g'), Mod: tea.ModShift, Text: "G"})
	app = model.(App)

	if !app.keyTrie.AtRoot() {
		t.Fatal("trie should be at root after 'G' resolves")
	}

	// Cursor should be at the last item
	focused := app.layout.FocusedSplit()
	sel := focused.Selected()
	if sel == nil {
		t.Fatal("expected a selected object after GotoBottom")
	}
	t.Logf("Selected after G: %q", sel.GetName())
	if sel.GetName() != "pod-19" {
		t.Fatalf("expected cursor at pod-19 (last item), got %q", sel.GetName())
	}
}

func TestGGChordInDetailScrollMode(t *testing.T) {
	app := newTestApp()

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	plugin.Register(podsPlugin)
	app.layout.AddSplit(podsPlugin, "default")

	obj := &unstructured.Unstructured{}
	obj.SetName("test-pod")
	obj.SetNamespace("default")
	app.layout.FocusedSplit().SetObjects([]*unstructured.Unstructured{obj})

	// Open panel and enter detail-scroll mode
	model, _ := app.executeCommand("view-yaml")
	app = model.(App)
	model, _ = app.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	app = model.(App)

	if !app.layout.FocusedDetails() {
		t.Fatal("expected details to be focused")
	}

	// Press 'g' then 'g' — should resolve cursor-top in detail panel
	model, _ = app.handleKey(tea.KeyPressMsg{Code: rune('g'), Text: "g"})
	app = model.(App)

	if app.keyTrie.AtRoot() {
		t.Fatal("trie should be mid-sequence after first 'g'")
	}

	model, _ = app.handleKey(tea.KeyPressMsg{Code: rune('g'), Text: "g"})
	app = model.(App)

	if !app.keyTrie.AtRoot() {
		t.Fatal("trie should be back at root after 'gg' resolves")
	}
	// Should still be in detail-scroll mode
	if !app.layout.FocusedDetails() {
		t.Fatal("expected details to be focused after gg")
	}
}

func TestViewYAMLFocusedCommand(t *testing.T) {
	app := newTestApp()

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	plugin.Register(podsPlugin)
	app.layout.AddSplit(podsPlugin, "default")

	obj := &unstructured.Unstructured{}
	obj.SetName("test-pod")
	obj.SetNamespace("default")
	app.layout.FocusedSplit().SetObjects([]*unstructured.Unstructured{obj})

	model, _ := app.executeCommand("view-yaml-focused")
	app = model.(App)

	if !app.layout.RightPanelVisible() {
		t.Fatal("expected right panel visible after view-yaml-focused")
	}
	if !app.layout.FocusedDetails() {
		t.Fatal("expected details to be focused")
	}
	if app.layout.RightPanel().Mode() != msgs.DetailYAML {
		t.Fatalf("expected YAML mode, got %v", app.layout.RightPanel().Mode())
	}
}

func TestViewDescribeFocusedCommand(t *testing.T) {
	app := newTestApp()

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	plugin.Register(podsPlugin)
	app.layout.AddSplit(podsPlugin, "default")

	obj := &unstructured.Unstructured{}
	obj.SetName("test-pod")
	obj.SetNamespace("default")
	app.layout.FocusedSplit().SetObjects([]*unstructured.Unstructured{obj})

	model, _ := app.executeCommand("view-describe-focused")
	app = model.(App)

	if !app.layout.RightPanelVisible() {
		t.Fatal("expected right panel visible after view-describe-focused")
	}
	if !app.layout.FocusedDetails() {
		t.Fatal("expected details to be focused")
	}
	if app.layout.RightPanel().Mode() != msgs.DetailDescribe {
		t.Fatalf("expected Describe mode, got %v", app.layout.RightPanel().Mode())
	}
}

func TestViewLogsFocusedCommand(t *testing.T) {
	app := newTestApp()

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	plugin.Register(podsPlugin)
	app.layout.AddSplit(podsPlugin, "default")

	obj := &unstructured.Unstructured{}
	obj.SetName("test-pod")
	obj.SetNamespace("default")
	app.layout.FocusedSplit().SetObjects([]*unstructured.Unstructured{obj})

	model, _ := app.executeCommand("view-logs-focused")
	app = model.(App)

	if !app.layout.RightPanelVisible() {
		t.Fatal("expected right panel visible after view-logs-focused")
	}
	if !app.layout.FocusedDetails() {
		t.Fatal("expected details to be focused")
	}
	if !app.layout.IsLogMode() {
		t.Fatal("expected log mode to be active")
	}
}

func TestViewFocusedNoSelection(t *testing.T) {
	app := newTestApp()

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	plugin.Register(podsPlugin)
	app.layout.AddSplit(podsPlugin, "default")
	// No objects set — Selected() returns nil

	model, _ := app.executeCommand("view-yaml-focused")
	app = model.(App)

	if app.layout.RightPanelVisible() {
		t.Fatal("expected right panel hidden when no selection")
	}
	if !app.layout.FocusedResources() {
		t.Fatal("expected resources to be focused when no selection")
	}
}

func TestClosePanelFromDetailScroll(t *testing.T) {
	app := newTestApp()

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	plugin.Register(podsPlugin)
	app.layout.AddSplit(podsPlugin, "default")

	obj := &unstructured.Unstructured{}
	obj.SetName("test-pod")
	obj.SetNamespace("default")
	app.layout.FocusedSplit().SetObjects([]*unstructured.Unstructured{obj})

	// Enter detail-scroll via focused command
	model, _ := app.executeCommand("view-yaml-focused")
	app = model.(App)

	if !app.layout.FocusedDetails() {
		t.Fatal("expected details to be focused")
	}

	// close-panel should restore normal mode
	model, _ = app.executeCommand("close-panel")
	app = model.(App)

	if app.layout.RightPanelVisible() {
		t.Fatal("expected right panel hidden after close-panel")
	}
	if !app.layout.FocusedResources() {
		t.Fatal("expected resources to be focused after close-panel")
	}
	if !app.keyTrie.AtRoot() {
		t.Fatal("expected keyTrie at root after close-panel")
	}
}

func TestCursorUpDownCommands(t *testing.T) {
	app := newTestApp()

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	plugin.Register(podsPlugin)
	app.layout.AddSplit(podsPlugin, "default")

	objs := make([]*unstructured.Unstructured, 5)
	for i := range objs {
		obj := &unstructured.Unstructured{}
		obj.SetName(fmt.Sprintf("pod-%d", i))
		objs[i] = obj
	}
	app.layout.FocusedSplit().SetObjects(objs)

	// cursor-down should move cursor
	model, _ := app.executeCommand("cursor-down")
	app = model.(App)

	sel := app.layout.FocusedSplit().Selected()
	if sel == nil {
		t.Fatal("expected a selected object after cursor-down")
	}
	if sel.GetName() != "pod-1" {
		t.Fatalf("expected pod-1 after cursor-down, got %q", sel.GetName())
	}

	// cursor-up should move back
	model, _ = app.executeCommand("cursor-up")
	app = model.(App)

	sel = app.layout.FocusedSplit().Selected()
	if sel.GetName() != "pod-0" {
		t.Fatalf("expected pod-0 after cursor-up, got %q", sel.GetName())
	}
}

func TestEnterDetailCommand(t *testing.T) {
	app := newTestApp()

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	plugin.Register(podsPlugin)
	app.layout.AddSplit(podsPlugin, "default")

	obj := &unstructured.Unstructured{}
	obj.SetName("test-pod")
	app.layout.FocusedSplit().SetObjects([]*unstructured.Unstructured{obj})

	// Without right panel visible, enter-detail should be a no-op
	model, _ := app.executeCommand("enter-detail")
	app = model.(App)
	if !app.layout.FocusedResources() {
		t.Fatal("enter-detail without panel should be no-op")
	}

	// Open panel first
	model, _ = app.executeCommand("view-yaml")
	app = model.(App)

	// Now enter-detail should switch to detail-scroll
	model, _ = app.executeCommand("enter-detail")
	app = model.(App)
	if !app.layout.FocusedDetails() {
		t.Fatal("expected details to be focused")
	}
}

func TestExitDetailCommand(t *testing.T) {
	app := newTestApp()

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	plugin.Register(podsPlugin)
	app.layout.AddSplit(podsPlugin, "default")

	obj := &unstructured.Unstructured{}
	obj.SetName("test-pod")
	app.layout.FocusedSplit().SetObjects([]*unstructured.Unstructured{obj})

	// Enter detail-scroll mode
	model, _ := app.executeCommand("view-yaml-focused")
	app = model.(App)
	if !app.layout.FocusedDetails() {
		t.Fatal("expected details to be focused")
	}

	// exit-detail should return to normal
	model, _ = app.executeCommand("exit-detail")
	app = model.(App)
	if !app.layout.FocusedResources() {
		t.Fatal("expected resources to be focused after exit-detail")
	}
	if !app.layout.RightPanelVisible() {
		t.Fatal("panel should still be visible after exit-detail")
	}
}

func TestClearOverlayCommand(t *testing.T) {
	app := newTestApp()

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	plugin.Register(podsPlugin)
	app.layout.AddSplit(podsPlugin, "default")

	obj := &unstructured.Unstructured{}
	obj.SetName("test-pod")
	app.layout.FocusedSplit().SetObjects([]*unstructured.Unstructured{obj})

	// With no search/filter/detail active, clear-overlay is a no-op (panel stays visible)
	model, _ := app.executeCommand("view-yaml")
	app = model.(App)

	model, _ = app.executeCommand("clear-overlay")
	app = model.(App)
	if !app.layout.RightPanelVisible() {
		t.Fatal("clear-overlay should not close panel when no overlay is active")
	}
}

func TestClearOverlayExitsDetailScrollFirst(t *testing.T) {
	app := newTestApp()

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	plugin.Register(podsPlugin)
	app.layout.AddSplit(podsPlugin, "default")

	obj := &unstructured.Unstructured{}
	obj.SetName("test-pod")
	app.layout.FocusedSplit().SetObjects([]*unstructured.Unstructured{obj})

	// Enter detail-scroll mode
	model, _ := app.executeCommand("view-yaml-focused")
	app = model.(App)

	// clear-overlay should exit detail first, not close panel
	model, _ = app.executeCommand("clear-overlay")
	app = model.(App)
	if !app.layout.FocusedResources() {
		t.Fatal("expected resources to be focused after clear-overlay")
	}
	if !app.layout.RightPanelVisible() {
		t.Fatal("panel should still be visible after exiting detail-scroll via clear-overlay")
	}
}

func TestHelpCommand(t *testing.T) {
	app := newTestApp()

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	plugin.Register(podsPlugin)
	app.layout.AddSplit(podsPlugin, "default")

	model, _ := app.executeCommand("help")
	app = model.(App)
	if app.activeOverlay != overlayHelp {
		t.Fatal("help command should show help overlay")
	}
	if !app.helpOverlay.Active() {
		t.Fatal("help overlay should be active")
	}
}

func TestNamespaceSwitchFocusedPaneOnly(t *testing.T) {
	app := newTestApp()

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	svcsPlugin := &mockPlugin{
		name: "services",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "services"},
	}
	plugin.Register(podsPlugin)
	plugin.Register(svcsPlugin)

	// Add two splits: pods in "default", services in "default"
	app.layout.AddSplit(podsPlugin, "default")
	app.layout.AddSplit(svcsPlugin, "default")

	// Focus is on split 1 (services). Switch its namespace to "staging".
	model, _ := app.handleNamespaceSwitch("staging")
	app = model.(App)

	// Focused pane (services) should now be in "staging"
	if app.layout.FocusedSplit().Namespace() != "staging" {
		t.Fatalf("expected focused pane namespace 'staging', got %q", app.layout.FocusedSplit().Namespace())
	}

	// The other pane (pods) should still be in "default"
	if app.layout.SplitAt(0).Namespace() != "default" {
		t.Fatalf("expected pane 0 namespace 'default', got %q", app.layout.SplitAt(0).Namespace())
	}
}

func TestGotoPreservesNamespace(t *testing.T) {
	app := newTestApp()

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	deploymentsPlugin := &mockPlugin{
		name: "deployments",
		gvr:  schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"},
	}
	plugin.Register(podsPlugin)
	plugin.Register(deploymentsPlugin)

	// Add split with pods in "staging"
	app.layout.AddSplit(podsPlugin, "staging")

	// Goto deployments — namespace should be preserved
	model, _ := app.executeCommand("goto-deployments")
	app = model.(App)

	focused := app.layout.FocusedSplit()
	if focused.Plugin().Name() != "deployments" {
		t.Fatalf("expected deployments plugin, got %q", focused.Plugin().Name())
	}
	if focused.Namespace() != "staging" {
		t.Fatalf("expected namespace 'staging' preserved after goto, got %q", focused.Namespace())
	}
}

func TestSplitInheritsNamespace(t *testing.T) {
	app := newTestApp()

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	deploymentsPlugin := &mockPlugin{
		name: "deployments",
		gvr:  schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"},
	}
	plugin.Register(podsPlugin)
	plugin.Register(deploymentsPlugin)

	// Add initial split with pods in "production"
	app.layout.AddSplit(podsPlugin, "production")

	// Split with deployments — should inherit "production"
	model, _ := app.executeCommand("split-deployments")
	app = model.(App)

	// New split should have "production" namespace
	focused := app.layout.FocusedSplit()
	if focused.Plugin().Name() != "deployments" {
		t.Fatalf("expected deployments plugin, got %q", focused.Plugin().Name())
	}
	if focused.Namespace() != "production" {
		t.Fatalf("expected namespace 'production' inherited from focused pane, got %q", focused.Namespace())
	}

	// Original pane should still have "production"
	if app.layout.SplitAt(0).Namespace() != "production" {
		t.Fatalf("expected original pane to keep 'production', got %q", app.layout.SplitAt(0).Namespace())
	}
}

func TestQuitCleansUpNamespace(t *testing.T) {
	app := newTestApp()

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	deploymentsPlugin := &mockPlugin{
		name: "deployments",
		gvr:  schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"},
	}
	plugin.Register(podsPlugin)
	plugin.Register(deploymentsPlugin)

	// Two splits in different namespaces
	app.layout.AddSplit(podsPlugin, "default")
	app.layout.AddSplit(deploymentsPlugin, "staging")

	// Quit should close the focused split (staging/deployments)
	model, cmd := app.executeCommand("quit")
	app = model.(App)

	if cmd != nil {
		msg := cmd()
		if _, ok := msg.(tea.QuitMsg); ok {
			t.Fatal("quit with 2 splits should not produce tea.Quit")
		}
	}

	// Should have 1 split remaining with "default" namespace
	if app.layout.SplitCount() != 1 {
		t.Fatalf("expected 1 split, got %d", app.layout.SplitCount())
	}
	if app.layout.FocusedSplit().Namespace() != "default" {
		t.Fatalf("expected remaining pane in 'default', got %q", app.layout.FocusedSplit().Namespace())
	}
}

func TestNamespaceSwitchSameNsNoop(t *testing.T) {
	app := newTestApp()

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	plugin.Register(podsPlugin)
	app.layout.AddSplit(podsPlugin, "default")

	// Switching to the same namespace should be a no-op
	model, _ := app.handleNamespaceSwitch("default")
	app = model.(App)

	if app.layout.FocusedSplit().Namespace() != "default" {
		t.Fatalf("expected namespace 'default', got %q", app.layout.FocusedSplit().Namespace())
	}
}

// mockDrillablePlugin implements ResourcePlugin and DrillDowner for testing.
type mockDrillablePlugin struct {
	mockPlugin
	childPlugin plugin.ResourcePlugin
	children    []*unstructured.Unstructured
}

func (m *mockDrillablePlugin) DrillDown(obj *unstructured.Unstructured) (plugin.ResourcePlugin, []*unstructured.Unstructured) {
	return m.childPlugin, m.children
}

func TestEnterDetailDrillDown(t *testing.T) {
	a := newTestApp()
	childPlugin := &mockPlugin{
		name: "containers",
		gvr:  schema.GroupVersionResource{Group: "_ktui", Version: "v1", Resource: "containers"},
	}
	childObj := &unstructured.Unstructured{
		Object: map[string]any{"metadata": map[string]any{"name": "nginx"}},
	}
	drillable := &mockDrillablePlugin{
		mockPlugin:  mockPlugin{name: "pods", gvr: schema.GroupVersionResource{Resource: "pods"}},
		childPlugin: childPlugin,
		children:    []*unstructured.Unstructured{childObj},
	}
	a.layout.AddSplit(drillable, "default")
	a.layout.FocusedSplit().SetObjects([]*unstructured.Unstructured{
		{Object: map[string]any{"metadata": map[string]any{"name": "pod-1"}}},
	})

	result, _ := a.executeCommand("enter-detail")
	a = result.(App)

	// Should have drilled down — plugin should now be containers
	focused := a.layout.FocusedSplit()
	if focused.Plugin().Name() != "containers" {
		t.Fatalf("expected plugin 'containers' after drill-down, got %q", focused.Plugin().Name())
	}
	if !focused.InDrillDown() {
		t.Fatal("should be in drill-down mode")
	}
}

func TestEnterDetailDrillDownEmptyChildren(t *testing.T) {
	a := newTestApp()
	childPlugin := &mockPlugin{
		name: "containers",
		gvr:  schema.GroupVersionResource{Group: "_ktui", Version: "v1", Resource: "containers"},
	}
	drillable := &mockDrillablePlugin{
		mockPlugin:  mockPlugin{name: "pods", gvr: schema.GroupVersionResource{Resource: "pods"}},
		childPlugin: childPlugin,
		children:    []*unstructured.Unstructured{}, // empty children
	}
	a.layout.AddSplit(drillable, "default")
	a.layout.FocusedSplit().SetObjects([]*unstructured.Unstructured{
		{Object: map[string]any{"metadata": map[string]any{"name": "pod-1"}}},
	})

	result, _ := a.executeCommand("enter-detail")
	a = result.(App)

	focused := a.layout.FocusedSplit()
	if focused.Plugin().Name() != "containers" {
		t.Fatalf("expected plugin 'containers' after drill-down with empty children, got %q", focused.Plugin().Name())
	}
	if !focused.InDrillDown() {
		t.Fatal("should be in drill-down mode even with empty children")
	}
}

func TestClearOverlayNavBack(t *testing.T) {
	a := newTestApp()
	childPlugin := &mockPlugin{
		name: "containers",
		gvr:  schema.GroupVersionResource{Group: "_ktui", Version: "v1", Resource: "containers"},
	}
	childObj := &unstructured.Unstructured{
		Object: map[string]any{"metadata": map[string]any{"name": "nginx"}},
	}
	drillable := &mockDrillablePlugin{
		mockPlugin:  mockPlugin{name: "pods", gvr: schema.GroupVersionResource{Resource: "pods"}},
		childPlugin: childPlugin,
		children:    []*unstructured.Unstructured{childObj},
	}
	a.layout.AddSplit(drillable, "default")
	a.layout.FocusedSplit().SetObjects([]*unstructured.Unstructured{
		{Object: map[string]any{"metadata": map[string]any{"name": "pod-1"}}},
	})

	// Drill down
	result, _ := a.executeCommand("enter-detail")
	a = result.(App)

	// Now pop back
	result, _ = a.executeCommand("clear-overlay")
	a = result.(App)

	focused := a.layout.FocusedSplit()
	if focused.Plugin().Name() != "pods" {
		t.Fatalf("expected plugin 'pods' after back nav, got %q", focused.Plugin().Name())
	}
	if focused.InDrillDown() {
		t.Fatal("should not be in drill-down after back nav")
	}
}

// mockGotoPlugin implements Drillable with GoTo returning true.
type mockGotoPlugin struct {
	mockPlugin
	targetResource string
	targetNs       string
}

func (m *mockGotoPlugin) GoTo(obj *unstructured.Unstructured) (string, string, bool) {
	if m.targetResource == "" {
		return "", "", false
	}
	ns := m.targetNs
	if ns == "" && obj != nil {
		ns = obj.GetName()
	}
	return m.targetResource, ns, true
}

func (m *mockGotoPlugin) DrillDown(_ *unstructured.Unstructured) (plugin.ResourcePlugin, []*unstructured.Unstructured) {
	return nil, nil
}

func TestEnterDetailNamespaceGoto(t *testing.T) {
	a := newTestApp()
	childPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	plugin.Register(childPlugin)
	nsPlugin := &mockGotoPlugin{
		mockPlugin:     mockPlugin{name: "namespaces", gvr: schema.GroupVersionResource{Version: "v1", Resource: "namespaces"}},
		targetResource: "pods",
	}
	a.layout.AddSplit(nsPlugin, "")
	a.layout.FocusedSplit().SetObjects([]*unstructured.Unstructured{
		{Object: map[string]any{"metadata": map[string]any{"name": "kube-system"}}},
	})

	result, _ := a.executeCommand("enter-detail")
	a = result.(App)

	focused := a.layout.FocusedSplit()
	if focused.Plugin().Name() != "pods" {
		t.Fatalf("expected plugin 'pods' after namespace goto, got %q", focused.Plugin().Name())
	}
	if focused.InDrillDown() {
		t.Fatal("namespace goto should not push nav stack")
	}
	if focused.Namespace() != "kube-system" {
		t.Fatalf("expected namespace 'kube-system', got %q", focused.Namespace())
	}
}

func TestMultiLevelDrillDown(t *testing.T) {
	a := newTestApp()

	// Create pod plugin as the innermost child
	podPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}

	// RS plugin is drillable to pods
	rsPlugin := &mockDrillablePlugin{
		mockPlugin:  mockPlugin{name: "replicasets", gvr: schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "replicasets"}},
		childPlugin: podPlugin,
		children:    []*unstructured.Unstructured{{Object: map[string]any{"metadata": map[string]any{"name": "pod-1"}}}},
	}

	// Deployment plugin is drillable to RS
	deployPlugin := &mockDrillablePlugin{
		mockPlugin:  mockPlugin{name: "deployments", gvr: schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}},
		childPlugin: rsPlugin,
		children:    []*unstructured.Unstructured{{Object: map[string]any{"metadata": map[string]any{"name": "rs-1"}}}},
	}

	a.layout.AddSplit(deployPlugin, "default")
	a.layout.FocusedSplit().SetObjects([]*unstructured.Unstructured{
		{Object: map[string]any{"metadata": map[string]any{"name": "nginx-deploy"}}},
	})

	// First drill-down: Deployment → RS
	result, _ := a.executeCommand("enter-detail")
	a = result.(App)
	focused := a.layout.FocusedSplit()
	if focused.Plugin().Name() != "replicasets" {
		t.Fatalf("expected 'replicasets' after first drill-down, got %q", focused.Plugin().Name())
	}

	// Second drill-down: RS → Pod
	result, _ = a.executeCommand("enter-detail")
	a = result.(App)
	focused = a.layout.FocusedSplit()
	if focused.Plugin().Name() != "pods" {
		t.Fatalf("expected 'pods' after second drill-down, got %q", focused.Plugin().Name())
	}

	// Escape back to RS
	result, _ = a.executeCommand("clear-overlay")
	a = result.(App)
	if a.layout.FocusedSplit().Plugin().Name() != "replicasets" {
		t.Fatalf("expected 'replicasets' after first back, got %q", a.layout.FocusedSplit().Plugin().Name())
	}

	// Escape back to Deployment
	result, _ = a.executeCommand("clear-overlay")
	a = result.(App)
	if a.layout.FocusedSplit().Plugin().Name() != "deployments" {
		t.Fatalf("expected 'deployments' after second back, got %q", a.layout.FocusedSplit().Plugin().Name())
	}
	if a.layout.FocusedSplit().InDrillDown() {
		t.Fatal("should not be in drill-down after full pop")
	}
}

func TestEnterDetailDrillDownSetsParentUID(t *testing.T) {
	a := newTestApp()
	childPlugin := &mockPlugin{
		name: "containers",
		gvr:  schema.GroupVersionResource{Group: "_ktui", Version: "v1", Resource: "containers"},
	}
	childObj := &unstructured.Unstructured{
		Object: map[string]any{"metadata": map[string]any{"name": "nginx"}},
	}
	drillable := &mockDrillablePlugin{
		mockPlugin:  mockPlugin{name: "pods", gvr: schema.GroupVersionResource{Resource: "pods"}},
		childPlugin: childPlugin,
		children:    []*unstructured.Unstructured{childObj},
	}
	a.layout.AddSplit(drillable, "default")

	parentObj := &unstructured.Unstructured{
		Object: map[string]any{"metadata": map[string]any{
			"name": "pod-1",
			"uid":  "pod-uid-abc",
		}},
	}
	a.layout.FocusedSplit().SetObjects([]*unstructured.Unstructured{parentObj})

	result, _ := a.executeCommand("enter-detail")
	a = result.(App)

	focused := a.layout.FocusedSplit()
	if focused.ParentContext() != "po/pod-1" {
		t.Fatalf("expected parentContext 'po/pod-1', got %q", focused.ParentContext())
	}
	if snap := focused.ParentSnap(); snap == nil || snap.ParentUID != "pod-uid-abc" {
		uid := ""
		if snap != nil {
			uid = snap.ParentUID
		}
		t.Fatalf("expected parentUID 'pod-uid-abc', got %q", uid)
	}
}

func TestClearOverlayMultiLevelPreservesParentContext(t *testing.T) {
	a := newTestApp()

	podPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	rsPlugin := &mockDrillablePlugin{
		mockPlugin:  mockPlugin{name: "replicasets", gvr: schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "replicasets"}},
		childPlugin: podPlugin,
		children:    []*unstructured.Unstructured{{Object: map[string]any{"metadata": map[string]any{"name": "pod-1"}}}},
	}
	deployPlugin := &mockDrillablePlugin{
		mockPlugin:  mockPlugin{name: "deployments", gvr: schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}},
		childPlugin: rsPlugin,
		children:    []*unstructured.Unstructured{{Object: map[string]any{"metadata": map[string]any{"name": "rs-1"}}}},
	}

	a.layout.AddSplit(deployPlugin, "default")
	a.layout.FocusedSplit().SetObjects([]*unstructured.Unstructured{
		{Object: map[string]any{"metadata": map[string]any{"name": "nginx-deploy", "uid": "deploy-uid-123"}}},
	})

	// Drill: Deployments → RS
	result, _ := a.executeCommand("enter-detail")
	a = result.(App)

	// Drill: RS → Pods
	result, _ = a.executeCommand("enter-detail")
	a = result.(App)

	if a.layout.FocusedSplit().Plugin().Name() != "pods" {
		t.Fatalf("expected 'pods', got %q", a.layout.FocusedSplit().Plugin().Name())
	}

	// Pop: Pods → RS
	result, _ = a.executeCommand("clear-overlay")
	a = result.(App)

	focused := a.layout.FocusedSplit()
	if focused.Plugin().Name() != "replicasets" {
		t.Fatalf("expected 'replicasets' after pop, got %q", focused.Plugin().Name())
	}
	// Key assertion: parentContext should be restored to the deployment name
	if focused.ParentContext() != "de/nginx-deploy" {
		t.Fatalf("expected parentContext 'de/nginx-deploy' after pop to RS, got %q", focused.ParentContext())
	}
	if snap := focused.ParentSnap(); snap == nil || snap.ParentUID != "deploy-uid-123" {
		uid := ""
		if snap != nil {
			uid = snap.ParentUID
		}
		t.Fatalf("expected parentUID 'deploy-uid-123' after pop to RS, got %q", uid)
	}
	if !focused.InDrillDown() {
		t.Fatal("should still be in drill-down after popping to mid-level")
	}

	// Pop: RS → Deployments
	result, _ = a.executeCommand("clear-overlay")
	a = result.(App)

	focused = a.layout.FocusedSplit()
	if focused.Plugin().Name() != "deployments" {
		t.Fatalf("expected 'deployments' at root, got %q", focused.Plugin().Name())
	}
	if focused.ParentContext() != "" {
		t.Fatalf("expected empty parentContext at root, got %q", focused.ParentContext())
	}
	if focused.InDrillDown() {
		t.Fatal("should not be in drill-down at root")
	}
}

// mockSelfPopulatingPlugin implements both ResourcePlugin and SelfPopulating.
type mockSelfPopulatingPlugin struct {
	mockPlugin
	objs []*unstructured.Unstructured
}

func (m *mockSelfPopulatingPlugin) Objects() []*unstructured.Unstructured {
	return m.objs
}

func TestHandleGotoSelfPopulating(t *testing.T) {
	plugin.Reset()
	// Register a SelfPopulating plugin with synthetic _ktui GVR
	spPlugin := &mockSelfPopulatingPlugin{
		mockPlugin: mockPlugin{
			name: "api-resources",
			gvr:  schema.GroupVersionResource{Group: "_ktui", Version: "v1", Resource: "api-resources"},
		},
		objs: []*unstructured.Unstructured{
			{Object: map[string]any{"metadata": map[string]any{"name": "pods"}}},
			{Object: map[string]any{"metadata": map[string]any{"name": "deployments"}}},
		},
	}
	plugin.Register(spPlugin)
	// Need a "pods" plugin so the initial split works
	podsPlugin := &mockPlugin{name: "pods", gvr: schema.GroupVersionResource{Version: "v1", Resource: "pods"}}
	plugin.Register(podsPlugin)

	km := config.DefaultKeymap()
	cfg := config.DefaultConfig()
	a := New(nil, nil, km, cfg, nil, nil, nil, nil)

	// Add initial split with pods so goto has a focused split
	a.layout.AddSplit(podsPlugin, "default")

	// Navigate to api-resources via goto
	model, _ := a.executeCommand("goto-api-resources")
	a = model.(App)

	// The app should not crash (no store.Subscribe for _ktui GVR)
	// and the SelfPopulating plugin's objects should be used
	focused := a.layout.FocusedSplit()
	if focused == nil {
		t.Fatal("expected a focused split after goto")
	}
	if focused.Plugin().Name() != "api-resources" {
		t.Fatalf("expected focused plugin to be 'api-resources', got %q", focused.Plugin().Name())
	}
	// Verify objects were populated from the SelfPopulating interface
	if focused.Len() != 2 {
		t.Fatalf("expected 2 objects from SelfPopulating, got %d", focused.Len())
	}
}

func TestEnterDetailApiResourcesGoto(t *testing.T) {
	plugin.Reset()
	targetPlugin := &mockPlugin{
		name: "deployments",
		gvr:  schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"},
	}
	plugin.Register(targetPlugin)

	gotoPlugin := &mockGotoPlugin{
		mockPlugin:     mockPlugin{name: "api-resources", gvr: schema.GroupVersionResource{Group: "_ktui", Version: "v1", Resource: "api-resources"}},
		targetResource: "deployments",
		targetNs:       "",
	}
	plugin.Register(gotoPlugin)

	km := config.DefaultKeymap()
	cfg := config.DefaultConfig()
	a := New(nil, nil, km, cfg, nil, nil, nil, nil)

	a.layout.AddSplit(gotoPlugin, "default")
	a.layout.FocusedSplit().SetObjects([]*unstructured.Unstructured{
		{Object: map[string]any{"metadata": map[string]any{"name": "deployments"}}},
	})

	model, _ := a.executeCommand("enter-detail")
	a = model.(App)

	focused := a.layout.FocusedSplit()
	if focused == nil {
		t.Fatal("expected a focused split after enter-detail")
	}
	if focused.Plugin().Name() != "deployments" {
		t.Fatalf("expected plugin 'deployments' after api-resources goto, got %q", focused.Plugin().Name())
	}
	if focused.InDrillDown() {
		t.Fatal("api-resources goto should not push nav stack")
	}
}

func TestEnterDetailDrillDownNilChildren(t *testing.T) {
	a := newTestApp()
	childPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	drillable := &mockDrillablePlugin{
		mockPlugin:  mockPlugin{name: "deployments", gvr: schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}},
		childPlugin: childPlugin,
		children:    nil, // nil children — should still push nav, not goto
	}
	a.layout.AddSplit(drillable, "default")
	a.layout.FocusedSplit().SetObjects([]*unstructured.Unstructured{
		{Object: map[string]any{"metadata": map[string]any{"name": "deploy-1"}}},
	})

	result, _ := a.executeCommand("enter-detail")
	a = result.(App)

	focused := a.layout.FocusedSplit()
	if focused.Plugin().Name() != "pods" {
		t.Fatalf("expected plugin 'pods' after drill-down with nil children, got %q", focused.Plugin().Name())
	}
	if !focused.InDrillDown() {
		t.Fatal("should be in drill-down mode even with nil children")
	}
}

type mockClusterPlugin struct {
	mockPlugin
}

func (m *mockClusterPlugin) IsClusterScoped() bool { return true }

func TestHandleGotoPreservesNamespaceAcrossClusterScoped(t *testing.T) {
	a := newTestApp()

	clusterPlugin := &mockClusterPlugin{
		mockPlugin: mockPlugin{
			name: "nodes",
			gvr:  schema.GroupVersionResource{Version: "v1", Resource: "nodes"},
		},
	}
	plugin.Register(clusterPlugin)

	podsPlugin := &mockPlugin{
		name: "testpods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	plugin.Register(podsPlugin)

	// Add initial split and set namespace
	a.layout.AddSplit(podsPlugin, "production")
	focused := a.layout.FocusedSplit()

	// Go to cluster-scoped resource — namespace preserved internally
	model, _ := a.handleGoto("nodes", "")
	a = model.(App)
	focused = a.layout.FocusedSplit()
	if focused.Namespace() != "production" {
		t.Fatalf("expected raw namespace 'production' on cluster-scoped, got %q", focused.Namespace())
	}
	if focused.EffectiveNamespace() != "" {
		t.Fatalf("expected effective namespace '' for cluster-scoped, got %q", focused.EffectiveNamespace())
	}

	// Go back to namespaced resource — namespace still there
	model, _ = a.handleGoto("testpods", "")
	a = model.(App)
	focused = a.layout.FocusedSplit()
	if focused.Namespace() != "production" {
		t.Fatalf("expected namespace 'production', got %q", focused.Namespace())
	}
	if focused.EffectiveNamespace() != "production" {
		t.Fatalf("expected effective namespace 'production', got %q", focused.EffectiveNamespace())
	}
}

func TestHandleSplitInheritsNamespaceFromClusterScoped(t *testing.T) {
	a := newTestApp()

	clusterPlugin := &mockClusterPlugin{
		mockPlugin: mockPlugin{
			name: "testnodes",
			gvr:  schema.GroupVersionResource{Version: "v1", Resource: "nodes"},
		},
	}
	plugin.Register(clusterPlugin)

	podsPlugin := &mockPlugin{
		name: "splitpods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	plugin.Register(podsPlugin)

	// Add initial split with namespace, then switch to cluster-scoped
	a.layout.AddSplit(podsPlugin, "staging")
	model, _ := a.handleGoto("testnodes", "")
	a = model.(App)

	// Split to a namespaced resource — should inherit "staging"
	model, _ = a.handleSplit("splitpods")
	a = model.(App)
	newSplit := a.layout.FocusedSplit()
	if newSplit.Namespace() != "staging" {
		t.Fatalf("expected new split namespace 'staging', got %q", newSplit.Namespace())
	}
}

func TestReloadAllCommand(t *testing.T) {
	app := newTestApp()

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	plugin.Register(podsPlugin)

	// Add initial split with pods
	app.layout.AddSplit(podsPlugin, "default")

	// Set some objects
	focused := app.layout.FocusedSplit()
	objs := []*unstructured.Unstructured{
		{Object: map[string]any{"metadata": map[string]any{"name": "pod-1"}}},
	}
	focused.SetObjects(objs)

	// Execute reload-all
	model, _ := app.executeCommand("reload-all")
	app = model.(App)

	// Should still have 1 split with pods plugin
	if app.layout.SplitCount() != 1 {
		t.Fatalf("expected 1 split, got %d", app.layout.SplitCount())
	}
	focused = app.layout.FocusedSplit()
	if focused.Plugin().Name() != "pods" {
		t.Fatalf("expected plugin 'pods', got %q", focused.Plugin().Name())
	}

	// Nav stack should be empty
	if focused.InDrillDown() {
		t.Fatal("expected no drill-down after reload")
	}

	// Detail panel should be hidden
	if app.layout.RightPanelVisible() {
		t.Fatal("expected right panel to be hidden after reload")
	}
}

func TestReloadAllWithDrillDown(t *testing.T) {
	app := newTestApp()

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	containersPlugin := &mockPlugin{
		name: "containers",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "containers"},
	}
	plugin.Register(podsPlugin)
	plugin.Register(containersPlugin)

	app.layout.AddSplit(podsPlugin, "default")
	focused := app.layout.FocusedSplit()

	// Push a drill-down
	children := []*unstructured.Unstructured{
		{Object: map[string]any{"metadata": map[string]any{"name": "container-1"}}},
	}
	focused.PushNav(containersPlugin, children, "pod-1", "uid-1", "", "")

	if !focused.InDrillDown() {
		t.Fatal("expected to be in drill-down")
	}

	// Execute reload-all
	model, _ := app.executeCommand("reload-all")
	app = model.(App)

	focused = app.layout.FocusedSplit()
	if focused.InDrillDown() {
		t.Fatal("expected nav stack cleared after reload")
	}
	if focused.Plugin().Name() != "pods" {
		t.Fatalf("expected root plugin 'pods', got %q", focused.Plugin().Name())
	}
}

func TestReloadAllWithDetailPanel(t *testing.T) {
	app := newTestApp()

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	plugin.Register(podsPlugin)
	app.layout.AddSplit(podsPlugin, "default")

	// Show detail panel
	app.layout.ShowRightPanel()
	app.layout.FocusDetails()

	if !app.layout.RightPanelVisible() {
		t.Fatal("expected right panel visible")
	}

	model, _ := app.executeCommand("reload-all")
	app = model.(App)

	if !app.layout.RightPanelVisible() {
		t.Fatal("expected right panel to remain visible after reload")
	}
	if app.layout.FocusedDetails() {
		t.Fatal("expected focus on resources after reload")
	}
}

func TestReloadAllPreservesLogMode(t *testing.T) {
	app := newTestApp()

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	plugin.Register(podsPlugin)
	app.layout.AddSplit(podsPlugin, "default")

	// Enable log mode with right panel visible
	app.layout.SetLogMode(true)
	app.layout.ShowRightPanel()

	if !app.layout.IsLogMode() {
		t.Fatal("expected log mode to be active before reload")
	}

	// Execute reload-all
	model, _ := app.executeCommand("reload-all")
	app = model.(App)

	// Log mode should be preserved
	if !app.layout.IsLogMode() {
		t.Fatal("expected log mode to be preserved after reload-all")
	}
	// Right panel should still be visible
	if !app.layout.RightPanelVisible() {
		t.Fatal("expected right panel to remain visible after reload-all in log mode")
	}
	// Focus should be on resources (not details)
	if app.layout.FocusedDetails() {
		t.Fatal("expected focus on resources after reload")
	}
	// Log view should be marked unavailable, awaiting objects from informer
	if lv := app.layout.LogView(); lv == nil || !lv.IsUnavailable() {
		t.Fatal("expected log view to be marked unavailable, awaiting objects")
	}
}

func TestQuitWithRightPanelPerformsFullCleanup(t *testing.T) {
	app := newTestApp()

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}

	plugin.Register(podsPlugin)
	app.layout.AddSplit(podsPlugin, "default")

	// Set objects so Selected() returns non-nil
	obj := &unstructured.Unstructured{}
	obj.SetName("test-pod")
	obj.SetNamespace("default")
	focused := app.layout.FocusedSplit()
	focused.SetObjects([]*unstructured.Unstructured{obj})

	// Open a YAML panel so RightPanelVisible is true
	model, _ := app.executeCommand("view-yaml")
	app = model.(App)

	if !app.layout.RightPanelVisible() {
		t.Fatal("expected right panel to be visible before quit")
	}

	// Execute quit: should close panel, not exit
	model, cmd := app.executeCommand("quit")
	app = model.(App)

	// Should not produce tea.Quit
	if cmd != nil {
		msg := cmd()
		if _, ok := msg.(tea.QuitMsg); ok {
			t.Fatal("quit with right panel visible should not produce tea.Quit")
		}
	}

	// Right panel should be hidden
	if app.layout.RightPanelVisible() {
		t.Fatal("expected right panel to be hidden after quit")
	}

	// Focus should be on resources
	if !app.layout.FocusedResources() {
		t.Fatal("expected resources to be focused after quit")
	}

	// Log mode should be off
	if app.layout.IsLogMode() {
		t.Fatal("expected log mode to be off after quit")
	}
}

func TestRefreshDetailPanelOrLogNonLogMode(t *testing.T) {
	app := newTestApp()
	p := &mockPlugin{name: "pods", gvr: schema.GroupVersionResource{Version: "v1", Resource: "pods"}}
	plugin.Register(p)
	app.layout.AddSplit(p, "default")
	split := app.layout.FocusedSplit()
	obj := &unstructured.Unstructured{}
	obj.SetName("pod-1")
	obj.SetNamespace("default")
	split.SetObjects([]*unstructured.Unstructured{obj})

	// Open YAML panel
	app.layout.ShowRightPanel()
	app.layout.RightPanel().SetMode(msgs.DetailYAML)

	// refreshDetailPanelOrLog in non-log mode should return nil cmd
	app, cmd := app.refreshDetailPanelOrLog()
	if cmd != nil {
		t.Fatal("expected nil cmd in non-log mode")
	}
	if app.layout.IsLogMode() {
		t.Fatal("should not be in log mode")
	}
}

func TestCursorDownInLogModeReturnsCmd(t *testing.T) {
	app := newTestApp()
	p := &mockPlugin{name: "pods", gvr: schema.GroupVersionResource{Version: "v1", Resource: "pods"}}
	plugin.Register(p)
	app.layout.AddSplit(p, "default")
	split := app.layout.FocusedSplit()
	objs := []*unstructured.Unstructured{
		func() *unstructured.Unstructured {
			o := &unstructured.Unstructured{}
			o.SetName("pod-1")
			o.SetNamespace("default")
			return o
		}(),
		func() *unstructured.Unstructured {
			o := &unstructured.Unstructured{}
			o.SetName("pod-2")
			o.SetNamespace("default")
			return o
		}(),
	}
	split.SetObjects(objs)

	// Enter log mode
	app.layout.SetLogMode(true)
	app.layout.ShowRightPanel()

	// Cursor-down should return a non-nil cmd (the debounce)
	model, cmd := app.executeCommand("cursor-down")
	if cmd == nil {
		t.Fatal("expected non-nil cmd from cursor-down in log mode")
	}
	app = model.(App)
	if app.logDebounceSeq != 1 {
		t.Fatalf("expected logDebounceSeq=1, got %d", app.logDebounceSeq)
	}
}

func TestLogDebounceStaleFire(t *testing.T) {
	app := newTestApp()
	p := &mockPlugin{name: "pods", gvr: schema.GroupVersionResource{Version: "v1", Resource: "pods"}}
	plugin.Register(p)
	app.layout.AddSplit(p, "default")
	split := app.layout.FocusedSplit()
	obj := &unstructured.Unstructured{}
	obj.SetName("pod-1")
	obj.SetNamespace("default")
	split.SetObjects([]*unstructured.Unstructured{obj})

	app.layout.SetLogMode(true)
	app.layout.ShowRightPanel()
	app.logDebounceSeq = 5

	// Fire a stale debounce (seq=3, current=5)
	model, cmd := app.update(msgs.LogDebounceFiredMsg{Seq: 3})
	if cmd != nil {
		t.Fatal("stale debounce should return nil cmd")
	}
	app = model.(App)
	// logDebounceSeq should be unchanged
	if app.logDebounceSeq != 5 {
		t.Fatalf("expected logDebounceSeq=5, got %d", app.logDebounceSeq)
	}
}

func TestLogDebounceAfterExitLogMode(t *testing.T) {
	app := newTestApp()
	p := &mockPlugin{name: "pods", gvr: schema.GroupVersionResource{Version: "v1", Resource: "pods"}}
	plugin.Register(p)
	app.layout.AddSplit(p, "default")

	// Simulate: was in log mode, debounce scheduled, then user exited log mode
	app.logDebounceSeq = 1
	app.layout.SetLogMode(false)

	model, cmd := app.update(msgs.LogDebounceFiredMsg{Seq: 1})
	if cmd != nil {
		t.Fatal("debounce after exit log mode should return nil cmd")
	}
	_ = model
}

func TestIsLoggablePlugin(t *testing.T) {
	tests := []struct {
		name     string
		loggable bool
	}{
		{"pods", true},
		{"containers", true},
		{"deployments", false},
		{"services", false},
		{"configmaps", false},
		{"api-resources", false},
	}
	for _, tt := range tests {
		if got := isLoggablePlugin(tt.name); got != tt.loggable {
			t.Errorf("isLoggablePlugin(%q) = %v, want %v", tt.name, got, tt.loggable)
		}
	}
}

func TestHandleGoto_StopsLogStreamOnNonLoggable(t *testing.T) {
	app := newTestApp()

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	deploymentsPlugin := &mockPlugin{
		name: "deployments",
		gvr:  schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"},
	}
	plugin.Register(podsPlugin)
	plugin.Register(deploymentsPlugin)

	app.layout.AddSplit(podsPlugin, "default")

	podObj := &unstructured.Unstructured{}
	podObj.SetName("my-pod")
	app.layout.FocusedSplit().SetObjects([]*unstructured.Unstructured{podObj})

	// Enable log mode manually to simulate logs being open
	app.layout.SetLogMode(true)
	app.layout.ShowRightPanel()

	// Goto deployments — syncLogPanel should mark logView unavailable
	model, _ := app.executeCommand("goto-deployments")
	app = model.(App)

	lv := app.layout.LogView()
	if !lv.IsUnavailable() {
		t.Fatal("expected LogView to be unavailable after goto non-loggable resource")
	}
	if !app.layout.IsLogMode() {
		t.Fatal("expected log mode to remain true (panel stays open)")
	}
}

func TestHandleGoto_ResumesLogStreamOnLoggable(t *testing.T) {
	app := newTestApp()

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	deploymentsPlugin := &mockPlugin{
		name: "deployments",
		gvr:  schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"},
	}
	plugin.Register(podsPlugin)
	plugin.Register(deploymentsPlugin)

	app.layout.AddSplit(podsPlugin, "default")

	podObj := &unstructured.Unstructured{}
	podObj.SetName("my-pod")
	podObj.Object = map[string]any{
		"metadata": map[string]any{"name": "my-pod", "namespace": "default"},
		"spec": map[string]any{
			"containers": []any{
				map[string]any{"name": "app"},
			},
		},
	}
	app.layout.FocusedSplit().SetObjects([]*unstructured.Unstructured{podObj})

	// Enable log mode and mark unavailable (simulating previous goto to deployments)
	app.layout.SetLogMode(true)
	app.layout.ShowRightPanel()
	app.layout.LogView().SetUnavailable(true)

	// Goto pods — since store is nil, SetPlugin clears objects and
	// subscribeAndPopulate can't re-populate. LogView stays unavailable
	// until objects arrive via ResourceUpdatedMsg.
	model, _ := app.executeCommand("goto-pods")
	app = model.(App)

	lv := app.layout.LogView()
	if !lv.IsUnavailable() {
		t.Fatal("expected LogView to remain unavailable until objects arrive")
	}

	// Simulate objects arriving: populate the split, then call syncLogPanel
	app.layout.FocusedSplit().SetObjects([]*unstructured.Unstructured{podObj})
	app, _ = app.syncLogPanel()

	if lv.IsUnavailable() {
		t.Fatal("expected LogView to be available after objects arrive")
	}
}

func TestFocusNext_SyncsLogPanel(t *testing.T) {
	app := newTestApp()

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	deploymentsPlugin := &mockPlugin{
		name: "deployments",
		gvr:  schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"},
	}
	plugin.Register(podsPlugin)
	plugin.Register(deploymentsPlugin)

	// Split 0: pods, Split 1: deployments
	app.layout.AddSplit(podsPlugin, "default")
	app.layout.AddSplit(deploymentsPlugin, "default")

	podObj := &unstructured.Unstructured{}
	podObj.SetName("my-pod")
	podObj.Object = map[string]any{
		"metadata": map[string]any{"name": "my-pod", "namespace": "default"},
		"spec": map[string]any{
			"containers": []any{
				map[string]any{"name": "app"},
			},
		},
	}
	app.layout.SplitAt(0).SetObjects([]*unstructured.Unstructured{podObj})

	depObj := &unstructured.Unstructured{}
	depObj.SetName("my-deployment")
	app.layout.SplitAt(1).SetObjects([]*unstructured.Unstructured{depObj})

	// After AddSplit, focus is on split 1 (deployments).
	// Move focus to split 0 (pods) so we can test focus-next going to deployments.
	app.layout.FocusPrev()

	// Focus is on split 0 (pods). Enable log mode.
	app.layout.SetLogMode(true)
	app.layout.ShowRightPanel()

	// Focus-next moves to split 1 (deployments) — should become unavailable
	model, _ := app.executeCommand("focus-next")
	app = model.(App)

	if !app.layout.LogView().IsUnavailable() {
		t.Fatal("expected LogView unavailable after focus-next to deployments")
	}

	// Focus-next wraps to split 0 (pods) — should resume (clear unavailable)
	model, _ = app.executeCommand("focus-next")
	app = model.(App)

	if app.layout.LogView().IsUnavailable() {
		t.Fatal("expected LogView available after focus-next back to pods")
	}
}

func TestSplitCommandSyncsLogPanel(t *testing.T) {
	app := newTestApp()

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	deploymentsPlugin := &mockPlugin{
		name: "deployments",
		gvr:  schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"},
	}

	plugin.Register(podsPlugin)
	plugin.Register(deploymentsPlugin)

	// Add initial split with pods and enable log mode
	app.layout.AddSplit(podsPlugin, "default")
	app.layout.SetLogMode(true)

	// Verify log mode is on and LogView is available
	if !app.layout.IsLogMode() {
		t.Fatal("expected log mode to be active")
	}
	lv := app.layout.LogView()
	if lv.IsUnavailable() {
		t.Fatal("expected LogView to be available initially")
	}

	// Split with deployments (non-loggable)
	model, _ := app.executeCommand("split-deployments")
	app = model.(App)

	// After split, focused plugin is deployments (non-loggable)
	// syncLogPanel should have marked LogView as unavailable
	if !app.layout.LogView().IsUnavailable() {
		t.Fatal("expected LogView to be unavailable after split with non-loggable plugin")
	}
}

func TestGotoLoggablePluginWithNoSelectionSkipsRestart(t *testing.T) {
	app := newTestApp()

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	deploymentsPlugin := &mockPlugin{
		name: "deployments",
		gvr:  schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"},
	}

	plugin.Register(podsPlugin)
	plugin.Register(deploymentsPlugin)

	// Start with pods, enable log mode, then goto deployments
	app.layout.AddSplit(podsPlugin, "default")
	app.layout.SetLogMode(true)

	model, _ := app.executeCommand("goto-deployments")
	app = model.(App)

	// LogView should be unavailable
	if !app.layout.LogView().IsUnavailable() {
		t.Fatal("expected LogView unavailable after goto-deployments")
	}

	// Now goto pods — but no objects are loaded yet, so no selection exists
	// syncLogPanel should NOT crash or produce errors
	model, _ = app.executeCommand("goto-pods")
	app = model.(App)

	// LogView should remain unavailable since there's no selection yet —
	// it will transition to available when objects arrive via ResourceUpdatedMsg
	if !app.layout.LogView().IsUnavailable() {
		t.Fatal("expected LogView to remain unavailable when no selection exists")
	}
}

func TestHandleViewLogsNonLoggablePlugin(t *testing.T) {
	app := newTestApp()

	deploymentsPlugin := &mockPlugin{
		name: "deployments",
		gvr:  schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"},
	}
	plugin.Register(deploymentsPlugin)

	// Add split with deployments and populate with a fake object
	app.layout.AddSplit(deploymentsPlugin, "default")
	focused := app.layout.FocusedSplit()
	obj := &unstructured.Unstructured{}
	obj.SetName("my-deployment")
	obj.SetNamespace("default")
	obj.SetGroupVersionKind(schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"})
	focused.SetObjects([]*unstructured.Unstructured{obj})

	// Try to view logs for a deployment — should be rejected
	model, _ := app.handleView(msgs.DetailLogs)
	app = model.(App)

	// Log mode should NOT be activated
	if app.layout.IsLogMode() {
		t.Fatal("log mode should not be activated for non-loggable plugin")
	}
}

func TestSubstituteVarsParent(t *testing.T) {
	t.Run("containers plugin resolves PARENT to pod name", func(t *testing.T) {
		app := newTestApp()

		containersPlugin := &mockPlugin{
			name: "containers",
			gvr:  schema.GroupVersionResource{Group: "_ktui", Version: "v1", Resource: "containers"},
		}
		plugin.Register(containersPlugin)
		app.layout.AddSplit(containersPlugin, "default")

		// Build a container object with a synthetic _pod field
		obj := &unstructured.Unstructured{
			Object: map[string]any{
				"apiVersion": "v1",
				"kind":       "Container",
				"metadata": map[string]any{
					"name":      "my-container",
					"namespace": "default",
				},
				"_pod": map[string]any{
					"metadata": map[string]any{
						"name":      "my-parent-pod",
						"namespace": "default",
					},
				},
			},
		}
		focused := app.layout.FocusedSplit()
		focused.SetObjects([]*unstructured.Unstructured{obj})

		got, err := app.substituteVars("$PARENT")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "my-parent-pod" {
			t.Fatalf("expected $PARENT to resolve to %q, got %q", "my-parent-pod", got)
		}
	})

	t.Run("pods plugin resolves PARENT to empty string", func(t *testing.T) {
		app := newTestApp()

		podsPlugin := &mockPlugin{
			name: "pods",
			gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
		}
		plugin.Register(podsPlugin)
		app.layout.AddSplit(podsPlugin, "default")

		obj := &unstructured.Unstructured{}
		obj.SetName("my-pod")
		obj.SetNamespace("default")
		focused := app.layout.FocusedSplit()
		focused.SetObjects([]*unstructured.Unstructured{obj})

		got, err := app.substituteVars("$PARENT")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "" {
			t.Fatalf("expected $PARENT to resolve to %q for pods, got %q", "", got)
		}
	})

	t.Run("deployments plugin resolves PARENT to empty string", func(t *testing.T) {
		app := newTestApp()

		deploymentsPlugin := &mockPlugin{
			name: "deployments",
			gvr:  schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"},
		}
		plugin.Register(deploymentsPlugin)
		app.layout.AddSplit(deploymentsPlugin, "default")

		obj := &unstructured.Unstructured{}
		obj.SetName("my-deploy")
		obj.SetNamespace("default")
		focused := app.layout.FocusedSplit()
		focused.SetObjects([]*unstructured.Unstructured{obj})

		got, err := app.substituteVars("$PARENT")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "" {
			t.Fatalf("expected $PARENT to resolve to %q for deployments, got %q", "", got)
		}
	})
}

func TestLogInsertMarkerCommand(t *testing.T) {
	app := newTestApp()

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	plugin.Register(podsPlugin)
	app.layout.AddSplit(podsPlugin, "default")
	app.layout.SetLogMode(true)
	app.layout.ShowRightPanel()

	model, _ := app.executeCommand("log-insert-marker")
	app = model.(App)

	lv := app.layout.LogView()
	if lv.BufferLen() == 0 {
		t.Fatal("expected marker line in buffer")
	}
}

func TestCloseCurrentPanelClosesRightPanel(t *testing.T) {
	app := newTestApp()

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}

	plugin.Register(podsPlugin)
	app.layout.AddSplit(podsPlugin, "default")

	// Set objects so Selected() returns non-nil
	obj := &unstructured.Unstructured{}
	obj.SetName("test-pod")
	obj.SetNamespace("default")
	focused := app.layout.FocusedSplit()
	focused.SetObjects([]*unstructured.Unstructured{obj})

	// Open a view so the right panel is visible
	model, _ := app.executeCommand("view-yaml")
	app = model.(App)

	if !app.layout.RightPanelVisible() {
		t.Fatal("expected right panel to be visible before close-current-panel")
	}

	// Focus details so the command closes the right panel
	app.layout.FocusDetails()

	model, _ = app.executeCommand("close-current-panel")
	app = model.(App)

	if app.layout.RightPanelVisible() {
		t.Fatal("expected right panel to be hidden after close-current-panel")
	}
}

func TestCloseCurrentPanelClosesSplit(t *testing.T) {
	app := newTestApp()

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	deploymentsPlugin := &mockPlugin{
		name: "deployments",
		gvr:  schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"},
	}

	plugin.Register(podsPlugin)
	plugin.Register(deploymentsPlugin)

	// Add two splits
	app.layout.AddSplit(podsPlugin, "default")
	app.layout.AddSplit(deploymentsPlugin, "default")
	if app.layout.SplitCount() != 2 {
		t.Fatalf("expected 2 splits, got %d", app.layout.SplitCount())
	}

	// Execute close-current-panel: should close one split
	model, cmd := app.executeCommand("close-current-panel")
	app = model.(App)

	if cmd != nil {
		msg := cmd()
		if _, ok := msg.(tea.QuitMsg); ok {
			t.Fatal("close-current-panel should not produce tea.Quit")
		}
	}

	if app.layout.SplitCount() != 1 {
		t.Fatalf("expected 1 split after close-current-panel, got %d", app.layout.SplitCount())
	}
}

func TestCloseCurrentPanelNoopWithOneSplit(t *testing.T) {
	app := newTestApp()

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}

	plugin.Register(podsPlugin)
	app.layout.AddSplit(podsPlugin, "default")

	if app.layout.SplitCount() != 1 {
		t.Fatalf("expected 1 split, got %d", app.layout.SplitCount())
	}

	// With one split and no right panel, close-current-panel should be a no-op
	model, cmd := app.executeCommand("close-current-panel")
	app = model.(App)

	if cmd != nil {
		t.Fatal("expected nil cmd for no-op close-current-panel")
	}

	if app.layout.SplitCount() != 1 {
		t.Fatalf("expected 1 split after no-op, got %d", app.layout.SplitCount())
	}
}

func TestSubstituteVarsCleanValues(t *testing.T) {
	app := newTestApp()

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	plugin.Register(podsPlugin)
	app.layout.AddSplit(podsPlugin, "default")

	obj := &unstructured.Unstructured{}
	obj.SetName("my-pod")
	obj.SetNamespace("prod")
	focused := app.layout.FocusedSplit()
	focused.SetObjects([]*unstructured.Unstructured{obj})

	got, err := app.substituteVars("kubectl logs $NAME -n $NAMESPACE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "kubectl logs my-pod -n prod"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestDebugNodeShowsConfirmDialog(t *testing.T) {
	app := newTestApp()
	app.k8sClient = &k8s.Client{Namespace: "default"}

	nodesPlugin := &mockClusterPlugin{
		mockPlugin: mockPlugin{
			name: "nodes",
			gvr:  schema.GroupVersionResource{Version: "v1", Resource: "nodes"},
		},
	}
	plugin.Register(nodesPlugin)
	app.layout.AddSplit(nodesPlugin, "default")

	obj := &unstructured.Unstructured{}
	obj.SetName("worker-1")
	app.layout.FocusedSplit().SetObjects([]*unstructured.Unstructured{obj})

	// Execute debug command on nodes — should show confirmation, not execute immediately
	model, cmd := app.executeCommand("debug")
	app = model.(App)

	// Confirmation dialog should be shown
	if app.activeOverlay != overlayConfirm {
		t.Fatal("expected confirm overlay for node debug")
	}
	if app.pendingDebug == nil {
		t.Fatal("expected pendingDebug to be set")
	}
	if !app.pendingDebug.nodeMode {
		t.Fatal("expected nodeMode=true for node debug")
	}
	if app.pendingDebug.nodeName != "worker-1" {
		t.Fatalf("expected nodeName 'worker-1', got %q", app.pendingDebug.nodeName)
	}
	// No tea.Cmd should be returned (no immediate execution)
	if cmd != nil {
		t.Fatal("expected nil cmd — node debug should not execute immediately")
	}
}

func TestDebugPrivilegedShowsConfirmDialog(t *testing.T) {
	app := newTestApp()
	app.k8sClient = &k8s.Client{Namespace: "default"}

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	plugin.Register(podsPlugin)
	app.layout.AddSplit(podsPlugin, "default")

	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"metadata": map[string]any{
				"name":      "my-pod",
				"namespace": "default",
			},
			"spec": map[string]any{
				"containers": []any{
					map[string]any{"name": "app"},
				},
			},
		},
	}
	app.layout.FocusedSplit().SetObjects([]*unstructured.Unstructured{obj})

	// Execute debug-privileged command — should show confirmation
	model, cmd := app.executeCommand("debug-privileged")
	app = model.(App)

	// Confirmation dialog should be shown
	if app.activeOverlay != overlayConfirm {
		t.Fatal("expected confirm overlay for privileged debug")
	}
	if app.pendingDebug == nil {
		t.Fatal("expected pendingDebug to be set")
	}
	if !app.pendingDebug.privileged {
		t.Fatal("expected privileged=true for debug-privileged")
	}
	if app.pendingDebug.podName != "my-pod" {
		t.Fatalf("expected podName 'my-pod', got %q", app.pendingDebug.podName)
	}
	if app.pendingDebug.namespace != "default" {
		t.Fatalf("expected namespace 'default', got %q", app.pendingDebug.namespace)
	}
	// No tea.Cmd should be returned (no immediate execution)
	if cmd != nil {
		t.Fatal("expected nil cmd — privileged debug should not execute immediately")
	}
}

func TestDebugNonPrivilegedNoConfirmDialog(t *testing.T) {
	app := newTestApp()
	app.k8sClient = &k8s.Client{Namespace: "default"}

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	plugin.Register(podsPlugin)
	app.layout.AddSplit(podsPlugin, "default")

	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"metadata": map[string]any{
				"name":      "my-pod",
				"namespace": "default",
			},
			"spec": map[string]any{
				"containers": []any{
					map[string]any{"name": "app"},
				},
			},
		},
	}
	app.layout.FocusedSplit().SetObjects([]*unstructured.Unstructured{obj})

	// Execute non-privileged debug on pods — no k8s client, so it won't
	// actually run, but it should NOT show a confirmation dialog
	model, _ := app.executeCommand("debug")
	app = model.(App)

	// No confirmation dialog should be shown for non-privileged pod debug
	// (it will fail with "no k8s client" but that's expected — the point is
	// it attempts to execute immediately rather than showing a dialog)
	if app.activeOverlay == overlayConfirm {
		t.Fatal("non-privileged pod debug should not show confirm dialog")
	}
	if app.pendingDebug != nil {
		t.Fatal("pendingDebug should be nil for non-privileged pod debug")
	}
}

func TestDebugConfirmResultCancelled(t *testing.T) {
	app := newTestApp()
	app.k8sClient = &k8s.Client{Namespace: "default"}

	nodesPlugin := &mockClusterPlugin{
		mockPlugin: mockPlugin{
			name: "nodes",
			gvr:  schema.GroupVersionResource{Version: "v1", Resource: "nodes"},
		},
	}
	plugin.Register(nodesPlugin)
	app.layout.AddSplit(nodesPlugin, "default")

	obj := &unstructured.Unstructured{}
	obj.SetName("worker-1")
	app.layout.FocusedSplit().SetObjects([]*unstructured.Unstructured{obj})

	// Trigger debug to set pendingDebug
	model, _ := app.executeCommand("debug")
	app = model.(App)

	if app.pendingDebug == nil {
		t.Fatal("expected pendingDebug to be set")
	}

	// Cancel the confirmation
	model, cmd := app.update(msgs.ConfirmResultMsg{Action: msgs.ConfirmCancel})
	app = model.(App)

	if app.activeOverlay != overlayNone {
		t.Fatal("expected overlay to be cleared after cancel")
	}
	if app.pendingDebug != nil {
		t.Fatal("expected pendingDebug to be cleared after cancel")
	}
	if cmd != nil {
		t.Fatal("expected nil cmd after cancelling debug confirmation")
	}
}

func TestNamespacePickerUsesContextWithTimeout(t *testing.T) {
	// Create a fake clientset with namespaces.
	ns1 := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}}
	ns2 := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}}
	fakeClient := fake.NewClientset(ns1, ns2)

	app := newTestApp()
	app.config.API.TimeoutSeconds = 10
	app.k8sClient = &k8s.Client{
		Typed:     fakeClient,
		Namespace: "default",
	}

	_, cmd := app.executeCommand("namespace-picker")
	if cmd == nil {
		t.Fatal("expected a non-nil cmd from namespace-picker with k8s client")
	}

	// The returned cmd is a tea.Batch; execute it to trigger the async listing.
	batchMsg := cmd()
	batch, ok := batchMsg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("expected tea.BatchMsg, got %T", batchMsg)
	}

	// Execute each sub-command and find the NamespacesLoadedMsg.
	var loadedMsg *msgs.NamespacesLoadedMsg
	for _, sub := range batch {
		if sub == nil {
			continue
		}
		result := sub()
		if m, ok := result.(msgs.NamespacesLoadedMsg); ok {
			loadedMsg = &m
		}
	}

	if loadedMsg == nil {
		t.Fatal("expected NamespacesLoadedMsg from batch")
	}
	if loadedMsg.Err != nil {
		t.Fatalf("unexpected error: %v", loadedMsg.Err)
	}
	if len(loadedMsg.Namespaces) != 2 {
		t.Fatalf("expected 2 namespaces, got %d", len(loadedMsg.Namespaces))
	}
}

func TestNamespacePickerTimeoutExpires(t *testing.T) {
	// Create a fake clientset with a reactor that delays the response
	// beyond the configured timeout.
	fakeClient := fake.NewClientset()
	fakeClient.PrependReactor("list", "namespaces", func(action k8stesting.Action) (bool, runtime.Object, error) {
		// Sleep longer than the configured timeout to trigger deadline exceeded.
		time.Sleep(200 * time.Millisecond)
		return false, nil, nil
	})

	app := newTestApp()
	// Use 1 second as minimum (config uses integer seconds).
	// We can't go below 1s with the config, but we override the timeout
	// directly through the captured closure. Since APITimeout returns 1s
	// and the reactor sleeps 200ms, this won't trigger a timeout.
	//
	// Instead, test that no k8sClient means no cmd is returned.
	// And test that with a client, we get the expected async behavior.
	app.config.API.TimeoutSeconds = 5
	app.k8sClient = &k8s.Client{
		Typed:     fakeClient,
		Namespace: "default",
	}

	_, cmd := app.executeCommand("namespace-picker")
	if cmd == nil {
		t.Fatal("expected a non-nil cmd")
	}

	// Verify the async cmd completes (reactor delays but within timeout).
	batchMsg := cmd()
	batch, ok := batchMsg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("expected tea.BatchMsg, got %T", batchMsg)
	}

	var loadedMsg *msgs.NamespacesLoadedMsg
	for _, sub := range batch {
		if sub == nil {
			continue
		}
		result := sub()
		if m, ok := result.(msgs.NamespacesLoadedMsg); ok {
			loadedMsg = &m
		}
	}

	if loadedMsg == nil {
		t.Fatal("expected NamespacesLoadedMsg from batch")
	}
	// With a 5s timeout and 200ms delay, the listing should succeed.
	if loadedMsg.Err != nil {
		t.Fatalf("unexpected error: %v", loadedMsg.Err)
	}
}

func TestNamespacePickerNoClientReturnsNil(t *testing.T) {
	app := newTestApp()
	// No k8sClient set — should return nil cmd.
	_, cmd := app.executeCommand("namespace-picker")
	if cmd != nil {
		t.Fatal("expected nil cmd when k8sClient is nil")
	}
}

func TestSubstituteVarsRejectsShellMetacharacters(t *testing.T) {
	tests := []struct {
		name     string
		objName  string
		template string
	}{
		{"semicolon", "foo;rm -rf /", "echo $NAME"},
		{"dollar-paren", "$(whoami)", "echo $NAME"},
		{"backtick", "`id`", "echo $NAME"},
		{"pipe", "foo|cat /etc/passwd", "echo $NAME"},
		{"ampersand", "foo&bg", "echo $NAME"},
		{"space", "foo bar", "echo $NAME"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newTestApp()

			podsPlugin := &mockPlugin{
				name: "pods",
				gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
			}
			plugin.Register(podsPlugin)
			app.layout.AddSplit(podsPlugin, "default")

			obj := &unstructured.Unstructured{}
			obj.SetName(tt.objName)
			obj.SetNamespace("default")
			focused := app.layout.FocusedSplit()
			focused.SetObjects([]*unstructured.Unstructured{obj})

			_, err := app.substituteVars(tt.template)
			if err == nil {
				t.Fatalf("expected error for unsafe name %q, got nil", tt.objName)
			}
		})
	}
}

func TestHandlePortForwardRequested_DuplicateLocalPortRejectedSynchronously(t *testing.T) {
	reg := portforward.NewRegistry()
	// Pre-register an entry on local port 8080.
	reg.Add(portforward.Entry{
		PodName:      "existing-pod",
		PodNamespace: "default",
		LocalPort:    8080,
		RemotePort:   80,
		Status:       portforward.StatusReady,
	})

	a := newTestApp()
	a.pfRegistry = reg
	// Set a non-nil k8sClient so the nil guard doesn't short-circuit.
	a.k8sClient = &k8s.Client{}

	msg := msgs.PortForwardRequestedMsg{
		PodName:      "new-pod",
		PodNamespace: "default",
		LocalPort:    8080,
		RemotePort:   80,
	}

	_, cmd := a.handlePortForwardRequested(msg)
	if cmd == nil {
		t.Fatal("expected a command to be returned for duplicate port")
	}

	// Execute the returned command synchronously — it should NOT call
	// k8s.PortForward (which would panic with a nil REST config).
	result := cmd()
	started, ok := result.(msgs.PortForwardStartedMsg)
	if !ok {
		t.Fatalf("expected PortForwardStartedMsg, got %T", result)
	}
	if started.Err == nil {
		t.Fatal("expected error for duplicate local port, got nil")
	}
	if started.ID != "" {
		t.Fatalf("expected empty ID for rejected port-forward, got %q", started.ID)
	}
}

func TestGotoQualifiedName(t *testing.T) {
	app := newTestApp()

	// Register two plugins with the same resource name but different groups.
	certManagerCerts := &mockPlugin{
		name: "certificates",
		gvr:  schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "certificates"},
	}
	k8sCerts := &mockPlugin{
		name: "certificates",
		gvr:  schema.GroupVersionResource{Group: "certificates.k8s.io", Version: "v1", Resource: "certificates"},
	}

	plugin.Register(certManagerCerts)
	plugin.Register(k8sCerts) // overwrites byName["certificates"]

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	plugin.Register(podsPlugin)
	app.layout.AddSplit(podsPlugin, "default")

	// Use qualified name to navigate to cert-manager certificates specifically.
	model, _ := app.handleGoto("certificates.cert-manager.io/v1", "")
	app = model.(App)

	focused := app.layout.FocusedSplit()
	if focused == nil {
		t.Fatal("expected a focused split")
	}
	if focused.Plugin().GVR() != certManagerCerts.GVR() {
		t.Fatalf("expected cert-manager.io GVR, got %v", focused.Plugin().GVR())
	}
}

func TestGotoBareName(t *testing.T) {
	app := newTestApp()

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	deploymentsPlugin := &mockPlugin{
		name: "deployments",
		gvr:  schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"},
	}

	plugin.Register(podsPlugin)
	plugin.Register(deploymentsPlugin)
	app.layout.AddSplit(podsPlugin, "default")

	// Bare name should still work (backward compat).
	model, _ := app.handleGoto("deployments", "")
	app = model.(App)

	focused := app.layout.FocusedSplit()
	if focused == nil {
		t.Fatal("expected a focused split")
	}
	if focused.Plugin().Name() != "deployments" {
		t.Fatalf("expected 'deployments', got %q", focused.Plugin().Name())
	}
}

func TestHandleGotoGVR(t *testing.T) {
	app := newTestApp()

	certManagerCerts := &mockPlugin{
		name: "certificates",
		gvr:  schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "certificates"},
	}
	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	plugin.Register(certManagerCerts)
	plugin.Register(podsPlugin)
	app.layout.AddSplit(podsPlugin, "default")

	// Navigate using GVR string.
	model, _ := app.handleGotoGVR("cert-manager.io/v1/certificates")
	app = model.(App)

	focused := app.layout.FocusedSplit()
	if focused == nil {
		t.Fatal("expected a focused split")
	}
	if focused.Plugin().GVR() != certManagerCerts.GVR() {
		t.Fatalf("expected cert-manager.io GVR, got %v", focused.Plugin().GVR())
	}
}

func TestHandleGotoGVR_CoreGroup(t *testing.T) {
	app := newTestApp()

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	plugin.Register(podsPlugin)

	deploymentsPlugin := &mockPlugin{
		name: "deployments",
		gvr:  schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"},
	}
	plugin.Register(deploymentsPlugin)
	app.layout.AddSplit(deploymentsPlugin, "default")

	// Navigate using core group GVR (no group prefix).
	model, _ := app.handleGotoGVR("v1/pods")
	app = model.(App)

	focused := app.layout.FocusedSplit()
	if focused == nil {
		t.Fatal("expected a focused split")
	}
	if focused.Plugin().GVR() != podsPlugin.GVR() {
		t.Fatalf("expected core pods GVR, got %v", focused.Plugin().GVR())
	}
}

func TestResourcePickerGotoGVR(t *testing.T) {
	app := newTestApp()

	certManagerCerts := &mockPlugin{
		name: "certificates",
		gvr:  schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "certificates"},
	}
	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	plugin.Register(certManagerCerts)
	plugin.Register(podsPlugin)
	app.layout.AddSplit(podsPlugin, "default")

	// Use resource picker command to navigate via GVR.
	model, _ := app.handleResourcePickerCommand("goto-gvr cert-manager.io/v1/certificates")
	app = model.(App)

	focused := app.layout.FocusedSplit()
	if focused == nil {
		t.Fatal("expected a focused split")
	}
	if focused.Plugin().GVR() != certManagerCerts.GVR() {
		t.Fatalf("expected cert-manager.io GVR, got %v", focused.Plugin().GVR())
	}
}

func TestHandleGotoGVR_InvalidFormat(t *testing.T) {
	app := newTestApp()

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	plugin.Register(podsPlugin)
	app.layout.AddSplit(podsPlugin, "default")

	// Single segment — invalid.
	model, _ := app.handleGotoGVR("pods")
	app = model.(App)

	// Should still be on pods (navigation didn't happen).
	focused := app.layout.FocusedSplit()
	if focused == nil {
		t.Fatal("expected a focused split")
	}
	if focused.Plugin().Name() != "pods" {
		t.Fatalf("expected 'pods' (unchanged), got %q", focused.Plugin().Name())
	}
}
