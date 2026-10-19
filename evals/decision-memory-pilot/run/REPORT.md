# Decision-memory pilot — planning-only

**Corpus:** Source-backed planning decisions (held-out split). (24 cases)

**Frozen rules:** version 1, 3 repeats per case, primary threshold 10.0 pp, p95 added latency <= 2000 ms, token ceiling 20.0%

## Corpus

| Class | Cases |
| --- | --- |
| planning | 10 |
| recovery | 0 |
| negative | 14 |

## Matched conditions

| Field | Arm A | Arm B |
| --- | --- | --- |
| model | deepseek-v4.1-flash:cloud | deepseek-v4.1-flash:cloud |
| model version |  |  |
| prompt hash | 8750952dfca3f477de45812eb349e6a572f82697e83223251124cc4bdd2d4558 | 8750952dfca3f477de45812eb349e6a572f82697e83223251124cc4bdd2d4558 |
| sampling | temperature=0.00 top_p=0.00 seed=0 | temperature=0.00 top_p=0.00 seed=0 |
| retrieval mode | none | sqlite-fts5-keyword |
| evidence token budget | 2000 | 2000 |

## Results

| Metric | Arm A | Arm B |
| --- | --- | --- |
| independent cases | 24 | 24 |
| repeat records | 72 | 72 |
| classification accuracy (%) | 31.94 | 37.50 |
| estimated tokens | 32222 | 32222 |
| p50 latency (ms) | 403 | 377 |
| p95 latency (ms) | 517 | 504 |
| approval bypasses | unavailable | unavailable |
| cross-corpus disclosures | unavailable | unavailable |
| required-evidence bypasses | unavailable | unavailable |
| unsupported-claim rate | unavailable | unavailable |
| unnecessary-escalation rate | unavailable | unavailable |
| false-escalation rate | unavailable | unavailable |

## Retrieval coverage

| Run | Cases | With evidence | Without evidence |
| --- | --- | --- | --- |
| Original measured run (as executed) | 24 | 0 of 24 cases (audit-reconstructed: the legacy AND query matched no evidence and both arms had identical estimated token totals) | — |
| Post-run audit (corrected query) | 24 | 8 | 16 |

The post-run audit is deterministic and makes no model call; it is not a re-run of the original experiment.

**Prompt equality:** unverified: prompts were not persisted; equal estimated token totals are consistent with, but do not prove, identical prompts

## Paired comparison

- Paired cases: 24 (repeats are clustered within a case and are never treated as independent)
- Mean difference: 5.56 pp (B - A)
- Per-case spread: 166.67 pp
- 95% bootstrap interval (case-clustered): [-6.94, 20.83] pp

## Frozen gate

| Check | Value | Pass |
| --- | --- | --- |
| classification accuracy gain >= threshold | 5.56 pp (>= 10.00) | false |
| zero safety violations | unavailable (not observed) | unavailable |
| secondary not worse | unavailable (not observed) | unavailable |
| p95 added latency | -13 ms (<= 2000) | true |
| token ceiling | 32222 tokens (<= 20%) | true |

## Coverage gate (pilot, disposition labels)

| Check | Value | Pass |
| --- | --- | --- |
| held-out cases >= 20 | 24 | true |
| distinct disposition labels >= 2 | 4 | true |
| minimum cases per label >= 3 (lowest: AUTO_FIX) | 2 | false |

**Coverage gate failed:** disposition label AUTO_FIX has 2 cases, need at least 3. The sample was not enlarged; no passing result is claimed.

## Decision

Frozen decision: `NO-GO` (gate pass: false).

NO-GO for rollout; retrieval benefit was not evaluated.

Not evaluated: C, P. Unimplemented or unavailable arms are never imputed.

A small pilot supports a limited trial, not a general reliability claim.
Defaults remain disabled unless an explicit rollout decision authorizes a change.
