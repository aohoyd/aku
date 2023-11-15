package horizontalpodautoscalers

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestHPAPluginColumns(t *testing.T) {
	p := New(nil, nil)
	cols := p.Columns()
	if len(cols) != 7 {
		t.Fatalf("expected 7 columns, got %d", len(cols))
	}
}

func TestHPAPluginRow(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "autoscaling/v2",
			"kind":       "HorizontalPodAutoscaler",
			"metadata": map[string]any{
				"name":              "web-hpa",
				"namespace":         "default",
				"creationTimestamp": "2024-01-01T00:00:00Z",
			},
			"spec": map[string]any{
				"scaleTargetRef": map[string]any{
					"kind": "Deployment",
					"name": "nginx",
				},
				"minReplicas": int64(2),
				"maxReplicas": int64(10),
			},
			"status": map[string]any{
				"currentReplicas": int64(5),
			},
		},
	}

	row := p.Row(obj)
	if row[0] != "web-hpa" {
		t.Fatalf("expected name 'web-hpa', got '%s'", row[0])
	}
	if row[1] != "Deployment/nginx" {
		t.Fatalf("expected reference 'Deployment/nginx', got '%s'", row[1])
	}
	if row[3] != "2" {
		t.Fatalf("expected minPods '2', got '%s'", row[3])
	}
	if row[4] != "10" {
		t.Fatalf("expected maxPods '10', got '%s'", row[4])
	}
	if row[5] != "5" {
		t.Fatalf("expected replicas '5', got '%s'", row[5])
	}
}

func TestHPAPluginDescribe(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "autoscaling/v2",
			"kind":       "HorizontalPodAutoscaler",
			"metadata": map[string]any{
				"name":              "web-hpa",
				"namespace":         "default",
				"creationTimestamp": "2026-02-24T10:00:00Z",
				"labels":            map[string]any{"app": "web"},
			},
			"spec": map[string]any{
				"scaleTargetRef": map[string]any{
					"kind":       "Deployment",
					"name":       "nginx",
					"apiVersion": "apps/v1",
				},
				"minReplicas": int64(1),
				"maxReplicas": int64(10),
				"metrics": []any{
					map[string]any{
						"type": "Resource",
						"resource": map[string]any{
							"name": "cpu",
							"target": map[string]any{
								"type":               "Utilization",
								"averageUtilization": int64(80),
							},
						},
					},
				},
			},
			"status": map[string]any{
				"currentReplicas": int64(3),
				"desiredReplicas": int64(3),
				"conditions": []any{
					map[string]any{
						"type":   "AbleToScale",
						"status": "True",
						"reason": "ReadyForNewScale",
					},
					map[string]any{
						"type":   "ScalingActive",
						"status": "True",
						"reason": "ValidMetricFound",
					},
				},
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
		"web-hpa", "default",
		"app=web",
		"Deployment/nginx",
		"Min Replicas", "1",
		"Max Replicas", "10",
		"Current Replicas", "3",
		"Conditions:", "AbleToScale", "True",
	}
	for _, want := range checks {
		if !strings.Contains(c.Raw, want) {
			t.Errorf("describe output should contain %q\n\nFull output:\n%s", want, c.Raw)
		}
	}
}
