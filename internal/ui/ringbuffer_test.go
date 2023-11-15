package ui

import (
	"fmt"
	"testing"
)

func TestRingBuffer_Basic(t *testing.T) {
	rb := NewRingBuffer(3)
	if rb.Len() != 0 {
		t.Fatalf("expected len 0, got %d", rb.Len())
	}
	if rb.Dropped() != 0 {
		t.Fatalf("expected dropped 0, got %d", rb.Dropped())
	}

	rb.Append("a")
	rb.Append("b")
	rb.Append("c")
	if rb.Len() != 3 {
		t.Fatalf("expected len 3, got %d", rb.Len())
	}
	got := rb.All()
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("All(): got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("All()[%d]: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestRingBuffer_Wrap(t *testing.T) {
	rb := NewRingBuffer(3)
	rb.Append("a")
	rb.Append("b")
	rb.Append("c")
	rb.Append("d") // overwrites "a"
	if rb.Len() != 3 {
		t.Fatalf("expected len 3, got %d", rb.Len())
	}
	if rb.Dropped() != 1 {
		t.Fatalf("expected dropped 1, got %d", rb.Dropped())
	}
	got := rb.All()
	want := []string{"b", "c", "d"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("All()[%d]: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestRingBuffer_Get(t *testing.T) {
	rb := NewRingBuffer(3)
	rb.Append("a")
	rb.Append("b")
	rb.Append("c")
	rb.Append("d")
	rb.Append("e") // buffer: c, d, e
	if rb.Get(0) != "c" {
		t.Fatalf("Get(0): got %q, want %q", rb.Get(0), "c")
	}
	if rb.Get(2) != "e" {
		t.Fatalf("Get(2): got %q, want %q", rb.Get(2), "e")
	}
}

func TestRingBuffer_Slice(t *testing.T) {
	rb := NewRingBuffer(4)
	for _, s := range []string{"a", "b", "c", "d", "e", "f"} {
		rb.Append(s)
	}
	// buffer contains: c, d, e, f
	got := rb.Slice(1, 3)
	want := []string{"d", "e"}
	if len(got) != len(want) {
		t.Fatalf("Slice(1,3): got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Slice(1,3)[%d]: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestRingBuffer_Reset(t *testing.T) {
	rb := NewRingBuffer(3)
	rb.Append("a")
	rb.Append("b")
	rb.Reset()
	if rb.Len() != 0 {
		t.Fatalf("expected len 0 after reset, got %d", rb.Len())
	}
	if rb.Dropped() != 0 {
		t.Fatalf("expected dropped 0 after reset, got %d", rb.Dropped())
	}
	rb.Append("x")
	if rb.Len() != 1 || rb.Get(0) != "x" {
		t.Fatalf("unexpected state after reset+append")
	}
}

func TestRingBuffer_CapacityOne(t *testing.T) {
	rb := NewRingBuffer(1)
	rb.Append("a")
	rb.Append("b")
	if rb.Len() != 1 {
		t.Fatalf("expected len 1, got %d", rb.Len())
	}
	if rb.Dropped() != 1 {
		t.Fatalf("expected dropped 1, got %d", rb.Dropped())
	}
	if rb.Get(0) != "b" {
		t.Fatalf("Get(0): got %q, want %q", rb.Get(0), "b")
	}
}

func TestRingBuffer_AllBulkCopy(t *testing.T) {
	// Test both wrapped and unwrapped cases with various capacities
	for _, cap := range []int{1, 2, 3, 5, 10} {
		for n := 0; n <= cap+5; n++ {
			rb := NewRingBuffer(cap)
			for i := range n {
				rb.Append(fmt.Sprintf("line-%d", i))
			}

			got := rb.All()

			// Verify length
			wantLen := min(n, cap)
			if len(got) != wantLen {
				t.Fatalf("cap=%d n=%d: All() len=%d, want %d", cap, n, len(got), wantLen)
			}

			// Verify contents match Get()
			for i := range got {
				if got[i] != rb.Get(i) {
					t.Fatalf("cap=%d n=%d: All()[%d]=%q, Get(%d)=%q", cap, n, i, got[i], i, rb.Get(i))
				}
			}
		}
	}
}

func TestRingBuffer_SliceBulkCopy(t *testing.T) {
	rb := NewRingBuffer(5)
	for i := range 8 {
		rb.Append(fmt.Sprintf("line-%d", i))
	}
	// Buffer contains: line-3, line-4, line-5, line-6, line-7

	tests := []struct {
		start, end int
		want       []string
	}{
		{0, 5, []string{"line-3", "line-4", "line-5", "line-6", "line-7"}},
		{1, 4, []string{"line-4", "line-5", "line-6"}},
		{0, 1, []string{"line-3"}},
		{4, 5, []string{"line-7"}},
		{0, 0, nil},
		{3, 3, nil},
		{-1, 3, []string{"line-3", "line-4", "line-5"}},
		{2, 100, []string{"line-5", "line-6", "line-7"}},
	}

	for _, tt := range tests {
		got := rb.Slice(tt.start, tt.end)
		if len(got) != len(tt.want) {
			t.Fatalf("Slice(%d,%d): got %v, want %v", tt.start, tt.end, got, tt.want)
		}
		for i := range tt.want {
			if got[i] != tt.want[i] {
				t.Fatalf("Slice(%d,%d)[%d]: got %q, want %q", tt.start, tt.end, i, got[i], tt.want[i])
			}
		}
	}
}

func BenchmarkRingBufferAll(b *testing.B) {
	rb := NewRingBuffer(10000)
	for i := range 10000 {
		rb.Append(fmt.Sprintf("line %d: some log content here with enough text to be realistic", i))
	}
	b.ResetTimer()
	for range b.N {
		_ = rb.All()
	}
}
