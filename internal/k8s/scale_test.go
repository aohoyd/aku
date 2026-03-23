package k8s

import (
	"encoding/json"
	"testing"
)

func TestBuildScalePatch(t *testing.T) {
	patch, err := buildScalePatch(3)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(patch, &m); err != nil {
		t.Fatal(err)
	}
	spec, ok := m["spec"].(map[string]any)
	if !ok {
		t.Fatal("expected spec key in patch")
	}
	replicas, ok := spec["replicas"]
	if !ok {
		t.Fatal("expected replicas key in spec")
	}
	// JSON numbers unmarshal as float64
	if replicas != float64(3) {
		t.Errorf("expected replicas=3, got %v", replicas)
	}
}

func TestBuildScalePatch_Zero(t *testing.T) {
	patch, err := buildScalePatch(0)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(patch, &m); err != nil {
		t.Fatal(err)
	}
	spec := m["spec"].(map[string]any)
	replicas := spec["replicas"]
	if replicas != float64(0) {
		t.Errorf("expected replicas=0, got %v", replicas)
	}
}

func TestBuildScalePatch_OnlyExpectedKeys(t *testing.T) {
	patch, err := buildScalePatch(5)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(patch, &m); err != nil {
		t.Fatal(err)
	}
	if len(m) != 1 {
		t.Fatalf("expected 1 top-level key, got %d", len(m))
	}
	spec := m["spec"].(map[string]any)
	if len(spec) != 1 {
		t.Fatalf("expected 1 key in spec, got %d", len(spec))
	}
}
