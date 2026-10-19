# RDM-004 corpus viability audit

**Date:** 2026-10-19
**Scope:** the 30-case frozen held-out corpus (`corpus-heldout.json`, revision 1).
**Outcome:** **insufficient evidence.** None of the twelve executable cases can be
connected to a recorded source. The corpus is preserved as an unusable evaluation
revision (`REVISION-2.md`, `REVISION-2-MANIFEST.json`).

## 1. Audit method

Searched, read-only:

- every persisted run directory under `.agent-sdlc/runs/`: `task.md`, `report.json`,
  `report.md`, `state.json`, `validation.json`, `counts.json`, `trace.json`,
  `diff.patch`;
- repository git history by commit message and path;
- the frozen corpus case text.

A case was to be recovered only when an artifact **established** the connection: a task
input matching the case, a recorded starting revision, a verification command, and
provenance. A similar error message was explicitly not sufficient.

## 2. Evidence inventory

| Resource | Available |
| --- | --- |
| Run directories | 309 |
| With `task.md` | 203 |
| With `report.json` | 202 |
| With `validation.json` (verification commands) | 190 |
| With a recorded **starting revision** | 0 found |
| Corpus cases matching a recorded task input | 0 |

Keyword searches (`missing go.sum`, `nil pointer`, `flaky`, `unused import`,
`stale lock`, `transient network`, `unknown status`, `schema file`, `timed out`) matched
only code text inside `diff.patch` files, unrelated reports, or the RDM-004 corpus itself.
No run records a repository base commit; `counts.json` records the SOP **binary**
revision, which is not the repository revision a case began from.

## 3. Case-by-case inventory

Every case is missing all four required inputs. "Candidate" means an artifact that was
inspected but did not establish identity; none did.

| Case | Class | Starting revision | Task input | Verification command | Provenance | Candidate found |
| --- | --- | --- | --- | --- | --- | --- |
| `dq-rec-001` | recovery | missing | missing | missing | missing | none |
| `dq-rec-002` | recovery | missing | missing | missing | missing | none |
| `dq-rec-003` | recovery | missing | missing | missing | missing | none |
| `dq-rec-004` | recovery | missing | missing | missing | missing | none |
| `dq-rec-005` | recovery | missing | missing | missing | missing | none |
| `dq-rec-006` | recovery | missing | missing | missing | missing | none |
| `dq-rec-007` | recovery | missing | missing | missing | missing | none |
| `dq-rec-008` | recovery | missing | missing | missing | missing | none |
| `dq-rec-009` | recovery | missing | missing | missing | missing | none |
| `dq-rec-010` | recovery | missing | missing | missing | missing | none |
| `dq-neg-003` | negative | missing | missing | missing | missing | none |
| `dq-neg-007` | negative | missing | missing | missing | missing | none |

**Recovered: 0 of 12.**

## 4. Evaluation target (must be declared before implementation)

The harness currently measures **decision-token accuracy**: `DecisionFunc` returns a
token and non-executable cases are scored by token equality.

| Target | Measurable today? | Reason |
| --- | --- | --- |
| Decision quality (planning/negative cases) | **yes** | token equality against a frozen label |
| Executable recovery success | **no** | requires applying the model's own action, then running verification; the current output is a token only |
| Both | no | would need a new action-output contract and source-backed fixtures |

A canned repair chosen from a decision token is **not** model-generated implementation
and must not be scored as recovery success. The target therefore must be chosen
explicitly; it cannot be inferred from the current code.

## 5. Replacement corpus proposal (not applied)

A viable corpus can be built from traceable runs, but it changes scope and must be an
operator decision.

**Eligible source:** a run directory containing `task.md`, `report.json` with a recorded
outcome, `validation.json` with the executed commands, and a decision timestamp. Its
provenance is the run artifact paths plus the recorded SOP binary revision.

**What can be supported:**

- **Planning/negative cases:** feasible. Task input is `task.md`; the expected decision
  is the recorded outcome/classification; pre-decision evidence is the artifacts that
  existed before that run.
- **Executable recovery cases:** still unsupported. No run records a starting repository
  revision, and the decision output is a token, not an action.

**Scope change:** the 10/10/10 class distribution and the 30-case size cannot be
preserved from source-backed material. A planning-only corpus would change the class mix
and the primary metric, and dropping executable cases would remove the recovery arm the
plan requires. Any such change is a new experiment, not a repair, and has not been made.

## 6. Conclusion

Insufficient evidence: the twelve executable cases cannot be recovered, and no source
records the information needed to rebuild them. The corpus remains frozen and unusable
for held-out evaluation. No cloud calls were made, no setup was expanded, and no
semantic retrieval was added during this audit.
