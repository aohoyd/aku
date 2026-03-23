package ui

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/aohoyd/aku/internal/msgs"
)

// PortItem represents a container port available for forwarding.
type PortItem struct {
	ContainerName string
	Port          int32
	Protocol      string
}

// portItemDisplay returns the display string for a port item.
func portItemDisplay(p PortItem) string {
	return fmt.Sprintf("%s:%d/%s", p.ContainerName, p.Port, p.Protocol)
}

// PortForward button indices.
const (
	pfBtnYes = 0
	pfBtnNo  = 1
)

// PortForwardOverlay is a single-view overlay for setting up port-forwards.
// It shows a filter input, a scrollable port list, and a local port input.
type PortForwardOverlay struct {
	overlay       Overlay
	allPorts      []PortItem
	filtered      []PortItem
	podName       string
	podNs         string
	inputFocused  bool
	focusedButton int
}

// NewPortForwardOverlay creates a new port-forward overlay with the given dimensions.
func NewPortForwardOverlay(width, height int) PortForwardOverlay {
	o := NewOverlay()
	o.SetSize(width, height)
	return PortForwardOverlay{overlay: o}
}

// Open activates the overlay for a given pod, showing its available ports.
func (p *PortForwardOverlay) Open(podName, podNs string, ports []PortItem) {
	p.podName = podName
	p.podNs = podNs
	p.allPorts = ports
	p.inputFocused = true
	p.focusedButton = pfBtnYes

	p.overlay.Reset()
	p.overlay.SetActive(true)
	p.overlay.SetTitle("Port Forward")
	p.overlay.SetNoItemsMsg("(no ports found)")
	p.overlay.SetMaxVisible(3)

	// Input 0: filter
	filter := textinput.New()
	filter.Prompt = "Filter: "
	filter.Focus()
	p.overlay.AddInput(filter)

	// Input 1: local port (renders below the list)
	local := textinput.New()
	local.Prompt = "Local: "
	p.overlay.AddInput(local)
	p.overlay.SetPostListInputIdx(1)

	p.overlay.FocusInput(0)
	p.applyFilter()
	p.updateFooter()
}

// Close deactivates the overlay.
func (p *PortForwardOverlay) Close() {
	p.overlay.SetActive(false)
}

// Active returns whether the overlay is currently active.
func (p PortForwardOverlay) Active() bool {
	return p.overlay.Active()
}

// InputFocused returns whether a text input has focus (vs the button bar).
func (p PortForwardOverlay) InputFocused() bool {
	return p.inputFocused
}

// FocusedButton returns the currently focused button index.
func (p PortForwardOverlay) FocusedButton() int {
	return p.focusedButton
}

// SetSize updates the terminal dimensions available to the overlay.
func (p *PortForwardOverlay) SetSize(w, h int) {
	p.overlay.SetSize(w, h)
}

// View renders the overlay panel.
func (p PortForwardOverlay) View() string {
	return p.overlay.View()
}

// Update handles key messages for the port-forward overlay.
func (p PortForwardOverlay) Update(msg tea.Msg) (PortForwardOverlay, tea.Cmd) {
	km, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return p, nil
	}

	switch km.Code {
	case tea.KeyEscape:
		p.Close()
		return p, nil

	case tea.KeyEnter:
		if p.inputFocused {
			return p.handleSubmit()
		}
		if p.focusedButton == pfBtnYes {
			return p.handleSubmit()
		}
		// No button selected.
		p.Close()
		return p, nil

	case tea.KeyTab:
		p.cycleFocusForward()
		return p, nil

	case tea.KeyUp, tea.KeyDown:
		if p.inputFocused {
			p.overlay.HandleListKeys(km)
			p.syncLocalPort()
			return p, nil
		}
		// From buttons, Up moves focus to local port input.
		if km.Code == tea.KeyUp {
			p.focusToInput(1)
		}
		return p, nil

	case tea.KeyLeft:
		if !p.inputFocused {
			p.focusedButton = pfBtnYes
			p.updateFooter()
		}
		return p, nil

	case tea.KeyRight:
		if !p.inputFocused {
			p.focusedButton = pfBtnNo
			p.updateFooter()
		}
		return p, nil

	default:
		if !p.inputFocused {
			// Button hotkeys.
			switch km.String() {
			case "y", "Y":
				return p.handleSubmit()
			case "n", "N":
				p.Close()
				return p, nil
			}
			return p, nil
		}
		focus := p.overlay.FocusedInput()
		cmd := p.overlay.UpdateInputs(km)
		if focus == 0 {
			p.applyFilter()
		}
		return p, cmd
	}
}

func (p PortForwardOverlay) handleSubmit() (PortForwardOverlay, tea.Cmd) {
	if len(p.filtered) == 0 {
		return p, nil
	}

	val := p.overlay.InputValue(1)
	port, err := parseLocalPort(val)
	if err != nil {
		p.overlay.SetFooter("Invalid port (1-65535)")
		return p, nil
	}

	idx := p.overlay.Cursor()
	if idx >= len(p.filtered) {
		return p, nil
	}
	selected := p.filtered[idx]
	p.Close()
	return p, func() tea.Msg {
		return msgs.PortForwardRequestedMsg{
			PodName:       p.podName,
			PodNamespace:  p.podNs,
			ContainerName: selected.ContainerName,
			LocalPort:     port,
			RemotePort:    int(selected.Port),
			Protocol:      selected.Protocol,
		}
	}
}

// parseLocalPort extracts and validates a port from "localhost:NNNN" or just "NNNN".
func parseLocalPort(val string) (int, error) {
	portStr := val
	if idx := strings.LastIndex(val, ":"); idx >= 0 {
		portStr = val[idx+1:]
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("invalid port")
	}
	return port, nil
}

// cycleFocusForward cycles: filter(0) -> list(-1) -> local port(1) -> buttons -> filter(0)
func (p *PortForwardOverlay) cycleFocusForward() {
	if !p.inputFocused {
		// buttons -> filter
		p.focusToInput(0)
		return
	}
	switch p.overlay.FocusedInput() {
	case 0: // filter -> list
		p.overlay.FocusList()
	case -1: // list -> local port
		p.overlay.FocusInput(1)
	case 1: // local port -> buttons
		p.focusToButtons()
	}
}

// focusToInput moves focus to the text input at the given index.
func (p *PortForwardOverlay) focusToInput(idx int) {
	p.inputFocused = true
	p.overlay.FocusInput(idx)
	p.updateFooter()
}

// focusToButtons moves focus to the button bar.
func (p *PortForwardOverlay) focusToButtons() {
	p.inputFocused = false
	p.overlay.blurAll()
	p.updateFooter()
}

// updateFooter renders the Yes/No button bar as the overlay footer.
func (p *PortForwardOverlay) updateFooter() {
	buttons := []Button{
		{Label: "Yes", Hotkey: "y"},
		{Label: "No", Hotkey: "n"},
	}
	focused := p.focusedButton
	if p.inputFocused {
		focused = -1
	}
	p.overlay.SetFooter(RenderButtonBar(buttons, focused))
}

// syncLocalPort updates the local port input to match the currently selected port.
// Skips when the local port input has focus (to avoid overwriting user edits).
func (p *PortForwardOverlay) syncLocalPort() {
	if p.overlay.FocusedInput() == 1 {
		return
	}
	if len(p.filtered) == 0 {
		p.overlay.Input(1).SetValue("")
		return
	}
	idx := p.overlay.Cursor()
	if idx >= 0 && idx < len(p.filtered) {
		p.overlay.Input(1).SetValue(fmt.Sprintf("localhost:%d", p.filtered[idx].Port))
	}
}

func (p *PortForwardOverlay) applyFilter() {
	query := ""
	if p.overlay.InputCount() > 0 {
		query = p.overlay.InputValue(0)
	}
	lower := strings.ToLower(query)
	p.filtered = nil
	for _, item := range p.allPorts {
		display := portItemDisplay(item)
		if query == "" || strings.Contains(strings.ToLower(display), lower) {
			p.filtered = append(p.filtered, item)
		}
	}
	displayItems := make([]string, len(p.filtered))
	for i, item := range p.filtered {
		displayItems[i] = portItemDisplay(item)
	}
	p.overlay.SetItems(displayItems)
	p.syncLocalPort()
}
