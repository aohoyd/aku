package ui

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/aohoyd/aku/internal/msgs"
	"github.com/aohoyd/aku/internal/theme"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// SetImage button indices.
const (
	setImageBtnYes = 0
	setImageBtnNo  = 1
)

// setImageFocus identifies which kind of element currently has focus.
type setImageFocus int

const (
	focusImage   setImageFocus = iota // an image text input
	focusPolicy                       // a pull-policy cycle field
	focusButtons                      // the Yes/No button bar
)

// containerRow pairs a container's image input (held by the shared Overlay at
// inputIdx) with its in-place pull-policy cycle field.
type containerRow struct {
	inputIdx int
	policy   CycleField
}

// SetImageOverlay is an overlay for changing container images.
type SetImageOverlay struct {
	overlay      Overlay
	containers   []msgs.ContainerImageChange // original state for diffing
	rows         []containerRow
	resourceName string
	namespace    string
	gvr          schema.GroupVersionResource
	pluginName   string

	// Focus model: focusKind says which element type is active. focusRow is
	// the container row for focusImage/focusPolicy. focusedButton is the
	// selected button when focusKind == focusButtons.
	focusKind     setImageFocus
	focusRow      int
	focusedButton int
}

// NewSetImageOverlay creates a new set-image overlay with the given dimensions.
func NewSetImageOverlay(width, height int) SetImageOverlay {
	o := NewOverlay()
	o.SetSize(width, height)
	return SetImageOverlay{overlay: o}
}

// Open activates the overlay, showing one image input + policy cycle per container.
func (s *SetImageOverlay) Open(resourceName, namespace string, gvr schema.GroupVersionResource, pluginName string, containers []msgs.ContainerImageChange) {
	s.resourceName = resourceName
	s.namespace = namespace
	s.gvr = gvr
	s.pluginName = pluginName
	s.containers = containers
	s.rows = nil
	s.focusKind = focusImage
	s.focusRow = 0
	s.focusedButton = setImageBtnYes

	s.overlay.Reset()
	s.overlay.SetActive(true)
	s.overlay.SetTitle("Set Image")
	s.overlay.SetFixedWidth(70)

	for _, c := range containers {
		ti := textinput.New()
		ti.Prompt = " image:  "
		ti.SetValue(c.Image)
		idx := s.overlay.AddInput(ti)
		s.rows = append(s.rows, containerRow{
			inputIdx: idx,
			policy:   NewPullPolicyField(c.PullPolicy),
		})
	}

	if len(s.rows) > 0 {
		s.focusImage(0)
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

// InputFocused reports whether an image text input has focus (vs a policy
// cycle or the button bar).
func (s SetImageOverlay) InputFocused() bool {
	return s.focusKind == focusImage
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
//
// Render seam: the shared Overlay can auto-render its text inputs, but it has
// no hook to interleave the per-row policy cycles between them. To keep a
// single clean render with working text-input cursors, we compose the whole
// body here — name header + image input.View() + policy.View() per row — and
// hand it to the panel via ViewWithBody so the panel supplies only the
// box/title/footer chrome and does NOT re-render the inputs.
func (s SetImageOverlay) View() string {
	if !s.overlay.Active() {
		return ""
	}
	return s.overlay.ViewWithBody(s.buildBody())
}

// buildBody composes the multi-field overlay body string.
func (s SetImageOverlay) buildBody() string {
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(theme.Highlight)

	var sections []string
	for i, row := range s.rows {
		c := s.containers[i]
		label := c.Name
		if c.Init {
			label += " (init)"
		}
		imgActive := s.focusKind == focusImage && s.focusRow == i
		section := strings.Join([]string{
			headerStyle.Render(label),
			s.imageLine(row, imgActive),
			row.policy.View(),
		}, "\n")
		sections = append(sections, section)
	}
	return strings.Join(sections, "\n\n")
}

// imageLine renders the focused image via the live textinput (bright text with a
// visible cursor); a blurred image as plain dimmed text, truncated to the input
// width so it can't wrap and so no bright rune shows under a hidden cursor.
func (s SetImageOverlay) imageLine(row containerRow, active bool) string {
	in := s.overlay.Input(row.inputIdx)
	if active {
		return in.View()
	}
	promptStyle := lipgloss.NewStyle().Foreground(theme.Prompt)
	valStyle := lipgloss.NewStyle().Foreground(theme.Muted)
	val := ansi.Truncate(in.Value(), in.Width(), "…")
	return promptStyle.Render(in.Prompt) + valStyle.Render(val)
}

// Update handles key messages for the set-image overlay.
func (s SetImageOverlay) Update(msg tea.Msg) (SetImageOverlay, tea.Cmd) {
	km, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return s, nil
	}

	// Shift+Tab arrives as KeyTab with the shift modifier; detect it before the
	// plain Tab case so it walks focus backward (the reverse of Tab), mirroring
	// how the confirm dialog distinguishes "shift+tab" from "tab".
	if km.String() == "shift+tab" {
		s.focusPrev()
		return s, nil
	}

	// Guard against an overlay opened with zero containers: the policy-cycle
	// cases below index s.rows[s.focusRow]. Escape must still close, and the
	// focus-walk helpers (focusNext/focusPrev) already no-op on empty rows.
	if len(s.rows) == 0 {
		switch km.Code {
		case tea.KeyEscape:
			s.Close()
			return s, nil
		case tea.KeyEnter:
			// handleSubmit is safe with no containers: it closes and emits no
			// command since there is nothing to diff.
			return s.handleSubmit()
		}
		return s, nil
	}

	switch km.Code {
	case tea.KeyEscape:
		s.Close()
		return s, nil

	case tea.KeyEnter:
		switch s.focusKind {
		case focusButtons:
			if s.focusedButton == setImageBtnYes {
				return s.handleSubmit()
			}
			s.Close()
			return s, nil
		default:
			return s.handleSubmit()
		}

	case tea.KeyTab:
		s.focusNext()
		return s, nil

	case tea.KeySpace:
		if s.focusKind == focusPolicy {
			// Space cycles the focused policy forward.
			s.rows[s.focusRow].policy.Next()
			return s, nil
		}
		if s.focusKind == focusImage {
			cmd := s.overlay.UpdateInputs(km)
			return s, cmd
		}
		return s, nil

	case tea.KeyDown:
		s.focusNext()
		return s, nil

	case tea.KeyUp:
		s.focusPrev()
		return s, nil

	case tea.KeyLeft:
		switch s.focusKind {
		case focusImage:
			cmd := s.overlay.UpdateInputs(km)
			return s, cmd
		case focusPolicy:
			s.rows[s.focusRow].policy.Prev()
			return s, nil
		default: // buttons
			s.focusedButton = setImageBtnYes
			s.updateFooter()
			return s, nil
		}

	case tea.KeyRight:
		switch s.focusKind {
		case focusImage:
			cmd := s.overlay.UpdateInputs(km)
			return s, cmd
		case focusPolicy:
			s.rows[s.focusRow].policy.Next()
			return s, nil
		default: // buttons
			s.focusedButton = setImageBtnNo
			s.updateFooter()
			return s, nil
		}

	default:
		switch s.focusKind {
		case focusImage:
			cmd := s.overlay.UpdateInputs(km)
			return s, cmd
		case focusPolicy:
			// Other keys (non-arrow, non-space) are ignored on a policy cycle.
			return s, nil
		default: // buttons
			switch km.String() {
			case "y", "Y":
				return s.handleSubmit()
			case "n", "N":
				s.Close()
				return s, nil
			}
			return s, nil
		}
	}
}

// focusImage focuses the image input of row r.
func (s *SetImageOverlay) focusImage(r int) {
	s.blurPolicies()
	s.focusKind = focusImage
	s.focusRow = r
	s.overlay.FocusInput(s.rows[r].inputIdx)
	s.updateFooter()
}

// focusPolicy focuses the policy cycle of row r.
func (s *SetImageOverlay) focusPolicy(r int) {
	s.overlay.blurAll()
	s.blurPolicies()
	s.focusKind = focusPolicy
	s.focusRow = r
	s.rows[r].policy.Focus()
	s.updateFooter()
}

// focusToButtons moves focus to the button bar.
func (s *SetImageOverlay) focusToButtons() {
	s.overlay.blurAll()
	s.blurPolicies()
	s.focusKind = focusButtons
	s.updateFooter()
}

// blurPolicies blurs every policy cycle field.
func (s *SetImageOverlay) blurPolicies() {
	for i := range s.rows {
		s.rows[i].policy.Blur()
	}
}

// focusNext advances focus in the order:
// img0 -> pull0 -> img1 -> pull1 -> ... -> buttons -> (wrap) img0.
func (s *SetImageOverlay) focusNext() {
	if len(s.rows) == 0 {
		return
	}
	switch s.focusKind {
	case focusImage:
		s.focusPolicy(s.focusRow)
	case focusPolicy:
		if s.focusRow >= len(s.rows)-1 {
			s.focusToButtons()
		} else {
			s.focusImage(s.focusRow + 1)
		}
	case focusButtons:
		s.focusImage(0)
	}
}

// focusPrev moves focus backward in the reverse order. Up from buttons lands
// on the last container's policy cycle.
func (s *SetImageOverlay) focusPrev() {
	if len(s.rows) == 0 {
		return
	}
	switch s.focusKind {
	case focusImage:
		if s.focusRow <= 0 {
			s.focusToButtons()
		} else {
			s.focusPolicy(s.focusRow - 1)
		}
	case focusPolicy:
		s.focusImage(s.focusRow)
	case focusButtons:
		s.focusPolicy(len(s.rows) - 1)
	}
}

// updateFooter renders the Yes/No button bar as the overlay footer.
func (s *SetImageOverlay) updateFooter() {
	buttons := []Button{
		{Label: "Yes", Hotkey: "y"},
		{Label: "No", Hotkey: "n"},
	}
	focused := s.focusedButton
	if s.focusKind != focusButtons {
		focused = -1
	}
	s.overlay.SetFooter(RenderButtonBar(buttons, focused))
}

func (s SetImageOverlay) handleSubmit() (SetImageOverlay, tea.Cmd) {
	// Diff each container's image and pull-policy independently so a change to
	// one does not silently carry the other.
	var changed []msgs.ContainerImageChange
	for i, orig := range s.containers {
		newImage := s.overlay.InputValue(s.rows[i].inputIdx)
		newPolicy := s.rows[i].policy.Value() // "" = (default)

		imgChanged := newImage != orig.Image
		policyChanged := newPolicy != orig.PullPolicy

		// An empty image is never a valid change: clearing the field would
		// otherwise emit a name-only no-op patch that silently discards the
		// user's edit. Treat it as "no image change" so the row is skipped
		// unless its policy changed.
		if imgChanged && newImage == "" {
			imgChanged = false
		}

		// Suppress reverts to default: k8s cannot reliably unset an
		// imagePullPolicy via a merge patch, so don't emit that change.
		if policyChanged && newPolicy == "" {
			policyChanged = false
		}

		if !imgChanged && !policyChanged {
			continue
		}

		c := msgs.ContainerImageChange{Name: orig.Name, Init: orig.Init}
		if imgChanged {
			c.Image = newImage
		}
		if policyChanged {
			c.PullPolicy = newPolicy
		}
		changed = append(changed, c)
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
