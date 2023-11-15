package k8s

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestDebouncerCoalesces(t *testing.T) {
	var count atomic.Int32
	d := NewDebouncer(50*time.Millisecond, func(key watchKey) {
		count.Add(1)
	})
	defer d.Stop()

	key := watchKey{Namespace: "default"}
	for range 10 {
		d.Trigger(key)
	}
	time.Sleep(150 * time.Millisecond)
	if c := count.Load(); c != 1 {
		t.Fatalf("expected 1 coalesced callback, got %d", c)
	}
}

func TestDebouncerPerKey(t *testing.T) {
	var count atomic.Int32
	d := NewDebouncer(50*time.Millisecond, func(key watchKey) {
		count.Add(1)
	})
	defer d.Stop()

	d.Trigger(watchKey{Namespace: "a"})
	d.Trigger(watchKey{Namespace: "b"})
	time.Sleep(150 * time.Millisecond)
	if c := count.Load(); c != 2 {
		t.Fatalf("expected 2 callbacks (one per key), got %d", c)
	}
}

func TestDebouncerRapidRetrigger(t *testing.T) {
	var count atomic.Int32
	d := NewDebouncer(20*time.Millisecond, func(key watchKey) {
		count.Add(1)
	})
	defer d.Stop()

	key := watchKey{Namespace: "default"}

	// Trigger and wait for timer to fire
	d.Trigger(key)
	time.Sleep(50 * time.Millisecond)

	// First callback should have fired
	if c := count.Load(); c != 1 {
		t.Fatalf("expected 1 callback after first fire, got %d", c)
	}

	// Rapid re-trigger — should coalesce to exactly 1 more callback
	for range 5 {
		d.Trigger(key)
	}
	time.Sleep(100 * time.Millisecond)

	if c := count.Load(); c != 2 {
		t.Fatalf("expected 2 total callbacks, got %d", c)
	}
}

func TestDebouncerCancel(t *testing.T) {
	var count atomic.Int32
	d := NewDebouncer(50*time.Millisecond, func(key watchKey) {
		count.Add(1)
	})
	defer d.Stop()

	key := watchKey{Namespace: "default"}
	d.Trigger(key)
	d.Cancel(key)
	time.Sleep(100 * time.Millisecond)
	if c := count.Load(); c != 0 {
		t.Fatalf("expected 0 callbacks after Cancel, got %d", c)
	}
}

func TestDebouncerTimerIdentityRace(t *testing.T) {
	// Stress test for the timer identity race. Without the pointer-identity
	// fix, a stale timer goroutine can consume a fresh timer's map entry,
	// causing a lost callback. Run with: go test -race -count=100
	//
	// Two triggers are issued with a tiny gap. Depending on goroutine
	// scheduling, the first timer may or may not complete before the
	// second Trigger. Either way, the *last* trigger's callback must
	// always fire — we must never get 0 callbacks.
	for range 100 {
		var count atomic.Int32
		got := make(chan struct{}, 2)

		d := NewDebouncer(1*time.Microsecond, func(key watchKey) {
			count.Add(1)
			got <- struct{}{}
		})

		key := watchKey{Namespace: "race"}

		// First trigger — timer fires almost immediately
		d.Trigger(key)
		// Tiny sleep to let T1 fire but (sometimes) not yet acquire the lock
		time.Sleep(5 * time.Microsecond)
		// Second trigger — if T1's goroutine hasn't acquired the lock yet,
		// this creates the race condition
		d.Trigger(key)

		// Wait for at least one callback (the last trigger must always fire)
		select {
		case <-got:
			// At least one callback fired — correct behavior
		case <-time.After(500 * time.Millisecond):
			c := count.Load()
			if c == 0 {
				d.Stop()
				t.Fatalf("expected at least 1 callback, got 0 (lost notification)")
			}
		}
		// Drain any second callback
		select {
		case <-got:
		case <-time.After(10 * time.Millisecond):
		}
		d.Stop()
	}
}

func TestDebouncerStop(t *testing.T) {
	var count atomic.Int32
	d := NewDebouncer(50*time.Millisecond, func(key watchKey) {
		count.Add(1)
	})

	d.Trigger(watchKey{Namespace: "default"})
	d.Stop()
	time.Sleep(100 * time.Millisecond)
	if c := count.Load(); c != 0 {
		t.Fatalf("expected 0 callbacks after Stop, got %d", c)
	}
}
