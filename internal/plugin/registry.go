package plugin

import (
	"sync"

	"github.com/aohoyd/aku/internal/k8s"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	mu      sync.RWMutex
	byName  = make(map[string]ResourcePlugin)
	byGVR   = make(map[schema.GroupVersionResource]ResourcePlugin)
	ordered []ResourcePlugin
)

// Register adds a plugin to the global registry.
func Register(p ResourcePlugin) {
	mu.Lock()
	defer mu.Unlock()
	byName[p.Name()] = p
	byGVR[p.GVR()] = p
	ordered = append(ordered, p)
}

// RegisterIfAbsent adds a plugin only if no plugin with the same name exists.
// Returns true if the plugin was registered.
func RegisterIfAbsent(p ResourcePlugin) bool {
	mu.Lock()
	defer mu.Unlock()
	if _, exists := byName[p.Name()]; exists {
		return false
	}
	byName[p.Name()] = p
	byGVR[p.GVR()] = p
	ordered = append(ordered, p)
	return true
}

// ByName looks up a plugin by its name.
func ByName(name string) (ResourcePlugin, bool) {
	mu.RLock()
	defer mu.RUnlock()
	p, ok := byName[name]
	return p, ok
}

// ByGVR looks up a plugin by its GroupVersionResource.
func ByGVR(gvr schema.GroupVersionResource) (ResourcePlugin, bool) {
	mu.RLock()
	defer mu.RUnlock()
	p, ok := byGVR[gvr]
	return p, ok
}

// ByKind looks up a plugin by Kubernetes apiVersion and kind,
// using the discovery index to resolve the GVR first.
func ByKind(apiVersion, kind string) (ResourcePlugin, bool) {
	gvr, ok := k8s.ResolveGVR(apiVersion, kind)
	if !ok {
		return nil, false
	}
	return ByGVR(gvr)
}

// All returns all registered plugins in registration order.
func All() []ResourcePlugin {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]ResourcePlugin, len(ordered))
	copy(out, ordered)
	return out
}

// Reset clears the global registry. For testing only.
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	byName = make(map[string]ResourcePlugin)
	byGVR = make(map[schema.GroupVersionResource]ResourcePlugin)
	ordered = nil
}
