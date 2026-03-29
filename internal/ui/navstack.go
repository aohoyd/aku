package ui

import (
	"github.com/aohoyd/aku/internal/plugin"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// NavSnapshot captures the full restorable state of a ResourceList pane.
type NavSnapshot struct {
	Plugin           plugin.ResourcePlugin
	Namespace        string
	Objects          []*unstructured.Unstructured
	Cursor           int
	SortState        SortState
	FilterState      SearchState
	SearchState      SearchState
	ParentUID        string // UID of the parent resource that produced this drilled view
	ParentName       string // Name of the parent resource for breadcrumb display
	ParentAPIVersion string // API version of the parent resource for kind disambiguation
	ParentKind       string // Kind of the parent resource for kind disambiguation
}

// NavStack is a simple LIFO stack for drill-down navigation.
type NavStack struct {
	frames []NavSnapshot
}

// Push saves a snapshot onto the stack.
func (s *NavStack) Push(snap NavSnapshot) {
	s.frames = append(s.frames, snap)
}

// Pop removes and returns the top snapshot. Returns false if empty.
func (s *NavStack) Pop() (NavSnapshot, bool) {
	if len(s.frames) == 0 {
		return NavSnapshot{}, false
	}
	snap := s.frames[len(s.frames)-1]
	s.frames = s.frames[:len(s.frames)-1]
	return snap, true
}

// Peek returns the top snapshot without removing it. Returns false if empty.
func (s *NavStack) Peek() (NavSnapshot, bool) {
	if len(s.frames) == 0 {
		return NavSnapshot{}, false
	}
	return s.frames[len(s.frames)-1], true
}

// Depth returns the number of snapshots on the stack.
func (s *NavStack) Depth() int {
	return len(s.frames)
}

// HasGVR reports whether any snapshot on the stack references the given GVR and namespace.
func (s *NavStack) HasGVR(gvr schema.GroupVersionResource, namespace string) bool {
	for _, f := range s.frames {
		effectiveNs := f.Namespace
		if f.Plugin.IsClusterScoped() {
			effectiveNs = ""
		}
		if f.Plugin.GVR() == gvr && effectiveNs == namespace {
			return true
		}
	}
	return false
}
