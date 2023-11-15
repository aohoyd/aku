package priorityclasses

import (
	"context"
	"fmt"

	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/render"
	schedulingv1 "k8s.io/api/scheduling/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var gvr = schema.GroupVersionResource{Group: "scheduling.k8s.io", Version: "v1", Resource: "priorityclasses"}

// Plugin implements plugin.ResourcePlugin for Kubernetes PriorityClasses.
type Plugin struct{}

// New creates a new PriorityClasses plugin.
func New(_ *k8s.Client, _ *k8s.Store) plugin.ResourcePlugin {
	return &Plugin{}
}

func (p *Plugin) Name() string                     { return "priorityclasses" }
func (p *Plugin) ShortName() string                { return "pc" }
func (p *Plugin) GVR() schema.GroupVersionResource { return gvr }
func (p *Plugin) IsClusterScoped() bool            { return true }

func (p *Plugin) Columns() []plugin.Column {
	return []plugin.Column{
		{Title: "NAME", Flex: true},
		{Title: "VALUE", Width: 10},
		{Title: "GLOBAL-DEFAULT", Width: 16},
		{Title: "AGE", Width: 8},
	}
}

func (p *Plugin) Row(obj *unstructured.Unstructured) []string {
	name := obj.GetName()

	value, _, _ := unstructured.NestedInt64(obj.Object, "value")
	valueStr := fmt.Sprintf("%d", value)

	globalDefault, _, _ := unstructured.NestedBool(obj.Object, "globalDefault")
	globalDefaultStr := "false"
	if globalDefault {
		globalDefaultStr = "true"
	}

	age := render.FormatAge(obj)

	return []string{name, valueStr, globalDefaultStr, age}
}

func (p *Plugin) YAML(obj *unstructured.Unstructured) (render.Content, error) {
	return plugin.MarshalYAML(obj)
}

func (p *Plugin) Describe(ctx context.Context, obj *unstructured.Unstructured) (render.Content, error) {
	pc, err := toPriorityClass(obj)
	if err != nil {
		return render.Content{}, fmt.Errorf("converting to PriorityClass: %w", err)
	}

	b := render.NewBuilder()

	b.KV(render.LEVEL_0, "Name", pc.Name)
	b.KVMulti(render.LEVEL_0, "Labels", pc.Labels)
	b.KVMulti(render.LEVEL_0, "Annotations", pc.Annotations)
	b.KV(render.LEVEL_0, "Value", fmt.Sprintf("%d", pc.Value))
	b.KV(render.LEVEL_0, "GlobalDefault", fmt.Sprintf("%t", pc.GlobalDefault))

	preemptionPolicy := "<none>"
	if pc.PreemptionPolicy != nil {
		preemptionPolicy = string(*pc.PreemptionPolicy)
	}
	b.KV(render.LEVEL_0, "PreemptionPolicy", preemptionPolicy)

	b.KV(render.LEVEL_0, "Description", pc.Description)

	return b.Build(), nil
}

// toPriorityClass converts an unstructured object to a typed schedulingv1.PriorityClass.
func toPriorityClass(obj *unstructured.Unstructured) (*schedulingv1.PriorityClass, error) {
	var pc schedulingv1.PriorityClass
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &pc); err != nil {
		return nil, err
	}
	return &pc, nil
}
