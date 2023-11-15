package configmaps

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/render"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var gvr = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}

// Plugin implements plugin.ResourcePlugin for Kubernetes ConfigMaps.
type Plugin struct{}

// New creates a new ConfigMap plugin.
func New(_ *k8s.Client, _ *k8s.Store) plugin.ResourcePlugin {
	return &Plugin{}
}

func (p *Plugin) Name() string                     { return "configmaps" }
func (p *Plugin) ShortName() string                { return "cm" }
func (p *Plugin) GVR() schema.GroupVersionResource { return gvr }
func (p *Plugin) IsClusterScoped() bool            { return false }

func (p *Plugin) Columns() []plugin.Column {
	return []plugin.Column{
		{Title: "NAME", Flex: true},
		{Title: "DATA", Width: 8},
		{Title: "AGE", Width: 8},
	}
}

func (p *Plugin) Row(obj *unstructured.Unstructured) []string {
	name := obj.GetName()

	dataCount := 0
	data, found, _ := unstructured.NestedMap(obj.Object, "data")
	if found {
		dataCount = len(data)
	}

	age := render.FormatAge(obj)

	return []string{name, fmt.Sprintf("%d", dataCount), age}
}

func (p *Plugin) YAML(obj *unstructured.Unstructured) (render.Content, error) {
	return plugin.MarshalYAML(obj)
}

func (p *Plugin) Describe(ctx context.Context, obj *unstructured.Unstructured) (render.Content, error) {
	cm, err := toConfigMap(obj)
	if err != nil {
		return render.Content{}, fmt.Errorf("converting to ConfigMap: %w", err)
	}

	b := render.NewBuilder()

	b.KV(render.LEVEL_0, "Name", cm.Name)
	b.KV(render.LEVEL_0, "Namespace", cm.Namespace)
	b.KV(render.LEVEL_0, "Age", render.FormatAge(obj))

	// Labels
	b.KVMulti(render.LEVEL_0, "Labels", cm.Labels)

	// Annotations
	b.KVMulti(render.LEVEL_0, "Annotations", cm.Annotations)

	// Data section
	b.Section(render.LEVEL_0, "Data")
	if len(cm.Data) > 0 {
		keys := slices.Sorted(maps.Keys(cm.Data))
		for _, k := range keys {
			b.Section(render.LEVEL_1, k)
			b.RawLine(render.LEVEL_1, "----")
			b.RawLine(render.LEVEL_1, cm.Data[k])
			b.RawLine(render.LEVEL_1, "")
		}
	}

	// BinaryData section
	b.Section(render.LEVEL_0, "BinaryData")
	if len(cm.BinaryData) > 0 {
		keys := slices.Sorted(maps.Keys(cm.BinaryData))
		for _, k := range keys {
			b.KV(render.LEVEL_1, k, fmt.Sprintf("%d bytes", len(cm.BinaryData[k])))
		}
	}

	return b.Build(), nil
}

// toConfigMap converts an unstructured object to a typed corev1.ConfigMap.
func toConfigMap(obj *unstructured.Unstructured) (*corev1.ConfigMap, error) {
	var cm corev1.ConfigMap
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &cm); err != nil {
		return nil, err
	}
	return &cm, nil
}
