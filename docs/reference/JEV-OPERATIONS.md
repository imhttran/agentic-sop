# JEV Operations Guide (JEV015)

This is the operator-facing guide for **JEV**, the optional
engineering-analysis capability inside `agentic-sop`. It documents how to
enable and disable JEV, its architecture and role separation, provider
configuration, severity policy, lifecycle placement, troubleshooting and
failure behavior, and safety boundaries.

It is documentation only. It does not implement or change any JEV behavior.
The authoritative boundary specification is
[`docs/specs/OPENJEV.md`](../specs/OPENJEV.md); the implementation plan is
[`docs/plans/PLAN-JEV-Implementation.md`](../plans/PLAN-JEV-Implementation.md); the original
requirements are in [`docs/requirements/PRD-JEV.md`](../requirements/PRD-JEV.md).

> **Core rule**
>
> ```text
> JEV analyzes.
> SOP decides.
> IMPLEMENT/FIX changes code.
> ```

JEV provides analysis. SOP retains execution authority. JEV is read-only in V1.

---

## Quick path: from default-disabled to JEV enabled

A new user can enable JEV without reading the implementation.

1. **JEV is disabled by default.** If you do nothing, nothing changes:
   `sop run` behaves exactly as it does today, and no JEV stage runs and no JEV
   report line appears.
2. **Enable it explicitly** in `.agent-sdlc/config.yaml` (created by
   `sop init`):

   ```yaml
   quality:
     jev:
       enabled: true
   ```

   See [Enabling and disabling JEV](#enabling-and-disabling-jev) for a full
   example, including the provider settings JEV uses.

3. **Run as usual:**

   ```bash
   sop run docs/PLAN.md
   ```

   When JEV is enabled it runs after validation and review, and its result
   appears in the run report and the run artifacts under
   `.agent-sdlc/runs/<TASK>/`.

4. **Disable it** by setting `enabled: false` (or removing the block). No state
   deletion or recovery is required — see
   [Enabling and disabling JEV](#enabling-and-disabling-jev).

---

## Architecture and role separation

JEV sits behind a small, replaceable analyzer boundary. SOP decides when (and
whether) to invoke it and what its result means.

The roles, in the repository's own terms:

- **Provider ≠ Agent Harness ≠ Model.** A provider supplies a language model
  endpoint; a harness turns a model into a coding agent; the model is the
  reasoning engine.
- **SOP** is the orchestration and execution authority: it owns control flow,
  task state, persistence, validation, review, the quality gate, fix-loop
  budgets, retries, and human-approval gates.
- **JEV** is an optional engineering-analysis and quality signal. It holds no
  lifecycle authority.
- **IMPLEMENT / FIX** are the SOP stages that change code.
- **Model** is a replaceable reasoning engine selected by configuration.

The lifecycle placement is:

```text
IMPLEMENT / FIX -> VALIDATION -> REVIEW -> (optional JEV) -> SOP quality policy -> PASS / FAIL
```

JEV is reached through a single interface:

```go
type Analyzer interface {
    Analyze(ctx context.Context, req Request) (Result, error)
}
```

- JEV receives bounded, read-only context (task, acceptance criteria, changed
  files, repository context, validation result, review result).
- JEV returns a structured `Result` (status, findings, summary).
- JEV is **optional**: when it is disabled or absent, the normal lifecycle runs
  unchanged.

This document does not restate the boundary inconsistently; the authoritative
role model, allowed/forbidden operations, and interface are specified in
[`docs/specs/OPENJEV.md`](../specs/OPENJEV.md) and the Architecture section of
[`docs/plans/PLAN-JEV-Implementation.md`](../plans/PLAN-JEV-Implementation.md).

**SOP keeps execution authority. JEV only provides analysis.**

---

## Enabling and disabling JEV

### Default: disabled

JEV is **disabled by default**. The generated `.agent-sdlc/config.yaml`
template does not enable it, and a configuration that omits JEV entirely is
valid. When JEV is disabled, no JEV stage runs, no JEV artifact is written, and
no JEV line is added to the report — disabled JEV adds no unnecessary noise.

### Disabled configuration (the default)

You may omit the block entirely, or state it explicitly for clarity:

```yaml
quality:
  jev:
    enabled: false
```

Both mean the same thing: JEV does not run.

### Enabled configuration

Enabling requires explicit configuration:

```yaml
quality:
  jev:
    enabled: true
```

There is no silent default change: JEV does not begin running because a model
or provider happens to be available. It runs only when `quality.jev.enabled`
is `true`.

### Compatibility and safety

- **Existing configs remain compatible.** A configuration written before JEV
  existed is valid and keeps its current behavior; the JEV block is additive and
  optional.
- **Invalid JEV configuration produces a focused error.** A malformed value
  (e.g. a non-boolean `enabled`) fails with a clear message rather than a silent
  fallback.
- **Enabling or disabling JEV requires no state deletion or recreation.** You
  never need to remove `.agent-sdlc/state.db` or perform any destructive
  recovery; toggling `quality.jev.enabled` is the whole change.
- **Disabled JEV adds no unnecessary noise.** The report stays as it was.

---

## Provider behavior and configuration

JEV obtains its reasoning engine by **reusing the existing agent/provider
abstraction**. It does not create a second provider stack.

```text
JEV capability
      |
JEV Analyzer interface
      |
existing agent/provider abstraction
      |
Ollama (local or cloud)
      |
replaceable model
```

### Model selection is configuration-driven

Model selection is configuration-driven and requires **no hardcoded model**. The
Ollama-backed analyzer reuses the existing Ollama provider path and takes its
settings from the shared `SOP_OLLAMA_*` environment variables, falling back to
built-in defaults when they are unset:

```bash
export SOP_OLLAMA_BASE_URL=http://localhost:11434   # default http://127.0.0.1:11434
export SOP_OLLAMA_MODEL=deepseek-v4.1-flash:cloud    # default deepseek-v4.1-flash:cloud
export SOP_OLLAMA_TIMEOUT=2m                          # a Go duration; default 2m
```

Because the model is selected by configuration, **JEV is not synonymous with
one model**. Changing the model (or the provider) does not require changing JEV
or SOP code.

### Configuration requirements

The Ollama-backed JEV analyzer is configured through the shared `SOP_OLLAMA_*`
variables. Each has a built-in default, so a project already configured for
Ollama needs no extra setup:

- a base URL (`SOP_OLLAMA_BASE_URL`, default `http://127.0.0.1:11434`),
- a model (`SOP_OLLAMA_MODEL`, default `deepseek-v4.1-flash:cloud`), and
- a positive timeout (`SOP_OLLAMA_TIMEOUT`, a Go duration such as `2m`, default `2m`).

A malformed or non-positive timeout (for example `SOP_OLLAMA_TIMEOUT=soon` or
`0s`) produces a **focused configuration error** at load time, not a silent
fallback.

### Provider failure behavior

Provider failures are surfaced as **focused errors**, classified by kind, and
they never imply a pass:

| Failure kind           | Meaning                                                          |
| ---------------------- | ---------------------------------------------------------------- |
| `PROVIDER_UNREACHABLE` | The provider could not be reached or returned an error.          |
| `CONFIG_INVALID`       | Base URL, model, timeout, or provider wiring is missing/invalid. |
| `TIMEOUT`              | The bounded call exceeded its deadline.                          |
| `MALFORMED_OUTPUT`     | Model output was not a valid JEV result.                         |

Execution is **bounded**: every analysis runs under a configured timeout, the
prompt length is bounded, and the output is bounded before parsing. A failure
never becomes `PASS`.

### Tests require no live Ollama and no network

Automated tests for JEV use deterministic doubles and require **no live Ollama
server and no network access**. Lifecycle tests never depend on a real model.

---

## Severity policy

JEV findings carry a severity; SOP policy decides what the severity means for
the task.

The severity scale is:

```text
INFO
LOW
MEDIUM
HIGH
CRITICAL
```

The initial JEV quality policy is:

| Severity   | Effect                |
| ---------- | --------------------- |
| `INFO`     | Report                |
| `LOW`      | Report                |
| `MEDIUM`   | Report                |
| `HIGH`     | Fail the quality gate |
| `CRITICAL` | Fail the quality gate |

- Lower severities (`INFO`/`LOW`/`MEDIUM`) **report without blocking by
  default**.
- `HIGH`/`CRITICAL` **can fail quality** and are blocking by default.

### Quality failures use the existing FIX lifecycle

A JEV quality failure enters the **existing FIX lifecycle**; it does not create
a JEV-specific retry machine. The existing fix-cycle budget remains
authoritative — the same `quality.max_fix_cycles` used by the rest of the
lifecycle. There is **no independent JEV retry counter** and no infinite loop.

### Where severities relate to existing quality configuration

Severities interact with the existing quality configuration, in particular the
`quality.fail_on` list:

```yaml
quality:
  max_fix_cycles: 3
  fail_on:
    - critical
    - high
```

`fail_on` names the severities that block a pass; it is the same mechanism used
by review findings, so JEV severities are interpreted by the existing gate
rather than a parallel policy.

---

## Lifecycle placement and the findings-to-FIX flow

### Where JEV sits

```text
IMPLEMENT / FIX -> VALIDATE -> REVIEW -> (optional JEV) -> QUALITY DECISION
```

- JEV runs **only when enabled**.
- JEV does **not replace validation or review**; both run first and their
  evidence is provided to JEV as read-only context.
- The **existing lifecycle transitions remain authoritative**, and disabled
  behavior remains compatible (see
  [Enabling and disabling JEV](#enabling-and-disabling-jev)).
- The **SOP quality gate makes the decision** (`PASS` / `FAIL` / `NEEDS_HUMAN`);
  JEV statuses and severities are inputs, not transitions.

### What happens to JEV findings

- Actionable findings are expressed as **severity, file, line, finding, and
  evidence**.
- Blocking findings are fed into the **existing FIX capability**.
- **FIX remains responsible for mutation.** JEV remains read-only; it never
  changes code.
- Blocking findings respect the **existing fix-cycle limits**
  (`quality.max_fix_cycles`). After a fix, validation, review, and JEV run again;
  exhausting the budget yields `NEEDS_HUMAN` rather than looping.
- Non-blocking findings do not enter FIX by default; they are reported.

### JEV artifacts are diagnostic evidence

JEV results are persisted as **diagnostic run artifacts distinct from
validation and review evidence**. They are not workflow state: SOP's persisted
task state remains authoritative, and JEV artifacts can be ignored or removed
without changing a decision. See
[Troubleshooting](#troubleshooting-and-failure-behavior) for artifact locations.

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

## Safety boundaries and the core rule

### JEV may

- inspect relevant repository files,
- inspect changes associated with the current task,
- inspect task acceptance criteria,
- inspect validation/review results,
- identify quality concerns,
- return structured findings.

### JEV may not

JEV is read-only in V1 and **cannot mutate persisted task state**. It must not:

- transition task state directly,
- mutate SOP persistence,
- mark validation or review successful,
- commit, push, open PRs, or merge,
- bypass human gates.

JEV receives read-only snapshots and returns structured data; **only SOP writes
state**. JEV must not manufacture `validation PASS`, `review PASS`, `CI PASS`,
`PR opened`, `merge completed`, or `task completed` unless those events actually
occurred through their owning subsystem.

These limits are consistent with
[`docs/specs/OPENJEV.md`](../specs/OPENJEV.md) and introduce no new behavior.

### Safety invariants

JEV does not relax any existing safety invariant. In particular, enabling JEV
must never require:

```bash
rm .agent-sdlc/state.db   # never required
git reset --hard          # never required
git clean                 # never required
```

SOP preserves working-tree changes, task/run history, PLAN provenance,
validation/review evidence, and human approval. Human approval remains
authoritative for consequential actions.

### The core rule

```text
JEV analyzes.
SOP decides.
IMPLEMENT/FIX changes code.
```

JEV provides analysis; SOP owns policy and control flow; IMPLEMENT/FIX is the
only part of the lifecycle that changes code.

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
