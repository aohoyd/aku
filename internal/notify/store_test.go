package notify

import (
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/aohoyd/aku/internal/msgs"
)

func TestLevelString(t *testing.T) {
	tests := []struct {
		level Level
		want  string
	}{
		{LevelInfo, "info"},
		{LevelWarning, "warning"},
		{LevelError, "error"},
		{Level(99), "info"},
	}
	for _, tt := range tests {
		if got := tt.level.String(); got != tt.want {
			t.Errorf("Level(%d).String() = %q, want %q", tt.level, got, tt.want)
		}
	}
}

func TestAddListNewestFirst(t *testing.T) {
	s := NewStore(10)
	s.Add(LevelInfo, "first", "ctx", "src")
	s.Add(LevelWarning, "second", "ctx", "src")
	s.Add(LevelError, "third", "ctx", "src")

	got := s.List()
	if len(got) != 3 {
		t.Fatalf("List() len = %d, want 3", len(got))
	}
	want := []string{"third", "second", "first"}
	for i, w := range want {
		if got[i].Text != w {
			t.Errorf("List()[%d].Text = %q, want %q", i, got[i].Text, w)
		}
	}
	// IDs are monotonically assigned.
	if got[0].ID != 3 || got[1].ID != 2 || got[2].ID != 1 {
		t.Errorf("unexpected IDs: %d %d %d", got[0].ID, got[1].ID, got[2].ID)
	}
}

func TestCapEviction(t *testing.T) {
	s := NewStore(2)
	s.Add(LevelInfo, "one", "", "")
	s.Add(LevelInfo, "two", "", "")
	s.Add(LevelInfo, "three", "", "")

	got := s.List()
	if len(got) != 2 {
		t.Fatalf("List() len = %d, want 2", len(got))
	}
	// Newest-first: "three", "two"; "one" evicted.
	if got[0].Text != "three" || got[1].Text != "two" {
		t.Errorf("unexpected texts after eviction: %q, %q", got[0].Text, got[1].Text)
	}
}

func TestNewStoreDefaultCapacity(t *testing.T) {
	for _, capArg := range []int{0, -5} {
		s := NewStore(capArg)
		if s.cap != defaultCapacity {
			t.Errorf("NewStore(%d).cap = %d, want %d", capArg, s.cap, defaultCapacity)
		}
	}
}

func TestLive(t *testing.T) {
	base := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	s := NewStore(10)

	// Deterministic timestamps via the injectable clock.
	s.now = func() time.Time { return base }
	s.Add(LevelInfo, "info-old", "", "")    // ID 1
	s.Add(LevelWarning, "warn-old", "", "") // ID 2
	s.Add(LevelError, "err-sticky", "", "") // ID 3

	// ttl: info 5s, warning 10s, error 0 (sticky).
	ttl := func(l Level) time.Duration {
		switch l {
		case LevelInfo:
			return 5 * time.Second
		case LevelWarning:
			return 10 * time.Second
		default:
			return 0
		}
	}

	// At base+7s: info expired (7>=5), warning live (7<10), error sticky.
	got := s.Live(base.Add(7*time.Second), ttl)
	gotTexts := make([]string, len(got))
	for i, m := range got {
		gotTexts[i] = m.Text
	}
	// Newest-first ordering: error first, then warning.
	want := []string{"err-sticky", "warn-old"}
	if len(gotTexts) != len(want) {
		t.Fatalf("Live() = %v, want %v", gotTexts, want)
	}
	for i := range want {
		if gotTexts[i] != want[i] {
			t.Errorf("Live()[%d] = %q, want %q", i, gotTexts[i], want[i])
		}
	}

	// At base (now): everything live.
	if got := s.Live(base, ttl); len(got) != 3 {
		t.Errorf("Live(base) len = %d, want 3", len(got))
	}

	// Far future: only sticky error remains.
	got = s.Live(base.Add(time.Hour), ttl)
	if len(got) != 1 || got[0].Text != "err-sticky" {
		t.Errorf("Live(future) = %v, want [err-sticky]", got)
	}
}

func TestSendCallbackOnAdd(t *testing.T) {
	s := NewStore(10)
	// The send callback now fires asynchronously (see Store.Add), so synchronize
	// via a buffered channel rather than reading a shared variable.
	gotCh := make(chan tea.Msg, 1)
	s.SetSend(func(m tea.Msg) { gotCh <- m })

	created := s.Add(LevelWarning, "hello", "ctx", "src")

	var got tea.Msg
	select {
	case got = <-gotCh:
	case <-time.After(time.Second):
		t.Fatal("send callback was not invoked")
	}

	added, ok := got.(msgs.MessageAddedMsg)
	if !ok {
		t.Fatalf("expected msgs.MessageAddedMsg, got %T", got)
	}
	if added.ID != created.ID {
		t.Errorf("MessageAddedMsg.ID = %d, want %d", added.ID, created.ID)
	}
	if added.Level != int(LevelWarning) {
		t.Errorf("MessageAddedMsg.Level = %d, want %d", added.Level, int(LevelWarning))
	}
}

// TestAddNeverBlocks verifies Add returns promptly even when the send callback
// blocks forever. This is the regression guard for the Bubble Tea deadlock: Add
// is called from inside the (single-threaded) Update loop, and the program
// message channel is unbuffered, so a synchronous send would wedge the app.
func TestAddNeverBlocks(t *testing.T) {
	s := NewStore(10)
	block := make(chan struct{})
	s.SetSend(func(tea.Msg) { <-block }) // never returns
	defer close(block)

	done := make(chan struct{})
	go func() {
		s.Add(LevelInfo, "x", "", "t")
		close(done)
	}()
	select {
	case <-done: // Add returned despite the blocked send
	case <-time.After(time.Second):
		t.Fatal("Add blocked on send callback")
	}
}

func TestNoSendWhenUnset(t *testing.T) {
	s := NewStore(10)
	// Must not panic when send is nil.
	s.Add(LevelInfo, "no-send", "", "")
	if len(s.List()) != 1 {
		t.Fatal("Add should still store the message without a send callback")
	}
}

// TestLiveTTLExactBoundary verifies the TTL predicate is strict: a message whose
// age exactly equals its TTL (now.Sub(m.Time) == d) is treated as EXPIRED, since
// the predicate is `< d`, not `<= d`.
func TestLiveTTLExactBoundary(t *testing.T) {
	base := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	s := NewStore(10)
	s.now = func() time.Time { return base }
	s.Add(LevelInfo, "edge", "", "")

	ttl := func(Level) time.Duration { return 5 * time.Second }

	// Exactly at the boundary: age == TTL => expired.
	if got := s.Live(base.Add(5*time.Second), ttl); len(got) != 0 {
		t.Errorf("Live at exact TTL boundary should be empty (strict <), got %d", len(got))
	}
	// One nanosecond before the boundary: still live.
	if got := s.Live(base.Add(5*time.Second-time.Nanosecond), ttl); len(got) != 1 {
		t.Errorf("Live just before boundary should keep the message, got %d", len(got))
	}
}

// TestEmptyStoreLiveAndList verifies Live and List on a fresh, non-nil store
// return empty (non-nil where applicable) results without panicking.
func TestEmptyStoreLiveAndList(t *testing.T) {
	s := NewStore(10)
	ttl := func(Level) time.Duration { return time.Second }

	if got := s.Live(time.Now(), ttl); len(got) != 0 {
		t.Errorf("Live() on empty store = %v, want empty", got)
	}
	if got := s.List(); got == nil {
		t.Error("List() on empty store should return a non-nil empty slice")
	} else if len(got) != 0 {
		t.Errorf("List() on empty store = %v, want empty", got)
	}
}

// TestConcurrentSetSendAndAdd exercises SetSend racing with Add under -race to
// confirm the mutex protects the send field against concurrent writers/readers.
func TestConcurrentSetSendAndAdd(t *testing.T) {
	s := NewStore(100)

	var wg sync.WaitGroup
	// Writers repeatedly swap the send callback.
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				s.SetSend(func(tea.Msg) {})
			}
		}()
	}
	// Adders concurrently append (each reads s.send under the lock).
	for a := 0; a < 4; a++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				s.Add(LevelInfo, "x", "", "")
			}
		}()
	}
	wg.Wait()

	// Buffer stays bounded and consistent after the interleaving.
	if got := len(s.List()); got != 100 {
		t.Errorf("List() len after concurrent SetSend+Add = %d, want 100", got)
	}
}

func TestConcurrentAdd(t *testing.T) {
	const cap = 50
	const goroutines = 20
	const perGoroutine = 100
	const total = goroutines * perGoroutine

	s := NewStore(cap)
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				s.Add(LevelInfo, "x", "", "")
			}
		}()
	}
	wg.Wait()

	got := len(s.List())
	want := cap
	if total < cap {
		want = total
	}
	if got != want {
		t.Errorf("List() len after concurrent Add = %d, want %d", got, want)
	}
}
