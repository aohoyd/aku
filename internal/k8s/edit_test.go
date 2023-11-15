package k8s

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestMarshalForEdit(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]any{
			"name":          "test",
			"namespace":     "default",
			"managedFields": []any{"should be stripped"},
		},
		"data": map[string]any{"key": "value"},
	}}
	got, err := marshalForEdit(obj)
	if err != nil {
		t.Fatal(err)
	}
	s := string(got)
	if strings.Contains(s, "managedFields") {
		t.Error("managedFields not stripped")
	}
	if !strings.Contains(s, "name: test") {
		t.Error("expected name field in output")
	}
	// Verify original object is not mutated
	if _, ok := obj.Object["metadata"].(map[string]any)["managedFields"]; !ok {
		t.Error("original object was mutated")
	}
}
