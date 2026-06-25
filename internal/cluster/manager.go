package cluster

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/aohoyd/aku/internal/k8s"
)

// ContextEntry names a kube-context and the kubeconfig file it lives in. The
// kubeconfig scan and the Manager both use this type, so it is defined here once.
type ContextEntry struct {
	Name string
	File string
}

// Manager owns one *Cluster per kube-context. Clusters are created lazily on
// first use (GetOrCreate) or installed from an off-thread dial (Dial +
// RegisterConnected). Reference counts are reconciled against the set of
// contexts panes currently reference via SyncRefs; a cluster whose pane count
// reaches zero is torn down. The Manager holds no notion of a "global" or
// default context — every caller passes an explicit context name.
//
// Thread-safety: all methods are intended to be called from the single Bubble
// Tea Update goroutine, like the rest of the app state. No internal locking is
// used; if the Manager is ever shared across goroutines the caller must
// serialize access. Keeping it lock-free avoids any chance of deadlock between
// methods that call one another (e.g. RegisterConnected -> Get). The sole
// exception is Dial, which is map-free and safe to run off-thread.
type Manager struct {
	clusters   map[string]*Cluster
	entries    []ContextEntry
	kubeconfig string
	send       func(tea.Msg)
	apiTimeout time.Duration

	// pinned names contexts whose cluster is preserved across the whole session:
	// it is never torn down by SyncRefs (even at zero pane refs) and its store is
	// never UnsubscribeAll'd. Used for the synthetic "manifests" pseudo-context,
	// whose pre-populated store has no informers to reconcile. The dial/connect
	// path also short-circuits for pinned names so the stored cluster is returned
	// verbatim rather than rebuilt as an empty store.
	pinned map[string]bool

	// connect builds a Client for a (file, ctx) pair. Injectable so tests can
	// avoid touching real clusters. file may be "" to use the Manager's
	// default kubeconfig path.
	connect func(file, ctx string) (*k8s.Client, error)
}

// NewManager creates a Manager over the given context entries. kubeconfig is
// the fallback kubeconfig path used when a context has no associated file.
// apiTimeout is retained for callers (e.g. heartbeat / discovery deadlines);
// the Manager does not impose it on connect itself.
func NewManager(entries []ContextEntry, kubeconfig string, apiTimeout time.Duration) *Manager {
	m := &Manager{
		clusters:   make(map[string]*Cluster),
		entries:    entries,
		kubeconfig: kubeconfig,
		apiTimeout: apiTimeout,
		pinned:     make(map[string]bool),
	}
	m.connect = func(file, ctx string) (*k8s.Client, error) {
		if file == "" {
			file = m.kubeconfig
		}
		return k8s.NewClient(file, ctx, "")
	}
	return m
}

// Register installs a pre-built Cluster under its context name. It is a seam
// for callers and tests that construct a Cluster directly (via cluster.New) and
// want the Manager to resolve it through Get. An existing cluster for the same
// context is replaced.
func (m *Manager) Register(c *Cluster) {
	m.clusters[c.context] = c
}

// RegisterPinned installs a pre-built Cluster as a persistent pseudo-context
// that the Manager preserves for the whole session. Unlike Register, a pinned
// cluster:
//
//   - appears in Entries() (so the gx/oX/contexts views, which build their lists
//     from Entries(), can offer it for selection);
//   - is NEVER torn down by SyncRefs even with zero referencing panes, and its
//     store is never UnsubscribeAll'd (it has no informers to reconcile);
//   - is returned verbatim by the get/dial path — GetOrCreate short-circuits on
//     the cached pinned cluster and never re-dials/rebuilds an empty store.
//
// It is used for the synthetic "manifests" cluster built by internal/manifest.
// Re-registering the same context replaces the stored cluster but never adds a
// duplicate Entries() row.
func (m *Manager) RegisterPinned(c *Cluster) {
	m.clusters[c.context] = c
	m.pinned[c.context] = true
	for _, e := range m.entries {
		if e.Name == c.context {
			return
		}
	}
	m.entries = append(m.entries, ContextEntry{Name: c.context})
}

// IsPinned reports whether ctx is a pinned pseudo-context (e.g. the synthetic
// "manifests" cluster). Pinned clusters carry no client, so callers that derive
// a default namespace from the client treat the all-namespaces view ("") as the
// sensible default for them.
func (m *Manager) IsPinned(ctx string) bool {
	return m.pinned[ctx]
}

// SetConnect overrides the function used to build a Client for a (file, ctx)
// pair. It is the injection seam described in the design: tests in other
// packages (e.g. internal/app) call it before GetOrCreate to wire a
// fake or no-op client without touching a real cluster. A nil fn is ignored.
func (m *Manager) SetConnect(fn func(file, ctx string) (*k8s.Client, error)) {
	if fn != nil {
		m.connect = fn
	}
}

// SetSend records the send function and applies it to any already-created
// clusters' stores (and warning handlers). Stores created later pick it up in
// GetOrCreate.
func (m *Manager) SetSend(send func(tea.Msg)) {
	m.send = send
	for _, c := range m.clusters {
		if c.store != nil {
			c.store.SetSend(send)
		}
		if c.client != nil && c.client.WarningHandler != nil {
			c.client.WarningHandler.SetSend(send)
		}
	}
}

// fileFor returns the kubeconfig file associated with ctx from the entries,
// falling back to "" (which connect resolves to the default kubeconfig path).
func (m *Manager) fileFor(ctx string) string {
	for _, e := range m.entries {
		if e.Name == ctx {
			return e.File
		}
	}
	return ""
}

// GetOrCreate returns the Cluster for ctx, creating and caching it on first
// use. A cached cluster is returned as-is. This is a lookup-or-create: it does
// NOT change the reference count. Callers must pass an explicit context name;
// an empty ctx is only meaningful at startup, where it resolves to the
// kubeconfig current-context and the resulting cluster is cached under that
// resolved name (so the caller can read Cluster.Context() to learn the explicit
// startup context to seed panes with).
//
// On connect failure a degraded Cluster (Connected()==false, Err set) is
// returned together with the error but is NOT cached, so a subsequent call
// re-dials. This keeps a transient connect failure from permanently blocking a
// context switch. On success the Store and Discovery are built and the send
// function (if set) is wired.
func (m *Manager) GetOrCreate(ctx string) (*Cluster, error) {
	if c, ok := m.clusters[ctx]; ok {
		return c, c.err
	}

	// A pinned pseudo-context (e.g. "manifests") must never go through the connect
	// path — doing so would build a fresh empty store and discard the pre-populated
	// one. If it is pinned but not in the map it was torn down erroneously; return
	// a degraded cluster rather than dialing a context that has no real client.
	if m.pinned[ctx] {
		return &Cluster{context: ctx}, nil
	}

	file := m.fileFor(ctx)
	client, err := m.connect(file, ctx)
	if err != nil {
		// Return a degraded (unstored) cluster so the caller can inspect Err();
		// the next GetOrCreate retries the dial.
		return &Cluster{context: ctx, file: file, err: err}, err
	}

	// When ctx was empty (startup current-context), stamp and cache the cluster
	// under the resolved context name the client reports, so later lookups by the
	// explicit name succeed.
	resolved := ctx
	if resolved == "" {
		resolved = client.Context
	}

	store := k8s.NewStore(client.Dynamic, resolved, m.send)
	if m.send != nil && client.WarningHandler != nil {
		client.WarningHandler.SetSend(m.send)
	}

	c := &Cluster{
		context:   resolved,
		file:      file,
		client:    client,
		store:     store,
		discovery: k8s.NewDiscovery(),
	}
	m.clusters[resolved] = c
	return c, nil
}

// Get returns the cached Cluster for ctx without creating it and without
// touching the reference count.
func (m *Manager) Get(ctx string) (*Cluster, bool) {
	c, ok := m.clusters[ctx]
	return c, ok
}

// Dial builds a Client for ctx using the configured connect function WITHOUT
// touching m.clusters or any refCount. It is the ONLY Manager method that is
// safe to call off the Bubble Tea Update goroutine: it performs the blocking
// k8s.NewClient dial (REST/raw-config reads) and returns the resulting client
// (or error). The returned *k8s.Client is an immutable handle; passing it back
// to the Update goroutine via a message is safe because it shares no mutable
// Manager state. The Update goroutine then calls RegisterConnected to install
// it. Callers must pass an explicit context name.
//
// Keeping Dial map-free preserves the Manager's no-lock, single-goroutine
// invariant: state mutation stays on the Update goroutine; only the dial runs
// off-thread.
func (m *Manager) Dial(ctx string) (*k8s.Client, error) {
	return m.connect(m.fileFor(ctx), ctx)
}

// RegisterConnected installs a Cluster for ctx built from an already-dialed
// client (typically produced off-thread by Dial). It must run on the Update
// goroutine. If a cluster for ctx already exists it is returned as-is (the
// freshly-dialed client is discarded), so a redundant connect cannot replace a
// live cluster out from under panes already using it. This does NOT change the
// reference count — refcounts are reconciled separately via SyncRefs. Callers
// must pass an explicit context name.
//
// The bool reports whether a NEW cluster was installed (true) or an existing one
// was returned from cache (false). Callers use it to avoid starting a duplicate
// heartbeat/discovery loop when a second pane connects to an already-live
// context (handleClusterHealth re-arms the heartbeat per tick, so a second loop
// would compound the probe rate).
func (m *Manager) RegisterConnected(ctx string, client *k8s.Client) (*Cluster, bool) {
	if c, ok := m.clusters[ctx]; ok {
		return c, false
	}
	// A pinned pseudo-context never accepts a dialed client; it carries no real
	// connection. Return a degraded cluster reporting not-newly so callers do not
	// start a heartbeat loop against it.
	if m.pinned[ctx] {
		return &Cluster{context: ctx}, false
	}
	file := m.fileFor(ctx)
	store := k8s.NewStore(client.Dynamic, ctx, m.send)
	if m.send != nil && client.WarningHandler != nil {
		client.WarningHandler.SetSend(m.send)
	}
	c := &Cluster{
		context:   ctx,
		file:      file,
		client:    client,
		store:     store,
		discovery: k8s.NewDiscovery(),
	}
	m.clusters[ctx] = c
	return c, true
}

// SyncRefs reconciles every cluster's reference count against the supplied set
// of contexts that panes currently reference. After SyncRefs:
//
//   - each cluster's refCount equals the number of panes referencing its context
//     (one ref per pane, per the app-level invariant);
//   - any cluster whose pane count is zero is torn down (its informers stopped)
//     and removed from the map.
//
// This makes ref bookkeeping idempotent and order-independent: no matter how
// connects, focus changes and re-targets interleave, calling SyncRefs with the
// CURRENT pane contexts always converges to the correct counts. It replaces
// the fragile in-flight Acquire/Release pairing for the per-pane connect flow
// (which could imbalance under rapid re-targets). Must run on the Update
// goroutine. Callers must pass explicit context names; empty entries are
// ignored.
func (m *Manager) SyncRefs(paneContexts []string) {
	want := make(map[string]int, len(paneContexts))
	for _, ctx := range paneContexts {
		if ctx == "" {
			continue
		}
		want[ctx]++
	}
	for ctx, c := range m.clusters {
		// Pinned pseudo-contexts (e.g. "manifests") are preserved for the whole
		// session: never torn down and never UnsubscribeAll'd, regardless of how
		// many panes reference them.
		if m.pinned[ctx] {
			continue
		}
		n := want[ctx]
		c.refCount = n
		if n <= 0 {
			if c.store != nil {
				c.store.UnsubscribeAll()
			}
			delete(m.clusters, ctx)
		}
	}
}

// Entries returns the scanned kubeconfig context entries. The returned slice is
// the Manager's own backing slice, not a copy, so callers must not mutate it.
func (m *Manager) Entries() []ContextEntry { return m.entries }

// ForEach calls fn for every currently-created cluster. Useful for teardown
// and heartbeat sweeps. Iteration order is unspecified.
func (m *Manager) ForEach(fn func(*Cluster)) {
	for _, c := range m.clusters {
		fn(c)
	}
}
