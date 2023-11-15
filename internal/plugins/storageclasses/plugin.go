package storageclasses

import (
	"context"
	"fmt"
	"strings"

	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/plugins/workload"
	"github.com/aohoyd/aku/internal/render"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var gvr = schema.GroupVersionResource{Group: "storage.k8s.io", Version: "v1", Resource: "storageclasses"}

// Plugin implements plugin.ResourcePlugin for Kubernetes StorageClasses.
type Plugin struct {
	store *k8s.Store
}

// New creates a new StorageClasses plugin.
func New(_ *k8s.Client, store *k8s.Store) plugin.ResourcePlugin {
	return &Plugin{
		store: store,
	}
}

func (p *Plugin) Name() string                     { return "storageclasses" }
func (p *Plugin) ShortName() string                { return "sc" }
func (p *Plugin) GVR() schema.GroupVersionResource { return gvr }
func (p *Plugin) IsClusterScoped() bool            { return true }

func (p *Plugin) Columns() []plugin.Column {
	return []plugin.Column{
		{Title: "NAME", Flex: true},
		{Title: "PROVISIONER", Flex: true},
		{Title: "RECLAIMPOLICY", Width: 16},
		{Title: "VOLUMEBINDINGMODE", Width: 22},
		{Title: "AGE", Width: 8},
	}
}

func (p *Plugin) Row(obj *unstructured.Unstructured) []string {
	name := obj.GetName()

	provisioner, _, _ := unstructured.NestedString(obj.Object, "provisioner")

	reclaimPolicy, _, _ := unstructured.NestedString(obj.Object, "reclaimPolicy")
	if reclaimPolicy == "" {
		reclaimPolicy = "Delete"
	}

	volumeBindingMode, _, _ := unstructured.NestedString(obj.Object, "volumeBindingMode")
	if volumeBindingMode == "" {
		volumeBindingMode = "Immediate"
	}

	age := render.FormatAge(obj)

	return []string{name, provisioner, reclaimPolicy, volumeBindingMode, age}
}

func (p *Plugin) YAML(obj *unstructured.Unstructured) (render.Content, error) {
	return plugin.MarshalYAML(obj)
}

func (p *Plugin) Describe(ctx context.Context, obj *unstructured.Unstructured) (render.Content, error) {
	sc, err := toStorageClass(obj)
	if err != nil {
		return render.Content{}, fmt.Errorf("converting to StorageClass: %w", err)
	}

	b := render.NewBuilder()

	// Metadata
	b.KV(render.LEVEL_0, "Name", sc.Name)
	b.KVMulti(render.LEVEL_0, "Labels", sc.Labels)
	b.KVMulti(render.LEVEL_0, "Annotations", sc.Annotations)

	// Provisioner
	b.KV(render.LEVEL_0, "Provisioner", sc.Provisioner)

	// Parameters
	b.KVMulti(render.LEVEL_0, "Parameters", sc.Parameters)

	// ReclaimPolicy
	reclaimPolicy := "Delete"
	if sc.ReclaimPolicy != nil {
		reclaimPolicy = string(*sc.ReclaimPolicy)
	}
	b.KV(render.LEVEL_0, "ReclaimPolicy", reclaimPolicy)

	// VolumeBindingMode
	volumeBindingMode := "Immediate"
	if sc.VolumeBindingMode != nil {
		volumeBindingMode = string(*sc.VolumeBindingMode)
	}
	b.KV(render.LEVEL_0, "VolumeBindingMode", volumeBindingMode)

	// AllowVolumeExpansion
	allowExpansion := "false"
	if sc.AllowVolumeExpansion != nil && *sc.AllowVolumeExpansion {
		allowExpansion = "true"
	}
	b.KV(render.LEVEL_0, "AllowVolumeExpansion", allowExpansion)

	// MountOptions
	if len(sc.MountOptions) > 0 {
		b.KV(render.LEVEL_0, "MountOptions", strings.Join(sc.MountOptions, ", "))
	}

	return b.Build(), nil
}

// DrillDown implements plugin.DrillDowner.
func (p *Plugin) DrillDown(obj *unstructured.Unstructured) (plugin.ResourcePlugin, []*unstructured.Unstructured) {
	if p.store == nil {
		return nil, nil
	}
	pvPlugin, ok := plugin.ByName("persistentvolumes")
	if !ok {
		return nil, nil
	}
	p.store.Subscribe(workload.PersistentVolumesGVR, "")
	pvs := workload.FindPVsByStorageClass(p.store, obj.GetName())
	return pvPlugin, pvs
}

// toStorageClass converts an unstructured object to a typed storagev1.StorageClass.
func toStorageClass(obj *unstructured.Unstructured) (*storagev1.StorageClass, error) {
	var sc storagev1.StorageClass
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &sc); err != nil {
		return nil, err
	}
	return &sc, nil
}
