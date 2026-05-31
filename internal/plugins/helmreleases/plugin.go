package helmreleases

import (
	"context"
	"fmt"
	"sync"

	"github.com/aohoyd/aku/internal/helm"
	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/render"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var syntheticGVR = schema.GroupVersionResource{
	Group: "_ktui", Version: "v1", Resource: "helmreleases",
}

type Plugin struct {
	helmClient helm.Client
	mu         sync.RWMutex
	objects    []*unstructured.Unstructured
}

// New constructs the helmreleases plugin. Store and discovery are not baked in:
// drill-down receives them at call time via plugin.Cluster so the same instance
// can serve any cluster. The Helm client is still built from the (single,
// global) client here because it is exercised through the app layer
// (values/rollback/uninstall via App.helmClientFor), not inside the plugin's
// describe/drill-down paths. Making the Helm client per-cluster is a later task.
func New(client *k8s.Client, resolver helm.ChartResolver) *Plugin {
	p := &Plugin{}
	if client != nil {
		p.helmClient = helm.NewClient(client.Config, resolver)
	}
	return p
}

// NewWithClient constructs a Plugin with an explicit Helm client. Used by
// tests that need to inject a stub client without bringing up the k8s wiring.
func NewWithClient(hc helm.Client) *Plugin {
	return &Plugin{helmClient: hc}
}

func (p *Plugin) Name() string                     { return "helmreleases" }
func (p *Plugin) ShortName() string                { return "release" }
func (p *Plugin) GVR() schema.GroupVersionResource { return syntheticGVR }
func (p *Plugin) IsClusterScoped() bool            { return false }

func (p *Plugin) Columns() []plugin.Column {
	return []plugin.Column{
		{Title: "NAME", Flex: true},
		{Title: "NAMESPACE", Width: 16},
		{Title: "REVISION", Width: 10},
		{Title: "CHART", Width: 24},
		{Title: "APP VERSION", Width: 14},
		{Title: "STATUS", Width: 14},
		{Title: "UPDATED", Width: 22},
	}
}

func (p *Plugin) Row(obj *unstructured.Unstructured) []string {
	name := obj.GetName()
	ns := obj.GetNamespace()
	rev, _, _ := unstructured.NestedString(obj.Object, "revision")
	chart, _, _ := unstructured.NestedString(obj.Object, "chart")
	appVer, _, _ := unstructured.NestedString(obj.Object, "appVersion")
	status, _, _ := unstructured.NestedString(obj.Object, "status")
	updated, _, _ := unstructured.NestedString(obj.Object, "updated")
	return []string{name, ns, rev, chart, appVer, status, updated}
}

func (p *Plugin) YAML(obj *unstructured.Unstructured) (render.Content, error) {
	manifest, _, _ := unstructured.NestedString(obj.Object, "_manifest")
	return render.YAML(map[string]any{"manifest": manifest})
}

// Values / ValuesAll fetch using the plugin's baked (global) helm client. They
// are kept for tests and any caller that has no per-cluster client; app code
// that knows the pane's cluster should use ValuesWith / ValuesAllWith so a
// pinned pane reads the RIGHT cluster's release rather than the global one.
func (p *Plugin) Values(obj *unstructured.Unstructured) (render.Content, error) {
	return fetchValues(p.helmClient, obj, false)
}

func (p *Plugin) ValuesAll(obj *unstructured.Unstructured) (render.Content, error) {
	return fetchValues(p.helmClient, obj, true)
}

// ValuesWith / ValuesAllWith fetch using an explicitly supplied helm client.
// The app passes the focused pane's cluster helm client (helmClientFor) so values
// are read from the pane's actual cluster, not the baked global one.
func (p *Plugin) ValuesWith(hc helm.Client, obj *unstructured.Unstructured) (render.Content, error) {
	return fetchValues(hc, obj, false)
}

func (p *Plugin) ValuesAllWith(hc helm.Client, obj *unstructured.Unstructured) (render.Content, error) {
	return fetchValues(hc, obj, true)
}

func fetchValues(hc helm.Client, obj *unstructured.Unstructured, all bool) (render.Content, error) {
	if hc == nil {
		return render.Content{}, fmt.Errorf("helm client unavailable")
	}
	vals, err := hc.GetValues(obj.GetName(), obj.GetNamespace(), all)
	if err != nil {
		return render.Content{}, err
	}
	if len(vals) == 0 {
		msg := "# No user-supplied values\n"
		if all {
			msg = "# No values\n"
		}
		return render.Content{Raw: msg, Display: msg}, nil
	}
	return render.YAML(vals)
}

func (p *Plugin) Describe(_ context.Context, obj *unstructured.Unstructured) (render.Content, error) {
	b := render.NewBuilder()
	b.KV(render.LEVEL_0, "Name", obj.GetName())
	b.KV(render.LEVEL_0, "Namespace", obj.GetNamespace())
	rev, _, _ := unstructured.NestedString(obj.Object, "revision")
	b.KV(render.LEVEL_0, "Revision", rev)
	chart, _, _ := unstructured.NestedString(obj.Object, "chart")
	b.KV(render.LEVEL_0, "Chart", chart)
	appVer, _, _ := unstructured.NestedString(obj.Object, "appVersion")
	b.KV(render.LEVEL_0, "App Version", appVer)
	status, _, _ := unstructured.NestedString(obj.Object, "status")
	b.KV(render.LEVEL_0, "Status", status)
	updated, _, _ := unstructured.NestedString(obj.Object, "updated")
	b.KV(render.LEVEL_0, "Updated", updated)
	return b.Build(), nil
}

func (p *Plugin) DrillDown(cl plugin.Cluster, obj *unstructured.Unstructured) (plugin.ResourcePlugin, []*unstructured.Unstructured) {
	manifest, _, _ := unstructured.NestedString(obj.Object, "_manifest")
	if manifest == "" {
		return nil, nil
	}
	children := ParseManifest(manifest)
	return &helmmanifest{
		store:            plugin.StoreOf(cl),
		discovery:        plugin.DiscoveryOf(cl),
		helmClient:       p.helmClient,
		releaseName:      obj.GetName(),
		releaseNamespace: obj.GetNamespace(),
		children:         children,
	}, children
}

func (p *Plugin) SortValue(obj *unstructured.Unstructured, column string) string {
	switch column {
	case "REVISION":
		rev, _, _ := unstructured.NestedString(obj.Object, "revision")
		return fmt.Sprintf("%010s", rev)
	case "UPDATED":
		updated, _, _ := unstructured.NestedString(obj.Object, "updated")
		return updated
	}
	return ""
}

func (p *Plugin) Objects() []*unstructured.Unstructured {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.objects
}

func (p *Plugin) Refresh(namespace string) {
	if p.helmClient == nil {
		return
	}
	releases, err := p.helmClient.ListReleases(namespace)
	if err != nil {
		p.mu.Lock()
		p.objects = nil
		p.mu.Unlock()
		return
	}
	objs := make([]*unstructured.Unstructured, len(releases))
	for i, r := range releases {
		objs[i] = helm.ReleaseToUnstructured(r)
	}
	p.mu.Lock()
	p.objects = objs
	p.mu.Unlock()
}
