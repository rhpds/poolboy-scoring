package placement

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestExtractPlacement(t *testing.T) {
	tests := []struct {
		name    string
		obj     *unstructured.Unstructured
		want    *Placement
		wantErr bool
	}{
		{
			name: "all three fields present (ocpvXX pattern)",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"name": "enterprise.aap-prod-2gdtf-1",
				},
				"spec": map[string]interface{}{
					"vars": map[string]interface{}{
						"job_vars": map[string]interface{}{
							"sandbox_openshift_cluster":   "ocpv06",
							"sandbox_openshift_name":      "2gdtf-1-f5da0b1e-abf4-5265-b9de-9e801f5b31af",
							"sandbox_openshift_namespace": "sandbox-2gdtf-1-ocp4-cluster",
						},
					},
				},
			}},
			want: &Placement{
				ClusterName: "ocpv06",
				Name:        "2gdtf-1-f5da0b1e-abf4-5265-b9de-9e801f5b31af",
				Namespace:   "sandbox-2gdtf-1-ocp4-cluster",
			},
		},
		{
			name: "tenant cluster — cluster only, no namespace",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"name": "ai-qs-data-gov-tenant-c8z5s",
				},
				"spec": map[string]interface{}{
					"vars": map[string]interface{}{
						"job_vars": map[string]interface{}{
							"sandbox_openshift_cluster": "cluster-nm66z",
						},
					},
				},
			}},
			want: &Placement{
				ClusterName: "cluster-nm66z",
			},
		},
		{
			name: "missing sandbox_openshift_cluster (non-CNV workload)",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"name": "agd-v2.openshift-ai-v3-aws-vzrjg",
				},
				"spec": map[string]interface{}{
					"vars": map[string]interface{}{
						"job_vars": map[string]interface{}{
							"aws_region":     "us-east-2",
							"cloud_provider": "aws",
							"guid":           "vzrjg",
						},
					},
				},
			}},
			wantErr: true,
		},
		{
			name: "empty job_vars map",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"name": "some-subject",
				},
				"spec": map[string]interface{}{
					"vars": map[string]interface{}{
						"job_vars": map[string]interface{}{},
					},
				},
			}},
			wantErr: true,
		},
		{
			name: "missing job_vars entirely",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"name": "some-subject",
				},
				"spec": map[string]interface{}{
					"vars": map[string]interface{}{
						"current_state": "started",
					},
				},
			}},
			wantErr: true,
		},
		{
			name: "missing spec.vars entirely",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"name": "some-subject",
				},
				"spec": map[string]interface{}{
					"governor": "some-governor",
				},
			}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractPlacement(tt.obj)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExtractPlacement() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.want != nil {
				if got == nil {
					t.Fatalf("ExtractPlacement() = nil, want %+v", tt.want)
				}
				if *got != *tt.want {
					t.Errorf("ExtractPlacement() = %+v, want %+v", *got, *tt.want)
				}
			}
		})
	}
}
