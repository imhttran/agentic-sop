# Execution Seam (implementation)

**Non-normative.** This page describes _how_ SOP's execution input is implemented.
The authoritative rules live in [`../specs/WORK-ITEMS.md`](../specs/WORK-ITEMS.md),
[`../specs/EXECUTION.md`](../specs/EXECUTION.md), and
[`../specs/PROMPT-EXECUTION.md`](../specs/PROMPT-EXECUTION.md); where this page and a
specification disagree, the specification wins.

## Inputs share one path

SOP has three input forms but one execution machinery:

```text
                  Inputs
           ┌────────┼─────────┐
           │        │         │
         Task     Prompt     Skill
           │        │         │
           └────────┼─────────┘
                    ↓
                WorkItem                 (internal/workitem)
                    ↓
              Capability                 (internal/agent capability guard)
                    ↓
              JEV evidence               (internal/jev, optional, read-only)
                    ↓
             SOP model router            (internal/router)
                    ↓
          SMALL / MEDIUM / LARGE
                    ↓
              model.Resolve              (internal/model)
                    ↓
           Provider validation           (internal/provider, opt-in)
                    ↓
              Agent / harness            (internal/agent, internal/ollamaagent)
                    ↓
                Execution
                    ↓
           Validation / Review           (internal/testrunner, internal/review)
                    ↓
               Run artifacts             (internal/run)
```

A planned task enters through `sop run`; a prompt enters through `sop prompt`; the
SOP skill and its one-per-capability Zed aliases (`/sop-plan`, `/sop-review`, …) each
call `sop prompt` with a capability fixed. None of them owns policy.

## Where the code lives

| Concern                                 | Package / file                                    |
| --------------------------------------- | ------------------------------------------------- |
| Unified input                           | `internal/workitem`                               |
| Model-class routing                     | `internal/router`, `internal/model`               |
| Prompt command + prompt routing seam    | `internal/cli/prompt.go`                          |
| Task lifecycle + routing seam           | `internal/cli/run.go`, `internal/cli/routing.go`  |
| Capability guard                        | `internal/agent` (`Checked`, `CapabilitiesOf`)    |
| Provider inspection / validation        | `internal/provider`                               |
| Early JEV checkpoints (task and prompt) | `internal/cli/early_jev.go` (`runJEVCheckpoint`)  |
| Run artifacts                           | `internal/run`                                    |
| Bounded escalation                      | `internal/recovery`, `internal/cli/escalation.go` |
| Command aliases + installation          | `skills/`, `scripts/install/install-skills.sh`            |

## Two execution modes for a prompt

A prompt's capability decides the path:

- **Read-only** (`PLAN`, `REVIEW`, `DIAGNOSE_FAILURE`, `DESIGN_TESTS`) → one bounded
  `Generate` call. No repository mutation.
- **Mutating** (`IMPLEMENT`) → the same `runAttempts` → `executeLifecycle` →
  `runStages` path a task uses, run against a `taskfile.Spec` projected from the
  prompt.

This is a reuse boundary, not a second engine: the guarded capability, the JEV
triage seam, the router, model resolution, provider validation, and the governed
lifecycle are the same code both `sop run` and `sop prompt` drive.

## Boundaries preserved

- JEV provides evidence only; SOP's router decides the class.
- Providers report health, models, and capabilities; they never choose a class.
- The `provider.Provider` interface is read-only (no `Execute`, `Route`, `Approve`).
- A work item carries no policy; it is a projection of the request.
- `sop run` behavior is unchanged by the prompt path.
