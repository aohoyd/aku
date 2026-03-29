package k8s

import (
	"fmt"
	"io"
	"sync"
	"time"
)

// spinnerFrames contains the braille dots animation pattern.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// spinner displays an animated terminal spinner with status text.
type spinner struct {
	writer   io.Writer
	status   string
	mu       sync.Mutex
	done     chan struct{}
	stopped  chan struct{}
	started  bool
	stopOnce sync.Once
}

// newSpinner creates a spinner that writes to w with the given initial status.
func newSpinner(w io.Writer, initialStatus string) *spinner {
	return &spinner{
		writer:  w,
		status:  initialStatus,
		done:    make(chan struct{}),
		stopped: make(chan struct{}),
	}
}

// Start spawns a goroutine that renders spinner frames at ~80ms intervals.
// Safe to call multiple times — only the first call starts the goroutine.
func (s *spinner) Start() {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return
	}
	s.started = true
	s.mu.Unlock()

	go func() {
		defer close(s.stopped)
		ticker := time.NewTicker(80 * time.Millisecond)
		defer ticker.Stop()

		frame := 0
		for {
			s.mu.Lock()
			status := s.status
			s.mu.Unlock()

			fmt.Fprintf(s.writer, "\r\033[2K%s %s", spinnerFrames[frame], status)

			frame = (frame + 1) % len(spinnerFrames)

			select {
			case <-s.done:
				return
			case <-ticker.C:
			}
		}
	}()
}

// SetStatus updates the spinner's status text in a thread-safe manner.
func (s *spinner) SetStatus(text string) {
	s.mu.Lock()
	s.status = text
	s.mu.Unlock()
}

// Stop signals the spinner goroutine to exit, waits for it, clears the line,
// and optionally prints a final message. Safe to call multiple times or without Start.
func (s *spinner) Stop(finalMsg string) {
	s.stopOnce.Do(func() {
		close(s.done)
		s.mu.Lock()
		started := s.started
		s.mu.Unlock()
		if started {
			<-s.stopped
		}

		// Clear the spinner line using ANSI escape (terminal-width-independent).
		fmt.Fprintf(s.writer, "\r\033[2K")

		if finalMsg != "" {
			fmt.Fprintln(s.writer, finalMsg)
		}
	})
}
