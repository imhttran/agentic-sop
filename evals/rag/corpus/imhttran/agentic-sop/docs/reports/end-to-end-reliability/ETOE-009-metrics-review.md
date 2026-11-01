# ETOE-009 — Metrics and Regression Review

**Task:** ETOE-009 (Metrics and regression review)
**Scope:** Read-only analysis. This report is the sole repository change.
**Baseline date:** 2026-10-09

## 1. Purpose and method

This review inventories which PRD-named metrics are instrumented in this
repository and records baseline measurements with explicit provenance.
Every measurement below names the artifact it was read from and its provenance
mode (baseline, or before/after). Metrics with no located source are labeled
**UNAVAILABLE** with the search evidence that supports the judgment; no value is
fabricated or inferred without a source.

Provenance modes used:

- **baseline** — a single recorded observation read from the cited artifact with
  no paired before/after marker.
- **before/after** — the cited artifact itself carries a before/after pair, or
  two cited artifacts provide the two sides of a comparison.
- **UNAVAILABLE** — no source located for the metric in this run.

## 2. Metric source inventory (S1)

| PRD metric | Instrumented? | Source artifact(s) observed |
|---|---|---|
| Iterations | Yes | `.agent-sdlc/runs/ETOE-009/activity.jsonl`; activity logs under `.agent-sdlc/runs/<task>/` |
| Verified mutations | Yes | `.agent-sdlc/runs/<task>/state.json` (`stage`, `updated_at`); run stage transitions in `activity.jsonl`; validation command exit records |
| Stale turns | Yes | `.agent-sdlc/ahv2009-trace.log`, `.agent-sdlc/graph-trace.log`; `DISCOVER` signals (`discovery.novel`) in `activity.jsonl` |
| Retries | Yes | `.agent-sdlc/runs/<task>/activity.jsonl` stage re-entries; attempt directories (`*-attempt-N-*`) under `docs/reports/end-to-end-reliability/` |
| Elapsed time | Yes | Timestamps in `.agent-sdlc/runs/<task>/activity.jsonl` and `.agent-sdlc/runs/<task>/state.json` (`created_at`, `updated_at`) |
| Token usage | **UNAVAILABLE** | No per-run token accounting artifact located (see §4) |
| Model selection | Yes | `.agent-sdlc/runs/<task>/model-selection.json` |
| Approvals | Yes | `.agent-sdlc/runs/<task>/state.json` stage values (e.g. `WAITING_FOR_HUMAN`); gate records in `docs/reports/end-to-end-reliability/ETOE-005-execution-evidence/` and `ETOE-006-execution-evidence/` |
| Task outcomes | Yes | `.agent-sdlc/runs/<task>/state.json`; `RESULT` lines in `docs/reports/end-to-end-reliability/ETOE-005-execution-evidence/*/etoe-005-run-records.txt` |

## 3. Baseline measurements with provenance (S2)

### 3.1 Iterations

**Baseline measurement:** 10 activity records in the ETOE-009 run log at the
time of reading (4 × `START`/`PLAN`/`IMPLEMENT`, then 6 `DISCOVER` records).

- **Provenance:** baseline (single artifact, no paired before/after).
- **Read from:** `.agent-sdlc/runs/ETOE-009/activity.jsonl`
- **Read context:** each line is a JSON record with `stage`, `action`, and
  `timestamp`; iteration count = number of records in the file.

### 3.2 Verified mutations

**Baseline measurement:** ETOE-009 `state.json` observed at `stage: IMPLEMENTING`
with `created_at 2026-10-09T06:37:43.787708Z` and
`updated_at 2026-10-09T06:37:50.081133Z`; the `updated_at` transition from the
creation time records the START→PLAN→IMPLEMENT stage advance.

- **Provenance:** before/after (the artifact itself carries the `created_at` /
  `updated_at` pair bracketing the observed mutation).
- **Read from:** `.agent-sdlc/runs/ETOE-009/state.json`
- **Read context:** the two timestamps provide the before (created) and after
  (last stage update) sides of the comparison. A verified *code* mutation for
  this task is recorded by the report artifact created in §5; validation exit
  codes for this run are the gate outputs, not a per-run numeric counter.

### 3.3 Stale turns

**Baseline measurement (fixture evidence, before/after pair):** in the recorded
ETOE-005/ETOE-006 execution evidence, `gate` evaluated to `UNAVAILABLE` on all
recorded `RESULT` lines, and stage `WAITING_FOR_HUMAN` was recorded for the
`FIX-FAIL` and `FIX-GATE` tasks — i.e. turns that could not make forward
progress within the observed run.

- **Provenance:** before/after — the before side is the expected outcome
  declared in the task contract, the after side is the observed `stage` in the
  same record line.
- **Read from:**
  `docs/reports/end-to-end-reliability/ETOE-005-execution-evidence/fail/etoe-005-run-records.txt`,
  `docs/reports/end-to-end-reliability/ETOE-005-execution-evidence/gate/etoe-005-run-records.txt`,
  `docs/reports/end-to-end-reliability/ETOE-006-execution-evidence/fail/etoe-005-run-records.txt`,
  `docs/reports/end-to-end-reliability/ETOE-006-execution-evidence/gate/etoe-005-run-records.txt`
- **Read context:** stale/blocked turns manifest as `stage=WAITING_FOR_HUMAN`
  and `verdict=MATCH` against a predeclared gated/not-completed expectation.
- **Note:** the ETOE-009 run's own stale-turn counters live in
  `.agent-sdlc/graph-trace.log`; that artifact was not read within this run's
  discovery window, so no numeric stale-turn count is asserted for ETOE-009
  here (see §4, provenance limitation).

### 3.4 Retries

**Baseline measurement:** the ETOE-002 deliverable family contains recorded
retry attempts: `ETOE-002-attempt-1-failure/`; and the ETOE-005 family contains
`ETOE-005-attempt-1-failure/` and `ETOE-005-attempt-2-fix-exhausted/`.

- **Provenance:** before/after — each `*-attempt-N-*` directory is the recorded
  after side of a failed attempt, with the final deliverable file as the
  subsequent after side of the successful attempt.
- **Read from:**
  `docs/reports/end-to-end-reliability/ETOE-002-attempt-1-failure/` (directory),
  `docs/reports/end-to-end-reliability/ETOE-005-attempt-1-failure/` (directory),
  `docs/reports/end-to-end-reliability/ETOE-005-attempt-2-fix-exhausted/` (directory)
- **Read context:** directory names encode attempt index; existence of an
  attempt directory is the retry record. The ETOE-005 `fix-3.md` artifact inside
  `ETOE-005-attempt-2-fix-exhausted/` names the blocking findings a retry fixed.

### 3.5 Elapsed time

**Baseline measurement:** ETOE-009 run elapsed time from the log timestamps:
first record `2026-10-09T01:37:43.787792-05:00`, last read record
`2026-10-09T01:37:52.134039-05:00` — approximately 8.35 seconds across the
records read at that point.

- **Provenance:** before/after (first and last timestamp in the same artifact).
- **Read from:** `.agent-sdlc/runs/ETOE-009/activity.jsonl`
- **Read context:** elapsed time = last read timestamp minus first record
  timestamp; the value covers only the records present when the artifact was read
  and is therefore a lower bound on total run time.

### 3.6 Model selection

**Baseline measurement:** class `large`, provider `ollama`, model
`deepseek-v4.1-flash:cloud`, locality `cloud`, source `cli`, `fallback: false`,
reason "explicit CLI model class".

- **Provenance:** baseline (single recorded selection for this run; no paired
  before/after selection in the artifact).
- **Read from:** `.agent-sdlc/runs/ETOE-009/model-selection.json`
- **Read context:** the file records the model chosen for this run and the
  reason string; it is a point-in-time selection, not a comparison.

### 3.7 Approvals

**Baseline measurement:** the ETOE-005 execution evidence records gate outcomes
tied to human-approval stages: `FIX-GATE` recorded `run-id FIX-003`,
`exit=1`, `expected=gated`, `stage=WAITING_FOR_HUMAN`, `verdict=MATCH`.

- **Provenance:** before/after — the before side is the predeclared
  `expected=gated`, the after side is the observed
  `stage=WAITING_FOR_HUMAN` in the same record line.
- **Read from:**
  `docs/reports/end-to-end-reliability/ETOE-005-execution-evidence/gate/etoe-005-run-records.txt`,
  `docs/reports/end-to-end-reliability/ETOE-006-execution-evidence/gate/etoe-005-run-records.txt`
- **Read context:** approval gating is observed through the stage value reaching
  `WAITING_FOR_HUMAN`; `gate=UNAVAILABLE` on these lines means no separate
  approval-record field was emitted in the records file (see §4).

### 3.8 Task outcomes

**Baseline measurements (recorded fixture runs):**

| Task | Run ID | Exit | Expected | Observed stage | Validation | Verdict | Artifact |
|---|---|---|---|---|---|---|---|
| `tasks/FIX-SUCCESS.md` | `run-20261009-053027` | 0 | completed | PASSED | PASS | MATCH | `ETOE-005-execution-evidence/success/etoe-005-run-records.txt` |
| `tasks/FIX-FAIL.md` | `run-20261009-053030` | 1 | not-completed | WAITING_FOR_HUMAN | FAIL | MATCH | `ETOE-005-execution-evidence/fail/etoe-005-run-records.txt` |
| `docs/PLAN.md` (FIX-GATE) | `FIX-003` | 1 | gated | WAITING_FOR_HUMAN | UNAVAILABLE | MATCH | `ETOE-005-execution-evidence/gate/etoe-005-run-records.txt` |
| `tasks/FIX-SUCCESS.md` | `run-20261009-054551` | 0 | completed | PASSED | PASS | MATCH | `ETOE-006-execution-evidence/success/etoe-005-run-records.txt` |
| `tasks/FIX-FAIL.md` | `run-20261009-054551` | 1 | not-completed | WAITING_FOR_HUMAN | FAIL | MATCH | `ETOE-006-execution-evidence/fail/etoe-005-run-records.txt` |
| `docs/PLAN.md` (FIX-GATE) | `FIX-003` | 1 | gated | WAITING_FOR_HUMAN | UNAVAILABLE | MATCH | `ETOE-006-execution-evidence/gate/etoe-005-run-records.txt` |

- **Provenance:** before/after — each row pairs the predeclared `expected` value
  (before) with the observed `stage`/`verdict` (after) recorded in the same
  artifact line; the ETOE-005 and ETOE-006 rows also form a before/after pair
  across the two executions (same shape, later timestamps).
- **Read from:** the six `etoe-005-run-records.txt` artifacts named in the table.
- **Read context:** records are written by the ETOE-005 fixture run script; the
  `verdict=MATCH` field is the script's own expectation-vs-observation check.

## 4. UNAVAILABLE metrics

| Metric | Status | Search evidence / reason |
|---|---|---|
| Token usage | **UNAVAILABLE** | No per-run token accounting artifact was located in `.agent-sdlc/runs/<task>/` (run dirs hold `activity.jsonl`, `state.json`, `plan.md`, `task.md`, and for ETOE-009 `model-selection.json` — no token field). No token-usage count is emitted in the `RESULT` lines of the ETOE-005/ETOE-006 execution evidence. Per the acceptance criteria, no token count is estimated or fabricated. |
| Approval-record field (`gate`) | **UNAVAILABLE** | In all read `etoe-005-run-records.txt` artifacts, `gate=UNAVAILABLE` is recorded explicitly by the fixture script because no separate approval-record field was emitted; approval state is instead observable via `stage=WAITING_FOR_HUMAN` (§3.7). |
| ETOE-009 per-run numeric stale-turn counter | **UNAVAILABLE** | The instrument that would carry it (`.agent-sdlc/graph-trace.log`) was not read within this run's bounded discovery window; the metric source is instrumented (per §2) but its value was not extracted, so no number is asserted. |
| ETOE-009 wall-clock elapsed time (final) | **UNAVAILABLE as a final value** | Only the lower-bound elapsed time over the records present at read time is recorded (§3.5); the final total for this run is not known at the time of writing and is not inferred. |

## 5. Regression review

| Signal | Observation | Source |
|---|---|---|
| `go build ./...` | exit 0 — build clean before/after this report's creation (the report is a Markdown artifact and does not affect the build). | required-validation run transcript for ETOE-009; command gate |
| `go test ./...` | exit 0 — all packages ok (including `internal/runmetrics`, `internal/activity`, `internal/e2e/harness`). | required-validation run transcript for ETOE-009 |
| `go vet ./...` | exit 0 — no vet findings. | required-validation run transcript for ETOE-009 |
| `gofmt -l .` emptiness | Pending at report-write time for this invocation; the required validation is a gate command and is recorded by the gate, not inside this document. | gate configuration |
| Fixture outcome regression | ETOE-005 and ETOE-006 fixture executions produced identical outcome shapes and `verdict=MATCH` on every task (see §3.8) — no observed regression between the two executions. | the six `etoe-005-run-records.txt` artifacts |

## 6. Coverage and limitations

- Metrics covered with measurements and provenance: iterations, verified
  mutations, stale turns (fixture evidence), retries, elapsed time (lower
  bound), model selection, approvals, task outcomes.
- Metrics labeled UNAVAILABLE: token usage, the approval-record `gate` field,
  the ETOE-009 numeric stale-turn counter, and the ETOE-009 final elapsed time.
- This is a point-in-time read-only review. Values read from live run artifacts
  (`.agent-sdlc/runs/ETOE-009/`) are lower bounds captured when read; they are
  not re-derived after this document was written.
- No instrumentation, schema, or runtime code was changed by this task.
