// Package notify provides a global, bounded store of aku's own
// info/warning/error messages. The store is the backing model for both the
// toast overlay and the aku-messages synthetic resource; later tasks consume
// it. It mirrors the conventions of internal/portforward.Registry and
// internal/k8s.Store: a mutex-guarded buffer plus a send func(tea.Msg) notify
// callback that is invoked AFTER the lock is released.
package notify

import (
	"slices"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/aohoyd/aku/internal/msgs"
)

// defaultCapacity bounds the message buffer when NewStore is given a
// non-positive capacity. This is a defensive fallback: production always passes
// config.NotifyBufferSize() (already defaulted), so this only triggers for
// direct NewStore(<=0) callers (e.g. tests). Keep in sync with
// config.defaultNotifyBufferSize.
const defaultCapacity = 1000

// Sticky is the sentinel TTL meaning "never auto-hide". When a level's TTL
// equals Sticky, Live keeps every message of that level visible until it is
// explicitly dismissed. Config's ToastTTL returns this (0) for a negative
// per-level timeout.
const Sticky = time.Duration(0)

// Level is the severity of a message.
type Level int

const (
	LevelInfo Level = iota
	LevelWarning
	LevelError
)

// String returns the lowercase severity name, used for column display.
func (l Level) String() string {
	switch l {
	case LevelWarning:
		return "warning"
	case LevelError:
		return "error"
	default:
		return "info"
	}
}

// Message is a single stored message.
type Message struct {
	ID      uint64
	Time    time.Time
	Level   Level
	Text    string
	Context string
	Source  string
}

// Store is a bounded, mutex-guarded ring of messages with a send callback.
type Store struct {
	mu     sync.Mutex
	msgs   []Message
	nextID uint64
	cap    int
	send   func(tea.Msg)
	now    func() time.Time
}

// NewStore creates a store bounded to capacity messages. A non-positive
// capacity falls back to defaultCapacity. The clock defaults to time.Now and
// can be overridden in tests via the now field.
func NewStore(capacity int) *Store {
	if capacity <= 0 {
		capacity = defaultCapacity
	}
	return &Store{
		cap: capacity,
		now: time.Now,
	}
}

// SetSend wires the notify callback invoked after each Add.
func (s *Store) SetSend(fn func(tea.Msg)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.send = fn
}

// SetClock overrides the timestamp source used by Add. A nil fn resets to
// time.Now. Intended for deterministic tests; production code leaves the
// default.
func (s *Store) SetClock(fn func() time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if fn == nil {
		fn = time.Now
	}
	s.now = fn
}

// Add appends a message stamped with the store clock, evicts the oldest
// entries past capacity, and returns the created Message. The send callback (if
// any) is invoked asynchronously on a separate goroutine after the lock is
// released, so Add NEVER blocks its caller (see the dispatch comment below).
func (s *Store) Add(level Level, text, ctx, source string) Message {
	s.mu.Lock()
	s.nextID++
	m := Message{
		ID:      s.nextID,
		Time:    s.now(),
		Level:   level,
		Text:    text,
		Context: ctx,
		Source:  source,
	}
	s.msgs = append(s.msgs, m)
	if len(s.msgs) > s.cap {
		// Drop the oldest entry by shifting the live window to the front of the
		// existing backing array and truncating to cap. This reuses one backing
		// array that holds exactly cap elements: no evicted Message is retained
		// (the slot past cap is overwritten on the next Add and is unreachable
		// via the cap-length slice), and memory is bounded to exactly cap.
		copy(s.msgs, s.msgs[len(s.msgs)-s.cap:])
		s.msgs = s.msgs[:s.cap]
	}
	send := s.send
	s.mu.Unlock()

	if send != nil {
		// Dispatch on its own goroutine so Add NEVER blocks its caller. The send
		// callback is wired to tea.Program.Send, whose channel is unbuffered;
		// calling it synchronously from within the Bubble Tea Update loop (the
		// common case — every in-Update notify.Add) deadlocks the program: Send
		// blocks until the loop reads the channel, but the loop cannot read until
		// Update returns. Toast FIFO ordering comes from the store buffer, not
		// delivery order, and the MessageAddedMsg handler tolerates
		// out-of-order/evicted/expired arrivals, so async delivery is safe.
		go send(msgs.MessageAddedMsg{ID: m.ID, Level: int(m.Level)})
	}
	return m
}

// List returns a newest-first copy of the buffer.
func (s *Store) List() []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Message, len(s.msgs))
	copy(out, s.msgs)
	slices.Reverse(out)
	return out
}

// Live returns newest-first messages still considered live at now. A message is
// live if ttl(m.Level) == Sticky (never auto-hide) or now.Sub(m.Time) <
// ttl(m.Level). This is a pure TTL predicate; explicit expiry/dismissal of
// specific IDs is handled by the App, not here.
func (s *Store) Live(now time.Time, ttl func(Level) time.Duration) []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Message, 0, len(s.msgs))
	for i := len(s.msgs) - 1; i >= 0; i-- {
		m := s.msgs[i]
		d := ttl(m.Level)
		if d == Sticky || now.Sub(m.Time) < d {
			out = append(out, m)
		}
	}
	return out
}
