package portforward

import (
	"testing"
)

func TestRegistryAddAndList(t *testing.T) {
	r := NewRegistry()
	id := r.Add(Entry{
		PodName:       "nginx-abc",
		PodNamespace:  "default",
		ContainerName: "nginx",
		LocalPort:     8080,
		RemotePort:    80,
		Protocol:      "TCP",
		Status:        "Active",
	})
	if id == "" {
		t.Fatal("expected non-empty ID")
	}
	entries := r.List()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].PodName != "nginx-abc" {
		t.Errorf("expected PodName nginx-abc, got %s", entries[0].PodName)
	}
	if r.Count() != 1 {
		t.Errorf("expected count 1, got %d", r.Count())
	}
}

func TestRegistryRemove(t *testing.T) {
	cancelled := false
	r := NewRegistry()
	id := r.Add(Entry{
		PodName:    "nginx",
		LocalPort:  8080,
		RemotePort: 80,
		Status:     "Active",
		Cancel:     func() { cancelled = true },
	})
	r.Remove(id)
	if !cancelled {
		t.Error("expected Cancel to be called")
	}
	if r.Count() != 0 {
		t.Errorf("expected count 0, got %d", r.Count())
	}
}

func TestRegistryStopAll(t *testing.T) {
	count := 0
	r := NewRegistry()
	r.Add(Entry{Cancel: func() { count++ }})
	r.Add(Entry{Cancel: func() { count++ }})
	r.StopAll()
	if count != 2 {
		t.Errorf("expected 2 cancels, got %d", count)
	}
	if r.Count() != 0 {
		t.Errorf("expected count 0 after StopAll, got %d", r.Count())
	}
}

func TestRegistryHasLocalPort(t *testing.T) {
	r := NewRegistry()
	r.Add(Entry{LocalPort: 8080, Status: "Active"})
	if !r.HasLocalPort(8080) {
		t.Error("expected HasLocalPort(8080) to be true")
	}
	if r.HasLocalPort(9090) {
		t.Error("expected HasLocalPort(9090) to be false")
	}
}

func TestRegistryAddIfNotPresent(t *testing.T) {
	r := NewRegistry()
	id, err := r.AddIfNotPresent(Entry{LocalPort: 8080, Status: "Active"})
	if err != nil {
		t.Fatalf("first add should succeed: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty ID")
	}
	if r.Count() != 1 {
		t.Fatalf("expected count 1, got %d", r.Count())
	}

	// Adding another entry with the same local port should fail
	_, err = r.AddIfNotPresent(Entry{LocalPort: 8080, Status: "Active"})
	if err == nil {
		t.Fatal("expected error for duplicate local port")
	}
	if r.Count() != 1 {
		t.Fatalf("count should still be 1, got %d", r.Count())
	}

	// Different port should succeed
	id2, err := r.AddIfNotPresent(Entry{LocalPort: 9090, Status: "Active"})
	if err != nil {
		t.Fatalf("different port should succeed: %v", err)
	}
	if id2 == "" {
		t.Fatal("expected non-empty ID for second entry")
	}
	if r.Count() != 2 {
		t.Fatalf("expected count 2, got %d", r.Count())
	}
}
