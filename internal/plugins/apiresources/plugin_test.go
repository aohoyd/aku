package apiresources

import (
	"context"
	"strings"
	"testing"

	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/render"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// mockPlugin implements plugin.ResourcePlugin for testing.
type mockPlugin struct {
	name string
}

func (m *mockPlugin) Name() string                              { return m.name }
func (m *mockPlugin) ShortName() string                         { return m.name[:2] }
func (m *mockPlugin) GVR() schema.GroupVersionResource          { return schema.GroupVersionResource{} }
func (m *mockPlugin) IsClusterScoped() bool                     { return false }
func (m *mockPlugin) Columns() []plugin.Column                  { return nil }
func (m *mockPlugin) Row(_ *unstructured.Unstructured) []string { return nil }
func (m *mockPlugin) YAML(_ *unstructured.Unstructured) (render.Content, error) {
	return render.Content{}, nil
}
func (m *mockPlugin) Describe(_ context.Context, _ *unstructured.Unstructured) (render.Content, error) {
	return render.Content{}, nil
}

func TestPluginMetadata(t *testing.T) {
	p := New()
	if p.Name() != "api-resources" {
		t.Fatalf("expected name 'api-resources', got '%s'", p.Name())
	}
	if p.ShortName() != "api" {
		t.Fatalf("expected short name 'api', got '%s'", p.ShortName())
	}
	if !p.IsClusterScoped() {
		t.Fatal("expected cluster-scoped")
	}
	if p.GVR().Group != "_ktui" {
		t.Fatalf("expected synthetic _ktui group, got '%s'", p.GVR().Group)
	}
}

func TestSetResourcesAndObjects(t *testing.T) {
	p := New()
	resources := []k8s.APIResource{
		{Name: "pods", ShortNames: []string{"po"}, APIVersion: "v1", Kind: "Pod", Namespaced: true},
		{Name: "nodes", ShortNames: nil, APIVersion: "v1", Kind: "Node", Namespaced: false},
	}
	p.SetResources(resources)

	sp, ok := plugin.ResourcePlugin(p).(plugin.SelfPopulating)
	if !ok {
		t.Fatal("expected plugin to implement SelfPopulating")
	}

	objs := sp.Objects()
	if len(objs) != 2 {
		t.Fatalf("expected 2 objects, got %d", len(objs))
	}
	if objs[0].GetName() != "pods" {
		t.Fatalf("expected first object name 'pods', got '%s'", objs[0].GetName())
	}
}

func TestRowRendering(t *testing.T) {
	p := New()
	resources := []k8s.APIResource{
		{Name: "deployments", ShortNames: []string{"deploy"}, APIVersion: "apps/v1", Kind: "Deployment", Namespaced: true},
	}
	p.SetResources(resources)

	objs := p.Objects()
	row := p.Row(objs[0])
	if len(row) != 5 {
		t.Fatalf("expected 5 columns, got %d", len(row))
	}
	if row[0] != "deployments" {
		t.Fatalf("expected NAME='deployments', got '%s'", row[0])
	}
	if row[1] != "deploy" {
		t.Fatalf("expected SHORTNAMES='deploy', got '%s'", row[1])
	}
	if row[2] != "apps/v1" {
		t.Fatalf("expected APIVERSION='apps/v1', got '%s'", row[2])
	}
	if row[3] != "true" {
		t.Fatalf("expected NAMESPACED='true', got '%s'", row[3])
	}
	if row[4] != "Deployment" {
		t.Fatalf("expected KIND='Deployment', got '%s'", row[4])
	}
}

func TestGoTo(t *testing.T) {
	plugin.Reset()
	plugin.Register(&mockPlugin{name: "pods"})

	p := New()
	resources := []k8s.APIResource{
		{Name: "pods", ShortNames: []string{"po"}, APIVersion: "v1", Kind: "Pod", Namespaced: true},
	}
	p.SetResources(resources)

	objs := p.Objects()
	resourceName, ns, ok := p.GoTo(objs[0])
	if !ok {
		t.Fatal("expected GoTo to return ok=true for registered resource")
	}
	if resourceName != "pods" {
		t.Fatalf("expected resource name 'pods', got '%s'", resourceName)
	}
	if ns != "" {
		t.Fatalf("expected empty namespace, got '%s'", ns)
	}
}

func TestGoToUnknownResource(t *testing.T) {
	plugin.Reset()
	p := New()
	resources := []k8s.APIResource{
		{Name: "nonexistent", APIVersion: "v1", Kind: "Nonexistent", Namespaced: true},
	}
	p.SetResources(resources)

	objs := p.Objects()
	_, _, ok := p.GoTo(objs[0])
	if ok {
		t.Fatal("expected GoTo to return ok=false for unregistered resource")
	}
}

func TestDescribe(t *testing.T) {
	p := New()
	resources := []k8s.APIResource{
		{Name: "pods", ShortNames: []string{"po"}, APIVersion: "v1", Kind: "Pod", Namespaced: true},
	}
	p.SetResources(resources)

	objs := p.Objects()
	c, err := p.Describe(nil, objs[0])
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(c.Raw, "pods") {
		t.Fatalf("expected 'pods' in describe output, got:\n%s", c.Raw)
	}
	if !strings.Contains(c.Raw, "po") {
		t.Fatalf("expected 'po' in describe output, got:\n%s", c.Raw)
	}
}
