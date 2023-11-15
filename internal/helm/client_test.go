package helm

import (
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

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
