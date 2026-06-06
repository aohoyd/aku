package helm

import (
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/rest"
)

// TestNewConfigFlagsSetsNamespace guards the namespace-propagation fix: mutating
// Helm actions resolve the target namespace for manifest resources from
// ConfigFlags.Namespace, so it must carry the release namespace (not just be
// passed to cfg.Init). A nil flag would default to the kubeconfig's current
// namespace and break uninstall/rollback/upgrade for releases elsewhere.
func TestNewConfigFlagsSetsNamespace(t *testing.T) {
	cfg := &rest.Config{Host: "https://example.test"}

	flags := newConfigFlags(cfg, "kube-system")

	if flags.Namespace == nil {
		t.Fatal("expected Namespace to be set, got nil")
	}
	if *flags.Namespace != "kube-system" {
		t.Fatalf("expected namespace kube-system, got %s", *flags.Namespace)
	}
	if flags.WrapConfigFn == nil {
		t.Fatal("expected WrapConfigFn to be set")
	}
	if got := flags.WrapConfigFn(nil); got != cfg {
		t.Fatalf("expected WrapConfigFn to return the supplied rest.Config")
	}
}

func TestReleaseToUnstructured(t *testing.T) {
	r := ReleaseInfo{
		Name:       "my-release",
		Namespace:  "default",
		Revision:   3,
		Chart:      "nginx-1.2.3",
		AppVersion: "1.25.0",
		Status:     "deployed",
		Updated:    time.Date(2026, 3, 6, 12, 0, 0, 0, time.UTC),
		Manifest:   "apiVersion: v1\nkind: Service\nmetadata:\n  name: nginx",
	}

	obj := ReleaseToUnstructured(r)

	if obj.GetName() != "my-release" {
		t.Fatalf("expected name my-release, got %s", obj.GetName())
	}
	if obj.GetNamespace() != "default" {
		t.Fatalf("expected namespace default, got %s", obj.GetNamespace())
	}

	rev, _, _ := unstructured.NestedString(obj.Object, "revision")
	if rev != "3" {
		t.Fatalf("expected revision '3', got %s", rev)
	}

	chart, _, _ := unstructured.NestedString(obj.Object, "chart")
	if chart != "nginx-1.2.3" {
		t.Fatalf("expected chart nginx-1.2.3, got %s", chart)
	}

	status, _, _ := unstructured.NestedString(obj.Object, "status")
	if status != "deployed" {
		t.Fatalf("expected status deployed, got %s", status)
	}

	manifest, _, _ := unstructured.NestedString(obj.Object, "_manifest")
	if manifest == "" {
		t.Fatal("expected _manifest to be set")
	}
}
