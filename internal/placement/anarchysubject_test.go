package placement

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestParseAnarchySubjectSpec(t *testing.T) {
	tests := []struct {
		name       string
		obj        *unstructured.Unstructured
		wantVars   bool
		wantJobVar string
	}{
		{
			name: "full spec with job_vars",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"vars": map[string]interface{}{
						"job_vars": map[string]interface{}{
							"sandbox_openshift_cluster": "ocpv06",
						},
					},
				},
			}},
			wantVars:   true,
			wantJobVar: "ocpv06",
		},
		{
			name: "spec without vars",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"governor": "some-governor",
				},
			}},
			wantVars: false,
		},
		{
			name: "no spec",
			obj:  &unstructured.Unstructured{Object: map[string]interface{}{}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec, err := ParseAnarchySubjectSpec(tt.obj)
			if err != nil {
				t.Fatalf("ParseAnarchySubjectSpec() error = %v", err)
			}
			if spec == nil {
				t.Fatal("ParseAnarchySubjectSpec() returned nil")
			}
			if tt.wantVars {
				if spec.Vars == nil || spec.Vars.JobVars == nil {
					t.Fatal("expected Vars.JobVars to be set")
				}
				if got := spec.Vars.JobVars["sandbox_openshift_cluster"]; got != tt.wantJobVar {
					t.Errorf("sandbox_openshift_cluster = %v, want %v", got, tt.wantJobVar)
				}
			}
		})
	}
}

func TestExtractPlacement(t *testing.T) {
	tests := []struct {
		name        string
		spec        *AnarchySubjectSpec
		subjectName string
		want        *Placement
		wantErr     bool
	}{
		{
			name: "all three fields present (ocpvXX pattern)",
			spec: &AnarchySubjectSpec{
				Vars: &AnarchySubjectVars{
					JobVars: map[string]interface{}{
						"sandbox_openshift_cluster":   "ocpv06",
						"sandbox_openshift_name":      "2gdtf-1-f5da0b1e-abf4-5265-b9de-9e801f5b31af",
						"sandbox_openshift_namespace": "sandbox-2gdtf-1-ocp4-cluster",
					},
				},
			},
			subjectName: "enterprise.aap-prod-2gdtf-1",
			want: &Placement{
				ClusterName: "ocpv06",
				Name:        "2gdtf-1-f5da0b1e-abf4-5265-b9de-9e801f5b31af",
				Namespace:   "sandbox-2gdtf-1-ocp4-cluster",
			},
		},
		{
			name: "tenant cluster — cluster only, no namespace",
			spec: &AnarchySubjectSpec{
				Vars: &AnarchySubjectVars{
					JobVars: map[string]interface{}{
						"sandbox_openshift_cluster": "cluster-nm66z",
					},
				},
			},
			subjectName: "ai-qs-data-gov-tenant-c8z5s",
			want: &Placement{
				ClusterName: "cluster-nm66z",
			},
		},
		{
			name: "missing sandbox_openshift_cluster",
			spec: &AnarchySubjectSpec{
				Vars: &AnarchySubjectVars{
					JobVars: map[string]interface{}{
						"aws_region":     "us-east-2",
						"cloud_provider": "aws",
						"guid":           "vzrjg",
					},
				},
			},
			subjectName: "agd-v2.openshift-ai-v3-aws-vzrjg",
			wantErr:     true,
		},
		{
			name: "empty job_vars map",
			spec: &AnarchySubjectSpec{
				Vars: &AnarchySubjectVars{
					JobVars: map[string]interface{}{},
				},
			},
			subjectName: "some-subject",
			wantErr:     true,
		},
		{
			name: "missing job_vars entirely",
			spec: &AnarchySubjectSpec{
				Vars: &AnarchySubjectVars{},
			},
			subjectName: "some-subject",
			wantErr:     true,
		},
		{
			name:        "missing spec.vars entirely",
			spec:        &AnarchySubjectSpec{},
			subjectName: "some-subject",
			wantErr:     true,
		},
		{
			name:        "nil spec",
			spec:        nil,
			subjectName: "some-subject",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractPlacement(tt.spec, tt.subjectName)
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
