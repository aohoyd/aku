package config

import (
	"cmp"
	"maps"
	"slices"
)

// RunConfig describes an external shell command to execute.
type RunConfig struct {
	Command    string `yaml:"command"`
	Background bool   `yaml:"background,omitempty"`
	Confirm    bool   `yaml:"confirm,omitempty"`
}

// Binding is the flat, user-facing key binding definition.
type Binding struct {
	Key     string     `yaml:"key"`
	Help    string     `yaml:"help"`
	Command string     `yaml:"command,omitempty"`
	Run     *RunConfig `yaml:"run,omitempty"`
	Scope   string     `yaml:"scope,omitempty"`
	For     []string   `yaml:"for,omitempty"`
	Visible bool       `yaml:"visible,omitempty"`
	Keys    []Binding  `yaml:"keys,omitempty"`
}

// matchesContext returns true if this binding applies to the given context.
func (b Binding) matchesContext(componentType, resourceName string) bool {
	if b.Scope != "" && b.Scope != componentType {
		return false
	}
	if len(b.For) == 0 {
		return true
	}
	return slices.Contains(b.For, resourceName)
}

// BindingSet compiles a flat binding list into context-specific tries and hints.
type BindingSet struct {
	bindings []Binding
}

// NewBindingSet creates a BindingSet from a flat binding list.
func NewBindingSet(bindings []Binding) *BindingSet {
	return &BindingSet{bindings: bindings}
}

// TrieFor returns a fresh KeyTrie for the given context.
func (bs *BindingSet) TrieFor(componentType, resourceName string) *KeyTrie {
	return NewKeyTrie(bs.compile(componentType, resourceName))
}

func (bs *BindingSet) compile(componentType, resourceName string) map[string]*KeyNode {
	root := make(map[string]*KeyNode)
	for _, b := range bs.bindings {
		insertBinding(root, b, componentType, resourceName)
	}
	return root
}

// insertBinding adds a binding (and its nested children) to the trie root.
// Skips bindings that don't match the context. Children inherit parent scope/for.
func insertBinding(root map[string]*KeyNode, b Binding, componentType, resourceName string) {
	if !b.matchesContext(componentType, resourceName) {
		return
	}

	if len(b.Keys) == 0 {
		// Leaf node
		root[b.Key] = &KeyNode{
			Help:    b.Help,
			Command: b.Command,
			Run:     b.Run,
			Hidden:  !b.Visible,
		}
		return
	}

	// Branch node: compile children
	children := make(map[string]*KeyNode)
	for _, child := range b.Keys {
		// Inherit scope/for from parent
		if child.Scope == "" {
			child.Scope = b.Scope
		}
		if len(child.For) == 0 && len(b.For) > 0 {
			child.For = b.For
		}
		// Children inside a chord are always visible (they're shown
		// when the user is already inside the chord prefix).
		child.Visible = true
		insertBinding(children, child, componentType, resourceName)
	}

	// Merge with existing branch if present
	if existing, ok := root[b.Key]; ok && len(existing.Keys) > 0 {
		maps.Copy(existing.Keys, children)
		existing.Help = b.Help
		existing.Hidden = !b.Visible
	} else {
		root[b.Key] = &KeyNode{
			Help:    b.Help,
			Hidden:  !b.Visible,
			Command: b.Command,
			Run:     b.Run,
			Keys:    children,
		}
	}
}

// sortHints sorts hints alphabetically by key.
func sortHints(hints []KeyHint) {
	slices.SortFunc(hints, func(a, b KeyHint) int {
		return cmp.Compare(a.Key, b.Key)
	})
}

// StatusHints returns only visible hints for the statusbar.
func (bs *BindingSet) StatusHints(componentType, resourceName string) []KeyHint {
	var hints []KeyHint
	for _, b := range bs.bindings {
		if !b.Visible {
			continue
		}
		if !b.matchesContext(componentType, resourceName) {
			continue
		}
		if b.Command != "" || b.Run != nil || len(b.Keys) > 0 {
			hints = append(hints, KeyHint{Key: b.Key, Help: b.Help})
		}
	}
	sortHints(hints)
	return hints
}

// HelpGroups returns all hints grouped by scope for the help overlay.
// Hidden (non-visible) bindings are included.
func (bs *BindingSet) HelpGroups(componentType, resourceName string) []HintGroup {
	grouped := make(map[string][]KeyHint)
	order := []string{}

	for _, b := range bs.bindings {
		if !b.matchesContext(componentType, resourceName) {
			continue
		}
		scope := b.Scope
		if scope == "" {
			scope = "global"
		}
		if _, ok := grouped[scope]; !ok {
			order = append(order, scope)
		}
		hints := flattenBinding(b, "")
		grouped[scope] = append(grouped[scope], hints...)
	}

	var groups []HintGroup
	for _, scope := range order {
		hints := grouped[scope]
		if len(hints) > 0 {
			sortHints(hints)
			groups = append(groups, HintGroup{Scope: scope, Hints: hints})
		}
	}
	return groups
}

// flattenBinding recursively collects all leaf hints from a binding,
// building multi-key sequences like "g p", "g d".
func flattenBinding(b Binding, prefix string) []KeyHint {
	fullKey := b.Key
	if prefix != "" {
		fullKey = prefix + " " + b.Key
	}

	if len(b.Keys) == 0 {
		if b.Command != "" || b.Run != nil {
			return []KeyHint{{Key: fullKey, Help: b.Help}}
		}
		return nil
	}

	var hints []KeyHint
	for _, child := range b.Keys {
		hints = append(hints, flattenBinding(child, fullKey)...)
	}
	return hints
}
