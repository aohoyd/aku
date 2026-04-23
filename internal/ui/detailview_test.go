package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/aohoyd/aku/internal/msgs"
	"github.com/aohoyd/aku/internal/render"
	"github.com/charmbracelet/x/ansi"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestDetailViewSetContent(t *testing.T) {
	dv := NewDetailView(40, 10)
	dv.SetMode(msgs.DetailYAML)
	s := "apiVersion: v1\nkind: Pod"
	dv.SetContent(render.Content{Raw: s, Display: s}, true)
	view := dv.View()
	if !strings.Contains(view, "apiVersion") {
		t.Fatal("detail view should contain YAML content")
	}
}

func TestDetailViewClearContent(t *testing.T) {
	dv := NewDetailView(40, 10)
	dv.SetMode(msgs.DetailLogs)
	s := "old line"
	dv.SetContent(render.Content{Raw: s, Display: s}, true)
	dv.ClearContent()
	view := dv.View()
	if strings.Contains(view, "old line") {
		t.Fatal("detail view should be clear after ClearContent")
	}
}

func TestDetailViewMode(t *testing.T) {
	dv := NewDetailView(40, 10)
	dv.SetMode(msgs.DetailDescribe)
	if dv.Mode() != msgs.DetailDescribe {
		t.Fatal("mode should be DetailDescribe")
	}
}

func TestDetailViewModeLabel(t *testing.T) {
	for _, tc := range []struct {
		mode msgs.DetailMode
		want string
	}{
		{msgs.DetailYAML, "YAML"},
		{msgs.DetailDescribe, "Describe"},
		{msgs.DetailLogs, "Logs"},
	} {
		dv := NewDetailView(60, 15)
		dv.SetMode(tc.mode)
		view := dv.View()
		stripped := ansi.Strip(view)
		if !strings.Contains(stripped, tc.want) {
			t.Fatalf("view in mode %d should contain %q in border", tc.mode, tc.want)
		}
	}
}

func TestDetailViewHeaderReflectsValuesMode(t *testing.T) {
	cases := []struct {
		name string
		mode msgs.DetailMode
		want string
	}{
		{"yaml", msgs.DetailYAML, "YAML"},
		{"values user", msgs.DetailValues, "Values (user)"},
		{"values all", msgs.DetailValuesAll, "Values (all)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dv := NewDetailView(60, 15)
			dv.SetMode(tc.mode)
			view := dv.View()
			stripped := ansi.Strip(view)
			if !strings.Contains(stripped, tc.want) {
				t.Fatalf("view for mode %v should contain %q in border, got:\n%s", tc.mode, tc.want, stripped)
			}
		})
	}
}

func TestDetailViewSetModeResetsContent(t *testing.T) {
	dv := NewDetailView(40, 10)
	dv.SetMode(msgs.DetailLogs)
	s := "some log"
	dv.SetContent(render.Content{Raw: s, Display: s}, true)
	dv.SetMode(msgs.DetailYAML) // switching mode should reset
	view := dv.View()
	if strings.Contains(view, "some log") {
		t.Fatal("switching mode should clear previous content")
	}
}

func TestSetContentPreservingKeepsPosition(t *testing.T) {
	dv := NewDetailView(80, 10)
	dv.SetMode(msgs.DetailYAML)

	// Build content taller than the viewport (viewport height = 10-3 = 7)
	lines := make([]string, 30)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %d: %s", i, strings.Repeat("x", 100))
	}
	content := strings.Join(lines, "\n")
	dv.SetContent(render.Content{Raw: content, Display: content}, true)

	// Scroll down by sending key events through the viewport
	for range 10 {
		dv, _ = dv.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}

	// Verify we scrolled past line 0
	view := dv.View()
	stripped := ansi.Strip(view)
	if strings.Contains(stripped, "line 0:") {
		t.Fatal("should have scrolled past line 0")
	}

	// Now call SetContent with refresh=false — position should not change
	dv.SetContent(render.Content{Raw: content, Display: content}, false)
	viewAfter := dv.View()
	strippedAfter := ansi.Strip(viewAfter)
	if strings.Contains(strippedAfter, "line 0:") {
		t.Fatal("SetContent with refresh=false should not reset scroll to top")
	}
}

func TestSetContentResetsPosition(t *testing.T) {
	dv := NewDetailView(80, 10)
	dv.SetMode(msgs.DetailYAML)

	lines := make([]string, 30)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %d", i)
	}
	content := strings.Join(lines, "\n")
	dv.SetContent(render.Content{Raw: content, Display: content}, true)

	// Scroll down
	for range 10 {
		dv, _ = dv.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}

	// SetContent with refresh=true should reset to top
	dv.SetContent(render.Content{Raw: content, Display: content}, true)
	view := dv.View()
	stripped := ansi.Strip(view)
	if !strings.Contains(stripped, "line 0") {
		t.Fatal("SetContent should reset scroll to top")
	}
}

func TestToggleWrap(t *testing.T) {
	dv := NewDetailView(30, 10)
	dv.SetMode(msgs.DetailYAML)
	s := "AAAAAAAABBBBBBBBCCCCCCCCDDDDDDDDEEEEEEEEFFFFFFFF"
	dv.SetContent(render.Content{Raw: s, Display: s}, true)

	// Initially wrap is off — scroll right should change the view
	dv.ScrollRight()
	viewScrolled := ansi.Strip(dv.View())

	// Enable wrap — should reset horizontal offset
	dv.ToggleWrap()
	viewWrapped := ansi.Strip(dv.View())
	if viewWrapped == viewScrolled {
		t.Fatal("ToggleWrap should reset horizontal scroll and change the view")
	}

	// ScrollRight should be a no-op when wrap is enabled
	dv.ScrollRight()
	viewAfterScroll := ansi.Strip(dv.View())
	if viewAfterScroll != viewWrapped {
		t.Fatal("ScrollRight should be a no-op when wrap is enabled")
	}

	// Toggle wrap off again
	dv.ToggleWrap()
	// Now ScrollRight should work again
	viewUnwrapped := ansi.Strip(dv.View())
	dv.ScrollRight()
	viewScrolledAgain := ansi.Strip(dv.View())
	if viewScrolledAgain == viewUnwrapped {
		t.Fatal("ScrollRight should work again after disabling wrap")
	}
}

func TestDetailViewApplySearch(t *testing.T) {
	dv := NewDetailView(80, 20)
	dv.SetMode(msgs.DetailYAML)

	lines := make([]string, 30)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %d: some content here", i)
	}
	s := strings.Join(lines, "\n")
	dv.SetContent(render.Content{Raw: s, Display: s}, true)

	err := dv.ApplySearch("content", msgs.SearchModeSearch)
	if err != nil {
		t.Fatalf("ApplySearch should not error: %v", err)
	}
	if !dv.SearchActive() {
		t.Fatal("search should be active")
	}
}

func TestDetailViewApplyFilter(t *testing.T) {
	dv := NewDetailView(80, 20)
	dv.SetMode(msgs.DetailYAML)
	s := "line one\nline two\nline three"
	dv.SetContent(render.Content{Raw: s, Display: s}, true)

	err := dv.ApplySearch("two", msgs.SearchModeFilter)
	if err != nil {
		t.Fatalf("ApplyFilter should not error: %v", err)
	}
	if !dv.FilterActive() {
		t.Fatal("filter should be active after filter")
	}
}

func TestDetailViewClearSearch(t *testing.T) {
	dv := NewDetailView(80, 20)
	dv.SetMode(msgs.DetailYAML)
	s := "line one\nline two"
	dv.SetContent(render.Content{Raw: s, Display: s}, true)

	dv.ApplySearch("one", msgs.SearchModeFilter)
	dv.ClearFilter()
	if dv.AnyActive() {
		t.Fatal("should be inactive after clear")
	}
}

func TestDetailViewSearchInvalidRegex(t *testing.T) {
	dv := NewDetailView(80, 20)
	dv.SetMode(msgs.DetailYAML)
	s := "some content"
	dv.SetContent(render.Content{Raw: s, Display: s}, true)

	err := dv.ApplySearch("[invalid", msgs.SearchModeSearch)
	if err == nil {
		t.Fatal("invalid regex should return error")
	}
}

func TestDetailViewSetContentReappliesSearch(t *testing.T) {
	dv := NewDetailView(80, 20)
	dv.SetMode(msgs.DetailYAML)
	s := "line one\nline two"
	dv.SetContent(render.Content{Raw: s, Display: s}, true)

	dv.ApplySearch("one", msgs.SearchModeFilter)
	// SetContent with new data should re-apply filter
	s2 := "line one\nline two\nline one-more"
	dv.SetContent(render.Content{Raw: s2, Display: s2}, true)
	// The filter should still be active
	if !dv.FilterActive() {
		t.Fatal("filter should remain active after SetContent")
	}
}

func TestDetailViewSetContentPreservingReappliesSearch(t *testing.T) {
	dv := NewDetailView(80, 20)
	dv.SetMode(msgs.DetailYAML)
	s := "line one\nline two"
	dv.SetContent(render.Content{Raw: s, Display: s}, true)

	dv.ApplySearch("one", msgs.SearchModeSearch)
	// SetContent with refresh=false should re-apply search highlights
	s2 := "line one\nline two\nline three"
	dv.SetContent(render.Content{Raw: s2, Display: s2}, false)
	if !dv.SearchActive() {
		t.Fatal("search should remain active after SetContent with refresh=false")
	}
}

func TestDetailViewSetModeClearsSearch(t *testing.T) {
	dv := NewDetailView(80, 20)
	dv.SetMode(msgs.DetailYAML)
	s := "content"
	dv.SetContent(render.Content{Raw: s, Display: s}, true)
	dv.ApplySearch("content", msgs.SearchModeSearch)

	dv.SetMode(msgs.DetailDescribe)
	if dv.AnyActive() {
		t.Fatal("SetMode should clear all search state")
	}
}

func TestDetailViewFilterAndSearchIndependent(t *testing.T) {
	dv := NewDetailView(80, 20)
	dv.SetMode(msgs.DetailYAML)
	s := "nginx line one\nnginx line two\nredis line three"
	dv.SetContent(render.Content{Raw: s, Display: s}, true)

	// Apply filter
	dv.ApplySearch("nginx", msgs.SearchModeFilter)
	if !dv.FilterActive() {
		t.Fatal("filter should be active")
	}

	// Apply search on top
	err := dv.ApplySearch("two", msgs.SearchModeSearch)
	if err != nil {
		t.Fatalf("search should not error: %v", err)
	}
	if !dv.SearchActive() {
		t.Fatal("search should be active")
	}
	if !dv.FilterActive() {
		t.Fatal("filter should still be active")
	}
	if !dv.AnyActive() {
		t.Fatal("AnyActive should be true")
	}
}

func TestDetailViewLayeredClear(t *testing.T) {
	dv := NewDetailView(80, 20)
	dv.SetMode(msgs.DetailYAML)
	s := "nginx one\nnginx two\nredis three"
	dv.SetContent(render.Content{Raw: s, Display: s}, true)

	dv.ApplySearch("nginx", msgs.SearchModeFilter)
	dv.ApplySearch("two", msgs.SearchModeSearch)

	// Clear search first
	dv.ClearSearch()
	if dv.SearchActive() {
		t.Fatal("search should be cleared")
	}
	if !dv.FilterActive() {
		t.Fatal("filter should still be active")
	}

	// Clear filter
	dv.ClearFilter()
	if dv.FilterActive() {
		t.Fatal("filter should be cleared")
	}
	if dv.AnyActive() {
		t.Fatal("nothing should be active")
	}
}

func TestScrollLeftRight(t *testing.T) {
	dv := NewDetailView(30, 10)
	dv.SetMode(msgs.DetailYAML)
	// Non-repeating long line so horizontal scroll changes visible content
	s := "AAAAAAAABBBBBBBBCCCCCCCCDDDDDDDDEEEEEEEEFFFFFFFF"
	dv.SetContent(render.Content{Raw: s, Display: s}, true)

	viewBefore := ansi.Strip(dv.View())

	dv.ScrollRight()
	viewAfterRight := ansi.Strip(dv.View())
	if viewAfterRight == viewBefore {
		t.Fatal("ScrollRight should change the rendered view")
	}

	dv.ScrollLeft()
	viewAfterLeftBack := ansi.Strip(dv.View())
	if viewAfterLeftBack != viewBefore {
		t.Fatal("ScrollRight then ScrollLeft should return to original view")
	}
}

func TestDetailViewSetObject(t *testing.T) {
	dv := NewDetailView(80, 20)
	dv.SetMode(msgs.DetailYAML)
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata":   map[string]any{"name": "nginx"},
	}}
	c, _ := render.YAML(obj.Object)
	dv.SetContent(c, true)
	view := dv.View()
	stripped := ansi.Strip(view)
	if !strings.Contains(stripped, "apiVersion:") {
		t.Fatal("view should contain YAML key")
	}
	if !strings.Contains(stripped, "nginx") {
		t.Fatal("view should contain value")
	}
	// The raw view (with ANSI) should be longer than stripped due to color codes
	if len(view) <= len(stripped) {
		t.Fatal("colored view should have ANSI codes making it longer")
	}
}

func TestDetailViewSetObjectPreserving(t *testing.T) {
	dv := NewDetailView(80, 10)
	dv.SetMode(msgs.DetailYAML)
	m := map[string]any{}
	for i := range 50 {
		m[fmt.Sprintf("field%02d", i)] = fmt.Sprintf("value%d", i)
	}
	obj := &unstructured.Unstructured{Object: m}
	c, _ := render.YAML(obj.Object)
	dv.SetContent(c, true)

	// Scroll down
	for range 10 {
		dv, _ = dv.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}

	// SetContent with refresh=false should keep scroll position
	c, _ = render.YAML(obj.Object)
	dv.SetContent(c, false)
	view := dv.View()
	stripped := ansi.Strip(view)
	if strings.Contains(stripped, "field00:") {
		t.Fatal("SetContent with refresh=false should maintain scroll position")
	}
}

func TestDetailViewSetObjectSearchCompatibility(t *testing.T) {
	dv := NewDetailView(80, 20)
	dv.SetMode(msgs.DetailYAML)
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
	}}
	c, _ := render.YAML(obj.Object)
	dv.SetContent(c, true)

	err := dv.ApplySearch("Pod", msgs.SearchModeSearch)
	if err != nil {
		t.Fatalf("search should not error: %v", err)
	}
	if !dv.SearchActive() {
		t.Fatal("search should be active on colored content")
	}
}

func TestDetailViewYAMLSearchHighlightPosition(t *testing.T) {
	dv := NewDetailView(80, 20)
	dv.SetMode(msgs.DetailYAML)
	obj := &unstructured.Unstructured{Object: map[string]any{
		"spec": map[string]any{
			"containers": []any{
				map[string]any{
					"image": "admission-controller-v0.1.30",
					"name":  "admission",
				},
			},
		},
	}}
	c, _ := render.YAML(obj.Object)
	dv.SetContent(c, true)

	err := dv.ApplySearch("admi", msgs.SearchModeSearch)
	if err != nil {
		t.Fatalf("search should not error: %v", err)
	}

	// Verify match positions are computed correctly
	if len(dv.matchPositions) == 0 {
		t.Fatal("should have match positions for YAML search")
	}

	// For each match, verify the position actually corresponds to "admi"
	rawLines := strings.Split(dv.rawContent, "\n")
	for i, pos := range dv.matchPositions {
		if pos.line >= len(rawLines) {
			t.Fatalf("match %d: line %d out of range", i, pos.line)
		}
		line := rawLines[pos.line]
		// Extract the matched text using display-column positions via ansi.Cut,
		// consistent with how the runtime and other tests extract matches.
		matched := ansi.Strip(ansi.Cut(line, pos.colStart, pos.colEnd))
		if matched != "admi" {
			t.Fatalf("match %d on line %d: expected 'admi' at cols [%d,%d], got %q (line: %q)",
				i, pos.line, pos.colStart, pos.colEnd, matched, line)
		}
	}
}

func TestDetailViewSearchHighlightPositionMultiByte(t *testing.T) {
	// Verify that computeMatchPositions produces correct grapheme-column
	// positions for content containing multi-byte UTF-8 characters.
	// We enable softWrap so that reapplySearch populates matchPositions
	// even for plain text content (the plain-text viewport path does not
	// populate matchPositions).
	dv := NewDetailView(80, 20)
	dv.SetMode(msgs.DetailDescribe)

	// Line with multi-byte characters: "prefix_\u4e16\u754c_target_end"
	// \u4e16 and \u754c are CJK chars (each 3 bytes, 2 display columns).
	// Byte layout: "prefix_" (7) + "\u4e16" (3) + "\u754c" (3) + "_target_end" (11) = 24 bytes
	// Display cols: "prefix_" (7) + "\u4e16" (2) + "\u754c" (2) + "_target_end" (11) = 22 cols
	// "target" starts at display col 7+2+2+1 = 12, ends at 12+6 = 18.
	content := "prefix_\u4e16\u754c_target_end"
	dv.SetContent(render.Content{Raw: content, Display: content}, true)
	dv.ToggleWrap() // enable softWrap so matchPositions is populated

	err := dv.ApplySearch("target", msgs.SearchModeSearch)
	if err != nil {
		t.Fatalf("ApplySearch should not error: %v", err)
	}

	if len(dv.matchPositions) != 1 {
		t.Fatalf("expected 1 match, got %d", len(dv.matchPositions))
	}

	pos := dv.matchPositions[0]
	if pos.line != 0 {
		t.Fatalf("expected match on line 0, got %d", pos.line)
	}
	// "target" starts at grapheme-column 12 (7 ASCII + 2 CJK widths + 2 CJK widths + 1 underscore).
	if pos.colStart != 12 || pos.colEnd != 18 {
		t.Fatalf("expected cols [12,18], got [%d,%d]", pos.colStart, pos.colEnd)
	}

	// Verify by extracting from the raw line using ansi.Cut (display-column based).
	rawLine := content
	extracted := ansi.Strip(ansi.Cut(rawLine, pos.colStart, pos.colEnd))
	if extracted != "target" {
		t.Fatalf("ansi.Cut at match position should yield %q, got %q", "target", extracted)
	}
}

func TestDetailViewYAMLSearchNavigation(t *testing.T) {
	dv := NewDetailView(80, 20)
	dv.SetMode(msgs.DetailYAML)
	obj := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{
			"name":      "test",
			"namespace": "default",
		},
	}}
	c, _ := render.YAML(obj.Object)
	dv.SetContent(c, true)

	err := dv.ApplySearch("test|default", msgs.SearchModeSearch)
	if err != nil {
		t.Fatalf("search should not error: %v", err)
	}
	if len(dv.matchPositions) < 2 {
		t.Fatalf("expected at least 2 matches, got %d", len(dv.matchPositions))
	}

	// Navigate forward
	initialIdx := dv.matchIndex
	dv.SearchNext()
	if dv.matchIndex == initialIdx {
		t.Fatal("SearchNext should advance matchIndex")
	}

	// Navigate backward
	dv.SearchPrev()
	if dv.matchIndex != initialIdx {
		t.Fatal("SearchPrev should return to initial matchIndex")
	}
}

func TestComputeMatchPositions(t *testing.T) {
	raw := "apiVersion: v1\nkind: Pod\nmetadata:\n  name: test\n"
	matches := [][]int{
		{0, 4},   // "apiV" on line 0
		{15, 18}, // "kin" on line 1 (byte 15 is 'k' after "apiVersion: v1\n")
		{43, 47}, // "test" on line 3 (line starts at byte 35, "test" at position 8)
	}
	positions := computeMatchPositions(raw, matches)
	if len(positions) != 3 {
		t.Fatalf("expected 3 positions, got %d", len(positions))
	}

	// Match 0: "apiV" at line 0, cols 0-4
	if positions[0].line != 0 || positions[0].colStart != 0 || positions[0].colEnd != 4 {
		t.Fatalf("match 0: expected line=0 cols=[0,4], got line=%d cols=[%d,%d]",
			positions[0].line, positions[0].colStart, positions[0].colEnd)
	}

	// Match 1: "kin" at line 1, cols 0-3
	if positions[1].line != 1 || positions[1].colStart != 0 || positions[1].colEnd != 3 {
		t.Fatalf("match 1: expected line=1 cols=[0,3], got line=%d cols=[%d,%d]",
			positions[1].line, positions[1].colStart, positions[1].colEnd)
	}

	// Match 2: "test" at line 3, cols 8-12
	if positions[2].line != 3 || positions[2].colStart != 8 || positions[2].colEnd != 12 {
		t.Fatalf("match 2: expected line=3 cols=[8,12], got line=%d cols=[%d,%d]",
			positions[2].line, positions[2].colStart, positions[2].colEnd)
	}
}

func TestComputeMatchPositionsMultiLineMatch(t *testing.T) {
	// Simulate a (?s)-style cross-line match (e.g. "foo.*bar" with DotAll, or
	// an explicit "\n" in the pattern). The match spans from byte 4 ("foo"
	// end position 4) in line 0 all the way to byte 11 (past "bar" on line 1).
	// Expect one matchPosition per line the match touches.
	//
	// Raw layout:
	//   line 0: "prefoo"  (bytes 0..6)
	//   line 1: "bar_end"  (bytes 7..14)
	// Match: start=3 (f of "foo"), end=10 (after "bar").
	raw := "prefoo\nbar_end"
	matches := [][]int{{3, 10}}

	positions := computeMatchPositions(raw, matches)
	if len(positions) != 2 {
		t.Fatalf("expected 2 positions for cross-line match, got %d", len(positions))
	}

	// First segment: line 0, colStart=3 ("pre"|"foo"), colEnd=6 (end of line).
	if positions[0].line != 0 || positions[0].colStart != 3 || positions[0].colEnd != 6 {
		t.Fatalf("segment 0: expected line=0 cols=[3,6], got line=%d cols=[%d,%d]",
			positions[0].line, positions[0].colStart, positions[0].colEnd)
	}
	// Second segment: line 1, colStart=0, colEnd=3 ("bar").
	if positions[1].line != 1 || positions[1].colStart != 0 || positions[1].colEnd != 3 {
		t.Fatalf("segment 1: expected line=1 cols=[0,3], got line=%d cols=[%d,%d]",
			positions[1].line, positions[1].colStart, positions[1].colEnd)
	}
}

func TestComputeMatchPositionsMultiLineMatchSpansThreeLines(t *testing.T) {
	// Match that spans three lines entirely consumes the middle line.
	//   line 0: "aaa"  (bytes 0..3)
	//   line 1: "bbb"  (bytes 4..7)
	//   line 2: "ccc"  (bytes 8..11)
	// Match: start=1 (second 'a'), end=10 (second 'c', inclusive up to col 2).
	raw := "aaa\nbbb\nccc"
	matches := [][]int{{1, 10}}

	positions := computeMatchPositions(raw, matches)
	if len(positions) != 3 {
		t.Fatalf("expected 3 positions for 3-line match, got %d", len(positions))
	}
	if positions[0].line != 0 || positions[0].colStart != 1 || positions[0].colEnd != 3 {
		t.Fatalf("segment 0: expected line=0 cols=[1,3], got line=%d cols=[%d,%d]",
			positions[0].line, positions[0].colStart, positions[0].colEnd)
	}
	if positions[1].line != 1 || positions[1].colStart != 0 || positions[1].colEnd != 3 {
		t.Fatalf("segment 1: expected line=1 cols=[0,3], got line=%d cols=[%d,%d]",
			positions[1].line, positions[1].colStart, positions[1].colEnd)
	}
	if positions[2].line != 2 || positions[2].colStart != 0 || positions[2].colEnd != 2 {
		t.Fatalf("segment 2: expected line=2 cols=[0,2], got line=%d cols=[%d,%d]",
			positions[2].line, positions[2].colStart, positions[2].colEnd)
	}
}

func TestDetailViewSetLoading(t *testing.T) {
	dv := NewDetailView(60, 15)
	dv.SetMode(msgs.DetailDescribe)

	dv.SetLoading(true)
	if !dv.Loading() {
		t.Fatal("Loading() should be true after SetLoading(true)")
	}
	if dv.LoadErr() != "" {
		t.Fatal("LoadErr() should be empty after SetLoading(true)")
	}
}

func TestDetailViewSetLoadError(t *testing.T) {
	dv := NewDetailView(60, 15)
	dv.SetMode(msgs.DetailDescribe)

	dv.SetLoadError("timeout")
	if dv.Loading() {
		t.Fatal("Loading() should be false after SetLoadError")
	}
	if dv.LoadErr() != "timeout" {
		t.Fatalf("LoadErr() = %q, want %q", dv.LoadErr(), "timeout")
	}
}

func TestDetailViewSetContentClearsLoading(t *testing.T) {
	dv := NewDetailView(60, 15)
	dv.SetMode(msgs.DetailDescribe)

	dv.SetLoading(true)
	dv.SetLoadError("some error")
	s := "real content"
	dv.SetContent(render.Content{Raw: s, Display: s}, true)

	if dv.Loading() {
		t.Fatal("Loading() should be false after SetContent")
	}
	if dv.LoadErr() != "" {
		t.Fatalf("LoadErr() should be empty after SetContent, got %q", dv.LoadErr())
	}
}

func TestDetailViewLoadingThenError(t *testing.T) {
	dv := NewDetailView(60, 15)
	dv.SetMode(msgs.DetailDescribe)

	dv.SetLoading(true)
	if !dv.Loading() {
		t.Fatal("Loading() should be true")
	}

	dv.SetLoadError("x")
	if dv.Loading() {
		t.Fatal("Loading() should be false after SetLoadError")
	}
	if dv.LoadErr() != "x" {
		t.Fatalf("LoadErr() = %q, want %q", dv.LoadErr(), "x")
	}
}

func TestDetailViewSetObjectFilterCompatibility(t *testing.T) {
	dv := NewDetailView(80, 20)
	dv.SetMode(msgs.DetailYAML)
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata":   map[string]any{"name": "nginx"},
	}}
	c, _ := render.YAML(obj.Object)
	dv.SetContent(c, true)

	err := dv.ApplySearch("kind", msgs.SearchModeFilter)
	if err != nil {
		t.Fatalf("filter should not error: %v", err)
	}
	if !dv.FilterActive() {
		t.Fatal("filter should be active")
	}
	view := dv.View()
	stripped := ansi.Strip(view)
	if strings.Contains(stripped, "apiVersion") {
		t.Fatal("filtered view should not contain non-matching lines")
	}
}

func TestDetailViewShowHeaderDefault(t *testing.T) {
	dv := NewDetailView(80, 20)
	if !dv.ShowHeader() {
		t.Fatal("ShowHeader should default to true")
	}
}

func TestDetailViewHeaderShownInBorderlessMode(t *testing.T) {
	dv := NewDetailView(80, 20)
	dv.SetMode(msgs.DetailYAML)
	dv.SetBorderless(true)
	dv.SetSize(80, 20)
	s := "apiVersion: v1\nkind: Pod"
	dv.SetContent(render.Content{Raw: s, Display: s}, true)

	view := dv.View()
	stripped := ansi.Strip(view)
	if !strings.Contains(stripped, "YAML") {
		t.Fatal("borderless view with showHeader=true should contain the mode name in the header")
	}
	if !strings.Contains(stripped, "apiVersion") {
		t.Fatal("borderless view with showHeader=true should still contain content")
	}
}

func TestDetailViewHeaderHiddenWhenToggledOff(t *testing.T) {
	dv := NewDetailView(80, 20)
	dv.SetMode(msgs.DetailYAML)
	dv.SetBorderless(true)
	dv.SetSize(80, 20)
	s := "apiVersion: v1\nkind: Pod"
	dv.SetContent(render.Content{Raw: s, Display: s}, true)

	dv.ToggleHeader()
	if dv.ShowHeader() {
		t.Fatal("ShowHeader should be false after ToggleHeader")
	}

	view := dv.View()
	stripped := ansi.Strip(view)
	// The mode name should NOT appear when header is off in borderless mode
	lines := strings.Split(stripped, "\n")
	if len(lines) > 0 && strings.Contains(lines[0], "YAML") {
		t.Fatal("borderless view with showHeader=false should not have a header line with mode name")
	}
	if !strings.Contains(stripped, "apiVersion") {
		t.Fatal("borderless view with showHeader=false should still contain content")
	}
}

func TestDetailViewSetSizeViewportHeightWithHeader(t *testing.T) {
	dv := NewDetailView(80, 20)
	dv.SetBorderless(true)
	dv.SetMode(msgs.DetailYAML)

	// With header (default), viewport height should be h-1
	dv.SetSize(80, 20)
	// Build enough content to verify viewport height matters
	lines := make([]string, 30)
	for i := range lines {
		lines[i] = fmt.Sprintf("line %d", i)
	}
	content := strings.Join(lines, "\n")
	dv.SetContent(render.Content{Raw: content, Display: content}, true)

	viewWithHeader := dv.View()
	strippedWithHeader := ansi.Strip(viewWithHeader)

	// Count content lines (lines containing "line NN" pattern)
	contentLinesWithHeader := 0
	for _, l := range strings.Split(strippedWithHeader, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "line ") {
			contentLinesWithHeader++
		}
	}

	// Toggle header off, viewport height should be h (one more row for content)
	dv.ToggleHeader()
	dv.SetContent(render.Content{Raw: content, Display: content}, true)
	viewNoHeader := dv.View()
	strippedNoHeader := ansi.Strip(viewNoHeader)

	contentLinesNoHeader := 0
	for _, l := range strings.Split(strippedNoHeader, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "line ") {
			contentLinesNoHeader++
		}
	}

	// Without header should show one more content line than with header
	if contentLinesNoHeader != contentLinesWithHeader+1 {
		t.Fatalf("no-header view should show 1 more content line: got %d (no header) vs %d (with header)\nwith header:\n%s\nno header:\n%s",
			contentLinesNoHeader, contentLinesWithHeader, strippedWithHeader, strippedNoHeader)
	}
}

func TestDetailViewToggleHeaderDoubleToggle(t *testing.T) {
	dv := NewDetailView(80, 20)
	if !dv.ShowHeader() {
		t.Fatal("initial ShowHeader should be true")
	}
	dv.ToggleHeader()
	if dv.ShowHeader() {
		t.Fatal("ShowHeader should be false after first toggle")
	}
	dv.ToggleHeader()
	if !dv.ShowHeader() {
		t.Fatal("ShowHeader should be true after second toggle")
	}
}

func TestDetailViewWrapSearchCoordinate(t *testing.T) {
	// Viewport width = 22-2 = 20, height = 6-2 = 4.
	// With 4 visible rows and content that wraps, a match on a continuation
	// row that is beyond the viewport must be scrolled into view.
	dv := NewDetailView(22, 6)
	dv.SetMode(msgs.DetailDescribe)

	// Build content: 4 short lines (fill viewport), then one long line that
	// wraps, containing the search term on the continuation row.
	// Logical lines:
	//   0: "line0"
	//   1: "line1"
	//   2: "line2"
	//   3: "line3"
	//   4: "abcdefghijklmnopqrstTARGEThere" (30 chars)
	// With viewport width 20, line 4 wraps:
	//   visual row 4: "abcdefghijklmnopqrst" (20 chars)
	//   visual row 5: "↪ RGEThere" (colOffset=20, "TA" replaced by "↪ ")
	// "TARGET" is at logical col 20-26 on line 4.
	// On visual row 5: visualCol = 20-20=0..6-20=6 but colOffset>0 so clamp start to 2.
	// Actually: colStart=20, colEnd=26, row colOffset=20
	//   visualColStart = 20-20 = 0, visualColEnd = 26-20 = 6
	//   Since colOffset>0 and colStart(0) < wrapIndicatorWidth(2), clamp to 2.
	// So EnsureVisible(5, 2, 6).
	content := "line0\nline1\nline2\nline3\nabcdefghijklmnopqrstTARGEThere"
	dv.SetContent(render.Content{Raw: content, Display: content}, true)
	dv.ToggleWrap()

	err := dv.ApplySearch("TARGET", msgs.SearchModeSearch)
	if err != nil {
		t.Fatalf("ApplySearch should not error: %v", err)
	}
	if len(dv.matchPositions) != 1 {
		t.Fatalf("expected 1 match, got %d", len(dv.matchPositions))
	}

	// Verify match is at logical line 4, cols 20-26.
	pos := dv.matchPositions[0]
	if pos.line != 4 || pos.colStart != 20 || pos.colEnd != 26 {
		t.Fatalf("expected logical match at line=4 cols=[20,26], got line=%d cols=[%d,%d]",
			pos.line, pos.colStart, pos.colEnd)
	}

	// SearchNext should scroll the viewport so that visual row 5 is visible.
	dv.SearchNext()

	// The viewport should have scrolled down — visual row 5 must be in view.
	// With viewport height 4, if YOffset is e.g. 2, rows 2-5 are visible.
	yOff := dv.viewport.YOffset()
	vpHeight := dv.viewport.Height()
	if yOff > 5 || yOff+vpHeight <= 5 {
		t.Fatalf("after SearchNext, visual row 5 should be visible: YOffset=%d, Height=%d", yOff, vpHeight)
	}
}

func TestDetailViewWrapSearchOnFirstSegment(t *testing.T) {
	// Verify that a match on the first segment of a wrapped line (no indicator)
	// is converted correctly.
	dv := NewDetailView(22, 6)
	dv.SetMode(msgs.DetailDescribe)

	// Line 0: "TARGETabcdefghijklmnopqrstuvwx" (30 chars)
	// Wraps at width 20:
	//   visual row 0: "TARGETabcdefghijklmn" (colOffset=0)
	//   visual row 1: "↪ rstuvwx"            (colOffset=20)
	// "TARGET" at logical col 0-6, on visual row 0 col 0-6.
	content := "TARGETabcdefghijklmnopqrstuvwx"
	dv.SetContent(render.Content{Raw: content, Display: content}, true)
	dv.ToggleWrap()

	err := dv.ApplySearch("TARGET", msgs.SearchModeSearch)
	if err != nil {
		t.Fatalf("ApplySearch should not error: %v", err)
	}
	if len(dv.matchPositions) != 1 {
		t.Fatalf("expected 1 match, got %d", len(dv.matchPositions))
	}

	// Verify logical coordinates.
	pos := dv.matchPositions[0]
	if pos.line != 0 || pos.colStart != 0 || pos.colEnd != 6 {
		t.Fatalf("expected logical match at line=0 cols=[0,6], got line=%d cols=[%d,%d]",
			pos.line, pos.colStart, pos.colEnd)
	}

	// logicalToVisual should map to visual row 0, cols 0-6 (no indicator).
	vLine, vColStart, vColEnd := dv.logicalToVisual(pos)
	if vLine != 0 || vColStart != 0 || vColEnd != 6 {
		t.Fatalf("expected visual line=0 cols=[0,6], got line=%d cols=[%d,%d]",
			vLine, vColStart, vColEnd)
	}
}

func TestDetailViewWrapSearchBoundaryClamp(t *testing.T) {
	// Edge case: match overlaps the wrap indicator region.
	dv := NewDetailView(22, 6)
	dv.SetMode(msgs.DetailDescribe)

	// Line: "abcdefghijklmnopqrstTXyz..." (wraps at 20)
	// "TX" starts at logical col 20. On continuation row (colOffset=20):
	//   visualColStart = 20-20 = 0, but the first 2 cols are the "↪ " indicator.
	//   logicalToVisual should clamp colStart to wrapIndicatorWidth (2).
	content := "abcdefghijklmnopqrstTXyzabcdefghij"
	dv.SetContent(render.Content{Raw: content, Display: content}, true)
	dv.ToggleWrap()

	err := dv.ApplySearch("TX", msgs.SearchModeSearch)
	if err != nil {
		t.Fatalf("ApplySearch should not error: %v", err)
	}
	if len(dv.matchPositions) != 1 {
		t.Fatalf("expected 1 match, got %d", len(dv.matchPositions))
	}

	pos := dv.matchPositions[0]
	// Logical: line 0, cols 20-22
	if pos.line != 0 || pos.colStart != 20 || pos.colEnd != 22 {
		t.Fatalf("expected logical match at line=0 cols=[20,22], got line=%d cols=[%d,%d]",
			pos.line, pos.colStart, pos.colEnd)
	}

	// logicalToVisual: continuation row has colOffset=20.
	// visualColStart = 20-20 + wrapIndicatorWidth = 0 + 2 = 2.
	// visualColEnd = 22-20 + wrapIndicatorWidth = 2 + 2 = 4.
	vLine, vColStart, vColEnd := dv.logicalToVisual(pos)
	if vColStart != wrapIndicatorWidth {
		t.Fatalf("expected visual colStart=%d, got %d", wrapIndicatorWidth, vColStart)
	}
	if vColEnd != wrapIndicatorWidth+2 {
		t.Fatalf("expected visual colEnd=%d, got %d", wrapIndicatorWidth+2, vColEnd)
	}
	// Visual row should be the continuation row (row 1 for single-line content).
	if vLine != 1 {
		t.Fatalf("expected visual line=1, got %d", vLine)
	}
}

func TestDetailViewWrapSearchPrevNavigatesCorrectly(t *testing.T) {
	// Verify SearchPrev also uses visual coordinates when wrapping.
	dv := NewDetailView(22, 6)
	dv.SetMode(msgs.DetailDescribe)

	// Two matches: one on a short line visible at top, one on a wrapped
	// continuation row below the viewport.
	content := "MATCHline\nline1\nline2\nline3\nabcdefghijklmnopqrstMATCHhere"
	dv.SetContent(render.Content{Raw: content, Display: content}, true)
	dv.ToggleWrap()

	err := dv.ApplySearch("MATCH", msgs.SearchModeSearch)
	if err != nil {
		t.Fatalf("ApplySearch should not error: %v", err)
	}
	if len(dv.matchPositions) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(dv.matchPositions))
	}

	// Navigate to second match (on wrapped continuation row).
	dv.SearchNext() // matchIndex 0 -> 1
	dv.SearchNext() // matchIndex 1 -> 0 (wraps around)

	// Now SearchPrev should go to match 1 (the continuation row match).
	dv.SearchPrev() // matchIndex 0 -> 1

	// Visual row for second match should be visible.
	yOff := dv.viewport.YOffset()
	vpHeight := dv.viewport.Height()

	// The second match is on logical line 4, col 20-25.
	// With wrapping, line 4 wraps to visual rows 4 and 5.
	// "MATCH" at logical col 20 is on visual row 5.
	if yOff > 5 || yOff+vpHeight <= 5 {
		t.Fatalf("after SearchPrev to wrapped match, visual row 5 should be visible: YOffset=%d, Height=%d",
			yOff, vpHeight)
	}
}

func TestDetailViewWrapSearchPlainTextBakesHighlights(t *testing.T) {
	// When softWrap is active, even plain text (raw == display) should use
	// baked highlights (matchPositions) rather than viewport.SetHighlights,
	// because the viewport's highlight byte ranges don't align with wrapped content.
	dv := NewDetailView(22, 10)
	dv.SetMode(msgs.DetailDescribe)

	content := "short\nabcdefghijklmnopqrstMATCHhere"
	dv.SetContent(render.Content{Raw: content, Display: content}, true)
	dv.ToggleWrap()

	err := dv.ApplySearch("MATCH", msgs.SearchModeSearch)
	if err != nil {
		t.Fatalf("ApplySearch should not error: %v", err)
	}

	// With softWrap, matchPositions should be populated even for plain text.
	if len(dv.matchPositions) == 0 {
		t.Fatal("softWrap + plain text search should populate matchPositions")
	}
	if dv.matchIndex != 0 {
		t.Fatalf("expected matchIndex=0, got %d", dv.matchIndex)
	}
}

// scrollableDetailView returns a DetailView with enough content that the
// viewport can be scrolled vertically.
func scrollableDetailView(t *testing.T) *DetailView {
	t.Helper()
	dv := NewDetailView(40, 6)
	dv.SetMode(msgs.DetailYAML)
	var lines []string
	for i := range 30 {
		lines = append(lines, fmt.Sprintf("line-%02d", i))
	}
	content := strings.Join(lines, "\n")
	dv.SetContent(render.Content{Raw: content, Display: content}, true)
	return &dv
}

func TestDetailViewScrollWheelDownIncreasesYOffset(t *testing.T) {
	dv := scrollableDetailView(t)
	before := dv.viewport.YOffset()
	dv.ScrollWheel(tea.MouseWheelDown)
	if got := dv.viewport.YOffset(); got <= before {
		t.Fatalf("wheel down: YOffset before=%d after=%d (expected increase)", before, got)
	}
}

func TestDetailViewScrollWheelUpAtTopStays(t *testing.T) {
	dv := scrollableDetailView(t)
	dv.viewport.GotoTop()
	if y := dv.viewport.YOffset(); y != 0 {
		t.Fatalf("expected YOffset 0 before wheel up, got %d", y)
	}
	dv.ScrollWheel(tea.MouseWheelUp)
	if got := dv.viewport.YOffset(); got != 0 {
		t.Fatalf("wheel up at top: YOffset expected 0, got %d", got)
	}
}

func TestDetailViewScrollWheelLeftRightNoOp(t *testing.T) {
	dv := scrollableDetailView(t)
	// Scroll down once so YOffset > 0, then left/right must be a no-op.
	dv.ScrollWheel(tea.MouseWheelDown)
	yBefore := dv.viewport.YOffset()
	xBefore := dv.viewport.XOffset()

	dv.ScrollWheel(tea.MouseWheelLeft)
	if got := dv.viewport.YOffset(); got != yBefore {
		t.Fatalf("wheel left changed YOffset: before=%d after=%d", yBefore, got)
	}
	if got := dv.viewport.XOffset(); got != xBefore {
		t.Fatalf("wheel left changed XOffset: before=%d after=%d", xBefore, got)
	}

	dv.ScrollWheel(tea.MouseWheelRight)
	if got := dv.viewport.YOffset(); got != yBefore {
		t.Fatalf("wheel right changed YOffset: before=%d after=%d", yBefore, got)
	}
	if got := dv.viewport.XOffset(); got != xBefore {
		t.Fatalf("wheel right changed XOffset: before=%d after=%d", xBefore, got)
	}
}
