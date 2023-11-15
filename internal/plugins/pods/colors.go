package pods

import (
	"fmt"

	"github.com/aohoyd/aku/internal/plugin"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// renderStatus delegates to the shared status color renderer.
func renderStatus(phase string) string {
	return plugin.RenderStatus(phase)
}

// renderReady returns a classic "ready/total" string (e.g. "1/1", "0/3").
func renderReady(obj *unstructured.Unstructured) string {
	containers, _, _ := unstructured.NestedSlice(obj.Object, "spec", "containers")
	total := len(containers)

	containerStatuses, found, _ := unstructured.NestedSlice(obj.Object, "status", "containerStatuses")
	if !found {
		return fmt.Sprintf("0/%d", total)
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
	return fmt.Sprintf("%d/%d", ready, total)
}
