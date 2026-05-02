---
name: rhdp-poolboy-ecosystem
description: How poolboy-scoring fits in the RHDP ecosystem — Poolboy sort, CRD relationships, cluster-scheduler integration, and provisioning flow. Load when understanding cross-system behavior.
user-invocable: false
---

# RHDP Ecosystem — poolboy-scoring Integration

## Poolboy's 7-tier ResourceHandleMatch sort

When a ResourceClaim arrives, Poolboy selects the best available handle using this sort order (lower tier number = higher priority):

1. `resource_count_difference` — how many resources the handle has vs. what the claim needs
2. `resource_name_difference` — whether resource names match
3. `template_difference_count` — how closely the handle's template matches
4. `is_healthy` — healthy handles preferred
5. `is_ready` — ready handles preferred
6. **`preference_score`** — higher score wins. **This is what poolboy-scoring sets.**
7. `creation_timestamp` — oldest handle wins (FIFO tiebreaker)

Without poolboy-scoring, all handles default to `preferenceScore: 0`, so tier 6 has no effect and binding falls back to tier 7 (FIFO). poolboy-scoring adds capacity awareness by setting differentiated scores.

## CRD relationships

```
ResourcePool
  └── status.resourceHandles[]         → list of {name, healthy} entries
        └── ResourceHandle
              ├── spec.preferenceScore       ← patched by poolboy-scoring
              ├── spec.resourceClaim         → non-nil = bound (skip)
              ├── status.placements[]        ← cached by poolboy-scoring
              ├── status.summary.provision_data  → may contain cluster placement
              └── status.resources[].reference
                    └── AnarchySubject
                          └── spec.vars.job_vars
                                ├── sandbox_openshift_cluster    → "ocpv06"
                                ├── sandbox_openshift_name       → optional
                                └── sandbox_openshift_namespace  → optional
```

Key relationships:

- **ResourcePool → ResourceHandle**: pool's `status.resourceHandles[]` lists handle names. The controller iterates these to find unbound handles.
- **ResourceHandle → AnarchySubject**: handle's `status.resources[].reference` points to the AnarchySubject managing the workload. The controller follows this reference to discover which cluster the handle is placed on.
- **AnarchySubject → job_vars**: the `spec.vars.job_vars` map contains `sandbox_openshift_cluster` — the cluster name used as a candidate for scoring.

## How poolboy-scoring connects

1. **Watches** ResourcePool objects (via controller-runtime informer)
2. **Collects** unbound handles from `status.resourceHandles[]` — skips bound (has `spec.resourceClaim`) and unhealthy handles
3. **Resolves** cluster placement for each handle (3-tier lookup):
   - Cached `status.placements` (written by the controller on previous runs)
   - `provision_data` shortcut (some catalog items propagate placement here)
   - AnarchySubject GET (extract `sandbox_openshift_cluster` from `spec.vars.job_vars`)
4. **Calls** cluster-scheduler `POST /api/v1/evaluate` with ALL unique clusters as candidates (one call per pool)
5. **Patches** `spec.preferenceScore` on each handle with its cluster's score
6. **Poolboy** reads `preferenceScore` at bind time (tier 6 in the 7-tier sort)

## Cluster-scheduler /evaluate API

Request:

```json
{
  "candidates": [
    { "cluster_name": "ocpv06" },
    { "cluster_name": "ocpv05" },
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

- **Auth**: `X-API-Key` header
- **Scoring strategy**: `most_capacity` — relative scores, higher = more available capacity
- **Key insight**: A single candidate always returns the same score (e.g., 80). Multiple candidates produce differentiated scores (e.g., 73.69, 65.58, 34.44). This is why the controller batches all clusters per pool into one call — relative scoring requires comparison.
- **Excluded candidates**: clusters that can't accept workloads, with `ineligibility_reason` explaining why

## Graceful degradation

All failure modes degrade to current FIFO behavior — no failure breaks Poolboy:

- **Controller is down**: Last scores persist on handles. New handles get `preferenceScore: 0` (default). Recovery: restart, resync re-scores everything.
- **Cluster-scheduler is down**: Existing scores persist. Recovery: scheduler returns, next resync updates scores.
- **Stale scores (5+ minutes)**: Better than no scores. Recovery: resync catches up.
- **AnarchySubject not found**: Handle is skipped, reconciliation is requeued. Recovery: subject appears, next reconcile resolves placement.

## Tenant clusters

Some catalog items provision on dynamically-named tenant clusters (e.g., `cluster-nm66z`) instead of shared clusters (e.g., `ocpv06`). The controller handles these transparently — any cluster name found in `job_vars` is sent to the scheduler. If the scheduler doesn't recognize the cluster, it appears in the `excluded` list and the handle keeps its current score.

## Related RHDP systems

- **Babylon UI** → **CatalogItem** → **ResourceClaim** → **Poolboy** → **AnarchySubject** → **Anarchy Governor**: the provisioning pipeline from user request to deployed workload
- **Poolboy**: manages ResourcePools and ResourceHandles, selects handles for claims using the 7-tier sort
- **Anarchy**: manages AnarchySubjects (workload lifecycle) via AnarchyGovernors (state machine definitions)
- **Cluster-scheduler**: capacity scoring service that knows cluster utilization metrics
- **AgnosticD/AgnosticV**: deployer and catalog tools (not directly used by poolboy-scoring)
