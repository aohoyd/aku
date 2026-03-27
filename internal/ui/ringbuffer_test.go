package ui

import (
	"fmt"
	"testing"
)

func TestDualRingBuffer_Basic(t *testing.T) {
	rb := NewDualRingBuffer(3)
	if rb.Len() != 0 {
		t.Fatalf("expected len 0, got %d", rb.Len())
	}
	if rb.Dropped() != 0 {
		t.Fatalf("expected dropped 0, got %d", rb.Dropped())
	}

	rb.Append("a", "A", 1)
	rb.Append("b", "B", 1)
	rb.Append("c", "C", 1)
	if rb.Len() != 3 {
		t.Fatalf("expected len 3, got %d", rb.Len())
	}
	raw := rb.RawAll()
	colored := rb.ColoredAll()
	wantRaw := []string{"a", "b", "c"}
	wantColored := []string{"A", "B", "C"}
	for i := range wantRaw {
		if raw[i] != wantRaw[i] {
			t.Fatalf("RawAll()[%d]: got %q, want %q", i, raw[i], wantRaw[i])
		}
		if colored[i] != wantColored[i] {
			t.Fatalf("ColoredAll()[%d]: got %q, want %q", i, colored[i], wantColored[i])
		}
	}
}

func TestDualRingBuffer_Wrap(t *testing.T) {
	rb := NewDualRingBuffer(3)
	rb.Append("a", "A", 1)
	rb.Append("b", "B", 1)
	rb.Append("c", "C", 1)
	rb.Append("d", "D", 1) // evicts "a"/"A"
	if rb.Len() != 3 {
		t.Fatalf("expected len 3, got %d", rb.Len())
	}
	if rb.Dropped() != 1 {
		t.Fatalf("expected dropped 1, got %d", rb.Dropped())
	}
	raw := rb.RawAll()
	wantRaw := []string{"b", "c", "d"}
	for i := range wantRaw {
		if raw[i] != wantRaw[i] {
			t.Fatalf("RawAll()[%d]: got %q, want %q", i, raw[i], wantRaw[i])
		}
	}
}

func TestDualRingBuffer_RawGet_ColoredGet(t *testing.T) {
	rb := NewDualRingBuffer(3)
	rb.Append("a", "A", 1)
	rb.Append("b", "B", 1)
	rb.Append("c", "C", 1)
	rb.Append("d", "D", 1)
	rb.Append("e", "E", 1) // buffer: c/C, d/D, e/E
	if rb.RawGet(0) != "c" {
		t.Fatalf("RawGet(0): got %q, want %q", rb.RawGet(0), "c")
	}
	if rb.ColoredGet(0) != "C" {
		t.Fatalf("ColoredGet(0): got %q, want %q", rb.ColoredGet(0), "C")
	}
	if rb.RawGet(2) != "e" {
		t.Fatalf("RawGet(2): got %q, want %q", rb.RawGet(2), "e")
	}
	if rb.ColoredGet(2) != "E" {
		t.Fatalf("ColoredGet(2): got %q, want %q", rb.ColoredGet(2), "E")
	}
}

func TestDualRingBuffer_Slice(t *testing.T) {
	rb := NewDualRingBuffer(4)
	for _, s := range []string{"a", "b", "c", "d", "e", "f"} {
		rb.Append(s, fmt.Sprintf("%s-colored", s), len(s))
	}
	// buffer: c, d, e, f
	raw := rb.RawSlice(1, 3)
	wantRaw := []string{"d", "e"}
	if len(raw) != len(wantRaw) {
		t.Fatalf("RawSlice(1,3): got %v, want %v", raw, wantRaw)
	}
	for i := range wantRaw {
		if raw[i] != wantRaw[i] {
			t.Fatalf("RawSlice(1,3)[%d]: got %q, want %q", i, raw[i], wantRaw[i])
		}
	}
	colored := rb.ColoredSlice(1, 3)
	wantColored := []string{"d-colored", "e-colored"}
	for i := range wantColored {
		if colored[i] != wantColored[i] {
			t.Fatalf("ColoredSlice(1,3)[%d]: got %q, want %q", i, colored[i], wantColored[i])
		}
	}
}

func TestDualRingBuffer_SliceBulkCopy(t *testing.T) {
	rb := NewDualRingBuffer(5)
	for i := range 8 {
		rb.Append(fmt.Sprintf("line-%d", i), fmt.Sprintf("LINE-%d", i), i+1)
	}
	// Buffer: line-3..line-7

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
		got := rb.RawSlice(tt.start, tt.end)
		if len(got) != len(tt.want) {
			t.Fatalf("RawSlice(%d,%d): got %v, want %v", tt.start, tt.end, got, tt.want)
		}
		for i := range tt.want {
			if got[i] != tt.want[i] {
				t.Fatalf("RawSlice(%d,%d)[%d]: got %q, want %q", tt.start, tt.end, i, got[i], tt.want[i])
			}
		}
	}
}

func TestDualRingBuffer_SetColored(t *testing.T) {
	rb := NewDualRingBuffer(3)
	rb.Append("a", "A", 1)
	rb.Append("b", "B", 1)

	rb.SetColored(0, "A-updated", 9)
	if rb.ColoredGet(0) != "A-updated" {
		t.Fatalf("expected updated colored, got %q", rb.ColoredGet(0))
	}
	// Raw unchanged
	if rb.RawGet(0) != "a" {
		t.Fatalf("raw should be unchanged, got %q", rb.RawGet(0))
	}
	// Width updated
	if rb.WidthGet(0) != 9 {
		t.Fatalf("expected updated width 9, got %d", rb.WidthGet(0))
	}
}

func TestDualRingBuffer_Reset(t *testing.T) {
	rb := NewDualRingBuffer(3)
	rb.Append("a", "A", 1)
	rb.Append("b", "B", 1)
	rb.Reset()
	if rb.Len() != 0 {
		t.Fatalf("expected len 0 after reset, got %d", rb.Len())
	}
	if rb.Dropped() != 0 {
		t.Fatalf("expected dropped 0 after reset, got %d", rb.Dropped())
	}
	rb.Append("x", "X", 1)
	if rb.Len() != 1 || rb.RawGet(0) != "x" || rb.ColoredGet(0) != "X" {
		t.Fatalf("unexpected state after reset+append")
	}
}

func TestDualRingBuffer_CapacityOne(t *testing.T) {
	rb := NewDualRingBuffer(1)
	rb.Append("a", "A", 1)
	rb.Append("b", "B", 1)
	if rb.Len() != 1 {
		t.Fatalf("expected len 1, got %d", rb.Len())
	}
	if rb.Dropped() != 1 {
		t.Fatalf("expected dropped 1, got %d", rb.Dropped())
	}
	if rb.RawGet(0) != "b" || rb.ColoredGet(0) != "B" {
		t.Fatalf("expected b/B, got %q/%q", rb.RawGet(0), rb.ColoredGet(0))
	}
}

func TestDualRingBuffer_AllBulkCopy(t *testing.T) {
	for _, cap := range []int{1, 2, 3, 5, 10} {
		for n := 0; n <= cap+5; n++ {
			rb := NewDualRingBuffer(cap)
			for i := range n {
				rb.Append(fmt.Sprintf("line-%d", i), fmt.Sprintf("LINE-%d", i), i+1)
			}
			raw := rb.RawAll()
			wantLen := min(n, cap)
			if len(raw) != wantLen {
				t.Fatalf("cap=%d n=%d: RawAll() len=%d, want %d", cap, n, len(raw), wantLen)
			}
			for i := range raw {
				if raw[i] != rb.RawGet(i) {
					t.Fatalf("cap=%d n=%d: RawAll()[%d]=%q, RawGet(%d)=%q", cap, n, i, raw[i], i, rb.RawGet(i))
				}
			}
		}
	}
}

// --- Width tracking tests ---

func TestDualRingBuffer_WidthGet_Basic(t *testing.T) {
	rb := NewDualRingBuffer(5)
	rb.Append("hi", "HI", 2)
	rb.Append("hello", "HELLO", 5)
	rb.Append("x", "X", 1)

	if got := rb.WidthGet(0); got != 2 {
		t.Fatalf("WidthGet(0): got %d, want 2", got)
	}
	if got := rb.WidthGet(1); got != 5 {
		t.Fatalf("WidthGet(1): got %d, want 5", got)
	}
	if got := rb.WidthGet(2); got != 1 {
		t.Fatalf("WidthGet(2): got %d, want 1", got)
	}
}

func TestDualRingBuffer_WidthGet_OutOfBounds(t *testing.T) {
	rb := NewDualRingBuffer(3)
	rb.Append("a", "A", 1)

	if got := rb.WidthGet(-1); got != 0 {
		t.Fatalf("WidthGet(-1): got %d, want 0", got)
	}
	if got := rb.WidthGet(1); got != 0 {
		t.Fatalf("WidthGet(1): got %d, want 0 (only 1 element)", got)
	}
	if got := rb.WidthGet(100); got != 0 {
		t.Fatalf("WidthGet(100): got %d, want 0", got)
	}
}

func TestDualRingBuffer_WidthGet_WrapAround(t *testing.T) {
	rb := NewDualRingBuffer(3)
	// Fill and wrap: widths 10, 20, 30, 40, 50 -> keeps last 3: 30, 40, 50
	rb.Append("a", "A", 10)
	rb.Append("bb", "BB", 20)
	rb.Append("ccc", "CCC", 30)
	rb.Append("dddd", "DDDD", 40)   // evicts "a" (width 10)
	rb.Append("eeeee", "EEEEE", 50) // evicts "bb" (width 20)

	if rb.Len() != 3 {
		t.Fatalf("expected len 3, got %d", rb.Len())
	}
	// Logical index 0 = oldest surviving = "ccc" (width 30)
	if got := rb.WidthGet(0); got != 30 {
		t.Fatalf("WidthGet(0) after wrap: got %d, want 30", got)
	}
	if got := rb.WidthGet(1); got != 40 {
		t.Fatalf("WidthGet(1) after wrap: got %d, want 40", got)
	}
	if got := rb.WidthGet(2); got != 50 {
		t.Fatalf("WidthGet(2) after wrap: got %d, want 50", got)
	}
}

func TestDualRingBuffer_WidthSlice_Basic(t *testing.T) {
	rb := NewDualRingBuffer(5)
	rb.Append("a", "A", 1)
	rb.Append("bb", "BB", 2)
	rb.Append("ccc", "CCC", 3)
	rb.Append("dddd", "DDDD", 4)
	rb.Append("eeeee", "EEEEE", 5)

	got := rb.WidthSlice(1, 4)
	want := []int{2, 3, 4}
	if len(got) != len(want) {
		t.Fatalf("WidthSlice(1,4): got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("WidthSlice(1,4)[%d]: got %d, want %d", i, got[i], want[i])
		}
	}
}

func TestDualRingBuffer_WidthSlice_WrapAround(t *testing.T) {
	rb := NewDualRingBuffer(4)
	// Insert 7 items into capacity-4 buffer -> keeps last 4
	for i := range 7 {
		rb.Append(fmt.Sprintf("line-%d", i), fmt.Sprintf("LINE-%d", i), (i+1)*10)
	}
	// Buffer: line-3(40), line-4(50), line-5(60), line-6(70)

	got := rb.WidthSlice(0, 4)
	want := []int{40, 50, 60, 70}
	if len(got) != len(want) {
		t.Fatalf("WidthSlice(0,4): got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("WidthSlice(0,4)[%d]: got %d, want %d", i, got[i], want[i])
		}
	}

	// Partial slice across wrap boundary
	got2 := rb.WidthSlice(1, 3)
	want2 := []int{50, 60}
	if len(got2) != len(want2) {
		t.Fatalf("WidthSlice(1,3): got %v, want %v", got2, want2)
	}
	for i := range want2 {
		if got2[i] != want2[i] {
			t.Fatalf("WidthSlice(1,3)[%d]: got %d, want %d", i, got2[i], want2[i])
		}
	}
}

func TestDualRingBuffer_WidthSlice_Empty(t *testing.T) {
	rb := NewDualRingBuffer(5)

	// Empty buffer
	got := rb.WidthSlice(0, 0)
	if got != nil {
		t.Fatalf("WidthSlice(0,0) on empty: got %v, want nil", got)
	}

	// start == end
	rb.Append("a", "A", 1)
	got = rb.WidthSlice(0, 0)
	if got != nil {
		t.Fatalf("WidthSlice(0,0): got %v, want nil", got)
	}

	// start > end
	got = rb.WidthSlice(2, 1)
	if got != nil {
		t.Fatalf("WidthSlice(2,1): got %v, want nil", got)
	}
}

func TestDualRingBuffer_WidthSlice_OutOfBounds(t *testing.T) {
	rb := NewDualRingBuffer(3)
	rb.Append("a", "A", 10)
	rb.Append("bb", "BB", 20)

	// Negative start is clamped to 0
	got := rb.WidthSlice(-5, 2)
	want := []int{10, 20}
	if len(got) != len(want) {
		t.Fatalf("WidthSlice(-5,2): got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("WidthSlice(-5,2)[%d]: got %d, want %d", i, got[i], want[i])
		}
	}

	// End beyond count is clamped
	got = rb.WidthSlice(0, 100)
	if len(got) != 2 {
		t.Fatalf("WidthSlice(0,100): got len %d, want 2", len(got))
	}
}

func TestDualRingBuffer_WidthPreservedAfterReset(t *testing.T) {
	rb := NewDualRingBuffer(3)
	rb.Append("hello", "HELLO", 5)
	rb.Append("world", "WORLD", 5)
	rb.Reset()

	// After reset, all widths should be gone
	if got := rb.WidthGet(0); got != 0 {
		t.Fatalf("WidthGet(0) after reset: got %d, want 0", got)
	}

	// Re-append and verify widths work correctly
	rb.Append("ab", "AB", 2)
	rb.Append("cde", "CDE", 3)
	if got := rb.WidthGet(0); got != 2 {
		t.Fatalf("WidthGet(0) after reset+append: got %d, want 2", got)
	}
	if got := rb.WidthGet(1); got != 3 {
		t.Fatalf("WidthGet(1) after reset+append: got %d, want 3", got)
	}

	widths := rb.WidthSlice(0, 2)
	want := []int{2, 3}
	if len(widths) != len(want) {
		t.Fatalf("WidthSlice after reset+append: got %v, want %v", widths, want)
	}
	for i := range want {
		if widths[i] != want[i] {
			t.Fatalf("WidthSlice after reset+append[%d]: got %d, want %d", i, widths[i], want[i])
		}
	}
}

func BenchmarkDualRingBufferColoredSlice(b *testing.B) {
	rb := NewDualRingBuffer(10000)
	for i := range 10000 {
		rb.Append(
			fmt.Sprintf("line %d: some log content", i),
			fmt.Sprintf("\x1b[31mline %d: some log content\x1b[0m", i),
			len(fmt.Sprintf("line %d: some log content", i)),
		)
	}
	b.ResetTimer()
	for range b.N {
		_ = rb.ColoredSlice(9978, 10000) // last 22 lines (viewport window)
	}
}

func BenchmarkDualRingBufferColoredAll(b *testing.B) {
	rb := NewDualRingBuffer(10000)
	for i := range 10000 {
		rb.Append(
			fmt.Sprintf("line %d: some log content", i),
			fmt.Sprintf("\x1b[31mline %d: some log content\x1b[0m", i),
			len(fmt.Sprintf("line %d: some log content", i)),
		)
	}
	b.ResetTimer()
	for range b.N {
		_ = rb.ColoredAll()
	}
}
