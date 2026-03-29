package leases

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestLeasePluginColumns(t *testing.T) {
	p := New(nil, nil)
	cols := p.Columns()
	if len(cols) != 3 {
		t.Fatalf("expected 3 columns, got %d", len(cols))
	}
}

func TestLeasePluginRow(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "coordination.k8s.io/v1",
			"kind":       "Lease",
			"metadata": map[string]any{
				"name":              "my-lease",
				"namespace":         "default",
				"creationTimestamp": "2024-01-01T00:00:00Z",
			},
			"spec": map[string]any{
				"holderIdentity": "node-1",
			},
		},
	}
	row := p.Row(obj)

	if row[0] != "my-lease" {
		t.Fatalf("expected name 'my-lease', got '%s'", row[0])
	}
	if row[1] != "node-1" {
		t.Fatalf("expected holder 'node-1', got '%s'", row[1])
	}
	if len(row) != 3 {
		t.Fatalf("expected 3 columns in row, got %d", len(row))
	}
}

func TestLeasePluginRowNoHolder(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "coordination.k8s.io/v1",
			"kind":       "Lease",
			"metadata": map[string]any{
				"name":              "no-holder-lease",
				"namespace":         "default",
				"creationTimestamp": "2024-01-01T00:00:00Z",
			},
			"spec": map[string]any{},
		},
	}
	row := p.Row(obj)

	if row[1] != "<none>" {
		t.Fatalf("expected holder '<none>', got '%s'", row[1])
	}
}

func TestLeasePluginDescribe(t *testing.T) {
	p := New(nil, nil)
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "coordination.k8s.io/v1",
			"kind":       "Lease",
			"metadata": map[string]any{
				"name":              "test-lease",
				"namespace":         "kube-system",
				"creationTimestamp": "2026-02-24T10:00:00Z",
				"labels":            map[string]any{"app": "controller"},
			},
			"spec": map[string]any{
				"holderIdentity":       "node-1",
				"leaseDurationSeconds": int64(15),
				"leaseTransitions":     int64(3),
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
		"test-lease",
		"kube-system",
		"node-1",
		"15",
	}
	for _, want := range checks {
		if !strings.Contains(c.Raw, want) {
			t.Errorf("describe output should contain %q\n\nFull output:\n%s", want, c.Raw)
		}
	}
}
