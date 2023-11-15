package ui

import (
	"cmp"
	"slices"
	"strings"

	"github.com/aohoyd/aku/internal/plugin"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// SortState holds the active sort column and direction for one ResourceList.
type SortState struct {
	Column    string // matches a Column.Title, e.g. "NAME", "AGE", "STATUS"
	Ascending bool
}

// DefaultSortState returns name ascending — the default for all new lists.
func DefaultSortState() SortState {
	return SortState{Column: "NAME", Ascending: true}
}

// Toggle returns a new SortState. Same column flips direction; new column
// resets to ascending.
func (s SortState) Toggle(column string) SortState {
	if s.Column == column {
		return SortState{Column: column, Ascending: !s.Ascending}
	}
	return SortState{Column: column, Ascending: true}
}

// Indicator returns "▲" or "▼" for the active column, "" for all others.
func (s SortState) Indicator(column string) string {
	if s.Column != column {
		return ""
	}
	if s.Ascending {
		return "▲"
	}
	return "▼"
}

// SortStateForPlugin returns the plugin's preferred default sort if it
// implements DefaultSorter, otherwise NAME ascending.
func SortStateForPlugin(p plugin.ResourcePlugin) SortState {
	if ds, ok := p.(plugin.DefaultSorter); ok {
		pref := ds.DefaultSort()
		return SortState{Column: pref.Column, Ascending: pref.Ascending}
	}
	return DefaultSortState()
}

// sortObjects sorts objs in-place using state and the plugin's optional
// Sortable implementation. Uses stable sort to preserve informer order for
// equal-key objects.
func sortObjects(objs []*unstructured.Unstructured, state SortState, p plugin.ResourcePlugin) {
	sortable, hasSortable := p.(plugin.Sortable)
	column := strings.ToUpper(state.Column)

	slices.SortStableFunc(objs, func(a, b *unstructured.Unstructured) int {
		va := sortValueFor(a, column, sortable, hasSortable)
		vb := sortValueFor(b, column, sortable, hasSortable)
		c := cmp.Compare(va, vb)
		if !state.Ascending {
			c = -c
		}
		if c != 0 {
			return c
		}
		return cmp.Compare(a.GetName(), b.GetName())
	})
}

// sortValueFor returns the comparison key for an object on the given column.
// Priority: plugin Sortable → built-in NAME/AGE → "" (preserves order).
// column must already be upper-cased by the caller.
func sortValueFor(
	obj *unstructured.Unstructured,
	column string,
	sortable plugin.Sortable,
	hasSortable bool,
) string {
	if hasSortable {
		if v := sortable.SortValue(obj, column); v != "" {
			return v
		}
	}

	switch column {
	case "NAME":
		return obj.GetName()
	case "AGE":
		ts := obj.GetCreationTimestamp()
		if ts.IsZero() {
			return ""
		}
		return ts.UTC().Format("2006-01-02T15:04:05Z")
	case "NAMESPACE":
		return obj.GetNamespace()
	}

	return ""
}
