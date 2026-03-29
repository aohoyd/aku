package ui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/aohoyd/aku/internal/msgs"
)

const maxDropdownItems = 10

// ResourcePicker is an overlay that lets the user search and select a resource type.
type ResourcePicker struct {
	Picker[PluginEntry]
}

// NewResourcePicker creates a new resource picker with the given dimensions.
func NewResourcePicker(width, height int) ResourcePicker {
	return ResourcePicker{Picker: NewPicker(PickerConfig[PluginEntry]{
		Title:      "Select Resource",
		NoItemsMsg: "(no matches)",
		MaxVisible: maxDropdownItems,
		Display: func(e PluginEntry) string {
			s := e.Name
			if e.ShortName != "" && e.ShortName != e.Name {
				s = e.Name + " (" + e.ShortName + ")"
			}
			if e.Qualified {
				s += " [" + e.GVR.Group + "/" + e.GVR.Version + "]"
			}
			return s
		},
		Filter: FilterPlugins,
		OnSelect: func(e PluginEntry) tea.Cmd {
			return func() tea.Msg {
				cmd := "goto " + e.Name
				if e.Qualified {
					cmd = "goto-gvr " + e.GVR.Group + "/" + e.GVR.Version + "/" + e.GVR.Resource
				}
				return msgs.ResourcePickedMsg{Command: cmd}
			}
		},
	}, width, height)}
}

// SetPlugins provides the list of available plugins.
func (rp *ResourcePicker) SetPlugins(entries []PluginEntry) {
	rp.SetItems(entries)
}

// Update handles key messages for the resource picker.
func (rp ResourcePicker) Update(msg tea.Msg) (ResourcePicker, tea.Cmd) {
	p, cmd := rp.Picker.Update(msg)
	rp.Picker = p
	return rp, cmd
}
