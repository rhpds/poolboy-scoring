package placement

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func boolPtr(b bool) *bool { return &b }

func TestParsePoolStatus(t *testing.T) {
	tests := []struct {
		name       string
		obj        *unstructured.Unstructured
		wantNil    bool
		wantAvail  *int64
		wantHandle []PoolHandleEntry
	}{
		{
			name: "full status with available count and handles",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"status": map[string]interface{}{
					"resourceHandleCount": map[string]interface{}{
						"available": int64(4),
					},
					"resourceHandles": []interface{}{
						map[string]interface{}{
							"name":    "guid-4w4l9",
							"healthy": true,
						},
						map[string]interface{}{
							"name":    "guid-h9fxn",
							"healthy": false,
						},
					},
				},
			}},
			wantAvail: int64Ptr(4),
			wantHandle: []PoolHandleEntry{
				{Name: "guid-4w4l9", Healthy: boolPtr(true)},
				{Name: "guid-h9fxn", Healthy: boolPtr(false)},
			},
		},
		{
			name: "available as float64 (JSON numeric)",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"status": map[string]interface{}{
					"resourceHandleCount": map[string]interface{}{
						"available": float64(3),
					},
				},
			}},
			wantAvail: int64Ptr(3),
		},
		{
			name: "available is zero",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"status": map[string]interface{}{
					"resourceHandleCount": map[string]interface{}{
						"available": int64(0),
					},
				},
			}},
			wantAvail: int64Ptr(0),
		},
		{
			name: "no resourceHandleCount",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"status": map[string]interface{}{
					"resourceHandles": []interface{}{
						map[string]interface{}{"name": "guid-abc"},
					},
				},
			}},
			wantAvail: nil,
			wantHandle: []PoolHandleEntry{
				{Name: "guid-abc"},
			},
		},
		{
			name:    "no status",
			obj:     &unstructured.Unstructured{Object: map[string]interface{}{}},
			wantNil: true,
		},
		{
			name: "handle without healthy field",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"status": map[string]interface{}{
					"resourceHandles": []interface{}{
						map[string]interface{}{
							"name": "guid-nohp",
						},
					},
				},
			}},
			wantHandle: []PoolHandleEntry{
				{Name: "guid-nohp", Healthy: nil},
			},
		},
		{
			name: "empty resourceHandles slice",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"status": map[string]interface{}{
					"resourceHandles": []interface{}{},
				},
			}},
			wantHandle: nil,
		},
		{
			name: "empty status object",
			obj: &unstructured.Unstructured{Object: map[string]interface{}{
				"status": map[string]interface{}{},
			}},
			wantAvail:  nil,
			wantHandle: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePoolStatus(tt.obj)
			if err != nil {
				t.Fatalf("ParsePoolStatus() error = %v", err)
			}
			if tt.wantNil {
				if got != nil {
					t.Errorf("ParsePoolStatus() = %+v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("ParsePoolStatus() = nil, want non-nil")
			}

			if tt.wantAvail == nil {
				if got.ResourceHandleCount != nil {
					t.Errorf("ResourceHandleCount = %+v, want nil", got.ResourceHandleCount)
				}
			} else {
				if got.ResourceHandleCount == nil {
					t.Fatalf("ResourceHandleCount = nil, want available=%d", *tt.wantAvail)
				}
				if got.ResourceHandleCount.Available != *tt.wantAvail {
					t.Errorf("Available = %d, want %d", got.ResourceHandleCount.Available, *tt.wantAvail)
				}
			}

			if len(tt.wantHandle) == 0 {
				if len(got.ResourceHandles) != 0 {
					t.Errorf("ResourceHandles = %+v, want empty", got.ResourceHandles)
				}
				return
			}
			if len(got.ResourceHandles) != len(tt.wantHandle) {
				t.Fatalf("ResourceHandles has %d entries, want %d", len(got.ResourceHandles), len(tt.wantHandle))
			}
			for i := range got.ResourceHandles {
				if got.ResourceHandles[i].Name != tt.wantHandle[i].Name {
					t.Errorf("entry[%d].Name = %q, want %q", i, got.ResourceHandles[i].Name, tt.wantHandle[i].Name)
				}
				gotH := got.ResourceHandles[i].Healthy
				wantH := tt.wantHandle[i].Healthy
				if (gotH == nil) != (wantH == nil) {
					t.Errorf("entry[%d].Healthy nil mismatch: got %v, want %v", i, gotH, wantH)
				} else if gotH != nil && *gotH != *wantH {
					t.Errorf("entry[%d].Healthy = %v, want %v", i, *gotH, *wantH)
				}
			}
		})
	}
}

func int64Ptr(v int64) *int64 { return &v }
