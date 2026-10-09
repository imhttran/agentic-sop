# PLAN — SOP E2E Reliability Hardening

**Status:** PROPOSED — derived from `PLAN-SOP-End-to-End-Reliability` (ETOE-001…010). NOT activated, NOT executed.
**Basis:** ETOE-009 scorecard and ETOE-010 final report (`docs/reports/end-to-end-reliability/`).
**Rule:** Do not declare the SOP platform production-ready. The deterministic fixture is NOT proof of real-model reliability.

## Project

SOP E2E Reliability Hardening

## Summary

Convert the end-to-end reliability assessment's observed defects and UNAVAILABLE coverage into a
scoped, prioritized hardening backlog. Priorities: P1 — deterministic acceptance-criteria
enforcement, operator-authorised cross-repository workspace, controller runtime parity; P2 —
successful FIX-recovery coverage, live decision-adapter dispatch/fallback, explicit project-root
execution safeguards, attempt-scoped artifact isolation, model PLAN/tool-protocol convergence.
Tasks are independent unless a dependency is named. Cross-repository tasks are isolated and
require an explicit operator authorization; the default repository confinement is preserved.

## HARDEN-001 — Deterministic acceptance-criteria enforcement

Make a declared-but-unmet acceptance criterion a blocking, model-free gate outcome. The
**authoritative enforcement location** is the gate evaluation that produces a task's PASS/FAIL
(the quality gate consumed by the run loop in `internal/cli`; the criterion check itself is owned
by the gate/quality evaluator, `internal/quality` or the gate evaluator that emits `gate.json`).
Criterion outcomes must be **machine-readable**: each acceptance criterion is recorded with a
tri-state outcome (MET / NOT_MET / UNAVAILABLE) in a task-level artifact (e.g.
`.agent-sdlc/runs/<id>/criteria.json`) written by the run loop. The gate reads that artifact and
**blocks completion** (no PASS) when any criterion is NOT_MET/UNAVAILABLE, unless an explicit
operator authorization record exists.

### Dependencies

None

### Deliverables

- The gate/quality evaluator change that blocks on NOT_MET/UNAVAILABLE criteria.
- The machine-readable criterion-outcome artifact writer + schema.
- A regression test reproducing ETOE-007 (3 declared criteria, 0 MET) asserting the gate now blocks.
- `docs/specs/` note recording the new gate semantics.

### Acceptance Criteria

- A task with any criterion NOT_MET/UNAVAILABLE cannot reach PASS without an explicit operator authorization record.
- The check is deterministic and model-free (no agent call).
- The ETOE-007 regression (0/3 MET) blocks; a fully-MET task is unchanged.

### Validation Commands

`gofmt -l .`; `go build ./...`; `go vet ./...`; `go test -count=1 ./...`; `go test -race -count=1 ./...`; `git diff --check`.

### Failure Behavior

If a fully-MET task regresses, revert the package; the gate must be additive and separately testable.

## HARDEN-002 — Operator-authorised cross-repository integration workspace

Provide a supported, explicit, non-production mechanism for a governed task to inspect an
allow-listed sibling repository without weakening repository-root confinement for ordinary work.
The allow-list is an **operator-controlled** configuration (`tool_allow_roots` config key and/or
`SOP_TOOL_ALLOW_ROOTS` env), default empty/OFF. It is read **only** from operator configuration or
the environment and **never** from plan, task, or model text. Granting is audited to a machine-
readable artifact (e.g. `.agent-sdlc/runs/<id>/authorized-roots.json`) recording the granted root,
grant time, and the operator source.

### Dependencies

None

### Deliverables

- `internal/toolharness` support for an operator-supplied additional authorized root (config/env), default OFF; read-only access only.
- The audit artifact writer.
- Tests proving (a) default confinement is byte-for-byte unchanged, (b) an allow-listed sibling path is readable read-only and the grant is audited, (c) plan/model text can never grant access.
- `docs/reference/` documentation.

### Acceptance Criteria

- With no allow-root configured, sibling access is denied exactly as today.
- With one explicitly configured, the allow-listed path is readable read-only and the authorization is recorded.
- No plan/task/model text can add an allow-root; the mechanism never enables writes to a sibling or production state.

### Validation Commands

`gofmt -l .`; `go build ./...`; `go vet ./...`; `go test -count=1 ./...`; `go test -race -count=1 ./...`; `git diff --check`.

### Failure Behavior

Any test showing ordinary runs gaining sibling access disables the feature (fail-closed) and reverts.

## HARDEN-003 — Controller runtime parity verification

Exercise `sop-controller` against a disposable SOP project and verify read-only state display
and delegated-action parity with the SOP CLI, including pending approvals, using HARDEN-002 for
reachability. Requires **actual runtime checks** on a running controller instance.

### Dependencies

- HARDEN-002

### Deliverables

- `docs/reports/end-to-end-reliability/HARDEN-003-controller-parity-evidence.md` — build/serve transcripts, `/healthz` response, parity table, permission findings, teardown log.

### Acceptance Criteria

- `/healthz` responds; the server binds loopback by default; a gated route returns 401 without a token; state-changing routes enforce CSRF.
- Controller display and delegated actions match `sop status`/`sop task`/`sop approvals --json` for the same disposable project, including pending approvals.
- The controller mutates authoritative state only through SOP's CLI; no direct state writes observed.
- The disposable controller instance and its project are torn down after the run.

### Validation Commands

Controller: `go build ./...`; `go test ./...`; live `/healthz`/auth/CSRF probes; SOP-side `sop status`/`sop approvals --json` parity. Agentic-SOP: `gofmt -l .`; `go build ./...`; `go vet ./...`; `go test -race -count=1 ./...`; `git diff --check`.

### Failure Behavior

If parity fails, record the exact divergent view/action and keep the prior UNAVAILABLE marking; do not assert parity.

## HARDEN-004 — Successful FIX recovery coverage

Add a fixture scenario with a **deterministic FAIL → FIX → PASS recovery**: the controlled
provider first introduces a validation-failing change, then on the FIX capability performs the
repair so revalidation passes and the task completes.

### Dependencies

None

### Deliverables

- An added fixture scenario + a run captured under `docs/reports/end-to-end-reliability/` evidence.

### Acceptance Criteria

- A run shows validation FAIL → FIX → revalidation PASS → `LOCAL_DONE`, with recorded run id and artifacts.
- The recovery is deterministic (controlled provider); no external model required.
- The failed→recovered transition is reproducible from the recorded commands.

### Validation Commands

`bash -n scripts/etoe-*-fixture-*.sh`; run the scenario in an isolated root; `gofmt -l .`; `go build ./...`; `go vet ./...`; `go test -race -count=1 ./...`; `git diff --check`; `scripts/checks/check-doc-links.sh`.

### Failure Behavior

If the scenario cannot reach PASS deterministically, record the bounded behavior and do not claim recovery.

## HARDEN-005 — Live decision-adapter dispatch and fallback verification

Configure `decision.provider: command` with `decision.command` pointing at an **adapter fixture**
in an isolated project whose stdout is a valid SEAM-002 result DTO, and verify a real dispatch
with a recorded **decision trace** (the run artifact evidencing the SOP→adapter invocation and
the normalized result), plus contract-supported fallback behavior. Unsupported paths stay UNAVAILABLE.

### Dependencies

- HARDEN-002

### Deliverables

- An adapter fixture + `docs/reports/end-to-end-reliability/HARDEN-005-decision-dispatch-evidence.md` — dispatch trace, normalized results, fallback outcomes.

### Acceptance Criteria

- A real SOP→adapter dispatch occurs (trace/log artifact proves it), and the normalized result is consumed by SOP policy.
- Provider failure/fallback is exercised only where the contract supports it; unsupported paths labeled UNAVAILABLE.
- Adapter output cannot override SOP policy (verified).

### Validation Commands

Agentic-SOP: `gofmt -l .`; `go build ./...`; `go vet ./...`; `go test -race -count=1 ./...`; `git diff --check`. Adapter repo: `go test ./...` (offline).

### Failure Behavior

If no dispatch can be evidenced, keep integration UNAVAILABLE and record the failing configuration.

## HARDEN-006 — Explicit project-root execution safeguards

Require **explicit project targeting** for mutating commands: a mutating `sop run` with no
selector (no plan path, no `--task`) must refuse (or require an explicit confirmation in
non-interactive contexts), so a bare run can never silently execute the active plan. Intended
explicit invocations are preserved.

### Dependencies

None

### Deliverables

- `internal/cli` change requiring an explicit selector for mutating runs, with tests.
- `docs/reference/CLI.md` update.

### Acceptance Criteria

- A mutating `sop run` with no selector refuses or requires explicit confirmation in non-interactive contexts.
- Existing explicit invocations (`sop run <PLAN.md>`, `--task`, `--max-tasks`) and read-only commands are unchanged.
- A regression test reproduces the recorded accidental-run scenario and asserts it is prevented.

### Validation Commands

`gofmt -l .`; `go build ./...`; `go vet ./...`; `go test -count=1 ./...`; `go test -race -count=1 ./...`; `git diff --check`.

### Failure Behavior

If any supported invocation regresses, revert; the safeguard must be conservative and separately testable.

## HARDEN-007 — Attempt-scoped artifact isolation

Store retry artifacts **attempt-scoped** (e.g. `.agent-sdlc/runs/<id>/attempt-N/`) instead of
overwriting the task run directory, so a failed attempt's evidence survives a retry. Consumers
(`sop report`, `sop continue`, `sop approvals`) must resolve the **latest attempt**.

### Dependencies

None

### Deliverables

- `internal/run`/`internal/cli` change for attempt-scoped artifacts + tests.
- Migration note (existing single-attempt dirs remain readable).

### Acceptance Criteria

- A retried task preserves the prior attempt's `state.json`/`trace.json`/`validation.json` under an attempt-scoped path.
- `sop report`/`continue`/`approvals` resolve the latest attempt; existing flat layouts keep working.
- No evidence is overwritten across attempts.

### Validation Commands

`gofmt -l .`; `go build ./...`; `go vet ./...`; `go test -count=1 ./...`; `go test -race -count=1 ./...`; `git diff --check`.

### Failure Behavior

If consumers break on the new layout, keep the flat layout and add the attempt-scoped copy alongside; never delete prior evidence.

## HARDEN-008 — Model / harness diagnostic convergence improvements

Improve **diagnostics** distinguishing model-output failures (PLAN synthesis limit, malformed
tool request with empty `tool`, non-progressing tool loops) from harness/infra failures, with
bounded, provider-neutral recovery — **without** raising convergence or retry budgets. Add
regression coverage for the malformed-tool-request and PLAN-synthesis outcomes.

### Dependencies

None

### Deliverables

- Bounded, provider-neutral improvements to PLAN/tool-protocol handling + tests (malformed `tool`, PLAN synthesis limit).
- `docs/specs/AGENT-PROVIDER.md` §9 update recording the model-vs-harness diagnostic distinction.

### Acceptance Criteria

- Malformed tool requests and PLAN synthesis failures produce a clear, actionable diagnostic (not a generic failure).
- No iteration, stale, tool-call, or retry budget is raised.
- Existing convergence behavior is preserved (regression-tested).

### Validation Commands

`gofmt -l .`; `go build ./...`; `go vet ./...`; `go test -count=1 ./...`; `go test -race -count=1 ./...`; `git diff --check`.

### Failure Behavior

If a change relaxes a bound or alters unrelated lifecycle behavior, revert; improvements must be additive.

## Parallelizable groups

- **Group A (independent, separate worktrees):** HARDEN-001, HARDEN-004, HARDEN-006, HARDEN-007, HARDEN-008.
- **Group B (prerequisite first):** HARDEN-002, then HARDEN-003 and HARDEN-005 in parallel (both depend on HARDEN-002).

## Recommended execution order

1. HARDEN-001 (highest priority — closes the acceptance-enforcement defect).
2. Group A remainder (HARDEN-006, HARDEN-007, HARDEN-008, HARDEN-004).
3. HARDEN-002, then Group B (HARDEN-003, HARDEN-005).
