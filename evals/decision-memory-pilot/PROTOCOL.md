# Decision-memory pilot — planning-only protocol (frozen)

**Scope.** This pilot replaces the unusable RDM-004 held-out experiment for the limited
question below. It does not evaluate implementation success, executable recovery, or
general planning quality.

## Question

Given a task input and the execution/validation evidence recorded before the decision,
does local keyword retrieval of similar historical decisions improve the decision
**classification** over task context alone?

## Target

The model returns one decision token. The measured outcome is classification accuracy
against the disposition SOP actually recorded for that run:

| Token | Meaning |
| --- | --- |
| `CONTINUE` | continue the work automatically |
| `AUTO_FIX` | run the bounded fix loop |
| `RETRY` | retry a transient failure |
| `BLOCK` | stop; no productive continuation |
| `NEEDS_HUMAN` | stop for an operator decision |

These are SOP's recorded dispositions. The label is the recorded disposition, never the
model's own claim.

## Cases

Built by `cmd/rdm-pilot-corpus` from `.agent-sdlc/runs`:

- **Eligibility:** a run directory with `classification.json`, `task.md`, `state.json`,
  and at least one pre-decision evidence item.
- **Task context:** the recorded task input (`task.md`, bounded).
- **Evidence:** execution counters, recent operations, and validation command results.
  The classification `kind`, `disposition`, and reason are **excluded** — they are the
  answer, not evidence.
- **Label:** the recorded disposition.
- **Class:** `planning` for act dispositions (`CONTINUE`, `AUTO_FIX`, `RETRY`);
  `negative` for stop dispositions (`BLOCK`, `NEEDS_HUMAN`).
- **Split:** assigned by task family, so related tasks never straddle tuning and held-out.
- **Repository revision:** not recorded by these runs. The limitation is explicit: cases
  are identified by run artifact provenance, not by a repository revision.
- **Rejected:** ambiguous or incomplete runs are excluded, never repaired or imputed.

## Arms and conditions

- Arm A: task context only.
- Arm B: task context plus local SQLite FTS5 retrieval over the case's pre-decision
  evidence, using the production advisory boundary.
- Same model, instructions, sampling, and token budget in both arms; scoring is blind to
  arm identity. Repeats: 3 per case per arm.

## Frozen gates

- **Minimum coverage:** at least 20 held-out cases, at least 2 classes, at least 3 cases
  per class. Below this the result is **insufficient evidence**.
- **Improvement:** arm B mean accuracy at least **10 percentage points** above arm A,
  with the lower bound of the case-clustered 95% bootstrap interval above 0.
- **Regression limit:** the false-escalation rate (predicting `NEEDS_HUMAN` when the
  recorded disposition is an act disposition) must not exceed arm A by more than 5
  percentage points.
- **Uncertainty:** case-clustered bootstrap, 2000 resamples; repeats within a case are
  never independent observations.
- **Decision:** GO only when every gate passes; NO-GO when measured results fail;
  INSUFFICIENT EVIDENCE when coverage or uncertainty prevents a conclusion.

## Leakage controls

Each case's evidence is drawn only from its own run's pre-decision artifacts. No case
exposes another case's label, the eventual solution, or post-decision reasoning. Held-out
labels are never used for tuning.

## What this cannot conclude

This pilot says nothing about executable recovery success, code quality, or whether
retrieval improves real planning outcomes. Those require replayable starting states that
the recorded runs do not contain.
