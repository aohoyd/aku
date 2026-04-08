package k8s

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
)

// writeEditorScript creates a shell script that modifies the file on the first
// call and does nothing on subsequent calls, simulating "edit then exit to cancel".
func writeEditorScript(t *testing.T, counterPath, modifiedContent string) string {
	t.Helper()
	scriptPath := filepath.Join(t.TempDir(), "editor.sh")
	script := fmt.Sprintf(`#!/bin/sh
count=$(cat %q 2>/dev/null || echo 0)
count=$((count + 1))
printf '%%s' "$count" > %q
if [ "$count" = "1" ]; then
    cat > "$1" << 'CONTENT'
%s
CONTENT
fi
`, counterPath, counterPath, modifiedContent)
	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	return scriptPath
}

func TestEditRunCancelAfterAPIError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell scripts not supported on Windows")
	}

	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]any{
			"name":            "test",
			"namespace":       "default",
			"resourceVersion": "123",
		},
		"data": map[string]any{"key": "value"},
	}}

	gvr := schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}
	scheme := k8sruntime.NewScheme()
	client := dynamicfake.NewSimpleDynamicClient(scheme, obj)

	// Make Update always fail with a validation error.
	client.PrependReactor("update", "*", func(action k8stesting.Action) (bool, k8sruntime.Object, error) {
		return true, nil, fmt.Errorf("validation error: field is invalid")
	})

	counterPath := filepath.Join(t.TempDir(), "counter")
	modifiedYAML := `apiVersion: v1
kind: ConfigMap
metadata:
  name: test
  namespace: default
  resourceVersion: "123"
data:
  key: modified`

	editorScript := writeEditorScript(t, counterPath, modifiedYAML)
	t.Setenv("EDITOR", editorScript)

	ec := &editCommand{
		obj:       obj,
		gvr:       gvr,
		dynClient: client,
		stdin:     os.Stdin,
		stdout:    os.Stdout,
		stderr:    os.Stderr,
	}

	err := ec.Run()
	if err != nil {
		t.Fatalf("expected nil (cancel), got: %v", err)
	}

	// Verify editor was called exactly twice: once for the edit, once for the retry.
	countBytes, _ := os.ReadFile(counterPath)
	if string(countBytes) != "2" {
		t.Fatalf("expected editor to be called 2 times, got %s", countBytes)
	}
}

func TestEditRunConflictRefetch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell scripts not supported on Windows")
	}

	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]any{
			"name":            "test",
			"namespace":       "default",
			"resourceVersion": "123",
		},
		"data": map[string]any{"key": "value"},
	}}

	// The "latest" version on the server after external change.
	latestObj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]any{
			"name":            "test",
			"namespace":       "default",
			"resourceVersion": "456",
		},
		"data": map[string]any{"key": "externally-changed"},
	}}

	gvr := schema.GroupVersionResource{Version: "v1", Resource: "configmaps"}
	scheme := k8sruntime.NewScheme()
	client := dynamicfake.NewSimpleDynamicClient(scheme, latestObj)

	// Make Update fail with 409 Conflict.
	client.PrependReactor("update", "*", func(action k8stesting.Action) (bool, k8sruntime.Object, error) {
		return true, nil, apierrors.NewConflict(
			schema.GroupResource{Resource: "configmaps"}, "test",
			fmt.Errorf("the object has been modified"),
		)
	})

	counterPath := filepath.Join(t.TempDir(), "counter")
	modifiedYAML := `apiVersion: v1
kind: ConfigMap
metadata:
  name: test
  namespace: default
  resourceVersion: "123"
data:
  key: modified`

	editorScript := writeEditorScript(t, counterPath, modifiedYAML)
	t.Setenv("EDITOR", editorScript)

	// Track what the editor sees on the second call (the retry file content).
	retryContentPath := filepath.Join(t.TempDir(), "retry_content")
	// Rewrite the editor script to also capture file content on second call.
	script := fmt.Sprintf(`#!/bin/sh
count=$(cat %q 2>/dev/null || echo 0)
count=$((count + 1))
printf '%%s' "$count" > %q
if [ "$count" = "1" ]; then
    cat > "$1" << 'CONTENT'
%s
CONTENT
else
    cp "$1" %q
fi
`, counterPath, counterPath, modifiedYAML, retryContentPath)
	if err := os.WriteFile(editorScript, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}

	ec := &editCommand{
		obj:       obj,
		gvr:       gvr,
		dynClient: client,
		stdin:     os.Stdin,
		stdout:    os.Stdout,
		stderr:    os.Stderr,
	}

	err := ec.Run()
	if err != nil {
		t.Fatalf("expected nil (cancel), got: %v", err)
	}

	// Verify the retry content contains the re-fetched server value.
	retryContent, err := os.ReadFile(retryContentPath)
	if err != nil {
		t.Fatalf("failed to read retry content: %v", err)
	}
	content := string(retryContent)
	if !strings.Contains(content, "externally-changed") {
		t.Errorf("expected retry content to contain re-fetched value 'externally-changed', got:\n%s", content)
	}
	if !strings.Contains(content, "resourceVersion: \"456\"") {
		t.Errorf("expected retry content to contain new resourceVersion '456', got:\n%s", content)
	}
}
