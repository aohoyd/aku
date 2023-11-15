package storageclasses

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestStorageClassPluginColumns(t *testing.T) {
	p := New(nil, nil)
	cols := p.Columns()
	if len(cols) != 5 {
		t.Fatalf("expected 5 columns, got %d", len(cols))
	}
}

func TestStorageClassPluginRow(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "storage.k8s.io/v1",
			"kind":       "StorageClass",
			"metadata": map[string]any{
				"name":              "standard",
				"creationTimestamp": "2024-01-01T00:00:00Z",
			},
			"provisioner":   "kubernetes.io/aws-ebs",
			"reclaimPolicy": "Retain",
			"volumeBindingMode": "WaitForFirstConsumer",
		},
	}
	row := p.Row(obj)

	if row[0] != "standard" {
		t.Fatalf("expected name 'standard', got '%s'", row[0])
	}
	if row[1] != "kubernetes.io/aws-ebs" {
		t.Fatalf("expected provisioner 'kubernetes.io/aws-ebs', got '%s'", row[1])
	}
	if row[2] != "Retain" {
		t.Fatalf("expected reclaimPolicy 'Retain', got '%s'", row[2])
	}
	if row[3] != "WaitForFirstConsumer" {
		t.Fatalf("expected volumeBindingMode 'WaitForFirstConsumer', got '%s'", row[3])
	}
}

func TestStorageClassPluginRowDefaults(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "storage.k8s.io/v1",
			"kind":       "StorageClass",
			"metadata": map[string]any{
				"name":              "minimal",
				"creationTimestamp": "2024-01-01T00:00:00Z",
			},
			"provisioner": "kubernetes.io/no-provisioner",
		},
	}
	row := p.Row(obj)

	if row[2] != "Delete" {
		t.Fatalf("expected default reclaimPolicy 'Delete', got '%s'", row[2])
	}
	if row[3] != "Immediate" {
		t.Fatalf("expected default volumeBindingMode 'Immediate', got '%s'", row[3])
	}
}

func TestStorageClassPluginDescribe(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "storage.k8s.io/v1",
			"kind":       "StorageClass",
			"metadata": map[string]any{
				"name":              "gp3",
				"creationTimestamp": "2026-02-24T10:00:00Z",
				"labels":            map[string]any{"env": "prod"},
				"annotations":       map[string]any{"note": "test"},
			},
			"provisioner":   "ebs.csi.aws.com",
			"reclaimPolicy": "Delete",
			"volumeBindingMode": "WaitForFirstConsumer",
			"allowVolumeExpansion": true,
			"mountOptions": []any{"debug", "noatime"},
			"parameters": map[string]any{
				"type":      "gp3",
				"encrypted": "true",
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
		"gp3",
		"ebs.csi.aws.com",
		"Delete",
		"WaitForFirstConsumer",
		"true",
		"debug, noatime",
		"type=gp3",
		"encrypted=true",
	}
	for _, want := range checks {
		if !strings.Contains(c.Raw, want) {
			t.Errorf("describe output should contain %q\n\nFull output:\n%s", want, c.Raw)
		}
	}
}
