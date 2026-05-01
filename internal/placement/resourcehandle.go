package placement

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// IsHandleBound returns true if the ResourceHandle has a spec.resourceClaim,
// meaning it is already bound to a claim and should not be scored.
func IsHandleBound(obj *unstructured.Unstructured) bool {
	_, found, _ := unstructured.NestedMap(obj.Object, "spec", "resourceClaim")
	return found
}

// GetPlacementFromStatus reads status.placement from a ResourceHandle.
// This is the cached placement written by our controller after the first
// resolution. Returns (placement, true) if present, (nil, false) otherwise.
func GetPlacementFromStatus(obj *unstructured.Unstructured) (*Placement, bool) {
	p, found, _ := unstructured.NestedMap(obj.Object, "status", "placement")
	if !found || len(p) == 0 {
		return nil, false
	}

	clusterName, _, _ := unstructured.NestedString(obj.Object, "status", "placement", "clusterName")
	if clusterName == "" {
		return nil, false
	}

	name, _, _ := unstructured.NestedString(obj.Object, "status", "placement", "name")
	namespace, _, _ := unstructured.NestedString(obj.Object, "status", "placement", "namespace")

	return &Placement{
		ClusterName: clusterName,
		Name:        name,
		Namespace:   namespace,
	}, true
}

// GetPlacementFromProvisionData reads status.summary.provision_data to extract
// placement fields directly from the ResourceHandle. Some catalog items
// propagate sandbox_openshift_cluster into provision_data, saving an
// AnarchySubject API call.
// Returns (placement, true) if sandbox_openshift_cluster is present,
// (nil, false) otherwise.
func GetPlacementFromProvisionData(obj *unstructured.Unstructured) (*Placement, bool) {
	pd, found, _ := unstructured.NestedMap(obj.Object, "status", "summary", "provision_data")
	if !found || len(pd) == 0 {
		return nil, false
	}

	clusterRaw, ok := pd["sandbox_openshift_cluster"]
	if !ok {
		return nil, false
	}
	clusterName, ok := clusterRaw.(string)
	if !ok || clusterName == "" {
		return nil, false
	}

	var name, namespace string
	if v, ok := pd["sandbox_openshift_name"].(string); ok {
		name = v
	}
	if v, ok := pd["sandbox_openshift_namespace"].(string); ok {
		namespace = v
	}

	return &Placement{
		ClusterName: clusterName,
		Name:        name,
		Namespace:   namespace,
	}, true
}

// GetAnarchySubjectRefs returns all AnarchySubject references from
// status.resources[]. A handle can have multiple resources (e.g. an AWS
// sandbox at index 0 and a CNV workload at index 1), and the one with
// sandbox_openshift_cluster may not be at index 0. Only entries matching
// the AnarchySubject apiVersion and kind are returned.
func GetAnarchySubjectRefs(obj *unstructured.Unstructured) ([]ResourceRef, error) {
	resources, found, _ := unstructured.NestedSlice(obj.Object, "status", "resources")
	if !found || len(resources) == 0 {
		return nil, fmt.Errorf("no status.resources found")
	}

	var refs []ResourceRef
	for _, r := range resources {
		res, ok := r.(map[string]interface{})
		if !ok {
			continue
		}
		refRaw, ok := res["reference"]
		if !ok {
			continue
		}
		ref, ok := refRaw.(map[string]interface{})
		if !ok {
			continue
		}

		apiVersion, _ := ref["apiVersion"].(string)
		kind, _ := ref["kind"].(string)
		if apiVersion != "anarchy.gpte.redhat.com/v1" || kind != "AnarchySubject" {
			continue
		}

		name, _ := ref["name"].(string)
		if name == "" {
			continue
		}

		namespace, _ := ref["namespace"].(string)

		refs = append(refs, ResourceRef{
			APIVersion: apiVersion,
			Kind:       kind,
			Name:       name,
			Namespace:  namespace,
		})
	}

	if len(refs) == 0 {
		return nil, fmt.Errorf("no AnarchySubject references found in status.resources")
	}
	return refs, nil
}

// GetCurrentScore reads spec.preferenceScore from a ResourceHandle.
// Returns (score, true) if present, (0, false) if the field is not set.
func GetCurrentScore(obj *unstructured.Unstructured) (float64, bool) {
	score, found, _ := unstructured.NestedFloat64(obj.Object, "spec", "preferenceScore")
	return score, found
}
