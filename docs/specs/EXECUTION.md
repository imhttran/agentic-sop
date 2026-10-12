# Execution

**Type:** Normative specification

## Purpose

This document is the normative specification for `sop run`, SOP's execution
entrypoint: planning-source discovery, idempotent preparation, machine-plan
compilation, change detection, run artifacts, the bounded fix loop, verify-first
execution, controlled parallelism, and the human gate. The task state graph and
remediation rules are owned by [WORKFLOW.md](WORKFLOW.md).

## Related Specifications

- [../architecture/OVERVIEW.md](../architecture/OVERVIEW.md) — §4 Plan Preparation,
  Task Runner, Execution Modes, Fix Loop, Graph Execution; §11 Parallelism.
- [../PRD.md](../PRD.md) — §5, §7 (FR-3, FR-4, FR-10, FR-22), §9 Idempotency.
- [WORKFLOW.md](WORKFLOW.md) — state vocabulary and remediation;
  [TASK-LIFECYCLE.md](TASK-LIFECYCLE.md) — per-task stages and Git conventions.
- [VALIDATION.md](VALIDATION.md) — configured checks; [QUALITY.md](QUALITY.md) — the
  gate and fix-loop budget; [RECOVERY.md](RECOVERY.md) — retries, resume, durable state.
- [../reference/CLI.md](../reference/CLI.md), [../reference/CONFIGURATION.md](../reference/CONFIGURATION.md),
  [../reference/PERFORMANCE.md](../reference/PERFORMANCE.md) — command reference;
  [../README.md](../README.md) indexes all docs.

## Normative Language

The terms MUST, MUST NOT, SHOULD, SHOULD NOT, and MAY are normative.

---

## 1. Planning-Source Discovery and Precedence

When no plan is named, `sop run` MUST select its planning source in this order: an
existing valid `.agent-sdlc/plan.json` (reused **only** if its source is unchanged),
then `docs/PLAN.md`, `PLAN.md`, `docs/PRD.md`, and finally `PRD.md`.

A human `PLAN.md` MUST be preferred over the PRD, and `plan.json` MUST NOT silently
override a newer PLAN: the source path and a content fingerprint are recorded, and a
changed document MUST trigger a rebuild. A file argument names an execution PLAN
authoritative for that run (`sop run docs/PLAN-Jev.md`); `--task TASK.md` runs one task.

## 2. Idempotent Preparation

`sop run` MUST discover the source, initialize SOP state if needed, compile or
generate the machine plan, create the tasks, and then drive the graph — each step
idempotent. Running it again MUST resume, and completed work MUST NOT be restarted.
Preparation MUST NOT require an agent: a `PLAN.md` is compiled and tasks created
even with no agent configured, and the missing agent MUST be reported only when
execution (or PRD-based plan generation) actually needs it.

## 3. Machine Plan Compilation

`sop run` MUST compile or generate `.agent-sdlc/plan.json` and build the task DAG
deterministically, without an agent; a compiled stage MUST carry its optional
`execution_mode` onto the task. Machine state and run reports MUST live under
`.agent-sdlc/` (self-ignoring for Git), and a PRD-generated plan MUST be written to
`docs/reports/<plan-id>.md` (`sop init`/`sop plan`/`sop tasks` stay lower-level).

**Implemented.** A human PLAN document is compiled deterministically in either of two
recognized shapes, so an agent is consulted only for a document that is not
recognizable as a plan:

- the **rendered shape**: one `## <id> — <title>` section per stage, with optional
  `### Dependencies` / `### Deliverables` / `### Acceptance Criteria` / `### Execution`
  sub-sections;
- the **task-list shape**: a `## Tasks` section whose stages are level-3 headings
  (`### P5-001 — <title>`) described by inline `- **Field:** value` lines
  (`Objective`, `Scope`, `Files`, `Depends on`, `Acceptance`, `Execution`).

**Required.** In the task-list shape a stage's dependencies MUST be an explicit list of
stage ids (`P5-002, P5-003`), or a marker for none (`—`, `none`). A range
(`P35-001..P35-005`), `all`, or a parenthetical is an error rather than a guess: a
dropped edge would silently reorder the graph. Only recognized fields become plan
fields — a stage's `Status:` and `Validation:` lines stay documentation, and the stage's
execution mode MUST NOT be inferred from them. When the document names no `## Project`
or `## Summary`, the project defaults to the document title and the summary to the
`## Objective` section.

## 4. Change Detection

Git MUST be the authority on what changed. Change detection MUST include untracked
files, so a file the agent creates counts even when no tracked file changed; SOP's
own `.agent-sdlc/` output MUST be excluded. Validation MUST fail fast before review.
A run MUST reject a provider that cannot `IMPLEMENT` before running any task.

## 5. Run Artifacts

Every stage SHOULD write an artifact under `.agent-sdlc/runs/<id>/` (`task.md`,
`plan.md`, `implementation.md`, `diff.patch`, `fix-N.md`, `validation.json`,
`review.json`, `outcome.json`, `report.md`, `report.json`, `metrics.json`, `state.json`); a run that
terminates MUST stay inspectable. Each task SHOULD end with a concise performance
line; see [../reference/PERFORMANCE.md](../reference/PERFORMANCE.md).

For a verified no-mutation completion, the IMPLEMENT/FIX outcome contract and
evidence requirements are owned by
[AGENT-PROVIDER.md §8a](AGENT-PROVIDER.md#8a-verified-already-satisfied-completion).
The lifecycle's reports preserve that completion and its evidence alongside the
independent validation and quality verdicts. This does not declare a plan stage
`done` or change the verify-first mode below.

When model routing applies, `routing.json` records the routing decision, and when
bounded escalation applies, `attempts/NNN.json` records one execution attempt per
try. Both are non-secret diagnostic evidence: neither is read back to drive a
decision, and neither is a second source of task state.

## 6. Verify-First Execution Mode

`execution_mode: verify-first` MUST be explicit plan/task metadata — never inferred
from a task's title or prose. It runs the configured validation **before** any agent:

- a pass MUST invoke no agent (not the micro-planner, `IMPLEMENT`, or `FIX`) and completes locally only because its configured checks passed;
- a failure MUST hand the deterministic failure to the implementation agent as context; the ordinary lifecycle then continues (not automatically terminal);
- a verify-first task with **no** configured check MUST fall back to the ordinary implementation path — a fast path that verifies nothing is not a verification.

Without the mode a task keeps implement-first behaviour. See [VALIDATION.md](VALIDATION.md).

## 6a. Declared-Complete Stages

`execution_mode: done` MUST be explicit plan metadata — never inferred from a task's
`Status:` line, title, or prose. It declares that a stage's work already exists in
the tree, so the plan asks SOP not to implement it again:

- the stage's task MUST be recorded **already satisfied** when the task graph is
  built, so its dependants are unblocked and a plan whose work is already green can
  complete instead of ending `BLOCKED` through the no-change path;
- the task MUST NOT be selected for execution, and SOP MUST NOT implement, verify,
  review, or gate it: the declaration is the record, and there is no evidence SOP
  could obtain for work it did not perform;
- the run MUST report the declared completions separately from work it performed, so
  a completion is never mistaken for work the run did;
- the declaration MUST NOT be settable by a model. A plan normalized by an agent
  MUST have the mode cleared, so only the plan document's own structure can declare a
  stage done (see §3);
- a single `--task` invocation, which has no plan to close, MUST reject the mode
  rather than ignore it.

A stage whose work must be _verified_ rather than accepted is `verify-first` (§6); a
stage with nothing to verify cannot become `done` by implication. A run still requires
a configured agent, exactly as a graph whose tasks are all `verify-first` does; the
agent is simply never invoked for a declared-complete task.

## 6b. Objective Acceptance Checks

The configured validation/review verdicts establish that work was done; they do not
establish that an objective was met. A plan stage MAY declare **operator-owned,
deterministic acceptance checks** — the verifiable half of acceptance — which `sop run`
MUST execute and record as an objective-level verdict, never as a model completion
claim.

**Declaration.** A stage declares its checks with the same sub-section (rendered shape)
or inline-field (task-list shape) keywords used for the other stage fields, one command
per line, each optionally wrapped in one pair of backticks (Markdown code formatting,
which the parser strips — not shell syntax):

- the rendered shape uses an `### Acceptance Checks` sub-section (also
  `### Acceptance Check`);
- the task-list shape uses one or more inline `- **Acceptance check:** <command>`
  lines (also `- **Acceptance checks:**`).

The two shapes are equivalent; a rendered plan round-trips its checks.

```markdown
## S001 — Greeting package

Create the greeting package.

### Deliverables

- `greeting/greeting.go`

### Acceptance Criteria

- the greeting package exists and passes `go vet`

### Acceptance Checks

`go vet ./greeting/`
go test ./greeting/
```

The same stage in the task-list shape:

```markdown
### S001 — Greeting package

- **Objective:** Create the greeting package.
- **Files:** greeting/greeting.go
- **Acceptance:** the greeting package exists and passes `go vet`
- **Acceptance check:** `go vet ./greeting/`
- **Acceptance check:** `go test ./greeting/`
```

**Operator-owned and never model-authored.** A check is a **SAFE argument vector,
never a shell**: `sop run` MUST tokenize it with the same splitter the command policy
uses, which rejects shell syntax (chaining, pipelines, redirection, command
substitution, expansion, and globs), so authorization validates the same structure that
runs. A command the policy does not classify SAFE — or whose form redirects or injects
tooling — MUST NOT execute and MUST be a boundary that HOLDS the verdict. Beyond the
SAFE classification the following forms MUST be rejected before execution, in every
spelling (`-f v`, `-f=v`, `--f v`, `--f=v`): `go`/`build`/`vet` `-exec`, `-toolexec`,
`-vettool`, `-o`, `-overlay`, and `-modfile`; `go fmt` (which rewrites files); and
`git -c` / `--config-env` config injection. A stage's checks MUST come only from the
operator's plan document; a plan normalized by an agent MUST have its checks cleared,
mirroring how a model-declared `done` mode is discarded (§6a), so a model can never
author its own acceptance checks. A free-text `### Acceptance Criteria` entry has no
deterministic determinant: it is recorded as informational when the stage declares
checks, and as unverifiable (never VERIFIED) when it does not.

**Evaluation.** A `go test` check MUST be judged from structured `go test -json`
(test2json) events produced by a **single execution**: it is met only when the process
exits 0 AND the event stream contains at least one test-level pass event (a package-level
pass with no test name does not count). An all-skipped run, a `-run` filter that matched
nothing, a package with no test files, and a `TestMain` that returns without calling
`m.Run` are therefore not met, reported as `no test executed`. `-json` MUST be added when
absent and MUST NOT be duplicated. A non-`go test` SAFE command is met by exit status 0.
Each check MUST execute exactly once.

**Verdict.** The verdict is derived deterministically at run time from the checks, never
from a confidence score or a model claim:

- **VERIFIED** — every required criterion was independently met, including at least one
  executed, passing acceptance check; a deliverable-only or free-text-only stage can
  therefore never be VERIFIED;
- **PARTIAL** — safe work was done but a named required criterion is unmet or
  unverifiable (a failing check, a `go test` that ran no test, or no declared check);
- **HOLD** — a protected/authority boundary was crossed (an unauthorized or rejected
  check), regardless of the other criteria.

The verdict is recorded in `.agent-sdlc/runs/<task>/outcome.json` — per criterion its
status, `Met`, `Command`, `Argv`, `ExitCode`, and a bounded `Output` excerpt — and
rendered in the run report's `## Outcome` section. It is diagnostic evidence only: a
task's `PASS`/`LOCAL_DONE` MUST NOT be equated with verified objective completion, and
the verdict is never read back to drive a decision.

A change to a stage's declared checks is a material task change: reconciliation MUST
detect it, an executed task MUST stop for the existing `--accept-changed` approval, and
a never-executed task MUST be updated (see [RECOVERY.md](RECOVERY.md)).

## 7. Fix Loop Bound

When validation fails, or review leaves blocking findings, `sop run` MUST send the
failure back to the agent with the plan, the deterministic failure (when a check
failed), the findings, and the current diff, then re-run validation and review. The
loop MUST be bounded by `quality.max_fix_cycles`; exhausting the budget MUST yield
`NEEDS_HUMAN` rather than looping forever. See [QUALITY.md](QUALITY.md), [RECOVERY.md](RECOVERY.md).

## 7a. Bounded Model Escalation

When `models.escalation_enabled` (or `SOP_MODEL_ESCALATION_ENABLED`) is on, a task
that fails a quality gate MAY be retried once more on the next larger model class in
the same invocation, before the existing human boundary applies. The escalation
happens outside the lifecycle: each retry is a fresh, bounded lifecycle on the
escalated class, with the previous attempt's bounded context handed forward, and
with the escalated selection resolved, validated, built, and capability-guarded
first. Escalation MUST NOT change state transitions, gates, approval, or autonomy
policy, MUST NOT wrap past `large`, and MUST be bounded by
`models.max_escalations`. It is OFF by default. See [RECOVERY.md](RECOVERY.md) §8 and
[MODEL-ROUTING.md](MODEL-ROUTING.md).

## 8. Controlled Parallelism

Only dependency-independent tasks MAY run concurrently, each in its own Git
worktree so no two tasks share a working directory. Parallelism MUST remain bounded
rather than allowing unrestricted agent spawning, and concurrency MUST be a
conservative, configurable limit (V1 SHOULD default to no more than two active
implementation tasks). **Partial:** the worktree-based runner (`internal/parallel`)
exists and is exercised by the e2e dogfood test, but `sop run` itself drives the
graph one task at a time and `max_parallel_tasks` is not yet a configuration key;
wiring the bounded runner to `sop run` remains future work.

## 9. Human Gate — No Remote Writes

`sop run` MUST stop at the human gate and MUST NOT commit, push, or merge. Local
execution has no remote PR/CI/merge, so a passing lifecycle completes the task at
`LOCAL_DONE` directly and MUST NOT fabricate remote states. The remote path is driven
by the explicit `sop commit`/`sop pr` commands.

## 10. Reuse Before Implementation (policy)

SOP's execution favors existing capabilities, configuration, and native tooling over new
code, tests, abstractions, or orchestration. This section records the nine policies and
their evidence. It adds **no runtime behavior**: existing execution, verification,
autonomy, and lifecycle mechanisms already satisfy it, so no code, plan, or test suite is
required.

1. **Inspect existing capabilities first.** Before implementing a requirement, determine
   whether the functionality already exists. The risk-based verification tier reuses the
   `internal/router` typed signals, prompt compilation reuses CTX-001/CTX-005, response
   handling reuses CTX-006, and between-task compression reuses `internal/handoff`.
2. **Configuration before code.** Behavior that can be enabled or adjusted through
   configuration MUST NOT introduce new implementation. `autonomy.*`, `quality.jev.*`,
   `verification.risk_based`, `models.*`, `orchestration.enabled`, and `decision.*` gate
   optional capabilities; each defaults off or to the conservative value.
3. **Minimal change.** Extend or reuse existing components rather than creating parallel
   systems, frameworks, or abstractions. SOP has one quality gate, one failure classifier,
   one autonomy policy, one performance model, and one validation runner. The worktree
   runner (§8) and the handoff capsule are staged components; the live path does not
   duplicate them.
4. **Tests proportional to change.** Configuration-only changes MUST reuse existing tests;
   focused tests are added only when behavior changes or a genuine coverage gap exists.
5. **Continuous execution.** Eligible tasks MUST be executed, satisfied plans closed, and
   explicitly queued plans activated through existing lifecycle capabilities (`sop run`,
   `verify-first`, `sop task complete --external`, `sop plan complete`/`supersede`,
   `sop reconcile`) without unnecessary interruptions.
6. **Approval by exception.** SOP continues autonomously within established authority and
   stops only for a genuine approval boundary, an unexpected conflict, or an unrecoverable
   failure ([HUMAN-APPROVAL.md](HUMAN-APPROVAL.md), [RECOVERY.md](RECOVERY.md)).
7. **No redundant reconciliation.** Reconcile only when the relevant plan, source, or
   execution state changes; unchanged evidence MUST NOT be re-validated
   ([RECOVERY.md](RECOVERY.md) §7, §12; [VALIDATION.md](VALIDATION.md) §8 reuse rules).
8. **Native toolchain first.** Prefer existing Go tooling, Git, repository conventions, and
   SOP components over additional infrastructure.
9. **Preserve determinism and safety.** No required approval, acceptance criterion, audit
   history, or security boundary MAY be bypassed to reduce overhead
   ([QUALITY.md](QUALITY.md), [HUMAN-APPROVAL.md](HUMAN-APPROVAL.md)).

**Compliance.** A read-only review of the live path found it already reuses the components
above and gates optional capability behind configuration; the deterministic gate, failure
classifier, autonomy policy, and approval surface remain the single source of truth for
each decision. Where helper duplication or staged-but-unwired packages exist, the policy is
to wire or drop them deliberately rather than add a parallel implementation; this section
does not mandate those changes. The policy documents existing behavior and requires no
change to it.
