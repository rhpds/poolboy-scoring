# Project Conventions

## Unstructured Kubernetes objects with partial typed access

The controller works with Poolboy and Anarchy CRDs via `unstructured.Unstructured`
(no generated Go types). Field access uses partial Go structs: parse the unstructured
object's spec/status into a typed struct with `runtime.DefaultUnstructuredConverter.FromUnstructured`,
then access fields with type safety. See `internal/placement/types.go` for all partial types.

## Pool-based reconciliation

The reconciler watches ResourcePool objects. Each reconciliation collects ALL unbound
handles in a pool, groups them by cluster, and makes ONE batch `/evaluate` call. This
produces relative scores across clusters — the cluster-scheduler's `most_capacity`
strategy requires multiple candidates to generate differentiated scores.

## Skills

Domain knowledge and conventions are documented in `skills/`:

- `poolboy-scoring-*` — Project-specific: architecture, code style, development workflow
- `rhdp-*` — RHDP ecosystem context (Poolboy, Anarchy, cluster-scheduler)

Check relevant skills before diving into unfamiliar code.
