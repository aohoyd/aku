# CLAUDE.md

Project-specific conventions and invariants for aku.

## Conventions

### Secret/ConfigMap data extraction invariant

`containers.IndexByName`, `containers.ConfigMapData`, and `containers.SecretData`
(`internal/plugins/containers/resolvers.go`) are the single source of truth for
turning `*unstructured.Unstructured` ConfigMaps/Secrets into a `map[string]string`:
`IndexByName` keys objects by `metadata.name`; `ConfigMapData` returns `.data`
as-is and base64-decodes `.binaryData`; `SecretData` base64-decodes both `.data`
and `.binaryData` (raw fallback on decode error). For both, `.data` keys win over
`.binaryData` keys, and a nil map is returned when neither field has entries.
The `env`/`envFrom` resolvers, `DescribeContainer` (describe.go), and the pods
volume/imagePullSecret reveal all index via `IndexByName` and read through these —
never re-decode `.data`/`.binaryData` inline; reuse these primitives. The
imagePullSecret non-docker fallback intentionally calls `SecretData` but emits
keys only (values suppressed).

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

### Manifest pseudo-context invariant

The `manifests` context (manifest-visualization mode) is a **pinned, nil-client
static cluster** installed via `Manager.RegisterPinned` and identified by
`Manager.IsPinned(ctx)`. Pinned clusters are NOT ordinary connected clusters:
they appear in `Entries()`, are exempt from `SyncRefs` teardown (a pinned cluster
with zero open panes is never deleted and its cache is never cleared), and are
returned verbatim on re-select — never re-dialed through `connect →
RegisterConnected` (which would build a fresh empty store). Code that tears down,
re-dials, or repopulates clusters by walking refs must skip pinned ones; `Ctrl+r`
on a manifest cluster *rebuilds and re-registers* it (informer-based
`UnsubscribeAll`/re-subscribe won't repopulate a static store).

Because `k8s.Store.List(gvr, ns)` reads only the `(gvr, ns)` bucket, every
namespaced manifest object is `CacheUpsert`'d twice — into its own-namespace
bucket AND the `""` (All Namespaces) bucket — so per-namespace and "All
Namespaces" views both populate.

Manifest provenance rides as the `manifest.SourceAnnotation`
(`aku.dev/manifest-source`) metadata annotation: the `# Source:` path for parsed
objects, the literal `"synthesized"` for fabricated ones. It surfaces in the YAML
view for every kind for free — do not add per-plugin describe plumbing for it.
