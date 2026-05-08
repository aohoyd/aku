package k8s

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/aohoyd/aku/internal/msgs"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
)

// deploymentGVR is the GVR used by the restart tests.
var deploymentGVR = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}

// newDeployment builds a minimal unstructured Deployment for the fake dynamic client.
func newDeployment(name, namespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]any{
			"name":      name,
			"namespace": namespace,
		},
		"spec": map[string]any{
			"template": map[string]any{
				"metadata": map[string]any{},
				"spec": map[string]any{
					"containers": []any{
						map[string]any{"name": "app", "image": "myapp:v1"},
					},
				},
			},
		},
	}}
}

// fakeSchemeForDeployments returns a scheme with the Deployment list kind registered for the fake dynamic client.
func fakeSchemeForDeployments() *k8sruntime.Scheme {
	scheme := k8sruntime.NewScheme()
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "DeploymentList"},
		&unstructured.UnstructuredList{},
	)
	return scheme
}

func TestRestartCmd_SingleDeployment(t *testing.T) {
	dep := newDeployment("web", "default")
	scheme := fakeSchemeForDeployments()
	client := dynamicfake.NewSimpleDynamicClient(scheme, dep)

	cmd := RestartCmd(client, deploymentGVR, []RestartTarget{{Name: "web", Namespace: "default"}})
	if cmd == nil {
		t.Fatal("RestartCmd returned nil")
	}
	msg := cmd()
	res, ok := msg.(msgs.ActionResultMsg)
	if !ok {
		t.Fatalf("expected ActionResultMsg, got %T", msg)
	}
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if res.ActionID != "restart:web" {
		t.Errorf("expected ActionID=restart:web, got %q", res.ActionID)
	}

	got, err := client.Resource(deploymentGVR).Namespace("default").Get(context.Background(), "web", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("get after patch: %v", err)
	}
	ann, found, err := unstructured.NestedStringMap(got.Object, "spec", "template", "metadata", "annotations")
	if err != nil {
		t.Fatalf("nested annotations: %v", err)
	}
	if !found {
		t.Fatal("expected annotations to be set under spec.template.metadata")
	}
	ts, ok := ann["kubectl.kubernetes.io/restartedAt"]
	if !ok || ts == "" {
		t.Fatalf("expected restartedAt annotation, got %v", ann)
	}
	if _, err := time.Parse(time.RFC3339, ts); err != nil {
		t.Errorf("restartedAt annotation %q is not RFC3339: %v", ts, err)
	}
}

func TestRestartCmd_BulkAcrossNamespaces(t *testing.T) {
	d1 := newDeployment("api", "alpha")
	d2 := newDeployment("worker", "beta")
	d3 := newDeployment("cron", "gamma")
	scheme := fakeSchemeForDeployments()
	client := dynamicfake.NewSimpleDynamicClient(scheme, d1, d2, d3)

	targets := []RestartTarget{
		{Name: "api", Namespace: "alpha"},
		{Name: "worker", Namespace: "beta"},
		{Name: "cron", Namespace: "gamma"},
	}

	// Capture the patch payload to ensure it's marshaled once and reused.
	var patchPayloads [][]byte
	client.PrependReactor("patch", "deployments", func(action k8stesting.Action) (bool, k8sruntime.Object, error) {
		pa := action.(k8stesting.PatchAction)
		// Copy because the action's underlying buffer may be reused.
		buf := make([]byte, len(pa.GetPatch()))
		copy(buf, pa.GetPatch())
		patchPayloads = append(patchPayloads, buf)
		return false, nil, nil
	})

	msg := RestartCmd(client, deploymentGVR, targets)()
	res, ok := msg.(msgs.ActionResultMsg)
	if !ok {
		t.Fatalf("expected ActionResultMsg, got %T", msg)
	}
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if res.ActionID != "restart:3-resources" {
		t.Errorf("expected ActionID=restart:3-resources, got %q", res.ActionID)
	}

	if len(patchPayloads) != 3 {
		t.Fatalf("expected 3 patch calls, got %d", len(patchPayloads))
	}
	first := string(patchPayloads[0])
	for i, p := range patchPayloads {
		if string(p) != first {
			t.Errorf("patch payload %d differs from first; want byte-identical:\n  first=%s\n  got  =%s", i, first, string(p))
		}
	}

	// Verify each Deployment ended up with the same timestamp annotation set on spec.template.metadata.annotations.
	var seen string
	for _, tgt := range targets {
		got, err := client.Resource(deploymentGVR).Namespace(tgt.Namespace).Get(context.Background(), tgt.Name, metav1.GetOptions{})
		if err != nil {
			t.Fatalf("get %s/%s after patch: %v", tgt.Namespace, tgt.Name, err)
		}
		ann, found, err := unstructured.NestedStringMap(got.Object, "spec", "template", "metadata", "annotations")
		if err != nil {
			t.Fatalf("nested annotations for %s/%s: %v", tgt.Namespace, tgt.Name, err)
		}
		if !found {
			t.Fatalf("expected annotations on %s/%s", tgt.Namespace, tgt.Name)
		}
		ts := ann["kubectl.kubernetes.io/restartedAt"]
		if ts == "" {
			t.Fatalf("missing restartedAt on %s/%s", tgt.Namespace, tgt.Name)
		}
		if seen == "" {
			seen = ts
		} else if ts != seen {
			t.Errorf("expected identical timestamp across targets; got %q vs %q", seen, ts)
		}
	}

	// Sanity-check that the captured patch is a JSON merge patch with the expected key.
	var m map[string]any
	if err := json.Unmarshal(patchPayloads[0], &m); err != nil {
		t.Fatalf("patch is not valid JSON: %v", err)
	}
	spec := m["spec"].(map[string]any)
	template := spec["template"].(map[string]any)
	meta := template["metadata"].(map[string]any)
	annAny := meta["annotations"].(map[string]any)
	if annAny["kubectl.kubernetes.io/restartedAt"] == nil {
		t.Errorf("patch missing restartedAt annotation: %s", string(patchPayloads[0]))
	}
}

func TestRestartCmd_PartialFailure(t *testing.T) {
	d1 := newDeployment("api", "alpha")
	d2 := newDeployment("worker", "beta")
	d3 := newDeployment("cron", "gamma")
	scheme := fakeSchemeForDeployments()
	client := dynamicfake.NewSimpleDynamicClient(scheme, d1, d2, d3)

	// Inject an error for the "worker" patch only; let the others through.
	client.PrependReactor("patch", "deployments", func(action k8stesting.Action) (bool, k8sruntime.Object, error) {
		pa := action.(k8stesting.PatchAction)
		if pa.GetName() == "worker" {
			return true, nil, fmt.Errorf("boom")
		}
		return false, nil, nil
	})

	targets := []RestartTarget{
		{Name: "api", Namespace: "alpha"},
		{Name: "worker", Namespace: "beta"},
		{Name: "cron", Namespace: "gamma"},
	}
	msg := RestartCmd(client, deploymentGVR, targets)()
	res, ok := msg.(msgs.ActionResultMsg)
	if !ok {
		t.Fatalf("expected ActionResultMsg, got %T", msg)
	}
	if res.Err == nil {
		t.Fatal("expected non-nil error on partial failure")
	}
	want := "bulk restart: 1/3 failed: worker: boom"
	if res.Err.Error() != want {
		t.Errorf("expected error %q, got %q", want, res.Err.Error())
	}
	// ActionID should be empty when there's an error.
	if res.ActionID != "" {
		t.Errorf("expected empty ActionID on failure, got %q", res.ActionID)
	}

	// Verify the other two were patched.
	for _, tgt := range []RestartTarget{{Name: "api", Namespace: "alpha"}, {Name: "cron", Namespace: "gamma"}} {
		got, err := client.Resource(deploymentGVR).Namespace(tgt.Namespace).Get(context.Background(), tgt.Name, metav1.GetOptions{})
		if err != nil {
			t.Fatalf("get %s/%s: %v", tgt.Namespace, tgt.Name, err)
		}
		ann, found, err := unstructured.NestedStringMap(got.Object, "spec", "template", "metadata", "annotations")
		if err != nil {
			t.Fatalf("nested annotations for %s/%s: %v", tgt.Namespace, tgt.Name, err)
		}
		if !found {
			t.Fatalf("expected annotations on %s/%s", tgt.Namespace, tgt.Name)
		}
		ts := ann["kubectl.kubernetes.io/restartedAt"]
		if ts == "" {
			t.Fatalf("missing restartedAt on %s/%s", tgt.Namespace, tgt.Name)
		}
		if _, err := time.Parse(time.RFC3339, ts); err != nil {
			t.Errorf("restartedAt annotation %q on %s/%s is not RFC3339: %v", ts, tgt.Namespace, tgt.Name, err)
		}
	}
}

// TestRestartCmd_EmptyTargets documents the contract for the zero-target case:
// no API calls are made, no error is returned, and the ActionID encodes
// "0-resources" via the multi-target branch (since len(targets) == 1 is false).
func TestRestartCmd_EmptyTargets(t *testing.T) {
	scheme := fakeSchemeForDeployments()
	client := dynamicfake.NewSimpleDynamicClient(scheme)

	var patchCalls int
	client.PrependReactor("patch", "deployments", func(action k8stesting.Action) (bool, k8sruntime.Object, error) {
		patchCalls++
		return false, nil, nil
	})

	msg := RestartCmd(client, deploymentGVR, []RestartTarget{})()
	res, ok := msg.(msgs.ActionResultMsg)
	if !ok {
		t.Fatalf("expected ActionResultMsg, got %T", msg)
	}
	if res.Err != nil {
		t.Fatalf("unexpected error: %v", res.Err)
	}
	if res.ActionID != "restart:0-resources" {
		t.Errorf("expected ActionID=restart:0-resources, got %q", res.ActionID)
	}
	if patchCalls != 0 {
		t.Errorf("expected 0 patch calls for empty targets, got %d", patchCalls)
	}
}
