package k8s

import (
	"encoding/json"
	"testing"

	"github.com/aohoyd/aku/internal/msgs"
)

func TestBuildImagePatch_Pod(t *testing.T) {
	images := []msgs.ContainerImageChange{
		{Name: "nginx", Image: "nginx:1.26", Init: false},
	}
	patch, err := buildImagePatch("pods", images)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(patch, &m); err != nil {
		t.Fatal(err)
	}
	spec := m["spec"].(map[string]any)
	containers := spec["containers"].([]any)
	if len(containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(containers))
	}
	c := containers[0].(map[string]any)
	if c["name"] != "nginx" || c["image"] != "nginx:1.26" {
		t.Errorf("unexpected container: %v", c)
	}
}

func TestBuildImagePatch_Deployment(t *testing.T) {
	images := []msgs.ContainerImageChange{
		{Name: "app", Image: "myapp:v2", Init: false},
		{Name: "init-db", Image: "db-init:v3", Init: true},
	}
	patch, err := buildImagePatch("deployments", images)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(patch, &m); err != nil {
		t.Fatal(err)
	}
	spec := m["spec"].(map[string]any)
	template := spec["template"].(map[string]any)
	templateSpec := template["spec"].(map[string]any)

	containers := templateSpec["containers"].([]any)
	if len(containers) != 1 {
		t.Fatalf("expected 1 regular container, got %d", len(containers))
	}
	initContainers := templateSpec["initContainers"].([]any)
	if len(initContainers) != 1 {
		t.Fatalf("expected 1 init container, got %d", len(initContainers))
	}
}

func TestBuildImagePatch_InitOnly(t *testing.T) {
	images := []msgs.ContainerImageChange{
		{Name: "init-db", Image: "db-init:v3", Init: true},
	}
	patch, err := buildImagePatch("pods", images)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(patch, &m); err != nil {
		t.Fatal(err)
	}
	spec := m["spec"].(map[string]any)
	if _, ok := spec["containers"]; ok {
		t.Fatal("should not have containers key when only init containers changed")
	}
	initContainers := spec["initContainers"].([]any)
	if len(initContainers) != 1 {
		t.Fatalf("expected 1 init container, got %d", len(initContainers))
	}
}

func TestBuildImagePatch_StatefulSet(t *testing.T) {
	images := []msgs.ContainerImageChange{
		{Name: "app", Image: "myapp:v2", Init: false},
	}
	patch, err := buildImagePatch("statefulsets", images)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(patch, &m); err != nil {
		t.Fatal(err)
	}
	spec := m["spec"].(map[string]any)
	template := spec["template"].(map[string]any)
	templateSpec := template["spec"].(map[string]any)
	containers := templateSpec["containers"].([]any)
	if len(containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(containers))
	}
}
