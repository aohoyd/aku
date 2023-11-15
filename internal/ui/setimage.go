package ui

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/aohoyd/aku/internal/msgs"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// SetImageOverlay is an overlay for changing container images.
type SetImageOverlay struct {
	overlay      Overlay
	containers   []msgs.ContainerImageChange // original state for diffing
	resourceName string
	namespace    string
	gvr          schema.GroupVersionResource
	pluginName   string
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
}

// Close deactivates the overlay.
func (s *SetImageOverlay) Close() {
	s.overlay.SetActive(false)
}

// Active returns whether the overlay is currently active.
func (s SetImageOverlay) Active() bool {
	return s.overlay.Active()
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
		return s.handleSubmit()

	case tea.KeyTab, tea.KeyDown:
		s.overlay.FocusNextInput()
		return s, nil

	case tea.KeyUp:
		s.overlay.FocusPrevInput()
		return s, nil

	default:
		cmd := s.overlay.UpdateInputs(km)
		return s, cmd
	}
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
