package ui

import (
	"testing"

	"github.com/aohoyd/aku/internal/msgs"
)

func TestSearchStateInitiallyInactive(t *testing.T) {
	var ss SearchState
	if ss.Active() {
		t.Fatal("should be inactive by default")
	}
}

func TestSearchStateCompile(t *testing.T) {
	var ss SearchState
	err := ss.Compile("test.*pattern", msgs.SearchModeSearch)
	if err != nil {
		t.Fatalf("valid regex should compile: %v", err)
	}
	if !ss.Active() {
		t.Fatal("should be active after Compile")
	}
	if ss.Pattern != "test.*pattern" {
		t.Fatalf("expected pattern 'test.*pattern', got %q", ss.Pattern)
	}
	if ss.Mode != msgs.SearchModeSearch {
		t.Fatal("mode should be SearchModeSearch")
	}
}

func TestSearchStateCompileInvalid(t *testing.T) {
	var ss SearchState
	err := ss.Compile("[invalid", msgs.SearchModeSearch)
	if err == nil {
		t.Fatal("invalid regex should return error")
	}
	if ss.Active() {
		t.Fatal("should be inactive after invalid compile")
	}
}

func TestSearchStateCompileInvalidPreservesPrevious(t *testing.T) {
	var ss SearchState
	ss.Compile("valid", msgs.SearchModeSearch)
	if !ss.Active() {
		t.Fatal("should be active after valid compile")
	}
	err := ss.Compile("[invalid", msgs.SearchModeSearch)
	if err == nil {
		t.Fatal("invalid regex should return error")
	}
	if !ss.Active() {
		t.Fatal("previous search state should be preserved on invalid compile")
	}
	if ss.Pattern != "valid" {
		t.Fatalf("expected pattern 'valid' preserved, got %q", ss.Pattern)
	}
}

func TestSearchStateCompileEmpty(t *testing.T) {
	var ss SearchState
	ss.Compile("test", msgs.SearchModeSearch)
	err := ss.Compile("", msgs.SearchModeSearch)
	if err != nil {
		t.Fatalf("empty pattern should not error: %v", err)
	}
	if ss.Active() {
		t.Fatal("empty pattern should clear state")
	}
}

func TestSearchStateClear(t *testing.T) {
	var ss SearchState
	ss.Compile("test", msgs.SearchModeSearch)
	ss.Clear()
	if ss.Active() {
		t.Fatal("should be inactive after Clear")
	}
	if ss.Pattern != "" {
		t.Fatal("pattern should be empty after Clear")
	}
}

func TestSearchStateNextPrevIdx(t *testing.T) {
	var ss SearchState
	ss.Compile("test", msgs.SearchModeSearch)
	ss.MatchCount = 3
	ss.CurrentIdx = 0

	if ss.NextIdx() != 1 {
		t.Fatal("NextIdx should return 1")
	}
	if ss.NextIdx() != 2 {
		t.Fatal("NextIdx should return 2")
	}
	if ss.NextIdx() != 0 {
		t.Fatal("NextIdx should wrap to 0")
	}

	ss.CurrentIdx = 0
	if ss.PrevIdx() != 2 {
		t.Fatal("PrevIdx should wrap to 2")
	}
}

func TestSearchStateNextIdxZeroMatches(t *testing.T) {
	var ss SearchState
	ss.Compile("test", msgs.SearchModeSearch)
	ss.MatchCount = 0
	if ss.NextIdx() != 0 {
		t.Fatal("NextIdx with zero matches should return 0")
	}
}
