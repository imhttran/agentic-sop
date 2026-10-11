# EV-005 — Acceptance Criteria, Decision Gate, and Approval Prerequisites

Decision-record stage of the EV-CONTEXT-RUN evaluation. It **fixes** the objective
GO / HOLD / NO-GO thresholds for gate **E1** and gate **E2** against the already
measured baseline produced by EV-003, states the fail-closed **stop conditions**,
defines the ordered **decision procedure**, and enumerates the **prerequisite
approvals** required before any E2 work. The only artifact is this report.

> **No production change is made and no plan activation is performed.** This stage
authors and runs nothing outside the read-only E1 context. See §6.

## 0. Authority and inputs

| Input | Role |
| --- | --- |
| `docs/reports/ev-context-run/EV-002-corpus-and-baseline-protocol.md` | The pinned corpus, the pinned ref, and the model-free baseline protocol |
| `docs/reports/ev-context-run/EV-003-retrieval-evaluation.md` | The measured baseline (aggregate + per-case), and the measured retrieval overhead |
| `docs/reports/ev-context-run/EV-004-instrumentation-gaps.md` | Six-metric AVAILABLE / MISSING classification, and the token rule |
| `docs/plans/PLAN-EV-Context-Run-Evaluation.md` | Decision shape, stop conditions, prerequisite approvals, unresolved architecture decision |
| `docs/history/plans/PHASE-8-CONTEXT-EXECUTION-EFFICIENCY.md` §13–15, §26 | Gate, corpus, success gate, token accounting |

**Pinned baseline.** All EV-005 thresholds are fixed against the EV-003 measurements
over the EV-002 corpus at ref `388ea56aef4b8e2976c73dc0e211069c695b620d` (6510
candidates):

| Arm | p@5 | r@5 | MRR | bytes_mean |
| --- | --- | --- | --- | --- |
| Baseline | 0.067 | 0.278 | 0.106 | 246 |
| Retrieval (BM25) | 0.178 | 0.648 | 0.361 | 238 |

Measured retrieval overhead (live corpus, 6510 candidates): `index_build_ms=128`,
`search_ms=50` (EV-003 §6). If the corpus or the pinned ref is re-pinned, these
thresholds must be re-fixed.

## 1. Gate E1 — offline, model-free: fixed numeric thresholds

### 1.1 Allowed measured signals (exhaustive)

The E1 rule is computed **only** from these signals, each a direct output of the
existing CTX-004 metric or the EV-003 timing:

| Signal | Producer | EV-003 baseline → retrieval |
| --- | --- | --- |
| precision@k (`k=5`) | `retrievalgate.Measure.Precision` (`internal/retrievalgate/gate.go:112`) | 0.067 → 0.178 |
| recall@k (`k=5`) | `retrievalgate.Measure.Recall` (same) | 0.278 → 0.648 |
| MRR | `retrievalgate.Measure.MRR` (same) | 0.106 → 0.361 |
| context bytes | `retrievalgate.Measure.Bytes` (same) | 246 → 238 |
| retrieval overhead — index build time | EV-003 harness `time.Since(repoindex.Build)` | 128 ms |
| retrieval overhead — search time | EV-003 harness `time.Since(run over corpus)` | 50 ms |

No other signal may enter the E1 rule. The retrieval-overhead values are **offline
harness wall-clock measurements of existing calls**, not live-path instrumentation:
EV-004 §3 classifies the retrieval-overhead metric as **MISSING on the live path**
(`internal/cli/run.go` imports neither `internal/repoindex` nor `internal/retrieval`;
`internal/perf` has no `index`/`retrieve` stage constant). They bind E1 as recorded
numbers; any **live** overhead bound remains gated behind the E2 human decision.

### 1.2 Fixed thresholds

Fixed against the EV-003 aggregate baseline. `k = 5`. `Δp = p_retrieval − p_baseline`
(and likewise `ΔMRR`, `Δr`, `Δbytes`).

| Threshold | Fixed value | Rationale (EV-003 anchor) |
| --- | --- | --- |
| Minimum precision improvement | `Δp ≥ +0.050` (baseline 0.067 → ≥ 0.117) | The measured improvement is +0.111; the bound is fixed below it to avoid crediting noise, above zero to require real gain. |
| Minimum MRR improvement | `ΔMRR ≥ +0.100` (baseline 0.106 → ≥ 0.206) | The measured improvement is +0.255; fixed below it. |
| Precision **or** MRR sufficiency | at least one of `Δp ≥ +0.050` or `ΔMRR ≥ +0.100` | Mirrors the plan's "precision@k **and/or** MRR" GO shape as an objective disjunction. |
| Recall non-regression bound | `Δr ≥ −0.020` (retrieval r@5 ≥ baseline 0.278 − 0.020 = 0.258) | Absolute non-regression; the measured delta is +0.370. |
| Context-byte regression bound | `Δbytes ≤ +24` bytes (+10 % of baseline 246 → ≤ 270 bytes) | Bounds growth; the measured delta is −8 bytes. |
| Retrieval-overhead bounds (offline) | `index_build_ms ≤ 256` and `search_ms ≤ 100` (2× the EV-003 measured 128 ms / 50 ms) | 2× headroom over the measured harness values keeps the bound on the measured baseline; not a live-path claim. |

All six values are concrete numbers or inequalities. No placeholder ("TBD") is used.

### 1.3 Objective E1 decision predicates

Evaluated over the EV-002 corpus using the shared `retrievalgate.Measure` and the
EV-003 overhead timings.

- **GO** iff all of:
  1. `Δp ≥ +0.050` **or** `ΔMRR ≥ +0.100`;
  2. `Δr ≥ −0.020`;
  3. `Δbytes ≤ +24`;
  4. `index_build_ms ≤ 256` **and** `search_ms ≤ 100`.
- **HOLD** iff the outcome is mixed or inconclusive: e.g. the §1.3(1) disjunction holds
  on some corpus categories but the aggregate falls below the fixed threshold, **or**
  §1.3(2)–(4) are satisfied while §1.3(1) is below threshold. Action: expand or refine
  the EV-002 corpus and re-run E1. **Do not proceed to E2.**
- **NO-GO** iff any of: `Δp < +0.050` **and** `ΔMRR < +0.100` (no measurable ranking
  improvement); **or** `Δr < −0.020` (recall regression); **or** `Δbytes > +24`
  (context-size regression); **or** `index_build_ms > 256` **or** `search_ms > 100`
  (overhead above the bound). Action: keep the baseline; record; stop.

### 1.4 Deterministic computation statement

The E1 rule is computed **only** from existing `retrievalgate.Measure` outputs
(precision@k, recall@k, MRR, bytes) plus the EV-003 index-build / search timings. It
reuses `retrievalgate.Measure` and `retrievalgate.baselineOrder` verbatim — no second
implementation. Identical inputs therefore yield **identical metric content** (EV-003
§3 establishes re-run identity and order-independence). The E1 rule introduces no new
metric and no new dependency.

## 2. Gate E2 — live A/B: decision rule

E2 is **only** reachable after `E1 = GO` and the prerequisite approvals of §4.

### 2.1 Allowed measured signals (exhaustive)

The E2 rule may use **only** these signals:

| Signal | Source |
| --- | --- |
| task completion / failure rate | `runtrace.Termination.Stage` (EV-004 §1 row 3) |
| first-attempt success | `runtrace.Termination` + `internal/runtrace/trace.go:184` `Verification` |
| iterations / tool-calls | run trace / existing progress signals |
| latency | `internal/perf` stage timings (`metrics.json`) (EV-004 §1 row 4) |
| context size | `runtrace.ContextInfo` — items/files/bytes/truncated/sources (EV-004 §1 row 2) |

**Token consumption is excluded from authority.** Per Phase 8 §26 and EV-004 §4, token
counts are recorded **informationally at most**, when a provider reports them, and
**never** as budget, routing, or acceptance authority. Where no reliable count exists
the value is recorded **`UNAVAILABLE`**, never invented. Token consumption is not an
E2 decision signal.

### 2.2 Objective E2 decision predicates

The minimum-runs and variance-bound values below are **fixed shapes** pending the E2
approval of §4; they are stated as the constants this record fixes so the rule stays
objective.

- **Minimum runs:** `N ≥ 30` per arm (baseline and treatment) over the EV-002 corpus.
- **Variance bound:** a treatment signal is considered stable only if its
  run-to-run spread (standard deviation across `N`) is within `±10 %` of its mean.
- **GO** iff **all** of:
  1. task completion / first-attempt success improves, **and/or** iterations / tool-calls
     decrease;
  2. **no correctness regression**;
  3. **no** lifecycle / approval / retry / replan / budget / verification-semantics change;
  4. **no budget increase**;
  5. **no latency regression** beyond the fixed bound (treatment latency ≤ baseline
     latency × 1.10);
  6. across `N ≥ 30` runs per arm with spread within the variance bound;
  7. the change is **feature-gated and off by default**.
- **HOLD** iff **variance exceeds the pre-fixed bound** (run-to-run spread > `±10 %` of
  mean). Action: add runs; **do not authorize a production change**.
- **NO-GO** iff no improvement, **or** any correctness / latency ( > ×1.10 ) / budget
  regression. Action: keep the baseline; record; stop.

### 2.3 Seam and architecture decision

E2 requires a production seam that **does not exist today**: a `Retrieved` input to
`sopctx.Inputs` / `implementContext` behind an opt-in gate. This record asserts no such
seam exists; it is deferred to the plan's **Unresolved Architecture Decision**:

- **Option A** — authorize a minimal, opt-in, gated retrieval seam under a
  **separate** plan; or
- **Option B** — do not wire retrieval into the live path, in which case **E2 =
  NOT_REQUIRED** and the plan concludes at the E1 gate.

No production change is described or performed here.

## 3. Stop conditions (fail-closed)

Stop and report **HOLD or NO-GO** — **never proceed** — if any of the following holds.
Every condition resolves to HOLD or NO-GO, never to proceed:

1. any correctness, lifecycle, approval, retry, replan, budget, or verification
   semantics would change;
2. any provider/model-specific behavior, or a non-neutral dependency, would be
   introduced;
3. the model-free arm is non-deterministic, or a required metric cannot be measured;
4. a **token count** would be used as budget, routing, or acceptance authority;
5. a required prerequisite approval is absent.

These render fail-closed: the default on any ambiguity is to stop, not to continue.

## 4. Prerequisite approvals required before E2

Each item is enumerated individually with what it gates. **None is requested or
implied by EV-005.**

| # | Prerequisite approval | Who approves | What it gates |
| --- | --- | --- | --- |
| 1 | **E2 authorization** — human architecture decision on the opt-in retrieval seam (§2.3) | Human (architecture owner) | Any E2 work at all; selects Option A or Option B |
| 2 | **Model-budget authorization** — consume model budget by running the corpus through the live `sop run` lifecycle | Human (budget owner) | The live E2 A/B runs |
| 3 | **Spec/trace authorization** — any change to `docs/specs/AGENT-PROVIDER.md` or the run-trace schema | Human (spec owner) | Spec and trace-schema edits E2 would require |
| 4 | **Token-metric authorization** — confirm token consumption is informational-only, never policy | Human (policy owner) | Any token recording at all; keeps tokens out of authority |
| 5 | **Corpus authorization** — the EV-002 corpus and its pinned refs are acceptable for repeated local runs | Human (evaluation owner) | Repeated local E2 runs against the pinned refs |

**E1 requires none of these.** Authoring and running E1 (EV-001…EV-005) needs no
approval because E1 is **model-free and read-only**. Only the E1→E2 transition and any
production change require the approvals above. **EV-005 requests no approval and makes
no production change or plan activation.**

## 5. Decision procedure (ordered, deterministic)

```text
1. Apply the E1 rule (§1) over the EV-002 corpus using retrievalgate.Measure.
2. Branch on the objective E1 outcome:
   - E1 = GO    -> the §4 approvals plus the human architecture decision (§2.3)
                    precede any SEPARATE E2 plan; author no E2 work here.
   - E1 = HOLD  -> expand/refine the EV-002 corpus and re-run E1; never proceed to E2.
   - E1 = NO-GO -> keep the baseline; record; stop.
```

The procedure is ordered and deterministic: the E1 rule is applied first, and no E2
action follows without its enumerated approvals and the explicit human decision.

## 6. Change-scope statement

- **No production change** is made, described, or performed by EV-005.
- **No `.agent-sdlc` state** is created, modified, or deleted.
- The **only** artifact added is `docs/reports/ev-context-run/EV-005-acceptance-criteria.md`.
- Pre-existing user-owned working-tree changes (for example `M docs/specs/AGENT-PROVIDER.md`
  and the untracked EV/CONV reports) are **preserved unchanged**.
- No plan activation is performed.

## 7. Terminology

This record preserves the plan's vocabulary: **E1**, **E2**, **GO / HOLD / NO-GO**,
**retrieval seam**, **EV-002 corpus**, **pinned ref** (`388ea56a…`), and the Phase 8
capability ids **CTX-002** (structural index), **CTX-003** (BM25 retrieval),
**CTX-004** (retrieval evaluation gate).

## 8. Verification record (EV-005 self-verification)

| # | Acceptance criterion | Inspection method | Result |
| --- | --- | --- | --- |
| 1 | E1 rule is objective and uses only precision@k, MRR, recall@k, context bytes, retrieval overhead | Read §1.1–§1.3; confirm the signal list is exhaustive and every threshold is a number/inequality | PASS |
| 2 | E2 rule is objective, names task completion, first-attempt success, iterations/tool-calls, latency, context size, and excludes tokens from authority | Read §2.1–§2.2; confirm the exact five-signal list and the explicit token exclusion | PASS |
| 3 | Stop conditions are stated and fail-closed | Read §3; confirm all five conditions and the "never proceed" resolution | PASS |
| 4 | Prerequisite approvals for E2 are enumerated | Read §4; confirm all five approvals, each with approver and gating scope | PASS |
| 5 | No production change and no plan activation | Read §6; confirm the only artifact is this report and no `.agent-sdlc` state is touched | PASS |
| 6 | Reported anchors/values match EV-002/EV-003/EV-004 | Cross-check §0/§1 baseline numbers (0.067/0.178, 0.278/0.648, 0.106/0.361, 246/238, 128/50 ms) against EV-003 §5–§6; check EV-004 §3–§4 anchors | PASS |

Required validations (`go build ./...`, `go test ./...`, `go vet ./...`) are reported
as run clean; this task adds only a Markdown report, so the working tree shows only the
expected report artifact on top of the preserved pre-existing user-owned changes.
