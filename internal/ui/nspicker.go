package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/aohoyd/aku/internal/msgs"
)

const allNamespacesLabel = "All Namespaces"

// NsPicker is an overlay that lets the user search and select a namespace.
type NsPicker struct {
	Picker[string]
}

// NewNsPicker creates a new namespace picker with the given dimensions.
func NewNsPicker(width, height int) NsPicker {
	return NsPicker{Picker: NewPicker(PickerConfig[string]{
		Title:      "Select Namespace",
		NoItemsMsg: "(no matches)",
		MaxVisible: maxDropdownItems,
		Display:    func(s string) string { return s },
		Filter: func(query string, items []string) []string {
			if query == "" {
				return items
			}
			lower := strings.ToLower(query)
			var out []string
			for _, ns := range items {
				if strings.Contains(strings.ToLower(ns), lower) {
					out = append(out, ns)
				}
			}
			return out
		},
		OnSelect: func(s string) tea.Cmd {
			ns := s
			if ns == allNamespacesLabel {
				ns = ""
			}
			return func() tea.Msg {
				return msgs.NamespaceSelectedMsg{Namespace: ns}
			}
		},
	}, width, height)}
}

// SetNamespaces stores the list of available namespaces, prepending "All Namespaces".
func (n *NsPicker) SetNamespaces(nss []string) {
	items := make([]string, 0, len(nss)+1)
	items = append(items, allNamespacesLabel)
	items = append(items, nss...)
	n.SetItems(items)
}

// Update handles key messages for the namespace picker.
func (n NsPicker) Update(msg tea.Msg) (NsPicker, tea.Cmd) {
	p, cmd := n.Picker.Update(msg)
	n.Picker = p
	return n, cmd
}
