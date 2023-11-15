package helm

import (
	"testing"
)

func TestMarshalValues(t *testing.T) {
	values := map[string]any{
		"replicaCount": 3,
		"image":        map[string]any{"tag": "latest"},
	}
	data, err := marshalValues(values)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty YAML")
	}
}

func TestMarshalValuesEmpty(t *testing.T) {
	data, err := marshalValues(nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "# No user-supplied values\n" {
		t.Fatalf("expected comment for empty values, got %q", string(data))
	}
}
