package events

import (
	"context"
	"fmt"
	"time"

	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/render"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var gvr = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "events"}

// Plugin implements plugin.ResourcePlugin for Kubernetes Events.
type Plugin struct{}

// New creates a new Events plugin.
func New(_ *k8s.Client, _ *k8s.Store) plugin.ResourcePlugin {
	return &Plugin{}
}

func (p *Plugin) Name() string                     { return "events" }
func (p *Plugin) ShortName() string                { return "ev" }
func (p *Plugin) GVR() schema.GroupVersionResource { return gvr }
func (p *Plugin) IsClusterScoped() bool            { return false }

func (p *Plugin) Columns() []plugin.Column {
	return []plugin.Column{
		{Title: "LAST SEEN", Width: 10},
		{Title: "TYPE", Width: 10},
		{Title: "REASON", Width: 20},
		{Title: "OBJECT", Flex: true},
		{Title: "MESSAGE", Flex: true},
		{Title: "AGE", Width: 8},
	}
}

func (p *Plugin) Row(obj *unstructured.Unstructured) []string {
	lastSeen := formatLastSeen(obj)
	eventType, _, _ := unstructured.NestedString(obj.Object, "type")
	reason, _, _ := unstructured.NestedString(obj.Object, "reason")
	object := formatObject(obj)
	message, _, _ := unstructured.NestedString(obj.Object, "message")
	age := render.FormatAge(obj)

	return []string{lastSeen, eventType, reason, object, message, age}
}

func (p *Plugin) YAML(obj *unstructured.Unstructured) (render.Content, error) {
	return plugin.MarshalYAML(obj)
}

func (p *Plugin) Describe(ctx context.Context, obj *unstructured.Unstructured) (render.Content, error) {
	ev, err := toEvent(obj)
	if err != nil {
		return render.Content{}, fmt.Errorf("converting to Event: %w", err)
	}

	b := render.NewBuilder()

	b.KV(render.LEVEL_0, "Name", ev.Name)
	b.KV(render.LEVEL_0, "Namespace", ev.Namespace)

	// Labels
	b.KVMulti(render.LEVEL_0, "Labels", ev.Labels)

	// Annotations
	b.KVMulti(render.LEVEL_0, "Annotations", ev.Annotations)

	// Event fields
	b.KV(render.LEVEL_0, "Type", ev.Type)
	b.KV(render.LEVEL_0, "Reason", ev.Reason)
	b.KV(render.LEVEL_0, "Message", ev.Message)

	// Source
	if ev.Source.Component != "" || ev.Source.Host != "" {
		src := ev.Source.Component
		if ev.Source.Host != "" {
			if src != "" {
				src += ", "
			}
			src += ev.Source.Host
		}
		b.KV(render.LEVEL_0, "Source", src)
	}

	// InvolvedObject
	b.Section(render.LEVEL_0, "InvolvedObject")
	b.KV(render.LEVEL_1, "Kind", ev.InvolvedObject.Kind)
	b.KV(render.LEVEL_1, "Name", ev.InvolvedObject.Name)
	b.KV(render.LEVEL_1, "Namespace", ev.InvolvedObject.Namespace)
	if ev.InvolvedObject.FieldPath != "" {
		b.KV(render.LEVEL_1, "FieldPath", ev.InvolvedObject.FieldPath)
	}

	// Timestamps
	if !ev.FirstTimestamp.IsZero() {
		b.KV(render.LEVEL_0, "First Timestamp", ev.FirstTimestamp.Format(time.RFC1123Z))
	}
	if !ev.LastTimestamp.IsZero() {
		b.KV(render.LEVEL_0, "Last Timestamp", ev.LastTimestamp.Format(time.RFC1123Z))
	}

	// Count
	b.KV(render.LEVEL_0, "Count", fmt.Sprintf("%d", ev.Count))

	return b.Build(), nil
}

// DefaultSort implements plugin.DefaultSorter.
func (p *Plugin) DefaultSort() plugin.SortPreference {
	return plugin.SortPreference{Column: "LAST SEEN", Ascending: false}
}

// SortValue implements plugin.Sortable.
func (p *Plugin) SortValue(obj *unstructured.Unstructured, column string) string {
	switch column {
	case "LAST SEEN":
		return extractTimestamp(obj)
	case "TYPE":
		v, _, _ := unstructured.NestedString(obj.Object, "type")
		return v
	default:
		return ""
	}
}

// toEvent converts an unstructured object to a typed corev1.Event.
func toEvent(obj *unstructured.Unstructured) (*corev1.Event, error) {
	var ev corev1.Event
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &ev); err != nil {
		return nil, err
	}
	return &ev, nil
}

// extractTimestamp returns the raw ISO string from lastTimestamp or eventTime.
func extractTimestamp(obj *unstructured.Unstructured) string {
	ts, found, _ := unstructured.NestedString(obj.Object, "lastTimestamp")
	if found && ts != "" {
		return ts
	}
	ts, _, _ = unstructured.NestedString(obj.Object, "eventTime")
	return ts
}

// formatLastSeen returns a human-readable duration since lastTimestamp or eventTime.
func formatLastSeen(obj *unstructured.Unstructured) string {
	ts := extractTimestamp(obj)
	if ts == "" {
		return "<unknown>"
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return "<unknown>"
	}
	return render.FormatDuration(time.Since(t))
}

// formatObject returns "Kind/Name" from involvedObject.
func formatObject(obj *unstructured.Unstructured) string {
	kind, _, _ := unstructured.NestedString(obj.Object, "involvedObject", "kind")
	name, _, _ := unstructured.NestedString(obj.Object, "involvedObject", "name")
	if kind == "" && name == "" {
		return ""
	}
	return kind + "/" + name
}
