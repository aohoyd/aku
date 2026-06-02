package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/aohoyd/aku/internal/msgs"
)

// inUseMarker is the glyph shown next to contexts that are currently backing at
// least one pane.
const inUseMarker = "●"

// ContextPicker is an overlay that lets the user search and select a kube
// context. It is a flat fuzzy list of context names, modeled on NsPicker.
//
// A selection emits a GlobalContextSelectedMsg, which retargets the focused
// pane's context group (the `gx` binding / context-picker command).
type ContextPicker struct {
	Picker[string]
	// counts maps a context name to the number of panes currently using it
	// (count>0 means in use). nil/zero means not in use.
	counts map[string]int
	// focused is the focused pane's CURRENT context — the group gx will move.
	// Its row is visually distinguished.
	focused string
}

// NewContextPicker creates a new context picker with the given dimensions.
// The embedded Picker has no OnSelect: ContextPicker.Update intercepts Enter
// and emits GlobalContextSelectedMsg for the chosen context.
func NewContextPicker(width, height int) ContextPicker {
	return ContextPicker{
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

// SetAnnotations records per-context display annotations for the overlay rows:
//   - counts maps a context name to the number of panes currently using it; a
//     count>0 renders an in-use marker (●) plus the pane count.
//   - focusedContext names the focused pane's CURRENT context (the group gx will
//     move); its row is visually distinguished.
//
// Annotations are presentation only. They flow through the embedded Picker's
// display function, so filtering and the selected VALUE still operate on the
// bare context name — a selection never leaks a marker. Calling with a nil map
// and an empty focusedContext restores plain rendering.
func (c *ContextPicker) SetAnnotations(counts map[string]int, focusedContext string) {
	c.counts = counts
	c.focused = focusedContext
	c.SetDisplay(c.displayContext)
}

// displayContext renders a single overlay row for context name. It is the
// display function installed by SetAnnotations and is presentation only: the
// returned string never feeds back into filtering or the selected value.
func (c *ContextPicker) displayContext(name string) string {
	var b strings.Builder
	if n := c.counts[name]; n > 0 {
		b.WriteString(ContextInUseStyle.Render(inUseMarker))
		b.WriteString(" ")
	}
	if name == c.focused && c.focused != "" {
		b.WriteString(ContextFocusedStyle.Render(name))
	} else {
		b.WriteString(name)
	}
	if n := c.counts[name]; n > 0 {
		b.WriteString(" ")
		b.WriteString(ContextPaneCntStyle.Render(fmt.Sprintf("(%d)", n)))
	}
	return b.String()
}

// Update handles key messages for the context picker. It intercepts the
// selection key (Enter) to emit GlobalContextSelectedMsg for the chosen
// context; all other keys (filter / navigation / esc) delegate to the embedded
// Picker.
func (c ContextPicker) Update(msg tea.Msg) (ContextPicker, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok && keyMsg.Code == tea.KeyEnter {
		filtered := c.Filtered()
		cursor := c.Cursor()
		c.Close()
		if cursor < 0 || cursor >= len(filtered) {
			return c, nil
		}
		item := filtered[cursor]
		return c, func() tea.Msg {
			return msgs.GlobalContextSelectedMsg{Context: item}
		}
	}
	p, cmd := c.Picker.Update(msg)
	c.Picker = p
	return c, cmd
}
