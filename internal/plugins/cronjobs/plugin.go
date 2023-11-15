package cronjobs

import (
	"context"
	"fmt"
	"time"

	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/plugins/workload"
	"github.com/aohoyd/aku/internal/render"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var gvr = schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "cronjobs"}

// Plugin implements plugin.ResourcePlugin for Kubernetes CronJobs.
type Plugin struct {
	store *k8s.Store
}

// New creates a new CronJobs plugin.
func New(_ *k8s.Client, store *k8s.Store) plugin.ResourcePlugin {
	return &Plugin{
		store: store,
	}
}

func (p *Plugin) Name() string                            { return "cronjobs" }
func (p *Plugin) ShortName() string                       { return "cj" }
func (p *Plugin) GVR() schema.GroupVersionResource        { return gvr }
func (p *Plugin) IsClusterScoped() bool                   { return false }

func (p *Plugin) Columns() []plugin.Column {
	return []plugin.Column{
		{Title: "NAME", Flex: true},
		{Title: "SCHEDULE", Width: 20},
		{Title: "SUSPEND", Width: 8},
		{Title: "ACTIVE", Width: 8},
		{Title: "LAST SCHEDULE", Width: 14},
		{Title: "AGE", Width: 8},
	}
}

func (p *Plugin) Row(obj *unstructured.Unstructured) []string {
	name := obj.GetName()

	schedule, _, _ := unstructured.NestedString(obj.Object, "spec", "schedule")

	suspend := "False"
	if s, found, _ := unstructured.NestedBool(obj.Object, "spec", "suspend"); found && s {
		suspend = "True"
	}

	activeList, _, _ := unstructured.NestedSlice(obj.Object, "status", "active")
	active := fmt.Sprintf("%d", len(activeList))

	lastSchedule := formatLastSchedule(obj)
	age := render.FormatAge(obj)

	return []string{name, schedule, suspend, active, lastSchedule, age}
}


func (p *Plugin) YAML(obj *unstructured.Unstructured) (render.Content, error) {
	return plugin.MarshalYAML(obj)
}

func (p *Plugin) Describe(ctx context.Context, obj *unstructured.Unstructured) (render.Content, error) {
	cj, err := toCronJob(obj)
	if err != nil {
		return render.Content{}, fmt.Errorf("converting to CronJob: %w", err)
	}

	b := render.NewBuilder()

	workload.DescribeMetadata(b, obj, cj.Name, cj.Namespace, cj.Labels, cj.Annotations)

	b.KV(render.LEVEL_0, "Schedule", cj.Spec.Schedule)
	b.KV(render.LEVEL_0, "Concurrency Policy", string(cj.Spec.ConcurrencyPolicy))
	b.KV(render.LEVEL_0, "Suspend", fmt.Sprintf("%v", ptrBoolValue(cj.Spec.Suspend)))

	if cj.Spec.SuccessfulJobsHistoryLimit != nil {
		b.KV(render.LEVEL_0, "Successful Job History Limit", fmt.Sprintf("%d", *cj.Spec.SuccessfulJobsHistoryLimit))
	}
	if cj.Spec.FailedJobsHistoryLimit != nil {
		b.KV(render.LEVEL_0, "Failed Job History Limit", fmt.Sprintf("%d", *cj.Spec.FailedJobsHistoryLimit))
	}

	if cj.Status.LastScheduleTime != nil {
		b.KV(render.LEVEL_0, "Last Schedule Time", cj.Status.LastScheduleTime.Format(time.RFC3339))
	} else {
		b.KV(render.LEVEL_0, "Last Schedule Time", "<none>")
	}

	b.KV(render.LEVEL_0, "Active Jobs", fmt.Sprintf("%d", len(cj.Status.Active)))

	return b.Build(), nil
}



// DrillDown implements plugin.DrillDowner.
func (p *Plugin) DrillDown(obj *unstructured.Unstructured) (plugin.ResourcePlugin, []*unstructured.Unstructured) {
	if p.store == nil {
		return nil, nil
	}
	jp, ok := plugin.ByName("jobs")
	if !ok {
		return nil, nil
	}
	p.store.Subscribe(workload.JobsGVR, obj.GetNamespace())
	children := workload.FindOwned(p.store, workload.JobsGVR, obj.GetNamespace(), string(obj.GetUID()))
	return jp, children
}

// formatLastSchedule returns a human-readable duration since status.lastScheduleTime.
func formatLastSchedule(obj *unstructured.Unstructured) string {
	ts, found, _ := unstructured.NestedString(obj.Object, "status", "lastScheduleTime")
	if !found || ts == "" {
		return "<none>"
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return "<unknown>"
	}
	return render.FormatDuration(time.Since(t))
}

// toCronJob converts an unstructured object to a typed batchv1.CronJob.
func toCronJob(obj *unstructured.Unstructured) (*batchv1.CronJob, error) {
	var cj batchv1.CronJob
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &cj); err != nil {
		return nil, err
	}
	return &cj, nil
}

// ptrBoolValue dereferences a *bool, returning false if nil.
func ptrBoolValue(b *bool) bool {
	if b == nil {
		return false
	}
	return *b
}
