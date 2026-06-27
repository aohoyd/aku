package plugin

import (
	"context"

	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/render"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Cluster is the per-call context a plugin needs to serve describe and
// drill-down requests against a specific cluster's data. The app layer supplies
// it from the focused pane's cluster at call time, so a single plugin instance
// (registered once) can serve any cluster.
//
// The interface is intentionally narrow and depends only on *k8s.Store and
// *k8s.Discovery, which this package already imports. This avoids importing the
// internal/cluster package here (which would create an import cycle, since
// cluster imports k8s and app imports both). *cluster.Cluster satisfies this
// interface structurally because it exposes Store() and Discovery() methods.
type Cluster interface {
	Store() *k8s.Store
	Discovery() *k8s.Discovery
}

// StoreOf returns cl.Store(), tolerating a nil Cluster (e.g. a call site that
// has no cluster wired yet) so callers can guard with a single nil check.
func StoreOf(cl Cluster) *k8s.Store {
	if cl == nil {
		return nil
	}
	return cl.Store()
}

// DiscoveryOf returns cl.Discovery(), tolerating a nil Cluster the same way as
// StoreOf.
func DiscoveryOf(cl Cluster) *k8s.Discovery {
	if cl == nil {
		return nil
	}
	return cl.Discovery()
}

// MarshalYAML serialises an Unstructured object to YAML, stripping noisy
// fields like metadata.managedFields to match kubectl's default behaviour.
func MarshalYAML(obj *unstructured.Unstructured) (render.Content, error) {
	clean := obj.DeepCopy()
	if md, ok := clean.Object["metadata"].(map[string]any); ok {
		delete(md, "managedFields")
	}
	return render.YAML(clean.Object)
}

// ResourcePlugin is the contract every resource type must satisfy.
type ResourcePlugin interface {
	Name() string
	ShortName() string
	GVR() schema.GroupVersionResource
	IsClusterScoped() bool

	Columns() []Column
	Row(obj *unstructured.Unstructured) []string

	YAML(obj *unstructured.Unstructured) (render.Content, error)
	Describe(ctx context.Context, obj *unstructured.Unstructured) (render.Content, error)
}

// Sortable is an optional interface plugins may implement to provide sort keys
// for their columns. If a plugin does not implement Sortable, ResourceList
// falls back to built-in defaults for NAME and AGE columns.
//
// The column parameter is always the upper-cased Column.Title (e.g. "STATUS").
// Return "" from SortValue to fall back to built-in handling for that column.
type Sortable interface {
	SortValue(obj *unstructured.Unstructured, column string) string
}

// SortPreference specifies a plugin's preferred default sort.
type SortPreference struct {
	Column    string
	Ascending bool
}

// DefaultSorter is an optional interface plugins may implement to override
// the default sort column. If not implemented, NAME ascending is used.
type DefaultSorter interface {
	DefaultSort() SortPreference
}

// Uncoverable is an optional interface plugins may implement to provide a
// describe view with resolved/uncovered environment variable values.
// The app layer probes for this interface via type assertion.
//
// The Cluster argument supplies the store/discovery for the cluster the request
// targets (the focused pane's cluster), so the plugin reads live objects from
// the correct cluster rather than a store baked in at construction time.
type Uncoverable interface {
	DescribeUncovered(ctx context.Context, cl Cluster, obj *unstructured.Unstructured) (render.Content, error)
}

// DrillDowner is an optional interface for plugins that show child resources on Enter.
//
// The Cluster argument supplies the store/discovery for the cluster the request
// targets (the focused pane's cluster).
type DrillDowner interface {
	DrillDown(cl Cluster, obj *unstructured.Unstructured) (ResourcePlugin, []*unstructured.Unstructured)
}

// DrillUp is an optional interface for plugins that navigate to a resource's
// logical parent on Backspace. It is the inverse of DrillDowner: where DrillDown
// returns child resources, DrillUp returns the single parent.
//
// The resolution strategy is implementation-specific: ownerReference-based for
// workload controllers (e.g. delegating to workload.FindParentByOwnerRef), or
// another relationship such as a label-based lookup for non-owned parents
// (e.g. endpointslices → service via workload.FindServiceForEndpointSlice, a
// kubernetes.io/service-name label match, not an ownerReference).
//
// The Cluster argument supplies the store/discovery for the cluster the request
// targets (the focused pane's cluster). Returns (nil, nil) when there is no
// navigable parent (no resolvable relationship, an unresolvable parent kind, or
// no registered plugin for the parent). The returned object may be nil even when
// the plugin is non-nil — the parent's store bucket may not be synced yet — in
// which case the caller pushes an empty view that fills asynchronously.
type DrillUp interface {
	DrillUp(cl Cluster, obj *unstructured.Unstructured) (ResourcePlugin, *unstructured.Unstructured)
}

// NodeLinker is an optional interface for plugins whose objects are scheduled
// onto a Node and can navigate to that hosting Node. It mirrors DrillUp but
// follows spec.nodeName rather than metadata.ownerReferences: where DrillUp
// returns the owning parent, GoToNode returns the Node the object runs on.
//
// The Cluster argument supplies the store/discovery for the cluster the request
// targets (the focused pane's cluster). Implementations typically delegate to
// workload.FindNodeForPod. Returns (nil, nil) when there is no navigable node
// (empty spec.nodeName, or no registered nodes plugin). The returned object may
// be nil even when the plugin is non-nil — the nodes store bucket may not be
// synced yet — in which case the caller pushes an empty view that fills
// asynchronously.
type NodeLinker interface {
	GoToNode(cl Cluster, obj *unstructured.Unstructured) (ResourcePlugin, *unstructured.Unstructured)
}

// GoToer is an optional interface for plugins that navigate to a different resource view on Enter.
type GoToer interface {
	GoTo(obj *unstructured.Unstructured) (resourceName string, namespace string, ok bool)
}

// Commander lets a plugin map Enter on a row to an app command string.
//
// Enter precedence: Commander is checked FIRST. If it returns ok=true, the
// returned cmd string is dispatched through executeCommand and no further
// handling occurs. If ok=false, Enter falls through to GoToer (navigate to a
// different resource view), and if that does not apply either, to the default
// drill-down / enter-detail behavior.
type Commander interface {
	Command(obj *unstructured.Unstructured) (cmd string, ok bool)
}

// PaneCountSetter is an optional interface a self-populating plugin may
// implement to receive the current per-context pane counts. The app layer
// pushes the counts in (App.distinctPaneContexts) via SetPaneCounts before/while
// the view is shown, so the plugin can render a per-context pane count without
// holding a back-reference to the app. A nil map is treated as all-zero.
type PaneCountSetter interface {
	SetPaneCounts(map[string]int)
}

// SelfPopulating is an optional interface for plugins that manage their own
// object list instead of using the k8s.Store informer system.
// Used by synthetic views like api-resources that don't watch real K8s resources.
type SelfPopulating interface {
	Objects() []*unstructured.Unstructured
}

// Refreshable is an optional interface for SelfPopulating plugins that need
// to re-fetch data when the namespace changes.
type Refreshable interface {
	Refresh(namespace string)
}

// Column defines a table column.
type Column struct {
	Title string
	Width int
	Flex  bool
}
