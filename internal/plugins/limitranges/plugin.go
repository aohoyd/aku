package limitranges

import (
	"context"
	"fmt"
	"slices"

	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/render"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var gvr = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "limitranges"}

// Plugin implements plugin.ResourcePlugin for Kubernetes LimitRanges.
type Plugin struct{}

// New creates a new LimitRange plugin.
func New(_ *k8s.Client, _ *k8s.Store) plugin.ResourcePlugin {
	return &Plugin{}
}

func (p *Plugin) Name() string                     { return "limitranges" }
func (p *Plugin) ShortName() string                { return "limits" }
func (p *Plugin) GVR() schema.GroupVersionResource { return gvr }
func (p *Plugin) IsClusterScoped() bool            { return false }

func (p *Plugin) Columns() []plugin.Column {
	return []plugin.Column{
		{Title: "NAME", Flex: true},
		{Title: "AGE", Width: 8},
	}
}

func (p *Plugin) Row(obj *unstructured.Unstructured) []string {
	name := obj.GetName()
	age := render.FormatAge(obj)
	return []string{name, age}
}

func (p *Plugin) YAML(obj *unstructured.Unstructured) (render.Content, error) {
	return plugin.MarshalYAML(obj)
}

func (p *Plugin) Describe(ctx context.Context, obj *unstructured.Unstructured) (render.Content, error) {
	lr, err := toLimitRange(obj)
	if err != nil {
		return render.Content{}, fmt.Errorf("converting to LimitRange: %w", err)
	}

	b := render.NewBuilder()

	b.KV(render.LEVEL_0, "Name", lr.Name)
	b.KV(render.LEVEL_0, "Namespace", lr.Namespace)

	// Labels
	b.KVMulti(render.LEVEL_0, "Labels", lr.Labels)

	// Annotations
	b.KVMulti(render.LEVEL_0, "Annotations", lr.Annotations)

	// Limits
	for _, item := range lr.Spec.Limits {
		b.Section(render.LEVEL_0, fmt.Sprintf("Type: %s", item.Type))

		if len(item.Default) > 0 {
			b.Section(render.LEVEL_1, "Default")
			renderResourceList(b, render.LEVEL_2, item.Default)
		}

		if len(item.DefaultRequest) > 0 {
			b.Section(render.LEVEL_1, "Default Request")
			renderResourceList(b, render.LEVEL_2, item.DefaultRequest)
		}

		if len(item.Min) > 0 {
			b.Section(render.LEVEL_1, "Min")
			renderResourceList(b, render.LEVEL_2, item.Min)
		}

		if len(item.Max) > 0 {
			b.Section(render.LEVEL_1, "Max")
			renderResourceList(b, render.LEVEL_2, item.Max)
		}

		if len(item.MaxLimitRequestRatio) > 0 {
			b.Section(render.LEVEL_1, "Max Limit/Request Ratio")
			renderResourceList(b, render.LEVEL_2, item.MaxLimitRequestRatio)
		}
	}

	return b.Build(), nil
}

// toLimitRange converts an unstructured object to a typed corev1.LimitRange.
func toLimitRange(obj *unstructured.Unstructured) (*corev1.LimitRange, error) {
	var lr corev1.LimitRange
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &lr); err != nil {
		return nil, err
	}
	return &lr, nil
}

// renderResourceList renders a corev1.ResourceList as sorted key-value pairs.
func renderResourceList(b *render.Builder, level int, rl corev1.ResourceList) {
	names := make([]string, 0, len(rl))
	for name := range rl {
		names = append(names, string(name))
	}
	slices.Sort(names)

	for _, name := range names {
		qty := rl[corev1.ResourceName(name)]
		b.KV(level, name, qty.String())
	}
}
