package portforward

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
)

// Entry represents an active port-forward.
type Entry struct {
	ID            string
	PodName       string
	PodNamespace  string
	ContainerName string
	LocalPort     int
	RemotePort    int
	Protocol      string
	Status        string
	Cancel        context.CancelFunc
}

// Registry tracks active port-forward processes.
type Registry struct {
	mu      sync.RWMutex
	entries map[string]*Entry
	nextID  atomic.Int64
}

// NewRegistry creates a new empty registry.
func NewRegistry() *Registry {
	return &Registry{entries: make(map[string]*Entry)}
}

// Add stores an entry and returns its unique ID.
func (r *Registry) Add(e Entry) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	e.ID = fmt.Sprintf("pf-%d", r.nextID.Add(1))
	r.entries[e.ID] = &e
	return e.ID
}

// AddIfNotPresent atomically checks that no entry uses the given local port
// and inserts the entry. Returns an error if the port is already in use.
func (r *Registry) AddIfNotPresent(e Entry) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.entries {
		if existing.LocalPort == e.LocalPort {
			return "", fmt.Errorf("local port %d already in use by another port-forward", e.LocalPort)
		}
	}
	e.ID = fmt.Sprintf("pf-%d", r.nextID.Add(1))
	r.entries[e.ID] = &e
	return e.ID, nil
}

// Remove stops and removes an entry by ID.
func (r *Registry) Remove(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e, ok := r.entries[id]; ok {
		if e.Cancel != nil {
			e.Cancel()
		}
		delete(r.entries, id)
	}
}

// List returns a snapshot of all entries.
func (r *Registry) List() []Entry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Entry, 0, len(r.entries))
	for _, e := range r.entries {
		out = append(out, *e)
	}
	return out
}

// Count returns the number of active entries.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.entries)
}

// HasLocalPort returns true if any entry uses the given local port.
func (r *Registry) HasLocalPort(port int) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, e := range r.entries {
		if e.LocalPort == port {
			return true
		}
	}
	return false
}

// StopAll cancels and removes all entries.
func (r *Registry) StopAll() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.entries {
		if e.Cancel != nil {
			e.Cancel()
		}
	}
	r.entries = make(map[string]*Entry)
}
