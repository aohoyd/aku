package helmreleases

import (
	"testing"
	"time"

	"github.com/aohoyd/aku/internal/helm"
	"github.com/aohoyd/aku/internal/plugin"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type mockClient struct {
	releases    []helm.ReleaseInfo
	values      map[string]any
	valuesErr   error
	lastAllFlag bool
}

func (m *mockClient) ListReleases(namespace string) ([]helm.ReleaseInfo, error) {
	if namespace == "" {
		return m.releases, nil
	}
	var filtered []helm.ReleaseInfo
	for _, r := range m.releases {
		if r.Namespace == namespace {
			filtered = append(filtered, r)
		}
	}
	return filtered, nil
}
func (m *mockClient) GetRelease(name, namespace string) (*helm.ReleaseInfo, error) {
	for _, r := range m.releases {
		if r.Name == name && r.Namespace == namespace {
			return &r, nil
		}
	}
	return nil, nil
}
func (m *mockClient) GetValues(_, _ string, all bool) (map[string]any, error) {
	m.lastAllFlag = all
	return m.values, m.valuesErr
}
func (m *mockClient) History(_, _ string) ([]helm.RevisionInfo, error) { return nil, nil }
func (m *mockClient) Upgrade(_, _ string, _ map[string]any) error      { return nil }
func (m *mockClient) Rollback(_, _ string, _ int) error                { return nil }
func (m *mockClient) Uninstall(_, _ string) error                      { return nil }

func testRelease() helm.ReleaseInfo {
	return helm.ReleaseInfo{
		Name: "nginx", Namespace: "default", Revision: 5,
		Chart: "nginx-1.0.0", AppVersion: "1.25", Status: "deployed",
		Updated:  time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC),
		Manifest: "---\napiVersion: v1\nkind: Service\nmetadata:\n  name: nginx-svc\n  namespace: default\n",
	}
}

func newTestPlugin(hc helm.Client) *Plugin {
	p := New(nil, nil, nil)
	p.helmClient = hc
	return p
}

func TestPluginColumns(t *testing.T) {
	p := New(nil, nil, nil)
	cols := p.Columns()
	if len(cols) != 7 {
		t.Fatalf("expected 7 columns, got %d", len(cols))
	}
	if cols[0].Title != "NAME" {
		t.Fatalf("expected first column NAME, got %s", cols[0].Title)
	}
}

func TestPluginRow(t *testing.T) {
	p := New(nil, nil, nil)
	obj := helm.ReleaseToUnstructured(testRelease())
	row := p.Row(obj)
	if row[0] != "nginx" {
		t.Fatalf("expected name nginx, got %s", row[0])
	}
	if row[2] != "5" {
		t.Fatalf("expected revision 5, got %s", row[2])
	}
}

func TestPluginObjects(t *testing.T) {
	mc := &mockClient{releases: []helm.ReleaseInfo{testRelease()}}
	p := newTestPlugin(mc)
	p.Refresh("default")
	objs := p.Objects()
	if len(objs) != 1 {
		t.Fatalf("expected 1 object, got %d", len(objs))
	}
}

func TestPluginDrillDown(t *testing.T) {
	mc := &mockClient{releases: []helm.ReleaseInfo{testRelease()}}
	p := newTestPlugin(mc)
	p.Refresh("default")
	obj := p.Objects()[0]
	childPlugin, children := p.DrillDown(obj)
	if childPlugin == nil {
		t.Fatal("expected child plugin")
	}
	if len(children) != 1 {
		t.Fatalf("expected 1 child resource, got %d", len(children))
	}
	if children[0].GetKind() != "Service" {
		t.Fatalf("expected Service, got %s", children[0].GetKind())
	}
}

func TestDrillDownPreservesRaw(t *testing.T) {
	mc := &mockClient{releases: []helm.ReleaseInfo{testRelease()}}
	p := newTestPlugin(mc)
	p.Refresh("default")
	obj := p.Objects()[0]
	childPlugin, children := p.DrillDown(obj)
	if childPlugin == nil {
		t.Fatal("expected child plugin")
	}
	if len(children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(children))
	}
	raw, _, _ := unstructured.NestedString(children[0].Object, "_raw")
	if raw == "" {
		t.Fatal("expected _raw to be preserved")
	}
}

func TestDrillDownReturnsRefreshableManifest(t *testing.T) {
	mc := &mockClient{releases: []helm.ReleaseInfo{testRelease()}}
	p := newTestPlugin(mc)
	p.Refresh("default")
	obj := p.Objects()[0]
	childPlugin, _ := p.DrillDown(obj)
	if childPlugin == nil {
		t.Fatal("expected child plugin")
	}
	if _, ok := childPlugin.(plugin.Refreshable); !ok {
		t.Fatal("expected child plugin to implement Refreshable")
	}
	if _, ok := childPlugin.(plugin.SelfPopulating); !ok {
		t.Fatal("expected child plugin to implement SelfPopulating")
	}
}

func TestDrillDownManifestCanRefresh(t *testing.T) {
	mc := &mockClient{releases: []helm.ReleaseInfo{testRelease()}}
	p := newTestPlugin(mc)
	p.Refresh("default")
	obj := p.Objects()[0]
	childPlugin, children := p.DrillDown(obj)
	if childPlugin == nil {
		t.Fatal("expected child plugin")
	}
	if len(children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(children))
	}

	// Verify the helmmanifest can refresh and get the same data
	sp := childPlugin.(plugin.SelfPopulating)
	r := childPlugin.(plugin.Refreshable)
	r.Refresh("")
	refreshed := sp.Objects()
	if len(refreshed) != 1 {
		t.Fatalf("expected 1 refreshed object, got %d", len(refreshed))
	}
	if refreshed[0].GetKind() != "Service" {
		t.Fatalf("expected Service, got %s", refreshed[0].GetKind())
	}
}

func newReleaseObj(name, namespace string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetName(name)
	obj.SetNamespace(namespace)
	return obj
}

func TestPluginValuesUserNonEmpty(t *testing.T) {
	mc := &mockClient{values: map[string]any{"replicaCount": 5, "image": "nginx"}}
	p := newTestPlugin(mc)
	content, err := p.Values(newReleaseObj("rel", "default"))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if mc.lastAllFlag {
		t.Errorf("expected GetValues called with all=false")
	}
	if content.Raw == "" {
		t.Fatal("expected non-empty Raw")
	}
	if content.Display == content.Raw {
		t.Errorf("expected styled Display (ANSI codes) different from Raw — render.YAML appears bypassed")
	}
}

func TestPluginValuesAllPropagatesFlag(t *testing.T) {
	mc := &mockClient{values: map[string]any{"k": "v"}}
	p := newTestPlugin(mc)
	if _, err := p.ValuesAll(newReleaseObj("rel", "default")); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !mc.lastAllFlag {
		t.Errorf("expected GetValues called with all=true")
	}
}

func TestPluginValuesEmptyUserPlaceholder(t *testing.T) {
	mc := &mockClient{values: map[string]any{}}
	p := newTestPlugin(mc)
	content, err := p.Values(newReleaseObj("rel", "default"))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if content.Raw != "# No user-supplied values\n" {
		t.Errorf("expected user-values placeholder, got %q", content.Raw)
	}
}

func TestPluginValuesAllEmptyPlaceholder(t *testing.T) {
	mc := &mockClient{values: map[string]any{}}
	p := newTestPlugin(mc)
	content, err := p.ValuesAll(newReleaseObj("rel", "default"))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if content.Raw != "# No values\n" {
		t.Errorf("expected all-values placeholder, got %q", content.Raw)
	}
}

func TestPluginValuesNilHelmClient(t *testing.T) {
	p := New(nil, nil, nil)
	_, err := p.Values(newReleaseObj("rel", "default"))
	if err == nil {
		t.Fatal("expected error when helmClient is nil")
	}
}
