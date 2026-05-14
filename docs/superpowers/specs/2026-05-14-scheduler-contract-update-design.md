# Scheduler Contract Update

Update the cluster-scheduler API contract in poolboy-scoring to match the metrics standardization changes in cluster-scheduler.

## Context

The cluster-scheduler underwent a major API standardization (see `cluster-scheduler/docs/superpowers/specs/2026-05-14-metrics-standardization-design.md`). Two changes affect poolboy-scoring's contract:

1. The CNV evaluate endpoint moved from `/api/v1/evaluate` to `/api/v1/evaluate/clusters`
2. The `cluster_name` field was renamed to `name` across all request/response models

Poolboy-scoring only consumes one endpoint (`POST /evaluate`) and uses three types: `Candidate` (request), `ScoredCandidate` (response item), and `EvaluateResponse` (response wrapper). The changes are mechanical renames with no logic impact.

## Non-Goals

- Consuming the new `scores` per-dimension breakdown (we add the field to the struct for schema completeness but don't use it)
- Changing reconciler logic, scoring behavior, or placement resolution
- Updating any other endpoint (poolboy-scoring only calls evaluate)

## Changes

### Endpoint URL

| Before | After |
| --- | --- |
| `POST /api/v1/evaluate` | `POST /api/v1/evaluate/clusters` |

In `internal/scheduler/client.go`, the URL concatenation changes from `c.baseURL+"/api/v1/evaluate"` to `c.baseURL+"/api/v1/evaluate/clusters"`.

### Type Renames

**`internal/scheduler/types.go`**

`Candidate` struct:
- `ClusterName string \`json:"cluster_name"\`` -> `Name string \`json:"name"\``

`ScoredCandidate` struct:
- `ClusterName string \`json:"cluster_name"\`` -> `Name string \`json:"name"\``
- Add: `Scores map[string]float64 \`json:"scores,omitempty"\``

`EvaluateResponse` and `EvaluateRequest` structs remain unchanged.

### Downstream Field Access

**`internal/controller/reconciler.go`**

Two helper functions access the renamed field:
- `buildScoreMap`: `sc.ClusterName` -> `sc.Name`
- `buildExcludedSet`: `sc.ClusterName` -> `sc.Name`

No other reconciler logic changes.

### Test Updates

**`internal/scheduler/types_test.go`**
- Go field references: `ClusterName` -> `Name`
- JSON strings in test cases: `"cluster_name"` -> `"name"`

**`internal/scheduler/client_test.go`**
- Go field references in mock responses: `ClusterName` -> `Name`

**`internal/controller/reconciler_test.go`**
- `scheduler.Candidate{ClusterName: ...}` -> `scheduler.Candidate{Name: ...}`
- `scheduler.ScoredCandidate{ClusterName: ...}` -> `scheduler.ScoredCandidate{Name: ...}`
- `scorer.received[0].ClusterName` -> `scorer.received[0].Name`

### What Does NOT Change

- `Scorer` interface signature
- `EvaluateRequest` / `EvaluateResponse` structure
- `Placement` type (internal to poolboy, not part of the scheduler contract)
- Reconciler logic (only field access names change)
- `handle_name`, `handle_namespace` fields (unchanged in the scheduler spec)
- Authentication (`X-API-Key` header)

## Files Modified

| File | Change |
| --- | --- |
| `internal/scheduler/types.go` | Rename fields, add `Scores` |
| `internal/scheduler/client.go` | Update endpoint URL |
| `internal/scheduler/types_test.go` | Update field names and JSON strings |
| `internal/scheduler/client_test.go` | Update field names in mocks |
| `internal/controller/reconciler.go` | Update field access in helpers |
| `internal/controller/reconciler_test.go` | Update field names in test data |
