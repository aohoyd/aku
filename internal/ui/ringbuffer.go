package ui

// DualRingBuffer is a fixed-capacity circular buffer storing parallel raw and
// colored (highlighted) log lines. O(1) append, oldest lines are evicted when full.
// Not goroutine-safe — only access from the bubbletea Update goroutine.
type DualRingBuffer struct {
	raw      []string
	colored  []string
	widths   []int
	head     int // next write position
	count    int // current number of valid lines
	capacity int
	dropped  int // total lines evicted since creation
}

// NewDualRingBuffer creates a dual ring buffer with the given capacity.
func NewDualRingBuffer(capacity int) *DualRingBuffer {
	if capacity < 1 {
		capacity = 1
	}
	return &DualRingBuffer{
		raw:      make([]string, capacity),
		colored:  make([]string, capacity),
		widths:   make([]int, capacity),
		capacity: capacity,
	}
}

// Append adds a raw and colored line pair with a pre-computed display width.
// If at capacity, the oldest pair is evicted.
func (d *DualRingBuffer) Append(raw, colored string, rawWidth int) {
	if d.count == d.capacity {
		d.dropped++
	}
	d.raw[d.head] = raw
	d.colored[d.head] = colored
	d.widths[d.head] = rawWidth
	d.head = (d.head + 1) % d.capacity
	if d.count < d.capacity {
		d.count++
	}
}

// Len returns the number of line pairs currently stored.
func (d *DualRingBuffer) Len() int { return d.count }

// Dropped returns the total number of pairs evicted since creation or last reset.
func (d *DualRingBuffer) Dropped() int { return d.dropped }

// RawGet returns the raw line at logical index i (0 = oldest).
func (d *DualRingBuffer) RawGet(i int) string {
	if i < 0 || i >= d.count {
		return ""
	}
	return d.raw[d.physIdx(i)]
}

// ColoredGet returns the colored line at logical index i (0 = oldest).
func (d *DualRingBuffer) ColoredGet(i int) string {
	if i < 0 || i >= d.count {
		return ""
	}
	return d.colored[d.physIdx(i)]
}

// WidthGet returns the pre-computed display width at logical index i (0 = oldest).
// Returns 0 for out-of-bounds indices.
func (d *DualRingBuffer) WidthGet(i int) int {
	if i < 0 || i >= d.count {
		return 0
	}
	return d.widths[d.physIdx(i)]
}

// WidthSlice returns a copy of widths[start:end] in logical order.
func (d *DualRingBuffer) WidthSlice(start, end int) []int {
	return d.copyIntSlice(d.widths, start, end)
}

// SetColored overwrites the colored line at logical index i.
func (d *DualRingBuffer) SetColored(i int, s string, width int) {
	if i < 0 || i >= d.count {
		return
	}
	idx := d.physIdx(i)
	d.colored[idx] = s
	d.widths[idx] = width
}

// RawAll returns all raw lines in order (oldest to newest) using bulk copy.
func (d *DualRingBuffer) RawAll() []string { return d.copySlice(d.raw, 0, d.count) }

// ColoredAll returns all colored lines in order (oldest to newest) using bulk copy.
func (d *DualRingBuffer) ColoredAll() []string { return d.copySlice(d.colored, 0, d.count) }

// RawSlice returns a copy of raw lines[start:end] in logical order.
func (d *DualRingBuffer) RawSlice(start, end int) []string { return d.copySlice(d.raw, start, end) }

// ColoredSlice returns a copy of colored lines[start:end] in logical order.
func (d *DualRingBuffer) ColoredSlice(start, end int) []string {
	return d.copySlice(d.colored, start, end)
}

// Reset clears all state without reallocating.
func (d *DualRingBuffer) Reset() {
	clear(d.raw)
	clear(d.colored)
	clear(d.widths)
	d.head = 0
	d.count = 0
	d.dropped = 0
}

// physIdx converts a logical index to a physical array index.
func (d *DualRingBuffer) physIdx(i int) int {
	return (d.head - d.count + d.capacity + i) % d.capacity
}

// copySlice extracts a logical range [start, end) from the given backing array
// using bulk copy where possible.
func (d *DualRingBuffer) copySlice(arr []string, start, end int) []string {
	if start < 0 {
		start = 0
	}
	if end > d.count {
		end = d.count
	}
	if start >= end {
		return nil
	}
	n := end - start
	result := make([]string, n)
	physStart := d.physIdx(start)
	if physStart+n <= d.capacity {
		copy(result, arr[physStart:physStart+n])
	} else {
		first := d.capacity - physStart
		copy(result[:first], arr[physStart:])
		copy(result[first:], arr[:n-first])
	}
	return result
}

// copyIntSlice extracts a logical range [start, end) from the given int backing array
// using bulk copy where possible.
func (d *DualRingBuffer) copyIntSlice(arr []int, start, end int) []int {
	if start < 0 {
		start = 0
	}
	if end > d.count {
		end = d.count
	}
	if start >= end {
		return nil
	}
	n := end - start
	result := make([]int, n)
	physStart := d.physIdx(start)
	if physStart+n <= d.capacity {
		copy(result, arr[physStart:physStart+n])
	} else {
		first := d.capacity - physStart
		copy(result[:first], arr[physStart:])
		copy(result[first:], arr[:n-first])
	}
	return result
}
