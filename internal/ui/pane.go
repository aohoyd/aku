package ui

// Pane is the common interface for anything that can live in the layout's
// splits list. Both *ResourceList and *TerminalPane satisfy it now, so the
// splits list is heterogeneous. The method signatures mirror ResourceList's
// existing methods exactly so adopting the interface was a pure refactor.
//
// Dispatch on the concrete pane kind is done via type assertions
// (e.g. pane.(*TerminalPane)), not a Kind() tag, so the interface stays minimal.
type Pane interface {
	SetSize(w, h int)
	View() string
	Focus()
	Blur()
	Title() string
	Context() string
}
