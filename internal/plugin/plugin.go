package plugin

import (
	"context"

	"github.com/aohoyd/aku/internal/render"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

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
type Uncoverable interface {
	DescribeUncovered(ctx context.Context, obj *unstructured.Unstructured) (render.Content, error)
}

// DrillDowner is an optional interface for plugins that show child resources on Enter.
type DrillDowner interface {
	DrillDown(obj *unstructured.Unstructured) (ResourcePlugin, []*unstructured.Unstructured)
}

// GoToer is an optional interface for plugins that navigate to a different resource view on Enter.
type GoToer interface {
	GoTo(obj *unstructured.Unstructured) (resourceName string, namespace string, ok bool)
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
