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
// mockPluginExplicitShort allows explicit control over the short name.
type mockPluginExplicitShort struct {
	mockPlugin
	shortName string
}

func (m *mockPluginExplicitShort) ShortName() string { return m.shortName }

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

func TestByNameShortNameFallback(t *testing.T) {
	Reset()
	mock := &mockPlugin{name: "pods", gvr: schema.GroupVersionResource{Version: "v1", Resource: "pods"}}
	Register(mock)

	// Look up by short name "po" (derived from name[:2])
	got, ok := ByName("po")
	if !ok {
		t.Fatal("expected to find 'pods' plugin by short name 'po'")
	}
	if got.Name() != "pods" {
		t.Fatalf("expected 'pods', got '%s'", got.Name())
	}
}

func TestByNameFullNamePrecedenceOverShortName(t *testing.T) {
	Reset()
	// Register plugin A with name "po" (full name matches the short name of plugin B)
	pluginA := &mockPluginExplicitShort{
		mockPlugin: mockPlugin{name: "po", gvr: schema.GroupVersionResource{Version: "v1", Resource: "po"}},
		shortName:  "p",
	}
	Register(pluginA)

	// Register plugin B with name "pods", short name "po"
	pluginB := &mockPluginExplicitShort{
		mockPlugin: mockPlugin{name: "pods", gvr: schema.GroupVersionResource{Version: "v1", Resource: "pods"}},
		shortName:  "po",
	}
	Register(pluginB)

	// ByName("po") should return pluginA (full name match), not pluginB (short name match)
	got, ok := ByName("po")
	if !ok {
		t.Fatal("expected to find plugin by name 'po'")
	}
	if got.Name() != "po" {
		t.Fatalf("expected full name 'po' to take precedence, got '%s'", got.Name())
	}
}

func TestRegisterIfAbsentIndexesShortName(t *testing.T) {
	Reset()
	mock := &mockPlugin{name: "services", gvr: schema.GroupVersionResource{Version: "v1", Resource: "services"}}
	ok := RegisterIfAbsent(mock)
	if !ok {
		t.Fatal("expected RegisterIfAbsent to return true for new plugin")
	}

	// Look up by short name "se" (derived from name[:2])
	got, ok := ByName("se")
	if !ok {
		t.Fatal("expected to find 'services' plugin by short name 'se'")
	}
	if got.Name() != "services" {
		t.Fatalf("expected 'services', got '%s'", got.Name())
	}
}
