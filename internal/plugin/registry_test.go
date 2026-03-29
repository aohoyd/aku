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

func (m *mockPlugin) Name() string                     { return m.name }
func (m *mockPlugin) ShortName() string {
	if len(m.name) < 2 {
		return m.name
	}
	return m.name[:2]
}
func (m *mockPlugin) GVR() schema.GroupVersionResource { return m.gvr }
func (m *mockPlugin) IsClusterScoped() bool            { return false }
func (m *mockPlugin) Columns() []Column                { return nil }
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

	// All() includes both: the duplicate is in ordered even though it's not the primary.
	if len(All()) != 2 {
		t.Fatalf("expected 2 plugins in All(), got %d", len(All()))
	}
}

func TestRegisterIfAbsentSameGVR(t *testing.T) {
	Reset()
	original := &mockPlugin{name: "pods", gvr: schema.GroupVersionResource{Version: "v1", Resource: "pods"}}
	Register(original)

	// Same GVR — should be skipped entirely (API discovery finding a built-in resource).
	discovered := &mockPlugin{name: "pods", gvr: schema.GroupVersionResource{Version: "v1", Resource: "pods"}}
	ok := RegisterIfAbsent(discovered)
	if ok {
		t.Fatal("expected RegisterIfAbsent to return false for same GVR")
	}

	if len(All()) != 1 {
		t.Fatalf("expected 1 plugin in All() (no duplicate), got %d", len(All()))
	}
	if len(AllByName("pods")) != 1 {
		t.Fatalf("expected 1 plugin in AllByName (no duplicate), got %d", len(AllByName("pods")))
	}
	if HasNameCollision("pods") {
		t.Fatal("expected no collision for same-GVR duplicate")
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

func TestRegisterSameNameAllByName(t *testing.T) {
	Reset()
	p1 := &mockPlugin{
		name: "certificates",
		gvr:  schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "certificates"},
	}
	p2 := &mockPlugin{
		name: "certificates",
		gvr:  schema.GroupVersionResource{Group: "networking.internal.knative.dev", Version: "v1alpha1", Resource: "certificates"},
	}
	Register(p1)
	Register(p2)

	// ByName returns the last registered (Register overwrites).
	got, ok := ByName("certificates")
	if !ok {
		t.Fatal("expected to find 'certificates' by name")
	}
	if got.GVR().Group != "networking.internal.knative.dev" {
		t.Fatalf("expected last-registered group, got %s", got.GVR().Group)
	}

	// AllByName returns both.
	all := AllByName("certificates")
	if len(all) != 2 {
		t.Fatalf("expected 2 plugins from AllByName, got %d", len(all))
	}
	if all[0].GVR().Group != "cert-manager.io" {
		t.Fatalf("expected first plugin group cert-manager.io, got %s", all[0].GVR().Group)
	}
	if all[1].GVR().Group != "networking.internal.knative.dev" {
		t.Fatalf("expected second plugin group networking.internal.knative.dev, got %s", all[1].GVR().Group)
	}
}

func TestRegisterIfAbsentSameNameByGVRAndAllByName(t *testing.T) {
	Reset()
	p1 := &mockPlugin{
		name: "certificates",
		gvr:  schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "certificates"},
	}
	p2 := &mockPlugin{
		name: "certificates",
		gvr:  schema.GroupVersionResource{Group: "networking.internal.knative.dev", Version: "v1alpha1", Resource: "certificates"},
	}
	RegisterIfAbsent(p1)
	ok := RegisterIfAbsent(p2)
	if ok {
		t.Fatal("expected RegisterIfAbsent to return false for second same-name plugin")
	}

	// ByName still returns the first.
	got, found := ByName("certificates")
	if !found {
		t.Fatal("expected to find 'certificates' by name")
	}
	if got.GVR().Group != "cert-manager.io" {
		t.Fatalf("expected primary plugin group cert-manager.io, got %s", got.GVR().Group)
	}

	// Second plugin is reachable via ByGVR.
	got2, found := ByGVR(p2.GVR())
	if !found {
		t.Fatal("expected to find second plugin by GVR")
	}
	if got2.GVR().Group != "networking.internal.knative.dev" {
		t.Fatalf("expected group networking.internal.knative.dev, got %s", got2.GVR().Group)
	}

	// AllByName returns both.
	all := AllByName("certificates")
	if len(all) != 2 {
		t.Fatalf("expected 2 plugins from AllByName, got %d", len(all))
	}
}

func TestHasNameCollision(t *testing.T) {
	Reset()

	// No plugins at all -- no collision.
	if HasNameCollision("certificates") {
		t.Fatal("expected no collision for unregistered name")
	}

	p1 := &mockPlugin{
		name: "certificates",
		gvr:  schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "certificates"},
	}
	Register(p1)

	// Single plugin -- no collision.
	if HasNameCollision("certificates") {
		t.Fatal("expected no collision with single plugin")
	}

	p2 := &mockPlugin{
		name: "certificates",
		gvr:  schema.GroupVersionResource{Group: "networking.internal.knative.dev", Version: "v1alpha1", Resource: "certificates"},
	}
	Register(p2)

	// Two plugins with same name -- collision.
	if !HasNameCollision("certificates") {
		t.Fatal("expected collision with two same-name plugins")
	}

	// Different name -- no collision.
	if HasNameCollision("pods") {
		t.Fatal("expected no collision for 'pods'")
	}
}

func TestResetClearsByNameAll(t *testing.T) {
	Reset()
	p := &mockPlugin{
		name: "certificates",
		gvr:  schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "certificates"},
	}
	Register(p)
	if len(AllByName("certificates")) != 1 {
		t.Fatal("expected 1 plugin before reset")
	}

	Reset()

	if len(AllByName("certificates")) != 0 {
		t.Fatal("expected 0 plugins after reset")
	}
	if HasNameCollision("certificates") {
		t.Fatal("expected no collision after reset")
	}
}

func TestAllByNameReturnsCopy(t *testing.T) {
	Reset()
	p := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	Register(p)

	slice1 := AllByName("pods")
	slice1[0] = nil // mutate returned slice
	slice2 := AllByName("pods")
	if slice2[0] == nil {
		t.Fatal("AllByName must return a copy; mutation leaked to internal state")
	}
}

func TestAllByNameNotFound(t *testing.T) {
	Reset()
	result := AllByName("nonexistent")
	if result != nil {
		t.Fatalf("expected nil for unknown name, got %v", result)
	}
}

func TestByQualifiedNameValid(t *testing.T) {
	Reset()
	p := &mockPlugin{
		name: "certificates",
		gvr:  schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "certificates"},
	}
	Register(p)

	got, ok := ByQualifiedName("certificates.cert-manager.io/v1")
	if !ok {
		t.Fatal("expected to find plugin by qualified name")
	}
	if got.GVR().Group != "cert-manager.io" {
		t.Fatalf("expected group cert-manager.io, got %s", got.GVR().Group)
	}
	if got.GVR().Version != "v1" {
		t.Fatalf("expected version v1, got %s", got.GVR().Version)
	}
	if got.GVR().Resource != "certificates" {
		t.Fatalf("expected resource certificates, got %s", got.GVR().Resource)
	}
}

func TestByQualifiedNameFallbackBareName(t *testing.T) {
	Reset()
	p := &mockPlugin{
		name: "pods",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "pods"},
	}
	Register(p)

	// No "." and no "/" — should fall through to ByName.
	got, ok := ByQualifiedName("pods")
	if !ok {
		t.Fatal("expected to find plugin via ByName fallback for bare name")
	}
	if got.Name() != "pods" {
		t.Fatalf("expected 'pods', got '%s'", got.Name())
	}
}

func TestByQualifiedNameFallbackDotNoSlash(t *testing.T) {
	Reset()
	p := &mockPlugin{
		name: "my.resource",
		gvr:  schema.GroupVersionResource{Version: "v1", Resource: "my.resource"},
	}
	Register(p)

	// Has "." but no "/" — should fall through to ByName.
	got, ok := ByQualifiedName("my.resource")
	if !ok {
		t.Fatal("expected to find plugin via ByName fallback for dotted name without slash")
	}
	if got.Name() != "my.resource" {
		t.Fatalf("expected 'my.resource', got '%s'", got.Name())
	}
}

func TestByQualifiedNameNotFound(t *testing.T) {
	Reset()

	// Valid qualified format but no matching GVR registered.
	_, ok := ByQualifiedName("widgets.example.com/v1")
	if ok {
		t.Fatal("expected not to find plugin for unregistered qualified name")
	}
}
