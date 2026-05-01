package placement

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

// ParseHandleSpec converts the spec section of a ResourceHandle into a typed
// struct. Returns a zero-value spec (not nil) if the field is missing.
func ParseHandleSpec(obj *unstructured.Unstructured) (*ResourceHandleSpec, error) {
	raw, found, _ := unstructured.NestedMap(obj.Object, "spec")
	if !found {
		return &ResourceHandleSpec{}, nil
	}
	var spec ResourceHandleSpec
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(raw, &spec); err != nil {
		return nil, err
	}
	return &spec, nil
}

// ParseHandleStatus converts the status section of a ResourceHandle into a
// typed struct. Returns (nil, nil) if the handle has no status.
func ParseHandleStatus(obj *unstructured.Unstructured) (*ResourceHandleStatus, error) {
	raw, found, _ := unstructured.NestedMap(obj.Object, "status")
	if !found {
		return nil, nil
	}
	var status ResourceHandleStatus
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(raw, &status); err != nil {
		return nil, err
	}
	return &status, nil
}

// PlacementFromProvisionData extracts placement fields from a HandleSummary's
// provision_data. Some catalog items propagate sandbox_openshift_cluster into
// provision_data, saving an AnarchySubject API call.
func PlacementFromProvisionData(summary *HandleSummary) (*Placement, bool) {
	if summary == nil || len(summary.ProvisionData) == 0 {
		return nil, false
	}

	clusterRaw, ok := summary.ProvisionData["sandbox_openshift_cluster"]
	if !ok {
		return nil, false
	}
	clusterName, ok := clusterRaw.(string)
	if !ok || clusterName == "" {
		return nil, false
	}

	var name, namespace string
	if v, ok := summary.ProvisionData["sandbox_openshift_name"].(string); ok {
		name = v
	}
	if v, ok := summary.ProvisionData["sandbox_openshift_namespace"].(string); ok {
		namespace = v
	}

	return &Placement{
		ClusterName: clusterName,
		Name:        name,
		Namespace:   namespace,
	}, true
}

// AnarchySubjectRefsFromResources filters HandleResource entries for
// AnarchySubject references. Only entries with the correct apiVersion/kind
// and a non-empty name are returned.
func AnarchySubjectRefsFromResources(resources []HandleResource) ([]ResourceRef, error) {
	if len(resources) == 0 {
		return nil, fmt.Errorf("no status.resources found")
	}

	var refs []ResourceRef
	for _, r := range resources {
		if r.Reference == nil {
			continue
		}
		ref := r.Reference
		if ref.APIVersion != "anarchy.gpte.redhat.com/v1" || ref.Kind != "AnarchySubject" {
			continue
		}
		if ref.Name == "" {
			continue
		}
		refs = append(refs, *ref)
	}

	if len(refs) == 0 {
		return nil, fmt.Errorf("no AnarchySubject references found in status.resources")
	}
	return refs, nil
}
