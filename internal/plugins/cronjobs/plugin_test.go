package cronjobs

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/render"
	"github.com/charmbracelet/x/ansi"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// mockPlugin implements plugin.ResourcePlugin for testing.
type mockPlugin struct {
	name string
}

func (m *mockPlugin) Name() string      { return m.name }
func (m *mockPlugin) ShortName() string { return m.name[:2] }
func (m *mockPlugin) GVR() schema.GroupVersionResource {
	return schema.GroupVersionResource{}
}
func (m *mockPlugin) IsClusterScoped() bool                     { return false }
func (m *mockPlugin) Columns() []plugin.Column                  { return nil }
func (m *mockPlugin) Row(_ *unstructured.Unstructured) []string { return nil }
func (m *mockPlugin) YAML(_ *unstructured.Unstructured) (render.Content, error) {
	return render.Content{}, nil
}
func (m *mockPlugin) Describe(_ context.Context, _ *unstructured.Unstructured) (render.Content, error) {
	return render.Content{}, nil
}

func TestCronJobPluginColumns(t *testing.T) {
	p := New(nil, nil)
	cols := p.Columns()
	if len(cols) != 6 {
		t.Fatalf("expected 6 columns, got %d", len(cols))
	}
	expected := []string{"NAME", "SCHEDULE", "SUSPEND", "ACTIVE", "LAST SCHEDULE", "AGE"}
	for i, col := range cols {
		if col.Title != expected[i] {
			t.Errorf("column %d: expected %q, got %q", i, expected[i], col.Title)
		}
	}
}

func TestCronJobPluginRow(t *testing.T) {
	p := New(nil, nil)
	obj := makeCronJob("backup-job", "*/5 * * * *", false, 0)
	row := p.Row(obj)
	if row[0] != "backup-job" {
		t.Fatalf("expected 'backup-job', got '%s'", row[0])
	}
	if row[1] != "*/5 * * * *" {
		t.Fatalf("expected '*/5 * * * *', got '%s'", row[1])
	}
	if row[2] != "False" {
		t.Fatalf("expected 'False', got '%s'", row[2])
	}
	if row[3] != "0" {
		t.Fatalf("expected '0', got '%s'", row[3])
	}
}

func TestCronJobPluginRowSuspended(t *testing.T) {
	p := New(nil, nil)
	obj := makeCronJob("suspended-job", "0 * * * *", true, 2)
	row := p.Row(obj)
	if row[2] != "True" {
		t.Fatalf("expected 'True', got '%s'", row[2])
	}
	if row[3] != "2" {
		t.Fatalf("expected '2', got '%s'", row[3])
	}
}

func TestCronJobPluginDescribe(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "batch/v1",
			"kind":       "CronJob",
			"metadata": map[string]any{
				"name":              "backup-job",
				"namespace":         "default",
				"creationTimestamp": "2026-02-24T10:00:00Z",
				"labels":            map[string]any{"app": "backup"},
			},
			"spec": map[string]any{
				"schedule":                   "*/5 * * * *",
				"concurrencyPolicy":          "Forbid",
				"suspend":                    false,
				"successfulJobsHistoryLimit": int64(3),
				"failedJobsHistoryLimit":     int64(1),
			},
			"status": map[string]any{
				"lastScheduleTime": "2026-03-03T10:00:00Z",
				"active":           []any{},
			},
		},
	}

	c, err := p.Describe(t.Context(), obj)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Display == "" {
		t.Fatal("display output should not be empty")
	}
	if stripped := ansi.Strip(c.Display); stripped != c.Raw {
		t.Errorf("strip invariant violated: ansi.Strip(c.Display) != raw\nstripped: %q\nraw:      %q", stripped, c.Raw)
	}

	checks := []string{
		"backup-job", "default",
		"Schedule:", "*/5 * * * *",
		"Concurrency Policy:", "Forbid",
		"Suspend:", "false",
		"Successful Job History Limit:", "3",
		"Failed Job History Limit:", "1",
		"Last Schedule Time:",
		"Active Jobs:", "0",
	}
	for _, want := range checks {
		if !strings.Contains(c.Raw, want) {
			t.Errorf("describe output should contain %q\n\nFull output:\n%s", want, c.Raw)
		}
	}
}

func TestCronJobDrillDown(t *testing.T) {
	jobsGVR := schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "jobs"}
	store := k8s.NewStore(nil, nil)

	plugin.Reset()
	mockJobs := &mockPlugin{name: "jobs"}
	plugin.Register(mockJobs)

	job1 := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{
			"name": "backup-job-12345", "namespace": "default",
			"ownerReferences": []any{map[string]any{"uid": "cronjob-uid-1"}},
		},
	}}
	store.CacheUpsert(jobsGVR, "default", job1)

	p := &Plugin{
		store: store,
	}
	cronjob := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{
			"name": "backup-job", "namespace": "default", "uid": "cronjob-uid-1",
		},
	}}

	childPlugin, children := p.DrillDown(cronjob)
	if childPlugin == nil {
		t.Fatal("expected child plugin, got nil")
	}
	if childPlugin.Name() != "jobs" {
		t.Fatalf("expected child plugin 'jobs', got %q", childPlugin.Name())
	}
	if len(children) != 1 {
		t.Fatalf("expected 1 child job, got %d", len(children))
	}
}

func TestCronJobDrillDownNilStore(t *testing.T) {
	p := &Plugin{
		store: nil,
	}
	cronjob := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{"name": "backup-job", "namespace": "default", "uid": "cronjob-uid-1"},
	}}
	childPlugin, children := p.DrillDown(cronjob)
	if childPlugin != nil || children != nil {
		t.Fatal("expected nil, nil for nil store")
	}
}

func TestCronJobPluginName(t *testing.T) {
	p := New(nil, nil)
	if p.Name() != "cronjobs" {
		t.Fatalf("expected 'cronjobs', got '%s'", p.Name())
	}
	if p.ShortName() != "cj" {
		t.Fatalf("expected 'cj', got '%s'", p.ShortName())
	}
}

func makeCronJob(name, schedule string, suspend bool, activeCount int) *unstructured.Unstructured {
	active := make([]any, activeCount)
	for i := range activeCount {
		active[i] = map[string]any{
			"name":      fmt.Sprintf("%s-%d", name, i),
			"namespace": "default",
		}
	}

	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "batch/v1",
			"kind":       "CronJob",
			"metadata": map[string]any{
				"name":              name,
				"namespace":         "default",
				"creationTimestamp": "2024-01-01T00:00:00Z",
			},
			"spec": map[string]any{
				"schedule": schedule,
				"suspend":  suspend,
			},
			"status": map[string]any{
				"active": active,
			},
		},
	}
}
