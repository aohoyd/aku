package events

import (
	"strings"
	"testing"

	"github.com/aohoyd/aku/internal/plugin"
	"github.com/charmbracelet/x/ansi"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestEventsPluginColumns(t *testing.T) {
	p := New(nil, nil)
	cols := p.Columns()
	if len(cols) != 6 {
		t.Fatalf("expected 6 columns, got %d", len(cols))
	}
	expected := []string{"LAST SEEN", "TYPE", "REASON", "OBJECT", "MESSAGE", "AGE"}
	for i, want := range expected {
		if cols[i].Title != want {
			t.Errorf("column %d: expected %q, got %q", i, want, cols[i].Title)
		}
	}
}

func TestEventsPluginRow(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Event",
			"metadata": map[string]any{
				"name":              "my-pod.abc123",
				"namespace":         "default",
				"creationTimestamp": "2024-01-01T00:00:00Z",
			},
			"lastTimestamp": "2024-01-01T00:00:00Z",
			"type":          "Normal",
			"reason":        "Scheduled",
			"message":       "Successfully assigned default/my-pod to node-1",
			"involvedObject": map[string]any{
				"kind":      "Pod",
				"name":      "my-pod",
				"namespace": "default",
			},
		},
	}
	row := p.Row(obj)
	if len(row) != 6 {
		t.Fatalf("expected 6 row values, got %d", len(row))
	}
	// LAST SEEN should be non-empty
	if row[0] == "" || row[0] == "<unknown>" {
		t.Errorf("expected non-empty LAST SEEN, got %q", row[0])
	}
	// TYPE
	if row[1] != "Normal" {
		t.Errorf("expected TYPE 'Normal', got %q", row[1])
	}
	// REASON
	if row[2] != "Scheduled" {
		t.Errorf("expected REASON 'Scheduled', got %q", row[2])
	}
	// OBJECT
	if row[3] != "Pod/my-pod" {
		t.Errorf("expected OBJECT 'Pod/my-pod', got %q", row[3])
	}
	// MESSAGE
	if row[4] != "Successfully assigned default/my-pod to node-1" {
		t.Errorf("expected MESSAGE, got %q", row[4])
	}
}

func TestEventsPluginSortable(t *testing.T) {
	p := New(nil, nil)
	sortable, ok := p.(plugin.Sortable)
	if !ok {
		t.Fatal("Plugin should implement plugin.Sortable")
	}

	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"lastTimestamp": "2024-01-01T00:00:00Z",
			"type":          "Warning",
		},
	}

	// LAST SEEN should return the raw timestamp string
	v := sortable.SortValue(obj, "LAST SEEN")
	if v == "" {
		t.Error("SortValue for LAST SEEN should return non-empty string")
	}
	if v != "2024-01-01T00:00:00Z" {
		t.Errorf("SortValue for LAST SEEN: expected '2024-01-01T00:00:00Z', got %q", v)
	}

	// TYPE should return raw type string
	v = sortable.SortValue(obj, "TYPE")
	if v != "Warning" {
		t.Errorf("SortValue for TYPE: expected 'Warning', got %q", v)
	}

	// Unknown column should return ""
	v = sortable.SortValue(obj, "UNKNOWN")
	if v != "" {
		t.Errorf("SortValue for UNKNOWN: expected '', got %q", v)
	}
}

func TestDefaultSort(t *testing.T) {
	p := &Plugin{}
	pref := p.DefaultSort()
	if pref.Column != "LAST SEEN" {
		t.Fatalf("expected column 'LAST SEEN', got %q", pref.Column)
	}
	if pref.Ascending {
		t.Fatal("expected descending sort")
	}
}

func TestEventsPluginDescribe(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Event",
			"metadata": map[string]any{
				"name":              "my-pod.abc123",
				"namespace":         "default",
				"creationTimestamp": "2026-02-24T10:00:00Z",
				"labels":            map[string]any{"app": "web"},
			},
			"type":           "Warning",
			"reason":         "BackOff",
			"message":        "Back-off restarting failed container",
			"lastTimestamp":  "2026-02-24T10:05:00Z",
			"firstTimestamp": "2026-02-24T10:00:00Z",
			"count":          int64(5),
			"source": map[string]any{
				"component": "kubelet",
				"host":      "node-1",
			},
			"involvedObject": map[string]any{
				"kind":      "Pod",
				"name":      "my-pod",
				"namespace": "default",
			},
		},
	}

	c, err := p.Describe(t.Context(), obj)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Display == "" {
		t.Fatal("display output should not be empty")
	}
	if stripped := ansi.Strip(c.Display); stripped != c.Raw {
		t.Errorf("strip invariant violated: ansi.Strip(c.Display) != raw\nstripped: %q\nraw:      %q", stripped, c.Raw)
	}

	checks := []string{
		"my-pod.abc123",
		"default",
		"Warning",
		"BackOff",
		"Back-off restarting failed container",
		"kubelet",
		"node-1",
		"Pod",
		"my-pod",
		"5",
		"app=web",
	}
	for _, want := range checks {
		if !strings.Contains(c.Raw, want) {
			t.Errorf("describe output should contain %q\n\nFull output:\n%s", want, c.Raw)
		}
	}
}
