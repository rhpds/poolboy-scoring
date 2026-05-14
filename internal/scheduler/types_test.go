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
				Name:            "ocpv05",
				HandleName:      stringPtr("abc12-98dab931"),
				HandleNamespace: stringPtr("sandbox-abc12-ocp4-cluster"),
			},
			expected: `{"name":"ocpv05","handle_name":"abc12-98dab931","handle_namespace":"sandbox-abc12-ocp4-cluster"}`,
		},
		{
			name: "only name",
			input: Candidate{
				Name: "ocpv06",
			},
			expected: `{"name":"ocpv06"}`,
		},
		{
			name: "handle_name without handle_namespace",
			input: Candidate{
				Name:       "ocpv07",
				HandleName: stringPtr("def34-04fa8c6b"),
			},
			expected: `{"name":"ocpv07","handle_name":"def34-04fa8c6b"}`,
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
			{Name: "ocpv05", HandleName: stringPtr("abc12")},
			{Name: "ocpv06"},
		},
	}

	got, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("json.Marshal() error: %v", err)
	}

	expected := `{"candidates":[{"name":"ocpv05","handle_name":"abc12"},{"name":"ocpv06"}]}`
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
			input:            `{"name":"ocpv05","handle_name":"abc12","score":82.5,"eligible":true}`,
			expectedCluster:  "ocpv05",
			expectedScore:    82.5,
			expectedEligible: true,
			hasReason:        false,
		},
		{
			name:             "ineligible candidate with reason",
			input:            `{"name":"ocpv06","score":0,"eligible":false,"ineligibility_reason":"cluster in maintenance"}`,
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
			if sc.Name != tc.expectedCluster {
				t.Errorf("Name = %q, want %q", sc.Name, tc.expectedCluster)
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

func TestScoredCandidate_UnmarshalJSON_WithScores(t *testing.T) {
	input := `{"name":"ocpv05","score":82.5,"scores":{"cpu":90.0,"memory":75.0,"vm_density":80.0},"eligible":true}`
	var sc ScoredCandidate
	if err := json.Unmarshal([]byte(input), &sc); err != nil {
		t.Fatalf("json.Unmarshal() error: %v", err)
	}
	if len(sc.Scores) != 3 {
		t.Fatalf("len(Scores) = %d, want 3", len(sc.Scores))
	}
	expected := map[string]float64{"cpu": 90.0, "memory": 75.0, "vm_density": 80.0}
	for dim, want := range expected {
		if got, ok := sc.Scores[dim]; !ok {
			t.Errorf("Scores missing key %q", dim)
		} else if got != want {
			t.Errorf("Scores[%q] = %v, want %v", dim, got, want)
		}
	}
}

func TestEvaluateResponse_UnmarshalJSON(t *testing.T) {
	input := `{
		"ranked": [
			{"name": "ocpv06", "handle_name": "def34", "score": 82.5, "eligible": true},
			{"name": "ocpv05", "handle_name": "abc12", "score": 65.3, "eligible": true}
		],
		"excluded": [
			{"name": "ocpv07", "score": 0, "eligible": false, "ineligibility_reason": "disabled"}
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

	if resp.Ranked[0].Name != "ocpv06" {
		t.Errorf("Ranked[0].Name = %q, want %q", resp.Ranked[0].Name, "ocpv06")
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
				Name:            "ocpv05",
				HandleName:      stringPtr("abc12"),
				HandleNamespace: stringPtr("sandbox-abc12"),
				Score:           75.0,
				Scores:          map[string]float64{"cpu": 90.0, "memory": 60.0},
				Eligible:        true,
			},
		},
		Excluded: []ScoredCandidate{
			{
				Name:                "ocpv08",
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
	if decoded.Ranked[0].Name != original.Ranked[0].Name {
		t.Errorf("Ranked[0].Name = %q, want %q", decoded.Ranked[0].Name, original.Ranked[0].Name)
	}
	if decoded.Ranked[0].Score != original.Ranked[0].Score {
		t.Errorf("Ranked[0].Score = %v, want %v", decoded.Ranked[0].Score, original.Ranked[0].Score)
	}
	if len(decoded.Ranked[0].Scores) != 2 {
		t.Fatalf("Ranked[0].Scores length = %d, want 2", len(decoded.Ranked[0].Scores))
	}
	if decoded.Ranked[0].Scores["cpu"] != 90.0 {
		t.Errorf("Ranked[0].Scores[cpu] = %v, want 90.0", decoded.Ranked[0].Scores["cpu"])
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
