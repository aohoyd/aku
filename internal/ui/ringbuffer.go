package ui

// RingBuffer is a fixed-capacity circular buffer for log lines.
// O(1) append, oldest lines are evicted when full.
// Not goroutine-safe — only access from the bubbletea Update goroutine.
type RingBuffer struct {
	lines    []string
	head     int // next write position
	count    int // current number of valid lines
	capacity int
	dropped  int // total lines evicted since creation
}

// NewRingBuffer creates a ring buffer with the given capacity.
func NewRingBuffer(capacity int) *RingBuffer {
	if capacity < 1 {
		capacity = 1
	}
	return &RingBuffer{
		lines:    make([]string, capacity),
		capacity: capacity,
	}
}

// Append adds a line. If at capacity, the oldest line is evicted.
func (r *RingBuffer) Append(line string) {
	if r.count == r.capacity {
		r.dropped++
	}
	r.lines[r.head] = line
	r.head = (r.head + 1) % r.capacity
	if r.count < r.capacity {
		r.count++
	}
}

// Len returns the number of lines currently stored.
func (r *RingBuffer) Len() int { return r.count }

// Dropped returns the total number of lines evicted since creation or last reset.
func (r *RingBuffer) Dropped() int { return r.dropped }

// Get returns the line at logical index i (0 = oldest retained line).
func (r *RingBuffer) Get(i int) string {
	if i < 0 || i >= r.count {
		return ""
	}
	start := (r.head - r.count + r.capacity) % r.capacity
	return r.lines[(start+i)%r.capacity]
}

// All returns all lines in order (oldest to newest).
func (r *RingBuffer) All() []string {
	if r.count == 0 {
		return nil
	}
	result := make([]string, r.count)
	start := (r.head - r.count + r.capacity) % r.capacity
	if start+r.count <= r.capacity {
		copy(result, r.lines[start:start+r.count])
	} else {
		n := r.capacity - start
		copy(result[:n], r.lines[start:])
		copy(result[n:], r.lines[:r.count-n])
	}
	return result
}

// Slice returns a copy of lines[start:end] in logical order.
func (r *RingBuffer) Slice(start, end int) []string {
	if start < 0 {
		start = 0
	}
	if end > r.count {
		end = r.count
	}
	if start >= end {
		return nil
	}
	n := end - start
	result := make([]string, n)
	physStart := (r.head - r.count + r.capacity + start) % r.capacity
	if physStart+n <= r.capacity {
		copy(result, r.lines[physStart:physStart+n])
	} else {
		first := r.capacity - physStart
		copy(result[:first], r.lines[physStart:])
		copy(result[first:], r.lines[:n-first])
	}
	return result
}

// Reset clears all state without reallocating.
// String references are zeroed so the GC can collect the underlying data.
func (r *RingBuffer) Reset() {
	clear(r.lines)
	r.head = 0
	r.count = 0
	r.dropped = 0
}
