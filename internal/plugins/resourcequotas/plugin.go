package resourcequotas

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

var gvr = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "resourcequotas"}

// Plugin implements plugin.ResourcePlugin for Kubernetes ResourceQuotas.
type Plugin struct{}

// New creates a new ResourceQuota plugin.
func New(_ *k8s.Client, _ *k8s.Store) plugin.ResourcePlugin {
	return &Plugin{}
}

func (p *Plugin) Name() string                     { return "resourcequotas" }
func (p *Plugin) ShortName() string                { return "quota" }
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

func (p *Plugin) Describe(_ context.Context, obj *unstructured.Unstructured) (render.Content, error) {
	rq, err := toResourceQuota(obj)
	if err != nil {
		return render.Content{}, fmt.Errorf("converting to ResourceQuota: %w", err)
	}

	b := render.NewBuilder()

	b.KV(render.LEVEL_0, "Name", rq.Name)
	b.KV(render.LEVEL_0, "Namespace", rq.Namespace)
	b.KV(render.LEVEL_0, "Age", render.FormatAge(obj))

	// Labels
	b.KVMulti(render.LEVEL_0, "Labels", rq.Labels)

	// Annotations
	b.KVMulti(render.LEVEL_0, "Annotations", rq.Annotations)

	// Resource section: hard vs used
	b.Section(render.LEVEL_0, "Resource")
	hard := rq.Status.Hard
	used := rq.Status.Used
	if len(hard) > 0 {
		keys := sortedResourceNames(hard)
		for _, k := range keys {
			hardVal := hard[corev1.ResourceName(k)]
			usedVal := used[corev1.ResourceName(k)]
			b.KV(render.LEVEL_1, string(k), fmt.Sprintf("%s / %s", usedVal.String(), hardVal.String()))
		}
	}

	return b.Build(), nil
}

// toResourceQuota converts an unstructured object to a typed corev1.ResourceQuota.
func toResourceQuota(obj *unstructured.Unstructured) (*corev1.ResourceQuota, error) {
	var rq corev1.ResourceQuota
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &rq); err != nil {
		return nil, err
	}
	return &rq, nil
}

func sortedResourceNames(m corev1.ResourceList) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, string(k))
	}
	slices.Sort(keys)
	return keys
}
