package replicasets

import (
	"context"
	"fmt"

	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/plugins/workload"
	"github.com/aohoyd/aku/internal/render"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var gvr = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "replicasets"}

// Plugin implements plugin.ResourcePlugin for Kubernetes ReplicaSets.
type Plugin struct{}

// New creates a new ReplicaSet plugin.
func New() plugin.ResourcePlugin {
	return &Plugin{}
}

func (p *Plugin) Name() string                     { return "replicasets" }
func (p *Plugin) ShortName() string                { return "rs" }
func (p *Plugin) GVR() schema.GroupVersionResource { return gvr }
func (p *Plugin) IsClusterScoped() bool            { return false }

func (p *Plugin) Columns() []plugin.Column {
	return []plugin.Column{
		{Title: "NAME", Flex: true},
		{Title: "DESIRED", Width: 8},
		{Title: "CURRENT", Width: 8},
		{Title: "READY", Width: 8},
		{Title: "AGE", Width: 8},
	}
}

func (p *Plugin) Row(obj *unstructured.Unstructured) []string {
	name := obj.GetName()

	desired := workload.GetInt64(obj, "spec", "replicas")
	current := workload.GetInt64(obj, "status", "replicas")
	ready := workload.GetInt64(obj, "status", "readyReplicas")
	age := render.FormatAge(obj)

	return []string{
		name,
		fmt.Sprintf("%d", desired),
		fmt.Sprintf("%d", current),
		fmt.Sprintf("%d", ready),
		age,
	}
}

// RowHealth tints the row yellow when fewer replicas are ready than desired and
// never red: a ReplicaSet has no clean error signal, so a failing pod turns red
// on its own pod row instead.
func (p *Plugin) RowHealth(obj *unstructured.Unstructured) plugin.Health {
	ready := int32(workload.GetInt64(obj, "status", "readyReplicas"))
	desired := int32(workload.GetInt64(obj, "spec", "replicas"))
	return plugin.WorkloadHealth(ready, desired)
}

func (p *Plugin) YAML(obj *unstructured.Unstructured) (render.Content, error) {
	return plugin.MarshalYAML(obj)
}

func (p *Plugin) Describe(ctx context.Context, obj *unstructured.Unstructured) (render.Content, error) {
	rs, err := toReplicaSet(obj)
	if err != nil {
		return render.Content{}, fmt.Errorf("converting to ReplicaSet: %w", err)
	}

	b := render.NewBuilder()

	workload.DescribeMetadata(b, obj, rs.Name, rs.Namespace, rs.Labels, rs.Annotations)

	if rs.Spec.Selector != nil {
		workload.DescribeSelector(b, rs.Spec.Selector.MatchLabels)
	}

	// Replicas
	var desired int32 = 1
	if rs.Spec.Replicas != nil {
		desired = *rs.Spec.Replicas
	}
	replicaStr := fmt.Sprintf("%d desired | %d total | %d ready",
		desired, rs.Status.Replicas, rs.Status.ReadyReplicas)
	b.KV(render.LEVEL_0, "Replicas", replicaStr)

	// Pod Template
	workload.DescribePodTemplate(b, rs.Spec.Template, nil, nil)

	// Conditions
	workload.DescribeConditions(b, rs)

	return b.Build(), nil
}

// DrillDown implements plugin.DrillDowner.
func (p *Plugin) DrillDown(cl plugin.Cluster, obj *unstructured.Unstructured) (plugin.ResourcePlugin, []*unstructured.Unstructured) {
	store := plugin.StoreOf(cl)
	if store == nil {
		return nil, nil
	}
	pp, ok := plugin.ByName("pods")
	if !ok {
		return nil, nil
	}
	store.Subscribe(workload.PodsGVR, obj.GetNamespace())
	pods := workload.FindOwnedPods(store, obj.GetNamespace(), string(obj.GetUID()))
	return pp, pods
}

// DrillUp implements plugin.DrillUp. A ReplicaSet's owner is its Deployment; the
// shared ownerReference helper resolves it.
func (p *Plugin) DrillUp(cl plugin.Cluster, obj *unstructured.Unstructured) (plugin.ResourcePlugin, *unstructured.Unstructured) {
	return workload.FindParentByOwnerRef(cl, obj)
}

// toReplicaSet converts an unstructured object to a typed appsv1.ReplicaSet.
func toReplicaSet(obj *unstructured.Unstructured) (*appsv1.ReplicaSet, error) {
	var rs appsv1.ReplicaSet
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &rs); err != nil {
		return nil, err
	}
	return &rs, nil
}
