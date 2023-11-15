package helmreleases

import (
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	sigsyaml "sigs.k8s.io/yaml"
)

// ParseManifest splits a multi-document YAML manifest and returns
// an unstructured object for each valid document. Malformed docs are skipped.
// Each object stores its raw YAML in the "_raw" field.
func ParseManifest(manifest string) []*unstructured.Unstructured {
	docs := strings.Split(manifest, "---")
	var result []*unstructured.Unstructured
	for _, doc := range docs {
		doc = strings.TrimSpace(doc)
		if doc == "" {
			continue
		}
		var obj map[string]any
		if err := sigsyaml.Unmarshal([]byte(doc), &obj); err != nil {
			continue
		}
		if obj == nil {
			continue
		}
		u := &unstructured.Unstructured{Object: obj}
		// Must have kind to be a valid K8s resource
		if u.GetKind() == "" {
			continue
		}
		u.Object["_raw"] = doc
		result = append(result, u)
	}
	return result
}
