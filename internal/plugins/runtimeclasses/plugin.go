package runtimeclasses

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/render"
	corev1 "k8s.io/api/core/v1"
	nodev1 "k8s.io/api/node/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var gvr = schema.GroupVersionResource{Group: "node.k8s.io", Version: "v1", Resource: "runtimeclasses"}

// Plugin implements plugin.ResourcePlugin for Kubernetes RuntimeClasses.
type Plugin struct{}

// New creates a new RuntimeClasses plugin.
func New(_ *k8s.Client, _ *k8s.Store) plugin.ResourcePlugin {
	return &Plugin{}
}

func (p *Plugin) Name() string                     { return "runtimeclasses" }
func (p *Plugin) ShortName() string                { return "runtimeclass" }
func (p *Plugin) GVR() schema.GroupVersionResource { return gvr }
func (p *Plugin) IsClusterScoped() bool            { return true }

func (p *Plugin) Columns() []plugin.Column {
	return []plugin.Column{
		{Title: "NAME", Flex: true},
		{Title: "HANDLER", Flex: true},
		{Title: "AGE", Width: 8},
	}
}

func (p *Plugin) Row(obj *unstructured.Unstructured) []string {
	name := obj.GetName()
	handler, _, _ := unstructured.NestedString(obj.Object, "handler")
	age := render.FormatAge(obj)

	return []string{name, handler, age}
}

func (p *Plugin) YAML(obj *unstructured.Unstructured) (render.Content, error) {
	return plugin.MarshalYAML(obj)
}

func (p *Plugin) Describe(ctx context.Context, obj *unstructured.Unstructured) (render.Content, error) {
	rc, err := toRuntimeClass(obj)
	if err != nil {
		return render.Content{}, fmt.Errorf("converting to RuntimeClass: %w", err)
	}

	b := render.NewBuilder()

	// Metadata
	b.KV(render.LEVEL_0, "Name", rc.Name)
	b.KVMulti(render.LEVEL_0, "Labels", rc.Labels)
	b.KVMulti(render.LEVEL_0, "Annotations", rc.Annotations)

	// Handler
	b.KV(render.LEVEL_0, "Handler", rc.Handler)

	// Overhead
	if rc.Overhead != nil && len(rc.Overhead.PodFixed) > 0 {
		b.Section(render.LEVEL_0, "Overhead")
		b.Section(render.LEVEL_1, "PodFixed")
		keys := make([]string, 0, len(rc.Overhead.PodFixed))
		for k := range rc.Overhead.PodFixed {
			keys = append(keys, string(k))
		}
		slices.Sort(keys)
		for _, k := range keys {
			v := rc.Overhead.PodFixed[corev1.ResourceName(k)]
			b.KV(render.LEVEL_2, k, v.String())
		}
	}

	// Scheduling
	if rc.Scheduling != nil {
		b.Section(render.LEVEL_0, "Scheduling")

		// NodeSelector
		b.KVMulti(render.LEVEL_1, "NodeSelector", rc.Scheduling.NodeSelector)

		// Tolerations
		if len(rc.Scheduling.Tolerations) > 0 {
			b.Section(render.LEVEL_1, "Tolerations")
			for _, tol := range rc.Scheduling.Tolerations {
				parts := []string{}
				if tol.Key != "" {
					parts = append(parts, tol.Key)
				}
				if tol.Operator != "" {
					parts = append(parts, string(tol.Operator))
				}
				if tol.Value != "" {
					parts = append(parts, tol.Value)
				}
				if tol.Effect != "" {
					parts = append(parts, string(tol.Effect))
				}
				b.RawLine(render.LEVEL_2, strings.Join(parts, " "))
			}
		}
	}

	return b.Build(), nil
}

// toRuntimeClass converts an unstructured object to a typed nodev1.RuntimeClass.
func toRuntimeClass(obj *unstructured.Unstructured) (*nodev1.RuntimeClass, error) {
	var rc nodev1.RuntimeClass
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &rc); err != nil {
		return nil, err
	}
	return &rc, nil
}
