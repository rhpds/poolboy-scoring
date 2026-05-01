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

// GVR constants for the Kubernetes dynamic client.
// These are var (not const) because schema.GroupVersionResource is a struct,
// and Go only allows const for primitive types.
var (
	ResourceHandleGVR = schema.GroupVersionResource{
		Group:    "poolboy.gpte.redhat.com",
		Version:  "v1",
		Resource: "resourcehandles",
	}

	AnarchySubjectGVR = schema.GroupVersionResource{
		Group:    "anarchy.gpte.redhat.com",
		Version:  "v1",
		Resource: "anarchysubjects",
	}

	// ResourceHandleGVK is the GroupVersionKind for ResourceHandle.
	// GVR (above) uses the plural resource name for REST endpoints via the dynamic client.
	// GVK uses the singular Kind for type identification via controller-runtime's client.Client.
	ResourceHandleGVK = schema.GroupVersionKind{
		Group:   "poolboy.gpte.redhat.com",
		Version: "v1",
		Kind:    "ResourceHandle",
	}
)
