package ui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/aohoyd/aku/internal/msgs"
)

// buttonCount is the number of buttons in the confirm dialog.
const buttonCount = 3

// button indices
const (
	btnYes   = 0
	btnForce = 1
	btnNo    = 2
)

// ConfirmDialog is an overlay that asks the user to confirm, force, or cancel an action.
type ConfirmDialog struct {
	overlay       Overlay
	focusedButton int
}

// NewConfirmDialog creates a new active confirm dialog with the given message and terminal width.
func NewConfirmDialog(message string, width int) ConfirmDialog {
	o := NewOverlay()
	o.SetActive(true)
	o.SetSize(width, 24)
	o.SetFixedWidth(70)
	o.SetContent(message)
	cd := ConfirmDialog{overlay: o, focusedButton: btnNo}
	cd.updateFooter()
	return cd
}

// Active returns whether the dialog is currently active.
func (c ConfirmDialog) Active() bool {
	return c.overlay.Active()
}

// SetWidth updates the dialog width on terminal resize.
func (c *ConfirmDialog) SetWidth(w int) {
	c.overlay.SetSize(w, 24)
}

// Update handles key messages for the confirm dialog.
func (c ConfirmDialog) Update(msg tea.Msg) (ConfirmDialog, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "y", "Y":
			c.overlay.SetActive(false)
			return c, confirmCmd(msgs.ConfirmYes)
		case "f", "F":
			c.overlay.SetActive(false)
			return c, confirmCmd(msgs.ConfirmForce)
		case "n", "N", "esc":
			c.overlay.SetActive(false)
			return c, confirmCmd(msgs.ConfirmCancel)
		case "enter":
			c.overlay.SetActive(false)
			return c, confirmCmd(c.focusedAction())
		case "left", "shift+tab":
			c.focusedButton = (c.focusedButton - 1 + buttonCount) % buttonCount
			c.updateFooter()
		case "right", "tab":
			c.focusedButton = (c.focusedButton + 1) % buttonCount
			c.updateFooter()
		}
	}
	return c, nil
}

// View renders the confirm dialog as a centered bordered box.
func (c ConfirmDialog) View() string {
	return c.overlay.View()
}

// updateFooter renders the button bar and sets it as the overlay footer.
func (c *ConfirmDialog) updateFooter() {
	buttons := []Button{
		{Label: "Yes", Hotkey: "y"},
		{Label: "Force", Hotkey: "f"},
		{Label: "No", Hotkey: "N"},
	}
	c.overlay.SetFooter(RenderButtonBar(buttons, c.focusedButton))
}

// focusedAction returns the ConfirmAction for the currently focused button.
func (c ConfirmDialog) focusedAction() msgs.ConfirmAction {
	switch c.focusedButton {
	case btnYes:
		return msgs.ConfirmYes
	case btnForce:
		return msgs.ConfirmForce
	default:
		return msgs.ConfirmCancel
	}
}

// confirmCmd returns a tea.Cmd that emits a ConfirmResultMsg.
func confirmCmd(action msgs.ConfirmAction) tea.Cmd {
	return func() tea.Msg {
		return msgs.ConfirmResultMsg{Action: action}
	}
}
