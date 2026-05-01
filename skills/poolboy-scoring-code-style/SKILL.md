---
name: poolboy-scoring-code-style
description: Go conventions, controller-runtime patterns, error handling, logging, and testing patterns used in this codebase. Load when writing or reviewing code.
user-invocable: false
---

# poolboy-scoring — Code Style & Patterns

## Go conventions

### Project layout

Standard Go layout: `cmd/main.go` entry point, `internal/` for all packages. No `pkg/` — nothing is exported outside this module.

### Configuration via envconfig

`internal/config/config.go` uses `github.com/kelseyhightower/envconfig` struct tags:

```go
type Config struct {
    ClusterSchedulerURL string `envconfig:"CLUSTER_SCHEDULER_URL" required:"true"`
    ClusterDomain       string `envconfig:"CLUSTER_DOMAIN" default:"babydev.dev.open.redhat.com"`
    LeaderElection      bool   `envconfig:"LEADER_ELECTION" default:"true"`
}
```

Tags: `envconfig` maps to env var name, `required` fails startup if missing, `default` provides fallback. Load with `envconfig.Process("", &cfg)` — empty prefix means env vars map directly (no `POOLBOY_` prefix).

Duration fields are stored as `string` with helper methods (`ScoreTimeoutDuration()`, `ResyncIntervalDuration()`, `RetryIntervalDuration()`) that parse via `time.ParseDuration` and fall back to a hardcoded default on parse error.

### JSON tags on types

All types in `internal/placement/types.go` and `internal/scheduler/types.go` use JSON tags matching the Kubernetes/Python API field names (snake_case for scheduler types, camelCase for Kubernetes types):

```go
// Kubernetes CRD fields — camelCase
type Placement struct {
    ClusterName string `json:"clusterName"`
}

// Cluster-scheduler API fields — snake_case
type Candidate struct {
    ClusterName string  `json:"cluster_name"`
    HandleName  *string `json:"handle_name,omitempty"`
}
```

Use `*string` for optional fields that should be omitted from JSON when nil (not sent as `""`). Use `*bool` to distinguish "field not set" from `false` (e.g., `PoolHandleEntry.Healthy`).

## controller-runtime patterns

### Unstructured access (no codegen)

The controller works with Poolboy and Anarchy CRDs via `unstructured.Unstructured`. No code generation (`controller-gen`, `client-gen`) — the CRDs are owned by other projects.

Set GVK before any client operation:

```go
var pool unstructured.Unstructured
pool.SetGroupVersionKind(placement.ResourcePoolGVK)
r.Get(ctx, req.NamespacedName, &pool)
```

### Partial typed structs + FromUnstructured

`internal/placement/types.go` defines partial Go structs covering only the fields this controller reads. The `Parse*` functions in `resourcehandle.go`, `resourcepool.go`, `anarchysubject.go` convert unstructured data to typed structs:

```go
func ParseHandleSpec(obj *unstructured.Unstructured) (*ResourceHandleSpec, error) {
    raw, found, _ := unstructured.NestedMap(obj.Object, "spec")
    if !found {
        return &ResourceHandleSpec{}, nil
    }
    var spec ResourceHandleSpec
    if err := runtime.DefaultUnstructuredConverter.FromUnstructured(raw, &spec); err != nil {
        return nil, err
    }
    return &spec, nil
}
```

Convention for `Parse*` return values:

- `ParseHandleSpec` returns `(*Type, nil)` with zero-value struct when field is missing — spec always exists conceptually
- `ParseHandleStatus` returns `(nil, nil)` when field is missing — status may genuinely not exist yet

### GVK variables

`internal/placement/types.go` defines GVK constants as `var` (not `const`) because `schema.GroupVersionKind` is a struct and Go only allows `const` for primitive types:

```go
var ResourcePoolGVK = schema.GroupVersionKind{
    Group:   "poolboy.gpte.redhat.com",
    Version: "v1",
    Kind:    "ResourcePool",
}
```

### Manager setup

`cmd/main.go` — single controller, empty scheme (unstructured doesn't need registered types):

```go
scheme := runtime.NewScheme()
mgr, err := ctrl.NewManager(restCfg, ctrl.Options{
    Scheme:                 scheme,
    HealthProbeBindAddress: cfg.HealthProbeBindAddress,
    LeaderElection:         cfg.LeaderElection,
    LeaderElectionID:       cfg.LeaderElectionID,
    Metrics: metricsserver.Options{
        BindAddress:    cfg.MetricsBindAddress,
        FilterProvider: appmetrics.BasicAuthFilterProvider(...),
    },
})
```

The `run()` function is separated from `main()` to allow testability — `main()` only loads config and sets up logging.

### Scorer interface for testability

`internal/scheduler/client.go` defines:

```go
type Scorer interface {
    Evaluate(ctx context.Context, candidates []Candidate) (*EvaluateResponse, error)
}
```

The reconciler depends on `Scorer` (interface), not `*Client` (concrete). Tests substitute a `mockScorer` without an HTTP server. The `PlacementResolver` interface follows the same pattern for `PlacementLookup`.

## Error handling

### 409 Conflict

Info-level log + `Requeue: true`. Not an error — expected in a concurrent environment:

```go
if apierrors.IsConflict(err) {
    log.Info("Conflict patching score, will retry",
        "pool", req.Name, "handle", hwc.handle.GetName())
    return ctrl.Result{Requeue: true}, nil
}
```

### Scheduler errors

Info-level log + `RequeueAfter`. Existing scores persist — stale scores are better than no scores:

```go
log.Info("Scheduler evaluation failed, keeping existing scores",
    "pool", req.Name, "namespace", req.Namespace, "error", err.Error())
return ctrl.Result{RequeueAfter: r.Config.RetryIntervalDuration()}, nil
```

### NotFound

Skip silently — handle or subject was deleted between pool status read and handle GET:

```go
if client.IgnoreNotFound(err) == nil {
    log.V(1).Info("Handle not found, skipping", ...)
    continue
}
```

### Placement resolution failure

Info-level log + continue processing other handles. Count failures and `RequeueAfter` if any failed:

```go
log.Info("Failed to resolve placement, will retry", ...)
placementFailed++
continue
```

### General principle

Never return `err` for expected conditions. All failures degrade gracefully to FIFO behavior (handles keep their last score or default to 0). Only return errors for truly unexpected conditions (API server unreachable, malformed data).

## Logging

Uses `logr/zap` via controller-runtime. Two levels:

**Info** (always visible): State changes and important events

- Score updates: "Updated preference score" with old/new values
- Pool reconciliation summary: "Pool reconciliation complete" with handle counts
- Transient failures: "Scheduler evaluation failed", "Conflict patching score"

**V(1)/Debug** (enabled with `DEBUG=true`): Operational details

- Skip reasons: "No available handles, skipping", "Handle not found, skipping"
- API calls: "Calling /evaluate" with cluster list, "Evaluate response" with full JSON
- No-ops: events where nothing changed

**Never Error** for:

- 409 Conflict (expected concurrency)
- Scheduler timeouts (transient)
- AnarchySubject not found (handle may be in transition)

Log key-value pairs use consistent naming: `pool`, `handle`, `cluster`, `namespace`, `error`, `oldPreferenceScore`, `newPreferenceScore`.

## Testing patterns

### Table-driven tests

All packages use the `[]struct{name, ...}` pattern. Test names describe the scenario, not the expected result:

```go
tests := []struct {
    name     string
    input    *unstructured.Unstructured
    expected *ResourceHandleSpec
    wantErr  bool
}{
    {name: "valid spec with score", ...},
    {name: "missing spec section", ...},
}
```

### Fake Kubernetes client

`sigs.k8s.io/controller-runtime/pkg/client/fake` for controller and placement tests:

```go
c := fake.NewClientBuilder().
    WithObjects(pool, handle).
    WithStatusSubresource(statusObj).  // needed for status patches
    Build()
```

### Mock scorer

Implements the `Scorer` interface. Records whether it was called and what candidates it received:

```go
type mockScorer struct {
    response *scheduler.EvaluateResponse
    err      error
    called   bool
    received []scheduler.Candidate
}
```

### Interceptor functions

`sigs.k8s.io/controller-runtime/pkg/client/interceptor` for simulating specific API server errors:

```go
c := fake.NewClientBuilder().
    WithObjects(pool, handle).
    WithInterceptorFuncs(interceptor.Funcs{
        Patch: func(ctx context.Context, client client.WithWatch, obj client.Object, ...) error {
            if obj.GetObjectKind().GroupVersionKind() == placement.ResourceHandleGVK {
                return apierrors.NewConflict(...)
            }
            return client.Patch(ctx, obj, patch, opts...)
        },
    }).
    Build()
```

### httptest.NewServer

`internal/scheduler/client_test.go` uses `httptest.NewServer` to test the HTTP client against a local server — validates request format, headers, and response parsing.

### Test helper conventions

- `newTestPool(name, available, handles)` — creates an unstructured pool
- `newTestHandle(name, ...opts)` — functional options: `withBound()`, `withScore(80)`, `withCachedPlacements("ocpv06")`
- `defaultResponse(clusters...)` — creates an `EvaluateResponse` with decreasing scores (80, 70, 60...)
- `testConfig()` — minimal config for tests (just `RetryInterval`)

## Metrics conventions

`internal/controller/metrics.go` — package-level `var` with `init()` registration:

```go
var ReconcileTotal = prometheus.NewCounterVec(...)

func init() {
    metrics.Registry.MustRegister(ReconcileTotal, ...)
}
```

All custom metrics use the `poolboy_scoring_` prefix. Labels always include `cluster_domain` for multi-cluster identification.

`internal/metrics/auth.go` — `BasicAuthFilterProvider` returns a triple-nested function matching the controller-runtime `metricsserver.Options.FilterProvider` signature. Uses `crypto/subtle.ConstantTimeCompare` for timing-safe credential comparison.
