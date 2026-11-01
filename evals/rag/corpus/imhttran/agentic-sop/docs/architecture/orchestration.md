# Orchestration Domain Model (ORCH-001)

The `internal/orchestration` package defines a provider/model-neutral domain
model for orchestrating work across the SOP lifecycle:

    Task -> Orchestrator -> Workers -> Integrator -> Reviewer -> Verification

plus the deterministic `SINGLE` / `SEQUENTIAL` / `PARALLEL` execution decision.

## Single-lifecycle-authority invariant

**Exactly one authoritative SOP lifecycle lives in `internal/domain`.** This is
non-negotiable and is enforced by a guard test
(`internal/orchestration/lifecycle_guard_test.go`).

### Owner

`internal/domain` owns `TaskStatus`, `BlockedReason`, `ExecutionMode`, the
`transitions` state machine, and every predicate:

- `Task.CanTransitionTo` / `Task.Transition` — the only legal state changes.
- `Task.Block` / `Task.Requeue` / `Task.RequeueWithoutSpending` /
  `Task.CompleteExternally` — the only lifecycle operations.
- `Task.IsSatisfied` / `Task.Executed` / `Task.IsRunnable` /
  `Task.IsBlockedRecoverable` / `Task.IsTerminalBlocked` — the only completion
  and selection predicates.
- `Task.ResolveDependencies` — the only dependency-resolution authority.

### What orchestration MAY do

- Read domain values: `Task.ID`, `Task.DependencyIDs`, `Task.ExecutionMode`,
  `Task.Status`, and the results of the domain predicates.
- Reference `agent.Capability` values for provider-neutral work descriptions.
- Compute a deterministic orchestration mode from task, dependency, and
  capability data via `Decide` / `PlanMode`.
- Express the decision as advisory metadata for a caller.

### What orchestration MUST NOT do

- Define its own status, blocked-reason, or execution-mode vocabulary.
- Define a transition table or a competing transition/completion predicate.
- Assign `Task.Status` directly or otherwise mutate lifecycle state.
- Import a mutating lifecycle path (`internal/scheduler`, `internal/run`).
- Depend on any concrete provider or model (`openai`, `ollama`, `mlx`,
  `openaicompatible`) or on a transport.

All lifecycle questions delegate to `internal/domain`; the `TaskView` type keeps
a reference to the authoritative `domain.Task` so its `Status`, `IsSatisfied`,
`IsRunnable`, and `ResolveDependencies` queries delegate rather than
re-implement.

## Execution-mode decision

`Mode` is orthogonal to `domain.ExecutionMode`. `ExecutionMode` describes how a
single task runs (`implement` / `verify-first` / `done`); `Mode` describes how
many workers run.

`Decide` is a pure function over `(units, dependencies, available capabilities)`.
It performs no I/O, consults no provider or model, and is independent of map
iteration order. The deterministic rule set, evaluated in order:

1. `SINGLE` when fewer than two units are present (lone task, empty input).
2. `SINGLE` on ambiguous input (blank ID or blank capability).
3. `SINGLE` when any required capability is unavailable (unsatisfiable).
4. `SEQUENTIAL` when any dependency edge exists among the units, or a
dependency reference cannot be resolved within the set.
5. `PARALLEL` when two or more distinct, mutually independent,
   capability-satisfiable, non-overlapping units are present.

`SINGLE` is a legitimate, efficient outcome and remains the default: a split is
only chosen on explicit positive evidence.

## Consumer boundary (placeholder / unsupported)

The `internal/scheduler` and `internal/run` runnable-task selection contracts
are **unconfirmed**. `PlanMode` therefore does not integrate with the scheduler
and makes no assumption about how it selects work. Real scheduler wiring is
deferred to a future task once that contract exists. Any current caller must
treat the returned mode as advisory metadata only; it grants no lifecycle
authority.
