# T047 --- Structured Command-Agent Outcomes

> Provider-neutral hardening of the command-agent contract, from a `sop run`
> dogfood failure where an agent asked for permission and SOP reported "no
> changes were produced".

## Status

DONE

## Problem

During IMPLEMENT, a command agent returned prose requesting permission:

```text
I need permission to run the build, test, and vet commands...
Would you like me to proceed?
```

SOP saw an empty diff and reported `FAIL — no changes were produced`. The real
outcome was a human boundary, not an empty implementation.

## Objective

Let a command agent state its execution outcome structurally so SOP can
distinguish: successful implementation; successful work with no repository
changes; human intervention required; provider failure; and a claimed change that
produced none. SOP stays the deterministic authority and never infers the outcome
from prose.

## Dependencies

- T024/T026 (command provider), T041–T044 (run lifecycle)

## Scope

- `internal/agent/agent.go`: `OutcomeStatus` (`completed` | `needs_human` |
  `failed`), `Outcome{Status, Summary, Reason, ChangesExpected}`, and
  `Response.Outcome`.
- `internal/agent/command.go`: `parseOutcome` — recognizes a JSON object with a
  known `status`; everything else (prose, non-JSON, unknown status) is not an
  outcome, so the change is backward compatible. `changes_expected` defaults to
  true for `completed` and is meaningless otherwise.
- `internal/cli/run.go`: IMPLEMENT/FIX honour the outcome for the run lifecycle,
  plus `outcomeResult`. A `needs_human` outcome yields NEEDS_HUMAN; `failed`
  yields FAIL; each stops the run.
- Tests and docs.

## Behaviour

```text
completed + changes_expected=true  + non-empty diff  → validate → review → gate
completed + changes_expected=true  + empty diff       → FAIL (claimed changes, produced none)
completed + changes_expected=false + empty diff       → PASS (no changes required)
needs_human                                            → NEEDS_HUMAN (stop)
failed                                                 → FAIL
legacy prose / no status                               → old behaviour (diff judged as before)
```

## Rules

- Provider-neutral: no Claude-specific flags or agent-implementation assumptions;
  the harness states the outcome, SOP acts on it.
- Never infer `needs_human` by scanning prose for "permission", "approve", etc.
- The existing no-change safety is preserved and made precise: a claimed change
  with none produced is a failure with an actionable reason; a legitimate
  no-change completion is allowed.
- A reported outcome is never treated as success unless it is `completed`.

## Tests

agent: `parseOutcome` (prose/empty/non-JSON/unknown status → nil; completed with
default and explicit `changes_expected`; needs_human; failed); the command agent
surfaces the outcome; prose has none. CLI: `needs_human` → NEEDS_HUMAN with the
reason; `failed` → FAIL; `completed+changes_expected=true` with an empty diff →
FAIL "no repository changes"; `completed+changes_expected=false` with an empty
diff → PASS. Verified end-to-end with a real binary.

## Acceptance Criteria

- [x] The command-agent protocol carries a structured outcome for mutating capabilities.
- [x] `needs_human` stops with NEEDS_HUMAN; `failed` stops with FAIL.
- [x] A claimed change with none produced fails with an actionable reason.
- [x] A legitimate no-change completion proceeds.
- [x] The outcome is never inferred from prose; legacy responses keep working.
- [x] `make check` passes.

## Git

Branch: `task/T047-command-agent-outcomes`
Commit: `task(T047): add structured command-agent outcomes`
PR: `[Task T047] Add structured command-agent outcomes`

## Out of Scope

Requeuing a task after a `needs_human`/`failed` outcome (it is currently BLOCKED,
as any non-PASS result, and is not auto-retried); applying outcomes to
non-mutating capabilities.
