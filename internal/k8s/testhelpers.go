package k8s

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// SetGVRForTest seeds a single GVR/Kind mapping into the Discovery index without
// a live API server. It is used by tests in other packages that need a resolver
// primed with a known mapping. It writes under the same lock Populate/readers
// use, so it is safe to call alongside the concurrent read paths.
func (d *Discovery) SetGVRForTest(apiVersion, kind string, gvr schema.GroupVersionResource) {
	gv, err := schema.ParseGroupVersion(apiVersion)
	if err != nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.gvrIndex == nil {
		d.gvrIndex = map[string]schema.GroupVersionResource{}
	}
	if d.kindIndex == nil {
		d.kindIndex = map[schema.GroupVersionResource]string{}
	}
	d.gvrIndex[gvrIndexKey(gv.Group, gv.Version, kind)] = gvr
	d.kindIndex[gvr] = kind
}

// SeededTestDiscovery returns a *Discovery pre-populated with common resources
// for testing.
func SeededTestDiscovery(t *testing.T) *Discovery {
	t.Helper()
	d := NewDiscovery()
	entries := []struct {
		group, version, kind, resource string
	}{
		{"", "v1", "Pod", "pods"},
		{"", "v1", "Service", "services"},
		{"", "v1", "Secret", "secrets"},
		{"", "v1", "ConfigMap", "configmaps"},
		{"", "v1", "Namespace", "namespaces"},
		{"apps", "v1", "Deployment", "deployments"},
		{"apps", "v1", "StatefulSet", "statefulsets"},
		{"apps", "v1", "DaemonSet", "daemonsets"},
		{"networking.k8s.io", "v1", "Ingress", "ingresses"},
	}
	resources := make([]APIResource, 0, len(entries))
	for _, e := range entries {
		resources = append(resources, APIResource{
			Name:    e.resource,
			Group:   e.group,
			Version: e.version,
			Kind:    e.kind,
			GVR:     schema.GroupVersionResource{Group: e.group, Version: e.version, Resource: e.resource},
		})
	}
	d.Populate(resources)
	return d
}
