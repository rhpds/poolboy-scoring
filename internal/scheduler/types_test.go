package scheduler

import (
	"encoding/json"
	"testing"
	"time"
)

// stringPtr is a helper that returns a pointer to a string.
// In Go, you can't take the address of a literal ("foo") directly,
// so this helper avoids verbose inline temp variables.
func stringPtr(s string) *string {
	return &s
}

func TestCandidate_MarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    Candidate
		expected string
	}{
		{
			name: "all fields present",
			input: Candidate{
				ClusterName:     "ocpv05",
				HandleName:      stringPtr("abc12-98dab931"),
				HandleNamespace: stringPtr("sandbox-abc12-ocp4-cluster"),
			},
			expected: `{"cluster_name":"ocpv05","handle_name":"abc12-98dab931","handle_namespace":"sandbox-abc12-ocp4-cluster"}`,
		},
		{
			name: "only cluster_name",
			input: Candidate{
				ClusterName: "ocpv06",
			},
			expected: `{"cluster_name":"ocpv06"}`,
		},
		{
			name: "handle_name without handle_namespace",
			input: Candidate{
				ClusterName: "ocpv07",
				HandleName:  stringPtr("def34-04fa8c6b"),
			},
			expected: `{"cluster_name":"ocpv07","handle_name":"def34-04fa8c6b"}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := json.Marshal(tc.input)
			if err != nil {
				t.Fatalf("json.Marshal() error: %v", err)
			}
			if string(got) != tc.expected {
				t.Errorf("json.Marshal() =\n  %s\nwant:\n  %s", string(got), tc.expected)
			}
		})
	}
}

func TestEvaluateRequest_MarshalJSON(t *testing.T) {
	req := EvaluateRequest{
		Candidates: []Candidate{
			{ClusterName: "ocpv05", HandleName: stringPtr("abc12")},
			{ClusterName: "ocpv06"},
		},
	}

	got, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("json.Marshal() error: %v", err)
	}

	expected := `{"candidates":[{"cluster_name":"ocpv05","handle_name":"abc12"},{"cluster_name":"ocpv06"}]}`
	if string(got) != expected {
		t.Errorf("json.Marshal() =\n  %s\nwant:\n  %s", string(got), expected)
	}
}

func TestScoredCandidate_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name             string
		input            string
		expectedCluster  string
		expectedScore    float64
		expectedEligible bool
		hasReason        bool
	}{
		{
			name:             "eligible candidate",
			input:            `{"cluster_name":"ocpv05","handle_name":"abc12","score":82.5,"eligible":true}`,
			expectedCluster:  "ocpv05",
			expectedScore:    82.5,
			expectedEligible: true,
			hasReason:        false,
		},
		{
			name:             "ineligible candidate with reason",
			input:            `{"cluster_name":"ocpv06","score":0,"eligible":false,"ineligibility_reason":"cluster in maintenance"}`,
			expectedCluster:  "ocpv06",
			expectedScore:    0,
			expectedEligible: false,
			hasReason:        true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var sc ScoredCandidate
			if err := json.Unmarshal([]byte(tc.input), &sc); err != nil {
				t.Fatalf("json.Unmarshal() error: %v", err)
			}
			if sc.ClusterName != tc.expectedCluster {
				t.Errorf("ClusterName = %q, want %q", sc.ClusterName, tc.expectedCluster)
			}
			if sc.Score != tc.expectedScore {
				t.Errorf("Score = %v, want %v", sc.Score, tc.expectedScore)
			}
			if sc.Eligible != tc.expectedEligible {
				t.Errorf("Eligible = %v, want %v", sc.Eligible, tc.expectedEligible)
			}
			if tc.hasReason && sc.IneligibilityReason == nil {
				t.Error("IneligibilityReason is nil, want non-nil")
			}
			if !tc.hasReason && sc.IneligibilityReason != nil {
				t.Errorf("IneligibilityReason = %q, want nil", *sc.IneligibilityReason)
			}
		})
	}
}

func TestEvaluateResponse_UnmarshalJSON(t *testing.T) {
	input := `{
		"ranked": [
			{"cluster_name": "ocpv06", "handle_name": "def34", "score": 82.5, "eligible": true},
			{"cluster_name": "ocpv05", "handle_name": "abc12", "score": 65.3, "eligible": true}
		],
		"excluded": [
			{"cluster_name": "ocpv07", "score": 0, "eligible": false, "ineligibility_reason": "disabled"}
		],
		"strategy": "most_capacity",
		"generated_at": "2026-04-26T14:30:00Z"
	}`

	var resp EvaluateResponse
	if err := json.Unmarshal([]byte(input), &resp); err != nil {
		t.Fatalf("json.Unmarshal() error: %v", err)
	}

	if len(resp.Ranked) != 2 {
		t.Fatalf("len(Ranked) = %d, want 2", len(resp.Ranked))
	}
	if len(resp.Excluded) != 1 {
		t.Fatalf("len(Excluded) = %d, want 1", len(resp.Excluded))
	}
	if resp.Strategy != "most_capacity" {
		t.Errorf("Strategy = %q, want %q", resp.Strategy, "most_capacity")
	}

	expectedTime := time.Date(2026, 4, 26, 14, 30, 0, 0, time.UTC)
	if !resp.GeneratedAt.Equal(expectedTime) {
		t.Errorf("GeneratedAt = %v, want %v", resp.GeneratedAt, expectedTime)
	}

	if resp.Ranked[0].ClusterName != "ocpv06" {
		t.Errorf("Ranked[0].ClusterName = %q, want %q", resp.Ranked[0].ClusterName, "ocpv06")
	}
	if resp.Ranked[0].Score != 82.5 {
		t.Errorf("Ranked[0].Score = %v, want %v", resp.Ranked[0].Score, 82.5)
	}
	if resp.Excluded[0].IneligibilityReason == nil || *resp.Excluded[0].IneligibilityReason != "disabled" {
		t.Error("Excluded[0].IneligibilityReason should be 'disabled'")
	}
}

func TestEvaluateResponse_RoundTrip(t *testing.T) {
	original := EvaluateResponse{
		Ranked: []ScoredCandidate{
			{
				ClusterName:     "ocpv05",
				HandleName:      stringPtr("abc12"),
				HandleNamespace: stringPtr("sandbox-abc12"),
				Score:           75.0,
				Eligible:        true,
			},
		},
		Excluded: []ScoredCandidate{
			{
				ClusterName:         "ocpv08",
				Score:               0,
				Eligible:            false,
				IneligibilityReason: stringPtr("cooldown"),
			},
		},
		Strategy:    "most_capacity",
		GeneratedAt: time.Date(2026, 4, 30, 10, 0, 0, 0, time.UTC),
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal() error: %v", err)
	}

	var decoded EvaluateResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error: %v", err)
	}

	if len(decoded.Ranked) != len(original.Ranked) {
		t.Fatalf("Ranked length = %d, want %d", len(decoded.Ranked), len(original.Ranked))
	}
	if decoded.Ranked[0].ClusterName != original.Ranked[0].ClusterName {
		t.Errorf("Ranked[0].ClusterName = %q, want %q", decoded.Ranked[0].ClusterName, original.Ranked[0].ClusterName)
	}
	if decoded.Ranked[0].Score != original.Ranked[0].Score {
		t.Errorf("Ranked[0].Score = %v, want %v", decoded.Ranked[0].Score, original.Ranked[0].Score)
	}
	if decoded.Strategy != original.Strategy {
		t.Errorf("Strategy = %q, want %q", decoded.Strategy, original.Strategy)
	}
	if !decoded.GeneratedAt.Equal(original.GeneratedAt) {
		t.Errorf("GeneratedAt = %v, want %v", decoded.GeneratedAt, original.GeneratedAt)
	}
	if len(decoded.Excluded) != 1 {
		t.Fatalf("Excluded length = %d, want 1", len(decoded.Excluded))
	}
	if decoded.Excluded[0].IneligibilityReason == nil || *decoded.Excluded[0].IneligibilityReason != "cooldown" {
		t.Error("Excluded[0].IneligibilityReason should be 'cooldown'")
	}
}
