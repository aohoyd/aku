package app

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/aohoyd/aku/internal/config"
	"github.com/aohoyd/aku/internal/layout"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/render"
	"github.com/aohoyd/aku/internal/ui"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// sendWindowSize drives the real resize path used at runtime, so the test
// state matches what Update would produce for a WindowSizeMsg. It is a thin
// wrapper kept for readability at call sites.
func sendWindowSize(a App, w, h int) App {
	model, _ := a.update(tea.WindowSizeMsg{Width: w, Height: h})
	return model.(App)
}

// TestOverlayRectClearedByDefault verifies the rect starts zeroed (no overlay).
func TestOverlayRectClearedByDefault(t *testing.T) {
	app := newTestApp()
	app = sendWindowSize(app, 120, 40)

	// Render once with no active overlay.
	_ = app.View()

	got := app.OverlayRect()
	if got.W != 0 || got.H != 0 || got.X != 0 || got.Y != 0 {
		t.Fatalf("expected zero rect when no overlay active, got %+v", got)
	}
}

// TestOverlayRectPopulatedWhenActive verifies the rect is non-zero after
// opening an overlay and rendering.
func TestOverlayRectPopulatedWhenActive(t *testing.T) {
	app := newTestApp()
	app = sendWindowSize(app, 120, 40)

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	plugin.Register(podsPlugin)
	app.layout.AddSplit(podsPlugin, "default")

	// Open the help overlay.
	model, _ := app.executeCommand("help")
	app = model.(App)
	if app.activeOverlay != overlayHelp {
		t.Fatalf("expected activeOverlay=overlayHelp, got %v", app.activeOverlay)
	}

	// Render to populate the rect.
	_ = app.View()

	got := app.OverlayRect()
	if got.W <= 0 || got.H <= 0 {
		t.Fatalf("expected populated rect with positive W and H, got %+v", got)
	}
	if got.X < 0 || got.Y < 0 {
		t.Fatalf("expected non-negative X, Y, got %+v", got)
	}
	// Overlay must fit within the terminal bounds.
	if got.X+got.W > 120 {
		t.Fatalf("overlay rect right edge %d exceeds terminal width 120 (%+v)", got.X+got.W, got)
	}
	if got.Y+got.H > 40 {
		t.Fatalf("overlay rect bottom edge %d exceeds terminal height 40 (%+v)", got.Y+got.H, got)
	}
}

// TestOverlayRectClearedOnClose verifies that closing an overlay clears the rect
// on the next render.
func TestOverlayRectClearedOnClose(t *testing.T) {
	app := newTestApp()
	app = sendWindowSize(app, 120, 40)

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	plugin.Register(podsPlugin)
	app.layout.AddSplit(podsPlugin, "default")

	// Open the help overlay and render to populate the rect.
	model, _ := app.executeCommand("help")
	app = model.(App)
	_ = app.View()

	if app.OverlayRect().W == 0 {
		t.Fatalf("expected non-zero rect after overlay open")
	}

	// Close the overlay by setting activeOverlay back to none (same terminal
	// state the ctrl+w handler leaves the app in). Direct mutation is used
	// instead of a KeyPressMsg because this test only needs the post-close
	// rect-clear to be verifiable — the ctrl+w dispatch path is covered by
	// the key-handling tests in app_test.go.
	app.activeOverlay = overlayNone
	_ = app.View()

	got := app.OverlayRect()
	if got.W != 0 || got.H != 0 || got.X != 0 || got.Y != 0 {
		t.Fatalf("expected zero rect after close, got %+v", got)
	}
}

// TestOverlayRectUpdatesOnSwitch verifies that switching from one overlay to
// another updates the stored rect to match the newly-active overlay.
func TestOverlayRectUpdatesOnSwitch(t *testing.T) {
	app := newTestApp()
	app = sendWindowSize(app, 120, 40)

	podsPlugin := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	plugin.Register(podsPlugin)
	app.layout.AddSplit(podsPlugin, "default")

	// Open help overlay.
	model, _ := app.executeCommand("help")
	app = model.(App)
	_ = app.View()
	rectHelp := app.OverlayRect()
	if rectHelp.W == 0 {
		t.Fatalf("expected non-zero rect for help overlay")
	}

	// Switch to a different overlay (resource picker) without rendering in
	// between. The View() call should refresh the stored rect to the new
	// overlay's dimensions.
	app.activeOverlay = overlayNone
	model, _ = app.executeCommand("resource-picker")
	app = model.(App)
	if app.activeOverlay != overlayResourcePicker {
		t.Fatalf("resource-picker command did not open resource picker (activeOverlay=%v)", app.activeOverlay)
	}
	_ = app.View()
	rectPicker := app.OverlayRect()
	if rectPicker.W == 0 {
		t.Fatalf("expected non-zero rect for resource picker")
	}
	// The help overlay uses the full terminal size (120x40) while the
	// resource picker is bounded at 80x30. They must differ in at least
	// one dimension after the switch.
	if rectPicker == rectHelp {
		t.Fatalf("expected rect to change when switching overlays; both were %+v", rectPicker)
	}
}

// TestOverlayRectZeroBeforeFirstRender ensures OverlayRect() is safe to call
// before View() has been invoked (pointer is initialized in New()).
func TestOverlayRectZeroBeforeFirstRender(t *testing.T) {
	app := newTestApp()
	got := app.OverlayRect()
	if got.W != 0 || got.H != 0 || got.X != 0 || got.Y != 0 {
		t.Fatalf("expected zero rect before first render, got %+v", got)
	}
}

// makeWheelApp creates a test App with two splits, each populated with enough
// objects that the cursor can move. Returns the app and the two plugins used.
func makeWheelApp(t *testing.T) App {
	t.Helper()
	app := newTestApp()
	// Mouse handlers gate on this; tests bypass the bubbletea MouseMode path.
	app.config.Mouse.Enabled = true
	app = sendWindowSize(app, 120, 40)

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

	app.layout.AddSplit(podsPlugin, "default")
	app.layout.AddSplit(svcsPlugin, "default")
	app.layout.FocusSplitAt(0)

	// Populate each split with objects so cursor movement is observable.
	for idx := 0; idx < app.layout.SplitCount(); idx++ {
		split := app.layout.SplitAt(idx)
		objs := make([]*unstructured.Unstructured, 10)
		for i := range objs {
			obj := &unstructured.Unstructured{}
			obj.SetName(fmt.Sprintf("item-%d-%02d", idx, i))
			objs[i] = obj
		}
		split.SetObjects(objs)
	}

	// Re-apply window size so the layout sizes everything based on the new splits.
	app = sendWindowSize(app, 120, 40)
	return app
}

// TestMouseWheelOnFocusedSplitScrollsCursor verifies wheel-down over the
// focused split advances its cursor.
func TestMouseWheelOnFocusedSplitScrollsCursor(t *testing.T) {
	app := makeWheelApp(t)

	// Split 0 is focused. Its rect starts at y=0. Click inside a body row.
	rect, ok := app.layout.PaneAt(5, 3)
	if !ok || rect.SplitIdx != 0 {
		t.Fatalf("expected hit in split 0, got ok=%v idx=%d", ok, rect.SplitIdx)
	}
	focused := app.layout.FocusedSplit()
	before := focused.Cursor()

	model, _ := app.update(tea.MouseWheelMsg{X: 5, Y: 3, Button: tea.MouseWheelDown})
	app = model.(App)

	after := app.layout.SplitAt(0).Cursor()
	if after != before+1 {
		t.Fatalf("expected focused split cursor to advance from %d to %d, got %d", before, before+1, after)
	}
	if app.layout.FocusIndex() != 0 {
		t.Fatalf("focus index must not change, got %d", app.layout.FocusIndex())
	}
}

// TestMouseWheelOnUnfocusedSplitDoesNotChangeFocus verifies that a wheel event
// over the non-focused split moves that split's cursor but leaves focus alone.
func TestMouseWheelOnUnfocusedSplitDoesNotChangeFocus(t *testing.T) {
	app := makeWheelApp(t)

	// Find coordinates that hit split 1 (below split 0 in vertical orientation).
	var split1Y int
	found := false
	for y := 0; y < 25; y++ {
		r, ok := app.layout.PaneAt(5, y)
		if ok && r.Kind == layout.PaneSplit && r.SplitIdx == 1 {
			split1Y = y + 1 // skip header/border
			found = true
			break
		}
	}
	if !found {
		t.Fatal("could not locate split 1 in the layout")
	}
	// Confirm the hit test at split1Y actually returns split 1.
	r, ok := app.layout.PaneAt(5, split1Y)
	if !ok || r.SplitIdx != 1 {
		t.Fatalf("expected hit in split 1 at y=%d, got ok=%v idx=%d", split1Y, ok, r.SplitIdx)
	}

	cursor0Before := app.layout.SplitAt(0).Cursor()
	cursor1Before := app.layout.SplitAt(1).Cursor()

	model, _ := app.update(tea.MouseWheelMsg{X: 5, Y: split1Y, Button: tea.MouseWheelDown})
	app = model.(App)

	if app.layout.FocusIndex() != 0 {
		t.Fatalf("focus must remain on split 0, got %d", app.layout.FocusIndex())
	}
	if app.layout.SplitAt(0).Cursor() != cursor0Before {
		t.Fatalf("unfocused-split wheel must not move split 0 cursor")
	}
	if app.layout.SplitAt(1).Cursor() != cursor1Before+1 {
		t.Fatalf("expected split 1 cursor to advance to %d, got %d", cursor1Before+1, app.layout.SplitAt(1).Cursor())
	}
}

// TestMouseWheelOutsideAllPanesIsNoOp confirms wheel events that miss every
// pane (status-bar row, outside bounds) do not change any state.
func TestMouseWheelOutsideAllPanesIsNoOp(t *testing.T) {
	app := makeWheelApp(t)

	cursor0 := app.layout.SplitAt(0).Cursor()
	cursor1 := app.layout.SplitAt(1).Cursor()
	focus := app.layout.FocusIndex()

	// The status-bar row is the last terminal row; content occupies 0..height-2.
	// Derive it from app.height so the test tracks future layout changes.
	statusY := app.height - 1
	if _, ok := app.layout.PaneAt(10, statusY); ok {
		t.Fatalf("precondition: expected no pane at status-bar row y=%d", statusY)
	}
	model, _ := app.update(tea.MouseWheelMsg{X: 10, Y: statusY, Button: tea.MouseWheelDown})
	app = model.(App)
	// Way off-screen.
	model, _ = app.update(tea.MouseWheelMsg{X: 500, Y: 500, Button: tea.MouseWheelDown})
	app = model.(App)

	if app.layout.FocusIndex() != focus {
		t.Fatalf("focus changed on out-of-bounds wheel: want %d got %d", focus, app.layout.FocusIndex())
	}
	if app.layout.SplitAt(0).Cursor() != cursor0 {
		t.Fatalf("split 0 cursor changed unexpectedly")
	}
	if app.layout.SplitAt(1).Cursor() != cursor1 {
		t.Fatalf("split 1 cursor changed unexpectedly")
	}
}

// TestMouseWheelInsideOverlayScrollsOverlay verifies wheel events inside the
// overlay rect advance the overlay's selection.
func TestMouseWheelInsideOverlayScrollsOverlay(t *testing.T) {
	app := makeWheelApp(t)

	// Open the ns picker with some namespaces and render so overlayRect
	// is populated.
	app.activeOverlay = overlayNsPicker
	app.nsPicker.Open()
	app.nsPicker.SetNamespaces([]string{"ns-a", "ns-b", "ns-c", "ns-d"})
	_ = app.View()

	rect := app.OverlayRect()
	if rect.W == 0 || rect.H == 0 {
		t.Fatalf("expected non-zero overlay rect, got %+v", rect)
	}

	before := app.nsPicker.Cursor()
	// Click inside the overlay rect.
	model, _ := app.update(tea.MouseWheelMsg{
		X:      rect.X + rect.W/2,
		Y:      rect.Y + rect.H/2,
		Button: tea.MouseWheelDown,
	})
	app = model.(App)

	after := app.nsPicker.Cursor()
	if after != before+1 {
		t.Fatalf("expected overlay cursor to advance from %d to %d, got %d", before, before+1, after)
	}
}

// TestMouseWheelRoutesToActiveOverlayPicker verifies that a single wheel-down
// inside the overlay rect advances the cursor of each picker overlay kind.
// The ns picker case is covered by TestMouseWheelInsideOverlayScrollsOverlay;
// these exercise the resource/container/time-range dispatch paths.
func TestMouseWheelRoutesToActiveOverlayPicker(t *testing.T) {
	cases := []struct {
		name    string
		openFn  func(a *App)
		cursor  func(a App) int
	}{
		{
			name: "resourcePicker",
			openFn: func(a *App) {
				a.activeOverlay = overlayResourcePicker
				a.resourcePicker.SetPlugins([]ui.PluginEntry{
					{Name: "pods", ShortName: "po"},
					{Name: "services", ShortName: "svc"},
					{Name: "deployments", ShortName: "deploy"},
				})
				a.resourcePicker.Open()
			},
			cursor: func(a App) int { return a.resourcePicker.Cursor() },
		},
		{
			name: "containerPicker",
			openFn: func(a *App) {
				a.activeOverlay = overlayContainerPicker
				a.containerPicker.SetContainers([]string{"app", "sidecar", "debug"})
				a.containerPicker.Open()
			},
			cursor: func(a App) int { return a.containerPicker.Cursor() },
		},
		{
			name: "timeRangePicker",
			openFn: func(a *App) {
				a.activeOverlay = overlayTimeRange
				a.timeRangePicker.OpenPresets()
			},
			cursor: func(a App) int { return a.timeRangePicker.Cursor() },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := makeWheelApp(t)
			tc.openFn(&app)
			_ = app.View()

			rect := app.OverlayRect()
			if rect.W == 0 || rect.H == 0 {
				t.Fatalf("expected non-zero overlay rect, got %+v", rect)
			}

			before := tc.cursor(app)
			model, _ := app.update(tea.MouseWheelMsg{
				X:      rect.X + rect.W/2,
				Y:      rect.Y + rect.H/2,
				Button: tea.MouseWheelDown,
			})
			app = model.(App)

			after := tc.cursor(app)
			if after != before+1 {
				t.Fatalf("%s: expected cursor to advance from %d to %d, got %d",
					tc.name, before, before+1, after)
			}
		})
	}
}

// TestMouseWheelOutsideOverlayIsNoOp verifies wheel events outside the overlay
// rect (with an overlay active) are dropped — no split or overlay state moves.
func TestMouseWheelOutsideOverlayIsNoOp(t *testing.T) {
	app := makeWheelApp(t)

	app.activeOverlay = overlayNsPicker
	app.nsPicker.Open()
	app.nsPicker.SetNamespaces([]string{"ns-a", "ns-b", "ns-c", "ns-d"})
	_ = app.View()

	pickerBefore := app.nsPicker.Cursor()
	splitBefore := app.layout.SplitAt(0).Cursor()

	// Click at (0,0): guaranteed outside the centered overlay rect.
	model, _ := app.update(tea.MouseWheelMsg{X: 0, Y: 0, Button: tea.MouseWheelDown})
	app = model.(App)

	if app.nsPicker.Cursor() != pickerBefore {
		t.Fatalf("overlay cursor moved despite click outside rect: before=%d after=%d", pickerBefore, app.nsPicker.Cursor())
	}
	if app.layout.SplitAt(0).Cursor() != splitBefore {
		t.Fatalf("split cursor moved while overlay was active: before=%d after=%d", splitBefore, app.layout.SplitAt(0).Cursor())
	}
}

// TestMouseClickOnSplitFocusesAndSelectsRow verifies a left click on a split
// focuses that split and moves the cursor to the clicked row.
func TestMouseClickOnSplitFocusesAndSelectsRow(t *testing.T) {
	app := makeWheelApp(t)

	// Focus split 1 first so we can verify that clicking on split 0 switches.
	app.layout.FocusSplitAt(1)
	if app.layout.FocusIndex() != 1 {
		t.Fatalf("precondition: expected focus on split 1, got %d", app.layout.FocusIndex())
	}

	// Find a body row inside split 0 — probe y until we hit a row >= 0.
	rect0, ok := app.layout.PaneAt(5, 0)
	if !ok || rect0.SplitIdx != 0 {
		t.Fatalf("expected split 0 at y=0, got ok=%v idx=%d", ok, rect0.SplitIdx)
	}
	var targetY, wantRow int
	found := false
	for y := rect0.Y; y < rect0.Y+rect0.H; y++ {
		split := app.layout.SplitAt(0)
		if split == nil {
			t.Fatal("split 0 is nil")
		}
		if row := split.RowAtY(y - rect0.Y); row == 3 {
			targetY = y
			wantRow = row
			found = true
			break
		}
	}
	if !found {
		t.Fatal("could not locate y coordinate corresponding to row 3 in split 0")
	}

	model, _ := app.update(tea.MouseClickMsg{X: 5, Y: targetY, Button: tea.MouseLeft})
	app = model.(App)

	if app.layout.FocusIndex() != 0 {
		t.Fatalf("expected focus on split 0 after click, got %d", app.layout.FocusIndex())
	}
	if !app.layout.FocusedResources() {
		t.Fatal("expected focus target to be resources after click on split")
	}
	if got := app.layout.SplitAt(0).Cursor(); got != wantRow {
		t.Fatalf("expected split 0 cursor at row %d, got %d", wantRow, got)
	}
}

// TestMouseClickOnDifferentSplitSwitchesFocus verifies clicking split 1 while
// split 0 is focused moves focus to split 1.
func TestMouseClickOnDifferentSplitSwitchesFocus(t *testing.T) {
	app := makeWheelApp(t)
	if app.layout.FocusIndex() != 0 {
		t.Fatalf("precondition: expected focus on split 0, got %d", app.layout.FocusIndex())
	}

	// Pre-move split 1's cursor so an incidental reset by the click handler
	// would be detectable. Then walk rows until we find a y that maps to a
	// concrete data row in split 1 — we verify both focus and cursor land.
	rect1Start, ok := findFirstY(app, layout.PaneSplit, 1)
	if !ok {
		t.Fatal("could not locate split 1 in layout")
	}
	rect1, _ := app.layout.PaneAt(5, rect1Start)
	split1 := app.layout.SplitAt(1)
	var targetY, wantRow int
	foundRow := false
	for y := rect1.Y; y < rect1.Y+rect1.H; y++ {
		if row := split1.RowAtY(y - rect1.Y); row == 2 {
			targetY = y
			wantRow = row
			foundRow = true
			break
		}
	}
	if !foundRow {
		t.Fatal("could not locate row 2 y in split 1")
	}

	model, _ := app.update(tea.MouseClickMsg{X: 5, Y: targetY, Button: tea.MouseLeft})
	app = model.(App)

	if app.layout.FocusIndex() != 1 {
		t.Fatalf("expected focus on split 1 after click, got %d", app.layout.FocusIndex())
	}
	if got := app.layout.SplitAt(1).Cursor(); got != wantRow {
		t.Fatalf("expected split 1 cursor at row %d, got %d", wantRow, got)
	}
}

// findFirstY scans column x=5 until it finds the first y whose PaneAt entry
// matches the requested kind and split index. Returns the y and true on hit.
func findFirstY(app App, kind layout.PaneKind, splitIdx int) (int, bool) {
	for y := 0; y < app.height; y++ {
		r, ok := app.layout.PaneAt(5, y)
		if ok && r.Kind == kind && (kind != layout.PaneSplit || r.SplitIdx == splitIdx) {
			return y, true
		}
	}
	return 0, false
}

// TestMouseClickOnHeaderRowLeavesCursorUnchanged verifies a click that lands
// on the split's border/header row focuses the split but does not change the
// cursor position.
func TestMouseClickOnHeaderRowLeavesCursorUnchanged(t *testing.T) {
	app := makeWheelApp(t)

	// Focus split 1 first so clicking on split 0's header row is a focus change.
	app.layout.FocusSplitAt(1)
	// Move split 0's cursor somewhere non-zero so a reset would be detectable.
	app.layout.SplitAt(0).SetCursor(5)
	cursorBefore := app.layout.SplitAt(0).Cursor()

	// Click on split 0's header row (rect.Y + 0 — border/title chrome).
	rect0, ok := app.layout.PaneAt(5, 0)
	if !ok || rect0.SplitIdx != 0 {
		t.Fatalf("expected split 0 at y=0, got ok=%v idx=%d", ok, rect0.SplitIdx)
	}
	// rect0.Y is the top border; RowAtY(0) should return -1.
	split := app.layout.SplitAt(0)
	if split.RowAtY(0) != -1 {
		t.Fatalf("precondition: expected RowAtY(0) to be -1 (chrome), got %d", split.RowAtY(0))
	}

	model, _ := app.update(tea.MouseClickMsg{X: 5, Y: rect0.Y, Button: tea.MouseLeft})
	app = model.(App)

	if app.layout.FocusIndex() != 0 {
		t.Fatalf("expected focus to move to split 0 on header click, got %d", app.layout.FocusIndex())
	}
	if got := app.layout.SplitAt(0).Cursor(); got != cursorBefore {
		t.Fatalf("expected cursor unchanged on header click; before=%d after=%d", cursorBefore, got)
	}
}

// TestMouseClickOnDetailFlipsFocusTarget verifies a click on the detail pane
// moves focusTarget to details without affecting the list cursor.
func TestMouseClickOnDetailFlipsFocusTarget(t *testing.T) {
	app := makeWheelApp(t)

	// Open the right panel so the detail pane exists.
	app.layout.ShowRightPanel()
	app = sendWindowSize(app, 120, 40)

	// Find detail pane rectangle.
	detailX, detailY := findDetailCoord(t, app)
	detailRect, ok := app.layout.PaneAt(detailX, detailY)
	if !ok {
		t.Fatal("could not locate detail pane rect")
	}

	cursorBefore := app.layout.SplitAt(0).Cursor()

	model, _ := app.update(tea.MouseClickMsg{
		X:      detailRect.X + detailRect.W/2,
		Y:      detailRect.Y + detailRect.H/2,
		Button: tea.MouseLeft,
	})
	app = model.(App)

	if !app.layout.FocusedDetails() {
		t.Fatal("expected focus target to be details after click on detail pane")
	}
	if got := app.layout.SplitAt(0).Cursor(); got != cursorBefore {
		t.Fatalf("expected split 0 cursor unchanged on detail click; before=%d after=%d", cursorBefore, got)
	}
}

// TestMouseClickWithOverlayActiveIsNoOp verifies a click does not change focus,
// move cursor, close an active overlay, or mutate the lastClick* double-click
// bookkeeping.
func TestMouseClickWithOverlayActiveIsNoOp(t *testing.T) {
	app := makeWheelApp(t)

	app.activeOverlay = overlayNsPicker
	app.nsPicker.Open()
	app.nsPicker.SetNamespaces([]string{"ns-a", "ns-b"})
	// Render once so overlayRect is populated — matches real runtime order
	// where View() always precedes the first mouse event.
	_ = app.View()

	focusBefore := app.layout.FocusIndex()
	cursorBefore := app.layout.SplitAt(0).Cursor()
	overlayBefore := app.activeOverlay
	lastClickTimeBefore := app.lastClickTime
	lastClickRowBefore := app.lastClickRow
	lastClickSplitBefore := app.lastClickSplit
	lastClickKindBefore := app.lastClickKind

	// Click anywhere — inside a split region.
	model, _ := app.update(tea.MouseClickMsg{X: 5, Y: 3, Button: tea.MouseLeft})
	app = model.(App)

	if app.activeOverlay != overlayBefore {
		t.Fatalf("overlay changed on click: before=%v after=%v", overlayBefore, app.activeOverlay)
	}
	if app.layout.FocusIndex() != focusBefore {
		t.Fatalf("focus moved on click under overlay: before=%d after=%d", focusBefore, app.layout.FocusIndex())
	}
	if app.layout.SplitAt(0).Cursor() != cursorBefore {
		t.Fatalf("cursor moved on click under overlay")
	}
	if !app.lastClickTime.Equal(lastClickTimeBefore) {
		t.Fatalf("lastClickTime changed: before=%v after=%v", lastClickTimeBefore, app.lastClickTime)
	}
	if app.lastClickRow != lastClickRowBefore {
		t.Fatalf("lastClickRow changed: before=%d after=%d", lastClickRowBefore, app.lastClickRow)
	}
	if app.lastClickSplit != lastClickSplitBefore {
		t.Fatalf("lastClickSplit changed: before=%d after=%d", lastClickSplitBefore, app.lastClickSplit)
	}
	if app.lastClickKind != lastClickKindBefore {
		t.Fatalf("lastClickKind changed: before=%v after=%v", lastClickKindBefore, app.lastClickKind)
	}
}

// TestMouseClickNonLeftButtonIsNoOp verifies that right/middle button clicks
// are ignored entirely.
func TestMouseClickNonLeftButtonIsNoOp(t *testing.T) {
	app := makeWheelApp(t)

	// Set split 1 focused so a spurious switch to split 0 would be detectable.
	app.layout.FocusSplitAt(1)
	focusBefore := app.layout.FocusIndex()
	cursor1Before := app.layout.SplitAt(1).Cursor()

	model, _ := app.update(tea.MouseClickMsg{X: 5, Y: 3, Button: tea.MouseRight})
	app = model.(App)

	if app.layout.FocusIndex() != focusBefore {
		t.Fatalf("focus changed on right-click: before=%d after=%d", focusBefore, app.layout.FocusIndex())
	}
	if app.layout.SplitAt(1).Cursor() != cursor1Before {
		t.Fatal("cursor changed on right-click")
	}

	// Also test middle button.
	model, _ = app.update(tea.MouseClickMsg{X: 5, Y: 3, Button: tea.MouseMiddle})
	app = model.(App)

	if app.layout.FocusIndex() != focusBefore {
		t.Fatalf("focus changed on middle-click: before=%d after=%d", focusBefore, app.layout.FocusIndex())
	}
}

// TestMouseClickOutsideAllPanesIsNoOp verifies that clicks that miss every
// pane (status bar, out of bounds) are dropped.
func TestMouseClickOutsideAllPanesIsNoOp(t *testing.T) {
	app := makeWheelApp(t)

	app.layout.FocusSplitAt(1)
	focusBefore := app.layout.FocusIndex()
	cursor0 := app.layout.SplitAt(0).Cursor()
	cursor1 := app.layout.SplitAt(1).Cursor()

	// Status-bar row, derived from app.height so the test tracks layout changes.
	statusY := app.height - 1
	model, _ := app.update(tea.MouseClickMsg{X: 10, Y: statusY, Button: tea.MouseLeft})
	app = model.(App)
	// Way off-screen.
	model, _ = app.update(tea.MouseClickMsg{X: 500, Y: 500, Button: tea.MouseLeft})
	app = model.(App)

	if app.layout.FocusIndex() != focusBefore {
		t.Fatalf("focus changed on out-of-bounds click: want %d got %d", focusBefore, app.layout.FocusIndex())
	}
	if app.layout.SplitAt(0).Cursor() != cursor0 {
		t.Fatal("split 0 cursor changed on out-of-bounds click")
	}
	if app.layout.SplitAt(1).Cursor() != cursor1 {
		t.Fatal("split 1 cursor changed on out-of-bounds click")
	}
}

// findDetailCoord returns the top-left corner of the detail pane in the
// given app, or fails the test if no detail rect is currently visible.
func findDetailCoord(t *testing.T, a App) (int, int) {
	t.Helper()
	for y := 0; y < a.height; y++ {
		for x := 0; x < a.width; x++ {
			r, ok := a.layout.PaneAt(x, y)
			if ok && r.Kind == layout.PaneDetail {
				return x, y
			}
		}
	}
	t.Fatal("could not locate detail pane")
	return 0, 0
}

// findLogCoord returns the centre of the log pane in the given app, or fails
// the test if no log rect is currently visible.
func findLogCoord(t *testing.T, a App) (int, int) {
	t.Helper()
	for y := 0; y < a.height; y++ {
		for x := 0; x < a.width; x++ {
			r, ok := a.layout.PaneAt(x, y)
			if ok && r.Kind == layout.PaneLog {
				return x + r.W/2, y + r.H/2
			}
		}
	}
	t.Fatal("could not locate log pane")
	return 0, 0
}

// mouseDoubleClickHarness wraps a test App with a controllable clock and a
// captured-command spy so double-click dispatch is observable without wiring
// a real drill-down.
type mouseDoubleClickHarness struct {
	app  App
	now  *time.Time
	seen []string
}

// newDoubleClickHarness returns an app with the same layout as makeWheelApp,
// but with a virtual clock and an executeCommand spy installed.
//
// IMPORTANT: harness state (h.app, h.seen, h.now) is not mutex-protected.
// Tests that use this harness MUST NOT call t.Parallel() — the executeCommandFn
// closure writes h.seen concurrently would race with click()'s h.app write.
// All current call sites are sequential; if you add parallel tests, wrap the
// harness in a mutex or give each goroutine its own harness instance.
func newDoubleClickHarness(t *testing.T) *mouseDoubleClickHarness {
	t.Helper()
	app := makeWheelApp(t)

	start := time.Unix(1_700_000_000, 0)
	h := &mouseDoubleClickHarness{now: &start}
	app.now = func() time.Time { return *h.now }
	app.executeCommandFn = func(a App, cmd string) (tea.Model, tea.Cmd) {
		// Record the dispatched command. Don't touch h.app here — click()
		// is the single writer; assigning here would race with click()'s
		// own model.(App) write back.
		h.seen = append(h.seen, cmd)
		return a, nil
	}
	h.app = app
	return h
}

// click sends a left MouseClickMsg at (x, y) through update() and refreshes
// the harness's app snapshot. click is the single writer of h.app — the
// executeCommand spy only records the dispatched command and never touches
// the harness's app pointer.
func (h *mouseDoubleClickHarness) click(x, y int) {
	model, _ := h.app.update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	h.app = model.(App)
}

// advance moves the virtual clock forward by d.
func (h *mouseDoubleClickHarness) advance(d time.Duration) {
	*h.now = h.now.Add(d)
}

// TestDoubleClickWithinWindowTriggersEnter verifies two left-clicks within
// 500 ms at the same split cell dispatch "enter-detail".
func TestDoubleClickWithinWindowTriggersEnter(t *testing.T) {
	h := newDoubleClickHarness(t)

	// Find a body row in split 0.
	rect, ok := h.app.layout.PaneAt(5, 0)
	if !ok || rect.Kind != layout.PaneSplit || rect.SplitIdx != 0 {
		t.Fatalf("expected split 0 at (5,0); got ok=%v kind=%v idx=%d", ok, rect.Kind, rect.SplitIdx)
	}
	var bodyY int
	found := false
	for y := rect.Y; y < rect.Y+rect.H; y++ {
		if h.app.layout.SplitAt(0).RowAtY(y-rect.Y) >= 0 {
			bodyY = y
			found = true
			break
		}
	}
	if !found {
		t.Fatal("could not locate body row y in split 0")
	}

	h.click(5, bodyY)
	if len(h.seen) != 0 {
		t.Fatalf("single click should not dispatch a command, got %v", h.seen)
	}

	h.advance(100 * time.Millisecond)
	h.click(5, bodyY)

	if len(h.seen) != 1 || h.seen[0] != "enter-detail" {
		t.Fatalf("expected exactly one enter-detail dispatch, got %v", h.seen)
	}
}

// TestDoubleClickOutsideWindowDoesNotTrigger verifies that two clicks more
// than 500 ms apart are treated as two separate single-clicks.
func TestDoubleClickOutsideWindowDoesNotTrigger(t *testing.T) {
	h := newDoubleClickHarness(t)

	rect, _ := h.app.layout.PaneAt(5, 0)
	var bodyY int
	for y := rect.Y; y < rect.Y+rect.H; y++ {
		if h.app.layout.SplitAt(0).RowAtY(y-rect.Y) >= 0 {
			bodyY = y
			break
		}
	}

	h.click(5, bodyY)
	h.advance(600 * time.Millisecond)
	h.click(5, bodyY)

	if len(h.seen) != 0 {
		t.Fatalf("clicks 600 ms apart should not dispatch, got %v", h.seen)
	}
}

// TestDoubleClickDifferentCellsDoesNotTrigger verifies that two clicks at
// distinct (x, y) within the window do not trigger a double-click.
func TestDoubleClickDifferentCellsDoesNotTrigger(t *testing.T) {
	h := newDoubleClickHarness(t)

	rect, _ := h.app.layout.PaneAt(5, 0)
	// Pick two distinct body rows.
	var y1, y2 int
	var pickedFirst bool
	for y := rect.Y; y < rect.Y+rect.H; y++ {
		if h.app.layout.SplitAt(0).RowAtY(y-rect.Y) >= 0 {
			if !pickedFirst {
				y1 = y
				pickedFirst = true
				continue
			}
			y2 = y
			break
		}
	}
	if y2 == 0 || y1 == y2 {
		t.Fatalf("could not locate two distinct body rows: y1=%d y2=%d", y1, y2)
	}

	h.click(5, y1)
	h.advance(100 * time.Millisecond)
	h.click(5, y2)

	if len(h.seen) != 0 {
		t.Fatalf("clicks at different cells should not dispatch, got %v", h.seen)
	}
}

// TestDoubleClickAcrossPaneKindsDoesNotTrigger verifies a click on the detail
// pane followed by a click on a split at the same coordinates does not fire
// a double-click (the pane kinds differ, so the guard must reject).
func TestDoubleClickAcrossPaneKindsDoesNotTrigger(t *testing.T) {
	h := newDoubleClickHarness(t)

	// Open the right panel so a detail pane exists.
	h.app.layout.ShowRightPanel()
	h.app = sendWindowSize(h.app, 120, 40)

	// A click on the detail pane writes lastClickKind=PaneDetail. A second
	// click on a split cell within the double-click window must NOT promote
	// to a double: the kinds differ, so the guard rejects.
	detailX, detailY := findDetailCoord(t, h.app)
	h.click(detailX, detailY)
	if len(h.seen) != 0 {
		t.Fatalf("first click should not dispatch, got %v", h.seen)
	}

	// Advance a short interval (still within the double-click window) and
	// click on split 0.
	rect, _ := h.app.layout.PaneAt(5, 0)
	var bodyY int
	for y := rect.Y; y < rect.Y+rect.H; y++ {
		if h.app.layout.SplitAt(0).RowAtY(y-rect.Y) >= 0 {
			bodyY = y
			break
		}
	}
	h.advance(100 * time.Millisecond)
	h.click(5, bodyY)

	// lastClickKind was PaneDetail; the new rect.Kind is PaneSplit. The
	// cross-kind guard in handleMouseClick must suppress the dispatch.
	if len(h.seen) != 0 {
		t.Fatalf("split click immediately after detail click must not double-click, got %v", h.seen)
	}
}

// TestDoubleClickOnDetailAloneNeverTriggers verifies two clicks on the detail
// pane (same coords, within window) do not dispatch — only splits drill down.
func TestDoubleClickOnDetailAloneNeverTriggers(t *testing.T) {
	h := newDoubleClickHarness(t)

	h.app.layout.ShowRightPanel()
	h.app = sendWindowSize(h.app, 120, 40)

	detailX, detailY := findDetailCoord(t, h.app)
	h.click(detailX, detailY)
	h.advance(100 * time.Millisecond)
	h.click(detailX, detailY)

	if len(h.seen) != 0 {
		t.Fatalf("two detail clicks must not fire a double-click, got %v", h.seen)
	}
}

// TestTripleClickFiresSingleDoubleClick verifies that three rapid clicks at
// the same split cell fire exactly one enter-detail dispatch (from click 2);
// click 3 is not a second double-click because the handler resets
// lastClickTime after firing.
func TestTripleClickFiresSingleDoubleClick(t *testing.T) {
	h := newDoubleClickHarness(t)

	rect, _ := h.app.layout.PaneAt(5, 0)
	var bodyY int
	for y := rect.Y; y < rect.Y+rect.H; y++ {
		if h.app.layout.SplitAt(0).RowAtY(y-rect.Y) >= 0 {
			bodyY = y
			break
		}
	}

	h.click(5, bodyY)
	h.advance(100 * time.Millisecond)
	h.click(5, bodyY)
	h.advance(100 * time.Millisecond)
	h.click(5, bodyY)

	if len(h.seen) != 1 || h.seen[0] != "enter-detail" {
		t.Fatalf("triple-click must fire exactly one enter-detail, got %v", h.seen)
	}
}

// TestSingleClickBehaviorPreservedWithClock verifies that installing a
// virtual clock and the executeCommand spy does not break the single-click
// focus/cursor logic from Task 9.
func TestSingleClickBehaviorPreservedWithClock(t *testing.T) {
	h := newDoubleClickHarness(t)
	h.app.layout.FocusSplitAt(1)
	if h.app.layout.FocusIndex() != 1 {
		t.Fatalf("precondition: focus not on split 1")
	}

	rect, _ := h.app.layout.PaneAt(5, 0)
	var bodyY, wantRow int
	for y := rect.Y; y < rect.Y+rect.H; y++ {
		if row := h.app.layout.SplitAt(0).RowAtY(y - rect.Y); row == 2 {
			bodyY = y
			wantRow = row
			break
		}
	}
	if bodyY == 0 {
		t.Fatal("could not locate row 2 y")
	}

	h.click(5, bodyY)

	if h.app.layout.FocusIndex() != 0 {
		t.Fatalf("expected focus on split 0 after click, got %d", h.app.layout.FocusIndex())
	}
	if got := h.app.layout.SplitAt(0).Cursor(); got != wantRow {
		t.Fatalf("expected cursor at row %d, got %d", wantRow, got)
	}
	if len(h.seen) != 0 {
		t.Fatalf("single click must not dispatch, got %v", h.seen)
	}
}

// TestMouseWheelOnDetailPaneScrollsDetail verifies wheel events over the
// detail pane advance the detail view's scroll position (observed via the
// rendered output differing before vs after the wheel).
func TestMouseWheelOnDetailPaneScrollsDetail(t *testing.T) {
	app := makeWheelApp(t)
	app.layout.ShowRightPanel()
	app = sendWindowSize(app, 120, 40)

	// Populate the detail panel with enough content to be scrollable.
	panel := app.layout.RightPanel()
	if panel == nil {
		t.Fatal("expected right panel after ShowRightPanel")
	}
	var b []string
	for i := 0; i < 200; i++ {
		b = append(b, fmt.Sprintf("line %03d", i))
	}
	content := strings.Join(b, "\n")
	panel.SetContent(renderDetailContent(content), true)

	before := panel.View()

	detailX, detailY := findDetailCoord(t, app)
	detailRect, _ := app.layout.PaneAt(detailX, detailY)
	cx := detailRect.X + detailRect.W/2
	cy := detailRect.Y + detailRect.H/2

	model, _ := app.update(tea.MouseWheelMsg{X: cx, Y: cy, Button: tea.MouseWheelDown})
	app = model.(App)

	after := app.layout.RightPanel().View()
	if after == before {
		t.Fatal("expected detail pane view to change after wheel down (content did not scroll)")
	}
}

// renderDetailContent wraps a plain string as a render.Content for tests.
func renderDetailContent(s string) render.Content {
	return render.Content{Raw: s, Display: s}
}

// TestMouseWheelOnLogPaneScrollsLog verifies wheel events over the log pane
// advance its scroll position. The rendered view is captured before and after
// the wheel — they must differ for the scroll to be observable.
func TestMouseWheelOnLogPaneScrollsLog(t *testing.T) {
	app := makeWheelApp(t)
	app.layout.ShowRightPanel()
	app.layout.SetLogMode(true)
	app = sendWindowSize(app, 120, 40)

	lv := app.layout.LogView()
	if lv == nil {
		t.Fatal("expected a log view in log mode")
	}
	// Append enough lines so scrolling is meaningful.
	for i := 0; i < 200; i++ {
		lv.AppendLine(fmt.Sprintf("log %03d", i))
	}
	// Turn autoscroll off so a wheel-up is observable.
	lv.ToggleAutoscroll()

	logX, logY := findLogCoord(t, app)
	before := app.layout.LogView().View()

	model, _ := app.update(tea.MouseWheelMsg{X: logX, Y: logY, Button: tea.MouseWheelUp})
	app = model.(App)

	after := app.layout.LogView().View()
	if after == before {
		t.Fatal("expected log pane view to change after wheel up (content did not scroll)")
	}
}

// TestMouseClickOnLogPaneFocusesDetails verifies a click on the log pane
// moves focus target to details.
func TestMouseClickOnLogPaneFocusesDetails(t *testing.T) {
	app := makeWheelApp(t)
	app.layout.ShowRightPanel()
	app.layout.SetLogMode(true)
	app = sendWindowSize(app, 120, 40)

	logX, logY := findLogCoord(t, app)

	model, _ := app.update(tea.MouseClickMsg{X: logX, Y: logY, Button: tea.MouseLeft})
	app = model.(App)

	if !app.layout.FocusedDetails() {
		t.Fatal("expected focus target to be details after click on log pane")
	}
}

// TestDoubleClickDifferentSplitsSameRowDoesNotTrigger verifies that two
// clicks landing on the same data-row index but in different splits do NOT
// fire a double-click. The row guard alone is not sufficient — the split
// index must also match.
func TestDoubleClickDifferentSplitsSameRowDoesNotTrigger(t *testing.T) {
	h := newDoubleClickHarness(t)

	// Locate split 0 and split 1 body coordinates, picking the same data-row
	// index (row 2) in both so only SplitIdx differs.
	rect0, ok := h.app.layout.PaneAt(5, 0)
	if !ok || rect0.Kind != layout.PaneSplit || rect0.SplitIdx != 0 {
		t.Fatalf("expected split 0 at (5,0); got ok=%v kind=%v idx=%d",
			ok, rect0.Kind, rect0.SplitIdx)
	}
	var y0 int
	foundY0 := false
	for y := rect0.Y; y < rect0.Y+rect0.H; y++ {
		if h.app.layout.SplitAt(0).RowAtY(y-rect0.Y) == 2 {
			y0 = y
			foundY0 = true
			break
		}
	}
	if !foundY0 {
		t.Fatal("could not locate row 2 in split 0")
	}

	// Locate split 1 rect and the y corresponding to row 2 there.
	split1StartY, ok := findFirstY(h.app, layout.PaneSplit, 1)
	if !ok {
		t.Fatal("could not locate split 1")
	}
	rect1, _ := h.app.layout.PaneAt(5, split1StartY)
	var y1 int
	foundY1 := false
	for y := rect1.Y; y < rect1.Y+rect1.H; y++ {
		if h.app.layout.SplitAt(1).RowAtY(y-rect1.Y) == 2 {
			y1 = y
			foundY1 = true
			break
		}
	}
	if !foundY1 {
		t.Fatal("could not locate row 2 in split 1")
	}

	// First click: split 0, row 2.
	h.click(5, y0)
	if h.app.lastClickSplit != 0 {
		t.Fatalf("expected lastClickSplit=0 after first click, got %d", h.app.lastClickSplit)
	}
	if h.app.lastClickRow != 2 {
		t.Fatalf("expected lastClickRow=2 after first click, got %d", h.app.lastClickRow)
	}

	// Second click within the window: split 1, same row index.
	h.advance(100 * time.Millisecond)
	h.click(5, y1)

	if len(h.seen) != 0 {
		t.Fatalf("same-row clicks on different splits must not dispatch, got %v", h.seen)
	}
	// The second click overwrites lastClick* state for its own cell.
	if h.app.lastClickSplit != 1 || h.app.lastClickRow != 2 {
		t.Fatalf("expected lastClickSplit=1, lastClickRow=2 after second click; got split=%d row=%d",
			h.app.lastClickSplit, h.app.lastClickRow)
	}
}

// TestDoubleClickAtExactly500msFires verifies the double-click boundary is
// inclusive: two clicks exactly 500 ms apart still fire enter-detail.
func TestDoubleClickAtExactly500msFires(t *testing.T) {
	h := newDoubleClickHarness(t)

	rect, _ := h.app.layout.PaneAt(5, 0)
	var bodyY int
	for y := rect.Y; y < rect.Y+rect.H; y++ {
		if h.app.layout.SplitAt(0).RowAtY(y-rect.Y) >= 0 {
			bodyY = y
			break
		}
	}

	h.click(5, bodyY)
	h.advance(500 * time.Millisecond)
	h.click(5, bodyY)

	if len(h.seen) != 1 || h.seen[0] != "enter-detail" {
		t.Fatalf("expected enter-detail at exactly 500 ms, got %v", h.seen)
	}
}

// TestMouseWheelInsideNonScrollableOverlayIsNoOp verifies wheel events inside
// the rect of a text-input overlay (confirm, port-forward, etc.) are silently
// dropped without panic and without touching the background splits. These
// overlays intentionally swallow wheel — they have no scrollable content.
func TestMouseWheelInsideNonScrollableOverlayIsNoOp(t *testing.T) {
	app := makeWheelApp(t)

	// Open the confirm dialog: a single-screen prompt with no scroll.
	app.activeOverlay = overlayConfirm
	app.confirmDialog = ui.NewConfirmDialog("proceed?", app.width)
	_ = app.View()

	rect := app.OverlayRect()
	if rect.W == 0 || rect.H == 0 {
		t.Fatalf("expected non-zero overlay rect, got %+v", rect)
	}

	// Should not panic, should not change split cursors.
	splitBefore := app.layout.SplitAt(0).Cursor()
	model, _ := app.update(tea.MouseWheelMsg{
		X:      rect.X + rect.W/2,
		Y:      rect.Y + rect.H/2,
		Button: tea.MouseWheelDown,
	})
	app = model.(App)
	if app.layout.SplitAt(0).Cursor() != splitBefore {
		t.Fatalf("wheel on non-scrollable overlay must not move split cursor")
	}
}

// TestMouseWheelInsideHelpOverlayScrollsHelp verifies that wheel events
// inside the help overlay advance its scroll offset (mirrors j/k behavior).
func TestMouseWheelInsideHelpOverlayScrollsHelp(t *testing.T) {
	app := makeWheelApp(t)

	// Open help with enough hints to force scrolling in the overlay.
	hints := make([]config.KeyHint, 0, 30)
	for range 30 {
		hints = append(hints, config.KeyHint{Key: "k", Help: "move"})
	}
	app.activeOverlay = overlayHelp
	// Shrink the terminal a bit so maxScroll > 0 even with generous screen.
	app = sendWindowSize(app, 80, 14)
	app.helpOverlay.Open([]config.HintGroup{{Scope: "Big", Hints: hints}})
	_ = app.View()

	rect := app.OverlayRect()
	if rect.W == 0 || rect.H == 0 {
		t.Fatalf("expected non-zero overlay rect, got %+v", rect)
	}

	scrollBefore := app.helpOverlay.ScrollForTest()
	model, _ := app.update(tea.MouseWheelMsg{
		X:      rect.X + rect.W/2,
		Y:      rect.Y + rect.H/2,
		Button: tea.MouseWheelDown,
	})
	app = model.(App)

	if after := app.helpOverlay.ScrollForTest(); after != scrollBefore+1 {
		t.Fatalf("expected help scroll to advance from %d to %d, got %d",
			scrollBefore, scrollBefore+1, after)
	}

	// Splits must be untouched.
	// (Wheel was routed to the overlay, not to PaneAt().)
}

// TestViewMouseModeDisabled verifies View() emits MouseModeNone when
// config.Mouse.Enabled is false.
func TestViewMouseModeDisabled(t *testing.T) {
	app := newTestApp()
	app = sendWindowSize(app, 120, 40)
	if app.config.Mouse.Enabled {
		t.Fatal("precondition: default config must have Mouse.Enabled=false")
	}
	v := app.View()
	if v.MouseMode != tea.MouseModeNone {
		t.Fatalf("expected MouseModeNone when disabled, got %v", v.MouseMode)
	}
}

// TestViewMouseModeEnabled verifies View() emits MouseModeCellMotion when
// config.Mouse.Enabled is true.
func TestViewMouseModeEnabled(t *testing.T) {
	app := newTestApp()
	app.config.Mouse.Enabled = true
	app = sendWindowSize(app, 120, 40)
	v := app.View()
	if v.MouseMode != tea.MouseModeCellMotion {
		t.Fatalf("expected MouseModeCellMotion when enabled, got %v", v.MouseMode)
	}
}

// TestMouseWheelDisabledIsNoOp verifies that when Mouse.Enabled is false, a
// MouseWheelMsg is dropped by the handler — no cursor, scroll, or focus
// change results. Guards the defense-in-depth check in handleMouseWheel.
func TestMouseWheelDisabledIsNoOp(t *testing.T) {
	app := makeWheelApp(t)
	// makeWheelApp enables mouse; explicitly disable for this test.
	app.config.Mouse.Enabled = false

	cursor0Before := app.layout.SplitAt(0).Cursor()
	cursor1Before := app.layout.SplitAt(1).Cursor()
	focusBefore := app.layout.FocusIndex()

	model, _ := app.update(tea.MouseWheelMsg{X: 5, Y: 3, Button: tea.MouseWheelDown})
	app = model.(App)

	if app.layout.FocusIndex() != focusBefore {
		t.Fatalf("focus changed while Mouse.Enabled=false: want %d got %d",
			focusBefore, app.layout.FocusIndex())
	}
	if app.layout.SplitAt(0).Cursor() != cursor0Before {
		t.Fatalf("split 0 cursor moved while Mouse.Enabled=false")
	}
	if app.layout.SplitAt(1).Cursor() != cursor1Before {
		t.Fatalf("split 1 cursor moved while Mouse.Enabled=false")
	}
}

// TestMouseClickDisabledIsNoOp verifies that when Mouse.Enabled is false, a
// MouseClickMsg is dropped — no focus change, no cursor change. Guards the
// defense-in-depth check in handleMouseClick.
func TestMouseClickDisabledIsNoOp(t *testing.T) {
	app := makeWheelApp(t)
	app.config.Mouse.Enabled = false

	// Focus split 1 first so a spurious switch to split 0 would show.
	app.layout.FocusSplitAt(1)
	focusBefore := app.layout.FocusIndex()
	cursor0Before := app.layout.SplitAt(0).Cursor()

	model, _ := app.update(tea.MouseClickMsg{X: 5, Y: 3, Button: tea.MouseLeft})
	app = model.(App)

	if app.layout.FocusIndex() != focusBefore {
		t.Fatalf("focus changed while Mouse.Enabled=false: want %d got %d",
			focusBefore, app.layout.FocusIndex())
	}
	if app.layout.SplitAt(0).Cursor() != cursor0Before {
		t.Fatalf("split 0 cursor moved while Mouse.Enabled=false")
	}
}

// TestMouseWheelInsideScaleOverlayIsNoOp verifies that a wheel event inside
// the scale overlay's rect is a silent no-op — scaleOverlay is not part of
// the ScrollWheel dispatch switch in handleMouseWheel.
func TestMouseWheelInsideScaleOverlayIsNoOp(t *testing.T) {
	app := makeWheelApp(t)
	// Open the scale overlay with plausible inputs.
	app.activeOverlay = overlayScale
	gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	app.scaleOverlay.Open("my-deploy", "default", gvr, 3)
	_ = app.View()

	rect := app.OverlayRect()
	if rect.W == 0 || rect.H == 0 {
		t.Fatalf("expected non-zero overlay rect for scale overlay, got %+v", rect)
	}

	splitBefore := app.layout.SplitAt(0).Cursor()

	// Should not panic, and must not affect any split.
	model, _ := app.update(tea.MouseWheelMsg{
		X:      rect.X + rect.W/2,
		Y:      rect.Y + rect.H/2,
		Button: tea.MouseWheelDown,
	})
	app = model.(App)

	if app.layout.SplitAt(0).Cursor() != splitBefore {
		t.Fatalf("wheel on scale overlay must not move split cursor")
	}
	if app.activeOverlay != overlayScale {
		t.Fatalf("wheel must not close the scale overlay; activeOverlay=%v", app.activeOverlay)
	}
}

// TestMouseReleaseIsNoOp verifies that tea.MouseReleaseMsg does not panic and
// leaves app state untouched (reserved for future drag support).
func TestMouseReleaseIsNoOp(t *testing.T) {
	app := makeWheelApp(t)
	focus := app.layout.FocusIndex()
	cursor := app.layout.SplitAt(0).Cursor()

	model, cmd := app.update(tea.MouseReleaseMsg{X: 10, Y: 5, Button: tea.MouseLeft})
	app = model.(App)

	if cmd != nil {
		t.Fatalf("MouseReleaseMsg should produce no command, got %v", cmd)
	}
	if app.layout.FocusIndex() != focus {
		t.Fatalf("MouseReleaseMsg changed focus index: want %d got %d", focus, app.layout.FocusIndex())
	}
	if app.layout.SplitAt(0).Cursor() != cursor {
		t.Fatalf("MouseReleaseMsg changed split cursor")
	}
}

// enableLogModeForRefresh wires the app into log mode with the right panel
// visible so logDebounceSeq is observable as a refresh side effect.
// refreshDetailPanelOrLog increments logDebounceSeq whenever it runs in log
// mode, which gives the mouse-click tests a clean signal to assert against.
func enableLogModeForRefresh(app App) App {
	app.layout.ShowRightPanel()
	app.layout.SetLogMode(true)
	return app
}

// bodyRowY returns the screen y-coordinate of the requested data-row index in
// the given split, or fails the test if the row is not visible.
func bodyRowY(t *testing.T, app App, splitIdx, wantRow int) int {
	t.Helper()
	startY, ok := findFirstY(app, layout.PaneSplit, splitIdx)
	if !ok {
		t.Fatalf("could not locate split %d", splitIdx)
	}
	rect, _ := app.layout.PaneAt(5, startY)
	split := app.layout.SplitAt(splitIdx)
	for y := rect.Y; y < rect.Y+rect.H; y++ {
		if split.RowAtY(y-rect.Y) == wantRow {
			return y
		}
	}
	t.Fatalf("could not locate row %d in split %d", wantRow, splitIdx)
	return 0
}

// TestMouseClickOnNewRowRefreshesDetailPane verifies that clicking a different
// row in the focused split triggers a detail-pane refresh, matching the
// keyboard cursor-move behavior.
func TestMouseClickOnNewRowRefreshesDetailPane(t *testing.T) {
	app := makeWheelApp(t)
	app = enableLogModeForRefresh(app)

	// Cursor starts at row 0; click on row 3.
	y := bodyRowY(t, app, 0, 3)
	before := app.logDebounceSeq

	model, _ := app.update(tea.MouseClickMsg{X: 5, Y: y, Button: tea.MouseLeft})
	app = model.(App)

	if app.layout.SplitAt(0).Cursor() != 3 {
		t.Fatalf("precondition: expected cursor at row 3, got %d", app.layout.SplitAt(0).Cursor())
	}
	if app.logDebounceSeq <= before {
		t.Fatalf("expected logDebounceSeq to increment after row change (was %d, got %d)", before, app.logDebounceSeq)
	}
}

// TestMouseClickOnSameRowSkipsDetailRefresh verifies that a click on the
// already-selected row does not trigger a detail-pane refresh. Skipping this
// avoids a redundant log-stream restart and prevents the first click of a
// double-click from stealing work from the subsequent drill-down.
func TestMouseClickOnSameRowSkipsDetailRefresh(t *testing.T) {
	app := makeWheelApp(t)
	app = enableLogModeForRefresh(app)

	// Move cursor to row 2, then click on row 2 — no change expected.
	app.layout.SplitAt(0).SetCursor(2)
	y := bodyRowY(t, app, 0, 2)
	before := app.logDebounceSeq

	model, _ := app.update(tea.MouseClickMsg{X: 5, Y: y, Button: tea.MouseLeft})
	app = model.(App)

	if app.logDebounceSeq != before {
		t.Fatalf("expected logDebounceSeq unchanged on same-row click (was %d, got %d)", before, app.logDebounceSeq)
	}
}

// TestMouseClickOnOtherSplitAlwaysRefreshes verifies that clicking into a
// non-focused split triggers a refresh even when the landed row index
// coincides with that split's previous cursor. Each split owns its own detail
// state, so a focus switch must always rebind the detail pane.
func TestMouseClickOnOtherSplitAlwaysRefreshes(t *testing.T) {
	app := makeWheelApp(t)
	app = enableLogModeForRefresh(app)

	// Focus is on split 0; split 1's cursor is 0. Click on split 1's row 0
	// so only the split index differs — the crossSplit rule must still refresh.
	if app.layout.FocusIndex() != 0 {
		t.Fatalf("precondition: expected focus on split 0, got %d", app.layout.FocusIndex())
	}
	if got := app.layout.SplitAt(1).Cursor(); got != 0 {
		t.Fatalf("precondition: expected split 1 cursor at 0, got %d", got)
	}
	y := bodyRowY(t, app, 1, 0)
	before := app.logDebounceSeq

	model, _ := app.update(tea.MouseClickMsg{X: 5, Y: y, Button: tea.MouseLeft})
	app = model.(App)

	if app.layout.FocusIndex() != 1 {
		t.Fatalf("expected focus on split 1 after click, got %d", app.layout.FocusIndex())
	}
	if app.logDebounceSeq <= before {
		t.Fatalf("expected logDebounceSeq to increment on cross-split click (was %d, got %d)", before, app.logDebounceSeq)
	}
}

// TestMouseClickOnHeaderRowSkipsRefresh verifies that a click on a split's
// chrome (border/header) focuses that split but does not trigger a refresh,
// because no data row was selected.
func TestMouseClickOnHeaderRowSkipsRefresh(t *testing.T) {
	app := makeWheelApp(t)
	app = enableLogModeForRefresh(app)

	// Focus split 1 first so the header click is a focus switch. Even then,
	// the refresh must not fire because no row was landed on.
	app.layout.FocusSplitAt(1)
	rect0, ok := app.layout.PaneAt(5, 0)
	if !ok || rect0.SplitIdx != 0 {
		t.Fatalf("expected split 0 at y=0, got ok=%v idx=%d", ok, rect0.SplitIdx)
	}
	if app.layout.SplitAt(0).RowAtY(0) != -1 {
		t.Fatal("precondition: expected RowAtY(0) to be -1 (chrome)")
	}
	before := app.logDebounceSeq

	model, _ := app.update(tea.MouseClickMsg{X: 5, Y: rect0.Y, Button: tea.MouseLeft})
	app = model.(App)

	if app.logDebounceSeq != before {
		t.Fatalf("expected logDebounceSeq unchanged on chrome click (was %d, got %d)", before, app.logDebounceSeq)
	}
}

// TestDoubleClickFirstClickRefreshesSecondSkipsRefresh verifies the
// interaction between single-click refresh and double-click drill: the first
// click (new row) refreshes; the second click (same row, within window) does
// not fire another refresh but does dispatch enter-detail.
func TestDoubleClickFirstClickRefreshesSecondSkipsRefresh(t *testing.T) {
	h := newDoubleClickHarness(t)
	h.app = enableLogModeForRefresh(h.app)

	// Pick a row that is not the current cursor so the first click is a row
	// change and will refresh.
	y := bodyRowY(t, h.app, 0, 3)
	beforeFirst := h.app.logDebounceSeq

	h.click(5, y)
	afterFirst := h.app.logDebounceSeq
	if afterFirst <= beforeFirst {
		t.Fatalf("first click must refresh (was %d, got %d)", beforeFirst, afterFirst)
	}
	if len(h.seen) != 0 {
		t.Fatalf("first click must not dispatch a command, got %v", h.seen)
	}

	// Second click within the double-click window at the same cell.
	h.advance(100 * time.Millisecond)
	h.click(5, y)

	if h.app.logDebounceSeq != afterFirst {
		t.Fatalf("second click (same row) must not refresh again (was %d, got %d)", afterFirst, h.app.logDebounceSeq)
	}
	if len(h.seen) != 1 || h.seen[0] != "enter-detail" {
		t.Fatalf("expected enter-detail dispatch on second click, got %v", h.seen)
	}
}
