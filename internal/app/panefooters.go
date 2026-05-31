package app

// syncPaneFooters updates every split's bottom-border footer so that a pane
// shows its context name only when that context differs from the global one.
// Panes that follow the global context (or whose context is empty/unresolved)
// show no footer.
//
// The footer reuses the bottom border line (see ui.injectBorderFooter), so it
// does not consume a content row and the table height is unchanged.
//
// App is a value type, but a.layout.SplitAt(i) returns a *ui.ResourceList that
// points into the layout's backing slice, so the SetContextFooter mutations are
// visible to subsequent renders without reassigning a.layout.
func (a App) syncPaneFooters() {
	global := a.mgr.GlobalContext()
	for i := 0; i < a.layout.SplitCount(); i++ {
		split := a.layout.SplitAt(i)
		if ctx := split.Context(); ctx != "" && ctx != global {
			split.SetContextFooter(ctx)
		} else {
			split.SetContextFooter("")
		}
	}
}
