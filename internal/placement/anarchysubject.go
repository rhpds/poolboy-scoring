package placement

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

// ParseAnarchySubjectSpec converts the spec section of an AnarchySubject into
// a typed struct. Returns a zero-value spec (not nil) if the field is missing.
func ParseAnarchySubjectSpec(obj *unstructured.Unstructured) (*AnarchySubjectSpec, error) {
	raw, found, err := unstructured.NestedMap(obj.Object, "spec")
	if err != nil {
		return nil, fmt.Errorf("reading spec from AnarchySubject %s: %w", obj.GetName(), err)
	}
	if !found {
		return &AnarchySubjectSpec{}, nil
	}
	var spec AnarchySubjectSpec
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(raw, &spec); err != nil {
		return nil, err
	}
	return &spec, nil
}

// ExtractPlacement reads sandbox_openshift_* fields from a parsed
// AnarchySubjectSpec's job_vars. Returns an error if sandbox_openshift_cluster
// is missing or empty.
func ExtractPlacement(spec *AnarchySubjectSpec, subjectName string) (*Placement, error) {
	if spec == nil || spec.Vars == nil || spec.Vars.JobVars == nil {
		return nil, fmt.Errorf("spec.vars.job_vars not found in AnarchySubject %s", subjectName)
	}
	jobVars := spec.Vars.JobVars

	clusterRaw, ok := jobVars["sandbox_openshift_cluster"]
	if !ok {
		return nil, fmt.Errorf("sandbox_openshift_cluster not found in job_vars of AnarchySubject %s", subjectName)
	}
	clusterName, ok := clusterRaw.(string)
	if !ok || clusterName == "" {
		return nil, fmt.Errorf("sandbox_openshift_cluster is empty in AnarchySubject %s", subjectName)
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
