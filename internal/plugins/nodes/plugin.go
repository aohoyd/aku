package nodes

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/plugins/workload"
	"github.com/aohoyd/aku/internal/render"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var gvr = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "nodes"}

// Plugin implements plugin.ResourcePlugin and plugin.DrillDowner for Kubernetes Nodes.
type Plugin struct {
	store *k8s.Store
}

// New creates a new Nodes plugin.
func New(_ *k8s.Client, store *k8s.Store) plugin.ResourcePlugin {
	return &Plugin{store: store}
}

func (p *Plugin) Name() string                     { return "nodes" }
func (p *Plugin) ShortName() string                { return "no" }
func (p *Plugin) GVR() schema.GroupVersionResource { return gvr }
func (p *Plugin) IsClusterScoped() bool            { return true }

func (p *Plugin) Columns() []plugin.Column {
	return []plugin.Column{
		{Title: "NAME", Flex: true},
		{Title: "STATUS", Width: 16},
		{Title: "ROLES", Width: 16},
		{Title: "VERSION", Width: 14},
		{Title: "AGE", Width: 8},
	}
}

func (p *Plugin) Row(obj *unstructured.Unstructured) []string {
	name := obj.GetName()
	status := nodeReadyStatus(obj)
	roles := nodeRoles(obj)
	version, _, _ := unstructured.NestedString(obj.Object, "status", "nodeInfo", "kubeletVersion")
	age := render.FormatAge(obj)
	return []string{name, status, roles, version, age}
}

func (p *Plugin) YAML(obj *unstructured.Unstructured) (render.Content, error) {
	return plugin.MarshalYAML(obj)
}

func (p *Plugin) Describe(ctx context.Context, obj *unstructured.Unstructured) (render.Content, error) {
	node, err := toNode(obj)
	if err != nil {
		return render.Content{}, fmt.Errorf("converting to Node: %w", err)
	}

	b := render.NewBuilder()

	// Metadata
	b.KV(render.LEVEL_0, "Name", node.Name)
	b.KV(render.LEVEL_0, "CreationTimestamp", render.FormatAge(obj))
	b.KVMulti(render.LEVEL_0, "Labels", node.Labels)
	b.KVMulti(render.LEVEL_0, "Annotations", node.Annotations)

	// Conditions
	if len(node.Status.Conditions) > 0 {
		b.Section(render.LEVEL_0, "Conditions")
		for _, cond := range node.Status.Conditions {
			b.KVStyled(render.LEVEL_1, render.ConditionKind(string(cond.Status)), string(cond.Type), string(cond.Status))
			if cond.Reason != "" {
				b.KV(render.LEVEL_2, "Reason", cond.Reason)
			}
			if cond.Message != "" {
				b.KV(render.LEVEL_2, "Message", cond.Message)
			}
		}
	}

	// Addresses
	if len(node.Status.Addresses) > 0 {
		b.Section(render.LEVEL_0, "Addresses")
		for _, addr := range node.Status.Addresses {
			b.KV(render.LEVEL_1, string(addr.Type), addr.Address)
		}
	}

	// Capacity
	if len(node.Status.Capacity) > 0 {
		b.Section(render.LEVEL_0, "Capacity")
		for _, key := range sortedResourceNames(node.Status.Capacity) {
			qty := node.Status.Capacity[key]
			b.KV(render.LEVEL_1, string(key), qty.String())
		}
	}

	// Allocatable
	if len(node.Status.Allocatable) > 0 {
		b.Section(render.LEVEL_0, "Allocatable")
		for _, key := range sortedResourceNames(node.Status.Allocatable) {
			qty := node.Status.Allocatable[key]
			b.KV(render.LEVEL_1, string(key), qty.String())
		}
	}

	// System Info
	info := node.Status.NodeInfo
	b.Section(render.LEVEL_0, "System Info")
	b.KV(render.LEVEL_1, "Operating System", info.OperatingSystem)
	b.KV(render.LEVEL_1, "Architecture", info.Architecture)
	b.KV(render.LEVEL_1, "Kernel Version", info.KernelVersion)
	b.KV(render.LEVEL_1, "Container Runtime", info.ContainerRuntimeVersion)
	b.KV(render.LEVEL_1, "Kubelet Version", info.KubeletVersion)

	// Taints
	if len(node.Spec.Taints) > 0 {
		b.Section(render.LEVEL_0, "Taints")
		for _, taint := range node.Spec.Taints {
			val := taint.Key
			if taint.Value != "" {
				val += "=" + taint.Value
			}
			val += ":" + string(taint.Effect)
			b.KV(render.LEVEL_1, val, "")
		}
	}

	return b.Build(), nil
}

// DrillDown implements plugin.DrillDowner.
func (p *Plugin) DrillDown(obj *unstructured.Unstructured) (plugin.ResourcePlugin, []*unstructured.Unstructured) {
	if p.store == nil {
		return nil, nil
	}
	pp, ok := plugin.ByName("pods")
	if !ok {
		return nil, nil
	}
	p.store.Subscribe(workload.PodsGVR, "") // all-namespaces
	pods := workload.FindPodsByNodeName(p.store, obj.GetName())
	return pp, pods
}

// nodeReadyStatus extracts the Ready condition status from a node object.
func nodeReadyStatus(obj *unstructured.Unstructured) string {
	conditions, found, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if !found {
		return "Unknown"
	}
	for _, c := range conditions {
		cMap, ok := c.(map[string]any)
		if !ok {
			continue
		}
		cType, _ := cMap["type"].(string)
		if cType == "Ready" {
			status, _ := cMap["status"].(string)
			switch status {
			case "True":
				return "Ready"
			case "False":
				return "NotReady"
			default:
				return "Unknown"
			}
		}
	}
	return "Unknown"
}

// nodeRoles extracts node roles from labels matching "node-role.kubernetes.io/*".
func nodeRoles(obj *unstructured.Unstructured) string {
	labels := obj.GetLabels()
	if len(labels) == 0 {
		return "<none>"
	}

	const prefix = "node-role.kubernetes.io/"
	var roles []string
	for k := range labels {
		if after, ok := strings.CutPrefix(k, prefix); ok {
			role := after
			if role != "" {
				roles = append(roles, role)
			}
		}
	}
	if len(roles) == 0 {
		return "<none>"
	}
	slices.Sort(roles)
	return strings.Join(roles, ",")
}

// toNode converts an unstructured object to a typed corev1.Node.
func toNode(obj *unstructured.Unstructured) (*corev1.Node, error) {
	var node corev1.Node
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &node); err != nil {
		return nil, err
	}
	return &node, nil
}

// sortedResourceNames returns resource names from a ResourceList in sorted order.
func sortedResourceNames(rl corev1.ResourceList) []corev1.ResourceName {
	keys := make([]string, 0, len(rl))
	for k := range rl {
		keys = append(keys, string(k))
	}
	slices.Sort(keys)
	result := make([]corev1.ResourceName, len(keys))
	for i, k := range keys {
		result[i] = corev1.ResourceName(k)
	}
	return result
}
