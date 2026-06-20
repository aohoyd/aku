package plugin

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Health classifies the overall state of a resource row for row tinting.
//
//   - Healthy: no tint.
//   - Warning: transitional / coming-up (yellow).
//   - Error:   broken (red).
type Health int

const (
	// Healthy renders with no row tint.
	Healthy Health = iota
	// Warning tints the row yellow (transitional / coming-up).
	Warning
	// Error tints the row red (broken).
	Error
)

// HealthReporter is an OPTIONAL interface a plugin may implement to report the
// overall health of a row, used to tint the whole table row. It is discovered
// via a type assertion (ResourceList probes r.plugin.(plugin.HealthReporter)),
// like Sortable and DrillDowner — plugins that don't implement it simply get no
// row tinting.
type HealthReporter interface {
	RowHealth(obj *unstructured.Unstructured) Health
}

// WorkloadHealth applies the replica-shortfall rule shared by workload plugins:
// a shortfall of ready replicas relative to desired is a transitional state
// (yellow). It never returns Error; per-type failure signals (red) are added by
// the individual plugins on top of this result.
func WorkloadHealth(ready, desired int32) Health {
	if desired == 0 {
		return Healthy
	}
	if ready >= desired {
		return Healthy
	}
	return Warning
}
