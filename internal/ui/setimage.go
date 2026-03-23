package ui

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/aohoyd/aku/internal/msgs"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// SetImage button indices.
const (
	setImageBtnYes = 0
	setImageBtnNo  = 1
)

// SetImageOverlay is an overlay for changing container images.
type SetImageOverlay struct {
	overlay       Overlay
	containers    []msgs.ContainerImageChange // original state for diffing
	resourceName  string
	namespace     string
	gvr           schema.GroupVersionResource
	pluginName    string
	inputFocused  bool
	focusedButton int
}

// NewSetImageOverlay creates a new set-image overlay with the given dimensions.
func NewSetImageOverlay(width, height int) SetImageOverlay {
	o := NewOverlay()
	o.SetSize(width, height)
	return SetImageOverlay{overlay: o}
}

// Open activates the overlay, showing one text input per container.
func (s *SetImageOverlay) Open(resourceName, namespace string, gvr schema.GroupVersionResource, pluginName string, containers []msgs.ContainerImageChange) {
	s.resourceName = resourceName
	s.namespace = namespace
	s.gvr = gvr
	s.pluginName = pluginName
	s.containers = containers
	s.inputFocused = true
	s.focusedButton = setImageBtnYes

	s.overlay.Reset()
	s.overlay.SetActive(true)
	s.overlay.SetTitle("Set Image")
	s.overlay.SetFixedWidth(70)

	for _, c := range containers {
		ti := textinput.New()
		ti.Prompt = c.Name + ": "
		ti.SetValue(c.Image)
		s.overlay.AddInput(ti)
	}

	if len(containers) > 0 {
		s.overlay.FocusInput(0)
	}
	s.updateFooter()
}

// Close deactivates the overlay.
func (s *SetImageOverlay) Close() {
	s.overlay.SetActive(false)
}

// Active returns whether the overlay is currently active.
func (s SetImageOverlay) Active() bool {
	return s.overlay.Active()
}

// InputFocused returns whether a text input has focus (vs the button bar).
func (s SetImageOverlay) InputFocused() bool {
	return s.inputFocused
}

// FocusedButton returns the currently focused button index.
func (s SetImageOverlay) FocusedButton() int {
	return s.focusedButton
}

// SetSize updates the terminal dimensions available to the overlay.
func (s *SetImageOverlay) SetSize(w, h int) {
	s.overlay.SetSize(w, h)
}

// View renders the overlay panel.
func (s SetImageOverlay) View() string {
	return s.overlay.View()
}

// Update handles key messages for the set-image overlay.
func (s SetImageOverlay) Update(msg tea.Msg) (SetImageOverlay, tea.Cmd) {
	km, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return s, nil
	}

	switch km.Code {
	case tea.KeyEscape:
		s.Close()
		return s, nil

	case tea.KeyEnter:
		if s.inputFocused {
			return s.handleSubmit()
		}
		if s.focusedButton == setImageBtnYes {
			return s.handleSubmit()
		}
		// No button selected.
		s.Close()
		return s, nil

	case tea.KeyTab:
		if s.inputFocused {
			// If on last input, move to buttons; otherwise next input.
			current := s.overlay.FocusedInput()
			if current >= s.overlay.InputCount()-1 {
				s.focusToButtons()
			} else {
				s.overlay.FocusNextInput()
			}
		} else {
			// From buttons, return to first input.
			s.focusToInput(0)
		}
		return s, nil

	case tea.KeyDown:
		if s.inputFocused {
			s.overlay.FocusNextInput()
		}
		return s, nil

	case tea.KeyUp:
		if s.inputFocused {
			s.overlay.FocusPrevInput()
		} else {
			// From buttons, move focus to last input.
			s.focusToInput(s.overlay.InputCount() - 1)
		}
		return s, nil

	case tea.KeyLeft:
		if !s.inputFocused {
			s.focusedButton = setImageBtnYes
			s.updateFooter()
		}
		return s, nil

	case tea.KeyRight:
		if !s.inputFocused {
			s.focusedButton = setImageBtnNo
			s.updateFooter()
		}
		return s, nil

	default:
		if !s.inputFocused {
			// Button hotkeys.
			switch km.String() {
			case "y", "Y":
				return s.handleSubmit()
			case "n", "N":
				s.Close()
				return s, nil
			}
			return s, nil
		}
		cmd := s.overlay.UpdateInputs(km)
		return s, cmd
	}
}

// focusToInput moves focus to the text input at the given index.
func (s *SetImageOverlay) focusToInput(idx int) {
	s.inputFocused = true
	s.overlay.FocusInput(idx)
	s.updateFooter()
}

// focusToButtons moves focus to the button bar.
func (s *SetImageOverlay) focusToButtons() {
	s.inputFocused = false
	s.overlay.blurAll()
	s.updateFooter()
}

// updateFooter renders the Yes/No button bar as the overlay footer.
func (s *SetImageOverlay) updateFooter() {
	buttons := []Button{
		{Label: "Yes", Hotkey: "y"},
		{Label: "No", Hotkey: "n"},
	}
	focused := s.focusedButton
	if s.inputFocused {
		focused = -1
	}
	s.overlay.SetFooter(RenderButtonBar(buttons, focused))
}

func (s SetImageOverlay) handleSubmit() (SetImageOverlay, tea.Cmd) {
	var changed []msgs.ContainerImageChange
	for i, orig := range s.containers {
		newImage := s.overlay.InputValue(i)
		if newImage != orig.Image {
			changed = append(changed, msgs.ContainerImageChange{
				Name:  orig.Name,
				Image: newImage,
				Init:  orig.Init,
			})
		}
	}

	s.Close()

	if len(changed) == 0 {
		return s, nil
	}

	return s, func() tea.Msg {
		return msgs.SetImageRequestedMsg{
			ResourceName: s.resourceName,
			Namespace:    s.namespace,
			GVR:          s.gvr,
			PluginName:   s.pluginName,
			Images:       changed,
		}
	}
}
