package workload

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ServicesGVR is the single-source core/v1 services GVR referenced by the services plugin.
var ServicesGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "services"}

// EndpointsGVR is the single-source core/v1 endpoints GVR referenced by the endpoints plugin.
var EndpointsGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "endpoints"}
