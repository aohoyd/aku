package ui

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestDefaultSortState(t *testing.T) {
	s := DefaultSortState()
	if s.Column != "NAME" {
		t.Fatalf("expected Column 'NAME', got %q", s.Column)
	}
	if !s.Ascending {
		t.Fatal("expected Ascending true")
	}
}

func TestSortStateToggleSameColumn(t *testing.T) {
	s := SortState{Column: "NAME", Ascending: true}
	s = s.Toggle("NAME")
	if s.Ascending {
		t.Fatal("toggle same column should flip to descending")
	}
	s = s.Toggle("NAME")
	if !s.Ascending {
		t.Fatal("toggle again should flip back to ascending")
	}
}

func TestSortStateToggleDifferentColumn(t *testing.T) {
	s := SortState{Column: "NAME", Ascending: false}
	s = s.Toggle("AGE")
	if s.Column != "AGE" {
		t.Fatalf("expected Column 'AGE', got %q", s.Column)
	}
	if !s.Ascending {
		t.Fatal("new column should default to ascending")
	}
}

func TestSortStateIndicator(t *testing.T) {
	s := SortState{Column: "NAME", Ascending: true}
	if ind := s.Indicator("NAME"); ind != "▲" {
		t.Fatalf("expected '▲', got %q", ind)
	}
	s.Ascending = false
	if ind := s.Indicator("NAME"); ind != "▼" {
		t.Fatalf("expected '▼', got %q", ind)
	}
	if ind := s.Indicator("AGE"); ind != "" {
		t.Fatalf("expected empty for non-active column, got %q", ind)
	}
}

func TestSortObjectsByName(t *testing.T) {
	objs := []*unstructured.Unstructured{
		makeObj("charlie"),
		makeObj("alpha"),
		makeObj("bravo"),
	}
	state := SortState{Column: "NAME", Ascending: true}
	sortObjects(objs, state, &testPlugin{})

	expected := []string{"alpha", "bravo", "charlie"}
	for i, obj := range objs {
		if obj.GetName() != expected[i] {
			t.Fatalf("index %d: expected %q, got %q", i, expected[i], obj.GetName())
		}
	}
}

func TestSortObjectsByNameDescending(t *testing.T) {
	objs := []*unstructured.Unstructured{
		makeObj("alpha"),
		makeObj("charlie"),
		makeObj("bravo"),
	}
	state := SortState{Column: "NAME", Ascending: false}
	sortObjects(objs, state, &testPlugin{})

	expected := []string{"charlie", "bravo", "alpha"}
	for i, obj := range objs {
		if obj.GetName() != expected[i] {
			t.Fatalf("index %d: expected %q, got %q", i, expected[i], obj.GetName())
		}
	}
}

func TestSortObjectsByAge(t *testing.T) {
	now := time.Now()
	objs := []*unstructured.Unstructured{
		makeObjWithTime("new-pod", now.Add(-1*time.Minute)),
		makeObjWithTime("old-pod", now.Add(-24*time.Hour)),
		makeObjWithTime("mid-pod", now.Add(-1*time.Hour)),
	}
	state := SortState{Column: "AGE", Ascending: true}
	sortObjects(objs, state, &testPlugin{})

	// Ascending by creation time = oldest first (smallest timestamp)
	expected := []string{"old-pod", "mid-pod", "new-pod"}
	for i, obj := range objs {
		if obj.GetName() != expected[i] {
			t.Fatalf("index %d: expected %q, got %q", i, expected[i], obj.GetName())
		}
	}
}

func TestSortObjectsUnsupportedColumn(t *testing.T) {
	objs := []*unstructured.Unstructured{
		makeObj("bravo"),
		makeObj("alpha"),
	}
	state := SortState{Column: "UNKNOWN", Ascending: true}
	sortObjects(objs, state, &testPlugin{})

	// Unsupported column: primary keys are all equal, secondary sort by name
	if objs[0].GetName() != "alpha" || objs[1].GetName() != "bravo" {
		t.Fatal("unsupported column should fall back to name sort")
	}
}

func TestSortObjectsSecondarySortByName(t *testing.T) {
	now := time.Now()
	// Three pods with the same creation time — should be stabilized by name
	objs := []*unstructured.Unstructured{
		makeObjWithTime("charlie", now),
		makeObjWithTime("alpha", now),
		makeObjWithTime("bravo", now),
	}
	state := SortState{Column: "AGE", Ascending: true}
	sortObjects(objs, state, &testPlugin{})

	expected := []string{"alpha", "bravo", "charlie"}
	for i, obj := range objs {
		if obj.GetName() != expected[i] {
			t.Fatalf("index %d: expected %q, got %q", i, expected[i], obj.GetName())
		}
	}
}

func makeObjWithTime(name string, created time.Time) *unstructured.Unstructured {
	obj := makeObj(name)
	obj.SetCreationTimestamp(metav1.NewTime(created))
	return obj
}
