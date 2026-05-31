package k8s

import (
	"cmp"
	"slices"
	"strings"
	"sync"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
)

// Discovery owns the per-cluster API resource indexes used to resolve between
// GroupVersionResources and Kinds. Each cluster must own its own Discovery so
// that resources from different clusters do not collide.
//
// Concurrency: Populate runs off the Update goroutine (Refresh is dispatched as
// a tea.Cmd) while ResolveGVR/KindForGVR/IsEmpty are read on the Update
// goroutine. The two index maps are guarded by a single RWMutex and replaced
// atomically — Populate builds fresh maps and swaps them in under the write lock
// — so a concurrent reader never observes a half-cleared index.
type Discovery struct {
	mu        sync.RWMutex
	gvrIndex  map[string]schema.GroupVersionResource // group/version/kind -> GVR
	kindIndex map[schema.GroupVersionResource]string // GVR -> Kind
}

// NewDiscovery creates an empty Discovery index.
func NewDiscovery() *Discovery {
	return &Discovery{
		gvrIndex:  map[string]schema.GroupVersionResource{},
		kindIndex: map[schema.GroupVersionResource]string{},
	}
}

func gvrIndexKey(group, version, kind string) string {
	return group + "/" + version + "/" + kind
}

// ResolveGVR looks up a GVR by apiVersion and kind from the discovery index.
func (d *Discovery) ResolveGVR(apiVersion, kind string) (schema.GroupVersionResource, bool) {
	gv, err := schema.ParseGroupVersion(apiVersion)
	if err != nil {
		return schema.GroupVersionResource{}, false
	}
	key := gvrIndexKey(gv.Group, gv.Version, kind)
	d.mu.RLock()
	defer d.mu.RUnlock()
	if val, ok := d.gvrIndex[key]; ok {
		return val, true
	}
	return schema.GroupVersionResource{}, false
}

// KindForGVR looks up the Kind string for a given GVR from the discovery index.
func (d *Discovery) KindForGVR(gvr schema.GroupVersionResource) (string, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if val, ok := d.kindIndex[gvr]; ok {
		return val, true
	}
	return "", false
}

// IsEmpty reports whether this Discovery has no resources indexed yet (i.e.
// Refresh has not populated it, or returned nothing). Callers use this to know
// whether a KindForGVR miss is authoritative ("resource not on this cluster")
// or merely premature ("discovery not refreshed yet").
func (d *Discovery) IsEmpty() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.gvrIndex) == 0
}

// APIResource holds discovery metadata for a single resource type.
type APIResource struct {
	Name       string
	ShortNames []string
	APIVersion string // "apps/v1" or "v1"
	Group      string
	Version    string
	Kind       string
	Namespaced bool
	GVR        schema.GroupVersionResource
}

// APIResourcesDiscoveredMsg is sent when background API discovery completes.
type APIResourcesDiscoveredMsg struct {
	// Context identifies the cluster context the discovery results belong to.
	// It may be empty until callers tag results per cluster.
	Context   string
	Resources []APIResource
	Err       error
}

// Refresh fetches all API resources from the cluster, populating this
// Discovery's indexes.
func (d *Discovery) Refresh(typed kubernetes.Interface) ([]APIResource, error) {
	lists, err := typed.Discovery().ServerPreferredResources()
	// ServerPreferredResources may return partial results with an error
	if lists == nil && err != nil {
		return nil, err
	}

	derefLists := make([]metav1.APIResourceList, 0, len(lists))
	for _, l := range lists {
		if l != nil {
			derefLists = append(derefLists, *l)
		}
	}

	return d.filterAPIResources(derefLists), err
}

// filterAPIResources processes raw API resource lists into our structured
// format and (via Populate) seeds this Discovery's indexes with the result.
func (d *Discovery) filterAPIResources(lists []metav1.APIResourceList) []APIResource {
	var result []APIResource
	for _, list := range lists {
		gv, err := schema.ParseGroupVersion(list.GroupVersion)
		if err != nil {
			continue
		}
		for _, r := range list.APIResources {
			// Skip sub-resources (e.g. "pods/log")
			if strings.Contains(r.Name, "/") {
				continue
			}
			// Only include resources that support list (navigable in TUI)
			if !hasVerb(r.Verbs, "list") {
				continue
			}
			apiVersion := gv.Group + "/" + gv.Version
			if gv.Group == "" {
				apiVersion = gv.Version
			}
			result = append(result, APIResource{
				Name:       r.Name,
				ShortNames: r.ShortNames,
				APIVersion: apiVersion,
				Group:      gv.Group,
				Version:    gv.Version,
				Kind:       r.Kind,
				Namespaced: r.Namespaced,
				GVR: schema.GroupVersionResource{
					Group:    gv.Group,
					Version:  gv.Version,
					Resource: r.Name,
				},
			})
		}
	}
	slices.SortFunc(result, func(a, b APIResource) int {
		return cmp.Compare(a.Name, b.Name)
	})
	d.Populate(result)
	return result
}

// Populate replaces this Discovery's indexes with the given resource set. It is
// the index-filling half of Refresh, exported so callers (and tests) that
// already have a resource list in hand can seed a Discovery without a live API
// server. After Populate, IsEmpty reports false (unless resources is empty) and
// KindForGVR/ResolveGVR resolve against exactly this set.
//
// The new index contents are built into fresh local maps first, then swapped in
// atomically under the write lock, so a concurrent reader sees either the old
// complete index or the new complete one — never a half-cleared state.
func (d *Discovery) Populate(resources []APIResource) {
	gvrIndex := make(map[string]schema.GroupVersionResource, len(resources))
	kindIndex := make(map[schema.GroupVersionResource]string, len(resources))
	for _, r := range resources {
		gvrIndex[gvrIndexKey(r.Group, r.Version, r.Kind)] = r.GVR
		kindIndex[r.GVR] = r.Kind
	}
	d.mu.Lock()
	d.gvrIndex = gvrIndex
	d.kindIndex = kindIndex
	d.mu.Unlock()
}

func hasVerb(verbs metav1.Verbs, target string) bool {
	return slices.Contains(verbs, target)
}
