# RSH-005 — Verification, Neutrality, and Documentation

Status: verification report for the review-hardening change set (RSH-002..RSH-004).

## 1. Scope

This report records the verification evidence for RSH-005: the repository gates,
the provider/model-neutrality guard (including its extension to the `internal/cli`
review and change-attribution seam), the approval/fail-closed/lifecycle/budget
invariants, the documentation link and spec-coverage checks, and the confirmation
that no Phase 8 / POST8-001 / EV evidence was modified.

## 2. Repository gates

| Command | Result |
| --- | --- |
| `go build ./...` | PASS |
| `go test ./...` | PASS |
| `go vet ./...` | PASS |
| `go test -race ./...` | PASS |

See §7 for the extended neutrality coverage added to the `internal/cli` seam.

## 3. Neutrality

The provider/model-neutrality architecture test in `internal/archtest`
(`TestArchitectureGuard`, `TestSameCorePipelineAcrossAdapters`,
`TestAdaptiveRoutingIsCapabilityAndEvidenceDriven`, …) continues to pass. No
provider, model, or domain name or branch was added to production code by
RSH-002..RSH-004.

### 3.1 Extended coverage: `internal/cli` review and change-attribution seam

The existing `internal/archtest` guard protects the Phase 8 core and policy
packages; it does **not** cover the review and change-attribution seam in
`internal/cli` (`reviewscope.go`, `jev.go`, `review.go`). RSH-005 adds a
dedicated provider/model-neutrality check for that seam in
`internal/archtest/review_seam_neutrality_test.go`, in the existing archtest style.

## 4. Invariants

Approval boundaries, fail-closed behavior, and lifecycle transitions are unchanged;
`scopedReviewDiff` still returns `ErrReviewScopeUnestablished` (fail closed) when a
task that required a change has no attributable change, and no budget value or
budget logic was changed by RSH-002..RSH-004.

## 5. Documentation

Documentation links under `docs/` resolve. `docs/specs/REVIEW.md` now describes the
scoped review (`§5` no-changes case) and the malformed-output boundary (`§6`
Malformed Review Output: per-finding parsing, bounded retry, never a pass).

## 6. Phase 8 / POST8-001 / EV evidence

No Phase 8 / POST8-001 / EV evidence artifact under `docs/reports/` is modified by
RSH-002..RSH-005.

## 7. Acceptance criteria map

| Criterion | Evidence |
| --- | --- |
| `go build ./...` passes | §2 |
| `go test ./...` passes | §2 |
| `go test -race ./...` passes | §2 |
| `go vet ./...` passes | §2 |
| Neutrality architecture test passes; no provider/model/domain name or branch added | §3 |
| Neutrality coverage extended to `internal/cli` review/change-attribution seam | §3.1 |
| Approval/fail-closed/lifecycle unchanged; no budget changed | §4, §8 |
| Documentation links pass; specs describe scoped review and malformed-output boundary | §5 |
| No Phase 8 / POST8-001 / EV evidence modified | §6 |

## 8. Invariant diff summary

Approval, fail-closed, lifecycle, and budget code paths were compared against the
RSH-002..RSH-004 diff. The changes are confined to `internal/review/provider.go`,
`internal/review/loop.go`, `internal/cli/jev.go` (rename attribution), and the two
spec files. No approval boundary, fail-closed path, lifecycle transition, or budget
constant is changed.
