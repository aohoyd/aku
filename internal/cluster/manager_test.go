package cluster

import (
	"errors"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/aohoyd/aku/internal/k8s"
)

// newTestManager returns a Manager whose connect func is stubbed to avoid
// touching real clusters. The returned counter map records how many times
// connect was called per context. The stub returns a Client with a nil Dynamic
// interface, which is safe: k8s.NewStore only stores the client and never
// dereferences it until Subscribe, which the Manager never calls.
func newTestManager(t *testing.T, fail map[string]error) (*Manager, map[string]int) {
	t.Helper()
	m := NewManager(nil, "", time.Second)
	calls := make(map[string]int)
	m.connect = func(_ /*file*/ string, ctx string) (*k8s.Client, error) {
		calls[ctx]++
		if err := fail[ctx]; err != nil {
			return nil, err
		}
		return &k8s.Client{Context: ctx}, nil
	}
	return m, calls
}

func TestGetOrCreateLazyAndCached(t *testing.T) {
	m, calls := newTestManager(t, nil)

	c1, err := m.GetOrCreate("alpha")
	if err != nil {
		t.Fatalf("GetOrCreate(alpha) err = %v, want nil", err)
	}
	if c1 == nil {
		t.Fatal("GetOrCreate(alpha) returned nil cluster")
	}
	if !c1.Connected() {
		t.Errorf("Connected() = false, want true")
	}
	if c1.Store() == nil {
		t.Errorf("Store() = nil, want non-nil")
	}
	if c1.Discovery() == nil {
		t.Errorf("Discovery() = nil, want non-nil")
	}
	if c1.Context() != "alpha" {
		t.Errorf("Context() = %q, want %q", c1.Context(), "alpha")
	}

	c2, err := m.GetOrCreate("alpha")
	if err != nil {
		t.Fatalf("second GetOrCreate(alpha) err = %v, want nil", err)
	}
	if c1 != c2 {
		t.Errorf("second GetOrCreate returned a different pointer; want cached")
	}
	if calls["alpha"] != 1 {
		t.Errorf("connect called %d times, want 1", calls["alpha"])
	}
}

func TestGetDoesNotCreate(t *testing.T) {
	m, calls := newTestManager(t, nil)

	if _, ok := m.Get("alpha"); ok {
		t.Errorf("Get(alpha) ok = true before create, want false")
	}
	if calls["alpha"] != 0 {
		t.Errorf("connect called %d times from Get, want 0", calls["alpha"])
	}

	created, _ := m.GetOrCreate("alpha")
	got, ok := m.Get("alpha")
	if !ok {
		t.Fatalf("Get(alpha) ok = false after create, want true")
	}
	if got != created {
		t.Errorf("Get returned different pointer than GetOrCreate")
	}
}

// TestSyncRefsTearsDownAtZero exercises the SyncRefs reconciliation that
// replaced the old Acquire/Release contract: a cluster whose pane count drops to
// zero is torn down and removed; while it has at least one referencing pane it
// survives, with refCount equal to the pane count.
func TestSyncRefsTearsDownAtZero(t *testing.T) {
	m, _ := newTestManager(t, nil)

	// Install a cluster and reference it from two panes.
	if _, err := m.GetOrCreate("alpha"); err != nil {
		t.Fatalf("GetOrCreate(alpha) err = %v", err)
	}
	m.SyncRefs(map[string]int{"alpha": 2})
	c, ok := m.Get("alpha")
	if !ok {
		t.Fatal("alpha torn down with 2 referencing panes")
	}
	if c.RefCount() != 2 {
		t.Errorf("alpha refCount = %d, want 2", c.RefCount())
	}

	// Drop to one referencing pane: still present, count 1.
	m.SyncRefs(map[string]int{"alpha": 1})
	c, ok = m.Get("alpha")
	if !ok {
		t.Fatal("alpha torn down with 1 referencing pane")
	}
	if c.RefCount() != 1 {
		t.Errorf("alpha refCount = %d, want 1", c.RefCount())
	}

	// Zero referencing panes: the cluster is torn down and removed.
	m.SyncRefs(nil)
	if _, ok := m.Get("alpha"); ok {
		t.Error("alpha still present after pane count reached 0")
	}
}

// TestSyncRefsTearsDownStartupClusterAtZero proves the new SyncRefs has no
// global/startup exemption: the cluster registered at startup is torn down like
// any other once no pane references it. The Manager is constructed with a
// startup context, the cluster registered under it, and then SyncRefs is called
// with a pane set that does not include the startup context.
func TestSyncRefsTearsDownStartupClusterAtZero(t *testing.T) {
	m := NewManager(nil, "startup", time.Second)
	m.Register(New("startup", "", &k8s.Client{Context: "startup"}, nil, k8s.NewDiscovery(), nil))
	if _, ok := m.Get("startup"); !ok {
		t.Fatal("precondition: startup cluster should be registered")
	}

	// Panes have all moved to another context; nothing references startup.
	m.SyncRefs(map[string]int{"other": 1})
	if _, ok := m.Get("startup"); ok {
		t.Error("startup cluster still present after no pane references it (no global exemption)")
	}
}

// TestSyncRefsKeepsReferencedAndIgnoresEmpty proves a referenced context
// survives and empty entries in the pane set are ignored (they no longer resolve
// to any default/global context).
func TestSyncRefsKeepsReferencedAndIgnoresEmpty(t *testing.T) {
	m, _ := newTestManager(t, nil)
	if _, err := m.GetOrCreate("home"); err != nil {
		t.Fatalf("GetOrCreate(home) err = %v", err)
	}

	// "home" referenced, plus a stray empty entry that must be ignored.
	m.SyncRefs(map[string]int{"home": 1, "": 1})
	c, ok := m.Get("home")
	if !ok {
		t.Fatalf("home torn down despite a referencing pane")
	}
	if c.RefCount() != 1 {
		t.Errorf("home refCount = %d, want 1 (empty entry must not count)", c.RefCount())
	}

	// Only empty entries: home is no longer referenced and is torn down.
	m.SyncRefs(map[string]int{"": 1})
	if _, ok := m.Get("home"); ok {
		t.Error("home still present when only empty entries reference it")
	}
}

func TestSyncRefsHandlesNilStore(t *testing.T) {
	// A degraded cluster (registered directly) has a nil store; SyncRefs teardown
	// must not panic on it.
	m, _ := newTestManager(t, nil)
	m.Register(New("bad", "", nil, nil, nil, errors.New("boom")))
	if _, ok := m.Get("bad"); !ok {
		t.Fatal("precondition: degraded cluster should be registered")
	}
	// No pane references "bad": teardown removes it without panicking on the nil
	// store.
	m.SyncRefs(nil)
	if _, ok := m.Get("bad"); ok {
		t.Error("degraded cluster still present after teardown")
	}
}

// TestConnectErrorDoesNotCacheDegraded proves GetOrCreate does NOT cache a
// connect-failed cluster: the entry is absent afterward and a subsequent call
// re-dials, so a transient failure cannot permanently block a context switch.
func TestConnectErrorDoesNotCacheDegraded(t *testing.T) {
	wantErr := errors.New("dial tcp: refused")
	m, calls := newTestManager(t, map[string]error{"down": wantErr})

	c, err := m.GetOrCreate("down")
	if !errors.Is(err, wantErr) {
		t.Fatalf("GetOrCreate(down) err = %v, want %v", err, wantErr)
	}
	if c == nil {
		t.Fatal("degraded cluster is nil, want returned")
	}
	if c.Connected() {
		t.Errorf("Connected() = true, want false for degraded cluster")
	}
	if !errors.Is(c.Err(), wantErr) {
		t.Errorf("Err() = %v, want %v", c.Err(), wantErr)
	}
	if c.Context() != "down" {
		t.Errorf("Context() = %q, want %q", c.Context(), "down")
	}

	// It must NOT be cached: Get returns nothing, so a retry is possible.
	if _, ok := m.Get("down"); ok {
		t.Error("degraded cluster was cached; retry would be permanently blocked")
	}

	// A second call re-dials (connect invoked again) instead of returning a
	// stale cached degraded entry.
	c2, err2 := m.GetOrCreate("down")
	if !errors.Is(err2, wantErr) {
		t.Errorf("second GetOrCreate err = %v, want %v", err2, wantErr)
	}
	if c2 == c {
		t.Error("second GetOrCreate returned the same (cached) degraded cluster")
	}
	if calls["down"] != 2 {
		t.Errorf("connect called %d times for degraded ctx, want 2 (retry)", calls["down"])
	}
}

// TestGetOrCreateResolvesEmptyToCurrentContext proves the startup seam:
// GetOrCreate("") dials with an empty context override (so k8s.NewClient picks
// the kubeconfig current-context), and the resulting cluster is cached under the
// resolved context name the client reports — letting the App seed panes with an
// explicit context.
func TestGetOrCreateResolvesEmptyToCurrentContext(t *testing.T) {
	m := NewManager(nil, "", time.Second)
	calls := make(map[string]int)
	m.connect = func(_ /*file*/ string, ctx string) (*k8s.Client, error) {
		calls[ctx]++
		// Simulate k8s.NewClient resolving "" to the kubeconfig current-context.
		resolved := ctx
		if resolved == "" {
			resolved = "current"
		}
		return &k8s.Client{Context: resolved}, nil
	}

	c, err := m.GetOrCreate("")
	if err != nil {
		t.Fatalf(`GetOrCreate("") err = %v`, err)
	}
	if c.Context() != "current" {
		t.Errorf("GetOrCreate(\"\").Context() = %q, want %q (resolved current-context)", c.Context(), "current")
	}

	// The cluster is cached under the RESOLVED name so an explicit lookup works.
	got, ok := m.Get("current")
	if !ok || got != c {
		t.Errorf("cluster not cached under resolved name 'current'")
	}
	// It must NOT be cached under the empty key.
	if _, ok := m.Get(""); ok {
		t.Errorf("cluster cached under empty key, want resolved name only")
	}
}

// TestDialDoesNotMutateManagerState proves Dial only performs the off-thread
// dial: it installs no cluster entry and changes no refCount, preserving the
// Manager's single-goroutine invariant for state mutation.
func TestDialDoesNotMutateManagerState(t *testing.T) {
	m, calls := newTestManager(t, nil)

	client, err := m.Dial("alpha")
	if err != nil {
		t.Fatalf("Dial(alpha) err = %v", err)
	}
	if client == nil {
		t.Fatal("Dial(alpha) returned nil client")
	}
	if calls["alpha"] != 1 {
		t.Errorf("Dial called connect %d times, want 1", calls["alpha"])
	}
	// Dial must NOT have installed a cluster entry for alpha.
	if _, ok := m.Get("alpha"); ok {
		t.Error("Dial installed a cluster entry; it must not mutate manager state")
	}
}

// TestDialUsesExplicitContext proves Dial dials exactly the context it is given
// (no empty-to-default resolution) and never reads or writes the cache.
func TestDialUsesExplicitContext(t *testing.T) {
	m, calls := newTestManager(t, nil)

	client, err := m.Dial("home")
	if err != nil {
		t.Fatalf(`Dial("home") err = %v`, err)
	}
	if client.Context != "home" {
		t.Errorf(`Dial("home") dialed ctx %q, want "home"`, client.Context)
	}
	if calls["home"] != 1 {
		t.Errorf("connect for home called %d times, want 1", calls["home"])
	}
	if _, ok := m.Get("home"); ok {
		t.Error("Dial installed a cluster entry; it must not mutate manager state")
	}
}

// TestDialEmptyContext documents Dial("") after the removal of the Manager's
// "global" notion: Dial passes the empty override straight through to connect
// (where k8s.NewClient would resolve the kubeconfig current-context) and, like
// any Dial, installs no cluster entry. The empty override is NOT resolved by the
// Manager itself — resolution happens in connect / k8s.NewClient.
func TestDialEmptyContext(t *testing.T) {
	m, calls := newTestManager(t, nil)

	client, err := m.Dial("")
	if err != nil {
		t.Fatalf(`Dial("") err = %v`, err)
	}
	if client == nil {
		t.Fatal(`Dial("") returned nil client`)
	}
	if calls[""] != 1 {
		t.Errorf(`connect for "" called %d times, want 1`, calls[""])
	}
	// Dial never caches, so no entry under the empty key (or any key).
	if _, ok := m.Get(""); ok {
		t.Error(`Dial("") installed a cluster entry; it must not mutate manager state`)
	}
}

// TestRegisterConnectedInstallsAndReportsNewly proves RegisterConnected installs
// a new cluster (reporting newlyConnected=true) and, on a second connect to the
// same live context, returns the existing cluster reporting false — the signal
// callers use to skip a duplicate heartbeat loop.
func TestRegisterConnectedInstallsAndReportsNewly(t *testing.T) {
	m, _ := newTestManager(t, nil)
	client := &k8s.Client{Context: "beta"}

	c, newly := m.RegisterConnected("beta", client)
	if c == nil {
		t.Fatal("RegisterConnected returned nil cluster")
	}
	if !newly {
		t.Error("first RegisterConnected should report newlyConnected=true")
	}
	if got, ok := m.Get("beta"); !ok || got != c {
		t.Error("RegisterConnected did not install the cluster")
	}

	// A second connect to the SAME live context returns the existing cluster and
	// reports newlyConnected=false so callers do not start a duplicate heartbeat.
	c2, newly2 := m.RegisterConnected("beta", &k8s.Client{Context: "beta"})
	if newly2 {
		t.Error("second RegisterConnected should report newlyConnected=false")
	}
	if c2 != c {
		t.Error("second RegisterConnected returned a different cluster")
	}
}

// TestSetSendAppliesToExistingStores verifies SetSend reaches stores created
// BOTH before the call (it walks the existing cluster map and re-wires each
// store in place) and after it. Beyond pointer identity, it asserts the wiring
// is FUNCTIONAL: after SetSend the pre-existing store, when it notifies, invokes
// the supplied send func.
func TestSetSendAppliesToExistingStores(t *testing.T) {
	m, _ := newTestManager(t, nil)

	// Create a cluster BEFORE SetSend so its store exists when SetSend runs. The
	// stub client has a nil Dynamic; a real store is still constructed.
	pre, err := m.GetOrCreate("alpha")
	if err != nil {
		t.Fatalf("GetOrCreate(alpha) err = %v", err)
	}
	preStore := pre.Store()
	if preStore == nil {
		t.Fatal("pre-existing cluster has no store")
	}

	var got tea.Msg
	m.SetSend(func(msg tea.Msg) { got = msg })

	// SetSend must re-wire the SAME store, not swap it out.
	if pre.Store() != preStore {
		t.Errorf("SetSend replaced the pre-existing store instead of re-wiring it")
	}

	// Functional check: drive the store's notify path; the supplied func must be
	// the one invoked. NotifyForTest sends a ResourceUpdatedMsg via the store's
	// send func (a no-op when unset), so observing a non-nil msg proves SetSend
	// functionally wired the supplied func into the pre-existing store.
	preStore.NotifyForTest(schema.GroupVersionResource{Version: "v1", Resource: "pods"}, "default")
	if got == nil {
		t.Error("SetSend did not functionally wire the pre-existing store's send")
	}
	if upd, ok := got.(k8s.ResourceUpdatedMsg); !ok || upd.Context != "alpha" {
		t.Errorf("SetSend wired wrong func: got msg %#v, want ResourceUpdatedMsg for ctx alpha", got)
	}

	// A cluster created AFTER SetSend must also get a live store.
	post, err := m.GetOrCreate("beta")
	if err != nil {
		t.Fatalf("GetOrCreate(beta) err = %v", err)
	}
	if post.Store() == nil {
		t.Fatal("post-SetSend cluster has no store")
	}
}

func TestForEachVisitsAllClusters(t *testing.T) {
	m, _ := newTestManager(t, nil)
	_, _ = m.GetOrCreate("a")
	_, _ = m.GetOrCreate("b")
	_, _ = m.GetOrCreate("c")

	seen := map[string]bool{}
	m.ForEach(func(cl *Cluster) { seen[cl.Context()] = true })
	for _, ctx := range []string{"a", "b", "c"} {
		if !seen[ctx] {
			t.Errorf("ForEach did not visit %q", ctx)
		}
	}
}

// TestRegisterPinnedSurvivesSyncRefs proves the manifest pseudo-context
// lifecycle: a pinned cluster appears in Entries(), is never torn down by
// SyncRefs even at zero pane refs (and its store is never UnsubscribeAll'd), and
// is returned verbatim (same pointer, pre-populated store intact) on re-select.
func TestRegisterPinnedSurvivesSyncRefs(t *testing.T) {
	m, _ := newTestManager(t, nil)

	gvr := schema.GroupVersionResource{Version: "v1", Resource: "pods"}
	store := k8s.NewStore(nil, "manifests", nil)
	obj := &unstructured.Unstructured{}
	obj.SetName("manifest-pod")
	obj.SetNamespace("default")
	store.CacheUpsert(gvr, "default", obj)

	pinned := New("manifests", "", nil, store, k8s.NewDiscovery(), nil)
	m.RegisterPinned(pinned)

	// Appears in Entries() so the gx/oX/contexts views can list it.
	found := false
	for _, e := range m.Entries() {
		if e.Name == "manifests" {
			found = true
		}
	}
	if !found {
		t.Errorf("Entries() does not contain pinned context %q: %+v", "manifests", m.Entries())
	}

	// SyncRefs with a pane set that does NOT include "manifests" (zero refs).
	m.SyncRefs(map[string]int{"other": 1})

	got, ok := m.Get("manifests")
	if !ok {
		t.Fatal("pinned cluster torn down by SyncRefs with zero pane refs")
	}
	if got != pinned {
		t.Error("Get(manifests) returned a different pointer than the pinned cluster")
	}

	// The store must NOT have been cleared (UnsubscribeAll skipped); the
	// pre-populated object still lists.
	items := got.Store().List(gvr, "default")
	if len(items) != 1 || items[0].GetName() != "manifest-pod" {
		t.Errorf("pinned store was cleared by SyncRefs: List = %+v", items)
	}

	// A second SyncRefs cycle (still zero refs) keeps it returnable, same pointer.
	m.SyncRefs(nil)
	got2, ok2 := m.Get("manifests")
	if !ok2 || got2 != pinned {
		t.Error("Get(manifests) after second SyncRefs did not return the same pinned cluster")
	}
}

// TestRegisterPinnedDoesNotDuplicateEntry proves RegisterPinned does not add a
// duplicate Entries() row when the context is already present in the entries.
func TestRegisterPinnedDoesNotDuplicateEntry(t *testing.T) {
	m := NewManager([]ContextEntry{{Name: "manifests"}}, "", time.Second)
	m.RegisterPinned(New("manifests", "", nil, k8s.NewStore(nil, "manifests", nil), k8s.NewDiscovery(), nil))

	n := 0
	for _, e := range m.Entries() {
		if e.Name == "manifests" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("Entries() has %d 'manifests' rows, want 1 (no duplicate)", n)
	}
}

// TestSyncRefsStillTearsDownNonPinned is a regression guard: pinning must not
// exempt ordinary zero-ref clusters from teardown.
func TestSyncRefsStillTearsDownNonPinned(t *testing.T) {
	m, _ := newTestManager(t, nil)
	m.RegisterPinned(New("manifests", "", nil, k8s.NewStore(nil, "manifests", nil), k8s.NewDiscovery(), nil))

	// A normal connected cluster with zero referencing panes must still go away.
	if _, _ = m.RegisterConnected("normal", &k8s.Client{Context: "normal"}); false {
	}
	m.SyncRefs(map[string]int{"other": 1})
	if _, ok := m.Get("normal"); ok {
		t.Error("non-pinned zero-ref cluster survived SyncRefs (pinning leaked exemption)")
	}
	if _, ok := m.Get("manifests"); !ok {
		t.Error("pinned cluster torn down alongside the non-pinned one")
	}
}

func TestEntriesAndFileResolution(t *testing.T) {
	entries := []ContextEntry{{Name: "x", File: "/tmp/kubeconfig-x"}}
	m := NewManager(entries, "/default/kubeconfig", time.Second)

	var gotFile string
	m.connect = func(file, _ string) (*k8s.Client, error) {
		gotFile = file
		return &k8s.Client{}, nil
	}

	if _, err := m.GetOrCreate("x"); err != nil {
		t.Fatalf("GetOrCreate(x) err = %v", err)
	}
	if gotFile != "/tmp/kubeconfig-x" {
		t.Errorf("connect file = %q, want %q", gotFile, "/tmp/kubeconfig-x")
	}

	// A context not in entries resolves to "" (default handled by connect).
	if _, err := m.GetOrCreate("unknown"); err != nil {
		t.Fatalf("GetOrCreate(unknown) err = %v", err)
	}
	if gotFile != "" {
		t.Errorf("connect file for unknown ctx = %q, want empty", gotFile)
	}

	if len(m.Entries()) != 1 || m.Entries()[0].Name != "x" {
		t.Errorf("Entries() = %+v, want one entry named x", m.Entries())
	}
}
