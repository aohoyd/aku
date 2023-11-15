package k8s

import (
	"context"
	"encoding/json"
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/aohoyd/aku/internal/msgs"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
)

// buildImagePatch constructs a strategic merge patch JSON for changing container images.
// pluginName determines the path: "pods" uses spec.containers, workloads use spec.template.spec.containers.
func buildImagePatch(pluginName string, images []msgs.ContainerImageChange) ([]byte, error) {
	var regular, init []map[string]string
	for _, img := range images {
		entry := map[string]string{"name": img.Name, "image": img.Image}
		if img.Init {
			init = append(init, entry)
		} else {
			regular = append(regular, entry)
		}
	}

	innerSpec := map[string]any{}
	if len(regular) > 0 {
		innerSpec["containers"] = regular
	}
	if len(init) > 0 {
		innerSpec["initContainers"] = init
	}

	var patch map[string]any
	if pluginName == "pods" {
		patch = map[string]any{"spec": innerSpec}
	} else {
		// deployments, statefulsets, daemonsets
		patch = map[string]any{
			"spec": map[string]any{
				"template": map[string]any{
					"spec": innerSpec,
				},
			},
		}
	}

	return json.Marshal(patch)
}

// SetImageCmd returns a tea.Cmd that applies a strategic merge patch to change container images.
func SetImageCmd(dynClient dynamic.Interface, gvr schema.GroupVersionResource,
	name, namespace, pluginName string, images []msgs.ContainerImageChange) tea.Cmd {
	return func() tea.Msg {
		patch, err := buildImagePatch(pluginName, images)
		if err != nil {
			return msgs.ActionResultMsg{Err: fmt.Errorf("build patch: %w", err)}
		}

		_, err = dynClient.Resource(gvr).Namespace(namespace).Patch(
			context.Background(), name, types.StrategicMergePatchType, patch, metav1.PatchOptions{})
		if err != nil {
			return msgs.ActionResultMsg{Err: err}
		}
		return msgs.ActionResultMsg{ActionID: "set-image:" + name}
	}
}
