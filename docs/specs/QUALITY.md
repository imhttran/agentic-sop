# Quality

**Type:** Normative specification

## Purpose

This document is the normative specification for SOP's deterministic quality gate:
the `PASS`/`FAIL`/`NEEDS_HUMAN` verdict, the blocking-severity policy, the shared
`BlockingFindings` rule, and the bounded fix loop. The checks the gate consumes are
owned by [VALIDATION.md](VALIDATION.md); the review engine that produces findings
is owned by [REVIEW.md](REVIEW.md).

## Related Specifications

- [../architecture/OVERVIEW.md](../architecture/OVERVIEW.md) — §4 Quality Gate and
  Fix Loop, §8 Review Architecture.
- [../PRD.md](../PRD.md) — FR-12 Review remediation, FR-17 Bounded retry,
  FR-18 Blocked state, §13 Default Guardrails.
- [VALIDATION.md](VALIDATION.md) — deterministic evidence; [REVIEW.md](REVIEW.md) —
  findings; [HUMAN-APPROVAL.md](HUMAN-APPROVAL.md) — the human boundary.
- [EXECUTION.md](EXECUTION.md) — the run lifecycle and fix loop; [RECOVERY.md](RECOVERY.md)
  — requeue and retry accounting.
- [../reference/CONFIGURATION.md](../reference/CONFIGURATION.md) — the configuration
  reference; [../README.md](../README.md) — documentation index.

## Normative Language

The terms MUST, MUST NOT, SHOULD, SHOULD NOT, and MAY are normative.

---

## 1. The Verdict

The quality gate MUST combine the verification status, unresolved review findings,
the fix-loop budget, and human-required flags into one deterministic verdict:

```text
PASS | FAIL | NEEDS_HUMAN
```

The verdict MUST be computed in code, MUST NOT be decided by a model, and MUST NOT
be inferred from prose.

## 2. Blocking Findings

A review finding is **blocking** when its severity is named in `quality.fail_on`
(default: `critical`, `high`). The single `BlockingFindings` rule MUST be shared by
review and the gate, so review and the gate MUST agree on what blocks, and the
`review` command's exit code MUST be non-zero while any blocking finding remains.
The model produces findings; the deterministic policy decides whether they block.

## 3. Configuration Policy

- `quality.fail_on` — the severities that block a pass (default `critical`, `high`).
- `quality.max_fix_cycles` — the fix-loop budget (default **3**).
- `quality.require_tests` — when `true` (the default), relevant tests MUST pass for
  a `PASS`.

An invalid severity, or any unknown key, MUST fail with a clear message rather than
falling back silently. See [../reference/CONFIGURATION.md](../reference/CONFIGURATION.md).

## 4. The Bounded Fix Loop

When validation fails, or review leaves blocking findings, the gate MUST send the
failure back to the agent with the plan, the deterministic validation failure (when
a check failed), the blocking findings, and the current diff. Validation and review
then MUST run again (regression protection).

- The loop MUST be bounded by `quality.max_fix_cycles`.
- A `require_tests` gate MUST NOT pass while the configured tests fail.
- Exhausting the budget MUST yield a `NEEDS_HUMAN` verdict rather than looping —
  never an unbounded agent loop. The verdict is the gate's; whether it becomes a
  human authorization or a bounded terminal stop is owned by the autonomy policy
  and [HUMAN-APPROVAL.md](HUMAN-APPROVAL.md).

## 5. Outcome Semantics

- `PASS` — verification passed (when required) and no blocking findings remain; the
  task may proceed to commit/integration gates.
- `FAIL` — a deterministic failure (including a claimed change with none produced)
  leaves the task `BLOCKED`.
- `NEEDS_HUMAN` — the fix budget is spent, or a human boundary was reported. The
  gate's verdict is `NEEDS_HUMAN`; what follows is owned by the autonomy policy and
  [HUMAN-APPROVAL.md](HUMAN-APPROVAL.md). At the conservative levels the task is
  requeued for a later run, bounded by `max_attempts`; a level that treats bounded
  automation exhaustion as terminal stops the task as an automation failure rather
  than a human decision. The gate itself never loops. See [RECOVERY.md](RECOVERY.md).

## 6. Relationship to Validation and Review

The gate owns the verdict; it does not run commands or produce findings itself.
Validation MUST fail fast before review so a broken check reaches the fix loop even
when review is skipped (see [VALIDATION.md](VALIDATION.md)). Review is skipped when
there is no change to review, and review/re-validation results MAY be reused only
under the safety rules in [../reference/PERFORMANCE.md](../reference/PERFORMANCE.md).
