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

func TestBuildImagePatch_PolicyOnly(t *testing.T) {
	images := []msgs.ContainerImageChange{
		{Name: "nginx", PullPolicy: "Always", Init: false},
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
	if c["name"] != "nginx" {
		t.Errorf("unexpected name: %v", c["name"])
	}
	if c["imagePullPolicy"] != "Always" {
		t.Errorf("expected imagePullPolicy=Always, got %v", c["imagePullPolicy"])
	}
	if _, ok := c["image"]; ok {
		t.Errorf("should not have image key for policy-only entry, got %v", c)
	}
}

func TestBuildImagePatch_ImageAndPolicy(t *testing.T) {
	images := []msgs.ContainerImageChange{
		{Name: "nginx", Image: "nginx:1.26", PullPolicy: "IfNotPresent", Init: false},
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
	c := containers[0].(map[string]any)
	if c["image"] != "nginx:1.26" {
		t.Errorf("expected image=nginx:1.26, got %v", c["image"])
	}
	if c["imagePullPolicy"] != "IfNotPresent" {
		t.Errorf("expected imagePullPolicy=IfNotPresent, got %v", c["imagePullPolicy"])
	}
}

func TestBuildImagePatch_InitPolicy(t *testing.T) {
	images := []msgs.ContainerImageChange{
		{Name: "init-db", PullPolicy: "Always", Init: true},
	}
	patch, err := buildImagePatch("deployments", images)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(patch, &m); err != nil {
		t.Fatal(err)
	}
	templateSpec := m["spec"].(map[string]any)["template"].(map[string]any)["spec"].(map[string]any)
	if _, ok := templateSpec["containers"]; ok {
		t.Fatal("should not have regular containers key")
	}
	initContainers := templateSpec["initContainers"].([]any)
	if len(initContainers) != 1 {
		t.Fatalf("expected 1 init container, got %d", len(initContainers))
	}
	c := initContainers[0].(map[string]any)
	if c["name"] != "init-db" {
		t.Errorf("unexpected name: %v", c["name"])
	}
	if c["imagePullPolicy"] != "Always" {
		t.Errorf("expected imagePullPolicy=Always, got %v", c["imagePullPolicy"])
	}
	if _, ok := c["image"]; ok {
		t.Errorf("should not have image key for policy-only init entry, got %v", c)
	}
}

func TestBuildImagePatch_Empty(t *testing.T) {
	patch, err := buildImagePatch("pods", nil)
	if err != nil {
		t.Fatal(err)
	}
	// With no images, the inner spec stays empty: {"spec":{}}.
	if got := string(patch); got != `{"spec":{}}` {
		t.Fatalf("expected empty pod patch %q, got %q", `{"spec":{}}`, got)
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
