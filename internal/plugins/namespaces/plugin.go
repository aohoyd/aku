package namespaces

import (
	"context"
	"fmt"

	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/render"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var gvr = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}

// Plugin implements plugin.ResourcePlugin for Kubernetes Namespaces.
type Plugin struct{}

// New creates a new Namespace plugin.
func New(_ *k8s.Client, _ *k8s.Store) plugin.ResourcePlugin {
	return &Plugin{}
}

func (p *Plugin) Name() string                     { return "namespaces" }
func (p *Plugin) ShortName() string                { return "ns" }
func (p *Plugin) GVR() schema.GroupVersionResource { return gvr }
func (p *Plugin) IsClusterScoped() bool            { return true }

func (p *Plugin) Columns() []plugin.Column {
	return []plugin.Column{
		{Title: "NAME", Flex: true},
		{Title: "STATUS", Width: 12},
		{Title: "AGE", Width: 8},
	}
}

func (p *Plugin) Row(obj *unstructured.Unstructured) []string {
	name := obj.GetName()

	phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
	if phase == "" {
		phase = "Active"
	}

	age := render.FormatAge(obj)

	return []string{name, phase, age}
}

func (p *Plugin) YAML(obj *unstructured.Unstructured) (render.Content, error) {
	return plugin.MarshalYAML(obj)
}

func (p *Plugin) Describe(ctx context.Context, obj *unstructured.Unstructured) (render.Content, error) {
	ns, err := toNamespace(obj)
	if err != nil {
		return render.Content{}, fmt.Errorf("namespaces: decode: %w", err)
	}

	b := render.NewBuilder()

	// Basic metadata
	b.KV(render.LEVEL_0, "Name", ns.Name)

	// Labels
	b.KVMulti(render.LEVEL_0, "Labels", ns.Labels)

	// Annotations
	b.KVMulti(render.LEVEL_0, "Annotations", ns.Annotations)

	// Status
	phase := string(ns.Status.Phase)
	if phase == "" {
		phase = "Active"
	}
	b.KVStyled(render.LEVEL_0, render.StatusKind(phase), "Status", phase)

	// Conditions
	if len(ns.Status.Conditions) > 0 {
		b.Section(render.LEVEL_0, "Conditions")
		for _, cond := range ns.Status.Conditions {
			b.KVStyled(render.LEVEL_1, render.ConditionKind(string(cond.Status)), string(cond.Type), string(cond.Status))
		}
	}

	return b.Build(), nil
}

// GoTo implements plugin.GoToer — Enter navigates to pods in the selected namespace.
func (p *Plugin) GoTo(obj *unstructured.Unstructured) (string, string, bool) {
	if _, ok := plugin.ByName("pods"); !ok {
		return "", "", false
	}
	return "pods", obj.GetName(), true
}

// toNamespace converts an unstructured object to a typed corev1.Namespace.
func toNamespace(obj *unstructured.Unstructured) (*corev1.Namespace, error) {
	var ns corev1.Namespace
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &ns); err != nil {
		return nil, err
	}
	return &ns, nil
}
