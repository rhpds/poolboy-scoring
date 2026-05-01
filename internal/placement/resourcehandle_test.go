package placement

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestParseHandleSpec(t *testing.T) {
	tests := []struct {
		name      string
		obj       *unstructured.Unstructured
		wantBound bool
		wantScore *float64
	}{
		{
			name: "bound handle with score",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"resourceClaim": map[string]interface{}{
						"apiVersion": "poolboy.gpte.redhat.com/v1",
						"kind":       "ResourceClaim",
						"name":       "claim-abc12",
						"namespace":  "user-foo",
					},
					"preferenceScore": float64(65.3),
				},
			}},
			wantBound: true,
			wantScore: float64Ptr(65.3),
		},
		{
			name: "unbound handle with score",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"preferenceScore": float64(80),
				},
			}},
			wantBound: false,
			wantScore: float64Ptr(80),
		},
		{
			name: "unbound handle without score",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"spec": map[string]interface{}{},
			}},
			wantBound: false,
			wantScore: nil,
		},
		{
			name: "no spec at all",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"metadata": map[string]interface{}{"name": "test"},
			}},
			wantBound: false,
			wantScore: nil,
		},
		{
			name: "score is zero (valid)",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"preferenceScore": float64(0),
				},
			}},
			wantScore: float64Ptr(0),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, err := ParseHandleSpec(tt.obj)
			if err != nil {
				t.Fatalf("ParseHandleSpec() error = %v", err)
			}
			if spec == nil {
				t.Fatal("ParseHandleSpec() returned nil")
			}

			gotBound := spec.ResourceClaim != nil
			if gotBound != tt.wantBound {
				t.Errorf("bound = %v, want %v", gotBound, tt.wantBound)
			}

			if tt.wantScore == nil {
				if spec.PreferenceScore != nil {
					t.Errorf("PreferenceScore = %v, want nil", *spec.PreferenceScore)
				}
			} else {
				if spec.PreferenceScore == nil {
					t.Fatalf("PreferenceScore = nil, want %v", *tt.wantScore)
				}
				if *spec.PreferenceScore != *tt.wantScore {
					t.Errorf("PreferenceScore = %v, want %v", *spec.PreferenceScore, *tt.wantScore)
				}
			}
		})
	}
}

func TestParseHandleStatus(t *testing.T) {
	tests := []struct {
		name           string
		obj            *unstructured.Unstructured
		wantNil        bool
		wantPlacements int
		wantResources  int
		wantSummary    bool
	}{
		{
			name: "full status with placements, resources, and summary",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"status": map[string]interface{}{
					"placements": []interface{}{
						map[string]interface{}{
							"clusterName": "ocpv06",
							"name":        "abc12-f5da0b1e",
							"namespace":   "sandbox-abc12-ocp4-cluster",
						},
					},
					"resources": []interface{}{
						map[string]interface{}{
							"name": "some-resource",
							"reference": map[string]interface{}{
								"apiVersion": "anarchy.gpte.redhat.com/v1",
								"kind":       "AnarchySubject",
								"name":       "subject-1",
								"namespace":  "babylon-anarchy-0",
							},
						},
					},
					"summary": map[string]interface{}{
						"provision_data": map[string]interface{}{
							"sandbox_openshift_cluster": "ocpv10",
						},
					},
				},
			}},
			wantPlacements: 1,
			wantResources:  1,
			wantSummary:    true,
		},
		{
			name: "multiple placements on different clusters",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"status": map[string]interface{}{
					"placements": []interface{}{
						map[string]interface{}{"clusterName": "ocpv10"},
						map[string]interface{}{"clusterName": "ocpv05"},
					},
				},
			}},
			wantPlacements: 2,
		},
		{
			name: "no status",
			obj:  &unstructured.Unstructured{Object: map[string]interface{}{}},
			wantNil: true,
		},
		{
			name: "empty status",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"status": map[string]interface{}{},
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, err := ParseHandleStatus(tt.obj)
			if err != nil {
				t.Fatalf("ParseHandleStatus() error = %v", err)
			}
			if tt.wantNil {
				if status != nil {
					t.Errorf("ParseHandleStatus() = %+v, want nil", status)
				}
				return
			}
			if status == nil {
				t.Fatal("ParseHandleStatus() = nil, want non-nil")
			}
			if len(status.Placements) != tt.wantPlacements {
				t.Errorf("Placements count = %d, want %d", len(status.Placements), tt.wantPlacements)
			}
			if len(status.Resources) != tt.wantResources {
				t.Errorf("Resources count = %d, want %d", len(status.Resources), tt.wantResources)
			}
			if tt.wantSummary && status.Summary == nil {
				t.Error("Summary = nil, want non-nil")
			}
			if !tt.wantSummary && status.Summary != nil {
				t.Errorf("Summary = %+v, want nil", status.Summary)
			}
		})
	}
}

func TestPlacementFromProvisionData(t *testing.T) {
	tests := []struct {
		name      string
		summary   *HandleSummary
		want      *Placement
		wantFound bool
	}{
		{
			name: "all three fields",
			summary: &HandleSummary{
				ProvisionData: map[string]interface{}{
					"cloud_provider":              "openshift_cnv",
					"sandbox_openshift_cluster":   "ocpv09",
					"sandbox_openshift_name":      "2s99p-abc123",
					"sandbox_openshift_namespace": "sandbox-2s99p-ocp4-cluster",
				},
			},
			want: &Placement{
				ClusterName: "ocpv09",
				Name:        "2s99p-abc123",
				Namespace:   "sandbox-2s99p-ocp4-cluster",
			},
			wantFound: true,
		},
		{
			name: "cluster only — sandbox_openshift_name is null in production",
			summary: &HandleSummary{
				ProvisionData: map[string]interface{}{
					"cloud_provider":              "openshift_cnv",
					"sandbox_openshift_cluster":   "ocpv10",
					"sandbox_openshift_name":      nil,
					"sandbox_openshift_namespace": "sandbox-2bclm-ocp4-cluster",
				},
			},
			want: &Placement{
				ClusterName: "ocpv10",
				Namespace:   "sandbox-2bclm-ocp4-cluster",
			},
			wantFound: true,
		},
		{
			name: "no sandbox_openshift_cluster — non-CNV provision_data",
			summary: &HandleSummary{
				ProvisionData: map[string]interface{}{
					"bastion_public_hostname": "bastion.abc.sandbox.com",
					"cloud_provider":          "openshift_cnv",
				},
			},
			wantFound: false,
		},
		{
			name: "empty provision_data",
			summary: &HandleSummary{
				ProvisionData: map[string]interface{}{},
			},
			wantFound: false,
		},
		{
			name:      "nil summary",
			summary:   nil,
			wantFound: false,
		},
		{
			name:      "nil provision_data",
			summary:   &HandleSummary{},
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := PlacementFromProvisionData(tt.summary)
			if found != tt.wantFound {
				t.Errorf("PlacementFromProvisionData() found = %v, want %v", found, tt.wantFound)
			}
			if tt.want == nil && got != nil {
				t.Errorf("PlacementFromProvisionData() = %+v, want nil", got)
			}
			if tt.want != nil {
				if got == nil {
					t.Fatalf("PlacementFromProvisionData() = nil, want %+v", tt.want)
				}
				if *got != *tt.want {
					t.Errorf("PlacementFromProvisionData() = %+v, want %+v", *got, *tt.want)
				}
			}
		})
	}
}

func TestAnarchySubjectRefsFromResources(t *testing.T) {
	tests := []struct {
		name      string
		resources []HandleResource
		want      []ResourceRef
		wantErr   bool
	}{
		{
			name: "single AnarchySubject reference",
			resources: []HandleResource{
				{
					Name: "some-resource",
					Reference: &ResourceRef{
						APIVersion: "anarchy.gpte.redhat.com/v1",
						Kind:       "AnarchySubject",
						Name:       "enterprise.aap-prod-2gdtf-1",
						Namespace:  "babylon-anarchy-0",
					},
				},
			},
			want: []ResourceRef{
				{
					APIVersion: "anarchy.gpte.redhat.com/v1",
					Kind:       "AnarchySubject",
					Name:       "enterprise.aap-prod-2gdtf-1",
					Namespace:  "babylon-anarchy-0",
				},
			},
		},
		{
			name: "multiple resources — mixed types",
			resources: []HandleResource{
				{
					Name: "aws",
					Reference: &ResourceRef{
						APIVersion: "anarchy.gpte.redhat.com/v1",
						Kind:       "AnarchySubject",
						Name:       "sandboxes-gpte.sandbox-open.prod-2gdtf",
						Namespace:  "babylon-anarchy-5",
					},
				},
				{
					Name: "cnv-workload",
					Reference: &ResourceRef{
						APIVersion: "anarchy.gpte.redhat.com/v1",
						Kind:       "AnarchySubject",
						Name:       "enterprise.aap-product-demos-cnv-aap25.prod-2gdtf-1",
						Namespace:  "babylon-anarchy-0",
					},
				},
			},
			want: []ResourceRef{
				{
					APIVersion: "anarchy.gpte.redhat.com/v1",
					Kind:       "AnarchySubject",
					Name:       "sandboxes-gpte.sandbox-open.prod-2gdtf",
					Namespace:  "babylon-anarchy-5",
				},
				{
					APIVersion: "anarchy.gpte.redhat.com/v1",
					Kind:       "AnarchySubject",
					Name:       "enterprise.aap-product-demos-cnv-aap25.prod-2gdtf-1",
					Namespace:  "babylon-anarchy-0",
				},
			},
		},
		{
			name: "skips entries without reference",
			resources: []HandleResource{
				{Name: "no-ref-resource"},
				{
					Name: "has-ref",
					Reference: &ResourceRef{
						APIVersion: "anarchy.gpte.redhat.com/v1",
						Kind:       "AnarchySubject",
						Name:       "valid-subject",
						Namespace:  "babylon-anarchy-0",
					},
				},
			},
			want: []ResourceRef{
				{
					APIVersion: "anarchy.gpte.redhat.com/v1",
					Kind:       "AnarchySubject",
					Name:       "valid-subject",
					Namespace:  "babylon-anarchy-0",
				},
			},
		},
		{
			name:      "empty resources slice",
			resources: []HandleResource{},
			wantErr:   true,
		},
		{
			name:    "nil resources",
			wantErr: true,
		},
		{
			name: "skips non-AnarchySubject references",
			resources: []HandleResource{
				{
					Name: "configmap-resource",
					Reference: &ResourceRef{
						APIVersion: "v1",
						Kind:       "ConfigMap",
						Name:       "some-config",
						Namespace:  "poolboy",
					},
				},
				{
					Name: "anarchy-resource",
					Reference: &ResourceRef{
						APIVersion: "anarchy.gpte.redhat.com/v1",
						Kind:       "AnarchySubject",
						Name:       "valid-subject",
						Namespace:  "babylon-anarchy-0",
					},
				},
			},
			want: []ResourceRef{
				{
					APIVersion: "anarchy.gpte.redhat.com/v1",
					Kind:       "AnarchySubject",
					Name:       "valid-subject",
					Namespace:  "babylon-anarchy-0",
				},
			},
		},
		{
			name: "all entries have empty names",
			resources: []HandleResource{
				{
					Reference: &ResourceRef{
						APIVersion: "anarchy.gpte.redhat.com/v1",
						Kind:       "AnarchySubject",
						Name:       "",
						Namespace:  "babylon-anarchy-0",
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := AnarchySubjectRefsFromResources(tt.resources)
			if (err != nil) != tt.wantErr {
				t.Errorf("AnarchySubjectRefsFromResources() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.want != nil {
				if len(got) != len(tt.want) {
					t.Fatalf("AnarchySubjectRefsFromResources() returned %d refs, want %d", len(got), len(tt.want))
				}
				for i := range tt.want {
					if got[i] != tt.want[i] {
						t.Errorf("AnarchySubjectRefsFromResources()[%d] = %+v, want %+v", i, got[i], tt.want[i])
					}
				}
			}
		})
	}
}

func float64Ptr(v float64) *float64 { return &v }
