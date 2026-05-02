package placement

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

// ParsePoolStatus converts the status section of a ResourcePool into a typed
// struct. Returns (nil, nil) if the pool has no status.
func ParsePoolStatus(obj *unstructured.Unstructured) (*ResourcePoolStatus, error) {
	raw, found, err := unstructured.NestedMap(obj.Object, "status")
	if err != nil {
		return nil, fmt.Errorf("reading status from ResourcePool %s: %w", obj.GetName(), err)
	}
	if !found {
		return nil, nil
	}
	var status ResourcePoolStatus
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(raw, &status); err != nil {
		return nil, err
	}
	return &status, nil
}
