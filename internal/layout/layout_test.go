package layout

import (
	"context"
	"testing"

	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/render"
	"github.com/aohoyd/aku/internal/ui"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type mockPlugin struct {
	name string
	gvr  schema.GroupVersionResource
}

func (m *mockPlugin) Name() string                     { return m.name }
func (m *mockPlugin) ShortName() string                { return m.name[:2] }
func (m *mockPlugin) GVR() schema.GroupVersionResource { return m.gvr }
func (m *mockPlugin) IsClusterScoped() bool            { return false }
func (m *mockPlugin) Columns() []plugin.Column {
	return []plugin.Column{{Title: "NAME", Flex: true}, {Title: "STATUS", Width: 10}}
}
func (m *mockPlugin) Row(obj *unstructured.Unstructured) []string {
	return []string{obj.GetName(), "ok"}
}
func (m *mockPlugin) YAML(_ *unstructured.Unstructured) (render.Content, error) {
	return render.Content{}, nil
}
func (m *mockPlugin) Describe(_ context.Context, _ *unstructured.Unstructured) (render.Content, error) {
	return render.Content{}, nil
}

func podsPlugin() *mockPlugin {
	return &mockPlugin{name: "pods", gvr: schema.GroupVersionResource{Version: "v1", Resource: "pods"}}
}
func svcsPlugin() *mockPlugin {
	return &mockPlugin{name: "services", gvr: schema.GroupVersionResource{Version: "v1", Resource: "services"}}
}

func TestLayoutAddAndRemoveSplit(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	if l.SplitCount() != 0 {
		t.Fatalf("expected 0 splits, got %d", l.SplitCount())
	}

	l.AddSplit(podsPlugin(), "default", "")
	if l.SplitCount() != 1 {
		t.Fatalf("expected 1 split, got %d", l.SplitCount())
	}

	l.AddSplit(svcsPlugin(), "default", "")
	if l.SplitCount() != 2 {
		t.Fatalf("expected 2 splits, got %d", l.SplitCount())
	}

	shouldQuit := l.CloseCurrentSplit()
	if shouldQuit {
		t.Fatal("should not signal quit when 1 split remains")
	}
	if l.SplitCount() != 1 {
		t.Fatalf("expected 1 split after close, got %d", l.SplitCount())
	}
}

func TestLayoutFocusedSplitRect(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)

	// No splits: no rect.
	if _, ok := l.FocusedSplitRect(); ok {
		t.Fatal("expected no rect when there are no splits")
	}

	l.AddSplit(podsPlugin(), "default", "")
	l.AddSplit(svcsPlugin(), "default", "")

	rect, ok := l.FocusedSplitRect()
	if !ok {
		t.Fatal("expected a focused split rect")
	}
	if rect.Kind != PaneSplit {
		t.Fatalf("rect.Kind = %v, want PaneSplit", rect.Kind)
	}
	if rect.SplitIdx != l.FocusIndex() {
		t.Fatalf("rect.SplitIdx = %d, want focus index %d", rect.SplitIdx, l.FocusIndex())
	}
	if rect.W <= 0 || rect.H <= 0 {
		t.Fatalf("rect has non-positive size: %+v", rect)
	}
	if rect.X < 0 || rect.Y < 0 || rect.X+rect.W > 80 || rect.Y+rect.H > 26 {
		t.Fatalf("rect out of bounds for 80x26: %+v", rect)
	}
}

func TestLayoutCloseLastSplit(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default", "")
	shouldQuit := l.CloseCurrentSplit()
	if !shouldQuit {
		t.Fatal("closing last split should signal quit")
	}
}

func TestLayoutFocusCycling(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default", "")
	l.AddSplit(svcsPlugin(), "default", "")

	if l.FocusIndex() != 1 {
		t.Fatalf("focus should be on newest split (1), got %d", l.FocusIndex())
	}

	l.FocusPrev()
	if l.FocusIndex() != 0 {
		t.Fatalf("after FocusPrev, focus should be 0, got %d", l.FocusIndex())
	}

	l.FocusPrev()
	if l.FocusIndex() != 1 {
		t.Fatalf("FocusPrev should wrap to 1, got %d", l.FocusIndex())
	}

	l.FocusNext()
	if l.FocusIndex() != 0 {
		t.Fatalf("FocusNext should wrap to 0, got %d", l.FocusIndex())
	}
}

// countFocusedSplits returns how many split borders are currently focused.
func countFocusedSplits(l *Layout) int {
	n := 0
	for i := 0; i < l.SplitCount(); i++ {
		if l.SplitAt(i).Focused() {
			n++
		}
	}
	return n
}

// TestLayoutFocusNextReleasesDetailFocus verifies that when the detail panel
// holds focus, FocusNext releases it (focusTarget returns to resources, the
// detail panel is blurred) and exactly one split border ends up focused — so
// the resource-list and detail borders never highlight simultaneously.
func TestLayoutFocusNextReleasesDetailFocus(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default", "")
	l.AddSplit(svcsPlugin(), "default", "")
	l.ShowRightPanel()

	l.FocusDetails()
	if !l.FocusedDetails() {
		t.Fatal("precondition: details should be focused after FocusDetails")
	}

	l.FocusNext()

	if !l.FocusedResources() {
		t.Fatal("FocusNext should reset focus target to resources")
	}
	if l.FocusedDetails() {
		t.Fatal("FocusNext should release detail focus")
	}
	if got := countFocusedSplits(&l); got != 1 {
		t.Fatalf("expected exactly one focused split border, got %d", got)
	}
}

// TestLayoutFocusPrevReleasesDetailFocus mirrors the FocusNext assertion for
// FocusPrev.
func TestLayoutFocusPrevReleasesDetailFocus(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default", "")
	l.AddSplit(svcsPlugin(), "default", "")
	l.ShowRightPanel()

	l.FocusDetails()
	if !l.FocusedDetails() {
		t.Fatal("precondition: details should be focused after FocusDetails")
	}

	l.FocusPrev()

	if !l.FocusedResources() {
		t.Fatal("FocusPrev should reset focus target to resources")
	}
	if l.FocusedDetails() {
		t.Fatal("FocusPrev should release detail focus")
	}
	if got := countFocusedSplits(&l); got != 1 {
		t.Fatalf("expected exactly one focused split border, got %d", got)
	}
}

// TestLayoutFocusCycleStillWorksFromResources is a regression guard: when
// resources are already focused, FocusNext/FocusPrev keep cycling the split
// focus index as before, leaving exactly one split border focused.
func TestLayoutFocusCycleStillWorksFromResources(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default", "")
	l.AddSplit(svcsPlugin(), "default", "")

	// Focus starts on the newest split (idx 1) with resources focused.
	if !l.FocusedResources() {
		t.Fatal("precondition: resources should be focused")
	}

	l.FocusNext() // wraps to 0
	if l.FocusIndex() != 0 {
		t.Fatalf("FocusNext should move to 0, got %d", l.FocusIndex())
	}
	if !l.FocusedResources() {
		t.Fatal("resources should remain focused after FocusNext")
	}
	if got := countFocusedSplits(&l); got != 1 {
		t.Fatalf("expected exactly one focused split border, got %d", got)
	}

	l.FocusPrev() // back to 1
	if l.FocusIndex() != 1 {
		t.Fatalf("FocusPrev should move to 1, got %d", l.FocusIndex())
	}
	if got := countFocusedSplits(&l); got != 1 {
		t.Fatalf("expected exactly one focused split border, got %d", got)
	}
}

func TestLayoutRightPanel(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	if l.RightPanelVisible() {
		t.Fatal("right panel should start hidden")
	}
	l.ShowRightPanel()
	if !l.RightPanelVisible() {
		t.Fatal("right panel should be visible after Show")
	}
	l.HideRightPanel()
	if l.RightPanelVisible() {
		t.Fatal("right panel should be hidden after Hide")
	}
}

func TestLayoutFocusedSplitNil(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	if l.FocusedSplit() != nil {
		t.Fatal("FocusedSplit should be nil with no splits")
	}
}

// TestLayoutAddSplitStoresPaneInterface is a regression guard that the
// heterogeneous splits slice still holds resource panes that round-trip back to
// *ui.ResourceList through SplitAt for every index after adds.
func TestLayoutAddSplitStoresPaneInterface(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default", "")
	l.AddSplit(svcsPlugin(), "default", "")
	l.AddSplit(podsPlugin(), "default", "")

	for i := 0; i < l.SplitCount(); i++ {
		if l.SplitAt(i) == nil {
			t.Fatalf("SplitAt(%d) should return a non-nil resource pane", i)
		}
	}
}

// TestAddTerminalSplitInsertsAdjacentAndFocuses asserts AddTerminalSplit inserts
// the new pane immediately after the focused split and transfers focus to it.
func TestAddTerminalSplitInsertsAdjacentAndFocuses(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default", "") // idx 0
	l.AddSplit(svcsPlugin(), "default", "") // idx 1 (focused)
	// Focus the first split so the terminal must be inserted at idx 1 (adjacent).
	l.FocusSplitAt(0)

	tp := ui.NewTerminalPane("term", "ctx-1", 40, 10)
	tp.SetID("t:1")
	l.AddTerminalSplit(tp)

	if l.SplitCount() != 3 {
		t.Fatalf("expected 3 splits after AddTerminalSplit, got %d", l.SplitCount())
	}
	// Inserted adjacent to the focused split (idx 0) → at idx 1.
	if l.FocusIndex() != 1 {
		t.Fatalf("expected new terminal focused at idx 1, got %d", l.FocusIndex())
	}
	got, ok := l.FocusedPane().(*ui.TerminalPane)
	if !ok || got != tp {
		t.Fatalf("focused pane is not the new terminal pane (ok=%v)", ok)
	}
	if l.PaneAtIdx(1) != tp {
		t.Fatalf("terminal pane not at insertion idx 1")
	}
}

// TestAddTerminalSplitFirstPane asserts AddTerminalSplit on an empty layout
// inserts at idx 0 and focuses it.
func TestAddTerminalSplitFirstPane(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	tp := ui.NewTerminalPane("term", "ctx-1", 40, 10)
	tp.SetID("t:first")
	l.AddTerminalSplit(tp)

	if l.SplitCount() != 1 {
		t.Fatalf("expected 1 split, got %d", l.SplitCount())
	}
	if l.FocusIndex() != 0 {
		t.Fatalf("expected focus idx 0, got %d", l.FocusIndex())
	}
	if l.FocusedPane() != tp {
		t.Fatal("first terminal pane should be focused")
	}
}

// TestTerminalPaneByID covers both the found and missing cases.
func TestTerminalPaneByID(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default", "") // a resource pane (not a terminal)

	tp := ui.NewTerminalPane("term", "ctx-1", 40, 10)
	tp.SetID("t:found")
	l.AddTerminalSplit(tp)

	got, ok := l.TerminalPaneByID("t:found")
	if !ok || got != tp {
		t.Fatalf("TerminalPaneByID(found): ok=%v got=%v want=%v", ok, got, tp)
	}
	if _, ok := l.TerminalPaneByID("t:missing"); ok {
		t.Fatal("TerminalPaneByID(missing) should report not found")
	}
	// A resource pane's id-space must not be matched as a terminal.
	if _, ok := l.TerminalPaneByID("pods"); ok {
		t.Fatal("a resource pane must not be returned by TerminalPaneByID")
	}
}

// TestTerminalPaneInnerSize asserts the inner size is reported for a present
// terminal pane and ok=false for a missing one. The inner size is the outer pane
// size minus the border chrome the pane reserves.
func TestTerminalPaneInnerSize(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	tp := ui.NewTerminalPane("term", "ctx-1", 40, 10)
	tp.SetID("t:size")
	l.AddTerminalSplit(tp) // recalcSizes assigns the pane its real size

	iw, ih, ok := l.TerminalPaneInnerSize("t:size")
	if !ok {
		t.Fatal("TerminalPaneInnerSize should report ok for a present pane")
	}
	// recalcSizes set the pane to fill the layout; inner size must be positive and
	// match the pane's own InnerSize accounting.
	wantW, wantH := tp.InnerSize()
	if iw != wantW || ih != wantH {
		t.Fatalf("inner size = %dx%d, want %dx%d", iw, ih, wantW, wantH)
	}
	if iw <= 0 || ih <= 0 {
		t.Fatalf("inner size should be positive, got %dx%d", iw, ih)
	}

	if _, _, ok := l.TerminalPaneInnerSize("t:missing"); ok {
		t.Fatal("TerminalPaneInnerSize(missing) should report ok=false")
	}
}

// TestTerminalPaneInnerSizeHiddenPaneNotOK asserts that a hidden (non-focused,
// zoomed-out) terminal pane is reported as ok=false so the app skips forwarding
// the clamped 1x1 inner size to the remote shell (which would reflow a
// background full-screen program). Under ZoomSplit recalcSizes assigns 0x0 to
// every non-focused split.
func TestTerminalPaneInnerSizeHiddenPaneNotOK(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default", "") // a second, focusable resource split
	tp := ui.NewTerminalPane("term", "ctx-1", 40, 10)
	tp.SetID("t:hidden")
	l.AddTerminalSplit(tp) // inserted and focused

	// The terminal pane is focused; move focus away so it becomes the hidden one
	// under zoom, then zoom the now-focused split.
	l.FocusNext()
	l.ToggleZoomSplit()

	if _, _, ok := l.TerminalPaneInnerSize("t:hidden"); ok {
		t.Fatal("a hidden (0x0) terminal pane should report ok=false")
	}
}

func TestLayoutView(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default", "")
	view := l.View()
	if view == "" {
		t.Fatal("view should not be empty with a split")
	}
}

func TestLayoutViewWithRightPanel(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default", "")
	l.ShowRightPanel()
	view := l.View()
	if view == "" {
		t.Fatal("view should not be empty with splits and right panel")
	}
}

func TestUpdateSplitObjectsNamespaceFiltering(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default", "")
	l.AddSplit(podsPlugin(), "staging", "")

	objs := []*unstructured.Unstructured{
		{Object: map[string]any{"metadata": map[string]any{"name": "pod-a"}}},
	}

	// Update only "staging" namespace — split 0 ("default") should be empty.
	// Empty ctxName matches the transitional empty-pane-context rule.
	l.UpdateSplitObjects(podsPlugin(), "staging", "", objs)

	if l.SplitAt(0).Len() != 0 {
		t.Fatal("split 0 (default) should not have received staging objects")
	}
	if l.SplitAt(1).Len() != 1 {
		t.Fatal("split 1 (staging) should have received objects")
	}
}

// TestUpdateSplitObjectsContextFiltering verifies UpdateSplitObjects only
// updates panes whose context matches the originating context under strict
// equality. The core simultaneous-multi-cluster correctness guarantee: a prod
// update repaints only the prod pane and leaves the staging-pinned pane (and an
// empty-context pane) untouched.
func TestUpdateSplitObjectsContextFiltering(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default", "") // split 0: context "prod"
	l.AddSplit(podsPlugin(), "default", "") // split 1: context "staging"
	l.AddSplit(podsPlugin(), "default", "") // split 2: context "" (matches nothing)

	l.SplitAt(0).SetContext("prod")
	l.SplitAt(1).SetContext("staging")
	// split 2 keeps the default empty context

	objs := []*unstructured.Unstructured{
		{Object: map[string]any{"metadata": map[string]any{"name": "pod-a"}}},
	}

	// Update only the "prod" context.
	l.UpdateSplitObjects(podsPlugin(), "default", "prod", objs)

	if l.SplitAt(0).Len() != 1 {
		t.Fatalf("split 0 (prod) should have received objects, got %d", l.SplitAt(0).Len())
	}
	if l.SplitAt(1).Len() != 0 {
		t.Fatalf("split 1 (staging) should NOT have received prod objects, got %d", l.SplitAt(1).Len())
	}
	// Strict equality: an empty-context pane matches no real context.
	if l.SplitAt(2).Len() != 0 {
		t.Fatalf("split 2 (empty context) should match nothing under strict equality, got %d", l.SplitAt(2).Len())
	}
}

// TestUpdateSplitObjectsEmptyPaneContextMatchesNonEmptyMsg verifies that under
// strict equality a pane with an empty context matches no real store context
// (empty no longer wildcard-matches as it did under the transitional rule).
func TestUpdateSplitObjectsEmptyPaneContextMatchesNonEmptyMsg(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default", "") // context "" by default

	objs := []*unstructured.Unstructured{
		{Object: map[string]any{"metadata": map[string]any{"name": "pod-a"}}},
	}

	// Store context is a real name, pane context is empty: strict equality means
	// it must NOT match.
	l.UpdateSplitObjects(podsPlugin(), "default", "my-real-context", objs)

	if l.SplitAt(0).Len() != 0 {
		t.Fatalf("empty-context pane should match nothing under strict equality, got %d", l.SplitAt(0).Len())
	}
}

// TestUpdateSplitObjectsTwoPanesDifferentClusters is the explicit side-by-side
// multi-cluster guarantee: with two panes on different clusters, a "prod"
// update repaints only the prod pane and leaves the staging pane untouched.
func TestUpdateSplitObjectsTwoPanesDifferentClusters(t *testing.T) {
	l := New(120, 40, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default", "")
	l.SplitAt(0).SetContext("prod")
	l.AddSplit(podsPlugin(), "default", "")
	l.SplitAt(1).SetContext("staging")

	objs := []*unstructured.Unstructured{
		{Object: map[string]any{"metadata": map[string]any{"name": "prod-a"}}},
		{Object: map[string]any{"metadata": map[string]any{"name": "prod-b"}}},
	}
	l.UpdateSplitObjects(podsPlugin(), "default", "prod", objs)

	if l.SplitAt(0).Len() != 2 {
		t.Fatalf("expected prod pane to show 2 objects, got %d", l.SplitAt(0).Len())
	}
	if l.SplitAt(1).Len() != 0 {
		t.Fatalf("expected staging pane untouched, got %d objects", l.SplitAt(1).Len())
	}
}

func TestAddSplitSetsNamespace(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "kube-system", "")
	if l.FocusedSplit().Namespace() != "kube-system" {
		t.Fatalf("expected namespace 'kube-system', got %q", l.FocusedSplit().Namespace())
	}
}

// TestAddSplitSeedsContext verifies the pane is born carrying the context passed
// to AddSplit, so it is never observed with an empty context (the source of the
// wrong-context badge flicker).
func TestAddSplitSeedsContext(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default", "prod")
	if got := l.FocusedSplit().Context(); got != "prod" {
		t.Fatalf("expected pane context 'prod' at creation, got %q", got)
	}
}

// TestAddSplitInsertsAfterFocusedNotLast verifies a new split lands directly
// after the focused pane (focusIdx+1) rather than at the end of the slice.
func TestAddSplitInsertsAfterFocusedNotLast(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default", "") // idx 0: pods
	l.AddSplit(podsPlugin(), "default", "") // idx 1: pods
	l.AddSplit(podsPlugin(), "default", "") // idx 2: pods
	if l.SplitCount() != 3 {
		t.Fatalf("expected 3 splits, got %d", l.SplitCount())
	}

	// Focus the middle pane.
	l.FocusSplitAt(1)
	if l.FocusIndex() != 1 {
		t.Fatalf("expected focus on 1, got %d", l.FocusIndex())
	}

	// Adding a split should insert at focusIdx+1 (== 2), not at the end.
	l.AddSplit(svcsPlugin(), "default", "")
	if l.SplitCount() != 4 {
		t.Fatalf("expected 4 splits after add, got %d", l.SplitCount())
	}
	if l.FocusIndex() != 2 {
		t.Fatalf("expected focus on inserted pane at index 2, got %d", l.FocusIndex())
	}
	if got := l.SplitAt(2).Plugin().Name(); got != "services" {
		t.Fatalf("expected inserted pane at index 2 to be 'services', got %q", got)
	}
	// The pane previously at index 2 should have shifted to index 3.
	if got := l.SplitAt(3).Plugin().Name(); got != "pods" {
		t.Fatalf("expected pre-existing pane shifted to index 3 to be 'pods', got %q", got)
	}

	// After a middle insert, only the newly inserted pane (index 2) is focused;
	// every other pane must be blurred.
	if !l.SplitAt(2).Focused() {
		t.Fatal("newly inserted pane at index 2 should be focused")
	}
	for _, idx := range []int{0, 1, 3} {
		if l.SplitAt(idx).Focused() {
			t.Fatalf("pane at index %d should be blurred after middle insert", idx)
		}
	}
}

// TestAddSplitWithSinglePaneAppendsAfter verifies that with one existing pane
// the new split lands at index 1 (after it) and becomes focused.
func TestAddSplitWithSinglePaneAppendsAfter(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default", "")
	if l.FocusIndex() != 0 {
		t.Fatalf("expected focus on 0 with a single pane, got %d", l.FocusIndex())
	}

	l.AddSplit(svcsPlugin(), "default", "")
	if l.SplitCount() != 2 {
		t.Fatalf("expected 2 splits, got %d", l.SplitCount())
	}
	if l.FocusIndex() != 1 {
		t.Fatalf("expected focus on inserted pane at index 1, got %d", l.FocusIndex())
	}
	if got := l.SplitAt(1).Plugin().Name(); got != "services" {
		t.Fatalf("expected inserted pane at index 1 to be 'services', got %q", got)
	}
}

// TestAddSplitBlursPreviousFocusesNew verifies the previously focused pane is
// blurred and the newly inserted pane is focused after AddSplit.
func TestAddSplitBlursPreviousFocusesNew(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default", "") // idx 0
	if !l.SplitAt(0).Focused() {
		t.Fatal("precondition: the single existing pane should be focused")
	}

	l.AddSplit(svcsPlugin(), "default", "") // inserted at idx 1, focused

	// slices.Insert reallocates the backing array, so any pointer captured before
	// AddSplit is stale. Re-fetch the previously-focused pane via its live index:
	// inserting a second pane goes to index 1, leaving the first pane at index 0.
	prev := l.SplitAt(0)
	if prev.Focused() {
		t.Fatal("previously focused pane should be blurred after AddSplit")
	}
	if !l.FocusedSplit().Focused() {
		t.Fatal("newly inserted pane should be focused after AddSplit")
	}
	if l.FocusIndex() != 1 {
		t.Fatalf("expected focus on inserted pane at index 1, got %d", l.FocusIndex())
	}
}

// TestMoveFocusedSplitNext verifies MoveFocusedSplit(+1) swaps the focused pane
// with the next one and focusIdx follows the moved pane.
func TestMoveFocusedSplitNext(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default", "") // idx 0: pods
	l.AddSplit(svcsPlugin(), "default", "") // idx 1: services
	l.AddSplit(podsPlugin(), "default", "") // idx 2: pods

	// Focus the services pane (idx 1).
	l.FocusSplitAt(1)
	if got := l.SplitAt(1).Plugin().Name(); got != "services" {
		t.Fatalf("precondition: expected 'services' at index 1, got %q", got)
	}

	l.MoveFocusedSplit(+1)

	if l.FocusIndex() != 2 {
		t.Fatalf("expected focusIdx to follow moved pane to 2, got %d", l.FocusIndex())
	}
	if got := l.SplitAt(2).Plugin().Name(); got != "services" {
		t.Fatalf("expected moved 'services' pane now at index 2, got %q", got)
	}
	if got := l.SplitAt(1).Plugin().Name(); got != "pods" {
		t.Fatalf("expected swapped 'pods' pane now at index 1, got %q", got)
	}
}

// TestMoveFocusedSplitPrev verifies MoveFocusedSplit(-1) swaps with the previous
// pane and focusIdx follows the moved pane.
func TestMoveFocusedSplitPrev(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default", "") // idx 0: pods
	l.AddSplit(svcsPlugin(), "default", "") // idx 1: services
	l.AddSplit(podsPlugin(), "default", "") // idx 2: pods

	// Focus the services pane (idx 1).
	l.FocusSplitAt(1)

	l.MoveFocusedSplit(-1)

	if l.FocusIndex() != 0 {
		t.Fatalf("expected focusIdx to follow moved pane to 0, got %d", l.FocusIndex())
	}
	if got := l.SplitAt(0).Plugin().Name(); got != "services" {
		t.Fatalf("expected moved 'services' pane now at index 0, got %q", got)
	}
	if got := l.SplitAt(1).Plugin().Name(); got != "pods" {
		t.Fatalf("expected swapped 'pods' pane now at index 1, got %q", got)
	}
}

// TestMoveFocusedSplitNoopAtEdges verifies MoveFocusedSplit(-1) at index 0 and
// MoveFocusedSplit(+1) at the last index leave the slice order and focusIdx
// unchanged.
func TestMoveFocusedSplitNoopAtEdges(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default", "") // idx 0: pods
	l.AddSplit(svcsPlugin(), "default", "") // idx 1: services

	// At index 0, moving toward the start is a no-op.
	l.FocusSplitAt(0)
	l.MoveFocusedSplit(-1)
	if l.FocusIndex() != 0 {
		t.Fatalf("expected focusIdx 0 unchanged at start edge, got %d", l.FocusIndex())
	}
	if got := l.SplitAt(0).Plugin().Name(); got != "pods" {
		t.Fatalf("expected 'pods' still at index 0, got %q", got)
	}
	if got := l.SplitAt(1).Plugin().Name(); got != "services" {
		t.Fatalf("expected 'services' still at index 1, got %q", got)
	}

	// At the last index, moving toward the end is a no-op.
	l.FocusSplitAt(1)
	l.MoveFocusedSplit(+1)
	if l.FocusIndex() != 1 {
		t.Fatalf("expected focusIdx 1 unchanged at end edge, got %d", l.FocusIndex())
	}
	if got := l.SplitAt(0).Plugin().Name(); got != "pods" {
		t.Fatalf("expected 'pods' still at index 0, got %q", got)
	}
	if got := l.SplitAt(1).Plugin().Name(); got != "services" {
		t.Fatalf("expected 'services' still at index 1, got %q", got)
	}
}

// TestMoveFocusedSplitPreservesFocusState verifies the moved pane keeps its
// focused state and the others stay blurred after the move.
func TestMoveFocusedSplitPreservesFocusState(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default", "") // idx 0: pods
	l.AddSplit(svcsPlugin(), "default", "") // idx 1: services
	l.AddSplit(podsPlugin(), "default", "") // idx 2: pods

	l.FocusSplitAt(1)
	l.MoveFocusedSplit(+1) // services moves to index 2

	if !l.SplitAt(2).Focused() {
		t.Fatal("moved pane at index 2 should still be focused")
	}
	if l.SplitAt(0).Focused() {
		t.Fatal("pane at index 0 should remain blurred")
	}
	if l.SplitAt(1).Focused() {
		t.Fatal("pane at index 1 should remain blurred")
	}
}

// TestMoveFocusedSplitReleasesDetailFocus verifies that when the detail panel
// holds focus, MoveFocusedSplit releases it (focusTarget returns to resources,
// the detail panel is blurred) and exactly one split border ends up focused —
// matching the FocusNext/FocusPrev detail-release behavior.
func TestMoveFocusedSplitReleasesDetailFocus(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default", "") // idx 0: pods
	l.AddSplit(svcsPlugin(), "default", "") // idx 1: services
	l.ShowRightPanel()

	// Focus index 0 then hand focus to the detail panel.
	l.FocusSplitAt(0)
	l.FocusDetails()
	if !l.FocusedDetails() {
		t.Fatal("precondition: details should be focused after FocusDetails")
	}

	l.MoveFocusedSplit(+1) // pods moves to index 1

	if !l.FocusedResources() {
		t.Fatal("MoveFocusedSplit should reset focus target to resources")
	}
	if l.FocusedDetails() {
		t.Fatal("MoveFocusedSplit should release detail focus")
	}
	if got := countFocusedSplits(&l); got != 1 {
		t.Fatalf("expected exactly one focused split border, got %d", got)
	}
	// The move itself must still have happened.
	if l.FocusIndex() != 1 {
		t.Fatalf("expected focusIdx to follow moved pane to 1, got %d", l.FocusIndex())
	}
	if got := l.SplitAt(1).Plugin().Name(); got != "pods" {
		t.Fatalf("expected moved 'pods' pane now at index 1, got %q", got)
	}
}

func TestLayoutZoomSplitToggle(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default", "")
	l.AddSplit(svcsPlugin(), "default", "")

	if l.EffectiveZoom() != ZoomNone {
		t.Fatal("should start with ZoomNone")
	}

	l.ToggleZoomSplit()
	if l.EffectiveZoom() != ZoomSplit {
		t.Fatalf("expected ZoomSplit, got %d", l.EffectiveZoom())
	}

	l.ToggleZoomSplit()
	if l.EffectiveZoom() != ZoomNone {
		t.Fatalf("expected ZoomNone after second toggle, got %d", l.EffectiveZoom())
	}
}

func TestLayoutZoomDetailToggle(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default", "")
	l.ShowRightPanel()

	l.ToggleZoomDetail()
	if l.EffectiveZoom() != ZoomDetail {
		t.Fatalf("expected ZoomDetail, got %d", l.EffectiveZoom())
	}

	l.ToggleZoomDetail()
	if l.EffectiveZoom() != ZoomNone {
		t.Fatalf("expected ZoomNone after second toggle, got %d", l.EffectiveZoom())
	}
}

func TestLayoutUnzoomAll(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default", "")
	l.AddSplit(svcsPlugin(), "default", "")

	l.ToggleZoomSplit()
	if l.EffectiveZoom() != ZoomSplit {
		t.Fatal("precondition: split should be zoomed")
	}
	l.UnzoomAll()
	if l.EffectiveZoom() != ZoomNone {
		t.Fatalf("expected ZoomNone after UnzoomAll, got %d", l.EffectiveZoom())
	}
}

func TestLayoutZoomSplitSizing(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900) // height = 26-1=25
	l.AddSplit(podsPlugin(), "default", "")
	l.AddSplit(svcsPlugin(), "default", "")

	l.ToggleZoomSplit() // focus is on split 1 (services)

	// Focused split (1) should get full height
	focused := l.FocusedSplit()
	if focused.Height() != 25 {
		t.Fatalf("focused split height: expected 25, got %d", focused.Height())
	}

	// Non-focused split (0) should be zero-sized
	other := l.SplitAt(0)
	if other.Height() != 0 || other.Width() != 0 {
		t.Fatalf("non-focused split should be 0x0, got %dx%d", other.Width(), other.Height())
	}
}

func TestLayoutZoomDetailSizing(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900) // height = 25
	l.AddSplit(podsPlugin(), "default", "")
	l.ShowRightPanel()

	l.ToggleZoomDetail()

	// Right panel should get full width and full terminal height (including status bar row)
	rp := l.RightPanel()
	if rp.Width() != 80 || rp.Height() != 26 {
		t.Fatalf("right panel should be 80x26, got %dx%d", rp.Width(), rp.Height())
	}

	// All splits should be zero-sized
	s := l.SplitAt(0)
	if s.Height() != 0 || s.Width() != 0 {
		t.Fatalf("split should be 0x0 in detail zoom, got %dx%d", s.Width(), s.Height())
	}
}

func TestLayoutZoomSplitView(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default", "")
	l.AddSplit(svcsPlugin(), "default", "")

	l.ToggleZoomSplit()
	view := l.View()
	if view == "" {
		t.Fatal("zoomed view should not be empty")
	}
}

func TestLayoutZoomDetailView(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default", "")
	l.ShowRightPanel()

	l.ToggleZoomDetail()
	view := l.View()
	if view == "" {
		t.Fatal("detail zoomed view should not be empty")
	}
}

func TestLayoutZoomFollowsFocus(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)      // height = 25
	l.AddSplit(podsPlugin(), "default", "") // idx 0
	l.AddSplit(svcsPlugin(), "default", "") // idx 1 (focused)

	l.ToggleZoomSplit()

	// Focus split 1 is zoomed — full height
	if l.FocusedSplit().Height() != 25 {
		t.Fatalf("focused split should be full height, got %d", l.FocusedSplit().Height())
	}

	// Tab to split 0 — zoom should follow
	l.FocusNext() // wraps to 0
	if l.FocusIndex() != 0 {
		t.Fatalf("expected focus on 0, got %d", l.FocusIndex())
	}
	if l.FocusedSplit().Height() != 25 {
		t.Fatalf("after FocusNext, new focused split should be full height, got %d", l.FocusedSplit().Height())
	}
	// The other split (1) should be zero
	if l.SplitAt(1).Height() != 0 {
		t.Fatalf("non-focused split should be 0 height, got %d", l.SplitAt(1).Height())
	}
}

func TestLayoutCloseCurrentSplitAutoUnzoom(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default", "")
	l.AddSplit(svcsPlugin(), "default", "")

	l.ToggleZoomSplit()
	l.CloseCurrentSplit()

	// Only 1 split remains — zoom should auto-reset
	if l.EffectiveZoom() != ZoomNone {
		t.Fatalf("expected ZoomNone after close to 1 split, got %d", l.EffectiveZoom())
	}
}

func TestLayoutFocusTarget(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default", "")
	l.ShowRightPanel()

	if !l.FocusedResources() {
		t.Fatal("should start with resources focused")
	}
	if l.FocusedDetails() {
		t.Fatal("details should not be focused initially")
	}

	l.FocusDetails()
	if !l.FocusedDetails() {
		t.Fatal("expected details focused after FocusDetails")
	}
	if l.FocusedResources() {
		t.Fatal("resources should not be focused")
	}

	l.FocusResources()
	if !l.FocusedResources() {
		t.Fatal("expected resources focused after FocusResources")
	}
}

func TestLayoutFocusDetailsNoopWithoutRightPanel(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default", "")

	l.FocusDetails()
	if l.FocusedDetails() {
		t.Fatal("FocusDetails should be no-op when right panel is hidden")
	}
}

func TestLayoutFocusResourcesSafeWithNoSplits(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)

	l.FocusResources()
	if !l.FocusedResources() {
		t.Fatal("should be in resources mode")
	}
}

func TestLayoutZoomSplitNoopWithOneSplit(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default", "")

	l.ToggleZoomSplit()
	if l.EffectiveZoom() != ZoomNone {
		t.Fatal("zoom should be no-op with single split")
	}
}

func TestLayoutIndependentZoomStates(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default", "")
	l.AddSplit(svcsPlugin(), "default", "")
	l.ShowRightPanel()

	l.ToggleZoomSplit()
	if !l.SplitZoomed() {
		t.Fatal("split should be zoomed")
	}

	l.ToggleZoomDetail()
	if !l.DetailZoomed() {
		t.Fatal("detail should be zoomed")
	}
	if !l.SplitZoomed() {
		t.Fatal("split zoom should be preserved when detail zooms")
	}
	if l.EffectiveZoom() != ZoomDetail {
		t.Fatal("detail should take visual precedence")
	}
}

func TestLayoutToggleDetailPreservesSplitZoom(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default", "")
	l.AddSplit(svcsPlugin(), "default", "")
	l.ShowRightPanel()

	l.ToggleZoomSplit()
	l.ToggleZoomDetail()
	l.ToggleZoomDetail() // unzoom detail

	if !l.SplitZoomed() {
		t.Fatal("split zoom should survive detail unzoom")
	}
	if l.DetailZoomed() {
		t.Fatal("detail should be unzoomed")
	}
	if l.EffectiveZoom() != ZoomSplit {
		t.Fatalf("expected ZoomSplit, got %d", l.EffectiveZoom())
	}
}

func TestLayoutToggleSplitPreservesDetailZoom(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default", "")
	l.AddSplit(svcsPlugin(), "default", "")
	l.ShowRightPanel()

	l.ToggleZoomDetail()
	l.ToggleZoomSplit()
	l.ToggleZoomSplit() // unzoom split

	if !l.DetailZoomed() {
		t.Fatal("detail zoom should survive split unzoom")
	}
	if l.SplitZoomed() {
		t.Fatal("split should be unzoomed")
	}
	if l.EffectiveZoom() != ZoomDetail {
		t.Fatalf("expected ZoomDetail, got %d", l.EffectiveZoom())
	}
}

func TestLayoutUnzoomAllNew(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default", "")
	l.AddSplit(svcsPlugin(), "default", "")
	l.ShowRightPanel()

	l.ToggleZoomSplit()
	l.ToggleZoomDetail()
	l.UnzoomAll()

	if l.SplitZoomed() || l.DetailZoomed() {
		t.Fatal("UnzoomAll should clear both zoom flags")
	}
	if l.EffectiveZoom() != ZoomNone {
		t.Fatal("expected ZoomNone after UnzoomAll")
	}
}

func TestLayoutAnyZoomed(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default", "")
	l.AddSplit(svcsPlugin(), "default", "")
	l.ShowRightPanel()

	if l.AnyZoomed() {
		t.Fatal("should not be zoomed initially")
	}
	l.ToggleZoomSplit()
	if !l.AnyZoomed() {
		t.Fatal("should be zoomed after split zoom")
	}
	l.ToggleZoomDetail()
	if !l.AnyZoomed() {
		t.Fatal("should be zoomed with both")
	}
	l.ToggleZoomSplit()
	if !l.AnyZoomed() {
		t.Fatal("should still be zoomed with detail only")
	}
	l.ToggleZoomDetail()
	if l.AnyZoomed() {
		t.Fatal("should not be zoomed after both off")
	}
}

func TestUpdateSplitObjectsSkipsDrillDown(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default", "")

	// Set initial objects on the split
	initial := []*unstructured.Unstructured{
		{Object: map[string]any{"metadata": map[string]any{"name": "filtered-pod"}}},
	}
	l.SplitAt(0).SetObjects(initial)

	// Push into drilldown state
	l.SplitAt(0).PushNav(podsPlugin(), initial, "my-deploy", "uid-1", "", "")

	// Try to overwrite via UpdateSplitObjects
	allPods := []*unstructured.Unstructured{
		{Object: map[string]any{"metadata": map[string]any{"name": "pod-1"}}},
		{Object: map[string]any{"metadata": map[string]any{"name": "pod-2"}}},
		{Object: map[string]any{"metadata": map[string]any{"name": "pod-3"}}},
	}
	l.UpdateSplitObjects(podsPlugin(), "default", "", allPods)

	// Drilldown split should NOT have been updated
	if l.SplitAt(0).Len() != 1 {
		t.Fatalf("drilldown split should still have 1 object, got %d", l.SplitAt(0).Len())
	}
}

func TestLayoutHideRightPanelClearsDetailZoom(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default", "")
	l.ShowRightPanel()

	l.ToggleZoomDetail()
	if !l.DetailZoomed() {
		t.Fatal("detail should be zoomed")
	}

	l.HideRightPanel()
	if l.DetailZoomed() {
		t.Fatal("HideRightPanel should clear detail zoom")
	}
}

func TestLayoutHorizontalZoomNoneSizing(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900) // height = 25
	l.AddSplit(podsPlugin(), "default", "")
	l.AddSplit(svcsPlugin(), "default", "")
	l.ShowRightPanel()
	l.ToggleOrientation() // switch to horizontal

	// Top section: height * 0.5 = 12
	// Bottom section: 25 - 12 = 13
	// Each split width: 80 / 2 = 40

	s0 := l.SplitAt(0)
	if s0.Width() != 40 || s0.Height() != 12 {
		t.Fatalf("split 0: expected 40x12, got %dx%d", s0.Width(), s0.Height())
	}

	s1 := l.SplitAt(1)
	if s1.Width() != 40 || s1.Height() != 12 {
		t.Fatalf("split 1: expected 40x12, got %dx%d", s1.Width(), s1.Height())
	}

	rp := l.RightPanel()
	if rp.Width() != 80 || rp.Height() != 13 {
		t.Fatalf("right panel: expected 80x13, got %dx%d", rp.Width(), rp.Height())
	}
}

func TestLayoutHorizontalZoomNoneNoRightPanel(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900) // height = 25
	l.AddSplit(podsPlugin(), "default", "")
	l.AddSplit(svcsPlugin(), "default", "")
	l.ToggleOrientation()

	// Without right panel, each split gets full height
	s0 := l.SplitAt(0)
	if s0.Width() != 40 || s0.Height() != 25 {
		t.Fatalf("split 0: expected 40x25, got %dx%d", s0.Width(), s0.Height())
	}

	s1 := l.SplitAt(1)
	if s1.Width() != 40 || s1.Height() != 25 {
		t.Fatalf("split 1: expected 40x25, got %dx%d", s1.Width(), s1.Height())
	}
}

func TestLayoutHorizontalZoomSplitSizing(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900) // height = 25
	l.AddSplit(podsPlugin(), "default", "")
	l.AddSplit(svcsPlugin(), "default", "")
	l.ShowRightPanel()
	l.ToggleOrientation()

	l.ToggleZoomSplit() // focus is on split 1

	// Focused split gets full width, topHeight
	focused := l.FocusedSplit()
	if focused.Width() != 80 || focused.Height() != 12 {
		t.Fatalf("focused split: expected 80x12, got %dx%d", focused.Width(), focused.Height())
	}

	// Non-focused split should be zero-sized
	other := l.SplitAt(0)
	if other.Width() != 0 || other.Height() != 0 {
		t.Fatalf("non-focused split: expected 0x0, got %dx%d", other.Width(), other.Height())
	}

	// Right panel gets full width, bottomHeight
	rp := l.RightPanel()
	if rp.Width() != 80 || rp.Height() != 13 {
		t.Fatalf("right panel: expected 80x13, got %dx%d", rp.Width(), rp.Height())
	}
}

func TestLayoutHorizontalZoomDetailSizing(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900) // height = 25
	l.AddSplit(podsPlugin(), "default", "")
	l.ShowRightPanel()
	l.ToggleOrientation()

	l.ToggleZoomDetail()

	// Detail fills entire screen regardless of orientation
	rp := l.RightPanel()
	if rp.Width() != 80 || rp.Height() != 26 {
		t.Fatalf("right panel: expected 80x26, got %dx%d", rp.Width(), rp.Height())
	}

	// Splits should be zero-sized
	s := l.SplitAt(0)
	if s.Width() != 0 || s.Height() != 0 {
		t.Fatalf("split should be 0x0 in detail zoom, got %dx%d", s.Width(), s.Height())
	}
}

func TestLayoutHorizontalOneSplitRemainder(t *testing.T) {
	// With 3 splits and width 80: 80/3=26, last split gets 80-26*2=28
	l := New(80, 26, 1000, "15m", 900) // height = 25
	l.AddSplit(podsPlugin(), "default", "")
	l.AddSplit(svcsPlugin(), "default", "")
	l.AddSplit(podsPlugin(), "default", "")
	l.ToggleOrientation()

	s0 := l.SplitAt(0)
	if s0.Width() != 26 {
		t.Fatalf("split 0 width: expected 26, got %d", s0.Width())
	}
	s2 := l.SplitAt(2)
	if s2.Width() != 28 {
		t.Fatalf("split 2 (last) width: expected 28, got %d", s2.Width())
	}
}

func TestLayoutHorizontalViewNotEmpty(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default", "")
	l.ToggleOrientation()

	view := l.View()
	if view == "" {
		t.Fatal("horizontal view should not be empty")
	}
}

func TestLayoutHorizontalViewWithRightPanel(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default", "")
	l.ShowRightPanel()
	l.ToggleOrientation()

	view := l.View()
	if view == "" {
		t.Fatal("horizontal view with right panel should not be empty")
	}
}

func TestLayoutHorizontalZoomSplitView(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default", "")
	l.AddSplit(svcsPlugin(), "default", "")
	l.ShowRightPanel()
	l.ToggleOrientation()
	l.ToggleZoomSplit()

	view := l.View()
	if view == "" {
		t.Fatal("horizontal zoom split view should not be empty")
	}
}

func TestPaneAtVerticalOneSplitWithDetail(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900) // content height = 25
	l.AddSplit(podsPlugin(), "default", "")
	l.ShowRightPanel()

	// Expect: split occupies x[0..40), detail x[40..80), both y[0..25).
	// Click inside the split pane.
	r, ok := l.PaneAt(5, 5)
	if !ok {
		t.Fatal("expected hit inside split pane")
	}
	if r.Kind != PaneSplit || r.SplitIdx != 0 {
		t.Fatalf("expected PaneSplit idx=0, got kind=%d idx=%d", r.Kind, r.SplitIdx)
	}

	// Click inside the detail pane.
	r, ok = l.PaneAt(50, 5)
	if !ok {
		t.Fatal("expected hit inside detail pane")
	}
	if r.Kind != PaneDetail {
		t.Fatalf("expected PaneDetail, got kind=%d", r.Kind)
	}
}

func TestPaneAtVerticalTwoSplitsWithDetail(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900) // content height = 25
	l.AddSplit(podsPlugin(), "default", "")
	l.AddSplit(svcsPlugin(), "default", "")
	l.ShowRightPanel()

	// Splits are stacked in vertical orientation: leftWidth=40, splitHeight=12
	// split 0: y[0..12), split 1: y[12..25) (last one takes remainder)
	r, ok := l.PaneAt(10, 3)
	if !ok || r.Kind != PaneSplit || r.SplitIdx != 0 {
		t.Fatalf("expected split 0, got ok=%v kind=%d idx=%d", ok, r.Kind, r.SplitIdx)
	}

	r, ok = l.PaneAt(10, 15)
	if !ok || r.Kind != PaneSplit || r.SplitIdx != 1 {
		t.Fatalf("expected split 1, got ok=%v kind=%d idx=%d", ok, r.Kind, r.SplitIdx)
	}

	r, ok = l.PaneAt(60, 10)
	if !ok || r.Kind != PaneDetail {
		t.Fatalf("expected detail, got ok=%v kind=%d", ok, r.Kind)
	}
}

func TestPaneAtHorizontalTwoSplitsWithDetail(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900) // content height = 25
	l.AddSplit(podsPlugin(), "default", "")
	l.AddSplit(svcsPlugin(), "default", "")
	l.ShowRightPanel()
	l.ToggleOrientation()

	// Horizontal: two splits side by side across full width (width=80),
	// topHeight = 12, bottomHeight = 13. Split 0 covers x[0..40), split 1
	// covers x[40..80). Detail spans full width at y[12..25).
	r, ok := l.PaneAt(10, 5)
	if !ok || r.Kind != PaneSplit || r.SplitIdx != 0 {
		t.Fatalf("expected split 0 at (10,5), got ok=%v kind=%d idx=%d", ok, r.Kind, r.SplitIdx)
	}
	r, ok = l.PaneAt(60, 5)
	if !ok || r.Kind != PaneSplit || r.SplitIdx != 1 {
		t.Fatalf("expected split 1 at (60,5), got ok=%v kind=%d idx=%d", ok, r.Kind, r.SplitIdx)
	}
	r, ok = l.PaneAt(10, 20)
	if !ok || r.Kind != PaneDetail {
		t.Fatalf("expected detail at (10,20), got ok=%v kind=%d", ok, r.Kind)
	}
	// Split-boundary edge: x=39 is the last column of split 0, x=40 is
	// the first column of split 1 (in 2-split evenly divided 80 width).
	r, ok = l.PaneAt(39, 5)
	if !ok || r.Kind != PaneSplit || r.SplitIdx != 0 {
		t.Fatalf("expected split 0 at x=39, got ok=%v kind=%d idx=%d", ok, r.Kind, r.SplitIdx)
	}
	r, ok = l.PaneAt(40, 5)
	if !ok || r.Kind != PaneSplit || r.SplitIdx != 1 {
		t.Fatalf("expected split 1 at x=40, got ok=%v kind=%d idx=%d", ok, r.Kind, r.SplitIdx)
	}
}

func TestPaneAtSplitZoomedDetailStillHittable(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default", "")
	l.AddSplit(svcsPlugin(), "default", "")
	l.ShowRightPanel()
	l.ToggleZoomSplit()

	// In ZoomSplit with right panel visible, the detail pane is still
	// rendered on the right side and must be hittable.
	r, ok := l.PaneAt(60, 5)
	if !ok {
		t.Fatal("expected a hit on right side during split-zoom with detail visible")
	}
	if r.Kind != PaneDetail {
		t.Fatalf("expected PaneDetail on right side during split-zoom, got kind=%d", r.Kind)
	}
}

func TestPaneAtHorizontalOneSplitWithDetail(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900) // content height = 25
	l.AddSplit(podsPlugin(), "default", "")
	l.ShowRightPanel()
	l.ToggleOrientation()

	// Horizontal: top = height*0.5 = 12, bottom = 13, full width.
	// Split at y[0..12), detail at y[12..25).
	r, ok := l.PaneAt(40, 5)
	if !ok || r.Kind != PaneSplit || r.SplitIdx != 0 {
		t.Fatalf("expected split, got ok=%v kind=%d idx=%d", ok, r.Kind, r.SplitIdx)
	}

	r, ok = l.PaneAt(40, 20)
	if !ok || r.Kind != PaneDetail {
		t.Fatalf("expected detail, got ok=%v kind=%d", ok, r.Kind)
	}
}

func TestPaneAtLogMode(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default", "")
	l.ShowRightPanel()
	l.SetLogMode(true)

	r, ok := l.PaneAt(60, 5)
	if !ok {
		t.Fatal("expected hit inside right panel")
	}
	if r.Kind != PaneLog {
		t.Fatalf("expected PaneLog, got kind=%d", r.Kind)
	}
}

func TestPaneAtSplitZoomed(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default", "")
	l.AddSplit(svcsPlugin(), "default", "")
	l.ShowRightPanel()

	l.ToggleZoomSplit() // focused is split 1

	// In ZoomSplit with right panel visible: the focused split fills the
	// left column and the detail panel remains visible on the right. Both
	// rects are emitted so clicks on the visible detail pane are still
	// routed correctly; only non-focused splits are hidden.
	// Click inside the zoomed split area — expect focused split's rect.
	r, ok := l.PaneAt(10, 10)
	if !ok {
		t.Fatal("expected hit inside zoomed split")
	}
	if r.Kind != PaneSplit || r.SplitIdx != 1 {
		t.Fatalf("expected PaneSplit idx=1, got kind=%d idx=%d", r.Kind, r.SplitIdx)
	}

	// No rect for non-focused split 0: clicking anywhere should hit either the
	// zoomed split or nothing (since it fills the whole left area).
	// Anywhere outside pane area (status bar row) → false.
	if _, ok := l.PaneAt(10, 25); ok {
		t.Fatal("status-bar row should not match any pane rect")
	}
}

func TestPaneAtDetailZoomed(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default", "")
	l.ShowRightPanel()

	l.ToggleZoomDetail()

	// Only detail rect. Any click in the pane area hits detail.
	r, ok := l.PaneAt(10, 10)
	if !ok {
		t.Fatal("expected hit inside zoomed detail")
	}
	if r.Kind != PaneDetail {
		t.Fatalf("expected PaneDetail, got kind=%d", r.Kind)
	}

	// A click far in what would normally be the split area — now detail.
	r, ok = l.PaneAt(1, 1)
	if !ok || r.Kind != PaneDetail {
		t.Fatalf("expected PaneDetail across whole area, got ok=%v kind=%d", ok, r.Kind)
	}

	// Status-bar row (y == 25) must not match, even though the detail panel
	// visually extends into it.
	if _, ok := l.PaneAt(10, 25); ok {
		t.Fatal("status-bar row should not match any pane rect")
	}
}

// TestPaneAtDetailZoomedWithoutRightPanel documents the interaction between
// ToggleZoomDetail and the right panel: calling ToggleZoomDetail without a
// visible right panel is a no-op (EffectiveZoom stays at ZoomNone), so PaneAt
// returns the standard vertical-split rects. This guards the invariant the
// rebuildPaneRects fallback branch relies on.
func TestPaneAtDetailZoomedWithoutRightPanel(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default", "")
	l.AddSplit(svcsPlugin(), "default", "")
	// Do NOT ShowRightPanel.

	l.ToggleZoomDetail()
	// ToggleZoomDetail is a no-op without the right panel visible, so
	// EffectiveZoom stays at ZoomNone and the splits render normally.
	if l.EffectiveZoom() != ZoomNone {
		t.Fatalf("expected EffectiveZoom ZoomNone without right panel, got %d", l.EffectiveZoom())
	}

	// Clicks at the top should hit split 0, and further down hit split 1.
	r0, ok := l.PaneAt(10, 2)
	if !ok {
		t.Fatal("expected hit near top of layout")
	}
	if r0.Kind != PaneSplit {
		t.Fatalf("expected PaneSplit, got kind=%d", r0.Kind)
	}
	// Status-bar row still excluded.
	if _, ok := l.PaneAt(10, 25); ok {
		t.Fatal("status-bar row should not match any pane rect")
	}
}

// TestPaneAtHorizontalSplitZoomed covers the ZoomSplit case when the
// orientation is horizontal — the zoomed split rect spans the full width and
// the detail pane occupies the bottom band.
func TestPaneAtHorizontalSplitZoomed(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default", "")
	l.AddSplit(svcsPlugin(), "default", "")
	l.ShowRightPanel()
	l.ToggleOrientation() // horizontal
	l.ToggleZoomSplit()   // zooms the focused split

	if l.EffectiveZoom() != ZoomSplit {
		t.Fatalf("expected ZoomSplit, got %d", l.EffectiveZoom())
	}

	// Top-left of the horizontal layout: should hit the focused split across
	// the full width.
	r, ok := l.PaneAt(5, 2)
	if !ok {
		t.Fatal("expected hit inside horizontally-zoomed split")
	}
	if r.Kind != PaneSplit || r.SplitIdx != l.FocusIndex() {
		t.Fatalf("expected PaneSplit idx=%d, got kind=%d idx=%d",
			l.FocusIndex(), r.Kind, r.SplitIdx)
	}
	// The full width should still route to the zoomed split at the same y.
	r2, ok := l.PaneAt(70, 2)
	if !ok || r2.Kind != PaneSplit || r2.SplitIdx != l.FocusIndex() {
		t.Fatalf("expected full-width zoomed split at right edge, got ok=%v kind=%d idx=%d",
			ok, r2.Kind, r2.SplitIdx)
	}
}

func TestPaneAtStatusBarRow(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900) // content height = 25, status-bar at y=25
	l.AddSplit(podsPlugin(), "default", "")
	l.ShowRightPanel()

	if _, ok := l.PaneAt(10, 25); ok {
		t.Fatal("click on status-bar row (y=25) should return false")
	}
	if _, ok := l.PaneAt(50, 25); ok {
		t.Fatal("click on status-bar row (y=25) should return false (detail side)")
	}
}

func TestPaneAtOutOfBounds(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default", "")
	l.ShowRightPanel()

	if _, ok := l.PaneAt(80, 5); ok {
		t.Fatal("x == width should be out of bounds")
	}
	if _, ok := l.PaneAt(100, 5); ok {
		t.Fatal("x > width should be out of bounds")
	}
	if _, ok := l.PaneAt(10, 30); ok {
		t.Fatal("y > height should be out of bounds")
	}
	if _, ok := l.PaneAt(-1, 5); ok {
		t.Fatal("negative x should be out of bounds")
	}
	if _, ok := l.PaneAt(10, -1); ok {
		t.Fatal("negative y should be out of bounds")
	}
}

func TestPaneAtRebuildsOnResize(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default", "")
	l.ShowRightPanel()

	// Resize to a smaller terminal — pane rects should reflect new geometry.
	l.Resize(40, 20)

	// leftWidth = 40*0.5 = 20; content height = 19
	r, ok := l.PaneAt(5, 5)
	if !ok || r.Kind != PaneSplit {
		t.Fatalf("expected split, got ok=%v kind=%d", ok, r.Kind)
	}
	r, ok = l.PaneAt(25, 5)
	if !ok || r.Kind != PaneDetail {
		t.Fatalf("expected detail, got ok=%v kind=%d", ok, r.Kind)
	}
	if _, ok := l.PaneAt(5, 19); ok {
		t.Fatal("status-bar row after resize (y=19) should not match")
	}
}

func TestLayoutToggleOrientationRecalcsSizes(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900) // height = 25
	l.AddSplit(podsPlugin(), "default", "")
	l.ShowRightPanel()

	// Vertical: split gets leftWidth=40, height=25
	s := l.FocusedSplit()
	if s.Width() != 40 || s.Height() != 25 {
		t.Fatalf("vertical: expected 40x25, got %dx%d", s.Width(), s.Height())
	}

	l.ToggleOrientation()

	// Horizontal: split gets full width=80, topHeight=12
	if s.Width() != 80 || s.Height() != 12 {
		t.Fatalf("horizontal: expected 80x12, got %dx%d", s.Width(), s.Height())
	}

	l.ToggleOrientation()

	// Back to vertical
	if s.Width() != 40 || s.Height() != 25 {
		t.Fatalf("back to vertical: expected 40x25, got %dx%d", s.Width(), s.Height())
	}
}
