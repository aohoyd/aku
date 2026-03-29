package pods

import (
	"github.com/aohoyd/aku/internal/plugin"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// renderStatus returns the phase string colored for display.
// When the phase is "Running" and notFullyReady is true, it uses
// the failed (red) color instead of the running (green) color.
func renderStatus(phase string, notFullyReady bool) string {
	if phase == "Running" && notFullyReady {
		return plugin.StyledFg(phase, plugin.FgFailed)
	}
	return plugin.RenderStatus(phase)
}

// readyCount returns (ready, total) container counts from the pod object.
func readyCount(obj *unstructured.Unstructured) (int, int) {
	containers, _, _ := unstructured.NestedSlice(obj.Object, "spec", "containers")
	total := len(containers)

	containerStatuses, found, _ := unstructured.NestedSlice(obj.Object, "status", "containerStatuses")
	if !found {
		return 0, total
	}

	ready := 0
	for _, cs := range containerStatuses {
		csMap, ok := cs.(map[string]any)
		if !ok {
			continue
		}
		isReady, _, _ := unstructured.NestedBool(csMap, "ready")
		if isReady {
			ready++
		}
	}
	return ready, total
}
