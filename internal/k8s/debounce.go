package k8s

import (
	"sync"
	"time"
)

// Debouncer coalesces rapid calls per key. After Trigger is called,
// it waits for a quiet period before invoking the callback.
// Additional Trigger calls during the window reset the timer.
type Debouncer struct {
	interval time.Duration
	callback func(watchKey)
	mu       sync.Mutex
	timers   map[watchKey]*time.Timer
}

// NewDebouncer creates a Debouncer with the given quiet interval.
func NewDebouncer(interval time.Duration, cb func(watchKey)) *Debouncer {
	return &Debouncer{
		interval: interval,
		callback: cb,
		timers:   make(map[watchKey]*time.Timer),
	}
}

// Trigger schedules a callback for key, resetting any existing timer.
func (d *Debouncer) Trigger(key watchKey) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if t, ok := d.timers[key]; ok {
		t.Stop()
		delete(d.timers, key)
	}
	var t *time.Timer
	t = time.AfterFunc(d.interval, func() {
		d.mu.Lock()
		if d.timers[key] == t {
			delete(d.timers, key)
			d.mu.Unlock()
			d.callback(key)
		} else {
			d.mu.Unlock()
		}
	})
	d.timers[key] = t
}

// Cancel stops the timer for a specific key if one is pending.
func (d *Debouncer) Cancel(key watchKey) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if t, ok := d.timers[key]; ok {
		t.Stop()
		delete(d.timers, key)
	}
}

// Stop cancels all pending timers.
func (d *Debouncer) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()
	for k, t := range d.timers {
		t.Stop()
		delete(d.timers, k)
	}
}
