package placement

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestIsHandleBound(t *testing.T) {
	tests := []struct {
		name string
		obj  *unstructured.Unstructured
		want bool
	}{
		{
			name: "bound handle",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"resourceClaim": map[string]interface{}{
						"apiVersion": "poolboy.gpte.redhat.com/v1",
						"kind":       "ResourceClaim",
						"name":       "claim-abc12",
						"namespace":  "user-foo",
					},
				},
			}},
			want: true,
		},
		{
			name: "unbound handle",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"preferenceScore": float64(0),
				},
			}},
			want: false,
		},
		{
			name: "empty spec",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"spec": map[string]interface{}{},
			}},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsHandleBound(tt.obj)
			if got != tt.want {
				t.Errorf("IsHandleBound() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetPlacementFromStatus(t *testing.T) {
	tests := []struct {
		name      string
		obj       *unstructured.Unstructured
		want      *Placement
		wantFound bool
	}{
		{
			name: "all fields present",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"status": map[string]interface{}{
					"placement": map[string]interface{}{
						"clusterName": "ocpv06",
						"name":        "abc12-f5da0b1e-abf4-5265",
						"namespace":   "sandbox-abc12-ocp4-cluster",
					},
				},
			}},
			want: &Placement{
				ClusterName: "ocpv06",
				Name:        "abc12-f5da0b1e-abf4-5265",
				Namespace:   "sandbox-abc12-ocp4-cluster",
			},
			wantFound: true,
		},
		{
			name: "cluster only (tenant pattern)",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"status": map[string]interface{}{
					"placement": map[string]interface{}{
						"clusterName": "cluster-nm66z",
					},
				},
			}},
			want: &Placement{
				ClusterName: "cluster-nm66z",
			},
			wantFound: true,
		},
		{
			name: "missing placement",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"status": map[string]interface{}{
					"healthy": true,
				},
			}},
			want:      nil,
			wantFound: false,
		},
		{
			name: "empty placement map",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"status": map[string]interface{}{
					"placement": map[string]interface{}{},
				},
			}},
			want:      nil,
			wantFound: false,
		},
		{
			name: "placement without clusterName",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"status": map[string]interface{}{
					"placement": map[string]interface{}{
						"name":      "some-name",
						"namespace": "some-ns",
					},
				},
			}},
			want:      nil,
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := GetPlacementFromStatus(tt.obj)
			if found != tt.wantFound {
				t.Errorf("GetPlacementFromStatus() found = %v, want %v", found, tt.wantFound)
			}
			if tt.want == nil && got != nil {
				t.Errorf("GetPlacementFromStatus() = %+v, want nil", got)
			}
			if tt.want != nil {
				if got == nil {
					t.Fatalf("GetPlacementFromStatus() = nil, want %+v", tt.want)
				}
				if *got != *tt.want {
					t.Errorf("GetPlacementFromStatus() = %+v, want %+v", *got, *tt.want)
				}
			}
		})
	}
}

func TestGetPlacementFromProvisionData(t *testing.T) {
	tests := []struct {
		name      string
		obj       *unstructured.Unstructured
		want      *Placement
		wantFound bool
	}{
		{
			name: "all three fields",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"status": map[string]interface{}{
					"summary": map[string]interface{}{
						"provision_data": map[string]interface{}{
							"cloud_provider":             "openshift_cnv",
							"sandbox_openshift_cluster":  "ocpv09",
							"sandbox_openshift_name":     "2s99p-abc123",
							"sandbox_openshift_namespace": "sandbox-2s99p-ocp4-cluster",
						},
					},
				},
			}},
			want: &Placement{
				ClusterName: "ocpv09",
				Name:        "2s99p-abc123",
				Namespace:   "sandbox-2s99p-ocp4-cluster",
			},
			wantFound: true,
		},
		{
			name: "cluster only — sandbox_openshift_name is null in production",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"status": map[string]interface{}{
					"summary": map[string]interface{}{
						"provision_data": map[string]interface{}{
							"cloud_provider":              "openshift_cnv",
							"sandbox_openshift_cluster":   "ocpv10",
							"sandbox_openshift_name":      nil,
							"sandbox_openshift_namespace": "sandbox-2bclm-ocp4-cluster",
						},
					},
				},
			}},
			want: &Placement{
				ClusterName: "ocpv10",
				Namespace:   "sandbox-2bclm-ocp4-cluster",
			},
			wantFound: true,
		},
		{
			name: "no sandbox_openshift_cluster — non-CNV provision_data",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"status": map[string]interface{}{
					"summary": map[string]interface{}{
						"provision_data": map[string]interface{}{
							"bastion_public_hostname": "bastion.abc.sandbox.com",
							"cloud_provider":          "openshift_cnv",
						},
					},
				},
			}},
			want:      nil,
			wantFound: false,
		},
		{
			name: "no provision_data at all",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"status": map[string]interface{}{
					"summary": map[string]interface{}{
						"state": "started",
					},
				},
			}},
			want:      nil,
			wantFound: false,
		},
		{
			name: "no summary at all",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"status": map[string]interface{}{},
			}},
			want:      nil,
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := GetPlacementFromProvisionData(tt.obj)
			if found != tt.wantFound {
				t.Errorf("GetPlacementFromProvisionData() found = %v, want %v", found, tt.wantFound)
			}
			if tt.want == nil && got != nil {
				t.Errorf("GetPlacementFromProvisionData() = %+v, want nil", got)
			}
			if tt.want != nil {
				if got == nil {
					t.Fatalf("GetPlacementFromProvisionData() = nil, want %+v", tt.want)
				}
				if *got != *tt.want {
					t.Errorf("GetPlacementFromProvisionData() = %+v, want %+v", *got, *tt.want)
				}
			}
		})
	}
}

func TestGetAnarchySubjectRefs(t *testing.T) {
	tests := []struct {
		name    string
		obj     *unstructured.Unstructured
		want    []ResourceRef
		wantErr bool
	}{
		{
			name: "single resource",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"status": map[string]interface{}{
					"resources": []interface{}{
						map[string]interface{}{
							"name":    "some-resource",
							"healthy": true,
							"reference": map[string]interface{}{
								"apiVersion": "anarchy.gpte.redhat.com/v1",
								"kind":       "AnarchySubject",
								"name":       "enterprise.aap-prod-2gdtf-1",
								"namespace":  "babylon-anarchy-0",
							},
						},
					},
				},
			}},
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
			name: "multiple resources — AWS sandbox at 0, CNV workload at 1",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"status": map[string]interface{}{
					"resources": []interface{}{
						map[string]interface{}{
							"name": "aws",
							"reference": map[string]interface{}{
								"apiVersion": "anarchy.gpte.redhat.com/v1",
								"kind":       "AnarchySubject",
								"name":       "sandboxes-gpte.sandbox-open.prod-2gdtf",
								"namespace":  "babylon-anarchy-5",
							},
						},
						map[string]interface{}{
							"name": "enterprise.aap-product-demos-cnv-aap25.prod",
							"reference": map[string]interface{}{
								"apiVersion": "anarchy.gpte.redhat.com/v1",
								"kind":       "AnarchySubject",
								"name":       "enterprise.aap-product-demos-cnv-aap25.prod-2gdtf-1",
								"namespace":  "babylon-anarchy-0",
							},
						},
					},
				},
			}},
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
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"status": map[string]interface{}{
					"resources": []interface{}{
						map[string]interface{}{
							"name":    "no-ref-resource",
							"healthy": true,
						},
						map[string]interface{}{
							"name": "has-ref",
							"reference": map[string]interface{}{
								"apiVersion": "anarchy.gpte.redhat.com/v1",
								"kind":       "AnarchySubject",
								"name":       "valid-subject",
								"namespace":  "babylon-anarchy-0",
							},
						},
					},
				},
			}},
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
			name: "empty resources slice",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"status": map[string]interface{}{
					"resources": []interface{}{},
				},
			}},
			wantErr: true,
		},
		{
			name: "no status.resources",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"status": map[string]interface{}{
					"healthy": true,
				},
			}},
			wantErr: true,
		},
		{
			name: "skips non-AnarchySubject references",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"status": map[string]interface{}{
					"resources": []interface{}{
						map[string]interface{}{
							"name": "configmap-resource",
							"reference": map[string]interface{}{
								"apiVersion": "v1",
								"kind":       "ConfigMap",
								"name":       "some-config",
								"namespace":  "poolboy",
							},
						},
						map[string]interface{}{
							"name": "anarchy-resource",
							"reference": map[string]interface{}{
								"apiVersion": "anarchy.gpte.redhat.com/v1",
								"kind":       "AnarchySubject",
								"name":       "valid-subject",
								"namespace":  "babylon-anarchy-0",
							},
						},
					},
				},
			}},
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
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"status": map[string]interface{}{
					"resources": []interface{}{
						map[string]interface{}{
							"reference": map[string]interface{}{
								"apiVersion": "anarchy.gpte.redhat.com/v1",
								"kind":       "AnarchySubject",
								"name":       "",
								"namespace":  "babylon-anarchy-0",
							},
						},
					},
				},
			}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetAnarchySubjectRefs(tt.obj)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetAnarchySubjectRefs() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.want != nil {
				if len(got) != len(tt.want) {
					t.Fatalf("GetAnarchySubjectRefs() returned %d refs, want %d", len(got), len(tt.want))
				}
				for i := range tt.want {
					if got[i] != tt.want[i] {
						t.Errorf("GetAnarchySubjectRefs()[%d] = %+v, want %+v", i, got[i], tt.want[i])
					}
				}
			}
		})
	}
}

func TestGetPlacementsFromStatus(t *testing.T) {
	tests := []struct {
		name      string
		obj       *unstructured.Unstructured
		want      []Placement
		wantFound bool
	}{
		{
			name: "single placement",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"status": map[string]interface{}{
					"placements": []interface{}{
						map[string]interface{}{
							"clusterName": "ocpv06",
							"name":        "abc12-f5da0b1e",
							"namespace":   "sandbox-abc12-ocp4-cluster",
						},
					},
				},
			}},
			want: []Placement{
				{ClusterName: "ocpv06", Name: "abc12-f5da0b1e", Namespace: "sandbox-abc12-ocp4-cluster"},
			},
			wantFound: true,
		},
		{
			name: "multiple placements on different clusters",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"status": map[string]interface{}{
					"placements": []interface{}{
						map[string]interface{}{
							"clusterName": "ocpv10",
							"name":        "52vx4-pool",
							"namespace":   "sandbox-52vx4-ocp4-cluster",
						},
						map[string]interface{}{
							"clusterName": "ocpv05",
							"name":        "52vx4-workload",
							"namespace":   "sandbox-52vx4-ocp4-cluster",
						},
					},
				},
			}},
			want: []Placement{
				{ClusterName: "ocpv10", Name: "52vx4-pool", Namespace: "sandbox-52vx4-ocp4-cluster"},
				{ClusterName: "ocpv05", Name: "52vx4-workload", Namespace: "sandbox-52vx4-ocp4-cluster"},
			},
			wantFound: true,
		},
		{
			name: "tenant cluster — no name or namespace",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"status": map[string]interface{}{
					"placements": []interface{}{
						map[string]interface{}{
							"clusterName": "cluster-nm66z",
						},
					},
				},
			}},
			want: []Placement{
				{ClusterName: "cluster-nm66z"},
			},
			wantFound: true,
		},
		{
			name: "empty placements array",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"status": map[string]interface{}{
					"placements": []interface{}{},
				},
			}},
			wantFound: false,
		},
		{
			name: "missing placements field",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"status": map[string]interface{}{
					"healthy": true,
				},
			}},
			wantFound: false,
		},
		{
			name: "skips entries without clusterName",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"status": map[string]interface{}{
					"placements": []interface{}{
						map[string]interface{}{
							"name":      "some-name",
							"namespace": "some-ns",
						},
						map[string]interface{}{
							"clusterName": "ocpv09",
							"name":        "valid",
						},
					},
				},
			}},
			want: []Placement{
				{ClusterName: "ocpv09", Name: "valid"},
			},
			wantFound: true,
		},
		{
			name: "all entries missing clusterName",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"status": map[string]interface{}{
					"placements": []interface{}{
						map[string]interface{}{
							"name": "no-cluster",
						},
					},
				},
			}},
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := GetPlacementsFromStatus(tt.obj)
			if found != tt.wantFound {
				t.Errorf("GetPlacementsFromStatus() found = %v, want %v", found, tt.wantFound)
			}
			if tt.want == nil && got != nil {
				t.Errorf("GetPlacementsFromStatus() = %+v, want nil", got)
			}
			if tt.want != nil {
				if len(got) != len(tt.want) {
					t.Fatalf("GetPlacementsFromStatus() returned %d placements, want %d", len(got), len(tt.want))
				}
				for i := range tt.want {
					if got[i] != tt.want[i] {
						t.Errorf("GetPlacementsFromStatus()[%d] = %+v, want %+v", i, got[i], tt.want[i])
					}
				}
			}
		})
	}
}

func TestGetCurrentScore(t *testing.T) {
	tests := []struct {
		name      string
		obj       *unstructured.Unstructured
		want      float64
		wantFound bool
	}{
		{
			name: "score set",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"preferenceScore": float64(65.3),
				},
			}},
			want:      65.3,
			wantFound: true,
		},
		{
			name: "score not set",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"spec": map[string]interface{}{},
			}},
			want:      0,
			wantFound: false,
		},
		{
			name: "score is zero (valid)",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"preferenceScore": float64(0),
				},
			}},
			want:      0,
			wantFound: true,
		},
		{
			name: "score stored as int64 (round number)",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"preferenceScore": int64(50),
				},
			}},
			want:      50.0,
			wantFound: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := GetCurrentScore(tt.obj)
			if found != tt.wantFound {
				t.Errorf("GetCurrentScore() found = %v, want %v", found, tt.wantFound)
			}
			if got != tt.want {
				t.Errorf("GetCurrentScore() = %v, want %v", got, tt.want)
			}
		})
	}
}
