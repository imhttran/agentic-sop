# JEV Troubleshooting and Reference

**Type:** Descriptive guide

Diagnosing JEV and its failure behavior, plus the terminology glossary and the
acceptance-criteria traceability for the JEV015 work. The operating guide
(enabling, provider configuration, severity policy, lifecycle placement, and
safety boundaries) is [JEV-OPERATIONS.md](JEV-OPERATIONS.md); the authoritative
boundary is [../specs/OPENJEV.md](../specs/OPENJEV.md).

---

## Troubleshooting and failure behavior

### Failure behavior

JEV **fails closed**. A failure is never treated as a pass.

| Situation                       | Behavior                                                                                                                                             |
| ------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Timeout**                     | The bounded call is abandoned; the run does not proceed as `PASS`. Falls back to deterministic policy or human review.                               |
| **Malformed output**            | Output that is not a valid JEV result is rejected; it fails closed and never becomes `PASS`.                                                         |
| **Low confidence / incomplete** | An incomplete or low-confidence analysis is reported as incomplete; it is not treated as a pass. Falls back to deterministic policy or human review. |
| **Unavailability**              | An unreachable provider yields a focused error; the run does not silently pass.                                                                      |
| **Schema violation**            | A result that violates the schema (unknown status, unknown severity, `PASS` carrying findings, `FINDINGS` without any) is rejected.                  |

For all of these, SOP **falls back to deterministic policy or human review** and
**never silently fails open for high-risk decisions**. A provider error or
malformed output **never becomes `PASS`**.

### Did JEV execute?

You can tell whether JEV executed without reading the implementation:

- **In the run report.** When JEV runs, the report shows a JEV line
  (for example `JEV: PASS` with a blocking-finding count). When JEV is disabled,
  the report has no JEV line at all — that absence is the
  disabled signal, not a failure.
- **In the run artifacts.** A JEV run writes its diagnostic artifacts under the
  run directory; if no JEV artifact exists, JEV did not run.

### Blocking versus non-blocking findings

- **Blocking findings** (`HIGH`/`CRITICAL` by default) are visible in the report
  with their severity and source location, and they drive the fix loop.
- **Non-blocking findings** (`INFO`/`LOW`/`MEDIUM` by default) are accessible in
  the report and artifacts without blocking the run.

### Artifact and report locations

Following repository conventions, JEV writes its diagnostic result alongside the
other run diagnostics:

```text
.agent-sdlc/runs/<TASK>/jev.json
```

The run directory also holds the other stage artifacts (`validation.json`,
`review.json`, `report.md`, `report.json`, …), so JEV results are distinguishable
from validation and review evidence. `sop report` prints the concise run summary
that includes the JEV result when JEV executed.

---

## Terminology glossary

- **SOP** — the orchestration and execution authority; the lifecycle owner.
- **JEV** — an optional engineering-analysis capability that returns structured
  quality findings; it holds no lifecycle authority.
- **IMPLEMENT / FIX** — SOP lifecycle stages that change code. JEV is not a
  coding or fixing agent.
- **VALIDATE** — the deterministic validation stage.
- **REVIEW** — the independent review stage.
- **quality gate** — the deterministic policy (in `internal/quality`) that maps
  evidence to a lifecycle decision (`PASS` / `FAIL` / `NEEDS_HUMAN`).
- **human approval** — the human gate that remains authoritative for
  consequential actions.
- **lifecycle** — SOP's orchestration: state transitions, retries, fix-loop
  budgets, and completion.
- **Provider / Harness / Model** — a provider supplies a model endpoint; a
  harness turns a model into a coding agent; the model is the replaceable
  reasoning engine. `Provider ≠ Agent Harness ≠ Model`.

This glossary is consistent with the glossary in
[`docs/specs/OPENJEV.md`](../specs/OPENJEV.md), the PRD, and the JEV implementation
plan.

---

## Acceptance-criteria traceability (JEV015)

| JEV015 acceptance criterion                             | Where it is satisfied                                                                                                                                  |
| ------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------ |
| New user can enable JEV without reading implementation. | [Quick path](#quick-path-from-default-disabled-to-jev-enabled) and [Enabling and disabling JEV](#enabling-and-disabling-jev).                          |
| Default disabled behavior clear.                        | [Enabling and disabling JEV](#enabling-and-disabling-jev) — disabled by default, no noise, existing configs compatible.                                |
| Failure behavior documented.                            | [Troubleshooting and failure behavior](#troubleshooting-and-failure-behavior) — fail-closed table, never becomes `PASS`.                               |
| Provider configuration documented.                      | [Provider behavior and configuration](#provider-behavior-and-configuration) — reused abstraction, configuration-driven model, focused errors.          |
| Safety boundaries documented.                           | [Safety boundaries and the core rule](#safety-boundaries-and-the-core-rule) — read-only, only SOP writes state, safety invariants, core rule verbatim. |

Auxiliary documentation of architecture, severity policy, lifecycle placement,
and terminology is provided above and is consistent with
[`docs/specs/OPENJEV.md`](../specs/OPENJEV.md) and
[`docs/plans/PLAN-JEV-Implementation.md`](../plans/PLAN-JEV-Implementation.md).
