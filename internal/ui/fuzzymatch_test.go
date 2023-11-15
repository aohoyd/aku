package ui

import "testing"

func TestScoreExactMatch(t *testing.T) {
	if s := scoreCandidate("pods", "pods"); s != 1000 {
		t.Fatalf("exact match should score 1000, got %d", s)
	}
}

func TestScoreExactMatchCaseInsensitive(t *testing.T) {
	if s := scoreCandidate("Pods", "pods"); s != 1000 {
		t.Fatalf("case-insensitive exact should score 1000, got %d", s)
	}
}

func TestScorePrefixMatch(t *testing.T) {
	s := scoreCandidate("deployments", "dep")
	if s < 800 || s >= 1000 {
		t.Fatalf("prefix match should be in [800,1000), got %d", s)
	}
}

func TestScoreShorterCandidateWinsPrefix(t *testing.T) {
	short := scoreCandidate("pods", "po")
	long := scoreCandidate("poddisruptionbudgets", "po")
	if short <= long {
		t.Fatalf("shorter candidate should score higher: pods=%d, pdb=%d", short, long)
	}
}

func TestScoreSubstringMatch(t *testing.T) {
	s := scoreCandidate("configmaps", "map")
	if s < 400 || s >= 800 {
		t.Fatalf("substring match should be in [400,800), got %d", s)
	}
}

func TestScoreAbbreviationMatch(t *testing.T) {
	s := scoreCandidate("poddisruptionbudgets", "pdb")
	if s < 200 || s >= 400 {
		t.Fatalf("abbreviation match should be in [200,400), got %d", s)
	}
}

func TestScoreNoMatch(t *testing.T) {
	if s := scoreCandidate("pods", "xyz"); s != 0 {
		t.Fatalf("expected 0 for no match, got %d", s)
	}
}

func TestFuzzyMatchPluginsEmpty(t *testing.T) {
	entries := []PluginEntry{
		{Name: "pods", ShortName: "po"},
		{Name: "services", ShortName: "svc"},
	}
	results := FuzzyMatchPlugins("", entries)
	if len(results) != 2 {
		t.Fatalf("empty query should return all, got %d", len(results))
	}
}

func TestFuzzyMatchPluginsShortName(t *testing.T) {
	entries := []PluginEntry{
		{Name: "pods", ShortName: "po"},
		{Name: "services", ShortName: "svc"},
		{Name: "deployments", ShortName: "deploy"},
	}
	results := FuzzyMatchPlugins("svc", entries)
	if len(results) == 0 {
		t.Fatal("expected at least 1 match for 'svc'")
	}
	if results[0].Name != "services" {
		t.Fatalf("expected 'services' first, got %q", results[0].Name)
	}
}

func TestFuzzyMatchPluginsPrefixOrdering(t *testing.T) {
	entries := []PluginEntry{
		{Name: "pods", ShortName: "po"},
		{Name: "poddisruptionbudgets", ShortName: "pdb"},
		{Name: "services", ShortName: "svc"},
	}
	results := FuzzyMatchPlugins("po", entries)
	if len(results) < 2 {
		t.Fatalf("expected at least 2 matches for 'po', got %d", len(results))
	}
	if results[0].Name != "pods" {
		t.Fatalf("expected 'pods' first for 'po', got %q", results[0].Name)
	}
}

func TestFuzzyMatchPluginsDisplayFormat(t *testing.T) {
	entries := []PluginEntry{
		{Name: "deployments", ShortName: "deploy"},
	}
	results := FuzzyMatchPlugins("dep", entries)
	if len(results) != 1 {
		t.Fatalf("expected 1 match, got %d", len(results))
	}
	if results[0].Display != "deployments (deploy)" {
		t.Fatalf("expected 'deployments (deploy)', got %q", results[0].Display)
	}
}

func TestFuzzyMatchPluginsNoShortName(t *testing.T) {
	entries := []PluginEntry{
		{Name: "events", ShortName: ""},
	}
	results := FuzzyMatchPlugins("", entries)
	if results[0].Display != "events" {
		t.Fatalf("expected 'events' without parens, got %q", results[0].Display)
	}
}

func TestFilterPluginsReturnsEntriesInScoredOrder(t *testing.T) {
	entries := []PluginEntry{
		{Name: "pods", ShortName: "po"},
		{Name: "deployments", ShortName: "deploy"},
		{Name: "services", ShortName: "svc"},
	}
	result := FilterPlugins("po", entries)
	if len(result) == 0 {
		t.Fatal("expected matches for 'po'")
	}
	if result[0].Name != "pods" {
		t.Fatalf("expected 'pods' first, got %q", result[0].Name)
	}
	for _, e := range result {
		if e.Name == "" {
			t.Fatal("expected non-empty Name in result")
		}
	}
}

func TestFilterPluginsEmptyQueryReturnsAll(t *testing.T) {
	entries := []PluginEntry{
		{Name: "pods", ShortName: "po"},
		{Name: "deployments", ShortName: "deploy"},
	}
	result := FilterPlugins("", entries)
	if len(result) != 2 {
		t.Fatalf("expected 2 results for empty query, got %d", len(result))
	}
}

func TestFuzzyMatchPluginsFiltersNonMatches(t *testing.T) {
	entries := []PluginEntry{
		{Name: "pods", ShortName: "po"},
		{Name: "services", ShortName: "svc"},
	}
	results := FuzzyMatchPlugins("xyz", entries)
	if len(results) != 0 {
		t.Fatalf("expected 0 matches for 'xyz', got %d", len(results))
	}
}

func TestMatchAbbreviationNonASCII(t *testing.T) {
	// Multi-byte UTF-8 characters: matchAbbreviation should handle them correctly
	if !matchAbbreviation("aö", "aöb") {
		t.Fatal("expected 'aö' to match 'aöb'")
	}
	if matchAbbreviation("ö", "abc") {
		t.Fatal("expected 'ö' not to match 'abc'")
	}
	if !matchAbbreviation("日本", "日xx本yy") {
		t.Fatal("expected '日本' to match '日xx本yy'")
	}
}
