package statefulsets

import (
	"context"
	"fmt"
	"strings"

	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/plugins/workload"
	"github.com/aohoyd/aku/internal/render"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var gvr = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"}

// Plugin implements plugin.ResourcePlugin for Kubernetes StatefulSets.
type Plugin struct {
	store *k8s.Store
}

// New creates a new StatefulSet plugin.
func New(_ *k8s.Client, store *k8s.Store) plugin.ResourcePlugin {
	return &Plugin{store: store}
}

func (p *Plugin) Name() string                            { return "statefulsets" }
func (p *Plugin) ShortName() string                       { return "sts" }
func (p *Plugin) GVR() schema.GroupVersionResource        { return gvr }
func (p *Plugin) IsClusterScoped() bool                   { return false }

func (p *Plugin) Columns() []plugin.Column {
	return []plugin.Column{
		{Title: "NAME", Flex: true},
		{Title: "READY", Width: 10},
		{Title: "AGE", Width: 8},
	}
}

func (p *Plugin) Row(obj *unstructured.Unstructured) []string {
	name := obj.GetName()

	replicas := workload.GetInt64(obj, "spec", "replicas")
	readyReplicas := workload.GetInt64(obj, "status", "readyReplicas")

	ready := fmt.Sprintf("%d/%d", readyReplicas, replicas)
	age := render.FormatAge(obj)

	return []string{name, ready, age}
}

func (p *Plugin) YAML(obj *unstructured.Unstructured) (render.Content, error) {
	return plugin.MarshalYAML(obj)
}

func (p *Plugin) Describe(ctx context.Context, obj *unstructured.Unstructured) (render.Content, error) {
	sts, err := toStatefulSet(obj)
	if err != nil {
		return render.Content{}, fmt.Errorf("converting to StatefulSet: %w", err)
	}

	b := render.NewBuilder()

	workload.DescribeMetadata(b, obj, sts.Name, sts.Namespace, sts.Labels, sts.Annotations)

	if sts.Spec.Selector != nil {
		workload.DescribeSelector(b, sts.Spec.Selector.MatchLabels)
	}

	// Replicas
	var desired int32 = 1
	if sts.Spec.Replicas != nil {
		desired = *sts.Spec.Replicas
	}
	b.KV(render.LEVEL_0, "Replicas", fmt.Sprintf("%d desired | %d total", desired, sts.Status.Replicas))

	// Update Strategy
	strategyType := string(sts.Spec.UpdateStrategy.Type)
	if strategyType != "" {
		b.KV(render.LEVEL_0, "Update Strategy", strategyType)
	}
	if sts.Spec.UpdateStrategy.Type == appsv1.RollingUpdateStatefulSetStrategyType &&
		sts.Spec.UpdateStrategy.RollingUpdate != nil &&
		sts.Spec.UpdateStrategy.RollingUpdate.Partition != nil {
		b.KV(render.LEVEL_1, "Partition", fmt.Sprintf("%d", *sts.Spec.UpdateStrategy.RollingUpdate.Partition))
	}

	// Pod Template
	workload.DescribePodTemplate(b, sts.Spec.Template)

	// Volume Claim Templates
	if len(sts.Spec.VolumeClaimTemplates) > 0 {
		b.Section(render.LEVEL_0, "Volume Claim Templates")
		for _, pvc := range sts.Spec.VolumeClaimTemplates {
			b.Section(render.LEVEL_1, pvc.Name)
			if len(pvc.Spec.AccessModes) > 0 {
				modes := make([]string, len(pvc.Spec.AccessModes))
				for i, m := range pvc.Spec.AccessModes {
					modes[i] = string(m)
				}
				b.KV(render.LEVEL_2, "Access Modes", strings.Join(modes, ", "))
			}
			if pvc.Spec.Resources.Requests != nil {
				if storage, ok := pvc.Spec.Resources.Requests["storage"]; ok {
					b.KV(render.LEVEL_2, "Storage", storage.String())
				}
			}
		}
	}

	// Conditions
	workload.DescribeConditions(b, sts)

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

// toStatefulSet converts an unstructured object to a typed appsv1.StatefulSet.
func toStatefulSet(obj *unstructured.Unstructured) (*appsv1.StatefulSet, error) {
	var s appsv1.StatefulSet
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &s); err != nil {
		return nil, err
	}
	return &s, nil
}
