package secrets

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

var gvr = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "secrets"}

// Plugin implements plugin.ResourcePlugin and plugin.Uncoverable for Kubernetes Secrets.
type Plugin struct{}

// New creates a new Secrets plugin.
func New(_ *k8s.Client, _ *k8s.Store) plugin.ResourcePlugin {
	return &Plugin{}
}

func (p *Plugin) Name() string                     { return "secrets" }
func (p *Plugin) ShortName() string                { return "sec" }
func (p *Plugin) GVR() schema.GroupVersionResource { return gvr }
func (p *Plugin) IsClusterScoped() bool            { return false }

func (p *Plugin) Columns() []plugin.Column {
	return []plugin.Column{
		{Title: "NAME", Flex: true},
		{Title: "TYPE", Width: 30},
		{Title: "DATA", Width: 8},
		{Title: "AGE", Width: 8},
	}
}

func (p *Plugin) Row(obj *unstructured.Unstructured) []string {
	name := obj.GetName()

	secretType, _, _ := unstructured.NestedString(obj.Object, "type")

	dataCount := 0
	data, found, _ := unstructured.NestedMap(obj.Object, "data")
	if found {
		dataCount = len(data)
	}

	age := render.FormatAge(obj)

	return []string{name, secretType, fmt.Sprintf("%d", dataCount), age}
}

func (p *Plugin) YAML(obj *unstructured.Unstructured) (render.Content, error) {
	return plugin.MarshalYAML(obj)
}

func (p *Plugin) Describe(ctx context.Context, obj *unstructured.Unstructured) (render.Content, error) {
	return p.renderDescribe(obj, false)
}

// DescribeUncovered implements plugin.Uncoverable.
func (p *Plugin) DescribeUncovered(ctx context.Context, obj *unstructured.Unstructured) (render.Content, error) {
	return p.renderDescribe(obj, true)
}

func (p *Plugin) renderDescribe(obj *unstructured.Unstructured, uncovered bool) (render.Content, error) {
	secret, err := toSecret(obj)
	if err != nil {
		return render.Content{}, fmt.Errorf("converting to Secret: %w", err)
	}

	b := render.NewBuilder()

	b.KV(render.LEVEL_0, "Name", secret.Name)
	b.KV(render.LEVEL_0, "Namespace", secret.Namespace)

	// Labels
	b.KVMulti(render.LEVEL_0, "Labels", secret.Labels)

	// Annotations
	b.KVMulti(render.LEVEL_0, "Annotations", secret.Annotations)

	// Type
	b.KV(render.LEVEL_0, "Type", string(secret.Type))

	// Data section
	b.Section(render.LEVEL_0, "Data")
	if len(secret.Data) > 0 {
		keys := slices.Sorted(maps.Keys(secret.Data))
		for _, k := range keys {
			if uncovered {
				// Data values are already base64-decoded by FromUnstructured
				b.KV(render.LEVEL_1, k, string(secret.Data[k]))
			} else {
				b.KV(render.LEVEL_1, k, fmt.Sprintf("%d bytes", len(secret.Data[k])))
			}
		}
	}

	return b.Build(), nil
}

// toSecret converts an unstructured object to a typed corev1.Secret.
func toSecret(obj *unstructured.Unstructured) (*corev1.Secret, error) {
	var secret corev1.Secret
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &secret); err != nil {
		return nil, err
	}
	return &secret, nil
}
