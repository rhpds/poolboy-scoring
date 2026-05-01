package placement

import "k8s.io/apimachinery/pkg/runtime/schema"

// Placement holds the three values that identify where a workload runs on a
// target cluster. Extracted from an AnarchySubject's spec.vars.job_vars or
// from a ResourceHandle's status.summary.provision_data.
//
// ClusterName is always required (e.g. "ocpv06" or "cluster-nm66z").
// Name and Namespace may be empty — tenant clusters often omit namespace.
type Placement struct {
	ClusterName string
	Name        string
	Namespace   string
}

// ResourceRef is a reference to a Kubernetes resource. Used to follow the link
// from ResourceHandle status.resources[0].reference to the managed AnarchySubject.
type ResourceRef struct {
	APIVersion string
	Kind       string
	Name       string
	Namespace  string
}

// GVK constants for controller-runtime's client.Client.
// These are var (not const) because schema.GroupVersionKind is a struct,
// and Go only allows const for primitive types.
var (
	ResourceHandleGVK = schema.GroupVersionKind{
		Group:   "poolboy.gpte.redhat.com",
		Version: "v1",
		Kind:    "ResourceHandle",
	}

	AnarchySubjectGVK = schema.GroupVersionKind{
		Group:   "anarchy.gpte.redhat.com",
		Version: "v1",
		Kind:    "AnarchySubject",
	}
)
