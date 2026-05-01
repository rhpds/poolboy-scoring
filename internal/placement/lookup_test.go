package placement

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func newAnarchySubject(name, namespace string, jobVars map[string]interface{}) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "anarchy.gpte.redhat.com/v1",
		"kind":       "AnarchySubject",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
		"spec": map[string]interface{}{
			"vars": map[string]interface{}{
				"job_vars": jobVars,
			},
		},
	}}
	return obj
}

func newFakeClient(objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
	scheme := runtime.NewScheme()
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "anarchy.gpte.redhat.com", Version: "v1", Kind: "AnarchySubject"},
		&unstructured.Unstructured{},
	)
	scheme.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "anarchy.gpte.redhat.com", Version: "v1", Kind: "AnarchySubjectList"},
		&unstructured.UnstructuredList{},
	)
	return dynamicfake.NewSimpleDynamicClient(scheme, objects...)
}

func handleWithResources(refs ...map[string]interface{}) *unstructured.Unstructured {
	resources := make([]interface{}, len(refs))
	for i, ref := range refs {
		resources[i] = map[string]interface{}{
			"name":      ref["name"],
			"reference": ref,
		}
	}
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{
			"name":      "guid-test",
			"namespace": "poolboy",
		},
		"status": map[string]interface{}{
			"resources": resources,
		},
	}}
}

func anarchySubjectRef(name, namespace string) map[string]interface{} {
	return map[string]interface{}{
		"apiVersion": "anarchy.gpte.redhat.com/v1",
		"kind":       "AnarchySubject",
		"name":       name,
		"namespace":  namespace,
	}
}

func TestLookup(t *testing.T) {
	tests := []struct {
		name    string
		handle  *unstructured.Unstructured
		objects []runtime.Object
		want    []Placement
		wantErr bool
	}{
		{
			name: "cached placements — returns from status without API call",
			handle: &unstructured.Unstructured{Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"name": "guid-cached", "namespace": "poolboy",
				},
				"status": map[string]interface{}{
					"placements": []interface{}{
						map[string]interface{}{
							"clusterName": "ocpv06",
							"name":        "cached-name",
							"namespace":   "cached-ns",
						},
						map[string]interface{}{
							"clusterName": "ocpv09",
							"name":        "cached-name-2",
							"namespace":   "cached-ns-2",
						},
					},
				},
			}},
			want: []Placement{
				{ClusterName: "ocpv06", Name: "cached-name", Namespace: "cached-ns"},
				{ClusterName: "ocpv09", Name: "cached-name-2", Namespace: "cached-ns-2"},
			},
		},
		{
			name: "provision data shortcut — returns single-element slice",
			handle: &unstructured.Unstructured{Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"name": "guid-provdata", "namespace": "poolboy",
				},
				"status": map[string]interface{}{
					"summary": map[string]interface{}{
						"provision_data": map[string]interface{}{
							"sandbox_openshift_cluster":   "ocpv10",
							"sandbox_openshift_name":      "provdata-name",
							"sandbox_openshift_namespace": "provdata-ns",
						},
					},
				},
			}},
			want: []Placement{
				{ClusterName: "ocpv10", Name: "provdata-name", Namespace: "provdata-ns"},
			},
		},
		{
			name: "AnarchySubject fallback — single ref",
			handle: handleWithResources(
				anarchySubjectRef("subject-1", "babylon-anarchy-0"),
			),
			objects: []runtime.Object{
				newAnarchySubject("subject-1", "babylon-anarchy-0", map[string]interface{}{
					"sandbox_openshift_cluster":   "ocpv06",
					"sandbox_openshift_name":      "test-name",
					"sandbox_openshift_namespace": "test-ns",
				}),
			},
			want: []Placement{
				{ClusterName: "ocpv06", Name: "test-name", Namespace: "test-ns"},
			},
		},
		{
			name: "AnarchySubject fallback — multiple refs, all resolve",
			handle: handleWithResources(
				anarchySubjectRef("pool-provisioner", "babylon-anarchy-ocp"),
				anarchySubjectRef("workload-subject", "babylon-anarchy-0"),
			),
			objects: []runtime.Object{
				newAnarchySubject("pool-provisioner", "babylon-anarchy-ocp", map[string]interface{}{
					"sandbox_openshift_cluster":   "ocpv10",
					"sandbox_openshift_name":      "52vx4-pool",
					"sandbox_openshift_namespace": "sandbox-52vx4",
				}),
				newAnarchySubject("workload-subject", "babylon-anarchy-0", map[string]interface{}{
					"sandbox_openshift_cluster":   "ocpv05",
					"sandbox_openshift_name":      "52vx4-workload",
					"sandbox_openshift_namespace": "sandbox-52vx4",
				}),
			},
			want: []Placement{
				{ClusterName: "ocpv10", Name: "52vx4-pool", Namespace: "sandbox-52vx4"},
				{ClusterName: "ocpv05", Name: "52vx4-workload", Namespace: "sandbox-52vx4"},
			},
		},
		{
			name: "multiple refs — first has no cluster, second succeeds",
			handle: handleWithResources(
				anarchySubjectRef("aws-sandbox", "babylon-anarchy-5"),
				anarchySubjectRef("cnv-workload", "babylon-anarchy-0"),
			),
			objects: []runtime.Object{
				newAnarchySubject("aws-sandbox", "babylon-anarchy-5", map[string]interface{}{
					"cloud_provider": "aws",
					"aws_region":     "us-east-2",
				}),
				newAnarchySubject("cnv-workload", "babylon-anarchy-0", map[string]interface{}{
					"sandbox_openshift_cluster":   "ocpv06",
					"sandbox_openshift_name":      "workload-name",
					"sandbox_openshift_namespace": "workload-ns",
				}),
			},
			want: []Placement{
				{ClusterName: "ocpv06", Name: "workload-name", Namespace: "workload-ns"},
			},
		},
		{
			name: "all AnarchySubjects not found — returns error",
			handle: handleWithResources(
				anarchySubjectRef("missing-subject", "babylon-anarchy-0"),
			),
			wantErr: true,
		},
		{
			name: "no placement sources — no cache, no provision_data, no resources",
			handle: &unstructured.Unstructured{Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"name": "guid-empty", "namespace": "poolboy",
				},
				"status": map[string]interface{}{
					"healthy": true,
				},
			}},
			wantErr: true,
		},
		{
			name: "provision data takes priority over AnarchySubject refs",
			handle: &unstructured.Unstructured{Object: map[string]interface{}{
				"metadata": map[string]interface{}{
					"name": "guid-both", "namespace": "poolboy",
				},
				"status": map[string]interface{}{
					"summary": map[string]interface{}{
						"provision_data": map[string]interface{}{
							"sandbox_openshift_cluster":   "ocpv09",
							"sandbox_openshift_name":      "from-provdata",
							"sandbox_openshift_namespace": "provdata-ns",
						},
					},
					"resources": []interface{}{
						map[string]interface{}{
							"name": "should-not-be-fetched",
							"reference": map[string]interface{}{
								"apiVersion": "anarchy.gpte.redhat.com/v1",
								"kind":       "AnarchySubject",
								"name":       "subject-not-fetched",
								"namespace":  "babylon-anarchy-0",
							},
						},
					},
				},
			}},
			want: []Placement{
				{ClusterName: "ocpv09", Name: "from-provdata", Namespace: "provdata-ns"},
			},
		},
		{
			name: "mirrors guid-52vx4 — both refs on different clusters",
			handle: handleWithResources(
				anarchySubjectRef("agd-v2.ocp-cluster-cnv-pools.prod-52vx4", "babylon-anarchy-ocp-wksp"),
				anarchySubjectRef("openshift-cnv.demystifying-aap.prod-52vx4-1", "babylon-anarchy-sandboxes"),
			),
			objects: []runtime.Object{
				newAnarchySubject("agd-v2.ocp-cluster-cnv-pools.prod-52vx4", "babylon-anarchy-ocp-wksp", map[string]interface{}{
					"sandbox_openshift_cluster":   "ocpv10",
					"sandbox_openshift_name":      "52vx4-infra",
					"sandbox_openshift_namespace": "sandbox-52vx4-ocp4-cluster",
				}),
				newAnarchySubject("openshift-cnv.demystifying-aap.prod-52vx4-1", "babylon-anarchy-sandboxes", map[string]interface{}{
					"sandbox_openshift_cluster":   "ocpv05",
					"sandbox_openshift_name":      "52vx4-workload",
					"sandbox_openshift_namespace": "sandbox-52vx4-ocp4-cluster",
				}),
			},
			want: []Placement{
				{ClusterName: "ocpv10", Name: "52vx4-infra", Namespace: "sandbox-52vx4-ocp4-cluster"},
				{ClusterName: "ocpv05", Name: "52vx4-workload", Namespace: "sandbox-52vx4-ocp4-cluster"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newFakeClient(tt.objects...)
			lookup := NewLookup(client)

			got, err := lookup.Lookup(context.Background(), tt.handle)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Lookup() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.want == nil {
				return
			}

			if len(got) != len(tt.want) {
				t.Fatalf("Lookup() returned %d placements, want %d: %+v", len(got), len(tt.want), got)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("Lookup()[%d] = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestLookupErrorMessages(t *testing.T) {
	t.Run("404 error includes AnarchySubject and handle identifiers", func(t *testing.T) {
		handle := handleWithResources(
			anarchySubjectRef("missing-subject", "babylon-anarchy-0"),
		)
		handle.SetName("guid-test-handle")
		handle.SetNamespace("poolboy")

		client := newFakeClient()
		lookup := NewLookup(client)

		_, err := lookup.Lookup(context.Background(), handle)
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		msg := err.Error()
		for _, want := range []string{"missing-subject", "babylon-anarchy-0", "guid-test-handle", "poolboy"} {
			if !contains(msg, want) {
				t.Errorf("error message %q does not contain %q", msg, want)
			}
		}
	})

	t.Run("no resources error includes handle identifiers", func(t *testing.T) {
		handle := &unstructured.Unstructured{Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"name": "guid-no-res", "namespace": "poolboy",
			},
			"status": map[string]interface{}{},
		}}

		client := newFakeClient()
		lookup := NewLookup(client)

		_, err := lookup.Lookup(context.Background(), handle)
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		msg := err.Error()
		for _, want := range []string{"guid-no-res", "poolboy"} {
			if !contains(msg, want) {
				t.Errorf("error message %q does not contain %q", msg, want)
			}
		}
	})
}

func TestLookupCachedPlacementsSkipsDynamicClient(t *testing.T) {
	handle := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{
			"name": "guid-cached", "namespace": "poolboy",
		},
		"status": map[string]interface{}{
			"placements": []interface{}{
				map[string]interface{}{
					"clusterName": "ocpv06",
				},
			},
		},
	}}

	// Pass nil dynamic client — if Lookup tries to use it, it panics
	lookup := &PlacementLookup{dynamicClient: nil}

	got, err := lookup.Lookup(context.Background(), handle)
	if err != nil {
		t.Fatalf("Lookup() error = %v", err)
	}
	if len(got) != 1 || got[0].ClusterName != "ocpv06" {
		t.Errorf("Lookup() = %+v, want [{ClusterName: ocpv06}]", got)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestLookupPartialFailure(t *testing.T) {
	handle := handleWithResources(
		anarchySubjectRef("missing-subject", "babylon-anarchy-0"),
		anarchySubjectRef("valid-subject", "babylon-anarchy-1"),
	)

	validSubject := newAnarchySubject("valid-subject", "babylon-anarchy-1", map[string]interface{}{
		"sandbox_openshift_cluster": "ocpv08",
	})

	client := newFakeClient(validSubject)
	lookup := NewLookup(client)

	got, err := lookup.Lookup(context.Background(), handle)
	if err != nil {
		t.Fatalf("Lookup() should succeed with partial results, got error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("Lookup() returned %d placements, want 1", len(got))
	}
	if got[0].ClusterName != "ocpv08" {
		t.Errorf("Lookup()[0].ClusterName = %q, want %q", got[0].ClusterName, "ocpv08")
	}
}

func TestNewLookup(t *testing.T) {
	client := newFakeClient()
	lookup := NewLookup(client)
	if lookup == nil {
		t.Fatal("NewLookup() returned nil")
	}
	if lookup.dynamicClient == nil {
		t.Fatal("NewLookup() did not set dynamicClient")
	}

	// Verify the dynamic client is functional by doing a Get
	// that should return not-found (not panic)
	_, err := lookup.dynamicClient.
		Resource(AnarchySubjectGVR).
		Namespace("test").
		Get(context.Background(), "nonexistent", metav1.GetOptions{})
	if err == nil {
		t.Fatal("expected not-found error, got nil")
	}
}
