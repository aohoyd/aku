package ui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/aohoyd/aku/internal/msgs"
)

// HelmRevisionEntry holds display data for one revision.
type HelmRevisionEntry struct {
	Revision int
	Display  string
}

// HelmRollbackOverlay shows revision history and lets the user pick one.
type HelmRollbackOverlay struct {
	overlay     Overlay
	revisions   []HelmRevisionEntry
	releaseName string
	namespace   string
}

// NewHelmRollbackOverlay creates a new rollback overlay with the given dimensions.
func NewHelmRollbackOverlay(width, height int) HelmRollbackOverlay {
	o := NewOverlay()
	o.SetSize(width, height)
	return HelmRollbackOverlay{overlay: o}
}

// Open activates the overlay with the given revision list.
func (h *HelmRollbackOverlay) Open(name, namespace string, revisions []HelmRevisionEntry) {
	h.releaseName = name
	h.namespace = namespace
	h.revisions = revisions
	h.overlay.Reset()
	h.overlay.SetActive(true)
	h.overlay.SetTitle("Rollback " + name)
	items := make([]string, len(revisions))
	for i, r := range revisions {
		items[i] = r.Display
	}
	h.overlay.SetItems(items)
}

// Active returns whether the overlay is currently active.
func (h HelmRollbackOverlay) Active() bool { return h.overlay.Active() }

// View renders the overlay panel.
func (h HelmRollbackOverlay) View() string { return h.overlay.View() }

// SetSize updates the terminal dimensions available to the overlay.
func (h *HelmRollbackOverlay) SetSize(w, height int) { h.overlay.SetSize(w, height) }

// Update handles key events for the rollback overlay.
func (h HelmRollbackOverlay) Update(msg tea.Msg) (HelmRollbackOverlay, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.Code {
		case tea.KeyEscape:
			h.overlay.SetActive(false)
			return h, nil
		case tea.KeyEnter:
			idx := h.overlay.Cursor()
			if idx >= 0 && idx < len(h.revisions) {
				rev := h.revisions[idx]
				h.overlay.SetActive(false)
				return h, func() tea.Msg {
					return msgs.HelmRollbackRequestedMsg{
						ReleaseName: h.releaseName,
						Namespace:   h.namespace,
						Revision:    rev.Revision,
					}
				}
			}
			return h, nil
		default:
			h.overlay.HandleListKeys(msg)
			return h, nil
		}
	}
	return h, nil
}
