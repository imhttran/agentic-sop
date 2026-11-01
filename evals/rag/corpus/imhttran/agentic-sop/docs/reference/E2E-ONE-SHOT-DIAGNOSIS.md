# End-to-End One-Shot Diagnosis

Baseline: `main` @ `72a463c`. Evidence: the Validation Evidence Reuse execution
(`agentic-sop-ver` @ `8b99ae0`) and the ONESHOT-001 run
(`agentic-sop-e2e-one-shot/.agent-sdlc/runs/ONESHOT-001/report.md`).

## Observed transitions (VER)

- `VER-001` PASSED only on its 4th continuation, after the agent made a real
  mutation; earlier attempts hit `ALREADY_SATISFIED rejected`.
- `VER-002` PASSED after 2 fix cycles; its objective outcome was `HOLD` (a plan
  acceptance check contained a `|`, rejected as a shell metacharacter).
- `VER-003` `ALREADY_SATISFIED rejected` -> `CONTINUE (INCOMPLETE_IMPLEMENTATION)`
  -> review `FAIL` + `sop approve --run` -> `ALREADY_SATISFIED rejected` again ->
  `BLOCKED` ("retry budget exhausted").
- `VER-004` never ran (DAG-blocked behind `VER-003`).
- Every task derived **MEDIUM**; the LOW targeted set was never selected.

## Root causes (source)

- **RC-1 ALREADY_SATISFIED is model-evidence-gated.** `internal/ollamaagent/already_satisfied.go`
  `verifySatisfiedDetailed` accepts a no-mutation claim only with a conformant
  `CompletionEvidence` block (criterion -> *read* paths) plus in-process
  `ValidationCommands`; a smaller model cannot, so a complete task is rejected.
  (`internal/ollamaagent/orchestrate.go:393-423`.)
- **RC-2 no recovery after an externally repaired failure.** `go build ./...`
  failed, auto-FIX exhausted 3/3 (`internal/quality/quality.go:188`,
  `config.DefaultMaxFixCycles=3`), the repair was not re-observed.
- **RC-3 tier collapses to MEDIUM.** `internal/cli/verification_tier.go` called
  `router.SignalsFrom(nil, ...)` so `JEVAvailable=false` and `verify.Derive`
  always returned `MEDIUM` (`internal/verify/verify.go:128-141`); LOW was
  reachable only via an operator `verify-first` declaration.
- **RC-4 the run halts on the first block.** `driveGraph` (`internal/cli/drive.go`)
  returns the task code on any block, so independent runnable tasks do not
  proceed; continuing needs a fresh `sop run`.
- **RC-5 stage-id grammar.** `internal/planner/markdown.go:26` `mdIDToken` needs a
  letters-only prefix; `E2E-001` parses as id `E2E`. Task ids therefore use
  `ONESHOT-00x`.

## ONESHOT-001 note (agent NO_PROGRESS)

The delegated `sop run` blocked `ONESHOT-001` with `NO_PROGRESS`. The two denied
tool calls were **not** a permission or harness bug: SOP's convergence enforcement
denies non-mutating repository tools (`read_file`/`search_files`/`list_files`) once
the agent exhausts its discovery window, to force a mutation
(`internal/ollamaagent/convergence_enforcement_test.go`). The agent never attempted
a mutation and terminated with `narrate`. Root cause: **model behavior**, not tool
permissions.

## Status

- ONESHOT-004 (tier selection) — implemented in `internal/cli/verification_tier.go`.
- ONESHOT-001 (this document + pinning test) — implemented.
- ONESHOT-002/003/005 (independent ALREADY_SATISFIED, recovery after repair,
  continuous graph) — **not implemented**: they change the agent-acceptance and
  the run-loop fail-closed semantics (`already_satisfied.go`, `drive.go`), and
  their validation depends on the lifecycle tests in the full `internal/cli`
  suite, which hangs in this environment. Implementing them without that
  validation would risk the fail-closed invariants.

## The "hanging" internal/cli suite (Phase 1 finding)

`internal/cli` is **not deadlocked**. Its `TestMain`
(`decision_evidence_crossmodule_test.go`) skips the whole package under `-short`
(`os.Exit(0)`) and otherwise builds an out-of-module fixture then runs ~651 tests
that drive real processes — about 10 minutes, which exceeds Go's default
per-package 10m timeout. `go test ./...` (no `-short`) therefore panics with
`test timed out after 10m0s`. Verified:

- `go test -short ./internal/cli/` -> `ok  0.003s` (package skipped).
- `go test -run LifecycleGate ./internal/cli/` -> `ok  0.188s` (targeted run works).

No production defect and no failing fixture. Remedy: bounded/targeted runs
(`-run`), `-short` for routine validation, and the full suite only as a
long-timeout regression gate — which is the design the package's own `TestMain`
already documents.

## Remaining work (ONESHOT-002/003/005)

These change fail-closed semantics and are not safe to land without the real
lifecycle regression suite:

- ONESHOT-002: an independent acceptance path in `internal/ollamaagent` that runs a
  task's deterministic acceptance checks to accept an already-complete task without
  model-authored evidence (`already_satisfied.go`, `orchestrate.go`).
- ONESHOT-003: bounded re-validate-and-continue when a previously failing gate is no
  longer reproducible (`internal/cli`, `internal/failure`).
- ONESHOT-005: continue independent runnable tasks after a task-local block while a
  genuine approval boundary still stops the run (`driveGraph`, `internal/scheduler`).

## Implementation attempt and revert (design conflict + validation failure)

An attempt to implement ONESHOT-004 and ONESHOT-005 was reverted because both
regressed committed tests that encode the intended design:

- ONESHOT-004 (lifting a plan-time task shape to LOW) broke
  `TestSelectValidationMediumIsBuildAndTest` (`verification_tier_test.go`), which
  pins that an ordinary task is MEDIUM. LOW is intentionally reachable only via
  operator `verify-first` (or real JEV evidence), so reclassifying ordinary tasks
  would also risk running package-scoped deep tasks on an under-covering targeted
  set.
- ONESHOT-005 (continuing independent tasks past a task-local block) broke
  `TestPlanRepairAccountingOnExhaustion` (`perf_test.go`), a plan-repair accounting
  invariant. The run loop's stop-on-block behavior is load-bearing for that
  accounting, so the change is not safe without deeper reconciliation.

Both were reverted to their committed behavior. ONESHOT-002 and ONESHOT-003 were
not attempted. These require a design decision on tier selection and on run-loop
continuation accounting, which is the boundary recorded here.

## Reconciled design (operator decisions) and status

Decisions:
1. Ordinary tasks stay MEDIUM; LOW requires an explicit verify-first declaration or
   trustworthy evidence under the existing risk policy; HIGH is unchanged.
2. Preserve plan-repair accounting, retry budgets, and terminal classification.
3. Independent tasks continue after a task-local terminal block only when scheduler
   policy permits; the blocked task is never cleared or overwritten.
4. Mandatory human approval and global safety boundaries still pause execution.
5. An ALREADY_SATISFIED claim is accepted only after deterministic acceptance verification.
6. A repaired failure recovers only after fresh validation + review evidence, bounded.

Status:
- ONESHOT-004 (tier selection): the existing contract already matches decision 1, so
  this task is closed through tests + documentation with no production change. Pinned
  by TestOneShotTierSelection (ordinary -> MEDIUM; verify-first -> LOW; risk_based off
  -> full) and the existing verification_tier_test.go.
- ONESHOT-001 (diagnosis): done (this document + TestOneShotDiagnosisClassificationKinds).
- ONESHOT-002/003/005: not implemented. ONESHOT-005 must preserve the plan-repair
  accounting — run.go:616-629 is coupled to the task attempt path — and an earlier
  attempt that changed the run loop broke TestPlanRepairAccountingOnExhaustion, so it
  was reverted. ONESHOT-002/003 change fail-closed agent-acceptance semantics. These
  remain open pending a focused implementation that preserves the contracts above.

## ONESHOT-005 implemented (this pass)

At the graph scheduling boundary, `driveGraph` now distinguishes:
- a **task-local terminal block** (the task is BLOCKED) — recorded and the loop
  continues to independent work;
- a **productive requeue / CONTINUE** (the task stays PLANNED) — stops the run as
  before;
- **WAITING_FOR_HUMAN** (approval boundary) — stops the run;
and it stops when no independent runnable task remains.

The scheduler did not expose a read-only "runnable exists" predicate, so
`Scheduler.HasRunnable()` was added (Tier-1 predicate, no mutation). Before each
`Next`, the driver stops when a task blocked in this invocation and no runnable
task remains — so `Next`'s Tier-2 recovery can never requeue (and thereby
overwrite) the blocked task. `HasRunnable` does not touch plan-repair accounting:
`TestPlanRepairAccountingOnExhaustion` and `TestRunGraphNoProgressBlocksWithoutApproval`
both pass.

Regression tests: `TestOneShotContinuesIndependentTaskAfterBlock` (S001 blocks,
independent S002 still executes; run fails closed) and
`TestOneShotBlockedDependencyDoesNotRun` (S002 depends on blocked S001 and never runs).

Full affected CLI suite: `ok 861.309s`.

## ONESHOT-006 acceptance (final)

Six canonical scenarios — all PASS via the injected-agent CLI integration harness
(real dispatcher, scheduler, graph driver, lifecycle):
1. Verified ALREADY_SATISFIED advances — `TestOneShotAlreadySatisfiedAdvancesGraph`.
2. Unverified claims cannot advance — `TestRunVerifiedAlreadySatisfied` (model_claim_only, independent_validation_fails).
3. Repaired failures revalidated — `TestOneShotRepairedFailureRevalidatesAndContinues`, `TestRunFixLoopRepairsFailingValidation`.
4. Unrepaired failures escalate bounded — `TestOneShotUnrepairedFailureEscalatesBounded`, `TestPlanRepairAccountingOnExhaustion`.
5. Independent tasks continue after a block — `TestOneShotContinuesIndependentTaskAfterBlock`.
6. Blocked dependencies + approval respected — `TestOneShotBlockedDependencyDoesNotRun`, `TestDeclineHumanGatePreservesLifecycle`, `TestRunGraphNoProgressBlocksWithoutApproval`.

Live disposable-project E2E — PASS: a real `sop run` (provider ollama / model
deepseek-v4.1-flash:cloud) executed a 2-task plan (S001 create `hello.txt`, S002 create
`world.txt`); the model produced the mutations; both tasks PASSED; the graph advanced
S001→S002; 2/2 complete ("SOP COMPLETE", final gate PASS). Deliverables verified with
native tools (`hello.txt`="hello", `world.txt`="world").

Recovery behavior: a failure that is no longer reproducible is re-validated within the
bounded fix loop and, when it passes, the task completes; a still-failing failure
escalates (AUTO_FIX_EXHAUSTED → NEEDS_HUMAN) bounded by `max_fix_cycles`. A task-local
terminal block does not halt independent runnable tasks (ONESHOT-005), while a blocked
dependency and a genuine human approval boundary still stop the graph.

Tier selection: `verification.risk_based` selects the gate's command set from a
deterministic, escalate-only tier — ordinary tasks are MEDIUM (build+test); LOW requires
an explicit verify-first declaration (or trustworthy evidence); HIGH runs the full set;
`risk_based: false` reproduces the full configured set exactly.

Full affected CLI suite: ok 879.523s.
