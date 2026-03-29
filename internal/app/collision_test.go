package app

import (
	"testing"

	"github.com/aohoyd/aku/internal/ui"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestIsBuiltInGroup(t *testing.T) {
	tests := []struct {
		group string
		want  bool
	}{
		{"", true},                                  // core
		{"apps", true},                              // well-known built-in group
		{"batch", true},                             // well-known built-in group
		{"autoscaling", true},                       // well-known built-in group
		{"policy", true},                            // well-known built-in group
		{"networking.k8s.io", true},                 // .k8s.io suffix
		{"storage.k8s.io", true},                    // .k8s.io suffix
		{"cert-manager.io", false},                  // CRD group
		{"argoproj.io", false},                      // CRD group
		{"admissionregistration.k8s.io", true},      // .k8s.io suffix
		{"flowcontrol.apiserver.k8s.io", true},      // .k8s.io suffix
		{"stable.example.com", false},               // CRD group
	}
	for _, tt := range tests {
		t.Run(tt.group, func(t *testing.T) {
			if got := isBuiltInGroup(tt.group); got != tt.want {
				t.Errorf("isBuiltInGroup(%q) = %v, want %v", tt.group, got, tt.want)
			}
		})
	}
}

func TestMarkCollisions_NoCollision(t *testing.T) {
	entries := []ui.PluginEntry{
		{Name: "pods", GVR: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}},
		{Name: "deployments", GVR: schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}},
	}
	result := markCollisions(entries)
	for _, e := range result {
		if e.Qualified {
			t.Errorf("entry %q should not be qualified, no collision", e.Name)
		}
	}
}

func TestMarkCollisions_CRDvsCRD(t *testing.T) {
	// Two entries with same name, both non-built-in groups -> both qualified.
	entries := []ui.PluginEntry{
		{Name: "certificates", GVR: schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "certificates"}},
		{Name: "certificates", GVR: schema.GroupVersionResource{Group: "networking.internal.knative.dev", Version: "v1alpha1", Resource: "certificates"}},
	}
	result := markCollisions(entries)
	for _, e := range result {
		if !e.Qualified {
			t.Errorf("entry %q (group=%q) should be qualified in CRD-CRD collision", e.Name, e.GVR.Group)
		}
	}
}

func TestMarkCollisions_BuiltInVsCRD(t *testing.T) {
	// One core entry, one CRD entry -> only CRD gets qualified.
	entries := []ui.PluginEntry{
		{Name: "events", GVR: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "events"}},
		{Name: "events", GVR: schema.GroupVersionResource{Group: "events.example.com", Version: "v1", Resource: "events"}},
	}
	result := markCollisions(entries)
	if result[0].Qualified {
		t.Error("core entry should NOT be qualified")
	}
	if !result[1].Qualified {
		t.Error("CRD entry should be qualified")
	}
}

func TestMarkCollisions_BuiltInK8sIOVsCRD(t *testing.T) {
	// One *.k8s.io entry, one CRD entry -> only CRD gets qualified.
	entries := []ui.PluginEntry{
		{Name: "ingresses", GVR: schema.GroupVersionResource{Group: "networking.k8s.io", Version: "v1", Resource: "ingresses"}},
		{Name: "ingresses", GVR: schema.GroupVersionResource{Group: "example.com", Version: "v1beta1", Resource: "ingresses"}},
	}
	result := markCollisions(entries)
	if result[0].Qualified {
		t.Error("networking.k8s.io entry should NOT be qualified")
	}
	if !result[1].Qualified {
		t.Error("CRD entry should be qualified")
	}
}

func TestMarkCollisions_AllBuiltIn(t *testing.T) {
	// Two entries from built-in groups with same name: keep only the first (primary).
	entries := []ui.PluginEntry{
		{Name: "events", GVR: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "events"}},
		{Name: "events", GVR: schema.GroupVersionResource{Group: "events.k8s.io", Version: "v1", Resource: "events"}},
	}
	result := markCollisions(entries)
	if len(result) != 1 {
		t.Fatalf("expected 1 entry (duplicate removed), got %d", len(result))
	}
	if result[0].GVR.Group != "" {
		t.Errorf("expected primary (core) entry kept, got group=%q", result[0].GVR.Group)
	}
	if result[0].Qualified {
		t.Error("primary entry should NOT be qualified")
	}
}

func TestMarkCollisions_DifferentNamesDontAffect(t *testing.T) {
	entries := []ui.PluginEntry{
		{Name: "pods", GVR: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}},
		{Name: "deployments", GVR: schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}},
		{Name: "certificates", GVR: schema.GroupVersionResource{Group: "cert-manager.io", Version: "v1", Resource: "certificates"}},
		{Name: "certificates", GVR: schema.GroupVersionResource{Group: "networking.internal.knative.dev", Version: "v1alpha1", Resource: "certificates"}},
	}
	result := markCollisions(entries)

	// pods and deployments: no collision, not qualified.
	if result[0].Qualified {
		t.Error("pods should not be qualified")
	}
	if result[1].Qualified {
		t.Error("deployments should not be qualified")
	}
	// certificates: collide, both CRD -> both qualified.
	if !result[2].Qualified {
		t.Error("certificates (cert-manager.io) should be qualified")
	}
	if !result[3].Qualified {
		t.Error("certificates (knative) should be qualified")
	}
}

func TestMarkCollisions_MixedThreeWay(t *testing.T) {
	// Built-in + two CRDs with same name: built-in stays unqualified, CRDs get qualified.
	entries := []ui.PluginEntry{
		{Name: "events", GVR: schema.GroupVersionResource{Group: "", Version: "v1", Resource: "events"}},
		{Name: "events", GVR: schema.GroupVersionResource{Group: "custom1.example.com", Version: "v1", Resource: "events"}},
		{Name: "events", GVR: schema.GroupVersionResource{Group: "custom2.example.com", Version: "v1", Resource: "events"}},
	}
	result := markCollisions(entries)
	if result[0].Qualified {
		t.Error("core events should NOT be qualified")
	}
	if !result[1].Qualified {
		t.Error("custom1 events should be qualified")
	}
	if !result[2].Qualified {
		t.Error("custom2 events should be qualified")
	}
}
