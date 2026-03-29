package ui

import (
	"context"
	"fmt"
	"testing"

	"github.com/aohoyd/aku/internal/msgs"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/render"
	"github.com/aohoyd/aku/internal/table"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

func TestContentWidthWithANSI(t *testing.T) {
	// Simulate a row with ANSI-colored cell (like colored status)
	ansiCell := "\x1b[38;2;80;250;123mRunning\x1b[0m" // green "Running"
	rows := []table.Row{
		{"my-pod", ansiCell},
	}
	// Column 1 is the ANSI cell; visual width of "Running" is 7
	w := contentWidth(1, 6, rows) // headerWidth=6 ("STATUS")
	// Should be 7 + 3 = 10, NOT inflated by ANSI byte length
	if w != 10 {
		t.Fatalf("expected contentWidth 10 (visual), got %d", w)
	}
}

// testPlugin implements plugin.ResourcePlugin for testing
type testPlugin struct{}

func (p *testPlugin) Name() string      { return "pods" }
func (p *testPlugin) ShortName() string { return "po" }
func (p *testPlugin) GVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{Version: "v1", Resource: "pods"}
}
func (p *testPlugin) IsClusterScoped() bool { return false }
func (p *testPlugin) Columns() []plugin.Column {
	return []plugin.Column{
		{Title: "NAME", Flex: true},
		{Title: "STATUS", Width: 10},
	}
}
func (p *testPlugin) Row(obj *unstructured.Unstructured) []string {
	return []string{obj.GetName(), "Running"}
}
func (p *testPlugin) YAML(_ *unstructured.Unstructured) (render.Content, error)   { return render.Content{}, nil }
func (p *testPlugin) Describe(_ context.Context, _ *unstructured.Unstructured) (render.Content, error) {
	return render.Content{}, nil
}

func makeObj(name string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetName(name)
	return obj
}

func makeNsObj(name, namespace string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetName(name)
	obj.SetNamespace(namespace)
	return obj
}

func makeKindObj(name, kind string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetName(name)
	obj.SetKind(kind)
	return obj
}

func TestResourceListSetObjects(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 40, 10)
	objs := []*unstructured.Unstructured{makeObj("pod-a"), makeObj("pod-b")}
	rl.SetObjects(objs)
	if rl.Len() != 2 {
		t.Fatalf("expected 2 objects, got %d", rl.Len())
	}
}

func TestResourceListSelected(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 40, 10)
	objs := []*unstructured.Unstructured{makeObj("pod-a"), makeObj("pod-b")}
	rl.SetObjects(objs)
	sel := rl.Selected()
	if sel == nil || sel.GetName() != "pod-a" {
		t.Fatal("expected first object selected by default")
	}
}

func TestResourceListSelectedEmpty(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 40, 10)
	sel := rl.Selected()
	if sel != nil {
		t.Fatal("expected nil for empty list")
	}
}

func TestResourceListSetPlugin(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 40, 10)
	objs := []*unstructured.Unstructured{makeObj("pod-a")}
	rl.SetObjects(objs)

	newPlugin := &testPlugin{}
	rl.SetPlugin(newPlugin)
	if rl.Len() != 0 {
		t.Fatal("changing plugin should clear the list")
	}
	if rl.Plugin().Name() != "pods" {
		t.Fatal("plugin should be updated")
	}
}

func TestResourceListFocusBlur(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 40, 10)
	if !rl.Focused() {
		t.Fatal("should start focused")
	}
	rl.Blur()
	if rl.Focused() {
		t.Fatal("should be blurred")
	}
	rl.Focus()
	if !rl.Focused() {
		t.Fatal("should be focused again")
	}
}

func TestPluginColumnsToTableColumnsNoRows(t *testing.T) {
	cols := []plugin.Column{
		{Title: "NAME", Flex: true},
		{Title: "STATUS", Width: 10},
	}
	result, _ := pluginColumnsToTableColumns(cols, 50, SortState{}, nil)
	if len(result) != 2 {
		t.Fatalf("expected 2 columns, got %d", len(result))
	}
	// With no rows, flex column width = len("NAME") + 3 = 7
	if result[0].Width != 7 {
		t.Fatalf("expected flex column width 7 (header only), got %d", result[0].Width)
	}
	if result[1].Width != 10 {
		t.Fatalf("expected fixed column width 10, got %d", result[1].Width)
	}
}

func TestPluginColumnsToTableColumnsWithRows(t *testing.T) {
	cols := []plugin.Column{
		{Title: "NAME", Flex: true},
		{Title: "STATUS", Width: 10},
	}
	rows := []table.Row{
		{"my-long-pod-name", "Running"},
		{"short", "Pending"},
	}
	result, _ := pluginColumnsToTableColumns(cols, 80, SortState{}, rows)
	// Flex width = max(len("NAME"), len("my-long-pod-name")) + 3 = 19
	if result[0].Width != 19 {
		t.Fatalf("expected flex column width 19 (content-based), got %d", result[0].Width)
	}
}

func TestPluginColumnsFlexUncapped(t *testing.T) {
	cols := []plugin.Column{
		{Title: "NAME", Flex: true},
		{Title: "AGE", Width: 8},
	}
	rows := []table.Row{
		{"a-very-very-very-very-very-very-very-very-long-name", "5d"},
	}
	// totalWidth=40, but flex should NOT be capped
	result, contentWidth := pluginColumnsToTableColumns(cols, 40, SortState{}, rows)
	// content width = len("a-very-...") + 3 = 54
	if result[0].Width != 54 {
		t.Fatalf("expected uncapped flex width 54, got %d", result[0].Width)
	}
	// contentWidth = sum of widths + padding (2 per col)
	expectedCW := 54 + 8 + 2*2
	if contentWidth != expectedCW {
		t.Fatalf("expected contentWidth %d, got %d", expectedCW, contentWidth)
	}
}

func TestPluginColumnsToTableColumnsReturnsContentWidth(t *testing.T) {
	cols := []plugin.Column{
		{Title: "NAME", Flex: true},
		{Title: "STATUS", Width: 10},
	}
	rows := []table.Row{
		{"my-pod", "Running"},
	}
	result, contentWidth := pluginColumnsToTableColumns(cols, 80, SortState{}, rows)
	// NAME flex = max(4, 6) + 3 = 9, STATUS = 10, padding = 2*2 = 4
	expectedCW := result[0].Width + result[1].Width + 2*len(cols)
	if contentWidth != expectedCW {
		t.Fatalf("expected contentWidth %d, got %d", expectedCW, contentWidth)
	}
}

func TestPluginColumnsWithSortIndicator(t *testing.T) {
	cols := []plugin.Column{
		{Title: "NAME", Flex: true},
		{Title: "STATUS", Width: 10},
	}
	state := SortState{Column: "NAME", Ascending: true}
	result, _ := pluginColumnsToTableColumns(cols, 50, state, nil)
	if result[0].Title != "NAME ▲" {
		t.Fatalf("expected 'NAME ▲', got %q", result[0].Title)
	}
	if result[1].Title != "STATUS" {
		t.Fatalf("expected 'STATUS' (no indicator), got %q", result[1].Title)
	}
}

func TestResourceListDefaultSort(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 40, 10)
	state := rl.SortState()
	if state.Column != "NAME" || !state.Ascending {
		t.Fatalf("expected default sort NAME ascending, got %+v", state)
	}
}

func TestResourceListSetSort(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 40, 10)
	objs := []*unstructured.Unstructured{makeObj("bravo"), makeObj("alpha")}
	rl.SetObjects(objs)

	// Default sort is NAME ascending, so objects should be reordered
	if rl.Selected().GetName() != "alpha" {
		t.Fatalf("expected 'alpha' first after default NAME sort, got %q", rl.Selected().GetName())
	}

	// Toggle to descending
	rl.SetSort("NAME")
	if rl.SortState().Ascending {
		t.Fatal("second press should toggle to descending")
	}
	if rl.Selected() == nil {
		t.Fatal("selected should not be nil after sort change")
	}
}

func TestResourceListSetSortNewColumn(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 40, 10)
	rl.SetSort("STATUS")
	state := rl.SortState()
	if state.Column != "STATUS" || !state.Ascending {
		t.Fatalf("expected STATUS ascending, got %+v", state)
	}
}

func TestResourceListSetPluginResetsSort(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 40, 10)
	rl.SetSort("AGE")
	rl.SetPlugin(&testPlugin{})
	state := rl.SortState()
	if state.Column != "NAME" || !state.Ascending {
		t.Fatalf("SetPlugin should reset sort to default, got %+v", state)
	}
}

func TestResourceListApplySearch(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 15)
	objs := []*unstructured.Unstructured{makeObj("nginx-pod"), makeObj("redis-pod"), makeObj("mysql-pod")}
	rl.SetObjects(objs)

	err := rl.ApplySearch("nginx", msgs.SearchModeSearch)
	if err != nil {
		t.Fatalf("ApplySearch should not error: %v", err)
	}
	if !rl.SearchActive() {
		t.Fatal("search should be active")
	}
	// All 3 rows should still be visible (search highlights, doesn't filter)
	if rl.Len() != 3 {
		t.Fatalf("search should not filter rows, expected 3, got %d", rl.Len())
	}
}

func TestResourceListApplyFilter(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 15)
	objs := []*unstructured.Unstructured{makeObj("nginx-pod"), makeObj("redis-pod"), makeObj("mysql-pod")}
	rl.SetObjects(objs)

	err := rl.ApplySearch("nginx", msgs.SearchModeFilter)
	if err != nil {
		t.Fatalf("ApplySearch filter should not error: %v", err)
	}
	// Only nginx-pod should be visible
	if rl.Len() != 1 {
		t.Fatalf("filter should leave 1 row, got %d", rl.Len())
	}
	if rl.Selected().GetName() != "nginx-pod" {
		t.Fatalf("expected nginx-pod, got %q", rl.Selected().GetName())
	}
}

func TestResourceListClearSearch(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 15)
	objs := []*unstructured.Unstructured{makeObj("nginx-pod"), makeObj("redis-pod")}
	rl.SetObjects(objs)

	rl.ApplySearch("nginx", msgs.SearchModeFilter)
	if rl.Len() != 1 {
		t.Fatalf("expected 1, got %d", rl.Len())
	}

	rl.ClearFilter()
	if rl.AnyActive() {
		t.Fatal("should be inactive after clear")
	}
	if rl.Len() != 2 {
		t.Fatalf("all rows should be restored, expected 2, got %d", rl.Len())
	}
}

func TestResourceListSearchInvalidRegex(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 15)
	objs := []*unstructured.Unstructured{makeObj("pod-a")}
	rl.SetObjects(objs)

	err := rl.ApplySearch("[invalid", msgs.SearchModeSearch)
	if err == nil {
		t.Fatal("invalid regex should return error")
	}
	if rl.SearchActive() {
		t.Fatal("search should not be active after invalid regex")
	}
}

func TestResourceListSearchNext(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 15)
	objs := []*unstructured.Unstructured{makeObj("alpha"), makeObj("beta-pod"), makeObj("gamma"), makeObj("delta-pod")}
	rl.SetObjects(objs)

	rl.ApplySearch("pod", msgs.SearchModeSearch)
	// Cursor should jump to first match
	sel := rl.Selected()
	if sel == nil || sel.GetName() != "beta-pod" {
		name := ""
		if sel != nil {
			name = sel.GetName()
		}
		t.Fatalf("expected cursor on first match 'beta-pod', got %q", name)
	}

	rl.SearchNext()
	sel = rl.Selected()
	if sel == nil || sel.GetName() != "delta-pod" {
		name := ""
		if sel != nil {
			name = sel.GetName()
		}
		t.Fatalf("expected cursor on next match 'delta-pod', got %q", name)
	}

	// Wrap around
	rl.SearchNext()
	sel = rl.Selected()
	if sel == nil || sel.GetName() != "beta-pod" {
		name := ""
		if sel != nil {
			name = sel.GetName()
		}
		t.Fatalf("expected cursor to wrap to 'beta-pod', got %q", name)
	}
}

func TestResourceListSearchPrev(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 15)
	objs := []*unstructured.Unstructured{makeObj("alpha"), makeObj("beta-pod"), makeObj("gamma"), makeObj("delta-pod")}
	rl.SetObjects(objs)

	rl.ApplySearch("pod", msgs.SearchModeSearch)
	// Should be on first match: beta-pod
	rl.SearchPrev()
	sel := rl.Selected()
	if sel == nil || sel.GetName() != "delta-pod" {
		name := ""
		if sel != nil {
			name = sel.GetName()
		}
		t.Fatalf("expected cursor to wrap backwards to 'delta-pod', got %q", name)
	}
}

func TestResourceListSearchNextAfterManualCursorMove(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 15)
	// Matches at indices 1, 3, 5
	objs := []*unstructured.Unstructured{
		makeObj("alpha"), makeObj("beta-pod"), makeObj("gamma"),
		makeObj("delta-pod"), makeObj("epsilon"), makeObj("zeta-pod"),
	}
	rl.SetObjects(objs)

	rl.ApplySearch("pod", msgs.SearchModeSearch)
	// Cursor on beta-pod (index 1)

	// Manually move cursor to index 4 (epsilon), simulating j/k navigation
	rl.table.SetCursor(4)

	// SearchNext should find next match after cursor pos 4 → zeta-pod (index 5)
	rl.SearchNext()
	sel := rl.Selected()
	if sel == nil || sel.GetName() != "zeta-pod" {
		name := ""
		if sel != nil {
			name = sel.GetName()
		}
		t.Fatalf("expected 'zeta-pod' after manual move, got %q", name)
	}

	// Move cursor back to index 0, SearchPrev should wrap to zeta-pod (index 5)
	rl.table.SetCursor(0)
	rl.SearchPrev()
	sel = rl.Selected()
	if sel == nil || sel.GetName() != "zeta-pod" {
		name := ""
		if sel != nil {
			name = sel.GetName()
		}
		t.Fatalf("expected 'zeta-pod' wrapping backwards from 0, got %q", name)
	}
}

func TestResourceListFilterReappliedOnSetObjects(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 15)
	objs := []*unstructured.Unstructured{makeObj("nginx-pod"), makeObj("redis-pod")}
	rl.SetObjects(objs)

	rl.ApplySearch("nginx", msgs.SearchModeFilter)
	if rl.Len() != 1 {
		t.Fatalf("expected 1, got %d", rl.Len())
	}

	// Simulate k8s data update with a new object that matches
	newObjs := []*unstructured.Unstructured{makeObj("nginx-pod"), makeObj("redis-pod"), makeObj("nginx-deploy")}
	rl.SetObjects(newObjs)
	if rl.Len() != 2 {
		t.Fatalf("filter should include new matching object, expected 2, got %d", rl.Len())
	}
}

func TestResourceListSetPluginClearsSearch(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 15)
	objs := []*unstructured.Unstructured{makeObj("nginx-pod")}
	rl.SetObjects(objs)

	rl.ApplySearch("nginx", msgs.SearchModeFilter)
	rl.SetPlugin(&testPlugin{})
	if rl.AnyActive() {
		t.Fatal("SetPlugin should clear all search state")
	}
}

func TestResourceListFilterAndSearchIndependent(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 15)
	objs := []*unstructured.Unstructured{
		makeObj("nginx-pod"), makeObj("redis-pod"), makeObj("nginx-deploy"),
	}
	rl.SetObjects(objs)

	// Apply filter first
	rl.ApplySearch("nginx", msgs.SearchModeFilter)
	if rl.Len() != 2 {
		t.Fatalf("filter should show 2 nginx objects, got %d", rl.Len())
	}

	// Apply search on top of filter
	err := rl.ApplySearch("pod", msgs.SearchModeSearch)
	if err != nil {
		t.Fatalf("search should not error: %v", err)
	}

	// Both should be active
	if !rl.SearchActive() {
		t.Fatal("search should be active")
	}
	if !rl.FilterActive() {
		t.Fatal("filter should be active")
	}
	if !rl.AnyActive() {
		t.Fatal("AnyActive should be true")
	}

	// Filter should still show only 2 rows (search doesn't change row count)
	if rl.Len() != 2 {
		t.Fatalf("search should not change filtered row count, expected 2, got %d", rl.Len())
	}
}

func TestResourceListLayeredClear(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 15)
	objs := []*unstructured.Unstructured{makeObj("nginx-pod"), makeObj("redis-pod")}
	rl.SetObjects(objs)

	rl.ApplySearch("nginx", msgs.SearchModeFilter)
	rl.ApplySearch("pod", msgs.SearchModeSearch)

	// Clear search first
	rl.ClearSearch()
	if rl.SearchActive() {
		t.Fatal("search should be cleared")
	}
	if !rl.FilterActive() {
		t.Fatal("filter should still be active")
	}
	if rl.Len() != 1 {
		t.Fatalf("filter should still apply, expected 1, got %d", rl.Len())
	}

	// Clear filter
	rl.ClearFilter()
	if rl.FilterActive() {
		t.Fatal("filter should be cleared")
	}
	if rl.AnyActive() {
		t.Fatal("nothing should be active")
	}
	if rl.Len() != 2 {
		t.Fatalf("all rows should be restored, expected 2, got %d", rl.Len())
	}
}

func TestResourceListAllNamespacesColumn(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 15)
	rl.SetNamespace("")

	objs := []*unstructured.Unstructured{
		makeNsObj("pod-a", "staging"),
		makeNsObj("pod-b", "production"),
	}
	rl.SetObjects(objs)

	// The table should have 3 columns: NAMESPACE, NAME, STATUS
	rows := rl.table.Rows()
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	// First cell of each row should be the namespace
	if rows[0][0] != "staging" {
		t.Fatalf("expected first cell 'staging', got %q", rows[0][0])
	}
	if rows[1][0] != "production" {
		t.Fatalf("expected first cell 'production', got %q", rows[1][0])
	}
}

func TestResourceListNoNamespaceColumnForSpecificNs(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 15)
	rl.SetNamespace("default")

	objs := []*unstructured.Unstructured{makeNsObj("pod-a", "default")}
	rl.SetObjects(objs)

	rows := rl.table.Rows()
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	// First cell should be the pod name (no namespace column)
	if rows[0][0] != "pod-a" {
		t.Fatalf("expected first cell 'pod-a', got %q", rows[0][0])
	}
}

func TestResourceListAllNamespacesFilterMatchesNamespace(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 15)
	rl.SetNamespace("")

	objs := []*unstructured.Unstructured{
		makeNsObj("pod-a", "staging"),
		makeNsObj("pod-b", "production"),
	}
	rl.SetObjects(objs)

	// Filter by namespace name "staging"
	err := rl.ApplySearch("staging", msgs.SearchModeFilter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rl.Len() != 1 {
		t.Fatalf("expected 1 filtered result, got %d", rl.Len())
	}
	if rl.Selected().GetNamespace() != "staging" {
		t.Fatalf("expected staging pod, got %q", rl.Selected().GetNamespace())
	}
}

func TestResourceListAllNamespacesCursorRestore(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 15)
	rl.SetNamespace("")

	// Two pods with same name in different namespaces
	objs := []*unstructured.Unstructured{
		makeNsObj("nginx", "production"),
		makeNsObj("nginx", "staging"),
	}
	rl.SetObjects(objs)

	// Move cursor to second item (staging/nginx)
	rl.table.SetCursor(1)

	// Re-set objects (simulating informer update)
	rl.SetObjects(objs)

	// Cursor should restore to staging/nginx (index 1), not production/nginx (index 0)
	sel := rl.Selected()
	if sel == nil {
		t.Fatal("expected a selected object")
	}
	if sel.GetNamespace() != "staging" {
		t.Fatalf("expected cursor restored to staging/nginx, got %s/%s", sel.GetNamespace(), sel.GetName())
	}
}

func TestResourceListCursorRestoreByKind(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 15)
	rl.SetNamespace("default")

	// Two objects with the same name but different kinds
	objs := []*unstructured.Unstructured{
		makeKindObj("my-app", "Deployment"),
		makeKindObj("my-app", "Service"),
	}
	rl.SetObjects(objs)

	// Move cursor to second item (Service/my-app)
	rl.table.SetCursor(1)

	// Re-set objects (simulating informer update or post-pop refresh)
	rl.SetObjects(objs)

	// Cursor should restore to Service/my-app (index 1), not Deployment/my-app (index 0)
	sel := rl.Selected()
	if sel == nil {
		t.Fatal("expected a selected object")
	}
	if sel.GetKind() != "Service" {
		t.Fatalf("expected cursor restored to Service/my-app, got %s/%s", sel.GetKind(), sel.GetName())
	}
}

func TestResourceListSwitchFromAllNamespacesToSpecific(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 15)
	rl.SetNamespace("")

	objs := []*unstructured.Unstructured{
		makeNsObj("pod-a", "staging"),
		makeNsObj("pod-b", "production"),
	}
	rl.SetObjects(objs)

	// Verify NAMESPACE column is present (3 cells per row)
	if len(rl.table.Rows()[0]) != 3 {
		t.Fatalf("expected 3 cells in all-ns mode, got %d", len(rl.table.Rows()[0]))
	}

	// Switch to specific namespace — must not panic when column count changes
	rl.SetNamespace("default")
	rl.SetObjects(nil)

	specificObjs := []*unstructured.Unstructured{makeNsObj("pod-c", "default")}
	rl.SetObjects(specificObjs)

	// Should now have 2 cells per row (no NAMESPACE column)
	if len(rl.table.Rows()) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rl.table.Rows()))
	}
	if len(rl.table.Rows()[0]) != 2 {
		t.Fatalf("expected 2 cells in specific-ns mode, got %d", len(rl.table.Rows()[0]))
	}
}

func TestFocusMakesCursorVisible(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 10)
	objs := make([]*unstructured.Unstructured, 50)
	for i := range objs {
		objs[i] = makeObj(fmt.Sprintf("pod-%03d", i))
	}
	rl.SetObjects(objs)

	// Navigate cursor deep into the list
	for range 30 {
		rl.CursorDown()
	}
	if rl.Cursor() != 30 {
		t.Fatalf("expected cursor at 30, got %d", rl.Cursor())
	}

	// Blur and re-focus — cursor position must be preserved
	rl.Blur()
	rl.Focus()

	if rl.Cursor() != 30 {
		t.Fatalf("expected cursor at 30 after blur/focus, got %d", rl.Cursor())
	}
	sel := rl.Selected()
	if sel == nil || sel.GetName() != "pod-030" {
		name := ""
		if sel != nil {
			name = sel.GetName()
		}
		t.Fatalf("expected pod-030 selected, got %q", name)
	}
}

func TestParentSnap(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 20)
	// Before drilldown, ParentSnap should be nil
	if snap := rl.ParentSnap(); snap != nil {
		t.Fatalf("expected nil ParentSnap before drilldown, got %+v", snap)
	}
	// Push a nav entry
	child := &testPlugin{}
	rl.PushNav(child, nil, "my-deployment", "uid-123", "", "")
	snap := rl.ParentSnap()
	if snap == nil {
		t.Fatal("expected non-nil ParentSnap after drilldown")
	}
	if snap.ParentName != "my-deployment" {
		t.Fatalf("expected ParentName 'my-deployment', got %q", snap.ParentName)
	}
	if snap.ParentUID != "uid-123" {
		t.Fatalf("expected ParentUID 'uid-123', got %q", snap.ParentUID)
	}
}

func TestParentSnapKindAndAPIVersion(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 20)
	if snap := rl.ParentSnap(); snap != nil {
		t.Fatalf("expected nil ParentSnap before drilldown, got %+v", snap)
	}
	child := &testPlugin{}
	rl.PushNav(child, nil, "my-app", "", "v1", "Secret")
	snap := rl.ParentSnap()
	if snap == nil {
		t.Fatal("expected non-nil ParentSnap after drilldown")
	}
	if snap.ParentKind != "Secret" {
		t.Fatalf("expected ParentKind 'Secret', got %q", snap.ParentKind)
	}
	if snap.ParentAPIVersion != "v1" {
		t.Fatalf("expected ParentAPIVersion 'v1', got %q", snap.ParentAPIVersion)
	}
}

func TestFocusBorderMakesCursorVisible(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 10)
	objs := make([]*unstructured.Unstructured, 50)
	for i := range objs {
		objs[i] = makeObj(fmt.Sprintf("pod-%03d", i))
	}
	rl.SetObjects(objs)

	for range 25 {
		rl.CursorDown()
	}

	rl.BlurBorder()
	rl.FocusBorder()

	if rl.Cursor() != 25 {
		t.Fatalf("expected cursor at 25 after BlurBorder/FocusBorder, got %d", rl.Cursor())
	}
}

// --- Selection tests ---

func makeUIDObj(name, uid string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetName(name)
	obj.SetUID(types.UID(uid))
	return obj
}

func TestToggleSelect(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 15)
	objs := []*unstructured.Unstructured{
		makeUIDObj("pod-a", "uid-a"),
		makeUIDObj("pod-b", "uid-b"),
		makeUIDObj("pod-c", "uid-c"),
	}
	rl.SetObjects(objs)

	rl.ToggleSelect()
	if !rl.HasSelection() {
		t.Fatal("expected selection after toggle")
	}
	if rl.SelectionCount() != 1 {
		t.Fatalf("expected 1 selected, got %d", rl.SelectionCount())
	}

	// Toggle same row again should deselect
	rl.table.SetCursor(0)
	rl.ToggleSelect()
	if rl.HasSelection() {
		t.Fatal("expected no selection after second toggle")
	}
}

func TestSelectAll(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 15)
	objs := []*unstructured.Unstructured{
		makeUIDObj("pod-a", "uid-a"),
		makeUIDObj("pod-b", "uid-b"),
	}
	rl.SetObjects(objs)

	rl.SelectAll()
	if rl.SelectionCount() != 2 {
		t.Fatalf("expected 2 selected, got %d", rl.SelectionCount())
	}

	// Second call should deselect all
	rl.SelectAll()
	if rl.HasSelection() {
		t.Fatal("expected deselect all on second call")
	}
}

func TestClearSelection(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 15)
	objs := []*unstructured.Unstructured{makeUIDObj("pod-a", "uid-a")}
	rl.SetObjects(objs)
	rl.ToggleSelect()
	rl.ClearSelection()
	if rl.HasSelection() {
		t.Fatal("expected no selection after clear")
	}
}

func TestSelectedObjects(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 15)
	objs := []*unstructured.Unstructured{
		makeUIDObj("pod-a", "uid-a"),
		makeUIDObj("pod-b", "uid-b"),
		makeUIDObj("pod-c", "uid-c"),
	}
	rl.SetObjects(objs)

	// Select first and third
	rl.ToggleSelect() // selects pod-a, cursor stays at 0
	rl.table.SetCursor(2)
	rl.ToggleSelect() // selects pod-c

	selected := rl.SelectedObjects()
	if len(selected) != 2 {
		t.Fatalf("expected 2 selected objects, got %d", len(selected))
	}
	if selected[0].GetName() != "pod-a" {
		t.Fatalf("expected pod-a first, got %s", selected[0].GetName())
	}
	if selected[1].GetName() != "pod-c" {
		t.Fatalf("expected pod-c second, got %s", selected[1].GetName())
	}
}

func TestSelectionClearedOnSetPlugin(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 15)
	objs := []*unstructured.Unstructured{makeUIDObj("pod-a", "uid-a")}
	rl.SetObjects(objs)
	rl.ToggleSelect()
	rl.SetPlugin(&testPlugin{})
	if rl.HasSelection() {
		t.Fatal("SetPlugin should clear selection")
	}
}

func TestSelectionPrunedOnSetObjects(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 15)
	objs := []*unstructured.Unstructured{
		makeUIDObj("pod-a", "uid-a"),
		makeUIDObj("pod-b", "uid-b"),
	}
	rl.SetObjects(objs)

	rl.SelectAll()
	if rl.SelectionCount() != 2 {
		t.Fatalf("expected 2, got %d", rl.SelectionCount())
	}

	// Simulate informer update removing pod-b
	newObjs := []*unstructured.Unstructured{makeUIDObj("pod-a", "uid-a")}
	rl.SetObjects(newObjs)

	if rl.SelectionCount() != 1 {
		t.Fatalf("expected 1 after prune, got %d", rl.SelectionCount())
	}
	selected := rl.SelectedObjects()
	if len(selected) != 1 || selected[0].GetName() != "pod-a" {
		t.Fatal("expected only pod-a after prune")
	}
}

func TestSelectionClearedOnPushNav(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 15)
	objs := []*unstructured.Unstructured{makeUIDObj("pod-a", "uid-a")}
	rl.SetObjects(objs)
	rl.ToggleSelect()
	rl.PushNav(&testPlugin{}, nil, "parent", "uid", "", "")
	if rl.HasSelection() {
		t.Fatal("PushNav should clear selection")
	}
}

func TestSelectionClearedOnPopNav(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 15)
	objs := []*unstructured.Unstructured{makeUIDObj("pod-a", "uid-a")}
	rl.SetObjects(objs)
	rl.PushNav(&testPlugin{}, []*unstructured.Unstructured{makeUIDObj("child", "uid-c")}, "parent", "uid", "", "")
	rl.ToggleSelect() // select child
	rl.PopNav()
	if rl.HasSelection() {
		t.Fatal("PopNav should clear selection")
	}
}

func TestResourceListScrollLeftRight(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 40, 10)
	// Use long names to force content wider than viewport
	objs := []*unstructured.Unstructured{
		makeObj("a-very-very-very-very-long-pod-name"),
	}
	rl.SetObjects(objs)

	// Content should exceed viewport width (40-2=38)
	rl.ScrollRight()
	if rl.xOffset != 8 {
		t.Fatalf("expected xOffset 8 after ScrollRight, got %d", rl.xOffset)
	}

	rl.ScrollLeft()
	if rl.xOffset != 0 {
		t.Fatalf("expected xOffset 0 after ScrollLeft, got %d", rl.xOffset)
	}

	// Should not go below 0
	rl.ScrollLeft()
	if rl.xOffset != 0 {
		t.Fatalf("expected xOffset clamped to 0, got %d", rl.xOffset)
	}
}

func TestResourceListScrollPreservedOnSetObjects(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 40, 10)
	objs := []*unstructured.Unstructured{
		makeObj("a-very-very-very-very-long-pod-name"),
	}
	rl.SetObjects(objs)
	rl.ScrollRight()
	if rl.xOffset == 0 {
		t.Fatal("xOffset should be non-zero after scroll")
	}

	// SetObjects should preserve xOffset
	saved := rl.xOffset
	rl.SetObjects(objs)
	if rl.xOffset != saved {
		t.Fatalf("expected xOffset preserved at %d on SetObjects, got %d", saved, rl.xOffset)
	}
}

func TestResourceListScrollResetOnSetPlugin(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 40, 10)
	objs := []*unstructured.Unstructured{
		makeObj("a-very-very-very-very-long-pod-name"),
	}
	rl.SetObjects(objs)
	rl.ScrollRight()

	rl.SetPlugin(&testPlugin{})
	if rl.xOffset != 0 {
		t.Fatalf("expected xOffset reset to 0 on SetPlugin, got %d", rl.xOffset)
	}
}

func TestResourceListScrollResetOnSetSize(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 40, 10)
	objs := []*unstructured.Unstructured{
		makeObj("a-very-very-very-very-long-pod-name"),
	}
	rl.SetObjects(objs)
	rl.ScrollRight()

	rl.SetSize(80, 20)
	if rl.xOffset != 0 {
		t.Fatalf("expected xOffset reset to 0 on SetSize, got %d", rl.xOffset)
	}
}

func TestColumnWidthsCachedWhenRowCountSame(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 15)
	objs := []*unstructured.Unstructured{makeObj("pod-a"), makeObj("pod-b")}
	rl.SetObjects(objs)

	// After SetObjects, the cache should be populated
	if rl.cachedColumnWidths == nil {
		t.Fatal("expected cachedColumnWidths to be populated after SetObjects")
	}
	if rl.cachedRowCount != 2 {
		t.Fatalf("expected cachedRowCount 2, got %d", rl.cachedRowCount)
	}
	savedCols := rl.cachedColumnWidths
	savedCW := rl.cachedContentWidth

	// Set objects with same count — cache should be reused (same slice pointer)
	objs2 := []*unstructured.Unstructured{makeObj("pod-c"), makeObj("pod-d")}
	rl.SetObjects(objs2)

	if rl.cachedRowCount != 2 {
		t.Fatalf("expected cachedRowCount still 2, got %d", rl.cachedRowCount)
	}
	// Column widths should be the same cached slice (pointer equality)
	if &rl.cachedColumnWidths[0] != &savedCols[0] {
		t.Fatal("expected cachedColumnWidths to be reused when row count is unchanged")
	}
	if rl.cachedContentWidth != savedCW {
		t.Fatalf("expected cachedContentWidth %d, got %d", savedCW, rl.cachedContentWidth)
	}
}

func TestColumnWidthsRecomputedWhenRowCountChanges(t *testing.T) {
	rl := NewResourceList(&testPlugin{}, 80, 15)
	objs := []*unstructured.Unstructured{makeObj("pod-a"), makeObj("pod-b")}
	rl.SetObjects(objs)

	if rl.cachedRowCount != 2 {
		t.Fatalf("expected cachedRowCount 2, got %d", rl.cachedRowCount)
	}
	savedCols := rl.cachedColumnWidths

	// Set objects with different count — cache should be invalidated and recomputed
	objs2 := []*unstructured.Unstructured{makeObj("pod-a"), makeObj("pod-b"), makeObj("pod-c-with-a-longer-name")}
	rl.SetObjects(objs2)

	if rl.cachedRowCount != 3 {
		t.Fatalf("expected cachedRowCount 3, got %d", rl.cachedRowCount)
	}
	// Column widths should have been recomputed (different slice)
	if &rl.cachedColumnWidths[0] == &savedCols[0] {
		t.Fatal("expected cachedColumnWidths to be recomputed when row count changes")
	}
}

