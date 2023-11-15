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

var gvrIndex sync.Map
var kindIndex sync.Map

func gvrIndexKey(group, version, kind string) string {
	return group + "/" + version + "/" + kind
}

// ResolveGVR looks up a GVR by apiVersion and kind from the discovery index.
func ResolveGVR(apiVersion, kind string) (schema.GroupVersionResource, bool) {
	gv, err := schema.ParseGroupVersion(apiVersion)
	if err != nil {
		return schema.GroupVersionResource{}, false
	}
	key := gvrIndexKey(gv.Group, gv.Version, kind)
	if val, ok := gvrIndex.Load(key); ok {
		return val.(schema.GroupVersionResource), true
	}
	return schema.GroupVersionResource{}, false
}

// KindForGVR looks up the Kind string for a given GVR from the discovery index.
func KindForGVR(gvr schema.GroupVersionResource) (string, bool) {
	if val, ok := kindIndex.Load(gvr); ok {
		return val.(string), true
	}
	return "", false
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
	Resources []APIResource
	Err       error
}

// DiscoverAPIResources fetches all API resources from the cluster.
func DiscoverAPIResources(typed kubernetes.Interface) ([]APIResource, error) {
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

	return filterAPIResources(derefLists), err
}

// filterAPIResources processes raw API resource lists into our structured format.
func filterAPIResources(lists []metav1.APIResourceList) []APIResource {
	gvrIndex.Clear()
	kindIndex.Clear()

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
	for _, r := range result {
		gvrIndex.Store(gvrIndexKey(r.Group, r.Version, r.Kind), r.GVR)
		kindIndex.Store(r.GVR, r.Kind)
	}
	return result
}

func hasVerb(verbs metav1.Verbs, target string) bool {
	return slices.Contains(verbs, target)
}
