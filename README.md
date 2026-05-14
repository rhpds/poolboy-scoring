# poolboy-scoring

Standalone Go controller that watches [Poolboy](https://github.com/redhat-cop/poolboy) ResourcePools, collects unbound ResourceHandles, calls the [cluster-scheduler](https://github.com/rhpds/cluster-scheduler)'s `/evaluate` endpoint, and patches `spec.preferenceScore` on each handle. Poolboy consumes the scores through its existing 7-tier sort with zero code changes.

## How it Works

### The Problem

Poolboy manages pre-provisioned ResourceHandles across multiple OpenShift clusters. When a ResourceClaim arrives, Poolboy selects the best handle using a 7-tier sort. Tier 6 uses `preferenceScore` to rank handles — but all handles in a multi-cluster pool default to 0, so binding falls back to FIFO ordering with no capacity awareness.

### The Solution

poolboy-scoring watches ResourcePool objects, collects all unbound handles, resolves which cluster each handle is placed on (via AnarchySubject introspection), sends one batch request to the cluster-scheduler with all clusters, and patches each handle with its cluster's capacity score. Poolboy then selects handles from less-loaded clusters at bind time.

### Why Batch Per-Pool

The cluster-scheduler's `most_capacity` strategy produces **relative** scores — meaningful only when comparing multiple clusters simultaneously. A single candidate always returns the same score (e.g. 80). Three candidates produce differentiated scores (e.g. 73.69, 65.58, 34.44). This is why the controller batches all clusters in a pool into one `/evaluate` call.

### Architecture

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
│                                                              │
│  1. Get pool from cache                                      │
│  2. Read status.resourceHandles[] → list of handle names     │
│  3. For each handle:                                         │
│     a. Get handle from cache                                 │
│     b. Skip if bound (has spec.resourceClaim)                │
│     c. Resolve placement:                                    │
│        - Cached status.placements → use it                   │
│        - provision_data shortcut → extract cluster           │
│        - GET AnarchySubject → extract from job_vars          │
│  4. Collect unique clusters across all unbound handles       │
│  5. POST /api/v1/evaluate/clusters with ALL clusters         │
│  6. For each handle: look up its cluster's score             │
│  7. Compare with current spec.preferenceScore                │
│  8. If changed: PATCH spec.preferenceScore                   │
└──────────────────────────────────────────────────────────────┘
```

**Example:** A pool with 5 handles across 3 clusters:

```
handle-A → ocpv06 ─┐
handle-B → ocpv06 ─┤ One /evaluate call: [ocpv06, ocpv05, ocpv10]
handle-C → ocpv05 ─┤ Returns: 73.69, 65.58, 34.44
handle-D → ocpv10 ─┤
handle-E → ocpv10 ─┘
                      handle-A,B → preferenceScore: 73.69
                      handle-C   → preferenceScore: 65.58
                      handle-D,E → preferenceScore: 34.44
```

Poolboy's sort picks handle-A or handle-B (highest score) → claims go to the cluster with the most available capacity.

### Graceful Degradation

All failure modes degrade to current FIFO behavior:

| Scenario                  | Effect                                 | Recovery                               |
| ------------------------- | -------------------------------------- | -------------------------------------- |
| Controller is down        | Last scores persist, new handles get 0 | Restart; resync re-scores              |
| Cluster Scheduler is down | Existing scores persist                | Scheduler returns; next resync updates |
| Stale scores (5+ min)     | Better than no scores                  | Resync catches up                      |
| AnarchySubject not found  | Handle skipped, requeued               | Subject appears; next resync resolves  |

## Quick Start

### Prerequisites

- Go 1.26+, Helm 3, oc CLI, pre-commit
- Access to an OpenShift cluster with Poolboy and Anarchy CRDs
- See [Development.md](Development.md) for detailed setup

### Build and Run Locally

```bash
# Build
make build

# Set required env vars
export CLUSTER_SCHEDULER_URL="http://cluster-scheduler.cluster-scheduler.svc.cluster.local:8080"
export CLUSTER_SCHEDULER_API_KEY="your-api-key"
export METRICS_PASSWORD="any-password"
export LEADER_ELECTION=false
export DEBUG=true

# Run (connects via KUBECONFIG)
make run
```

### Deploy to Cluster

```bash
# Create BuildConfig
oc new-project poolboy-scoring-dev
oc process --local -f build-template.yaml | oc apply -f -

# Build image on cluster
oc start-build poolboy-scoring --from-dir=. --follow

# Deploy with Helm
cp helm-vars-dev.yaml.example helm-vars-dev.yaml
# Edit helm-vars-dev.yaml with your settings
helm template poolboy-scoring helm/ \
  -f helm-vars-dev.yaml \
  --set=image.tagOverride=- \
  --set=image.repository=$(oc get is poolboy-scoring -o jsonpath='{.status.dockerImageRepository}') \
  | oc apply -f -
```

See [Development.md](Development.md) for the full guide including testing, linting, version bumping, and troubleshooting.

## Configuration

All configuration is via environment variables, loaded by [envconfig](https://github.com/kelseyhightower/envconfig) in `internal/config/config.go`:

| Env Var                     | Required | Default                       | Description                                               |
| --------------------------- | -------- | ----------------------------- | --------------------------------------------------------- |
| `CLUSTER_SCHEDULER_URL`     | Yes      | --                            | Cluster Scheduler service URL                             |
| `CLUSTER_SCHEDULER_API_KEY` | Yes      | --                            | API key for `X-API-Key` header                            |
| `METRICS_PASSWORD`          | Yes      | --                            | HTTP Basic Auth password for `/metrics`                   |
| `CLUSTER_DOMAIN`            | No       | `babydev.dev.open.redhat.com` | Babylon cluster FQDN, used as label on Prometheus metrics |
| `RESYNC_INTERVAL`           | No       | `5m`                          | How often the informer re-lists all ResourcePools         |
| `SCORE_TIMEOUT`             | No       | `5s`                          | HTTP timeout for the `/evaluate/clusters` API call        |
| `RETRY_INTERVAL`            | No       | `30s`                         | Delay before retrying a failed reconciliation             |
| `LEADER_ELECTION`           | No       | `true`                        | Enable leader election (use `false` for local dev)        |
| `LEADER_ELECTION_ID`        | No       | `poolboy-scoring`             | Name of the Lease used for leader election                |
| `HEALTH_PROBE_BIND_ADDRESS` | No       | `:8081`                       | Health/readiness probe endpoint                           |
| `METRICS_BIND_ADDRESS`      | No       | `:9090`                       | Prometheus metrics endpoint                               |
| `METRICS_USERNAME`          | No       | `metrics`                     | HTTP Basic Auth username for `/metrics`                   |
| `DRY_RUN`                   | No       | `false`                       | Log score changes without patching ResourceHandles        |
| `DEBUG`                     | No       | `false`                       | Enable debug-level logging (V(1) messages)                |

## Metrics

Prometheus metrics are exposed on `:9090` with HTTP Basic Auth. Custom metrics:

| Metric                                       | Type      | Labels                    | Description                                        |
| -------------------------------------------- | --------- | ------------------------- | -------------------------------------------------- |
| `poolboy_scoring_reconcile_total`            | Counter   | `cluster_domain, result`  | Reconciliations by outcome (success/error/skipped) |
| `poolboy_scoring_score_patches_total`        | Counter   | `cluster_domain, cluster` | Score patches applied per cluster (e.g. ocpv06)    |
| `poolboy_scoring_scheduler_duration_seconds` | Histogram | `cluster_domain`          | Time spent calling `/evaluate`                     |
| `poolboy_scoring_handles_scored`             | Gauge     | `cluster_domain`          | Handles scored in the last reconciliation          |

Plus controller-runtime built-in metrics (`controller_runtime_reconcile_total`, work queue depth, Go runtime stats).

## Project Structure

```
poolboy-scoring/
├── cmd/
│   └── main.go                    # Entry point (Manager, leader election, wiring)
├── internal/
│   ├── config/                    # Centralized Config struct (envconfig)
│   ├── controller/                # ResourcePoolReconciler, Prometheus metrics
│   ├── metrics/                   # HTTP Basic Auth for /metrics endpoint
│   ├── placement/                 # Placement resolution (types, handle/subject/pool parsing, lookup)
│   └── scheduler/                 # Cluster Scheduler HTTP client (Scorer interface)
├── helm/                          # Helm chart (Deployment, RBAC, ConfigMap, Secrets, ServiceMonitor)
├── skills/                        # AI agent knowledge base (project overview, code style, dev workflow, ecosystem)
├── .github/workflows/             # CI (test + validate) and Release (multi-arch image)
├── Containerfile                  # Multi-stage build (golang:1.26-alpine → scratch)
├── Makefile                       # build, run, test, cover, lint, clean, container-build
├── build-template.yaml            # OpenShift ImageStream + BuildConfig
├── bump-version.sh                # Version bump helper (Chart.yaml, tag, push)
├── .pre-commit-config.yaml        # go fmt, go mod tidy, go vet, helm lint
├── AGENTS.md                      # AI agent conventions and skills index
├── Development.md                 # Full developer guide
└── README.md                      # This file
```

## Links

- [Development Guide](Development.md) — Build, test, debug, deploy, troubleshooting
- [Cluster Scheduler](https://github.com/rhpds/cluster-scheduler) — Capacity scoring service
- [Poolboy](https://github.com/redhat-cop/poolboy) — Resource pool manager
