# PLAN --- Implementation Convergence

**Repository:** `~/agentic-workspace/agentic-sop`\
**Scope:** `agentic-sop` only\
**Status:** PROPOSED (not activated; not implemented)\
**Basis:** completed read-only convergence investigation (2026-10-07)\
**External dogfood evidence:** `sop-decision-adapters` CLEF-014 (IMPLEMENT), CLEF-013 (FIX)

## Project

Implementation Convergence

## Summary

Harden the bounded-convergence behavior of the shared IMPLEMENT/FIX execution
engine (`internal/ollamaagent`) so that a run which has sufficient context but
keeps performing non-mutating activity converges toward one of: a repository
mutation / mutation attempt, a concrete blocker, or an explicit terminal
failure. The change is provider-neutral, model-neutral, task-domain-neutral,
independent of Clef and of Ollama/oMLX, and compatible with the existing
lifecycle, retry, continuation/checkpoint, and approval semantics. It does
**not** raise any iteration, stale, tool-call, or retry budget.

The work proceeds through six stages: baseline capture (CONV-001), a
deterministic IMPLEMENT regression (CONV-002), a deterministic FIX regression
(CONV-003), the minimal provider-neutral enforcement change plus spec update
(CONV-004), full regression and safety verification (CONV-005), and a bounded
Clef dogfood comparison against the preserved CLEF-014 evidence (CONV-006).

CONV-001..003 are non-mutating to production; CONV-004 is the only
production-behavior change; CONV-006 is external, read-only dogfood.

## Capabilities

### Go build/test toolchain — EXISTS

- Evidence: `agentic-sop` builds and tests with the repository Go toolchain; the
  existing `internal/ollamaagent` test suite exercises the harness with fake
  model/transport servers.
- Owner: operator-supplied development environment
- Location: Go executable available on PATH

## Goal

Make convergence in IMPLEMENT and FIX **enforced** rather than merely
**bounded**. Today the engine bounds a non-mutating run (it stops with
`*_NO_PROGRESS`) but does not require the model to attempt a mutation or to
produce a concrete blocker. The goal is the smallest provider-neutral change
that requires bounded forward progress toward one of:

- a successful repository mutation, or
- a concrete blocker / explicit terminal failure reported by the model.

## Non-Goals

- Do not increase `SOP_OLLAMA_IMPLEMENT_ITERATIONS`, `SOP_OLLAMA_FIX_ITERATIONS`,
  `SOP_OLLAMA_STALE_ITERATIONS`, `SOP_OLLAMA_TOOL_CALLS`, or any retry limit.
- Do not implement or change any Clef, Julia, Nimble, or other provider/model.
- Do not add provider- or model-specific behavior or thresholds.
- Do not introduce a new execution architecture if the existing state machine
  can express the invariant.
- Do not make every read in IMPLEMENT/FIX an automatic failure.
- Do not remove legitimate bounded reasoning before the first mutation.
- Do not change approval boundaries, commit/push authorization, provider/model
  selection, or fail-closed behavior.
- Do not modify `sop-decision-adapters`.
- Do not treat the Clef plan or its tasks as part of this plan.

## Architecture Invariant

No subsystem outside `internal/ollamaagent` (and, where a bound already lives
there, `internal/budget`) is required to change. The enforcement is expressed in
the shared phased engine (`executePhased`) and the invocation-scoped
`executionState`, so IMPLEMENT and FIX inherit it identically and no other
capability (PLAN, REVIEW, DESIGN_TESTS, DIAGNOSE) is affected.

```text
capability policy (policy.go)
        |
        v
executePhased (orchestrate.go)  <-- enforcement lives here
        |
        +-- executionState (state.go)  progress / mutation / stale accounting
        |
        +-- toolharness (unchanged)    tool policy, mutation verification
```

The invariant must hold identically for every provider and model, and must not
name any provider, model, or task domain.

## Diagnosis (authoritative basis)

The completed read-only investigation established the following in-repository
facts. They are the authoritative basis for this plan.

### Observed failure signature (external dogfood evidence)

```text
FAIL: IMPLEMENT_NO_PROGRESS: the Ollama agent IMPLEMENT made no repository
progress after 5 consecutive stale iterations (iterations=17,
discovery_inspections=10, repository_mutations=0, changed_files=0,
tool_calls=16, termination=no_progress, last_action="run_command ...")
continuation checkpoint (phase=DISCOVER, inspected=...)
```

```text
FAIL: FIX_NO_PROGRESS: the Ollama agent FIX made no repository progress after
5 consecutive stale iterations (iterations=17, discovery_inspections=9,
repository_mutations=0, changed_files=0, tool_calls=17,
termination=no_progress, last_action="run_command ...")
```

### Termination arithmetic (verified across runs)

For a run that continuously performs credited discovery and never mutates, the
engine terminates at `implementNowAfter + staleIterations`:

| Run | Capability | staleIterations | iterations at stop | mutations |
| --- | --- | --- | --- | --- |
| CLEF-004 | IMPLEMENT | 8 | 20 | 0 |
| CLEF-005 | IMPLEMENT | 8 | 20 | 0 |
| CLEF-016 | IMPLEMENT | 5 | 17 | 0 |
| CLEF-014 | IMPLEMENT | 5 | 17 | 0 |
| CLEF-013 | FIX | 5 | 17 | 0 |

Every observed stop lands exactly on `12 + staleIterations`. The run is bounded
and fails closed, but it is not required to converge: the model may consume the
full discovery window (12 model turns) and then the stale allowance doing
read-only work.

### Authoritative implementation points

- `internal/ollamaagent/orchestrate.go` — `executePhased`, the shared
  IMPLEMENT/FIX engine: the loop, phase transitions, stale/repeat termination,
  steering, and `staleLimitFor`.
- `internal/ollamaagent/state.go` — `executionState`: `consecutiveNoProgress`,
  `observeMutation`, `observeDiscovery`, `stalled`, `countNonMutatingInteraction`,
  `finalizeEligible`.
- `internal/ollamaagent/policy.go` — `CapabilityPolicy`, `policyFor`, and the
  thresholds `implementNowAfter` (12), `implementClosingAfter` (19),
  `implementLateStageAfter` (22), `implementForceFinalizeAfter` (28).
- `internal/ollamaagent/protocol.go` — `turnProgress.observe` (repetition guard,
  `noProgressThreshold` = 3), `actionFingerprint`.
- `internal/ollamaagent/evidence.go` — `discoveryIdentity`, `controlledMutation`,
  `commandMutates`.
- `internal/ollamaagent/implement.go`, `internal/ollamaagent/fix.go` —
  `executeImplement` / `executeFix` → `executePhased`.
- `internal/ollamaagent/harness.go` — invocation lifecycle and outcome
  grounding (`CompleteWithEvidence`, `retryNoChangeFailure`,
  `ensureStructuredOutcome`).
- `internal/budget/budget.go` — the `SOP_OLLAMA_*` ceilings (unchanged here).
- Spec of record: `docs/specs/AGENT-PROVIDER.md` §9 "Tool-Harness Bounds and
  Phases".
- Existing backlog: `docs/plans/BACKLOG.md#task-scoped-discovery-budgets`.

### Root-cause classification

- Relative to the current spec, the engine is **conformant**:
  `docs/specs/AGENT-PROVIDER.md` §9 already prescribes that "Continuous novel
  discovery with no mutation therefore stops by turn 17." The bounded
  termination is intended.
- The demonstrated defect is the **absence of a convergence
  requirement**: the engine bounds non-mutating activity but never requires a
  mutation attempt or a concrete blocker. Against the desired invariant this is
  an `IMPLEMENT_ENFORCEMENT_DEFECT` / `FIX_ENFORCEMENT_DEFECT` (same root in the
  shared engine). Adopting the invariant is a deliberate **spec change**
  (see "Unresolved Architecture Decision").

## Progress Semantics (current, authoritative)

The engine distinguishes activity from progress as follows.

| Signal | Counts as progress? | Resets the stale streak? |
| --- | --- | --- |
| Model iteration (a turn) | no | no |
| File read (`read_file`, first-seen, non-empty) | "discovery" | yes, only while turn <= 12 |
| Listing / search (first-seen, non-empty) | "discovery" | yes, only while turn <= 12 |
| Successful non-mutating command (`run_command` that cannot mutate) | "discovery" | yes, only while turn <= 12 |
| Failed read / empty listing / no-match search | no | no |
| Repeated identical action | no (repetition guard) | no |
| Narration / denied tool call | no | no |
| Test execution / test failure | **no** (not counted) | no |
| Phase transition | no | no |
| Mutation attempt (failed, denied, or unverified) | no | no |
| Successful verified repository mutation | **yes (the only mutation signal)** | yes |
| Concrete blocker / terminal failure | terminal | n/a |

Consequences that this plan must address without conflating activity and
progress:

- A successful non-mutating command (for example `go test`) currently earns
  discovery credit and resets convergence pressure inside the discovery window,
  even though it is not implementation progress.
- After turn 12, every non-mutating turn increments the stale streak; the run
  terminates at the stale bound rather than being required to converge.

## Planning and discovery constraint

Repository inspection and discovery performed by tasks in this plan are work to
be performed by the task, not prerequisite capabilities that must already be
VERIFIED before a task may execute. Source inspection, call-path tracing,
existing-test discovery, and report creation are not prerequisite capabilities.
A task may require a capability only when it is an actual externally supplied
runtime, permission, tool, service, or completed dependency.

## CONV-001 — Capture Convergence Baseline

Record the exact current convergence behavior and the authoritative
implementation points that govern it, before any production change, so the
"before" state is unambiguous and the CONV-004 change is anchored.

### Authoritative Inputs

- `internal/ollamaagent/orchestrate.go`
- `internal/ollamaagent/state.go`
- `internal/ollamaagent/policy.go`
- `internal/ollamaagent/protocol.go`
- `internal/ollamaagent/evidence.go`
- `docs/specs/AGENT-PROVIDER.md` §9

### Mutation Targets

- `docs/reports/implementation-convergence/CONV-001-convergence-baseline.md` - the only file this task writes; no production change.

### Dependencies

None

### Requires

- Go build/test toolchain

### Deliverables

- `docs/reports/implementation-convergence/CONV-001-convergence-baseline.md` - the convergence-baseline report recording the current progress semantics, the termination arithmetic, and the exact authoritative files/line anchors for DISCOVER, IMPLEMENT, FIX, progress accounting, mutation accounting, stale/no-progress detection, continuation/checkpoint behavior, and NO_PROGRESS termination.

### Acceptance Criteria

- The DISCOVER -> CHANGE -> FINALIZE state machine and the point at which
  `CHANGE` is entered (first verified mutation) are recorded.
- The current progress semantics table is reproduced from the implementation.
- The termination arithmetic (`implementNowAfter + staleIterations`) is recorded
  with the observed run evidence.
- Continuation/checkpoint behavior (`inspected`, `maxCheckpointFiles`,
  `inspectedSummary`) is recorded.
- The NO_PROGRESS diagnostics (`IMPLEMENT_NO_PROGRESS` / `FIX_NO_PROGRESS`) and
  their retryable disposition are recorded.
- No production change is made.

### Execution Contract

1. Read `state.go`, then `policy.go`.
2. Read `orchestrate.go` and `protocol.go`.
3. Read `docs/specs/AGENT-PROVIDER.md` §9.
4. Produce the baseline report.
5. Finish.

Expected first action: read `internal/ollamaagent/state.go`.

### Production-Change Scope

None.

## CONV-002 — Deterministic IMPLEMENT Convergence Regression

Establish a deterministic, model-free regression that reproduces the current
IMPLEMENT condition: a run that has sufficient context and a concrete target but
performs repeated non-mutating activity until the bound is reached.

### Authoritative Inputs

- `internal/ollamaagent/harness_test.go` (fake Ollama server, `testConfig`,
  `implementRequest`, `writeFile`, `fake.count`)
- `internal/ollamaagent/no_progress_guard_test.go` (current guard tests)
- `internal/ollamaagent/implement_steering_test.go` (steering tests)

### Mutation Targets

- `internal/ollamaagent/convergence_regression_test.go` (new test file)

### Allowed Precedent

- `internal/ollamaagent/no_progress_guard_test.go` — the existing
  scripted-model guard test pattern. Do not survey other packages.

### Dependencies

- CONV-001

### Requires

- Go build/test toolchain

### Deliverables

- A deterministic regression test that drives IMPLEMENT with a scripted model
  performing novel, successful, non-mutating reads of real repository files
  (so discovery is credited) and never mutating.

### Acceptance Criteria

- The test uses only the fake model/transport; no real model, provider, network,
  or Clef dependency.
- The test asserts the run terminates with `*noProgressError`
  (`IMPLEMENT_NO_PROGRESS`, `termination=no_progress`, `repository_mutations=0`)
  and stops at the bound (`implementNowAfter + staleIterations`), not at the
  iteration ceiling.
- The test pins the credited-discovery path, which the existing guard tests do
  not exercise (they read missing files and thus earn no credit).
- The test is deterministic across repeated runs.

### Focused Tests

- `go test ./internal/ollamaagent/ -run ConvergenceImplement`

### Execution Contract

1. Read this task and `no_progress_guard_test.go`.
2. Read `harness_test.go` helpers.
3. Create `convergence_regression_test.go`.
4. Run the focused test.
5. Finish.

Expected first action: create `internal/ollamaagent/convergence_regression_test.go`.

### Production-Change Scope

None (test-only).

## CONV-003 — Deterministic FIX Convergence Regression

Establish the analogous deterministic, model-free regression for FIX.

### Authoritative Inputs

- `internal/ollamaagent/harness_test.go` (`fixRequest`, `testConfig`,
  `writeFile`, `fake.count`)
- `internal/ollamaagent/fix_lifecycle_test.go`
- `internal/ollamaagent/convergence_regression_test.go` (from CONV-002)

### Mutation Targets

- `internal/ollamaagent/convergence_regression_test.go` (extend)

### Allowed Precedent

- `internal/ollamaagent/fix_lifecycle_test.go` — the existing FIX scripted-model
  pattern. Do not survey other packages.

### Dependencies

- CONV-002

### Requires

- Go build/test toolchain

### Deliverables

- A deterministic regression test that drives FIX with a scripted model
  performing novel, successful, non-mutating reads and never mutating.

### Acceptance Criteria

- The test uses only the fake model/transport; no real model, provider, network,
  or Clef dependency.
- The test asserts the FIX run terminates with `*noProgressError`
  (`FIX_NO_PROGRESS`, `termination=no_progress`, `repository_mutations=0`).
- The test is deterministic across repeated runs.

### Focused Tests

- `go test ./internal/ollamaagent/ -run ConvergenceFix`

### Execution Contract

1. Read this task and `fix_lifecycle_test.go`.
2. Extend `convergence_regression_test.go` with the FIX case.
3. Run the focused test.
4. Finish.

Expected first action: extend `internal/ollamaagent/convergence_regression_test.go`.

### Production-Change Scope

None (test-only).

## CONV-004 — Minimal Provider-Neutral Convergence Enforcement

Implement the smallest provider-neutral change in the shared phased engine so a
run in IMPLEMENT/FIX that has sufficient context must converge toward a mutation
attempt, a concrete blocker, or an explicit terminal failure, instead of merely
being bounded.

This is a bounded implementation task, not an architecture-discovery task. The
authoritative production targets, the primary acceptance regressions, the root
cause, and the required mutation are stated here; read only those, then implement
the change. Do not perform broad repository discovery.

AUTHORITATIVE PRODUCTION TARGETS (read; implement in these):
- `internal/ollamaagent/orchestrate.go` - `executePhased`, the shared IMPLEMENT/FIX loop, and its existing `implFinalize` tool-denial path (the in-repo precedent to reuse).
- `internal/ollamaagent/state.go` - `executionState`: `stalled`, `observeDiscovery`, `observeMutation`, `countNonMutatingInteraction`.
- `internal/ollamaagent/policy.go` - `CapabilityPolicy`, `policyFor`, and the thresholds `implementNowAfter` (12) and `staleIterations` (5).

AUTHORITATIVE REGRESSION TESTS (primary acceptance; they MUST pass UNCHANGED):
- `internal/ollamaagent/convergence_regression_test.go`:
  - `TestConvergenceImplementCreditedDiscoveryStopsAtNoProgressBound`
  - `TestConvergenceImplementIsDeterministic`
  - `TestConvergenceFixCreditedDiscoveryStopsAtNoProgressBound`
  - `TestConvergenceFixIsDeterministic`

ROOT CAUSE (established by CONV-001; do not re-diagnose):
The shared phased engine bounds a non-mutating run - it stops at
`implementNowAfter + staleIterations` (12 + 5 = 17) - but it never REQUIRES
convergence: after the discovery window it still allows non-mutating repository
tools, so a model can consume the entire pre-mutation window and the stale
allowance with read-only activity and produce no mutation, no blocker, and no
outcome. `stalled` only counts stale turns; the only tool-revocation phase
today is `implFinalize`, reachable only after a mutation (or at the
unreachable `implementLateStageAfter`). There is no pre-mutation denial of
non-mutating tools.

REQUIRED MUTATION:
In the shared IMPLEMENT/FIX phased path, once the bounded discovery window has
closed (`iteration > implementNowAfter`) and no mutation has been observed
for the `stale_iterations` allowance, deny non-mutating repository tools
(`read_file`, `list_files`, `search_files`, and non-mutating
`run_command`) with a correction prompt, while keeping mutation tools
available, so the model must either attempt a mutation or return a truthful
`needs_human`/`failed` outcome. A run that still refuses terminates
with the existing `IMPLEMENT_NO_PROGRESS` / `FIX_NO_PROGRESS`
(`*noProgressError`) and the retryable disposition. Reuse the existing
`stalled`/`observeDiscovery` accounting and the existing
`implFinalize` denial mechanism; introduce no new budget knob.

NON-GOALS:
- No architecture redesign and no new execution engine.
- No provider-, model-, or domain-specific behavior (no Clef/Ollama/oMLX/Julia).
- No new iteration/stale/tool-call budget knob; change no budget.
- No plan-schema, approval, lifecycle, retry, or checkpoint change.
- No model-routing or provider-selection change.
- Do not edit, weaken, or rewrite the CONV-002/CONV-003 regressions.

DISCOVERY CUTOFF:
Read only the named production targets, the named regression tests, and at most
the directly required adjacent helper/type definitions. After those bounded
reads, IMPLEMENT THE CHANGE. Do not perform broad repository discovery.

FIRST EXPECTED MUTATION:
`internal/ollamaagent/state.go` (the bounded enforcement accounting), then
`internal/ollamaagent/orchestrate.go` (the pre-mutation denial in
`executePhased`).

### Authoritative Inputs

- `internal/ollamaagent/orchestrate.go` (`executePhased`)
- `internal/ollamaagent/state.go` (`executionState`)
- `internal/ollamaagent/policy.go` (`CapabilityPolicy`, `policyFor`)
- `internal/ollamaagent/implement.go` (IMPLEMENT instructions)
- `internal/ollamaagent/fix.go`
- `internal/ollamaagent/convergence_regression_test.go` (from CONV-002/003)

### Mutation Targets

- `internal/ollamaagent/state.go`
- `internal/ollamaagent/orchestrate.go`
- `internal/ollamaagent/policy.go`
- `internal/ollamaagent/implement.go` (instruction text only, if required)
- `docs/specs/AGENT-PROVIDER.md` §9 (invariant update)

### Allowed Precedent

- `internal/ollamaagent/orchestrate.go` — the existing `implFinalize` tool
  revocation, which is the in-repo precedent for a phase that denies tools and
  requires a structured outcome. Reuse the mechanism; do not invent a new engine.

### Dependencies

- CONV-003

### Requires

- Go build/test toolchain

### Deliverables

- A bounded enforcement in the shared engine such that, after the discovery
  window plus a small non-mutating allowance with no mutation, non-mutating
  repository tools are denied with a correction prompt while mutation tools stay
  available, so the model must either attempt a mutation or return a truthful
  `needs_human`/`failed` outcome; a run that still refuses terminates
  `*_NO_PROGRESS` (unchanged fail-closed behavior).
- An update to `docs/specs/AGENT-PROVIDER.md` §9 recording the invariant.

### Acceptance Criteria

- The change lives in the shared phased engine; IMPLEMENT and FIX inherit it
  identically, and no other capability is affected.
- Legitimate bounded reasoning before the first mutation is preserved: reads
  remain available through the existing bounded discovery window.
- A run that mutates is unaffected (the mutation-based CHANGE/FINALIZE lifecycle
  and the multi-file-write protection are unchanged).
- A run that never mutates and never converges reaches a bounded terminal with
  `IMPLEMENT_NO_PROGRESS` / `FIX_NO_PROGRESS` and the retryable disposition, or
  returns a truthful concrete blocker/failure.
- No iteration, stale, tool-call, or retry budget is changed.
- No provider/model/domain-specific behavior is introduced.
- Approval boundaries, commit/push authorization, provider selection, and
  fail-closed behavior are unchanged.
- CONV-002 and CONV-003 pass (as the "after" behavior), and the pre-existing
  `internal/ollamaagent` suite passes.

### Focused Tests

- `go test ./internal/ollamaagent/...`

### Execution Contract

1. Read `orchestrate.go` (`executePhased` and the `implFinalize` denial path).
2. Read `state.go` and `policy.go`.
3. Implement the smallest enforcement in the shared engine.
4. Update `docs/specs/AGENT-PROVIDER.md` §9.
5. Run the focused tests; harden only failures attributable to this task.
6. Finish.

Expected first action: edit `internal/ollamaagent/state.go` to add the bounded
enforcement accounting.

### Production-Change Scope

- `internal/ollamaagent` engine behavior (bounded enforcement).
- `docs/specs/AGENT-PROVIDER.md` §9 invariant.

## CONV-005 — Regression and Safety Verification

Verify the enforcement change against the regressions and the full repository
gates, with explicit attention to safety and lifecycle behavior.

### Authoritative Inputs

- `internal/ollamaagent/convergence_regression_test.go`
- `internal/ollamaagent/no_progress_guard_test.go`
- `internal/ollamaagent/implement_steering_test.go`
- `internal/ollamaagent/fix_lifecycle_test.go`
- `internal/ollamaagent/checkpoint_continuation_test.go`
- The repository validation configuration (build/test/lint)

### Mutation Targets

- Test files only, and only where the diagnosis or the new invariant requires:
  `internal/ollamaagent/convergence_regression_test.go`.

### Dependencies

- CONV-004

### Requires

- Go build/test toolchain

### Deliverables

- `docs/reports/implementation-convergence/CONV-005-regression-safety.md` - the verification report recording the focused regressions, the full suite, and the safety/lifecycle checks below.

### Acceptance Criteria

- `go build ./...`, `go test ./...`, and `go vet ./...` pass.
- The IMPLEMENT and FIX convergence regressions pass.
- Retry semantics are unchanged: a non-converging run still yields the retryable
  incomplete disposition; retries are never silently reset.
- Continuation/checkpoint behavior is unchanged: the inspected-path checkpoint is
  still produced and still bounded.
- Approval boundaries, fail-closed behavior, and lifecycle transitions are
  unchanged.
- Provider neutrality holds: no provider/model/domain name or branch is added.
- The change does not regress a run that mutates.

### Focused Tests

- `go test ./internal/ollamaagent/...`
- `go test ./...`

### Execution Contract

1. Run the convergence regressions.
2. Run `go test ./...` and `go vet ./...`.
3. Inspect the diff for provider/model/domain leakage and budget changes.
4. Produce the verification report.
5. Finish.

Expected first action: run the focused convergence regressions.

### Production-Change Scope

None beyond test files.

## CONV-006 — Clef Dogfood Verification

Compare the harness before and after CONV-004 against the preserved CLEF-014
case, as external, read-only dogfood evidence.

### Authoritative Inputs

- `sop-decision-adapters` preserved run artifacts for CLEF-014
  (`.agent-sdlc/runs/CLEF-014/`: `attempt.txt`, `trace.json`,
  `classification.json`, `report.md`) — read-only
- The repaired CLEF-014 task definition (unchanged)

### Mutation Targets

None in `sop-decision-adapters` (external, read-only). No CLEF-014 retry is
authorized by this plan.

### Dependencies

- CONV-005

### Requires

- Go build/test toolchain

### Deliverables

- A comparison report: same repaired CLEF-014 task/context, same default budget,
  harness before vs after the CONV-004 change.

### Acceptance Criteria

- The comparison uses the same repaired CLEF-014 task/context and the same
  default budget, with no increase to `SOP_OLLAMA_STALE_ITERATIONS`.
- The preserved CLEF-014 evidence is not rewritten, and its task definition is
  not modified.
- The report states whether the run converged (mutation attempt / concrete
  blocker) or terminated, and whether the terminal remained fail-closed and
  retryable.
- No Clef production code is changed by this task.

### Execution Contract

1. Read the preserved CLEF-014 artifacts (read-only).
2. Record the "before" behavior from CONV-001.
3. Record the "after" behavior under operator authorization.
4. Produce the comparison report.
5. Finish.

Expected first action: read the preserved CLEF-014 `attempt.txt` and
`trace.json`.

### Production-Change Scope

None.

## Safety Invariants

The proposed change must not:

- automatically authorize mutations;
- bypass human approvals;
- change commit/push authorization;
- alter task approval boundaries;
- change provider/model selection;
- weaken fail-closed behavior;
- silently reset retries;
- rewrite lifecycle history;
- depend on Clef-specific behavior;
- increase any iteration, stale, tool-call, or retry budget.

## Clef Track

- **CLEF-014 remains BLOCKED and preserved** while CONV work is reviewed. Do not
  retry it, do not modify its task definition, do not increase its budget, do not
  skip it to continue, and do not overwrite its run evidence.
- CLEF-014 is **external dogfood evidence only** and must not become the vehicle
  for the harness change.
- The successful CLEF-016/012/013 adapter work remains outside this plan. Task
  context repair is demonstrated to be one useful variable but does not explain
  the independent CLEF-014 failure.

## Unresolved Architecture Decision

CONV-004 changes an intended behavior of `docs/specs/AGENT-PROVIDER.md` §9,
which today deliberately **bounds** rather than **requires** convergence
("Continuous novel discovery with no mutation therefore stops by turn 17").

Before CONV-004, the human must choose one of:

- **A. Adopt the convergence invariant** (require a mutation attempt or a
  concrete blocker after bounded discovery) — the path this plan implements,
  with a spec §9 update.
- **B. Keep the current bounded behavior and address convergence elsewhere** —
  for example via the existing backlog item
  `docs/plans/BACKLOG.md#task-scoped-discovery-budgets` or
  stronger configured-model guidance — in which case CONV-004 is **NOT_REQUIRED**.

CONV-001..003 and CONV-005 remain valuable under either choice (they close a real
test-coverage gap: the current guard/steering tests exercise only the
non-credited path and assume behavior that credited discovery violates).

## Dependency Graph

```text
CONV-001 (baseline, read-only)
    |
CONV-002 (IMPLEMENT regression, test-only)
    |
CONV-003 (FIX regression, test-only)
    |
CONV-004 (minimal enforcement + spec update)   [requires decision A]
    |
CONV-005 (regression + safety verification)
    |
CONV-006 (Clef dogfood comparison, external, read-only)
```
