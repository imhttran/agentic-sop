# PLAN-Workload-B.md — Workload B named plan (frozen)

> Frozen artifact for CLOSE-009 Workload B. Do not edit after freezing.

- module: example.com/sop-baseline/b
- fixture: isolated task-sized Go module with two production packages.

## Tasks

### B001 — Implement Add and Total

- Description: Implement `internal/sum.Add(a, b int) int` to return `a + b`, and
  make `internal/caller.Total(a, b int) int` forward to `sum.Add`. Change at
  least those **TWO** production files (`internal/sum/sum.go` and
  `internal/caller/caller.go`). No new production architecture.
- Acceptance: immutable `sum.Add` A-cases (`Add(1,2)=3`, `Add(-2,5)=3`,
  `Add(0,0)=0`) and immutable `caller.Total(4,5)=9`, `caller.Total(-4,2)=-2`.
- Deterministic checks: `gofmt -l .` (empty), `go vet ./...`, `go test ./...`,
  `go build ./...`.
- Gate: full governed named-plan IMPLEMENT/FIX lifecycle with review and the
  actually-applicable approval gates.

## Frozen input hashes

See `HASHES.md`.
