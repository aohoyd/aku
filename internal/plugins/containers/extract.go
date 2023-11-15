package containers

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// ExtractContainers converts a pod unstructured object into a slice of
// synthetic unstructured objects, one per container (regular, init, ephemeral).
func ExtractContainers(pod *unstructured.Unstructured) []*unstructured.Unstructured {
	var result []*unstructured.Unstructured
	podObj := pod.Object
	podNs := pod.GetNamespace()

	// Regular containers
	specs, _, _ := unstructured.NestedSlice(pod.Object, "spec", "containers")
	statuses := buildStatusMap(pod, "status", "containerStatuses")
	for _, spec := range specs {
		specMap, ok := spec.(map[string]any)
		if !ok {
			continue
		}
		name, _ := specMap["name"].(string)
		result = append(result, buildSynthetic(name, "regular", specMap, statuses[name], podObj, podNs))
	}

	// Init containers
	initSpecs, _, _ := unstructured.NestedSlice(pod.Object, "spec", "initContainers")
	initStatuses := buildStatusMap(pod, "status", "initContainerStatuses")
	for _, spec := range initSpecs {
		specMap, ok := spec.(map[string]any)
		if !ok {
			continue
		}
		name, _ := specMap["name"].(string)
		result = append(result, buildSynthetic(name, "init", specMap, initStatuses[name], podObj, podNs))
	}

	// Ephemeral containers
	ephSpecs, _, _ := unstructured.NestedSlice(pod.Object, "spec", "ephemeralContainers")
	ephStatuses := buildStatusMap(pod, "status", "ephemeralContainerStatuses")
	for _, spec := range ephSpecs {
		specMap, ok := spec.(map[string]any)
		if !ok {
			continue
		}
		name, _ := specMap["name"].(string)
		result = append(result, buildSynthetic(name, "ephemeral", specMap, ephStatuses[name], podObj, podNs))
	}

	return result
}

func buildSynthetic(name, ctype string, spec, status map[string]any, podObj map[string]any, podNs string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{
			"name":      name,
			"namespace": podNs,
		},
		"_type": ctype,
		"_spec": spec,
		"_pod":  podObj,
	}}
	if status != nil {
		obj.Object["_status"] = status
	}
	return obj
}

func buildStatusMap(pod *unstructured.Unstructured, path ...string) map[string]map[string]any {
	m := make(map[string]map[string]any)
	statuses, _, _ := unstructured.NestedSlice(pod.Object, path...)
	for _, s := range statuses {
		sMap, ok := s.(map[string]any)
		if !ok {
			continue
		}
		name, _ := sMap["name"].(string)
		m[name] = sMap
	}
	return m
}
