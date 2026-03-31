package ui

import (
	"regexp"

	"github.com/aohoyd/aku/internal/msgs"
)

// Searchable is the common interface for components that support search/filter.
// Both ResourceList and DetailView satisfy this interface.
type Searchable interface {
	ApplySearch(pattern string, mode msgs.SearchMode) error
	ClearSearch()
	ClearFilter()
	SearchActive() bool
	FilterActive() bool
	AnyActive() bool
	SearchNext()
	SearchPrev()
}

// DetailPanel is the shared interface between DetailView and LogView,
// covering scrolling, focus, sizing, and search operations used by the layout.
type DetailPanel interface {
	Searchable
	ScrollLeft()
	ScrollRight()
	ScrollUp()
	ScrollDown()
	PageUp()
	PageDown()
	GotoTop()
	GotoBottom()
	ToggleWrap()
	ToggleHeader()
	ShowHeader() bool
	Focus()
	Blur()
	SetSize(w, h int)
	SetBorderless(b bool)
	SetInlineSearch(s string)
	View() string
}

// SearchState holds compiled regex and navigation cursor for one pane.
type SearchState struct {
	Pattern    string
	Mode       msgs.SearchMode
	Re         *regexp.Regexp
	MatchCount int
	CurrentIdx int
}

// Compile parses pattern as a regex. Empty pattern clears state.
// On error, the previous search state is preserved (vim-like behavior).
func (s *SearchState) Compile(pattern string, mode msgs.SearchMode) error {
	if pattern == "" {
		s.Clear()
		return nil
	}
	re, err := regexp.Compile("(?i)" + pattern)
	if err != nil {
		return err
	}
	s.Pattern = pattern
	s.Mode = mode
	s.Re = re
	s.CurrentIdx = 0
	s.MatchCount = 0
	return nil
}

// Clear resets all search state.
func (s *SearchState) Clear() {
	s.Pattern = ""
	s.Re = nil
	s.MatchCount = 0
	s.CurrentIdx = 0
}

// Active reports whether a search/filter is compiled and ready.
func (s SearchState) Active() bool { return s.Re != nil }

// DisplayPattern returns the pattern if active, empty string otherwise.
func (s SearchState) DisplayPattern() string {
	if s.Active() {
		return s.Pattern
	}
	return ""
}

// NextIdx advances the current index cyclically and returns it.
func (s *SearchState) NextIdx() int {
	if s.MatchCount == 0 {
		return 0
	}
	s.CurrentIdx = (s.CurrentIdx + 1) % s.MatchCount
	return s.CurrentIdx
}

// PrevIdx reverses the current index cyclically and returns it.
func (s *SearchState) PrevIdx() int {
	if s.MatchCount == 0 {
		return 0
	}
	s.CurrentIdx = (s.CurrentIdx - 1 + s.MatchCount) % s.MatchCount
	return s.CurrentIdx
}
