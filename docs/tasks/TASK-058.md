# T058 --- Capability-Aware Loop Bounds for the Ollama Agent

> The bootstrap Ollama agent used one global 48-iteration budget for every
> capability. It is now bounded per capability, denies tools a capability may not
> use, and stops early with a diagnostic when the model repeats non-progressing
> turns instead of consuming the whole budget.

## Status

DONE

## Objective

An IMPLEMENT task failed in the bootstrap harness with:

```text
iteration limit reached (48); the model did not finish
```

Every capability shared one budget (`defaultMaxIterations = 48`), so a PLAN run
received as many turns as an IMPLEMENT, and a model stuck repeating the same
action burned all 48 turns before failing. The report could not show what happened
during those turns.

Target lifecycle:

```text
SOP request
    ↓
capability → CapabilityPolicy (budget + tool set)
    ↓
bounded loop with no-progress detection
    ↓
FINAL, or an early diagnostic termination
```

```text
PLAN           read-only     max  8
DESIGN_TESTS   read-only     max 12
DIAGNOSE       read-only     max 12
REVIEW         read-only     max 12
IMPLEMENT      mutation      max 24
FIX            mutation      max 24
```

## Scope

- `internal/ollamaagent/policy.go` (new): `CapabilityPolicy{MaxIterations,
  ReadOnly, AllowedTools}` and `PolicyFor(capability)` as the single authority for
  a capability's budget and tool set. Unknown capabilities default to read-only.
  An unknown *tool* is allowed through so the shared harness still reports it as
  unsupported.
- `internal/ollamaagent/harness.go`: the loop runs `policy.MaxIterations` turns;
  a tool outside the policy is refused (reported to the model, recorded as a
  denial) rather than executed; no-progress detection stops a repeating model
  early; errors and the trace name the capability, termination reason, model, and
  last/repeated action.
- `internal/ollamaagent/trace.go` (new): a bounded, secret-free per-turn trace
  (capability, iteration, tool, content-free request, progress, recovery,
  termination). Written to stderr on failure and, when `SOP_AGENT_TRACE_LOG` is
  set, appended durably as JSON lines.
- `internal/ollamaagent/config.go`: remove `defaultMaxIterations = 48` and
  `Config.MaxIterations`; add the trace bound.
- `internal/ollamaagent/prompt.go`: advertise only the policy's tools, and give
  each capability explicit termination instructions — IMPLEMENT/FIX are told SOP
  performs independent validation and review after they return; PLAN is told it is
  PLAN, not IMPLEMENT.
- `internal/toolharness/harness.go`: export `SummarizeRequest` and add
  `RecordDenied` so a capability-level refusal is audited like any other denial.
- README and tests.

This commit also lands the uncommitted bootstrap rename (`deepseek*` →
`ollama*`, package `ollamaagent`, shared `internal/toolharness`) and the earlier
tool-call fixes (native tool calls, multiline arguments, narration nudging).

## Rules

- **No global budget.** `defaultMaxIterations` no longer governs every capability;
  the per-capability policy does. A capability that exhausts its budget is a
  diagnostic, not a silent continuation.
- **One place for capability checks.** Budgets and tool sets live only in
  `PolicyFor`; the loop does not scatter `switch capability` logic.
- **Progress is material, not nominal.** The no-progress fingerprint includes the
  tool result, so a repeated action whose result changed (a file edited, a test now
  passing) is progress. Three identical consecutive turns get one recovery
  instruction; a further repetition stops the run.
- **SOP stays authoritative.** This changes only the implementation adapter's loop;
  validation, review, retries, quality gates, and human approval are untouched, and
  the harness never touches SOP state.
- **Diagnostics are safe.** The trace carries no prompts, file contents, or
  secrets.

## Tests

Deterministic, against a fake Ollama server (`internal/ollamaagent`): the
capability budgets; PLAN/REVIEW read-only tool restrictions; PLAN normal
completion; a PLAN mutation attempt is denied with the repository unchanged; an
IMPLEMENT read/write/run completion; PLAN stops at 8 and IMPLEMENT at 24 (not 48)
with `termination=iteration_limit`; a repeated tool action is stopped early after
a recovery instruction; progress after a single repetition continues; and the
trace records turns without leaking file content.

## Acceptance Criteria

- [x] `48` is no longer the effective budget for every capability.
- [x] PLAN gets 8 read-only turns; IMPLEMENT/FIX get 24 mutation turns.
- [x] A repeated non-progressing action is stopped early with a diagnostic.
- [x] Termination reasons and a safe per-turn trace are reported.
- [x] Existing `internal/ollamaagent` and command-provider tests still pass.
- [x] `gofmt -l .`, `go vet ./...`, `go test ./...`, `go test -race ./...`, and
      `go build ./...` pass.
- [x] No SOP task/run state was modified.

## Git

Branch: `main`
Commit: `task(T058): make the ollama agent capability-aware`

## Out of Scope

Agent Harness V2; SOP lifecycle, validation, review, or gate changes; the plan
parser; adding another agent; changing Ollama/DeepSeek behavior; committing or
pushing the plan documents (`docs/PLAN-Agent-Harness-V2.md`,
`docs/PLAN-SOP-Performance.md`); deleting or resetting SOP state.
