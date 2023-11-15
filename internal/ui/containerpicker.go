package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/aohoyd/aku/internal/msgs"
)

// ContainerPicker is an overlay that lets the user select a container for log streaming.
type ContainerPicker struct {
	Picker[string]
}

// NewContainerPicker creates a new container picker with the given dimensions.
func NewContainerPicker(width, height int) ContainerPicker {
	return ContainerPicker{Picker: NewPicker(PickerConfig[string]{
		Title:      "Select Container",
		NoItemsMsg: "(no containers)",
		MaxVisible: maxDropdownItems,
		Display:    func(s string) string { return s },
		Filter: func(query string, items []string) []string {
			if query == "" {
				return items
			}
			lower := strings.ToLower(query)
			var out []string
			for _, c := range items {
				if strings.Contains(strings.ToLower(c), lower) {
					out = append(out, c)
				}
			}
			return out
		},
		OnSelect: func(s string) tea.Cmd {
			return func() tea.Msg {
				return msgs.LogContainerSelectedMsg{Container: s}
			}
		},
	}, width, height)}
}

// SetContainers stores the list of available containers.
func (c *ContainerPicker) SetContainers(names []string) {
	c.SetItems(names)
}

// Update handles key messages for the container picker.
func (c ContainerPicker) Update(msg tea.Msg) (ContainerPicker, tea.Cmd) {
	p, cmd := c.Picker.Update(msg)
	c.Picker = p
	return c, cmd
}
