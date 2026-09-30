# Review

**Type:** Normative specification

## Purpose

This document is the normative specification for the review pipeline: its layers,
the `self` and `open-code-review` engines, the rule that the model produces
findings but never decides the verdict, the "no changes to review" case, the
self-review focus areas, and the external second pass. The blocking rule and the
gate verdict are owned by [QUALITY.md](QUALITY.md); the deterministic checks that
precede review are owned by [VALIDATION.md](VALIDATION.md).

## Related Specifications

- [../architecture/OVERVIEW.md](../architecture/OVERVIEW.md) — §4 Review Runner,
  §8 Review Architecture.
- [../PRD.md](../PRD.md) — FR-11 Automated review, FR-12 Review remediation.
- [QUALITY.md](QUALITY.md) — the verdict and the `BlockingFindings` rule shared by
  review and the gate.
- [VALIDATION.md](VALIDATION.md) — the cheap deterministic checks that run first.
- [EXECUTION.md](EXECUTION.md) — the run lifecycle and the bounded fix loop.
- [OPENJEV.md](OPENJEV.md) — JEV is a separate read-only analysis, not part of review.
- [../reference/CONFIGURATION.md](../reference/CONFIGURATION.md) — `review.engine`,
  `review.delegation`, and `quality.fail_on`.

## Normative Language

The terms MUST, MUST NOT, SHOULD, SHOULD NOT, and MAY are normative.

---

## 1. Pipeline Layers

Review is layered, cheapest first:

```text
Implementation → Local Tests → Self Review → Open Code Review → Fix Findings → PR → CI
```

- Compilation and tests MUST happen before expensive AI review; a check that
  fails MUST reach the fix loop even when review is skipped, because review is
  skipped when validation fails (see [VALIDATION.md](VALIDATION.md)).
- Findings MUST feed the bounded fix loop rather than terminating a workflow on
  their own (see [EXECUTION.md](EXECUTION.md) and [QUALITY.md](QUALITY.md)).

## 2. Engines

`review.engine` selects the engine: `self` (default) or `open-code-review`.

- **`self`** — asks the agent for structured findings. It is the default and MUST
  always be available.
- **`open-code-review`** — an adapter that runs the external command named by
  `SOP_REVIEW_COMMAND`.
- When `open-code-review` is selected but `SOP_REVIEW_COMMAND` is not configured,
  the command MUST report guidance; it MUST NOT silently substitute the internal
  reviewer.

Both engines MUST produce structured findings (severity, category, path, line,
message). The engine is replaceable and MUST NOT change how findings are judged.

## 3. The Model Never Decides the Verdict

- The model MUST produce findings only; it MUST NOT decide the verdict, and the
  verdict MUST NOT be inferred from prose.
- A finding is blocking when its severity is named in `quality.fail_on` (default
  `critical`, `high`). This MUST be the single rule shared with the gate — see
  [QUALITY.md](QUALITY.md) §2.
- `sop review` MUST exit non-zero while any blocking finding remains, and zero
  otherwise.
- Findings are advisory until deterministic policy classifies them as blocking.
  The orchestrator MUST NOT blindly implement every suggestion.

## 4. No Changes to Review

- Git MUST remain the authority on what changed.
- When the working tree is unchanged, review MUST report "no changes to review"
  and MUST NOT manufacture findings; review is skipped (see
  [QUALITY.md](QUALITY.md)).

## 5. Self-Review Focus

Self-review MUST examine at least:

- acceptance criteria;
- correctness;
- tests;
- edge cases;
- error handling;
- architecture boundaries;
- unnecessary complexity.

## 6. External Review

- External automated review provides an independent second pass.
- Correctness findings SHOULD be fixed.
- Suggestions that only add unnecessary complexity MAY be declined, and declining
  a non-blocking suggestion MUST NOT be treated as a gate failure.

## 7. Relationship to JEV

JEV is a separate, optional, read-only analysis invoked after validation and
review. It is **not** part of the review pipeline and MUST NOT be presented as
one; its findings reach the same quality gate as their own signal. See
[OPENJEV.md](OPENJEV.md).
