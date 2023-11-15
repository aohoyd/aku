package generic

import (
	"strings"
	"testing"

	"github.com/aohoyd/aku/internal/render"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestDescribeGenericAllFields(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"metadata": map[string]any{
				"name":      "my-resource",
				"namespace": "my-namespace",
				"labels": map[string]any{
					"app":  "web",
					"tier": "frontend",
				},
				"annotations": map[string]any{
					"description":                 "A test resource",
					"app.kubernetes.io/component": "server",
				},
			},
		},
	}

	b := render.NewBuilder()
	describeGeneric(b, obj)
	c := b.Build()

	if !strings.Contains(c.Raw, "Name:") {
		t.Fatalf("expected 'Name:' in output, got:\n%s", c.Raw)
	}
	if !strings.Contains(c.Raw, "my-resource") {
		t.Fatalf("expected resource name in output, got:\n%s", c.Raw)
	}
	if !strings.Contains(c.Raw, "Namespace:") {
		t.Fatalf("expected 'Namespace:' in output, got:\n%s", c.Raw)
	}
	if !strings.Contains(c.Raw, "my-namespace") {
		t.Fatalf("expected namespace in output, got:\n%s", c.Raw)
	}
	if !strings.Contains(c.Raw, "Labels:") {
		t.Fatalf("expected 'Labels:' in output, got:\n%s", c.Raw)
	}
	if !strings.Contains(c.Raw, "app=web") {
		t.Fatalf("expected label 'app=web' in output, got:\n%s", c.Raw)
	}
	if !strings.Contains(c.Raw, "tier=frontend") {
		t.Fatalf("expected label 'tier=frontend' in output, got:\n%s", c.Raw)
	}
	if !strings.Contains(c.Raw, "Annotations:") {
		t.Fatalf("expected 'Annotations:' in output, got:\n%s", c.Raw)
	}
	if !strings.Contains(c.Raw, "description=A test resource") {
		t.Fatalf("expected annotation 'description=A test resource' in output, got:\n%s", c.Raw)
	}
	if !strings.Contains(c.Raw, "app.kubernetes.io/component=server") {
		t.Fatalf("expected annotation 'app.kubernetes.io/component=server' in output, got:\n%s", c.Raw)
	}

	nameIdx := strings.Index(c.Raw, "Name:")
	nsIdx := strings.Index(c.Raw, "Namespace:")
	labelsIdx := strings.Index(c.Raw, "Labels:")
	annotationsIdx := strings.Index(c.Raw, "Annotations:")
	if nameIdx >= nsIdx {
		t.Fatalf("Name should come before Namespace")
	}
	if nsIdx >= labelsIdx {
		t.Fatalf("Namespace should come before Labels")
	}
	if labelsIdx >= annotationsIdx {
		t.Fatalf("Labels should come before Annotations")
	}
}

func TestDescribeGenericEmptyFields(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"metadata": map[string]any{
				"name": "empty-resource",
			},
		},
	}

	b := render.NewBuilder()
	describeGeneric(b, obj)
	c := b.Build()

	if !strings.Contains(c.Raw, "Name:") {
		t.Fatalf("expected 'Name:' in output, got:\n%s", c.Raw)
	}
	if !strings.Contains(c.Raw, "empty-resource") {
		t.Fatalf("expected resource name in output, got:\n%s", c.Raw)
	}
	if !strings.Contains(c.Raw, "Namespace:") {
		t.Fatalf("expected 'Namespace:' in output, got:\n%s", c.Raw)
	}
	if !strings.Contains(c.Raw, "Labels:") {
		t.Fatalf("expected 'Labels:' in output, got:\n%s", c.Raw)
	}
	if !strings.Contains(c.Raw, "<none>") {
		t.Fatalf("expected '<none>' for empty labels, got:\n%s", c.Raw)
	}
	if !strings.Contains(c.Raw, "Annotations:") {
		t.Fatalf("expected 'Annotations:' in output, got:\n%s", c.Raw)
	}
}

func TestDescribeGenericMinimalObject(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]any{},
	}

	b := render.NewBuilder()
	describeGeneric(b, obj)
	c := b.Build()

	if !strings.Contains(c.Raw, "Name:") {
		t.Fatalf("expected 'Name:' in output, got:\n%s", c.Raw)
	}
	if !strings.Contains(c.Raw, "Namespace:") {
		t.Fatalf("expected 'Namespace:' in output, got:\n%s", c.Raw)
	}
	if !strings.Contains(c.Raw, "Labels:") {
		t.Fatalf("expected 'Labels:' in output, got:\n%s", c.Raw)
	}
	if !strings.Contains(c.Raw, "Annotations:") {
		t.Fatalf("expected 'Annotations:' in output, got:\n%s", c.Raw)
	}
}

func TestNewDiscoveredClusterScoped(t *testing.T) {
	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}
	p := NewDiscovered(gvr, "namespaces", "ns", true)
	if p.Name() != "namespaces" {
		t.Fatalf("expected name 'namespaces', got '%s'", p.Name())
	}
	if p.ShortName() != "ns" {
		t.Fatalf("expected short name 'ns', got '%s'", p.ShortName())
	}
	if !p.IsClusterScoped() {
		t.Fatal("expected cluster-scoped to be true")
	}
}

func TestNewDiscoveredNamespaced(t *testing.T) {
	gvr := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	p := NewDiscovered(gvr, "deployments", "deploy", false)
	if p.IsClusterScoped() {
		t.Fatal("expected cluster-scoped to be false")
	}
	if p.ShortName() != "deploy" {
		t.Fatalf("expected short name 'deploy', got '%s'", p.ShortName())
	}
}

func TestNewDiscoveredEmptyShortName(t *testing.T) {
	gvr := schema.GroupVersionResource{Group: "example.com", Version: "v1", Resource: "foos"}
	p := NewDiscovered(gvr, "foos", "", false)
	if p.ShortName() != "foos" {
		t.Fatalf("expected short name to fall back to resource name 'foos', got '%s'", p.ShortName())
	}
}
