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

### Nav-stack direction invariant

The per-pane nav stack grows in **multiple** directions, encoded on each frame as
`NavSnapshot.Direction NavDirection` (an enum, not a bool): `NavChild` (the zero
value — Enter/`enter-detail`, via the `DrillDowner.DrillDown` method, child-ward),
`NavParent` (Backspace/`nav-parent`, via `DrillUp`, parent-ward), and `NavNode`
(`gN`/`oN`, via `GoToNode`, the pod→hosting-node jump). Escape is the single linear
unwind that pops any kind. `refreshDrillDownSplit` MUST switch on `snap.Direction`
BEFORE the `snap.Plugin.(plugin.DrillDowner)` type-assert: **any non-`NavChild`
direction branches before the `DrillDowner` assert**, because a `NavParent`/`NavNode`
frame's `snap.Plugin` is itself a `DrillDowner` (e.g. pods → containers), so falling
through would re-run that drill-down and clobber the view. On the `NavParent` branch
it delegates to `refreshParentWardSplit` (`internal/app/app.go`), which re-fetches the
source child (via `resolveSnapSourceObject`) and re-runs `DrillUp` to rebuild the
parent. Parent resolution goes through the per-plugin `DrillUp` interface. For
ownerRef-based parents this delegates to the single `workload.FindParentByOwnerRef`
(ownerReferences-based, `controller: true` preferred, else first) — never re-read
`metadata.ownerReferences` inline elsewhere; add a `DrillUp` method instead. Not
every parent is ownerRef-based, though: non-owned parents (e.g. endpointslices → service,
which carry no ownerReference) instead resolve through a label-match resolver
(`workload.FindServiceForEndpointSlice`) per the Service↔EndpointSlice drilldown invariant
below — still behind a `DrillUp` method, just not via `FindParentByOwnerRef`.

On the `NavNode` branch it delegates to `refreshNodeWardSplit` (`internal/app/app.go`),
which re-fetches the source pod (via `resolveSnapSourceObject`) and re-runs the source
plugin's `GoToNode` to rebuild the single-node view. Crucially — and symmetrically with
`refreshParentWardSplit`'s `DrillUp` route — it asserts `snap.Plugin.(plugin.NodeLinker)`
and calls that, NOT `workload.FindNodeForPod` directly: a `NavNode` frame's `snap.Plugin`
is the SOURCE plugin (pods), which IS a `NodeLinker`, so this is the inverse-symmetric of
the parent-ward `DrillUp` route. Pod→node resolution thus goes through the per-plugin
`plugin.NodeLinker` interface (`GoToNode`, implemented by pods), which delegates to the
single `workload.FindNodeForPod` (reads `spec.nodeName`, resolves the `nodes` plugin,
reads the cluster-scoped `""` bucket) — the inverse of `workload.FindPodsByNodeName`, and
the single source for pod→node resolution; never re-read `spec.nodeName` inline elsewhere.

`refreshParentWardSplit` and `refreshNodeWardSplit` both deliberately return WITHOUT
calling `SetObjects` when the source object can't be found, or when `DrillUp` yields a
nil parent / `GoToNode` yields a nil (or not-yet-cached) node (e.g. the target
store bucket isn't synced yet) — they leave the view unchanged so the next
`ResourceUpdatedMsg` can retry. This is the opposite of the child-ward path, which
`SetObjects` even on an empty child set.

`ResourceList.navFloor` is Escape's pop floor. The clear-overlay pop guard in
`commands.go` is `Depth() > NavFloor()`, NOT `InDrillDown()`. Split-opened drills
(`split-children`/`split-parent`/`split-nav-node`) `SetNavFloor(1)` so they live-refresh
(`InDrillDown()`, which stays `Depth() > 0`, still triggers `refreshDrillDownSplits`)
yet Escape can't unwind past their home drill into a root the split never showed.
In-pane drills keep `navFloor = 0`.

### Service↔EndpointSlice drilldown invariant

The `svc → endpointslice → pods` chain and its `endpointslice → svc` parent-ward
`DrillUp` are **label-matched** via `kubernetes.io/service-name`, NOT name-matched
and NOT ownerRef-based: an EndpointSlice carries the `kubernetes.io/service-name`
label pointing at its Service, and a Service may have N sharded slices — so these
drilldowns do NOT go through `workload.FindParentByOwnerRef`.
`workload.FindEndpointSlicesForService` (svc→slices, label match in the svc
namespace, returns N), `workload.FindPodsByEndpointSlice` (slice→pods, scanning the
FLAT `endpoints[]` for `targetRef` with `kind == "Pod"`, INCLUDING all regardless
of `conditions.ready`, deduped by `targetRef.uid` when present else the resolved
pod's namespace/name composite — NOT the pod's `GetUID()`, which would collapse
distinct blank-uid pods), and `workload.FindServiceForEndpointSlice` (slice→svc
`DrillUp`, label match in the slice namespace) are the ONLY resolvers — never
re-scan `endpoints[].targetRef` or re-match by the `kubernetes.io/service-name`
label inline elsewhere; add a method/delegate instead. The
`services`/`endpointslices` plugins reference `workload.ServicesGVR` /
`workload.EndpointSlicesGVR` as the single-source GVRs (mirroring `nodes` →
`workload.NodesGVR`) — never redefine the literals. The manifest fabricator
(`internal/manifest/synth.go` `fabricateEndpointSlice`) stamps the
`kubernetes.io/service-name` label, a flat `endpoints[]` with `targetRef`
(`kind:Pod` and `name` always present; `namespace` and `uid` included when
non-empty and omitted otherwise), `conditions.ready: true`, and top-level
`ports[]` — so manifest-mode `endpointslice → pods` resolves through the same
`FindPodsByEndpointSlice` path as a live cluster. The `endpoints` plugin is now a
plain listable/describable resource with no drilldown.
