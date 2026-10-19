# Closeout — decision-memory experiments

**Date:** 2026-10-19
**Status:** decision memory remains **disabled by default**.

## 1. Original experiment (RDM-004, executable-inclusive)

**Intent:** compare no historical retrieval (A) with local SQLite keyword retrieval (B)
over 30 held-out cases, 3 repeats, 2 arms, including 12 executable recovery cases.

**Outcome: unevaluable.** `CORPUS-AUDIT.md` records that 0 of 12 executable cases could be
connected to a source: no starting revision, task input, verification command, or
provenance exists in the recorded runs. No fixture was invented.

**Disposition:** `RDM-004`, `RDM-004B`, `RDM-002C`, `RDM-005`, `RDM-006` were recorded
`NOT_REQUIRED` with evidence. RDM-004 was **not** verified, and its historical verdicts
were not rewritten.

## 2. Planning-only pilot (authorized scope change)

### 2.1 Corpus accounting

| Stage | Count | Explanation |
| --- | --- | --- |
| Directory entries scanned at build time | 309 | `os.ReadDir` entries |
| Eligible | 62 | classification + task input + state + trace/validation evidence |
| Excluded: no recorded classification | 247 | 309 − 62; no other exclusion category occurred |
| Corpus cases | 62 | 24 held-out + 38 tuning; equality verified against the 62 eligible run IDs |
| Current run directories | 317 | 8 run directories created after the corpus build (DMP/administrative), none with a classification, so none is eligible and the corpus is unchanged |

Held-out composition: 10 planning, 14 negative; recorded dispositions `NEEDS_HUMAN` 8,
`BLOCK` 6, `CONTINUE` 8, `AUTO_FIX` 2. Tuning and held-out splits are disjoint by task
family.

### 2.2 Freeze before execution

```text
PROTOCOL.sha256 / CORPUS.sha256 written   2026-10-19T15:34:06
run-A.json / usage.json written           2026-10-19T15:36:48
checkpoint/usage binding                 corpus+protocol+rubric+harness+model+sampling
```

The gates, corpus, labels, and harness identity were fixed before any model call.

### 2.3 How each metric was derived

| Metric | Derivation |
| --- | --- |
| Accuracy | Correct repeats ÷ evaluated repeats × 100, per arm |
| Paired difference | Per-case mean over its 3 repeats, then the mean of (B − A); repeats are never independent |
| Uncertainty | 2,000-resample bootstrap clustered by case, 2.5th/97.5th percentiles |
| Tokens | `chars ÷ 4` estimated over prompt + output per decision, summed per arm |
| Latency | Wall-clock per provider request; p50/p95 over repeats |
| Safety counters | **Unavailable** — the runner never observed approval bypasses, cross-corpus disclosures, or required-evidence bypasses, so `0` would be a fabricated pass |
| False escalations | **Unavailable** — predictions were not persisted when this run executed |
| Cost | **Unavailable** — no pricing configured in the repository |

### 2.4 Measured result

| Metric | Arm A | Arm B |
| --- | --- | --- |
| Cases / repeats | 24 / 72 | 24 / 72 |
| Accuracy | 31.94% | 37.50% |
| Tokens | 32,222 | 32,222 |
| p50 / p95 latency | 403 / 517 ms | 377 / 504 ms |
| Safety violations | unavailable | unavailable |
| False escalations | unavailable | unavailable |

Paired difference **+5.56 pp**, case-clustered 95% interval **[-6.94, 20.83] pp**.
Usage: 144 decisions, 144 requests, 0 retries, 64,444 tokens, 60 s active.

### 2.5 Retrieval actually occurred? **No.**

Deterministic reconstruction (`TestPilotRetrievalCoverage`, no model call) over the frozen
corpus:

```text
held-out: 24 cases, 0 retrieved evidence, 24 retrieved none
tuning:   38 cases, 0 retrieved evidence, 38 retrieved none
```

The identical token totals across arms (32,222) confirm both arms produced identical
prompts. **Arm B was therefore inert**: the +5.56 pp difference is model noise and cannot
be attributed to retrieval. The retrieval query (all task-context tokens ANDed) matched no
evidence record.

### 2.6 What the result means

- The measured quantity is **agreement with historical SOP classifications**, not
  independently validated decision quality. The labels are the system's own recorded
  dispositions.
- Because retrieval returned nothing, the pilot did **not** test the retrieval question.
- **Decision: NO-GO.** No benefit was demonstrated; the gate is not met. The NO-GO stands
  on the measurement and on the inert arm.
- Memory stays off by default. The result does not generalize to executable recovery.

## 3. Remaining limitations

1. No repository revision is recorded per case; identity rests on run-artifact provenance.
2. No per-decision retrieval or prompt-inclusion log exists; coverage was reconstructed
   deterministically, not captured at call time.
3. Safety and false-escalation metrics are unavailable for this run.
4. 24 held-out cases with imbalanced labels produce a wide interval.
5. Cost is unknown; no pricing is configured.

## 4. Next actions (not taken here)

- A corrected retrieval query (OR/scored matching rather than all-token AND) is required
  before any claim about retrieval benefit can be tested.
- A larger source-backed corpus and persisted prediction labels would allow false-escalation
  measurement.
- Executable recovery remains deferred until runs capture reproducible starting states.

## 5. Post-run audit and retrieval repair

The pilot's arm B retrieved nothing: the legacy query ANDed every task-context token. The
result was an A/A noise comparison. The query was repaired to OR-join tokens with a
two-term relevance floor and validated on the tuning split for **lexical threshold compliance only**
(57 items at or above the floor, 0 below, 110 below-floor rejected; semantic relevance
was not judged). The held-out split was audited deterministically (8 of 24
cases, 25 threshold-compliant items, 63 below-floor rejected) but **not re-run**; `REVISION-2.md` records the repair as development evidence and requires a new frozen
held-out revision for any claim. Prompt equality for the original run is unverified
(prompts were not persisted). The frozen coverage gate applied to disposition labels
fails on `AUTO_FIX` (2 cases, need 3); the sample was not enlarged.
