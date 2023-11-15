package deployments

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

var gvr = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}

// Plugin implements plugin.ResourcePlugin for Kubernetes Deployments.
type Plugin struct {
	store *k8s.Store
}

// New creates a new Deployment plugin.
func New(_ *k8s.Client, store *k8s.Store) plugin.ResourcePlugin {
	return &Plugin{store: store}
}

func (p *Plugin) Name() string                     { return "deployments" }
func (p *Plugin) ShortName() string                { return "deploy" }
func (p *Plugin) GVR() schema.GroupVersionResource { return gvr }
func (p *Plugin) IsClusterScoped() bool            { return false }

func (p *Plugin) Columns() []plugin.Column {
	return []plugin.Column{
		{Title: "NAME", Flex: true},
		{Title: "READY", Width: 10},
		{Title: "UP-TO-DATE", Width: 12},
		{Title: "AVAILABLE", Width: 10},
		{Title: "AGE", Width: 8},
	}
}

func (p *Plugin) Row(obj *unstructured.Unstructured) []string {
	name := obj.GetName()

	replicas := workload.GetInt64(obj, "spec", "replicas")
	readyReplicas := workload.GetInt64(obj, "status", "readyReplicas")
	updatedReplicas := workload.GetInt64(obj, "status", "updatedReplicas")
	availableReplicas := workload.GetInt64(obj, "status", "availableReplicas")

	ready := fmt.Sprintf("%d/%d", readyReplicas, replicas)
	upToDate := fmt.Sprintf("%d", updatedReplicas)
	available := fmt.Sprintf("%d", availableReplicas)
	age := render.FormatAge(obj)

	return []string{name, ready, upToDate, available, age}
}

func (p *Plugin) YAML(obj *unstructured.Unstructured) (render.Content, error) {
	return plugin.MarshalYAML(obj)
}

func (p *Plugin) Describe(ctx context.Context, obj *unstructured.Unstructured) (render.Content, error) {
	deploy, err := toDeployment(obj)
	if err != nil {
		return render.Content{}, fmt.Errorf("converting to Deployment: %w", err)
	}

	b := render.NewBuilder()

	b.KV(render.LEVEL_0, "Name", deploy.Name)
	b.KV(render.LEVEL_0, "Namespace", deploy.Namespace)
	b.KV(render.LEVEL_0, "CreationTimestamp", render.FormatAge(obj))

	// Selector from spec.selector.matchLabels
	if deploy.Spec.Selector != nil && len(deploy.Spec.Selector.MatchLabels) > 0 {
		b.KV(render.LEVEL_0, "Selector", workload.FormatSelector(deploy.Spec.Selector.MatchLabels))
	}

	// Labels
	b.KVMulti(render.LEVEL_0, "Labels", deploy.Labels)

	// Annotations
	b.KVMulti(render.LEVEL_0, "Annotations", deploy.Annotations)

	// Replicas
	var desired int32 = 1
	if deploy.Spec.Replicas != nil {
		desired = *deploy.Spec.Replicas
	}
	total := deploy.Status.Replicas
	updated := deploy.Status.UpdatedReplicas
	available := deploy.Status.AvailableReplicas
	unavailable := deploy.Status.UnavailableReplicas
	replicaStr := fmt.Sprintf("%d desired | %d updated | %d total | %d available | %d unavailable",
		desired, updated, total, available, unavailable)
	b.KV(render.LEVEL_0, "Replicas", replicaStr)

	// Strategy
	strategyType := string(deploy.Spec.Strategy.Type)
	if strategyType != "" {
		b.KV(render.LEVEL_0, "StrategyType", strategyType)
	}
	if deploy.Spec.Strategy.Type == appsv1.RollingUpdateDeploymentStrategyType && deploy.Spec.Strategy.RollingUpdate != nil {
		ru := deploy.Spec.Strategy.RollingUpdate
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

	// Pod Template section
	b.Section(render.LEVEL_0, "Pod Template")

	// Template labels
	if len(deploy.Spec.Template.Labels) > 0 {
		b.KVMulti(render.LEVEL_1, "Labels", deploy.Spec.Template.Labels)
	}

	// Template annotations
	if len(deploy.Spec.Template.Annotations) > 0 {
		b.KVMulti(render.LEVEL_1, "Annotations", deploy.Spec.Template.Annotations)
	}

	// Service Account
	if deploy.Spec.Template.Spec.ServiceAccountName != "" {
		b.KV(render.LEVEL_1, "Service Account", deploy.Spec.Template.Spec.ServiceAccountName)
	}

	// Containers
	workload.DescribeTemplateContainers(b, "Containers", deploy.Spec.Template.Spec.Containers)
	workload.DescribeTemplateContainers(b, "Init Containers", deploy.Spec.Template.Spec.InitContainers)

	// Conditions
	workload.DescribeConditions(b, deploy)

	return b.Build(), nil
}

var replicasetsGVR = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "replicasets"}

// DrillDown implements plugin.DrillDowner.
func (p *Plugin) DrillDown(obj *unstructured.Unstructured) (plugin.ResourcePlugin, []*unstructured.Unstructured) {
	if p.store == nil {
		return nil, nil
	}
	rsPlugin, ok := plugin.ByName("replicasets")
	if !ok {
		return nil, nil
	}
	p.store.Subscribe(replicasetsGVR, obj.GetNamespace())
	children := workload.FindOwned(p.store, replicasetsGVR, obj.GetNamespace(), string(obj.GetUID()))
	return rsPlugin, children
}

// toDeployment converts an unstructured object to a typed appsv1.Deployment.
func toDeployment(obj *unstructured.Unstructured) (*appsv1.Deployment, error) {
	var d appsv1.Deployment
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &d); err != nil {
		return nil, err
	}
	return &d, nil
}
