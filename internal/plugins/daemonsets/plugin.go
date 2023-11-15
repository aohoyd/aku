package daemonsets

import (
	"context"
	"fmt"

	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/plugins/workload"
	"github.com/aohoyd/aku/internal/render"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var gvr = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "daemonsets"}

// Plugin implements plugin.ResourcePlugin for Kubernetes DaemonSets.
type Plugin struct {
	store *k8s.Store
}

// New creates a new DaemonSet plugin.
func New(_ *k8s.Client, store *k8s.Store) plugin.ResourcePlugin {
	return &Plugin{store: store}
}

func (p *Plugin) Name() string                     { return "daemonsets" }
func (p *Plugin) ShortName() string                { return "ds" }
func (p *Plugin) GVR() schema.GroupVersionResource { return gvr }
func (p *Plugin) IsClusterScoped() bool            { return false }

func (p *Plugin) Columns() []plugin.Column {
	return []plugin.Column{
		{Title: "NAME", Flex: true},
		{Title: "DESIRED", Width: 8},
		{Title: "CURRENT", Width: 8},
		{Title: "READY", Width: 8},
		{Title: "UP-TO-DATE", Width: 12},
		{Title: "AVAILABLE", Width: 10},
		{Title: "AGE", Width: 8},
	}
}

func (p *Plugin) Row(obj *unstructured.Unstructured) []string {
	name := obj.GetName()

	desired := workload.GetInt64(obj, "status", "desiredNumberScheduled")
	current := workload.GetInt64(obj, "status", "currentNumberScheduled")
	ready := workload.GetInt64(obj, "status", "numberReady")
	upToDate := workload.GetInt64(obj, "status", "updatedNumberScheduled")
	available := workload.GetInt64(obj, "status", "numberAvailable")
	age := render.FormatAge(obj)

	return []string{
		name,
		fmt.Sprintf("%d", desired),
		fmt.Sprintf("%d", current),
		fmt.Sprintf("%d", ready),
		fmt.Sprintf("%d", upToDate),
		fmt.Sprintf("%d", available),
		age,
	}
}

func (p *Plugin) YAML(obj *unstructured.Unstructured) (render.Content, error) {
	return plugin.MarshalYAML(obj)
}

func (p *Plugin) Describe(ctx context.Context, obj *unstructured.Unstructured) (render.Content, error) {
	ds, err := toDaemonSet(obj)
	if err != nil {
		return render.Content{}, fmt.Errorf("converting to DaemonSet: %w", err)
	}

	b := render.NewBuilder()

	workload.DescribeMetadata(b, obj, ds.Name, ds.Namespace, ds.Labels, ds.Annotations)

	if ds.Spec.Selector != nil {
		workload.DescribeSelector(b, ds.Spec.Selector.MatchLabels)
	}

	// Node Selector
	if len(ds.Spec.Template.Spec.NodeSelector) > 0 {
		b.KVMulti(render.LEVEL_0, "Node-Selector", ds.Spec.Template.Spec.NodeSelector)
	}

	// Update Strategy
	strategyType := string(ds.Spec.UpdateStrategy.Type)
	if strategyType != "" {
		b.KV(render.LEVEL_0, "UpdateStrategy", strategyType)
	}
	if ds.Spec.UpdateStrategy.Type == appsv1.RollingUpdateDaemonSetStrategyType && ds.Spec.UpdateStrategy.RollingUpdate != nil {
		ru := ds.Spec.UpdateStrategy.RollingUpdate
		maxUnavail := ""
		maxSurge := ""
		if ru.MaxUnavailable != nil {
			maxUnavail = ru.MaxUnavailable.String()
		}
		if ru.MaxSurge != nil {
			maxSurge = ru.MaxSurge.String()
		}
		b.KV(render.LEVEL_0, "RollingUpdateStrategy", fmt.Sprintf("max unavailable %s, max surge %s", maxUnavail, maxSurge))
	}

	// Pod Template
	workload.DescribePodTemplate(b, ds.Spec.Template)

	// Conditions
	workload.DescribeConditions(b, ds)

	return b.Build(), nil
}

// DrillDown implements plugin.DrillDowner.
func (p *Plugin) DrillDown(obj *unstructured.Unstructured) (plugin.ResourcePlugin, []*unstructured.Unstructured) {
	if p.store == nil {
		return nil, nil
	}
	pp, ok := plugin.ByName("pods")
	if !ok {
		return nil, nil
	}
	p.store.Subscribe(workload.PodsGVR, obj.GetNamespace())
	pods := workload.FindOwnedPods(p.store, obj.GetNamespace(), string(obj.GetUID()))
	return pp, pods
}

// toDaemonSet converts an unstructured object to a typed appsv1.DaemonSet.
func toDaemonSet(obj *unstructured.Unstructured) (*appsv1.DaemonSet, error) {
	var ds appsv1.DaemonSet
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &ds); err != nil {
		return nil, err
	}
	return &ds, nil
}
