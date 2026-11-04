# Plan status audit — 2026-11-04

**Type:** Report-only audit (non-normative). No plan status was changed.
**Revision audited:** `origin/main` at `443fd94f`.
**Scope:** the two plans whose declared status may be stale:
[PLAN-SOP-RUN-ACCEPT-001](../plans/PLAN-SOP-RUN-ACCEPT-001.md) (declared **ACTIVE**) and
[PLAN-SOP-E2E-Reliability-Hardening](../plans/PLAN-SOP-E2E-Reliability-Hardening.md)
(declared **PROPOSED**).

## Method

- Read both plans at the audited revision and mapped each declared task to
  implementation and tests reachable from `origin/main`.
- Ran the ordinary checks and the plans' own tagged acceptance suite
  (`acceptance` build tag, which ordinary CI does not run):

  ```text
  go test -tags acceptance -count=1 -run TestOperatorAcceptance \
    ./internal/cli/ ./internal/planner/ ./internal/planflow/ ./internal/archtest/
  ```

  Result: **15 passed, 6 failed** (all six in `internal/cli`).
- The ordinary suite (`go vet ./...`, `go test ./...`) is green on `main`; the
  failures below are only visible under `-tags acceptance`.

## Verdict

| Plan | Declared | Still accurate? | Basis |
| --- | --- | --- | --- |
| `PLAN-SOP-RUN-ACCEPT-001` | ACTIVE | **Yes — but not because implementation is unfinished** | All eleven scoped tasks are merged and covered. The plan's own operator acceptance checks are red on current `main` after a later criterion-mapping contract landed, so `COMPLETE` would over-claim. |
| `PLAN-SOP-E2E-Reliability-Hardening` | PROPOSED | **Yes** | None of its ten scoped tasks is merged. The prerequisite task (`HARDEN-001b`) remains unmerged, and the repository states so. |

Neither declaration needs an immediate status edit. The RUN-ACCEPT plan needs an
acceptance re-baseline before it can be closed; the hardening plan remains a
proposal.

## PLAN-SOP-RUN-ACCEPT-001

### Declaration

**Status:** ACTIVE. **Baseline:** `c348ab4`.

### Implementation is merged

The plan's work landed through PR #3 (`3be4a90f`), with later closure commits, all
ancestors of `origin/main`:

- `56ff82ac` — native deterministic acceptance checks (ACC-008 closure + prerequisites)
- `ef35166b` — structured `go test -json` evaluation and the repository gofmt guard (ACC-009, ACC-011)
- `94f2c6d1` — reconciliation coverage for acceptance-check changes (ACC-007)
- `b8076c76` — documentation (`README.md`, `docs/reference/CLI.md`, [EXECUTION](../specs/EXECUTION.md))

Task-level evidence in the merged tree:

| Task | Evidence |
| --- | --- |
| ACC-001 | Wrapped list-item continuation joining in `internal/planner/markdown.go` and `internal/taskfile/taskfile.go` (`listItems`) |
| ACC-002 | `planner.Stage.AcceptanceChecks` parsing, rendering, task-file support, and `decodePlan` clearing in `internal/planner` and `internal/taskfile` |
| ACC-003 | `internal/cli/drive.go` sets `spec.AcceptanceChecks`; `report_deliverable.go:planTaskAcceptanceChecks`; `acceptance_gotest.go` |
| ACC-004 | `internal/planflow/acceptance_checks_reconcile_test.go` |
| ACC-005 | `internal/cli/outcome.go` authorization hardening (`-exec`, `-toolexec`, `-vettool`, `-o`, `-overlay`, `-modfile`, `go fmt`, `git -c`/`--config-env`) and per-check evidence |
| ACC-006 | Execution-mode parsing restored; model-authored checks cleared in `decodePlan`; task-file renderer writes `## Acceptance checks` |
| ACC-007 | `internal/planflow/compare.go:sameAcceptanceChecks`; `planflow.go:hydrateAcceptanceChecks`; tests in `acceptance_checks_reconcile_test.go` |
| ACC-008 | `internal/cli/outcome.go` calls `PropagateEvidence` for each executed check (argv, exit code, bounded output) |
| ACC-009 | `internal/cli/acceptance_gotest.go` (`goTestJSONMet`, `ensureGoTestJSON`); `acceptance_gotest_test.go` |
| ACC-010 | Multi-file `Files` parsing in `internal/planner/markdown.go`; `internal/planner/taskshape_test.go` |
| ACC-011 | `internal/archtest/gofmt_test.go` |

Operator acceptance tests for ACC-007/008/009/010/011 also exist under
`zz_operator_acceptance_*_test.go` files.

### Why ACTIVE is still accurate

The plan's own operator acceptance checks are not green on current `main`. Six of
them fail, all in `internal/cli`:

- `TestOperatorAcceptanceACC008GitConfigInjectionHolds`
- `TestOperatorAcceptanceACC008AllowedChecksStillRun`
- `TestOperatorAcceptanceACC009VerifiedWhenRealTestsPass`
- `TestOperatorAcceptanceACC009MixedModuleWithNoTestPackageVerified`
- `TestOperatorAcceptanceACC009ExecutesEachCommandOnce`
- `TestOperatorAcceptanceACC010EachDeliverableVerifiedIndividually`

Each failure is the same: the outcome is `PARTIAL` instead of `VERIFIED`, with the
reason

```text
legacy plan: no acceptance mapping is declared, so criteria [...] are unmapped
and cannot be verified; add an '### Acceptance Mapping' section ...
```

The cause is not a missing ACC task. A later contract (`planner.AcceptanceMapping`
in `internal/planner/plan.go`, enforced by `internal/planner/lint.go` and
`internal/outcome/criteria.go`, introduced in `ca8f0ea0`) requires an explicit
`### Acceptance Mapping` binding free-text criteria to checks or manual evidence.
The RUN-ACCEPT fixtures predate that contract: they declare free-text criteria and
passing checks, but no mapping, so a passing check no longer yields `VERIFIED` on
its own.

Consequently:

- `ACTIVE` is truthful. Declaring the plan `COMPLETE` would claim an acceptance
  state that current `main` does not produce.
- The remaining work is an **acceptance re-baseline**, not implementation. The six
  failing fixtures need `### Acceptance Mapping` sections (or an explicit decision
  to supersede those checks) before the plan can close.
- Ordinary CI cannot catch this: the `acceptance` tag is not part of the CI suite,
  and the ordinary tests pass.

**Recommendation (not applied):** keep `ACTIVE`; open a small follow-up to add the
missing acceptance mappings to the six fixtures, then re-run the tagged suite. If
the operator prefers, the alternative is to record the checks as superseded and
close the plan with that decision.

## PLAN-SOP-E2E-Reliability-Hardening

### Declaration

**Status:** PROPOSED — "NOT activated, NOT executed."

### Per-task findings

| Task | Finding |
| --- | --- |
| HARDEN-001b (prerequisite) | Not merged. `internal/cli/outcome.go` states that per-criterion operator-owned verifier bindings "live in the unmerged harden-001 work"; no `harden-001` branch or tag exists in the repository. |
| HARDEN-001 | Not merged. No `criteria.json` writer exists, and the lifecycle verdict gate is OFF by default (`internal/config/config.go`, `Lifecycle.VerdictGate`), contrary to the plan's "enforced by default". `internal/quality` contains no tri-state criterion enforcement. |
| HARDEN-002 | Not merged. No `tool_allow_roots` or `SOP_TOOL_ALLOW_ROOTS` support exists outside the plan document. |
| HARDEN-003 | Not merged. No `HARDEN-003-controller-parity-evidence.md` exists under `docs/reports/end-to-end-reliability/`. |
| HARDEN-004 | Not merged. No HARDEN-004 fixture or recovery evidence exists (only the earlier ETOE-005 fixture scripts). |
| HARDEN-005 | Not merged. No `HARDEN-005-decision-dispatch-evidence.md` exists. |
| HARDEN-006 | Not merged. `internal/cli/drive.go:runGraph` still resolves the active plan when no plan argument is given; there is no explicit-selector refusal or confirmation for mutating runs. |
| HARDEN-007 | Not merged as specified. Phase-5 attempt records exist (`internal/run/attempt.go`, `.agent-sdlc/runs/<task>/attempts/NNN.json`), but `state.json` and the other run artifacts remain flat per task; there are no attempt-scoped state/trace/validation artifacts. |
| HARDEN-008 | Not merged. [AGENT-PROVIDER](../specs/AGENT-PROVIDER.md) §9 is "Tool-Harness Bounds and Phases"; it does not record the model-versus-harness diagnostic distinction the plan asks for. |
| HARDEN-009 | Not adopted. Dispositions remain `COMPLETE` and `SUPERSEDED` only ([PLAN-HISTORICALIZATION](../specs/PLAN-HISTORICALIZATION.md)); no handoff/closure disposition was added. |

Adjacent, already-merged work does not satisfy any scoped task: the SOP-003
criterion-evidence contract (`internal/outcome/criteria.go`) and the Phase-5 attempt
records are narrower than HARDEN-001b/001 and HARDEN-007 respectively.

### Verdict

`PROPOSED` remains accurate. The plan has not been activated; its prerequisite is
unmerged, and none of the ten task deliverables exists on `main`.

## Limitations

- This audit compares declarations with merged code, tests, and documentation; it
  does not judge whether either plan's priorities are still the right ones.
- It does not run live controller, adapter, or model-provider evidence; those
  HARDEN tasks have no recorded evidence to verify.
- The six acceptance failures were reproduced at the audited revision and are
  deterministic: they follow from the missing `### Acceptance Mapping` sections,
  not from flakiness.
- Per the request, this report changes no plan status; status decisions remain with
  the operator.
