package layout

import (
	"context"
	"testing"

	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/render"
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

	l.AddSplit(podsPlugin(), "default")
	if l.SplitCount() != 1 {
		t.Fatalf("expected 1 split, got %d", l.SplitCount())
	}

	l.AddSplit(svcsPlugin(), "default")
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

func TestLayoutCloseLastSplit(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default")
	shouldQuit := l.CloseCurrentSplit()
	if !shouldQuit {
		t.Fatal("closing last split should signal quit")
	}
}

func TestLayoutFocusCycling(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default")
	l.AddSplit(svcsPlugin(), "default")

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

func TestLayoutView(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default")
	view := l.View()
	if view == "" {
		t.Fatal("view should not be empty with a split")
	}
}

func TestLayoutViewWithRightPanel(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default")
	l.ShowRightPanel()
	view := l.View()
	if view == "" {
		t.Fatal("view should not be empty with splits and right panel")
	}
}

func TestUpdateSplitObjectsNamespaceFiltering(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default")
	l.AddSplit(podsPlugin(), "staging")

	objs := []*unstructured.Unstructured{
		{Object: map[string]any{"metadata": map[string]any{"name": "pod-a"}}},
	}

	// Update only "staging" namespace — split 0 ("default") should be empty
	l.UpdateSplitObjects(podsPlugin(), "staging", objs)

	if l.SplitAt(0).Len() != 0 {
		t.Fatal("split 0 (default) should not have received staging objects")
	}
	if l.SplitAt(1).Len() != 1 {
		t.Fatal("split 1 (staging) should have received objects")
	}
}

func TestAddSplitSetsNamespace(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "kube-system")
	if l.FocusedSplit().Namespace() != "kube-system" {
		t.Fatalf("expected namespace 'kube-system', got %q", l.FocusedSplit().Namespace())
	}
}

func TestLayoutZoomSplitToggle(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default")
	l.AddSplit(svcsPlugin(), "default")

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
	l.AddSplit(podsPlugin(), "default")
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
	l.AddSplit(podsPlugin(), "default")
	l.AddSplit(svcsPlugin(), "default")

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
	l.AddSplit(podsPlugin(), "default")
	l.AddSplit(svcsPlugin(), "default")

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
	l.AddSplit(podsPlugin(), "default")
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
	l.AddSplit(podsPlugin(), "default")
	l.AddSplit(svcsPlugin(), "default")

	l.ToggleZoomSplit()
	view := l.View()
	if view == "" {
		t.Fatal("zoomed view should not be empty")
	}
}

func TestLayoutZoomDetailView(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default")
	l.ShowRightPanel()

	l.ToggleZoomDetail()
	view := l.View()
	if view == "" {
		t.Fatal("detail zoomed view should not be empty")
	}
}

func TestLayoutZoomFollowsFocus(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)  // height = 25
	l.AddSplit(podsPlugin(), "default") // idx 0
	l.AddSplit(svcsPlugin(), "default") // idx 1 (focused)

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
	l.AddSplit(podsPlugin(), "default")
	l.AddSplit(svcsPlugin(), "default")

	l.ToggleZoomSplit()
	l.CloseCurrentSplit()

	// Only 1 split remains — zoom should auto-reset
	if l.EffectiveZoom() != ZoomNone {
		t.Fatalf("expected ZoomNone after close to 1 split, got %d", l.EffectiveZoom())
	}
}

func TestLayoutFocusTarget(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default")
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
	l.AddSplit(podsPlugin(), "default")

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
	l.AddSplit(podsPlugin(), "default")

	l.ToggleZoomSplit()
	if l.EffectiveZoom() != ZoomNone {
		t.Fatal("zoom should be no-op with single split")
	}
}

func TestLayoutIndependentZoomStates(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default")
	l.AddSplit(svcsPlugin(), "default")
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
	l.AddSplit(podsPlugin(), "default")
	l.AddSplit(svcsPlugin(), "default")
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
	l.AddSplit(podsPlugin(), "default")
	l.AddSplit(svcsPlugin(), "default")
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
	l.AddSplit(podsPlugin(), "default")
	l.AddSplit(svcsPlugin(), "default")
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
	l.AddSplit(podsPlugin(), "default")
	l.AddSplit(svcsPlugin(), "default")
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
	l.AddSplit(podsPlugin(), "default")

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
	l.UpdateSplitObjects(podsPlugin(), "default", allPods)

	// Drilldown split should NOT have been updated
	if l.SplitAt(0).Len() != 1 {
		t.Fatalf("drilldown split should still have 1 object, got %d", l.SplitAt(0).Len())
	}
}

func TestLayoutHideRightPanelClearsDetailZoom(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default")
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
	l.AddSplit(podsPlugin(), "default")
	l.AddSplit(svcsPlugin(), "default")
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
	l.AddSplit(podsPlugin(), "default")
	l.AddSplit(svcsPlugin(), "default")
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
	l.AddSplit(podsPlugin(), "default")
	l.AddSplit(svcsPlugin(), "default")
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
	l.AddSplit(podsPlugin(), "default")
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
	l.AddSplit(podsPlugin(), "default")
	l.AddSplit(svcsPlugin(), "default")
	l.AddSplit(podsPlugin(), "default")
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
	l.AddSplit(podsPlugin(), "default")
	l.ToggleOrientation()

	view := l.View()
	if view == "" {
		t.Fatal("horizontal view should not be empty")
	}
}

func TestLayoutHorizontalViewWithRightPanel(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default")
	l.ShowRightPanel()
	l.ToggleOrientation()

	view := l.View()
	if view == "" {
		t.Fatal("horizontal view with right panel should not be empty")
	}
}

func TestLayoutHorizontalZoomSplitView(t *testing.T) {
	l := New(80, 26, 1000, "15m", 900)
	l.AddSplit(podsPlugin(), "default")
	l.AddSplit(svcsPlugin(), "default")
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
	l.AddSplit(podsPlugin(), "default")
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
	l.AddSplit(podsPlugin(), "default")
	l.AddSplit(svcsPlugin(), "default")
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
	l.AddSplit(podsPlugin(), "default")
	l.AddSplit(svcsPlugin(), "default")
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
	l.AddSplit(podsPlugin(), "default")
	l.AddSplit(svcsPlugin(), "default")
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
	l.AddSplit(podsPlugin(), "default")
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
	l.AddSplit(podsPlugin(), "default")
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
	l.AddSplit(podsPlugin(), "default")
	l.AddSplit(svcsPlugin(), "default")
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
	l.AddSplit(podsPlugin(), "default")
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
	l.AddSplit(podsPlugin(), "default")
	l.AddSplit(svcsPlugin(), "default")
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
	l.AddSplit(podsPlugin(), "default")
	l.AddSplit(svcsPlugin(), "default")
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
	l.AddSplit(podsPlugin(), "default")
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
	l.AddSplit(podsPlugin(), "default")
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
	l.AddSplit(podsPlugin(), "default")
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
	l.AddSplit(podsPlugin(), "default")
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
