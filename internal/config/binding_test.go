package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBindingMatchesContext_Global(t *testing.T) {
	b := Binding{Key: "q", Help: "quit", Command: "quit"}
	if !b.matchesContext("resources", "pods") {
		t.Fatal("global binding should match any context")
	}
	if !b.matchesContext("details", "deployments") {
		t.Fatal("global binding should match any context")
	}
}

func TestBindingMatchesContext_Scoped(t *testing.T) {
	b := Binding{Key: "tab", Help: "next", Command: "focus-next", Scope: "resources"}
	if !b.matchesContext("resources", "pods") {
		t.Fatal("should match matching scope")
	}
	if b.matchesContext("details", "pods") {
		t.Fatal("should not match different scope")
	}
}

func TestBindingMatchesContext_ForFilter(t *testing.T) {
	b := Binding{Key: "l", Help: "logs", Command: "logs", For: []string{"pods", "containers"}}
	if !b.matchesContext("resources", "pods") {
		t.Fatal("should match listed resource")
	}
	if b.matchesContext("resources", "deployments") {
		t.Fatal("should not match unlisted resource")
	}
}

func TestBindingMatchesContext_ScopeAndFor(t *testing.T) {
	b := Binding{Key: "x", Help: "env", Command: "env", Scope: "details", For: []string{"pods"}}
	if !b.matchesContext("details", "pods") {
		t.Fatal("should match when both scope and for match")
	}
	if b.matchesContext("resources", "pods") {
		t.Fatal("should not match when scope differs")
	}
	if b.matchesContext("details", "deployments") {
		t.Fatal("should not match when resource differs")
	}
}

func TestNewBindingSet_TrieFor_GlobalBinding(t *testing.T) {
	bs := NewBindingSet([]Binding{
		{Key: "q", Help: "quit", Command: "quit"},
	})
	trie := bs.TrieFor("resources", "pods")
	cmd, _, resolved := trie.Press("q")
	if !resolved || cmd != "quit" {
		t.Fatalf("expected quit, got resolved=%v cmd=%q", resolved, cmd)
	}
}

func TestNewBindingSet_TrieFor_ScopedBinding(t *testing.T) {
	bs := NewBindingSet([]Binding{
		{Key: "tab", Help: "next", Command: "focus-next", Scope: "resources"},
	})
	trie := bs.TrieFor("resources", "pods")
	cmd, _, resolved := trie.Press("tab")
	if !resolved || cmd != "focus-next" {
		t.Fatalf("expected focus-next, got resolved=%v cmd=%q", resolved, cmd)
	}
	trie2 := bs.TrieFor("details", "pods")
	cmd, _, resolved = trie2.Press("tab")
	if !resolved || cmd != "" {
		t.Fatalf("scoped binding should not appear in different scope, got resolved=%v cmd=%q", resolved, cmd)
	}
}

func TestNewBindingSet_TrieFor_ForFilter(t *testing.T) {
	bs := NewBindingSet([]Binding{
		{Key: "l", Help: "logs", Command: "logs", For: []string{"pods", "containers"}},
	})
	trie := bs.TrieFor("resources", "pods")
	cmd, _, resolved := trie.Press("l")
	if !resolved || cmd != "logs" {
		t.Fatalf("expected logs for pods, got resolved=%v cmd=%q", resolved, cmd)
	}
	trie2 := bs.TrieFor("resources", "deployments")
	cmd, _, resolved = trie2.Press("l")
	if !resolved || cmd != "" {
		t.Fatalf("for-filtered binding should not appear for deployments, got resolved=%v cmd=%q", resolved, cmd)
	}
}

func TestNewBindingSet_TrieFor_NestedChords(t *testing.T) {
	bs := NewBindingSet([]Binding{
		{Key: "g", Help: "go to", Scope: "resources", Keys: []Binding{
			{Key: "g", Help: "top", Command: "cursor-top"},
			{Key: "p", Help: "pods", Command: "goto-pods"},
		}},
	})
	trie := bs.TrieFor("resources", "pods")
	cmd, _, resolved := trie.Press("g")
	if resolved {
		t.Fatal("g should descend into branch")
	}
	cmd, _, resolved = trie.Press("p")
	if !resolved || cmd != "goto-pods" {
		t.Fatalf("expected goto-pods, got resolved=%v cmd=%q", resolved, cmd)
	}
}

func TestNewBindingSet_TrieFor_ChordInheritsFor(t *testing.T) {
	bs := NewBindingSet([]Binding{
		{Key: "v", Help: "view", For: []string{"pods"}, Keys: []Binding{
			{Key: "l", Help: "logs", Command: "view-logs"},
		}},
	})
	trie := bs.TrieFor("resources", "pods")
	cmd, _, resolved := trie.Press("v")
	if resolved {
		t.Fatal("v should descend for pods")
	}
	cmd, _, resolved = trie.Press("l")
	if !resolved || cmd != "view-logs" {
		t.Fatalf("expected view-logs, got resolved=%v cmd=%q", resolved, cmd)
	}
	trie2 := bs.TrieFor("resources", "deployments")
	cmd, _, resolved = trie2.Press("v")
	if !resolved || cmd != "" {
		t.Fatalf("chord should not exist for deployments, got resolved=%v cmd=%q", resolved, cmd)
	}
}

func TestNewBindingSet_TrieFor_CacheReturnsFreshTrie(t *testing.T) {
	bs := NewBindingSet([]Binding{
		{Key: "q", Help: "quit", Command: "quit"},
	})
	trie1 := bs.TrieFor("resources", "pods")
	trie2 := bs.TrieFor("resources", "pods")
	if trie1 == trie2 {
		t.Fatal("TrieFor should return fresh trie instances")
	}
}

func TestNewBindingSet_TrieFor_LaterBindingWins(t *testing.T) {
	bs := NewBindingSet([]Binding{
		{Key: "x", Help: "global-x", Command: "global-action"},
		{Key: "x", Help: "exec", Command: "exec", For: []string{"pods"}},
	})
	trie := bs.TrieFor("resources", "pods")
	cmd, _, resolved := trie.Press("x")
	if !resolved || cmd != "exec" {
		t.Fatalf("later binding should win, got resolved=%v cmd=%q", resolved, cmd)
	}
}

func TestBindingSet_StatusHints_OnlyVisible(t *testing.T) {
	bs := NewBindingSet([]Binding{
		{Key: "q", Help: "quit", Command: "quit", Visible: true},
		{Key: "j", Help: "down", Command: "cursor-down"},
		{Key: "l", Help: "logs", Command: "logs", Visible: true, For: []string{"pods"}},
	})
	hints := bs.StatusHints("resources", "pods")
	if len(hints) != 2 {
		t.Fatalf("expected 2 visible hints, got %d: %v", len(hints), hints)
	}
	keys := map[string]bool{}
	for _, h := range hints {
		keys[h.Key] = true
	}
	if !keys["q"] || !keys["l"] {
		t.Fatalf("expected q and l, got %v", hints)
	}
}

func TestBindingSet_StatusHints_ForFiltering(t *testing.T) {
	bs := NewBindingSet([]Binding{
		{Key: "l", Help: "logs", Command: "logs", Visible: true, For: []string{"pods"}},
	})
	hints := bs.StatusHints("resources", "pods")
	if len(hints) != 1 {
		t.Fatalf("expected 1 hint for pods, got %d", len(hints))
	}
	hints = bs.StatusHints("resources", "deployments")
	if len(hints) != 0 {
		t.Fatalf("expected 0 hints for deployments, got %d", len(hints))
	}
}

func TestBindingSet_StatusHints_ChordPrefix(t *testing.T) {
	bs := NewBindingSet([]Binding{
		{Key: "g", Help: "go to", Visible: true, Keys: []Binding{
			{Key: "g", Help: "top", Command: "cursor-top"},
		}},
	})
	hints := bs.StatusHints("resources", "pods")
	if len(hints) != 1 || hints[0].Key != "g" {
		t.Fatalf("expected 1 hint for branch 'g', got %v", hints)
	}
}

func TestBindingSet_HelpGroups_IncludesAll(t *testing.T) {
	bs := NewBindingSet([]Binding{
		{Key: "q", Help: "quit", Command: "quit", Visible: true},
		{Key: "j", Help: "down", Command: "cursor-down"},
		{Key: "tab", Help: "next", Command: "focus-next", Scope: "resources"},
		{Key: "l", Help: "logs", Command: "logs", For: []string{"pods"}},
	})
	groups := bs.HelpGroups("resources", "pods")
	totalHints := 0
	for _, g := range groups {
		totalHints += len(g.Hints)
	}
	if totalHints != 4 {
		t.Fatalf("expected 4 total hints (including hidden), got %d", totalHints)
	}
}

func TestBindingSet_HelpGroups_GroupedByScope(t *testing.T) {
	bs := NewBindingSet([]Binding{
		{Key: "q", Help: "quit", Command: "quit"},
		{Key: "tab", Help: "next", Command: "focus-next", Scope: "resources"},
		{Key: "l", Help: "logs", Command: "logs", For: []string{"pods"}},
	})
	groups := bs.HelpGroups("resources", "pods")
	if len(groups) < 2 {
		t.Fatalf("expected at least 2 groups, got %d", len(groups))
	}
	if groups[0].Scope != "global" {
		t.Fatalf("first group should be global, got %q", groups[0].Scope)
	}
}

func TestBindingSet_HelpGroups_FlattensBranches(t *testing.T) {
	bs := NewBindingSet([]Binding{
		{Key: "g", Help: "go to", Keys: []Binding{
			{Key: "p", Help: "pods", Command: "goto-pods"},
			{Key: "d", Help: "deploys", Command: "goto-deployments"},
		}},
	})
	groups := bs.HelpGroups("resources", "")
	found := map[string]bool{}
	for _, g := range groups {
		for _, h := range g.Hints {
			found[h.Key] = true
		}
	}
	if !found["g p"] {
		t.Fatal("should have flattened 'g p'")
	}
	if !found["g d"] {
		t.Fatal("should have flattened 'g d'")
	}
}

func TestBindingSet_HelpGroups_ForFiltering(t *testing.T) {
	bs := NewBindingSet([]Binding{
		{Key: "l", Help: "logs", Command: "logs", For: []string{"pods"}},
	})
	groups := bs.HelpGroups("resources", "pods")
	totalHints := 0
	for _, g := range groups {
		totalHints += len(g.Hints)
	}
	if totalHints != 1 {
		t.Fatalf("expected 1 hint for pods, got %d", totalHints)
	}
	groups = bs.HelpGroups("resources", "deployments")
	totalHints = 0
	for _, g := range groups {
		totalHints += len(g.Hints)
	}
	if totalHints != 0 {
		t.Fatalf("expected 0 hints for deployments, got %d", totalHints)
	}
}

func TestKeymap_BindingSetFromDefault(t *testing.T) {
	km := DefaultKeymap()
	bs := km.BindingSet()
	trie := bs.TrieFor("resources", "pods")
	cmd, _, resolved := trie.Press("q")
	if !resolved || cmd != "quit" {
		t.Fatalf("expected quit from default config, got resolved=%v cmd=%q", resolved, cmd)
	}
}

func TestLoadKeymap_NewFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	data := []byte("bindings:\n  - key: x\n    help: custom\n    command: custom-action\n    visible: true\n")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	km, err := LoadKeymap(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	bs := km.BindingSet()
	trie := bs.TrieFor("resources", "pods")
	cmd, _, resolved := trie.Press("x")
	if !resolved || cmd != "custom-action" {
		t.Fatalf("expected custom-action, got resolved=%v cmd=%q", resolved, cmd)
	}
	cmd, _, resolved = trie.Press("q")
	if !resolved || cmd != "quit" {
		t.Fatalf("defaults should be preserved, got resolved=%v cmd=%q", resolved, cmd)
	}
}

func TestBindingWithRunConfig(t *testing.T) {
	bs := NewBindingSet([]Binding{
		{Key: "L", Help: "stern", Run: &RunConfig{Command: "stern $NAME"}, For: []string{"pods"}, Visible: true},
	})
	trie := bs.TrieFor("resources", "pods")
	cmd, run, resolved := trie.Press("L")
	if !resolved {
		t.Fatal("should resolve immediately")
	}
	if cmd != "" {
		t.Fatalf("command should be empty for run binding, got %q", cmd)
	}
	if run == nil || run.Command != "stern $NAME" {
		t.Fatal("run config should be populated")
	}
}

func TestBindingWithRunBackground(t *testing.T) {
	bs := NewBindingSet([]Binding{
		{Key: "o", Help: "open", Run: &RunConfig{Command: "open http://example.com", Background: true, Confirm: true}},
	})
	trie := bs.TrieFor("resources", "pods")
	cmd, run, resolved := trie.Press("o")
	if !resolved || cmd != "" {
		t.Fatalf("expected resolved with empty cmd, got resolved=%v cmd=%q", resolved, cmd)
	}
	if run == nil || !run.Background || !run.Confirm {
		t.Fatal("run config should have background and confirm set")
	}
}

func TestStatusHints_IncludesRunBindings(t *testing.T) {
	bs := NewBindingSet([]Binding{
		{Key: "L", Help: "stern", Run: &RunConfig{Command: "stern $NAME"}, Visible: true},
	})
	hints := bs.StatusHints("resources", "pods")
	if len(hints) != 1 || hints[0].Key != "L" {
		t.Fatalf("expected 1 hint for run binding, got %v", hints)
	}
}

func TestHelpGroups_IncludesRunBindings(t *testing.T) {
	bs := NewBindingSet([]Binding{
		{Key: "L", Help: "stern", Run: &RunConfig{Command: "stern $NAME"}},
	})
	groups := bs.HelpGroups("resources", "pods")
	total := 0
	for _, g := range groups {
		total += len(g.Hints)
	}
	if total != 1 {
		t.Fatalf("expected 1 hint for run binding, got %d", total)
	}
}

func TestLoadKeymap_WithRunConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keymap.yaml")
	data := []byte(`bindings:
  - key: L
    help: stern logs
    run:
      command: stern $NAME -n $NAMESPACE
    for: [pods]
    visible: true
  - key: o
    help: open browser
    run:
      command: open https://example.com/$NAMESPACE/$NAME
      background: true
      confirm: true
    for: [pods]
`)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	km, err := LoadKeymap(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	bs := km.BindingSet()

	// Test foreground run binding
	trie := bs.TrieFor("resources", "pods")
	cmd, run, resolved := trie.Press("L")
	if !resolved || cmd != "" {
		t.Fatalf("expected resolved with empty cmd, got resolved=%v cmd=%q", resolved, cmd)
	}
	if run == nil || run.Command != "stern $NAME -n $NAMESPACE" {
		t.Fatal("run config should be populated with stern command")
	}
	if run.Background {
		t.Fatal("foreground command should not be background")
	}

	// Test background run binding with confirm
	trie2 := bs.TrieFor("resources", "pods")
	cmd, run, resolved = trie2.Press("o")
	if !resolved || cmd != "" {
		t.Fatalf("expected resolved, got resolved=%v cmd=%q", resolved, cmd)
	}
	if run == nil || !run.Background || !run.Confirm {
		t.Fatal("run config should have background and confirm")
	}

	// Should not appear for deployments
	trie3 := bs.TrieFor("resources", "deployments")
	cmd, run, resolved = trie3.Press("L")
	if !resolved || run != nil {
		t.Fatalf("run binding should not appear for deployments, got resolved=%v run=%v", resolved, run)
	}
}

func TestDefaultBindings_PodsExecNotInDeployments(t *testing.T) {
	bs := NewBindingSet(DefaultBindings())

	// "s" is a prefix key; press it once to enter the submenu, then "s" again to select exec
	trie := bs.TrieFor("resources", "pods")
	cmd, _, resolved := trie.Press("s")
	if resolved {
		t.Fatalf("expected prefix (resolved=false) for first 's', got resolved=%v cmd=%q", resolved, cmd)
	}
	cmd, _, resolved = trie.Press("s")
	if !resolved || cmd != "exec" {
		t.Fatalf("expected exec for pods, got resolved=%v cmd=%q", resolved, cmd)
	}

	// For deployments, "s" prefix should still exist (nodes share it) but inner "s" (exec)
	// should NOT be available because exec is scoped to pods/containers only
	trie2 := bs.TrieFor("resources", "deployments")
	cmd, _, resolved = trie2.Press("s")
	// If "s" prefix doesn't exist for deployments, that's fine too
	if !resolved {
		// We entered a submenu; try pressing "s" for exec
		cmd, _, resolved = trie2.Press("s")
		if cmd == "exec" {
			t.Fatal("exec should not be available for deployments")
		}
	} else if cmd == "exec" {
		t.Fatal("exec should not be available for deployments")
	}
}

func TestBindingMatchesContext_DetailsHierarchy(t *testing.T) {
	b := Binding{Key: "w", Help: "wrap", Command: "toggle-wrap", Scope: "details"}
	for _, ct := range []string{"details", "yaml", "describe", "logs"} {
		if !b.matchesContext(ct, "pods") {
			t.Fatalf("details scope should match %q context", ct)
		}
	}
	if b.matchesContext("resources", "pods") {
		t.Fatal("details scope should not match list context")
	}
}

func TestBindingMatchesContext_LogsScope(t *testing.T) {
	b := Binding{Key: "a", Help: "autoscroll", Command: "toggle-autoscroll", Scope: "logs"}
	if !b.matchesContext("logs", "pods") {
		t.Fatal("logs scope should match logs context")
	}
	if b.matchesContext("yaml", "pods") {
		t.Fatal("logs scope should not match yaml context")
	}
	if b.matchesContext("describe", "pods") {
		t.Fatal("logs scope should not match describe context")
	}
	if b.matchesContext("details", "pods") {
		t.Fatal("logs scope should not match generic details context")
	}
	if b.matchesContext("resources", "pods") {
		t.Fatal("logs scope should not match list context")
	}
}
