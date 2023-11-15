package persistentvolumeclaims

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

var gvr = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "persistentvolumeclaims"}

// Plugin implements plugin.ResourcePlugin for Kubernetes PersistentVolumeClaims.
type Plugin struct {
	store *k8s.Store
}

// New creates a new PersistentVolumeClaim plugin.
func New(_ *k8s.Client, store *k8s.Store) plugin.ResourcePlugin {
	return &Plugin{store: store}
}

func (p *Plugin) Name() string                     { return "persistentvolumeclaims" }
func (p *Plugin) ShortName() string                { return "pvc" }
func (p *Plugin) GVR() schema.GroupVersionResource { return gvr }
func (p *Plugin) IsClusterScoped() bool            { return false }

func (p *Plugin) Columns() []plugin.Column {
	return []plugin.Column{
		{Title: "NAME", Flex: true},
		{Title: "STATUS", Width: 10},
		{Title: "VOLUME", Width: 30},
		{Title: "CAPACITY", Width: 10},
		{Title: "ACCESS MODES", Width: 14},
		{Title: "STORAGECLASS", Width: 18},
		{Title: "AGE", Width: 8},
	}
}

func (p *Plugin) Row(obj *unstructured.Unstructured) []string {
	name := obj.GetName()

	status, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
	volumeName, _, _ := unstructured.NestedString(obj.Object, "spec", "volumeName")
	capacity, _, _ := unstructured.NestedString(obj.Object, "status", "capacity", "storage")
	storageClass, _, _ := unstructured.NestedString(obj.Object, "spec", "storageClassName")
	accessModes := formatAccessModes(obj)
	age := render.FormatAge(obj)

	return []string{name, status, volumeName, capacity, accessModes, storageClass, age}
}

func (p *Plugin) YAML(obj *unstructured.Unstructured) (render.Content, error) {
	return plugin.MarshalYAML(obj)
}

func (p *Plugin) Describe(ctx context.Context, obj *unstructured.Unstructured) (render.Content, error) {
	pvc, err := toPVC(obj)
	if err != nil {
		return render.Content{}, fmt.Errorf("converting to PersistentVolumeClaim: %w", err)
	}

	b := render.NewBuilder()

	// Metadata
	b.KV(render.LEVEL_0, "Name", pvc.Name)
	b.KV(render.LEVEL_0, "Namespace", pvc.Namespace)
	b.KV(render.LEVEL_0, "CreationTimestamp", render.FormatAge(obj))
	b.KVMulti(render.LEVEL_0, "Labels", pvc.Labels)
	b.KVMulti(render.LEVEL_0, "Annotations", pvc.Annotations)

	// Status
	b.KV(render.LEVEL_0, "Status", string(pvc.Status.Phase))

	// Volume
	b.KV(render.LEVEL_0, "Volume", pvc.Spec.VolumeName)

	// Capacity
	if pvc.Status.Capacity != nil {
		if storage, ok := pvc.Status.Capacity[corev1.ResourceStorage]; ok {
			b.KV(render.LEVEL_0, "Capacity", storage.String())
		}
	}

	// Access Modes
	if len(pvc.Spec.AccessModes) > 0 {
		modes := make([]string, len(pvc.Spec.AccessModes))
		for i, m := range pvc.Spec.AccessModes {
			modes[i] = string(m)
		}
		b.KV(render.LEVEL_0, "Access Modes", strings.Join(modes, ","))
	}

	// Storage Class
	if pvc.Spec.StorageClassName != nil {
		b.KV(render.LEVEL_0, "StorageClass", *pvc.Spec.StorageClassName)
	}

	// Volume Mode
	if pvc.Spec.VolumeMode != nil {
		b.KV(render.LEVEL_0, "VolumeMode", string(*pvc.Spec.VolumeMode))
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
	p.store.Subscribe(workload.PodsGVR, obj.GetNamespace())
	pods := workload.FindPodsByVolumeClaim(p.store, obj.GetNamespace(), obj.GetName())
	return pp, pods
}

// toPVC converts an unstructured object to a typed corev1.PersistentVolumeClaim.
func toPVC(obj *unstructured.Unstructured) (*corev1.PersistentVolumeClaim, error) {
	var pvc corev1.PersistentVolumeClaim
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &pvc); err != nil {
		return nil, err
	}
	return &pvc, nil
}

// formatAccessModes extracts spec.accessModes and abbreviates them.
func formatAccessModes(obj *unstructured.Unstructured) string {
	modes, found, _ := unstructured.NestedStringSlice(obj.Object, "spec", "accessModes")
	if !found || len(modes) == 0 {
		return ""
	}
	abbreviated := make([]string, len(modes))
	for i, m := range modes {
		abbreviated[i] = abbreviateAccessMode(m)
	}
	return strings.Join(abbreviated, ",")
}

func abbreviateAccessMode(mode string) string {
	switch mode {
	case "ReadWriteOnce":
		return "RWO"
	case "ReadOnlyMany":
		return "ROX"
	case "ReadWriteMany":
		return "RWX"
	default:
		return mode
	}
}
