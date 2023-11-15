package containers

import (
	"context"
	"fmt"

	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/render"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	sentinelGVR   = schema.GroupVersionResource{Group: "_ktui", Version: "v1", Resource: "containers"}
	configMapsGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}
	secretsGVR    = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "secrets"}
)

// Plugin implements plugin.ResourcePlugin for container drill-down views.
type Plugin struct {
	store *k8s.Store
}

// New creates a new container plugin.
func New(_ *k8s.Client, store *k8s.Store) plugin.ResourcePlugin {
	return &Plugin{store: store}
}

func (p *Plugin) Name() string                            { return "containers" }
func (p *Plugin) ShortName() string                       { return "co" }
func (p *Plugin) GVR() schema.GroupVersionResource        { return sentinelGVR }
func (p *Plugin) IsClusterScoped() bool                   { return false }

func (p *Plugin) Columns() []plugin.Column {
	return []plugin.Column{
		{Title: "NAME", Flex: true},
		{Title: "TYPE", Width: 10},
		{Title: "IMAGE", Flex: true},
		{Title: "STATUS", Width: 16},
		{Title: "READY", Width: 7},
		{Title: "RESTARTS", Width: 10},
	}
}

func (p *Plugin) Row(obj *unstructured.Unstructured) []string {
	name := obj.GetName()
	ctype, _, _ := unstructured.NestedString(obj.Object, "_type")

	spec, _ := obj.Object["_spec"].(map[string]any)
	image, _ := spec["image"].(string)

	status := extractStatus(obj)
	ready := extractReady(obj)
	restarts := extractRestarts(obj)

	return []string{name, ctype, image, plugin.RenderStatus(status), ready, restarts}
}

func (p *Plugin) YAML(obj *unstructured.Unstructured) (render.Content, error) {
	spec, _ := obj.Object["_spec"].(map[string]any)
	if spec == nil {
		return render.Content{}, fmt.Errorf("no container spec")
	}
	return render.YAML(spec)
}

func (p *Plugin) Describe(ctx context.Context, obj *unstructured.Unstructured) (render.Content, error) {
	pod, _ := p.extractPod(obj)
	return p.renderDescribe(obj, pod, nil, nil)
}

// DescribeUncovered implements plugin.Uncoverable.
func (p *Plugin) DescribeUncovered(ctx context.Context, obj *unstructured.Unstructured) (render.Content, error) {
	pod, err := p.extractPod(obj)
	if err != nil {
		return p.renderDescribe(obj, nil, nil, nil)
	}
	if p.store == nil {
		return p.renderDescribe(obj, pod, nil, nil)
	}
	ns := obj.GetNamespace()
	p.store.Subscribe(configMapsGVR, ns)
	p.store.Subscribe(secretsGVR, ns)
	return p.renderDescribe(obj, pod, p.store.List(configMapsGVR, ns), p.store.List(secretsGVR, ns))
}



// SortValue implements plugin.Sortable.
func (p *Plugin) SortValue(obj *unstructured.Unstructured, column string) string {
	switch column {
	case "STATUS":
		return extractStatus(obj)
	case "TYPE":
		ctype, _, _ := unstructured.NestedString(obj.Object, "_type")
		return ctype
	}
	return ""
}

// podName extracts the pod name from the embedded _pod object.
func podName(obj *unstructured.Unstructured) string {
	podObj, _ := obj.Object["_pod"].(map[string]any)
	if podObj == nil {
		return ""
	}
	name, _, _ := unstructured.NestedString(podObj, "metadata", "name")
	return name
}

// renderDescribe produces container describe output with optional resolution.
func (p *Plugin) renderDescribe(
	obj *unstructured.Unstructured,
	pod *corev1.Pod,
	configMaps, secrets []*unstructured.Unstructured,
) (render.Content, error) {
	specMap, _ := obj.Object["_spec"].(map[string]any)
	statusMap, _ := obj.Object["_status"].(map[string]any)
	ctype, _, _ := unstructured.NestedString(obj.Object, "_type")

	var container corev1.Container
	if specMap != nil {
		runtime.DefaultUnstructuredConverter.FromUnstructured(specMap, &container)
	}

	b := render.NewBuilder()
	b.KV(render.LEVEL_0, "Name", obj.GetName())
	b.KV(render.LEVEL_0, "Pod", podName(obj))
	b.KV(render.LEVEL_0, "Namespace", obj.GetNamespace())
	b.KV(render.LEVEL_0, "Type", ctype)

	var status *corev1.ContainerStatus
	if statusMap != nil {
		var cs corev1.ContainerStatus
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(statusMap, &cs); err == nil {
			status = &cs
		}
	}
	DescribeContainer(b, render.LEVEL_0, container, status, pod, configMaps, secrets)

	return b.Build(), nil
}

// extractPod converts the embedded _pod map to a typed corev1.Pod.
func (p *Plugin) extractPod(obj *unstructured.Unstructured) (*corev1.Pod, error) {
	podObj, _ := obj.Object["_pod"].(map[string]any)
	if podObj == nil {
		return nil, fmt.Errorf("no embedded pod")
	}
	var pod corev1.Pod
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(podObj, &pod); err != nil {
		return nil, fmt.Errorf("containers: decode pod: %w", err)
	}
	return &pod, nil
}

// --- status helpers ---

func extractStatus(obj *unstructured.Unstructured) string {
	statusMap, _ := obj.Object["_status"].(map[string]any)
	if statusMap == nil {
		return "Pending"
	}
	if _, found, _ := unstructured.NestedMap(statusMap, "state", "running"); found {
		return "Running"
	}
	if waiting, found, _ := unstructured.NestedMap(statusMap, "state", "waiting"); found {
		if reason, ok := waiting["reason"].(string); ok && reason != "" {
			return reason
		}
		return "Waiting"
	}
	if terminated, found, _ := unstructured.NestedMap(statusMap, "state", "terminated"); found {
		if reason, ok := terminated["reason"].(string); ok && reason != "" {
			return reason
		}
		return "Terminated"
	}
	return "Unknown"
}

func extractReady(obj *unstructured.Unstructured) string {
	statusMap, _ := obj.Object["_status"].(map[string]any)
	if statusMap == nil {
		return "false"
	}
	ready, _, _ := unstructured.NestedBool(statusMap, "ready")
	return fmt.Sprintf("%v", ready)
}

func extractRestarts(obj *unstructured.Unstructured) string {
	statusMap, _ := obj.Object["_status"].(map[string]any)
	if statusMap == nil {
		return "0"
	}
	count, _, _ := unstructured.NestedInt64(statusMap, "restartCount")
	return fmt.Sprintf("%d", count)
}
