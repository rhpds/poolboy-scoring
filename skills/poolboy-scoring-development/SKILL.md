---
name: poolboy-scoring-development
description: Build, test, debug, and deploy procedures. Load when making code changes, running tests, or deploying.
user-invocable: false
---

# poolboy-scoring — Development Workflow

## Build

```bash
make build              # → ./poolboy-scoring binary
make container-build    # → podman build with Containerfile (multi-stage: golang:1.26-alpine → scratch, ~20 MB)
make container-build IMAGE=quay.io/rhpds/poolboy-scoring TAG=v0.2.0  # custom image/tag
```

## Test

```bash
make test                                              # all tests, verbose, no cache
make cover                                             # with coverage report (coverage.out)
go test ./internal/placement/ -v                       # single package
go test ./internal/controller/ -run TestReconcile -v   # single test
```

144 tests, 93% coverage. All tests are unit tests — no cluster required.

## Lint

```bash
make lint                          # go vet (static analysis)
pre-commit run --all-files         # all hooks: go fmt, go mod tidy, go vet, helm lint
pre-commit install                 # install hooks (one-time setup)
```

Pre-commit hooks run on every `git commit`. If `go fmt` reformats a file, the commit is rejected — stage the changes and commit again.

## Run locally

Required env vars:

```bash
export CLUSTER_SCHEDULER_URL="http://cluster-scheduler.cluster-scheduler.svc.cluster.local:8080"
export CLUSTER_SCHEDULER_API_KEY="your-api-key"
export METRICS_PASSWORD="any-password"
export LEADER_ELECTION=false    # no Lease permission from local context
export DEBUG=true               # human-readable logs
# export DRY_RUN=true           # optional: log score changes without patching
```

```bash
make run    # connects via KUBECONFIG, watches all ResourcePools
```

Requires network access to the cluster-scheduler service (may need VPN). Full env var reference: `internal/config/config.go`.

## Deploy to cluster (BuildConfig)

```bash
# One-time setup
oc new-project poolboy-scoring-dev
oc process --local -f build-template.yaml | oc apply -f -

# Build image on cluster
oc start-build poolboy-scoring --from-dir=. --follow

# Deploy with Helm
cp helm-vars-dev.yaml.example helm-vars-dev.yaml
# Edit helm-vars-dev.yaml with your namespace + settings
helm template poolboy-scoring helm/ \
  -f helm-vars-dev.yaml \
  --set=image.tagOverride=- \
  --set=image.repository=$(oc get is poolboy-scoring -o jsonpath='{.status.dockerImageRepository}') \
  | oc apply -f -
```

After code changes:

```bash
oc start-build poolboy-scoring --from-dir=. --follow
oc rollout restart deployment/poolboy-scoring    # required: running pod won't detect new image digest
```

## Version bump

```bash
bash bump-version.sh           # auto-increment patch (v0.1.0 → v0.1.1)
bash bump-version.sh v0.2.0    # explicit version
```

Updates `version` and `appVersion` in `helm/Chart.yaml`, creates a signed commit and git tag, pushes both. Must be on `main` branch (use `--dev` or `--force` to override).

## CI/CD

- `.github/workflows/ci.yml` — runs on push/PR: `make test`, `make lint`, `helm lint helm/`
- `.github/workflows/release.yml` — runs on `v*` tag: validates Chart.yaml appVersion matches tag, builds multi-arch image (amd64 + arm64), pushes to `quay.io/rhpds/poolboy-scoring` with tags `latest`, `vMAJOR.MINOR`, `vMAJOR.MINOR.PATCH`

## Troubleshooting

| Problem                                                 | Fix                                                                           |
| ------------------------------------------------------- | ----------------------------------------------------------------------------- |
| `Failed to load config: required key ... missing value` | Set `CLUSTER_SCHEDULER_URL`, `CLUSTER_SCHEDULER_API_KEY`, `METRICS_PASSWORD`  |
| `cluster-scheduler returned 401`                        | Check `CLUSTER_SCHEDULER_API_KEY` against the scheduler's Secret              |
| `context deadline exceeded` on /evaluate                | Increase `SCORE_TIMEOUT` (default 5s) or check network                        |
| `failed to acquire lease`                               | Use `LEADER_ELECTION=false` for local dev                                     |
| RBAC errors                                             | Apply Helm chart with RBAC resources, check `helm/templates/rbac.yaml`        |
| Score patches not appearing                             | Check `DRY_RUN` is `false` (default)                                          |
| All handles get same score                              | Expected when pool has only one cluster — scheduler needs multiple candidates |
| Pre-commit hook fails on `helm lint`                    | `brew install helm`                                                           |
