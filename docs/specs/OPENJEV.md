# JEV Capability Boundary (JEV001)

**Type:** Normative specification

## Purpose

Defines the OpenJEV (JEV) integration contract for SOP: where JEV runs, what it
may read and return, how it interacts with the quality gate, review, and human
approval, and how it fails. JEV provides analysis or bounded judgments only;
**SOP owns policy, lifecycle transitions, execution authority, approval
requirements, and authoritative state.**

## Related Specifications

- [WORKFLOW.md](WORKFLOW.md) — lifecycle stages and transitions.
- [QUALITY.md](QUALITY.md) — the deterministic quality gate JEV feeds.
- [REVIEW.md](REVIEW.md) — the review stage, distinct from JEV.
- [HUMAN-APPROVAL.md](HUMAN-APPROVAL.md) — the human gate JEV must never bypass.
- [../reference/CONFIGURATION.md](../reference/CONFIGURATION.md) — `quality.jev.*` configuration.
- [../reference/JEV-OPERATIONS.md](../reference/JEV-OPERATIONS.md) — enabling JEV, provider configuration, and severity policy.

## Normative Language

The terms MUST, MUST NOT, SHOULD, SHOULD NOT, and MAY are normative.

---

This document is the authoritative specification of the domain boundary between
**SOP** and **JEV**. It defines what JEV may and may not do, and records the
invariant:

```text
JEV analyzes.
SOP decides.
IMPLEMENT/FIX changes code.
```

It satisfies the JEV001 work item in `docs/plans/PLAN-JEV-Implementation.md` ("Define JEV Capability
Boundary") and the JEV decision layer described in `docs/requirements/PRD-JEV.md` §15.

---

## 1. Roles

- **SOP** is the orchestration and lifecycle owner. SOP owns control flow,
  task state, persistence, validation, review, the quality gate, fix-loop
  budgets, retries, and human approval gates.
- **JEV** is an optional, non-owner engineering-analysis capability. SOP may
  invoke JEV to obtain an additional quality signal; JEV has no authority over
  any lifecycle decision.
- **IMPLEMENT / FIX** are the SOP lifecycle stages that change code. JEV is not
  a coding or fixing agent.
- **Model / Provider** is a replaceable reasoning engine behind a provider
  interface. JEV is not synonymous with a model or a provider.

Core rule:

```text
Provider ≠ Agent Harness ≠ Model

SOP = orchestration and execution authority
JEV = engineering analysis and quality signal
Model = replaceable reasoning engine
```

---

## 2. Allowed JEV operations

JEV may:

- inspect relevant repository files,
- inspect changes associated with the current task,
- inspect task acceptance criteria,
- inspect validation/review results,
- identify quality concerns,
- return structured findings.

These map to the read-only operations permitted by the boundary package
(`internal/jev`): `read_file`, `search_files`, `list_files`, `inspect_diff`,
and `inspect_context`.

---

## 3. Forbidden JEV operations

JEV may not:

- transition task state directly,
- mutate SOP persistence,
- mark validation or review successful,
- commit,
- push,
- open PRs,
- merge,
- bypass human gates.

JEV **cannot directly modify persisted task state**. It receives read-only
snapshots and returns structured data; only SOP writes state. The boundary
exposes no method by which a JEV implementation (or a caller of JEV) can change
`state.db`, mark validation `PASS`, mark review `PASS`, open a PR, or complete a
task. JEV must not manufacture `validation PASS`, `review PASS`, `CI PASS`, `PR
opened`, `merge completed`, or `task completed` unless those events actually
occurred through their owning subsystem.

The corresponding deny-list in `internal/jev` is `write_file`, `delete_file`,
`git reset`, `git clean`, `commit`, `push`, and `merge`.

---

## 4. Interface

JEV is reached through a single, small, replaceable interface:

```go
// Analyzer is the boundary through which SOP invokes JEV.
type Analyzer interface {
    Analyze(ctx context.Context, req Request) (Result, error)
}
```

- **Request** carries only the bounded context JEV needs: task, acceptance
  criteria, changed files, repository context, validation result, and review
  result. It does **not** pass ownership of the SOP runtime or persistence
  layer.
- **Result** is structured: a status (`PASS`, `FINDINGS`, `INCOMPLETE`,
  `ERROR`), a list of findings (severity, category, path, line, message,
  evidence), and a summary.
- **Statuses and severities are result values, not lifecycle transitions.**
  SOP policy determines what a finding implies for a task.
- Implementations are selected **outside** core lifecycle code, so JEV is
  replaceable and optional. `internal/jev.FakeAnalyzer` is a deterministic test
  double that performs no model or network access.

Malformed results **fail closed**: `Result.Validate` rejects an unknown status,
an unknown severity, a `PASS` that carries findings, or `FINDINGS` with none.
An invalid result is never treated as a pass.

---

## 5. SOP remains the lifecycle owner

The JEV interface is **additive**. It does not replace or reorder the existing
lifecycle:

```text
IMPLEMENT / FIX -> VALIDATE -> REVIEW -> (optional JEV) -> QUALITY DECISION
```

The existing quality gate in `internal/quality` remains the lifecycle decision
owner. JEV001 does not wire JEV into the quality lifecycle; it only proves the
boundary leaves the existing lifecycle authoritative and untouched. No change to
existing lifecycle transitions is required by JEV001.

---

## 6. Existing execution providers remain usable without JEV

JEV is disabled/absent by default. When JEV is not configured, the normal SOP
lifecycle runs unchanged. JEV is optional behind an interface, so removing or
not configuring it does not break any execution path — IMPLEMENT, FIX, VALIDATE,
and REVIEW continue to run without any JEV dependency being satisfied, and
IMPLEMENT/FIX tool policies are unchanged.

---

## 7. Architecture

```text
                   +-------------------+
                   |      sop run      |
                   +---------+---------+
                             |
                             v
                    +--------+--------+
                    | IMPLEMENT / FIX |
                    +--------+--------+
                             |
                             v
                    +--------+--------+
                    |   VALIDATION    |
                    +--------+--------+
                             |
                             v
                    +--------+--------+
                    |      REVIEW     |
                    +--------+--------+
                             |
                +------------+-------------+
                | JEV enabled for quality? |
                +------+-------------------+
                       |
              no       |       yes
              |        |        |
              v        |        v
         existing      |   +----+----+
         SOP flow      |   |   JEV   |   (read-only analysis)
                       |   +----+----+
                       |        |
                       |   structured result
                       |        |
                       +--------v
                        SOP quality policy
                             |
                    +--------+--------+
                    |                 |
                   PASS             FAIL
                    |                 |
                    v                 v
               LOCAL_DONE        FIX / STOP
```

SOP owns policy and control flow; JEV provides bounded analysis only. JEV does
not perform scheduling, retries, code modification, commit, PR, or merge
behavior.

---

## 8. Terminology glossary

- **SOP** — the orchestration and execution authority; the lifecycle owner.
- **JEV** — an optional engineering-analysis capability that returns structured
  quality findings; it holds no lifecycle authority.
- **IMPLEMENT / FIX** — SOP lifecycle stages that change code.
- **VALIDATE** — the deterministic validation stage.
- **REVIEW** — the independent review stage.
- **quality gate** — the deterministic policy in `internal/quality` that maps
  evidence to a lifecycle decision.
- **human approval** — the human gate that remains authoritative for
  consequential actions.
- **lifecycle** — SOP's orchestration: state transitions, retries, fix-loop
  budgets, and completion.

---

## 9. Acceptance-criteria traceability

| JEV001 acceptance criterion                                  | Satisfied by                                                                                                        |
| ------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------- |
| JEV has a clearly defined interface.                         | §4 — `internal/jev.Analyzer` (`Analyze(ctx, Request) (Result, error)`), `Request`, `Result`, and `FakeAnalyzer`.    |
| SOP remains the lifecycle owner.                             | §1, §5 — SOP owns control flow and the quality gate in `internal/quality`.                                          |
| JEV cannot directly modify persisted task state.             | §3, §4 — request carries read-only snapshots; the interface exposes no state-mutation path and results fail closed. |
| Existing execution providers remain usable without JEV.      | §6 — JEV is optional behind a replaceable interface with no lifecycle dependency.                                   |
| Architecture is documented in code or package documentation. | This document plus the package doc comment in `internal/jev/jev.go` §7 diagram.                                     |

---

## 10. Scope

This boundary is JEV001 only. It does **not** implement scheduling, retries,
code modification, commits, PR creation, merges, quality-lifecycle wiring, or any
behavior beyond defining and enforcing the boundary. Those are separate JEV work
items (`docs/plans/PLAN-JEV-Implementation.md`).

---

## 11. Configuration

JEV is configured in `.agent-sdlc/config.yaml` under `quality.jev` (see
[../reference/CONFIGURATION.md](../reference/CONFIGURATION.md)):

- `quality.jev.enabled` — the feature flag. **Disabled by default**; enabling
  requires explicit configuration, and JEV MUST NOT be enabled by inference.
- `quality.jev.mode` — the execution form; only `review` is supported today. An
  unknown mode is a focused load-time error, never a silent fallback.
- `quality.jev.fail_on` — the finding severities that block; when unset it
  defaults to `quality.fail_on`.

The `quality.jev` block is optional, so a project without it keeps working
unchanged.

## 12. Where JEV runs

In the current pipeline JEV is an optional, read-only analysis stage that runs
**after** validation and review and feeds the same quality gate and fix loop:

```text
IMPLEMENT / FIX -> VALIDATE -> REVIEW -> (optional JEV) -> QUALITY DECISION
```

When `quality.jev.enabled` is false the stage is absent and the normal SOP
lifecycle runs unchanged.

The early-JEV decision layer (§18) adds two optional, disabled-by-default
checkpoints that run the same read-only analyzer **before** implementation:

```text
TASK SELECTED -> (optional JEV TASK TRIAGE) -> SOP POLICY
PRECHECK -> (optional JEV PRE-EXECUTION) -> SOP POLICY -> IMPLEMENT / FIX -> ...
```

Both are gated by the separate `early_jev` namespace and default OFF, so the
pipeline above is unchanged unless they are explicitly enabled. See §18.

## 13. Inputs and outputs

- **Inputs** — bounded, read-only snapshots only: the task, its acceptance
  criteria, the changed files and repository context, the validation result, and
  the review result. JEV MUST NOT receive ownership of the SOP runtime or
  persistence layer.
- **Outputs** — a structured result: a status (`PASS`, `FINDINGS`, `INCOMPLETE`,
  `ERROR`), findings (severity, category, path, line, message, evidence), and a
  summary. Statuses and severities are result values, not lifecycle transitions.
  An early checkpoint additionally returns structured evidence that names its
  purpose (`QUALITY`, `TASK_TRIAGE`, or `PRE_EXECUTION`, §18) and carries typed
  items (purpose, typed category, severity, detail, evidence).
- **Confidence** — bounded (`0.0 <= c <= 1.0`) advisory metadata on structured
  evidence. It MUST NOT directly control lifecycle state; SOP owns the thresholds
  that interpret it (§18).

## 14. Findings, severity, and the quality gate

- JEV findings carry severities. A finding whose severity is named in
  `quality.jev.fail_on` (defaulting to `quality.fail_on`) is blocking for the JEV
  result.
- SOP MUST reduce the JEV result into the deterministic quality gate
  ([QUALITY.md](QUALITY.md)). JEV MUST NOT decide PASS/FAIL, mark validation or
  review successful, or cause a task to pass on its own.

## 15. Failure and fallback behavior

JEV has two failure-shaped outcomes that MUST NOT be conflated:

- **An analysis result** — a structured `Result` with status `PASS`, `FINDINGS`,
  `INCOMPLETE`, or `ERROR`. Findings are evidence; `INCOMPLETE`/`ERROR` are not a
  pass.
- **An infrastructure/provider failure** — a timeout, an unavailable provider, a
  transport failure, or a malformed/invalid response. It is a failure to analyze,
  never a finding.

Rules:

- Malformed JEV results MUST fail closed (§4): an unknown status, an unknown
  severity, a `PASS` carrying findings, or `FINDINGS` with none is rejected and
  never treated as a pass.
- An infrastructure/provider failure MUST NOT be interpreted as a finding and MUST
  NOT be treated as a pass.
- When JEV is disabled, absent, unavailable, times out, or returns
  `ERROR`/`INCOMPLETE`, SOP MUST fall back to deterministic policy; JEV MUST NOT
  silently fail open for a high-risk decision.
- JEV is optional behind a replaceable interface, so removing or not configuring
  it MUST NOT break any execution path (§6).

### Who blocks

JEV itself never blocks workflow: it holds no authority to transition state, reject
a task, or hold up a merge. What follows from JEV evidence is decided by SOP:

- **Configured SOP policy may block or escalate** because of JEV evidence. That
  decision is deterministic and lives in SOP — the quality gate for quality
  evidence ([QUALITY.md](QUALITY.md)), and autonomy/policy elsewhere — never in
  JEV.
- **Advisory JEV failure does not automatically block normal work.** An optional
  early/advisory JEV failure MAY be recorded and the workflow MAY continue under
  deterministic policy.
- **High-risk policy may fail closed or require human authorization.** A policy
  governing a consequential decision MAY treat a JEV failure as non-clearing and/or
  require a human; the policy owns that choice.

Recording a JEV failure does not by itself change a verdict; SOP policy decides
whether that failure is blocking for the decision at hand. Optional capabilities
are additive and MUST NOT gate core work
([../architecture/SOP-BOUNDARY.md](../architecture/SOP-BOUNDARY.md)).

## 16. Relationship to review and human approval

- JEV and review are separate stages with separate owners. Review findings and
  JEV findings MUST be presented distinctly; see [REVIEW.md](REVIEW.md).
- JEV MUST NOT bypass or satisfy a human-approval gate; consequential actions
  remain governed by SOP policy and the human gate. See
  [HUMAN-APPROVAL.md](HUMAN-APPROVAL.md).

## 17. Proposed / Future (Not Implemented)

The following are **not implemented** and are recorded as candidates only. They
MUST NOT be presented as current behavior.

- **JEV decision layer** — using JEV as an optional decision provider for bounded
  judgments (task-complexity classification, model routing, review escalation,
  finding prioritization, risk classification, whether another review pass is
  warranted). It would be gated by the `decision` configuration and disabled by
  default (`features.jev_decisions: false`), per
  [../requirements/PRD-JEV.md](../requirements/PRD-JEV.md).
- **Additional JEV modes** — execution forms other than `review`.

## 18. Early checkpoints (Phase 3, implemented)

Two optional, disabled-by-default checkpoints run the same read-only analyzer
before implementation. They are owned by the separate `early_jev` configuration
namespace ([../reference/CONFIGURATION.md](../reference/CONFIGURATION.md)).

- **Task triage** (`early_jev.gates.task_triage`) runs after SOP deterministically
  selects a runnable task and before implementation.
- **Pre-execution** (`early_jev.gates.pre_execution`) runs after the precheck and
  immediately before implementation. It is analysis only: it MUST NOT execute or
  mutate anything.

Both preserve the boundary: JEV produces structured evidence; SOP policy
(`internal/autonomy`) decides; IMPLEMENT/FIX changes code. The three analysis
purposes (quality, task triage, pre-execution) are distinct so evidence is never
conflated across checkpoints. Early findings carry a typed category (ambiguity,
missing context, scope, requirement conflict, dependency concern, security,
destructive, credential sensitivity, unexpected area, approval-sensitive) and a
severity; SOP maps that typed pair against `early_jev.fail_on` and MUST NOT infer a
lifecycle action from free-form summary text. An unclassified category fails closed
to a human boundary.

Disabling or omitting the gates is a strict no-op: existing SOP behavior is
unchanged (§6, FR-P3-6). A provider failure or malformed result is recorded as a
failure with no evidence payload, never as a finding; under the advisory default it
does not block normal work, while a policy naming the relevant severities may fail
closed or require human authorization (§15). Early results are persisted as
diagnostic run artifacts (`early-jev.json`, `early-jev-history.jsonl`) and are
never read back to drive a decision.
