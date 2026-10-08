# Validation

**Type:** Normative specification

## Purpose

This document is the normative specification for `sop validate` and the validation
runner: how configured commands are declared, ordered, and executed, and the role
of validation as deterministic evidence required before a pass. The quality gate
that consumes this evidence is owned by [QUALITY.md](QUALITY.md).

## Related Specifications

- [../architecture/OVERVIEW.md](../architecture/OVERVIEW.md) — §4 Validation Runner
  and Fix Loop.
- [../PRD.md](../PRD.md) — FR-10 Local verification, §5 Tests before trust.
- [AGENT-PROVIDER.md](AGENT-PROVIDER.md) — the `changes_expected: false` and
  verify-first paths that still require validation.
- [QUALITY.md](QUALITY.md) — the gate and fix loop; [EXECUTION.md](EXECUTION.md) —
  when validation runs in `sop run`.
- [../reference/PERFORMANCE.md](../reference/PERFORMANCE.md) — validation/review
  reuse safety rules.
- [../reference/CLI.md](../reference/CLI.md),
  [../reference/CONFIGURATION.md](../reference/CONFIGURATION.md) — command and
  configuration reference; [../README.md](../README.md) — documentation index.

## Normative Language

The terms MUST, MUST NOT, SHOULD, SHOULD NOT, and MAY are normative.

---

## 1. Command Source

Validation commands MUST come from the `validation` section (`build`, `test`,
`lint`) of `.agent-sdlc/config.yaml`. Categories a project may configure include
`build`, unit test, integration test, `lint`, and Docker build. See
[../reference/CONFIGURATION.md](../reference/CONFIGURATION.md).

A task that changes the repository MUST NOT pass when no validation command is
configured: SOP fails closed rather than treat the vacuously passing empty suite as
verification. The gate returns `FAIL` and the run reports the outcome as
`VALIDATION_NOT_CONFIGURED` — a terminal operator-intervention/configuration state,
not an implementation failure and not a human approval. Configuring validation (for
example via `sop init`) and re-running clears it.

## 2. Execution Order and Directory

The configured commands MUST run in the project directory, in the fixed order
`build`, then `test`, then `lint`. Commands MUST NOT be reordered or run out of
directory.

## 3. Fail Fast

Validation MUST stop at the first failing command, so an uncompilable change never
reaches review. The runner MUST NOT continue to later commands after a failure.

## 4. Exit Code

`sop validate` MUST exit non-zero when any configured command fails, and zero when
all pass.

## 5. Validation Is Deterministic Evidence

Validation is deterministic evidence produced by executing the configured commands;
it is **not** a model judgment. A task MUST NOT pass without the required
validation succeeding. When validation fails, the deterministic failure MUST be
handed to the fix loop as context (see [QUALITY.md](QUALITY.md)) — a failing check
is actionable in its own right.

"The required validation" is the configured command set. When a task requires
validation and none is configured, the run MUST NOT pass and MUST NOT loop: the
deterministic outcome is the `VALIDATION_NOT_CONFIGURED` configuration failure from
§1, which the autonomy policy treats as a terminal operator-intervention state rather
than a human approval (see [QUALITY.md](QUALITY.md) and [HUMAN-APPROVAL.md](HUMAN-APPROVAL.md)).

## 6. Paths That Still Validate

- A `changes_expected: false` completion MUST still run the configured validation
  before it can pass (see [AGENT-PROVIDER.md](AGENT-PROVIDER.md)).
- A `verify-first` task MUST run the configured validation before any agent; a pass
  completes locally, and a failure continues into the ordinary lifecycle.

## 7. No Change to Review

Review MUST be skipped when there is no working-tree change to review: `sop review`
reports "no changes to review". Validation is likewise skipped when there is
nothing it needs to re-prove, but a run MUST NOT treat "nothing to check" as a pass
unless the configured checks were actually satisfied.

## 8. Reuse Is Bounded by Safety Rules

A single run MAY reuse a prior validation or review result, but ONLY when SOP can
prove the inputs are identical (validation: the exact command set and the
working-tree diff, hashed; review: the task, diff, and engine, hashed). Only a
**passing** validation or a **clean** review is cacheable, reuse MUST NOT cross a
run or a plan, and a miss MUST always be safe because correctness never depends on
a hit. The authoritative identity and rules are owned by
[../reference/PERFORMANCE.md](../reference/PERFORMANCE.md); this document MUST NOT
restate them.
