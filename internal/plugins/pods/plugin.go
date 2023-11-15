package pods

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/plugins/containers"
	"github.com/aohoyd/aku/internal/plugins/workload"
	"github.com/aohoyd/aku/internal/render"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	gvr           = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
	configMapsGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}
	secretsGVR    = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "secrets"}
)

// Plugin implements plugin.ResourcePlugin for Kubernetes Pods.
type Plugin struct {
	store *k8s.Store
}

// New creates a new Pod plugin.
func New(_ *k8s.Client, store *k8s.Store) plugin.ResourcePlugin {
	return &Plugin{store: store}
}

func (p *Plugin) Name() string                     { return "pods" }
func (p *Plugin) ShortName() string                { return "po" }
func (p *Plugin) GVR() schema.GroupVersionResource { return gvr }
func (p *Plugin) IsClusterScoped() bool            { return false }

func (p *Plugin) Columns() []plugin.Column {
	return []plugin.Column{
		{Title: "NAME", Flex: true},
		{Title: "READY", Width: 8},
		{Title: "STATUS", Width: 16},
		{Title: "RESTARTS", Width: 10},
		{Title: "AGE", Width: 8},
	}
}

func (p *Plugin) Row(obj *unstructured.Unstructured) []string {
	name := obj.GetName()
	status := renderStatus(extractPodPhase(obj))
	ready := renderReady(obj)
	restarts := extractRestarts(obj)
	age := render.FormatAge(obj)
	return []string{name, ready, status, restarts, age}
}

func (p *Plugin) YAML(obj *unstructured.Unstructured) (render.Content, error) {
	return plugin.MarshalYAML(obj)
}

func (p *Plugin) Describe(ctx context.Context, obj *unstructured.Unstructured) (render.Content, error) {
	pod, err := toPod(obj)
	if err != nil {
		return render.Content{}, fmt.Errorf("pods: decode: %w", err)
	}
	return p.renderDescribe(pod, nil, nil)
}

func (p *Plugin) DescribeUncovered(ctx context.Context, obj *unstructured.Unstructured) (render.Content, error) {
	pod, err := toPod(obj)
	if err != nil {
		return render.Content{}, fmt.Errorf("pods: decode: %w", err)
	}
	if p.store == nil {
		return p.renderDescribe(pod, nil, nil)
	}
	ns := pod.Namespace
	p.store.Subscribe(configMapsGVR, ns)
	p.store.Subscribe(secretsGVR, ns)
	return p.renderDescribe(pod, p.store.List(configMapsGVR, ns), p.store.List(secretsGVR, ns))
}

// DrillDown implements plugin.DrillDowner.
func (p *Plugin) DrillDown(obj *unstructured.Unstructured) (plugin.ResourcePlugin, []*unstructured.Unstructured) {
	cp, ok := plugin.ByName("containers")
	if !ok {
		return nil, nil
	}
	children := containers.ExtractContainers(obj)
	return cp, children
}

// renderDescribe produces the full describe output using the typed pod and optional lookup.
func (p *Plugin) renderDescribe(pod *corev1.Pod, configMaps, secrets []*unstructured.Unstructured) (render.Content, error) {
	phase := computePodStatus(pod.Status, len(pod.Spec.InitContainers), pod.DeletionTimestamp != nil)

	b := render.NewBuilder()

	// Preamble fields
	b.KV(render.LEVEL_0, "Name", pod.Name)
	b.KV(render.LEVEL_0, "Namespace", pod.Namespace)

	if pod.Spec.Priority != nil {
		b.KV(render.LEVEL_0, "Priority", fmt.Sprintf("%d", *pod.Spec.Priority))
	}
	if pod.Spec.PriorityClassName != "" {
		b.KV(render.LEVEL_0, "Priority Class Name", pod.Spec.PriorityClassName)
	}

	serviceAccount := pod.Spec.ServiceAccountName
	if serviceAccount == "" {
		serviceAccount = "<none>"
	}
	b.KV(render.LEVEL_0, "Service Account", serviceAccount)

	nodeName := pod.Spec.NodeName
	if nodeName == "" {
		nodeName = "<none>"
	}
	b.KV(render.LEVEL_0, "Node", nodeName)

	// Start Time: use pod.Status.StartTime if available, else creationTimestamp
	if pod.Status.StartTime != nil {
		b.KV(render.LEVEL_0, "Start Time", pod.Status.StartTime.Format(time.RFC1123Z))
	} else {
		b.KV(render.LEVEL_0, "Start Time", pod.CreationTimestamp.Format(time.RFC1123Z))
	}

	// Labels and Annotations
	b.KVMulti(render.LEVEL_0, "Labels", pod.Labels)
	b.KVMulti(render.LEVEL_0, "Annotations", pod.Annotations)

	b.KVStyled(render.LEVEL_0, render.StatusKind(phase), "Status", phase)

	podIP := pod.Status.PodIP
	if podIP == "" {
		podIP = "<none>"
	}
	b.KV(render.LEVEL_0, "IP", podIP)

	// IPs (podIPs list)
	if len(pod.Status.PodIPs) > 0 {
		var ipStrs []string
		for _, pip := range pod.Status.PodIPs {
			if pip.IP != "" {
				ipStrs = append(ipStrs, pip.IP)
			}
		}
		if len(ipStrs) > 0 {
			b.KV(render.LEVEL_0, "IPs", strings.Join(ipStrs, ", "))
		}
	}

	// Controlled By from ownerReferences
	if len(pod.OwnerReferences) > 0 {
		ref := pod.OwnerReferences[0]
		b.KV(render.LEVEL_0, "Controlled By", ref.Kind+"/"+ref.Name)
	}

	// QoS Class
	if pod.Status.QOSClass != "" {
		b.KV(render.LEVEL_0, "QoS Class", string(pod.Status.QOSClass))
	}

	// Container sections
	statusMap := buildContainerStatusMap(pod)
	describeContainerSection(b, "Containers", pod.Spec.Containers, pod, configMaps, secrets, statusMap)
	describeContainerSection(b, "Init Containers", pod.Spec.InitContainers, pod, configMaps, secrets, statusMap)
	describeContainerSection(b, "Ephemeral Containers", ephemeralToContainers(pod.Spec.EphemeralContainers), pod, configMaps, secrets, statusMap)

	// Conditions
	workload.DescribeConditions(b, pod)

	// Volumes section
	if len(pod.Spec.Volumes) > 0 {
		b.Section(render.LEVEL_0, "Volumes")
		for _, vol := range pod.Spec.Volumes {
			b.Section(render.LEVEL_1, vol.Name)
			describeVolume(b, vol)
		}
	}

	// Node-Selectors section
	if len(pod.Spec.NodeSelector) > 0 {
		b.Section(render.LEVEL_0, "Node-Selectors")
		b.KVMulti(render.LEVEL_1, "", pod.Spec.NodeSelector)
	}

	// Tolerations section
	if len(pod.Spec.Tolerations) > 0 {
		t := make(map[string]string, len(pod.Spec.Tolerations))
		for _, tol := range pod.Spec.Tolerations {
			if ft := formatToleration(tol); ft != "" {
				t[ft] = ""
			}
		}
		b.KVMulti(render.LEVEL_0, "Tolerations", t)
	}

	return b.Build(), nil
}

func describeContainerSection(
	b *render.Builder,
	section string,
	ctrs []corev1.Container,
	pod *corev1.Pod,
	configMaps, secrets []*unstructured.Unstructured,
	statusMap map[string]corev1.ContainerStatus,
) {
	if len(ctrs) == 0 {
		return
	}
	b.Section(render.LEVEL_0, section)
	for i := range ctrs {
		c := &ctrs[i]
		b.Section(render.LEVEL_1, c.Name)
		var status *corev1.ContainerStatus
		if cs, ok := statusMap[c.Name]; ok {
			status = &cs
		}
		containers.DescribeContainer(b, render.LEVEL_2, *c, status, pod, configMaps, secrets)
	}
}

// ephemeralToContainers converts EphemeralContainers to regular Container type
// so they can be rendered by the same container section function.
func ephemeralToContainers(ecs []corev1.EphemeralContainer) []corev1.Container {
	containers := make([]corev1.Container, len(ecs))
	for i, ec := range ecs {
		containers[i] = corev1.Container(ec.EphemeralContainerCommon)
	}
	return containers
}

// buildContainerStatusMap builds a lookup map from container name to its status.
func buildContainerStatusMap(pod *corev1.Pod) map[string]corev1.ContainerStatus {
	m := make(map[string]corev1.ContainerStatus)
	for _, cs := range pod.Status.ContainerStatuses {
		m[cs.Name] = cs
	}
	for _, cs := range pod.Status.InitContainerStatuses {
		m[cs.Name] = cs
	}
	for _, cs := range pod.Status.EphemeralContainerStatuses {
		m[cs.Name] = cs
	}
	return m
}

// describeVolume writes volume type and fields to the Builder.
func describeVolume(b *render.Builder, vol corev1.Volume) {
	switch {
	case vol.ConfigMap != nil:
		b.KV(render.LEVEL_2, "Type", "ConfigMap (a volume populated by a ConfigMap)")
		b.KV(render.LEVEL_2, "Name", vol.ConfigMap.Name)
	case vol.Secret != nil:
		b.KV(render.LEVEL_2, "Type", "Secret (a volume populated by a Secret)")
		b.KV(render.LEVEL_2, "SecretName", vol.Secret.SecretName)
	case vol.EmptyDir != nil:
		b.KV(render.LEVEL_2, "Type", "EmptyDir (a temporary directory that shares a pod's lifetime)")
	case vol.HostPath != nil:
		b.KV(render.LEVEL_2, "Type", "HostPath (bare host directory volume)")
		b.KV(render.LEVEL_2, "Path", vol.HostPath.Path)
	case vol.PersistentVolumeClaim != nil:
		b.KV(render.LEVEL_2, "Type", "PersistentVolumeClaim")
		b.KV(render.LEVEL_2, "ClaimName", vol.PersistentVolumeClaim.ClaimName)
	case vol.Projected != nil:
		b.KV(render.LEVEL_2, "Type", "Projected (a volume that contains injected data from multiple sources)")
	default:
		b.KV(render.LEVEL_2, "Type", "<unknown>")
	}
}

// formatToleration formats a typed toleration for display.
func formatToleration(tol corev1.Toleration) string {
	var s string
	if tol.Operator == corev1.TolerationOpExists {
		s = tol.Key
	} else {
		if tol.Value != "" {
			s = tol.Key + "=" + tol.Value
		} else {
			s = tol.Key
		}
	}
	if tol.Effect != "" {
		s += ":" + string(tol.Effect)
	}
	if tol.TolerationSeconds != nil {
		s += fmt.Sprintf(" for %ds", *tol.TolerationSeconds)
	}
	return s
}

// SortValue implements plugin.Sortable.
// Returns a custom sort key only for STATUS; built-in handles NAME and AGE.
func (p *Plugin) SortValue(obj *unstructured.Unstructured, column string) string {
	if column == "STATUS" {
		return extractPodPhase(obj)
	}
	return ""
}

// toPod converts an unstructured object to a typed corev1.Pod.
func toPod(obj *unstructured.Unstructured) (*corev1.Pod, error) {
	var pod corev1.Pod
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &pod); err != nil {
		return nil, err
	}
	return &pod, nil
}

func toPodStatus(obj *unstructured.Unstructured) corev1.PodStatus {
	statusRaw, _, _ := unstructured.NestedMap(obj.Object, "status")
	var ps corev1.PodStatus
	_ = runtime.DefaultUnstructuredConverter.FromUnstructured(statusRaw, &ps)
	return ps
}

func computePodStatus(status corev1.PodStatus, initTotal int, deleted bool) string {
	reason := string(status.Phase)
	if status.Reason != "" {
		reason = status.Reason
	}
	if reason == "" {
		reason = "Unknown"
	}

	initializing := false
	for i, cs := range status.InitContainerStatuses {
		switch {
		case cs.State.Terminated != nil && cs.State.Terminated.ExitCode == 0:
			continue
		case cs.State.Terminated != nil:
			if cs.State.Terminated.Reason != "" {
				reason = "Init:" + cs.State.Terminated.Reason
			} else if cs.State.Terminated.Signal != 0 {
				reason = fmt.Sprintf("Init:Signal:%d", cs.State.Terminated.Signal)
			} else {
				reason = fmt.Sprintf("Init:ExitCode:%d", cs.State.Terminated.ExitCode)
			}
			initializing = true
		case cs.State.Waiting != nil && cs.State.Waiting.Reason != "" && cs.State.Waiting.Reason != "PodInitializing":
			reason = "Init:" + cs.State.Waiting.Reason
			initializing = true
		default:
			reason = fmt.Sprintf("Init:%d/%d", i, initTotal)
			initializing = true
		}
		break
	}

	if !initializing {
		hasRunning := false
		for i := len(status.ContainerStatuses) - 1; i >= 0; i-- {
			cs := status.ContainerStatuses[i]
			switch {
			case cs.State.Waiting != nil && cs.State.Waiting.Reason != "":
				reason = cs.State.Waiting.Reason
			case cs.State.Terminated != nil && cs.State.Terminated.Reason != "":
				reason = cs.State.Terminated.Reason
			case cs.State.Terminated != nil:
				if cs.State.Terminated.Signal != 0 {
					reason = fmt.Sprintf("Signal:%d", cs.State.Terminated.Signal)
				} else {
					reason = fmt.Sprintf("ExitCode:%d", cs.State.Terminated.ExitCode)
				}
			case cs.State.Running != nil:
				hasRunning = true
			}
		}

		if reason == "Completed" && hasRunning {
			if hasPodReadyCondition(status.Conditions) {
				reason = "Running"
			} else {
				reason = "NotReady"
			}
		}
	}

	if deleted {
		reason = "Terminating"
	}

	return reason
}

func hasPodReadyCondition(conditions []corev1.PodCondition) bool {
	for _, c := range conditions {
		if c.Type == corev1.PodReady && c.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func extractPodPhase(obj *unstructured.Unstructured) string {
	initSpecs, _, _ := unstructured.NestedSlice(obj.Object, "spec", "initContainers")
	return computePodStatus(toPodStatus(obj), len(initSpecs), obj.GetDeletionTimestamp() != nil)
}

func extractRestarts(obj *unstructured.Unstructured) string {
	containerStatuses, found, _ := unstructured.NestedSlice(obj.Object, "status", "containerStatuses")
	if !found {
		return "0"
	}

	total := int64(0)
	for _, cs := range containerStatuses {
		csMap, ok := cs.(map[string]any)
		if !ok {
			continue
		}
		count, _, _ := unstructured.NestedInt64(csMap, "restartCount")
		total += count
	}
	return fmt.Sprintf("%d", total)
}
