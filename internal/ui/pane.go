package ui

// PaneKind identifies the content type of a pane. It is distinct from the
// layout-level PaneKind (which classifies screen rects for mouse hit-testing);
// this one describes what a split pane *holds* so a heterogeneous splits list
// can carry both resource panes and (later) terminal panes.
type PaneKind int

const (
	PaneResources PaneKind = iota // a resource-list pane
	PaneTerminal                  // a live terminal pane
)

// Pane is the common interface for anything that can live in the layout's
// splits list. Both *ResourceList and *TerminalPane satisfy it now, so the
// splits list is heterogeneous. The method signatures mirror ResourceList's
// existing methods exactly so adopting the interface was a pure refactor.
type Pane interface {
	SetSize(w, h int)
	View() string
	Focus()
	Blur()
	Title() string
	Context() string
	Kind() PaneKind
}
