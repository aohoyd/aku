package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/aohoyd/aku/internal/msgs"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
)

// RestartTarget identifies a single resource to restart.
type RestartTarget struct {
	Name      string
	Namespace string
}

// buildRestartPatch constructs a JSON merge patch that sets
// spec.template.metadata.annotations["kubectl.kubernetes.io/restartedAt"] to ts.
func buildRestartPatch(ts string) ([]byte, error) {
	patch := map[string]any{
		"spec": map[string]any{
			"template": map[string]any{
				"metadata": map[string]any{
					"annotations": map[string]any{
						"kubectl.kubernetes.io/restartedAt": ts,
					},
				},
			},
		},
	}
	return json.Marshal(patch)
}

// RestartCmd returns a tea.Cmd that applies a JSON merge patch to set the
// kubectl.kubernetes.io/restartedAt annotation on each target's pod template,
// triggering a rolling restart. All targets share a single timestamp captured
// once before the loop, mirroring `kubectl rollout restart` semantics.
func RestartCmd(dynClient dynamic.Interface, gvr schema.GroupVersionResource, targets []RestartTarget) tea.Cmd {
	return func() tea.Msg {
		ts := time.Now().UTC().Format(time.RFC3339)
		patch, err := buildRestartPatch(ts)
		if err != nil {
			return msgs.ActionResultMsg{Err: fmt.Errorf("build patch: %w", err)}
		}

		var errs []string
		for _, t := range targets {
			_, err := dynClient.Resource(gvr).Namespace(t.Namespace).Patch(
				context.Background(), t.Name, types.MergePatchType, patch, metav1.PatchOptions{})
			if err != nil {
				errs = append(errs, t.Name+": "+err.Error())
			}
		}
		if len(errs) > 0 {
			return msgs.ActionResultMsg{Err: fmt.Errorf("bulk restart: %d/%d failed: %s", len(errs), len(targets), strings.Join(errs, "; "))}
		}
		if len(targets) == 1 {
			return msgs.ActionResultMsg{ActionID: "restart:" + targets[0].Name}
		}
		return msgs.ActionResultMsg{ActionID: fmt.Sprintf("restart:%d-resources", len(targets))}
	}
}
