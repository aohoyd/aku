package ui

import (
	"fmt"
	"math"
	"strconv"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/aohoyd/aku/internal/msgs"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Scale button indices.
const (
	scaleBtnYes = 0
	scaleBtnNo  = 1
)

// ScaleOverlay is an overlay for changing the replica count of a resource.
type ScaleOverlay struct {
	overlay         Overlay
	resourceName    string
	namespace       string
	gvr             schema.GroupVersionResource
	currentReplicas int32
	inputFocused    bool
	focusedButton   int
}

// NewScaleOverlay creates a new scale overlay with the given dimensions.
func NewScaleOverlay(width, height int) ScaleOverlay {
	o := NewOverlay()
	o.SetSize(width, height)
	return ScaleOverlay{overlay: o}
}

// Open activates the overlay, showing a single text input pre-filled with
// the current replica count.
func (s *ScaleOverlay) Open(resourceName, namespace string, gvr schema.GroupVersionResource, replicas int32) {
	s.resourceName = resourceName
	s.namespace = namespace
	s.gvr = gvr
	s.currentReplicas = replicas
	s.inputFocused = true
	s.focusedButton = scaleBtnYes

	s.overlay.Reset()
	s.overlay.SetActive(true)
	s.overlay.SetTitle(fmt.Sprintf("Scale: %s", resourceName))
	s.overlay.SetFixedWidth(40)

	ti := textinput.New()
	ti.Prompt = "Replicas: "
	ti.SetValue(fmt.Sprintf("%d", replicas))
	s.overlay.AddInput(ti)
	s.overlay.FocusInput(0)
	s.updateFooter()
}

// Close deactivates the overlay.
func (s *ScaleOverlay) Close() {
	s.overlay.SetActive(false)
}

// Active returns whether the overlay is currently active.
func (s ScaleOverlay) Active() bool {
	return s.overlay.Active()
}

// InputFocused returns whether the text input has focus (vs the button bar).
func (s ScaleOverlay) InputFocused() bool {
	return s.inputFocused
}

// FocusedButton returns the currently focused button index.
func (s ScaleOverlay) FocusedButton() int {
	return s.focusedButton
}

// SetSize updates the terminal dimensions available to the overlay.
func (s *ScaleOverlay) SetSize(w, h int) {
	s.overlay.SetSize(w, h)
}

// View renders the overlay panel.
func (s ScaleOverlay) View() string {
	return s.overlay.View()
}

// Update handles key messages for the scale overlay.
func (s ScaleOverlay) Update(msg tea.Msg) (ScaleOverlay, tea.Cmd) {
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
		// Buttons focused: activate focused button.
		if s.focusedButton == scaleBtnYes {
			return s.handleSubmit()
		}
		// No button selected.
		s.Close()
		return s, nil

	case tea.KeyTab:
		s.toggleFocus()
		return s, nil

	case tea.KeyUp:
		if s.inputFocused {
			s.adjustReplicas(1)
			return s, nil
		}
		// From buttons, move focus back to input.
		s.focusToInput()
		return s, nil

	case tea.KeyDown:
		if s.inputFocused {
			s.adjustReplicas(-1)
			return s, nil
		}
		return s, nil

	case tea.KeyLeft:
		if !s.inputFocused {
			s.focusedButton = scaleBtnYes
			s.updateFooter()
		}
		return s, nil

	case tea.KeyRight:
		if !s.inputFocused {
			s.focusedButton = scaleBtnNo
			s.updateFooter()
		}
		return s, nil

	default:
		if !s.inputFocused {
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

// toggleFocus switches focus between the text input and the button bar.
func (s *ScaleOverlay) toggleFocus() {
	if s.inputFocused {
		s.focusToButtons()
	} else {
		s.focusToInput()
	}
}

// focusToInput moves focus to the text input.
func (s *ScaleOverlay) focusToInput() {
	s.inputFocused = true
	s.overlay.FocusInput(0)
	s.updateFooter()
}

// focusToButtons moves focus to the button bar.
func (s *ScaleOverlay) focusToButtons() {
	s.inputFocused = false
	s.overlay.blurAll()
	s.updateFooter()
}

// adjustReplicas increments or decrements the replica value in the input.
func (s *ScaleOverlay) adjustReplicas(delta int) {
	val := s.overlay.InputValue(0)
	n, err := strconv.Atoi(val)
	if err != nil {
		return
	}
	n += delta
	if n < 0 {
		n = 0
	}
	if n > math.MaxInt32 {
		n = math.MaxInt32
	}
	s.overlay.Input(0).SetValue(strconv.Itoa(n))
}

// updateFooter renders the Yes/No button bar as the overlay footer.
func (s *ScaleOverlay) updateFooter() {
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

func (s ScaleOverlay) handleSubmit() (ScaleOverlay, tea.Cmd) {
	val := s.overlay.InputValue(0)
	n, err := strconv.Atoi(val)
	if err != nil || n < 0 || n > math.MaxInt32 {
		// Invalid input: keep overlay open, no command.
		return s, nil
	}

	newReplicas := int32(n)

	s.Close()

	if newReplicas == s.currentReplicas {
		return s, nil
	}

	return s, func() tea.Msg {
		return msgs.ScaleRequestedMsg{
			ResourceName: s.resourceName,
			Namespace:    s.namespace,
			GVR:          s.gvr,
			Replicas:     newReplicas,
		}
	}
}
