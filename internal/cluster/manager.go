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
// RegisterConnected). Reference counts are reconciled against the set of panes
// currently pinned to each context via SyncRefs; a non-global cluster whose
// pinned-pane count reaches zero is torn down.
//
// Thread-safety: all methods are intended to be called from the single Bubble
// Tea Update goroutine, like the rest of the app state. No internal locking is
// used; if the Manager is ever shared across goroutines the caller must
// serialize access. Keeping it lock-free avoids any chance of deadlock between
// methods that call one another (e.g. SetGlobal -> GetOrCreate). The sole
// exception is Dial, which is map-free and safe to run off-thread.
type Manager struct {
	clusters   map[string]*Cluster
	global     string
	entries    []ContextEntry
	kubeconfig string
	send       func(tea.Msg)
	apiTimeout time.Duration

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
	}
	m.connect = func(file, ctx string) (*k8s.Client, error) {
		if file == "" {
			file = m.kubeconfig
		}
		return k8s.NewClient(file, ctx, "")
	}
	return m
}

// Register installs a pre-built Cluster under its context name and (when
// global is true) makes it the global context. It is a seam for callers and
// tests that construct a Cluster directly (via cluster.New) and want the
// Manager to resolve it through Get/Global. An existing cluster for the same
// context is replaced.
func (m *Manager) Register(c *Cluster, global bool) {
	m.clusters[c.context] = c
	if global {
		m.global = c.context
	}
}

// SetConnect overrides the function used to build a Client for a (file, ctx)
// pair. It is the injection seam described in the design: tests in other
// packages (e.g. internal/app) call it before SetGlobal/GetOrCreate to wire a
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
// use. An empty ctx resolves to the global context. A cached cluster is
// returned as-is. This is a lookup-or-create: it does NOT change the reference
// count.
//
// On connect failure a degraded Cluster (Connected()==false, Err set) is
// returned together with the error but is NOT cached, so a subsequent call
// re-dials. This keeps a transient connect failure from permanently blocking a
// global-context switch to that context (the global-switch path calls
// GetOrCreate). On success the Store and Discovery are built and the send
// function (if set) is wired.
func (m *Manager) GetOrCreate(ctx string) (*Cluster, error) {
	if ctx == "" {
		ctx = m.global
	}

	if c, ok := m.clusters[ctx]; ok {
		return c, c.err
	}

	file := m.fileFor(ctx)
	client, err := m.connect(file, ctx)
	if err != nil {
		// Return a degraded (unstored) cluster so the caller can inspect Err();
		// the next GetOrCreate retries the dial.
		return &Cluster{context: ctx, file: file, err: err}, err
	}

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
	return c, nil
}

// Get returns the cached Cluster for ctx without creating it and without
// touching the reference count. An empty ctx resolves to the global context.
func (m *Manager) Get(ctx string) (*Cluster, bool) {
	if ctx == "" {
		ctx = m.global
	}
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
// it. An empty ctx resolves to the global context.
//
// Keeping Dial map-free preserves the Manager's no-lock, single-goroutine
// invariant: state mutation stays on the Update goroutine; only the dial runs
// off-thread.
func (m *Manager) Dial(ctx string) (*k8s.Client, error) {
	if ctx == "" {
		ctx = m.global
	}
	return m.connect(m.fileFor(ctx), ctx)
}

// RegisterConnected installs a Cluster for ctx built from an already-dialed
// client (typically produced off-thread by Dial). It must run on the Update
// goroutine. If a cluster for ctx already exists it is returned as-is (the
// freshly-dialed client is discarded), so a redundant connect cannot replace a
// live cluster out from under panes already using it. This does NOT change the
// reference count — refcounts are reconciled separately via SyncRefs. An empty
// ctx resolves to the global context.
//
// The bool reports whether a NEW cluster was installed (true) or an existing one
// was returned from cache (false). Callers use it to avoid starting a duplicate
// heartbeat/discovery loop when a second pane connects to an already-live
// context (handleClusterHealth re-arms the heartbeat per tick, so a second loop
// would compound the probe rate).
func (m *Manager) RegisterConnected(ctx string, client *k8s.Client) (*Cluster, bool) {
	if ctx == "" {
		ctx = m.global
	}
	if c, ok := m.clusters[ctx]; ok {
		return c, false
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
// of contexts that panes are currently pinned to. After SyncRefs:
//
//   - each cluster's refCount equals the number of panes pinned to its context
//     (one ref per pinned pane, per the app-level invariant);
//   - any non-global cluster whose pinned-pane count is zero is torn down (its
//     informers stopped) and removed from the map;
//   - the global cluster is never torn down regardless of its count.
//
// This makes ref bookkeeping idempotent and order-independent: no matter how
// connects, focus changes and re-pins interleave, calling SyncRefs with the
// CURRENT pinned contexts always converges to the correct counts. It replaces
// the fragile in-flight Acquire/Release pairing for the per-pane connect flow
// (which could imbalance under rapid re-pins). Must run on the Update goroutine.
func (m *Manager) SyncRefs(pinnedContexts []string) {
	want := make(map[string]int, len(pinnedContexts))
	for _, ctx := range pinnedContexts {
		if ctx == "" {
			ctx = m.global
		}
		want[ctx]++
	}
	for ctx, c := range m.clusters {
		n := want[ctx]
		c.refCount = n
		if n <= 0 && ctx != m.global {
			if c.store != nil {
				c.store.UnsubscribeAll()
			}
			delete(m.clusters, ctx)
		}
	}
}

// SetGlobal makes ctx the global context (creating its cluster if needed) and
// returns it. The previously global cluster, if any, is no longer pinned and
// becomes eligible for teardown once its reference count reaches zero.
func (m *Manager) SetGlobal(ctx string) (*Cluster, error) {
	c, err := m.GetOrCreate(ctx)
	m.PromoteGlobal(ctx, c)
	return c, err
}

// PromoteGlobal points the global context at an already-resolved cluster
// without dialing. It performs no I/O and cannot fail, so callers that already
// hold a connected cluster (e.g. from a prior GetOrCreate) can promote it with
// no dead error check. The cluster is also recorded in the cache under ctx so
// the global pointer and the cache never disagree. A nil cluster still moves the
// global context name (matching SetGlobal's behavior for a degraded connect).
func (m *Manager) PromoteGlobal(ctx string, c *Cluster) {
	m.global = ctx
	if c != nil {
		m.clusters[ctx] = c
	}
}

// Global returns the global Cluster, or nil if it has not been created yet.
func (m *Manager) Global() *Cluster {
	return m.clusters[m.global]
}

// GlobalContext returns the name of the global context.
func (m *Manager) GlobalContext() string { return m.global }

// Entries returns the known context entries.
func (m *Manager) Entries() []ContextEntry { return m.entries }

// ForEach calls fn for every currently-created cluster. Useful for teardown
// and heartbeat sweeps. Iteration order is unspecified.
func (m *Manager) ForEach(fn func(*Cluster)) {
	for _, c := range m.clusters {
		fn(c)
	}
}
