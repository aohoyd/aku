package config

import (
	"cmp"
	"slices"
	"testing"
)

func TestKeyTrieSingleKey(t *testing.T) {
	trie := DefaultKeyTrie()
	cmd, _, resolved := trie.Press("q")
	if !resolved || cmd != "quit" {
		t.Fatalf("expected resolved 'quit', got resolved=%v cmd='%s'", resolved, cmd)
	}
}

func TestKeyTrieSequenceGP(t *testing.T) {
	trie := DefaultKeyTrie()

	cmd, _, resolved := trie.Press("g")
	if resolved {
		t.Fatal("'g' should not resolve immediately")
	}
	if cmd != "" {
		t.Fatalf("mid-sequence should have empty command, got '%s'", cmd)
	}

	cmd, _, resolved = trie.Press("p")
	if !resolved || cmd != "goto-pods" {
		t.Fatalf("expected resolved 'goto-pods', got resolved=%v cmd='%s'", resolved, cmd)
	}
}

func TestKeyTrieSequenceGNNavNode(t *testing.T) {
	trie := DefaultKeyTrie()

	cmd, _, resolved := trie.Press("g")
	if resolved {
		t.Fatal("'g' should not resolve immediately")
	}

	// Capital 'N' is distinct from lowercase 'n' and must resolve to nav-node.
	cmd, _, resolved = trie.Press("N")
	if !resolved || cmd != "nav-node" {
		t.Fatalf("expected resolved 'nav-node', got resolved=%v cmd='%s'", resolved, cmd)
	}
}

func TestKeyTrieSequenceGNNamespacesUnchanged(t *testing.T) {
	trie := DefaultKeyTrie()
	trie.Press("g")

	// Lowercase 'n' must still resolve to goto-namespaces (no collision with gN).
	cmd, _, resolved := trie.Press("n")
	if !resolved || cmd != "goto-namespaces" {
		t.Fatalf("expected resolved 'goto-namespaces', got resolved=%v cmd='%s'", resolved, cmd)
	}
}

func TestKeyTrieSequenceONSplitNavNode(t *testing.T) {
	trie := DefaultKeyTrie()

	cmd, _, resolved := trie.Press("o")
	if resolved {
		t.Fatal("'o' should not resolve immediately")
	}

	// Capital 'N' is distinct from lowercase 'n' and must resolve to split-nav-node.
	cmd, _, resolved = trie.Press("N")
	if !resolved || cmd != "split-nav-node" {
		t.Fatalf("expected resolved 'split-nav-node', got resolved=%v cmd='%s'", resolved, cmd)
	}
}

func TestKeyTrieSequenceONNamespacesUnchanged(t *testing.T) {
	trie := DefaultKeyTrie()
	trie.Press("o")

	// Lowercase 'n' must still resolve to split-namespaces (no collision with oN).
	cmd, _, resolved := trie.Press("n")
	if !resolved || cmd != "split-namespaces" {
		t.Fatalf("expected resolved 'split-namespaces', got resolved=%v cmd='%s'", resolved, cmd)
	}
}

func TestKeyTrieSequenceGM(t *testing.T) {
	trie := DefaultKeyTrie()

	cmd, _, resolved := trie.Press("g")
	if resolved {
		t.Fatal("'g' should not resolve immediately")
	}

	cmd, _, resolved = trie.Press("m")
	if !resolved || cmd != "goto-aku-messages" {
		t.Fatalf("expected resolved 'goto-aku-messages', got resolved=%v cmd='%s'", resolved, cmd)
	}
}

func TestKeyTrieCtrlXClearNotifications(t *testing.T) {
	trie := DefaultKeyTrie()
	cmd, _, resolved := trie.Press("ctrl+x")
	if !resolved || cmd != "clear-notifications" {
		t.Fatalf("expected resolved 'clear-notifications', got resolved=%v cmd='%s'", resolved, cmd)
	}
}

func TestKeyTrieSingleKeyYamlFocused(t *testing.T) {
	trie := DefaultKeyTrie()
	cmd, _, resolved := trie.Press("y")
	if !resolved || cmd != "view-yaml-focused" {
		t.Fatalf("expected resolved 'view-yaml-focused', got resolved=%v cmd='%s'", resolved, cmd)
	}
}

func TestKeyTrieCtrlDDelete(t *testing.T) {
	trie := DefaultKeyTrie()
	cmd, _, resolved := trie.Press("ctrl+d")
	if !resolved || cmd != "delete" {
		t.Fatalf("expected resolved 'delete', got resolved=%v cmd='%s'", resolved, cmd)
	}
}

func TestKeyTrieSequencePF(t *testing.T) {
	trie := DefaultKeyTrie()
	trie.Press("p")
	cmd, _, resolved := trie.Press("f")
	if !resolved || cmd != "port-forward" {
		t.Fatalf("expected resolved 'port-forward', got resolved=%v cmd='%s'", resolved, cmd)
	}
}

func TestKeyTrieInvalidSecondKey(t *testing.T) {
	trie := DefaultKeyTrie()
	trie.Press("g")
	cmd, _, resolved := trie.Press("z")
	if !resolved || cmd != "" {
		t.Fatalf("invalid second key should resolve empty, got resolved=%v cmd='%s'", resolved, cmd)
	}
}

func TestKeyTrieUnknownKey(t *testing.T) {
	trie := DefaultKeyTrie()
	cmd, _, resolved := trie.Press("X")
	if !resolved || cmd != "" {
		t.Fatalf("unknown root key should resolve empty, got resolved=%v cmd='%s'", resolved, cmd)
	}
}

func TestKeyTrieCurrentHintsAtRoot(t *testing.T) {
	trie := DefaultKeyTrie()
	hints := trie.CurrentHints()
	if len(hints) == 0 {
		t.Fatal("root should have hints")
	}
}

func TestKeyTrieCurrentHintsAfterPrefix(t *testing.T) {
	trie := DefaultKeyTrie()
	trie.Press("g")
	hints := trie.CurrentHints()

	// Sort for deterministic checking
	slices.SortFunc(hints, func(a, b KeyHint) int { return cmp.Compare(a.Key, b.Key) })

	foundPods := false
	for _, h := range hints {
		if h.Key == "p" && h.Help == "pods" {
			foundPods = true
		}
	}
	if !foundPods {
		t.Fatal("after 'g', hints should include p=pods")
	}
}

func TestKeyTrieReset(t *testing.T) {
	trie := DefaultKeyTrie()
	trie.Press("g")
	if trie.AtRoot() {
		t.Fatal("should not be at root after pressing 'g'")
	}
	trie.Reset()
	if !trie.AtRoot() {
		t.Fatal("should be at root after Reset")
	}
	cmd, _, resolved := trie.Press("q")
	if !resolved || cmd != "quit" {
		t.Fatal("after reset, single keys should work from root")
	}
}

func TestKeyTriePendingPrefix(t *testing.T) {
	trie := DefaultKeyTrie()
	if trie.PendingPrefix() != "" {
		t.Fatal("at root, PendingPrefix should be empty")
	}
	trie.Press("g")
	if trie.PendingPrefix() != "go to" {
		t.Fatalf("expected 'go to', got '%s'", trie.PendingPrefix())
	}
}

func TestKeyTrieSequenceBackToRoot(t *testing.T) {
	trie := DefaultKeyTrie()
	// Complete a sequence, then do another
	trie.Press("g")
	trie.Press("p") // resolves goto-pods
	// Should be back at root
	cmd, _, resolved := trie.Press("e")
	if !resolved || cmd != "edit" {
		t.Fatalf("after completing sequence, should be back at root, got resolved=%v cmd='%s'", resolved, cmd)
	}
}

func TestLoadKeymapFallsBackToDefaults(t *testing.T) {
	km, err := LoadKeymap("/nonexistent/path/config.yaml")
	if err != nil {
		t.Fatalf("should fall back to defaults, got error: %v", err)
	}
	bs := km.BindingSet()
	trie := bs.TrieFor("resources", "pods")
	cmd, _, resolved := trie.Press("q")
	if !resolved || cmd != "quit" {
		t.Fatal("default config should have quit binding")
	}
}

func TestSplitKeys(t *testing.T) {
	trie := DefaultKeyTrie()
	trie.Press("o")
	cmd, _, resolved := trie.Press("p")
	if !resolved || cmd != "split-pods" {
		t.Fatalf("expected 'split-pods', got resolved=%v cmd='%s'", resolved, cmd)
	}
}

func TestKeyTrieSequenceSortName(t *testing.T) {
	trie := DefaultKeyTrie()
	cmd, _, resolved := trie.Press("S")
	if resolved {
		t.Fatal("'S' should not resolve immediately (it's a prefix)")
	}
	cmd, _, resolved = trie.Press("n")
	if !resolved || cmd != "sort-NAME" {
		t.Fatalf("expected resolved 'sort-NAME', got resolved=%v cmd='%s'", resolved, cmd)
	}
}

func TestKeyTrieSequenceSortAge(t *testing.T) {
	trie := DefaultKeyTrie()
	trie.Press("S")
	cmd, _, resolved := trie.Press("a")
	if !resolved || cmd != "sort-AGE" {
		t.Fatalf("expected resolved 'sort-AGE', got resolved=%v cmd='%s'", resolved, cmd)
	}
}

func TestKeyTrieSequenceSortStatus(t *testing.T) {
	trie := DefaultKeyTrie()
	trie.Press("S")
	cmd, _, resolved := trie.Press("s")
	if !resolved || cmd != "sort-STATUS" {
		t.Fatalf("expected resolved 'sort-STATUS', got resolved=%v cmd='%s'", resolved, cmd)
	}
}

func TestKeyTrieSortHints(t *testing.T) {
	trie := DefaultKeyTrie()
	trie.Press("S")
	hints := trie.CurrentHints()
	if len(hints) != 4 {
		t.Fatalf("expected 4 sort hints, got %d", len(hints))
	}
}

func TestKeyTrieHiddenKeysExcludedFromHints(t *testing.T) {
	trie := NewKeyTrie(map[string]*KeyNode{
		"q": {Help: "quit", Command: "quit"},
		":": {Help: "command", Command: "resource-picker", Hidden: true},
	})
	hints := trie.CurrentHints()
	if len(hints) != 1 {
		t.Fatalf("expected 1 hint (hidden excluded), got %d", len(hints))
	}
	if hints[0].Key != "q" {
		t.Fatalf("expected hint key 'q', got '%s'", hints[0].Key)
	}
}

func TestKeyTrieHiddenKeysStillResolve(t *testing.T) {
	trie := NewKeyTrie(map[string]*KeyNode{
		":": {Help: "command", Command: "resource-picker", Hidden: true},
	})
	cmd, _, resolved := trie.Press(":")
	if !resolved || cmd != "resource-picker" {
		t.Fatalf("hidden key should still resolve, got resolved=%v cmd='%s'", resolved, cmd)
	}
}

func TestNavigationBindingsInGlobal(t *testing.T) {
	trie := DefaultKeyTrie()
	tests := []struct {
		key     string
		command string
	}{
		{"j", "cursor-down"},
		{"k", "cursor-up"},
		{"n", "search-next"},
		{"N", "search-prev"},
		{"?", "help"},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			trie := DefaultKeyTrie() // fresh trie each test
			cmd, _, resolved := trie.Press(tt.key)
			if !resolved || cmd != tt.command {
				t.Fatalf("key %q: expected %q, got resolved=%v cmd=%q", tt.key, tt.command, resolved, cmd)
			}
		})
	}
	_ = trie // suppress unused
}

func TestEscResolvesClearOverlay(t *testing.T) {
	trie := DefaultKeyTrie()
	cmd, _, resolved := trie.Press("esc")
	if !resolved || cmd != "clear-overlay" {
		t.Fatalf("esc should resolve to clear-overlay, got resolved=%v cmd=%q", resolved, cmd)
	}
}

func TestBackspaceResolvesNavParent(t *testing.T) {
	trie := DefaultKeyTrie() // resources scope + pods
	cmd, _, resolved := trie.Press("backspace")
	if !resolved || cmd != "nav-parent" {
		t.Fatalf("backspace should resolve to nav-parent in resources scope, got resolved=%v cmd=%q", resolved, cmd)
	}

	// nav-parent is scoped to "resources": backspace must NOT resolve to it in
	// other scopes (e.g. details/logs), guarding against an accidental all-scope
	// binding.
	bs := NewBindingSet(DefaultBindings())
	for _, scope := range []string{"details", "logs"} {
		t.Run(scope, func(t *testing.T) {
			trie := bs.TrieFor(scope, "pods")
			cmd, _, _ := trie.Press("backspace")
			if cmd == "nav-parent" {
				t.Fatalf("backspace must not resolve to nav-parent in %q scope", scope)
			}
		})
	}
}

func TestCtrlWResolvesCloseCurrentPanel(t *testing.T) {
	trie := DefaultKeyTrie()
	cmd, _, resolved := trie.Press("ctrl+w")
	if !resolved || cmd != "close-current-panel" {
		t.Fatalf("ctrl+w should resolve to close-current-panel, got resolved=%v cmd=%q", resolved, cmd)
	}
}

func TestDetailPanelExitBindings(t *testing.T) {
	bs := NewBindingSet(DefaultBindings())
	trie := bs.TrieFor("details", "pods")

	tests := []struct {
		key     string
		command string
	}{
		{"h", "scroll-left"},
		{"left", "scroll-left"},
		{"esc", "clear-overlay"},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			trie := bs.TrieFor("details", "pods")
			cmd, _, resolved := trie.Press(tt.key)
			if !resolved || cmd != tt.command {
				t.Fatalf("key %q: expected %q, got resolved=%v cmd=%q", tt.key, tt.command, resolved, cmd)
			}
		})
	}
	_ = trie // suppress unused
}

func TestNavigationBindingsAreHidden(t *testing.T) {
	trie := DefaultKeyTrie()
	hints := trie.CurrentHints()
	hidden := map[string]bool{"j": true, "k": true, "h": true, "n": true, "N": true, "?": true, "up": true, "down": true, "left": true, "right": true, "enter": true}
	for _, h := range hints {
		if hidden[h.Key] {
			t.Fatalf("key %q should be hidden from hints", h.Key)
		}
	}
}

func TestKeyTrieHelmValuesUser(t *testing.T) {
	bs := NewBindingSet(DefaultBindings())
	trie := bs.TrieFor("resources", "helmreleases")

	cmd, _, resolved := trie.Press("v")
	if !resolved || cmd != "view-helm-values-user" {
		t.Fatalf("expected resolved 'view-helm-values-user', got resolved=%v cmd='%s'", resolved, cmd)
	}
}

func TestKeyTrieHelmValuesAll(t *testing.T) {
	bs := NewBindingSet(DefaultBindings())
	trie := bs.TrieFor("resources", "helmreleases")

	cmd, _, resolved := trie.Press("V")
	if !resolved || cmd != "view-helm-values-all" {
		t.Fatalf("expected resolved 'view-helm-values-all', got resolved=%v cmd='%s'", resolved, cmd)
	}
}

func TestKeyTrieHelmValuesInactiveOutsideHelmreleases(t *testing.T) {
	bs := NewBindingSet(DefaultBindings())
	// In a non-helmreleases context, neither 'v' nor 'V' should resolve to
	// any helm-values command — they should fall through to empty.
	trie := bs.TrieFor("resources", "pods")

	cmd, _, resolved := trie.Press("v")
	if !resolved || cmd != "" {
		t.Fatalf("'v' should not match in pods context, got resolved=%v cmd='%s'", resolved, cmd)
	}

	trie = bs.TrieFor("resources", "pods")
	cmd, _, resolved = trie.Press("V")
	if !resolved || cmd != "" {
		t.Fatalf("'V' should not match in pods context, got resolved=%v cmd='%s'", resolved, cmd)
	}
}

// TestKeyTrieRRollbackOnHelmreleases is a regression test ensuring that the
// `R` binding on a `helmreleases` plugin still resolves to `helm-rollback`.
// This binding is separate from the `rollout-restart` `R` binding (which is
// scoped to deployments/statefulsets/daemonsets via `For`) and must not be
// affected when the rollout-restart `For` list changes.
func TestKeyTrieRRollbackOnHelmreleases(t *testing.T) {
	bs := NewBindingSet(DefaultBindings())
	trie := bs.TrieFor("resources", "helmreleases")

	cmd, _, resolved := trie.Press("R")
	if !resolved || cmd != "helm-rollback" {
		t.Fatalf("expected 'R' on helmreleases to resolve to 'helm-rollback', got resolved=%v cmd=%q", resolved, cmd)
	}
}

// TestKeyTrieRDoesNotFireOnPods is a regression test ensuring that the `R`
// binding for `rollout-restart` does NOT fire on a `pods` plugin. Pods are
// not in the `For` list (only deployments/statefulsets/daemonsets are), so
// pressing `R` in a pods context should resolve to an empty command.
func TestKeyTrieRDoesNotFireOnPods(t *testing.T) {
	bs := NewBindingSet(DefaultBindings())
	trie := bs.TrieFor("resources", "pods")

	cmd, _, resolved := trie.Press("R")
	if !resolved || cmd != "" {
		t.Fatalf("'R' on pods should not resolve to any command, got resolved=%v cmd=%q", resolved, cmd)
	}
}

// TestKeyTrieRRolloutRestartScope is a regression test ensuring that the `R`
// binding resolves to `rollout-restart` for each of the three resources in
// the binding's `For` list. A scope misconfiguration that accidentally
// dropped any of these would be caught here.
func TestKeyTrieRRolloutRestartScope(t *testing.T) {
	for _, plugin := range []string{"deployments", "statefulsets", "daemonsets"} {
		t.Run(plugin, func(t *testing.T) {
			bs := NewBindingSet(DefaultBindings())
			trie := bs.TrieFor("resources", plugin)
			cmd, _, resolved := trie.Press("R")
			if !resolved || cmd != "rollout-restart" {
				t.Fatalf("'R' on %s should resolve to 'rollout-restart', got resolved=%v cmd=%q", plugin, resolved, cmd)
			}
		})
	}
}

func TestKeyTrieContainerRebindToCapitalC(t *testing.T) {
	bs := NewBindingSet(DefaultBindings())
	trie := bs.TrieFor("logs", "")
	cmd, _, resolved := trie.Press("C")
	if !resolved || cmd != "select-container" {
		t.Fatalf("'C' in logs scope should resolve to 'select-container', got resolved=%v cmd=%q", resolved, cmd)
	}
}

func TestKeyTrieLowercaseCIsCopyChordInLogs(t *testing.T) {
	bs := NewBindingSet(DefaultBindings())
	trie := bs.TrieFor("logs", "")

	// 'c' must no longer resolve to select-container — it is now the copy chord
	// prefix (mid-sequence, not yet resolved).
	cmd, _, resolved := trie.Press("c")
	if resolved {
		t.Fatalf("'c' in logs scope should be a chord prefix, not resolve immediately (got cmd=%q)", cmd)
	}

	// 'co' resolves to open-current (the second key after the chord prefix).
	cmd, _, resolved = trie.Press("o")
	if !resolved || cmd != "open-current" {
		t.Fatalf("'co' in logs scope should resolve to 'open-current', got resolved=%v cmd=%q", resolved, cmd)
	}
}

// TestKeyTrieCopyChordResolutions asserts all three copy-chord second keys
// resolve to their commands in both the root and logs scopes: cc→copy-current,
// cy→copy-yaml, co→open-current. The trie is reopened (fresh) for each sequence.
func TestKeyTrieCopyChordResolutions(t *testing.T) {
	bs := NewBindingSet(DefaultBindings())

	scopes := []struct {
		name         string
		ct, resource string
	}{
		{"root", "resources", "pods"},
		{"logs", "logs", ""},
	}
	chords := []struct {
		second  string
		command string
	}{
		{"c", "copy-current"},
		{"y", "copy-yaml"},
		{"o", "open-current"},
	}

	for _, sc := range scopes {
		for _, ch := range chords {
			t.Run(sc.name+"_c"+ch.second, func(t *testing.T) {
				trie := bs.TrieFor(sc.ct, sc.resource)

				// First 'c' is a chord prefix and must not resolve.
				cmd, _, resolved := trie.Press("c")
				if resolved {
					t.Fatalf("'c' in %s scope should be a chord prefix, got cmd=%q", sc.name, cmd)
				}

				// The second key resolves to the chord command.
				cmd, _, resolved = trie.Press(ch.second)
				if !resolved || cmd != ch.command {
					t.Fatalf("'c%s' in %s scope should resolve to %q, got resolved=%v cmd=%q",
						ch.second, sc.name, ch.command, resolved, cmd)
				}
			})
		}
	}
}

func TestKeyTrieCopyChordOpenCurrent(t *testing.T) {
	trie := DefaultKeyTrie()

	cmd, _, resolved := trie.Press("c")
	if resolved {
		t.Fatalf("'c' should be a chord prefix, got cmd=%q", cmd)
	}
	cmd, _, resolved = trie.Press("o")
	if !resolved || cmd != "open-current" {
		t.Fatalf("'co' should resolve to 'open-current', got resolved=%v cmd=%q", resolved, cmd)
	}
}

func TestDefaultKeyMapCommandBarHidden(t *testing.T) {
	trie := DefaultKeyTrie()
	hints := trie.CurrentHints()
	for _, h := range hints {
		if h.Key == ":" {
			t.Fatal("':' should be hidden from root hints")
		}
	}
	// But it should still work
	cmd, _, resolved := trie.Press(":")
	if !resolved || cmd != "resource-picker" {
		t.Fatalf("':' should still resolve to resource-picker, got resolved=%v cmd='%s'", resolved, cmd)
	}
}
