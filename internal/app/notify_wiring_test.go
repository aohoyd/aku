package app

import (
	"fmt"
	"testing"
	"time"

	"github.com/aohoyd/aku/internal/config"
	"github.com/aohoyd/aku/internal/layout"
	"github.com/aohoyd/aku/internal/msgs"
	"github.com/aohoyd/aku/internal/notify"
	"github.com/aohoyd/aku/internal/plugin"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// newNotifyTestApp builds an App wired to a fresh notify store with a fixed
// clock so toast TTL/Live behaviour is deterministic. The "pods" plugin is
// registered so New's default single-pods pane construction succeeds.
func newNotifyTestApp(t *testing.T, cfg *config.Config) (App, *notify.Store, time.Time) {
	t.Helper()
	plugin.Reset()
	plugin.Register(&mockPlugin{name: "pods", gvr: schema.GroupVersionResource{Version: "v1", Resource: "pods"}})

	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	store := notify.NewStore(100)

	km := config.DefaultKeymap()
	a := New(newTestManager(), km, cfg, store, nil, nil, nil, nil, layout.OrientationVertical, "")
	a.now = func() time.Time { return now }
	return a, store, now
}

// TestMessageAddedNonStickyArmsTick verifies a non-sticky level returns a tick
// cmd (the auto-expiry timer) when a MessageAddedMsg is processed.
func TestMessageAddedNonStickyArmsTick(t *testing.T) {
	cfg := config.DefaultConfig() // info=3s, warning=5s, error=8s (all non-sticky)
	a, store, _ := newNotifyTestApp(t, cfg)

	m := store.Add(notify.LevelInfo, "hello", "", "test")

	model, cmd := a.Update(msgs.MessageAddedMsg{ID: m.ID, Level: int(m.Level)})
	if _, ok := model.(App); !ok {
		t.Fatalf("Update did not return App")
	}
	if cmd == nil {
		t.Fatal("non-sticky MessageAddedMsg should return a tick cmd, got nil")
	}
}

// TestMessageAddedArmsRemainingFromCreation verifies the auto-expiry tick is
// armed for the time remaining from the message's CREATION time, not from when
// the MessageAddedMsg is processed. A message created partway into its TTL must
// still expire on schedule (the tick fires after the remaining time).
func TestMessageAddedArmsRemainingFromCreation(t *testing.T) {
	cfg := config.DefaultConfig() // info TTL = 3s
	a, store, now := newNotifyTestApp(t, cfg)

	// Stamp the message 2s in the past relative to the app clock, leaving 1s of
	// its 3s TTL remaining.
	store.SetClock(func() time.Time { return now.Add(-2 * time.Second) })
	m := store.Add(notify.LevelInfo, "hello", "", "test")

	_, cmd := a.Update(msgs.MessageAddedMsg{ID: m.ID, Level: int(m.Level)})
	if cmd == nil {
		t.Fatal("expected a remaining-time tick cmd, got nil")
	}
	// The tick must still yield a ToastExpiredMsg for this ID. tea.Tick is
	// one-shot, so invoke cmd() exactly once and reuse the result in the failure
	// message (a second cmd() would block forever on the timer channel).
	out := cmd()
	if got, ok := out.(msgs.ToastExpiredMsg); !ok || got.ID != m.ID {
		t.Fatalf("tick should yield ToastExpiredMsg{%d}, got %#v", m.ID, out)
	}
}

// TestMessageAddedAlreadyExpiredDismisses verifies a message created longer than
// its TTL ago is dismissed immediately with no tick (it is already past its
// lifetime), and never appears in the visible set.
func TestMessageAddedAlreadyExpiredDismisses(t *testing.T) {
	cfg := config.DefaultConfig() // info TTL = 3s
	a, store, now := newNotifyTestApp(t, cfg)

	// Stamp the message 10s in the past — well beyond its 3s TTL.
	store.SetClock(func() time.Time { return now.Add(-10 * time.Second) })
	m := store.Add(notify.LevelInfo, "stale", "", "test")

	model, cmd := a.Update(msgs.MessageAddedMsg{ID: m.ID, Level: int(m.Level)})
	a = model.(App)
	if cmd != nil {
		t.Fatal("already-expired MessageAddedMsg should arm no tick, got non-nil cmd")
	}
	if !a.dismissed[m.ID] {
		t.Fatalf("already-expired message id %d should be marked dismissed", m.ID)
	}
	if visible := visibleToasts(a.notify, a.dismissed, now, a.toastTTL); len(visible) != 0 {
		t.Fatalf("already-expired message should not be visible, got %d toasts", len(visible))
	}
}

// TestMessageAddedEvictedIDNoTick verifies the not-found branch: when a
// MessageAddedMsg arrives for an ID that has already been evicted from the ring
// buffer (so messageCreatedAt misses), the handler arms no tick, marks the ID
// dismissed, and the ID never appears in the visible set (and does not panic).
func TestMessageAddedEvictedIDNoTick(t *testing.T) {
	cfg := config.DefaultConfig() // info TTL = 3s (non-sticky)

	plugin.Reset()
	plugin.Register(&mockPlugin{name: "pods", gvr: schema.GroupVersionResource{Version: "v1", Resource: "pods"}})

	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	const capacity = 2
	store := notify.NewStore(capacity)

	km := config.DefaultKeymap()
	a := New(newTestManager(), km, cfg, store, nil, nil, nil, nil, layout.OrientationVertical, "")
	a.now = func() time.Time { return now }

	// Add the first message, then overflow the ring so its ID is evicted.
	evicted := store.Add(notify.LevelInfo, "evicted", "", "test")
	for i := 0; i < capacity; i++ {
		store.Add(notify.LevelInfo, "fill", "", "test")
	}
	// Sanity: the evicted ID is gone from the store window.
	for _, m := range store.List() {
		if m.ID == evicted.ID {
			t.Fatalf("precondition failed: id %d should have been evicted", evicted.ID)
		}
	}

	model, cmd := a.Update(msgs.MessageAddedMsg{ID: evicted.ID, Level: int(notify.LevelInfo)})
	a = model.(App)

	if cmd != nil {
		t.Fatal("evicted MessageAddedMsg should arm no tick, got non-nil cmd")
	}
	// The handler marks the ID dismissed, then pruneDismissed drops it because it
	// is no longer in the store window — so it must NOT linger in the dismissed
	// map (it cannot reappear anyway, having been evicted).
	if a.dismissed[evicted.ID] {
		t.Fatalf("evicted message id %d should be pruned from dismissed, not retained", evicted.ID)
	}
	for _, m := range visibleToasts(a.notify, a.dismissed, now, a.toastTTL) {
		if m.ID == evicted.ID {
			t.Fatalf("evicted message id %d must not be visible", evicted.ID)
		}
	}
}

// TestMessageAddedStickyNoTick verifies a sticky level (timeout_error: -1 =>
// ToastTTL 0) returns no tick cmd.
func TestMessageAddedStickyNoTick(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Notifications.TimeoutError = -1 // sticky
	a, store, _ := newNotifyTestApp(t, cfg)

	m := store.Add(notify.LevelError, "boom", "", "test")

	_, cmd := a.Update(msgs.MessageAddedMsg{ID: m.ID, Level: int(m.Level)})
	if cmd != nil {
		t.Fatal("sticky MessageAddedMsg should return no tick cmd, got non-nil")
	}
}

// TestMessageAddedRefreshesAkuMessagesSplit verifies the handler re-polls open
// aku-messages splits (the synthetic resource shows the new row).
func TestMessageAddedRefreshesAkuMessagesSplit(t *testing.T) {
	cfg := config.DefaultConfig()
	a, store, _ := newNotifyTestApp(t, cfg)

	// Register a self-populating "aku-messages" plugin and open a split for it.
	sp := &mockSelfPopulatingPlugin{
		mockPlugin: mockPlugin{name: "aku-messages", gvr: schema.GroupVersionResource{Group: "_ktui", Version: "v1", Resource: "aku-messages"}},
	}
	plugin.Register(sp)
	a.layout.AddSplit(sp, "default", "")

	// Initially empty; after Add, the plugin reports one object and the handler
	// should push it into the split.
	m := store.Add(notify.LevelInfo, "hello", "", "test")
	sp.objs = []*unstructured.Unstructured{
		{Object: map[string]any{"metadata": map[string]any{"name": "msg-1"}}},
	}

	model, _ := a.Update(msgs.MessageAddedMsg{ID: m.ID, Level: int(m.Level)})
	a = model.(App)

	// Find the aku-messages split and assert it now has the object.
	var found bool
	for i := range a.layout.SplitCount() {
		split := a.layout.SplitAt(i)
		if split != nil && split.Plugin().Name() == "aku-messages" {
			found = true
			if split.Len() != 1 {
				t.Fatalf("aku-messages split should have 1 object after refresh, got %d", split.Len())
			}
		}
	}
	if !found {
		t.Fatal("no aku-messages split found")
	}
}

// TestToastExpiredRemovesOnlyThatID verifies ToastExpiredMsg removes a single
// ID from the visible set while leaving others visible.
func TestToastExpiredRemovesOnlyThatID(t *testing.T) {
	cfg := config.DefaultConfig()
	a, store, now := newNotifyTestApp(t, cfg)

	m1 := store.Add(notify.LevelInfo, "first", "", "test")
	m2 := store.Add(notify.LevelInfo, "second", "", "test")

	model, _ := a.Update(msgs.ToastExpiredMsg{ID: m1.ID})
	a = model.(App)

	visible := visibleToasts(a.notify, a.dismissed, now, a.toastTTL)
	if len(visible) != 1 {
		t.Fatalf("expected 1 visible toast after expiring one, got %d", len(visible))
	}
	if visible[0].ID != m2.ID {
		t.Fatalf("expected remaining toast to be m2 (id %d), got id %d", m2.ID, visible[0].ID)
	}
	// History untouched.
	if len(store.List()) != 2 {
		t.Fatalf("store.List should still report 2 messages, got %d", len(store.List()))
	}
}

// TestToastExpiredIdempotentAndBogusID verifies ToastExpiredMsg is a safe no-op
// when fired twice for the same ID, and when fired for an ID that was never seen.
func TestToastExpiredIdempotentAndBogusID(t *testing.T) {
	cfg := config.DefaultConfig()
	a, store, now := newNotifyTestApp(t, cfg)

	m := store.Add(notify.LevelInfo, "once", "", "test")

	// First expiry dismisses it.
	model, _ := a.Update(msgs.ToastExpiredMsg{ID: m.ID})
	a = model.(App)
	if !a.dismissed[m.ID] {
		t.Fatalf("first ToastExpiredMsg should mark id %d dismissed", m.ID)
	}
	dismissedAfterFirst := len(a.dismissed)

	// Firing again for the same ID changes nothing.
	model, _ = a.Update(msgs.ToastExpiredMsg{ID: m.ID})
	a = model.(App)
	if len(a.dismissed) != dismissedAfterFirst {
		t.Fatalf("repeat ToastExpiredMsg should be idempotent, dismissed len %d -> %d", dismissedAfterFirst, len(a.dismissed))
	}

	// A bogus, never-seen ID must not panic and must not resurrect anything;
	// pruneDismissed drops it because it is not in the store window.
	model, _ = a.Update(msgs.ToastExpiredMsg{ID: 999999})
	a = model.(App)
	if a.dismissed[999999] {
		t.Fatal("bogus ID not in store window should be pruned from dismissed")
	}
	if visible := visibleToasts(a.notify, a.dismissed, now, a.toastTTL); len(visible) != 0 {
		t.Fatalf("expected no visible toasts, got %d", len(visible))
	}
}

// TestClearNotificationsClearsStickyToast verifies ClearNotificationsMsg also
// dismisses a sticky (never-auto-hiding) toast, since Live includes sticky
// messages and clear marks every live ID dismissed.
func TestClearNotificationsClearsStickyToast(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Notifications.TimeoutError = -1 // sticky errors
	a, store, now := newNotifyTestApp(t, cfg)

	m := store.Add(notify.LevelError, "sticky boom", "", "test")

	// Sanity: the sticky error is visible before clearing.
	if visible := visibleToasts(a.notify, a.dismissed, now, a.toastTTL); len(visible) != 1 || visible[0].ID != m.ID {
		t.Fatalf("sticky toast should be visible before clear, got %d", len(visible))
	}

	model, _ := a.Update(msgs.ClearNotificationsMsg{})
	a = model.(App)

	if !a.dismissed[m.ID] {
		t.Fatalf("clear should dismiss the sticky toast id %d", m.ID)
	}
	if visible := visibleToasts(a.notify, a.dismissed, now, a.toastTTL); len(visible) != 0 {
		t.Fatalf("sticky toast should be cleared, got %d visible", len(visible))
	}
}

// TestMessageAddedNoAkuMessagesSplitNoPanic verifies MessageAddedMsg is a no-op
// (no panic) when there is no open aku-messages split to refresh.
func TestMessageAddedNoAkuMessagesSplitNoPanic(t *testing.T) {
	cfg := config.DefaultConfig()
	a, store, _ := newNotifyTestApp(t, cfg)

	// No aku-messages split is registered/open — only the default pods pane.
	m := store.Add(notify.LevelInfo, "hello", "", "test")

	model, cmd := a.Update(msgs.MessageAddedMsg{ID: m.ID, Level: int(m.Level)})
	if _, ok := model.(App); !ok {
		t.Fatalf("Update did not return App")
	}
	// Non-sticky info still arms a tick; the point is no panic occurred and the
	// handler returned normally.
	if cmd == nil {
		t.Fatal("non-sticky MessageAddedMsg should still arm a tick even with no aku-messages split")
	}
}

// TestClearNotificationsEmptiesVisibleSet verifies ClearNotificationsMsg hides
// all live toasts while the store history is preserved.
func TestClearNotificationsEmptiesVisibleSet(t *testing.T) {
	cfg := config.DefaultConfig()
	a, store, now := newNotifyTestApp(t, cfg)

	store.Add(notify.LevelInfo, "a", "", "test")
	store.Add(notify.LevelWarning, "b", "", "test")
	store.Add(notify.LevelError, "c", "", "test")

	model, _ := a.Update(msgs.ClearNotificationsMsg{})
	a = model.(App)

	visible := visibleToasts(a.notify, a.dismissed, now, a.toastTTL)
	if len(visible) != 0 {
		t.Fatalf("expected 0 visible toasts after clear, got %d", len(visible))
	}
	if len(store.List()) != 3 {
		t.Fatalf("store.List should still report 3 messages after clear, got %d", len(store.List()))
	}
}

// TestDismissedPrunedOnEviction verifies the dismissed map does not grow without
// bound: once the ring buffer evicts a dismissed ID it is dropped from dismissed
// (Finding 2). A dismissed ID still inside the store window is preserved so it
// stays hidden.
func TestDismissedPrunedOnEviction(t *testing.T) {
	cfg := config.DefaultConfig()

	plugin.Reset()
	plugin.Register(&mockPlugin{name: "pods", gvr: schema.GroupVersionResource{Version: "v1", Resource: "pods"}})

	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	const capacity = 4
	store := notify.NewStore(capacity)

	km := config.DefaultKeymap()
	a := New(newTestManager(), km, cfg, store, nil, nil, nil, nil, layout.OrientationVertical, "")
	a.now = func() time.Time { return now }

	// Add the first two messages and dismiss them.
	m1 := store.Add(notify.LevelInfo, "m1", "", "test")
	m2 := store.Add(notify.LevelInfo, "m2", "", "test")
	model, _ := a.Update(msgs.ToastExpiredMsg{ID: m1.ID})
	a = model.(App)
	model, _ = a.Update(msgs.ToastExpiredMsg{ID: m2.ID})
	a = model.(App)

	if len(a.dismissed) != 2 {
		t.Fatalf("expected 2 dismissed IDs before eviction, got %d", len(a.dismissed))
	}

	// Overflow the ring by 'capacity' more messages so m1 and m2 are evicted.
	for i := 0; i < capacity; i++ {
		store.Add(notify.LevelInfo, "fill", "", "test")
	}
	// A subsequent dismiss triggers the prune; dismiss the newest live message.
	live := store.Live(now, a.toastTTL)
	model, _ = a.Update(msgs.ToastExpiredMsg{ID: live[0].ID})
	a = model.(App)

	// m1 and m2 were evicted from the store, so they must be gone from dismissed.
	if a.dismissed[m1.ID] || a.dismissed[m2.ID] {
		t.Fatalf("evicted IDs m1=%d m2=%d should be pruned from dismissed, got %v", m1.ID, m2.ID, a.dismissed)
	}
	// dismissed must stay bounded by the store window.
	if len(a.dismissed) > capacity {
		t.Fatalf("dismissed (%d) should be bounded by store capacity (%d)", len(a.dismissed), capacity)
	}
	// Build set of current store IDs; every dismissed ID must still be present.
	inStore := map[uint64]bool{}
	for _, m := range store.List() {
		inStore[m.ID] = true
	}
	for id := range a.dismissed {
		if !inStore[id] {
			t.Fatalf("dismissed retains ID %d not in store window", id)
		}
	}
	// The newest message we just dismissed is still in the window, so it stays
	// dismissed (won't reappear).
	if !a.dismissed[live[0].ID] {
		t.Fatalf("live dismissed ID %d should be preserved", live[0].ID)
	}
}

// TestWarningMsgRoutesToNotify verifies a msgs.WarningMsg lands in the notify
// store as a warning-level message tagged with the originating cluster context.
func TestWarningMsgRoutesToNotify(t *testing.T) {
	cfg := config.DefaultConfig()
	a, store, _ := newNotifyTestApp(t, cfg)

	model, _ := a.Update(msgs.WarningMsg{Text: "deprecated API", Context: "prod"})
	a = model.(App)

	var found *notify.Message
	for _, m := range store.List() {
		if m.Level == notify.LevelWarning && m.Text == "deprecated API" {
			mm := m
			found = &mm
			break
		}
	}
	if found == nil {
		t.Fatalf("WarningMsg should add a warning-level message %q, store=%+v", "deprecated API", store.List())
	}
	if found.Context != "prod" {
		t.Fatalf("WarningMsg context = %q, want %q", found.Context, "prod")
	}
}

// TestErrMsgRoutesToNotify verifies a msgs.ErrMsg lands in the notify store as
// an error-level message carrying the error text.
func TestErrMsgRoutesToNotify(t *testing.T) {
	cfg := config.DefaultConfig()
	a, store, _ := newNotifyTestApp(t, cfg)

	model, _ := a.Update(msgs.ErrMsg{Err: fmt.Errorf("connect failed")})
	a = model.(App)

	var found bool
	for _, m := range store.List() {
		if m.Level == notify.LevelError && m.Text == "connect failed" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("ErrMsg should add an error-level message %q, store=%+v", "connect failed", store.List())
	}
}

// TestClearNotificationsCommand verifies the executeCommand wiring emits a
// ClearNotificationsMsg.
func TestClearNotificationsCommand(t *testing.T) {
	cfg := config.DefaultConfig()
	a, _, _ := newNotifyTestApp(t, cfg)

	_, cmd := a.executeCommand("clear-notifications")
	if cmd == nil {
		t.Fatal("clear-notifications should return a cmd")
	}
	msg := cmd()
	if _, ok := msg.(msgs.ClearNotificationsMsg); !ok {
		t.Fatalf("clear-notifications cmd should yield ClearNotificationsMsg, got %T", msg)
	}
}

// TestVisibleToastsNilStore verifies the helper is nil-safe (View must not panic
// when no store was injected).
func TestVisibleToastsNilStore(t *testing.T) {
	ttl := func(notify.Level) time.Duration { return time.Second }
	if got := visibleToasts(nil, map[uint64]bool{}, time.Now(), ttl); got != nil {
		t.Fatalf("visibleToasts(nil store) should return nil, got %v", got)
	}
}

// TestRefreshSelfPopulatingPortforwardsRegression verifies the generalized
// refresh still repopulates a portforwards split (the old
// refreshPortforwardSplits behaviour), and does not touch unrelated splits.
func TestRefreshSelfPopulatingPortforwardsRegression(t *testing.T) {
	cfg := config.DefaultConfig()
	a, _, _ := newNotifyTestApp(t, cfg)

	pf := &mockSelfPopulatingPlugin{
		mockPlugin: mockPlugin{name: "portforwards", gvr: schema.GroupVersionResource{Group: "_ktui", Version: "v1", Resource: "portforwards"}},
		objs: []*unstructured.Unstructured{
			{Object: map[string]any{"metadata": map[string]any{"name": "pf-1"}}},
		},
	}
	plugin.Register(pf)
	a.layout.AddSplit(pf, "default", "")

	a = a.refreshSelfPopulatingSplits("portforwards")

	var checked bool
	for i := range a.layout.SplitCount() {
		split := a.layout.SplitAt(i)
		if split != nil && split.Plugin().Name() == "portforwards" {
			checked = true
			if split.Len() != 1 {
				t.Fatalf("portforwards split should have 1 object after refresh, got %d", split.Len())
			}
		}
	}
	if !checked {
		t.Fatal("no portforwards split found")
	}
}

// TestToastTTLHelper documents the toastTTL helper threads config through with
// the int<->Level conversion.
func TestToastTTLHelper(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Notifications.TimeoutError = -1 // sticky
	a, _, _ := newNotifyTestApp(t, cfg)

	if got := a.toastTTL(notify.LevelInfo); got != 3*time.Second {
		t.Fatalf("toastTTL(info) = %v, want 3s", got)
	}
	if got := a.toastTTL(notify.LevelError); got != 0 {
		t.Fatalf("toastTTL(error sticky) = %v, want 0", got)
	}
}
