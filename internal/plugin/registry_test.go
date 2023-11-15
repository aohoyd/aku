package plugin

import (
	"context"
	"testing"

	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/render"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type mockPlugin struct {
	name string
	gvr  schema.GroupVersionResource
}

func (m *mockPlugin) Name() string                    { return m.name }
func (m *mockPlugin) ShortName() string               { return m.name[:2] }
func (m *mockPlugin) GVR() schema.GroupVersionResource { return m.gvr }
func (m *mockPlugin) IsClusterScoped() bool            { return false }
func (m *mockPlugin) Columns() []Column               { return nil }
func (m *mockPlugin) Row(_ *unstructured.Unstructured) []string {
	return nil
}
func (m *mockPlugin) YAML(_ *unstructured.Unstructured) (render.Content, error) {
	return render.Content{}, nil
}
func (m *mockPlugin) Describe(_ context.Context, _ *unstructured.Unstructured) (render.Content, error) {
	return render.Content{}, nil
}
func TestRegistryRegisterAndLookup(t *testing.T) {
	Reset()
	mock := &mockPlugin{name: "pods", gvr: schema.GroupVersionResource{Version: "v1", Resource: "pods"}}
	Register(mock)

	got, ok := ByName("pods")
	if !ok || got.Name() != "pods" {
		t.Fatal("expected to find 'pods' plugin by name")
	}

	got2, ok := ByGVR(mock.GVR())
	if !ok || got2.Name() != "pods" {
		t.Fatal("expected to find 'pods' plugin by GVR")
	}
}

func TestRegistryAll(t *testing.T) {
	Reset()
	Register(&mockPlugin{name: "pods"})
	Register(&mockPlugin{name: "deployments"})
	if len(All()) != 2 {
		t.Fatalf("expected 2 plugins, got %d", len(All()))
	}
}

func TestRegistryByNameNotFound(t *testing.T) {
	Reset()
	_, ok := ByName("nonexistent")
	if ok {
		t.Fatal("should not find nonexistent plugin")
	}
}

func TestRegistryByGVRNotFound(t *testing.T) {
	Reset()
	_, ok := ByGVR(schema.GroupVersionResource{Resource: "nonexistent"})
	if ok {
		t.Fatal("should not find nonexistent GVR")
	}
}

func TestRegisterIfAbsent(t *testing.T) {
	Reset()
	original := &mockPlugin{name: "pods", gvr: schema.GroupVersionResource{Version: "v1", Resource: "pods"}}
	Register(original)

	duplicate := &mockPlugin{name: "pods", gvr: schema.GroupVersionResource{Version: "v2", Resource: "pods"}}
	ok := RegisterIfAbsent(duplicate)
	if ok {
		t.Fatal("expected RegisterIfAbsent to return false for existing plugin")
	}

	got, _ := ByName("pods")
	if got.GVR().Version != "v1" {
		t.Fatal("expected original plugin to be preserved")
	}

	if len(All()) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(All()))
	}
}

func TestRegisterIfAbsentNew(t *testing.T) {
	Reset()
	p := &mockPlugin{name: "certificates", gvr: schema.GroupVersionResource{Resource: "certificates"}}
	ok := RegisterIfAbsent(p)
	if !ok {
		t.Fatal("expected RegisterIfAbsent to return true for new plugin")
	}
	if len(All()) != 1 {
		t.Fatalf("expected 1 plugin, got %d", len(All()))
	}
}

func TestByKindFound(t *testing.T) {
	Reset()
	k8s.TestSeedGVRIndex(t)

	mock := &mockPlugin{
		name: "deployments",
		gvr:  schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"},
	}
	Register(mock)

	got, ok := ByKind("apps/v1", "Deployment")
	if !ok {
		t.Fatal("expected to find plugin by kind")
	}
	if got.Name() != "deployments" {
		t.Fatalf("expected 'deployments', got '%s'", got.Name())
	}
}

func TestByKindNotFound(t *testing.T) {
	Reset()
	_, ok := ByKind("v1", "NonExistent")
	if ok {
		t.Fatal("should not find plugin for unknown kind")
	}
}

func TestByKindNoPlugin(t *testing.T) {
	Reset()
	k8s.TestSeedGVRIndex(t)

	_, ok := ByKind("apps/v1", "Deployment")
	if ok {
		t.Fatal("should not find plugin when none registered for GVR")
	}
}
