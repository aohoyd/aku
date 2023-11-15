package ui

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/aohoyd/aku/internal/msgs"
)

// ChartInputOverlay prompts for a chart source (OCI ref or local path).
type ChartInputOverlay struct {
	overlay     Overlay
	releaseName string
	namespace   string
}

func NewChartInputOverlay(width, height int) ChartInputOverlay {
	o := NewOverlay()
	o.SetSize(width, height)
	return ChartInputOverlay{overlay: o}
}

func (c *ChartInputOverlay) Open(releaseName, namespace, currentRef string) {
	c.releaseName = releaseName
	c.namespace = namespace

	c.overlay.Reset()
	c.overlay.SetActive(true)
	c.overlay.SetTitle("Chart Source: " + releaseName)
	c.overlay.SetFixedWidth(70)

	ti := textinput.New()
	ti.Prompt = "Chart: "
	ti.Placeholder = "oci://registry/chart or /path/to/chart"
	ti.SetValue(currentRef)
	c.overlay.AddInput(ti)
	c.overlay.FocusInput(0)
}

func (c *ChartInputOverlay) Close() {
	c.overlay.SetActive(false)
}

func (c ChartInputOverlay) Active() bool {
	return c.overlay.Active()
}

func (c *ChartInputOverlay) SetSize(w, h int) {
	c.overlay.SetSize(w, h)
}

func (c ChartInputOverlay) View() string {
	return c.overlay.View()
}

func (c ChartInputOverlay) Update(msg tea.Msg) (ChartInputOverlay, tea.Cmd) {
	km, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return c, nil
	}

	switch km.Code {
	case tea.KeyEscape:
		c.Close()
		return c, nil

	case tea.KeyEnter:
		return c.handleSubmit()

	case tea.KeyTab:
		return c, nil

	default:
		cmd := c.overlay.UpdateInputs(km)
		return c, cmd
	}
}

func (c ChartInputOverlay) handleSubmit() (ChartInputOverlay, tea.Cmd) {
	ref := c.overlay.InputValue(0)
	c.Close()

	if ref == "" {
		return c, nil
	}

	return c, func() tea.Msg {
		return msgs.HelmChartRefSetMsg{
			ReleaseName: c.releaseName,
			Namespace:   c.namespace,
			ChartRef:    ref,
		}
	}
}
