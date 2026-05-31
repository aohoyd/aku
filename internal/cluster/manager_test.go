package cluster

import (
	"errors"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
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

// TestSyncRefsTearsDownNonGlobalAtZero exercises the SyncRefs reconciliation
// that replaced the old Acquire/Release contract: a non-global cluster whose
// pinned-pane count drops to zero is torn down and removed; while it has at
// least one pinned pane it survives, with refCount equal to the pinned count.
func TestSyncRefsTearsDownNonGlobalAtZero(t *testing.T) {
	m, _ := newTestManager(t, nil)
	if _, err := m.SetGlobal("home"); err != nil {
		t.Fatalf("SetGlobal(home) err = %v", err)
	}

	// Install a non-global cluster and pin two panes to it.
	if _, err := m.GetOrCreate("alpha"); err != nil {
		t.Fatalf("GetOrCreate(alpha) err = %v", err)
	}
	m.SyncRefs([]string{"alpha", "alpha"})
	c, ok := m.Get("alpha")
	if !ok {
		t.Fatal("alpha torn down with 2 pinned panes")
	}
	if c.RefCount() != 2 {
		t.Errorf("alpha refCount = %d, want 2", c.RefCount())
	}

	// Drop to one pinned pane: still present, count 1.
	m.SyncRefs([]string{"alpha"})
	c, ok = m.Get("alpha")
	if !ok {
		t.Fatal("alpha torn down with 1 pinned pane")
	}
	if c.RefCount() != 1 {
		t.Errorf("alpha refCount = %d, want 1", c.RefCount())
	}

	// Zero pinned panes: non-global cluster is torn down and removed.
	m.SyncRefs(nil)
	if _, ok := m.Get("alpha"); ok {
		t.Error("non-global alpha still present after pinned count reached 0")
	}
}

// TestSyncRefsNeverTearsDownGlobal proves the global context is pinned: SyncRefs
// with no pinned panes leaves it intact regardless of its refCount.
func TestSyncRefsNeverTearsDownGlobal(t *testing.T) {
	m, _ := newTestManager(t, nil)
	if _, err := m.SetGlobal("home"); err != nil {
		t.Fatalf("SetGlobal(home) err = %v", err)
	}
	m.SyncRefs(nil)
	if _, ok := m.Get("home"); !ok {
		t.Errorf("global cluster torn down by SyncRefs, want pinned")
	}
	if m.Global() == nil {
		t.Errorf("Global() = nil after SyncRefs(nil), want pinned cluster")
	}
	// An empty pinned context resolves to the global one, so it never counts as a
	// separate teardown candidate.
	m.SyncRefs([]string{""})
	if _, ok := m.Get("home"); !ok {
		t.Errorf("global torn down after SyncRefs with empty ctx, want pinned")
	}
}

func TestSyncRefsHandlesNilStore(t *testing.T) {
	// A degraded cluster (registered directly) has a nil store; SyncRefs teardown
	// must not panic on it.
	m, _ := newTestManager(t, nil)
	if _, err := m.SetGlobal("home"); err != nil {
		t.Fatalf("SetGlobal(home) err = %v", err)
	}
	m.Register(New("bad", "", nil, nil, nil, errors.New("boom")), false)
	if _, ok := m.Get("bad"); !ok {
		t.Fatal("precondition: degraded cluster should be registered")
	}
	// No pinned pane references "bad": teardown removes it without panicking on
	// the nil store.
	m.SyncRefs(nil)
	if _, ok := m.Get("bad"); ok {
		t.Error("degraded non-global cluster still present after teardown")
	}
}

// TestConnectErrorDoesNotCacheDegraded proves GetOrCreate does NOT cache a
// connect-failed cluster: the entry is absent afterward and a subsequent call
// re-dials, so a transient failure cannot permanently block a global switch.
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

func TestSetGlobalSwitchesGlobalPointer(t *testing.T) {
	m, _ := newTestManager(t, nil)

	first, err := m.SetGlobal("one")
	if err != nil {
		t.Fatalf("SetGlobal(one) err = %v", err)
	}
	if m.GlobalContext() != "one" {
		t.Errorf("GlobalContext() = %q, want %q", m.GlobalContext(), "one")
	}
	if m.Global() != first {
		t.Errorf("Global() != cluster returned by SetGlobal(one)")
	}

	second, err := m.SetGlobal("two")
	if err != nil {
		t.Fatalf("SetGlobal(two) err = %v", err)
	}
	if m.GlobalContext() != "two" {
		t.Errorf("GlobalContext() = %q, want %q", m.GlobalContext(), "two")
	}
	if m.Global() != second {
		t.Errorf("Global() did not switch to the new global cluster")
	}
	if first == second {
		t.Errorf("SetGlobal(two) returned the same cluster as SetGlobal(one)")
	}
}

func TestEmptyContextResolvesToGlobal(t *testing.T) {
	m, calls := newTestManager(t, nil)

	g, err := m.SetGlobal("g")
	if err != nil {
		t.Fatalf("SetGlobal(g) err = %v", err)
	}

	// "" should resolve to the global context and return the same cluster
	// without a second connect.
	got, err := m.GetOrCreate("")
	if err != nil {
		t.Fatalf(`GetOrCreate("") err = %v`, err)
	}
	if got != g {
		t.Errorf(`GetOrCreate("") returned different cluster than global`)
	}
	if calls["g"] != 1 {
		t.Errorf("connect called %d times for global, want 1", calls["g"])
	}
}

// TestDialDoesNotMutateManagerState proves Dial only performs the off-thread
// dial: it installs no cluster entry and changes no refCount, preserving the
// Manager's single-goroutine invariant for state mutation.
func TestDialDoesNotMutateManagerState(t *testing.T) {
	m, calls := newTestManager(t, nil)
	if _, err := m.SetGlobal("home"); err != nil {
		t.Fatalf("SetGlobal(home) err = %v", err)
	}

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

func TestDialResolvesEmptyToGlobal(t *testing.T) {
	m, calls := newTestManager(t, nil)
	if _, err := m.SetGlobal("home"); err != nil {
		t.Fatalf("SetGlobal(home) err = %v", err)
	}
	client, err := m.Dial("")
	if err != nil {
		t.Fatalf(`Dial("") err = %v`, err)
	}
	if client.Context != "home" {
		t.Errorf(`Dial("") dialed ctx %q, want global "home"`, client.Context)
	}
	// SetGlobal already connected "home" once; Dial connects it again (it never
	// reads the cache).
	if calls["home"] != 2 {
		t.Errorf("connect for home called %d times, want 2", calls["home"])
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
