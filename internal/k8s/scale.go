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

// buildScalePatch constructs a strategic merge patch JSON for scaling replicas.
func buildScalePatch(replicas int32) ([]byte, error) {
	patch := map[string]any{
		"spec": map[string]any{
			"replicas": replicas,
		},
	}
	return json.Marshal(patch)
}

// ScaleCmd returns a tea.Cmd that patches the replica count of a resource.
func ScaleCmd(dynClient dynamic.Interface, gvr schema.GroupVersionResource,
	name, namespace string, replicas int32) tea.Cmd {
	return func() tea.Msg {
		patch, err := buildScalePatch(replicas)
		if err != nil {
			return msgs.ActionResultMsg{Err: fmt.Errorf("build patch: %w", err)}
		}

		_, err = dynClient.Resource(gvr).Namespace(namespace).Patch(
			context.Background(), name, types.StrategicMergePatchType, patch, metav1.PatchOptions{})
		if err != nil {
			return msgs.ActionResultMsg{Err: err}
		}
		return msgs.ActionResultMsg{ActionID: "scale:" + name}
	}
}
