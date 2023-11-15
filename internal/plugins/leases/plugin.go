package leases

import (
	"context"
	"fmt"

	"github.com/aohoyd/aku/internal/k8s"
	"github.com/aohoyd/aku/internal/plugin"
	"github.com/aohoyd/aku/internal/render"
	coordinationv1 "k8s.io/api/coordination/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var gvr = schema.GroupVersionResource{Group: "coordination.k8s.io", Version: "v1", Resource: "leases"}

// Plugin implements plugin.ResourcePlugin for Kubernetes Leases.
type Plugin struct{}

// New creates a new Leases plugin.
func New(_ *k8s.Client, _ *k8s.Store) plugin.ResourcePlugin {
	return &Plugin{}
}

func (p *Plugin) Name() string                     { return "leases" }
func (p *Plugin) ShortName() string                { return "lease" }
func (p *Plugin) GVR() schema.GroupVersionResource { return gvr }
func (p *Plugin) IsClusterScoped() bool            { return false }

func (p *Plugin) Columns() []plugin.Column {
	return []plugin.Column{
		{Title: "NAME", Flex: true},
		{Title: "HOLDER", Flex: true},
		{Title: "AGE", Width: 8},
	}
}

func (p *Plugin) Row(obj *unstructured.Unstructured) []string {
	name := obj.GetName()

	holder, found, _ := unstructured.NestedString(obj.Object, "spec", "holderIdentity")
	if !found || holder == "" {
		holder = "<none>"
	}

	age := render.FormatAge(obj)

	return []string{name, holder, age}
}

func (p *Plugin) YAML(obj *unstructured.Unstructured) (render.Content, error) {
	return plugin.MarshalYAML(obj)
}

func (p *Plugin) Describe(ctx context.Context, obj *unstructured.Unstructured) (render.Content, error) {
	lease, err := toLease(obj)
	if err != nil {
		return render.Content{}, fmt.Errorf("converting to Lease: %w", err)
	}

	b := render.NewBuilder()

	b.KV(render.LEVEL_0, "Name", lease.Name)
	b.KV(render.LEVEL_0, "Namespace", lease.Namespace)

	// Labels
	b.KVMulti(render.LEVEL_0, "Labels", lease.Labels)

	// Annotations
	b.KVMulti(render.LEVEL_0, "Annotations", lease.Annotations)

	// HolderIdentity
	holderIdentity := "<none>"
	if lease.Spec.HolderIdentity != nil && *lease.Spec.HolderIdentity != "" {
		holderIdentity = *lease.Spec.HolderIdentity
	}
	b.KV(render.LEVEL_0, "HolderIdentity", holderIdentity)

	// LeaseDurationSeconds
	leaseDuration := "<unset>"
	if lease.Spec.LeaseDurationSeconds != nil {
		leaseDuration = fmt.Sprintf("%d", *lease.Spec.LeaseDurationSeconds)
	}
	b.KV(render.LEVEL_0, "LeaseDurationSeconds", leaseDuration)

	// AcquireTime
	acquireTime := "<unset>"
	if lease.Spec.AcquireTime != nil {
		acquireTime = lease.Spec.AcquireTime.Time.String()
	}
	b.KV(render.LEVEL_0, "AcquireTime", acquireTime)

	// RenewTime
	renewTime := "<unset>"
	if lease.Spec.RenewTime != nil {
		renewTime = lease.Spec.RenewTime.Time.String()
	}
	b.KV(render.LEVEL_0, "RenewTime", renewTime)

	// LeaseTransitions
	leaseTransitions := "<unset>"
	if lease.Spec.LeaseTransitions != nil {
		leaseTransitions = fmt.Sprintf("%d", *lease.Spec.LeaseTransitions)
	}
	b.KV(render.LEVEL_0, "LeaseTransitions", leaseTransitions)

	return b.Build(), nil
}

// toLease converts an unstructured object to a typed coordinationv1.Lease.
func toLease(obj *unstructured.Unstructured) (*coordinationv1.Lease, error) {
	var lease coordinationv1.Lease
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &lease); err != nil {
		return nil, err
	}
	return &lease, nil
}
