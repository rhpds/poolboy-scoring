package placement

import "k8s.io/apimachinery/pkg/runtime/schema"

// Placement holds the three values that identify where a workload runs on a
// target cluster. Extracted from an AnarchySubject's spec.vars.job_vars or
// from a ResourceHandle's status.summary.provision_data.
//
// ClusterName is always required (e.g. "ocpv06" or "cluster-nm66z").
// Name and Namespace may be empty — tenant clusters often omit namespace.
type Placement struct {
	ClusterName string `json:"clusterName"`
	Name        string `json:"name,omitempty"`
	Namespace   string `json:"namespace,omitempty"`
}

// ResourceRef is a reference to a Kubernetes resource. Used to follow the link
// from ResourceHandle status.resources[].reference to the managed AnarchySubject.
type ResourceRef struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Namespace  string `json:"namespace,omitempty"`
}

// PoolHandleEntry represents one entry from ResourcePool.status.resourceHandles[].
// The healthy field uses *bool to distinguish "healthy: false" from "healthy not set".
type PoolHandleEntry struct {
	Name    string `json:"name"`
	Healthy *bool  `json:"healthy,omitempty"`
}

// --- ResourcePool partial types ---

type ResourcePoolStatus struct {
	ResourceHandleCount *ResourceHandleCount `json:"resourceHandleCount,omitempty"`
	ResourceHandles     []PoolHandleEntry    `json:"resourceHandles,omitempty"`
}

type ResourceHandleCount struct {
	Available int64 `json:"available"`
}

// --- ResourceHandle partial types ---

type ResourceHandleSpec struct {
	ResourceClaim   map[string]interface{} `json:"resourceClaim,omitempty"`
	PreferenceScore *float64               `json:"preferenceScore,omitempty"`
}

type ResourceHandleStatus struct {
	Summary    *HandleSummary   `json:"summary,omitempty"`
	Resources  []HandleResource `json:"resources,omitempty"`
	Placements []Placement      `json:"placements,omitempty"`
}

type HandleSummary struct {
	ProvisionData map[string]interface{} `json:"provision_data,omitempty"`
}

type HandleResource struct {
	Name      string       `json:"name,omitempty"`
	Reference *ResourceRef `json:"reference,omitempty"`
}

// --- AnarchySubject partial types ---

type AnarchySubjectSpec struct {
	Vars *AnarchySubjectVars `json:"vars,omitempty"`
}

type AnarchySubjectVars struct {
	JobVars map[string]interface{} `json:"job_vars,omitempty"`
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

	ResourcePoolGVK = schema.GroupVersionKind{
		Group:   "poolboy.gpte.redhat.com",
		Version: "v1",
		Kind:    "ResourcePool",
	}
)
