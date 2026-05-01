package placement

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// ExtractPlacement reads sandbox_openshift_* fields from an AnarchySubject's
// spec.vars.job_vars. Returns an error if sandbox_openshift_cluster is missing,
// which signals to the caller that this is not a CNV workload and should be
// skipped (e.g. an AWS sandbox or RHEL VM).
//
// sandbox_openshift_name and sandbox_openshift_namespace are optional — tenant
// clusters (e.g. cluster-nm66z) may have empty values for these fields.
func ExtractPlacement(obj *unstructured.Unstructured) (*Placement, error) {
	jobVars, found, _ := unstructured.NestedMap(obj.Object, "spec", "vars", "job_vars")
	if !found {
		return nil, fmt.Errorf("spec.vars.job_vars not found in AnarchySubject %s", obj.GetName())
	}

	clusterRaw, ok := jobVars["sandbox_openshift_cluster"]
	if !ok {
		return nil, fmt.Errorf("sandbox_openshift_cluster not found in job_vars of AnarchySubject %s", obj.GetName())
	}
	clusterName, ok := clusterRaw.(string)
	if !ok || clusterName == "" {
		return nil, fmt.Errorf("sandbox_openshift_cluster is empty in AnarchySubject %s", obj.GetName())
	}

	var name, namespace string
	if v, ok := jobVars["sandbox_openshift_name"].(string); ok {
		name = v
	}
	if v, ok := jobVars["sandbox_openshift_namespace"].(string); ok {
		namespace = v
	}

	return &Placement{
		ClusterName: clusterName,
		Name:        name,
		Namespace:   namespace,
	}, nil
}
