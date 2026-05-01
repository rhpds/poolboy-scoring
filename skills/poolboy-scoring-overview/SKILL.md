---
name: poolboy-scoring-overview
description: Project architecture, repo structure, key patterns, configuration, and API contract. Load when working on any part of the codebase.
user-invocable: false
---

# poolboy-scoring — Project Overview

## What is this

Go controller that watches Poolboy ResourcePools, collects unbound ResourceHandles, resolves their cluster placement from AnarchySubjects, calls the cluster-scheduler's `/evaluate` endpoint, and patches `spec.preferenceScore` on each handle. Poolboy consumes the scores through its existing 7-tier sort with zero code changes. Standalone project — its own repository, Helm chart, container image, and release cycle.

## Repo structure

```
cmd/
  main.go                         Entry point: config, logging, Manager, controller wiring

internal/
  config/
    config.go                     Centralized Config struct (envconfig), all env vars
  controller/
    reconciler.go                 ResourcePoolReconciler: Reconcile(), SetupWithManager()
    metrics.go                    Prometheus counters/histograms (4 custom metrics)
    metrics_test.go
    reconciler_test.go
  metrics/
    auth.go                       HTTP Basic Auth FilterProvider for /metrics endpoint
    auth_test.go
  placement/
    types.go                      Placement, ResourceRef, GVK constants, partial CRD types
    resourcehandle.go             ParseHandleSpec/Status, PlacementFromProvisionData
    resourcepool.go               ParsePoolStatus
    anarchysubject.go             ParseAnarchySubjectSpec, ExtractPlacement
    lookup.go                     PlacementLookup (3-tier: cached → provision_data → GET)
    *_test.go
  scheduler/
    types.go                      Candidate, EvaluateRequest/Response, ScoredCandidate
    client.go                     Scorer interface + HTTP Client implementation
    *_test.go

helm/                             Full Helm chart (Deployment, RBAC, ConfigMap, Secrets, ServiceMonitor)
skills/                           AI agent knowledge base
.github/workflows/                CI (test + validate) and Release (multi-arch image)
```

## Architecture

```
┌──────────────────────────────────────────────────────────────┐
│                   Kubernetes API Server                      │
│  ResourcePool watch stream (persistent connection)           │
└────────────────────────┬─────────────────────────────────────┘
                         │ ADDED / MODIFIED / DELETED
                         ▼
┌──────────────────────────────────────────────────────────────┐
│       controller-runtime Manager + Informer Cache            │
│  Watches: ResourcePool (unstructured)                        │
│  Built-in: leader election, health probes, metrics           │
│  Resync period: 5 minutes (re-enqueue from cache)            │
└────────────────────────┬─────────────────────────────────────┘
                         ▼
┌──────────────────────────────────────────────────────────────┐
│             ResourcePoolReconciler.Reconcile(req)            │
│  1. Get pool → read status.resourceHandles[]                 │
│  2. For each handle: get, skip bound, resolve placement      │
│  3. Collect unique clusters → POST /evaluate (one call)      │
│  4. Map scores → PATCH spec.preferenceScore where changed    │
└──────────────────────────────────────────────────────────────┘
```

## Key patterns

### Pool-based reconciliation

The reconciler watches ResourcePool, not individual ResourceHandles. Each pool reconciliation collects all unbound handles, groups by cluster, and sends ONE batch `/evaluate` call. The cluster-scheduler's `most_capacity` strategy produces relative scores — multiple candidates are required for differentiated scores.

Entry point: `internal/controller/reconciler.go` — `ResourcePoolReconciler.Reconcile()`

### Placement resolution (3-tier)

`internal/placement/lookup.go` — `PlacementLookup.Lookup()` resolves which cluster a handle is placed on:

1. **Cached `status.placements`** — written by the reconciler on previous runs, zero API calls
2. **`provision_data` shortcut** — some catalog items propagate placement in `status.summary.provision_data`
3. **AnarchySubject GET** — fetch the referenced AnarchySubject and extract `sandbox_openshift_cluster` from `spec.vars.job_vars`

First match wins. Tier 3 results are cached as `status.placements` for subsequent reconciliations.

### Self-update detection

`internal/controller/reconciler.go` — `LastWrittenScores sync.Map`

When the controller patches `spec.preferenceScore`, the API server generates a MODIFIED event. The controller stores each patch in a `sync.Map` keyed by `namespace/name`. On the next event, it compares the score — if it matches what was written, the event is a self-update and is skipped. Lost on restart, but the resync catches up.

### Typed access via partial structs

`internal/placement/types.go` defines partial Go structs (e.g., `ResourcePoolStatus`, `ResourceHandleSpec`, `AnarchySubjectSpec`) with JSON tags. The `Parse*` functions convert `unstructured.Unstructured` objects into these typed structs via `runtime.DefaultUnstructuredConverter.FromUnstructured`. This provides type safety without code generation.

## Configuration

All env vars defined in `internal/config/config.go`:

| Env Var                     | Required | Default                       | Description                                  |
| --------------------------- | -------- | ----------------------------- | -------------------------------------------- |
| `CLUSTER_SCHEDULER_URL`     | Yes      | --                            | Cluster Scheduler service URL                |
| `CLUSTER_SCHEDULER_API_KEY` | Yes      | --                            | API key for `X-API-Key` header               |
| `METRICS_PASSWORD`          | Yes      | --                            | HTTP Basic Auth password for `/metrics`      |
| `CLUSTER_DOMAIN`            | No       | `babydev.dev.open.redhat.com` | Prometheus metric label for cluster identity |
| `RESYNC_INTERVAL`           | No       | `5m`                          | Informer resync period                       |
| `SCORE_TIMEOUT`             | No       | `5s`                          | HTTP timeout for `/evaluate`                 |
| `RETRY_INTERVAL`            | No       | `30s`                         | RequeueAfter delay on transient failures     |
| `LEADER_ELECTION`           | No       | `true`                        | Enable leader election                       |
| `DRY_RUN`                   | No       | `false`                       | Log score changes without patching           |
| `DEBUG`                     | No       | `false`                       | Enable V(1) debug logging                    |

## API contract — POST /api/v1/evaluate

Request:

```json
{
  "candidates": [
    { "cluster_name": "ocpv05" },
    { "cluster_name": "ocpv06" },
    { "cluster_name": "ocpv10" }
  ]
}
```

Response:

```json
{
  "ranked": [
    { "cluster_name": "ocpv06", "score": 73.69, "eligible": true },
    { "cluster_name": "ocpv05", "score": 65.58, "eligible": true },
    { "cluster_name": "ocpv10", "score": 34.44, "eligible": true }
  ],
  "excluded": [],
  "strategy": "most_capacity",
  "generated_at": "2026-04-26T14:30:00Z"
}
```

- Auth: `X-API-Key` header
- `handle_name` and `handle_namespace` are optional (nil omits from JSON)
- Multiple handles on the same cluster receive the same score
- Types defined in `internal/scheduler/types.go`

## RBAC

ClusterRole defined in `helm/templates/rbac.yaml`:

| Resource                 | Verbs                                    | Why                                    |
| ------------------------ | ---------------------------------------- | -------------------------------------- |
| `resourcepools`          | get, list, watch                         | Watch pools to trigger reconciliation  |
| `resourcehandles`        | get, list, watch, patch                  | Read handles, patch preferenceScore    |
| `resourcehandles/status` | patch                                    | Cache placements in status.placements  |
| `anarchysubjects`        | get                                      | Read job_vars for placement resolution |
| `leases`                 | create, delete, get, list, update, watch | Leader election                        |
| `events`                 | create, get, list, patch, watch          | controller-runtime event recording     |

## Failure modes

All failures degrade to current FIFO behavior — no failure mode breaks Poolboy.

| Scenario                  | Effect                                 | Recovery                               |
| ------------------------- | -------------------------------------- | -------------------------------------- |
| Controller is down        | Last scores persist, new handles get 0 | Restart; resync re-scores              |
| Cluster Scheduler is down | Existing scores persist                | Scheduler returns; next resync updates |
| Stale scores (5+ min)     | Better than no scores                  | Resync catches up                      |
| AnarchySubject not found  | Handle skipped, requeued               | Subject appears; next resync resolves  |
| 409 Conflict on patch     | Logged at Info, requeued               | Next reconcile succeeds                |
