# CLAUDE.md

Project-specific conventions and invariants for aku.

## Conventions

### Pane focus invariant

`Layout.reconcileFocus()` is the single source of truth for pane + detail-panel
focus/selection state. Focus transitions (`FocusNext`/`FocusPrev`,
`FocusSplitAt`, `MoveFocusedSplit`, `AddSplit`, `AddTerminalSplit`,
`CloseCurrentSplit`, `FocusDetails`, `FocusResources`) must only mutate
`focusIdx`/`focusTarget` then call `reconcileFocus()` — never hand-roll
Blur/Focus loops. (`CloseCurrentSplit` skips `reconcileFocus()` on its quit
path — when it returns `true` for the last split — because the app exits rather
than re-renders.)

Cursor-row *styling* in table.go `renderRow` is driven by `RowStyleFunc` + the
live `selectionActive` flag and never consults the table's internal `focus` flag;
that flag only gates keyboard movement in the table's `Update()`. `selectionActive`
is a field on `table.Model` (set via `SetSelectionActive`), read live by
`renderRow`, and passed as the `active` arg to `RowStyleFunc(index, isCursor,
active)`. There is no `styles.Selected` accent/dim swap anymore (and
`TableSelectedDimStyle` is gone): the cursor row gets a cursor style only when
`selectionActive` is true — identically for healthy and unhealthy rows, differing
only in style (accent fill vs red/yellow fill); when `selectionActive` is false the
cursor row renders like any other (plain, or just the health fg-tint).

`ResourceList.applyFocus(border, selection)` is the ONLY setter of the two focus
bits: `focused` drives border + keyboard input (→ `table.Focus()/Blur()`), and
`selectionActive` drives cursor visibility (→ `table.SetSelectionActive`). The three
methods `Focus`/`Blur`/`BlurBorder` are thin wrappers over it.
