// Package cluster provides a session manager that owns one Cluster per
// kube-context. Each Cluster bundles its own Client, Store, and Discovery so
// resources from different clusters never collide. Clusters are created lazily
// on first use and reference-counted; the global context is pinned and never
// torn down by ref-count.
package cluster

import (
	"github.com/aohoyd/aku/internal/k8s"
)

// Cluster bundles everything needed to talk to a single kube-context: the
// Kubernetes Client, the informer Store, and the API Discovery index.
//
// Fields are unexported and exposed through accessor methods (Client, Store,
// Discovery, Context, Err, Connected). This lets *Cluster directly satisfy the
// plugin.Cluster interface (which requires Store() *k8s.Store and
// Discovery() *k8s.Discovery METHODS) without a field vs. method name clash.
//
// A Cluster may be "degraded": when connecting to the context failed, client,
// store, and discovery are nil and err holds the connection error. Such a
// Cluster is still cached and returned so callers can render the error state.
type Cluster struct {
	context   string
	file      string
	client    *k8s.Client
	store     *k8s.Store
	discovery *k8s.Discovery
	err       error

	// refCount tracks how many panes currently reference this cluster. It is
	// reconciled by Manager.SyncRefs against the set of panes currently pinned to
	// this context (one ref per pinned pane). When it reaches zero the Manager
	// tears the cluster down, unless the cluster is the global one (which is
	// pinned and never torn down).
	refCount int
}

// New builds a Cluster from already-constructed parts. It is primarily a seam
// for callers (and tests) that have a client/store/discovery in hand and want a
// ready Cluster without going through the Manager's lazy connect path. A nil
// client yields a degraded-looking Cluster (Connected() == false) unless err is
// also nil and store/discovery are supplied — callers decide the semantics.
func New(context, file string, client *k8s.Client, store *k8s.Store, discovery *k8s.Discovery, err error) *Cluster {
	return &Cluster{
		context:   context,
		file:      file,
		client:    client,
		store:     store,
		discovery: discovery,
		err:       err,
	}
}

// The accessor methods below are nil-receiver-safe: a nil *Cluster (e.g. the
// result of Manager.Global() before any global cluster is created, as in tests)
// behaves like a fully degraded cluster — empty context, nil client/store/
// discovery, not connected. This matters because a typed-nil *Cluster wrapped in
// the plugin.Cluster interface is NOT interface-nil, so plugin.StoreOf would
// otherwise call Store() on a nil pointer and panic.

// Context returns the kube-context name this cluster belongs to.
func (c *Cluster) Context() string {
	if c == nil {
		return ""
	}
	return c.context
}

// File returns the kubeconfig file this cluster was loaded from. It may be
// empty, meaning the Manager's default kubeconfig path was used.
func (c *Cluster) File() string {
	if c == nil {
		return ""
	}
	return c.file
}

// Client returns the Kubernetes client, or nil if the cluster is degraded.
func (c *Cluster) Client() *k8s.Client {
	if c == nil {
		return nil
	}
	return c.client
}

// Store returns the informer store, or nil if the cluster is degraded.
func (c *Cluster) Store() *k8s.Store {
	if c == nil {
		return nil
	}
	return c.store
}

// Discovery returns the API discovery index, or nil if the cluster is degraded.
func (c *Cluster) Discovery() *k8s.Discovery {
	if c == nil {
		return nil
	}
	return c.discovery
}

// Err returns the connection error, or nil if the cluster connected.
func (c *Cluster) Err() error {
	if c == nil {
		return nil
	}
	return c.err
}

// Connected reports whether the cluster has a live client and no error.
func (c *Cluster) Connected() bool { return c != nil && c.client != nil && c.err == nil }

// RefCount returns the current reference count (the number of panes the Manager
// believes are pinned to this cluster). It is primarily an observability/test
// seam for asserting the per-pane refcount invariant; production code reconciles
// it via Manager.SyncRefs rather than reading this directly. Nil-receiver-safe.
func (c *Cluster) RefCount() int {
	if c == nil {
		return 0
	}
	return c.refCount
}
