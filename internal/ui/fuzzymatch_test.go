package ui

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

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

func TestFuzzyMatchGroupQualifier(t *testing.T) {
	entries := []PluginEntry{
		{
			Name:      "certificates",
			ShortName: "cert",
			GVR:       schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "certificates"},
			Qualified: true,
		},
	}
	results := FuzzyMatchPlugins("cert-manager", entries)
	if len(results) == 0 {
		t.Fatal("expected match for 'cert-manager' via group name")
	}
	if results[0].Name != "certificates" {
		t.Fatalf("expected 'certificates', got %q", results[0].Name)
	}
}

func TestFuzzyMatchNoQualifierWithoutCollision(t *testing.T) {
	entries := []PluginEntry{
		{
			Name:      "pods",
			ShortName: "po",
			GVR:       schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"},
			Qualified: false,
		},
	}
	results := FuzzyMatchPlugins("", entries)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Display != "pods (po)" {
		t.Fatalf("expected 'pods (po)' without qualifier, got %q", results[0].Display)
	}
}

func TestFuzzyMatchQualifiedDisplayFormat(t *testing.T) {
	entries := []PluginEntry{
		{
			Name:      "certificates",
			ShortName: "cert",
			GVR:       schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "certificates"},
			Qualified: true,
		},
		{
			Name:      "certificates",
			ShortName: "",
			GVR:       schema.GroupVersionResource{Group: "acme.cert-manager.io", Version: "v1", Resource: "certificates"},
			Qualified: true,
		},
	}
	results := FuzzyMatchPlugins("", entries)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	// Entry with shortname distinct from name
	found := false
	for _, r := range results {
		if r.Display == "certificates (cert) [cert-manager.io/v1]" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected display 'certificates (cert) [cert-manager.io/v1]' in results: %v", results)
	}
	// Entry without shortname
	found = false
	for _, r := range results {
		if r.Display == "certificates [acme.cert-manager.io/v1]" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected display 'certificates [acme.cert-manager.io/v1]' in results: %v", results)
	}
}

func TestScoreExactNameMatchBeatsGroupMatch(t *testing.T) {
	// An entry whose name exactly matches should score higher than
	// one that only matches via its GVR group.
	nameEntry := PluginEntry{
		Name:      "cert-manager",
		ShortName: "",
		GVR:       schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "cert-manager"},
	}
	groupEntry := PluginEntry{
		Name:      "certificates",
		ShortName: "cert",
		GVR:       schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "certificates"},
		Qualified: true,
	}
	nameScore := scoreEntry("cert-manager", nameEntry)
	groupScore := scoreEntry("cert-manager", groupEntry)
	if nameScore <= groupScore {
		t.Fatalf("exact name match should score higher than group match: name=%d, group=%d", nameScore, groupScore)
	}
}

func TestFilterPluginsWithDuplicateNames(t *testing.T) {
	entries := []PluginEntry{
		{
			Name:      "certificates",
			ShortName: "cert",
			GVR:       schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "certificates"},
			Qualified: true,
		},
		{
			Name:      "certificates",
			ShortName: "",
			GVR:       schema.GroupVersionResource{Group: "acme.cert-manager.io", Version: "v1", Resource: "certificates"},
			Qualified: true,
		},
		{
			Name:      "pods",
			ShortName: "po",
		},
	}
	result := FilterPlugins("cert", entries)
	if len(result) < 2 {
		t.Fatalf("expected at least 2 results for 'cert', got %d", len(result))
	}
	// Both certificate entries should be present
	groups := make(map[string]bool)
	for _, e := range result {
		if e.Name == "certificates" {
			groups[e.GVR.Group] = true
		}
	}
	if !groups["cert-manager.io"] {
		t.Fatal("expected cert-manager.io group in results")
	}
	if !groups["acme.cert-manager.io"] {
		t.Fatal("expected acme.cert-manager.io group in results")
	}
}

func TestFuzzyMatchGroupVersionQuery(t *testing.T) {
	entries := []PluginEntry{
		{
			Name:      "certificates",
			ShortName: "cert",
			GVR:       schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "certificates"},
			Qualified: true,
		},
	}
	results := FuzzyMatchPlugins("cert-manager.io/v1", entries)
	if len(results) == 0 {
		t.Fatal("expected match for 'cert-manager.io/v1' via group/version")
	}
	if results[0].Name != "certificates" {
		t.Fatalf("expected 'certificates', got %q", results[0].Name)
	}
}
