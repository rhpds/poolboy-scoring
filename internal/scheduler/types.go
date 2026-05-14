package scheduler

import "time"

// Candidate represents a ResourceHandle candidate sent to the cluster-scheduler
// for scoring. Maps to the Python PoolCandidate schema.
//
// Name is required (the cluster name). HandleName and HandleNamespace are
// optional — using *string so nil omits them from JSON (vs "" which would
// send an empty string).
type Candidate struct {
	Name            string  `json:"name"`
	HandleName      *string `json:"handle_name,omitempty"`
	HandleNamespace *string `json:"handle_namespace,omitempty"`
}

// EvaluateRequest is the payload sent to POST /api/v1/evaluate/clusters.
type EvaluateRequest struct {
	Candidates []Candidate `json:"candidates"`
}

// ScoredCandidate is a single candidate returned by the cluster-scheduler
// with a capacity score. Maps to the Python EvaluatedCandidate schema.
//
// Score ranges from 0.0 to 100.0. Higher is better (more available capacity).
// Eligible indicates whether the cluster can accept workloads.
// IneligibilityReason explains why a cluster was excluded (nil when eligible).
// Scores contains per-dimension score breakdown (e.g. "cpu": 90.0).
type ScoredCandidate struct {
	Name                string             `json:"name"`
	HandleName          *string            `json:"handle_name,omitempty"`
	HandleNamespace     *string            `json:"handle_namespace,omitempty"`
	Score               float64            `json:"score"`
	Scores              map[string]float64 `json:"scores,omitempty"`
	Eligible            bool               `json:"eligible"`
	IneligibilityReason *string            `json:"ineligibility_reason,omitempty"`
}

// EvaluateResponse is the response from POST /api/v1/evaluate/clusters.
//
// Ranked contains eligible candidates sorted by score (highest first).
// Excluded contains ineligible candidates with reasons.
// Strategy is the scoring strategy used (e.g. "most_capacity").
// GeneratedAt is the timestamp when the scores were calculated.
type EvaluateResponse struct {
	Ranked      []ScoredCandidate `json:"ranked"`
	Excluded    []ScoredCandidate `json:"excluded"`
	Strategy    string            `json:"strategy"`
	GeneratedAt time.Time         `json:"generated_at"`
}
