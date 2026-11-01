# ETOE-010 — Final end-to-end reliability report and prioritized backlog

**Task:** ETOE-010 (Final reliability report and prioritized backlog)
**Scope:** Read-only synthesis. This report is the sole repository change intended by this task.
**Baseline date:** 2026-10-09
**Status:** HOLD — see §1.

> **Method note.** This report consolidates the *already-recorded* evidence from
> ETOE-001…ETOE-009 (preserved under `docs/reports/end-to-end-reliability/`) and derives a
> single GO/HOLD verdict strictly from that evidence. It executes **no** new scenario, does
> **not** re-derive any recorded status, and performs **no** closure. Every claim below
> cites a source artifact and section; no value is fabricated for any UNAVAILABLE item.

---

## 1. GO/HOLD verdict

> ### **HOLD**
>
> The end-to-end reliability series **cannot be declared GO**. Closure is **withheld**.
> The governed SOP-side lifecycle demonstrates deterministic, reproducible behavior
> end to end (ETOE-005 §4–§6, ETOE-006 §3–§7), but a material portion of the ETOE series'
> acceptance scope is **BLOCKED/UNAVAILABLE** because the `sop-controller` sibling checkout
> and its runtime were unreachable from the governed tool surface, and live-model/provider
> behavior was never exercised (ETOE-007 §1/§3/§9; ETOE-004 §3.1; ETOE-008 §3).

**The HOLD is backed by the recorded evidence as follows:**

- **SOP-side lifecycle: MET.** Three scenarios executed end to end in isolated disposable
  projects with concrete run IDs and MATCH verdicts: success `run-20261009-053027`
  (`stage=PASSED`, `validation=PASS`, exit 0), intentional-failure `run-20261009-053030`
  (`stage=WAITING_FOR_HUMAN`, `validation=FAIL`, `AUTO_FIX_EXHAUSTED` after FIX×3, exit 1),
  and the human-gated refusal `FIX-003` (`status=PENDING`, `sop approve` never invoked)
  (ETOE-005 §4–§5; ETOE-006 §3–§7).
- **Controller-side parity: BLOCKED/UNAVAILABLE.** 0 of 12 planned controller parity tests
  (PT-1…PT-12) were executed; all twelve are recorded BLOCKED/UNAVAILABLE with the failing
  reachability probe (ETOE-007 §1/§3). The sibling checkout is outside the authorized
  repository root and is not covered by this run's tool surface.
- **Decision-adapter integration: UNAVAILABLE.** The process-level `command` seam exists in
  SOP's config schema and validation, and a composition-root factory now exists, but the
  seam is unconfigured and **no verified SOP-to-adapter dispatch** was observed (ETOE-004 §3;
  ETOE-008 §3/§3.1).
- **Live model/provider: UNAVAILABLE.** No live `ollama`/remote model service was contacted;
  evidence rests on the deterministic controlled provider only (ETOE-005 header; ETOE-006
  §9.1; ETOE-008 §6).

A GO would require the controller-dependent and live-provider scope to be verified. It was
not. The verdict is therefore **HOLD**, with the BLOCKED/UNAVAILABLE items enumerated
implicitly in §2.

---

## 2. Explicit BLOCKED/UNAVAILABLE list (consolidated)

Every BLOCKED/UNAVAILABLE item recorded across ETOE-001…ETOE-009, with the source section
that recorded it and its stated reason. None is silently dropped.

### 2.1 Controller-dependent items (ETOE-003 C-G1…C-G7; carried forward by ETOE-007 §9)

| ID | Item | Status | Source | Reason (as recorded) |
|----|------|--------|--------|----------------------|
| C-G1 | The four sop-controller contract documents (`docs/architecture/SOP-BOUNDARY.md`, `docs/reference/CLI.md`, `docs/specs/WORKFLOW.md`, `docs/specs/HUMAN-APPROVAL.md` in the sibling repo) | **UNAVAILABLE** | ETOE-003 §5 C-G1; ETOE-007 §9 | Sibling checkout not an authorized tool root; all three probe surfaces rejected (ETOE-003 §2.1; ETOE-007 §1) |
| C-G2 | Approval-action asymmetry (whether the dashboard exposes approve/decline) | **UNAVAILABLE** | ETOE-003 §5 C-G2; ETOE-007 §9 | No approval button documented on the dashboard surface; controller unreachable to confirm presence/absence |
| C-G3 | Pending-approval visibility on the dashboard | **UNAVAILABLE** | ETOE-003 §5 C-G3; ETOE-007 §9 | No approvals view/field in the documented view table; dashboard not rendered |
| C-G4 | Runtime display/action readiness (live build/serve, `/healthz`, HTMX cadence, view/action parity) | **BLOCKED** | ETOE-003 §5 C-G4; ETOE-007 §3/§9 | Controller not built/served; PT-1/PT-5/PT-6 not executable |
| C-G5 | Live network/auth enforcement (loopback rewrite, token gate, CSRF 401/403, `/healthz` exemption) | **BLOCKED** | ETOE-003 §5 C-G5; ETOE-007 §9 | No isolated-network server; PT-2/PT-3/PT-4/PT-12 not executable |
| C-G6 | `gate.json` single-producer (no covering unit test) and live `gate.json` vs `sop approvals --json` parity | **UNAVAILABLE** | ETOE-002 §3 G1; ETOE-003 §5 C-G6; ETOE-007 §9 | PT-11 requires the controller-driven parked run; only the SOP-side approvals listing is preserved |
| C-G7 | Code-level second-authority enforcement (no direct state write path; documented refusals) | **UNAVAILABLE** | ETOE-003 §5 C-G7; ETOE-007 §9 | Sibling code unreachable; refusal checks PT-6/PT-9 not executable |

### 2.2 Controller parity test items (ETOE-007 §3)

| Items | Status | Reason |
|-------|--------|--------|
| PT-1 … PT-12 (all 12) | **0 executed; BLOCKED/UNAVAILABLE** | Controller process could not be built, served or observed: sibling unreachable (ETOE-007 §1). No PT item is asserted as pass. |

(PT-1…PT-4 recorded **BLOCKED**; PT-5…PT-12 recorded **UNAVAILABLE**; ETOE-007 §3.)

### 2.3 Decision-adapter integration

| Item | Status | Source | Reason |
|------|--------|--------|--------|
| Real SOP-to-adapter dispatch for `decision.provider: command` | **UNAVAILABLE** | ETOE-004 §3.1; ETOE-008 §3 | Seam not configured (default `deterministic`, `decision.enabled: false`); no run trace/log/artifact shows a dispatch |

### 2.4 Metrics (ETOE-009 §4)

| Metric | Status | Source | Reason |
|--------|--------|--------|--------|
| Token usage | **UNAVAILABLE** | ETOE-009 §4 | No per-run token accounting artifact in `.agent-sdlc/runs/<task>/`; no token field in `RESULT` lines. Not estimated or fabricated |
| Approval-record field (`gate`) | **UNAVAILABLE** | ETOE-009 §4 | Fixture script emitted no separate approval-record field; `gate=UNAVAILABLE` on all read records. Approval state instead observable via `stage=WAITING_FOR_HUMAN` |
| ETOE-009 per-run numeric stale-turn counter | **UNAVAILABLE** | ETOE-009 §4 | Instrument (`.agent-sdlc/graph-trace.log`) not read within the bounded discovery window; no number asserted |
| ETOE-009 final wall-clock elapsed time | **UNAVAILABLE (as a final value)** | ETOE-009 §4 | Only a lower-bound over records present at read time is recorded; the final total is not known and not inferred |

### 2.5 Recorded discrepancy (not smoothed over)

| Item | Status | Source | Reason |
|------|--------|--------|--------|
| ETOE-008 §3.1 divergence from ETOE-004 | **RECORDED FINDING** | ETOE-008 §3.1 | ETOE-004 §4 stated "no SOP call site constructs it"; the current tree wires a composition-root factory (`internal/cli/cli.go:177` → `internal/cli/decision_provider.go`). Integration status is unchanged (UNAVAILABLE); the *reason* narrows from "no call site" to "call site exists, seam unconfigured, no verified dispatch" |

### 2.6 Live-provider / runtime limitation

| Item | Status | Source | Reason |
|------|--------|--------|--------|
| Live external model / provider runtime (ollama or remote model service) behavior and metrics | **UNAVAILABLE** | ETOE-005 header/§8.1; ETOE-006 §1/§9.1; ETOE-008 §6 | No live model service contacted; only the deterministic controlled provider was used. Real-model behavior is out of scope of the recorded evidence |

### 2.7 Verification-command gaps (plan-required, not gate-enforced)

| Command | Status | Source | Reason |
|---------|--------|--------|--------|
| `go test -race -count=1 ./...` | **UNAVAILABLE** | ETOE-003 §7; ETOE-007 §11 | Not admitted by the runs' command allow-lists |
| `git diff --check` | **UNAVAILABLE** | ETOE-003 §7; ETOE-007 §11 | Same; additive-only change instead attested by the mutation statements |

---

## 3. Consolidated evidence table (ETOE-001…ETOE-009)

Per-finding status with source artifact+section and the exact run ID / artifact path.

| Source | Finding | Status | Evidence (run ID / path / section) |
|--------|---------|--------|------------------------------------|
| ETOE-001 | Preflight baseline: three repositories' revisions/worktrees; installed-vs-source binary delta; active/archived plan status; contract paths named | **MET** (operator-supplied, outside harness) | ETOE-001 §1/§3/§4/§5; operator-verified HEADs |
| ETOE-002 | Core workflow contract matrix: transition table, validation/review/FIX/retry, approval-refusal paths, closure, trace/metrics — unit coverage with G1/G4/G5/G6 gaps | **MET (unit)** with gaps | ETOE-002 §2.1–§2.5, §3 (G1–G6) |
| ETOE-003 | Controller contract matrix: documented SOP-side controller integration; second-authority analysis; gaps C-G1…C-G7 | **DOC-level MET**; controller code UNAVAILABLE | ETOE-003 §3–§5; probes §2.1 |
| ETOE-004 | Decision-adapter integration matrix: seam existence vs actual wiring | **UNAVAILABLE** for integration | ETOE-004 §3.1, §4 |
| ETOE-005 | Disposable fixture baseline: 3 scenarios executed, MATCH on each | **MET** | ETOE-005 §4–§6: `run-20261009-053027`, `run-20261009-053030`, `FIX-003`; `ETOE-005-execution-evidence/` |
| ETOE-006 | Core end-to-end success/failure/recovery + human-gated refusal | **MET** | ETOE-006 §3–§7: `run-20261009-053027` (PASSED/PASS), `run-20261009-053030` (WAITING_FOR_HUMAN/FAIL, FIX×3), `FIX-003` (PENDING) |
| ETOE-007 | Controller parity: SOP-side half verified; controller side unreachable | **BLOCKED/UNAVAILABLE** | ETOE-007 §1/§3/§9: 0/12 PT items; C-G1…C-G7 carried |
| ETOE-008 | Adapter integration or documented absence: isolated policy/failure exercises MET; integration UNAVAILABLE | **UNAVAILABLE** (integration); isolated MET | ETOE-008 §3/§5/§6/§3.1 |
| ETOE-009 | Metrics/regression review: measured metrics with provenance; UNAVAILABLE metrics | **MET with UNAVAILABLE items** | ETOE-009 §3–§5; §4 UNAVAILABLE set |

Every ETOE-002…ETOE-009 report is represented above by at least one finding or an explicit
integration-status note. No standalone finding exists for ETOE-001 beyond the preflight
facts (recorded above).

---

## 4. Prioritized backlog (P0/P1/P2) with owners and scoped follow-ups

Each entry states reproducibility with the retained evidence/reproduction reference and is
labeled P0, P1, or P2. Any item lacking a reproduction path is labeled **UNAVAILABLE** rather
than presented as a defect.

### P0 — Blocks a GO / closure

| # | Failure | Reproducible? | Reproduction reference | Owner | Scoped follow-up |
|---|---------|---------------|------------------------|-------|------------------|
| P0-1 | Controller parity unverified: 0/12 PT items executed; display/action/authz/second-authority parity unknown (C-G1, C-G4, C-G5, C-G7) | **Reproducible by re-probe** | Probe recorded in ETOE-007 §1 (same three-surface rejection as ETOE-003 §2.1); PT plan in ETOE-003 §6 | sop-controller + a tool surface authorized for the sibling checkout + controller runtime | Provide an authorized tool surface for the sibling checkout and a disposable controller runtime; execute PT-1…PT-12 (ETOE-003 §6) against the ETOE-005 disposable project |
| P0-2 | Live provider/model behavior never exercised (controlled provider only) | **Reproducible by re-run** | ETOE-005 header/§8.1; ETOE-006 §9.1; ETOE-008 §6 | Provider runtime owner / operator (a live model service) | Stand up a live `ollama`/remote provider and re-run the ETOE-005/006 scenarios against it; record live-model metrics |

### P1 — High-severity gaps that degrade trust but do not by themselves block a GO decision

| # | Failure | Reproducible? | Reproduction reference | Owner | Scoped follow-up |
|---|---------|---------------|------------------------|-------|------------------|
| P1-1 | gate.json emitted at a single producer site with **no covering unit test**; consumer gate parity unverified (C-G6) | **Reproducible** | ETOE-002 §3 G1 (producer `internal/cli/run.go:868`); PT-11 (ETOE-003 §6) | run/gate producer layer (`internal/cli` run loop) | Add a unit/artifact-shape test for gate.json emission; then close the live parity check via PT-11 |
| P1-2 | Decision-adapter integration UNAVAILABLE: seam unconfigured; no verified dispatch (also the ETOE-004↔ETOE-008 discrepancy) | **Reproducible** | ETOE-004 §3.1; ETOE-008 §3/§3.1 (`internal/cli/cli.go:177` → `internal/cli/decision_provider.go`) | Decision seam layer (`internal/cli`, `internal/config`, `internal/decision`) | Configure `decision.provider: command` + `decision.command` in a disposable project and produce a verified SOP-to-adapter dispatch trace; reconcile ETOE-004 wording |
| P1-3 | Approval-action presence/phone-safety and pending-approval dashboard visibility unverified (C-G2, C-G3) | **Reproducible by re-probe** | ETOE-003 §5 C-G2/C-G3; ETOE-007 §4/§9 (PT-7/PT-8) | sop-controller UI layer + operator | Execute PT-7/PT-8 in the disposable environment; confirm any approval route is POST+CSRF+token gated |
| P1-4 | metrics.json is deliberately unwired from production; live emission unverified (G2; C-G4 context) | **Reproducible** | ETOE-002 §2.5 T4/T5, §3 G2 (`internal/runmetrics/unwired_test.go`) | metrics producer wiring (`internal/runmetrics` + run loop) | Wire metrics.json emission in a coding-stage task; verify live aggregate shape against the goldens |

### P2 — Lower-severity / informational gaps and instrumentation follow-ups

| # | Item | Reproducible? | Reproduction reference | Owner | Scoped follow-up |
|---|------|---------------|------------------------|-------|------------------|
| P2-1 | Remote PR/CI/merge lifecycle (PR_OPEN→…→DONE) not executed live (G3) | **Reproducible with CI env** | ETOE-002 §2.1 A14–A18, §3 G3 | remote lifecycle driver + CI environment | Run the remote lifecycle against a disposable repo with real PR/CI/merge |
| P2-2 | `IsBlockedRecoverable`/`IsTerminalBlocked` guard for `BLOCKED(CONTINUATION_EXHAUSTED)` has no covering test (G5) | **Reproducible** | ETOE-002 §2.1 A23, §3 G5 | core lifecycle (`internal/domain`) + `sop retry` path | Add a focused unit test for the terminal-blocked guard |
| P2-3 | Live-run closure dispositions (archive COMPLETE / failures closed) have no covering unit test (G6) | **Reproducible** | ETOE-002 §2.4 C4, §3 G6 | CLI closure path (`internal/cli`, `internal/planflow`) | Cover live-run closure dispositions in the disposable fixture |
| P2-4 | Token usage not instrumented | **UNAVAILABLE** (no source) | ETOE-009 §2/§4 | orchestrator instrumentation | Add per-run token accounting; then record as a measured metric (do not estimate) |
| P2-5 | Approval-record `gate` field not emitted in fixture records | **UNAVAILABLE** (no source) | ETOE-009 §4 | fixture run-script instrumentation | Emit a structured approval-record field; keep `stage=WAITING_FOR_HUMAN` as the observable proxy meanwhile |
| P2-6 | ETOE-009 numeric stale-turn counter and final wall-clock elapsed time | **UNAVAILABLE** (no source) | ETOE-009 §4 | orchestrator instrumentation / run artifacts | Read `.agent-sdlc/graph-trace.log` and final `activity.jsonl` within the run window; record numeric values |
| P2-7 | Plan-required `go test -race -count=1 ./...` and `git diff --check` not admitted by the audit runs' allow-lists | **Reproducible where the commands are admitted** | ETOE-003 §7; ETOE-007 §11 | CI / gate owner | Run race and diff-check in a stage whose command allow-list admits them |

**Reproducibility rule applied:** the P0/P1/P2 defects above all cite either (a) the exact
reproduction procedure and preserved evidence in ETOE-005 §3/§5 (e.g. `scripts/etoe-005-fixture-setup.sh`,
`etoe-005-fixture-run.sh`, per-scenario run artifacts incl. `validation.json` with `ExitCode 1`
and `broken_test.go:7`), or (b) a re-runnable probe/test. Items without such provenance
(P2-4…P2-6) are labeled **UNAVAILABLE**, not presented as defects.

---

## 5. Dependency graph

Edges are drawn only from relationships evidenced in the assembled artifacts. Arrows point
from a dependency to what depends on it.

```text
ETOE-001 (preflight baseline)
   │  provides: revisions, fixture dir, plan status, contract paths
   ├──► ETOE-002 (workflow contract matrix; unit coverage, gaps G1–G6)
   ├──► ETOE-003 (controller contract matrix; gaps C-G1..C-G7)
   │        │
   │        ├──► ETOE-007 (controller parity)  ── BLOCKED: sibling checkout unreachable
   │        │        └── carries C-G1..C-G7 forward (no gap closed)
   │        │
   │        └──► ETOE-008 (adapter evidence) ── integration UNAVAILABLE
   │                 └── §3.1 divergence recorded vs ETOE-004
   │
   ├──► ETOE-004 (adapter integration matrix) ── integration UNAVAILABLE
   │
   └──► ETOE-005 (disposable fixture baseline)  ── MET
            │  provides: run-20261009-053027, run-20261009-053030, FIX-003,
            │            preserved evidence under ETOE-005-execution-evidence/
            ├──► ETOE-006 (core e2e success/failure/gate) ── MET
            │        └── provides the pending FIX-003 gate reused by ETOE-007
            └──► ETOE-009 (metrics/regression review) ── MET with UNAVAILABLE items
                     │
                     └──► ETOE-010 (this report) ── HOLD

Gap dependency edges (evidence-recorded):
   C-G1 ─► (blocks) C-G3, C-G7   (controller docs unread blocks visibility + code-level checks)
   C-G2 ◄─ related ─► C-G3       (approval-action presence and dashboard visibility are the two
                                  sides of the same undocumented approval surface)
   C-G6 depends on ETOE-002-G1 (gate.json single producer) and on a controller-driven parked run (PT-11)
   P0-1 depends on C-G1, C-G4, C-G5, C-G7
   P1-1 depends on ETOE-002-G1 (gate.json producer)
   P1-2 depends on ETOE-004 §3.1 / ETOE-008 §3.1
```

**Consistency:** the graph uses only edges evidenced in ETOE-001 §5, ETOE-002 §3, ETOE-003
§4–§6, ETOE-005 §4–§5, ETOE-006 §3–§5, ETOE-007 §9/§12, ETOE-008 §3.1, and ETOE-009 §3–§5.

---

## 6. Closure withheld

> **Closure is NOT performed by this task and is NOT authorized.**
>
> This report states a **HOLD** verdict and a prioritized backlog. It does **not** mark the
> ETOE series done, resolve any gate, approve anything, or mutate SOP state
> (`.agent-sdlc/`), configuration, or orchestrator source. Closure remains contingent on
> **all gates passing** and **operator authorization**, neither of which is recorded
> (ETOE-006 §5; ETOE-007 §2.4 — the FIX-003 gate remains `status=PENDING`; `sop approve`
> was never invoked).

---

## 7. Acceptance-criteria mapping

| # | Acceptance criterion | Report section | Status |
|---|----------------------|----------------|--------|
| 1 | GO/HOLD verdict backed by recorded evidence, BLOCKED/UNAVAILABLE listed explicitly | §1, §2 | **MET** (HOLD, with §2 enumeration) |
| 2 | Every discovered failure reproducible and prioritized P0/P1/P2 with owner + scoped follow-up | §4 | **MET** (reproducible entries cite evidence; non-reproducible labeled UNAVAILABLE) |
| 3 | Closure not performed unless all gates pass and operator authorizes | §6 | **MET** (withheld; no closure) |
| — | Dependency graph | §5 | **MET** |
| — | BLOCKED/UNAVAILABLE explicit (incl. C-G1…C-G7 and ETOE-009 UNAVAILABLE metrics) | §2.1, §2.4 | **MET** |

---

## 8. Mutation statement

- The **only** repository change made by ETOE-010 is this report file,
  `docs/reports/end-to-end-reliability/ETOE-010-final-reliability-report.md`.
- No orchestrator source (`cmd/sop`, `internal/`), configuration, or CLI behavior was
  changed. No SOP state (`.agent-sdlc/`) was edited by hand.
- This task executed no new scenario; it consolidates recorded evidence only. Pre-existing
  worktree entries are user-owned and were left untouched.
- Closure is explicitly withheld (§6).

---

## 9. Sources consolidated

`ETOE-001-preflight-baseline.md`; `ETOE-002-workflow-contract-matrix.md`; `ETOE-003-controller-contract-matrix.md`;
`ETOE-004-adapter-integration-matrix.md`; `ETOE-005-fixture-baseline.md` (+ `ETOE-005-execution-evidence/`);
`ETOE-006-e2e-evidence.md`; `ETOE-007-controller-parity.md`; `ETOE-008-adapter-evidence.md`;
`ETOE-009-metrics-review.md` — all under `docs/reports/end-to-end-reliability/`.
