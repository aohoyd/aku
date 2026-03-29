package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/aohoyd/aku/internal/msgs"
)

func TestSearchBarInitiallyInactive(t *testing.T) {
	sb := NewSearchBar(80)
	if sb.Active() {
		t.Fatal("search bar should be inactive initially")
	}
	if sb.View() != "" {
		t.Fatal("inactive search bar should render empty")
	}
}

func TestSearchBarOpenSearch(t *testing.T) {
	sb := NewSearchBar(80)
	sb.Open(msgs.SearchModeSearch)
	if !sb.Active() {
		t.Fatal("should be active after Open")
	}
	view := sb.View()
	if view == "" {
		t.Fatal("active search bar should render something")
	}
}

func TestSearchBarOpenFilter(t *testing.T) {
	sb := NewSearchBar(80)
	sb.Open(msgs.SearchModeFilter)
	if !sb.Active() {
		t.Fatal("should be active after Open")
	}
}

func TestSearchBarEnterSubmits(t *testing.T) {
	sb := NewSearchBar(80)
	sb.Open(msgs.SearchModeSearch)
	sb.SetValue("test.*pattern")
	updated, cmd := sb.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if updated.Active() {
		t.Fatal("should close after Enter")
	}
	if cmd == nil {
		t.Fatal("Enter should produce a command")
	}
	msg := cmd()
	submitted, ok := msg.(msgs.SearchSubmittedMsg)
	if !ok {
		t.Fatalf("expected SearchSubmittedMsg, got %T", msg)
	}
	if submitted.Pattern != "test.*pattern" {
		t.Fatalf("expected pattern 'test.*pattern', got %q", submitted.Pattern)
	}
	if submitted.Mode != msgs.SearchModeSearch {
		t.Fatal("mode should be SearchModeSearch")
	}
}

func TestSearchBarEscClears(t *testing.T) {
	sb := NewSearchBar(80)
	sb.Open(msgs.SearchModeSearch)
	updated, cmd := sb.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if updated.Active() {
		t.Fatal("should close after Esc")
	}
	if cmd == nil {
		t.Fatal("Esc should produce a command")
	}
	msg := cmd()
	cleared, ok := msg.(msgs.SearchClearedMsg)
	if !ok {
		t.Fatalf("expected SearchClearedMsg, got %T", msg)
	}
	if cleared.Mode != msgs.SearchModeSearch {
		t.Fatalf("expected SearchModeSearch, got %d", cleared.Mode)
	}
}

func TestSearchBarSetError(t *testing.T) {
	sb := NewSearchBar(80)
	sb.Open(msgs.SearchModeSearch)
	sb.SetError("invalid regex")
	view := sb.View()
	if view == "" {
		t.Fatal("should render with error")
	}
}

func TestSearchBarEscCarriesMode(t *testing.T) {
	for _, mode := range []msgs.SearchMode{msgs.SearchModeSearch, msgs.SearchModeFilter} {
		sb := NewSearchBar(80)
		sb.Open(mode)
		_, cmd := sb.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
		msg := cmd()
		cleared, ok := msg.(msgs.SearchClearedMsg)
		if !ok {
			t.Fatalf("expected SearchClearedMsg, got %T", msg)
		}
		if cleared.Mode != mode {
			t.Fatalf("expected Mode %d, got %d", mode, cleared.Mode)
		}
	}
}

func TestSearchBarInlineViewInactive(t *testing.T) {
	sb := NewSearchBar(80)
	if sb.InlineView() != "" {
		t.Fatal("inactive search bar InlineView should return empty")
	}
}

func TestSearchBarInlineViewActive(t *testing.T) {
	sb := NewSearchBar(80)
	sb.Open(msgs.SearchModeSearch)
	iv := sb.InlineView()
	if iv == "" {
		t.Fatal("active search bar InlineView should return non-empty")
	}
}

func TestSearchBarInlineViewWithError(t *testing.T) {
	sb := NewSearchBar(80)
	sb.Open(msgs.SearchModeSearch)

	// Capture view before error
	viewBefore := sb.InlineView()

	sb.SetError("bad regex")
	viewWithError := sb.InlineView()
	if viewWithError == "" {
		t.Fatal("InlineView with error should still return content")
	}
	// Error styling changes the textinput's own styles, so output should differ
	if viewWithError == viewBefore {
		t.Fatal("error InlineView should differ from normal InlineView (error styling applied)")
	}

	// Clearing error should restore default styles
	sb.SetError("")
	viewAfterClear := sb.InlineView()
	if viewAfterClear != viewBefore {
		t.Fatal("InlineView after clearing error should match original")
	}
}
