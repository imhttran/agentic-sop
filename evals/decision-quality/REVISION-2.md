# RDM-004 evaluation revision 2 — missing-fixture defect

**Status:** **UNUSABLE for held-out evaluation.** The corpus is preserved unchanged for audit; this revision does **not** supply corrected evaluation inputs.
**Parent revision:** 1 (`PROTOCOL.md`, `RUBRIC.md`, `RUBRIC.json`, `corpus-heldout.json`,
`corpus-tuning.json`), preserved byte-for-byte and still verified by
`PROTOCOL.sha256` / `CORPUS.sha256`.

This revision does **not** modify the experiment and does **not** repair the corpus. The
corpus remains 30 held-out cases (10 planning, 10 recovery, 10 negative), three repeats
per case, two arms (A and B), but it cannot serve as a viable evaluation input until the
missing sources are supplied or the corpus is replaced.

## Defect

Twelve cases declare `"executable": true`, which per the protocol requires sandboxed
replay. They carry **no replayable source**: no starting revision, no task input, no
verification command, and no provenance tying the case to repository history. The case
texts are generic summaries, not references to identifiable commits or artifacts.

Inventing fixtures would manufacture historical evidence, so none were added. The
affected cases and their missing inputs are listed below. Every one is missing **all
four** required inputs:

| Case         | Class    | Missing starting revision | Missing task input | Missing verification command | Missing provenance |
| ------------ | -------- | ------------------------- | ------------------ | ---------------------------- | ------------------ |
| `dq-rec-001` | recovery | yes                       | yes                | yes                          | yes                |
| `dq-rec-002` | recovery | yes                       | yes                | yes                          | yes                |
| `dq-rec-003` | recovery | yes                       | yes                | yes                          | yes                |
| `dq-rec-004` | recovery | yes                       | yes                | yes                          | yes                |
| `dq-rec-005` | recovery | yes                       | yes                | yes                          | yes                |
| `dq-rec-006` | recovery | yes                       | yes                | yes                          | yes                |
| `dq-rec-007` | recovery | yes                       | yes                | yes                          | yes                |
| `dq-rec-008` | recovery | yes                       | yes                | yes                          | yes                |
| `dq-rec-009` | recovery | yes                       | yes                | yes                          | yes                |
| `dq-rec-010` | recovery | yes                       | yes                | yes                          | yes                |
| `dq-neg-003` | negative | yes                       | yes                | yes                          | yes                |
| `dq-neg-007` | negative | yes                       | yes                | yes                          | yes                |

## Consequence

A run over the current corpus cannot evaluate those twelve cases. Their results would be
`not_evaluable` with an explicit reason, coverage would be 18 of 30 cases, and
`InsufficientEvidence` would report it. **No go/no-go conclusion may be drawn from
partial coverage**, and no attempt was made to substitute a canned repair for a measured
implementation.

## What is required to unblock

For each affected case, a replay fixture must name:

1. the repository and starting revision the case began from;
2. the task input presented to the model at the decision point;
3. the verification command that decides success;
4. provenance (artifact path or commit) that makes the case auditable.

Those inputs do not exist in the frozen corpus and are not recoverable from the generic
case text. Supplying them is a corpus-authoring task, not a harness change.

## Measured outcome for executable cases

Success for an executable case is defined as: **the sandbox applies the model's own
proposed action to the fixture's starting revision, and then the fixture's verification
command passes.**

A canned or fixture-selected repair is not model-generated implementation and must never
be reported as success. The replay runner therefore requires a fixture that names both
the starting revision and the verification command; applying a fixed command chosen from
a decision token would measure the fixture, not the model.

Because the corpus provides neither an action format nor a fixture for the twelve
executable cases, replay is **not implemented** rather than implemented dishonestly.

## Freeze

`REVISION-2.sha256` covers this document and `REVISION-2-MANIFEST.json`. The revision-1
sidecars remain in force and unchanged.
