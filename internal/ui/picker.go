package ui

import (
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

// PickerConfig holds construction-time options for a Picker.
type PickerConfig[T any] struct {
	Title      string
	NoItemsMsg string
	MaxVisible int // 0 = auto from terminal height
	Display    func(T) string
	Filter     func(query string, items []T) []T
	OnSelect   func(T) tea.Cmd
}

// Picker is a generic filterable selection overlay.
type Picker[T any] struct {
	overlay  Overlay
	cfg      PickerConfig[T]
	all      []T
	filtered []T
}

// NewPicker creates a new Picker with the given config and dimensions.
func NewPicker[T any](cfg PickerConfig[T], width, height int) Picker[T] {
	o := NewOverlay()
	o.SetTitle(cfg.Title)
	o.SetSize(width, height)
	if cfg.NoItemsMsg != "" {
		o.SetNoItemsMsg(cfg.NoItemsMsg)
	}
	if cfg.MaxVisible > 0 {
		o.SetMaxVisible(cfg.MaxVisible)
	}
	return Picker[T]{overlay: o, cfg: cfg}
}

// SetItems replaces the source data and reapplies the filter.
func (p *Picker[T]) SetItems(items []T) {
	p.all = items
	p.applyFilter()
}

// Open activates the picker, resets state, creates the filter input.
func (p *Picker[T]) Open() {
	p.overlay.Reset()
	p.overlay.SetActive(true)
	p.overlay.SetTitle(p.cfg.Title)
	if p.cfg.NoItemsMsg != "" {
		p.overlay.SetNoItemsMsg(p.cfg.NoItemsMsg)
	}
	if p.cfg.MaxVisible > 0 {
		p.overlay.SetMaxVisible(p.cfg.MaxVisible)
	}
	ti := textinput.New()
	ti.Prompt = "Filter: "
	ti.Focus()
	p.overlay.AddInput(ti)
	p.overlay.FocusInput(0)
	p.applyFilter()
}

// Close deactivates the picker.
func (p *Picker[T]) Close() {
	p.overlay.SetActive(false)
}

// Active returns whether the picker is currently displayed.
func (p Picker[T]) Active() bool {
	return p.overlay.Active()
}

// SetSize updates the dimensions.
func (p *Picker[T]) SetSize(w, h int) {
	p.overlay.SetSize(w, h)
}

// Cursor returns the current cursor position.
func (p Picker[T]) Cursor() int {
	return p.overlay.Cursor()
}

// Filtered returns the current filtered item slice.
func (p Picker[T]) Filtered() []T {
	return p.filtered
}

// Update handles key messages for the picker.
func (p Picker[T]) Update(msg tea.Msg) (Picker[T], tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.Code {
		case tea.KeyEnter:
			if len(p.filtered) > 0 && p.overlay.Cursor() < len(p.filtered) {
				selected := p.filtered[p.overlay.Cursor()]
				p.Close()
				return p, p.cfg.OnSelect(selected)
			}
			p.Close()
			return p, nil
		case tea.KeyEscape:
			p.Close()
			return p, nil
		case tea.KeyUp, tea.KeyDown:
			p.overlay.HandleListKeys(msg)
			return p, nil
		default:
			cmd := p.overlay.UpdateInputs(msg)
			p.applyFilter()
			return p, cmd
		}
	}
	return p, nil
}

// View renders the picker overlay.
func (p Picker[T]) View() string {
	return p.overlay.View()
}

// applyFilter runs the filter callback and updates the overlay item list.
func (p *Picker[T]) applyFilter() {
	query := ""
	if p.overlay.InputCount() > 0 {
		query = p.overlay.InputValue(0)
	}
	p.filtered = p.cfg.Filter(query, p.all)
	displays := make([]string, len(p.filtered))
	for i, item := range p.filtered {
		displays[i] = p.cfg.Display(item)
	}
	p.overlay.SetItems(displays)
}
