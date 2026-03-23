package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultKeymap(t *testing.T) {
	km := DefaultKeymap()
	if len(km.Bindings) == 0 {
		t.Fatal("default keymap should have bindings")
	}
}

func TestKeymap_BindingSet(t *testing.T) {
	km := DefaultKeymap()
	bs := km.BindingSet()
	trie := bs.TrieFor("resources", "pods")
	cmd, _, resolved := trie.Press("q")
	if !resolved || cmd != "quit" {
		t.Fatalf("expected quit from default keymap, got resolved=%v cmd=%q", resolved, cmd)
	}
}

func TestLoadKeymapMissing(t *testing.T) {
	km, err := LoadKeymap("/nonexistent/path/keymap.yaml")
	if err != nil {
		t.Fatalf("missing file should return defaults, got error: %v", err)
	}
	if len(km.Bindings) == 0 {
		t.Fatal("missing keymap should fall back to defaults")
	}
}

func TestLoadKeymapFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keymap.yaml")
	data := []byte("bindings:\n  - key: x\n    help: custom\n    command: custom-action\n    for: [pods]\n")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
	km, err := LoadKeymap(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	bs := km.BindingSet()
	// User binding should work
	trie := bs.TrieFor("resources", "pods")
	cmd, _, resolved := trie.Press("x")
	if !resolved || cmd != "custom-action" {
		t.Fatalf("expected custom-action, got resolved=%v cmd=%q", resolved, cmd)
	}
	// Defaults should be prepended
	trie2 := bs.TrieFor("resources", "pods")
	cmd, _, resolved = trie2.Press("q")
	if !resolved || cmd != "quit" {
		t.Fatalf("defaults should be preserved, got resolved=%v cmd=%q", resolved, cmd)
	}
}

func TestLoadKeymapMalformed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keymap.yaml")
	os.WriteFile(path, []byte("{{invalid yaml"), 0644)
	_, err := LoadKeymap(path)
	if err == nil {
		t.Fatal("malformed keymap should return error")
	}
}
