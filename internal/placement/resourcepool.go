package placement

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

// ParsePoolStatus converts the status section of a ResourcePool into a typed
// struct. Returns (nil, nil) if the pool has no status.
func ParsePoolStatus(obj *unstructured.Unstructured) (*ResourcePoolStatus, error) {
	raw, found, _ := unstructured.NestedMap(obj.Object, "status")
	if !found {
		return nil, nil
	}
	var status ResourcePoolStatus
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(raw, &status); err != nil {
		return nil, err
	}
	return &status, nil
}
