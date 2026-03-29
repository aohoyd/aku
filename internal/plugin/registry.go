package plugin

import (
	"strings"
	"sync"

	"github.com/aohoyd/aku/internal/k8s"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	mu          sync.RWMutex
	byName      = make(map[string]ResourcePlugin)
	byNameAll   = make(map[string][]ResourcePlugin)
	byShortName = make(map[string]ResourcePlugin)
	byGVR       = make(map[schema.GroupVersionResource]ResourcePlugin)
	ordered     []ResourcePlugin
)

// Register adds a plugin to the global registry.
func Register(p ResourcePlugin) {
	mu.Lock()
	defer mu.Unlock()
	byName[p.Name()] = p
	byNameAll[p.Name()] = append(byNameAll[p.Name()], p)
	if sn := p.ShortName(); sn != "" {
		byShortName[sn] = p
	}
	byGVR[p.GVR()] = p
	ordered = append(ordered, p)
}

// RegisterIfAbsent adds a plugin only if its GVR is not already registered.
// When the GVR is new but another plugin with the same name exists, the plugin
// is added to byNameAll, byGVR, and ordered (so it appears in All()) but NOT
// set as the primary byName entry. Returns true only when the plugin becomes
// the primary entry for its name.
func RegisterIfAbsent(p ResourcePlugin) bool {
	mu.Lock()
	defer mu.Unlock()
	// If this exact GVR is already registered, skip entirely.
	// This prevents duplicates when API discovery finds a resource
	// that already has a built-in plugin.
	if _, exists := byGVR[p.GVR()]; exists {
		return false
	}
	byNameAll[p.Name()] = append(byNameAll[p.Name()], p)
	byGVR[p.GVR()] = p
	ordered = append(ordered, p)
	if _, exists := byName[p.Name()]; exists {
		return false
	}
	byName[p.Name()] = p
	if sn := p.ShortName(); sn != "" {
		byShortName[sn] = p
	}
	return true
}

// ByName looks up a plugin by its name.
func ByName(name string) (ResourcePlugin, bool) {
	mu.RLock()
	defer mu.RUnlock()
	if p, ok := byName[name]; ok {
		return p, true
	}
	p, ok := byShortName[name]
	return p, ok
}

// ByQualifiedName looks up a plugin by a qualified name in the format
// "name.group/version" (e.g. "certificates.cert-manager.io/v1").
// If the input does not contain a "/" it is not a qualified name and the
// lookup falls through to ByName for backward compatibility.
func ByQualifiedName(qualified string) (ResourcePlugin, bool) {
	slashIdx := strings.LastIndex(qualified, "/")
	if slashIdx < 0 {
		// No "/" means it's not a qualified name — fall through.
		return ByName(qualified)
	}
	version := qualified[slashIdx+1:]
	left := qualified[:slashIdx]

	dotIdx := strings.Index(left, ".")
	if dotIdx < 0 {
		// Has "/" but no "." in the left part — not a valid qualified name.
		return ByName(qualified)
	}

	name := left[:dotIdx]
	group := left[dotIdx+1:]

	gvr := schema.GroupVersionResource{Group: group, Version: version, Resource: name}
	return ByGVR(gvr)
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

// AllByName returns all plugins registered under a given name, across all
// API groups. Returns a copy of the internal slice.
func AllByName(name string) []ResourcePlugin {
	mu.RLock()
	defer mu.RUnlock()
	src := byNameAll[name]
	if len(src) == 0 {
		return nil
	}
	out := make([]ResourcePlugin, len(src))
	copy(out, src)
	return out
}

// HasNameCollision reports whether more than one plugin has been registered
// with the given name (i.e. same plural name but different API groups).
func HasNameCollision(name string) bool {
	mu.RLock()
	defer mu.RUnlock()
	return len(byNameAll[name]) > 1
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
	byNameAll = make(map[string][]ResourcePlugin)
	byShortName = make(map[string]ResourcePlugin)
	byGVR = make(map[schema.GroupVersionResource]ResourcePlugin)
	ordered = nil
}
