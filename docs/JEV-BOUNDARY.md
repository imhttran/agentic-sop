# JEV Capability Boundary (JEV001)

This document is the authoritative specification of the domain boundary between
**SOP** and **JEV**. It defines what JEV may and may not do, and records the
invariant:

```text
JEV analyzes.
SOP decides.
IMPLEMENT/FIX changes code.
```

It satisfies the JEV001 work item in `docs/PLAN-JEV.md` ("Define JEV Capability
Boundary") and the JEV decision layer described in `docs/PRD-JEV.md` §15.

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

| JEV001 acceptance criterion | Satisfied by |
| --- | --- |
| JEV has a clearly defined interface. | §4 — `internal/jev.Analyzer` (`Analyze(ctx, Request) (Result, error)`), `Request`, `Result`, and `FakeAnalyzer`. |
| SOP remains the lifecycle owner. | §1, §5 — SOP owns control flow and the quality gate in `internal/quality`. |
| JEV cannot directly modify persisted task state. | §3, §4 — request carries read-only snapshots; the interface exposes no state-mutation path and results fail closed. |
| Existing execution providers remain usable without JEV. | §6 — JEV is optional behind a replaceable interface with no lifecycle dependency. |
| Architecture is documented in code or package documentation. | This document plus the package doc comment in `internal/jev/jev.go` §7 diagram. |

---

## 10. Scope

This boundary is JEV001 only. It does **not** implement scheduling, retries,
code modification, commits, PR creation, merges, quality-lifecycle wiring, or any
behavior beyond defining and enforcing the boundary. Those are separate JEV work
items (`docs/PLAN-JEV.md`).
