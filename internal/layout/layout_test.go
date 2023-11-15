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
func (m *mockPlugin) YAML(_ *unstructured.Unstructured) (render.Content, error) { return render.Content{}, nil }
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
	l := New(80, 26, 1000)
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
	l := New(80, 26, 1000)
	l.AddSplit(podsPlugin(), "default")
	shouldQuit := l.CloseCurrentSplit()
	if !shouldQuit {
		t.Fatal("closing last split should signal quit")
	}
}

func TestLayoutFocusCycling(t *testing.T) {
	l := New(80, 26, 1000)
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
	l := New(80, 26, 1000)
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
	l := New(80, 26, 1000)
	if l.FocusedSplit() != nil {
		t.Fatal("FocusedSplit should be nil with no splits")
	}
}

func TestLayoutView(t *testing.T) {
	l := New(80, 26, 1000)
	l.AddSplit(podsPlugin(), "default")
	view := l.View()
	if view == "" {
		t.Fatal("view should not be empty with a split")
	}
}

func TestLayoutViewWithRightPanel(t *testing.T) {
	l := New(80, 26, 1000)
	l.AddSplit(podsPlugin(), "default")
	l.ShowRightPanel()
	view := l.View()
	if view == "" {
		t.Fatal("view should not be empty with splits and right panel")
	}
}

func TestUpdateSplitObjectsNamespaceFiltering(t *testing.T) {
	l := New(80, 26, 1000)
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
	l := New(80, 26, 1000)
	l.AddSplit(podsPlugin(), "kube-system")
	if l.FocusedSplit().Namespace() != "kube-system" {
		t.Fatalf("expected namespace 'kube-system', got %q", l.FocusedSplit().Namespace())
	}
}

func TestLayoutZoomSplitToggle(t *testing.T) {
	l := New(80, 26, 1000)
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
	l := New(80, 26, 1000)
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
	l := New(80, 26, 1000)
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
	l := New(80, 26, 1000) // height = 26-1=25
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
	l := New(80, 26, 1000) // height = 25
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
	l := New(80, 26, 1000)
	l.AddSplit(podsPlugin(), "default")
	l.AddSplit(svcsPlugin(), "default")

	l.ToggleZoomSplit()
	view := l.View()
	if view == "" {
		t.Fatal("zoomed view should not be empty")
	}
}

func TestLayoutZoomDetailView(t *testing.T) {
	l := New(80, 26, 1000)
	l.AddSplit(podsPlugin(), "default")
	l.ShowRightPanel()

	l.ToggleZoomDetail()
	view := l.View()
	if view == "" {
		t.Fatal("detail zoomed view should not be empty")
	}
}

func TestLayoutZoomFollowsFocus(t *testing.T) {
	l := New(80, 26, 1000) // height = 25
	l.AddSplit(podsPlugin(), "default")  // idx 0
	l.AddSplit(svcsPlugin(), "default")  // idx 1 (focused)

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
	l := New(80, 26, 1000)
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
	l := New(80, 26, 1000)
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
	l := New(80, 26, 1000)
	l.AddSplit(podsPlugin(), "default")

	l.FocusDetails()
	if l.FocusedDetails() {
		t.Fatal("FocusDetails should be no-op when right panel is hidden")
	}
}

func TestLayoutFocusResourcesSafeWithNoSplits(t *testing.T) {
	l := New(80, 26, 1000)

	l.FocusResources()
	if !l.FocusedResources() {
		t.Fatal("should be in resources mode")
	}
}

func TestLayoutZoomSplitNoopWithOneSplit(t *testing.T) {
	l := New(80, 26, 1000)
	l.AddSplit(podsPlugin(), "default")

	l.ToggleZoomSplit()
	if l.EffectiveZoom() != ZoomNone {
		t.Fatal("zoom should be no-op with single split")
	}
}

func TestLayoutIndependentZoomStates(t *testing.T) {
	l := New(80, 26, 1000)
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
	l := New(80, 26, 1000)
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
	l := New(80, 26, 1000)
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
	l := New(80, 26, 1000)
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
	l := New(80, 26, 1000)
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
	l := New(80, 26, 1000)
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
	l := New(80, 26, 1000)
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
