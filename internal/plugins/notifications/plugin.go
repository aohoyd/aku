// Package notifications implements the aku-messages synthetic resource, a
// table view over aku's own info/warning/error messages stored in
// internal/notify.Store. It mirrors the events plugin's timestamp handling and
// the portforwards plugin's self-populating synthetic structure.
package notifications

import (
	"context"
	"strconv"
	"time"

	"github.com/aohoyd/aku/internal/notify"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/render"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	_ plugin.ResourcePlugin = (*Plugin)(nil)
	_ plugin.SelfPopulating = (*Plugin)(nil)
	_ plugin.Sortable       = (*Plugin)(nil)
	_ plugin.DefaultSorter  = (*Plugin)(nil)
)

var syntheticGVR = schema.GroupVersionResource{
	Group: "_ktui", Version: "v1", Resource: "aku-messages",
}

// Plugin displays aku's own messages in a table view.
type Plugin struct {
	store *notify.Store
}

// New creates a new aku-messages plugin backed by the given message store.
func New(store *notify.Store) *Plugin {
	return &Plugin{store: store}
}

func (p *Plugin) Name() string                     { return "aku-messages" }
func (p *Plugin) ShortName() string                { return "msg" }
func (p *Plugin) GVR() schema.GroupVersionResource { return syntheticGVR }
func (p *Plugin) IsClusterScoped() bool            { return true }

func (p *Plugin) Columns() []plugin.Column {
	return []plugin.Column{
		{Title: "TIME", Width: 10},
		{Title: "LEVEL", Width: 10},
		{Title: "CONTEXT", Width: 20},
		{Title: "SOURCE", Width: 20},
		{Title: "MESSAGE", Flex: true},
	}
}

func (p *Plugin) Row(obj *unstructured.Unstructured) []string {
	level, _, _ := unstructured.NestedString(obj.Object, "level")
	ctx, _, _ := unstructured.NestedString(obj.Object, "context")
	source, _, _ := unstructured.NestedString(obj.Object, "source")
	text, _, _ := unstructured.NestedString(obj.Object, "text")
	return []string{formatTime(obj), level, ctx, source, text}
}

// Objects implements plugin.SelfPopulating. It builds one synthetic
// unstructured object per stored message, newest-first (the order returned by
// store.List()).
func (p *Plugin) Objects() []*unstructured.Unstructured {
	if p.store == nil {
		return nil
	}
	messages := p.store.List()
	objs := make([]*unstructured.Unstructured, len(messages))
	for i, m := range messages {
		objs[i] = &unstructured.Unstructured{
			Object: map[string]any{
				"metadata": map[string]any{
					"name":              strconv.FormatUint(m.ID, 10),
					"creationTimestamp": nil,
				},
				"time":    m.Time.Format(time.RFC3339),
				"level":   m.Level.String(),
				"context": m.Context,
				"source":  m.Source,
				"text":    m.Text,
			},
		}
	}
	return objs
}

func (p *Plugin) YAML(obj *unstructured.Unstructured) (render.Content, error) {
	return plugin.MarshalYAML(obj)
}

func (p *Plugin) Describe(_ context.Context, obj *unstructured.Unstructured) (render.Content, error) {
	level, _, _ := unstructured.NestedString(obj.Object, "level")
	ctx, _, _ := unstructured.NestedString(obj.Object, "context")
	source, _, _ := unstructured.NestedString(obj.Object, "source")
	text, _, _ := unstructured.NestedString(obj.Object, "text")

	b := render.NewBuilder()
	b.KV(render.LEVEL_0, "Message", text)
	b.KV(render.LEVEL_0, "Level", level)
	b.KV(render.LEVEL_0, "Time", exactTime(obj))
	b.KV(render.LEVEL_0, "Context", ctx)
	b.KV(render.LEVEL_0, "Source", source)
	return b.Build(), nil
}

// DefaultSort implements plugin.DefaultSorter: newest messages first.
func (p *Plugin) DefaultSort() plugin.SortPreference {
	return plugin.SortPreference{Column: "TIME", Ascending: false}
}

// SortValue implements plugin.Sortable. For the TIME column it returns the raw
// RFC3339 string so lexicographic comparison matches chronological order. For
// the other columns it returns the displayed value so they sort
// lexicographically; returning "" would make those columns fall through to the
// generic NAME/AGE handling in internal/ui/sort.go, which (since the synthetic
// objects carry no AGE and a numeric NAME) would silently sort by message ID
// instead of the requested column.
func (p *Plugin) SortValue(obj *unstructured.Unstructured, column string) string {
	switch column {
	case "TIME":
		return rawTime(obj)
	case "LEVEL":
		v, _, _ := unstructured.NestedString(obj.Object, "level")
		return v
	case "CONTEXT":
		v, _, _ := unstructured.NestedString(obj.Object, "context")
		return v
	case "SOURCE":
		v, _, _ := unstructured.NestedString(obj.Object, "source")
		return v
	case "MESSAGE":
		v, _, _ := unstructured.NestedString(obj.Object, "text")
		return v
	default:
		return ""
	}
}

// rawTime returns the stored RFC3339 timestamp string, or "" if absent.
func rawTime(obj *unstructured.Unstructured) string {
	ts, _, _ := unstructured.NestedString(obj.Object, "time")
	return ts
}

// formatTime renders the stored timestamp as a relative age, mirroring how the
// events plugin displays its "LAST SEEN" column.
func formatTime(obj *unstructured.Unstructured) string {
	ts := rawTime(obj)
	if ts == "" {
		return "<unknown>"
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return "<unknown>"
	}
	return render.FormatDuration(time.Since(t))
}

// exactTime renders the stored timestamp in a human-readable absolute form for
// the describe panel.
func exactTime(obj *unstructured.Unstructured) string {
	ts := rawTime(obj)
	if ts == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ts
	}
	return t.Format(time.RFC1123Z)
}
