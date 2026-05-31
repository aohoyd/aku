package helmreleases

import (
	"context"
	"fmt"
	"sync"

	"github.com/aohoyd/aku/internal/helm"
	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/plugins/generic"
	"github.com/aohoyd/aku/internal/render"
	releaseutil "helm.sh/helm/v4/pkg/release/v1/util"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var kindOrder map[string]int

func init() {
	kindOrder = make(map[string]int, len(releaseutil.InstallOrder))
	for i, kind := range releaseutil.InstallOrder {
		kindOrder[kind] = i
	}
}

var manifestGVR = schema.GroupVersionResource{
	Group: "_ktui", Version: "v1", Resource: "helmmanifest",
}

type helmmanifest struct {
	store            *k8s.Store
	discovery        *k8s.Discovery
	helmClient       helm.Client
	releaseName      string
	releaseNamespace string
	mu               sync.RWMutex
	children         []*unstructured.Unstructured
}

// resolveGVRWith returns the discovery resolver for the supplied cluster,
// falling back to the discovery captured when this manifest was created (e.g.
// for Refresh paths or tests that have no live cluster). Returns nil when no
// discovery is available at all.
func (p *helmmanifest) resolveGVRWith(disc *k8s.Discovery) plugin.GVRResolver {
	if disc == nil {
		disc = p.discovery
	}
	if disc == nil {
		return nil
	}
	return disc.ResolveGVR
}

func (p *helmmanifest) Name() string                     { return "helmmanifest" }
func (p *helmmanifest) ShortName() string                { return "hm" }
func (p *helmmanifest) GVR() schema.GroupVersionResource { return manifestGVR }
func (p *helmmanifest) IsClusterScoped() bool            { return false }

func (p *helmmanifest) Objects() []*unstructured.Unstructured {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.children
}

func (p *helmmanifest) Refresh(_ string) {
	if p.helmClient == nil {
		return
	}
	rel, err := p.helmClient.GetRelease(p.releaseName, p.releaseNamespace)
	if err != nil || rel == nil {
		return
	}
	children := ParseManifest(rel.Manifest)
	p.mu.Lock()
	p.children = children
	p.mu.Unlock()
}

func (p *helmmanifest) isClusterScoped(disc *k8s.Discovery, apiVersion, kind string) bool {
	if cp, ok := plugin.ByKind(p.resolveGVRWith(disc), apiVersion, kind); ok {
		return cp.IsClusterScoped()
	}
	return false
}

func (p *helmmanifest) effectiveNamespace(disc *k8s.Discovery, obj *unstructured.Unstructured) string {
	ns := obj.GetNamespace()
	if ns == "" && !p.isClusterScoped(disc, obj.GetAPIVersion(), obj.GetKind()) {
		return p.releaseNamespace
	}
	return ns
}

func (p *helmmanifest) Columns() []plugin.Column {
	return []plugin.Column{
		{Title: "KIND", Width: 24},
		{Title: "NAME", Flex: true},
		{Title: "NAMESPACE", Width: 20},
	}
}

func (p *helmmanifest) Row(obj *unstructured.Unstructured) []string {
	// Row has no per-call cluster; use the discovery captured at drill-down time.
	return []string{obj.GetKind(), obj.GetName(), p.effectiveNamespace(p.discovery, obj)}
}

func (p *helmmanifest) YAML(obj *unstructured.Unstructured) (render.Content, error) {
	raw, _, _ := unstructured.NestedString(obj.Object, "_raw")
	clean := obj.DeepCopy()
	delete(clean.Object, "_raw")
	c, _ := render.YAML(clean.Object)
	return render.Content{Raw: raw, Display: c.Display}, nil
}

func (p *helmmanifest) SortValue(obj *unstructured.Unstructured, column string) string {
	if column != "KIND" {
		return ""
	}
	kind := obj.GetKind()
	if idx, ok := kindOrder[kind]; ok {
		return fmt.Sprintf("%04d", idx)
	}
	return fmt.Sprintf("9999-%s", kind)
}

func (p *helmmanifest) DefaultSort() plugin.SortPreference {
	return plugin.SortPreference{Column: "KIND", Ascending: true}
}

func (p *helmmanifest) DrillDown(cl plugin.Cluster, obj *unstructured.Unstructured) (plugin.ResourcePlugin, []*unstructured.Unstructured) {
	store := plugin.StoreOf(cl)
	if store == nil {
		store = p.store
	}
	if store == nil {
		return nil, nil
	}

	disc := plugin.DiscoveryOf(cl)
	if disc == nil {
		disc = p.discovery
	}
	if disc == nil {
		return nil, nil
	}

	apiVersion := obj.GetAPIVersion()
	kind := obj.GetKind()

	gvr, ok := disc.ResolveGVR(apiVersion, kind)
	if !ok {
		return nil, nil
	}

	type nk struct{ name, ns string }
	wanted := make(map[nk]struct{})
	namespaces := make(map[string]struct{})
	p.mu.RLock()
	for _, c := range p.children {
		if c.GetAPIVersion() == apiVersion && c.GetKind() == kind {
			ns := p.effectiveNamespace(disc, c)
			wanted[nk{c.GetName(), ns}] = struct{}{}
			namespaces[ns] = struct{}{}
		}
	}
	p.mu.RUnlock()

	var matched []*unstructured.Unstructured
	for ns := range namespaces {
		store.Subscribe(gvr, ns)
		for _, live := range store.List(gvr, ns) {
			if _, ok := wanted[nk{live.GetName(), live.GetNamespace()}]; ok {
				matched = append(matched, live)
			}
		}
	}

	if childPlugin, ok := plugin.ByKind(p.resolveGVRWith(disc), apiVersion, kind); ok {
		return childPlugin, matched
	}
	return generic.New(gvr), matched
}

func (p *helmmanifest) Describe(ctx context.Context, obj *unstructured.Unstructured) (render.Content, error) {
	// Describe is the base interface and carries no per-call cluster; use the
	// discovery captured at drill-down time.
	disc := p.discovery
	if delegate, ok := plugin.ByKind(p.resolveGVRWith(disc), obj.GetAPIVersion(), obj.GetKind()); ok {
		content, err := delegate.Describe(ctx, obj)
		if err == nil {
			return content, nil
		}
	}

	b := render.NewBuilder()
	b.KV(render.LEVEL_0, "Kind", obj.GetKind())
	b.KV(render.LEVEL_0, "Name", obj.GetName())
	b.KV(render.LEVEL_0, "Namespace", p.effectiveNamespace(disc, obj))
	b.KV(render.LEVEL_0, "API Version", obj.GetAPIVersion())
	return b.Build(), nil
}

func (p *helmmanifest) DescribeUncovered(ctx context.Context, cl plugin.Cluster, obj *unstructured.Unstructured) (render.Content, error) {
	disc := plugin.DiscoveryOf(cl)
	if disc == nil {
		disc = p.discovery
	}
	if delegate, ok := plugin.ByKind(p.resolveGVRWith(disc), obj.GetAPIVersion(), obj.GetKind()); ok {
		if unc, ok := delegate.(plugin.Uncoverable); ok {
			content, err := unc.DescribeUncovered(ctx, cl, obj)
			if err == nil {
				return content, nil
			}
		}
	}
	return p.Describe(ctx, obj)
}
