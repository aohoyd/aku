package k8s

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewSpinner(t *testing.T) {
	var buf bytes.Buffer
	s := newSpinner(&buf, "initializing")

	if s.writer != &buf {
		t.Fatal("writer not set")
	}
	if s.status != "initializing" {
		t.Fatalf("expected status 'initializing', got %q", s.status)
	}
	if s.done == nil {
		t.Fatal("done channel is nil")
	}
	if s.stopped == nil {
		t.Fatal("stopped channel is nil")
	}
}

func TestSpinnerSetStatus(t *testing.T) {
	var buf bytes.Buffer
	s := newSpinner(&buf, "step 1")

	s.SetStatus("step 2")
	if s.status != "step 2" {
		t.Fatalf("expected 'step 2', got %q", s.status)
	}

	s.SetStatus("step 3")
	if s.status != "step 3" {
		t.Fatalf("expected 'step 3', got %q", s.status)
	}
}

func TestSpinnerStartAndStop(t *testing.T) {
	var buf bytes.Buffer
	s := newSpinner(&buf, "working...")
	s.Start()

	// Let the spinner render a few frames.
	time.Sleep(250 * time.Millisecond)

	s.Stop("")

	output := buf.String()
	// Verify spinner frames were written.
	foundFrame := false
	for _, frame := range spinnerFrames {
		if strings.Contains(output, frame) {
			foundFrame = true
			break
		}
	}
	if !foundFrame {
		t.Fatal("expected at least one spinner frame in output")
	}

	// Verify the status text was written.
	if !strings.Contains(output, "working...") {
		t.Fatal("expected status text in output")
	}
}

func TestSpinnerStopTerminatesGoroutine(t *testing.T) {
	var buf bytes.Buffer
	s := newSpinner(&buf, "running")
	s.Start()

	// Stop should return (not hang), proving the goroutine exited.
	done := make(chan struct{})
	go func() {
		s.Stop("")
		close(done)
	}()

	select {
	case <-done:
		// Success — goroutine terminated.
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not return in time — goroutine may be stuck")
	}
}

func TestSpinnerStopWithFinalMsg(t *testing.T) {
	var buf bytes.Buffer
	s := newSpinner(&buf, "loading")
	s.Start()

	time.Sleep(100 * time.Millisecond)
	s.Stop("Done!")

	output := buf.String()
	if !strings.HasSuffix(output, "Done!\n") {
		t.Fatalf("expected output to end with 'Done!\\n', got %q", output)
	}
}

func TestSpinnerStopEmptyFinalMsg(t *testing.T) {
	var buf bytes.Buffer
	s := newSpinner(&buf, "loading")
	s.Start()

	time.Sleep(100 * time.Millisecond)
	s.Stop("")

	output := buf.String()
	// After stop with empty message, the last char should be \r from the line clear,
	// not a newline from fmt.Fprintln.
	if strings.HasSuffix(output, "\n") {
		t.Fatal("expected no trailing newline when finalMsg is empty")
	}
}

func TestSpinnerConcurrentSetStatus(t *testing.T) {
	var buf bytes.Buffer
	s := newSpinner(&buf, "init")
	s.Start()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			s.SetStatus("status update")
		}(i)
	}
	wg.Wait()

	s.Stop("")
	// If we get here without a race detector complaint, the test passes.
}

func TestSpinnerDoubleStart(t *testing.T) {
	var buf bytes.Buffer
	s := newSpinner(&buf, "test")
	s.Start()
	s.Start() // must not panic or spawn second goroutine
	time.Sleep(100 * time.Millisecond)
	s.Stop("")
}

func TestSpinnerDoubleStop(t *testing.T) {
	var buf bytes.Buffer
	s := newSpinner(&buf, "test")
	s.Start()
	time.Sleep(100 * time.Millisecond)
	s.Stop("done")
	s.Stop("done again") // must not panic
}

func TestSpinnerStopWithoutStart(t *testing.T) {
	var buf bytes.Buffer
	s := newSpinner(&buf, "test")
	done := make(chan struct{})
	go func() {
		s.Stop("")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop without Start deadlocked")
	}
}

func TestSpinnerStatusReflectsInOutput(t *testing.T) {
	var buf bytes.Buffer
	s := newSpinner(&buf, "phase 1")
	s.Start()

	time.Sleep(200 * time.Millisecond)
	s.SetStatus("phase 2")
	time.Sleep(200 * time.Millisecond)

	s.Stop("")

	output := buf.String()
	if !strings.Contains(output, "phase 1") {
		t.Fatal("expected 'phase 1' in output")
	}
	if !strings.Contains(output, "phase 2") {
		t.Fatal("expected 'phase 2' in output")
	}
}
