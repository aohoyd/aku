package k8s

import (
	"testing"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

// TestSeedGVRIndex populates the GVR index with common resources for testing.
func TestSeedGVRIndex(t *testing.T) {
	t.Helper()
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
	for _, e := range entries {
		gvr := schema.GroupVersionResource{Group: e.group, Version: e.version, Resource: e.resource}
		gvrIndex.Store(
			gvrIndexKey(e.group, e.version, e.kind),
			gvr,
		)
		kindIndex.Store(gvr, e.kind)
	}
	t.Cleanup(func() {
		gvrIndex.Range(func(key, _ any) bool { gvrIndex.Delete(key); return true })
		kindIndex.Range(func(key, _ any) bool { kindIndex.Delete(key); return true })
	})
}
