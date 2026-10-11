# Review

**Type:** Normative specification

## Purpose

This document is the normative specification for the review pipeline: its layers,
the `self` and `open-code-review` engines, the rule that the model produces
findings but never decides the verdict, the invocation-scoped review input, the
"no changes to review" case, the malformed-output boundary, the self-review focus
areas, and the external second pass. The blocking rule and the gate verdict are
owned by [QUALITY.md](QUALITY.md); the deterministic checks that precede review are
owned by [VALIDATION.md](VALIDATION.md).

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

## 4. Invocation-Scoped Review Input

The review input MUST be built from the task's own change set, never from the
whole working tree. The review input is:

- the invocation-scoped changed files the task produced — the paths attributed to
the task by the harness (`invocationChanges`) and recorded for the task
  (`recordTaskChanges`, surfaced as the task's changed files) — unioned with
- the task's declared deliverables.

Rules:

- Unrelated pre-existing working-tree modifications MUST be excluded. A file that
  was already dirty before the task ran and is not attributable to the task MUST
  NOT appear in the review input.
- A mutation made *during* the task MUST remain in scope and be reviewed, whether
  it is a tracked edit, a new untracked file, a deletion, or a rename.
- A task-owned untracked deliverable MUST be included; the untracked portion of
  the diff MUST be filtered to the same path set as the tracked portion.
- A declared deliverable MUST be included even when its path is not otherwise
  present in the raw change listing.
- The scope is recomputed on each review iteration against the accumulated task
  change set, so a change made by a fix cycle is reviewed.
- SOP-owned runtime state and generated output MUST NOT be attributed to a task
  and MUST NOT enter the review input.

### 4.1 Fail-Closed

When the task's change set cannot be established — for example the working tree is
dirty but no change is attributable to the task — the seam MUST fail closed: it
MUST refuse to emit an unscoped review input rather than reviewing the whole
working tree, and the refusal MUST be an explicit, testable outcome. No code path
MAY fall back to the whole working tree. An opaque change listing that names no
repository file cannot leak an unrelated modification and is not, by itself, a
fail-closed condition.

## 5. No Changes to Review

- Git MUST remain the authority on what changed.
- When the task's change set is empty and there is nothing to review, review MUST
  report "no changes to review" and MUST NOT manufacture findings; review is
  skipped (see [QUALITY.md](QUALITY.md)).

## 6. Malformed Review Output

A review response that cannot be parsed into a valid report is a deterministic,
bounded outcome. The allowed severity set is exactly `INFO`, `LOW`, `MEDIUM`,
`HIGH`, and `CRITICAL`; it MUST NOT be widened or narrowed.

### 6.1 Per-Finding Parsing

- Parsing MUST be per finding. Every finding whose severity is in the allowed set
  MUST be retained in the report and MUST block at its own severity; the verdict
  MUST NOT be lowered by the presence of a malformed sibling finding.
- A finding whose severity is missing or not in the allowed set (an invalid or
  empty severity) MUST be rejected explicitly and deterministically. It MUST NOT
  be silently dropped, silently defaulted, or tolerated.
- A valid `HIGH` or `CRITICAL` finding MUST still be retained and MUST still block
  when the same response also contains a malformed finding. The whole report MUST
  NOT be discarded because one finding is malformed.

### 6.2 Bounded Retry

- When a response cannot be parsed, the REVIEW capability MUST be retried up to a
  small, fixed bound. The bound counts total attempts and MUST be small: malformed
  output is a bounded outcome, not an unbounded loop.
- When a valid report is still not obtained within the bound, the outcome MUST be
  the bounded malformed-output classification. It MUST preserve the raw response
  and every finding that parsed with a valid severity on the final attempt.

### 6.3 Never a Pass

- Malformed review output MUST NEVER yield `quality.Pass`.
- Malformed review output MUST NEVER be a silent fix-loop exit. It MUST terminate
  the loop as an explicit, classifiable outcome that the run maps to the EXISTING
  human boundary (`NEEDS_HUMAN`), recorded through the existing approval path.
- The human approval boundary, the severity thresholds, and `quality.Evaluate`
  acceptance MUST be unchanged by this boundary.

## 7. Self-Review Focus

Self-review MUST examine at least:

- acceptance criteria;
- correctness;
- tests;
- edge cases;
- error handling;
- architecture boundaries;
- unnecessary complexity.

## 8. External Review

- External automated review provides an independent second pass.
- Correctness findings SHOULD be fixed.
- Suggestions that only add unnecessary complexity MAY be declined, and declining
  a non-blocking suggestion MUST NOT be treated as a gate failure.

## 9. Relationship to JEV

JEV is a separate, optional, read-only analysis invoked after validation and
review. It is **not** part of the review pipeline and MUST NOT be presented as
one; its findings reach the same quality gate as their own signal. See
[OPENJEV.md](OPENJEV.md).
