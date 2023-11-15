package persistentvolumes

import (
	"context"
	"fmt"
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

var gvr = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "persistentvolumes"}

// Plugin implements plugin.ResourcePlugin for Kubernetes PersistentVolumes.
type Plugin struct {
	store *k8s.Store
}

// New creates a new PersistentVolumes plugin.
func New(_ *k8s.Client, store *k8s.Store) plugin.ResourcePlugin {
	return &Plugin{store: store}
}

func (p *Plugin) Name() string                     { return "persistentvolumes" }
func (p *Plugin) ShortName() string                { return "pv" }
func (p *Plugin) GVR() schema.GroupVersionResource { return gvr }
func (p *Plugin) IsClusterScoped() bool            { return true }

func (p *Plugin) Columns() []plugin.Column {
	return []plugin.Column{
		{Title: "NAME", Flex: true},
		{Title: "CAPACITY", Width: 10},
		{Title: "ACCESS MODES", Width: 14},
		{Title: "RECLAIM POLICY", Width: 16},
		{Title: "STATUS", Width: 10},
		{Title: "CLAIM", Width: 30},
		{Title: "STORAGECLASS", Width: 18},
		{Title: "AGE", Width: 8},
	}
}

func (p *Plugin) Row(obj *unstructured.Unstructured) []string {
	name := obj.GetName()

	capacity, _, _ := unstructured.NestedString(obj.Object, "spec", "capacity", "storage")

	accessModes := abbreviateAccessModes(obj)

	reclaimPolicy, _, _ := unstructured.NestedString(obj.Object, "spec", "persistentVolumeReclaimPolicy")

	phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")

	claim := formatClaimRef(obj)

	storageClass, _, _ := unstructured.NestedString(obj.Object, "spec", "storageClassName")

	age := render.FormatAge(obj)

	return []string{name, capacity, accessModes, reclaimPolicy, phase, claim, storageClass, age}
}

func (p *Plugin) YAML(obj *unstructured.Unstructured) (render.Content, error) {
	return plugin.MarshalYAML(obj)
}

func (p *Plugin) Describe(ctx context.Context, obj *unstructured.Unstructured) (render.Content, error) {
	pv, err := toPersistentVolume(obj)
	if err != nil {
		return render.Content{}, fmt.Errorf("converting to PersistentVolume: %w", err)
	}

	b := render.NewBuilder()

	// Metadata
	b.KV(render.LEVEL_0, "Name", pv.Name)
	b.KV(render.LEVEL_0, "CreationTimestamp", render.FormatAge(obj))
	b.KVMulti(render.LEVEL_0, "Labels", pv.Labels)
	b.KVMulti(render.LEVEL_0, "Annotations", pv.Annotations)

	// Capacity
	if pv.Spec.Capacity != nil {
		if storage, ok := pv.Spec.Capacity["storage"]; ok {
			b.KV(render.LEVEL_0, "Capacity", storage.String())
		}
	}

	// Access Modes
	if len(pv.Spec.AccessModes) > 0 {
		modes := make([]string, len(pv.Spec.AccessModes))
		for i, m := range pv.Spec.AccessModes {
			modes[i] = string(m)
		}
		b.KV(render.LEVEL_0, "Access Modes", strings.Join(modes, ", "))
	}

	// Reclaim Policy
	b.KV(render.LEVEL_0, "Reclaim Policy", string(pv.Spec.PersistentVolumeReclaimPolicy))

	// Status
	phase := string(pv.Status.Phase)
	b.KVStyled(render.LEVEL_0, pvStatusKind(phase), "Status", phase)

	// Claim Ref
	if pv.Spec.ClaimRef != nil {
		claim := pv.Spec.ClaimRef.Namespace + "/" + pv.Spec.ClaimRef.Name
		b.KV(render.LEVEL_0, "Claim", claim)
	} else {
		b.KV(render.LEVEL_0, "Claim", "<none>")
	}

	// Storage Class
	b.KV(render.LEVEL_0, "StorageClass", pv.Spec.StorageClassName)

	// Volume Source Type
	volumeSource := detectVolumeSourceType(pv)
	if volumeSource != "" {
		b.KV(render.LEVEL_0, "Source Type", volumeSource)
	}

	// Mount Options
	if len(pv.Spec.MountOptions) > 0 {
		b.KV(render.LEVEL_0, "Mount Options", strings.Join(pv.Spec.MountOptions, ", "))
	}

	return b.Build(), nil
}

// DrillDown implements plugin.DrillDowner.
func (p *Plugin) DrillDown(obj *unstructured.Unstructured) (plugin.ResourcePlugin, []*unstructured.Unstructured) {
	if p.store == nil {
		return nil, nil
	}
	pvcPlugin, ok := plugin.ByName("persistentvolumeclaims")
	if !ok {
		return nil, nil
	}
	claimNs, _, _ := unstructured.NestedString(obj.Object, "spec", "claimRef", "namespace")
	claimName, _, _ := unstructured.NestedString(obj.Object, "spec", "claimRef", "name")
	if claimName == "" {
		return nil, nil
	}
	p.store.Subscribe(workload.PVCsGVR, claimNs)
	pvcs := workload.FindPVCByClaimRef(p.store, claimNs, claimName)
	return pvcPlugin, pvcs
}

// abbreviateAccessModes returns shortened access mode string (e.g. "RWO,ROX").
func abbreviateAccessModes(obj *unstructured.Unstructured) string {
	modes, found, _ := unstructured.NestedStringSlice(obj.Object, "spec", "accessModes")
	if !found || len(modes) == 0 {
		return ""
	}
	abbrs := make([]string, len(modes))
	for i, m := range modes {
		switch m {
		case "ReadWriteOnce":
			abbrs[i] = "RWO"
		case "ReadOnlyMany":
			abbrs[i] = "ROX"
		case "ReadWriteMany":
			abbrs[i] = "RWX"
		case "ReadWriteOncePod":
			abbrs[i] = "RWOP"
		default:
			abbrs[i] = m
		}
	}
	return strings.Join(abbrs, ",")
}

// formatClaimRef returns "namespace/name" or "<none>".
func formatClaimRef(obj *unstructured.Unstructured) string {
	ns, _, _ := unstructured.NestedString(obj.Object, "spec", "claimRef", "namespace")
	name, _, _ := unstructured.NestedString(obj.Object, "spec", "claimRef", "name")
	if name == "" {
		return "<none>"
	}
	return ns + "/" + name
}

// toPersistentVolume converts an unstructured object to a typed corev1.PersistentVolume.
func toPersistentVolume(obj *unstructured.Unstructured) (*corev1.PersistentVolume, error) {
	var pv corev1.PersistentVolume
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &pv); err != nil {
		return nil, err
	}
	return &pv, nil
}

// detectVolumeSourceType returns the name of the volume source type in the PV spec.
func detectVolumeSourceType(pv *corev1.PersistentVolume) string {
	src := pv.Spec.PersistentVolumeSource
	switch {
	case src.HostPath != nil:
		return "HostPath"
	case src.NFS != nil:
		return "NFS"
	case src.CSI != nil:
		return "CSI"
	case src.Local != nil:
		return "Local"
	case src.FC != nil:
		return "FC"
	case src.ISCSI != nil:
		return "ISCSI"
	default:
		return ""
	}
}

// pvStatusKind maps PV phase to a ValueKind for styling.
func pvStatusKind(phase string) render.ValueKind {
	switch phase {
	case "Bound":
		return render.ValueStatusOK
	case "Available":
		return render.ValueStatusWarn
	case "Released", "Failed":
		return render.ValueStatusError
	default:
		return render.ValueDefault
	}
}
