package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/aohoyd/aku/internal/msgs"
)

// ContextPicker is an overlay that lets the user search and select a kube
// context. It is a flat fuzzy list of context names, modeled on NsPicker.
//
// A single picker serves both switch scopes. SetScope chooses whether a
// selection emits a GlobalContextSelectedMsg (global baseline switch, `gx`) or
// a PaneContextSelectedMsg (pin the focused pane, `gX`). The scope is a plain
// bool field read at selection time in Update — not captured in a closure at
// construction — so SetScope simply assigns the field and takes effect on the
// next selection.
type ContextPicker struct {
	Picker[string]
	// global selects which message a selection emits: true ->
	// GlobalContextSelectedMsg, false -> PaneContextSelectedMsg. Read at
	// selection time in Update.
	global bool
}

// NewContextPicker creates a new context picker with the given dimensions.
// The picker defaults to global scope; callers set scope explicitly via
// SetScope before Open. The embedded Picker has no OnSelect: ContextPicker.Update
// intercepts Enter and builds the message from the current scope field, so the
// emitted message always reflects the latest SetScope.
func NewContextPicker(width, height int) ContextPicker {
	return ContextPicker{
		global: true,
		Picker: NewPicker(PickerConfig[string]{
			Title:      "Select Context",
			NoItemsMsg: "(no matches)",
			MaxVisible: maxDropdownItems,
			Display:    func(s string) string { return s },
			Filter: func(query string, items []string) []string {
				if query == "" {
					return items
				}
				lower := strings.ToLower(query)
				var out []string
				for _, ctx := range items {
					if strings.Contains(strings.ToLower(ctx), lower) {
						out = append(out, ctx)
					}
				}
				return out
			},
		}, width, height),
	}
}

// SetContexts stores the list of available context names. Unlike the namespace
// picker there is no sentinel entry; the list is exactly the supplied names.
func (c *ContextPicker) SetContexts(names []string) {
	items := make([]string, len(names))
	copy(items, names)
	c.SetItems(items)
}

// SetScope selects the message emitted on the next selection: true emits a
// GlobalContextSelectedMsg (global baseline switch), false emits a
// PaneContextSelectedMsg (pin the focused pane).
func (c *ContextPicker) SetScope(global bool) {
	c.global = global
}

// Update handles key messages for the context picker. It intercepts the
// selection key (Enter) so the emitted message is chosen from the CURRENT scope
// field rather than a construction-time closure; all other keys (filter /
// navigation / esc) delegate to the embedded Picker.
func (c ContextPicker) Update(msg tea.Msg) (ContextPicker, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok && keyMsg.Code == tea.KeyEnter {
		filtered := c.Filtered()
		cursor := c.Cursor()
		c.Close()
		if cursor < 0 || cursor >= len(filtered) {
			return c, nil
		}
		item := filtered[cursor]
		global := c.global
		return c, func() tea.Msg {
			if global {
				return msgs.GlobalContextSelectedMsg{Context: item}
			}
			return msgs.PaneContextSelectedMsg{Context: item}
		}
	}
	p, cmd := c.Picker.Update(msg)
	c.Picker = p
	return c, cmd
}
