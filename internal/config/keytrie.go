package config

import (
	"cmp"
	"slices"
)

// KeyNode represents a node in the key binding trie.
type KeyNode struct {
	Help    string              `yaml:"help"`
	Command string              `yaml:"command,omitempty"`
	Run     *RunConfig          `yaml:"-"`
	Keys    map[string]*KeyNode `yaml:"keys,omitempty"`
	Hidden  bool                `yaml:"hidden,omitempty"`
}

// KeyHint is a key+help pair for status bar display.
type KeyHint struct {
	Key  string
	Help string
}

// HintGroup groups key hints under a named scope for the help overlay.
type HintGroup struct {
	Scope string
	Hints []KeyHint
}

// KeyTrie is a prefix tree for resolving key sequences to commands.
type KeyTrie struct {
	root    map[string]*KeyNode
	current *KeyNode // nil = at root level
}

// NewKeyTrie creates a trie from a root key map.
func NewKeyTrie(root map[string]*KeyNode) *KeyTrie {
	return &KeyTrie{root: root}
}

// Press processes one keypress. Returns (command, run, resolved).
// resolved=true means the sequence is complete (command may be "" for invalid keys).
// resolved=false means we're mid-sequence waiting for more keys.
// If the binding has a RunConfig, command will be "" and run will be non-nil.
func (t *KeyTrie) Press(key string) (command string, run *RunConfig, resolved bool) {
	var lookup map[string]*KeyNode
	if t.current == nil {
		lookup = t.root
	} else {
		lookup = t.current.Keys
	}

	node, ok := lookup[key]
	if !ok {
		// Unknown key at this level — reset and swallow
		t.current = nil
		return "", nil, true
	}

	// If node is a leaf (has command or run, no sub-keys), resolve
	if (node.Command != "" || node.Run != nil) && len(node.Keys) == 0 {
		t.current = nil
		return node.Command, node.Run, true
	}

	// If node is a branch (has sub-keys), descend
	if len(node.Keys) > 0 {
		t.current = node
		return "", nil, false
	}

	t.current = nil
	return "", nil, true
}

// Reset clears any in-progress sequence.
func (t *KeyTrie) Reset() {
	t.current = nil
}

// CurrentHints returns help strings for available next keys at the current level.
func (t *KeyTrie) CurrentHints() []KeyHint {
	var lookup map[string]*KeyNode
	if t.current == nil {
		lookup = t.root
	} else {
		lookup = t.current.Keys
	}

	hints := make([]KeyHint, 0, len(lookup))
	for key, node := range lookup {
		if node.Hidden {
			continue
		}
		hints = append(hints, KeyHint{Key: key, Help: node.Help})
	}
	slices.SortFunc(hints, func(a, b KeyHint) int {
		return cmp.Compare(a.Key, b.Key)
	})
	return hints
}

// AtRoot returns true if no prefix sequence is in progress.
func (t *KeyTrie) AtRoot() bool {
	return t.current == nil
}

// PendingPrefix returns the help text of the current prefix node, or "" if at root.
func (t *KeyTrie) PendingPrefix() string {
	if t.current == nil {
		return ""
	}
	return t.current.Help
}
