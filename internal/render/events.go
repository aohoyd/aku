package render

import (
	"fmt"
	"slices"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// RenderEvents filters events matching the given resource, sorts newest-first,
// and returns a kubectl-style "Events:" table section as Content.
func RenderEvents(allEvents []*unstructured.Unstructured, kind, name, namespace string) Content {
	var matched []*unstructured.Unstructured
	for _, ev := range allEvents {
		ek, _, _ := unstructured.NestedString(ev.Object, "involvedObject", "kind")
		en, _, _ := unstructured.NestedString(ev.Object, "involvedObject", "name")
		ens, _, _ := unstructured.NestedString(ev.Object, "involvedObject", "namespace")
		if ek == kind && en == name && ens == namespace {
			matched = append(matched, ev)
		}
	}

	// Sort newest first by lastTimestamp (RFC3339 strings sort lexicographically)
	slices.SortFunc(matched, func(a, b *unstructured.Unstructured) int {
		ta := eventTimestamp(a)
		tb := eventTimestamp(b)
		if ta > tb {
			return -1
		}
		if ta < tb {
			return 1
		}
		return 0
	})

	b := NewBuilder()

	if len(matched) == 0 {
		b.KV(LEVEL_0, "Events", "<none>")
		return b.Build()
	}

	// Extract row data so we can compute column widths.
	type row struct{ typ, reason, age, from, message string }
	rows := make([]row, len(matched))
	for i, ev := range matched {
		rows[i].typ, _, _ = unstructured.NestedString(ev.Object, "type")
		rows[i].reason, _, _ = unstructured.NestedString(ev.Object, "reason")
		rows[i].message, _, _ = unstructured.NestedString(ev.Object, "message")
		rows[i].from, _, _ = unstructured.NestedString(ev.Object, "source", "component")
		rows[i].age = eventAge(ev)
	}

	// Compute max width per column, starting with header label lengths.
	wType, wReason, wAge, wFrom := len("Type"), len("Reason"), len("Age"), len("From")
	for _, r := range rows {
		wType = max(wType, len(r.typ))
		wReason = max(wReason, len(r.reason))
		wAge = max(wAge, len(r.age))
		wFrom = max(wFrom, len(r.from))
	}

	fmtRow := fmt.Sprintf("%%-%ds  %%-%ds  %%-%ds  %%-%ds  %%s", wType, wReason, wAge, wFrom)

	b.Section(LEVEL_0, "Events")
	b.RawLine(LEVEL_1, fmt.Sprintf(fmtRow, "Type", "Reason", "Age", "From", "Message"))
	b.RawLine(LEVEL_1, fmt.Sprintf(fmtRow, "----", "------", "---", "----", "-------"))

	for _, r := range rows {
		b.RawLine(LEVEL_1, fmt.Sprintf(fmtRow, r.typ, r.reason, r.age, r.from, r.message))
	}

	return b.Build()
}

// eventTimestamp extracts lastTimestamp or eventTime from an event object.
func eventTimestamp(obj *unstructured.Unstructured) string {
	ts, found, _ := unstructured.NestedString(obj.Object, "lastTimestamp")
	if found && ts != "" {
		return ts
	}
	ts, _, _ = unstructured.NestedString(obj.Object, "eventTime")
	return ts
}

// eventAge returns a human-readable duration since the event's timestamp.
func eventAge(obj *unstructured.Unstructured) string {
	ts := eventTimestamp(obj)
	if ts == "" {
		return "<unknown>"
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return "<unknown>"
	}
	return FormatDuration(time.Since(t))
}
