# PLAN-Workload-A.md — Workload A named plan (frozen)

> Frozen artifact for CLOSE-009 Workload A. Do not edit after freezing.

- module: example.com/sop-baseline/a
- fixture: isolated task-sized Go module adapted from existing SOP/controller
  disposable dogfood scaffolding (scripts/), not a new performance subsystem.

## Tasks

### A001 — Implement addition

- Description: Implement `sum.Add(a, b int) int` so it returns `a + b`, changing
  **ONLY** `sum.go`.
- Acceptance: immutable `sum_test.go` table `Add(1,2)=3`, `Add(-2,5)=3`,
  `Add(0,0)=0` passes.
- Deterministic checks: `gofmt -l .` (empty), `go vet ./...`, `go test ./...`,
  `go build ./...`.
- Gate: full governed named-plan IMPLEMENT/FIX lifecycle with review and the
  actually-applicable approval gates. A successful no-op is **not** reclassified as
  implementation.

## Frozen input hashes

See `HASHES.md`.
