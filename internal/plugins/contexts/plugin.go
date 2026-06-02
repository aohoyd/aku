// Package contexts provides a synthetic _ktui resource plugin that lists the
// kube-contexts discovered from the kubeconfig(s) as a selectable table. It is
// modeled on the portforwards plugin: SelfPopulating, cluster-scoped, and fed
// from an in-process source (the cluster.Manager) rather than a real informer.
//
// Selecting a row maps (via Commander) to the "pane-switch-context <name>" app
// command, which switches the focused pane's context. Building the row set
// performs only local work — kubeconfig file reads and a lookup of the
// Manager's already-known per-context state — and never dials a cluster or runs
// a blocking health probe.
package contexts

import (
	"context"

	"github.com/aohoyd/aku/internal/cluster"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/render"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

var (
	_ plugin.ResourcePlugin  = (*Plugin)(nil)
	_ plugin.SelfPopulating  = (*Plugin)(nil)
	_ plugin.Commander       = (*Plugin)(nil)
	_ plugin.Sortable        = (*Plugin)(nil)
	_ plugin.DefaultSorter   = (*Plugin)(nil)
	_ plugin.Refreshable     = (*Plugin)(nil)
	_ plugin.PaneCountSetter = (*Plugin)(nil)
)

var syntheticGVR = schema.GroupVersionResource{
	Group: "_ktui", Version: "v1", Resource: "contexts",
}

// STATUS glyphs. A context in use by at least one pane shows a filled ● —
// colored green when its cluster is connected and red when it is not (offline or
// failed to connect). A context no pane uses shows a neutral hollow ○.
const (
	glyphInUse = "●"
	glyphIdle  = "○"
)

// Plugin lists kube-contexts as a synthetic resource view.
//
// It holds the cluster.Manager (the source of context entries and per-context
// connection state) plus a settable per-context pane-count map. The pane counts
// live in the App layer (App.distinctPaneContexts); the App pushes the current
// counts in via SetPaneCounts before/while the view is shown. They are no longer
// shown as a column — they drive the STATUS glyph: a context with no panes is
// idle (○), while a context with panes is in use (●, green if connected, red if
// not). Keeping the count as a plain settable field avoids giving the plugin a
// back-reference to the App and keeps Objects() free of any network traversal.
type Plugin struct {
	mgr        *cluster.Manager
	paneCounts map[string]int
}

// New creates a contexts plugin over the given Manager.
func New(mgr *cluster.Manager) *Plugin {
	return &Plugin{mgr: mgr}
}

// SetPaneCounts records the current per-context pane counts. They are not shown
// as a column; they drive the STATUS glyph (a context with >0 panes is "in use",
// so it renders a filled ● colored by connectivity; 0 panes renders a hollow ○).
// The App supplies this from distinctPaneContexts(); a nil map is treated as
// all-zero. The map is used read-only, so the caller may keep mutating its own
// copy after the call only if it passes a fresh map each time.
func (p *Plugin) SetPaneCounts(counts map[string]int) {
	p.paneCounts = counts
}

// PaneCount returns the number of panes recorded for ctx by the last
// SetPaneCounts (0 if none). It exposes the pane-presence signal that drives the
// STATUS glyph for introspection and tests.
func (p *Plugin) PaneCount(ctx string) int {
	return p.paneCounts[ctx]
}

func (p *Plugin) Name() string                     { return "contexts" }
func (p *Plugin) ShortName() string                { return "ctx" }
func (p *Plugin) GVR() schema.GroupVersionResource { return syntheticGVR }
func (p *Plugin) IsClusterScoped() bool            { return true }

func (p *Plugin) Columns() []plugin.Column {
	return []plugin.Column{
		{Title: "NAME", Flex: true},
		{Title: "CLUSTER", Flex: true},
		{Title: "SERVER", Flex: true},
		{Title: "STATUS", Width: 8},
	}
}

// Objects implements plugin.SelfPopulating. It builds one unstructured object
// per Manager entry, encoding the fields Row() needs. CLUSTER and SERVER are
// resolved from the entry's kubeconfig file (a local read, cached per call);
// STATUS is derived from the Manager's already-known per-context state plus the
// pane counts from the last SetPaneCounts. No network call or blocking probe
// occurs.
func (p *Plugin) Objects() []*unstructured.Unstructured {
	if p.mgr == nil {
		return nil
	}
	entries := p.mgr.Entries()
	// Cache parsed kubeconfig files so multiple contexts in the same file are
	// only loaded once per Objects() call. A nil value caches a load failure.
	parsed := make(map[string]*clientcmdapi.Config)
	objs := make([]*unstructured.Unstructured, 0, len(entries))
	for _, e := range entries {
		clusterName, server := p.clusterAndServer(parsed, e)
		objs = append(objs, &unstructured.Unstructured{
			Object: map[string]any{
				"metadata": map[string]any{
					"name":              e.Name,
					"creationTimestamp": nil,
				},
				"cluster": clusterName,
				"server":  server,
				"status":  p.status(e.Name),
			},
		})
	}
	return objs
}

// clusterAndServer resolves the kubeconfig cluster nickname and API server host
// for the given entry by reading its kubeconfig file. Failures (unreadable file,
// missing context/cluster) degrade gracefully to empty strings — never an error
// or a network call. parsed caches loaded files across calls within one
// Objects() pass.
func (p *Plugin) clusterAndServer(parsed map[string]*clientcmdapi.Config, e cluster.ContextEntry) (clusterName, server string) {
	cfg, seen := parsed[e.File]
	if !seen {
		loaded, err := clientcmd.LoadFromFile(e.File)
		if err != nil {
			loaded = nil
		}
		parsed[e.File] = loaded
		cfg = loaded
	}
	if cfg == nil {
		return "", ""
	}
	// Resolve directly off the raw api.Config to avoid building a REST config,
	// which could trigger an exec credential plugin (network/blocking).
	ctx, ok := cfg.Contexts[e.Name]
	if !ok || ctx == nil {
		return "", ""
	}
	clusterName = ctx.Cluster
	if cl, ok := cfg.Clusters[clusterName]; ok && cl != nil {
		server = cl.Server
	}
	return clusterName, server
}

// status returns the colored STATUS glyph for ctx, without dialing. The glyph
// reflects pane usage and the Manager's already-known connection state:
//   - green ● — the cluster is connected;
//   - red ●   — at least one pane uses ctx but it is not connected (offline or
//     a failed dial, which leaves no Manager entry — pane presence is the only
//     signal in that case);
//   - neutral ○ — no pane uses ctx (idle).
func (p *Plugin) status(ctx string) string {
	c, ok := p.mgr.Get(ctx)
	if ok && c.Connected() {
		return plugin.StyledFg(glyphInUse, plugin.FgRunning)
	}
	if p.paneCounts[ctx] > 0 {
		return plugin.StyledFg(glyphInUse, plugin.FgFailed)
	}
	return glyphIdle
}

// Row reads the fields encoded by Objects(). The found bool from NestedString is
// intentionally ignored: Row only ever receives objects produced by Objects()
// above, which always sets these fields, so a missing field would be a
// programming error and the zero value renders fine.
func (p *Plugin) Row(obj *unstructured.Unstructured) []string {
	name := obj.GetName()
	clusterName, _, _ := unstructured.NestedString(obj.Object, "cluster")
	server, _, _ := unstructured.NestedString(obj.Object, "server")
	status, _, _ := unstructured.NestedString(obj.Object, "status")
	return []string{name, clusterName, server, status}
}

func (p *Plugin) YAML(obj *unstructured.Unstructured) (render.Content, error) {
	return plugin.MarshalYAML(obj)
}

func (p *Plugin) Describe(_ context.Context, obj *unstructured.Unstructured) (render.Content, error) {
	b := render.NewBuilder()
	b.KV(render.LEVEL_0, "Context", obj.GetName())
	clusterName, _, _ := unstructured.NestedString(obj.Object, "cluster")
	b.KV(render.LEVEL_0, "Cluster", clusterName)
	server, _, _ := unstructured.NestedString(obj.Object, "server")
	b.KV(render.LEVEL_0, "Server", server)
	status, _, _ := unstructured.NestedString(obj.Object, "status")
	b.KV(render.LEVEL_0, "Status", status)
	return b.Build(), nil
}

// Command implements plugin.Commander: Enter on a context row switches the
// focused pane to that context. The pane-switch-context handler is wired in the
// app layer.
func (p *Plugin) Command(obj *unstructured.Unstructured) (string, bool) {
	name := obj.GetName()
	if name == "" {
		return "", false
	}
	return "pane-switch-context " + name, true
}

// SortValue implements plugin.Sortable. All columns fall back to built-in
// (lexical) handling; NAME is the default sort (see DefaultSort).
func (p *Plugin) SortValue(_ *unstructured.Unstructured, _ string) string {
	return ""
}

// DefaultSort implements plugin.DefaultSorter: sort by NAME ascending.
func (p *Plugin) DefaultSort() plugin.SortPreference {
	return plugin.SortPreference{Column: "NAME", Ascending: true}
}

// Refresh implements plugin.Refreshable. The object list is rebuilt on demand in
// Objects(), so Refresh is a no-op kept to satisfy the refresh contract (the
// namespace argument is irrelevant for cluster-scoped contexts).
func (p *Plugin) Refresh(string) {}
