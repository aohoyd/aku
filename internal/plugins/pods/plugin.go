package pods

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

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
type Plugin struct{}

// New creates a new Pod plugin.
func New() plugin.ResourcePlugin {
	return &Plugin{}
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
		{Title: "IP", Width: 16},
		{Title: "AGE", Width: 8},
	}
}

func (p *Plugin) Row(obj *unstructured.Unstructured) []string {
	name := obj.GetName()
	phase := extractPodPhase(obj)
	ready, total := readyCount(obj)
	status := renderStatus(phase, phase == "Running" && ready < total)
	readyStr := fmt.Sprintf("%d/%d", ready, total)
	restarts := extractRestarts(obj)
	podIP, _, _ := unstructured.NestedString(obj.Object, "status", "podIP")
	if podIP == "" {
		podIP = "<none>"
	}
	age := render.FormatAge(obj)
	return []string{name, readyStr, status, restarts, podIP, age}
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

func (p *Plugin) DescribeUncovered(ctx context.Context, cl plugin.Cluster, obj *unstructured.Unstructured) (render.Content, error) {
	pod, err := toPod(obj)
	if err != nil {
		return render.Content{}, fmt.Errorf("pods: decode: %w", err)
	}
	store := plugin.StoreOf(cl)
	if store == nil {
		return p.renderDescribe(pod, nil, nil)
	}
	ns := pod.Namespace
	store.Subscribe(configMapsGVR, ns)
	store.Subscribe(secretsGVR, ns)
	return p.renderDescribe(pod, store.List(configMapsGVR, ns), store.List(secretsGVR, ns))
}

// DrillDown implements plugin.DrillDowner.
func (p *Plugin) DrillDown(_ plugin.Cluster, obj *unstructured.Unstructured) (plugin.ResourcePlugin, []*unstructured.Unstructured) {
	cp, ok := plugin.ByName("containers")
	if !ok {
		return nil, nil
	}
	children := containers.ExtractContainers(obj)
	return cp, children
}

// DrillUp implements plugin.DrillUp. A pod's owner may be a ReplicaSet,
// StatefulSet, DaemonSet, or Job; the shared ownerReference helper resolves all
// of them.
func (p *Plugin) DrillUp(cl plugin.Cluster, obj *unstructured.Unstructured) (plugin.ResourcePlugin, *unstructured.Unstructured) {
	return workload.FindParentByOwnerRef(cl, obj)
}

// GoToNode implements plugin.NodeLinker. It resolves the Node a pod is scheduled
// onto via the shared spec.nodeName helper.
func (p *Plugin) GoToNode(cl plugin.Cluster, obj *unstructured.Unstructured) (plugin.ResourcePlugin, *unstructured.Unstructured) {
	return workload.FindNodeForPod(cl, obj)
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

	// Lookup indexes shared by the Volumes and Image Pull Secrets sections.
	// Empty (plain Describe) → names only, no value leak.
	cms := containers.IndexByName(configMaps)
	secs := containers.IndexByName(secrets)

	// Volumes section
	if len(pod.Spec.Volumes) > 0 {
		b.Section(render.LEVEL_0, "Volumes")
		for _, vol := range pod.Spec.Volumes {
			b.Section(render.LEVEL_1, vol.Name)
			describeVolume(b, vol, cms, secs)
		}
	}

	// Image Pull Secrets section. In uncover mode (secs populated) reveal
	// registry → username only — never the password or auth token.
	if len(pod.Spec.ImagePullSecrets) > 0 {
		b.Section(render.LEVEL_0, "Image Pull Secrets")
		for _, ref := range pod.Spec.ImagePullSecrets {
			b.Section(render.LEVEL_1, ref.Name)
			describeImagePullSecret(b, secs[ref.Name])
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

// describeVolume writes volume type and fields to the Builder. When cms/secs
// contain the referenced object, ConfigMap/Secret volume data is revealed at
// LEVEL_3 (honoring Items). Empty maps render names only (no value leak).
func describeVolume(b *render.Builder, vol corev1.Volume, cms, secs map[string]*unstructured.Unstructured) {
	switch {
	case vol.ConfigMap != nil:
		b.KV(render.LEVEL_2, "Type", "ConfigMap (a volume populated by a ConfigMap)")
		b.KV(render.LEVEL_2, "Name", vol.ConfigMap.Name)
		if obj, ok := cms[vol.ConfigMap.Name]; ok {
			revealVolumeData(b, containers.ConfigMapData(obj), itemKeyPaths(vol.ConfigMap.Items))
		}
	case vol.Secret != nil:
		b.KV(render.LEVEL_2, "Type", "Secret (a volume populated by a Secret)")
		b.KV(render.LEVEL_2, "SecretName", vol.Secret.SecretName)
		if obj, ok := secs[vol.Secret.SecretName]; ok {
			revealVolumeData(b, containers.SecretData(obj), itemKeyPaths(vol.Secret.Items))
		}
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
		for _, src := range vol.Projected.Sources {
			switch {
			case src.ConfigMap != nil:
				b.KV(render.LEVEL_2, "ConfigMap", src.ConfigMap.Name)
				if obj, ok := cms[src.ConfigMap.Name]; ok {
					revealVolumeData(b, containers.ConfigMapData(obj), itemKeyPaths(src.ConfigMap.Items))
				}
			case src.Secret != nil:
				b.KV(render.LEVEL_2, "Secret", src.Secret.Name)
				if obj, ok := secs[src.Secret.Name]; ok {
					revealVolumeData(b, containers.SecretData(obj), itemKeyPaths(src.Secret.Items))
				}
			case src.ServiceAccountToken != nil:
				path := src.ServiceAccountToken.Path
				if path == "" {
					path = "<token>"
				}
				b.KV(render.LEVEL_2, "ServiceAccountToken", path)
			case src.DownwardAPI != nil:
				b.KV(render.LEVEL_2, "DownwardAPI", "<field refs>")
			}
		}
	default:
		b.KV(render.LEVEL_2, "Type", "<unknown>")
	}
}

// itemKeyPaths maps a volume Items subset to key->displayPath, where displayPath
// is item.Path when set, else item.Key. Returns nil when there are no items
// (whole-object reveal).
func itemKeyPaths(items []corev1.KeyToPath) map[string]string {
	if len(items) == 0 {
		return nil
	}
	m := make(map[string]string, len(items))
	for _, it := range items {
		path := it.Path
		if path == "" {
			path = it.Key
		}
		m[it.Key] = path
	}
	return m
}

// revealVolumeData emits sorted key->value pairs at LEVEL_3. When items is
// non-nil, only those keys are emitted (displayed by their path); keys absent
// from data are skipped.
func revealVolumeData(b *render.Builder, data, items map[string]string) {
	if len(data) == 0 {
		return
	}
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		if items != nil {
			path, ok := items[k]
			if !ok {
				continue
			}
			b.KV(render.LEVEL_3, path, data[k])
			continue
		}
		b.KV(render.LEVEL_3, k, data[k])
	}
}

// describeImagePullSecret reveals the registry → username pairs of a
// docker-config pull secret. obj is the referenced Secret object, or nil when
// not available (plain Describe, or secret missing) in which case only the name
// (already emitted by the caller) is shown. Passwords and auth tokens are NEVER
// emitted. Non-docker secrets fall back to listing their decoded data keys.
func describeImagePullSecret(b *render.Builder, obj *unstructured.Unstructured) {
	if obj == nil {
		return
	}
	data := containers.SecretData(obj)
	for _, key := range []string{".dockerconfigjson", ".dockercfg"} {
		if raw, ok := data[key]; ok {
			for _, sv := range parseDockerConfig(raw) {
				b.KV(render.LEVEL_2, sv.server, sv.username)
			}
			return
		}
	}
	// Not a docker-config secret: list decoded data keys only (no values).
	emitSortedKeys(b, render.LEVEL_2, data)
}

// emitSortedKeys writes each map key (sorted) as a value-less KV at level. Used
// to surface key names without their values.
func emitSortedKeys(b *render.Builder, level int, data map[string]string) {
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		b.KV(level, k, "")
	}
}

// serverUser is a registry → username pair extracted from a docker-config
// secret. It intentionally carries no password or auth token.
type serverUser struct {
	server   string
	username string
}

// parseDockerConfig parses a dockerconfigjson or legacy dockercfg JSON string
// and returns sorted server → username pairs. It handles both the
// {"auths":{server:{...}}} shape (dockerconfigjson) and the legacy top-level
// {server:{...}} shape (dockercfg). It never returns password or auth fields.
func parseDockerConfig(raw string) []serverUser {
	type entry struct {
		Username string `json:"username"`
		Auth     string `json:"auth"`
	}

	// Detect the dockerconfigjson shape by the presence of an "auths" key —
	// even when it is empty — so {"auths":{}} is NOT mistaken for the legacy
	// flat shape (which would yield a bogus "auths" server).
	var wrapped struct {
		Auths map[string]entry `json:"auths"`
	}
	var probe map[string]json.RawMessage
	auths := map[string]entry{}
	if err := json.Unmarshal([]byte(raw), &probe); err != nil {
		return nil
	}
	if _, hasAuths := probe["auths"]; hasAuths {
		if err := json.Unmarshal([]byte(raw), &wrapped); err != nil {
			return nil
		}
		auths = wrapped.Auths
	} else {
		// Legacy dockercfg: server map at top level.
		if err := json.Unmarshal([]byte(raw), &auths); err != nil {
			return nil
		}
	}

	servers := make([]string, 0, len(auths))
	for s := range auths {
		servers = append(servers, s)
	}
	slices.Sort(servers)

	out := make([]serverUser, 0, len(servers))
	for _, s := range servers {
		username := auths[s].Username
		if username == "" {
			// Auth-only configs (GCR/ECR/GHCR) store credentials in the
			// base64 "auth" field as "username:password". Surface only the
			// username; NEVER the password.
			username = usernameFromAuth(auths[s].Auth)
		}
		out = append(out, serverUser{server: s, username: username})
	}
	return out
}

// usernameFromAuth decodes a base64 "username:password" auth string and returns
// the username portion only. Returns "" on decode failure or when no username
// is present. The password is never returned.
func usernameFromAuth(auth string) string {
	if auth == "" {
		return ""
	}
	decoded, err := base64.StdEncoding.DecodeString(auth)
	if err != nil {
		// Some configs use URL-safe base64; fall back before giving up.
		decoded, err = base64.URLEncoding.DecodeString(auth)
		if err != nil {
			return ""
		}
	}
	user, _, found := strings.Cut(string(decoded), ":")
	if !found {
		return ""
	}
	return user
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
	switch column {
	case "STATUS":
		return extractPodPhase(obj)
	case "IP":
		ip, _, _ := unstructured.NestedString(obj.Object, "status", "podIP")
		return ip
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
