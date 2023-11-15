package jobs

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

var gvr = schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "jobs"}

// Plugin implements plugin.ResourcePlugin for Kubernetes Jobs.
type Plugin struct {
	store *k8s.Store
}

// New creates a new Job plugin.
func New(_ *k8s.Client, store *k8s.Store) plugin.ResourcePlugin {
	return &Plugin{store: store}
}

func (p *Plugin) Name() string                            { return "jobs" }
func (p *Plugin) ShortName() string                       { return "job" }
func (p *Plugin) GVR() schema.GroupVersionResource        { return gvr }
func (p *Plugin) IsClusterScoped() bool                   { return false }

func (p *Plugin) Columns() []plugin.Column {
	return []plugin.Column{
		{Title: "NAME", Flex: true},
		{Title: "COMPLETIONS", Width: 12},
		{Title: "DURATION", Width: 12},
		{Title: "AGE", Width: 8},
	}
}

func (p *Plugin) Row(obj *unstructured.Unstructured) []string {
	name := obj.GetName()

	succeeded := workload.GetInt64(obj, "status", "succeeded")
	completions := workload.GetInt64(obj, "spec", "completions")

	completionStr := fmt.Sprintf("%d/%d", succeeded, completions)
	duration := formatJobDuration(obj)
	age := render.FormatAge(obj)

	return []string{name, completionStr, duration, age}
}

func (p *Plugin) YAML(obj *unstructured.Unstructured) (render.Content, error) {
	return plugin.MarshalYAML(obj)
}

func (p *Plugin) Describe(ctx context.Context, obj *unstructured.Unstructured) (render.Content, error) {
	job, err := toJob(obj)
	if err != nil {
		return render.Content{}, fmt.Errorf("converting to Job: %w", err)
	}

	b := render.NewBuilder()

	workload.DescribeMetadata(b, obj, job.Name, job.Namespace, job.Labels, job.Annotations)

	if job.Spec.Selector != nil {
		workload.DescribeSelector(b, job.Spec.Selector.MatchLabels)
	}

	// Parallelism
	var parallelism int32 = 1
	if job.Spec.Parallelism != nil {
		parallelism = *job.Spec.Parallelism
	}
	b.KV(render.LEVEL_0, "Parallelism", fmt.Sprintf("%d", parallelism))

	// Completions
	var completions int32 = 1
	if job.Spec.Completions != nil {
		completions = *job.Spec.Completions
	}
	b.KV(render.LEVEL_0, "Completions", fmt.Sprintf("%d", completions))

	// Backoff Limit
	var backoffLimit int32 = 6
	if job.Spec.BackoffLimit != nil {
		backoffLimit = *job.Spec.BackoffLimit
	}
	b.KV(render.LEVEL_0, "BackoffLimit", fmt.Sprintf("%d", backoffLimit))

	// Status counts
	b.KV(render.LEVEL_0, "Active", fmt.Sprintf("%d", job.Status.Active))
	b.KV(render.LEVEL_0, "Succeeded", fmt.Sprintf("%d", job.Status.Succeeded))
	b.KV(render.LEVEL_0, "Failed", fmt.Sprintf("%d", job.Status.Failed))

	// Conditions
	workload.DescribeConditions(b, job)

	// Pod Template
	workload.DescribePodTemplate(b, job.Spec.Template)

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

// toJob converts an unstructured object to a typed batchv1.Job.
func toJob(obj *unstructured.Unstructured) (*batchv1.Job, error) {
	var j batchv1.Job
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &j); err != nil {
		return nil, err
	}
	return &j, nil
}

// formatJobDuration computes the duration from status.startTime to
// status.completionTime (or now if still running).
func formatJobDuration(obj *unstructured.Unstructured) string {
	startStr, found, _ := unstructured.NestedString(obj.Object, "status", "startTime")
	if !found || startStr == "" {
		return ""
	}
	start, err := time.Parse(time.RFC3339, startStr)
	if err != nil {
		return ""
	}

	end := time.Now()
	completionStr, found, _ := unstructured.NestedString(obj.Object, "status", "completionTime")
	if found && completionStr != "" {
		if ct, err := time.Parse(time.RFC3339, completionStr); err == nil {
			end = ct
		}
	}

	d := end.Sub(start)
	return formatDuration(d)
}

// formatDuration formats a duration in a kubectl-like style.
func formatDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		m := int(d.Minutes())
		s := int(d.Seconds()) % 60
		if s == 0 {
			return fmt.Sprintf("%dm", m)
		}
		return fmt.Sprintf("%dm%ds", m, s)
	case d < 24*time.Hour:
		h := int(d.Hours())
		m := int(d.Minutes()) % 60
		if m == 0 {
			return fmt.Sprintf("%dh", h)
		}
		return fmt.Sprintf("%dh%dm", h, m)
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
