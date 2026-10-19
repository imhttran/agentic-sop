# RDM-004 — Decision-quality comparison protocol (frozen)

This protocol is frozen before any final run. `PROTOCOL.sha256` records its hash;
a verification test recomputes the hash and fails on any change. Changing this
protocol after the final run requires an explicit re-freeze and a new held-out
set.

## Question

Does historical retrieval improve decision quality over task context alone? The
comparison answers this for local SQLite FTS5 keyword retrieval.

## Arms

- **A (control)** — current task context only, no historical retrieval.
- **B** — current task context plus SQLite FTS5 keyword retrieval over the frozen
  corpus, restricted to records produced at or before the case evidence cutoff.
- **C (conditional)** — current task context plus local hybrid keyword and
  semantic retrieval. RDM-004 does not implement semantic retrieval. C is
  recorded as *not evaluated* unless a prior stage produces it.
- **P (optional)** — current task context plus the existing PostgreSQL backend,
  with its retrieval mode recorded explicitly. Not evaluated when the backend is
  not confirmed available.

A passing B result is sufficient to complete the initial experiment. C and P are
not mandatory deliverables.

## Matched conditions

A and B run with the same model, model version, sampling settings, prompt
template (same hash), execution constraints, and a comparable evidence-token
budget. The run artifact records model, model version, temperature, top_p, seed,
prompt-template hash, and the evidence-token budget per arm.

## Repeats and case counts

Each case runs **three times per evaluated arm**. Repeated runs measure
variability. Repeats **never** increase the number of independent cases. The
report states the independent case count separately from the repeat count.

## Evidence discipline

- Each case exposes only evidence available **before** its decision point. The
  case records its `evidence_cutoff`; no evidence item may be produced after it.
- The case excludes its **eventual solution** and near-duplicate solution
  records. The corpus validator fails if any excluded record is exposed.
- Held-out cases are **never** used for tuning. A material tuning change freezes
  a fresh held-out set; the earlier set is no longer used for an independent
  final evaluation.

## Scoring

**Primary** — the percentage of cases with a correct plan or verified recovery,
averaged across repeats. A plan is correct when the arm-blind rubric passes it. A
recovery is correct only when a sandboxed replay verifies it.

**Secondary** — repeated failed actions, unnecessary escalations, human
interventions, unsupported claims, and citation accuracy. Unsupported-claim and
unnecessary-escalation rates are compared against the arm A baseline.

**Operational** — p50/p95 latency, token use, and actual or estimated cost.

**Diagnostic only** — recall@5, precision@5, and MRR. These never decide the gate.

## Pilot gate (frozen)

1. The selected retrieval arm improves the primary score by at least **10
   percentage points** over A.
2. C must also improve the primary score by at least **10 percentage points**
   over B to justify hybrid retrieval.
3. **Zero** approval bypasses, cross-corpus disclosures, or required-evidence
   bypasses.
4. Unsupported-claim and unnecessary-escalation rates do not exceed the arm A
   baseline.
5. Added p95 retrieval latency is at most **2 seconds**.
6. Total token use is at most **20%** above A.
7. The report states case counts, paired differences, variability, and
   uncertainty.

## Decision

- If C is evaluated and passes, recommend an opt-in local hybrid pilot.
- If B passes against A and C is not evaluated or does not beat B, recommend
  local keyword retrieval.
- If no evaluated retrieval arm passes, retain disabled defaults and document
  the result.

## Scope

A small pilot supports a **limited trial**, not a general reliability claim. If
sufficient independent cases do not exist, the report records insufficient
evidence and the feature stays experimental.
