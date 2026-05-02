# Development Guide

## Prerequisites

- **Go 1.26+**: check with `go version`
- **Helm 3**: for chart development and cluster deploy (`brew install helm`)
- **oc CLI**: for OpenShift cluster access and `oc start-build`
- **pre-commit**: for automated linting on every commit (`pip install pre-commit` or `brew install pre-commit`)
- **podman**: for local container builds (`brew install podman`)
- **Access to an OpenShift cluster** with the required CRDs:
  - Primary dev cluster: `babydev.dev.open.redhat.com`
  - Any OpenShift cluster with [Poolboy](https://github.com/redhat-cop/poolboy) and [Anarchy](https://github.com/redhat-cop/anarchy) installed
- **Cluster Scheduler** API access: the controller calls the [cluster-scheduler](https://github.com/rhpds/cluster-scheduler) service to get capacity scores

## Development Methods

Two methods are available:

| Method         | Use Case                                    | Feedback Time              | Runs In       |
| -------------- | ------------------------------------------- | -------------------------- | ------------- |
| 1. `make run`  | Quick code iteration, debugging             | ~2s (compile)              | Local machine |
| 2. BuildConfig | Test full deployment (RBAC, probes, limits) | ~1-2 min (build + rollout) | Cluster pod   |

---

## Method 1: Local Binary (`make run`)

Fastest feedback loop for code changes. The controller runs as a local process and connects to the cluster via your current KUBECONFIG context.

### Setup

1. Install pre-commit hooks:

```bash
pre-commit install
```

This installs git hooks that run `go fmt`, `go mod tidy`, `go vet`, and `helm lint` automatically on every `git commit`. If `go fmt` reformats a file, the commit will fail -- stage the reformatted files and commit again.

2. Set the required environment variables.

The controller uses [envconfig](https://github.com/kelseyhightower/envconfig) to load configuration from environment variables. Three variables are required (the rest have sensible defaults):

```bash
# Required: cluster-scheduler API endpoint
export CLUSTER_SCHEDULER_URL="http://cluster-scheduler.cluster-scheduler.svc.cluster.local:8080"

# Required: API key for the cluster-scheduler (X-API-Key header)
# Get this from the cluster-scheduler's Kubernetes Secret
export CLUSTER_SCHEDULER_API_KEY="your-api-key"

# Required: password for the /metrics endpoint (HTTP Basic Auth)
export METRICS_PASSWORD="any-password-for-dev"
```

For local development, you'll also want to disable leader election and enable debug logging:

```bash
# Disable leader election (avoids needing a Lease object in the cluster)
export LEADER_ELECTION=false

# Enable debug logging (human-readable output instead of JSON)
export DEBUG=true
```

Optional: use dry-run mode to see what the controller would do without actually patching any ResourceHandles:

```bash
# Log score changes without applying them
export DRY_RUN=true
```

3. Run the controller:

```bash
make run
```

The controller connects to the cluster via your current KUBECONFIG context, watches all ResourcePool objects, resolves placements via AnarchySubjects, and calls the cluster-scheduler's `/evaluate` endpoint. Press `Ctrl+C` to stop.

### All Environment Variables

The full set of environment variables is defined in `internal/config/config.go`. Here's the complete reference:

| Env Var                     | Required | Default                       | Description                                               |
| --------------------------- | -------- | ----------------------------- | --------------------------------------------------------- |
| `CLUSTER_SCHEDULER_URL`     | Yes      | --                            | Cluster Scheduler service URL                             |
| `CLUSTER_SCHEDULER_API_KEY` | Yes      | --                            | API key for `X-API-Key` header                            |
| `METRICS_PASSWORD`          | Yes      | --                            | HTTP Basic Auth password for `/metrics`                   |
| `CLUSTER_DOMAIN`            | No       | `babydev.dev.open.redhat.com` | Babylon cluster FQDN, used as label on Prometheus metrics |
| `RESYNC_INTERVAL`           | No       | `5m`                          | How often the informer re-lists all ResourcePools         |
| `SCORE_TIMEOUT`             | No       | `5s`                          | HTTP timeout for the `/evaluate` API call                 |
| `RETRY_INTERVAL`            | No       | `30s`                         | Delay before retrying a failed reconciliation             |
| `LEADER_ELECTION`           | No       | `true`                        | Enable leader election (use `false` for local dev)        |
| `LEADER_ELECTION_ID`        | No       | `poolboy-scoring`             | Name of the Lease used for leader election                |
| `HEALTH_PROBE_BIND_ADDRESS` | No       | `:8081`                       | Health/readiness probe endpoint                           |
| `METRICS_BIND_ADDRESS`      | No       | `:9090`                       | Prometheus metrics endpoint                               |
| `METRICS_USERNAME`          | No       | `metrics`                     | HTTP Basic Auth username for `/metrics`                   |
| `DRY_RUN`                   | No       | `false`                       | Log score changes without patching ResourceHandles        |
| `DEBUG`                     | No       | `false`                       | Enable debug-level logging (V(1) messages)                |

### When to use

- Iterating on reconciler logic
- Debugging with a local debugger (delve)
- Running unit tests
- Validating behavior against a live cluster

### Limitations

- Runs outside the cluster (no real RBAC enforcement, probes, or resource limits)
- Requires `LEADER_ELECTION=false` (no Lease permission from local context)
- Requires network access to the cluster-scheduler service (may need VPN)

---

## Method 2: BuildConfig (Image on Cluster)

Build the image inside the OpenShift cluster and deploy via Helm. Tests the full deployment pipeline including RBAC, probes, and resource limits.

### Setup

1. Create a namespace and the BuildConfig:

```bash
oc new-project poolboy-scoring-dev
oc process --local -f build-template.yaml | oc apply -f -
```

The `build-template.yaml` creates an ImageStream and a BuildConfig that uses the `Containerfile` for building. The Containerfile is a multi-stage build: it compiles a static Go binary in a `golang:1.26-alpine` stage, then copies it to a `scratch` base image (no OS, ~20 MB total).

2. Start a build (uploads local source to the cluster and builds the image):

```bash
oc start-build poolboy-scoring --from-dir=. --follow
```

3. Create your dev Helm values:

```bash
cp helm-vars-dev.yaml.example helm-vars-dev.yaml
```

Edit `helm-vars-dev.yaml` and set:

- `namespace.name`: your dev namespace (e.g., `poolboy-scoring-dev`)
- `clusterScheduler.url`: the cluster-scheduler's service URL
- Any other overrides (see `helm-vars-dev.yaml.example` for documented options)

4. Deploy with Helm using the cluster-built image:

```bash
helm template poolboy-scoring helm/ \
  -f helm-vars-dev.yaml \
  --set=image.tagOverride=- \
  --set=image.repository=$(oc get is poolboy-scoring -o jsonpath='{.status.dockerImageRepository}') \
  | oc apply -f -
```

The `--set=image.tagOverride=-` tells the chart to use the ImageStream's `latest` tag. The `--set=image.repository=...` points to the cluster's internal registry.

5. Verify:

```bash
oc get pods -l app.kubernetes.io/name=poolboy-scoring
oc logs -f deployment/poolboy-scoring
```

### Rebuilding after code changes

Each time you change code, rebuild and restart the pod:

```bash
oc start-build poolboy-scoring --from-dir=. --follow
oc rollout restart deployment/poolboy-scoring
```

The `rollout restart` is required because Kubernetes only pulls the image when a new pod is created -- the running pod will not detect that the ImageStream tag points to a new digest.

### Cleanup

```bash
# Delete Helm resources
helm template poolboy-scoring helm/ \
  -f helm-vars-dev.yaml \
  --set=image.tagOverride=- \
  --set=image.repository=$(oc get is poolboy-scoring -o jsonpath='{.status.dockerImageRepository}') \
  | oc delete -f -

# Delete BuildConfig and ImageStream
oc process --local -f build-template.yaml | oc delete -f -
```

### When to use

- Testing the full deployment pipeline (RBAC, probes, resource limits)
- Validating Helm chart changes
- Testing leader election behavior with multiple replicas
- When you don't have podman/Docker installed locally

### Limitations

- Each code change requires `oc start-build` + wait (~1-2 min) + rollout restart
- No auto-rebuild on file changes

---

## Running Tests

```bash
make test                                           # all tests (verbose, no cache)
make cover                                          # with coverage report
go test ./internal/placement/ -v                    # single package
go test ./internal/controller/ -run TestReconcile -v  # single test
```

The project has 144 tests across all packages with 93% coverage. Tests use:

- **Table-driven tests**: `[]struct{name, input, expected}` pattern throughout
- **Fake Kubernetes client**: `fake.NewClientBuilder()` from controller-runtime for testing reconciler and placement code without a real cluster
- **Mock scorer**: implements the `scheduler.Scorer` interface for testing reconciler logic without calling the real cluster-scheduler
- **httptest.NewServer**: for testing the HTTP client against a local test server
- **Interceptor functions**: for simulating specific API server errors (e.g., 409 Conflict)

## Linting and Formatting

```bash
make lint                          # go vet (static analysis)
pre-commit run --all-files         # all hooks: go fmt, go mod tidy, go vet, helm lint
```

Pre-commit hooks run automatically on `git commit`. If a hook fails (e.g., `go fmt` reformats a file), the commit is rejected. Stage the changes and commit again.

The four pre-commit hooks (defined in `.pre-commit-config.yaml`):

| Hook          | What it does                                 |
| ------------- | -------------------------------------------- |
| `go-fmt`      | Formats all `.go` files with `gofmt`         |
| `go-mod-tidy` | Cleans `go.mod` and `go.sum`                 |
| `go-vet`      | Runs static analysis (`go vet ./...`)        |
| `helm-lint`   | Validates the Helm chart (`helm lint helm/`) |

## Building

```bash
make build              # produces ./poolboy-scoring binary
make container-build    # builds container image with podman
```

The `container-build` target uses `podman build` with the `Containerfile`. The resulting image is based on `scratch` (no OS) and is approximately 20 MB.

To customize the image name and tag:

```bash
make container-build IMAGE=quay.io/rhpds/poolboy-scoring TAG=v0.2.0
```

## Version Bumping and Release

1. Ensure you're on the main branch and up to date:

```bash
git checkout main && git pull
```

2. Run the version bump script:

```bash
bash bump-version.sh           # auto-increments patch version (e.g., v0.1.0 → v0.1.1)
bash bump-version.sh v0.2.0    # explicit version
```

The script:

- Validates the version is semantic (`vMAJOR.MINOR.PATCH`)
- Checks that you're on the `main` branch (use `--dev` or `--force` to override)
- Updates `version` and `appVersion` in `helm/Chart.yaml`
- Commits (signed with `-S`), creates a git tag, and pushes both

3. GitHub Actions automatically builds a multi-arch container image (amd64 + arm64) and pushes it to `quay.io/rhpds/poolboy-scoring` with tags: `latest`, `vMAJOR.MINOR`, `vMAJOR.MINOR.PATCH`.

The release workflow (`.github/workflows/release.yml`) validates that `helm/Chart.yaml`'s `appVersion` matches the git tag before building.

## Troubleshooting

| Problem                                                 | Cause                                     | Fix                                                                                           |
| ------------------------------------------------------- | ----------------------------------------- | --------------------------------------------------------------------------------------------- |
| `Failed to load config: required key ... missing value` | Missing required environment variable     | Set `CLUSTER_SCHEDULER_URL`, `CLUSTER_SCHEDULER_API_KEY`, and `METRICS_PASSWORD`              |
| `cluster-scheduler returned 401 Unauthorized`           | Wrong or expired API key                  | Check `CLUSTER_SCHEDULER_API_KEY` value against the cluster-scheduler's Secret                |
| `cluster-scheduler returned 5xx`                        | Cluster Scheduler service is down         | Controller retries on next resync cycle; check cluster-scheduler pod logs                     |
| `context deadline exceeded` on /evaluate                | Cluster Scheduler too slow or unreachable | Increase `SCORE_TIMEOUT` (default 5s) or check network connectivity                           |
| `failed to acquire lease`                               | Another instance holds the leader lock    | Use `LEADER_ELECTION=false` for local dev, or stop the other instance                         |
| RBAC errors (cannot list/watch/patch)                   | ClusterRole not applied or missing verbs  | Apply the Helm chart with `deploy=true` (includes RBAC), or check `helm/templates/rbac.yaml`  |
| Score patches not appearing on handles                  | DRY_RUN is enabled                        | Set `DRY_RUN=false` (default) to apply patches                                                |
| All handles get the same score                          | Pool has handles on only one cluster      | Expected behavior -- the scheduler produces differentiated scores only with multiple clusters |
| Pre-commit hook fails on `helm lint`                    | Helm not installed                        | `brew install helm`                                                                           |
